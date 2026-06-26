# bass — Backendless App State Sync

**Status:** Draft v0.2 — initial implementation landed.
**Owner:** michal.jaskolski@hexagon.com
**Last updated:** 2026-06-26

---

## 1. Problem & Goals

### 1.1 Problem

Web apps designed to work without a backend (PWAs, local-first apps such as `wdsx`) store user state — settings, favorites, history, view preferences — in `localStorage` or IndexedDB. Users lose this state when:

- Switching browsers or devices.
- Clearing site data.
- Using the app on a fresh install.

The current workaround is per-app import/export, which is manual, friction-heavy, and asymmetric (no continuous sync).

### 1.2 Goal

Provide an **opt-in, self-hostable sync service + client library** that lets a backendless app continuously synchronize its `localStorage` state across a single user's devices, without the app developer giving up the backendless deployment model.

### 1.3 Non-goals (MVP)

- **IndexedDB sync.** Different data shape (structured stores, schemas, versions, blobs) — separate product surface. Deferred.
- **Multi-user collaboration / sharing.** Single-user, multi-device only.
- **Conflict-free merging.** Last-write-wins per key only. No CRDTs, no operational transforms.
- **Quotas, rate limiting, billing.** Sized for self-host MVP; surfaces flagged in §13.
- **Client-side end-to-end encryption.** Reserved as a protocol-level option (§5.4) but not implemented in MVP.
- **Pluggable storage backends.** SQLite only.
- **Mobile SDKs / non-browser clients.** Browser TS lib only.

### 1.4 Success criteria

A developer of a backendless app (e.g. `wdsx`) can:

1. Register their app with a self-hosted `bass` instance (admin action).
2. Add `@emdzej/bass-client` to the app and call one initializer.
3. Have user settings sync across browsers/devices within seconds of a write, with no further code changes to the app's existing `localStorage` calls.
4. Continue to work fully when the sync service is offline or the user is unauthenticated (local-only fallback).

The repo also ships:
- **`@emdzej/bass-svelte`** — a thin Svelte adapter exposing Svelte stores and Svelte 5 runes wrappers over `@emdzej/bass-client`. Lets Svelte apps consume synced keys with the same ergonomics as a normal `writable` store or `$state` rune.
- **`@emdzej/bass-demo`** — a Svelte + TypeScript demo app that exercises the manual API, the localStorage proxy, and the Svelte adapter against a locally-running `bass` service. Acts as a copy-pasteable integration reference and the end-to-end test target.

---

## 2. Glossary

| Term | Meaning |
|---|---|
| **Service** | The Go server (`bass`) hosting the sync API. Self-hostable. |
| **App** | A backendless web app registered with the service (e.g. `wdsx`). Identified by `app_id`, scoped to one or more origins. |
| **User** | An end user authenticated via OIDC. |
| **Device** | A specific browser instance for a given (user, app). Identified by `device_id` minted at pairing. |
| **Pairing** | One-time flow that binds (user, app, device) and mints sync credentials. |
| **OIDC token** | JWT from the external identity provider. Used only during pairing and admin operations. |
| **Sync token** | Opaque per-device bearer token minted by the service. Used for all data-plane calls. |
| **Refresh token** | Opaque token used to rotate an expired sync token. |
| **Item** | A single key/value entry being synchronized. |
| **Allowlist** | Key patterns this app is permitted to sync. Two layers: server-cap (admin) and client-filter (developer). |
| **Version** | Monotonic per-(user, app, key) server-assigned integer. Sole basis for LWW resolution. |

---

## 3. Architecture overview

```
┌─────────────────────┐         ┌──────────────────────────────────────┐
│  Web app (wdsx)     │         │  bass service (Go)                   │
│  ┌───────────────┐  │ HTTPS   │  ┌────────────────────────────────┐  │
│  │ @emdzej/bass- │  │◄───────►│  │  REST API  /v1/...             │  │
│  │ client (TS)   │  │   WSS   │  │  WS  /v1/changes              │  │
│  │               │◄─┼─────────┼─►│  Discovery /.well-known/...    │  │
│  │  - proxy()    │  │         │  └────────────────────────────────┘  │
│  │  - manual API │  │         │  ┌────────────────────────────────┐  │
│  └───────────────┘  │         │  │  Auth: OIDC verifier + opaque  │  │
│  localStorage       │         │  │        sync-token table         │  │
└─────────────────────┘         │  ├────────────────────────────────┤  │
                                │  │  Storage: SQLite (modernc)     │  │
                                │  └────────────────────────────────┘  │
                                └───────────────┬──────────────────────┘
                                                │
                                                ▼
                                  ┌───────────────────────┐
                                  │  External OIDC IdP    │
                                  │  (Keycloak, Authelia, │
                                  │   Auth0, Entra, …)    │
                                  └───────────────────────┘
```

### 3.1 Auth-plane split

Mirroring `swsrs`:

- **Control plane** (pairing, admin) uses **OIDC JWT**, verified via `coreos/go-oidc/v3`.
- **Data plane** (sync reads/writes, WS subscribe) uses **opaque sync tokens** minted by the service after pairing. Constant-time comparison server-side.

Rationale: opaque tokens are revocable instantly, scoped per device, and a leak only grants sync access for that one (user, app, device) tuple — not OIDC identity, not other apps.

### 3.2 OIDC scopes

| Scope | Granted to | Permits |
|---|---|---|
| `bass.admin` | Admin users | Register/edit/delete apps; set server-cap allowlists; list devices |
| `bass.sync` | End users | Pair their device with a registered app |

Scopes are mapped from IdP groups/claims via service config.

---

## 4. Threat model

