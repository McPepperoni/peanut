# Runtime Hardening and Plug-and-Play Models Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden Peanut startup/API/runtime boundaries and make local sherpa model replacement work by dropping validated profiles into `models/`, while selecting the intent model through the flat external `Models.IntentModel` GGUF contract.

**Architecture:** Keep core Go packages platform-neutral. Add one filesystem model registry that discovers sherpa profiles, persists their metadata in SQLite, and atomically swaps validated sherpa roles. Resolve the intent model separately as the flat external GGUF named by `Models.IntentModel`. Extend the existing API and CLI; keep native audio/ML behind optional platform/build boundaries.

**Tech Stack:** Go standard library, existing SQLite driver, existing HTTP server, OpenAPI 3.1 JSON, locally served Scalar API Reference asset, Go build tags.

## Global Constraints

- Production target: Raspberry Pi 5 / Linux ARM64 / CPU only; no CUDA or x86-only assumptions.
- Runtime stays local-first and offline-capable; Peanut makes no cloud calls.
- SQLite is configuration source of truth; no runtime configuration from environment variables.
- Models and audio streams stay outside SQLite and Git.
- Intent model only emits structured intent plans; CPU-only and reasoning disabled; it never writes spoken responses.
- Core Go packages remain OS-neutral; unsupported native backends return runtime errors instead of breaking pure builds.
- `models/` is model root. GET `/api/v1/models` scans and reloads sherpa profiles; `Models.IntentModel` selects a separate flat `.gguf` file directly under that root.
- Invalid sherpa profile reloads preserve prior valid runtime for that role.
- LAN API binding requires pairing/authentication; secrets remain write-only and redacted.
- Every production behavior change starts with a failing test and ends with `cd app; go test ./...` passing.

---

## File map

- `app/internal/models/registry.go`: sherpa model profile schema, filesystem discovery, validation, and role selection.
- `app/internal/models/registry_test.go`: temporary-directory discovery and reload tests.
- `app/internal/storage/sqlite/migrations/001_initial.sql`: model metadata table.
- `app/internal/storage/sqlite/models.go`: typed model metadata store.
- `app/internal/intent/model.go`, `app/internal/intent/model_test.go`: generic intent-model parser contract and JSON behavior; rename current vendor-named files.
- `app/internal/api/server.go`: API routes, auth, model GET reload, OpenAPI, Scalar page.
- `app/internal/api/server_test.go`: API auth, reload, redaction, and docs tests.
- `app/internal/api/openapi.json`, `app/internal/api/docs.html`, `app/internal/api/scalar.js`: embedded local API documentation.
- `app/cmd/peanut/commands.go`, `app/cmd/peanut/main.go`: model CLI and runtime startup/shutdown.
- `app/internal/pipeline/coordinator.go`: cancellation and close behavior only where required by startup ownership.
- `app/internal/audio/capture/*`, `app/internal/audio/playback/*`: OS build-tag boundaries and file fallback.
- `README.md`, `docs/pi5-validation.md`, `.github/workflows/build.yml`: platform and deployment verification.

---

### Task 1: Generic intent model contract and filesystem model registry

**Files:**
- Create: `app/internal/models/registry.go`
- Create: `app/internal/models/registry_test.go`
- Create: `app/internal/storage/sqlite/models.go`
- Modify: `app/internal/storage/sqlite/migrations/001_initial.sql`
- Modify: `app/internal/config/config.go`
- Rename: `app/internal/intent/qwen.go` to `app/internal/intent/model.go`
- Modify: `app/internal/intent/prompt.go`
- Rename: `app/internal/intent/qwen_test.go` to `app/internal/intent/model_test.go`
- Modify: `app/cmd/peanut/runtime.go`

