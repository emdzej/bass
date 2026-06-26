# bass

**Backendless app state synchronization.** Opt-in, self-hostable sync of
`localStorage` across a user's devices for web apps that intentionally have
no backend.

Built for PWAs and local-first apps where users today have to use manual
import/export to move their settings between browsers and devices. Add one
client library and a self-hosted service, and settings flow across devices
within seconds — without changing the rest of the app.

- Single-user, multi-device. Not a collaboration product.
- Last-write-wins per key. No CRDTs, no operational transforms.
- Opt-in: the app works fully offline / unauthenticated.
- Self-hostable: single Go binary + SQLite. No external dependencies beyond
  your own OIDC provider.

> 📐 Design rationale lives in [`SPEC.md`](./SPEC.md).
> 📡 Detailed REST + WS reference: [`docs/API.md`](./docs/API.md).
> 🧪 API examples ready to run: [`bruno/`](./bruno/).

## Table of contents

- [Architecture](#architecture)
- [Quick start (Docker)](#quick-start-docker)
- [For app developers](#for-app-developers)
  - [@emdzej/bass-client](#emdzejbass-client)
  - [@emdzej/bass-svelte](#emdzejbass-svelte)
- [For operators](#for-operators)
  - [Configuration](#configuration)
  - [Registering apps](#registering-apps)
  - [OIDC setup](#oidc-setup)
- [Security model](#security-model)
- [Development](#development)
- [Project layout](#project-layout)
- [License](#license)

## Architecture

```
┌──────────────────────┐         ┌───────────────────────────────────────┐
│  Your web app        │         │  bass service (Go, SQLite)            │
│  ┌────────────────┐  │  HTTPS  │  ┌─────────────────────────────────┐  │
│  │ @emdzej/       │  │◄───────►│  │  REST  /v1/sync, /v1/pair/*     │  │
│  │ bass-client    │  │  WSS    │  │  WS    /v1/changes              │  │
│  │                │◄─┼─────────┼─►│  Discovery /.well-known/bass-…  │  │
│  │ ┌────────────┐ │  │         │  └─────────────────────────────────┘  │
│  │ │ localStorage│ │  │         │  ┌─────────────────────────────────┐  │
│  │ │  ↕ outbox   │ │  │         │  │ OIDC verifier (control plane)   │  │
│  │ └────────────┘ │  │         │  │ Opaque tokens   (data plane)    │  │
│  └────────────────┘  │         │  └─────────────────────────────────┘  │
└──────────────────────┘         └────────────┬──────────────────────────┘
                                              │
                                              ▼
                                  ┌───────────────────────┐
                                  │  Your OIDC provider   │
                                  │  (Keycloak, Authelia, │
                                  │   Auth0, Entra, …)    │
                                  └───────────────────────┘
```

**Two-plane auth.** Pairing and admin endpoints verify OIDC JWTs from your
IdP. The sync data plane uses opaque per-device tokens minted by the service
after a one-time pairing — short-lived, instantly revocable, never carry
identity claims.

## Quick start (Docker)

```sh
git clone https://github.com/emdzej/bass.git
cd bass
docker compose -f docker/docker-compose.yml up --build
```

This brings up three services:

| Service | URL | Purpose |
|---|---|---|
| `bass` | http://localhost:8080 | The sync service (runs in `--no-auth` mode for the demo) |
| `demo` | http://localhost:5173 | The bass demo Svelte app served by nginx |

The demo runs bass with OIDC disabled (`BASS_NO_AUTH=true`) so the focus stays
on the sync mechanism rather than IdP plumbing. For a real deployment you
swap that out for your own OIDC provider — see [For operators](#for-operators).

Then:

1. Register the demo app (one-shot):
   ```sh
   docker compose -f docker/docker-compose.yml run --rm setup
   ```
2. Open http://localhost:5173 — the demo app.
3. Click **Pair (redirect)** or **Pair (popup)**. In `--no-auth` mode pairing
   short-circuits past any IdP and mints tokens for a synthetic `dev-user`.
4. Open `/settings`, change values, open the page in a second tab or browser —
   watch them converge.
5. Open `/inspector` to see the outbox, cursor, and token state.

### Running locally without Docker

```sh
# Service (with OIDC bypassed for fast iteration)
go run ./cmd/bass serve --no-auth

# Demo app, second terminal
pnpm install
pnpm dev   # http://localhost:5173
```

In `--no-auth` mode the pairing flow short-circuits past the IdP and mints a
device for a synthetic `dev-user` — useful for client-lib work without
standing up an IdP. The bundled docker-compose stack uses this mode by default.

## For app developers

### `@emdzej/bass-client`

```sh
pnpm add @emdzej/bass-client
```

```ts
import { createBassClient } from '@emdzej/bass-client';

const bass = createBassClient({
  serviceUrl: 'https://bass.example.com',
  appId: 'my-app',
  keys: ['myapp-*'],     // optional, default ['*']
  debounceMs: 500,       // optional
});

// 1. Pair this device — call from a "sync settings" button.
//    Routes the user to your OIDC provider, then back to redirectUri.
await bass.pair({
  redirectUri: location.origin + '/sync-cb',
  mode: 'redirect',                  // or 'popup' — falls back to redirect
  deviceLabel: 'MacBook Chrome',
});

// 2. In your /sync-cb page, capture the tokens from the URL fragment:
bass.completePairingFromUrl();
//    For popup mode this also posts tokens to window.opener and closes.

// 3. On app boot, hydrate localStorage from the server before mounting UI.
if (bass.isPaired()) {
  await bass.hydrate({ timeoutMs: 2000 });
  //   Times out into local-cache-then-catch-up so a slow network never
  //   blocks the app indefinitely.
}

// 4. Pick how to sync:

// (a) Transparent proxy — keep using window.localStorage as usual.
bass.attachLocalStorageProxy();
localStorage.setItem('myapp-theme', 'dark');  // gets sync'd automatically

// (b) Manual API — explicit reads/writes.
await bass.set('myapp-theme', 'dark');
const v = bass.get('myapp-theme');
const unsub = bass.subscribe('myapp-theme', (v) => render(v));

// 5. Open the change channel so remote writes arrive live.
await bass.startNotifications();
```

**Local-first behaviour.** If `bass.isPaired()` is false, the proxy is a
no-op and the manual API just reads/writes `localStorage`. Your app works
fully offline / unauthenticated.

**The outbox.** Writes hit `localStorage` immediately for synchronous reads
*and* go to an outbox in `localStorage` (one entry per key, latest-wins).
The outbox drains on a debounce timer, on reconnect, and on `flush()`. So
chatty apps coalesce naturally, and writes survive a tab close.

### `@emdzej/bass-svelte`

```sh
pnpm add @emdzej/bass-svelte
```

Svelte stores backed by bass-synced keys:

```svelte
<script lang="ts">
  import { bassWritable, useBassAuth } from '@emdzej/bass-svelte';
  import { bass } from '$lib/bass';

  const theme = bassWritable(bass(), 'myapp-theme', 'light');
  const auth = useBassAuth(bass());
</script>

<p>paired: {$auth.isPaired}</p>

<select bind:value={$theme}>
  <option value="light">light</option>
  <option value="dark">dark</option>
</select>
```

The store reads from the local cache, writes go through the bass outbox.
When another device pushes a change, the store re-emits and any component
using `$theme` re-renders.

API:

| Function | Returns | Purpose |
|---|---|---|
| `bassWritable(bass, key, default)` | `Writable<string>` | Two-way sync. |
| `bassReadable(bass, key, default)` | `Readable<string>` | Read-only sync. |
| `useBassAuth(bass)` | `Readable<AuthState>` | Pairing / refresh / unpair events. |

## For operators

### Configuration

The service takes config from env vars (or matching `--flag`s):

| Variable | Default | Purpose |
|---|---|---|
| `BASS_ADDR` | `:8080` | Listen address |
| `BASS_DB_PATH` | `bass.db` | SQLite database path |
| `BASS_PUBLIC_BASE_URL` | `http://localhost:8080` | Public HTTP URL (used in discovery + pair callback) |
| `BASS_PUBLIC_WS_BASE_URL` | derived from base URL | Public WS URL |
| `BASS_OIDC_ISSUER` | *(required)* | OIDC issuer URL |
| `BASS_OIDC_AUDIENCE` | *(required)* | Expected JWT audience |
| `BASS_OIDC_CLIENT_ID` | *(required)* | OAuth client id used for the pairing code exchange |
| `BASS_OIDC_CLIENT_SECRET` | *(required)* | OAuth client secret |
| `BASS_TOKEN_TTL` | `24h` | Sync token lifetime |
| `BASS_REFRESH_TTL` | `720h` (30d) | Refresh token lifetime |
| `BASS_MAX_VALUE_BYTES` | `65536` | Max bytes per item |
| `BASS_MAX_BATCH_ITEMS` | `1024` | Max items per push |
| `BASS_ALLOWED_ORIGINS` | empty | CSV of CORS origin glob patterns |
| `BASS_TLS_CERT` / `BASS_TLS_KEY` | empty | Enable native TLS (otherwise run behind a TLS terminator) |
| `BASS_NO_AUTH` | `false` | **DEV ONLY** — disable OIDC verification |

Run migrations explicitly:

```sh
bass migrate --db /data/bass.db
```

`bass serve` runs them automatically on startup, but the explicit command is
useful in init containers or when verifying database state.

### Registering apps

Apps are registered by an admin (a user with `bass.admin` scope on their
OIDC token).

```sh
curl -X POST $BASS_URL/v1/admin/apps \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-app",
    "name": "My App",
    "origins": ["https://my-app.example.com"],
    "redirect_uris": ["https://my-app.example.com/sync-cb"],
    "key_allowlist": ["myapp-*"]
  }'
```

- **`origins`** — exact-match Origin headers permitted to make CORS calls.
- **`redirect_uris`** — exact-match redirect URIs permitted in the pairing flow.
- **`key_allowlist`** — server-side cap. Glob patterns (`*` matches any
  character sequence). Writes to keys outside this cap return `403
  forbidden_key`. Default `["*"]`.

Use the Bruno collection (`bruno/`) for a click-through admin UI.

### OIDC setup

bass uses two scopes:

- **`bass.sync`** — granted to end users. Required for pairing.
- **`bass.admin`** — granted to administrators. Required for app
  registration and admin device management.

Map these to groups / roles in your IdP. Example for Keycloak: create two
client scopes (`bass.sync`, `bass.admin`), add them to the bass client, and
assign `bass.admin` to an admin group.

bass discovers OIDC config from the issuer's
`/.well-known/openid-configuration` automatically. Tokens are verified
against the configured audience (`BASS_OIDC_AUDIENCE`).

## Security model

| Concern | Mitigation |
|---|---|
| Sync token theft via XSS | Short TTL (24h default), per-device scope, instant revoke endpoint, refresh-token rotation with reuse detection. |
| Refresh token reuse | If the same refresh token is presented twice, the entire device chain is revoked. Standard OAuth2 RTR. |
| Cross-user data leak | Every query scoped by `user_sub` from the verified token. |
| Token in URL / logs | Tokens never appear in URLs or query strings. Pairing returns them in the URL **fragment** (not sent to the server). WS auth via `Sec-WebSocket-Protocol`. |
| Malicious app impersonation | App registration is admin-only. Origin and redirect_uri checks are exact-match. OIDC `state` parameter + PKCE on the code exchange. |
| Server reads my data | Not mitigated in MVP. Self-host trust model. Optional client-side encryption is reserved via the protocol's `payload_ver` byte. |

See [`SPEC.md` §4](./SPEC.md#4-threat-model) for the full threat model.

## Development

```sh
# Initial setup
go mod download
pnpm install

# Build everything
pnpm build              # turbo run build across all TS packages
go build ./cmd/bass     # Go binary

# Test
pnpm test               # vitest across TS packages
go test -race ./...     # Go tests with race detector

# Type / lint
pnpm typecheck
pnpm lint               # biome
go vet ./...

# Run dev server
go run ./cmd/bass serve --no-auth
pnpm dev                # demo app at :5173

# Smoke test the full E2E without docker
./scripts/smoke.sh      # (not yet created — see SPEC.md §15 follow-ups)
```

## Project layout

```
cmd/bass/                Go binary (serve, migrate, version)
internal/                Go service internals — apps, devices, pairing,
                         sync, changes (WS hub), discovery, auth, cors,
                         storage, httpx
packages/client/         @emdzej/bass-client (TypeScript)
packages/svelte/         @emdzej/bass-svelte (TypeScript)
apps/demo/               SvelteKit demo app
docker/                  Dockerfile + docker-compose for the no-auth demo
bruno/                   API testing collection
.github/workflows/       CI (js + go + docker) + release (npm + GHCR)
docs/                    Long-form docs (API.md)
SPEC.md                  Design rationale
```

## License

MIT