| Threat | Mitigation |
|---|---|
| Stolen sync token (XSS in host app) | Short token TTL (24h default), refresh rotation, per-device scope, instant server-side revoke endpoint, audit log of token usage. |
| Stolen refresh token | Rotation on every use; reuse detection (if same refresh used twice → revoke entire device chain). |
| Malicious app posing as a registered app | Origin pinning checked on every request via `Origin`/`Sec-Fetch-Site` headers AND on the OIDC redirect URL. App registration is admin-only. |
| Compromised sync service reads user data | **Not mitigated in MVP.** Self-host trust model. Optional client-side encryption is reserved (§5.4). |
| Cross-user data leak | All queries scoped by `user_id` from the sync token; no shared keys. |
| Token in URL / logs | Sync tokens never appear in URLs or query strings. WS auth via `Sec-WebSocket-Protocol`, not `?token=`. REST auth via `Authorization` header. |
| Replay of sync writes | Server-assigned versions are monotonic; client-supplied `base_version` provides optimistic concurrency. |
| Denial of service via large values | Hard limit: 64 KiB per value, 1024 items per batch (configurable, post-MVP). |
| Cross-origin CSRF on pairing | OAuth `state` parameter (CSRF nonce) + origin check on callback. PKCE on the OIDC code exchange. |

---

## 5. Data model

### 5.1 Tables (SQLite)

```sql
-- Registered apps. Created by admin via API or CLI.
CREATE TABLE apps (
  id            TEXT PRIMARY KEY,          -- slug, e.g. "wdsx"
  name          TEXT NOT NULL,
  origins       TEXT NOT NULL,             -- JSON array of allowed origins
  redirect_uris TEXT NOT NULL,             -- JSON array of allowed redirect URIs
  key_allowlist TEXT NOT NULL,             -- JSON array of glob patterns, default ["*"]
  created_at    INTEGER NOT NULL,          -- unix seconds
  updated_at    INTEGER NOT NULL
);

-- Paired devices. Created during pairing.
CREATE TABLE devices (
  id              TEXT PRIMARY KEY,        -- ULID
  user_sub        TEXT NOT NULL,           -- OIDC `sub` claim
  app_id          TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  label           TEXT,                    -- user-supplied "MacBook Chrome"
  sync_token_hash TEXT NOT NULL,           -- SHA-256 of current sync token
  refresh_hash    TEXT NOT NULL,           -- SHA-256 of current refresh token
  token_expires   INTEGER NOT NULL,
  created_at      INTEGER NOT NULL,
  last_seen_at    INTEGER NOT NULL,
  revoked_at      INTEGER                  -- NULL = active
);
CREATE INDEX idx_devices_user_app ON devices(user_sub, app_id);

-- Per-key state. One row per (user, app, key).
CREATE TABLE items (
  user_sub    TEXT NOT NULL,
  app_id      TEXT NOT NULL,
  key         TEXT NOT NULL,
  value       BLOB,                        -- NULL when deleted=1 (tombstone)
  payload_ver INTEGER NOT NULL DEFAULT 1,  -- reserved for future encryption schemes
  version     INTEGER NOT NULL,            -- monotonic per (user_sub, app_id)
  deleted     INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL,
  updated_by  TEXT NOT NULL,               -- device_id that wrote this
  PRIMARY KEY (user_sub, app_id, key)
);
CREATE INDEX idx_items_version ON items(user_sub, app_id, version);

-- Per-(user, app) version counter. Drives the monotonic cursor.
CREATE TABLE version_counters (
  user_sub TEXT NOT NULL,
  app_id   TEXT NOT NULL,
  version  INTEGER NOT NULL,
  PRIMARY KEY (user_sub, app_id)
);
```

### 5.2 Versioning

Versions are monotonic **per (user_sub, app_id)** — not per key, not global. This means a single integer is the only cursor a client needs to resume sync ("everything since version N"). On any write, the counter is incremented and stamped onto the item.

### 5.3 Tombstones

Deletes set `deleted=1` and clear `value`, but keep the row. Required so that a device that was offline during the delete can learn about it on next sync. Tombstones are not garbage-collected in MVP (deferred — §13).

### 5.4 `payload_ver` — reserved for future encryption

`payload_ver=1` means "plaintext, server-readable". Future values can introduce client-side encryption (e.g. `payload_ver=2` = AES-GCM with key derived from OIDC `sub` + salt) without a schema migration. Client and server both read/write the field but MVP only uses `1`.

---

## 6. Pairing flow

```
Browser (host app + bass-client)        bass service                External IdP
            │                                │                         │
            │ user clicks "Sync settings"    │                         │
            │ window.location = /v1/pair/start                        │
            │    ?app_id=wdsx                                          │
            │    &redirect_uri=https://wdsx.example/sync-cb            │
            │    &device_label=MacBook+Chrome                          │
            │───────────────────────────────►│                         │
            │                                │ validate app_id +        │
            │                                │ redirect_uri vs registry │
            │                                │ store {state,            │
            │                                │  app_id, redirect_uri,   │
            │                                │  device_label,           │
            │                                │  pkce_verifier} in       │
            │                                │  short-lived cache       │
            │                                │                         │
            │   302 to IdP authorize URL                                │
            │   with state + PKCE challenge                             │
            │◄───────────────────────────────│                         │
            │                                                          │
            │ user logs in & consents                                  │
            │─────────────────────────────────────────────────────────►│
            │                                                          │
            │   302 to /v1/pair/callback?code=...&state=...            │
            │◄─────────────────────────────────────────────────────────│
            │                                │                         │
            │ GET /v1/pair/callback          │                         │
            │   ?code=...&state=...          │                         │
            │───────────────────────────────►│                         │
            │                                │ retrieve state cache    │
            │                                │ exchange code for       │
            │                                │ OIDC tokens (PKCE)      │
            │                                │────────────────────────►│
            │                                │ ID token + claims       │
            │                                │◄────────────────────────│
            │                                │ verify `bass.sync` scope│
            │                                │ create device row       │
            │                                │ mint sync_token,        │
            │                                │   refresh_token         │
            │                                │                         │
            │  302 to redirect_uri#sync_token=...&refresh_token=...    │
            │  &device_id=...&expires_in=...                           │
            │◄───────────────────────────────│                         │
            │                                                          │
            │ bass-client captures hash, stores in localStorage,       │
            │ strips hash from URL, begins sync.                       │
```

**Why URL fragment, not query string:** the fragment is not sent to the server hosting the host app and is not logged in HTTP access logs. Browser strips it after read.

### 6.1 Two transport modes — redirect and popup

Both modes share the same endpoints, state machine, and token-delivery URL fragment. They differ only in *how* the browser opens the IdP and *how* the resulting tokens reach the originating page.

