# Pinned llama.cpp CGO Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a pinned, in-process CPU llama.cpp runtime for external FunctionGemma GGUF files, with four-platform builds and tag-triggered GitHub Releases.

**Architecture:** `third-party/llama.cpp` is a gitlink pinned to one commit. CMake builds static llama/ggml archives; only `app/internal/llama` uses CGO and exposes a small Go engine interface. Native Linux builds use opt-in tag `peanut_llama`; Windows/macOS default builds use an explicit unavailable stub. Existing intent validation remains the policy boundary.

**Tech Stack:** Go 1.27, CGO, C/C++, CMake, llama.cpp, GitHub Actions, existing SQLite configuration and model/runtime packages.

## Global Constraints

- Submodule path is `third-party/llama.cpp`.
- Upstream URL is `https://github.com/ggml-org/llama.cpp.git`.
- Pinned commit is `a894dae939d426954ce54bb604824f1ae918a0c5`.
- Setup runs only `git submodule update --init --recursive third-party/llama.cpp`.
- No `--remote`, branch tracking, model downloads, package installation, Docker, or latest resolution in setup.
- `BUILD_SHARED_LIBS=OFF`; CPU backend only; accelerator backends disabled.
- Native llama build tag is `peanut_llama`.
- GGUF files are external flat files under configurable `Models.Root`; first binding is `Models.IntentModel`.
- No model bytes in Git or SQLite.
- No subprocess inference, raw tensors, C pointers, llama types, secrets, or shell escapes across Go boundaries.
- Linux ARM64 and Linux amd64 release artifacts use native static CGO llama.
- Windows amd64 and macOS arm64 default artifacts use explicit llama-unavailable stub.
- Every behavior change starts with a failing test.
- No new Go dependency; any dependency update requires a separate reviewed PR.

---

### Task 1: Pin llama.cpp and add four-platform CI/release plumbing

**Files:**
- Create: `.gitmodules`
- Create: `third-party/llama.cpp` gitlink at exact pinned commit
- Create: `tools/setup-llama.sh`
- Create: `tools/setup-llama.ps1`
- Create: `.github/workflows/release.yml`
- Modify: `.github/workflows/build.yml`
- Modify: `app/internal/install/install_test.go`
- Modify: `tools/llama.cpp.md`

**Interfaces:**
- Setup scripts accept no source/model arguments and run only the exact submodule init command.
- CI verifies the checked-out gitlink resolves to `a894dae939d426954ce54bb604824f1ae918a0c5`.
- Release workflow runs on tags matching `v*`, builds four artifacts, and creates one GitHub Release using `GITHUB_TOKEN`.

- [ ] **Step 1: Write failing setup-contract tests.**

Extend `app/internal/install/install_test.go` to read repository-root `tools/setup-llama.sh`, `tools/setup-llama.ps1`, and `.gitmodules`. Assert:

```go
for _, path := range []string{"tools/setup-llama.sh", "tools/setup-llama.ps1"} {

	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil { t.Fatal(err) }
	text := string(data)
	if !strings.Contains(text, "git submodule update --init --recursive third-party/llama.cpp") {
		t.Fatalf("%s does not initialize pinned submodule", path)
	}
	if strings.Contains(text, "--remote") || strings.Contains(text, "git pull") || strings.Contains(text, "curl") || strings.Contains(text, "wget") {
		t.Fatalf("%s performs unpinned setup: %q", path, text)
	}
}

gitmodules, err := os.ReadFile(filepath.Join(root, ".gitmodules"))
if err != nil { t.Fatal(err) }
if !strings.Contains(string(gitmodules), "third-party/llama.cpp") || !strings.Contains(string(gitmodules), "https://github.com/ggml-org/llama.cpp.git") {
	t.Fatalf("unexpected .gitmodules: %q", gitmodules)
}
```

- [ ] **Step 2: Run the focused test and verify failure.**

Run from `app/`:

```text
go test ./internal/install -run 'Test(LlamaSetup|InstallScripts)'
```

Expected: FAIL because setup files and submodule metadata do not exist.

- [ ] **Step 3: Add the pinned submodule and setup scripts.**

Create `.gitmodules` with the exact path and URL. Add the submodule, checkout exact SHA, and stage the gitlink. Both scripts must contain only shell/PowerShell safety setup plus:

```sh
git submodule update --init --recursive third-party/llama.cpp
```

Do not add a branch, `--remote`, package installation, model download, or build command. Preserve non-zero exit status.

- [ ] **Step 4: Update build CI.**

Change checkout to initialize submodules and add a pinned-commit check:

