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

## Release security gates

- `verify.yml` is the main gate (manifest/structure checks, tests, `go vet`, `gosec`, Docker build, and Trivy scan).
- `release.yml` runs `verify.yml` as a required stage, so publish only proceeds after verification passes.
- The shared `PetoAdam/homenavi/.github/actions/integration-release@main` action also enforces central verification (`integration-verify` + `go vet` + `gosec`) during release.
- Release emits SBOM + provenance and signs published image digests keylessly with Cosign.

## Configuration model

This integration now follows the standard Homenavi setup flow:

- Admins open the Spotify `Setup` page from the integrations sidebar.
- They save the Spotify app `client_id` and `client_secret`.
- They click `Connect Spotify` and finish the OAuth login in the browser.
- The backend stores the resulting refresh token in `integration.setup.json` and rotates it when Spotify returns a new one.

Environment variables are still supported for local bootstrap and emergency overrides.

Copy the example env file if you want to run it standalone:

```bash
cp .env.example .env
```

Supported variables:

- `SPOTIFY_CLIENT_ID`
- `SPOTIFY_CLIENT_SECRET`
- `SPOTIFY_REFRESH_TOKEN` optional legacy/manual override

For central management, mount the Homenavi JWT public key and set `JWT_PUBLIC_KEY_PATH`. The integration exposes:

- `GET/PUT /api/admin/setup`
- `GET /api/admin/auth/status`
- `POST /api/admin/auth/login`
- `GET /api/admin/auth/callback`
- `POST /api/admin/auth/disconnect`

The integration reads setup from `INTEGRATION_SETUP_PATH` (or `INTEGRATIONS_SETUP_PATH`) and stores it in `config/integration.setup.json` by default. Legacy `INTEGRATION_SECRETS_PATH` input is still accepted as a fallback for old deployments.

## How to get the Spotify credentials

1) Create a Spotify developer app at https://developer.spotify.com/dashboard
2) Copy the **Client ID** and **Client Secret** from the app settings.
3) Add the integration callback URL shown on the Setup page as a Redirect URI in the Spotify app settings.
4) Save setup in Homenavi and click `Connect Spotify`.
5) Complete the Spotify login once in the browser.

## Refresh token lifecycle

Spotify refresh tokens can expire after roughly 180 days depending on provider policy.

- This integration now stores refresh tokens in setup storage, not in static secrets.
- When Spotify returns a rotated refresh token during refresh, the integration persists the new token automatically.
- The setup status exposes `refresh_token_expires_at` so Homenavi can warn admins before expiry.

If Spotify enforces a hard 180-day re-authentication window, there is no legitimate way to bypass it completely. The best operational model is:

- use a dedicated household Spotify account for the integration
- rotate and persist any new refresh token automatically
- surface expiry in admin status and alert before the deadline
- reconnect once when Spotify requires it

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

Marketplace installs also need a Compose definition. Keep the production Compose file in
`compose/docker-compose.integration.yml` and reference it in `marketplace/metadata.json`
as `compose_file`. The release action embeds the Compose YAML into the marketplace payload
so HomeNavi can install and start the service.

## Local build + run with Homenavi stack

Use this to test the integration through integration-proxy with local assets:

```bash
cd src/frontend
npm install
npm run build

cd ../..
docker build -t homenavi-spotify:local .

export INTEGRATIONS_ROOT=/path/to/homenavi

docker run --rm -d \
  --name spotify \
  --network homenavi-network \
  -v ${INTEGRATIONS_ROOT}/integrations/setup/spotify.setup.json:/app/config/integration.setup.json \
  -e INTEGRATION_SETUP_PATH=/app/config/integration.setup.json \
  -v ${INTEGRATIONS_ROOT}/keys/jwt_public.pem:/app/keys/jwt_public.pem:ro \
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
  -v $(pwd)/integrations/spotify/config/integration.setup.json:/app/config/integration.setup.json \
  -e INTEGRATION_SETUP_PATH=/app/config/integration.setup.json \
  -v $(pwd)/keys/jwt_public.pem:/app/keys/jwt_public.pem:ro \
  -e JWT_PUBLIC_KEY_PATH=/app/keys/jwt_public.pem \
  homenavi-spotify:local
```

If you don’t need the admin setup/auth endpoints, omit the JWT mount/env lines.

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
  -v $(pwd)/config/integration.setup.json:/app/config/integration.setup.json \
  -e INTEGRATION_SETUP_PATH=/app/config/integration.setup.json \
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
  INTEGRATION_SETUP_PATH: /app/config/integration.setup.json
  JWT_PUBLIC_KEY_PATH: /app/keys/jwt_public.pem

integrations:
  - id: spotify
    upstream: http://spotify:8099
```

The chart will create a Deployment + Service and add an `installed.yaml` snippet for integration‑proxy. JWT public key mounting will be optional for deployments that do not use the admin setup/auth endpoints.
