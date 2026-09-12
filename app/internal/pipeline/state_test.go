package pipeline

import (
	"testing"
	"time"
)

func TestMachineFollowsExplicitInteractionStates(t *testing.T) {
	now := time.Unix(0, 0)
	machine := NewMachineAt(DefaultConfig(), now)
	if machine.State() != WaitingWake {
		t.Fatalf("initial state = %v, want WaitingWake", machine.State())
	}
	for _, step := range []struct {
		event Event
		want  State
	}{
		{WakeDetected, Acknowledging},
		{AcknowledgementFinished, WaitingSpeech},
		{SpeechStarted, Recording},
		{SpeechEnded, Processing},
		{ProcessingFinished, Synthesizing},
		{SynthesisFinished, Speaking},
		{PlaybackFinished, WaitingWake},
	} {
		if err := machine.TransitionAt(step.event, now); err != nil {
			t.Fatalf("event %v: %v", step.event, err)
		}
		if machine.State() != step.want {
			t.Fatalf("state = %v, want %v", machine.State(), step.want)
		}
	}
}

func TestMachineRejectsPrematureEvents(t *testing.T) {
	machine := NewMachine(DefaultConfig())
	if err := machine.Transition(SpeechStarted); err == nil {
		t.Fatal("accepted SpeechStarted while waiting for wake")
	}
	if err := machine.Transition(WakeDetected); err != nil {
		t.Fatal(err)
	}
	if err := machine.Transition(SpeechStarted); err == nil {
		t.Fatal("accepted SpeechStarted while acknowledging")
	}
	if machine.State() != Acknowledging {
		t.Fatalf("premature event changed state to %v", machine.State())
	}
}

func TestMachineTimeoutsAreDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	start := time.Unix(0, 0)
	tests := []struct {
		name     string
		enter    func(*Machine, time.Time) error
		duration time.Duration
		want     State
	}{
		{"acknowledgement", func(m *Machine, at time.Time) error { return m.TransitionAt(WakeDetected, at) }, cfg.AcknowledgementTimeout, WaitingWake},
		{"waiting speech", func(m *Machine, at time.Time) error {
			return transitionThrough(m, at, WakeDetected, AcknowledgementFinished)
		}, cfg.NoSpeechTimeout, WaitingWake},
		{"recording", func(m *Machine, at time.Time) error {
			return transitionThrough(m, at, WakeDetected, AcknowledgementFinished, SpeechStarted)
		}, cfg.MaximumCommandDuration, Processing},
		{"processing", func(m *Machine, at time.Time) error {
			return transitionThrough(m, at, WakeDetected, AcknowledgementFinished, SpeechStarted, SpeechEnded)
		}, cfg.ProcessingTimeout, WaitingWake},
		{"synthesizing", func(m *Machine, at time.Time) error {
			return transitionThrough(m, at, WakeDetected, AcknowledgementFinished, SpeechStarted, SpeechEnded, ProcessingFinished)
		}, cfg.SynthesisTimeout, WaitingWake},
		{"speaking playback", func(m *Machine, at time.Time) error {
			return transitionThrough(m, at, WakeDetected, AcknowledgementFinished, SpeechStarted, SpeechEnded, ProcessingFinished, SynthesisFinished)
		}, cfg.PlaybackTimeout, WaitingWake},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := NewMachineAt(cfg, start)
			if err := tt.enter(machine, start); err != nil {
				t.Fatal(err)
			}
			if err := machine.TransitionAt(Timeout, start.Add(tt.duration-time.Nanosecond)); err == nil {
				t.Fatal("accepted premature timeout")
			}
			if err := machine.TransitionAt(Timeout, start.Add(tt.duration)); err != nil {
				t.Fatal(err)
			}
			if machine.State() != tt.want {
				t.Fatalf("timeout state = %v, want %v", machine.State(), tt.want)
			}
		})
	}
}

func transitionThrough(machine *Machine, at time.Time, events ...Event) error {
	for _, event := range events {
		if err := machine.TransitionAt(event, at); err != nil {
			return err
		}
	}
	return nil
}
