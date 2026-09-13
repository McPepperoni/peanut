#!/usr/bin/env sh
set -eu

IMAGE=ghcr.io/home-assistant/home-assistant:stable
CONTAINER=homeassistant
VOLUME=home-assistant-config
API=http://127.0.0.1:8080/api/v1/config
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
APP_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
ROOT_DIR=$(CDPATH= cd -- "$APP_DIR/.." && pwd)

command -v go >/dev/null 2>&1 || { echo "go is required" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
mkdir -p "$ROOT_DIR/build"
(cd "$APP_DIR" && go build -o "$ROOT_DIR/build/peanut" ./cmd/peanut)

printf "Do you already have Home Assistant? [y/N] "
IFS= read -r HAS_HA
case "$HAS_HA" in
  y|Y|yes|YES)
    printf "Home Assistant URL: "
    IFS= read -r HA_URL
    ;;
  *)
    if command -v docker >/dev/null 2>&1; then
      RUNTIME=docker
    elif command -v podman >/dev/null 2>&1; then
      RUNTIME=podman
    else
      echo "Docker or Podman is required to run Home Assistant Container" >&2
      exit 1
    fi
    "$RUNTIME" pull "$IMAGE"
    "$RUNTIME" volume create "$VOLUME" >/dev/null
    if "$RUNTIME" container inspect "$CONTAINER" >/dev/null 2>&1; then
      "$RUNTIME" start "$CONTAINER" >/dev/null
    else
      "$RUNTIME" run -d --name "$CONTAINER" --restart unless-stopped --privileged \
        --network host -v "$VOLUME:/config" "$IMAGE" >/dev/null
    fi
    HA_URL=http://127.0.0.1:8123
    echo "Home Assistant Container started at $HA_URL. Complete onboarding before creating a long-lived access token."
    ;;
esac

printf "Home Assistant long-lived access token: "
TTY=false
if [ -t 0 ]; then
  TTY=true
  stty -echo
  trap 'stty echo 2>/dev/null || true' EXIT
fi
IFS= read -r HA_TOKEN
if [ "$TTY" = true ]; then
  stty echo
  trap - EXIT
  printf "\n"
fi

printf '%s' "$HA_URL" | grep -Eq '^https?://[^[:space:]"\\]+$' || { echo "Invalid Home Assistant URL" >&2; exit 1; }
printf '%s' "$HA_TOKEN" | grep -Eq '^[A-Za-z0-9._-]+$' || { echo "Invalid Home Assistant token" >&2; exit 1; }
PAYLOAD=$(printf '{"home_assistant":{"url":"%s","token":"%s"}}' "$HA_URL" "$HA_TOKEN")
printf '%s' "$PAYLOAD" | curl --fail --silent --show-error -X PUT -H 'Content-Type: application/json' --data-binary @- "$API" >/dev/null || {
  echo "Could not reach Peanut's native config API at $API; start Peanut and rerun this installer" >&2
  exit 1
}
echo "Peanut configured for external Home Assistant. Native binary: $ROOT_DIR/build/peanut"
