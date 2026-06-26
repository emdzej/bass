# Docker setup

The compose stack runs bass in **`--no-auth` mode** so the demo stays
focused on the sync mechanism rather than OIDC plumbing. For a real
deployment, drop `BASS_NO_AUTH` and set `BASS_OIDC_*` env vars to point at
your own IdP.

## Start the stack

```sh
docker compose -f docker/docker-compose.yml up -d --build
```

| Service | URL | Purpose |
|---|---|---|
| `bass` | http://localhost:8080 | The sync service (auth disabled) |
| `demo` | http://localhost:5173 | The bass demo Svelte app, served by nginx |

## Register the demo app

The bass service starts with an empty app registry. Register the demo
app once with the one-shot `setup` service:

```sh
docker compose -f docker/docker-compose.yml run --rm setup
```

This sends a single `POST /v1/admin/apps` request. Because bass is in
`--no-auth` mode it accepts the request without a token. Idempotent —
repeat runs are no-ops.

You can also run it against a local `bass serve --no-auth` instead of
the compose stack:

```sh
./apps/demo/scripts/setup.sh
```

## Use the demo

Open http://localhost:5173. Click **Pair (redirect)** or **Pair (popup)** —
in `--no-auth` mode pairing short-circuits past any IdP and mints
tokens for a synthetic `dev-user`. Open `/settings`, change values,
open the page in a second tab or browser — watch them converge.

## Going beyond the demo

For a real deployment:

1. Remove `BASS_NO_AUTH: "true"` from `docker-compose.yml`.
2. Add the OIDC env vars (see [main README](../README.md#configuration)).
3. Pre-register apps via `bass /v1/admin/apps` using a token from your IdP
   that carries the `bass.admin` scope. The Bruno collection in
   [`../bruno/`](../bruno/) has ready-made requests.
4. Make sure the `demo`'s `VITE_BASS_URL` build arg points at the public
   URL of your bass instance, not `localhost`.
