package wikilink

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
)

// mapResolver resolves the targets it knows and reports "not found" as a nil
// destination, which is what tells the renderer to drop the link.
type mapResolver map[string]string

func (m mapResolver) ResolveWikilink(n *Node) ([]byte, error) {
	dest, ok := m[string(n.Target)]
	if !ok {
		return nil, nil
	}
	if len(n.Fragment) > 0 {
		dest += "#" + string(n.Fragment)
	}
	return []byte(dest), nil
}

// errResolver fails every lookup, to check the error path halts rendering.
type errResolver struct{ err error }

func (e errResolver) ResolveWikilink(*Node) ([]byte, error) { return nil, e.err }

func render(t *testing.T, r Resolver, src string) (string, error) {
	t.Helper()

	p := parser.New(parser.WithExtensions(NewParser()))
	rend := html.New(html.WithExtensions(NewHTMLRenderer(r)))

	var buf bytes.Buffer
	source := []byte(src)
	err := rend.Render(&buf, source, p.Parse(source))
	return buf.String(), err
}

func TestParseAndRender(t *testing.T) {
	resolver := mapResolver{
		"notes":     "/notes.md",
		"photo.png": "/photo.png",
		"doc.pdf":   "/doc.pdf",
	}

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"plain", "[[notes]]", `<a href="/notes.md">notes</a>`},
		{"labelled", "[[notes|the notes]]", `<a href="/notes.md">the notes</a>`},
		{"fragment", "[[notes#head]]", `<a href="/notes.md#head">notes#head</a>`},
		{"labelled fragment", "[[notes#head|go]]", `<a href="/notes.md#head">go</a>`},

		// An embed of an image becomes <img>; the alt attribute only appears
		// when the label differs from the target.
		{"image embed", "![[photo.png]]", `<img src="/photo.png">`},
		{"image embed with label", "![[photo.png|A photo]]", `<img src="/photo.png" alt="A photo">`},
		// A non-image embed stays a link.
		{"non-image embed", "![[doc.pdf]]", `<a href="/doc.pdf">doc.pdf</a>`},

		// Unresolved links render their label and nothing else, so a missing
		// page never becomes a broken link.
		{"unresolved", "[[missing]]", `missing`},
		{"unresolved embed image", "![[missing.png]]", `missing.png`},

		// Not wikilinks at all.
		{"unterminated", "[[notes", `[[notes`},
		{"empty target", "[[]]", `[[]]`},
		{"empty label", "[[notes|]]", `[[notes|]]`},
		{"bare brackets", "[notes]", `[notes]`},
		{"closes on a later line", "[[notes\nmore]]", "[[notes\nmore]]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := render(t, resolver, tc.src)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("rendering %q:\n got: %s\nwant it to contain: %s", tc.src, got, tc.want)
			}
		})
	}
}

// TestRenderEscapesLabelAndURL checks the two places document text reaches the
// output: the alt attribute and the href.
func TestRenderEscapesLabelAndURL(t *testing.T) {
	got, err := render(t, mapResolver{`x.png`: `/a b"c.png`}, `![[x.png|"><script>alert(1)</script>]]`)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(got, "<script>") {
		t.Errorf("script tag survived into alt text: %s", got)
	}
	if strings.Contains(got, `/a b"c.png`) {
		t.Errorf("unescaped URL in src: %s", got)
	}
}

// TestResolverErrorHaltsRendering checks a resolver failure surfaces rather
// than rendering a half-formed link.
func TestResolverErrorHaltsRendering(t *testing.T) {
	sentinel := errors.New("index unavailable")
	_, err := render(t, errResolver{err: sentinel}, "[[notes]]")
	if !errors.Is(err, sentinel) {
		t.Fatalf("render error = %v, want it to wrap %v", err, sentinel)
	}
}

// TestNoStrayClosingTag is the regression test for replacing upstream's
// sync.Map with a recomputed opensAnchor: a node that does not open an <a>
// must not close one, and the two must stay in step when both kinds of link
// appear in one document.
func TestNoStrayClosingTag(t *testing.T) {
	got, err := render(t, mapResolver{"notes": "/notes.md", "photo.png": "/photo.png"},
		"[[missing]] ![[photo.png]] [[notes]] [[missing]]")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if opens, closes := strings.Count(got, "<a href="), strings.Count(got, "</a>"); opens != closes {
		t.Errorf("%d <a> opened but %d closed: %s", opens, closes, got)
	}
}

func TestNodeDumpAndKind(t *testing.T) {
	n := NewNode([]byte("target"), []byte("frag"), true)
	if n.Kind() != Kind {
		t.Errorf("Kind() = %v, want %v", n.Kind(), Kind)
	}
	if n.Dump(nil) == nil {
		t.Error("Dump returned nil")
	}
	// Init must have run, or the argument-free tree mutators panic.
	if n.Parent() != nil {
		t.Error("fresh node should have no parent")
	}
}

func TestResolveAsImage(t *testing.T) {
	for _, tc := range []struct {
		target string
		embed  bool
		want   bool
	}{
		{"a.png", true, true},
		{"a.JPG", true, false}, // extension match is case-sensitive, as upstream
		{"a.jpg", true, true},
		{"a.svg", true, true},
		{"a.webp", true, true},
		{"a.pdf", true, false},
		{"a", true, false},
		{"a.png", false, false}, // not an embed
	} {
		if got := resolveAsImage(&Node{Target: []byte(tc.target), Embed: tc.embed}); got != tc.want {
			t.Errorf("resolveAsImage(%q, embed=%v) = %v, want %v", tc.target, tc.embed, got, tc.want)
		}
	}
}
