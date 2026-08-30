package main

import (
	"os"
	"path/filepath"
	"runtime"
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
	// See the handler tests: chmod 000 can't make a directory unreadable
	// on Windows, so the EACCES path isn't simulatable there.
	if runtime.GOOS == "windows" {
		t.Skip("cannot simulate unreadable directories on windows")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Skipf("cannot chmod: %v", err)
	}
	defer os.Chmod(dir, 0o755)

	if err := validateReadable(dir); err == nil {
		t.Fatal("validateReadable(unreadable dir) = nil, want error")
	}
}

func TestLoadCustomCSS(t *testing.T) {
	if css, err := loadCustomCSS(""); err != nil || css != "" {
		t.Errorf("loadCustomCSS(empty) = (%q, %v), want empty/nil", css, err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "custom.css")
	if err := os.WriteFile(path, []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}
	css, err := loadCustomCSS(path)
	if err != nil {
		t.Fatalf("loadCustomCSS: %v", err)
	}
	if css != "body{color:red}" {
		t.Errorf("loadCustomCSS = %q", css)
	}

	if _, err := loadCustomCSS(filepath.Join(dir, "missing.css")); err == nil {
		t.Error("loadCustomCSS(missing) should error")
	}
}