**Interfaces:**
- Produces `models.Profile`, `models.Role`, `models.Registry`, `(*Registry).Scan(context.Context) (models.Snapshot, error)`, and `(*Registry).Active(models.Role) (models.Profile, bool)` for sherpa profiles.
- `Profile` fields: `ID string`, `Role Role`, `Runtime string`, `Path string`, `Entry string`, `SHA256 string`, `Threads int`, `Valid bool`, `Error string`.
- `Role` values: `kws`, `vad`, `stt`, `speaker`, `tts`.
- `Registry` consumes `models.Root string`, SQLite model store, and a callback `func(context.Context, Snapshot) error` for runtime swap.
- The intent model is not a registry profile: validate `Models.IntentModel` as a flat regular `.gguf` filename contained directly under `Models.Root`.
- Generic intent parser name becomes `ModelParser`; constructor becomes `NewModelParser`; parser still implements `intent.IntentParser` and returns only validated `ActionPlan`.

- [ ] **Step 1: Write failing model discovery tests.**

```go
func TestRegistryScansManifestProfiles(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "stt/local", `{"id":"stt-local","role":"stt","runtime":"local","entry":"model.int8.onnx","sha256":""}`)
	registry := NewRegistry(root, fakeModelStore{}, nil)
	snapshot, err := registry.Scan(context.Background())
	if err != nil { t.Fatal(err) }
	if got := snapshot.Profiles[0].ID; got != "stt-local" { t.Fatalf("id = %q", got) }
}

func TestRegistryRejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "stt/bad", `{"id":"bad","role":"stt","runtime":"local","entry":"../../secret.onnx"}`)
	snapshot, err := NewRegistry(root, fakeModelStore{}, nil).Scan(context.Background())
	if err != nil { t.Fatal(err) }
	if snapshot.Profiles[0].Valid { t.Fatal("path traversal marked valid") }
}

func TestRegistryKeepsPriorRoleWhenReloadFails(t *testing.T) {
	registry := registryWithValidSTT(t)
	registry.swap = func(context.Context, Snapshot) error { return errors.New("load failed") }
	if _, err := registry.Scan(context.Background()); err == nil { t.Fatal("want swap error") }
	if _, ok := registry.Active(RoleSTT); !ok { t.Fatal("prior role was discarded") }
}
```

- [ ] **Step 2: Run focused tests and verify failure.**

Run: `Set-Location app; go test ./internal/models ./internal/storage/sqlite ./internal/intent`

Expected: FAIL because model registry/store and generic parser names do not exist.

- [ ] **Step 3: Add SQLite model metadata table and typed store.**

Add idempotent table creation to `001_initial.sql`:

