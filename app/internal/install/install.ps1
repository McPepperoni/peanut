$ErrorActionPreference = 'Stop'

$Image = 'ghcr.io/home-assistant/home-assistant:stable'
$Container = 'homeassistant'
$Volume = 'home-assistant-config'
$Api = 'http://127.0.0.1:8080/api/v1/config'
$AppDir = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$RootDir = (Resolve-Path (Join-Path $AppDir '..')).Path
$Binary = Join-Path $RootDir 'build\peanut'

if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'go is required' }
New-Item -ItemType Directory -Force (Join-Path $RootDir 'build') | Out-Null
Push-Location $AppDir
try {
    & go build -o $Binary ./cmd/peanut
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
} finally {
    Pop-Location
}

$HasHA = Read-Host 'Do you already have Home Assistant? [y/N]'
if ($HasHA -match '^(?i:y|yes)$') {
    $HAUrl = Read-Host 'Home Assistant URL'
} else {
    $Runtime = Get-Command docker -ErrorAction SilentlyContinue
    if (-not $Runtime) { $Runtime = Get-Command podman -ErrorAction SilentlyContinue }
    if (-not $Runtime) { throw 'Docker or Podman is required to run Home Assistant Container' }

    & $Runtime.Source pull $Image
    if ($LASTEXITCODE -ne 0) { throw 'Home Assistant image pull failed' }
    & $Runtime.Source volume create $Volume | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Home Assistant volume creation failed' }
    & $Runtime.Source container inspect $Container 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) {
        & $Runtime.Source start $Container | Out-Null
    } else {
        & $Runtime.Source run -d --name $Container --restart unless-stopped --privileged --network host -v "${Volume}:/config" $Image | Out-Null
    }
    if ($LASTEXITCODE -ne 0) { throw 'Home Assistant Container start failed' }
    $HAUrl = 'http://127.0.0.1:8123'
    Write-Host "Home Assistant Container started at $HAUrl. Complete onboarding before creating a long-lived access token."
}

if ($HAUrl -notmatch '^https?://[^\s"\\]+$') { throw 'Invalid Home Assistant URL' }
$SecureToken = Read-Host 'Home Assistant long-lived access token' -AsSecureString
$TokenPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureToken)
try {
    $HAToken = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($TokenPointer)
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($TokenPointer)
}
if ($HAToken -notmatch '^[A-Za-z0-9._-]+$') { throw 'Invalid Home Assistant token' }
$Body = @{ home_assistant = @{ url = $HAUrl; token = $HAToken } } | ConvertTo-Json -Compress
try {
    Invoke-RestMethod -Method Put -Uri $Api -ContentType 'application/json' -Body $Body | Out-Null
} catch {
    throw "Could not reach Peanut's native config API at $Api; start Peanut and rerun this installer"
}
Write-Host "Peanut configured for external Home Assistant. Native binary: $Binary"
