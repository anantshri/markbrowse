# markbrowse

A minimal markdown file viewer. Point it at any directory and it starts a web server that renders `.md` files as styled HTML, with a collapsible sidebar file tree for navigation.

## Features

- Renders markdown files as HTML with GitHub-flavored styling (light + dark mode)
- Collapsible sidebar file tree for navigating the directory
- Uses directories' `README.md` or `INDEX.md` as the index page
- Falls back to a file listing when no index markdown is present
- Serves non-markdown files as-is (images, PDFs, etc.)
- [Mermaid](https://mermaid.js.org/) diagram rendering (mermaid 12, bundled — needs an ES2024 browser: Safari 17.4+, current Chrome/Firefox/Edge)
- Wiki-style `[[link]]` syntax with file-tree resolution
- GitHub/Obsidian-style admonition callouts (`> [!NOTE]`, `> [!WARNING]`, etc.)
- YAML front matter rendered as a GitHub-style metadata table, with `title` used as the page title
- Client-side table sorting (click column headers; numbers, byte sizes, timestamps, `%`/unit values, version-like values, and mixed text/number labels such as `Chapter 10` all sort naturally)
- Collapsible right-hand table of contents built from the heading hierarchy
- Custom CSS support to override the built-in stylesheet
- Breadcrumb navigation on all pages
- Single binary, zero config

## Install

```bash
go install github.com/anantshri/markbrowse@latest
```

Or build from source:

```bash
git clone https://github.com/anantshri/markbrowse.git
cd markbrowse
go build -o markbrowse .
```

## Usage

```bash
# Serve current directory on port 8080
markbrowse

# Serve a specific directory
markbrowse /path/to/docs

# Custom port
markbrowse --port 3000

# Expose on the network (default is loopback-only, since the server is
# unauthenticated — only do this on trusted networks)
markbrowse --listen 0.0.0.0

# Custom CSS (replaces built-in stylesheet entirely)
markbrowse --css ./my-theme.css

# Render raw HTML in markdown (only for content you trust; also re-enables
# javascript:/data: link targets that are filtered by default)
markbrowse --raw-html
```

Open `http://localhost:8080` in your browser. If the requested port is busy,
`markbrowse` automatically falls back to the next free port (searching from
10000 upward) and logs the port it bound to.

By default the server binds `127.0.0.1` only. It serves files without
authentication, so it is deliberately not reachable from the network unless
you pass `--listen 0.0.0.0` (or a specific interface address). If you run it
in a container with a port mapping (e.g. `docker run -p 8080:8080 ...`),
add `--listen 0.0.0.0` or the mapping will not reach it.

## How it works

| Request | Behavior |
|---|---|
| `/` (directory with `README.md`) | Renders `README.md` as HTML |
| `/subdir/` (directory with `INDEX.md`) | Renders `INDEX.md` as HTML |
| `/subdir/` (directory without index) | Shows file/folder listing |
| `/notes.md` | Renders as HTML |
| `/image.png` | Serves raw file |

Markdown rendering supports GFM features: tables, strikethrough, task lists, autolinks, heading anchors, mermaid diagrams, wiki links, and admonition callouts.

Raw HTML embedded in markdown is omitted by default (`<!-- raw HTML omitted -->`,
matching GitHub's behavior), and `javascript:`/`vbscript:`/`file:`/`data:` link
targets are filtered. Pass `--raw-html` to render embedded HTML verbatim — only
do this for content you trust, since it re-enables both.

## Flags

```
  --port int    Port to listen on (default 8080)
  --listen IP   IP or hostname to bind (default 127.0.0.1; use 0.0.0.0 to
                expose on the network)
  --css path    Path to custom CSS file (replaces built-in stylesheet)
  --raw-html    Render raw HTML in markdown unescaped and allow
                javascript:/data: URLs (only for content you trust)
  --version     Print version and exit
```

## Dependencies

- [goldmark](https://github.com/yuin/goldmark) — markdown rendering with GFM extension
- [goldmark/mermaid](https://go.abhg.dev/goldmark/mermaid) — mermaid diagram support
- [goldmark/wikilink](https://go.abhg.dev/goldmark/wikilink) — `[[wiki link]]` parsing
- [gm-alert-callouts](https://github.com/zmtcreative/gm-alert-callouts) — `> [!NOTE]` admonition callouts

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for release notes.

## Acknowledgments

- Front matter rendering idea: [AngieGit](https://github.com/AngieGit) —
  [issue #14](https://github.com/anantshri/markbrowse/issues/14)

## License

GPL-3.0
