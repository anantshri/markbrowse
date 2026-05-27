package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	alertcallouts "github.com/zmtcreative/gm-alert-callouts"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"
)

type markdownConverter struct {
	gm goldmark.Markdown
}

func newMarkdownConverter(rootDir string) *markdownConverter {
	idx := buildFileIndex(rootDir)

	gm := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&mermaid.Extender{NoScript: true},
			alertcallouts.NewAlertCallouts(),
			&wikilink.Extender{
				Resolver: &wikilinkResolver{idx: idx},
			},
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	return &markdownConverter{gm: gm}
}

func (m *markdownConverter) convert(source []byte) (string, error) {
	var buf bytes.Buffer
	if err := m.gm.Convert(source, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// fileIndex maps lowercase file stems to their URL paths.
type fileIndex map[string]string

func buildFileIndex(rootDir string) fileIndex {
	idx := make(fileIndex)
	filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
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
	idx fileIndex
}

func (r *wikilinkResolver) ResolveWikilink(n *wikilink.Node) ([]byte, error) {
	target := strings.ToLower(string(n.Target))

	// Try stem match first (e.g., "notes" -> "notes.md")
	if p, ok := r.idx[target]; ok {
		return r.buildURL(p, n.Fragment), nil
	}

	// Try with .md extension
	if p, ok := r.idx[target+".md"]; ok {
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
