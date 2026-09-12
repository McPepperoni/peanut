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
	frame := Frame{Samples: samples}
	if err := frame.Validate(); err != nil {
		return Frame{}, err
	}
	return Frame{Samples: append([]float32(nil), samples...)}, nil
}

func NewAudio(sampleRate, channels int, samples []float32) (Audio, error) {
	audio := Audio{SampleRate: sampleRate, Channels: channels, Samples: samples}
	if err := audio.Validate(); err != nil {
		return Audio{}, err
	}
	return Audio{SampleRate: sampleRate, Channels: channels, Samples: append([]float32(nil), samples...)}, nil
}

func (f Frame) Validate() error {
	if len(f.Samples) != FrameSamples {
		return errors.New("audio frame must contain 320 samples")
	}
	return nil
}

func (a Audio) Validate() error {
	if a.SampleRate != SampleRate || a.Channels != Channels {
		return errors.New("audio must be 16 kHz mono")
	}
	if len(a.Samples) == 0 {
		return errors.New("audio must contain samples")
	}
	return nil
}
