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
	"peanut/internal/models"
	"peanut/internal/speaker"
)

type commandDependencies struct {
	ModelRoot   string
	Player      audio.Player
	Synthesizer tts.Synthesizer
	Transcriber stt.Transcriber
	Speaker     mlspeaker.SpeakerIdentifier
	Enroll      func(context.Context, string) error
	Run         func(context.Context) error
}

func dispatch(ctx context.Context, args []string, dependencies commandDependencies, output io.Writer) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "model":
		return dispatchModel(ctx, args[1:], dependencies.ModelRoot, output)
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
		if len(args) != 2 || dependencies.Transcriber == nil {
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
		speakerName := "unknown"
		if dependencies.Speaker != nil {
			speakerName = speaker.Name(dependencies.Speaker.Identify(ctx, input))
		}
		_, err = fmt.Fprintf(output, "speaker=%s\n%s\n", speakerName, result.Text)
		return err
	case "enroll":
		if len(args) != 2 || dependencies.Enroll == nil {
			return usage()
		}
		return dependencies.Enroll(ctx, args[1])
	case "run":
		if len(args) != 1 || dependencies.Run == nil {
			return usage()
		}
		return dependencies.Run(ctx)
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

func dispatchModel(ctx context.Context, args []string, root string, output io.Writer) error {
	if len(args) != 1 || (args[0] != "list" && args[0] != "verify") || root == "" {
		return usage()
	}
	snapshot, err := models.NewRegistry(root, nil, nil).Scan(ctx)
	if err != nil {
		return err
	}
	if args[0] == "list" {
		if _, err := fmt.Fprintln(output, "id\trole\truntime\tvalid\tpath"); err != nil {
			return err
		}
		for _, profile := range snapshot.Profiles {
			if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%t\t%s\n", profile.ID, profile.Role, profile.Runtime, profile.Valid, profile.Path); err != nil {
				return err
			}
		}
		return nil
	}
	for _, profile := range snapshot.Profiles {
		if profile.Valid {
			continue
		}
		if _, err := fmt.Fprintf(output, "%s: %s\n", profile.Path, profile.Error); err != nil {
			return err
		}
	}
	for _, profile := range snapshot.Profiles {
		if !profile.Valid {
			return errors.New("one or more model profiles are invalid")
		}
	}
	return nil
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
	return errors.New("usage: run | model <list|verify> | enroll <id> | speak <text> | transcribe <input.wav> | test-audio <input.wav> (speak/transcribe require configured models)")
}
