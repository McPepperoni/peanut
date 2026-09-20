# Pinned llama.cpp CGO Runtime Design

Status: Approved for implementation
Date: 2026-09-20

## Scope

Milestone 1 adds an in-process, CPU-only llama.cpp runtime for FunctionGemma. It pins llama.cpp as a git submodule, builds static llama/ggml libraries with CMake, and exposes them to Go through a small CGO boundary.

Console, structured live event streams, traces, TTS, browser audio, systemd packaging, and the remaining playground modes are later milestones. This milestone must not add Docker Compose or model weights.

Build targets are four explicit platforms:

- Linux ARM64: native llama backend;
- Linux amd64: native llama backend;
- Windows amd64: default Go build with explicit llama-unavailable stub; native CGO is opt-in;
- macOS arm64: default Go build with explicit llama-unavailable stub; native CGO is opt-in.

## Source pin and setup

Use submodule path `third-party/llama.cpp` with upstream URL:

`https://github.com/ggml-org/llama.cpp.git`

Pin the gitlink to exact commit:

`a894dae939d426954ce54bb604824f1ae918a0c5`

Setup performs only:

```sh
git submodule update --init --recursive third-party/llama.cpp
```

It must not use `--remote`, track a branch, download model files, install packages, run Docker, or silently resolve a newer commit. Documentation and setup checks must make the pinned SHA visible.

## Native build

Build llama.cpp from the pinned submodule with CMake and static libraries:

- `BUILD_SHARED_LIBS=OFF`;
- CPU backend only;
- CUDA, Metal, Vulkan, ROCm, OpenCL, and other accelerator backends disabled;
- ARM64 runtime CPU kernels enabled where supported by the pinned source;
- examples, tests, and CLI tools disabled for the Peanut production build.

The production Go binary links the static llama/ggml archives through CGO. Full static libc linking is not required; static llama/ggml is the target, while the platform libc remains dynamically linked when required by Linux ARM64.

The native build command is separate from submodule setup. It may invoke CMake and the host compiler, but it must never fetch source or models. Build output belongs under ignored `build/`.

Default desktop builds use the explicit stub so ordinary Windows and macOS development does not require a native llama toolchain. Platform-specific native builds remain available through an opt-in build tag and documented CMake flags. Linux release artifacts must use the native backend.

## Go and CGO boundary

Create `app/internal/llama` as the only package that includes llama.cpp headers or uses CGO. The exported Go surface stays small and testable:

```go
type Request struct {
    Prompt    string
    Schema    string
    Threads   int
    MaxTokens int
}

type Engine interface {
    Generate(context.Context, Request) ([]byte, error)
    Close() error
}

func Open(context.Context, string, int) (Engine, error)
```

The exact internal names may vary, but these constraints are binding:

- no raw tensors, llama structs, C pointers, or C strings escape the package;
- no shell command or subprocess is used for inference;
- context cancellation reaches a native abort callback;
- native failures become typed Go errors without leaking model contents;
- a non-CGO, unsupported-platform, or build without the opt-in native tag returns an explicit unavailable error through a stub;
- engine ownership is explicit and `Close` is safe during failed reload cleanup.

The C shim handles model/context creation, deterministic generation, JSON-schema constrained decoding, output copying, cancellation, and cleanup. It does not expose a general tensor API.

## Model placement and configuration

GGUF files live outside Git in a flat configurable model directory:

```text
/var/lib/peanut/models/<model-name>.gguf
```

SQLite remains runtime configuration source of truth. Keep `Models.Root` as the external directory and add the first binding as `Models.IntentModel`, a filename relative to `Models.Root`. The loader resolves and validates `Root + IntentModel`:

- filename must be non-empty and end in `.gguf`;
- resolved path must remain inside `Models.Root`;
- symlink escapes, directories, and non-regular files are rejected;
- model bytes never enter SQLite or Git;
- the same filename may later bind multiple runtime purposes.

Existing Sherpa model bundles remain unchanged in this milestone because they require role-specific companion files. GGUF placement is flat and shared.

Native intent parsing continues to implement the existing `intent.IntentParser` contract. For a llama profile/binding it validates the capability snapshot, sends the deterministic prompt and strict plan schema, decodes JSON, and runs existing plan validation. It produces action plans only; no conversational output.

Reload constructs and validates the replacement engine before swapping it into the live runtime. Any load, validation, or swap failure preserves the previous engine.

## Documentation and files

Implementation may add or modify only files serving this milestone:

- `.gitmodules` and the `third-party/llama.cpp` gitlink;
- `app/internal/llama` CGO, stub, and tests;
- intent/runtime/config wiring and focused tests;
- pinned setup/build helpers under `tools/` if needed;
- `tools/llama.cpp.md`, README, and Pi validation notes;
- `.github/workflows/build.yml` and a tag-triggered release workflow.

No frontend, systemd unit, API schema, event stream, trace store, or browser-audio code belongs in this milestone.

## Testing and acceptance

Every behavior change starts with a failing test. Tests cover:

- safe path resolution and `.gguf` validation;
- native parser request construction, strict output parsing, and plan validation using a fake engine;
- engine replacement and rollback behavior;
- CGO stub behavior on unsupported builds;
- setup/build text proving exact submodule initialization and no latest/remote behavior.

Verification from `app/`:

```sh
go test ./...
go vet ./...
go build -o ../build/peanut ./cmd/peanut
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test ./... -run '^$'
```

On Linux ARM64, also run the pinned CMake static build and a CGO Go build. Native inference smoke requires a user-provided external GGUF and is not a repository test fixture.

## GitHub Actions release

Keep pull-request and main-branch verification in GitHub Actions. The build matrix must compile and test these four targets:

| Target | Default artifact | llama backend |
| --- | --- | --- |
| `linux/arm64` | `peanut-linux-arm64` | native static CGO |
| `linux/amd64` | `peanut-linux-amd64` | native static CGO |
| `windows/amd64` | `peanut-windows-amd64.exe` | explicit stub by default |
| `darwin/arm64` | `peanut-darwin-arm64` | explicit stub by default |

On pushes of tags matching `v*`, the workflow uploads these artifacts to one GitHub Release. Release creation uses the repository's `GITHUB_TOKEN`, requires `contents: write`, and must not require secrets, model weights, or external package registries. Non-tag builds upload only temporary workflow artifacts or run verification; they must not create releases.

The release workflow must initialize the exact submodule commit, verify the gitlink matches `a894dae939d426954ce54bb604824f1ae918a0c5`, build native Linux archives before the Go build, and fail on drift. Windows/macOS jobs still compile the same Go API against the unavailable stub unless explicitly running a native opt-in job.

## Non-goals

- no model download or model weights in Git;
- no runtime configuration from environment variables;
- no llama.cpp subprocess or CLI dependency;
- no Docker Compose;
- no new Go dependency unless implementation proves one unavoidable; any dependency update requires a separate reviewed PR;
- no Console or live log/trace UI yet.
