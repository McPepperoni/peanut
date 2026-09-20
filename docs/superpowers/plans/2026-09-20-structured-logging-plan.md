# Structured Console Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Add structured, stderr-only console logging for Peanut lifecycle, API, pipeline, model-load, reload, fallback, and error events without exposing user or secret data.

**Architecture:** Use Go stdlib log/slog. A small internal/logging package creates text loggers and normalizes nil loggers to a no-op logger. main owns the production logger; API, runtime, and pipeline receive explicit logger dependencies. Existing public/internal constructors remain usable without logging.

**Tech Stack:** Go 1.27, log/slog, net/http, existing Peanut test doubles and httptest.

## Global Constraints

- Use slog.NewTextHandler writing structured logs to stderr.
- Keep logs at event boundaries; never log audio/VAD/KWS frames.
- Never log request URLs/query values, remote addresses, headers, tokens, request/response bodies, transcripts, utterances, TTS text/audio, audio samples, embeddings, speaker samples, model paths, manifests, or checksums.
- Keep CLI command results on stdout; diagnostics go to stderr.
- Do not add dependencies, API schema changes, SQLite schema changes, audio contract changes, native Sherpa interface changes, or model-selection semantic changes.
- Optional speaker loading for transcribe remains advisory and falls back to STT-only behavior.
- Start each behavior change with a failing test.
- Run commands from app/.

---

### Task 1: Add logging foundation

**Files:**
- Create: app/internal/logging/logging.go
- Create: app/internal/logging/logging_test.go

**Interfaces:**
- Produces logging.New(io.Writer) *slog.Logger, logging.Nop() *slog.Logger, and logging.Normalize(*slog.Logger) *slog.Logger.
- Later tasks use logging.Normalize at optional injection boundaries.

- [ ] **Step 1: Write failing tests**

Add tests proving production output is structured and nil normalization is safe:

~~~go
func TestNewWritesStructuredText(t *testing.T) {
    var output bytes.Buffer
    logger := New(&output)
    logger.Info("process.start", "component", "runtime", "status", "started")

    got := output.String()
    for _, want := range []string{"msg=process.start", "component=runtime", "status=started"} {
        if !strings.Contains(got, want) {
            t.Fatalf("log output %q missing %q", got, want)
        }
    }
}

func TestNormalizeNilReturnsUsableLogger(t *testing.T) {
    logger := Normalize(nil)
    if logger == nil {
        t.Fatal("Normalize(nil) returned nil")
    }
    logger.Info("test")
}
~~~

- [ ] **Step 2: Run tests and verify failure**

Run:

~~~text
go test ./internal/logging -run 'Test(NewWritesStructuredText|NormalizeNilReturnsUsableLogger)$'
~~~

Expected: FAIL because internal/logging does not exist.

- [ ] **Step 3: Implement minimal logger helpers**

Implement:

~~~go
package logging

import (
    "io"
    "log/slog"
)

func New(output io.Writer) *slog.Logger {
    if output == nil {
        output = io.Discard
    }
    return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
}

func Nop() *slog.Logger {
    return New(io.Discard)
}

func Normalize(logger *slog.Logger) *slog.Logger {
    if logger == nil {
        return Nop()
    }
    return logger
}
~~~

- [ ] **Step 4: Run focused and package tests**

Run:

~~~text
go test ./internal/logging
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~text
git add app/internal/logging
git commit -m "feat: add structured logging helpers"
~~~

### Task 2: Instrument API request completion

**Files:**
- Modify: app/internal/api/server.go
- Modify: app/internal/api/server_test.go

**Interfaces:**
- Preserve NewServer(db, refresher, modelReloader) *Server.
- Add NewServerWithLogger(db, refresher, modelReloader, logger *slog.Logger) *Server.
- NewServer delegates to NewServerWithLogger(..., nil).
- Add private requestLogger(next http.Handler) http.Handler and a response writer wrapper that records first status and bytes.

- [ ] **Step 1: Write failing request-log test**

Construct the existing test server with NewServerWithLogger, issue GET /docs, and assert captured output contains method=GET, route=/docs, status=200, and bytes=. Assert it does not contain a known authorization token or request-body sentinel.

Use a bytes.Buffer and logging.New(&buffer). Do not assert timestamps or exact line spacing.

- [ ] **Step 2: Run focused test**

Run:

~~~text
go test ./internal/api -run TestServerLogsRequestCompletion
~~~

Expected: FAIL because logger injection and request middleware are absent.

- [ ] **Step 3: Add logger injection and middleware**

Extend Server with logger *slog.Logger. Make NewServer delegate to:

~~~go
func NewServerWithLogger(db *sqlite.DB, refresher Refresher, modelReloader ModelReloader, logger *slog.Logger) *Server
~~~

Normalize the logger, build the mux as today, wrap authentication with request logging, and keep authentication behavior unchanged:

~~~go
server.handler = server.requestLogger(server.authenticate(mux))
~~~

The wrapper must:

- default status to 200 if no explicit status is written;
- count bytes from both WriteHeader and Write;
- use r.Pattern as route field, falling back to the constant value `unmatched` only when no pattern exists;
- log method, route, status, bytes, and duration milliseconds;
- use error level only for status >= 500;
- never log URL, query, headers, body, or error response text.

- [ ] **Step 4: Run API tests**

Run:

~~~text
go test ./internal/api
~~~

Expected: PASS, including existing auth/config behavior.

- [ ] **Step 5: Commit**

~~~text
git add app/internal/api/server.go app/internal/api/server_test.go
git commit -m "feat: log structured API requests"
~~~

### Task 3: Wire process and runtime/model lifecycle logging

