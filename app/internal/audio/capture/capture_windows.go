//go:build windows

package capture

import (
	"context"
	"fmt"
	"runtime"

	"peanut/internal/audio"
)

type SystemCapture struct{ Device string }

func (SystemCapture) Capture(context.Context) (<-chan audio.Frame, error) {
	return nil, fmt.Errorf("%w on %s", ErrUnsupported, runtime.GOOS)
}
