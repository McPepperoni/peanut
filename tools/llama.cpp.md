# llama.cpp runtime

Peanut pins llama.cpp as a submodule at:

```text
a894dae939d426954ce54bb604824f1ae918a0c5
```

Setup fetches only that gitlink. Run from the repository root:

```sh
./tools/setup-llama.sh
git -C third-party/llama.cpp rev-parse HEAD
```

The setup command is:

```sh
git submodule update --init --recursive third-party/llama.cpp
```

GGUF model files stay outside Git and SQLite. Store the intent model as a flat
file under the configured model root, for example:

```text
Models.Root=/var/lib/peanut/models
Models.IntentModel=functiongemma.gguf
```

Native Linux builds use the `peanut_llama` CGO tag and static CPU-only llama.cpp
libraries. Windows amd64 and macOS arm64 default builds use the explicit
unavailable stub. Building llama.cpp is separate from submodule setup and must
not download model files.

Build the native Linux archive from the repository root after setup:

```sh
./tools/build-llama.sh amd64
```

The helper verifies the pinned commit, disables accelerator backends and
examples/tests/tools, and installs static archives at
`build/llama/amd64/prefix`. It prints the `CGO_CFLAGS`, `CGO_LDFLAGS`, and
`go build -tags peanut_llama` command for the selected target. It never fetches
source, downloads models, or runs inference subprocesses.
