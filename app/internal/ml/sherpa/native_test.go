//go:build !cgo || !sherpa

package sherpa

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"peanut/internal/audio"
	"peanut/internal/ml"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
)

var (
	_ kws.WakeDetector          = (*WakeDetector)(nil)
	_ vad.VAD                   = (*VoiceActivityDetector)(nil)
	_ stt.Transcriber           = (*Transcriber)(nil)
	_ speaker.SpeakerIdentifier = (*SpeakerIdentifier)(nil)
	_ tts.Synthesizer           = (*Synthesizer)(nil)
)

func TestTypedAdaptersReportUnavailableWithoutNativeRuntime(t *testing.T) {
	manifest := validManifest(t)
	constructors := []struct {
		name string
		open func() error
	}{
		{"KWS", func() error { _, err := NewWakeDetector(manifest); return err }},
		{"VAD", func() error { _, err := NewVAD(manifest); return err }},
		{"STT", func() error { _, err := NewTranscriber(manifest); return err }},
		{"speaker", func() error { _, err := NewSpeakerIdentifier(manifest); return err }},
		{"TTS", func() error { _, err := NewSynthesizer(manifest); return err }},
	}
	for _, constructor := range constructors {
		t.Run(constructor.name, func(t *testing.T) {
			if err := constructor.open(); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("constructor error = %v, want unavailable runtime", err)
			}
		})
	}
}

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
	manifest := validManifest(t)
	if err := os.Remove(filepath.Join(manifest.Paths.STT, "tokens.txt")); err != nil {
		t.Fatal(err)
	}
	_, err := newNativeConfig(manifest)
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
		return path
	}
	file := func(name string) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	manifest := ml.Manifest{Paths: ml.Paths{
		KWS: directory(ml.KWSBundle), VAD: file(ml.VADBundle), STT: directory(ml.STTBundle),
		Speaker: file(ml.SpeakerBundle), TTS: directory(ml.TTSBundle),
	}, Provider: ml.CPUProvider, Threads: 1}
	for _, path := range []string{
		filepath.Join(manifest.Paths.KWS, "encoder-epoch-12-avg-2-chunk-16-left-64.int8.onnx"),
		filepath.Join(manifest.Paths.KWS, "decoder-epoch-12-avg-2-chunk-16-left-64.onnx"),
		filepath.Join(manifest.Paths.KWS, "joiner-epoch-12-avg-2-chunk-16-left-64.int8.onnx"),
		filepath.Join(manifest.Paths.KWS, "tokens.txt"),
		filepath.Join(manifest.Paths.STT, "model.int8.onnx"),
		filepath.Join(manifest.Paths.STT, "tokens.txt"),
		filepath.Join(manifest.Paths.TTS, "en_GB-cori-medium.onnx"),
		filepath.Join(manifest.Paths.TTS, "tokens.txt"),
	} {
		if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(manifest.Paths.TTS, "espeak-ng-data"), 0o755); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func writeSTTFiles(t *testing.T, directory string) {
	t.Helper()
	for _, name := range []string{"model.int8.onnx", "tokens.txt"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
