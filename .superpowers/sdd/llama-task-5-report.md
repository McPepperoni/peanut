# Task 5 report: llama.cpp documentation and final verification

## Result

Updated the runtime documentation for the pinned llama.cpp milestone:

- README and llama.cpp tooling docs show exact submodule setup, pin verification, flat external GGUF placement, native Linux build, and the four-platform release matrix.
- Pi 5 validation docs distinguish native Linux CGO artifacts from the Windows/macOS unavailable stub and document tag-triggered release behavior.
- Model docs remove the obsolete intent profile and `llama-cli` subprocess instructions; intent uses `Models.IntentModel` as a flat GGUF filename.
- Added a repository test that locks the pin, setup command, flat layout, build tag, platform matrix, and absence of `llama-cli` documentation claims.
- No model weights, frontend/API/TTS/voice implementation, Docker Compose setup, or Node.js production runtime was added.

## TDD evidence

The initial documentation-contract run failed because the current docs lacked the required flat GGUF path:

```text
go test ./internal/install -run 'TestLlamaDocumentationContract'
FAIL: documentation does not contain "/var/lib/peanut/models/<model-name>.gguf"
```

After the docs update:

```text
go test ./internal/install -run 'Test(LlamaSetupScripts|LlamaDocumentationContract|ProductionInstallersKeepHomeAssistantExternal)'
ok   peanut/internal/install
```

## Verification

Passed:

```text
go test ./internal/llama -count=1 -v -timeout 30s
go vet ./internal/install
git diff --check
```

The full suite and command package build did not complete in this managed
Windows host within the requested time window. The full test run emitted only
`ok peanut/assets` before being stopped; the command-package run emitted no
test output before being stopped. The production build was also blocked before
compilation because the host denied creation of the required root `build/`
directory. Native Linux CGO smoke was not claimed: this host has no verified
Linux/CMake llama archive toolchain or external GGUF fixture.
