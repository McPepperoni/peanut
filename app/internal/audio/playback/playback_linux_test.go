//go:build linux

package playback

import (
	"encoding/binary"
	"testing"

)

func TestALSAArgsHonorDevice(t *testing.T) {
	if got := alsaPlaybackArgs("hw:1,0"); len(got) != 11 || got[0] != "-D" || got[1] != "hw:1,0" {
		t.Fatalf("args = %v", got)
	}
	if got := alsaPlaybackArgs(""); len(got) != 9 {
		t.Fatalf("default args = %v", got)
	}
}

func TestPCMEncodingClampsFloatSamples(t *testing.T) {
	raw := encodePCM([]float32{-1, 1, 0})
	if got := int16(binary.LittleEndian.Uint16(raw[0:])); got != -32768 {
		t.Fatalf("minimum = %d", got)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[2:])); got != 32767 {
		t.Fatalf("maximum = %d", got)
	}
}
