// Package alerts renders GitHub/Obsidian-style admonition callouts:
//
//	> [!NOTE]
//	> Body text.
//
// It is a port of github.com/thiagokokada/goldmark-gh-alerts to goldmark v2,
// with the upstream "details" and "summary" packages merged into one. See
// LICENSE in this directory for the original MIT terms and copyright (c) 2024
// Adam Chovanec. The line-scanning logic in scanQuoteMarker derives from
// goldmark's own blockquote parser, originally written by Yusuke Inuzuka and
// also MIT licensed.
//
// Changes from upstream:
//   - Ported to the goldmark/v2 API: parser/renderer extensions split,
//     generic NodeRenderer, ast.TextBlock replaced by block source.
//   - The alert kind and the collapsed marker move from AST attributes to
//     struct fields. goldmark v2 attributes are text values only, and these
//     were a []byte and a bool; a typed field says what they are and drops the
//     interface round-trip the renderer used to do.
package alerts

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

// Icons maps a lowercase alert kind ("note", "warning", ...) to the SVG markup
// rendered before its title. A kind with no entry renders no icon.
type Icons map[string]string

// KindAlert is the node kind of an alert block.
var KindAlert = ast.NewNodeKind("Alert")

// Alert is the block node for a callout: the <div> wrapping title and body.
type Alert struct {
	ast.BaseBlock

	// AlertKind is the word inside the brackets, lowercased: "note", "tip",
	// "important", "warning", "caution" — or anything else the document used.
	AlertKind string

	// Closed reports whether the marker carried the collapsed suffix
	// ("> [!NOTE]-"). Parsed so the syntax is accepted; the HTML renderer
	// does not currently distinguish collapsed alerts.
	Closed bool
}

// Kind implements ast.Node.
func (n *Alert) Kind() ast.NodeKind { return KindAlert }

// Dump implements ast.Node.
func (n *Alert) Dump(_ []byte) *ast.NodeDump {
	return ast.NewNodeDump(n, map[string]any{
		"AlertKind": n.AlertKind,
		"Closed":    n.Closed,
	})
}

// NewAlert builds an alert block.
func NewAlert(kind string, closed bool) *Alert {
	n := &Alert{AlertKind: kind, Closed: closed}
	n.Init(n)
	return n
}

// KindAlertTitle is the node kind of an alert's title line.
var KindAlertTitle = ast.NewNodeKind("AlertTitle")

// AlertTitle is the block node for the title paragraph of an alert.
//
// When the document supplies its own title ("> [!NOTE] Custom"), that text is
// the node's block source and goldmark parses it into inline children;
// DefaultKind is empty and no icon is drawn. When there is no custom title,
// DefaultKind carries the alert kind and the renderer draws the icon plus the
// capitalized kind instead.
type AlertTitle struct {
	ast.BaseBlock

	DefaultKind string
}

// Kind implements ast.Node.
func (n *AlertTitle) Kind() ast.NodeKind { return KindAlertTitle }

// Dump implements ast.Node.
func (n *AlertTitle) Dump(_ []byte) *ast.NodeDump {
	return ast.NewNodeDump(n, map[string]any{"DefaultKind": n.DefaultKind})
}

// NewAlertTitle builds an alert title block.
func NewAlertTitle(defaultKind string) *AlertTitle {
	n := &AlertTitle{DefaultKind: defaultKind}
	n.Init(n)
	return n
}

// markerRe matches the alert marker that follows the blockquote ">":
// the kind in brackets, an optional "-" for the collapsed form, then either
// end-of-line or whitespace and a custom title.
var markerRe = regexp.MustCompile(`^\[!(?P<kind>\w+)\](?P<closed>-?)($|\s+(?P<title>.*))`)

