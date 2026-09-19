//go:build linux

package playback

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"

	"peanut/internal/audio"
)

type SystemPlayer struct{ Device string }

func (p SystemPlayer) Play(ctx context.Context, input audio.Audio) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "aplay", alsaPlaybackArgs(p.Device)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open aplay input: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start aplay: %w", err)
	}
	_, writeErr := stdin.Write(encodePCM(input.Samples))
	closeErr := stdin.Close()
	waitErr := cmd.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	if writeErr != nil {
		return fmt.Errorf("write aplay input: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close aplay input: %w", closeErr)
	}
	if waitErr != nil {
		return fmt.Errorf("aplay: %w", waitErr)
	}
	return nil
}

func alsaPlaybackArgs(device string) []string {
	args := []string{"-q", "-t", "raw", "-f", "S16_LE", "-c", "1", "-r", "16000"}
	if device == "" {
		return args
	}
	return append([]string{"-D", device}, args...)
}

func encodePCM(samples []float32) []byte {
	raw := make([]byte, len(samples)*2)
	for i, sample := range samples {
		if sample <= -1 {
			binary.LittleEndian.PutUint16(raw[i*2:], 0x8000)
			continue
		}
		if sample >= 1 {
			binary.LittleEndian.PutUint16(raw[i*2:], uint16(int16(32767)))
			continue
		}
		value := int16(math.Round(float64(sample) * 32767))
		binary.LittleEndian.PutUint16(raw[i*2:], uint16(value))
	}
	return raw
}
