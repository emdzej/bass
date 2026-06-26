# Docker setup

`docker compose -f docker/docker-compose.yml up` brings up a full local stack:

| Service | URL | Purpose |
|---|---|---|
| `dex` | http://localhost:5556/dex | Local OIDC IdP (single static user: `demo@example.com` / `demo`) |
| `bass` | http://localhost:8080 | The sync service |
| `demo` | http://localhost:5173 | The bass demo Svelte app served by nginx |

First-run setup (admin registers the demo app — requires a token with `bass.admin`,
or running bass with `BASS_NO_AUTH=true` for first registration):

```sh
curl -X POST http://localhost:8080/v1/admin/apps \
  -H "Content-Type: application/json" \
  -d '{
    "id": "bass-demo",
    "name": "bass demo",
    "origins": ["http://localhost:5173"],
    "redirect_uris": ["http://localhost:5173/sync-cb"],
    "key_allowlist": ["demo-*"]
  }'
```

See `bruno/` for ready-made API requests.
