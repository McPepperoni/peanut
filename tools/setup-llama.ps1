$ErrorActionPreference = 'Stop'

git submodule update --init --recursive third-party/llama.cpp
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
