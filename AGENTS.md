# Peanut contributor guide

Peanut is a local-first, CPU-only voice runtime targeting Raspberry Pi 5 Linux ARM64 and desktop development.

## Layout

- `app/`: all Go source and the Go module.
- `build/`: generated binaries; never commit.
- `assets/`: user-provided source assets; do not delete or commit unless explicitly requested.
- `local-model/`: user-provided model files; never commit.
- `dev/home-assistant-core/`: optional development-only Home Assistant checkout; never commit.
- `docs/`: architecture and implementation plans.

## Constraints

- SQLite is the runtime configuration source of truth. Do not read runtime configuration from environment variables.
- Keep audio streams, recordings, model files, and generated runtime data outside SQLite and Git.
- Normalize audio to 16 kHz mono float32 in 20 ms frames of 320 samples.
- Keep hardware, ML runtimes, Home Assistant, storage, and TTS behind narrow package boundaries.
- Keep production code portable to Linux ARM64 and CPU-only execution.
- Start every behavior change with a failing test.

## Commands

Run from `app/`:

```sh
go test ./...
go vet ./...
```

Build output belongs at repository root:

```sh
go build -o ../build/peanut ./cmd/peanut
```

## Home Assistant

Development may use any reachable Home Assistant instance. A local source checkout belongs only at `dev/home-assistant-core/`.

Production must use an existing Home Assistant instance or the official Home Assistant Container image. Never clone or run Home Assistant source as production setup.
