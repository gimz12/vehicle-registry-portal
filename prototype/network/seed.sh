#!/usr/bin/env bash
# Seeds the smoke-test fleet: two vehicles of one owner, each with a rollback, an
# accident and a claim by the same assessor (F6b), plus a duplicate-anchor claim pair (F3c).
# Usage: ./network/seed.sh   (network up, chaincode deployed, roles registered)
set -euo pipefail
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

# invoke Contract:Function json: endorsed by the caller's own peer plus two others.
invoke() {
  local fn="$1" raw="$2"
  local args; args=$(python3 -c 'import json,sys; print(json.dumps({"function": sys.argv[1], "Args": [sys.argv[2]]}))' "$fn" "$raw")
  raw_invoke "$fn" "$args"
}
raw_invoke() { # raw_invoke label json-payload
  peer_args $(endorsers_for)
  peer chaincode invoke "${ORDERER_ARGS[@]}" -C "$CHANNEL" -n "$CC" "${PEER_ARGS[@]}" \
    --waitForEvent -c "$2" 2>&1 | grep -E "status:|Error|error" | sed "s/^/  [$1] /"
}

VID1=3333333333333333333333333333333333333333333333333333333333333333
VID2=4444444444444444444444444444444444444444444444444444444444444444
VID3=5555555555555555555555555555555555555555555555555555555555555555
CH1=$(printf 'a%.0s' {1..64}); EN1=$(printf 'b%.0s' {1..64})
CH2=$(printf 'c%.0s' {1..64}); EN2=$(printf 'd%.0s' {1..64})
CH3=$(printf 'f%.0s' {1..64}); EN3=$(printf '1%.0s' {1..64})
STORE="${OFFCHAIN_DIR:-offchain/dev-store}"
NIC_NIMAL="751111111V"; NIC_KAMALA="198298765432"
export GOCACHE="${GOCACHE:-/tmp/rn-bol-go-cache}"
# Sealing is not idempotent, so seal into a staging store and move records in only
# after the registrations commit; a refused or repeated run leaves $STORE untouched.
STAGE=$(mktemp -d); trap 'rm -rf "$STAGE"' EXIT
cp "$STORE/registry-keys.json" "$STAGE/"

# Synthetic owner records are sealed off-ledger; only the anchor goes on-chain.
echo "== Sealing synthetic owner records (staged until the ledger commits)"
A1=$(go run ./cmd/seal -store "$STAGE" -vid "$VID1" -name "Nimal Perera (synthetic)" -nic "$NIC_NIMAL" -address "No. 12, Temple Road, Kandy")
A2=$(go run ./cmd/seal -store "$STAGE" -vid "$VID2" -name "Nimal Perera (synthetic)" -nic "$NIC_NIMAL" -address "No. 12, Temple Road, Kandy")
A3=$(go run ./cmd/seal -store "$STAGE" -vid "$VID3" -name "Kamala Silva (synthetic)" -nic "$NIC_KAMALA" -address "45/2, Galle Road, Colombo 3")
echo "  anchors: ${A1:0:12}… ${A2:0:12}… ${A3:0:12}…"
echo "== Owner index (DMT custody) and per-vehicle pseudonyms"
P1=$(go run ./cmd/ownerindex -store "$STAGE" -nic "$NIC_NIMAL" -vid "$VID1")
P2=$(go run ./cmd/ownerindex -store "$STAGE" -nic "$NIC_NIMAL" -vid "$VID2")
P3=$(go run ./cmd/ownerindex -store "$STAGE" -nic "$NIC_KAMALA" -vid "$VID3")
echo "  pseudonyms: ${P1:0:12}… ${P2:0:12}… ${P3:0:12}… (all different)"

echo "== Customs imports"
as customs1
invoke VehicleRegistry:CreateImportAttestation "{\"recordId\":\"import-1\",\"idxChassis\":\"$CH1\",\"idxEngine\":\"$EN1\",\"source\":\"CUSTOMS\"}"
invoke VehicleRegistry:CreateImportAttestation "{\"recordId\":\"import-2\",\"idxChassis\":\"$CH2\",\"idxEngine\":\"$EN2\",\"source\":\"CUSTOMS\"}"
invoke VehicleRegistry:CreateImportAttestation "{\"recordId\":\"import-3\",\"idxChassis\":\"$CH3\",\"idxEngine\":\"$EN3\",\"source\":\"CUSTOMS\"}"

