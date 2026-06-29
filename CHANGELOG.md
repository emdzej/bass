# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A single version tag (`vX.Y.Z`) drives releases of every artifact in the
monorepo — both npm packages (`@emdzej/bass-client`, `@emdzej/bass-svelte`)
and the `ghcr.io/emdzej/bass` container image.

---

## [0.1.0] — 2026-06-29

Initial public release. Backendless app state synchronization — opt-in,
self-hostable sync of `localStorage` across a user's devices.

### bass service (Go)

- **HTTP API** (`/v1/...`) for pairing, sync push/pull, device management,
  app registry, token refresh, and a public `/.well-known/bass-config`
  discovery endpoint.
- **WebSocket** (`/v1/changes`) notification channel — change events only,
  payloads still fetched via the REST sync endpoint.
- **OIDC pairing** as a public OAuth2 client using PKCE. Verifies tokens
  against the configured issuer's JWKS; supports the `bass.sync` and
  `bass.admin` scopes for end-user pairing and admin operations.
- **Last-write-wins sync** with a monotonic per-(user, app) version
  counter; `accepted_overwrite` status returned when a stale client wins.
- **Storage**: SQLite via `modernc.org/sqlite` (CGO-free), with embedded
  migrations through `golang-migrate`.
- **Token model**: opaque per-device sync + refresh tokens, SHA-256 at
  rest, constant-time compare; refresh-token rotation with reuse detection.
- **`--no-auth` dev mode** that short-circuits the OIDC dance and mints a
  device for a synthetic `dev-user` — used by the docker-compose demo.
- **Resilience**: OIDC discovery retry with exponential backoff (~30s)
  on startup so the service comes up cleanly alongside an IdP.

### `@emdzej/bass-client` (npm)

- **Manual KV API**: `bass.get / set / delete / subscribe`.
- **Transparent localStorage proxy** (`bass.attachLocalStorageProxy()`)
  that forwards writes matching a configurable pattern allowlist into
  the outbox without changes to host-app code.
- **Pairing flow** in both **redirect** and **popup** modes, sharing a
  single `/sync-cb` route in the host app. Popup mode falls back to
  redirect when `window.open` is blocked.
- **Debounced outbox** persisted in `localStorage`; chatty hosts coalesce
  by key, writes survive a tab close.
- **Blocking hydration** with configurable timeout
  (`bass.hydrate({ timeoutMs: 2000 })`) to avoid stale-paint flicker.
- **WS notification channel** with exponential-backoff reconnect; falls
  back to polling if the handshake is blocked by a proxy.
- ESM-only build, browser-targeted. Zero runtime dependencies.

### `@emdzej/bass-svelte` (npm)

- **`bassWritable(bass, key, default)`** — Svelte writable store backed
  by a synced key; assignments push through the outbox; remote changes
  re-emit. Echo-loop protected.
- **`bassReadable(bass, key, default)`** — read-only sync view.
- **`useBassAuth(bass)`** — Svelte readable store of the current auth
  state; re-emits on pair / refresh / unpair.
- Peer deps on `svelte ^5` and `@emdzej/bass-client`. ESM-only build.

### `@emdzej/bass-demo` (private, in repo)

- SvelteKit (Svelte 5 + TS) demo app exercising every public surface of
  the client + svelte adapter: pairing, settings (side-by-side
  `bassWritable` vs manual API), device list, and a live outbox/cursor
  inspector.
- Ships as the docker-compose `demo` service, served by nginx as a static
  bundle.

### Infrastructure & tooling

- **Docker**: multi-stage `Dockerfile` produces a static binary on
  distroless. `docker-compose.yml` runs bass (with `BASS_NO_AUTH=true`)
  + demo + a one-shot `setup` profile that registers the demo app via
  `POST /v1/admin/apps`.
- **GitHub Actions**:
  - `ci.yml` runs lint / typecheck / test / build for both JS (turbo +
    pnpm) and Go (golangci-lint + race-tested), plus a Docker build.
  - `release.yml` (on `v*` tags) publishes npm packages with provenance
    and pushes multi-arch (`linux/amd64,linux/arm64`) images to GHCR.
  - `docker-publish.yml` (manual `workflow_dispatch`) pushes a
    one-off-tagged image to GHCR on demand.
- **Bruno API collection** under `bruno/`:
  - Folder-level auth across all five endpoint folders (OAuth2 with
    PKCE for `admin/`; bearer for `sync/` + `devices/`; none for
    `discovery/` + `pairing/`).
  - Three environments shipped: `local`, `solvely`, `self-hosted`.
  - `~`-prefixed `vars:secret` lists keep sync/refresh tokens out of git.

### Documentation

- `README.md` — user/operator quick start, configuration, security model.
- `SPEC.md` — design rationale, threat model, full protocol, deferred items.
- `docs/API.md` — detailed REST + WS reference, status semantics.
- Per-package READMEs for npmjs.com display.

### Known limitations / explicitly deferred to a future version

- IndexedDB sync (localStorage only for now).
- Client-side end-to-end encryption (protocol reserves a `payload_ver`
  byte for it).
- Per-user / per-app quotas.
- Tombstone garbage collection.
- Server-side payload pushes over WS (current design: notifications only).
- Browser-driven `bassState` runes API in `@emdzej/bass-svelte` (Svelte
  stores ship; runes deferred until there's a real consumer asking).

[0.1.0]: https://github.com/emdzej/bass/releases/tag/v0.1.0
