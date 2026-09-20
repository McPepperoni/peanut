# Structured Console Logging Design

Status: Approved
Date: 2026-09-20

## Goal

Add structured console logging to Peanut's Go backend for lifecycle, API, pipeline, model-load, reload, fallback, and error events. Logs must remain portable to Linux ARM64 and CPU-only execution, preserve existing behavior, and never expose secrets or user content.

## Chosen approach

Use the Go standard library `log/slog` with a small `internal/logging` package.

- Production logger: `slog.NewTextHandler` writing to `stderr`.
- Component wiring: explicit logger injection into runtime, API, and pipeline boundaries.
- Compatibility: existing constructors remain usable; logger-aware construction is additive or uses an optional dependency field.
- Tests: in-memory `slog` handler/buffer assertions.

Rejected alternatives:

1. Global `slog.Default()` would reduce parameter plumbing but hide ownership and introduce global test state.
2. A third-party logger would add dependency and portability cost without a required capability.

## Architecture and data flow

`main` creates one process logger at startup. It passes that logger into API and runtime construction. Runtime passes it to the pipeline coordinator and uses it around model registry reload and model-set construction. The API attaches an outer request middleware that records completion metadata.

All logs are emitted at event boundaries. Audio-frame, VAD-frame, and wake-detector loops remain silent unless a meaningful state transition or failure occurs.

The logger package provides:

- production text logger construction with stable handler options;
- a no-op logger for tests and callers that do not opt into logging;
- shared event/attribute names where centralization improves consistency.

No mutable package-global logger is introduced.

## Event contract

Messages are stable event names. Attributes are low-cardinality operational metadata:

- Process: `process.start`, database open/migration, API/runtime start, graceful shutdown, terminal failure.
- Models: role-level load start/success/failure, reload success/failure, optional speaker fallback. Attributes: `component`, `role`, `stage`, `status`, `duration_ms`.
- API: one completion event per request. Attributes: `method`, matched route pattern, `status`, response bytes, `duration_ms`; failures use error level when status is 5xx or an internal operation fails.
- Pipeline: start/stop, wake detected, speech start/end, processing stages, playback completion, timeout/recovery, and failure stage. Attributes describe stage/status/duration only.

Logs must not contain request URLs or query values, remote addresses, headers, authorization or pairing tokens, request/response bodies, transcripts, parsed utterances, TTS text/audio, audio samples, embeddings, speaker samples, model paths, manifests, or checksums.

CLI command results remain on stdout. Structured diagnostics go to stderr.

## Error handling

Logging must not change control flow or mask the original error. Every fatal startup/runtime error is logged once at the boundary that returns it. Cleanup errors remain combined using existing error behavior; shutdown logging reports the outcome without replacing returned errors.

Optional speaker loading for `transcribe` remains advisory: a failed speaker construction logs a warning and preserves STT-only operation with `speaker=unknown` behavior. Full runtime and `enroll` keep their existing mandatory model requirements.

## Testing

Add focused tests for:

- logger output being structured and written to the configured writer;
- lifecycle/model events containing role/status/duration fields;
- API request logs containing method, route pattern, status, and bytes;
- pipeline events being emitted at meaningful boundaries without transcript/audio values;
- model-load failures and optional-speaker fallback;
- absence of authorization/config secrets, request bodies, model paths/checksums, transcripts, TTS text, embeddings, and audio payloads;
- no per-frame logging.

Run from `app/`:

```text
go test ./... -p 1 -timeout 120s
go vet ./...
go build ./...
```

Also run the existing Linux ARM64 compile-only check. Run native Sherpa smoke tests when local fixtures/toolchain are available.

## Non-goals

- No API schema changes.
- No SQLite schema or configuration-source changes.
- No audio contract changes.
- No native Sherpa interface changes.
- No model selection or reload semantic changes.
- No log file, rotation, remote shipping, or runtime log-level configuration in this change.
