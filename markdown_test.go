package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownConvert(t *testing.T) {
	dir := t.TempDir()
	md := newMarkdownConverter(dir)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"heading", "# Hello", "h1"},
		{"bold", "**bold**", "<strong>bold</strong>"},
		{"link", "[url](http://example.com)", `<a href="http://example.com">url</a>`},
		{"callout", "> [!NOTE]\n> hi", "markdown-alert-note"},
		{"mermaid", "```mermaid\ngraph TD\nA-->B\n```", `class="mermaid"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := md.convert([]byte(tt.input))
			if err != nil {
				t.Fatalf("convert error: %v", err)
			}
			if !contains(got, tt.want) {
				t.Errorf("convert(%q) = %q, want to contain %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWikilinkResolution(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "notes"), 0755)
	os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target"), 0644)
	os.WriteFile(filepath.Join(dir, "notes", "deep.md"), []byte("# Deep"), 0644)

	md := newMarkdownConverter(dir)

	got, err := md.convert([]byte("see [[target]] for info"))
	if err != nil {
		t.Fatalf("convert error: %v", err)
	}
	if !contains(got, "/target.md") {
		t.Errorf("wikilink: got %q, want to contain /target.md", got)
	}

	got2, err := md.convert([]byte("see [[deep]] for info"))
	if err != nil {
		t.Fatalf("convert error: %v", err)
	}
	if !contains(got2, "/notes/deep.md") {
		t.Errorf("wikilink nested: got %q, want to contain /notes/deep.md", got2)
	}
}

func TestBrokenWikilink(t *testing.T) {
	dir := t.TempDir()
	md := newMarkdownConverter(dir)

	got, err := md.convert([]byte("see [[nonexistent]] for info"))
	if err != nil {
		t.Fatalf("convert error: %v", err)
	}
	if contains(got, "href") {
		t.Errorf("broken wikilink should not render as link, got: %q", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFrontMatterRenderedAsTable(t *testing.T) {
	dir := t.TempDir()
	md := newMarkdownConverter(dir)

	src := "---\ntitle: Case Assignment Memo\nclassification: Internal Use Only\ncase_id: BBT-2026-001\n---\n\n# ByteBrew\n\nBody text."
	got, err := md.convert([]byte(src))
	if err != nil {
		t.Fatalf("convert error: %v", err)
	}

	if !contains(got, `<table class="meta-table">`) {
		t.Errorf("front matter should render as meta table, got: %q", got[:min(300, len(got))])
	}
	// GitHub layout: one row per key, first pair in thead, rest in tbody.
	if !contains(got, "<thead><tr><th>title</th><td>Case Assignment Memo</td></tr></thead>") {
		t.Errorf("thead row missing or wrong, got: %q", got[:min(400, len(got))])
	}
	if !contains(got, "<tbody><tr><th>classification</th><td>Internal Use Only</td></tr>") {
		t.Errorf("tbody classification row missing")
	}
	if !contains(got, "<tr><th>case_id</th><td>BBT-2026-001</td></tr>") {
		t.Errorf("case_id row missing")
	}
	if !contains(got, "</tbody></table>") {
		t.Errorf("table not closed properly")
	}
	// Document body must survive.
	if !contains(got, "<h1") || !contains(got, "Body text.") {
		t.Errorf("document content lost after front matter")
	}
}

func TestNoFrontMatterNoTable(t *testing.T) {
	dir := t.TempDir()
	md := newMarkdownConverter(dir)

	got, err := md.convert([]byte("# Just a heading\n\nText."))
	if err != nil {
		t.Fatalf("convert error: %v", err)
	}
	if contains(got, "meta-table") {
		t.Errorf("no front matter should mean no meta table, got: %q", got)
	}
}

func TestFrontMatterValueTypes(t *testing.T) {
	dir := t.TempDir()
	md := newMarkdownConverter(dir)

	src := "---\ndraft: false\ncount: 42\ntags:\n  - one\n  - two\n---\n\n# H\n"
	got, err := md.convert([]byte(src))
	if err != nil {
		t.Fatalf("convert error: %v", err)
	}
	for _, want := range []string{
		"<th>draft</th><td>false</td>",
		"<th>count</th><td>42</td>",
		"<th>tags</th><td>one<br>two</td>",
	} {
		if !contains(got, want) {
			t.Errorf("missing %q in output: %q", want, got)
		}
	}
}

func TestFrontMatterHTMLEscaping(t *testing.T) {
	dir := t.TempDir()
	md := newMarkdownConverter(dir)

	src := "---\ntitle: \"<script>alert(1)</script>\"\n---\n\n# H\n"
	got, err := md.convert([]byte(src))
	if err != nil {
		t.Fatalf("convert error: %v", err)
	}
	if contains(got, "<script>alert") {
		t.Errorf("front matter value must be HTML-escaped, got: %q", got)
	}
	if !contains(got, "&lt;script&gt;") {
		t.Errorf("escaped value missing, got: %q", got)
	}
}

func TestMetaTitle(t *testing.T) {
	dir := t.TempDir()
	md := newMarkdownConverter(dir)

	if got := md.metaTitleOf([]byte("---\ntitle: Hello World\n---\n\n# H")); got != "Hello World" {
		t.Errorf("metaTitleOf = %q, want Hello World", got)
	}
	if got := md.metaTitleOf([]byte("---\nauthor: x\n---\n\n# H")); got != "" {
		t.Errorf("metaTitleOf without title = %q, want empty", got)
	}
	if got := md.metaTitleOf([]byte("# No front matter")); got != "" {
		t.Errorf("metaTitleOf without front matter = %q, want empty", got)
	}
}

func TestWikilinkResolvesInDotDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vault"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".vault", "hidden.md"), []byte("# Hidden"), 0o644); err != nil {
		t.Fatal(err)
	}

	md := newMarkdownConverter(dir)
	got, err := md.convert([]byte("see [[hidden]]"))
	if err != nil {
		t.Fatalf("convert error: %v", err)
	}
	if !contains(got, "/.vault/hidden.md") {
		t.Errorf("wikilink into dot dir failed, got: %q", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
