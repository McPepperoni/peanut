//go:build !cgo || !sherpa

// Package sherpa isolates the optional sherpa-onnx native runtime.
package sherpa

import (
	"context"
	"fmt"

	"peanut/internal/audio"
	"peanut/internal/ml"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
)

type WakeDetector struct{}
type VoiceActivityDetector struct{}
type Transcriber struct{}
type SpeakerIdentifier struct{}
type Synthesizer struct{}

func unavailable() error {
	return fmt.Errorf("%w: build with the sherpa-onnx native adapter", ErrUnavailable)
}

func NewWakeDetector(manifest ml.Manifest) (*WakeDetector, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return nil, unavailable()
}

func NewVAD(manifest ml.Manifest) (*VoiceActivityDetector, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return nil, unavailable()
}

func NewTranscriber(manifest ml.Manifest) (*Transcriber, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return nil, unavailable()
}

func NewSpeakerIdentifier(manifest ml.Manifest) (*SpeakerIdentifier, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return nil, unavailable()
}

func NewSynthesizer(manifest ml.Manifest) (*Synthesizer, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return nil, unavailable()
}

func (*WakeDetector) Detect(_ context.Context, frame audio.Frame) (kws.Result, error) {
	if err := frame.Validate(); err != nil {
		return kws.Result{}, err
	}
	return kws.Result{}, unavailable()
}

func (*WakeDetector) Reset() error { return unavailable() }

func (*VoiceActivityDetector) Detect(_ context.Context, frame audio.Frame) (vad.Result, error) {
	if err := frame.Validate(); err != nil {
		return vad.Result{}, err
	}
	return vad.Result{}, unavailable()
}

func (*VoiceActivityDetector) Reset() error { return unavailable() }

func (*Transcriber) Transcribe(_ context.Context, input audio.Audio) (stt.Result, error) {
	if err := input.Validate(); err != nil {
		return stt.Result{}, err
	}
	return stt.Result{}, unavailable()
}

func (*SpeakerIdentifier) Identify(_ context.Context, input audio.Audio) speaker.Result {
	if err := input.Validate(); err != nil {
		return speaker.Result{Err: err}
	}
	return speaker.Result{Err: unavailable()}
}

func (*Synthesizer) Synthesize(_ context.Context, _ string) (tts.Result, error) {
	return tts.Result{}, unavailable()
}

// Open reports the omitted native runtime without making default builds depend on CGO.
func Open(manifest ml.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	return unavailable()
}

// Transcribe reports the omitted runtime after validating boundary inputs.
func Transcribe(manifest ml.Manifest, input audio.Audio) (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	if err := input.Validate(); err != nil {
		return "", err
	}
	return "", unavailable()
}
