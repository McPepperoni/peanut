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
	for _, provider := range []string{"", "CPU", " cpu", "cpu "} {
		manifest := validTestManifest(t)
		manifest.Provider = provider
		if err := manifest.Validate(); !errors.Is(err, ErrModelInvalid) {
			t.Fatalf("provider %q error = %v, want invalid model", provider, err)
		}
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

func TestManifestRequiresBundleFiles(t *testing.T) {
	for _, model := range []struct {
		name string
		file string
		path func(Paths) string
	}{
		{"KWS", "encoder-epoch-12-avg-2-chunk-16-left-64.int8.onnx", func(paths Paths) string { return paths.KWS }},
		{"KWS", "decoder-epoch-12-avg-2-chunk-16-left-64.onnx", func(paths Paths) string { return paths.KWS }},
		{"KWS", "joiner-epoch-12-avg-2-chunk-16-left-64.int8.onnx", func(paths Paths) string { return paths.KWS }},
		{"KWS", "tokens.txt", func(paths Paths) string { return paths.KWS }},
		{"STT", "model.int8.onnx", func(paths Paths) string { return paths.STT }},
		{"STT", "tokens.txt", func(paths Paths) string { return paths.STT }},
		{"TTS", "en_GB-cori-medium.onnx", func(paths Paths) string { return paths.TTS }},
		{"TTS", "tokens.txt", func(paths Paths) string { return paths.TTS }},
		{"TTS", "espeak-ng-data", func(paths Paths) string { return paths.TTS }},
	} {
		t.Run(model.name+"/"+model.file, func(t *testing.T) {
			manifest := validTestManifest(t)
			if err := os.RemoveAll(filepath.Join(model.path(manifest.Paths), model.file)); err != nil {
				t.Fatal(err)
			}
			if err := manifest.Validate(); !errors.Is(err, ErrModelInvalid) {
				t.Fatalf("missing %s file error = %v, want invalid model", model.file, err)
			}
		})
	}
}

func TestManifestRejectsEmptyBundleFiles(t *testing.T) {
	for _, model := range []struct {
		file string
		path func(Paths) string
	}{
		{"tokens.txt", func(paths Paths) string { return paths.KWS }},
		{"model.int8.onnx", func(paths Paths) string { return paths.STT }},
		{"en_GB-cori-medium.onnx", func(paths Paths) string { return paths.TTS }},
	} {
		manifest := validTestManifest(t)
		if err := os.WriteFile(filepath.Join(model.path(manifest.Paths), model.file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := manifest.Validate(); !errors.Is(err, ErrModelInvalid) {
			t.Fatalf("empty %s error = %v, want invalid model", model.file, err)
		}
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
	switch name {
	case KWSBundle:
		for _, file := range []string{
			"encoder-epoch-12-avg-2-chunk-16-left-64.int8.onnx",
			"decoder-epoch-12-avg-2-chunk-16-left-64.onnx",
			"joiner-epoch-12-avg-2-chunk-16-left-64.int8.onnx",
			"tokens.txt",
		} {
			touch(t, path, file)
		}
	case STTBundle:
		for _, file := range []string{"model.int8.onnx", "tokens.txt"} {
			touch(t, path, file)
		}
	case TTSBundle:
		for _, file := range []string{"en_GB-cori-medium.onnx", "tokens.txt"} {
			touch(t, path, file)
		}
		mkdir(t, path, "espeak-ng-data")
	}
	return path
}
