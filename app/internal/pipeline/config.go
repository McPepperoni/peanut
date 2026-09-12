package pipeline

import (
	"time"

	"peanut/internal/audio"
)

type Config struct {
	PreRollFrames          int
	PreRollSamples         int
	VADThreshold           float32
	AcknowledgementTimeout time.Duration
	NoSpeechTimeout        time.Duration
	MaximumCommandDuration time.Duration
	ProcessingTimeout      time.Duration
	SynthesisTimeout       time.Duration
	PlaybackTimeout        time.Duration
}

func DefaultConfig() Config {
	return Config{PreRollFrames: audio.PreRollFrames, PreRollSamples: audio.PreRollSamples, VADThreshold: 0.5, AcknowledgementTimeout: 2 * time.Second, NoSpeechTimeout: 5 * time.Second, MaximumCommandDuration: 30 * time.Second, ProcessingTimeout: 10 * time.Second, SynthesisTimeout: 10 * time.Second, PlaybackTimeout: 10 * time.Second}
}