```yaml
- uses: actions/checkout@v7
  with:
    submodules: recursive
- name: Verify llama.cpp pin
  shell: bash
  run: test "$(git -C third-party/llama.cpp rev-parse HEAD)" = "a894dae939d426954ce54bb604824f1ae918a0c5"
```

Keep existing tests for Linux, Windows, macOS, and Linux ARM64 compile. Add explicit build jobs for Linux amd64, Linux ARM64, Windows amd64, and macOS ARM64 using the stub default where native libraries are not built.

- [ ] **Step 5: Add tag-triggered release workflow.**

Create `.github/workflows/release.yml` with:

- trigger `push.tags: ["v*"]`;
- `permissions: contents: write`;
- one matrix for `linux/amd64`, `linux/arm64`, `windows/amd64`, `darwin/arm64`;
- native CMake + `go build -tags peanut_llama` for Linux targets;
- default stub Go builds for Windows/macOS;
- artifacts named `peanut-linux-amd64`, `peanut-linux-arm64`, `peanut-windows-amd64.exe`, and `peanut-darwin-arm64`;
- one release upload step using `GITHUB_TOKEN`.

Native Linux jobs must build the pinned submodule before Go and fail if the gitlink differs. No model files or registry credentials enter the workflow.

- [ ] **Step 6: Run focused tests and commit.**

Run:

```text
go test ./internal/install
git diff --check
```

Expected: PASS. Commit:

```text
git add .gitmodules third-party/llama.cpp tools/setup-llama.sh tools/setup-llama.ps1 .github/workflows/build.yml .github/workflows/release.yml app/internal/install/install_test.go tools/llama.cpp.md
git commit -m "build: pin llama.cpp and publish four-platform releases"
```

### Task 2: Add flat external GGUF configuration and path validation

**Files:**
- Modify: `app/internal/config/config.go`
- Modify: `app/internal/config/config_test.go`
- Create: `app/internal/models/gguf.go`
- Create: `app/internal/models/gguf_test.go`

**Interfaces:**
- `config.Models.IntentModel string` stores a filename relative to `config.Models.Root`.
- `models.ResolveGGUFPath(root, filename string) (string, error)` returns a validated existing regular file.
- `models.ResolveGGUFPath` rejects absolute paths, separators, `..`, wrong extension, missing files, directories, and symlink escapes.

- [ ] **Step 1: Write failing path and config tests.**

Add tests:

```go
func TestResolveGGUFPathAcceptsFlatModel(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "functiongemma.gguf")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil { t.Fatal(err) }
	got, err := ResolveGGUFPath(root, "functiongemma.gguf")
	if err != nil { t.Fatal(err) }
	if got != path { t.Fatalf("path = %q, want %q", got, path) }
}

func TestResolveGGUFPathRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"", "../outside.gguf", "nested/model.gguf", "/tmp/model.gguf", "model.bin"} {
		if _, err := ResolveGGUFPath(t.TempDir(), name); err == nil {
			t.Fatalf("ResolveGGUFPath(%q) accepted unsafe name", name)
		}
	}
}
```

Update default-config assertions to require `Models.IntentModel == "functiongemma.gguf"`. Add migration coverage proving missing field receives default while existing secrets and model root remain unchanged.

- [ ] **Step 2: Run focused tests and verify failure.**

Run:

```text
go test ./internal/models ./internal/config -run 'Test(ResolveGGUF|LoadDefaults|PersistDefaults)'
```

Expected: FAIL because the resolver and config field do not exist.

- [ ] **Step 3: Implement minimal safe resolver.**

Use `filepath.Base`, `filepath.Clean`, `filepath.Rel`, `os.Stat`, and `filepath.EvalSymlinks`. Require `.gguf` case-insensitively and a regular file. Evaluate both root and target before containment check. Return errors that identify validation class, not file contents.

- [ ] **Step 4: Add the config field and default/migration behavior.**

Add:

```go
type Models struct {
	Root        string
	IntentModel string
	Threads     int
	CPUOnly     bool
}
```

Set default `IntentModel` to `functiongemma.gguf`. Preserve it when loading existing SQLite JSON. If old stored JSON omits it, fill the default during persistence migration. Do not read it from environment variables.

- [ ] **Step 5: Run package tests and commit.**

Run:

```text
go test ./internal/models ./internal/config
go vet ./internal/models ./internal/config
```

Expected: PASS. Commit:

```text
git add app/internal/models/gguf.go app/internal/models/gguf_test.go app/internal/config/config.go app/internal/config/config_test.go
git commit -m "feat: validate flat external GGUF models"
```

### Task 3: Isolate llama.cpp behind CGO and build the native engine

