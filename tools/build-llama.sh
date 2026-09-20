#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
pin=a894dae939d426954ce54bb604824f1ae918a0c5
target=${1:-$(uname -m)}
case "$target" in
  x86_64) target=amd64 ;;
  aarch64) target=arm64 ;;
  amd64|arm64) ;;
  *) echo "unsupported target: $target" >&2; exit 2 ;;
esac

source="$root/third-party/llama.cpp"
build="$root/build/llama/$target"
prefix="$build/prefix"
actual=$(git -C "$source" rev-parse HEAD)
test "$actual" = "$pin"

cmake_args=(
  -S "$source"
  -B "$build"
  -DCMAKE_BUILD_TYPE=Release
  -DCMAKE_INSTALL_PREFIX="$prefix"
  -DBUILD_SHARED_LIBS=OFF
  -DGGML_NATIVE=OFF
  -DGGML_CPU=ON
  -DGGML_CUDA=OFF
  -DGGML_HIP=OFF
  -DGGML_METAL=OFF
  -DGGML_OPENCL=OFF
  -DGGML_VULKAN=OFF
  -DGGML_SYCL=OFF
  -DGGML_RPC=OFF
  -DGGML_OPENMP=OFF
  -DLLAMA_BUILD_COMMON=OFF
  -DLLAMA_BUILD_EXAMPLES=OFF
  -DLLAMA_BUILD_TESTS=OFF
  -DLLAMA_BUILD_TOOLS=OFF
  -DLLAMA_BUILD_SERVER=OFF
  -DLLAMA_BUILD_APP=OFF
  -DLLAMA_BUILD_UI=OFF
  -DLLAMA_TOOLS_INSTALL=OFF
  -DLLAMA_TESTS_INSTALL=OFF
)
if [[ -n "${CMAKE_TOOLCHAIN_FILE:-}" ]]; then
  cmake_args+=("-DCMAKE_TOOLCHAIN_FILE=$CMAKE_TOOLCHAIN_FILE")
fi

cmake "${cmake_args[@]}"
cmake --build "$build" --config Release --parallel
cmake --install "$build" --config Release

echo "CGO_CFLAGS=-I$prefix/include"
echo "CGO_LDFLAGS=-L$prefix/lib"
echo "go build -tags peanut_llama ./cmd/peanut"
