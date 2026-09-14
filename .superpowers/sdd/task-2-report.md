# Task 2 report: Configuration API, model reload, OpenAPI, and Scalar

## Implementation

- Changed `api.NewServer` to accept `ModelReloader` and added `Registry.Reload` as the API-facing model refresh operation.
- Added `GET /api/v1/models`; each GET reloads profiles and returns `profiles`, `active`, and validation `errors`. Reload failures return JSON 502; unsupported methods return 405 with `Allow: GET`.
- Changed authentication middleware to load SQLite configuration for every request. Bearer authentication is enforced only while `AllowLAN` is true and pairing tokens are compared with `subtle.ConstantTimeCompare`.
- Added embedded OpenAPI 3.1 JSON, local Scalar HTML, and vendored Scalar 1.68.0 browser bundle. Docs load `/api/v1/openapi.json`, disable telemetry, and contain no remote script source.
- Wired API startup to a registry using configured `Models.Root` and the SQLite model store.

## TDD evidence

RED:

```text
$env:GOCACHE=(Join-Path (Get-Location) '.gocache'); go test ./internal/api
# peanut/internal/api [peanut/internal/api.test]
internal\api\server_test.go:29:31: too many arguments in call to NewServer
    have (*sqlite.DB, nil, nil)
    want (*sqlite.DB, Refresher)
FAIL peanut/internal/api [build failed]
```

GREEN:

```text
$env:GOCACHE=(Join-Path (Get-Location) '.gocache'); go test ./internal/api
ok peanut/internal/api 1.000s
```

Focused verification:

```text
go test ./internal/api ./internal/models ./cmd/peanut
ok peanut/internal/api 1.271s
ok peanut/internal/models 0.354s
ok peanut/cmd/peanut 1.352s
```

Full verification:

```text
go test ./...
PASS: all packages

go vet ./...
PASS: exit 0, no output
```

## Files

- `app/internal/api/server.go`
- `app/internal/api/server_test.go`
- `app/internal/api/openapi.json`
- `app/internal/api/docs.html`
- `app/internal/api/scalar.js`
- `app/internal/models/registry.go`
- `app/cmd/peanut/main.go` (necessary API bootstrap wiring)
- `app/cmd/peanut/main_test.go` (live swap boundary regression)
- `app/cmd/peanut/runtime.go` (shared registry owner wiring)
- `.superpowers/sdd/task-2-report.md`

User-owned untracked `models/` was not read or modified. Generated Go cache directories remain untracked and unstaged.

## Self-review

- Confirmed every request loads current SQLite config before authorization; configuration-load failures return JSON 500.
- Confirmed LAN requests require bearer credentials, localhost defaults remain unauthenticated, and existing secret-redaction tests pass.
- Confirmed model GET invokes reload exactly once, serializes all three required fields, returns 502 on failure, and does not mutate the previous active snapshot.
- Confirmed OpenAPI parses as JSON and covers config, pairing, provider refresh, model reload, OpenAPI, and docs routes.
- Confirmed docs reference only local `/scalar.js` and `/api/v1/openapi.json`; vendored bundle SHA-256 is `6A1407DB14F57F7BE9C98464B6D7E8899BA417C63B0D81363DB2EFB3E1022E1F`.
- Confirmed no new Go dependency, unrelated refactor, CDN runtime dependency, or tracked temp/cache artifact.

## Review fixes

- LAN-bound servers now reject authenticated config PUTs that disable `AllowLAN` or move the bound address to loopback. The request returns JSON 400 with `restart required`, and the rejected config is not persisted.
- LAN authentication fails closed with JSON 500 `authentication unavailable` when the stored pairing token is empty; an empty bearer is never accepted.
- API model reload now uses the same `reloadableModels` owner boundary as configured runtime startup. `serveAPI` no longer creates a registry with a nil swap callback.
- Added `TestModelsAPIReloadReachesRuntimeSwapBoundary` covering GET `/api/v1/models` through registry reload into the runtime swap callback.

## Verification evidence

Focused swap-boundary test:

```text
cd app
$env:GOCACHE=(Join-Path (Get-Location) '..\\.gocache-task3')
go test ./cmd/peanut -run TestModelsAPIReloadReachesRuntimeSwapBoundary -count=1 -v
=== RUN   TestModelsAPIReloadReachesRuntimeSwapBoundary
--- PASS: TestModelsAPIReloadReachesRuntimeSwapBoundary (0.11s)
PASS
ok peanut/cmd/peanut 1.570s
```

