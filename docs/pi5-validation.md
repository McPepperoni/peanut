# Raspberry Pi 5 validation

Peanut is a local-first, CPU-only runtime. Mandatory validation uses portable Go code and does not download models, call cloud services, require CUDA, or require CGO. Native sherpa and system-audio checks are optional and run only on a machine with matching dependencies.

## Platform matrix

| Target | Mandatory check | Native check |
| --- | --- | --- |
| Linux desktop | Pure test, vet, and build | Optional sherpa smoke and audio check |
| Windows desktop | Pure test, vet, and build | Optional platform backend check |
| macOS desktop | Pure test, vet, and build | Optional platform backend check |
| Linux ARM64 / Pi 5 | ARM64 compile-only check | Optional sherpa/audio/runtime check |

From `app/`, run the mandatory commands:

```sh
go test ./...
go vet ./...
go build -o ../build/peanut ./cmd/peanut
```

PowerShell is also supported after `Set-Location app`. This command writes `build/peanut`; pass `-o ../build/peanut.exe` when a Windows `.exe` suffix is required.

The Linux ARM64 compile-only check is:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test ./... -run '^$' -exec true
```

`-exec true` compiles target test binaries without trying to execute ARM64 binaries on the build host.

## Model installation

Set the model root through SQLite configuration; the default is `models`. Do not use environment variables for runtime configuration. Install profiles at:

```text
models/<role>/<profile>/model.json
models/<role>/<profile>/<entry and companion files>
```

The manifest fields are:

| Field | Meaning |
| --- | --- |
| `id` | Stable profile ID; duplicate IDs are invalid. |
| `role` | `kws`, `vad`, `stt`, `speaker`, `tts`, or `intent`; must match the directory. |
| `runtime` | Non-empty adapter/executable name. |
| `entry` | Relative regular entry file contained by the profile directory. |
| `sha256` | Optional SHA-256 of `entry`; empty means no checksum assertion. |
| `threads` | Declared positive profile thread count, kept aligned with SQLite CPU settings. |

Unknown fields, path traversal, missing entry files, and checksum mismatches make a profile invalid. Invalid profiles appear in `model verify` and `GET /api/v1/models`; they do not replace a previously active valid role. `models/` is ignored by Git. SQLite stores profile metadata and selection only, never model bytes.

Create a checksum with `sha256sum models/<role>/<profile>/<entry>` on Linux or `(Get-FileHash .\models\<role>\<profile>\<entry> -Algorithm SHA256).Hash.ToLowerInvariant()` in PowerShell, then place the result in `sha256`.

Verify from the repository root:

```sh
./build/peanut model list
./build/peanut model verify
```

```powershell
.\build\peanut model list
.\build\peanut model verify
```

`GET /api/v1/models` rescans manifests and attempts a complete runtime reload, then returns `profiles`, `active`, and `errors`. A failed reload preserves the prior active runtime. If LAN access is enabled, send the SQLite-configured pairing token as `Authorization: Bearer <token>`; localhost remains unauthenticated by default.

## Native prerequisites and failure boundary

Optional sherpa validation needs a released sherpa-onnx C API archive, matching headers and shared library, a CPU-only build with `CGO_ENABLED=1`, and the official model bundles. Do not clone sherpa-onnx into this repository. The native build must use the matching include/library paths and ARM64 shared library on Pi 5.

Linux uses the system `arecord` and `aplay` utilities as its narrow audio boundary. Install ALSA utilities, grant the service access to the selected devices, and set SQLite `Audio.InputDevice` / `Audio.OutputDevice` when the defaults are not correct. Capture and playback use raw signed 16-bit little-endian PCM at 16 kHz mono; capture is emitted as 320-sample frames. Non-Linux platforms keep the unsupported runtime boundary. File WAV adapters and pure tests remain available.

Startup validates all six active roles before opening native audio. Useful failure prefixes:

- `load <role> model:` — required profile missing or invalid.
- `scan model roles at <root>:` — model-root scan or runtime-swap failure.
- `build with the sherpa-onnx native adapter` — binary lacks the optional native runtime.
- `system audio ... is unsupported on <platform>` — no system audio backend for the target.
- `start arecord` or `start aplay` — ALSA utility is missing or cannot be started.

## Pi 5 runtime checks

Run with the exact model profiles and audio device intended for deployment. Record model IDs, config, commit, ambient temperature, and results.

1. Real-time factor: record `processing seconds / audio seconds`. A value below `1.0` keeps up with real time; compare each profile/configuration against its baseline.
2. Memory: record peak resident memory while idle, loading models, and processing speech. Raspberry Pi OS `/usr/bin/time -v ./build/peanut run` reports maximum resident set size.
3. Thermal: during sustained processing, record `vcgencmd measure_temp` and `vcgencmd get_throttled`. Investigate throttling, crashes, or steadily rising temperature before deployment.
4. Long run: operate for at least one representative session window, exercise wake/VAD/STT/intent/TTS, reload through `GET /api/v1/models`, and confirm no resource, memory, or latency drift.
5. Failure preservation: introduce a bad checksum or missing entry for one role, call reload, confirm the invalid profile is reported, and confirm previously active valid roles remain available.

Pure test, vet, and build commands are mandatory for every change. Native sherpa inference, hardware audio, real-time factor, memory, thermal, and long-run checks are optional smoke checks until Pi hardware, model files, shared libraries, and an audio backend are available.
