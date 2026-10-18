#!/usr/bin/env bash
# Writes offchain/custody/.env (git-ignored): tokens, database passwords and the
# existing HMAC keys, so every handle stays the same. Only appends missing lines.
# Then writes portal.env with the portal's own tokens only.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV="$HERE/.env"
PORTAL="$HERE/portal.env"
KEYS="${1:-$HERE/../dev-store/registry-keys.json}"
umask 077

has() { [[ -f "$ENV" ]] && grep -q "^$1=" "$ENV"; }
add() { has "$1" || { echo "$1=$2" >> "$ENV"; echo "  added $1"; }; }

if [[ ! -f "$ENV" ]]; then
  KV=$(python3 -c 'import json,sys; k=json.load(open(sys.argv[1])); print(k["kv"]+":"+k["kown"])' "$KEYS")
  echo "# Generated $(date -u +%Y-%m-%dT%H:%MZ) by setup-env.sh. Never commit this file." > "$ENV"
  for o in DMT POLICE CUSTOMS VET INSURER; do
    add "CUSTODY_TOKEN_$o" "$(openssl rand -hex 32)"
    add "DB_PASSWORD_$o" "$(openssl rand -hex 16)"
  done
  add REGISTRY_KEYS "$KV"
fi
# Service to service; neither goes to the portal.
add CUSTODY_TOKEN_DMT_TO_POLICE "$(openssl rand -hex 32)"
add CUSTODY_TOKEN_POLICE_TO_DMT "$(openssl rand -hex 32)"
# Desk keying at the DMT service: chassis to handle only, never a NIC to a pseudonym.
for o in POLICE CUSTOMS VET INSURER; do
  add "CUSTODY_TOKEN_${o}_KEYING" "$(openssl rand -hex 32)"
done
# The buyer page: a vehicle's description and the one-bit chassis match only.
add CUSTODY_TOKEN_BUYER_PORTAL "$(openssl rand -hex 32)"
echo "wrote $ENV"

grep -E '^CUSTODY_TOKEN_(DMT|POLICE|CUSTOMS|VET|INSURER|POLICE_KEYING|CUSTOMS_KEYING|VET_KEYING|INSURER_KEYING|BUYER_PORTAL)=' "$ENV" > "$PORTAL.tmp"
mv "$PORTAL.tmp" "$PORTAL"
echo "wrote $PORTAL ($(wc -l < "$PORTAL" | tr -d ' ') tokens; no service-to-service credential, password or key)"
