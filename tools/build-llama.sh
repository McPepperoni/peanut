#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
pin=a894dae939d426954ce54bb604824f1ae918a0c5
normalize_arch() {
  case "$1" in
    x86_64|amd64) printf 'amd64' ;;
    aarch64|arm64) printf 'arm64' ;;
    *) return 1 ;;
  esac
}

host=$(normalize_arch "$(uname -m)") || { echo "unsupported host architecture: $(uname -m)" >&2; exit 2; }
target=$(normalize_arch "${1:-$host}") || { echo "unsupported target: ${1:-$host}" >&2; exit 2; }
toolchain=${CMAKE_TOOLCHAIN_FILE:-}
if [[ -n "$toolchain" && ! -f "$toolchain" ]]; then
  echo "cross toolchain file not found: $toolchain" >&2
  exit 2
fi
if [[ "$target" != "$host" && -z "$toolchain" ]]; then
  echo "target $target differs from host $host; set CMAKE_TOOLCHAIN_FILE for an explicit cross build" >&2
  exit 2
fi

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
  -DLLAMA_BUILD_COMMON=ON
  -DLLAMA_SUBPROCESS=OFF
  -DLLAMA_OPENSSL=OFF
  -DLLAMA_BUILD_EXAMPLES=OFF
  -DLLAMA_BUILD_TESTS=OFF
  -DLLAMA_BUILD_TOOLS=OFF
  -DLLAMA_BUILD_SERVER=OFF
  -DLLAMA_BUILD_APP=OFF
  -DLLAMA_BUILD_UI=OFF
  -DLLAMA_TOOLS_INSTALL=OFF
  -DLLAMA_TESTS_INSTALL=OFF
)
if [[ -n "$toolchain" ]]; then
  cmake_args+=("-DCMAKE_TOOLCHAIN_FILE=$toolchain")
fi

cmake "${cmake_args[@]}"
cmake --build "$build" --config Release --parallel
cmake --install "$build" --config Release
mkdir -p "$prefix/lib"
find "$build" -path "$prefix" -prune -o -type f -name '*.a' -exec cp {} "$prefix/lib/" \;
test -f "$prefix/lib/libllama.a"
test -f "$prefix/lib/libllama-common.a"

echo "CGO_CFLAGS=-I$prefix/include"
echo "CGO_CXXFLAGS=-I$source/common"
echo "CGO_LDFLAGS=$prefix/lib/libllama-common.a $prefix/lib/libllama-common-base.a $prefix/lib/libllama.a $prefix/lib/libggml.a $prefix/lib/libggml-cpu.a $prefix/lib/libggml-base.a -lstdc++ -lm -ldl -pthread"
echo "go build -tags peanut_llama ./cmd/peanut"
