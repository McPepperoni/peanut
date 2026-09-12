package stt

import (
	"context"

	"peanut/internal/audio"
)

type Result struct {
	Text string
}

type Transcriber interface {
	Transcribe(context.Context, audio.Audio) (Result, error)
}
