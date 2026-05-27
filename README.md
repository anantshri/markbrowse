# markbrowse

A minimal markdown file viewer. Point it at any directory and it starts a web server that renders `.md` files as styled HTML, with a collapsible sidebar file tree for navigation.

## Features

- Renders markdown files as HTML with GitHub-flavored styling (light + dark mode)
- Collapsible sidebar file tree for navigating the directory
- Uses directories' `README.md` or `INDEX.md` as the index page
- Falls back to a file listing when no index markdown is present
- Serves non-markdown files as-is (images, PDFs, etc.)
- [Mermaid](https://mermaid.js.org/) diagram rendering
- Wiki-style `[[link]]` syntax with file-tree resolution
- GitHub/Obsidian-style admonition callouts (`> [!NOTE]`, `> [!WARNING]`, etc.)
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

# Custom CSS (replaces built-in stylesheet entirely)
markbrowse --css ./my-theme.css
```

Open `http://localhost:8080` in your browser.

## How it works

| Request | Behavior |
|---|---|
| `/` (directory with `README.md`) | Renders `README.md` as HTML |
| `/subdir/` (directory with `INDEX.md`) | Renders `INDEX.md` as HTML |
| `/subdir/` (directory without index) | Shows file/folder listing |
| `/notes.md` | Renders as HTML |
| `/image.png` | Serves raw file |

Markdown rendering supports GFM features: tables, strikethrough, task lists, autolinks, heading anchors, mermaid diagrams, wiki links, and admonition callouts.

## Flags

```
  --port int    Port to listen on (default 8080)
  --css path    Path to custom CSS file (replaces built-in stylesheet)
  --version     Print version and exit
```

## Dependencies

- [goldmark](https://github.com/yuin/goldmark) — markdown rendering with GFM extension
- [goldmark/mermaid](https://go.abhg.dev/goldmark/mermaid) — mermaid diagram support
- [goldmark/wikilink](https://go.abhg.dev/goldmark/wikilink) — `[[wiki link]]` parsing
- [gm-alert-callouts](https://github.com/zmtcreative/gm-alert-callouts) — `> [!NOTE]` admonition callouts

## License

GPL-3.0