**Mode A — Redirect (default, mobile-safe):**
The host app navigates the current tab to `/v1/pair/start`. The IdP redirects back to `redirect_uri#sync_token=…`. The dedicated callback route in the host app captures the fragment, hands the tokens to the client lib, and routes the user back to where they were.

- ✅ Survives third-party cookie blocking everywhere
- ✅ Works in mobile webviews and embedded browsers
- ✅ No cross-window messaging
- ❌ Full page navigation — host app state must survive the round trip (URL params, sessionStorage, or a "return to here after pairing" cookie)

**Mode B — Popup (opt-in, desktop convenience):**
The host app calls `bass.pair({ mode: 'popup' })`, which `window.open()`s `/v1/pair/start` with a special `redirect_uri` pointing at a tiny `/sync-cb` page inside the host app's origin. That page reads the fragment, `postMessage`s the tokens to `window.opener` constrained to `targetOrigin === window.location.origin`, then closes itself. The opener receives the message, validates the origin, hands the tokens to the client lib.

- ✅ Host app state is preserved — no full navigation
- ✅ Snappy UX on desktop
- ❌ Blocked by some mobile webviews and aggressive popup blockers — client lib detects `window.open() === null` and falls back to redirect automatically
- ❌ Requires the host app to ship the `/sync-cb` page either way

**Both modes share `/sync-cb`** — same handler, same fragment-reading code; it just behaves differently depending on whether `window.opener` is set. The host app integrates a single callback route and gets both flows for free.

**Selection:** `bass.pair()` defaults to `redirect`. Apps that want the popup experience pass `{ mode: 'popup' }`; the lib will silently fall back to redirect if `window.open` is blocked.

---

## 7. REST API

All endpoints versioned under `/v1/`. JSON request/response, `application/json`. Errors:

```json
{ "error": "code", "message": "human-readable", "details": { ... } }
```

Standard codes: `400 invalid_request`, `401 unauthorized`, `403 forbidden`, `404 not_found`, `409 conflict`, `413 payload_too_large`, `429 rate_limited`, `500 internal`.

### 7.1 Discovery — public

```
GET /.well-known/bass-config
→ 200
{
  "issuer": "https://idp.example/realms/main",
  "scopes": { "user": "bass.sync", "admin": "bass.admin" },
  "endpoints": {
    "pair_start":    "https://bass.example/v1/pair/start",
    "pair_callback": "https://bass.example/v1/pair/callback",
    "sync":          "https://bass.example/v1/sync",
    "changes_ws":    "wss://bass.example/v1/changes",
    "token_refresh": "https://bass.example/v1/token/refresh",
    "devices":       "https://bass.example/v1/devices"
  },
  "limits": { "max_value_bytes": 65536, "max_batch_items": 1024 }
}
```

Allows the client lib to be configured with only a service base URL.

### 7.2 Pairing — public + OIDC

```
GET /v1/pair/start
  query: app_id, redirect_uri, device_label
→ 302 to IdP authorize URL

GET /v1/pair/callback
  query: code, state
→ 302 to redirect_uri#sync_token=…&refresh_token=…&device_id=…&expires_in=…
```

### 7.3 Sync — opaque sync token

#### Pull

```
GET /v1/sync?since=<version>&limit=<n>
Authorization: Bearer <sync_token>

→ 200
{
  "items": [
    {
      "key": "wdsx-favorites:vehicle-42",
      "value": "...base64...",
      "payload_ver": 1,
      "version": 47,
      "deleted": false,
      "updated_at": 1719320000,
      "updated_by": "01H..."
    },
    {
      "key": "theme",
      "value": null,
      "payload_ver": 1,
      "version": 48,
      "deleted": true,
      "updated_at": 1719320500,
      "updated_by": "01H..."
    }
  ],
  "cursor": 48,        // pass back as `since=` next time
  "has_more": false
}
```

`value` is base64-encoded so the lib can sync binary as well as strings without protocol changes.

#### Push

```
POST /v1/sync
Authorization: Bearer <sync_token>
Content-Type: application/json
{
  "items": [
    { "key": "theme", "value": "ZGFyaw==", "payload_ver": 1, "base_version": 47, "deleted": false },
    { "key": "wdsx-favorites:vehicle-42", "value": null, "base_version": 12, "deleted": true }
  ]
}

→ 200
{
  "results": [
    { "key": "theme", "status": "accepted", "version": 49 },
    { "key": "wdsx-favorites:vehicle-42", "status": "rejected_stale",
      "server_version": 50, "server_value": "...", "server_deleted": false }
  ],
  "cursor": 50
}
```

**LWW reconciliation:** server compares `base_version` to the row's current `version`:

- If equal **or** if the client's intent has a later wall-clock and the difference is within configured skew → **accept**, increment counter, return new version.
- If client's `base_version` is stale → **reject** with the server's current value so the client can decide.

For pure LWW (MVP), the rule simplifies to: server's `updated_at` decides; if client's `base_version` is stale, the server returns its winning value and the client overwrites locally. The `base_version` field lets us layer in finer-grained conflict policies later without changing the wire format.

### 7.4 Token refresh

```
POST /v1/token/refresh
Content-Type: application/json
{ "refresh_token": "..." }

→ 200
{
  "sync_token":    "...",
  "refresh_token": "...",   // new — old one is invalidated
  "expires_in":    86400
}
```

**Reuse detection:** if the same refresh token is presented twice (i.e. an old, already-rotated value), revoke the entire device and require re-pairing. Standard OAuth2 RTR pattern.

### 7.5 Device management

```
GET /v1/devices
Authorization: Bearer <sync_token>  — user's own devices for this app

→ 200
{ "devices": [{ "id": "...", "label": "...", "last_seen_at": ..., "current": true }, ...] }

DELETE /v1/devices/{id}
Authorization: Bearer <sync_token>

→ 204 (idempotent)
```

Deleting the current device terminates this session.

### 7.6 Admin — OIDC JWT with `bass.admin`

