# 2026-10-16 — P3: linear tree build, wider lock, graceful shutdown, compressed mermaid

**Symptom / goal.** The polish-and-efficiency batch from a whole-code review of
v0.4.0: a quadratic tree rebuild, a cache lock held across it, a shutdown that
drops requests, an uncompressed 5.4 MB asset, an O(n) TOC highlight, a doc
comment on the wrong function, and a CI-referenced doc that did not exist.

**Diagnosis.**

*Quadratic rebuild.* `insertIntoTree` located each path segment by scanning the
parent's children:

```go
for j := range current.Children {
	if current.Children[j].Name == seg {
		current = &current.Children[j]
		found = true
		break
	}
}
```

The walk calls it once per markdown file and per directory. Every call therefore
cost one comparison per sibling already present, so a rebuild was O(files ×
width). `BenchmarkTreeBuildWideDir` — 2,000 files directly in one folder — was
added first and measured **6.19 ms/op**, against 5.55 ms for the narrow 200-dir
tree that already existed. (The `urlPath` parameter was also dead: the function
computed each node's path from `segments` and never read it.)

*Lock held across the rebuild.* `treeJSONCached` took `treeMu` and `defer`red the
unlock before the walk, so the slowest path in the server ran with the lock held
and every concurrent tree request queued behind it — on a 200-dir vault that is
5.5 ms of serialisation per rebuild, and the rebuild happens whenever the 5s TTL
has expired.

*Abrupt shutdown.* `main` ended in `log.Fatal(srv.Serve(listener))`. A signal
killed the process; a request being served when it arrived was dropped rather
than finished.

*Uncompressed asset.* `serveAsset` wrote the embedded bytes as they are.
`static/js/mermaid.min.js` is 5.4 MB of minified JavaScript and is fetched in
full by any page containing a diagram.

*O(n) TOC highlight.* The IntersectionObserver callback cleared the active class
from every link in the outline and then found the matching link with
`links.filter(...)[0]`, on every intersection.

*Misdirected doc comment.* The block describing `parentPath` (`path.Dir("/sub")
== "/"` and why `"//"` must be avoided) sat above `dirCurrentPath`, which left
`parentPath` undocumented and made `dirCurrentPath`'s doc describe the wrong
function.

*Missing referenced doc.* `.github/workflows/sbom.yml` said "See
docs/security.md for the env-in/exit-code-out contract". There is no `docs/`
directory and `git log --diff-filter=D -- docs/` shows none was ever deleted.

**Change.**

- `handler.go`: `treeBuilder` replaces `insertIntoTree`. It keeps
  `map[string]int` from a node's path to its position in the parent's
  `Children`, so a segment resolves in constant time. The dead `urlPath`
  parameter is gone.
- `handler.go`: `treeJSONCached` checks the cache under the lock, releases it,
  calls the new `buildTreeJSON`, and swaps the result in under the lock again.
- `handler.go`: `gzipAsset` compresses an asset once per URL path on first
  request, and `serveAsset` uses it when the asset is at or above
  `gzipMinBytes` (16 KiB) and the request advertises gzip, setting
  `Vary: Accept-Encoding`. `gzippedAssets` is a `sync.Map` because entries are
  written once and read on every later request.
- `main.go`: `serve` runs `srv.Serve` and, on context cancellation, calls
  `srv.Shutdown` with a 5s grace period and waits for it. `main` derives that
  context from `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`.
- `static/js/toc.js`: a heading-to-link `Map` replaces the scan, and only the
  previously active entry is cleared.
- `handler.go`: the `parentPath` comment was moved onto `parentPath`.
- `docs/security.md`: written, covering the threat model, containment
  (`os.Root`, the VCS-name policy and why it is deliberately loose), rendering
  guarantees (raw HTML off, CSP nonce, sandboxed executable types, the 32 MiB
  cap), what CI checks, and the SBOM/license contract (env vars and exit codes
  taken from `scripts/ci/`).

**Benchmarks.** `TMPDIR` had to be redirected (see the P1 log); the container's
`/tmp` is `noexec`, so test binaries will not run from it.

```
Before (main):
BenchmarkTreeBuild-2           5,563,160 ns/op   265,605 B/op    4,248 allocs/op
BenchmarkTreeBuildWideDir-2    6,216,313 ns/op 1,412,834 B/op   10,069 allocs/op

After:
BenchmarkTreeBuild-2           5,458,311 ns/op   290,448 B/op    4,061 allocs/op
BenchmarkTreeBuildWideDir-2    2,985,182 ns/op 1,672,436 B/op   10,103 allocs/op
```

The wide-directory rebuild is **2.06× faster**; the narrow tree is unchanged, as
expected — it has almost no siblings, so the index has nothing to save. The
index costs ~230 KB on the 2,000-file tree, which is the map itself.

**Verification.**

- `go test ./... -count=1` — all packages pass. New tests, run verbosely to
  confirm they execute: `TestMermaidBundleIsCompressed`,
  `TestMermaidBundleUncompressedWithoutAcceptEncoding`,
  `TestSmallAssetServedUncompressed`, `TestTreeBuilderReusesExistingNodes`,
  `TestServeShutsDownOnContextCancel`, `TestTocBuildsOutlineFromHeadings`,
  `TestTocHighlightsOnlyTheHeadingInView`, plus the updated `TestInsertIntoTree`
  (which now also asserts the derived paths).
- `TestTreeBuilderReusesExistingNodes` exists for the specific hazard this
  change introduces: children live in a slice, so appending siblings can
  reallocate the backing array. Caching `*treeEntry` pointers would have left
  stale pointers behind — nodes that later inserts extend but that never appear
  in the tree. The index stores positions instead, and the test inserts a node,
  forces the slice to grow past 64 siblings, then adds more children to the
  first node and asserts all three are present.
- `TestServeShutsDownOnContextCancel` uses a client with keep-alives disabled, so
  the post-shutdown request genuinely has to open a new connection and cannot
  pass on a pooled one.
- `toc.js` could not be unwrapped for testing the way `tablesort.js` is: it
  returns early when there is no outline, and a bare `return` is illegal at the
  top level of a script. Following `sidebar_test.go`, the tests run the file
  as-is against a DOM shim and drive the captured IntersectionObserver callback;
  `tocVM` asserts exactly one observer was registered, so a renamed or
  restructured toc.js fails loudly instead of passing vacuously.
- `gosec ./...`: 0 issues. `semgrep --config auto` over Go and JS: 0 findings.
  `gitleaks detect`: no leaks. (Figures in the PR.)

**Notes.**

- `docs/security.md` states plainly that secret scanning is *not* in CI. Claiming
  otherwise would have been the easy thing to write and a false assurance.
- Not done, deliberately: migrating `gopkg.in/yaml.v2` (deprecated, v3 is
  current). Front matter order is preserved through `yaml.MapSlice`, which v3
  does not have, so the migration needs a custom `yaml.Node` decode to keep the
  rendered table's key order — a focused change of its own, not a rider on this
  one.
- Merge interaction: this branch and the P1 branch both edit `serveAsset` (P1's
  cache headers, this one's compression). Whichever merges second needs a
  one-hunk rebase there. Both also append to `## [Unreleased]`.
