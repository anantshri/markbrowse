package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateReadableOK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateReadable(dir); err != nil {
		t.Fatalf("validateReadable(%q) = %v, want nil", dir, err)
	}
}

func TestValidateReadableEmptyDirOK(t *testing.T) {
	dir := t.TempDir()
	if err := validateReadable(dir); err != nil {
		t.Fatalf("validateReadable(empty) = %v, want nil", err)
	}
}

func TestValidateReadablePermissionDenied(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Skipf("cannot chmod: %v", err)
	}
	defer os.Chmod(dir, 0o755)

	if err := validateReadable(dir); err == nil {
		t.Fatal("validateReadable(unreadable dir) = nil, want error")
	}
}
