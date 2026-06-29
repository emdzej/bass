# bass — Bruno API collection

Open this directory in [Bruno](https://www.usebruno.com/) (`File → Open Collection`).

## Environments

Switch via the env dropdown at the top right. Three are shipped:

- **`local`** — `http://localhost:8080`, no OIDC (the docker-compose stack with
  `BASS_NO_AUTH=true`).
- **`solvely`** — `https://bass.solvely.pl`, OIDC against
  `https://auth.solvely.pl/realms/solvely`.
- **`self-hosted`** — generic template; edit `baseUrl` + OIDC URLs for your
  own deployment.

All three commit only the URLs and client id. Secrets — `~oidcClientSecret`,
`~syncToken`, `~refreshToken` — use Bruno's secret-var prefix (`~`): you enter
their values once via Bruno's env editor UI, and Bruno stores them in a
local-only file that's never committed.

## Folder-level auth

Each endpoint folder has a `folder.bru` declaring its default auth mode;
every request inside uses `auth: inherit`. Refresh-token endpoint is the
single exception — it overrides to `auth: none` (the refresh token lives
in the request body, not in a header).

| Folder | Auth | What it uses |
|---|---|---|
| `discovery/` | none | Public — discovery doc + healthz |
| `pairing/` | none | Public — pair initiation |
| `admin/` | **oauth2** (authorization_code + PKCE) | Bruno auto-fetches a token from the IdP |
| `sync/` | bearer `{{syncToken}}` | Opaque bass sync token |
| `devices/` | bearer `{{syncToken}}` | Opaque bass sync token |

## How OAuth2 works for the admin folder

Bruno drives the authorization-code flow against the configured IdP:

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

**Local dev (`BASS_NO_AUTH=true`):** the admin endpoints accept anything;
the OAuth2 round-trip is unnecessary. Either ignore the auth failure
(Bruno still sends the request without a token) or temporarily set the
admin requests to `auth: none`.

## Getting a sync token

The `sync/` and `devices/` folders use the opaque bass sync token, which
isn't an OIDC token and isn't issued by your IdP. Bruno can't fetch it
for you — it's the output of bass's pairing flow.

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
