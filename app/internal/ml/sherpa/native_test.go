package sherpa

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"peanut/internal/audio"
	"peanut/internal/ml"
)

func TestOpenReportsUnavailableWithoutNativeRuntime(t *testing.T) {
	err := Open(validManifest(t))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open error = %v, want unavailable runtime", err)
	}
}

func TestOpenValidatesManifestBeforeNativeRuntime(t *testing.T) {
	err := Open(ml.Manifest{Provider: ml.CPUProvider, Threads: 1})
	if !errors.Is(err, ml.ErrModelMissing) {
		t.Fatalf("Open error = %v, want missing model", err)
	}
}

func TestTranscribeReportsUnavailableWithoutNativeRuntime(t *testing.T) {
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Transcribe(validManifest(t), input)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Transcribe error = %v, want unavailable runtime", err)
	}
}

func TestNativeConfigUsesCPUAndConfiguredThreads(t *testing.T) {
	manifest := validManifest(t)
	manifest.Threads = 3
	writeSTTFiles(t, manifest.Paths.STT)
	config, err := newNativeConfig(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider != ml.CPUProvider || config.Threads != 3 {
		t.Fatalf("native config = %+v", config)
	}
}

func TestNativeConfigRequiresOfficialSTTFiles(t *testing.T) {
	_, err := newNativeConfig(validManifest(t))
	if !errors.Is(err, ml.ErrModelInvalid) {
		t.Fatalf("native config error = %v, want invalid model", err)
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
		if err := os.WriteFile(filepath.Join(path, "model"), []byte("model"), 0o644); err != nil {
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
	}, Provider: ml.CPUProvider, Threads: 1}
}

func writeSTTFiles(t *testing.T, directory string) {
	t.Helper()
	for _, name := range []string{"model.int8.onnx", "tokens.txt"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
