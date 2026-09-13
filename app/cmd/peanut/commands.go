package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"peanut/internal/audio"
	"peanut/internal/audio/playback"
	mlspeaker "peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/speaker"
)

type commandDependencies struct {
	Player      audio.Player
	Synthesizer tts.Synthesizer
	Transcriber stt.Transcriber
	Speaker     mlspeaker.SpeakerIdentifier
}

func dispatch(ctx context.Context, args []string, dependencies commandDependencies, output io.Writer) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "speak":
		if len(args) < 2 || dependencies.Synthesizer == nil || dependencies.Player == nil {
			return usage()
		}
		result, err := dependencies.Synthesizer.Synthesize(ctx, strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
		if err := result.Validate(); err != nil {
			return err
		}
		return dependencies.Player.Play(ctx, result.Audio)
	case "transcribe":
		if len(args) != 2 || dependencies.Transcriber == nil || dependencies.Speaker == nil {
			return usage()
		}
		input, err := readWAV(args[1])
		if err != nil {
			return err
		}
		result, err := dependencies.Transcriber.Transcribe(ctx, input)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "speaker=%s\n%s\n", speaker.Name(dependencies.Speaker.Identify(ctx, input)), result.Text)
		return err
	case "test-audio":
		if len(args) != 2 || dependencies.Player == nil {
			return usage()
		}
		file, err := os.Open(args[1])
		if err != nil {
			return err
		}
		defer file.Close()
		return playback.PlayWAV(ctx, dependencies.Player, file)
	default:
		return usage()
	}
}

func readWAV(path string) (audio.Audio, error) {
	file, err := os.Open(path)
	if err != nil {
		return audio.Audio{}, err
	}
	defer file.Close()
	return audio.ReadWAV(file)
}

func usage() error {
	return errors.New("usage: speak <text> | transcribe <input.wav> | test-audio <input.wav> (speak/transcribe require configured models)")
}
