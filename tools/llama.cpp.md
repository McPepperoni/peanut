# llama.cpp intent runtime

Peanut runs `llama-cli` as a CPU-only subprocess. Keep llama.cpp source and GGUF files outside Git. The expected model path, relative to the repository root, is:

```text
local-model/Qwen3-0.6B-Q4_K_M.gguf
```

## Build

Clone llama.cpp beside Peanut, not inside it. On Raspberry Pi OS 64-bit or another Linux ARM64 host:

```sh
sudo apt-get update
sudo apt-get install -y build-essential cmake git libopenblas-dev
cd ..
git clone --depth 1 https://github.com/ggml-org/llama.cpp.git
cmake -S llama.cpp -B llama.cpp/build \
  -DCMAKE_BUILD_TYPE=Release \
  -DGGML_CPU_KLEIDIAI=ON \
  -DGGML_BLAS=ON \
  -DGGML_BLAS_VENDOR=OpenBLAS \
  -DGGML_CUDA=OFF
cmake --build llama.cpp/build --config Release --parallel 4 --target llama-cli
```

`GGML_CPU_KLEIDIAI=ON` enables runtime-selected ARM64 CPU kernels. `GGML_BLAS=ON` accelerates prompt processing. Peanut also passes `--device none`, so inference cannot offload to an accelerator. Set SQLite model configuration `LlamaPath` to the resulting `llama.cpp/build/bin/llama-cli`; do not use environment variables for runtime configuration.

## Local smoke test

Run from the Peanut repository root. Adjust the executable path and thread count for the host:

```sh
../llama.cpp/build/bin/llama-cli \
  --model ./local-model/Qwen3-0.6B-Q4_K_M.gguf \
  --threads 4 --threads-batch 4 \
  --device none \
  --seed 1 --temp 0 \
  --reasoning off \
  --chat-template-kwargs '{"enable_thinking":false}' \
  --jinja --single-turn \
  --json-schema '{"type":"object","properties":{"version":{"type":"integer","const":1},"status":{"type":"string","enum":["execute","clarify","unknown"]},"language":{"type":"string"},"steps":{"type":"array","items":{"type":"object","properties":{"device_id":{"type":"string"},"action_id":{"type":"string"},"arguments":{"type":"object"}},"required":["device_id","action_id","arguments"],"additionalProperties":false}},"clarification":{"type":"string"},"confidence":{"type":"number","minimum":0,"maximum":1}},"required":["version","status","language","steps","clarification","confidence"],"additionalProperties":false}' \
  --no-display-prompt --no-show-timings --simple-io --offline --n-predict 512 \
  --prompt 'Convert the transcript into one JSON intent plan. Output JSON only; never answer the user. Use only capabilities in this snapshot: {"capabilities":[{"id":"capability.demo","provider_id":"provider.demo","device_id":"device.demo","type":"switch","name":"Demo switch","actions":[{"id":"switch.turn_on"}]}]}. Transcript: "turn on the demo switch"'
```

Expected stdout is one plan envelope using `device.demo` and `switch.turn_on`, with no reasoning or conversational response. Peanut still strictly decodes and validates every returned plan against the current capability snapshot.

Current flag and build references: [llama-cli options](https://github.com/ggml-org/llama.cpp/blob/master/tools/cli/README.md), [JSON schema grammars](https://github.com/ggml-org/llama.cpp/blob/master/grammars/README.md), and [CPU/ARM64 builds](https://github.com/ggml-org/llama.cpp/blob/master/docs/build.md).
