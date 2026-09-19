//go:build cgo && sherpa

package sherpa

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"peanut/internal/audio"
	"peanut/internal/ml"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
	speakerstore "peanut/internal/speaker"
)

var (
	_ kws.WakeDetector          = (*WakeDetector)(nil)
	_ vad.VAD                   = (*VoiceActivityDetector)(nil)
	_ stt.Transcriber           = (*Transcriber)(nil)
	_ speaker.SpeakerIdentifier = (*SpeakerIdentifier)(nil)
	_ speakerstore.Embedder     = (*SpeakerIdentifier)(nil)
	_ tts.Synthesizer           = (*Synthesizer)(nil)
)

func TestNativeAdaptersHonorCanceledContextBeforeInference(t *testing.T) {
	frame, err := audio.NewFrame(make([]float32, audio.FrameSamples))
	if err != nil {
		t.Fatal(err)
	}
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (&WakeDetector{}).Detect(ctx, frame); !errors.Is(err, context.Canceled) {
		t.Fatalf("KWS error = %v, want canceled", err)
	}
	if _, err := (&VoiceActivityDetector{}).Detect(ctx, frame); !errors.Is(err, context.Canceled) {
		t.Fatalf("VAD error = %v, want canceled", err)
	}
	if result := (&SpeakerIdentifier{}).Identify(ctx, input); !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("speaker error = %v, want canceled", result.Err)
	}
	if _, err := (&Synthesizer{}).Synthesize(ctx, "test"); !errors.Is(err, context.Canceled) {
		t.Fatalf("TTS error = %v, want canceled", err)
	}
}

func TestNativeAdapterCloseIsIdempotent(t *testing.T) {
	for name, close := range map[string]func() error{
		"KWS":     (&WakeDetector{}).Close,
		"VAD":     (&VoiceActivityDetector{}).Close,
		"speaker": (&SpeakerIdentifier{}).Close,
		"TTS":     (&Synthesizer{}).Close,
	} {
		t.Run(name, func(t *testing.T) {
			if err := close(); err != nil {
				t.Fatal(err)
			}
			if err := close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNativeSmoke(t *testing.T) {
	root := os.Getenv("PEANUT_MODEL_ROOT")
	if root == "" {
		t.Skip("set PEANUT_MODEL_ROOT to run native sherpa smoke test")
	}
	manifest := ml.Manifest{Paths: ml.Paths{
		KWS:     filepath.Join(root, ml.KWSBundle),
		VAD:     filepath.Join(root, ml.VADBundle),
		STT:     filepath.Join(root, ml.STTBundle),
		Speaker: filepath.Join(root, ml.SpeakerBundle),
		TTS:     filepath.Join(root, ml.TTSBundle),
	}, Provider: ml.CPUProvider, Threads: 1}
	if err := manifest.Validate(); err != nil {
		t.Skipf("native sherpa models unavailable: %v", err)
	}
	wake, err := NewWakeDetector(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer wake.Close()
	voice, err := NewVAD(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer voice.Close()
	embedder, err := NewSpeakerIdentifier(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer embedder.Close()
	synthesizer, err := NewSynthesizer(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer synthesizer.Close()

	frame, err := audio.NewFrame(make([]float32, audio.FrameSamples))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wake.Detect(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	if err := wake.Reset(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := voice.Detect(context.Background(), frame); err != nil {
			t.Fatal(err)
		}
	}
	if err := voice.Reset(); err != nil {
		t.Fatal(err)
	}

	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, make([]float32, 2*audio.SampleRate))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Transcribe(manifest, input); err != nil {
		t.Fatal(err)
	}
	if _, err := embedder.Embed(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	result, err := synthesizer.Synthesize(context.Background(), "Hello from Peanut.")
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
}
