package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGGUFPathAcceptsFlatModel(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "functiongemma.gguf")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveGGUFPath(root, "functiongemma.gguf")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestResolveGGUFPathRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"", "../outside.gguf", "nested/model.gguf", "/tmp/model.gguf", "model.bin"} {
		if _, err := ResolveGGUFPath(t.TempDir(), name); err == nil {
			t.Fatalf("ResolveGGUFPath(%q) accepted unsafe name", name)
		}
	}
}

func TestResolveGGUFPathRejectsDirectoriesAndMissingFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveGGUFPath(root, "missing.gguf"); err == nil {
		t.Fatal("ResolveGGUFPath accepted missing file")
	}
	if err := os.Mkdir(filepath.Join(root, "directory.gguf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveGGUFPath(root, "directory.gguf"); err == nil {
		t.Fatal("ResolveGGUFPath accepted directory")
	}
}

func TestResolveGGUFPathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.gguf")
	if err := os.WriteFile(outside, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "model.gguf")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := ResolveGGUFPath(root, "model.gguf"); err == nil {
		t.Fatal("ResolveGGUFPath accepted symlink escape")
	}
}
