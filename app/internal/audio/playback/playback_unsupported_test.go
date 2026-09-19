//go:build !linux

package playback

import (
	"context"
	"errors"
	"testing"

	"peanut/internal/audio"
)

func TestSystemPlayerIsUnsupported(t *testing.T) {
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	if err != nil {
		t.Fatal(err)
	}
	if err := (SystemPlayer{}).Play(context.Background(), input); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want unsupported", err)
	}
}
