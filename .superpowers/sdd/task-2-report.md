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
