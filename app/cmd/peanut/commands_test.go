package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"peanut/internal/audio"
	"peanut/internal/audio/capture"
	"peanut/internal/audio/playback"
	mlspeaker "peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
)

type fakeSynthesizer struct{ text string }

func (f *fakeSynthesizer) Synthesize(_ context.Context, text string) (tts.Result, error) {
	f.text = text
	a, _ := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	return tts.Result{Audio: a}, nil
}

type fakeTranscriber struct{}

func (fakeTranscriber) Transcribe(context.Context, audio.Audio) (stt.Result, error) {
	return stt.Result{Text: "hello"}, nil
}

type failedSpeaker struct{}

func (failedSpeaker) Identify(context.Context, audio.Audio) mlspeaker.Result {
	return mlspeaker.Result{ID: "wrong", Err: errors.New("unavailable")}
}

func TestDispatchSpeak(t *testing.T) {
	synth := &fakeSynthesizer{}
	player := &playback.Fake{}
	if err := dispatch(context.Background(), []string{"speak", "hello", "world"}, commandDependencies{Player: player, Synthesizer: synth}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if synth.text != "hello world" || len(player.Played) != 1 {
		t.Fatalf("text = %q, plays = %d", synth.text, len(player.Played))
	}
}

func TestDispatchTranscribeUsesUnknownSpeakerFallback(t *testing.T) {
	path := writeTestWAV(t)
	var output bytes.Buffer
	err := dispatch(context.Background(), []string{"transcribe", path}, commandDependencies{Transcriber: fakeTranscriber{}, Speaker: failedSpeaker{}}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "speaker=unknown\nhello\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDispatchTestAudio(t *testing.T) {
	path := writeTestWAV(t)
	player := &playback.Fake{}
	if err := dispatch(context.Background(), []string{"test-audio", path}, commandDependencies{Player: player}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(player.Played) != 1 {
		t.Fatalf("plays = %d, want 1", len(player.Played))
	}
}

func TestDispatchRejectsUnknownCommand(t *testing.T) {
	err := dispatch(context.Background(), []string{"nope"}, commandDependencies{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("error = %v, want usage", err)
	}
}

func writeTestWAV(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.wav")
	a, err := audio.NewAudio(audio.SampleRate, audio.Channels, make([]float32, audio.FrameSamples))
	if err != nil {
		t.Fatal(err)
	}
	if err := capture.WriteFile(path, a); err != nil {
		t.Fatal(err)
	}
	return path
}
