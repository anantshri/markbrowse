package alerts

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
)

var testIcons = Icons{
	"note":    `<svg id="note"></svg>`,
	"warning": `<svg id="warning"></svg>`,
}

func render(t *testing.T, src string) string {
	t.Helper()

	p := parser.New(parser.WithExtensions(NewParser()))
	rend := html.New(html.WithExtensions(NewHTMLRenderer(testIcons)))

	var buf bytes.Buffer
	source := []byte(src)
	if err := rend.Render(&buf, source, p.Parse(source)); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestAlertKinds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		src      string
		contains []string
		omits    []string
	}{
		{
			name:     "known kind draws its icon and capitalized name",
			src:      "> [!NOTE]\n> Body.",
			contains: []string{`class="markdown-alert markdown-alert-note"`, `<svg id="note">`, `>Note</p>`, "Body."},
		},
		{
			name:     "kind is lowercased for the class and the icon lookup",
			src:      "> [!WaRnInG]\n> Body.",
			contains: []string{`markdown-alert-warning`, `<svg id="warning">`, `>Warning</p>`},
		},
		{
			name:     "unknown kind renders without an icon",
			src:      "> [!NOPE]\n> Body.",
			contains: []string{`markdown-alert-nope`, `>Nope</p>`},
			omits:    []string{"<svg"},
		},
		{
			// A custom title replaces both the icon and the kind name.
			name:     "custom title",
			src:      "> [!NOTE] Look here\n> Body.",
			contains: []string{`markdown-alert-note`, `>Look here</p>`},
			omits:    []string{"<svg", ">Note</p>"},
		},
		{
			name:     "custom title is inline-parsed",
			src:      "> [!NOTE] With **bold** text\n> Body.",
			contains: []string{"<strong>bold</strong>"},
		},
		{
			name:     "collapsed marker is accepted",
			src:      "> [!NOTE]-\n> Body.",
			contains: []string{`markdown-alert-note`, `>Note</p>`},
		},
		{
			name:     "collapsed marker with a custom title",
			src:      "> [!NOTE]- Collapsed title\n> Body.",
			contains: []string{`>Collapsed title</p>`},
		},
		{
			// Without a marker it is an ordinary blockquote and goldmark's own
			// parser must get it.
			name:     "plain blockquote is untouched",
			src:      "> Just a quote.",
			contains: []string{"<blockquote>"},
			omits:    []string{"markdown-alert"},
		},
		{
			name:     "marker needs the brackets",
			src:      "> !NOTE\n> Body.",
			contains: []string{"<blockquote>"},
			omits:    []string{"markdown-alert"},
		},
		{
			name:     "empty blockquote is not an alert",
			src:      ">",
			contains: []string{"<blockquote>"},
			omits:    []string{"markdown-alert"},
		},
		{
			name:     "body spans multiple lines",
			src:      "> [!TIP]\n> First.\n> Second.",
			contains: []string{"First.", "Second."},
		},
		{
			name:     "alert interrupts a paragraph",
			src:      "Some text.\n> [!NOTE]\n> Body.",
			contains: []string{"<p>Some text.</p>", "markdown-alert-note"},
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

// TestAlertKindIsEscaped covers the hardening added during the port: upstream
// interpolated the kind straight into the class attribute with fmt.Sprintf.
// The marker regexp already limits it to word characters, so this guards the
// invariant rather than a known escape.
func TestAlertKindIsEscaped(t *testing.T) {
	n := NewAlert(`"><script>alert(1)</script>`, false)
	if n.Kind() != KindAlert {
		t.Fatalf("Kind() = %v, want %v", n.Kind(), KindAlert)
	}

	var buf bytes.Buffer
	// *bufio.Writer satisfies util.BufWriter.
	bw := bufio.NewWriter(&buf)
	if _, err := (&alertRenderer{}).Render(bw, nil, n, true, nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	_ = bw.Flush()
	if strings.Contains(buf.String(), "<script>") {
		t.Errorf("alert kind was not escaped into the class attribute: %s", buf.String())
	}
}

func TestCapitalize(t *testing.T) {
	for in, want := range map[string]string{
		"note": "Note",
		"n":    "N",
		"":     "",
		"NOTE": "NOTE",
		"1st":  "1st",
	} {
		if got := capitalize(in); got != want {
			t.Errorf("capitalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNodeDumps(t *testing.T) {
	if NewAlert("note", true).Dump(nil) == nil {
		t.Error("Alert.Dump returned nil")
	}
	title := NewAlertTitle("note")
	if title.Dump(nil) == nil {
		t.Error("AlertTitle.Dump returned nil")
	}
	if title.Kind() != KindAlertTitle {
		t.Errorf("AlertTitle.Kind() = %v, want %v", title.Kind(), KindAlertTitle)
	}
}

// TestBlockParserFlags pins the two scheduling answers the parsers give
// goldmark: an alert may interrupt a paragraph but may not open on an indented
// line, and its title may not interrupt a paragraph but may be indented.
func TestBlockParserFlags(t *testing.T) {
	a := &alertParser{}
	if !a.CanInterruptParagraph() || a.CanAcceptIndentedLine() {
		t.Errorf("alertParser flags = (%v, %v), want (true, false)",
			a.CanInterruptParagraph(), a.CanAcceptIndentedLine())
	}
	if got := string(a.Trigger()); got != ">" {
		t.Errorf("alertParser.Trigger() = %q, want %q", got, ">")
	}

	tp := &alertTitleParser{}
	if tp.CanInterruptParagraph() || !tp.CanAcceptIndentedLine() {
		t.Errorf("alertTitleParser flags = (%v, %v), want (false, true)",
			tp.CanInterruptParagraph(), tp.CanAcceptIndentedLine())
	}
	if got := string(tp.Trigger()); got != "]" {
		t.Errorf("alertTitleParser.Trigger() = %q, want %q", got, "]")
	}
}
