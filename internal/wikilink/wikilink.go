// Package wikilink parses and renders `[[wiki link]]` syntax.
//
// It is a trimmed port of go.abhg.dev/goldmark/wikilink v0.6.0 to goldmark v2.
// See LICENSE in this directory for the original BSD-3-Clause terms and
// copyright (c) 2023 Abhinav Gupta.
//
// Changes from upstream:
//   - Ported to the goldmark/v2 API: split Extender into separate parser and
//     renderer extensions, generic NodeRenderer, text.Value node fields.
//   - DefaultResolver and its ".html" suffix logic are dropped. markbrowse
//     resolves targets against its own file index, and an unresolved link
//     renders as plain text rather than pointing at a page that is not there.
package wikilink

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

// Kind is the kind of the wikilink AST node.
var Kind = ast.NewNodeKind("WikiLink")

// Node is a wikilink AST node.
//
// Wikilinks have two components: the target and the label. In [[Foo bar]] the
// two are the same; in [[Foo bar|baz qux]] the target is left of the "|" and
// the label is to the right. A target may carry a fragment: [[Foo#Bar]].
//
// Target and Fragment are plain []byte rather than text.Value because a
// Resolver matches them against a file index, which wants bytes it can look up
// directly, not a source-backed value that has to be materialised first.
type Node struct {
	ast.BaseInline

	// Page this wikilink points to. May be empty for same-document links
	// such as [[#Foo]].
	Target []byte

	// Fragment portion of the link, if any: the part after "#".
	Fragment []byte

	// Embed reports whether the link started with a bang, as in ![[foo.png]],
	// meaning the resource should be embedded rather than linked.
	Embed bool
}

var _ ast.Node = (*Node)(nil)

// Kind reports the kind of this node.
func (n *Node) Kind() ast.NodeKind { return Kind }

// Dump implements ast.Node.
func (n *Node) Dump(_ []byte) *ast.NodeDump {
	return ast.NewNodeDump(n, map[string]any{
		"Target":   string(n.Target),
		"Fragment": string(n.Fragment),
		"Embed":    n.Embed,
	})
}

// NewNode builds a wikilink node. Init is required by goldmark v2: BaseNode
// keeps a self reference so the tree-mutation methods can drop their explicit
// self argument.
func NewNode(target, fragment []byte, embed bool) *Node {
	n := &Node{Target: target, Fragment: fragment, Embed: embed}
	n.Init(n)
	return n
}

// Resolver resolves the page a wikilink points to.
type Resolver interface {
	// ResolveWikilink returns the destination for a wikilink. The result is
	// URL-escaped before being written into the document.
	//
	// A non-nil error halts rendering. A nil destination and nil error tells
	// the renderer to drop the link and render only its text.
	ResolveWikilink(*Node) (destination []byte, err error)
}

var (
	openBracket  = []byte("[[")
	embedOpen    = []byte("![[")
	pipe         = []byte{'|'}
	hash         = []byte{'#'}
	closeBracket = []byte("]]")
)

// inlineParser parses wikilinks. Its priority must be below the 200 used by
// goldmark's own link parser so that the "[" trigger reaches us first.
type inlineParser struct{}

var _ parser.InlineParser = (*inlineParser)(nil)

// Trigger returns the bytes that activate this parser.
func (p *inlineParser) Trigger() []byte { return []byte{'!', '['} }

// Parse parses [[target]], [[target|label]], [[target#fragment]] and their
// ![[...]] embedding forms. A wikilink must open and close on one line.
func (p *inlineParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, seg := block.PeekLine()
	stop := bytes.Index(line, closeBracket)
	if stop < 0 {
		return nil // must close on the same line
	}

	var embed bool
	switch {
	case bytes.HasPrefix(line, embedOpen):
		embed = true
		seg = text.NewSegment(seg.Start+len(embedOpen), seg.Start+stop)
	case bytes.HasPrefix(line, openBracket):
		seg = text.NewSegment(seg.Start+len(openBracket), seg.Start+stop)
	default:
		return nil
	}

	target := seg.Bytes(block.Source())
	if idx := bytes.Index(target, pipe); idx >= 0 {
		target = target[:idx]                    // [[ ... |
		seg = seg.WithStart(seg.Start + idx + 1) // | ... ]]
	}

	if len(target) == 0 || seg.Len() == 0 {
		return nil // target and label must not be empty
	}

	// A target may be Foo#Bar, so split the fragment off.
	var fragment []byte
	if idx := bytes.LastIndex(target, hash); idx >= 0 {
		fragment = target[idx+1:] // Foo#Bar => Bar
		target = target[:idx]     // Foo#Bar => Foo
	}

	n := NewNode(target, fragment, embed)
	// The label is ordinary inline text, so it is decoded with the reader's
	// decoder (escapes, entity references) rather than IdentityDecoder.
	n.AppendChild(ast.NewText(text.NewSingleLineValueFromSegment(seg, block.Decoder())))
	block.Advance(stop + len(closeBracket))
	return n
}

