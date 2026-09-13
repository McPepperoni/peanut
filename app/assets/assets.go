package assets

import (
	"bytes"
	_ "embed"

	"peanut/internal/audio"
)

//go:embed ack.wav
var acknowledgementWAV []byte

func Acknowledgement() (audio.Audio, error) {
	return audio.ReadWAV(bytes.NewReader(acknowledgementWAV))
}