**Files:**
- Create: `app/internal/llama/llama.go`
- Create: `app/internal/llama/native_cgo.go`
- Create: `app/internal/llama/native_stub.go`
- Create: `app/internal/llama/native.h`
- Create: `app/internal/llama/native.c`
- Create: `app/internal/llama/llama_test.go`
- Create: `tools/build-llama.sh`
- Modify: `tools/llama.cpp.md`

**Interfaces:**
- Build tag `peanut_llama` selects CGO implementation.
- Default `!peanut_llama` or `!cgo` selects stub.
- `llama.Request` has `Prompt string`, `Schema string`, `Threads int`, `MaxTokens int`.
- `llama.Engine` has `Generate(context.Context, Request) ([]byte, error)` and `Close() error`.
- `llama.Open(context.Context, modelPath string, threads int) (Engine, error)` opens one external GGUF.

- [ ] **Step 1: Write failing Go contract tests.**

Add a fake-engine contract test for request validation and a stub test compiled without `peanut_llama`:

```go
func TestStubOpenIsExplicitlyUnavailable(t *testing.T) {
	_, err := Open(context.Background(), "functiongemma.gguf", 2)
	if !errors.Is(err, ErrUnavailable) { t.Fatalf("error = %v", err) }
}

func TestRequestRejectsInvalidLimits(t *testing.T) {
	if err := validateRequest(Request{Threads: 0, MaxTokens: 1}); err == nil { t.Fatal("accepted zero threads") }
	if err := validateRequest(Request{Threads: 1, MaxTokens: 0}); err == nil { t.Fatal("accepted zero max tokens") }
}
```

- [ ] **Step 2: Run focused tests and verify failure.**

Run:

```text
go test ./internal/llama
```

Expected: FAIL because package and boundary do not exist.

- [ ] **Step 3: Implement Go boundary and stub.**

Define `ErrUnavailable`, `Request`, `Engine`, `Open`, and request validation. Stub `Open` returns `ErrUnavailable` without inspecting or logging model bytes. Keep `Close` safe on the stub.

- [ ] **Step 4: Implement the C ABI shim.**

Use the pinned `include/llama.h` API only. Keep opaque native state in a private C struct containing model, context, sampler, output buffer, and cancellation state. Export C functions for open, generate, abort flag, free output, and close. Configure CPU threads, deterministic seed/temperature, JSON-schema grammar, and bounded output. Copy output into Go-owned memory before returning. Never export tensors or native structs.

- [ ] **Step 5: Implement CGO wrapper under `cgo && peanut_llama`.**

Convert Go strings to temporary C strings, call the shim, copy returned bytes, free native memory on every path, and poll `ctx.Done()` through the shim abort callback. Translate native status codes to typed Go errors. Do not log model paths, prompts, schemas, or output.

- [ ] **Step 6: Add Linux native build helper.**

`tools/build-llama.sh` must:

1. verify `third-party/llama.cpp` is at the exact SHA;
2. configure CMake with `BUILD_SHARED_LIBS=OFF`, CPU-only options, and disabled examples/tests/tools;
3. build and install static archives into ignored `build/llama/<target>`;
4. print the `CGO_CFLAGS`, `CGO_LDFLAGS`, and `go build -tags peanut_llama` command needed by the caller.

It must not run `git pull`, `git fetch`, model downloads, or Docker. Add ARM64 cross-toolchain parameters only where the host provides them.

- [ ] **Step 7: Run boundary tests and commit.**

Run:

```text
go test ./internal/llama
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test ./internal/llama -run '^$'
git diff --check
```

Expected: PASS. Commit:

```text
git add app/internal/llama tools/build-llama.sh tools/llama.cpp.md
git commit -m "feat: isolate llama.cpp behind CGO"
```

### Task 4: Replace intent subprocess inference with native FunctionGemma and preserve reload safety

**Files:**
- Create: `app/internal/intent/native.go`
- Create: `app/internal/intent/native_test.go`
- Modify: `app/cmd/peanut/runtime.go`
- Modify: `app/cmd/peanut/runtime_test.go`
- Modify: `app/internal/intent/model_test.go`

**Interfaces:**
- `intent.NativeParser` implements existing `intent.IntentParser` and `io.Closer`.
- Constructor: `intent.NewNativeParser(engine llama.Engine, threads int, timeout time.Duration) *NativeParser`.
- Existing prompt rendering, JSON extraction, `DecodePlan`, `ValidateSnapshot`, and `ValidatePlan` remain policy code.

- [ ] **Step 1: Write failing native parser tests.**

Use a fake engine recording the request:

