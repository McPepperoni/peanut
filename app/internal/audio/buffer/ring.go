package buffer

import (
	"sync"

	"peanut/internal/audio"
)

type Ring struct {
	mu     sync.Mutex
	frames []audio.Frame
	limit  int
}

func New(limit int) *Ring {
	if limit < 1 {
		panic("ring capacity must be positive")
	}
	return &Ring{limit: limit}
}

func (r *Ring) Push(frame audio.Frame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyFrame := audio.Frame{Samples: append([]float32(nil), frame.Samples...)}
	if len(r.frames) == r.limit {
		copy(r.frames, r.frames[1:])
		r.frames[len(r.frames)-1] = copyFrame
		return
	}
	r.frames = append(r.frames, copyFrame)
}

func (r *Ring) Snapshot() []audio.Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audio.Frame, len(r.frames))
	for i, frame := range r.frames {
		out[i] = audio.Frame{Samples: append([]float32(nil), frame.Samples...)}
	}
	return out
}
