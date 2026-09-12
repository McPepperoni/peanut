package audio

import (
	"context"
	"errors"
)

const (
	SampleRate     = 16000
	Channels       = 1
	FrameSamples   = 320
	PreRollFrames  = 15
	PreRollSamples = FrameSamples * PreRollFrames
)

type Frame struct {
	Samples []float32
}

type Audio struct {
	SampleRate int
	Channels   int
	Samples    []float32
}

type Capture interface {
	Capture(context.Context) (<-chan Frame, error)
}

type Player interface {
	Play(context.Context, Audio) error
}

func NewFrame(samples []float32) (Frame, error) {
	if len(samples) != FrameSamples {
		return Frame{}, errors.New("audio frame must contain 320 samples")
	}
	return Frame{Samples: append([]float32(nil), samples...)}, nil
}

func NewAudio(sampleRate, channels int, samples []float32) (Audio, error) {
	if sampleRate != SampleRate || channels != Channels {
		return Audio{}, errors.New("audio must be 16 kHz mono")
	}
	return Audio{SampleRate: sampleRate, Channels: channels, Samples: append([]float32(nil), samples...)}, nil
}
