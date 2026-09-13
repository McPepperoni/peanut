package playback

import (
	"context"
	"errors"
	"io"

	"peanut/internal/audio"
)

type Fake struct{ Played []audio.Audio }

func (f *Fake) Play(_ context.Context, input audio.Audio) error {
	if err := input.Validate(); err != nil {
		return err
	}
	copy, _ := audio.NewAudio(input.SampleRate, input.Channels, input.Samples)
	f.Played = append(f.Played, copy)
	return nil
}

func PlayWAV(ctx context.Context, player audio.Player, input io.Reader) error {
	if player == nil {
		return errors.New("audio player is required")
	}
	decoded, err := audio.ReadWAV(input)
	if err != nil {
		return err
	}
	return player.Play(ctx, decoded)
}
