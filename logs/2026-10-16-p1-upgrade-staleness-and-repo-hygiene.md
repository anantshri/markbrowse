# 2026-10-16 — P1: upgrade staleness (assets + page ETag) and the committed `.exe`

**Symptom / goal.** Two independent problems surfaced in a whole-code review
after the fork was synced to upstream `main` (`d95e343`, v0.4.0):

1. A 16 MB `markbrowse.exe` was tracked in git.
2. Upgrading the binary left browsers serving the previous build's assets and
   rendering, which is reported upstream as "stale pages after upgrade".

**Diagnosis.**

*Repo bloat.* `git ls-files` reported `markbrowse.exe` (16,207 KB) as tracked.
`git log --diff-filter=A -- markbrowse.exe` blamed `0a186d7` ("Skip
permission-denial tests on Windows CI") — a release build of the Windows target
that was swept into a commit. `.gitignore` covered `markbrowse` and `/mdview`
but not `markbrowse.exe`, so nothing stopped it.

*Stale assets.* `serveAsset` set:

```
Cache-Control: public, max-age=86400
```

for `/__mdview/mermaid.js`, `sidebar.js`, `tablesort.js` and `toc.js`. Those
URLs are fixed at compile time, so from the browser's point of view the
previous build's script and the new build's script are the same resource. For
24 hours after an upgrade, a cached script keeps running against pages rendered
by the new template.

*Stale rendering.* The markdown ETag was:

```go
etag := fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size())
```

— a function of the file alone. The rendered page also depends on the running
build's templates and stylesheet, so after an upgrade an unchanged document
still matched its ETag and the server answered `304`, leaving the browser on
the old rendering indefinitely.

**Change.**

- `handler.go`: added `serverID` (nanotime, base-36) as the identity of the
  running build. `serveAsset` now takes the request, sets `Cache-Control:
  no-cache` and `ETag: "<serverID>"`, and answers `304` when the client's
  `If-None-Match` matches — so an unchanged build costs one revalidation and a
  new build invalidates every cached asset.
- `handler.go`: the markdown ETag became `"<serverID>-<mtime>-<size>"`.
- `templates.go`: both templates gained an `assetFuncs` FuncMap and stamp
  `?v={{v}}` (the same `serverID`) onto every script URL, so the URL a previous
  build cached is not a URL this build serves — the stale copy is unreachable,
  not merely revalidated.
- `cache_test.go` (new): `TestServeAssetRevalidates`,
  `TestMarkdownETagIncludesBuildIdentity`, `TestPagesVersionScriptURLs`.
- Removed `markbrowse.exe` from the index; added `*.exe` to `.gitignore`.

Revalidation and versioned URLs are both applied deliberately: the `?v=` makes
a stale copy unusable, and the ETag keeps the unchanged case cheap.

**Commands.**

```
$ git ls-files | grep markbrowse.exe          # tracked artefact confirmed
$ git rm --cached markbrowse.exe              # rm 'markbrowse.exe'
$ TMPDIR=/Users/glt5/WORK/.gotmp go build ./... && go vet ./...
$ TMPDIR=/Users/glt5/WORK/.gotmp go test ./... -count=1
ok  github.com/anantshri/markbrowse            0.098s
ok  .../internal/alerts 0.002s  .../frontmatter 0.002s
ok  .../internal/mermaid 0.002s .../wikilink 0.002s
```

`go test` initially failed with `fork/exec /tmp/go-build.../markbrowse.test:
permission denied`: `/tmp` is mounted `noexec` in this container, so the test
binaries could not run. `TMPDIR` was pointed at the writable work mount
(`/Users/glt5/WORK/.gotmp`), which is also why every command above carries it.

**Verification.**

- `gosec ./...` (v2.29.0, the CI version): 9 files / 2608 lines → **0 issues**.
- `gitleaks detect --no-banner --redact`: 50 commits, 1.27 MB → **no leaks**.
- `semgrep scan --config auto --include='*.go'`: no findings.
- `go test -cover ./...`: main package 84.5%; `serveAsset` **100.0%**.
- New tests fail against the old behaviour: with `public, max-age=86400` and a
  file-only ETag, `TestServeAssetRevalidates` sees a `Cache-Control` mismatch
  and `TestMarkdownETagIncludesBuildIdentity` sees an ETag without `serverID`.

**Notes.**

- The `?v=` suffix and the ETag are not redundant. `no-cache` only means
  *revalidate*; a browser that revalidates still needs the URL to change for a
  different build to be fetched at all when a proxy or a service worker holds a
  copy. Versioning the URL removes that dependency.
- `static/js/mermaid.min.js` is sha256-pinned in `handler_test.go`; it was not
  touched, and the pin still holds.
- **Follow-up:** the 5.4 MB mermaid bundle is still served uncompressed. Pre-
  compressing it at build or init time is tracked separately (P3) and is
  deliberately not bundled here, because it edits the same function this change
  does and would make the two PRs conflict.