```go
func TestNativeParserUsesSchemaAndValidatesPlan(t *testing.T) {
	engine := &fakeEngine{output: validPlanJSON}
	parser := NewNativeParser(engine, 2, time.Second)
	plan, err := parser.Parse(context.Background(), "turn on demo", demoSnapshot())
	if err != nil { t.Fatal(err) }
	if plan.Status != "execute" { t.Fatalf("status = %q", plan.Status) }
	if engine.request.Schema == "" || engine.request.Prompt == "" { t.Fatal("native request missing schema or prompt") }
	if engine.request.Threads != 2 || engine.request.MaxTokens != 512 { t.Fatalf("request = %+v", engine.request) }
}

func TestNativeParserRejectsInventedCapability(t *testing.T) {
	parser := NewNativeParser(&fakeEngine{output: inventedPlanJSON}, 1, time.Second)
	if _, err := parser.Parse(context.Background(), "turn on demo", demoSnapshot()); err == nil { t.Fatal("accepted invented capability") }
}
```

- [ ] **Step 2: Run focused tests and verify failure.**

Run:

```text
go test ./internal/intent -run 'TestNativeParser'
```

Expected: FAIL because native parser does not exist.

- [ ] **Step 3: Implement parser and close behavior.**

Render the existing deterministic prompt, send `planJSONSchema`, `Threads`, and `MaxTokens: 512` to the engine, extract the first JSON object, decode, and validate. Respect timeout. `Close` delegates to engine once and remains safe on repeated calls. Never return conversational text.

- [ ] **Step 4: Wire runtime to flat model path.**

In `buildModelSet`, stop resolving `llama-cli` and never call `exec.Command` for intent. When `models.RoleIntent` is requested:

1. call `models.ResolveGGUFPath(cfg.Models.Root, cfg.Models.IntentModel)`;
2. call `llama.Open(ctx, resolvedPath, cfg.Models.Threads)`;
3. wrap it with `intent.NewNativeParser`;
4. include parser in `modelSet` so reload/close owns it.

Keep `requiredProfiles` for Sherpa roles. Intent no longer requires a role-directory profile. On error, include role and stable failure context but not model bytes or prompt text. Remove obsolete intent executable lookup.

- [ ] **Step 5: Add reload rollback coverage.**

Extend `runtime_test.go` with a fake native engine factory or injectable opener proving a failed replacement leaves the previous parser/engine active and closes only the failed candidate. Keep current successful swap behavior.

- [ ] **Step 6: Run focused tests and commit.**

Run:

```text
go test ./internal/intent ./cmd/peanut
go vet ./internal/intent ./cmd/peanut
```

Expected: PASS. Commit:

```text
git add app/internal/intent app/cmd/peanut/runtime.go app/cmd/peanut/runtime_test.go
git commit -m "feat: run FunctionGemma intent in process"
```

### Task 5: Documentation and final verification

**Files:**
- Modify: `README.md`
- Modify: `tools/llama.cpp.md`
- Modify: `docs/pi5-validation.md`

**Interfaces:**
- Documentation gives exact setup, pin verification, native Linux build, four-platform release matrix, flat GGUF placement, and stub/native tag behavior.

- [ ] **Step 1: Write documentation checks.**

Add or extend repository tests to assert documentation contains the exact SHA, flat `<model-name>.gguf` layout, `git submodule update --init --recursive third-party/llama.cpp`, `peanut_llama`, and no `llama-cli` inference command.

- [ ] **Step 2: Run checks and update docs.**

Replace old subprocess instructions in `tools/llama.cpp.md`. Document:

```text
./tools/setup-llama.sh
./tools/build-llama.sh
Models.Root=/var/lib/peanut/models
Models.IntentModel=functiongemma.gguf
```

Explain that Linux release builds use native CGO and Windows/macOS default builds use the unavailable stub. State that model bytes remain external.

- [ ] **Step 3: Run full local verification.**

From `app/`:

```text
go test ./...
go vet ./...
go build -o ../build/peanut ./cmd/peanut
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test ./... -run '^$'
```

If a local Linux native toolchain exists, run the pinned CMake build and:

```text
CGO_ENABLED=1 go test -tags peanut_llama ./internal/llama ./internal/intent
CGO_ENABLED=1 go build -tags peanut_llama -o ../build/peanut ./cmd/peanut
```

Record unavailable native-toolchain smoke tests; do not add fixtures or weaken production code.

- [ ] **Step 4: Inspect safety and commit docs-only fixes.**

Search changed files for `exec.Command`, `llama-cli`, model bytes, raw tensor APIs, secrets, and environment-based runtime configuration. Confirm subprocess inference is gone. Commit only required fixes:

```text
git add README.md tools/llama.cpp.md docs/pi5-validation.md
git commit -m "docs: document native llama build and model placement"
```

