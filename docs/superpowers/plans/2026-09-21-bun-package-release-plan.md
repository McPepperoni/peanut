# Peanut Bun Package Release Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement task-by-task.

**Goal:** Add Bun package metadata, cross-platform local checks/pre-commit setup, and version-driven GitHub releases for Peanut.

**Architecture:** Root `package.json` owns developer commands while Go remains production runtime. A small Bun validator checks version changes and local/remote tag collisions before release builds. Existing native Linux llama builds and non-native desktop stubs remain; release publishing attaches only artifacts produced by the matrix build.

**Global constraints:** Bun `1.4.2`; version starts at `0.1.0`; no model weights; no Node in production; no Docker Compose; targets Linux amd64, Linux arm64, Windows amd64, macOS arm64; preserve existing native/stub release behavior; fail before build on unchanged/invalid version or existing `v<version>` tag.

### Task 1: Bun metadata, hook, scripts, and validator

**Files:** `package.json`, `bun.lock`, `.githooks/pre-commit`, `scripts/build.ts`, `scripts/release-version.ts`, `scripts/release-version.test.ts`.

- Add `version`, `packageManager`, Go test/vet/build scripts, Bun release test, `ci`, `release:validate`, and `prepare` that sets `core.hooksPath` to `.githooks`.
- Keep pre-commit bounded to `bun run test && bun run vet`; no model or release work.
- Validator uses direct Bun subprocess arguments, reads parent `package.json`, accepts initial missing parent version, rejects invalid/unchanged versions and existing local/remote tags, and writes `GITHUB_OUTPUT`.
- Tests cover valid/invalid/unchanged versions and optional suffixes.
- Run `bun install --frozen-lockfile`, release tests, and `bun run ci`; commit only Task 1 files.

### Task 2: Version-driven release workflow

**Files:** `.github/workflows/release.yml`, `README.md` or `docs/releasing.md`.

- Trigger only on pushes to `main` where `package.json` changed.
- Validate before any matrix build; then preserve existing Linux native llama setup and desktop stub builds.
- Upload uniquely named matrix artifacts with `upload-artifact@v4`; publish downloads those exact artifacts, checks expected non-empty files, emits `SHA256SUMS`, and runs `gh release create` with `GITHUB_TOKEN`.
- Document version bump, hook, local commands, four artifact names, tag format, duplicate-tag failure, and external models.
- Run local verification plus YAML/static review; commit only Task 2 files.
