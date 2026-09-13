package capture

import (
	"context"
	"errors"
	"fmt"
	"os"

	"peanut/internal/audio"
)

var ErrUnsupported = errors.New("system audio capture is unsupported")

type SystemCapture struct{}

func (SystemCapture) Capture(context.Context) (<-chan audio.Frame, error) {
	return nil, ErrUnsupported
}

type File struct{ path string }

func NewFile(path string) *File { return &File{path: path} }

func WriteFile(path string, input audio.Audio) error {
	if err := input.Validate(); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create WAV: %w", err)
	}
	defer file.Close()
	return audio.WriteWAV(file, input)
}

func (f *File) Capture(ctx context.Context) (<-chan audio.Frame, error) {
	if f == nil || f.path == "" {
		return nil, errors.New("WAV path is required")
	}
	file, err := os.Open(f.path)
	if err != nil {
		return nil, fmt.Errorf("open WAV: %w", err)
	}
	defer file.Close()
	input, err := audio.ReadWAV(file)
	if err != nil {
		return nil, err
	}
	frames := make(chan audio.Frame, (len(input.Samples)+audio.FrameSamples-1)/audio.FrameSamples)
	for offset := 0; offset < len(input.Samples); offset += audio.FrameSamples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(offset+audio.FrameSamples, len(input.Samples))
		samples := input.Samples[offset:end]
		if len(samples) < audio.FrameSamples {
			padded := make([]float32, audio.FrameSamples)
			copy(padded, samples)
			samples = padded
		}
		frame, err := audio.NewFrame(samples)
		if err != nil {
			return nil, err
		}
		frames <- frame
	}
	close(frames)
	return frames, nil
}

type Fake struct{ frames []audio.Frame }

func NewFake(frames []audio.Frame) *Fake {
	copy := make([]audio.Frame, len(frames))
	for index, frame := range frames {
		copy[index], _ = audio.NewFrame(frame.Samples)
	}
	return &Fake{frames: copy}
}

func (f *Fake) Capture(ctx context.Context) (<-chan audio.Frame, error) {
	if f == nil {
		return nil, errors.New("fake capture is required")
	}
	output := make(chan audio.Frame, len(f.frames))
	for _, frame := range f.frames {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		copy, err := audio.NewFrame(frame.Samples)
		if err != nil {
			return nil, err
		}
		output <- copy
	}
	close(output)
	return output, nil
}
