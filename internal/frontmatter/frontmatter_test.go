package frontmatter

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"gopkg.in/yaml.v2"
)

func run(t *testing.T, src string) (string, yaml.MapSlice) {
	t.Helper()

	p := parser.New(parser.WithExtensions(NewParser()))
	rend := html.New(html.WithExtensions(NewHTMLRenderer()))

	ctx := parser.NewContext()
	source := []byte(src)
	doc := p.Parse(source, parser.WithContext(ctx))

	var buf bytes.Buffer
	if err := rend.Render(&buf, source, doc); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String(), Items(ctx)
}

func TestParsesLeadingBlock(t *testing.T) {
	out, items := run(t, "---\ntitle: Hello\ncount: 3\n---\n\nBody.\n")

	if len(items) != 2 {
		t.Fatalf("items = %#v, want 2 entries", items)
	}
	// Document order has to survive, since the metadata table is built from it.
	if got := items[0].Key; got != "title" {
		t.Errorf("first key = %v, want title", got)
	}
	if got := items[1].Key; got != "count" {
		t.Errorf("second key = %v, want count", got)
	}
	if got := items[0].Value; got != "Hello" {
		t.Errorf("title = %v, want Hello", got)
	}

	// A block that parses is removed from the document entirely.
	if strings.Contains(out, "title:") {
		t.Errorf("front matter leaked into the output:\n%s", out)
	}
	if !strings.Contains(out, "<p>Body.</p>") {
		t.Errorf("body missing from the output:\n%s", out)
	}
}

func TestOnlyAtTopOfDocument(t *testing.T) {
	out, items := run(t, "Intro.\n\n---\ntitle: Nope\n---\n\nBody.\n")
	if items != nil {
		t.Errorf("items = %#v, want nil for a block that is not at the top", items)
	}
	// It is a thematic break plus a setext heading, not metadata.
	if !strings.Contains(out, "<hr>") {
		t.Errorf("expected a thematic break:\n%s", out)
	}
}

func TestNoFrontMatter(t *testing.T) {
	out, items := run(t, "# Heading\n\nBody.\n")
	if items != nil {
		t.Errorf("items = %#v, want nil", items)
	}
	if !strings.Contains(out, "<h1") {
		t.Errorf("document did not render normally:\n%s", out)
	}
}

// TestMalformedBlockStaysVisible pins the diagnostic behaviour: broken YAML is
// left in the document so the author sees it, with the parser error appended
// as an HTML comment.
func TestMalformedBlockStaysVisible(t *testing.T) {
	out, items := run(t, "---\ntitle: [unclosed\n---\n\nBody.\n")

	if items != nil {
		t.Errorf("items = %#v, want nil when the YAML does not parse", items)
	}
	if !strings.Contains(out, "title: [unclosed") {
		t.Errorf("the broken block should remain visible:\n%s", out)
	}
	if !strings.Contains(out, "<!-- yaml:") {
		t.Errorf("expected the parse error as a comment:\n%s", out)
	}
	if !strings.Contains(out, "-->") || !strings.Contains(out, "<p>Body.</p>") {
		t.Errorf("comment or body malformed:\n%s", out)
	}
}

// TestErrorCommentCannotBeClosedEarly covers the hardening added during the
// port. Upstream interpolated the YAML error into an HTML comment verbatim,
// and error text is partly derived from document content, so a message
// containing "-->" would end the comment and put the rest into the page.
func TestErrorCommentCannotBeClosedEarly(t *testing.T) {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf) // *bufio.Writer satisfies util.BufWriter
	node := NewError(`boom --> <script>alert(1)</script>`)

	if _, err := (&errorRenderer{}).Render(bw, nil, node, true, nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	_ = bw.Flush()

	out := buf.String()
	if strings.Count(out, "-->") != 1 {
		t.Errorf("the comment must close exactly once, got: %s", out)
	}
	if !strings.HasSuffix(out, " -->") {
		t.Errorf("the comment must close at the end, got: %s", out)
	}
}

func TestIsSeparator(t *testing.T) {
	for in, want := range map[string]bool{
		"---":     true,
		"  ---  ": true,
		"-":       true,
		"":        false,
		"   ":     false,
		"--- a":   false,
		"a---":    false,
	} {
		if got := isSeparator([]byte(in)); got != want {
			t.Errorf("isSeparator(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestItemsWithoutParse(t *testing.T) {
	// No parse has run, so there is nothing in the context.
	if got := Items(parser.NewContext()); got != nil {
		t.Errorf("Items on an empty context = %#v, want nil", got)
	}
}

func TestNodeDumpsAndKinds(t *testing.T) {
	b := NewBlock()
	if b.Kind() != KindBlock || b.Dump(nil) == nil {
		t.Error("Block Kind/Dump wrong")
	}
	e := NewError("msg")
	if e.Kind() != KindError || e.Dump(nil) == nil {
		t.Error("Error Kind/Dump wrong")
	}
}

func TestBlockParserFlags(t *testing.T) {
	b := &blockParser{}
	if b.CanInterruptParagraph() || b.CanAcceptIndentedLine() {
		t.Errorf("flags = (%v, %v), want (false, false)",
			b.CanInterruptParagraph(), b.CanAcceptIndentedLine())
	}
	if got := string(b.Trigger()); got != "-" {
		t.Errorf("Trigger() = %q, want %q", got, "-")
	}
}