```
POST /v1/admin/apps
{
  "id": "wdsx",
  "name": "WDSX",
  "origins": ["https://wdsx.example"],
  "redirect_uris": ["https://wdsx.example/sync-cb"],
  "key_allowlist": ["wdsx-*", "wds-viewer-*", "theme"]
}

GET    /v1/admin/apps
GET    /v1/admin/apps/{id}
PATCH  /v1/admin/apps/{id}
DELETE /v1/admin/apps/{id}

GET    /v1/admin/apps/{id}/devices       — list all devices for an app
DELETE /v1/admin/apps/{id}/devices/{id}  — admin revoke
```

---

## 8. WebSocket — change notifications

### 8.1 Endpoint and auth

```
GET wss://bass.example/v1/changes
Sec-WebSocket-Protocol: bass.v1, bearer.<sync_token>
```

Bearer token rides on `Sec-WebSocket-Protocol`, not in the URL. Server selects `bass.v1` and validates the bearer.

### 8.2 Protocol

JSON messages, one per frame.

**Client → server (initial / resume):**
```json
{ "type": "subscribe", "since": 48 }
```

**Server → client (wake-up):**
```json
{ "type": "change", "cursor": 49 }
```

The server **does not** push payload data over WS. On `change`, the client issues `GET /v1/sync?since=<last-seen>` to fetch.

Rationale: keeps WS thin and stateless. Reconnects, lost messages, hostile proxies, and partial frames all reduce to "client's `since` cursor is now behind, next pull catches up." Payload retries, large values, encoding all live in well-understood HTTP semantics.

**Heartbeat:**
```json
{ "type": "ping", "t": 1719320000 }
{ "type": "pong", "t": 1719320000 }
```

Server pings every 30 s. Client must respond within 30 s or the server closes the socket.

**Server → client (terminal):**
```json
{ "type": "error", "code": "unauthorized" | "revoked" | "server_shutdown" }
```

### 8.3 Client reconnect behaviour

- Reconnect with exponential backoff (1 s, 2 s, 4 s, …, capped at 60 s, with jitter).
- On reconnect, send `subscribe` with the last cursor the client persisted.
- If WS handshake fails (proxy block, etc.), fall back to polling `GET /v1/sync` on a 30 s interval.

### 8.4 Single connection per device

A device may hold at most one WS connection. The server closes any prior connection from the same device when a new one connects. This bounds resource use and makes it predictable when a user has many tabs (see §10.3).

---

## 9. Conflict resolution — LWW details

For MVP, conflict resolution is purely last-write-wins, with the **server's monotonic version** as the tiebreaker (not wall clocks — clock skew makes them unreliable).

### 9.1 Write path

1. Client calls `POST /v1/sync` with `base_version` (the version it last observed for that key) and the new value.
2. Server reads current row:
   - **No row exists** → insert with new version. Status: `accepted`.
   - **Row exists, `current.version == base_version`** → update, increment version. Status: `accepted`.
   - **Row exists, `current.version > base_version`** → the client is stale. Server **still applies the write**, increments version, and returns `accepted_overwrite` with `previous_version` so the client can log it. (Pure LWW: the latest write wins regardless of whether the writer saw the prior state.)

Calling this `accepted_overwrite` rather than `rejected_stale` matches what users actually want from settings sync — the value they just typed wins — but exposes enough information for the client to surface a warning if it cares.

### 9.2 Deletes

A delete is a write with `value=null, deleted=true`. Tombstone wins the LWW comparison the same way any other write does.

### 9.3 Initial bootstrap

New device after pairing has `since=0`. Server returns all non-tombstone items. Client hydrates `localStorage` before the host app reads it (see §10.2).

---

## 10. Client library: `@emdzej/bass-client`

### 10.1 Public API

```ts
import { createBassClient } from '@emdzej/bass-client';

const bass = createBassClient({
  serviceUrl: 'https://bass.example',
  appId: 'wdsx',
  // optional — defaults to ['*']
  keys: ['wdsx-*', 'theme'],
  // optional — defaults shown
  debounceMs: 500,
  storage: window.localStorage,         // override for tests
});

// — pairing —
await bass.pair({
  redirectUri: location.origin + '/sync-cb',
  mode: 'redirect',                   // 'redirect' | 'popup', default 'redirect'
});
//   redirect mode: navigates to /v1/pair/start; control returns
//     after the round trip on the next page load.
//   popup mode: opens /v1/pair/start in a popup; promise resolves
//     when the popup posts tokens back; falls back to redirect
//     if window.open is blocked.
bass.completePairingFromUrl();         // call once on /sync-cb load
//   handles both modes: forwards tokens via postMessage if it's
//   running inside a popup, otherwise resolves the local pair() promise.

// — auth state —
bass.isPaired();                       // boolean
bass.devices.list();
bass.devices.unpair();                 // forget this device

// — manual API —
await bass.set('theme', 'dark');
await bass.get('theme');               // returns local cache, triggers sync in bg
await bass.delete('theme');
const unsub = bass.subscribe('theme', (value) => { ... });

// — transparent proxy —
bass.attachLocalStorageProxy();
//   from this point, every window.localStorage.setItem/removeItem
//   for a key matching `keys` is queued to the outbox.
//   localStorage itself is the source of truth for reads.
```

### 10.2 Bootstrap ordering

New devices must hydrate `localStorage` from the server **before** the host app reads it, otherwise the app paints with stale local state and flickers when the sync arrives. Recommended integration:

```ts
// in app entry, before mounting UI
const bass = createBassClient({ ... });
if (bass.isPaired()) {
  await bass.hydrate({ timeoutMs: 2000 });
  //   pulls /v1/sync?since=<cursor>; resolves on success.
  //   if the timeout elapses, resolves with { timedOut: true } and
  //   the app proceeds with whatever is in localStorage. Outbox
  //   keeps any local writes; on reconnect, hydrate completes in
  //   the background and re-emits via the subscribe channel.
}
bass.attachLocalStorageProxy();
mountApp();
```

