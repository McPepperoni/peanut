# Releasing Peanut

Peanut releases are version-driven. A push to `main` triggers the release workflow only when the root `package.json` changes.

## Release workflow

1. Bump `version` in the root `package.json` and commit the change.
2. Run the local checks and pre-commit commands below.
3. Push the commit to `main`.

The workflow validates the version before any build. It rejects invalid or unchanged versions and fails if the tag already exists locally or remotely. A valid version such as `0.1.1` produces the tag `v0.1.1`.

The four release artifacts are:

- `peanut-linux-amd64` — native Linux build with CGO and llama.cpp.
- `peanut-linux-arm64` — native Linux build with CGO and llama.cpp.
- `peanut-windows-amd64.exe` — Windows stub build.
- `peanut-darwin-arm64` — macOS stub build.

The publish job downloads those exact build artifacts, verifies that every binary is non-empty, creates `SHA256SUMS`, and attaches the files to the GitHub release. It does not rebuild binaries or include model files.

If `v<version>` already exists, validation fails before the build jobs. Bump `package.json` to a new version before retrying; the workflow does not overwrite releases.

## Local commands

Install the pinned Bun toolchain dependencies:

```sh
bun install --frozen-lockfile
```

Run the same checks used before a release:

```sh
bun run test
bun run vet
bun run release
bun run build
bun run ci
```

`bun run release:validate` checks the version change and tag availability. It is intended for the release workflow and can fail when run locally without a new version or when the tag already exists.

The pre-commit hook runs:

```sh
bun run test && bun run vet
```

## External models

Model weights are not committed, built into artifacts, or uploaded to GitHub Releases. Install and configure models separately on the target device using the external model guidance in [`tools/models.md`](../tools/models.md).
