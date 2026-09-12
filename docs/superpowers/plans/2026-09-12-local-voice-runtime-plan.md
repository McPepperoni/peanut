# Local Voice Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Peanut's CPU-only local voice runtime with sherpa-onnx speech components, Qwen3 intent planning, SQLite-backed generalized capabilities, local/remote Home Assistant music control, configuration API, and a dev-only Home Assistant checkout path.

**Architecture:** Keep one Go coordinator as the only state owner. Hardware, sherpa-onnx, llama.cpp, Home Assistant, storage, and TTS stay behind narrow interfaces. Capabilities are provider-defined data; Qwen chooses only validated capability IDs and Go routes execution.

**Tech Stack:** Go, SQLite, CGO-backed sherpa-onnx C API, llama.cpp/ GGUF, standard-library HTTP/CLI/audio plumbing, Home Assistant REST API, WAV fixtures.

## Global Constraints

- Production target: Raspberry Pi 5 / Linux ARM64 / CPU only; no CUDA or x86-only assumptions.
- Runtime stays local-first and offline-capable; Peanut makes no cloud calls.
- Wake and command are separate utterances; KWS is inactive after wake until interaction completion.
- Normalized audio is 16 kHz, mono, float32, 20 ms frames of 320 samples.
- VAD starts only after ACK playback plus configurable acoustic tail; pre-roll is 300 ms.
- Speaker-ID failure never blocks STT; playback disables detector triggers and re-arms after configurable tail.
- SQLite is configuration source of truth; no runtime configuration from environment variables.
- Models and audio streams stay outside SQLite and Git.
- Qwen3 only emits structured intent plans; CPU-only and thinking disabled; it never writes spoken responses.
- Home Assistant is external: dev may use any reachable instance or clone/run source locally; production installer uses official container only.
- LAN configuration API binding is opt-in and requires pairing/authentication; secrets are write-only and redacted.
- Every production behavior change starts with a failing test and ends with `go test ./...` passing.

---

## File map

- `app/cmd/peanut/main.go`: minimal CLI and dependency assembly.
- `app/internal/config`: defaults, SQLite-loaded settings, validation.
- `app/internal/storage/sqlite`: migrations and typed stores.
- `app/internal/audio`: normalized frames, WAV I/O, ring buffer, capture/player interfaces and adapters.
- `app/internal/pipeline`: states, coordinator, timeouts, detector gating, metrics.
- `app/internal/ml`: model manifest and sherpa-backed KWS/VAD/STT/speaker/TTS adapters.
- `app/internal/intent`: capability types, JSON plan schema/validation, Qwen/llama.cpp parser.
- `app/internal/providers`: provider registry and Home Assistant provider.
- `app/internal/api`: versioned configuration HTTP API.
- `app/internal/install`: production installer and dev HA runner documentation/scripts.
- `dev/home-assistant-core`: Git-ignored development-only Home Assistant source checkout.
- `build/peanut`: Git-ignored Peanut build output.
- `assets/ack.wav`: embedded acknowledgement sound.
- `samples`: small test WAV fixtures only.
- `AGENTS.md`, `CLAUDE.md`, `README.md`: repository and developer guidance.

### Task 1: Application foundation, config, SQLite

**Files:** Create `app/go.mod`, `AGENTS.md`, `CLAUDE.md` symlink, `.gitignore`, `README.md`, `app/cmd/peanut/main.go`, `app/internal/config/config.go`, `app/internal/storage/sqlite/db.go`, `app/internal/storage/sqlite/migrations.go`, `app/internal/storage/sqlite/db_test.go`; ignore `dev/home-assistant-core/`, `build/`, `local-model/`, and generated runtime data.

**Interfaces:** `config.Load(path string) (config.Config, error)`, `sqlite.Open(ctx context.Context, path string) (*DB, error)`, `sqlite.Migrate(ctx context.Context) error`.

- [ ] Write failing tests for default config validation, migration-created tables, and reopen persistence.
- [ ] Run `cd app; go test ./internal/config ./internal/storage/sqlite`; expect undefined package/function failures.
- [ ] Implement minimal config structs with all audio/model/HA/API values centralized; use a CGO-compatible SQLite driver; embed one migration creating `settings`, `speakers`, `providers`, `devices`, `capabilities`, and schema metadata.
- [ ] Create canonical under-200-line `AGENTS.md`; create `CLAUDE.md` symlink and verify it resolves; document dev HA checkout separately from production container setup.
- [ ] Add `go test ./...` and `go vet ./...` commands to README; keep model files and `dev/home-assistant-core/` ignored.
- [ ] Run focused tests, then `cd app; go test ./...`; commit `feat: add application foundation`.

### Task 2: Normalized audio, ring buffer, state machine

