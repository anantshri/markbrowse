package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync"

	alerts "github.com/thiagokokada/goldmark-gh-alerts"
	ghsummary "github.com/thiagokokada/goldmark-gh-alerts/summary"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"
	"gopkg.in/yaml.v2"
)

// alertIcons maps each GitHub alert kind to its octicon SVG, mirroring GitHub's
// rendered output. The octicon/mr-2 classes hook into the bundled stylesheet so
// the icon inherits the title color (fill:currentColor) and gets right margin.
var alertIcons = ghsummary.Icons{
	"note":      `<svg class="octicon octicon-info mr-2" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Zm8-6.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM6.5 7.75A.75.75 0 0 1 7.25 7h1a.75.75 0 0 1 .75.75v2.75h.25a.75.75 0 0 1 0 1.5h-2a.75.75 0 0 1 0-1.5h.25v-2h-.25a.75.75 0 0 1-.75-.75ZM8 6a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>`,
	"tip":       `<svg class="octicon octicon-light-bulb mr-2" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.211c-.04-.282-.163-.547-.37-.847a8.456 8.456 0 0 0-.542-.68c-.084-.1-.173-.205-.268-.32C3.201 7.75 2.5 6.766 2.5 5.25 2.5 2.31 4.863 0 8 0s5.5 2.31 5.5 5.25c0 1.516-.701 2.5-1.328 3.259-.095.115-.184.22-.268.319-.207.245-.383.453-.541.681-.208.3-.33.565-.37.847a.751.751 0 0 1-1.485-.212c.084-.593.337-1.078.621-1.489.203-.292.45-.584.673-.848.075-.088.147-.173.213-.253.561-.679.985-1.32.985-2.304 0-2.06-1.637-3.75-4-3.75ZM5.75 12h4.5a.75.75 0 0 1 0 1.5h-4.5a.75.75 0 0 1 0-1.5ZM6 15.25a.75.75 0 0 1 .75-.75h2.5a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Z"/></svg>`,
	"important": `<svg class="octicon octicon-report mr-2" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M0 1.75C0 .784.784 0 1.75 0h12.5C15.216 0 16 .784 16 1.75v9.5A1.75 1.75 0 0 1 14.25 13H8.06l-2.573 2.573A1.458 1.458 0 0 1 3 14.543V13H1.75A1.75 1.75 0 0 1 0 11.25Zm1.75-.25a.25.25 0 0 0-.25.25v9.5c0 .138.112.25.25.25h2a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h6.5a.25.25 0 0 0 .25-.25v-9.5a.25.25 0 0 0-.25-.25Zm7 2.25v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 9a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"/></svg>`,
	"warning":   `<svg class="octicon octicon-alert mr-2" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M6.457 1.047c.659-1.234 2.427-1.234 3.086 0l6.082 11.378A1.75 1.75 0 0 1 14.082 15H1.918a1.75 1.75 0 0 1-1.543-2.575Zm1.763.707a.25.25 0 0 0-.44 0L1.698 13.132a.25.25 0 0 0 .22.368h12.164a.25.25 0 0 0 .22-.368Zm.53 3.996v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 11a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"/></svg>`,
	"caution":   `<svg class="octicon octicon-stop mr-2" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M4.47.22A.749.749 0 0 1 5 0h6c.199 0 .389.079.53.22l4.25 4.25c.141.14.22.331.22.53v6a.749.749 0 0 1-.22.53l-4.25 4.25A.749.749 0 0 1 11 16H5a.749.749 0 0 1-.53-.22L.22 11.53A.749.749 0 0 1 0 11V5c0-.199.079-.389.22-.53Zm.84 1.28L1.5 5.31v5.38l3.81 3.81h5.38l3.81-3.81V5.31L10.69 1.5ZM8 4a.75.75 0 0 1 .75.75v3.5a.75.75 0 0 1-1.5 0v-3.5A.75.75 0 0 1 8 4Zm0 8a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>`,
}

type markdownConverter struct {
	gm goldmark.Markdown
}

func newMarkdownConverter(rootDir string) *markdownConverter {
	gm := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&mermaid.Extender{NoScript: true},
			&alerts.GhAlerts{Icons: alertIcons},
			&wikilink.Extender{
				Resolver: &wikilinkResolver{rootDir: rootDir},
			},
			// Registers the YAML front matter parser only; the table is
			// rendered by renderMetaTable below so we control the layout
			// (GitHub-style rows) instead of the extension's column layout.
			meta.New(),
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(ghtml.WithUnsafe()),
	)
	return &markdownConverter{gm: gm}
}

