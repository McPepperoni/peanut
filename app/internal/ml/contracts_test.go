package ml_test

import (
	"context"
	"errors"
	"testing"

	"peanut/internal/audio"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
)

type fakeWake struct{ calls int }

func (f *fakeWake) Detect(context.Context, audio.Frame) (kws.Result, error) {
	f.calls++
	return kws.Result{Detected: true, Keyword: "peanut", Score: 0.9}, nil
}
func (*fakeWake) Reset() error { return nil }

func TestWakeGateSkipsDisabledDetector(t *testing.T) {
	fake := &fakeWake{}
	gate := kws.NewGate(fake)
	gate.SetEnabled(false)
	result, err := gate.Detect(context.Background(), audio.Frame{Samples: make([]float32, audio.FrameSamples)})
	if err != nil || result.Detected || fake.calls != 0 {
		t.Fatalf("disabled gate result=%+v err=%v calls=%d", result, err, fake.calls)
	}
	gate.SetEnabled(true)
	result, err = gate.Detect(context.Background(), audio.Frame{Samples: make([]float32, audio.FrameSamples)})
	if err != nil || !result.Detected || fake.calls != 1 {
		t.Fatalf("enabled gate result=%+v err=%v calls=%d", result, err, fake.calls)
	}
}

func TestVADEndpointValues(t *testing.T) {
	for _, endpoint := range []vad.Endpoint{vad.NoEndpoint, vad.SpeechStarted, vad.SpeechEnded} {
		result := vad.Result{Endpoint: endpoint, Probability: 0.75}
		if err := result.Validate(); err != nil {
			t.Fatalf("endpoint %v: %v", endpoint, err)
		}
	}
	if err := (vad.Result{Endpoint: vad.Endpoint(99), Probability: 0.5}).Validate(); err == nil {
		t.Fatal("accepted invalid endpoint")
	}
}

type fakeTranscriber struct {
	result stt.Result
	err    error
}

func (f fakeTranscriber) Transcribe(context.Context, audio.Audio) (stt.Result, error) {
	return f.result, f.err
}

func TestTranscriberAllowsEmptyResultAndReturnsErrors(t *testing.T) {
	pcm, _ := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	empty, err := (fakeTranscriber{}).Transcribe(context.Background(), pcm)
	if err != nil || empty.Text != "" {
		t.Fatalf("empty transcription result=%+v err=%v", empty, err)
	}
	want := errors.New("decode failed")
	_, err = (fakeTranscriber{err: want}).Transcribe(context.Background(), pcm)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

type fakeSpeaker struct{ result speaker.Result }

func (f fakeSpeaker) Identify(context.Context, audio.Audio) speaker.Result { return f.result }

func TestSpeakerFailureIsAdvisory(t *testing.T) {
	want := errors.New("embedding failed")
	result := (fakeSpeaker{result: speaker.Result{Err: want}}).Identify(context.Background(), audio.Audio{})
	if !errors.Is(result.Err, want) || result.ID != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestTTSResultRequiresNormalizedNonEmptyAudio(t *testing.T) {
	valid, _ := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	if err := (tts.Result{Audio: valid}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (tts.Result{Audio: audio.Audio{SampleRate: 8000, Channels: 1, Samples: []float32{0}}}).Validate(); err == nil {
		t.Fatal("accepted non-normalized TTS audio")
	}
	if err := (tts.Result{Audio: audio.Audio{SampleRate: audio.SampleRate, Channels: audio.Channels}}).Validate(); err == nil {
		t.Fatal("accepted empty TTS audio")
	}
}