```sql
CREATE TABLE IF NOT EXISTS models (
    id TEXT PRIMARY KEY,
    role TEXT NOT NULL,
    runtime TEXT NOT NULL,
    path TEXT NOT NULL,
    entry TEXT NOT NULL,
    sha256 TEXT NOT NULL DEFAULT '',
    threads INTEGER NOT NULL DEFAULT 1,
    valid INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    active INTEGER NOT NULL DEFAULT 0,
    refreshed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Implement `ModelStore.ReplaceSnapshot(ctx, []Profile) error` in one transaction and `ModelStore.List(ctx) ([]Profile, error)`. Store relative paths only; never store model bytes.

- [ ] **Step 4: Implement manifest discovery and validation.**

Scan only direct sherpa profile children matching `models/<role>/<profile>/model.json`. Decode with `DisallowUnknownFields`; require a valid sherpa role, non-empty ID/runtime/entry, and a regular entry path contained within the profile directory. If `sha256` is present, hash the entry file with `crypto/sha256` and require exact lowercase/uppercase-insensitive match. Return invalid profiles in the snapshot instead of aborting the whole scan. Sort profiles by ID. Do not scan the flat intent GGUF as a profile; resolve and validate `Models.IntentModel` separately.

Use this manifest shape:

```json
{
  "id": "stt-local",
  "role": "stt",
  "runtime": "local",
  "entry": "model.int8.onnx",
  "sha256": "",
  "threads": 4
}
```

- [ ] **Step 5: Generalize intent parser naming and error text.**

Rename `QwenParser` to `ModelParser`, `NewQwenParser` to `NewModelParser`, and test names/errors to generic intent-model wording. Rename vendor-named source/test files to `model.go` and `model_test.go`. Remove vendor-specific wording from `prompt.go`, `model.go`, and tests. Keep strict plan JSON schema, capability snapshot injection, CPU-only deterministic runner settings, timeout, and validation. Do not add conversational output.

- [ ] **Step 6: Connect runtime configuration to `models/`.**

Replace default role paths with `Models.Root = "models"`; preserve `Models.IntentModel`, `Threads`, and `CPUOnly`. In `runtime.go`, create a registry, scan sherpa profiles before constructing adapters, resolve the flat `Models.IntentModel` GGUF separately, and map active sherpa profiles to the existing `ml.Manifest`. Return an error containing the sherpa role and path when a required profile is missing or invalid, or the configured GGUF filename/path when the intent model is missing or invalid.

- [ ] **Step 7: Run tests and commit.**

Run: `Set-Location app; go test ./internal/models ./internal/storage/sqlite ./internal/intent ./cmd/peanut; go vet ./...`

Expected: PASS. Commit:

```text
feat: add generic model registry
```

---

### Task 2: Configuration API, model reload, OpenAPI, and Scalar

**Files:**
- Modify: `app/internal/api/server.go`
- Modify: `app/internal/api/server_test.go`
- Create: `app/internal/api/openapi.json`
- Create: `app/internal/api/docs.html`
- Create: `app/internal/api/scalar.js`
- Modify: `app/internal/models/registry.go`

**Interfaces:**
- `api.NewServer(db *sqlite.DB, refresher Refresher, modelReloader ModelReloader) *Server`.
- `ModelReloader` exposes `Reload(context.Context) (models.Snapshot, error)`.
- GET `/api/v1/models` returns `{profiles, active, errors}` and invokes `Reload` before encoding.
- GET `/api/v1/openapi.json` returns embedded OpenAPI JSON.
- GET `/docs` returns embedded Scalar HTML.

- [ ] **Step 1: Write failing API tests.**

```go
func TestModelsGETReloadsAndReturnsSnapshot(t *testing.T) {
	reloader := &fakeModelReloader{snapshot: models.Snapshot{Profiles: []models.Profile{{ID:"stt-local", Role:models.RoleSTT, Valid:true}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	rec := httptest.NewRecorder()
	NewServer(db, nil, reloader).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("status = %d", rec.Code) }
	if reloader.calls != 1 { t.Fatalf("reload calls = %d", reloader.calls) }
}

func TestModelsGETKeepsPreviousSnapshotOnReloadError(t *testing.T) {
	reloader := &fakeModelReloader{err: errors.New("bad model")}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	rec := httptest.NewRecorder()
	NewServer(db, nil, reloader).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway { t.Fatalf("status = %d", rec.Code) }
	if _, ok := reloader.Active(models.RoleSTT); !ok { t.Fatal("active role was erased") }
}

func TestDocsEndpointsAreLocalAndJSON(t *testing.T) {
	for _, path := range []string{"/api/v1/openapi.json", "/docs"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		NewServer(db, nil, nil).Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK { t.Fatalf("%s status = %d", path, rec.Code) }
	}
}
```

- [ ] **Step 2: Run focused tests and verify failure.**

Run: `Set-Location app; go test ./internal/api`

Expected: FAIL because model routes and docs endpoints are absent.

- [ ] **Step 3: Implement per-request auth boundary.**

Read current SQLite config inside middleware for every request. Require `Authorization: Bearer <pairing token>` only when `AllowLAN` is true. Use `crypto/subtle.ConstantTimeCompare`. Keep localhost default unauthenticated. Do not expose token values. Return JSON `401` for missing/invalid credentials and `500` when configuration cannot load.

- [ ] **Step 4: Add model reload route.**

Add `ModelReloader` to `Server`, register `/api/v1/models`, call `Reload` only for GET, and encode valid/invalid profiles. On reload error return `502` while leaving the reloader's previous active snapshot unchanged. Reject other methods with `Allow: GET`.

- [ ] **Step 5: Add OpenAPI and local Scalar assets.**

Define OpenAPI 3.1 paths for existing config/pairing/refresh routes plus `/api/v1/models`, `/api/v1/openapi.json`, and `/docs`. Embed JSON and HTML with `go:embed`. Serve HTML that points Scalar at `/api/v1/openapi.json`; package the resolved Scalar browser bundle as `scalar.js` under the repository so normal docs use no CDN. Test that HTML contains `/api/v1/openapi.json` and no `http://` or `https://` script source.

- [ ] **Step 6: Run tests and commit.**

Run: `Set-Location app; go test ./internal/api ./internal/models; go vet ./...`

Expected: PASS. Commit:

```text
feat: expose model reload and local api docs
```

---

### Task 3: Cross-platform startup, CLI, and shutdown boundaries

**Files:**
- Modify: `app/cmd/peanut/main.go`
- Modify: `app/cmd/peanut/commands.go`
- Modify: `app/cmd/peanut/commands_test.go`
- Modify: `app/cmd/peanut/runtime.go`
- Modify: `app/internal/audio/capture/capture.go`
- Modify: `app/internal/audio/playback/playback.go`
- Create: `app/internal/audio/capture/capture_windows.go`
- Create: `app/internal/audio/capture/capture_unix.go`
- Create: `app/internal/audio/playback/playback_windows.go`
- Create: `app/internal/audio/playback/playback_unix.go`
- Create: `app/internal/audio/capture/capture_platform_test.go`
- Create: `app/internal/audio/playback/playback_platform_test.go`

**Interfaces:**
- `runConfigured(context.Context, config.Config, *sqlite.DB) error` validates all required roles before opening native audio.
- `dispatch` adds `model list` and `model verify`; both use configured `models/` root and print stable tabular/text output.
- `SystemCapture` and `SystemPlayer` preserve `audio.Capture`/`audio.Player`; unsupported platforms return `ErrUnsupported` at runtime.

- [ ] **Step 1: Write failing startup/CLI tests.**

```go
func TestModelCommandsUseModelsRoot(t *testing.T) {
	root := t.TempDir()
	writeValidSTTManifest(t, root)
	var output bytes.Buffer
	deps := commandDependencies{ModelRoot: root}
	if err := dispatch(context.Background(), []string{"model", "list"}, deps, &output); err != nil { t.Fatal(err) }
	if !strings.Contains(output.String(), "stt-local") { t.Fatalf("output = %q", output.String()) }
}

func TestRunFailsBeforeAudioWhenRequiredModelMissing(t *testing.T) {
	err := runConfigured(context.Background(), config.Config{Models: config.Models{Root: t.TempDir(), Threads: 1, CPUOnly: true}}, nil)
	if err == nil || !strings.Contains(err.Error(), "role") { t.Fatalf("error = %v", err) }
}
```

- [ ] **Step 2: Run focused tests and verify failure.**

Run: `Set-Location app; go test ./cmd/peanut ./internal/audio/capture ./internal/audio/playback`

Expected: FAIL because model command dependencies and platform files do not exist.

- [ ] **Step 3: Add model CLI commands.**

Extend `commandDependencies` with `ModelRoot string`. Handle `model list` by scanning sherpa profiles and printing `id role runtime valid path`; handle `model verify` by scanning and printing invalid profile errors, returning nonzero error when any profile is invalid. Keep flat intent-GGUF validation in runtime startup; keep no CLI framework and no environment lookup.

- [ ] **Step 4: Make startup validate before native construction.**

Have `runConfigured` scan the model registry, require active valid profiles for `kws`, `vad`, `stt`, `speaker`, and `tts`, resolve `Models.IntentModel` as the flat external GGUF, then construct adapters. Wrap sherpa failures as `load <role> model: ...` and intent-GGUF failures as `load intent model: ...`. Do not start capture or coordinator when validation fails.

- [ ] **Step 5: Add platform build boundaries.**

Keep pure file adapters in common files. Move OS-specific system capture/player implementations into build-tagged files. Linux, Windows, and macOS builds must compile common code; unavailable native implementations return `ErrUnsupported` with platform name. Do not add a cross-platform audio framework in this task.

- [ ] **Step 6: Close runtime cleanly.**

Create one child context in `runConfigured`, defer cancellation, and ensure coordinator return closes capture/player resources when they implement `io.Closer`. Preserve the existing coordinator event-loop behavior and propagate close errors only when no earlier runtime error exists.

- [ ] **Step 7: Run tests and compile matrix.**

Run:

```text
Set-Location app
go test ./...
go vet ./...
$env:GOOS='windows'; $env:GOARCH='amd64'; go test ./... -run '^$'
$env:GOOS='darwin'; $env:GOARCH='amd64'; go test ./... -run '^$'
$env:GOOS='linux'; $env:GOARCH='arm64'; go test ./... -run '^$'
```

Expected: all tests pass; cross-platform compile commands exit zero. Commit:

```text
feat: harden portable runtime startup
```

---

### Task 4: Pi validation docs and build verification

**Files:**
- Create: `docs/pi5-validation.md`
- Modify: `README.md`
- Create: `.github/workflows/build.yml`
- Modify: `tools/models.md`
- Modify: `.gitignore`
- Modify: `.superpowers/sdd/progress.md`

**Interfaces:** Documentation and CI consume the commands and model manifest contract from Tasks 1–3. No runtime behavior changes.

- [ ] **Step 1: Write documentation checks.**

Add a small shell/PowerShell verification section to README and test it manually with:

```text
Set-Location app
go test ./...
go vet ./...
go build -o ../build/peanut ./cmd/peanut
```

Expected: commands are copyable and output path is `build/peanut` or `build/peanut.exe`.

- [ ] **Step 2: Document model installation and verification.**

Document the five sherpa roles in `models/<role>/<profile>/model.json`, each manifest field, and the separate flat `Models.IntentModel` GGUF directly under `Models.Root`. Cover CPU-only operation, checksum creation for profile entries, `peanut model verify`, and GET `/api/v1/models` reload. State that model files are ignored by Git and never written to SQLite.

- [ ] **Step 3: Document platform matrix and Pi checks.**

Document pure builds on Linux/Windows/macOS, Linux ARM64 cross-build, native runtime prerequisites, audio backend prerequisites, startup failure messages, real-time factor, memory, thermal, and long-run checks. Distinguish optional native smoke tests from mandatory pure tests.

- [ ] **Step 4: Add CI build matrix.**

Run pure `go test ./...` and `go vet ./...` on Linux, Windows, and macOS runners. Add Linux ARM64 compile-only job. Never fetch model bytes or cloud services in CI.

- [ ] **Step 5: Verify final state and commit.**

Run:

```text
Set-Location app
go test ./...
go vet ./...
go build -o ../build/peanut ./cmd/peanut
```

Run: `git diff --check; git status --short`

Expected: tests/vet/build pass; only intended source/docs changes remain; user-owned `models/` stays untracked and untouched. Commit:

```text
docs: add portable deployment validation
```

## Plan self-review

- Spec coverage: startup validation and shutdown Task 3; API auth and Scalar Task 2; model discovery/reload Task 1–2; platform boundaries Task 3; Pi/build docs Task 4; failure preservation and tests are explicit in Tasks 1–3.
- Placeholder scan: no placeholder markers or undefined future step. “Later slice” appears only as scope boundary for native implementations excluded by the approved spec.
- Type consistency: `models.Snapshot`, `models.Profile`, `models.Role`, `Registry.Scan`, `Registry.Active`, `ModelReloader.Reload`, and `ModelParser` are defined before use by later tasks.
- Scope: four tasks are independent at review boundaries and each ends with tests plus a commit.
