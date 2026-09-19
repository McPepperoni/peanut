//go:build linux

package capture

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"

	"peanut/internal/audio"
)

type SystemCapture struct{ Device string }

func (c SystemCapture) Capture(ctx context.Context) (<-chan audio.Frame, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "arecord", alsaCaptureArgs(c.Device)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open arecord output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start arecord: %w", err)
	}
	frames := make(chan audio.Frame, 4)
	go func() {
		defer close(frames)
		defer stdout.Close()
		defer cmd.Wait()
		for {
			frameBytes := make([]byte, audio.FrameSamples*2)
			if _, err := io.ReadFull(stdout, frameBytes); err != nil {
				return
			}
			frame, err := pcmFrame(frameBytes)
			if err != nil {
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	return frames, nil
}

func alsaCaptureArgs(device string) []string {
	args := []string{"-q", "-t", "raw", "-f", "S16_LE", "-c", "1", "-r", "16000"}
	if device == "" {
		return args
	}
	return append([]string{"-D", device}, args...)
}

func pcmFrame(raw []byte) (audio.Frame, error) {
	if len(raw) != audio.FrameSamples*2 {
		return audio.Frame{}, fmt.Errorf("PCM frame has %d bytes", len(raw))
	}
	samples := make([]float32, audio.FrameSamples)
	for i := range samples {
		samples[i] = float32(int16(binary.LittleEndian.Uint16(raw[i*2:]))) / 32768
	}
	return audio.NewFrame(samples)
}
