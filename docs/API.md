# bass API reference

This is the detailed reference for the bass HTTP + WebSocket API. For
design rationale see [`../SPEC.md`](../SPEC.md). For ready-to-run examples
see [`../bruno/`](../bruno/).

All endpoints versioned under `/v1/`. JSON request/response,
`application/json`. Error responses share an envelope:

```json
{ "error": "code", "message": "human-readable" }
```

Standard status codes: `400 invalid_request`, `401 unauthorized`, `403
forbidden`, `404 not_found`, `409 conflict`, `413 payload_too_large`, `429
rate_limited`, `500 internal`.

---

## Authentication

bass has two auth planes:

| Plane | Endpoints | Credential |
|---|---|---|
| Control | `/v1/admin/*`, `/v1/pair/*` (start) | OIDC JWT with `bass.admin` or `bass.sync` scope as appropriate |
| Data | `/v1/sync`, `/v1/devices`, `/v1/token/refresh`, `/v1/changes` | Opaque sync token in `Authorization: Bearer ...` |

Sync tokens are minted by the service after a successful pairing flow. They
are SHA-256-hashed at rest and compared in constant time on the request
path. A leaked sync token grants sync access only — not OIDC identity, not
refresh capability, not other apps.

---

## Endpoints

### `GET /.well-known/bass-config`

Public discovery. No auth.

```json
{
  "issuer": "https://idp.example.com/realms/main",
  "scopes": { "user": "bass.sync", "admin": "bass.admin" },
  "endpoints": {
    "pair_start":    "https://bass.example/v1/pair/start",
    "pair_callback": "https://bass.example/v1/pair/callback",
    "sync":          "https://bass.example/v1/sync",
    "changes_ws":    "wss://bass.example/v1/changes",
    "token_refresh": "https://bass.example/v1/token/refresh",
    "devices":       "https://bass.example/v1/devices"
  },
  "limits": { "max_value_bytes": 65536, "max_batch_items": 1024 },
  "idp_endpoints": {
    "authorization_endpoint": "...",
    "token_endpoint": "...",
    "userinfo_endpoint": "..."
  }
}
```

Clients should call this on init so they only need the service base URL.

### `GET /healthz`

Liveness probe. Returns `200 ok` with body `ok`.

---

## Pairing

### `GET /v1/pair/start`

Initiate pairing. Public endpoint.

**Query params:**

| Name | Required | Description |
|---|---|---|
| `app_id` | yes | Registered app id |
| `redirect_uri` | yes | Where to send the user after pairing. Must be in the app's `redirect_uris`. |
| `device_label` | no | Human-readable device name surfaced in `/v1/devices` |
| `mode` | no | `redirect` (default) or `popup`. Both end at the same callback. |

**Response:** `302 Found` with `Location:` pointing at the configured IdP's
authorize URL (carrying `state` + PKCE challenge).

In `--no-auth` dev mode the response goes straight to `redirect_uri#…` with
the tokens already set.

### `GET /v1/pair/callback`

OIDC redirect target. Validates `state`, exchanges code for tokens,
verifies the ID token, mints a device, and 302s to the original
`redirect_uri` with tokens in the **URL fragment**:

```
https://my-app.example/sync-cb#sync_token=…&refresh_token=…&device_id=…&expires_in=86400
```

The fragment is **not sent to the host app's server** and **does not appear
in HTTP access logs**. The host app reads it from `location.hash` and
should strip it via `history.replaceState` immediately.

### `POST /v1/token/refresh`

Rotate a sync token. Public endpoint (the refresh token is the credential).

```json
{ "refresh_token": "..." }
```

**Response:**

```json
{
  "sync_token":    "...",
  "refresh_token": "...",
  "expires_in":    86400,
  "device_id":     "01H..."
}
```

The old refresh token is invalidated. **Reuse detection**: if the same
refresh token is presented twice, the entire device chain is revoked and
the user must re-pair.

---

## Sync

### `GET /v1/sync`

Pull items written since a cursor.

**Auth:** Bearer sync token.

**Query params:**

| Name | Default | Description |
|---|---|---|
| `since` | `0` | Server version to pull from |
| `limit` | service max | Max items returned this call |

**Response:**

```json
{
  "items": [
    {
      "key": "myapp-theme",
      "value": "ZGFyaw==",
      "payload_ver": 1,
      "version": 47,
      "deleted": false,
      "updated_at": 1719320000,
      "updated_by": "01H..."
    },
    {
      "key": "myapp-history",
      "value": null,
      "payload_ver": 1,
      "version": 48,
      "deleted": true,
      "updated_at": 1719320500,
      "updated_by": "01H..."
    }
  ],
  "cursor": 48,
  "has_more": false
}
```

