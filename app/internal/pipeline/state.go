package pipeline

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type State uint8

const (
	Idle State = iota
	Wake
	Ack
	Listen
	Record
	Process
	Speak
)

type Event uint8

const (
	WakeDetected Event = iota
	AcknowledgementFinished
	SpeechStarted
	SpeechEnded
	ProcessingFinished
	PlaybackFinished
	Timeout
)

type Machine struct {
	mu        sync.Mutex
	state     State
	enteredAt time.Time
	cfg       Config
}

func NewMachine(cfg Config) *Machine { return NewMachineAt(cfg, time.Now()) }

func NewMachineAt(cfg Config, now time.Time) *Machine { return &Machine{cfg: cfg, enteredAt: now} }

func (m *Machine) State() State { m.mu.Lock(); defer m.mu.Unlock(); return m.state }

func (m *Machine) Transition(event Event) error { return m.TransitionAt(event, time.Now()) }

func (m *Machine) TransitionAt(event Event, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if event == Timeout {
		duration := timeoutFor(m.state, m.cfg)
		if duration == 0 {
			return fmt.Errorf("invalid timeout from %v", m.state)
		}
		if now.Sub(m.enteredAt) < duration {
			return errors.New("state timeout not reached")
		}
		m.state, m.enteredAt = timeoutState(m.state), now
		return nil
	}
	next, err := reduce(m.state, event)
	if err != nil {
		return err
	}
	if next != m.state {
		m.state, m.enteredAt = next, now
	}
	return nil
}

func reduce(state State, event Event) (State, error) {
	transitions := map[State]map[Event]State{
		Idle: {WakeDetected: Wake}, Wake: {AcknowledgementFinished: Listen}, Listen: {SpeechStarted: Record},
		Record: {SpeechEnded: Process}, Process: {ProcessingFinished: Speak}, Speak: {PlaybackFinished: Idle},
	}
	if next, ok := transitions[state][event]; ok {
		return next, nil
	}
	return state, fmt.Errorf("invalid transition from %v on %v", state, event)
}

func timeoutFor(state State, cfg Config) time.Duration {
	switch state {
	case Wake:
		return cfg.AcknowledgementTimeout
	case Listen:
		return cfg.NoSpeechTimeout
	case Record:
		return cfg.MaximumCommandDuration
	case Process:
		return cfg.ProcessingTimeout
	case Speak:
		return cfg.PlaybackTimeout
	default:
		return 0
	}
}

func timeoutState(state State) State {
	if state == Record {
		return Process
	}
	return Idle
}
