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
	speakerstore "peanut/internal/speaker"
)

var (
	_ kws.WakeDetector          = (*WakeDetector)(nil)
	_ vad.VAD                   = (*VoiceActivityDetector)(nil)
	_ stt.Transcriber           = (*Transcriber)(nil)
	_ speaker.SpeakerIdentifier = (*SpeakerIdentifier)(nil)
	_ speakerstore.Embedder     = (*SpeakerIdentifier)(nil)
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
