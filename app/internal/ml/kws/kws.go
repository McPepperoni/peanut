package kws

import (
	"context"
	"errors"
	"math"

	"peanut/internal/audio"
)

type Result struct {
	Detected bool
	Keyword  string
	Score    float32
}

func (r Result) Validate() error {
	if math.IsNaN(float64(r.Score)) || math.IsInf(float64(r.Score), 0) {
		return errors.New("invalid KWS score")
	}
	return nil
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
	if err := frame.Validate(); err != nil {
		return Result{}, err
	}
	if !g.enabled {
		return Result{}, nil
	}
	result, err := g.detector.Detect(ctx, frame)
	if err != nil {
		return Result{}, err
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}
