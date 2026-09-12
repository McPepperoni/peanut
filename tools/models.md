# Local model bundles

Peanut uses local sherpa-onnx models only. Download these official bundles outside the repository, then record their paths in SQLite configuration:

- `sherpa-onnx-kws-zipformer-wenetspeech-3.3M-2024-01-01-mobile`
- `silero_vad.onnx`
- `sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17`
- `3dspeaker_speech_eres2net_base_sv_zh-cn_3dspeaker_16k.onnx`
- `vits-piper-en_GB-cori-medium`

Use the [sherpa-onnx release bundles](https://github.com/k2-fsa/sherpa-onnx/releases). Configure provider `cpu` and a positive thread count. Default builds and unit tests do not require CUDA, cloud inference, CGO, or a native runtime.

Do not commit model bundles, extracted model files, source archives, or recordings. Keep them under `local-model/` or another local path outside Git. No thinking is needed: this runtime executes the configured local models directly.

## Optional native build

The `sherpa` build tag enables the CGO C API boundary. Install a released sherpa-onnx C API archive containing `sherpa-onnx/c-api/c-api.h` and the `sherpa-onnx-c-api` shared library; do not clone sherpa-onnx source into this repository. Build from `app/` with its include and library directories:

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

Native load/inference smoke expects the five bundles directly under `PEANUT_MODEL_ROOT` and runs one second of silent 16 kHz mono audio through SenseVoice:

```powershell
$env:PEANUT_MODEL_ROOT = 'C:\models'
go test -tags sherpa ./internal/ml/sherpa -run TestNativeSmoke -v
```
