//go:build cgo && sherpa

package sherpa

import (
	"os"
	"path/filepath"
	"testing"

	"peanut/internal/audio"
	"peanut/internal/ml"
)

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
