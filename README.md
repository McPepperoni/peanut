# Peanut

Local-first, CPU-only voice runtime for Raspberry Pi 5 Linux ARM64 and desktop development.

All Go source lives under `app/`. Generated binaries belong in root `build/`; models and runtime data stay outside Git.

## Verify

```sh
cd app
go test ./...
go vet ./...
```

## Build

```sh
cd app
go build -o ../build/peanut ./cmd/peanut
```

## Home Assistant

For development, point Peanut at any reachable Home Assistant instance. An optional source checkout may live at ignored `dev/home-assistant-core/`.

For production, use an existing Home Assistant deployment or the official Home Assistant Container image. Production setup does not clone Home Assistant source.

Runtime configuration lives in SQLite. Environment-based runtime configuration is intentionally unsupported.
