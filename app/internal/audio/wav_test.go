package audio

import (
	"bytes"
	"encoding/binary"
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

func TestReadWAVConvertsPCM16ToFloat32(t *testing.T) {
	data := []int16{-32768, 0, 16384, 32767}
	wav := make([]byte, 44+len(data)*2)
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], 16000)
	binary.LittleEndian.PutUint32(wav[28:32], 32000)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(data)*2))
	for i, sample := range data {
		binary.LittleEndian.PutUint16(wav[44+i*2:], uint16(sample))
	}
	audio, err := ReadWAV(bytes.NewReader(wav))
	if err != nil {
		t.Fatal(err)
	}
	want := []float32{-1, 0, 0.5, 32767.0 / 32768.0}
	for i := range want {
		if audio.Samples[i] != want[i] {
			t.Fatalf("sample %d = %v, want %v", i, audio.Samples[i], want[i])
		}
	}
}
