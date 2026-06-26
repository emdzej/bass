# bass — Bruno API collection

Open this directory in [Bruno](https://www.usebruno.com/) (`File → Open Collection`).

## Environments

Switch via the env dropdown at the top right.

- `local` — defaults pointing at `http://localhost:8080` and the demo app id. Drop-in for
  the `docker compose` stack.
- `self-hosted.bru.example` — copy to `self-hosted.bru` and fill in your own service URL,
  app id, and admin token.

## How to authenticate

| Endpoint group | Token type | How to obtain |
|---|---|---|
| `admin/*` | OIDC access token (`adminToken`) | Log in to your IdP and copy the access token, OR run `bass serve --no-auth` for local dev — then admin endpoints accept any value. |
| `sync/*`, `devices/*` (user) | Opaque sync token (`syncToken`) | Drive the pairing flow once (in a browser via the demo app, or via `pairing/pair-start-redirect.bru` with `--no-auth`), copy `sync_token` from the URL fragment into the env var. |
| `pairing/*`, `discovery/*` | None | Public endpoints. |

## curl equivalents

```sh
# Discovery
curl -s http://localhost:8080/.well-known/bass-config

# Register an app (admin)
curl -X POST http://localhost:8080/v1/admin/apps \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "bass-demo",
    "name": "bass demo",
    "origins": ["http://localhost:5173"],
    "redirect_uris": ["http://localhost:5173/sync-cb"],
    "key_allowlist": ["demo-*"]
  }'

# Pull sync items
curl -s -H "Authorization: Bearer $SYNC_TOKEN" \
  "http://localhost:8080/v1/sync?since=0"

# Push a write (value "dark" base64-encoded)
curl -X POST http://localhost:8080/v1/sync \
  -H "Authorization: Bearer $SYNC_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"items":[{"key":"demo-theme","value":"ZGFyaw==","base_version":0,"deleted":false}]}'

# Refresh tokens
curl -X POST http://localhost:8080/v1/token/refresh \
  -H "Content-Type: application/json" \
  -d "{\"refresh_token\":\"$REFRESH_TOKEN\"}"

# Devices (own)
curl -s -H "Authorization: Bearer $SYNC_TOKEN" http://localhost:8080/v1/devices
curl -X DELETE -H "Authorization: Bearer $SYNC_TOKEN" \
  "http://localhost:8080/v1/devices/$DEVICE_ID"
```
