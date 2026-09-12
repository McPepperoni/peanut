package kws

import (
	"context"

	"peanut/internal/audio"
)

type Result struct {
	Detected bool
	Keyword  string
	Score    float32
}

type WakeDetector interface {
	Detect(context.Context, audio.Frame) (Result, error)
	Reset() error
}

type Gate struct {
	detector WakeDetector
	enabled  bool
}

func NewGate(detector WakeDetector) *Gate { return &Gate{detector: detector, enabled: true} }

func (g *Gate) SetEnabled(enabled bool) { g.enabled = enabled }

func (g *Gate) Detect(ctx context.Context, frame audio.Frame) (Result, error) {
	if !g.enabled {
		return Result{}, nil
	}
	return g.detector.Detect(ctx, frame)
}
