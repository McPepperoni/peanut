package models

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveGGUFPath validates a flat GGUF filename and returns its canonical path.
func ResolveGGUFPath(root, filename string) (string, error) {
	if root == "" {
		return "", errors.New("GGUF root is required")
	}
	if filename == "" || filepath.IsAbs(filename) || filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\\`) {
		return "", fmt.Errorf("invalid GGUF filename %q", filename)
	}
	if !strings.EqualFold(filepath.Ext(filename), ".gguf") {
		return "", fmt.Errorf("invalid GGUF extension %q", filename)
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve GGUF root: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, filepath.Clean(filename)))
	if err != nil {
		return "", fmt.Errorf("resolve GGUF file %q: %w", filename, err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("check GGUF path containment: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("GGUF path %q escapes model root", filename)
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("stat GGUF file %q: %w", filename, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("GGUF path %q is not a regular file", filename)
	}
	return resolvedPath, nil
}
