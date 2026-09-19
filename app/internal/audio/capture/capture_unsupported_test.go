//go:build !linux

package capture

import (
	"context"
	"errors"
	"testing"
)

func TestSystemCaptureIsUnsupported(t *testing.T) {
	frames, err := (SystemCapture{}).Capture(context.Background())
	if frames != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("frames = %v, error = %v, want unsupported", frames, err)
	}
}
