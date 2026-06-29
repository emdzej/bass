# bass — Bruno API collection

Open this directory in [Bruno](https://www.usebruno.com/) (`File → Open Collection`).

## Environments

Switch via the env dropdown at the top right.

- **`local`** — defaults pointing at `http://localhost:8080` (the docker-compose
  stack with `BASS_NO_AUTH=true`). Admin endpoints accept any request — no real
  token round-trip happens.
- **`solvely.bru.example`** / **`self-hosted.bru.example`** — copy to
  `solvely.bru` / `self-hosted.bru` (gitignored) and fill in `oidcClientSecret`.
  These envs wire OAuth2 against a real Keycloak / Authentik / etc.

## Authentication

### Admin endpoints — automatic OAuth2

The `admin/` folder is configured with **folder-level OAuth2** in
`admin/folder.bru`. Requests in that folder inherit auth via Bruno's
authorization-code flow:

1. First request in the folder → Bruno opens a browser popup pointed at
   `{{oidcAuthorizationUrl}}`.
2. You log in to your IdP. The popup redirects to `{{oidcCallbackUrl}}`
   (default `http://localhost:3000/callback`), which Bruno intercepts.
3. Bruno exchanges the code for an access token, caches it, and refreshes
   it automatically on subsequent runs.

**IdP setup:** the `bass` OAuth2 client in your realm must have
`http://localhost:3000/callback` registered as a Valid Redirect URI
**in addition to** bass's own `https://<bass-host>/v1/pair/callback`.
Keycloak supports multiple values per client.

**Scopes:** the client must be configured to issue `bass.admin` when
requested. In Keycloak this means defining a Client Scope `bass.admin`
and attaching it (as default or optional) to the `bass` client.

**Local dev (`BASS_NO_AUTH=true`):** OAuth2 is unnecessary — the admin
endpoints accept anything. Either ignore the auth failures (Bruno will
still send the request), or flip each request's `auth: inherit` to
`auth: none` for the local env.

### Sync / device endpoints — manual

The `sync/`, `devices/`, and `pairing/` folders use the opaque sync token
minted by bass itself (not by your IdP). Bruno can't automate this — the
token is the output of the bass pairing flow.

To obtain one:
1. Open the demo app at `http://localhost:5173` (or your deployed equivalent).
2. Click **Pair (redirect)**, log in.
3. Open browser devtools → Application → Local Storage → copy the value of
   `__bass_tokens__:bass-demo`.
4. Paste the `syncToken` and `refreshToken` fields into your Bruno env.

## curl equivalents

```sh
# Discovery
curl -s $BASS_URL/.well-known/bass-config

# Get an admin token from Keycloak (authorization_code is interactive — for
# scripts, use a Keycloak service account with client_credentials)
ADMIN_TOKEN=$(curl -s -X POST \
  https://auth.example.com/realms/main/protocol/openid-connect/token \
  -d grant_type=client_credentials \
  -d client_id=bass \
  -d client_secret=$BASS_CLIENT_SECRET \
  -d scope='openid bass.admin' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')

# Register an app
curl -X POST $BASS_URL/v1/admin/apps \
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
  "$BASS_URL/v1/sync?since=0"

# Push a write (value "dark" base64-encoded)
curl -X POST $BASS_URL/v1/sync \
  -H "Authorization: Bearer $SYNC_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"items":[{"key":"demo-theme","value":"ZGFyaw==","base_version":0,"deleted":false}]}'

# Refresh tokens
curl -X POST $BASS_URL/v1/token/refresh \
  -H "Content-Type: application/json" \
  -d "{\"refresh_token\":\"$REFRESH_TOKEN\"}"

# Devices (own)
curl -s -H "Authorization: Bearer $SYNC_TOKEN" $BASS_URL/v1/devices
curl -X DELETE -H "Authorization: Bearer $SYNC_TOKEN" \
  "$BASS_URL/v1/devices/$DEVICE_ID"
```
