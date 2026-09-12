package pipeline

import (
	"testing"
	"time"
)

func TestMachineFollowsInteractionStates(t *testing.T) {
	now := time.Unix(0, 0)
	machine := NewMachineAt(DefaultConfig(), now)
	for _, step := range []struct {
		event Event
		want  State
	}{
		{WakeDetected, Wake},
		{AcknowledgementFinished, Listen},
		{SpeechStarted, Record},
		{SpeechEnded, Process},
		{ProcessingFinished, Speak},
		{PlaybackFinished, Idle},
	} {
		if err := machine.TransitionAt(step.event, now); err != nil {
			t.Fatalf("event %v: %v", step.event, err)
		}
		if machine.State() != step.want {
			t.Fatalf("state = %v, want %v", machine.State(), step.want)
		}
	}
}

func TestMachineRejectsInvalidTransition(t *testing.T) {
	machine := NewMachine(DefaultConfig())
	if err := machine.Transition(WakeDetected); err != nil {
		t.Fatal(err)
	}
	if err := machine.Transition(SpeechStarted); err == nil {
		t.Fatal("accepted SpeechStarted during wake acknowledgement")
	}
	if machine.State() != Wake {
		t.Fatalf("invalid transition changed state to %v", machine.State())
	}
}

func TestMachineRecoversFromNoSpeechTimeout(t *testing.T) {
	now := time.Unix(0, 0)
	machine := NewMachineAt(DefaultConfig(), now)
	if err := machine.TransitionAt(WakeDetected, now); err != nil {
		t.Fatal(err)
	}
	if err := machine.TransitionAt(AcknowledgementFinished, now); err != nil {
		t.Fatal(err)
	}
	if err := machine.TransitionAt(Timeout, now.Add(DefaultConfig().NoSpeechTimeout)); err != nil {
		t.Fatal(err)
	}
	if machine.State() != Idle {
		t.Fatalf("timeout state = %v, want idle", machine.State())
	}
}
