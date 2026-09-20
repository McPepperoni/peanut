# Runtime Hardening and Plug-and-Play Models

## Goal

Make Peanut runnable from the same source on supported desktop platforms and Linux ARM64, while hardening startup/API boundaries and allowing model replacement through the `models/` directory.

## Scope

This slice covers:

- startup validation and clean shutdown;
- configuration API authentication boundaries;
- local Scalar API documentation;
- cross-platform build boundaries;
- model discovery and reload from root `models/`;
- model configuration through the CLI and API;
- ARM64 build and deployment documentation.

Native sherpa component implementations and OS audio backends remain a later slice. This work preserves their existing interfaces and explicit unavailable errors.

## Architecture

```text
CLI/API
  -> SQLite configuration
  -> startup validator
  -> runtime assembly
  -> coordinator
```

SQLite remains the configuration source of truth. Model bytes stay on disk. The provider registry remains the extension point for future plugins; no new plugin framework is added.

Core Go packages remain OS-neutral. File/stdin audio adapters compile everywhere. Native capture/playback adapters are selected behind build tags for Linux, Windows, and macOS. Unsupported native backends return clear runtime errors instead of breaking pure builds. Native ML runtimes stay optional behind native build tags. CPU-only settings remain shared across platforms.

## Model contract

Sherpa model bundles live under the repository/runtime root `models/` directory. SQLite stores selected profile metadata, never model bytes. The intent model is separate: `Models.IntentModel` selects one external `.gguf` filename directly under `Models.Root`.

Each discovered sherpa model profile contains:

- stable ID;
- role: `kws`, `vad`, `stt`, `speaker`, or `tts`;
- local path;
- format/runtime;
- checksum;
- CPU thread count;
- enabled state.

One profile is active per sherpa role. Small local manifests map bundle files to roles; absolute paths are not hardcoded into runtime code. The flat intent GGUF has no profile or role manifest and is validated as a regular `.gguf` file contained directly in `Models.Root`.

`GET /api/v1/models` is intentionally a read-and-reload operation for sherpa profiles. It scans `models/`, discovers supported profiles, validates manifests/checksums, reconciles SQLite metadata, attempts a complete runtime swap for valid sherpa roles, and returns active, available, and invalid profiles. The flat intent GGUF is resolved separately from `Models.IntentModel`. A failed role keeps its previous valid runtime. A reload lock ensures each request observes either the old or new complete runtime.

The CLI exposes the same sherpa-profile behavior with `peanut model list` and `peanut model verify`. Explicit model upload/download and delete commands are out of scope. Users place sherpa bundles and the external intent GGUF in `models/`.

## API and Scalar

Peanut serves `/api/v1/openapi.json` and a local `/docs` Scalar API Reference page. The page is locally served and has no CDN dependency during normal operation. OpenAPI describes configuration reads/writes, secret redaction, model discovery/reload, provider refresh, authentication, status, and error responses.

The API remains localhost-only by default. LAN binding requires pairing/authentication. Secrets are write-only and redacted in responses and logs. The model GET reload follows the same auth policy.

## Runtime and failure behavior

```text
models scan -> manifest validation -> SQLite metadata -> atomic runtime swap
audio -> detector -> transcript -> structured intent model JSON -> Go validation
     -> provider action -> static result/TTS -> playback
```

- Missing required sherpa profile: startup fails with role and exact path.
- Missing or invalid `Models.IntentModel`: startup fails with the configured GGUF filename/path.
- Invalid sherpa replacement: role is reported invalid; prior valid role remains active.
- Malformed intent JSON or unknown capability: reject without provider execution.
- Home Assistant failure: return classified provider error; keep runtime alive.
- Unsupported audio backend: return platform-specific prerequisite error.
- Shutdown cancels the coordinator, stops capture/playback, and closes SQLite.

## Testing and verification

- Pure Go tests run on desktop platforms.
- Temporary-directory tests cover sherpa profile discovery, checksum validation, flat intent-GGUF path validation, reload, and preservation of a prior valid sherpa role.
- API tests cover GET-triggered reload, auth boundaries, redaction, OpenAPI response, and failed reload behavior.
- Runtime swap concurrency test covers old/new complete-runtime visibility.
- Existing coordinator fake-component tests remain the behavior gate.
- Native tests skip only when native libraries or model files are absent.
- CI/build documentation covers Linux, Windows, macOS pure builds and Linux ARM64 cross-build.

## Explicit non-goals

- Automatic model downloads.
- Environment-variable runtime configuration.
- Cloud model inference.
- New plugin framework.
- Native sherpa KWS/VAD/speaker/TTS implementation in this slice.
- Complete OS audio implementation in this slice.
