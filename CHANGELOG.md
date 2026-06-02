# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.2.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.2.0
[0.1.2]: https://github.com/anantshri/markbrowse/releases/tag/v0.1.2
[0.1.1]: https://github.com/anantshri/markbrowse/releases/tag/v0.1.1
[0.1.0]: https://github.com/anantshri/markbrowse/releases/tag/v0.1
