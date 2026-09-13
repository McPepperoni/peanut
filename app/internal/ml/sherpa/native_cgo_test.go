//go:build cgo && sherpa

package sherpa

import (
	"context"
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

func TestUnsupportedNativeAdaptersReportUnavailable(t *testing.T) {
	frame, err := audio.NewFrame(make([]float32, audio.FrameSamples))
	if err != nil {
		t.Fatal(err)
	}
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := (&WakeDetector{}).Detect(ctx, frame); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("KWS error = %v, want unavailable", err)
	}
	if _, err := (&VoiceActivityDetector{}).Detect(ctx, frame); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("VAD error = %v, want unavailable", err)
	}
	if result := (&SpeakerIdentifier{}).Identify(ctx, input); !errors.Is(result.Err, ErrUnavailable) {
		t.Fatalf("speaker error = %v, want unavailable", result.Err)
	}
	if _, err := (&Synthesizer{}).Synthesize(ctx, "test"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("TTS error = %v, want unavailable", err)
	}
}

func TestNativeSmoke(t *testing.T) {
	root := os.Getenv("PEANUT_MODEL_ROOT")
	if root == "" {
		t.Skip("set PEANUT_MODEL_ROOT to run native sherpa smoke test")
	}
	manifest := ml.Manifest{Paths: ml.Paths{
		KWS:     filepath.Join(root, ml.KWSBundle),
		VAD:     filepath.Join(root, ml.VADBundle),
		STT:     filepath.Join(root, ml.STTBundle),
		Speaker: filepath.Join(root, ml.SpeakerBundle),
		TTS:     filepath.Join(root, ml.TTSBundle),
	}, Provider: ml.CPUProvider, Threads: 1}
	if err := Open(manifest); err != nil {
		t.Fatal(err)
	}
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, make([]float32, audio.SampleRate))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Transcribe(manifest, input); err != nil {
		t.Fatal(err)
	}
}
