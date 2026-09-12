# Task 2: Normalized Audio and State Machine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add normalized 16 kHz mono float32 audio primitives, a 15-frame pre-roll ring, WAV boundary conversion, and a centralized interaction state machine.

**Architecture:** Audio values validate the fixed 320-sample frame contract at construction boundaries. The ring owns copies and snapshots copies. Pipeline transitions are pure reducer logic, with `Machine` as the only mutable state owner and timeout events recovering to safe states.

**Tech Stack:** Go standard library, `encoding/binary`, `encoding/bufio`, `sync`, `time`, and `testing`.

## Global Constraints

- Normalized audio is 16 kHz, mono, float32, 20 ms frames of 320 samples.
- Pre-roll is exactly 15 frames / 4800 samples.
- Audio abstractions remain hardware-neutral.
- No goroutine mutates pipeline state directly; transitions have one owner.
- Runtime configuration remains SQLite-owned; this task adds no environment configuration.

### Task 1: Audio contracts, WAV conversion, ring buffer, and pipeline reducer

**Files:**
- Create: `app/internal/audio/audio.go`, `app/internal/audio/wav.go`, `app/internal/audio/audio_test.go`, `app/internal/audio/wav_test.go`
- Create: `app/internal/audio/buffer/ring.go`, `app/internal/audio/buffer/ring_test.go`
- Create: `app/internal/pipeline/config.go`, `app/internal/pipeline/state.go`, `app/internal/pipeline/state_test.go`
- Create: `.superpowers/sdd/task-2-report.md`

**Interfaces:** `audio.NewFrame`, `audio.NewAudio`, `audio.Capture`, `audio.Player`; `buffer.Ring.Push`, `buffer.Ring.Snapshot`; `pipeline.Machine.Transition`.

- [ ] Write failing tests for fixed frames, WAV PCM16/float32 conversion, 15-frame/4800-sample copy-safe pre-roll, valid and invalid transitions, and timeout recovery.
- [ ] Run focused tests and confirm failures are missing symbols/behavior.
- [ ] Implement the smallest standard-library solution.
- [ ] Run focused tests, full `cd app; go test ./...`, and `go vet ./...`.
- [ ] Record exact RED/GREEN evidence and concerns in `.superpowers/sdd/task-2-report.md`.
- [ ] Stage only Task 2 files and commit `feat: add audio core and state machine`.