**Files:** Create `app/internal/audio/audio.go`, `app/internal/audio/wav.go`, `app/internal/audio/buffer/ring.go`, `app/internal/pipeline/state.go`, `app/internal/pipeline/config.go`, tests beside each package.

**Interfaces:** `audio.Frame`, `audio.Audio`, `audio.Capture`, `audio.Player`; `buffer.Ring.Push(audio.Frame)`, `buffer.Ring.Snapshot()`; `pipeline.State` and transition reducer.

- [ ] Write failing tests for 320-sample frames, 15-frame/4800-sample pre-roll, wake/ack/listen/record/process/speak transitions, invalid transitions, and timeout recovery.
- [ ] Run focused tests; confirm failures target missing behavior.
- [ ] Implement copy-safe frame buffering, configurable thresholds/timeouts, explicit states, and one transition owner; no goroutine mutates state directly.
- [ ] Add WAV reader/writer for 16 kHz mono float32 conversion at boundary only.
- [ ] Run focused tests and `cd app; go test ./...`; commit `feat: add audio core and state machine`.

### Task 3: Generalized capabilities and intent validation

**Files:** Create `app/internal/intent/types.go`, `app/internal/intent/schema.go`, `app/internal/intent/validate.go`, `app/internal/providers/provider.go`, tests.

**Interfaces:** `CapabilityProvider`, `Capability`, `ActionDefinition`, `ActionRequest`, `ActionPlan`, `ValidatePlan(plan, snapshot) error`, provider registry.

- [ ] Write failing tests for valid single-step music plans, ordered multi-step plans, unknown device/action rejection, bad argument types, `clarify`, `unknown`, and multilingual language fields.
- [ ] Run focused tests; confirm missing validator failures.
- [ ] Implement strict JSON decoding with unknown-field rejection, stable IDs, JSON-compatible argument schemas, bounded step count, and capability-only validation.
- [ ] Implement provider registration and snapshot assembly without domain-specific core logic.
- [ ] Run tests and `cd app; go test ./...`; commit `feat: add capability intent contract`.

### Task 4: Qwen3 CPU intent parser

**Files:** Create `app/internal/intent/qwen.go`, `app/internal/intent/prompt.go`, `app/internal/intent/qwen_test.go`, `tools/llama.cpp.md`.

**Interfaces:** `IntentParser.Parse(ctx context.Context, transcript string, snapshot CapabilitySnapshot) (ActionPlan, error)`; `QwenParser` configured with GGUF path and llama.cpp executable/runtime.

- [ ] Write failing tests using a fake inference runner for prompt capability inclusion, no-thinking flags, JSON extraction, malformed output, timeout, and no invented capability acceptance.
- [ ] Run focused tests; confirm parser is absent.
- [ ] Implement a small llama.cpp runner boundary. Pass Qwen `enable_thinking=false`/reasoning disabled, CPU-only settings, deterministic generation, and JSON grammar/schema constraints. Parse only the defined plan envelope; never expose model text as spoken response.
- [ ] Document model path, llama.cpp build, ARM64 CPU flags, and a real local smoke command; do not commit llama.cpp source into Peanut.
- [ ] Run tests and `cd app; go test ./...`; commit `feat: add qwen intent parser`.

### Task 5: Home Assistant provider and configuration API

**Files:** Create `app/internal/providers/homeassistant.go`, `app/internal/providers/homeassistant_test.go`, `app/internal/api/server.go`, `app/internal/api/server_test.go`, `app/internal/storage/sqlite/capabilities.go`, `docs/home-assistant.md`.

**Interfaces:** `HomeAssistantProvider.Discover`, `HomeAssistantProvider.Execute`; versioned `/api/v1/config` handlers; typed SQLite capability/config stores.

- [ ] Write failing fake-HTTP-server tests for HA auth, media-player discovery, `music.play` mapping to `media_player.play_media`, unavailable HA, secret redaction, localhost default, LAN auth, transactional writes, and rediscovery.
- [ ] Run focused tests; confirm provider/API behavior is missing.
- [ ] Implement standard-library HTTP client with context/timeouts, local URL default, bearer token from SQLite config, capability cache, strict action payload mapping, and provider error classification.
- [ ] Implement localhost-only API default, opt-in LAN binding, generated pairing token storage, write-only HA secret fields, redacted responses, and refresh endpoint.
- [ ] Add docs for pointing dev Peanut at remote/local HA and token creation; never include real credentials.
- [ ] Run tests and `cd app; go test ./...`; commit `feat: add home assistant provider and config api`.

### Task 6: sherpa-onnx model manifest and adapters

**Files:** Create `app/internal/ml/manifest.go`, interfaces under `app/internal/ml/{kws,vad,stt,speaker,tts}`, CGO adapters under `app/internal/ml/sherpa`, tests, `tools/models.md`.

