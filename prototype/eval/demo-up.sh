#!/usr/bin/env bash
# Starts the Fabric network, the custody services, a custody check, then the portal.
# Safe to re-run; it starts only what is not already running.
# Usage: ./eval/demo-up.sh            (stop with ./eval/demo-down.sh)
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
. "$ROOT/network/lib.sh"
started=$(date +%s)

echo "== Fabric: five peers, three orderers, six CAs"
if [[ ! -f "$TN/five-org/compose.yaml" ]]; then
  echo "No five-organisation network found. Build it once: ./network/five-org/up.sh, deploy.sh, register-roles.sh, seed.sh" >&2; exit 1
fi
docker compose -f "$TN/five-org/compose.yaml" start >/dev/null 2>&1 || DOCKER_SOCK=/var/run/docker.sock docker compose -f "$TN/five-org/compose.yaml" up -d >/dev/null
for i in $(seq 1 30); do
  hs=$(for p in 9444 9445 9449 9450 9451; do curl -s "localhost:$p/metrics" | sed -n "s/^ledger_blockchain_height{channel=\"$CHANNEL\"} //p"; done | sort -u)
  [[ $(echo "$hs" | grep -c .) == 1 && -n "$hs" ]] && break; sleep 2
done
echo "  ledger height on all five peers: ${hs:-not ready}"

echo "== Custody: five services, five databases"
docker compose -f "$ROOT/offchain/custody/docker-compose.yml" up -d --wait >/dev/null 2>&1 || { echo "custody containers did not become healthy; see docker compose logs" >&2; exit 1; }
"$HERE/custody-verify.sh" | tail -3 || { echo "custody verification failed; see eval/custody-verify.sh" >&2; exit 1; }

echo "== Portal"
if ! lsof -nP -iTCP:3000 -sTCP:LISTEN >/dev/null 2>&1; then
  # Detach fully, so a piped caller (demo-up.sh | tee) is not held open by the portal.
  (cd "$ROOT" && nohup yarn dev &) > /tmp/rn-portal.log 2>&1 < /dev/null
fi
for i in $(seq 1 60); do
  up=$(curl -s localhost:3000/api/health | python3 -c 'import json,sys; print(json.load(sys.stdin)["custody"]["up"])' 2>/dev/null) && [[ $up == 5 ]] && break; sleep 2
done
echo "  portal on :3000, custody services up: ${up:-0} of 5"
echo "== Ready in $(( $(date +%s) - started )) s. Tabs: see documents/DEMO_GUIDE.md"
echo "  http://localhost:3000/live    http://localhost:3000/desk    http://localhost:3000/police/5da5983b12a886e257fa2afac0a3177c859040cc05fee289a1f9e74f2fb6a91c"
[[ ${up:-0} == 5 ]]
