package pipeline

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	completed := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, ok, timedOut := c.nextFrame(ctx, frames)
		if timedOut {
			if err := c.machine.Transition(Timeout); err != nil {
				return c.recover(err)
			}
			if err := c.resetDetectors(); err != nil {
				return c.recover(err)
			}
			preRoll = nil
			recording = nil
			continue
		}
		if !ok {
			if completed {
				return nil
			}
			return c.recover(errors.New("capture ended before command completed"))
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
			if err := runStage(ctx, c.cfg.AcknowledgementTimeout, func(stageCtx context.Context) error {
				return c.deps.Player.Play(stageCtx, c.deps.Acknowledgement)
			}); err != nil {
				return c.recover(err)
			}
			if err := drain(ctx, frames, c.cfg.AcknowledgementTail); err != nil {
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
			if err := runStage(ctx, c.cfg.ProcessingTimeout, func(stageCtx context.Context) error {
				return c.process(stageCtx, recording)
			}); err != nil {
				return c.recover(err)
			}
			if err := drain(ctx, frames, c.cfg.PlaybackTail); err != nil {
				return c.recover(err)
			}
			completed = true
			preRoll = nil
			recording = nil
		}
	}
}

func (c *Coordinator) nextFrame(ctx context.Context, frames <-chan audio.Frame) (audio.Frame, bool, bool) {
	duration := timeoutFor(c.State(), c.cfg)
	if duration <= 0 {
		select {
		case <-ctx.Done():
			return audio.Frame{}, false, false
		case frame, ok := <-frames:
			return frame, ok, false
		}
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return audio.Frame{}, false, false
	case <-timer.C:
		return audio.Frame{}, false, true
	case frame, ok := <-frames:
		return frame, ok, false
	}
}

func runStage(ctx context.Context, duration time.Duration, fn func(context.Context) error) error {
	if duration <= 0 {
		return fn(ctx)
	}
	stageCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	return fn(stageCtx)
}

func drain(ctx context.Context, frames <-chan audio.Frame, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		case _, ok := <-frames:
			if !ok {
				return nil
			}
		}
	}
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
	if plan.Status != intent.StatusExecute {
		message := "I don't know."
		if plan.Status == intent.StatusClarify {
			message = plan.Clarification
		}
		response, err := runSynthesis(ctx, c.cfg.SynthesisTimeout, c.deps.Synthesizer, message)
		if err != nil {
			return err
		}
		if err := response.Validate(); err != nil {
			return err
		}
		if err := c.machine.Transition(ProcessingFinished); err != nil {
			return err
		}
		if err := c.machine.Transition(SynthesisFinished); err != nil {
			return err
		}
		if err := runStage(ctx, c.cfg.PlaybackTimeout, func(stageCtx context.Context) error {
			return c.deps.Player.Play(stageCtx, response.Audio)
		}); err != nil {
			return err
		}
		if err := c.resetDetectors(); err != nil {
			return err
		}
		return c.machine.Transition(PlaybackFinished)
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
	response, err := runSynthesis(ctx, c.cfg.SynthesisTimeout, c.deps.Synthesizer, interaction.Success)
	if err != nil {
		return err
	}
	if err := response.Validate(); err != nil {
		return err
	}
	if err := c.machine.Transition(SynthesisFinished); err != nil {
		return err
	}
	if err := runStage(ctx, c.cfg.PlaybackTimeout, func(stageCtx context.Context) error {
		return c.deps.Player.Play(stageCtx, response.Audio)
	}); err != nil {
		return err
	}
	if err := c.resetDetectors(); err != nil {
		return err
	}
	return c.machine.Transition(PlaybackFinished)
}

func runSynthesis(ctx context.Context, timeout time.Duration, synthesizer tts.Synthesizer, text string) (tts.Result, error) {
	var result tts.Result
	err := runStage(ctx, timeout, func(stageCtx context.Context) error {
		var err error
		result, err = synthesizer.Synthesize(stageCtx, text)
		return err
	})
	return result, err
}

func (c *Coordinator) resetDetectors() error {
	if err := c.deps.WakeDetector.Reset(); err != nil {
		return fmt.Errorf("reset wake detector: %w", err)
	}
	if err := c.deps.VAD.Reset(); err != nil {
		return fmt.Errorf("reset VAD: %w", err)
	}
	return nil
}

func (c *Coordinator) recover(err error) error {
	if resetErr := c.resetDetectors(); resetErr != nil {
		err = fmt.Errorf("%w; %v", err, resetErr)
	}
	c.machine = NewMachine(c.cfg)
	return err
}
