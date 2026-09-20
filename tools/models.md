# Local sherpa model bundles

Peanut discovers sherpa model profiles under the configured `Models.Root`
directory (default `models`). Runtime configuration, including that root, lives
in SQLite; model bytes never do.

The intent model is separate: place one external GGUF directly under the model
root and set `Models.IntentModel` to its filename:

```text
models/<model-name>.gguf
Models.Root=models
Models.IntentModel=functiongemma.gguf
```

The filename is flat, regular, and `.gguf`; it is not a role/profile bundle and
is never stored in Git or SQLite. Linux native builds load it in-process with
the `peanut_llama` tag. Default Windows/macOS builds use the explicit
unavailable stub.

## Layout and manifest

Each profile has this shape:

```text
models/<role>/<profile>/model.json
models/<role>/<profile>/<entry and companion files>
```

Allowed roles are `kws`, `vad`, `stt`, `speaker`, and `tts`. Example:

```json
{
  "id": "stt-local",
  "role": "stt",
  "runtime": "local",
  "entry": "model.int8.onnx",
  "sha256": ""
}
```

Manifest fields:

- `id`: stable profile identifier; duplicate IDs are invalid.
- `role`: one of the five sherpa roles and must match the parent directory.
- `runtime`: adapter name used by the role.
- `entry`: relative regular file used as the profile entry point; it cannot escape the profile directory.
- `sha256`: optional checksum for `entry`, compared case-insensitively; empty disables the checksum check.

Unknown manifest fields are rejected. Invalid profiles are reported without discarding a previously active valid role.

Download official bundles from the [sherpa-onnx releases](https://github.com/k2-fsa/sherpa-onnx/releases), extract them into their profile directories, and add the manifest. Bundle roles may keep their required companion files beside `entry`.

Native sherpa profiles require these exact files:

- KWS: `encoder-epoch-12-avg-2-chunk-16-left-64.int8.onnx`, `decoder-epoch-12-avg-2-chunk-16-left-64.onnx`, `joiner-epoch-12-avg-2-chunk-16-left-64.int8.onnx`, `tokens.txt`, and profile-owned `keywords.txt` in sherpa keyword-file format.
- VAD: `silero_vad.onnx` as profile entry.
- STT: `model.int8.onnx` and `tokens.txt`.
- Speaker: `3dspeaker_speech_eres2net_base_sv_zh-cn_3dspeaker_16k.onnx` as profile entry.
- TTS: `en_GB-cori-medium.onnx`, `tokens.txt`, and `espeak-ng-data/`.

Create a checksum before filling `sha256`:

```powershell
(Get-FileHash .\models\stt\local\model.int8.onnx -Algorithm SHA256).Hash.ToLowerInvariant()
```

```sh
sha256sum models/stt/local/model.int8.onnx
```

Copy the resulting 64-character hexadecimal value into `model.json`. Do not commit model bundles, extracted files, source archives, or recordings. `models/` is ignored by Git, and model files are never written to SQLite.

## Verify and reload

From the repository root, after building the binary:

```powershell
.\build\peanut model list
.\build\peanut model verify
```

```sh
./build/peanut model list
./build/peanut model verify
```

`model verify` returns an error when any discovered profile is invalid. `GET /api/v1/models` rescans manifests, verifies checksums, reconciles SQLite metadata, and attempts a complete runtime reload. A failed reload keeps the prior active runtime.

Peanut always uses CPU execution. Configure provider `cpu` and a positive SQLite thread count. Pure builds and unit tests do not require CUDA, cloud inference, CGO, model downloads, or Home Assistant.

## Optional native build

The `sherpa` build tag enables the CGO C API boundary. Install a released sherpa-onnx C API archive containing `sherpa-onnx/c-api/c-api.h` and the `sherpa-onnx-c-api` shared library; do not clone sherpa-onnx source into this repository. Build from `app/` with its include and library directories:

The tagged build enables native KWS, Silero VAD, SenseVoice STT, speaker embedding extraction, and Piper/VITS TTS. VAD defaults to a `0.5` speech threshold; Go callers needing another value can pass `sherpa.WithVADThreshold(value)` to `sherpa.NewVAD` without changing manifest or SQLite schema. Speaker extraction implements `internal/speaker.Embedder`; runtime matching returns an enrolled ID or `unknown`, never a fabricated identity. TTS output is linearly resampled to runtime-required 16 kHz mono and then validated.

```powershell
$env:CGO_ENABLED = '1'
$env:CGO_CFLAGS = '-IC:\sherpa-onnx\include'
$env:CGO_LDFLAGS = '-LC:\sherpa-onnx\lib'
$env:PATH = "C:\sherpa-onnx\lib;$env:PATH"
go build -tags sherpa -o ../build/peanut.exe ./cmd/peanut
```

Linux ARM64 uses the same header plus its ARM64 shared library:

```sh
CGO_ENABLED=1 \
CGO_CFLAGS="-I/opt/sherpa-onnx/include" \
CGO_LDFLAGS="-L/opt/sherpa-onnx/lib -Wl,-rpath,/opt/sherpa-onnx/lib" \
go build -tags sherpa -o ../build/peanut ./cmd/peanut
```

Native load/inference smoke is optional, expects five official bundles directly under test-only `PEANUT_MODEL_ROOT`, and exercises all sherpa adapters. Missing model root or required model files skips smoke. Header and shared library must be installed before tagged test can compile. This environment variable selects test fixtures only; it is not a runtime configuration mechanism.

```powershell
$env:PEANUT_MODEL_ROOT = 'C:\models'
go test -tags sherpa ./internal/ml/sherpa -run TestNativeSmoke -v
```
