# Home Assistant provider

Peanut treats Home Assistant as an external provider. It can target any reachable Home Assistant instance during development, including the dev-only checkout at `dev/home-assistant-core` or an existing box on the network.

Production setup uses the official Home Assistant Container image when the user does not already have Home Assistant. The installer does not clone Home Assistant source. Peanut remains a native process and stores the configured HA URL and bearer token in its SQLite database.

Default provider URL: `http://127.0.0.1:8123`.

Create a Home Assistant long-lived access token in the HA profile page, then configure Peanut through its local API:

```text
PUT /api/v1/config
Content-Type: application/json

{"home_assistant":{"url":"http://127.0.0.1:8123","token":"..."}}
```

Refresh discovered media players with:

```text
POST /api/v1/config/refresh
```

The provider discovers `media_player.*` entities and exposes generic `music.play`. Validated requests map to Home Assistant's `media_player.play_media` service. YouTube selection remains Home Assistant integration behavior; Peanut does not call YouTube directly.

Configuration API binds to `127.0.0.1` by default. LAN binding requires explicit opt-in and a generated pairing token. API responses redact HA and pairing secrets.