func (m *markdownConverter) convert(source []byte) (string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	ctx := parser.NewContext()
	if err := m.gm.Convert(source, buf, parser.WithContext(ctx)); err != nil {
		return "", err
	}
	body := buf.String()

	// Prepend a GitHub-style table for YAML front matter, if any. The items
	// form preserves the order keys appear in the document.
	table := renderMetaTable(meta.GetItems(ctx))
	if table != "" {
		body = table + "\n" + body
	}
	return body, nil
}

// renderMetaTable renders parsed front matter as the key/value table GitHub
// produces: one row per key, key in <th>, value in <td>, first pair in
// <thead>. YAML errors fall back to the unordered map (GetItems is nil when
// the second Unmarshal failed); an empty result renders nothing.
func renderMetaTable(items yaml.MapSlice) string {
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<table class="meta-table">`)
	for i, item := range items {
		if i == 0 {
			b.WriteString("<thead>")
		} else if i == 1 {
			b.WriteString("<tbody>")
		}
		b.WriteString("<tr><th>")
		b.WriteString(html.EscapeString(fmt.Sprint(item.Key)))
		b.WriteString("</th><td>")
		b.WriteString(renderMetaValue(item.Value))
		b.WriteString("</td></tr>")
		if i == 0 {
			b.WriteString("</thead>")
		}
	}
	if len(items) > 1 {
		b.WriteString("</tbody>")
	}
	b.WriteString("</table>")
	return b.String()
}

// renderMetaValue stringifies a front matter value: scalars verbatim
// (escaped), slices one per line.
func renderMetaValue(v interface{}) string {
	switch val := v.(type) {
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, html.EscapeString(fmt.Sprint(item)))
		}
		return strings.Join(parts, "<br>")
	case string:
		return html.EscapeString(val)
	default:
		return html.EscapeString(fmt.Sprint(val))
	}
}

// metaTitle returns the document title from front matter, if present.
func metaTitle(items yaml.MapSlice) string {
	for _, item := range items {
		if key, ok := item.Key.(string); ok && strings.EqualFold(key, "title") {
			if s, ok := item.Value.(string); ok {
				return s
			}
			return fmt.Sprint(item.Value)
		}
	}
	return ""
}

// metaTitleOf parses just enough of source to pull the front matter title,
// without running a full render. Returns "" when absent.
func (m *markdownConverter) metaTitleOf(source []byte) string {
	ctx := parser.NewContext()
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	if err := m.gm.Convert(source, buf, parser.WithContext(ctx)); err != nil {
		return ""
	}
	return metaTitle(meta.GetItems(ctx))
}

// bufPool reuses the scratch buffer used for markdown rendering. Rendering is
// a per-request hot path, so avoiding an allocation on every page view adds
// up quickly under load.
var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// fileIndex maps lowercase file stems to their URL paths.
type fileIndex map[string]string

func buildFileIndex(rootDir string) fileIndex {
	idx := make(fileIndex)
	filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Dot directories are indexed too, mirroring serveTreeJSON, so
		// wikilinks resolve consistently with what the sidebar shows.
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			return nil
		}
		urlPath := "/" + filepath.ToSlash(rel)

		// Index by stem (e.g., "notes" for "notes.md")
		ext := filepath.Ext(d.Name())
		stem := strings.ToLower(strings.TrimSuffix(d.Name(), ext))
		idx[stem] = urlPath

		// Also index with extension (e.g., "notes.md")
		idx[strings.ToLower(d.Name())] = urlPath

		return nil
	}) // #nosec G104 -- best-effort index, errors handled in walk func
	return idx
}

type wikilinkResolver struct {
	rootDir string

	once sync.Once
	idx  fileIndex
}

// index returns the file index, building it lazily on first use. The index
// walks the whole tree, which is expensive on large directories, so it is
// deferred until a [[wikilink]] actually needs resolution.
func (r *wikilinkResolver) index() fileIndex {
	r.once.Do(func() {
		r.idx = buildFileIndex(r.rootDir)
	})
	return r.idx
}

func (r *wikilinkResolver) ResolveWikilink(n *wikilink.Node) ([]byte, error) {
	idx := r.index()
	target := strings.ToLower(string(n.Target))

	// Try stem match first (e.g., "notes" -> "notes.md")
	if p, ok := idx[target]; ok {
		return r.buildURL(p, n.Fragment), nil
	}

	// Try with .md extension
	if p, ok := idx[target+".md"]; ok {
		return r.buildURL(p, n.Fragment), nil
	}

	// Not found: nil means render as plain text
	return nil, nil
}

func (r *wikilinkResolver) buildURL(path string, fragment []byte) []byte {
	url := path
	if len(fragment) > 0 {
		url += "#" + string(fragment)
	}
	return []byte(url)
}
