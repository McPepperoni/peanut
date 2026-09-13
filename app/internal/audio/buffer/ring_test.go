package buffer

import (
	"testing"

	"peanut/internal/audio"
)

func TestRingKeeps15FramesAnd4800Samples(t *testing.T) {
	ring := New(15)
	for i := 0; i < 16; i++ {
		samples := make([]float32, 320)
		samples[0] = float32(i) / 16
		frame, err := audio.NewFrame(samples)
		if err != nil {
			t.Fatal(err)
		}
		ring.Push(frame)
	}
	snapshot := ring.Snapshot()
	if len(snapshot) != 15 {
		t.Fatalf("snapshot frame count = %d, want 15", len(snapshot))
	}
	if snapshot[0].Samples[0] != 1.0/16 || snapshot[14].Samples[0] != 15.0/16 {
		t.Fatalf("snapshot does not contain newest 15 frames")
	}
	if len(snapshot)*len(snapshot[0].Samples) != 4800 {
		t.Fatalf("snapshot samples = %d, want 4800", len(snapshot)*len(snapshot[0].Samples))
	}
	frame := snapshot[0]
	frame.Samples[0] = 99
	if ring.Snapshot()[0].Samples[0] == 99 {
		t.Fatal("ring snapshot aliases stored frame")
	}
}