// scanQuoteMarker reports whether the current line opens or continues a
// blockquote, and how far to advance past the ">" and its optional space.
//
// Derived from goldmark's blockquote parser (MIT, Yusuke Inuzuka): an alert is
// a blockquote whose first line carries a marker, so it has to agree with
// goldmark about where a blockquote line starts.
func scanQuoteMarker(reader text.Reader) (ok bool, advance int) {
	line, _ := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w > 3 || pos >= len(line) || line[pos] != '>' {
		return false, 0
	}

	advance = 1
	if pos+advance >= len(line) || line[pos+advance] == '\n' {
		return true, advance
	}
	if line[pos+advance] == ' ' || line[pos+advance] == '\t' {
		advance++
	}
	if line[pos+advance-1] == '\t' {
		reader.SetPadding(2)
	}
	return true, advance
}

// alertParser opens an alert for a blockquote whose first line carries a
// marker, and otherwise declines so goldmark's blockquote parser handles it.
type alertParser struct{}

var _ parser.BlockParser = (*alertParser)(nil)

func (b *alertParser) Trigger() []byte { return []byte{'>'} }

func (b *alertParser) Open(_ ast.Node, reader text.Reader, _ parser.Context) (ast.Node, parser.State) {
	ok, advance := scanQuoteMarker(reader)
	if !ok {
		return nil, parser.NoChildren
	}

	line, _ := reader.PeekLine()
	if len(line) <= advance {
		return nil, parser.NoChildren // empty blockquote
	}

	match := markerRe.FindSubmatch(line[advance:])
	if match == nil {
		return nil, parser.NoChildren // an ordinary blockquote
	}

	alert := NewAlert(strings.ToLower(string(match[1])), len(match[2]) != 0)

	// Stop on the "]" so the title parser, which triggers on that byte, picks
	// up the rest of the line.
	if i := strings.IndexByte(string(line), ']'); i >= 0 {
		reader.Advance(i)
	}
	return alert, parser.HasChildren
}

func (b *alertParser) Continue(_ ast.Node, reader text.Reader, _ parser.Context) parser.State {
	ok, advance := scanQuoteMarker(reader)
	if !ok {
		return parser.Close
	}
	reader.Advance(advance)
	return parser.Continue | parser.HasChildren
}

func (b *alertParser) Close(_ ast.Node, _ text.Reader, _ parser.Context) {}

func (b *alertParser) CanInterruptParagraph() bool { return true }

func (b *alertParser) CanAcceptIndentedLine() bool { return false }

// alertTitleParser consumes the remainder of the marker line as the alert's
// title. It only ever fires as the first child of an Alert.
type alertTitleParser struct{}

var _ parser.BlockParser = (*alertTitleParser)(nil)

func (b *alertTitleParser) Trigger() []byte { return []byte{']'} }

func (b *alertTitleParser) Open(parent ast.Node, reader text.Reader, _ parser.Context) (ast.Node, parser.State) {
	alert, ok := parent.(*Alert)
	if !ok || parent.ChildCount() != 0 {
		return nil, parser.NoChildren
	}

	reader.Advance(1) // "]"
	if reader.Peek() == '-' {
		reader.Advance(1) // collapsed marker
	}

	line, _ := reader.PeekLine()
	w, _ := util.IndentWidth(line, reader.LineOffset())
	reader.Advance(w)

	_, segment := reader.Position()
	line, _ = reader.PeekLine()
	if len(line) > 0 && line[len(line)-1] == '\n' {
		segment.Stop--
	}

	if segment.Len() == 0 {
		// No custom title: the renderer draws the icon and the kind.
		return NewAlertTitle(alert.AlertKind), parser.NoChildren
	}

	// A custom title is ordinary inline content. In v2 a block node's source
	// is what gets inline-parsed, so it goes straight on this node — v1 needed
	// a child ast.TextBlock, which no longer exists.
	title := NewAlertTitle("")
	title.AppendSource(segment)
	return title, parser.NoChildren
}

func (b *alertTitleParser) Continue(_ ast.Node, _ text.Reader, _ parser.Context) parser.State {
	return parser.Close
}

