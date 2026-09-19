//go:build !windows && !linux

package playback

import (
	"context"
	"fmt"
	"runtime"

	"peanut/internal/audio"
)

type SystemPlayer struct{ Device string }

func (SystemPlayer) Play(context.Context, audio.Audio) error {
	return fmt.Errorf("%w on %s", ErrUnsupported, runtime.GOOS)
}
