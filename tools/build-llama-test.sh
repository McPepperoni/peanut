#!/usr/bin/env bash
set -euo pipefail

script=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/build-llama.sh
archive_copy=$(grep -F 'find "$build"' "$script")

case "$archive_copy" in
  *'-path "$prefix" -prune -o'*) ;;
  *)
    echo 'archive copy must prune the install prefix from its source scope' >&2
    exit 1
    ;;
esac

case "$archive_copy" in
  *'-type f -name '\''*.a'\'' -exec cp {} "$prefix/lib/"'*) ;;
  *)
    echo 'archive copy command no longer copies static archives into the prefix' >&2
    exit 1
    ;;
esac
