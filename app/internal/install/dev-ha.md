# Home Assistant development setup

Peanut always treats Home Assistant as an external service.

For development, use any reachable Home Assistant instance. A source checkout is also allowed at `dev/home-assistant-core/`; that directory is ignored by Git and is never a production deployment method.

Production uses an existing Home Assistant deployment or the official `ghcr.io/home-assistant/home-assistant:stable` container created by `install.sh` or `install.ps1`. The installers never clone Home Assistant source. Peanut itself remains a native binary.

After Peanut's local API is running, the installer writes the Home Assistant URL and long-lived token through `PUT /api/v1/config`. Peanut validates the values and persists them in SQLite. Runtime configuration is not read from environment variables.