func (b *alertTitleParser) Close(_ ast.Node, _ text.Reader, _ parser.Context) {}

func (b *alertTitleParser) CanInterruptParagraph() bool { return false }

func (b *alertTitleParser) CanAcceptIndentedLine() bool { return true }

// alertRenderer renders the <div> wrapper.
type alertRenderer struct{}

func (r *alertRenderer) Render(w io.Writer, _ []byte, node ast.Node, entering bool, _ renderer.Context) (ast.WalkStatus, error) {
	n, ok := node.(*Alert)
	if !ok {
		return ast.WalkContinue, nil
	}
	bw, ok := w.(util.BufWriter)
	if !ok {
		return ast.WalkStop, fmt.Errorf("alerts: renderer needs a util.BufWriter, got %T", w)
	}

	if entering {
		_, _ = bw.WriteString(`<div class="markdown-alert markdown-alert-`)
		// The kind comes from the document, so it is escaped before it lands
		// in a class attribute. The marker regexp already restricts it to
		// word characters; this is belt and braces.
		_, _ = bw.Write(util.EscapeHTML([]byte(n.AlertKind)))
		_, _ = bw.WriteString(`">`)
	} else {
		_, _ = bw.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

// alertTitleRenderer renders the title paragraph, with an icon when the
// document did not supply its own title.
type alertTitleRenderer struct {
	icons Icons
}

func (r *alertTitleRenderer) Render(w io.Writer, _ []byte, node ast.Node, entering bool, _ renderer.Context) (ast.WalkStatus, error) {
	n, ok := node.(*AlertTitle)
	if !ok {
		return ast.WalkContinue, nil
	}
	bw, ok := w.(util.BufWriter)
	if !ok {
		return ast.WalkStop, fmt.Errorf("alerts: renderer needs a util.BufWriter, got %T", w)
	}

	if !entering {
		_, _ = bw.WriteString(`</p>`)
		return ast.WalkContinue, nil
	}

	_, _ = bw.WriteString(`<p class="markdown-alert-title">`)
	if n.DefaultKind != "" {
		// Icons are markup supplied by the host application, not by the
		// document, so they are written through verbatim.
		if icon, ok := r.icons[n.DefaultKind]; ok {
			_, _ = bw.WriteString(icon)
		}
		_, _ = bw.Write(util.EscapeHTML([]byte(capitalize(n.DefaultKind))))
	}
	return ast.WalkContinue, nil
}

// capitalize upper-cases the first letter of an alert kind ("note" -> "Note").
// The marker regexp limits kinds to ASCII word characters, so byte-wise is
// correct here and avoids strings.Title, which is deprecated.
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type parserExtension struct{}

// NewParser returns the alerts parser extension.
func NewParser() parser.Extension { return &parserExtension{} }

func (e *parserExtension) ParserOptions(_ *parser.Config) []parser.Option {
	return []parser.Option{
		parser.WithBlockParsers(
			// Above goldmark's blockquote parser (800) so a marked blockquote
			// becomes an alert before it becomes a blockquote.
			util.Prioritized[parser.BlockParser](&alertParser{}, 799),
			util.Prioritized[parser.BlockParser](&alertTitleParser{}, 799),
		),
	}
}

type htmlRendererExtension struct {
	icons Icons
}

// NewHTMLRenderer returns the alerts renderer extension. icons may be nil, in
// which case titles render without an icon.
func NewHTMLRenderer(icons Icons) html.Extension {
	return &htmlRendererExtension{icons: icons}
}

func (e *htmlRendererExtension) RendererOptions(_ *html.Config) []html.Option {
	return []html.Option{
		html.WithNodeRenderers(map[ast.NodeKind]html.NodeRenderer{
			KindAlert:      &alertRenderer{},
			KindAlertTitle: &alertTitleRenderer{icons: e.icons},
		}),
	}
}
