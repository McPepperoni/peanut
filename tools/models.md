# Local model bundles

Peanut discovers local model profiles under the configured `Models.Root` directory (default `models`). Runtime configuration, including that root, lives in SQLite; model bytes never do.

## Layout and manifest

Each profile has this shape:

```text
models/<role>/<profile>/model.json
models/<role>/<profile>/<entry and companion files>
```

Allowed roles are `kws`, `vad`, `stt`, `speaker`, `tts`, and `intent`. Example:

```json
{
  "id": "intent-local",
  "role": "intent",
  "runtime": "local",
  "entry": "model.gguf",
  "sha256": "",
  "threads": 4
}
```

Manifest fields:

- `id`: stable profile identifier; duplicate IDs are invalid.
- `role`: one of the six runtime roles and must match the parent directory.
- `runtime`: adapter/executable name used by the role; non-empty.
- `entry`: relative regular file used as the profile entry point; it cannot escape the profile directory.
- `sha256`: optional checksum for `entry`, compared case-insensitively; empty disables the checksum check.
- `threads`: declared profile thread count; keep it positive and aligned with the SQLite CPU configuration.

Unknown manifest fields are rejected. Invalid profiles are reported without discarding a previously active valid role.

Download official bundles from the [sherpa-onnx releases](https://github.com/k2-fsa/sherpa-onnx/releases), extract them into their profile directories, and add the manifest. Bundle roles may keep their required companion files beside `entry`.

Create a checksum before filling `sha256`:

```powershell
(Get-FileHash .\models\intent\local\model.gguf -Algorithm SHA256).Hash.ToLowerInvariant()
```

```sh
sha256sum models/intent/local/model.gguf
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

The tagged build currently performs native inference only for the typed SenseVoice transcriber. KWS, VAD, speaker identification, and TTS expose their typed interfaces but return `sherpa.ErrUnavailable`; their C API wiring still requires validation against installed headers and model bundles. They do not return fabricated inference results.

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

Native load/inference smoke is optional, expects the five official bundles directly under a test-only `PEANUT_MODEL_ROOT`, and runs one second of silent 16 kHz mono audio through SenseVoice. This environment variable selects test fixtures only; it is not a runtime configuration mechanism.

```powershell
$env:PEANUT_MODEL_ROOT = 'C:\models'
go test -tags sherpa ./internal/ml/sherpa -run TestNativeSmoke -v
```
