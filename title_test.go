package main

import (
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pageTitle renders doc.md through the real handler and returns the decoded
// <title>, so the tests exercise the same path a reader's browser does rather
// than calling docTitle directly.
func pageTitle(t *testing.T, src string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/doc.md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	start := strings.Index(body, "<title>")
	end := strings.Index(body, "</title>")
	if start == -1 || end == -1 || end < start {
		t.Fatalf("no <title> element in the response")
	}
	// html/template escapes the value for its context; compare the value.
	return html.UnescapeString(body[start+len("<title>") : end])
}

// TestPageTitleFromH1UnderFrontMatter is the case the previous prefix test
// could not reach: a front matter table is prepended to the rendered body, so
// a document with front matter but no "title" key begins with <table>. The old
// strings.HasPrefix(body, "<h1") was therefore false whenever front matter was
// present, and the page silently fell back to the file name.
func TestPageTitleFromH1UnderFrontMatter(t *testing.T) {
	if got := pageTitle(t, "---\nauthor: x\n---\n\n# Real Title\n"); got != "Real Title" {
		t.Errorf("title = %q, want %q", got, "Real Title")
	}
}

// TestPageTitlePrefersFrontMatter: the front matter title still outranks the
// heading, as before.
func TestPageTitlePrefersFrontMatter(t *testing.T) {
	if got := pageTitle(t, "---\ntitle: From Front Matter\n---\n\n# Heading\n"); got != "From Front Matter" {
		t.Errorf("title = %q, want %q", got, "From Front Matter")
	}
}

// TestPageTitleStripsInlineMarkup: a heading carrying rendered inline markup
// titles as its text. The old code sliced the body between ">" and "</h1>",
// which for "# A `x`" kept the <code> tags and stopped at the wrong place.
func TestPageTitleStripsInlineMarkup(t *testing.T) {
	if got := pageTitle(t, "# A `x` & more\n"); got != "A x & more" {
		t.Errorf("title = %q, want %q", got, "A x & more")
	}
}

// TestPageTitleFallsBackToFileName: with neither front matter title nor a
// heading, the file name is still the last resort.
func TestPageTitleFallsBackToFileName(t *testing.T) {
	if got := pageTitle(t, "no headings and no front matter\n"); got != "doc.md" {
		t.Errorf("title = %q, want %q", got, "doc.md")
	}
}

// TestPageTitleIgnoresEmptyHeading: an empty <h1> must not produce an empty
// title.
func TestPageTitleIgnoresEmptyHeading(t *testing.T) {
	if got := pageTitle(t, "# \n\ntext\n"); got != "doc.md" {
		t.Errorf("title = %q, want the file name %q", got, "doc.md")
	}
}
