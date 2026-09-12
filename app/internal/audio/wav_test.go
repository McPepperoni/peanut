package audio

import (
	"bytes"
	"testing"
)

func TestWAVRoundTripUsesNormalizedFloat32(t *testing.T) {
	samples := []float32{-1, -0.25, 0, 0.5, 1}
	want, err := NewAudio(16000, 1, samples)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := WriteWAV(&encoded, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadWAV(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.SampleRate != 16000 || got.Channels != 1 || len(got.Samples) != len(samples) {
		t.Fatalf("unexpected decoded audio: %+v", got)
	}
	for i := range samples {
		if got.Samples[i] != samples[i] {
			t.Fatalf("sample %d = %v, want %v", i, got.Samples[i], samples[i])
		}
	}
}
