package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkTreeBuild measures the full walk+prune+sort that used to run on
// every tree.json request before caching. Each iteration uses a fresh handler
// to force a rebuild.
func BenchmarkTreeBuild(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 200; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("dir%03d", i))
		os.MkdirAll(sub, 0o755)
		os.WriteFile(filepath.Join(sub, "file.md"), []byte("# x"), 0o644)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := &fileHandler{root: dir}
		if h.treeJSONCached() == nil {
			b.Fatal("nil tree")
		}
	}
}

// BenchmarkTreeJSONCached measures repeated tree.json fetches, which is what
// the sidebar does on every page load. With caching this should be a single
// mutex + slice read (0 allocs).
func BenchmarkTreeJSONCached(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 200; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("dir%03d", i))
		os.MkdirAll(sub, 0o755)
		os.WriteFile(filepath.Join(sub, "file.md"), []byte("# x"), 0o644)
	}
	h := &fileHandler{root: dir}
	h.treeJSONCached() // warm the cache once
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if h.treeJSONCached() == nil {
			b.Fatal("nil tree")
		}
	}
}

// BenchmarkConvert measures markdown rendering, which runs per page view.
func BenchmarkConvert(b *testing.B) {
	md := newMarkdownConverter(b.TempDir())
	src := []byte("# Title\n\nSome **bold** text with a [link](http://example.com).\n\n> [!NOTE]\n> A callout here.\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := md.convert(src); err != nil {
			b.Fatal(err)
		}
	}
}
