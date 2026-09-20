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
