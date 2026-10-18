#!/usr/bin/env bash
# Checks the custody databases against the ledger: counts, anchors (recomputed and
# as committed), key halves, the serial owner's fleet and the HMAC keys. Non-zero on any mismatch.
#
# Usage: ./eval/custody-verify.sh [out-file]   (five-org network and custody containers up)
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../network/lib.sh"
set -a; . "$HERE/../offchain/custody/.env"; set +a
OUT="${1:-}"
FAIL=0
say() { echo "$*"; [[ -n "$OUT" ]] && echo "$*" >> "$OUT"; }
check() { if [[ "$2" == "$3" ]]; then say "  ok   $1"; else say "  FAIL $1 (got '$2', want '$3')"; FAIL=$((FAIL+1)); fi; }
dmt() { docker exec custody-dmt-db psql -U dmt -d dmt -tAc "$1"; }
pol() { docker exec custody-police-db psql -U police -d police -tAc "$1"; }
[[ -n "$OUT" ]] && : > "$OUT"

say "== Custody verification $(date -u +%Y-%m-%dT%H:%MZ)"
N=$(dmt "select count(*) from owner_record")
check "DMT key halves, one per owner record ($N)" "$(dmt 'select count(*) from key_share')" "$N"
check "Police key halves, one per owner record" "$(pol 'select count(*) from key_share')" "$N"
check "owner index entries, one per owner record" "$(dmt 'select count(*) from owner_index')" "$N"
check "every stored anchor recomputes from its own bytes" "$(dmt "select count(*) from owner_record where anchor_sha256 <> encode(sha256(set_byte('\\x00'::bytea, 0, envelope_version) || convert_to(vid, 'UTF8') || nonce || ciphertext), 'hex')")" "0"
POLICE_VIDS=$(pol "select vid from key_share order by vid")
check "the Police hold a half for exactly the DMT's records" "$POLICE_VIDS" "$(dmt 'select vid from owner_record order by vid')"
# Both custodians' parts must belong to the same owner record, or a reveal would fail to decrypt.
check "every Police half belongs to the DMT's current record (anchors equal)" \
  "$(pol "select vid || ':' || coalesce(anchor_sha256, 'none') from key_share order by vid")" \
  "$(dmt "select vid || ':' || anchor_sha256 from owner_record order by vid")"

say "== Stored anchors against the ledger (DMT view on peer0.org1)"
as dmt
MATCH=0; MISMATCH=0; MISSING=0
while IFS='|' read -r vid anchor; do
  [[ -z "$vid" ]] && continue
  onchain=$(peer chaincode query -C "$CHANNEL" -n "$CC" -c "{\"function\":\"VehicleRegistry:GetVehicleHistoryFor\",\"Args\":[\"DMT\",\"$vid\",\"\"]}" 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["record"].get("envelopeAnchor",""))' 2>/dev/null)
  if [[ -z "$onchain" ]]; then MISSING=$((MISSING+1)); say "  not on the ledger: ${vid:0:12}…"
  elif [[ "$onchain" == "$anchor" ]]; then MATCH=$((MATCH+1))
  else MISMATCH=$((MISMATCH+1)); say "  MISMATCH ${vid:0:12}…: stored ${anchor:0:12}…, ledger ${onchain:0:12}…"; fi
done < <(dmt "select vid || '|' || anchor_sha256 from owner_record order by vid")
say "  matched $MATCH, mismatched $MISMATCH, not on the ledger $MISSING"
check "no stored anchor differs from the ledger" "$MISMATCH" "0"
check "every stored record was found on the ledger (an unreachable ledger fails here)" "$MISSING" "0"
for v in 8c1a95cc98e195603fff56dd699ec7a2bfe6a62fddbbf78a2ec0cca64d565417 5da5983b12a886e257fa2afac0a3177c859040cc05fee289a1f9e74f2fb6a91c \
         1339a27a6ffb3006e62bc93d78fbbb9f4286ad6e108bb35e2282d5704fb3ea4e fcbdf05b13ad3c026a6c4c6f36f6dc79b1da856ca97bef4e044eb57199cc3733; do
  check "demo vehicle ${v:0:8}… has a stored record" "$(dmt "select count(*) from owner_record where vid = '$v'")" "1"
done

say "== Owner index and keys"
FLEET=$(curl -s -H "Authorization: Bearer $CUSTODY_TOKEN_DMT" localhost:8101/records/5da5983b12a886e257fa2afac0a3177c859040cc05fee289a1f9e74f2fb6a91c/fleet \
  | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["fleet"]))')
check "the serial owner's fleet (R1–R3) is three vehicles" "$FLEET" "3"
HANDLE=$(curl -s -X POST -H "Authorization: Bearer $CUSTODY_TOKEN_DMT" -H 'content-type: application/json' localhost:8101/handles \
  -d '{"chassis":"NZE161-3150001"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["vid"])')
check "the DMT service's keys give S1 its handle" "$HANDLE" "8c1a95cc98e195603fff56dd699ec7a2bfe6a62fddbbf78a2ec0cca64d565417"

say "== Plates and descriptions (v1.8, DMT database only)"
check "the four demo vehicles each have a description" "$(dmt "select count(*) from vehicle_description where vid in ('8c1a95cc98e195603fff56dd699ec7a2bfe6a62fddbbf78a2ec0cca64d565417','5da5983b12a886e257fa2afac0a3177c859040cc05fee289a1f9e74f2fb6a91c','1339a27a6ffb3006e62bc93d78fbbb9f4286ad6e108bb35e2282d5704fb3ea4e','fcbdf05b13ad3c026a6c4c6f36f6dc79b1da856ca97bef4e044eb57199cc3733')")" "4"
check "no plate is open on two vehicles" "$(dmt "select count(*) from (select plate from plate_history where to_day is null group by plate having count(*) > 1) d")" "0"
PLATE_VID=$(curl -s -X POST -H "Authorization: Bearer $CUSTODY_TOKEN_DMT" -H 'content-type: application/json' localhost:8101/plates/lookup \
  -d '{"plate":"CAB-1234"}' | python3 -c 'import json,sys; print(json.load(sys.stdin).get("vid"))')
check "S1's plate CAB-1234 leads to S1" "$PLATE_VID" "8c1a95cc98e195603fff56dd699ec7a2bfe6a62fddbbf78a2ec0cca64d565417"

say "custody-verify: $FAIL failed"
[[ $FAIL -eq 0 ]]
