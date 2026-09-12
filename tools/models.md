# Local model bundles

Peanut uses local sherpa-onnx models only. Download these official bundles outside the repository, then record their paths in SQLite configuration:

- `sherpa-onnx-kws-zipformer-wenetspeech-3.3M-2024-01-01-mobile`
- `silero_vad.onnx`
- `sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17`
- `3dspeaker_speech_eres2net_base_sv_zh-cn_3dspeaker_16k.onnx`
- `vits-piper-en_GB-cori-medium`

Use the [sherpa-onnx release bundles](https://github.com/k2-fsa/sherpa-onnx/releases). Configure provider `cpu` and a positive thread count. Peanut does not require CUDA, cloud inference, CGO, or a native runtime for unit tests.

Do not commit model bundles, extracted model files, source archives, or recordings. Keep them under `local-model/` or another local path outside Git. No thinking is needed: this runtime executes the configured local models directly.
