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

All three commit only the URLs and the public client id — bass is a
**public OAuth client** (no shared secret; PKCE binds the exchange to the
flow). The only `vars:secret` entries are `syncToken` and `refreshToken`,
which are bass-minted opaque tokens (not OIDC), and you enter their
values once via Bruno's env editor UI.

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

Bruno drives the authorization-code + PKCE flow against the configured IdP:

1. First request in the folder → Bruno opens a browser popup pointed at
   `{{oidcAuthorizationUrl}}` with a PKCE challenge.
2. You log in to your IdP. The popup redirects to `{{oidcCallbackUrl}}`,
   which Bruno intercepts.
3. Bruno exchanges the code (with the PKCE verifier — no client secret)
   for an access token and caches it for subsequent requests.

**IdP setup** (Keycloak example):
- The `bass` client must be **public** — *Client authentication: OFF* in
  the Capability config tab. (PKCE replaces the shared secret.)
- Advanced → *PKCE Code Challenge Method*: `S256`.
- `oidcCallbackUrl` (e.g. `http://localhost:3000/callback`) must be in
  the Valid Redirect URIs list, alongside bass's own
  `https://<bass-host>/v1/pair/callback`.
- A `bass.admin` Client Scope must exist and be attached (as default or
  optional) so the issued token includes it.

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

# For scripts you can't drive the authorization_code+PKCE flow from curl;
# either grab an access token from your browser session, or configure a
# separate service-account client in the IdP and use client_credentials
# with that.

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