**Files:**
- Modify: app/cmd/peanut/commands.go
- Modify: app/cmd/peanut/main.go
- Modify: app/cmd/peanut/runtime.go
- Modify: app/cmd/peanut/main_test.go
- Modify: app/cmd/peanut/runtime_test.go

**Interfaces:**
- Add optional Logger *slog.Logger to commandDependencies; nil means no-op.
- Keep runMain test callers valid. Normalize dependencies.Logger at entry.
- Keep NewServer compatibility; production paths call api.NewServerWithLogger.
- Add logger-aware private helpers where existing tests need no-logger wrappers: runConfiguredWithLogger, newConfiguredRuntimeWithLogger, newCommandRuntimeWithLogger, and serveAPIWithLogger. Existing private helpers delegate to no-op logger variants.

- [ ] **Step 1: Write failing lifecycle/model tests**

Add tests using logging.New(&bytes.Buffer) and injected dependencies:

1. runMain with a logger records process.start and a database/migration completion event while preserving command dispatch.
2. A model-set builder with a test snapshot records role/status events without the configured model root appearing in output.
3. reloadableModels.Swap records reload success and records failure without replacing the previous model set.

Use sentinel model-root text and assert it is absent from the buffer.

- [ ] **Step 2: Run focused tests**

Run:

~~~text
go test ./cmd/peanut -run 'Test(RunMainLogs|ModelSetLogs|ReloadableModelsLogs)'
~~~

Expected: FAIL because logger fields and events are absent.

- [ ] **Step 3: Add process and runtime logging**

At main, create logging.New(os.Stderr) and pass it through commandDependencies.Logger. In runMain, log stable events around database open, migration/default persistence, command dispatch, and terminal errors. Do not include database paths or command arguments containing user text.

In runtime helpers:

- log model role load start/success/failure with component, role, stage, status, and duration_ms;
- log registry reload success/failure and preserve current swap semantics;
- log optional speaker fallback at warning level;
- log runtime start, server start, shutdown, and cleanup outcomes;
- pass logger into pipeline.Dependencies and api.NewServerWithLogger;
- keep returned errors and cleanup ordering unchanged.

Use time.Now() around role construction/reload and time.Since(start).Milliseconds(). Log stable failure metadata such as `status=failed` and `error_type=operation_failed`; never serialize raw errors, paths, or payload values.

- [ ] **Step 4: Run command-package tests**

Run:

~~~text
go test ./cmd/peanut
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~text
git add app/cmd/peanut
git commit -m "feat: log process and model lifecycle"
~~~

### Task 4: Instrument pipeline event boundaries

**Files:**
- Modify: app/internal/pipeline/coordinator.go
- Modify: app/internal/pipeline/coordinator_test.go

**Interfaces:**
- Add optional Logger *slog.Logger to pipeline.Dependencies.
- Normalize logger in NewCoordinator; no existing caller must provide one.

- [ ] **Step 1: Write failing event-boundary test**

Extend the existing happy-path coordinator test with a bytes.Buffer logger. Assert output contains event names for pipeline start, wake, speech start/end, processing, and playback completion. Assert output does not contain the known fake transcript turn it on or any audio sample value.

Add an error-path test with a failing capture or provider and assert one failure event contains stage and status=failed.

- [ ] **Step 2: Run focused tests**

Run:

~~~text
go test ./internal/pipeline -run 'TestCoordinator(Logs|RunsSeparateUtteranceHappyPath)'
~~~

Expected: FAIL because coordinator has no logger events.

- [ ] **Step 3: Add event-boundary logs**

Store normalized logger on Coordinator. Emit logs only at these existing boundaries:

- pipeline.start after capture starts;
- pipeline.timeout and pipeline.recovery for timeout/reset paths;
- pipeline.wake_detected;
- pipeline.speech_started and pipeline.speech_ended;
- pipeline.stage for processing, synthesis, and playback start/completion/failure;
- pipeline.stop on clean completion/cancellation;
- pipeline.error from recover.

Use stage/status/duration fields. Do not pass transcript text into log attributes. Do not log frame loops, recordings, VAD probabilities, keyword text, capability payloads, device IDs, provider arguments, response text, audio, or embeddings.

- [ ] **Step 4: Run pipeline tests**

Run:

~~~text
go test ./internal/pipeline
~~~

Expected: PASS with existing state-machine behavior unchanged.

- [ ] **Step 5: Commit**

~~~text
git add app/internal/pipeline/coordinator.go app/internal/pipeline/coordinator_test.go
git commit -m "feat: log pipeline boundaries"
~~~

### Task 5: Full verification and review handoff

**Files:**
- Modify only files already listed above if verification exposes a logging defect.

- [ ] **Step 1: Run full Go verification**

From app/, run:

~~~text
go test ./... -p 1 -timeout 120s
go vet ./...
go build -o ../build/peanut ./cmd/peanut
~~~

Expected: all commands exit 0.

- [ ] **Step 2: Run Linux ARM64 compile-only verification**

Run `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test ./... -run '^$' -exec 'cmd /c exit 0'`. Expected: exit 0 with no native execution.

- [ ] **Step 3: Run native Sherpa smoke test when fixtures exist**

Run:

~~~text
go test -tags sherpa ./internal/ml/sherpa -run TestNativeSmoke -v
~~~

If local native fixtures/toolchain are absent, record that limitation; do not change production code to manufacture fixtures.

- [ ] **Step 4: Inspect logging safety**

Search changed code for logging of URL, Path, Token, Authorization, request bodies, transcript fields, TTS text, audio, embeddings, or model manifests. Confirm no log call sits inside a frame-processing loop.

- [ ] **Step 5: Commit verification-only fixes if needed**

~~~text
git add app
git commit -m "fix: harden structured logging verification gaps"
~~~

Do not create this commit when no fix is required.
