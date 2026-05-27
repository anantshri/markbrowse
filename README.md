# mdview

A minimal markdown file viewer. Point it at any directory and it starts a web server that renders `.md` files as styled HTML and serves everything else as static files.

## Features

- Renders markdown files as HTML with GitHub-flavored styling (light + dark mode)
- Uses directories' `README.md` or `INDEX.md` as the index page
- Falls back to a file listing when no index markdown is present
- Serves non-markdown files as-is (images, PDFs, etc.)
- Custom CSS support to override the built-in stylesheet
- Breadcrumb navigation on all pages
- Single binary, zero config

## Install

```bash
go install github.com/ion1/mdview@latest
```

Or build from source:

```bash
git clone https://github.com/ion1/mdview.git
cd mdview
go build -o mdview .
```

## Usage

```bash
# Serve current directory on port 8080
mdview

# Serve a specific directory
mdview /path/to/docs

# Custom port
mdview --port 3000

# Custom CSS (replaces built-in stylesheet entirely)
mdview --css ./my-theme.css
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

Markdown rendering supports GFM features: tables, strikethrough, task lists, autolinks, and heading anchors.

## Flags

```
  --port int   Port to listen on (default 8080)
  --css path   Path to custom CSS file (replaces built-in stylesheet)
```

## Dependencies

- [goldmark](https://github.com/yuin/goldmark) — markdown rendering with GFM extension

## License

MIT
