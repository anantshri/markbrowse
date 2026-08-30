# Detailed Changelog

The long-form companion to `CHANGELOG.md`. Where `CHANGELOG.md` says *what*
changed in one line, this file records *why* and *how* — enough for a future
reader to audit, reproduce, or roll back any change without re-deriving it.

Add a new entry (newest first) for every meaningful change. Use the template
below; drop sections that genuinely don't apply.

---

## 2026-08-30 — Fix macOS CI failure in TestListenWithFallback

**Summary:** The test that guards the port-fallback feature failed on
macOS GitHub runners (`fallback returned the busy port 49169`). Rewrote it
to hold the busy port with the same wildcard bind the production code
uses, making the conflict platform-independent; also hardened the
requested-port and error-path coverage.

**Why:** CI matrix (`ci.yml`) runs ubuntu/macos/windows. The test held
`127.0.0.1:<ephemeral>` (IPv4 loopback) while `listenWithFallback` binds
`:<port>` (wildcard). On Linux the two conflict → fallback triggered →
test passed. On macOS/BSD the IPv6 dual-stack wildcard socket coexists
with the IPv4-loopback holder, so the function bound the "busy" port and
returned it → assertion failed. A test bug, not a product bug — the
feature itself works on macOS.

**What changed:**
- `listen_test.go`:
  - `TestListenWithFallback` now holds `:0` (wildcard, same spec as
    production) and asserts `actual > port` (strictly above, catching
    equal-or-lower regressions) plus the 10000 floor; skips if the
    ephemeral port leaves no headroom.
  - `TestListenWithFallbackPrefersRequestedPort` (new): a just-freed
    random port is rebound without fallback (skip on TOCTOU loss).
  - `TestListenWithFallbackErrorPath` (new): invalid port `-1` fails the
    initial bind and the scan starts at the 10000 floor — asserted as
    documented behavior (a bind at/above 10000 may legitimately succeed,
    so the old "must error" expectation was wrong on every platform).

**How / commands run:**
```
go vet ./...
go test -run TestListenWithFallback -v .   # 3 passed
go test ./...                              # 42 passed
```

**Errors encountered & resolution:** First draft of the error-path test
asserted `listenWithFallback(-1)` must error — it doesn't: the scan starts
at the 10000 floor and binds there. Rewrote to assert the floor behavior.
Also removed a leftover `fmt` import guard. Finally, the wildcard `":0"`
holders tripped semgrep's `avoid-bind-to-all-interfaces` — annotated both
with `nosemgrep:` justifications (test-only transient holders mirroring the
production bind spec deliberately; that mirroring is the fix itself).

**Verification:** All three tests pass locally (Linux/arm64); the wildcard
holder guarantees the same conflict semantics on macOS/Windows since it
now uses the exact production bind spec. Full suite 42/42; `aidc-scan`
clean (semgrep + gosec).

**Notes:** Folded into the 0.3.0 changelog entry (release not yet cut).
The production `listenWithFallback` code is unchanged.

---

## 2026-08-30 — Release 0.3.0 preparation

**Summary:** Cut the release paperwork for 0.3.0: the `Unreleased` changelog
section becomes `0.3.0` dated 2026-08-30, with compare links added. No tags,
branches, or pushes — release mechanics (tag `v0.3.0`, workflow draft) are
done manually by the owner.

