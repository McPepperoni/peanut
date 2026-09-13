package capture

import (
	"context"
	"errors"
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

func TestFileCaptureZeroPadsFinalPartialFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.wav")
	samples := make([]float32, audio.FrameSamples+1)
	samples[audio.FrameSamples] = 0.5
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, samples)
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
	<-frames
	last := <-frames
	if last.Samples[0] != 0.5 || last.Samples[1] != 0 || last.Samples[audio.FrameSamples-1] != 0 {
		t.Fatalf("final frame was not zero-padded: first=%v second=%v last=%v", last.Samples[0], last.Samples[1], last.Samples[audio.FrameSamples-1])
	}
	if _, ok := <-frames; ok {
		t.Fatal("capture returned extra frame")
	}
}

func TestSystemCaptureIsUnsupported(t *testing.T) {
	var capture audio.Capture = SystemCapture{}
	frames, err := capture.Capture(context.Background())
	if frames != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("frames = %v, error = %v, want unsupported", frames, err)
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
