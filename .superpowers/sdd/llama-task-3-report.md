# Task 3 Report: llama.cpp CGO Boundary

## Result

Implemented Task 3 in isolated worktree:

`C:\Users\KN\.codex\worktrees\pinned-llama-cgo\peanut`

Commit:

`5967f1b feat: isolate llama.cpp behind CGO`

No runtime wiring, frontend changes, dependency updates, model files, or
subprocess inference added.

## Files

- `app/internal/llama/llama.go`
  - Defines `Request`, `Engine`, typed native errors, request/open validation,
    and the output-token bound.
- `app/internal/llama/native_stub.go`
  - Default `!peanut_llama` or `!cgo` implementation.
  - Returns explicit `ErrUnavailable`; does not inspect model bytes.
  - `Close` is safe.
- `app/internal/llama/native_cgo.go`
  - Opt-in `cgo && peanut_llama` wrapper.
  - Owns temporary C strings, copies output into Go memory, frees native output
    on every path, serializes engine access, and forwards context cancellation
    to the native abort flag.
- `app/internal/llama/native.h`
  - Opaque C ABI only: engine handle, status codes, open/generate/abort/output
    free/close operations.
- `app/internal/llama/native.c`
  - Uses only the pinned `include/llama.h` API.
  - Keeps model, context, sampler, output, and cancellation state private.
  - Configures CPU threads, deterministic temperature-zero greedy sampling,
    grammar-constrained generation, bounded output, and abort callbacks.
  - Exposes no tensors, llama structs, C strings, or native pointers to Go.
- `app/internal/llama/llama_test.go`
  - Stub availability contract.
  - Invalid thread/token-limit contract.
- `tools/build-llama.sh`
  - Verifies pinned SHA `a894dae939d426954ce54bb604824f1ae918a0c5`.
  - Builds static CPU-only llama/ggml archives into
    `build/llama/<target>/prefix`.
  - Prints `CGO_CFLAGS`, `CGO_LDFLAGS`, and the tagged Go build command.
  - Does not fetch, pull, download models, run Docker, or invoke inference.
- `tools/llama.cpp.md`
  - Documents native build prefix, tag, and stub behavior.

## TDD evidence

### RED

Before production files existed:

```text
go test -count=1 ./internal/llama
```

Failed at the missing boundary as required:

```text
undefined: Open
undefined: ErrUnavailable
undefined: validateRequest
undefined: Request
```

### GREEN

After implementation:

```text
go test -count=1 ./internal/llama
```

Passed:

```text
ok  peanut/internal/llama  0.292s
```

## Verification

Passed:

```text
go test -count=1 ./internal/llama
go vet ./internal/llama
```

Passed compile-only Linux ARM64 check from Windows:

```text
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c -o .llama-arm64.test ./internal/llama
```

The exact temporary output was removed after compilation.

Passed native C syntax check with the pinned llama and ggml include paths:

```text
gcc -std=c11 -fsyntax-only \
  -Iapp/internal/llama \
  -Ithird-party/llama.cpp/include \
  -Ithird-party/llama.cpp/ggml/include \
  app/internal/llama/native.c
```

Passed staged whitespace check:

```text
git diff --cached --check
```

The forbidden-pattern scan found no `exec.Command`, `llama-cli`, raw tensor
usage, or fetch/download/Docker commands in Task 3 files.

## Environment limits

The host has no built native llama static archives. Therefore:

```text
go test -tags peanut_llama -run '^$' ./internal/llama
```

compiled the CGO/native sources successfully, then stopped at expected linker
errors for missing `-llama`, `-lggml`, `-lggml-cpu`, and `-lggml-base` archives.
Those archives are produced by `tools/build-llama.sh` on a Linux native build
host or CI runner.

The host also has no WSL distribution, so `bash -n tools/build-llama.sh` could
not run locally. The script was reviewed for `set -euo pipefail`, exact pin
checking, quoted paths, no-fetch behavior, and the required CMake flags.

## Task 3 review fixes

Applied all requested review fixes without Task 4 runtime wiring:

