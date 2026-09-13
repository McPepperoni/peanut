package capture

import (
	"context"
	"path/filepath"
	"testing"

	"peanut/internal/audio"
)

func TestFileCaptureReturnsWAVFrames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.wav")
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, make([]float32, audio.FrameSamples*2))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, input); err != nil {
		t.Fatal(err)
	}
	frames, err := NewFile(path).Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for frame := range frames {
		if err := frame.Validate(); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("frame count = %d, want 2", count)
	}
}

func TestFakeCaptureCopiesFrames(t *testing.T) {
	frame, err := audio.NewFrame(make([]float32, audio.FrameSamples))
	if err != nil {
		t.Fatal(err)
	}
	fake := NewFake([]audio.Frame{frame})
	frames, err := fake.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	frame.Samples[0] = 1
	if got := (<-frames).Samples[0]; got != 0 {
		t.Fatalf("captured sample = %v, want copy", got)
	}
}
