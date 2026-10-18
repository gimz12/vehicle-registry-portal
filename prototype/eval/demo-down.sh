#!/usr/bin/env bash
# Stops the portal and the custody services (their databases keep their data:
# stop, never down -v). With --all, also stops the Fabric network (ledger kept).
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$ROOT/network/lib.sh"
# Every dev portal this checkout started, not only the one on :3000 (a restart can leave one on :3001).
pkill -f "$ROOT/node_modules/nuxt/bin/nuxt.mjs dev" || true
pkill -f 'node .*/yarn dev' || true
lsof -ti tcp:3000 -sTCP:LISTEN | xargs -r kill
docker compose -f "$ROOT/offchain/custody/docker-compose.yml" stop
[[ "${1:-}" == --all ]] && docker compose -f "$TN/five-org/compose.yaml" stop
echo "stopped${1:+ (all)}"
