package main

import (
	"bytes"
	"context"
	"errors"
	"os"
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

func TestModelCommandsUseModelsRoot(t *testing.T) {
	root := t.TempDir()
	writeValidIntentManifest(t, root)
	var output bytes.Buffer
	deps := commandDependencies{ModelRoot: root}
	if err := dispatch(context.Background(), []string{"model", "list"}, deps, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "intent-local") {
		t.Fatalf("output = %q", output.String())
	}
}

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

func TestDispatchTranscribeWithoutSpeakerIdentifier(t *testing.T) {
	path := writeTestWAV(t)
	var output bytes.Buffer
	err := dispatch(context.Background(), []string{"transcribe", path}, commandDependencies{Transcriber: fakeTranscriber{}}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "speaker=unknown\nhello\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDispatchEnrollAndRun(t *testing.T) {
	var enrolled string
	ran := false
	dependencies := commandDependencies{
		Enroll: func(_ context.Context, id string) error {
			enrolled = id
			return nil
		},
		Run: func(context.Context) error {
			ran = true
			return nil
		},
	}
	if err := dispatch(context.Background(), []string{"enroll", "alice"}, dependencies, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := dispatch(context.Background(), []string{"run"}, dependencies, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if enrolled != "alice" || !ran {
		t.Fatalf("enrolled = %q, ran = %v", enrolled, ran)
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

func writeValidIntentManifest(t *testing.T, root string) {
	t.Helper()
	directory := filepath.Join(root, "intent", "local")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "model.gguf"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "model.json"), []byte(`{"id":"intent-local","role":"intent","runtime":"local","entry":"model.gguf"}`), 0644); err != nil {
		t.Fatal(err)
	}
}
