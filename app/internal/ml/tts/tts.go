package tts

import (
	"context"
	"errors"

	"peanut/internal/audio"
)

type Result struct{ Audio audio.Audio }

func (r Result) Validate() error {
	if r.Audio.SampleRate != audio.SampleRate || r.Audio.Channels != audio.Channels || len(r.Audio.Samples) == 0 {
		return errors.New("TTS audio must be non-empty 16 kHz mono")
	}
	return nil
}

type Synthesizer interface {
	Synthesize(context.Context, string) (Result, error)
}