`value` is base64-encoded raw bytes (binary-safe; values are arbitrary
strings or blobs from the app's perspective). `cursor` is the highest
server version observed — pass it as `since` on the next pull. Loop until
`has_more` is false to drain the backlog.

### `POST /v1/sync`

Push writes.

**Auth:** Bearer sync token.

**Body:**

```json
{
  "items": [
    {
      "key": "myapp-theme",
      "value": "ZGFyaw==",
      "payload_ver": 1,
      "base_version": 47,
      "deleted": false
    },
    {
      "key": "myapp-history",
      "base_version": 12,
      "deleted": true
    }
  ]
}
```

| Field | Required | Description |
|---|---|---|
| `key` | yes | Sync key. Must match the app's `key_allowlist`. |
| `value` | only when not deleted | Base64-encoded raw bytes. Omit / null for deletes. |
| `payload_ver` | no | Reserved for future encryption schemes. Default `1` (plaintext). |
| `base_version` | yes | Last server version the client observed for this key. `0` for new keys. |
| `deleted` | no | `true` for deletes. Default `false`. |

**Response:**

```json
{
  "results": [
    { "key": "myapp-theme", "status": "accepted", "version": 49 },
    { "key": "myapp-history",
      "status": "accepted_overwrite",
      "version": 50,
      "previous_version": 13 }
  ],
  "cursor": 50
}
```

**LWW status semantics:**

- `accepted` — the client's `base_version` matched the row's current
  version (or the row was new). Normal happy path.
- `accepted_overwrite` — the client wrote based on stale state, but the
  write was applied anyway (pure LWW). `previous_version` is included so
  the client can surface a warning if it cares.
- `rejected` — currently only used for hard validation failures
  (forbidden key, oversized value); LWW conflicts always accept.

---

## Devices

### `GET /v1/devices`

List the current user's devices for this app.

**Auth:** Bearer sync token.

```json
{
  "devices": [
    {
      "id": "01H...",
      "user_sub": "...",
      "app_id": "my-app",
      "label": "MacBook Chrome",
      "token_expires": "2026-06-26T10:30:00Z",
      "refresh_expires": "2026-07-25T10:30:00Z",
      "created_at": "2026-06-25T10:30:00Z",
      "last_seen_at": "2026-06-25T14:10:00Z",
      "revoked": false
    }
  ],
  "current": "01H..."
}
```

### `DELETE /v1/devices/{id}`

Revoke a device. `id` must belong to the authenticated user.

**Auth:** Bearer sync token. Returns `204 No Content` (idempotent).

If the current device's id is passed, the calling session is terminated.

---

## Admin

### `POST /v1/admin/apps`

Register a new app.

**Auth:** OIDC JWT with `bass.admin` scope.

```json
{
  "id": "my-app",
  "name": "My App",
  "origins": ["https://my-app.example.com"],
  "redirect_uris": ["https://my-app.example.com/sync-cb"],
  "key_allowlist": ["myapp-*"]
}
```

Returns the persisted record (with `created_at` / `updated_at`).
`409 already_exists` if the id is taken.

### `GET /v1/admin/apps`

List all apps.

### `GET /v1/admin/apps/{id}`

Get one app.

### `PATCH /v1/admin/apps/{id}`

Partial update — any field may be omitted to leave it unchanged. Allowed
fields: `name`, `origins`, `redirect_uris`, `key_allowlist`.

### `DELETE /v1/admin/apps/{id}`

Delete an app. Cascades to all its devices and items. Idempotent.

### `GET /v1/admin/apps/{id}/devices`

List all devices for an app (across all users). Admin view.

### `DELETE /v1/admin/apps/{id}/devices/{deviceId}`

Admin revoke for a specific device.

---

## WebSocket — change notifications

### `wss://bass.example/v1/changes`

Subscribe to changes for the authenticated (user, app).

**Auth:** Bearer sync token rides on `Sec-WebSocket-Protocol`. From a browser:

```js
new WebSocket('wss://bass.example/v1/changes', ['bass.v1', `bearer.${syncToken}`]);
```

The server selects `bass.v1` and validates the bearer. **The bearer token
is therefore in the protocol header in plaintext — only ever use WSS, never
plain ws://.**

### Protocol

JSON messages, one per WebSocket frame.

**Client → server (initial, also on each reconnect):**
```json
{ "type": "subscribe", "since": 48 }
```

**Server → client (wake-up):**
```json
{ "type": "change", "cursor": 49 }
```

On `change`, the client should issue `GET /v1/sync?since=<last-known>` to
fetch the payload. **The server never pushes payload data over WS.** This
keeps the WS thin: reconnects, lost messages, partial frames all reduce to
"the next pull catches up."

**Heartbeat:**
```json
{ "type": "ping", "t": 1719320000 }
{ "type": "pong", "t": 1719320000 }
```

Server pings every 30 s. Client must respond within 30 s or the socket is
closed.

**Reconnect.** Client should reconnect with exponential backoff (1 s, 2 s,
4 s, …, capped at 60 s, with jitter). On reconnect, send `subscribe` with
the last persisted cursor.

**Fallback.** If the WS handshake fails (proxy block, etc.), the client
falls back to polling `GET /v1/sync` on a 30 s interval. No data loss
either way.

---

## Limits

The defaults below ship in `bass`; check `/.well-known/bass-config`'s
`limits` for the running instance's values.

| Limit | Default | Override |
|---|---|---|
| Max bytes per item value | 64 KiB (65536) | `BASS_MAX_VALUE_BYTES` |
| Max items per push batch | 1024 | `BASS_MAX_BATCH_ITEMS` |
| Sync token TTL | 24 h | `BASS_TOKEN_TTL` |
| Refresh token TTL | 30 d (720 h) | `BASS_REFRESH_TTL` |
| Pairing state TTL | 5 min | `BASS_PAIR_STATE_TTL` |

Quotas (per-user, per-app) are not enforced in MVP — see [`SPEC.md` §13](../SPEC.md#13-deferred--flagged-for-future-versions).
