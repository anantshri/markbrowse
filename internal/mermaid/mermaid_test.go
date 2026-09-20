package mermaid

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

func pipeline() (parser.Parser, html.Renderer) {
	return parser.New(parser.WithExtensions(NewParser())),
		html.New(html.WithExtensions(NewHTMLRenderer()))
}

func render(t *testing.T, src string) string {
	t.Helper()

	p, rend := pipeline()
	var buf bytes.Buffer
	source := []byte(src)
	if err := rend.Render(&buf, source, p.Parse(source)); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestMermaidBlocks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		src      string
		contains []string
		omits    []string
	}{
		{
			name:     "fenced mermaid becomes a diagram container",
			src:      "```mermaid\ngraph TD;\n  A-->B;\n```",
			contains: []string{`<pre class="mermaid">`, "graph TD;", "</pre>"},
			omits:    []string{"<code"},
		},
		{
			name:     "another language is left as code",
			src:      "```go\nfmt.Println(1)\n```",
			contains: []string{`<code class="language-go">`},
			omits:    []string{`class="mermaid"`},
		},
		{
			name:     "a fence with no language is left as code",
			src:      "```\nplain\n```",
			contains: []string{"<code>"},
			omits:    []string{`class="mermaid"`},
		},
		{
			// An indented code block has no info string at all, so it must
			// never be mistaken for a diagram.
			name:     "indented code is left alone",
			src:      "    mermaid\n    graph TD;",
			contains: []string{"<code>"},
			omits:    []string{`class="mermaid"`},
		},
		{
			name:     "the language must match exactly",
			src:      "```mermaidjs\ngraph TD;\n```",
			omits:    []string{`class="mermaid"`},
			contains: []string{"language-mermaidjs"},
		},
		{
			name:     "several diagrams in one document",
			src:      "```mermaid\nA\n```\n\ntext\n\n```mermaid\nB\n```",
			contains: []string{"A", "B", "<p>text</p>"},
		},
		{
			name:     "an empty diagram still renders its container",
			src:      "```mermaid\n```",
			contains: []string{`<pre class="mermaid"></pre>`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := render(t, tc.src)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q:\n%s", want, got)
				}
			}
			for _, unwanted := range tc.omits {
				if strings.Contains(got, unwanted) {
					t.Errorf("output should not contain %q:\n%s", unwanted, got)
				}
			}
		})
	}
}

// TestDiagramSourceIsEscaped checks diagram text is treated as untrusted
// document content on its way into HTML, not written through raw.
func TestDiagramSourceIsEscaped(t *testing.T) {
	got := render(t, "```mermaid\n</pre><script>alert(1)</script>\n```")
	if strings.Contains(got, "<script>") {
		t.Errorf("script tag survived into the diagram container:\n%s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected the diagram source to be escaped:\n%s", got)
	}
}

// TestTransformPreservesNodeMetadata covers the two properties a replacement
// node does not inherit for free. Losing SetPos breaks source mapping; losing
// the blank-line flag changes spacing in list contexts.
func TestTransformPreservesNodeMetadata(t *testing.T) {
	p, _ := pipeline()
	source := []byte("intro\n\n```mermaid\nA\n```")
	doc := p.Parse(source)

	var block *Block
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if b, ok := n.(*Block); ok && entering {
			block = b
		}
		return ast.WalkContinue, nil
	})
	if block == nil {
		t.Fatal("no mermaid block in the parsed document")
	}
	if block.Pos() < 0 {
		t.Errorf("Pos() = %d, want the position copied from the code block", block.Pos())
	}
	if !block.HasBlankPreviousLines() {
		t.Error("the blank line before the fence was not carried over")
	}
	if len(block.Value.Segments()) == 0 {
		t.Error("the diagram source was not carried over")
	}
}

func TestNodeDumpAndKind(t *testing.T) {
	var lines text.Lines
	n := NewBlock(lines)
	if n.Kind() != Kind {
		t.Errorf("Kind() = %v, want %v", n.Kind(), Kind)
	}
	if n.Dump(nil) == nil {
		t.Error("Dump returned nil")
	}
}