Focused changed-package tests:

```text
cd app
$env:GOCACHE=(Join-Path (Get-Location) '..\\.gocache-task3')
go test ./internal/api ./internal/models ./cmd/peanut -count=1
ok peanut/internal/api 1.620s
ok peanut/internal/models 0.557s
ok peanut/cmd/peanut 1.850s
```

Full verification:

```text
cd app
$env:GOCACHE=(Join-Path (Get-Location) '..\\.gocache-task3')
go test ./... -count=1
PASS: all packages
go vet ./...
PASS: exit 0, no output
```

Concerns: `peanut run` shares the reloadable owner between API and coordinator. `peanut api` remains an API-only process by design; it now performs the owner swap on reload, but has no coordinator consumer in that process.

## Review fix: immutable API authentication boundary

- `boundLAN` now starts true when the startup address is non-loopback or `AllowLAN` is true.
- `SetBoundAddress` refreshes the bind boundary from the address returned by `Address(ctx)` before `ListenAndServe` in both API entry points.
- Authentication now requires a bearer token when `boundLAN` or current `AllowLAN` is true; empty tokens still return JSON 500.
- Added an external SQLite downgrade regression proving unauthenticated requests remain rejected after `AllowLAN` is changed to false outside the API. Existing PUT downgrade rejection remains covered.
- Removed the contradictory OpenAPI global bearer-security declaration; the document-level description now states the conditional LAN behavior.

RED:

```text
$env:GOCACHE=(Join-Path (Get-Location) '.gocache-task2-red3'); go test ./internal/api -run TestServerKeepsAuthenticationAfterExternalLANDowngrade -count=1 -v
=== RUN   TestServerKeepsAuthenticationAfterExternalLANDowngrade
    server_test.go:119: downgraded request status = 200, body = {"home_assistant":{"url":"http://127.0.0.1:8123","token":"","timeout_seconds":10},"api":{"address":"0.0.0.0:8080","allow_lan":false,"pairing_token":"[REDACTED]"}}
--- FAIL: TestServerKeepsAuthenticationAfterExternalLANDowngrade (0.02s)
FAIL
FAIL	peanut/internal/api	1.003s
FAIL
```

GREEN:

```text
$env:GOCACHE=(Join-Path (Get-Location) '.gocache-task2-red3'); go test ./internal/api ./cmd/peanut -run 'TestServerKeepsAuthenticationAfterExternalLANDowngrade|TestServerRejectsLANAuthenticationDowngradeUntilRestart|TestServerFailsClosedWhenLANPairingTokenIsEmpty|TestModelsAPIReloadReachesRuntimeSwapBoundary|TestRuntimeRegistryReloadInvokesLiveSwapBoundary' -count=1 -v
=== RUN   TestServerRejectsLANAuthenticationDowngradeUntilRestart
--- PASS: TestServerRejectsLANAuthenticationDowngradeUntilRestart (0.03s)
=== RUN   TestServerKeepsAuthenticationAfterExternalLANDowngrade
--- PASS: TestServerKeepsAuthenticationAfterExternalLANDowngrade (0.02s)
=== RUN   TestServerFailsClosedWhenLANPairingTokenIsEmpty
--- PASS: TestServerFailsClosedWhenLANPairingTokenIsEmpty (0.01s)
PASS
ok  peanut/internal/api 0.968s
=== RUN   TestModelsAPIReloadReachesRuntimeSwapBoundary
--- PASS: TestModelsAPIReloadReachesRuntimeSwapBoundary (0.03s)
=== RUN   TestRuntimeRegistryReloadInvokesLiveSwapBoundary
--- PASS: TestRuntimeRegistryReloadInvokesLiveSwapBoundary (0.01s)
PASS
ok  peanut/cmd/peanut 1.277s
```

Required verification:

```text
cd app
$env:GOCACHE=(Join-Path (Get-Location) '.gocache-task2-red3'); go test ./... -count=1
PASS: all packages; exit 0

$env:GOCACHE=(Join-Path (Get-Location) '.gocache-task2-red3'); go vet ./...
PASS: exit 0, no output
```