**Why:** The integration round (PR fold-in + #13/#14 implementation), the
testdata fixtures, the TOC toggle, and the README credit are all merged on
`enhancement-29-aug-2026`; the changeset is feature-sized (semver: new
functionality, no breaking changes → minor bump from 0.2.0 to 0.3.0).

**What changed:**
- `CHANGELOG.md`: `## [Unreleased]` → empty section retained for future
  work + `## [0.3.0] - 2026-08-30` heading over the existing entries;
  added `[Unreleased]` (compare v0.3.0...HEAD) and `[0.3.0]` link refs.
- Nothing else — `main.version` stays `"dev"` (CI injects the real version
  via `-ldflags -X main.version=` from the tag name on release builds; both
  `ci.yml` and `release.yml` already do this), so no code changes are
  needed for the release.

**How / commands run:**
```
go build ./... && go vet ./... && go test ./...   # 40 passed
aidc-scan                                          # clean
```

**Verification:** Build/vet/tests/scan all clean; changelog renders with
the new heading and both link references resolve to the intended GitHub
URLs (release URL for 0.3.0 will exist once the owner tags it).

**Notes / follow-ups — owner's manual steps for the release:**
1. Merge `enhancement-29-aug-2026` → `main`.
2. `git tag v0.3.0 && git push origin v0.3.0` — this triggers
   `.github/workflows/release.yml`, which builds linux/darwin/windows
   (amd64/arm64) archives with the version baked in and creates a **draft**
   GitHub release with generated notes.
3. Edit/publish the draft release (workflow sets `draft: true`).
4. Optionally close out PRs #5/#7/#8/#9/#11/#12 and issues
   #1/#2/#3/#6/#13/#14 — the integration commit message carries `Fixes`
   references, so merging the branch into default-branch commits closes
   them automatically.

---

## 2026-08-30 — Collapsible right sidebar (TOC toggle)

**Summary:** The right-hand TOC panel now collapses and restores via a
toggle button, mirroring the left file sidebar's behavior exactly.

**Why:** The TOC added for #13 was always visible on wide viewports with no
way to reclaim the reading width. The left sidebar already had a collapse
pattern; the right side deserved the same.

**What changed:**
- `templates.go`: `<button id="toc-toggle">` after the TOC nav in `mdTmpl`
  only (» glyph; JS flips to « when collapsed).
- `static.go`: `.mdview-toc` gains `transition:width .2s ease,min-width
  .2s ease`; new `#toc-toggle` fixed-position button rule (mirrored
  geometry: `right:240px` at the TOC's edge), `body.toc-collapsed` rules
  collapsing the panel and parking the button at `right:12px`; the 1100px
  media query now hides the button alongside the panel.
- `static/js/toc.js`: toggle handler flipping `body.toc-collapsed` and the
  glyph; the no-headings early exit now hides the button too (no dead
  button on heading-less pages).
- Tests: `TestTemplatesIncludeClientScripts` asserts the button on mdTmpl
  and its absence on dirTmpl; new `TestTocToggleCSSPresent` covers the
  button, collapse, and media-query rules.

**How / commands run:**
```
node --check static/js/toc.js
go build ./... && go vet ./... && go test ./...   # 40 passed
go build -o /tmp/markbrowse-e2e . && /tmp/markbrowse-e2e -port 18452 testdata/
curl -s http://localhost:18452/guides/table-of-contents.md | grep toc-toggle
curl -s http://localhost:18452/guides/ | grep -o '[^<>]*toc-toggle[^<>]*'
```

**Errors encountered & resolution:** An initial live check looked like the
dir page "matched toc-toggle 3×" — inspection showed those were the CSS
rules in the shared stylesheet (`#toc-toggle` selectors), not the button
element; selectors are inert without the element, and the Go test asserts
the `id="toc-toggle"` attribute form, which correctly appears only on md
pages. No fix needed.

**Verification:** Button present on `guides/table-of-contents.md`, absent
(dir-listing HTML) on `/guides/`; served `toc.js` contains the
`toc-collapsed` handler; served CSS carries the collapse rules. 40 tests
pass; `aidc-scan` clean.

**Notes / follow-ups:** No state persistence (matches left sidebar);
no mobile drawer mode for the TOC — left sidebar's ≤767px pattern stays
unique to it.

---

## 2026-08-30 — Test fixtures for the new feature set

**Summary:** Added markdown fixtures under `testdata/` exercising every
feature from the 2026-08-29 integration round, so `markbrowse testdata/`
demonstrates and manually verifies all of it out of the box.

**Why:** The new features (front matter table, TOC, table sorting, dot-dirs,
alert tints, ETag/tree caching) shipped with Go unit tests and temporary
`/tmp` fixtures, but the in-repo sample vault still showcased only the
original feature set — nothing reproducible for a human to click through.

**What changed:**
- `testdata/frontmatter.md` (new): full metadata block (strings, bool,
  number, two lists) — renders the GitHub-style table and sets the tab
  title; body links onward via wikilinks.
- `testdata/guides/table-of-contents.md` (new): h1–h6 hierarchy including a
  level skip, exercising the TOC's nesting, expand/collapse defaults, and
  anchors.
- `testdata/guides/table-sorting.md` (new): four tables (comma numbers, byte
  sizes, timestamps, mixed text) starting out of order, with per-table
  instructions on what correct sorting proves.
- `testdata/guides/callout-tints.md` (new): all five alert types + dark-mode
  note; also carries front matter (title check).
- `testdata/guides/performance.md` (new): documents and demonstrates the
  ETag/304 flow (copy-paste curl), tree TTL, lazy wikilink index, and the
  permission behavior table.
- `testdata/.obsidian/vault-notes.md` (new): dot-directory fixture reachable
  as `[[vault-notes]]` and visible in the sidebar.
- `testdata/README.md`: feature list extended; wikilinks to every new
  fixture (which is itself the wikilink-resolution test).

**How / commands run:**
```
go build -o /tmp/markbrowse-e2e .
/tmp/markbrowse-e2e -port 18441 /workspace/testdata
curl -s http://localhost:18441/__mdview/tree.json   # .obsidian + new guides listed
curl -s http://localhost:18441/frontmatter.md       # meta table + <title>
curl -sI http://localhost:18441/guides/performance.md | grep -i etag
curl -s http://localhost:18441/README.md            # wikilinks resolved
go build ./... && go vet ./... && go test ./...     # 39 passed
aidc-scan                                           # clean
```

**Verification:** Live server against `testdata/` — tree lists `.obsidian/`
and all five new pages; `frontmatter.md` renders the full meta table with
`<title>Case Assignment Memo</title>`; both titled guides pick up their
front-matter titles; `performance.md` carries an ETag; all README wikilinks
resolve to real hrefs (`/.obsidian/vault-notes.md`, `/frontmatter.md`, both
new guides); the TOC fixture produces 11 anchored headings across all six
levels; the sorting fixture renders 4 tables; `getting-started.md` remains
meta-table-free (regression guard). `aidc-scan` clean.

**Notes / follow-ups:** Fixtures are prose-first (they double as user-facing
demos); assertions about them live in the Go test suite via temp dirs, not
in testdata itself, to keep the sample vault clean.

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
