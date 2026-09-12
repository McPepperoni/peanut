package tts

import (
	"context"

	"peanut/internal/audio"
)

type Result struct{ Audio audio.Audio }

func (r Result) Validate() error {
	return r.Audio.Validate()
}

type Synthesizer interface {
	Synthesize(context.Context, string) (Result, error)
}