echo "== DMT registrations (one owner, two vehicles, two unlinkable pseudonyms)"
as User1
invoke VehicleRegistry:RegisterVehicle "{\"vid\":\"$VID1\",\"pseudonym\":\"$P1\",\"idxChassis\":\"$CH1\",\"idxEngine\":\"$EN1\",\"provenance\":\"CUSTOMS\",\"envelopeAnchor\":\"$A1\"}"
invoke VehicleRegistry:RegisterVehicle "{\"vid\":\"$VID2\",\"pseudonym\":\"$P2\",\"idxChassis\":\"$CH2\",\"idxEngine\":\"$EN2\",\"provenance\":\"CUSTOMS\",\"envelopeAnchor\":\"$A2\"}"
invoke VehicleRegistry:RegisterVehicle "{\"vid\":\"$VID3\",\"pseudonym\":\"$P3\",\"idxChassis\":\"$CH3\",\"idxEngine\":\"$EN3\",\"provenance\":\"CUSTOMS\",\"envelopeAnchor\":\"$A3\"}"

echo "== Registrations committed: moving the sealed records and owner-index entries into $STORE"
for v in "$VID1" "$VID2" "$VID3"; do
  for d in owner_record custody_dmt custody_police; do mkdir -p "$STORE/$d"; mv "$STAGE/$d/$v.json" "$STORE/$d/"; done
done
go run ./cmd/ownerindex -store "$STORE" -nic "$NIC_NIMAL" -vid "$VID1" >/dev/null
go run ./cmd/ownerindex -store "$STORE" -nic "$NIC_NIMAL" -vid "$VID2" >/dev/null
go run ./cmd/ownerindex -store "$STORE" -nic "$NIC_KAMALA" -vid "$VID3" >/dev/null

echo "== VET mileage (rollback on both vehicles)"
as vet1
for v in "$VID1" "$VID2"; do
  invoke VehicleRegistry:AppendMileage "{\"recordId\":\"mileage-a-${v:0:4}\",\"vid\":\"$v\",\"day\":0,\"odometer\":80000,\"vetLaneId\":\"lane-1\"}"
  invoke VehicleRegistry:AppendMileage "{\"recordId\":\"mileage-b-${v:0:4}\",\"vid\":\"$v\",\"day\":200,\"odometer\":60000,\"vetLaneId\":\"lane-1\"}"
done

echo "== Police accident reports on vehicles 1 and 2"
as police1
invoke VehicleRegistry:AppendAccident "{\"recordId\":\"police-report-1\",\"vid\":\"$VID1\",\"day\":500}"
invoke VehicleRegistry:AppendAccident "{\"recordId\":\"police-report-2\",\"vid\":\"$VID2\",\"day\":610}"

echo "== Insurer claims: duplicate anchor on vehicle 1 (F3c); one assessor across both vehicles (F6b)"
as insurer1
invoke VehicleRegistry:AppendClaim "{\"recordId\":\"claim-1\",\"vid\":\"$VID1\",\"day\":505,\"evidenceAnchor\":\"police-report-1\",\"evidenceKind\":\"POLICE_REPORT\",\"insurerId\":\"insurer-a\",\"attestorId\":\"assessor-77\"}"
as insurer2
invoke VehicleRegistry:AppendClaim "{\"recordId\":\"claim-2\",\"vid\":\"$VID1\",\"day\":509,\"evidenceAnchor\":\"police-report-1\",\"evidenceKind\":\"POLICE_REPORT\",\"insurerId\":\"insurer-b\"}"
as insurer1
invoke VehicleRegistry:AppendClaim "{\"recordId\":\"claim-3\",\"vid\":\"$VID2\",\"day\":612,\"evidenceAnchor\":\"police-report-2\",\"evidenceKind\":\"POLICE_REPORT\",\"insurerId\":\"insurer-a\",\"attestorId\":\"assessor-77\"}"

echo "== Vehicle 3: written off by the insurer, then retired by the DMT"
as insurer1
raw_invoke SetWriteOffStatus "{\"function\":\"VehicleRegistry:SetWriteOffStatus\",\"Args\":[\"$VID3\",\"true\"]}"
as User1
raw_invoke SetRetiredStatus "{\"function\":\"VehicleRegistry:SetRetiredStatus\",\"Args\":[\"$VID3\",\"true\"]}"

echo "== done. Portal: http://localhost:3000/vehicle/$VID1  (retired vehicle: $VID3; its chassis ${CH3:0:8}… is free to try re-registering at the desk)"
