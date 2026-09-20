# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-09-20

### Added
- **Sidebar quick filter** (#18). A search box above the file tree filters the
  whole vault, not just the folders that happen to be open: type part of a
  name and every match appears with the folders it lives under, matched
  characters highlighted. A query containing `/` matches the full path instead
  of the name, `Esc` clears, and results are capped at 200 with a count.
- `golden/`: golden-file coverage for the markdown pipeline. Every document in
  `testdata/` plus 12 synthetic cases (all wikilink forms, all alert kinds,
  mermaid fences, front matter including malformed input, GFM, raw HTML, and
  the dangerous URL schemes) is rendered in both `--raw-html` modes and
  compared. Regenerate with `go test -run TestRenderGolden -update-golden`.
- Unit tests for each vendored extension, at 90–95% statement coverage.
- `tablesort_test.go`: the embedded `static/js/tablesort.js` is now executed
  in a JS engine from `go test`, covering ascending and descending order for
  every value shape the sorter supports and the `../` row detection.
- The vendored mermaid bundle is pinned by sha256 in `handler_test.go`, and
  its `securityLevel:"strict"` bootstrap is asserted. Nothing else tracks that
  file — dependabot reads `go.mod` and the workflows, syft's SBOM sees only Go
  modules — so bumping mermaid now fails a test until the recorded version and
  digest are updated with it.

### Changed
- **Sidebar renders lazily** (#18). A folder's contents are built the first
  time it is opened rather than for the entire tree on page load; only the
  branch containing the current page is expanded up front. On a 4,800-file
  vault this is 174 DOM elements per page load instead of 10,923. The whole
  tree is still fetched and held in memory, which is what lets the filter
  search files it has never drawn.
- `/__mdview/tree.json` now carries an `ETag` and honours `If-None-Match`, so
  the sidebar's refetch on every page navigation returns a bodiless `304`
  instead of re-sending the tree — 311 KB on that same vault.
- **Migrated to goldmark v2**, and the four markdown extensions are now
  vendored under `internal/` instead of imported. goldmark v2 removed the
  `goldmark.Extender` interface the upstream extensions implement, and none of
  them has a v2 release, so each was ported and trimmed to what markbrowse
  actually uses. Rendering is unchanged: all 44 golden cases produce
  byte-identical HTML. The module now depends on two libraries at runtime
  (`goldmark/v2` and `yaml.v2`) instead of six.
  - `internal/wikilink` — from `go.abhg.dev/goldmark/wikilink` (BSD-3-Clause)
  - `internal/mermaid` — from `go.abhg.dev/goldmark/mermaid` (BSD-3-Clause),
    ~85% smaller: the mermaid-CLI and headless-Chrome rendering backends are
    gone, along with the chromedp dependency tree
  - `internal/alerts` — from `goldmark-gh-alerts` (MIT)
  - `internal/frontmatter` — from `goldmark-meta` (MIT), parser only
- **Default bind changed to `127.0.0.1`** (was `0.0.0.0`). The server is
  unauthenticated, so it is no longer network-reachable by default; pass
  `--listen 0.0.0.0` (or a specific address) to expose it. Container port
  mappings (`-p 8080:8080`) now need `--listen 0.0.0.0`.
  (secreports/report1.md finding 7)
- CI/release workflows pass `github.ref_name` through an env var and scrub
  it before use, instead of interpolating `${{ }}}` into `run:` steps
  (secreports/report1.md findings 5+6).
- Raw HTML embedded in markdown is now omitted by default (matching GitHub)
  and `javascript:`/`vbscript:`/`file:`/`data:` link targets are filtered;
  `--raw-html` restores the previous pass-through behavior for trusted
  content.
- **Mermaid upgraded 11.15.0 → 12.0.0**, adopting its new defaults: the ELK
  layout engine (was dagre), the `neo` look, and the `redux-color` theme.
  **Existing diagrams will re-lay-out and restyle.** Mermaid 12 needs an
  ES2024 browser (Safari 17.4+, current Chrome/Firefox/Edge), and the bundle
  grows from 3.2 MB to 5.3 MB — it is still only loaded on pages that
  actually contain a diagram. `securityLevel:"strict"` is unchanged and now
  covered by a test.
- Minimum Go version is now 1.26 (was 1.24), and CI/release build with it.
  Go 1.25 is required by `github.com/dop251/goja`, the JS engine the new
  table-sorting tests run `static/js/tablesort.js` in; 1.26 by
  `golang.org/x/text`.
- Dependencies at latest: `github.com/yuin/goldmark` 1.8.5 → 1.8.6 (#17, two
  `URLEscape` fixes plus an extension fix), `gopkg.in/yaml.v2` 2.3.0 → 2.4.0
  (#16), plus `regexp2` 2.5.2 → 2.8.0, `sourcemap` 2.1.3 → 2.1.4, `pprof` and
  `golang.org/x/text` 0.3.8 → 0.42.0.
- CI/release pinned tooling at latest: `actions/checkout` v6 → v7.0.1,
  `actions/setup-go` v6.4.0 → v7.0.0, `actions/upload-artifact` v4 → v7.0.1
  in the SBOM workflow, `softprops/action-gh-release` v3.0.0 → v3.0.3, gosec
  v2.22.3 → v2.29.0, syft v1.18.1 → v1.52.0, grype v0.87.0 → v0.119.0.
- Dependabot now watches `github-actions` as well as `gomod`; without it the
  action pins never moved and had drifted a full major behind.

### Fixed
- The sidebar now reveals and highlights the page you opened directly, not
  just ones you clicked through to (#18). When a directory serves its index,
  the page reports the file actually rendered (`/guides/README.md`) rather
  than the directory that was requested (`/guides`) — the sidebar only has a
  node for the file, so the old value matched nothing and the tree stayed
  collapsed with nothing marked. A directory *listing* reports itself with a
  trailing slash, which opens that folder without marking any file.
- Table sorting now compares the whole value instead of the first digits
  `parseFloat` happened to find: cells are split into text and number
  segments and compared segment by segment, so `8x.0` sorts before `9.0`,
  `95%` before `100%`, `A1` before `A10` before `B2`, and `Chapter 3` before
  `Chapter 10`. Values with `%`/unit/currency attachments (`1.2m`, `$1,234`)
  and version-like values (`8.9.0` before `8.10.0`) no longer fall into the
  plain-text branch (#19).
- Directory listings: the `../` row of a first-level directory now links to
  `/` instead of the `//` protocol-relative URL, and the sorter matches the
  row by its `../` label, so the parent row really stays pinned above the
  sorted rows (#19).

### Security
- The front matter YAML parse error, which is emitted as an HTML comment when
  a metadata block fails to parse, can no longer close that comment early:
  `-->` is escaped out of the message. Error text is partly derived from
  document content, and the upstream extension interpolated it verbatim.
- The alert kind is HTML-escaped before it reaches the `class` attribute. The
  marker syntax already restricts it to word characters, so this closes the
  shape of the hole rather than a reachable one.
- Fix stored XSS: `<script>`/event-handler HTML in served markdown no longer
  executes in the viewer's browser (secreports/report1.md findings 1+2).
- Symlinks inside the served tree can no longer point the server at files
  outside it: every request path is resolved with `filepath.EvalSymlinks` and
  must remain under the resolved root (secreports/report1.md finding 3).
- `.git`, `.hg`, `.svn` and `.bzr` directories are never served, listed in the
  sidebar, or wikilink-indexed (secreports/report1.md finding 4). Other
  dot-directories (`.obsidian` vaults) remain browsable.

## [0.3.0] - 2026-08-30

### Added
- Client-side table sorting: click any table header (markdown tables and the
  directory listing) to sort ascending/descending, with value-aware ordering
  for numbers, byte sizes (`1.5 KB`), timestamps, and `../` rows pinned on
  top (#6).
- Right-hand collapsible table of contents on markdown pages, built from the
  heading hierarchy with active-section highlighting and smooth scrolling;
  the panel itself collapses via a `»` toggle button (mirroring the left
  sidebar's animation), and hides below 1100px viewport width (#13).
- YAML front matter now renders as a GitHub-style key/value table (one row
  per key, first pair in `<thead>`), and a `title` key sets the page title
  (#14).
- `ETag`/`304` support for markdown pages: conditional requests skip
  re-reading and re-rendering unchanged files.
- Sidebar tree JSON is cached for 5 seconds, sparing a full directory walk
  on every page load.
- Wikilink file index is built lazily on first `[[link]]` instead of at
  startup.

### Fixed
- Test suite now passes on macOS/Windows: `TestListenWithFallback` held the
  busy port on `127.0.0.1` while `listenWithFallback` binds the wildcard
  `:port` — an IPv6 dual-stack wildcard bind coexists with an IPv4-loopback
  holder on macOS/BSD, so the "busy" port was bindable and the fallback
  never triggered there.
- Permission-denial tests skip on Windows, where Unix permission bits don't
  model readability (`chmod 000` only toggles the read-only attribute on
  files and is a no-op on directories), so EACCES can't be simulated; the
  403/startup-validation code paths themselves are unchanged.
- Dot directories (e.g. `.obsidian`) now appear in the sidebar tree and
  resolve in wikilinks, provided they contain markdown files (#1).
- Permission errors now surface as `403 Forbidden` (previously masked as
  404/500), and the server validates at startup that the root directory is
  readable, failing fast with a clear message (#2).
- GitHub-style alert callouts (`> [!NOTE]` etc.) render with per-type tinted
  backgrounds in both light and dark themes, matching GitHub (#3).

### Changed
- Bumped `github.com/yuin/goldmark` from 1.8.2 to 1.8.5.
- Expanded the sample vault in `testdata/` with fixtures for every new
  feature: front matter (`frontmatter.md`, plus titles on two guides),
  heading-depth TOC and sortable-table demos under `guides/`, an
  `.obsidian/` dot-directory page reachable via `[[vault-notes]]`, and a
  performance/behavior reference.

## [0.2.0] - 2026-06-02

### Fixed
- Dark mode now activates correctly under `prefers-color-scheme: dark`. The
  embedded GitHub markdown stylesheet had a malformed media query that
  prevented the dark palette from being applied.

## [0.1.2] - 2026-06-01

### Added
- Automatic port fallback: when the requested port is already in use,
  `markbrowse` searches upward (starting at 10000, up to 65535) and binds to
  the first free port instead of exiting.

### Fixed
- Admonition callouts (`> [!NOTE]`, `> [!WARNING]`, etc.) render reliably by
  switching to the maintained `gm-alert-callouts` extension.

## [0.1.1] - 2026-05-29

### Added
- Dependabot configuration for dependency updates.

### Changed
- Release workflow hardening so tagged releases build and publish reliably via
  GitHub Actions.

## [0.1.0] - 2026-05-28

Initial public release.

### Added
- Markdown rendering with GitHub-flavored Markdown (tables, strikethrough,
  task lists, autolinks, heading anchors).
- Collapsible sidebar file tree for navigating the served directory.
- Directory index resolution via `README.md` / `INDEX.md`, with a file listing
  fallback when no index is present.
- Pass-through serving of non-markdown files (images, PDFs, etc.).
- [Mermaid](https://mermaid.js.org/) diagram rendering.
- Wiki-style `[[link]]` syntax with file-tree resolution.
- GitHub/Obsidian-style admonition callouts.
- Custom CSS support via `--css` to replace the built-in stylesheet.
- Breadcrumb navigation on all pages.
- Single-binary distribution, zero configuration.

[Unreleased]: https://github.com/anantshri/markbrowse/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.4.0
[0.3.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.3.0
[0.2.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.2.0
[0.1.2]: https://github.com/anantshri/markbrowse/releases/tag/v0.1.2
[0.1.1]: https://github.com/anantshri/markbrowse/releases/tag/v0.1.1
[0.1.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.1
