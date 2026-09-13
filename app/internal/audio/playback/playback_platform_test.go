package playback

import (
	"context"
	"errors"
	"testing"

	"peanut/internal/audio"
)

func TestSystemPlayerPlatformBoundary(t *testing.T) {
	var system audio.Player = SystemPlayer{}
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	if err != nil {
		t.Fatal(err)
	}
	if err := system.Play(context.Background(), input); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v", err)
	}
}
