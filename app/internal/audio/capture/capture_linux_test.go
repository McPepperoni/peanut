//go:build linux

package capture

import (
	"encoding/binary"
	"testing"

	"peanut/internal/audio"
)

func TestALSAArgsHonorDevice(t *testing.T) {
	if got := alsaCaptureArgs("hw:1,0"); len(got) != 11 || got[0] != "-D" || got[1] != "hw:1,0" {
		t.Fatalf("args = %v", got)
	}
	if got := alsaCaptureArgs(""); len(got) != 9 {
		t.Fatalf("default args = %v", got)
	}
}

func TestPCMFrameNormalizesSigned16LittleEndian(t *testing.T) {
	raw := make([]byte, audio.FrameSamples*2)
	binary.LittleEndian.PutUint16(raw[0:], 0x8000)
	binary.LittleEndian.PutUint16(raw[2:], 0x7fff)
	frame, err := pcmFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Samples[0] != -1 || frame.Samples[1] <= 0.99 {
		t.Fatalf("samples = %v, want -1 and near 1", frame.Samples[:2])
	}
}
