// Package sherpa is the sole boundary for a future sherpa-onnx native runtime.
package sherpa

import (
	"errors"
	"fmt"

	"peanut/internal/ml"
)

var ErrUnavailable = errors.New("sherpa native runtime unavailable")

// Open reserves the native-runtime boundary without making builds depend on CGO.
func Open(manifest ml.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	return fmt.Errorf("%w: build with the sherpa-onnx native adapter", ErrUnavailable)
}