- Renamed the native shim to `native.cpp`. `Request.Schema` remains JSON
  Schema text; the shim parses it with pinned `common_json::parse`, converts it
  with `json_schema_to_grammar(..., true)`, then passes the resulting GBNF to
  `llama_sampler_init_grammar`. Conversion exceptions remain strict grammar
  failures. No subprocess or raw tensor API was added.
- Added deferred CGO output cleanup after every native generation call. Success
  bytes are copied before the deferred free runs.
- Added fixed sampler seed `42`, zero temperature, and seeded distribution
  sampling for deterministic constrained generation.
- Enabled `LLAMA_BUILD_COMMON`, disabled common subprocess support, copied all
  static archives including `llama-common` into the install prefix, and added
  common libraries to the printed and CGO linker flags.
- `tools/build-llama.sh` now rejects target/host mismatch unless an existing
  `CMAKE_TOOLCHAIN_FILE` is supplied.

## Review-fix verification

Passed:

```text
g++ -std=c++17 -fsyntax-only -I./app/internal/llama -I./third-party/llama.cpp/include -I./third-party/llama.cpp/ggml/include -I./third-party/llama.cpp/common ./app/internal/llama/native.cpp
bash -n tools/build-llama.sh
go test -count=1 ./internal/llama
ok   peanut/internal/llama  0.325s
go vet ./internal/llama
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c -o .llama-arm64.test ./internal/llama
git diff --check
```

The temporary ARM64 test binary was removed. Native archive linking was not
run because this host has no built pinned llama.cpp archives; the updated
build helper now produces and installs the required `llama-common` archive.

## Task 3 test-gap follow-up

Updated the invalid thread and max-token requests in
`app/internal/llama/llama_test.go` to include `Prompt: "x"`, ensuring validation
reaches the intended branches.

Fresh verification:

```text
go test -count=1 ./internal/llama
ok   peanut/internal/llama 0.347s

go vet ./internal/llama
passed
```

## Repeated-generation context reset fix

`peanut_llama_generate` now clears the pinned llama memory with
`llama_memory_clear(llama_get_memory(engine->context), true)` after native
input validation and before tokenization. A null context or memory handle
returns `PEANUT_LLAMA_CONTEXT_FAILED` safely. This prevents a reused native
context from carrying KV state between generations.

Added `TestNativeGenerateClearsContextMemoryBeforeTokenization`, a focused
source-order regression proving the pinned clear call remains before
`llama_tokenize`.

TDD evidence:

```text
RED: go test -count=1 ./internal/llama
FAIL TestNativeGenerateClearsContextMemoryBeforeTokenization: native generation does not clear context memory with pinned API

GREEN: go test -count=1 ./internal/llama -run TestNativeGenerateClearsContextMemoryBeforeTokenization -v
PASS
```

Verification:

```text
go test -count=1 ./internal/llama
ok   peanut/internal/llama 0.283s

go vet ./internal/llama
passed

g++ -std=c++17 -fsyntax-only -Iapp/internal/llama -Ithird-party/llama.cpp/include -Ithird-party/llama.cpp/ggml/include -Ithird-party/llama.cpp/common app/internal/llama/native.cpp
passed

git diff --check
passed
```

## Remaining Task 3 idempotence fix

`tools/build-llama.sh` previously searched all of `$build` after install,
including `$prefix/lib`. On a rerun, `cp` therefore attempted to copy the
installed archives onto themselves. The archive search now prunes `$prefix`
from its source scope. Added `tools/build-llama-test.sh` as a focused source
regression check for that guard.

TDD evidence:

```text
RED: tools/build-llama-test.sh
archive copy must prune the install prefix from its source scope

GREEN: tools/build-llama-test.sh
passed
```

Verification:

```text
bash -n tools/build-llama.sh
passed

bash -n tools/build-llama-test.sh
passed

tools/build-llama.sh riscv64
rejected with unsupported target (exit 2)

tools/build-llama.sh arm64
rejected host/target mismatch without a toolchain (exit 2)

go test -count=1 ./internal/llama
ok   peanut/internal/llama 0.321s

go vet ./internal/llama
passed

git diff --check
passed
```
