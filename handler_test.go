package main

import (
	"testing"
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
