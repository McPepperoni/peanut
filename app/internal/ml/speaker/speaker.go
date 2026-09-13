package speaker

import (
	"context"
	"errors"
	"math"

	"peanut/internal/audio"
)

// Result keeps identification failures advisory so a caller can continue.
type Result struct {
	ID    string
	Score float32
	Err   error
}

func (r Result) Validate() error {
	if r.Score < 0 || r.Score > 1 || math.IsNaN(float64(r.Score)) || math.IsInf(float64(r.Score), 0) {
		return errors.New("invalid speaker score")
	}
	return nil
}

type SpeakerIdentifier interface {
	Identify(context.Context, audio.Audio) Result
}
