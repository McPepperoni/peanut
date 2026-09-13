package capture

import (
	"context"
	"errors"
	"fmt"
	"os"

	"peanut/internal/audio"
)

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
	if len(input.Samples)%audio.FrameSamples != 0 {
		return nil, errors.New("WAV samples must align to audio frames")
	}
	frames := make(chan audio.Frame, len(input.Samples)/audio.FrameSamples)
	for offset := 0; offset < len(input.Samples); offset += audio.FrameSamples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		frame, err := audio.NewFrame(input.Samples[offset : offset+audio.FrameSamples])
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
