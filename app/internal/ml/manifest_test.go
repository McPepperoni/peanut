package ml

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValidatesOfficialModels(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		KWS:     mkdir(t, root, KWSBundle),
		VAD:     touch(t, root, VADBundle),
		STT:     mkdir(t, root, STTBundle),
		Speaker: touch(t, root, SpeakerBundle),
		TTS:     mkdir(t, root, TTSBundle),
	}
	if err := (Manifest{Paths: paths, Threads: 2}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestReportsMissingAndInvalidModels(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		KWS:     filepath.Join(root, KWSBundle),
		VAD:     touch(t, root, VADBundle),
		STT:     mkdir(t, root, STTBundle),
		Speaker: touch(t, root, SpeakerBundle),
		TTS:     mkdir(t, root, TTSBundle),
	}
	err := (Manifest{Paths: paths, Threads: 1}).Validate()
	if !errors.Is(err, ErrModelMissing) {
		t.Fatalf("missing model error = %v", err)
	}
	paths.KWS = touch(t, root, KWSBundle)
	err = (Manifest{Paths: paths, Threads: 1}).Validate()
	if !errors.Is(err, ErrModelInvalid) {
		t.Fatalf("invalid model error = %v", err)
	}
}

func TestManifestRequiresPositiveThreads(t *testing.T) {
	err := (Manifest{Threads: 0}).Validate()
	if !errors.Is(err, ErrModelInvalid) {
		t.Fatalf("thread error = %v", err)
	}
}

func mkdir(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func touch(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
