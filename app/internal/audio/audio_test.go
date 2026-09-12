package audio

import "testing"

func TestNewFrameRequires320Samples(t *testing.T) {
	if _, err := NewFrame(make([]float32, 319)); err == nil {
		t.Fatal("NewFrame accepted 319 samples")
	}
	frame, err := NewFrame(make([]float32, 320))
	if err != nil {
		t.Fatal(err)
	}
	if len(frame.Samples) != 320 {
		t.Fatalf("sample count = %d, want 320", len(frame.Samples))
	}
}

func TestNewFrameCopiesSamples(t *testing.T) {
	samples := make([]float32, 320)
	samples[0] = 1
	frame, err := NewFrame(samples)
	if err != nil {
		t.Fatal(err)
	}
	samples[0] = 2
	if frame.Samples[0] != 1 {
		t.Fatalf("frame changed after source mutation: %v", frame.Samples[0])
	}
}

func TestNewAudioRequiresNormalizedFormat(t *testing.T) {
	if _, err := NewAudio(8000, 1, make([]float32, 320)); err == nil {
		t.Fatal("NewAudio accepted non-normalized sample rate")
	}
	audio, err := NewAudio(16000, 1, make([]float32, 320))
	if err != nil {
		t.Fatal(err)
	}
	if audio.SampleRate != 16000 || audio.Channels != 1 {
		t.Fatalf("unexpected audio format: %+v", audio)
	}
}
