package sherpa

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"peanut/internal/ml"
)

func TestOpenReportsUnavailableWithoutNativeRuntime(t *testing.T) {
	err := Open(validManifest(t))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open error = %v, want unavailable runtime", err)
	}
}

func TestOpenValidatesManifestBeforeNativeRuntime(t *testing.T) {
	err := Open(ml.Manifest{Threads: 1})
	if !errors.Is(err, ml.ErrModelMissing) {
		t.Fatalf("Open error = %v, want missing model", err)
	}
}

func validManifest(t *testing.T) ml.Manifest {
	t.Helper()
	root := t.TempDir()
	directory := func(name string) string {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	file := func(name string) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	return ml.Manifest{Paths: ml.Paths{
		KWS: directory(ml.KWSBundle), VAD: file(ml.VADBundle), STT: directory(ml.STTBundle),
		Speaker: file(ml.SpeakerBundle), TTS: directory(ml.TTSBundle),
	}, Threads: 1}
}
