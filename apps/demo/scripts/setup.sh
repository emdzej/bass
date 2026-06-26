#!/usr/bin/env bash
#
# Register the bass-demo app with a locally-running bass service.
#
# The demo runs bass in --no-auth mode (BASS_NO_AUTH=true), so the admin
# endpoint accepts requests without a bearer token. For a real deployment
# you would obtain an OIDC token with the bass.admin scope first.
#
# Usage from the docker-compose stack:
#   docker compose -f docker/docker-compose.yml run --rm setup
#
# Or from the host against a local `bass serve --no-auth`:
#   ./apps/demo/scripts/setup.sh
#
# Env overrides (defaults shown):
#   BASS_URL      http://localhost:8080
#   APP_ID        bass-demo
#   APP_NAME      bass demo
#   APP_ORIGIN    http://localhost:5173
#   APP_REDIRECT  http://localhost:5173/sync-cb
#   KEY_ALLOWLIST '["demo-*"]'

set -euo pipefail

BASS_URL="${BASS_URL:-http://localhost:8080}"
APP_ID="${APP_ID:-bass-demo}"
APP_NAME="${APP_NAME:-bass demo}"
APP_ORIGIN="${APP_ORIGIN:-http://localhost:5173}"
APP_REDIRECT="${APP_REDIRECT:-http://localhost:5173/sync-cb}"
KEY_ALLOWLIST="${KEY_ALLOWLIST:-[\"demo-*\"]}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo >&2 "error: required command '$1' not found in PATH"
    exit 1
  }
}
require_cmd curl
require_cmd python3

wait_for() {
  local url="$1"
  local label="$2"
  local printed=0
  for i in $(seq 1 60); do
    if curl -sf -o /dev/null "$url"; then
      [ "$printed" = "1" ] && printf '\n'
      return 0
    fi
    if [ "$i" -eq 1 ]; then
      printf 'waiting for %s' "$label"
      printed=1
    fi
    printf '.'
    sleep 1
  done
  printf '\n'
  echo >&2 "error: $label did not become ready at $url"
  exit 1
}

echo "▸ Waiting for bass..."
wait_for "$BASS_URL/healthz" "bass"
printf 'ready.\n'

echo "▸ Registering app '$APP_ID' with bass..."
REG_BODY=$(python3 -c "
import json
print(json.dumps({
  'id': '$APP_ID',
  'name': '$APP_NAME',
  'origins': ['$APP_ORIGIN'],
  'redirect_uris': ['$APP_REDIRECT'],
  'key_allowlist': $KEY_ALLOWLIST,
}))
")

HTTP_CODE=$(curl -s -o /tmp/bass-setup-resp.json -w '%{http_code}' \
  -X POST "$BASS_URL/v1/admin/apps" \
  -H "Content-Type: application/json" \
  -d "$REG_BODY")

case "$HTTP_CODE" in
  201)
    echo "✓ App registered:"
    python3 -m json.tool < /tmp/bass-setup-resp.json
    ;;
  409)
    echo "ℹ App '$APP_ID' already registered — nothing to do."
    ;;
  401|403)
    echo >&2 "✗ bass rejected the request (HTTP $HTTP_CODE):"
    python3 -m json.tool < /tmp/bass-setup-resp.json >&2 || cat /tmp/bass-setup-resp.json >&2
    echo >&2 "Hint: this demo expects BASS_NO_AUTH=true. If you've enabled OIDC,"
    echo >&2 "      get an admin token and call /v1/admin/apps with it manually."
    exit 1
    ;;
  *)
    echo >&2 "✗ unexpected HTTP $HTTP_CODE from bass:"
    cat /tmp/bass-setup-resp.json >&2
    exit 1
    ;;
esac

rm -f /tmp/bass-setup-resp.json
echo
echo "✓ Setup complete. Open http://localhost:5173 to use the demo."
