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
		KWS:     modelDir(t, root, KWSBundle),
		VAD:     touch(t, root, VADBundle),
		STT:     modelDir(t, root, STTBundle),
		Speaker: touch(t, root, SpeakerBundle),
		TTS:     modelDir(t, root, TTSBundle),
	}
	if err := (Manifest{Paths: paths, Provider: CPUProvider, Threads: 2}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestReportsMissingAndInvalidModels(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		KWS:     filepath.Join(root, KWSBundle),
		VAD:     touch(t, root, VADBundle),
		STT:     modelDir(t, root, STTBundle),
		Speaker: touch(t, root, SpeakerBundle),
		TTS:     modelDir(t, root, TTSBundle),
	}
	err := (Manifest{Paths: paths, Provider: CPUProvider, Threads: 1}).Validate()
	if !errors.Is(err, ErrModelMissing) {
		t.Fatalf("missing model error = %v", err)
	}
	paths.KWS = touch(t, root, KWSBundle)
	err = (Manifest{Paths: paths, Provider: CPUProvider, Threads: 1}).Validate()
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

func TestManifestRequiresCPUProvider(t *testing.T) {
	manifest := validTestManifest(t)
	manifest.Provider = ""
	if err := manifest.Validate(); !errors.Is(err, ErrModelInvalid) {
		t.Fatalf("provider error = %v, want invalid model", err)
	}
}

func TestManifestRejectsEmptyModelDirectories(t *testing.T) {
	for _, model := range []struct {
		name string
		set  func(*Paths, string)
	}{
		{"KWS", func(paths *Paths, path string) { paths.KWS = path }},
		{"STT", func(paths *Paths, path string) { paths.STT = path }},
		{"TTS", func(paths *Paths, path string) { paths.TTS = path }},
	} {
		t.Run(model.name, func(t *testing.T) {
			manifest := validTestManifest(t)
			model.set(&manifest.Paths, mkdir(t, t.TempDir(), model.name))
			if err := manifest.Validate(); !errors.Is(err, ErrModelInvalid) {
				t.Fatalf("empty directory error = %v, want invalid model", err)
			}
		})
	}
}

func TestManifestRejectsEmptyOrNonONNXModelFiles(t *testing.T) {
	manifest := validTestManifest(t)
	manifest.Paths.VAD = emptyFile(t, t.TempDir(), VADBundle)
	if err := manifest.Validate(); !errors.Is(err, ErrModelInvalid) {
		t.Fatalf("empty VAD error = %v, want invalid model", err)
	}

	manifest = validTestManifest(t)
	manifest.Paths.Speaker = touch(t, t.TempDir(), "speaker.txt")
	if err := manifest.Validate(); !errors.Is(err, ErrModelInvalid) {
		t.Fatalf("non-ONNX speaker error = %v, want invalid model", err)
	}
}

func validTestManifest(t *testing.T) Manifest {
	t.Helper()
	root := t.TempDir()
	return Manifest{Paths: Paths{
		KWS:     modelDir(t, root, KWSBundle),
		VAD:     touch(t, root, VADBundle),
		STT:     modelDir(t, root, STTBundle),
		Speaker: touch(t, root, SpeakerBundle),
		TTS:     modelDir(t, root, TTSBundle),
	}, Provider: CPUProvider, Threads: 1}
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

func emptyFile(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func modelDir(t *testing.T, root, name string) string {
	t.Helper()
	path := mkdir(t, root, name)
	touch(t, path, "model")
	return path
}
