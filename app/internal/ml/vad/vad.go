package vad

import (
	"context"
	"errors"

	"peanut/internal/audio"
)

type Endpoint uint8

const (
	NoEndpoint Endpoint = iota
	SpeechStarted
	SpeechEnded
)

type Result struct {
	Endpoint    Endpoint
	Probability float32
}

func (r Result) Validate() error {
	if r.Endpoint > SpeechEnded || r.Probability < 0 || r.Probability > 1 {
		return errors.New("invalid VAD result")
	}
	return nil
}

type VAD interface {
	Detect(context.Context, audio.Frame) (Result, error)
	Reset() error
}
