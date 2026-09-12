package ml_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"peanut/internal/audio"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
)

type fakeWake struct {
	calls  int
	result kws.Result
}

func (f *fakeWake) Detect(context.Context, audio.Frame) (kws.Result, error) {
	f.calls++
	if f.result == (kws.Result{}) {
		return kws.Result{Detected: true, Keyword: "peanut", Score: 0.9}, nil
	}
	return f.result, nil
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

func TestWakeGateRejectsInvalidFrame(t *testing.T) {
	fake := &fakeWake{}
	_, err := kws.NewGate(fake).Detect(context.Background(), audio.Frame{})
	if err == nil || fake.calls != 0 {
		t.Fatalf("invalid frame err=%v calls=%d", err, fake.calls)
	}
}

func TestKWSScoreMustBeFinite(t *testing.T) {
	for _, score := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		if err := (kws.Result{Score: score}).Validate(); err == nil {
			t.Fatalf("accepted KWS score %v", score)
		}
	}
}

func TestWakeGateRejectsInvalidDetectorResult(t *testing.T) {
	fake := &fakeWake{result: kws.Result{Score: float32(math.NaN())}}
	frame, _ := audio.NewFrame(make([]float32, audio.FrameSamples))
	if _, err := kws.NewGate(fake).Detect(context.Background(), frame); err == nil {
		t.Fatal("gate accepted invalid detector result")
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

func TestVADProbabilityMustBeFinite(t *testing.T) {
	for _, probability := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		if err := (vad.Result{Probability: probability}).Validate(); err == nil {
			t.Fatalf("accepted VAD probability %v", probability)
		}
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

func TestSpeakerScoreMustBeFinite(t *testing.T) {
	for _, score := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		if err := (speaker.Result{Score: score}).Validate(); err == nil {
			t.Fatalf("accepted speaker score %v", score)
		}
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
