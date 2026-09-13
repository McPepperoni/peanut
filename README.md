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

The production artifact is the native CPU-only binary at root `build/peanut`. Qwen intent parsing runs with thinking disabled; Peanut does not generate conversational responses.

## Production installer

With Peanut's local configuration API running, run `app/internal/install/install.sh` on Linux or `app/internal/install/install.ps1` on Windows. The installer builds native Peanut, then asks whether Home Assistant already exists. It accepts that instance's URL/token or pulls and runs only the official Home Assistant Container image with Docker or Podman. Complete Home Assistant onboarding and create a long-lived token when prompted.

Peanut is not containerized. The installer sends Home Assistant credentials to the local Peanut configuration API, which validates and stores them in SQLite; runtime environment variables are not configuration.

## Home Assistant

For development, point Peanut at any reachable Home Assistant instance. An optional source checkout may live at ignored `dev/home-assistant-core/`.

For production, use an existing Home Assistant deployment or the official Home Assistant Container image. Production setup does not clone Home Assistant source.

Runtime configuration lives in SQLite. Environment-based runtime configuration is intentionally unsupported.

See [`app/internal/install/dev-ha.md`](app/internal/install/dev-ha.md) for the development/production distinction.