**Behaviour on cold start (new device, empty local cache):**
- Network healthy: hydrate completes in ~200–800 ms, app paints with sync'd state. No flicker.
- Network slow: hydrate hits the 2 s timeout, app paints with empty cache, sync completes in the background, the subscribe channel notifies subscribers (and the Svelte adapter's stores re-emit), UI updates.

**Behaviour on warm start (cursor known, no remote changes):**
- Hydrate is a no-op — server returns an empty result and the app proceeds in ~30–50 ms.

**Tunable, not mandatory.** `timeoutMs: 0` opts out of waiting entirely (fully async hydration, "render stale, update on arrival"); `timeoutMs: Infinity` blocks indefinitely until success or error. Default 2000 ms is the sweet spot for settings apps — long enough for healthy networks to complete, short enough that hostile networks don't strand the user.

**SvelteKit and other frameworks with their own hydration lifecycle:** call `bass.hydrate()` inside a top-level `+layout.ts` load function (which awaits before children render), or — if that's awkward — call it non-blocking and let the Svelte adapter's stores drive the UI update when data arrives.

### 10.3 Multi-tab

A single device may have many tabs open. Behaviour:

- One tab holds the WS connection (elected via a `BroadcastChannel('bass') ` leader election; first to grab the lock wins, others get pinged via the channel on change).
- All tabs listen for `storage` events on `window` to learn about same-device cross-tab writes.
- All tabs listen on the `BroadcastChannel` to learn about remote-device writes that the leader tab pulled.

The proxy uses `window.localStorage` directly — no IndexedDB lock games — so the leader tab triggers a `storage` event in other tabs naturally on hydration.

### 10.4 Offline outbox

Writes go to `localStorage` immediately (for synchronous reads) AND to an outbox in `localStorage` under a single key (`__bass_outbox__`). The outbox is drained on a debounce timer, on reconnect, and on `flush()`.

Outbox entries are deduplicated by key — only the latest write for each key is kept, so a chatty app (like `wdsx`) coalesces naturally.

### 10.5 Local-only mode

If `bass.isPaired()` is false, the proxy is a no-op pass-through to `localStorage`. Apps depending on the lib continue to work fully offline / unauthenticated. No code paths block on auth state.

### 10.6 Allowlist enforcement on the client

Client-supplied `keys` patterns are matched as case-sensitive globs (`*` matches any sequence except `:` to keep model-scoped keys distinct unless explicitly requested via `wdsx-favorites:*`). Non-matching writes pass through to `localStorage` unmodified and are not queued.

If the server rejects a key as outside the server-cap allowlist, the client logs a warning and stops queueing that key (without erroring out the host app).

---

## 11. Allowlist — two layers

**Server cap (enforced):** declared on app registration. Hard limit. Writes to keys outside the cap are rejected with `403 forbidden_key`. Default `["*"]`.

**Client filter (advisory):** declared in `createBassClient({ keys })`. Default `["*"]`. Used to narrow what the client sends — for performance or to avoid syncing keys the developer doesn't want sync'd even though the server would allow them.

Both layers default to `*` to make adoption painless; the admin UI / docs should nudge production apps toward explicit lists.

---

## 12. Repository layout

`bass` is a single monorepo containing the Go service, the TypeScript client library, and a Svelte demo app — driven by a pnpm workspace for the JS side and a `go.mod` at the root for the Go side.

```
bass/
├── cmd/bass/                 # Go binary entrypoint
├── internal/                 # Go service internals (§12.1)
├── db/migrations/            # golang-migrate SQL files
├── pkg/client-go/            # optional Go SDK (post-MVP)
├── packages/                 # publishable libraries (§12.5)
│   ├── client/               # @emdzej/bass-client
│   └── svelte/               # @emdzej/bass-svelte
├── apps/                     # end-user applications, not published
│   └── demo/                 # @emdzej/bass-demo (Svelte)
├── docker/                   # §12.8
│   ├── Dockerfile            # multi-stage build for the service
│   └── docker-compose.yml    # bass + dex (IdP) + demo
├── .github/workflows/        # CI + release (§12.9)
│   ├── ci.yml
│   └── release.yml
├── bruno/                    # Bruno API collection (§12.10)
│   ├── bruno.json
│   ├── environments/
│   └── ...endpoints
├── go.mod / go.sum
├── pnpm-workspace.yaml
├── turbo.json                # Turborepo pipeline (§12.5)
├── package.json              # workspace root, scripts + devDeps (turbo, biome)
└── SPEC.md
```

**Service tree (Go), mirroring `swsrs`:**

```
cmd/bass/
  main.go              — subcommand dispatch
  serve.go             — `bass serve`
  migrate.go           — `bass migrate up|down|status`
  version.go

internal/auth/
  oidc.go              — verifier + middleware (port from swsrs)
  scopes.go            — scope constants

internal/apps/
  store.go             — app CRUD
  handler.go           — /v1/admin/apps/*

internal/pairing/
  state.go             — short-lived state cache (in-memory, TTL 5 min)
  handler.go           — /v1/pair/start, /v1/pair/callback

internal/devices/
  store.go             — device + token CRUD
  token.go             — opaque token mint, hash, verify
  handler.go           — /v1/devices/*

internal/sync/
  store.go             — items + version counter
  handler.go           — /v1/sync GET, POST
  resolver.go          — LWW logic

internal/changes/
  hub.go               — fan-out: (user_sub, app_id) → []*conn
  handler.go           — /v1/changes WS

internal/discovery/
  handler.go           — /.well-known/bass-config

internal/cors/
  cors.go              — per-app origin enforcement (port from swsrs)

internal/storage/
  sqlite.go            — modernc.org/sqlite open + pragmas
  migrations/          — golang-migrate SQL files

pkg/client-go/         — optional Go SDK (post-MVP)

Dockerfile
docker-compose.yml
go.mod / go.sum
```

### 12.1 Dependencies

- `github.com/coreos/go-oidc/v3` — OIDC verifier (same as swsrs)
- `github.com/coder/websocket` — WS server (same as swsrs)
- `modernc.org/sqlite` — CGO-free SQLite driver
- `github.com/golang-migrate/migrate/v4` — migrations
- `github.com/oklog/ulid/v2` — IDs
- stdlib `log/slog`, `net/http`, `database/sql`, `flag`

No router framework. Go 1.22+ `http.ServeMux` with named patterns (`"GET /v1/sync"`).

### 12.2 Config

Env-first, CLI-flag override. Same pattern as swsrs:

| Env | Default | Purpose |
|---|---|---|
| `BASS_ADDR` | `:8080` | Listen address |
| `BASS_DB_PATH` | `bass.db` | SQLite path |
| `BASS_OIDC_ISSUER` | (required) | IdP issuer URL |
| `BASS_OIDC_AUDIENCE` | (required) | JWT audience |
| `BASS_OIDC_CLIENT_ID` | (required) | OAuth client used for pairing code exchange |
| `BASS_OIDC_CLIENT_SECRET` | (required) | OAuth client secret |
| `BASS_TOKEN_TTL` | `24h` | Sync token TTL |
| `BASS_REFRESH_TTL` | `30d` | Refresh token TTL |
| `BASS_MAX_VALUE_BYTES` | `65536` | Per-item limit |
| `BASS_MAX_BATCH_ITEMS` | `1024` | Per-request batch limit |
| `BASS_NO_AUTH` | `false` | Dev only — bypass OIDC verification |

### 12.3 Logging & errors

- `slog` JSON to stdout.
- Errors wrapped with `fmt.Errorf("...: %w", err)`. Sentinel errors as package vars.
- HTTP errors via a small `httperr.Write(w, code, message)` helper.

### 12.4 Testing

- stdlib `testing`, no testify.
- `httptest.NewServer` for integration tests.
- Fake IdP server (pattern from `swsrs/internal/discovery/discovery_test.go`) for OIDC flow tests.
- In-memory SQLite (`:memory:`) for storage tests.

### 12.5 pnpm workspace

```
pnpm-workspace.yaml             — { packages: ["packages/*", "apps/*"] }
package.json                    — { private: true, scripts: {...} }
packages/
  client/                       — @emdzej/bass-client
    src/
      index.ts                  — public createBassClient export
      client.ts                 — Bass class
      proxy.ts                  — localStorage proxy + storage event bridge
      outbox.ts                 — debounced write queue
      pairing.ts                — redirect flow + completePairingFromUrl
      transport/
        rest.ts                 — fetch wrapper, auth header, refresh
        ws.ts                   — change-channel client w/ backoff
      discovery.ts              — /.well-known/bass-config loader
      storage/
        tokens.ts               — localStorage-backed token store
        cursor.ts               — last-seen version persistence
    test/
      *.test.ts                 — vitest, jsdom env
    package.json
    tsconfig.json
    vite.config.ts              — library build (ESM + CJS + d.ts)
  svelte/                       — @emdzej/bass-svelte (see §12.6)
    src/
      index.ts                  — bassWritable, bassReadable, useBassAuth
      runes.ts                  — bassState (Svelte 5 runes API)
      stores.ts                 — internal store factory
    test/
      *.test.ts                 — vitest + @testing-library/svelte
    package.json                — peerDep: svelte ^5, @emdzej/bass-client
    tsconfig.json
    vite.config.ts              — library build, externalises svelte + client
apps/
  demo/                         — Svelte demo app (see §12.7)
    src/
    static/
    svelte.config.js
    vite.config.ts
    package.json
    tsconfig.json
```

**Convention:** `packages/` holds publishable libraries (consumed by external apps); `apps/` holds end-user applications (binaries, sites, demos) that are built and deployed but never published to npm. Same split as pnpm/turborepo/nx conventions, so it's familiar.

**Build & tooling:**
- pnpm 9+, Node 20+
- **Turborepo** as the task orchestrator across packages (incremental builds, dependency-aware ordering, local caching). Remote cache disabled by default; opt-in via `TURBO_TOKEN`/`TURBO_TEAM` env if a team Vercel cache is wired up later.
- TypeScript 5.x, strict mode, `moduleResolution: bundler`
- `client` builds with Vite library mode → `dist/index.js` (ESM), `dist/index.cjs`, `dist/index.d.ts`. Tree-shakeable; zero runtime deps.
- `client` test runner: vitest with jsdom for DOM APIs (localStorage, storage events, BroadcastChannel polyfilled).
- Lint/format: Biome (single tool covers both, fast). Configured at the workspace root.

**`turbo.json`:**
```json
{
  "$schema": "https://turbo.build/schema.json",
  "tasks": {
    "build": {
      "dependsOn": ["^build"],
      "outputs": ["dist/**", ".svelte-kit/**", "build/**"]
    },
    "test": {
      "dependsOn": ["^build"],
      "outputs": ["coverage/**"]
    },
    "lint": {},
    "typecheck": { "dependsOn": ["^build"] },
    "dev": { "cache": false, "persistent": true }
  }
}
```

`^build` makes turbo build dependencies first, so `apps/demo` always sees a freshly-built `@emdzej/bass-client` and `@emdzej/bass-svelte` before its own build runs. Packages reference each other via `workspace:*` in their `package.json`.

**Workspace scripts (root `package.json`):**
```json
{
  "scripts": {
    "build":     "turbo run build",
    "test":      "turbo run test",
    "typecheck": "turbo run typecheck",
    "lint":      "biome check .",
    "format":    "biome format --write .",
    "dev":       "turbo run dev --filter=@emdzej/bass-demo",
    "clean":     "turbo run clean && rm -rf node_modules .turbo"
  }
}
```

### 12.6 Svelte adapter — `@emdzej/bass-svelte`

A thin layer over `@emdzej/bass-client` that makes synced keys feel native to Svelte. Published as a separate package so apps that don't use Svelte don't pull svelte as a dependency.

**Peer dependencies:** `svelte ^5`, `@emdzej/bass-client` (workspace `*`).

**Public API:**

```ts
import { bassWritable, bassReadable, useBassAuth } from '@emdzej/bass-svelte';

const theme = bassWritable(bass, 'theme', 'light');
// Svelte store contract: { subscribe, set, update }
// $theme reads the local cached value; assignment triggers set() + sync.

const history = bassReadable(bass, 'wdsx-history', []);
// Subscribes to remote changes; writes are not exposed.

const auth = useBassAuth(bass);
// Store of { isPaired, deviceId, label, lastSync } — re-emits on pair/unpair/refresh.
```

**Svelte 5 runes API** (`@emdzej/bass-svelte/runes`):

```ts
import { bassState } from '@emdzej/bass-svelte/runes';

const theme = bassState(bass, 'theme', 'light');
// theme.value reads/writes via the $state rune
// theme.synced — boolean: is local in sync with server?
```

Both APIs are thin wrappers around `bass.subscribe(key, cb)` + `bass.set(key, value)`. Same debouncing/outbox semantics as the bare client — the adapter never adds its own batching.

**SSR:** all factories no-op safely during SSR (guard on `typeof window`). Hydration starts when the client is paired and `bass.hydrate()` resolves.

**Build:** Vite library mode, ESM + d.ts. Svelte and the client lib are externalized, not bundled.

### 12.7 Demo app — `@emdzej/bass-demo`

A Svelte 5 + TypeScript SPA built with SvelteKit (static adapter), mirroring `wdsx`'s stack so the integration story is realistic.

**Purpose:**
1. Provide a copy-pasteable integration reference for app authors.
2. Serve as the manual end-to-end test target during development.
3. Exercise every public surface of the client library at least once.

**Scope (intentionally minimal):**

| Page / feature | What it demonstrates |
|---|---|
| `/` — landing | Pairing button, current device status, "open in another tab" hint for multi-tab demo |
| `/settings` | A handful of UI controls (theme toggle, slider, text input, tag list) wired through both the **manual API** and the **localStorage proxy** in two side-by-side panels — so the developer can see both patterns |
| `/devices` | Lists paired devices via `bass.devices.list()`; allows unpair |
| `/inspector` | Live view of the outbox + last cursor + WS connection state, for debugging |
| `/sync-cb` | Pairing redirect target — calls `bass.completePairingFromUrl()` and bounces home |

**Configuration:** reads `VITE_BASS_URL` (default `http://localhost:8080`) and `VITE_APP_ID` (default `bass-demo`) so the same build can target local dev or a deployed bass.

**Not published.** `private: true` in `package.json`; built into a static bundle that `docker-compose` serves alongside `bass` for local dev.

### 12.8 Docker & local development

Mirrors `swsrs`'s patterns — multi-stage Dockerfile producing a static binary, distroless runtime, env-driven config via `docker-compose.yml`.

**`docker/Dockerfile` (service):**
```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/bass ./cmd/bass

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bass /bass
VOLUME ["/data"]
ENV BASS_DB_PATH=/data/bass.db BASS_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/bass", "serve"]
```

`modernc.org/sqlite` keeps `CGO_ENABLED=0` so the binary is fully static and the distroless image works.

**`docker/docker-compose.yml`** spins up three services for full local end-to-end:

```yaml
services:
  dex:
    image: ghcr.io/dexidp/dex:v2.40.0
    command: ["dex", "serve", "/etc/dex/config.yaml"]
    volumes:
      - ./dev/dex-config.yaml:/etc/dex/config.yaml:ro
    ports: ["5556:5556"]

  bass:
    build:
      context: ..
      dockerfile: docker/Dockerfile
    environment:
      BASS_ADDR: ":8080"
      BASS_DB_PATH: "/data/bass.db"
      BASS_OIDC_ISSUER: "http://dex:5556/dex"
      BASS_OIDC_AUDIENCE: "bass"
      BASS_OIDC_CLIENT_ID: "bass"
      BASS_OIDC_CLIENT_SECRET: "bass-dev-secret"
    volumes: ["bass-data:/data"]
    ports: ["8080:8080"]
    depends_on: [dex]

  demo:
    build:
      context: ..
      dockerfile: docker/Dockerfile.demo    # static build served by nginx:alpine
    environment:
      VITE_BASS_URL: "http://localhost:8080"
      VITE_APP_ID: "bass-demo"
    ports: ["5173:80"]

volumes:
  bass-data:
```

**Why dex** (not Keycloak): single static binary, ~20 MB image, file-based config — much faster to spin up for dev than Keycloak. Acts as a stand-in for whatever real IdP a self-hoster ends up running.

**`dev/dex-config.yaml`** ships a single static user (`demo@example.com` / `demo`) and the `bass` OIDC client preconfigured with redirect URI `http://localhost:8080/v1/pair/callback`. Enough for click-through local testing; not for production.

**Local dev without Docker:**
- `go run ./cmd/bass serve --no-auth` — runs bass with OIDC bypassed; client lib gets a "mock paired" state.
- `pnpm dev` — runs the demo via Vite dev server at `http://localhost:5173`.
- Useful for fast iteration on client lib and demo UI without round-tripping through dex.

### 12.9 Continuous integration (GitHub Actions)

Two workflows under `.github/workflows/`: `ci.yml` runs on every PR and push to `main`; `release.yml` runs on `v*` tags.

**`ci.yml` — parallel jobs:**

```yaml
name: CI
on:
  pull_request:
  push:
    branches: [main]
permissions:
  contents: read

jobs:
  js:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with: { version: 9 }
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: pnpm
      - uses: actions/cache@v4
        with:
          path: .turbo
          key: turbo-${{ runner.os }}-${{ github.sha }}
          restore-keys: turbo-${{ runner.os }}-
      - run: pnpm install --frozen-lockfile
      - run: pnpm lint
      - run: pnpm typecheck
      - run: pnpm test
      - run: pnpm build

  go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache: true
      - uses: golangci/golangci-lint-action@v6
        with: { version: latest }
      - run: go test -race -coverprofile=coverage.out ./...
      - run: go build -o /tmp/bass ./cmd/bass

  docker:
    needs: [js, go]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v5
        with:
          context: .
          file: docker/Dockerfile
          push: false
          tags: bass:ci
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

Notes:
- **Turbo local cache** is persisted between runs via `actions/cache@v4` on `.turbo/`. Builds that didn't change skip entirely. Remote cache (Vercel) can be wired up later via `TURBO_TOKEN` secret.
- **Go module cache** is handled by `setup-go` with `cache: true`.
- **Buildx GHA cache** speeds up Docker image builds across runs.
- **golangci-lint** config lives at `.golangci.yml` (linters: govet, errcheck, staticcheck, gosimple, unused, ineffassign, gofmt, gocritic).
- The `docker` job builds but doesn't push — only `release.yml` pushes.

**`release.yml` — runs on tags (`v0.1.0`, etc.):**

```yaml
name: Release
on:
  push:
    tags: ['v*']
permissions:
  contents: write       # for GitHub Release
  packages: write       # for ghcr.io push
  id-token: write       # for npm provenance

jobs:
  npm:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with: { version: 9 }
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          registry-url: 'https://registry.npmjs.org'
          cache: pnpm
      - run: pnpm install --frozen-lockfile
      - run: pnpm build
      - name: Publish @emdzej/bass-* to npm with provenance
        run: pnpm -r --filter "./packages/*" publish --access public --provenance --no-git-checks
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}

  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=sha,prefix=
      - uses: docker/build-push-action@v5
        with:
          context: .
          file: docker/Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: VERSION=${{ github.ref_name }}
          platforms: linux/amd64,linux/arm64

  gh-release:
    needs: [npm, docker]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
