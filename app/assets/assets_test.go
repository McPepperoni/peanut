package assets

import (
	"testing"

	"peanut/internal/audio"
)

func TestAcknowledgementLoadsNormalizedAudio(t *testing.T) {
	ack, err := Acknowledgement()
	if err != nil {
		t.Fatal(err)
	}
	if ack.SampleRate != audio.SampleRate || ack.Channels != audio.Channels || len(ack.Samples) == 0 {
		t.Fatalf("unexpected ACK format: %+v", ack)
	}
}