**Interfaces:** `WakeDetector`, `VAD`, `Transcriber`, `SpeakerIdentifier`, `Synthesizer`; each receives normalized PCM and returns typed results.

- [ ] Write failing contract tests with deterministic fake adapters for wake gating, VAD endpoint values, STT empty/error results, advisory speaker failure, and TTS audio validation.
- [ ] Run focused tests; confirm adapters are missing.
- [ ] Implement model manifest validation and sherpa-onnx C API boundary using CPU provider and configurable threads. Pin these official bundles in docs/scripts, not Git: `sherpa-onnx-kws-zipformer-wenetspeech-3.3M-2024-01-01-mobile`, `silero_vad.onnx`, `sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17`, `3dspeaker_speech_eres2net_base_sv_zh-cn_3dspeaker_16k.onnx`, and `vits-piper-en_GB-cori-medium`.
- [ ] Keep native loading behind one package; return clear missing/invalid-model errors; provide fake adapters for desktop/unit tests.
- [ ] Run contract tests; run native smoke tests only when model/runtime paths exist; commit `feat: add sherpa runtime adapters`.

### Task 7: Capture, playback, enrollment, and file commands

**Files:** Create `app/internal/audio/capture`, `app/internal/audio/playback`, `app/internal/speaker`, `app/cmd/peanut/commands.go`, tests, embedded `app/assets/ack.wav` copied from root `assets/ack.wav`.

**Interfaces:** Native capture/player implementations behind existing interfaces; `SpeakerStore`; CLI command handlers.

- [ ] Write failing tests for embedded ACK loading, WAV playback routing, enrollment aggregation without raw recording persistence, `speak`, `transcribe`, and `test-audio` command dispatch.
- [ ] Run focused tests; confirm missing handlers.
- [ ] Implement desktop capture/player where supported and fake/file adapters otherwise; embed `assets/ack.wav`; keep hardware format normalization at boundary.
- [ ] Implement multiple enrollment samples, aggregated embedding storage, and `speaker=unknown` fallback.
- [ ] Add minimal flag/argument parsing with no CLI framework; document model prerequisites.
- [ ] Run tests and `cd app; go test ./...`; commit `feat: add audio adapters and cli flows`.

### Task 8: End-to-end coordinator and installer

**Files:** Create `app/internal/pipeline/coordinator.go`, `app/internal/interaction/response.go`, `app/internal/install/install.ps1`, `app/internal/install/install.sh`, `app/internal/install/dev-ha.md`, `app/samples/command.wav`, integration tests, update README. Build executable to root `build/peanut`.

**Interfaces:** Coordinator consumes all earlier interfaces; dry-run executor logs validated plans and returns `Okay.`; Home Assistant provider executes music plans.

- [ ] Write failing fake-component tests for full separate-utterance happy path, ACK tail, pre-roll, wake suppression during playback, no-speech timeout, max-command timeout, capture overflow, speaker failure, STT/TTS/playback failure recovery, and ordered multi-step routing.
- [ ] Run focused tests; confirm coordinator behavior is absent.
- [ ] Implement one coordinator event loop, detector reset/rearm, concurrent speaker/STT processing, Qwen plan validation, sequential provider execution, dry-run fallback, static spoken result, metrics, clean shutdown, and debug WAV output only when enabled.
- [ ] Implement production installer prompts: existing HA URL/token or optional official HA Container pull/run; never clone HA source. Keep Peanut native and configure SQLite through API.
- [ ] Add dev instructions using `dev/home-assistant-core` or any reachable HA box; production docs distinguish container from source checkout.
- [ ] Run integration tests, `cd app; go test ./...`, `cd app; go vet ./...`; commit `feat: add end to end voice runtime`.

### Task 9: Whole-branch hardening

**Files:** Update `README.md`, `docs/pi5-validation.md`, CI/build scripts, model/install docs, tests as needed.

- [ ] Write failing tests for configuration API auth boundaries, model startup validation, clean shutdown, and ARM64 build-tag selection.
- [ ] Run focused tests; confirm missing hardening behavior.
- [ ] Implement startup dependency checks, redacted structured logs/metrics, Linux ARM64 build documentation, model download verification, and Pi validation checklist.
- [ ] Run `cd app; go test ./...`, `cd app; go vet ./...`, native smoke tests when available, and a documented cross-build check; commit `chore: harden pi deployment path`.

## Verification matrix

- Pure Go: `cd app; go test ./...` covers config, migrations, ring buffer, state machine, capability schema, intent validation, provider registry, API auth/redaction, coordinator fakes.
- File-driven: WAV normalization, VAD, STT, speaker, Qwen intent, provider dry run, TTS when model bundles exist.
- Native: sherpa/llama.cpp library loading and CPU inference on desktop; Raspberry Pi ARM64, real-time factor, memory, device, thermal, and long-run checks remain pending until hardware exists.
