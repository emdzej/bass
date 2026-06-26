# @emdzej/bass-demo

SvelteKit + TypeScript demo app for [bass](https://github.com/emdzej/bass).
Not published.

Exercises every public surface of `@emdzej/bass-client` and
`@emdzej/bass-svelte` so:

1. App authors have a copy-pasteable integration reference.
2. The client lib has an end-to-end test target during development.

## Pages

| Route | What it shows |
|---|---|
| `/` | Pairing buttons (redirect + popup), unpair, status |
| `/settings` | Side-by-side: Svelte adapter (`bassWritable`) vs. manual API |
| `/devices` | Lists paired devices, supports revoke |
| `/inspector` | Live view of outbox, cursor, token state |
| `/sync-cb` | OAuth callback target — calls `completePairingFromUrl()` |

## Run

```sh
# from the repo root
pnpm dev
```

Defaults to `http://localhost:5173`, talks to bass at `http://localhost:8080`.
Override via env:

```
VITE_BASS_URL=https://bass.example.com
VITE_APP_ID=my-app
```
