package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"peanut/internal/audio"
	"peanut/internal/intent"
	"peanut/internal/ml/kws"
	mlspeaker "peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
	"peanut/internal/providers"
)

func TestWriteDebugAudioWritesValidWAV(t *testing.T) {
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0.25})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := writeDebugAudio(root, input); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("debug files = %d, want 1", len(entries))
	}
	file, err := os.Open(filepath.Join(root, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoded, err := audio.ReadWAV(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Samples) != 1 || decoded.Samples[0] != input.Samples[0] {
		t.Fatalf("decoded debug audio = %#v", decoded.Samples)
	}
}

func TestCoordinatorRunsSeparateUtteranceHappyPath(t *testing.T) {
	frame := func(sample float32) audio.Frame {
		samples := make([]float32, audio.FrameSamples)
		for i := range samples {
			samples[i] = sample
		}
		value, err := audio.NewFrame(samples)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	ack, _ := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0.1})
	reply, _ := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0.2})
	provider := &coordinatorProvider{}
	registry := providers.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	player := &coordinatorPlayer{}
	transcriber := &coordinatorTranscriber{}
	speaker := &blockingCoordinatorSpeaker{started: make(chan struct{}), release: make(chan struct{})}
	defer close(speaker.release)
	cfg := DefaultConfig()
	cfg.PreRollFrames = 2
	cfg.PreRollSamples = 2 * audio.FrameSamples
	coordinator, err := NewCoordinator(cfg, Dependencies{
		Capture:         coordinatorCapture{frames: []audio.Frame{frame(0.1), frame(0.2), frame(0.3), frame(0.4)}},
		Player:          player,
		Acknowledgement: ack,
		WakeDetector:    &coordinatorWake{},
		VAD:             &coordinatorVAD{},
		Transcriber:     transcriber,
		Speaker:         speaker,
		IntentParser: coordinatorParser{plan: intent.ActionPlan{
			Version: 1, Status: intent.StatusExecute, Language: "en", Confidence: 1,
			Steps: []intent.ActionRequest{{DeviceID: "device.test", ActionID: "power.set", Arguments: map[string]any{"on": true}}},
		}},
		Synthesizer: coordinatorSynthesizer{result: tts.Result{Audio: reply}},
		Registry:    registry,
		Capabilities: intent.CapabilitySnapshot{Capabilities: []intent.Capability{{
			ID: "capability.test", ProviderID: "provider.test", DeviceID: "device.test", Type: "switch", Name: "Test switch",
			Actions: []intent.ActionDefinition{{ID: "power.set", Arguments: map[string]intent.ArgumentDefinition{"on": {Type: intent.TypeBoolean, Required: true}}}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := coordinator.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-speaker.started:
	case <-time.After(time.Second):
		t.Fatal("speaker identification did not start")
	}

	if coordinator.State() != WaitingWake {
		t.Fatalf("state = %v, want WaitingWake", coordinator.State())
	}
	if len(player.played) != 2 || player.played[0].Samples[0] != 0.1 || player.played[1].Samples[0] != 0.2 {
		t.Fatalf("played = %#v, want ACK then response", player.played)
	}
	if got := len(transcriber.input.Samples); got != 3*audio.FrameSamples {
		t.Fatalf("recorded samples = %d, want %d", got, 3*audio.FrameSamples)
	}
	if len(provider.executed) != 1 || provider.executed[0].ActionID != "power.set" {
		t.Fatalf("executed = %#v", provider.executed)
	}
}

type coordinatorCapture struct{ frames []audio.Frame }

func (f coordinatorCapture) Capture(context.Context) (<-chan audio.Frame, error) {
	frames := make(chan audio.Frame, len(f.frames))
	for _, frame := range f.frames {
		frames <- frame
	}
	close(frames)
	return frames, nil
}

type coordinatorPlayer struct{ played []audio.Audio }

func (f *coordinatorPlayer) Play(_ context.Context, input audio.Audio) error {
	f.played = append(f.played, input)
	return nil
}

type coordinatorWake struct{ detected bool }

func (f *coordinatorWake) Detect(context.Context, audio.Frame) (kws.Result, error) {
	if !f.detected {
		f.detected = true
		return kws.Result{Detected: true, Keyword: "peanut", Score: 1}, nil
	}
	return kws.Result{}, nil
}
func (*coordinatorWake) Reset() error { return nil }

type coordinatorVAD struct{ calls int }

func (f *coordinatorVAD) Detect(context.Context, audio.Frame) (vad.Result, error) {
	f.calls++
	switch f.calls {
	case 2:
		return vad.Result{Endpoint: vad.SpeechStarted, Probability: 1}, nil
	case 3:
		return vad.Result{Endpoint: vad.SpeechEnded, Probability: 1}, nil
	default:
		return vad.Result{}, nil
	}
}
func (*coordinatorVAD) Reset() error { return nil }

type coordinatorTranscriber struct{ input audio.Audio }

func (f *coordinatorTranscriber) Transcribe(_ context.Context, input audio.Audio) (stt.Result, error) {
	f.input = input
	return stt.Result{Text: "turn it on"}, nil
}

type blockingCoordinatorSpeaker struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingCoordinatorSpeaker) Identify(context.Context, audio.Audio) mlspeaker.Result {
	close(s.started)
	<-s.release
	return mlspeaker.Result{ID: "advisory"}
}

type coordinatorParser struct{ plan intent.ActionPlan }

func (f coordinatorParser) Parse(context.Context, string, intent.CapabilitySnapshot) (intent.ActionPlan, error) {
	return f.plan, nil
}

type coordinatorSynthesizer struct{ result tts.Result }

func (f coordinatorSynthesizer) Synthesize(context.Context, string) (tts.Result, error) {
	return f.result, nil
}

type coordinatorProvider struct{ executed []intent.ActionRequest }

func (*coordinatorProvider) Descriptor() providers.ProviderDescriptor {
	return providers.ProviderDescriptor{ID: "provider.test", Type: "test", Name: "Test"}
}
func (*coordinatorProvider) Discover(context.Context) ([]intent.Capability, error) { return nil, nil }
func (f *coordinatorProvider) Execute(_ context.Context, request intent.ActionRequest) (providers.ActionResult, error) {
	f.executed = append(f.executed, request)
	return providers.ActionResult{}, nil
}
