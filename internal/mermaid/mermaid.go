// Package mermaid turns ```mermaid fenced code blocks into markup that a
// client-side mermaid bundle renders into diagrams.
//
// It is a trimmed port of go.abhg.dev/goldmark/mermaid v0.6.0 to goldmark v2.
// See LICENSE in this directory for the original BSD-3-Clause terms and
// copyright (c) 2023 Abhinav Gupta.
//
// Changes from upstream:
//   - Ported to the goldmark/v2 API: parser/renderer extensions split, generic
//     NodeRenderer, and ast.FencedCodeBlock folded into ast.CodeBlock.
//   - Server-side rendering is gone. Upstream can shell out to the mermaid CLI
//     or drive a headless Chrome through chromedp to rasterize diagrams at
//     build time; markbrowse always renders in the browser, so the RenderMode
//     switch, the CLI and chromedp backends, and the chromedp dependency tree
//     go with it — about 85% of the upstream package.
//   - The <script> tag that pulls mermaid from a CDN is gone too. markbrowse
//     embeds its own bundle and emits the tag from its page template, which is
//     what upstream's NoScript option was set to suppress.
package mermaid

import (
	"fmt"
	"io"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

// Kind is the node kind of a mermaid diagram block.
var Kind = ast.NewNodeKind("MermaidBlock")

// Block is a mermaid diagram: the body of a ```mermaid fenced code block,
// held verbatim for the client-side renderer to consume.
type Block struct {
	ast.BaseBlock

	// Value is the diagram source, line by line, exactly as written.
	Value text.Lines
}

// Kind implements ast.Node.
func (n *Block) Kind() ast.NodeKind { return Kind }

// Dump implements ast.Node.
func (n *Block) Dump(_ []byte) *ast.NodeDump {
	return ast.NewNodeDump(n, nil)
}

// NewBlock builds a mermaid block from the lines of a fenced code block.
func NewBlock(value text.Lines) *Block {
	n := &Block{Value: value}
	n.Init(n)
	return n
}

const mermaidLang = "mermaid"

// transformer replaces ```mermaid code blocks with Block nodes.
type transformer struct{}

var _ parser.ASTTransformer = (*transformer)(nil)

// Transform walks the document and swaps every mermaid-tagged fenced code
// block for a Block node.
func (t *transformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()

	// Collect first, mutate second: replacing nodes while walking the tree
	// would disturb the walk.
	var blocks []*ast.CodeBlock
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cb, ok := node.(*ast.CodeBlock)
		// v2 merged FencedCodeBlock into CodeBlock, so the fenced kind has to
		// be checked explicitly: an indented block has no info string and must
		// never be treated as a diagram.
		if !ok || cb.CodeBlockKind != ast.CodeBlockKindFenced {
			return ast.WalkContinue, nil
		}
		if lang, found := cb.Language(source); !found || lang != mermaidLang {
			return ast.WalkContinue, nil
		}
		blocks = append(blocks, cb)
		return ast.WalkContinue, nil
	})

	for _, cb := range blocks {
		parent := cb.Parent()
		if parent == nil {
			continue
		}
		b := NewBlock(cb.Value)
		// A replacement node inherits neither of these, and losing them
		// changes spacing and source mapping.
		b.SetPos(cb.Pos())
		b.SetBlankPreviousLines(cb.HasBlankPreviousLines())
		parent.ReplaceChild(cb, b)
	}
}

// nodeRenderer renders a Block as <pre class="mermaid">, which is what the
// bundled mermaid script looks for.
type nodeRenderer struct{}

func (r *nodeRenderer) Render(w io.Writer, source []byte, node ast.Node, entering bool, rc renderer.Context) (ast.WalkStatus, error) {
	n, ok := node.(*Block)
	if !ok {
		return ast.WalkContinue, nil
	}
	bw, ok := w.(util.BufWriter)
	if !ok {
		return ast.WalkStop, fmt.Errorf("mermaid: renderer needs a util.BufWriter, got %T", w)
	}

	if !entering {
		_, _ = bw.WriteString("</pre>")
		return ast.WalkContinue, nil
	}

	_, _ = bw.WriteString(`<pre class="mermaid">`)
	// Diagram source is untrusted document content going into HTML text, so it
	// goes through the context text writer, which escapes & < > and ".
	_, _ = n.Value.WriteTo(html.ContextTextWriter(rc), source)
	return ast.WalkContinue, nil
}

type parserExtension struct{}

// NewParser returns the mermaid parser extension.
func NewParser() parser.Extension { return &parserExtension{} }

func (e *parserExtension) ParserOptions(_ *parser.Config) []parser.Option {
	return []parser.Option{
		parser.WithASTTransformers(util.Prioritized[parser.ASTTransformer](&transformer{}, 100)),
	}
}

type htmlRendererExtension struct{}

// NewHTMLRenderer returns the mermaid renderer extension.
func NewHTMLRenderer() html.Extension { return &htmlRendererExtension{} }

func (e *htmlRendererExtension) RendererOptions(_ *html.Config) []html.Option {
	return []html.Option{
		html.WithNodeRenderers(map[ast.NodeKind]html.NodeRenderer{
			Kind: &nodeRenderer{},
		}),
	}
}