// nodeRenderer renders wikilinks as HTML.
type nodeRenderer struct {
	resolver Resolver
}

// Render renders a wikilink. Everything becomes an <a>, except an embed link
// to an image, which becomes an <img>.
func (r *nodeRenderer) Render(w io.Writer, src []byte, node ast.Node, entering bool, rc renderer.Context) (ast.WalkStatus, error) {
	n, ok := node.(*Node)
	if !ok {
		return ast.WalkContinue, nil
	}
	bw, ok := w.(util.BufWriter)
	if !ok {
		return ast.WalkStop, fmt.Errorf("wikilink: renderer needs a util.BufWriter, got %T", w)
	}

	if !entering {
		// Only a link that actually opened an <a> closes one. An unresolved
		// link and an <img> both leave nothing to close.
		if r.opensAnchor(n) {
			_, _ = bw.WriteString("</a>")
		}
		return ast.WalkContinue, nil
	}

	dest, err := r.resolver.ResolveWikilink(n)
	if err != nil {
		return ast.WalkStop, fmt.Errorf("resolve %q: %w", n.Target, err)
	}
	if len(dest) == 0 {
		// Unresolved: render the label and nothing else.
		return ast.WalkContinue, nil
	}

	if !resolveAsImage(n) {
		_, _ = bw.WriteString(`<a href="`)
		_, _ = bw.Write(util.URLEscape(dest))
		_, _ = bw.WriteString(`">`)
		return ast.WalkContinue, nil
	}

	_, _ = bw.WriteString(`<img src="`)
	_, _ = bw.Write(util.URLEscape(dest))
	// The label becomes alt text only when it differs from the target, so
	// [[foo.jpg]] does not gain alt="foo.jpg" but [[foo.jpg|bar]] gains
	// alt="bar".
	if n.ChildCount() == 1 {
		if label := nodeText(src, n.FirstChild()); !bytes.Equal(label, n.Target) {
			_, _ = bw.WriteString(`" alt="`)
			_, _ = bw.Write(util.EscapeHTML(label))
		}
	}
	_, _ = bw.WriteString(`">`)
	return ast.WalkSkipChildren, nil
}

// opensAnchor reports whether the entering pass wrote an <a> for this node.
//
// Upstream tracked this in a sync.Map keyed by node pointer, populated on
// enter and drained on exit. Re-deriving it is cheaper and, more to the point,
// stateless: a renderer that keeps per-node state cannot be shared safely
// across concurrent renders, and markbrowse renders on every request.
func (r *nodeRenderer) opensAnchor(n *Node) bool {
	if resolveAsImage(n) {
		return false
	}
	dest, err := r.resolver.ResolveWikilink(n)
	return err == nil && len(dest) > 0
}

// resolveAsImage reports whether an embed link points at an image.
func resolveAsImage(n *Node) bool {
	if !n.Embed {
		return false
	}
	switch filepath.Ext(string(n.Target)) {
	// Common image file types, per
	// https://developer.mozilla.org/en-US/docs/Web/Media/Formats/Image_types
	case ".apng", ".avif", ".gif", ".jpg", ".jpeg", ".jfif", ".pjpeg", ".pjp", ".png", ".svg", ".webp":
		return true
	default:
		return false
	}
}

// nodeText flattens a node's text content, used for an <img> alt attribute.
func nodeText(src []byte, n ast.Node) []byte {
	var buf bytes.Buffer
	writeNodeText(src, &buf, n)
	return buf.Bytes()
}

func writeNodeText(src []byte, dst io.Writer, n ast.Node) {
	// ast.String is gone in v2; every literal is an ast.Text carrying a
	// text.Value, so one case covers both of the v1 branches.
	if t, ok := n.(*ast.Text); ok {
		_, _ = io.WriteString(dst, t.Value.Value(src))
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(src, dst, c)
	}
}

type parserExtension struct{}

// NewParser returns the wikilink parser extension.
func NewParser() parser.Extension { return &parserExtension{} }

func (e *parserExtension) ParserOptions(_ *parser.Config) []parser.Option {
	return []parser.Option{
		// Below goldmark's link parser (200) so the "[" trigger reaches us.
		parser.WithInlineParsers(util.Prioritized[parser.InlineParser](&inlineParser{}, 199)),
	}
}

type htmlRendererExtension struct {
	resolver Resolver
}

// NewHTMLRenderer returns the wikilink renderer extension. The resolver maps a
// wikilink target to a destination and must not be nil.
func NewHTMLRenderer(resolver Resolver) html.Extension {
	return &htmlRendererExtension{resolver: resolver}
}

func (e *htmlRendererExtension) RendererOptions(_ *html.Config) []html.Option {
	return []html.Option{
		html.WithNodeRenderers(map[ast.NodeKind]html.NodeRenderer{
			Kind: &nodeRenderer{resolver: e.resolver},
		}),
	}
}
