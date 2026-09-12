# Peanut Local Voice Runtime Design

## Scope

Build the first offline-capable voice loop for Raspberry Pi 5 Linux ARM64 and desktop development. The loop uses normalized 16 kHz mono float32 audio, separate wake and command utterances, CPU-only sherpa-onnx inference, local Qwen3 intent parsing, local TTS, SQLite persistence, and a local Home Assistant provider for music playback.

Qwen3 never writes the spoken answer. It converts the transcript plus the current capability snapshot into a validated action plan. V1 uses a dry-run executor when no real provider is configured; Home Assistant music execution is the first real provider path.

## Runtime flow

`WaitingWake -> Acknowledging -> WaitingSpeech -> Recording -> Processing -> Synthesizing -> Speaking -> WaitingWake`.

One coordinator owns state. KWS runs only in `WaitingWake`. VAD starts only after acknowledgement playback and the configurable acoustic tail. A 300 ms ring-buffer pre-roll is prepended at speech start. Microphone frames never run ML work in the capture callback. KWS and VAD reset before rearming. No microphone input can trigger Peanut during acknowledgement or speech playback.

Processing runs speaker identification and offline STT concurrently. Speaker failure is advisory and cannot block transcription. Recoverable failures return to `WaitingWake`; fatal missing or invalid startup models fail clearly.

## Capability and intent contract

Providers expose domain-neutral capabilities:

```go
type CapabilityProvider interface {
    Descriptor() ProviderDescriptor
    Discover(context.Context) ([]Capability, error)
    Execute(context.Context, ActionRequest) (ActionResult, error)
}
```

Each capability has a stable device ID, provider ID, type, name, room, action IDs, and JSON-compatible argument schemas. SQLite persists provider/device/action metadata and refreshed discovery timestamps. Fresh discovery wins; cached metadata supports offline parsing but never pretends execution succeeded.

For each transcript, Peanut renders a strict Qwen system prompt containing the current capability snapshot and emits JSON constrained by schema:

```json
{
  "version": 1,
  "status": "execute|clarify|unknown",
  "language": "en",
  "steps": [{
    "device_id": "...",
    "action_id": "...",
    "arguments": {}
  }],
  "clarification": "",
  "confidence": 0.0
}
```

Go validates JSON, status, step order, device IDs, action IDs, argument types, and required fields before execution. The model may select only capabilities in the supplied snapshot. Multilingual transcripts map to the same stable IDs. Ambiguous requests become `clarify`; unsupported requests become `unknown`.

## Home Assistant provider

Home Assistant runs locally beside Peanut, defaulting to `http://127.0.0.1:8123`. A bearer token comes from configuration, environment, or a secret file and is never stored in SQLite or logs. Discovery uses HA REST state/service APIs. The first provider exposes `music.play` for media-player entities, with source/query/URI arguments. Execution maps only validated requests to `media_player.play_media`; YouTube resolution remains Home Assistant integration detail. HA outages yield `provider_unavailable`.

The provider interface is the plugin seam. V1 uses compiled in-process providers and a fake provider for tests. Dynamic Go loading is excluded because it is fragile across ARM64 builds. A later process plugin can implement the same contract without changing the coordinator or intent schema.

## ML and platform boundaries

Sherpa-onnx is isolated behind KWS, VAD, STT, speaker, TTS, and playback interfaces. Native bindings live at infrastructure edges. CPU provider and configurable thread counts are mandatory; no CUDA or x86-only code. Initial model manifest pins a mobile int8 KWS model, Silero VAD, multilingual tiny offline STT, lightweight 16 kHz speaker embedding, and configurable Piper/VITS-class TTS. Models stay outside Git and are validated at startup.

Qwen3 `Qwen3-0.6B-Q4_K_M.gguf` is optional only as intent parser input. llama.cpp runs CPU-only with structured JSON grammar, deterministic low-temperature generation, and Qwen `enable_thinking=false` / reasoning disabled. No chat response generation.

## Storage and configuration

SQLite uses embedded lightweight migrations and stores speakers, embeddings, settings, providers, devices, and capabilities. Audio streams, recordings, TTS buffers, and models stay out of SQLite. Database path, model paths, HA URL/token, audio devices, thresholds, timeouts, pre-roll, acoustic tails, and debug-audio output are centralized in configuration with development overrides.

## CLI and testing

Provide minimal commands: `run`, `enroll <id>`, `speak <text>`, `transcribe <wav>`, and `test-audio <wav>`. Use embedded `assets/ack.wav`. File-driven tests exercise WAV -> VAD -> STT -> speaker -> intent -> provider -> TTS where models exist. Unit tests cover state transitions, ring buffer, pre-roll, config, SQLite, intent validation, capability snapshots, timeout recovery, and fake provider routing. Hardware adapters remain separately testable and Pi validation is documented as pending.

## Explicit non-goals

No cloud APIs, streaming STT, barge-in, echo cancellation, voice cloning, direct YouTube API, generic plugin marketplace, raw audio persistence, or home-agent reasoning/tool intelligence beyond validated capability plans.
