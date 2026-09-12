package speaker

import (
	"context"

	"peanut/internal/audio"
)

// Result keeps identification failures advisory so a caller can continue.
type Result struct {
	ID    string
	Score float32
	Err   error
}

type SpeakerIdentifier interface {
	Identify(context.Context, audio.Audio) Result
}
