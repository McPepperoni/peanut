//go:build !cgo || !sherpa

// Package sherpa isolates the optional sherpa-onnx native runtime.
package sherpa

import (
	"fmt"

	"peanut/internal/audio"
	"peanut/internal/ml"
)

// Open reports the omitted native runtime without making default builds depend on CGO.
func Open(manifest ml.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	return fmt.Errorf("%w: build with the sherpa-onnx native adapter", ErrUnavailable)
}

// Transcribe reports the omitted runtime after validating boundary inputs.
func Transcribe(manifest ml.Manifest, input audio.Audio) (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	if err := input.Validate(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%w: build with the sherpa-onnx native adapter", ErrUnavailable)
}
