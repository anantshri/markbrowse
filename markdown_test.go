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
