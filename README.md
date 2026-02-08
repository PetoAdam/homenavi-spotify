# Spotify Integration (Homenavi)

[![Build](https://github.com/PetoAdam/homenavi-spotify/actions/workflows/build.yml/badge.svg)](https://github.com/PetoAdam/homenavi-spotify/actions/workflows/build.yml)
[![Verify](https://github.com/PetoAdam/homenavi-spotify/actions/workflows/verify.yml/badge.svg)](https://github.com/PetoAdam/homenavi-spotify/actions/workflows/verify.yml)
[![Release](https://github.com/PetoAdam/homenavi-spotify/actions/workflows/release.yml/badge.svg)](https://github.com/PetoAdam/homenavi-spotify/actions/workflows/release.yml)

A full Spotify player integration with a dedicated tab and a dashboard widget. Supports:

- Play / pause
- Seek
- Volume
- Skip forward/backward
- Shuffle + loop
- Device selection
- Queue rendering
- Search (tab) with Play Now + Add to Queue

## Environment variables

Copy the example env file and fill in your secrets:

```bash
cp .env.example .env
```

Set the following values in the `.env` file:

- `SPOTIFY_CLIENT_ID`
- `SPOTIFY_CLIENT_SECRET`
- `SPOTIFY_REFRESH_TOKEN`

If you prefer central management, use the Admin → Integrations page to set these secrets (declared in the manifest). Values are write-only and cannot be read back. The integration exposes a write-only admin endpoint at `GET/PUT /api/admin/secrets` (admin-only; values are never returned). For admin access, mount the Homenavi JWT public key and set `JWT_PUBLIC_KEY_PATH` in the container.

The integration reads secrets from `INTEGRATION_SECRETS_PATH` (or `INTEGRATIONS_SECRETS_PATH` for compatibility) if environment variables are not set. By default it uses `config/integration.secrets.json` in the repo/container.

## How to get the Spotify credentials

1) Create a Spotify developer app at https://developer.spotify.com/dashboard
2) Copy the **Client ID** and **Client Secret** from the app settings.
3) Add a Redirect URI (e.g. `http://localhost:8888/callback`) in the app settings.
4) Run an OAuth authorization flow (with `user-read-playback-state`, `user-modify-playback-state`, and `user-read-currently-playing` scopes) to obtain a **refresh token**.
5) Set the environment variables above.

## Local dev (frontend)

```bash
cd src/frontend
npm install
npm run dev:tab
# in another terminal
npm run dev:widget
```

UI preview during dev:

- Tab dev server: http://localhost:10000/tab.html
- Widget dev server: http://localhost:10001/widget.html

If the port changed (free-port auto-pick), use the exact URL printed by the dev server.

## Build + run

```bash
cd src/frontend
npm install
npm run build

cd ../..
go run ./src/backend/cmd/integration
```

## Marketplace metadata

Marketplace-specific metadata and assets live in:

- `marketplace/metadata.json`
- `marketplace/assets/`

Update the icon and images there to control how the integration appears in the marketplace.

## Local build + run with Homenavi stack

Use this to test the integration through integration-proxy with local assets:

```bash
cd src/frontend
npm install
npm run build

cd ../..
docker build -t homenavi-spotify:local .

docker run --rm -d \
  --name spotify \
  --network homenavi-network \
  -v /home/adam/Projects/homenavi/integrations/secrets/spotify.secrets.json:/app/config/integration.secrets.json \
  -e INTEGRATION_SECRETS_PATH=/app/config/integration.secrets.json \
  -v /home/adam/Projects/homenavi/keys/jwt_public.pem:/app/keys/jwt_public.pem:ro \
  -e JWT_PUBLIC_KEY_PATH=/app/keys/jwt_public.pem \
  homenavi-spotify:local
```

Ensure the Homenavi integrations list includes:

```yaml
integrations:
  - id: spotify
    upstream: http://spotify:8099
```

Then use Admin → Integrations → “Refresh integrations” to reload the proxy registry.

## Docker Compose (integration-proxy install)

This uses the production image and matches how the marketplace installs it:

```bash
INTEGRATIONS_ROOT=/path/to/homenavi \
  docker compose -f compose/docker-compose.integration.yml up -d
```

Set `HN_VERSION=vX.Y.Z` to pin a release tag.

## Docker Compose (local dev image)

Use this to build and run your local image against a running Homenavi stack:

```bash
HOMENAVI_ROOT=/path/to/homenavi \
  docker compose -f compose/docker-compose.dev.yml up --build
```

## Docker

From the repo root:

```bash
docker build -t homenavi-spotify:local .
```

Run the container on the Homenavi network (using the repo file path):

```bash
docker run --rm \
  --name spotify \
  --network homenavi_homenavi-network \
  -v $(pwd)/integrations/spotify/config/integration.secrets.json:/app/config/integration.secrets.json \
  -e INTEGRATION_SECRETS_PATH=/app/config/integration.secrets.json \
  -v $(pwd)/keys/jwt_public.pem:/app/keys/jwt_public.pem:ro \
  -e JWT_PUBLIC_KEY_PATH=/app/keys/jwt_public.pem \
  homenavi-spotify:local
```

If you don’t need the admin secrets endpoint, omit the JWT mount/env lines.

## Integration proxy installation (recommended)

1) Build or pull the image:

```bash
docker build -t ghcr.io/petoadam/homenavi-spotify:latest .
```

2) Run the container on the Homenavi network:

```bash
docker run --rm \
  --name spotify \
  --network homenavi_homenavi-network \
  -v $(pwd)/config/integration.secrets.json:/app/config/integration.secrets.json \
  -e INTEGRATION_SECRETS_PATH=/app/config/integration.secrets.json \
  -v $(pwd)/keys/jwt_public.pem:/app/keys/jwt_public.pem:ro \
  -e JWT_PUBLIC_KEY_PATH=/app/keys/jwt_public.pem \
  ghcr.io/petoadam/homenavi-spotify:latest
```

3) Register the integration in the Homenavi config:

```yaml
integrations:
  - id: spotify
    upstream: http://spotify:8099
```

After updating the installed integrations list, use the Admin → Integrations page and click “Refresh integrations” to reload the proxy registry.

## Helm installation (coming soon)

Planned chart values (subject to change):

```yaml
image:
  repository: ghcr.io/petoadam/homenavi-spotify
  tag: latest

env:
  INTEGRATION_SECRETS_PATH: /app/config/integration.secrets.json
  JWT_PUBLIC_KEY_PATH: /app/keys/jwt_public.pem

secrets:
  spotifyClientId: "<set-via-secret>"
  spotifyClientSecret: "<set-via-secret>"
  spotifyRefreshToken: "<set-via-secret>"

integrations:
  - id: spotify
    upstream: http://spotify:8099
```

The chart will create a Deployment + Service and add an `installed.yaml` snippet for integration‑proxy. JWT public key mounting will be optional for deployments that do not use the admin secrets endpoint.