```

Notes:
- **npm provenance** (`--provenance` + `id-token: write`) cryptographically attests that the published packages came from this GitHub Actions run on this commit. Standard for new packages.
- **Multi-arch Docker** images (amd64 + arm64) — small extra cost, big convenience for Apple Silicon self-hosters.
- **Versioning:** packages and the Docker image share the git tag (`v0.1.0` → `0.1.0` on npm and `ghcr.io/.../bass:0.1.0`). Single source of truth, no per-package version drift in MVP. (If that becomes painful we can switch to Changesets later.)
- **Required secrets:** `NPM_TOKEN` (with publish access to `@emdzej`). `GITHUB_TOKEN` is auto-provisioned.

**Branch protection (set in GitHub UI, not in YAML):**
- `main` requires the `js`, `go`, `docker` checks before merge.
- Squash-merge only; PR titles follow conventional-commits style so release notes auto-generate cleanly.

### 12.10 API testing — Bruno collection

Admin operations (registering apps, listing devices, revoking) are exposed only via the REST API. Instead of shipping a CLI, the repo includes a **Bruno collection** under `bruno/` that admins can open in the [Bruno client](https://www.usebruno.com/) — file-based, git-friendly, no cloud account required.

```
bruno/
  bruno.json                  — { name: "bass", version: "1", type: "collection" }
  environments/
    local.bru                 — base URLs for docker-compose dev setup
    self-hosted.bru.example   — template for users to copy + edit
  discovery/
    get-config.bru
  admin/
    apps-create.bru           — POST /v1/admin/apps
    apps-list.bru             — GET  /v1/admin/apps
    apps-get.bru
    apps-patch.bru
    apps-delete.bru
    devices-list.bru          — GET  /v1/admin/apps/{id}/devices
    devices-revoke.bru
  sync/
    pull.bru                  — GET  /v1/sync?since=
    push.bru                  — POST /v1/sync
    refresh.bru               — POST /v1/token/refresh
  devices/
    list.bru
    unpair.bru
