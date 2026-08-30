# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
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

### Security
- Fix stored XSS: `<script>`/event-handler HTML in served markdown no longer
  executes in the viewer's browser (secreports/report1.md findings 1+2).
- Symlinks inside the served tree can no longer point the server at files
  outside it: every request path is resolved with `filepath.EvalSymlinks` and
  must remain under the resolved root (secreports/report1.md finding 3).
- `.git`, `.hg`, `.svn` and `.bzr` directories are never served, listed in the
  sidebar, or wikilink-indexed (secreports/report1.md finding 4). Other
  dot-directories (`.obsidian` vaults) remain browsable.


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

[Unreleased]: https://github.com/anantshri/markbrowse/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.3.0
[0.2.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.2.0
[0.1.2]: https://github.com/anantshri/markbrowse/releases/tag/v0.1.2
[0.1.1]: https://github.com/anantshri/markbrowse/releases/tag/v0.1.1
[0.1.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.1
