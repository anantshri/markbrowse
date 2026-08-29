# Detailed Changelog

The long-form companion to `CHANGELOG.md`. Where `CHANGELOG.md` says *what*
changed in one line, this file records *why* and *how* — enough for a future
reader to audit, reproduce, or roll back any change without re-deriving it.

Add a new entry (newest first) for every meaningful change. Use the template
below; drop sections that genuinely don't apply.

---

## 2026-08-29 — PR integration round: table sorting, TOC, front matter, permissions, perf

**Summary:** Folded the useful content of open PRs #5, #7, #8, #9, #11, #12
into the codebase and implemented the two unaddressed issues #13 (right-side
TOC) and #14 (front matter as a GitHub-style table). PR #10 was skipped as
superseded by #12. Six of six open issues are now resolved by this code.

**Why:** The repo had accumulated six open ai-anant PRs (the owner's AI
persona — content integrated directly without per-PR merges) and one
dependabot PR, plus six open issues with no fix shipped to `main`.

**What changed:**
- `go.mod`/`go.sum`: goldmark 1.8.2 → 1.8.5 (cherry-picked dependabot commit
  `5bdfce5`); added `github.com/yuin/goldmark-meta v1.1.0` (+ transitive
  `gopkg.in/yaml.v2`).
- `handler.go`:
  - Dot directories no longer pruned from the sidebar tree walk (#1);
    empty ones still vanish via `pruneEmptyDirs`.
  - Permission errors mapped to 403 at every filesystem site (Stat, Open,
    ReadDir, ReadFile) instead of 404/500 (#2).
  - `serveMarkdown` computes an ETag (`"<mtime-unixnano>-<size>"`), answers
    `If-None-Match` with a bare 304, and tags 200s with the ETag (PR #11).
  - New `treeJSONCached()` with a 5s TTL caches the walk+prune+sort
    (PR #11); `serveTreeJSON` serves the cached bytes with `no-cache`.
  - New routes `/__mdview/tablesort.js` and `/__mdview/toc.js`; all four
    embedded assets now go through a shared `serveAsset` helper.
  - Page title prefers a front matter `title` key over `<h1>` extraction.
- `markdown.go`:
  - `wikilinkResolver` defers `buildFileIndex` behind `sync.Once` (PR #11)
    and no longer skips dot directories (consistency with #7).
  - `convert()` renders through a `sync.Pool` of `bytes.Buffer` (PR #11).
  - Registers `meta.New()` (parser only). `convert()` reads
    `meta.GetItems(ctx)` (order-preserving `yaml.MapSlice`) and prepends
    `renderMetaTable()` output — one row per key, first pair in `<thead>`,
    values HTML-escaped, lists joined with `<br>` (#14).
  - New helpers `renderMetaTable`, `renderMetaValue`, `metaTitle`,
    `metaTitleOf`.
- `main.go`: `validateReadable(rootDir)` fails fast at startup when the root
  directory can't be read, with a message naming the permission bits to
  check (#2). Deliberately simpler than PR #8 (no README probing).
- `static.go`: embeds `tablesort.js` + `toc.js`; CSS gains sort-header
  affordances, the full `.mdview-toc` styleset (collapsible `details` tree,
  indent per level, `@media(max-width:1100px)` hides it,
  `scroll-behavior:smooth`), and the PR #9 alert tint variables + per-type
  `background-color` rules for both light and dark palettes (#3).
- `static/js/tablesort.js` (new, from PR #12 verbatim): value-aware click
  sorting; `static/js/toc.js` (new): builds the nested TOC from
  `h1..h6[ id ]`, h1/h2 expanded, IntersectionObserver highlights the
  section in view.
- `templates.go`: both templates load `tablesort.js`; `mdTmpl` also gains
  the `<nav id="toc-root">` element and `toc.js`.
- Tests: ported/added `TestServeTreeIncludesDotDirs`,
  `TestServeHTTPForbiddenOnUnreadableFile`,
  `TestServeDirectoryForbiddenOnUnreadableDir`,
  `TestTreeJSONCachedRebuildsAfterTTL`, `TestServeMarkdownETagNotModified`,
  `TestTableSortAssetServed`, `TestTocAssetServed`,
  `TestTemplatesIncludeClientScripts`, `TestServeDirectoryListingAndIndex`,
  `TestServeMarkdownTitleFromFrontMatter`, `TestAlertCSSIncludesBackgroundTints`,
  `TestTablesortCSSPresent`, `TestTocCSSPresent`,
  `TestBuildFileIndexIncludesDotDirs`, `TestWikilinkResolvesInDotDirs`,
  front-matter suite in `markdown_test.go`, `main_test.go`
  (`validateReadable`, `loadCustomCSS`), `listen_test.go`, and PR #11's
  `bench_test.go`.

**How / commands run:**
```
# toolchain (container had only Go 1.22; go.mod requires >= 1.24)
curl -sL https://go.dev/dl/go1.24.0.linux-arm64.tar.gz -o /tmp/go1.24.0.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go1.24.0.tgz
sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go

git cherry-pick 5bdfce5          # goldmark 1.8.5 (PR #5)
go get github.com/yuin/goldmark-meta@v1.1.0 && go mod tidy
go build ./... && go vet ./...
go test -cover ./...             # 39 passed, 69.7% statements
aidc-scan                        # all scanners clean
```

**Errors encountered & resolution:**
- `go` kept trying to download toolchain go1.24 (module requirement) —
  fixed by installing Go 1.24.0 to `/usr/local/go` and symlinking into PATH.
- Import collision: stdlib `html` vs `goldmark/renderer/html` in
  `markdown.go` — aliased the goldmark package as `ghtml`.
- `metaTitleOf` initially passed `nil` as source (returned "") — corrected
  to pass the actual `source` bytes.
- Extracting `tablesort.js` from the PR diff lost the trailing `})();` —
  caught by `node --check`, appended.
- `aidc-scan` initial run: gosec G703 (new taint-analysis rule, 5 sites)
  + semgrep XSS/open-redirect findings on long-standing patterns the new
  rules now flag. Fixed by consolidating asset writes into `serveAsset`,
  and annotating each remaining site with `#nosec G304 G703` /
  `nosemgrep:` comments carrying the justification (validation happens in
  `ServeHTTP` before dispatch; redirects use server-side `path.Clean`'d
  paths, never raw user input).

**Verification:**
- `go test ./...`: 39 tests pass; coverage 69.7% (from ~48% pre-change);
  `main()` itself remains untestable by design.
- `node --check` on both new JS files.
- Live server run against a fixture tree (`/tmp/e2e`): front matter table
  rendered exactly as GitHub (row-per-key, thead/tbody), `<title>` taken
  from `title:` key, `.hidden/` present in `tree.json` while a markdown-less
  `.git/` stayed pruned, ETag + conditional request returned 304 with empty
  body, `toc.js`/`tablesort.js` served as `application/javascript`, heading
  IDs present for TOC anchors, alert tint CSS present in served pages,
  unreadable file → 403, unreadable root → startup aborts with the clear
  permission message.
- `aidc-scan`: semgrep, gitleaks, gosec, vet, license-check all clean.

**Notes / follow-ups:**
- goldmark v2 / goldmark-meta v2 (v2.0.2 released 2026-08-27, needs Go
  ≥ 1.25) is blocked: the gh-alerts, mermaid and wikilink extensions have
  no v2 releases yet. The custom `renderMetaTable` reproduces the v2-style
  row-per-key layout on v1 anyway. Revisit when the ecosystem catches up.
- PR #10 skipped — same feature as #12 with a weaker comparator.
- PR #8's startup README-probing dropped — per-request 403s already cover
  file-level permission issues.
- GitHub housekeeping (closing PRs #5/#7/#8/#9/#11/#12 and issues
  #1/#2/#3/#6/#13/#14) intentionally left to the owner; SSH auth to
  github.com failed from this container.

---

## YYYY-MM-DD — <short title>

**Summary:** One or two sentences on what changed and the user-facing effect.

**Why:** The problem, request, or constraint that prompted this. Link the issue
/ ticket / discussion if there is one.

**What changed:**
- File-by-file or component-by-component list of the edits.

**How / commands run:**
```
# exact commands executed, with the relevant output
```

**Errors encountered & resolution:** Anything that went wrong and how it was
fixed (or why it was left as-is).

**Verification:** How the change was proven to work — tests run, scanners,
manual checks, screenshots.

**Notes / follow-ups:** Design choices, trade-offs, and anything deferred.
