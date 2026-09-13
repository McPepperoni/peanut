package audio

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

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

func TestReadWAVRejectsInputLargerThan64MiB(t *testing.T) {
	_, err := ReadWAV(io.LimitReader(zeroReader{}, 64<<20+1))
	if err == nil || !strings.Contains(err.Error(), "64 MiB") {
		t.Fatalf("error = %v, want 64 MiB limit", err)
	}
}

func TestReadWAVBoxAveragesWhenDownsampling(t *testing.T) {
	data := make([]int16, 640)
	for i := range data {
		if i%2 == 0 {
			data[i] = 32767
		} else {
			data[i] = -32767
		}
	}
	wav := pcm16WAV(32000, data)

	got, err := ReadWAV(bytes.NewReader(wav))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Samples) != 320 {
		t.Fatalf("sample count = %d, want 320", len(got.Samples))
	}
	for i, sample := range got.Samples {
		if sample != 0 {
			t.Fatalf("sample %d = %v, want box average 0", i, sample)
		}
	}
}

func pcm16WAV(rate uint32, data []int16) []byte {
	wav := make([]byte, 44+len(data)*2)
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], rate)
	binary.LittleEndian.PutUint32(wav[28:32], rate*2)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(data)*2))
	for i, sample := range data {
		binary.LittleEndian.PutUint16(wav[44+i*2:], uint16(sample))
	}
	return wav
}
