package pipeline

import (
	"context"
	"errors"
	"fmt"

	"peanut/internal/audio"
	"peanut/internal/intent"
	"peanut/internal/interaction"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
	"peanut/internal/providers"
)

type Dependencies struct {
	Capture         audio.Capture
	Player          audio.Player
	Acknowledgement audio.Audio
	WakeDetector    kws.WakeDetector
	VAD             vad.VAD
	Transcriber     stt.Transcriber
	IntentParser    intent.IntentParser
	Synthesizer     tts.Synthesizer
	Registry        *providers.Registry
	Capabilities    intent.CapabilitySnapshot
}

type Coordinator struct {
	cfg     Config
	deps    Dependencies
	machine *Machine
}

func NewCoordinator(cfg Config, deps Dependencies) (*Coordinator, error) {
	if deps.Capture == nil || deps.Player == nil || deps.WakeDetector == nil || deps.VAD == nil || deps.Transcriber == nil || deps.IntentParser == nil || deps.Synthesizer == nil || deps.Registry == nil {
		return nil, errors.New("coordinator dependencies are incomplete")
	}
	if err := intent.ValidateSnapshot(deps.Capabilities); err != nil {
		return nil, fmt.Errorf("invalid coordinator capabilities: %w", err)
	}
	return &Coordinator{cfg: cfg, deps: deps, machine: NewMachine(cfg)}, nil
}

func (c *Coordinator) State() State { return c.machine.State() }

func (c *Coordinator) Run(ctx context.Context) error {
	frames, err := c.deps.Capture.Capture(ctx)
	if err != nil {
		return err
	}
	var preRoll []audio.Frame
	var recording []float32
	for frame := range frames {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch c.State() {
		case WaitingWake:
			result, err := c.deps.WakeDetector.Detect(ctx, frame)
			if err != nil {
				return err
			}
			if !result.Detected {
				continue
			}
			if err := c.machine.Transition(WakeDetected); err != nil {
				return err
			}
			if err := c.deps.Player.Play(ctx, c.deps.Acknowledgement); err != nil {
				return c.recover(err)
			}
			if err := c.machine.Transition(AcknowledgementFinished); err != nil {
				return c.recover(err)
			}
			preRoll = nil
		case WaitingSpeech, Recording:
			result, err := c.deps.VAD.Detect(ctx, frame)
			if err != nil {
				return c.recover(err)
			}
			if c.State() == WaitingSpeech {
				preRoll = append(preRoll, frame)
				if len(preRoll) > c.cfg.PreRollFrames {
					preRoll = preRoll[1:]
				}
				if result.Endpoint != vad.SpeechStarted {
					continue
				}
				if err := c.machine.Transition(SpeechStarted); err != nil {
					return c.recover(err)
				}
				for _, buffered := range preRoll {
					recording = append(recording, buffered.Samples...)
				}
				continue
			}
			recording = append(recording, frame.Samples...)
			if result.Endpoint != vad.SpeechEnded {
				continue
			}
			if err := c.machine.Transition(SpeechEnded); err != nil {
				return c.recover(err)
			}
			if err := c.process(ctx, recording); err != nil {
				return c.recover(err)
			}
			return nil
		}
	}
	return c.recover(errors.New("capture ended before command completed"))
}

func (c *Coordinator) process(ctx context.Context, samples []float32) error {
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, samples)
	if err != nil {
		return err
	}
	transcription, err := c.deps.Transcriber.Transcribe(ctx, input)
	if err != nil {
		return err
	}
	plan, err := c.deps.IntentParser.Parse(ctx, transcription.Text, c.deps.Capabilities)
	if err != nil {
		return err
	}
	if err := intent.ValidatePlan(plan, c.deps.Capabilities); err != nil {
		return err
	}
	for _, step := range plan.Steps {
		var executed bool
		for _, capability := range c.deps.Capabilities.Capabilities {
			if capability.DeviceID != step.DeviceID {
				continue
			}
			provider, ok := c.deps.Registry.Provider(capability.ProviderID)
			if !ok {
				return fmt.Errorf("provider %q is not registered", capability.ProviderID)
			}
			if _, err := provider.Execute(ctx, step); err != nil {
				return err
			}
			executed = true
			break
		}
		if !executed {
			return fmt.Errorf("no provider for device %q", step.DeviceID)
		}
	}
	if err := c.machine.Transition(ProcessingFinished); err != nil {
		return err
	}
	response, err := c.deps.Synthesizer.Synthesize(ctx, interaction.Success)
	if err != nil {
		return err
	}
	if err := response.Validate(); err != nil {
		return err
	}
	if err := c.machine.Transition(SynthesisFinished); err != nil {
		return err
	}
	if err := c.deps.Player.Play(ctx, response.Audio); err != nil {
		return err
	}
	return c.machine.Transition(PlaybackFinished)
}

func (c *Coordinator) recover(err error) error {
	_ = c.deps.WakeDetector.Reset()
	_ = c.deps.VAD.Reset()
	c.machine = NewMachine(c.cfg)
	return err
}
