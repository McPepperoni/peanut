package capture

import (
	"context"
	"errors"
	"testing"

	"peanut/internal/audio"
)

func TestSystemCapturePlatformBoundary(t *testing.T) {
	var system audio.Capture = SystemCapture{}
	frames, err := system.Capture(context.Background())
	if frames != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("frames = %v, error = %v", frames, err)
	}
}