```

**Auth in the collection:**
- Admin endpoints use a `{{adminToken}}` env var that the operator pastes (raw OIDC access token from their IdP login). A `pre-request` script can optionally exchange a client-credentials grant if the IdP supports it.
- Sync endpoints use `{{syncToken}}` — populated by running a "pair via browser" flow first, then pasting the fragment value.

**Also ship `bruno/README.md`** with curl equivalents for every endpoint, so users who don't want Bruno still have a self-contained reference. Single source of truth: when an endpoint changes, the `.bru` file is the canonical example and the README's curl is regenerated.

**CI smoke test:** `release.yml` includes an optional `bruno-cli` job that runs the collection against the docker-compose stack on every release tag, catching protocol regressions before publish. (Stretch goal — wire up after MVP if it earns its keep.)

---

## 13. Deferred — flagged for future versions

| Feature | Why deferred | Where reserved in design |
|---|---|---|
| IndexedDB sync | 10× complexity (schemas, blobs, transactions) | Separate client package later |
| Client-side E2E encryption | Out of MVP per discussion; useful for self-host trust | `payload_ver` byte in protocol |
| Per-user / per-app quotas | Self-host MVP; not urgent | Add `quotas` table; enforce in sync handler |
| Tombstone GC | Needs careful "have all devices seen this?" logic | Add `tombstone_after` to items, prune job |
| Server-pushed payloads on WS | WS-as-notification is simpler and more robust | New WS message type `change_with_payload` |
| Pluggable storage backends | SQLite is fine for self-host scale | `internal/storage` already isolates this |
| Audit log / conflict history | Useful for debugging | Add `audit_log` table; events written by handlers |
| Mobile SDKs | Out of scope; browser only | — |
| Admin UI | CLI is sufficient for MVP | Could be a separate package later |
| Rate limiting | Self-host trust model | Middleware slot in `cmd/bass/serve.go` |

---

## 14. Resolved decisions

Captured here for posterity — each was an open question during design and now has a locked-in answer reflected elsewhere in the spec. New questions can be added above as they arise.

| # | Question | Decision | See |
|---|---|---|---|
| 1 | Pairing UX — redirect vs popup | **Both.** Redirect default; popup opt-in via `bass.pair({ mode: 'popup' })` with auto-fallback to redirect when blocked. | §6.1, §10.1 |
| 2 | Origin policy — exact vs subdomain wildcards | **Exact match only.** Admin lists multiple explicit origins when needed. | §4, §5.1 |
| 3 | Wire encoding for values | **Base64.** Binary-safe; ergonomics handled via the Bruno collection's pre-request scripts. | §7.3 |
| 4 | Hydrate vs lazy bootstrap | **Blocking with timeout** — `await bass.hydrate({ timeoutMs: 2000 })` is the recommended default. Times out into local-cache-then-catch-up so a slow network never blocks the app indefinitely. Apps that prefer "render stale, update on arrival" can call `bass.hydrate()` non-blocking and rely on the Svelte adapter's stores to re-emit. | §10.2 |
| 5 | Admin CLI vs API-only | **API-only.** Repo ships a Bruno collection + curl examples instead of a CLI subcommand. | §12.10 |

---

## 15. Out-of-scope clarifications

The following are **not** what `bass` is, to set expectations:

- Not a CRDT framework. If you need collaborative editing, use Yjs/Automerge.
- Not a general-purpose KV cloud. It's scoped to "this user's settings for this app, across this user's devices."
- Not a database. The server stores opaquely-keyed values; it has no schema awareness.
- Not a CDN, not a static asset host, not a feature flag service.
