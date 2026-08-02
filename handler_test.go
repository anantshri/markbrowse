package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{2621440, "2.5 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.input)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildBreadcrumbs(t *testing.T) {
	tests := []struct {
		path string
		want []breadcrumb
	}{
		{"/", nil},
		{"/README.md", nil},
		{"/docs/README.md", []breadcrumb{{Name: "docs", Path: "/docs/"}}},
		{"/a/b/c.md", []breadcrumb{
			{Name: "a", Path: "/a/"},
			{Name: "b", Path: "/a/b/"},
		}},
	}
	for _, tt := range tests {
		got := buildBreadcrumbs(tt.path)
		if len(got) != len(tt.want) {
			t.Errorf("buildBreadcrumbs(%q) = %v, want %v", tt.path, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("buildBreadcrumbs(%q)[%d] = %v, want %v", tt.path, i, got[i], tt.want[i])
			}
		}
	}
}

func TestBuildFileIndex(t *testing.T) {
	idx := buildFileIndex("testdata")
	if len(idx) == 0 {
		t.Fatal("buildFileIndex returned empty index")
	}

	if p, ok := idx["readme"]; !ok || p != "/README.md" {
		t.Errorf("idx[readme] = %q, ok=%v, want /README.md", p, ok)
	}
	if p, ok := idx["notes"]; !ok || p != "/notes.md" {
		t.Errorf("idx[notes] = %q, ok=%v, want /notes.md", p, ok)
	}
}

func TestPruneEmptyDirs(t *testing.T) {
	root := treeEntry{
		Name:  "root",
		Path:  "/",
		IsDir: true,
		Children: []treeEntry{
			{Name: "empty", Path: "/empty/", IsDir: true},
			{Name: "hasmd", Path: "/hasmd/", IsDir: true, Children: []treeEntry{
				{Name: "file.md", Path: "/hasmd/file.md"},
			}},
			{Name: "top.md", Path: "/top.md"},
		},
	}
	pruneEmptyDirs(&root)

	if len(root.Children) != 2 {
		t.Fatalf("pruneEmptyDirs: got %d children, want 2", len(root.Children))
	}
	for _, c := range root.Children {
		if c.Name == "empty" {
			t.Error("pruneEmptyDirs: empty directory should have been pruned")
		}
	}
}

func TestSortTree(t *testing.T) {
	root := treeEntry{
		Name:  "root",
		Path:  "/",
		IsDir: true,
		Children: []treeEntry{
			{Name: "zebra.md", Path: "/zebra.md"},
			{Name: "alpha", Path: "/alpha/", IsDir: true},
			{Name: "beta.md", Path: "/beta.md"},
		},
	}
	sortTree(&root)

	if len(root.Children) != 3 {
		t.Fatalf("sortTree: got %d children, want 3", len(root.Children))
	}
	if root.Children[0].Name != "alpha" {
		t.Errorf("sortTree: first child = %q, want alpha (dir first)", root.Children[0].Name)
	}
	if root.Children[1].Name != "beta.md" {
		t.Errorf("sortTree: second child = %q, want beta.md (alpha sort)", root.Children[1].Name)
	}
}

func TestInsertIntoTree(t *testing.T) {
	root := treeEntry{Name: "root", Path: "/", IsDir: true}
	insertIntoTree(&root, []string{"sub", "file.md"}, "/sub/file.md", false)

	if len(root.Children) != 1 {
		t.Fatalf("insertIntoTree: got %d children, want 1", len(root.Children))
	}
	sub := root.Children[0]
	if sub.Name != "sub" || !sub.IsDir {
		t.Errorf("insertIntoTree: sub = {%q, isDir=%v}, want {sub, true}", sub.Name, sub.IsDir)
	}
	if len(sub.Children) != 1 || sub.Children[0].Name != "file.md" {
		t.Errorf("insertIntoTree: sub children = %v, want [file.md]", sub.Children)
	}
}

func TestTreeJSONCachedRebuildsAfterTTL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# a"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir}

	first := h.treeJSONCached()
	if first == nil || !strings.Contains(string(first), "a.md") {
		t.Fatalf("first tree missing a.md: %s", first)
	}

	// A second call inside the TTL must return the cached bytes (no rebuild).
	again := h.treeJSONCached()
	if !bytes.Equal(first, again) {
		t.Fatal("cached tree should be identical within TTL")
	}

	// Add a file, then force expiry and confirm the tree is rebuilt.
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# b"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.treeAt = time.Now().Add(-treeCacheTTL - time.Second)
	rebuilt := h.treeJSONCached()
	if !strings.Contains(string(rebuilt), "b.md") {
		t.Fatalf("rebuilt tree missing b.md: %s", rebuilt)
	}
}

func TestServeMarkdownETagNotModified(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Doc\n\ncontent"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir)}

	req := httptest.NewRequest(http.MethodGet, "/doc.md", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("first response should carry an ETag")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/doc.md", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("conditional request = %d, want 304", rec2.Code)
	}
	if rec2.Body.Len() != 0 {
		t.Errorf("304 response should have empty body, got %d bytes", rec2.Body.Len())
	}
}
