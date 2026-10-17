#!/usr/bin/env bash
# Minimum-coalition table on the live five-organisation network (MAJORITY = 3 of 5):
# for each insider goal, too few identities (refused) and the smallest group that
# succeeds (committed). Also chaincode-level access checks the HTTP tests cannot make.
#
# Usage: ./eval/coalition.sh [out-dir]   (five-org network up, chaincode deployed, roles registered)
set -uo pipefail
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../network/lib.sh"
OUT="${1:-eval/results/2026-10-03-ext/coalition-5org}"
mkdir -p "$OUT"; LOG="$OUT/transcripts.txt"; : > "$LOG"
ROWS=(); ACCESS=(); WHO=""; LAST=""; MISMATCH=0

payload() { python3 -c 'import json,sys; print(json.dumps({"function": sys.argv[1], "Args": sys.argv[2:]}))' "$@"; }
invoke() { # invoke maj|<org,org,...> Contract:Function args...   (maj = caller's peer + two more)
  local peers="$1"; shift
  local args; args=$(payload "$@")
  if [[ $peers == maj ]]; then peer_args $(endorsers_for); else peer_args ${peers//,/ }; fi
  LAST=$(peer chaincode invoke "${ORDERER_ARGS[@]}" -C "$CHANNEL" -n "$CC" "${PEER_ARGS[@]}" --waitForEvent -c "$args" 2>&1)
  printf '### %s invoke %s (endorsed by %s)\n%s\n%s\n\n' "$WHO" "$1" "${peers/maj/$(endorsers_for | tr ' ' ,)}" "$args" "$LAST" >> "$LOG"
}
query() {
  local args; args=$(payload "$@")
  LAST=$(peer chaincode query -C "$CHANNEL" -n "$CC" -c "$args" 2>&1)
  printf '### %s query %s\n%s\n%s\n\n' "$WHO" "$1" "$args" "$LAST" >> "$LOG"
}
outcome() {
  if [[ $LAST == *"ENDORSEMENT_POLICY_FAILURE"* ]]; then echo "refused: endorsement policy failure"
  elif [[ $LAST == *"committed with status (VALID)"* ]]; then echo "committed"
  elif [[ $LAST =~ (registry: [^\"]*) ]]; then echo "refused: ${BASH_REMATCH[1]}"
  elif [[ $LAST == *"not found in contract"* ]]; then echo "refused: function does not exist"
  elif [[ $LAST == *"Error"* || $LAST == *"error"* ]]; then echo "error: $(echo "$LAST" | tail -1 | cut -c1-140)"
  else echo "read"; fi
}
# judge <wanted prefix>: sets GOT to the outcome, flagged and counted when it differs.
# Runs in the current shell (never inside $(...)), so the MISMATCH count survives.
judge() {
  GOT=$(outcome)
  [[ $GOT == "$1"* ]] || { MISMATCH=$((MISMATCH+1)); GOT="$GOT **(expected: $1)**"; }
}
row() { judge "$4"; ROWS+=("| $1 | $2 | $3 | $GOT |"); }   # row goal attempt identities expected
access() { judge "$3"; ACCESS+=("| $1 | $2 | $GOT |"); }   # access check function expected
hex() { python3 -c 'import hashlib,sys; print(hashlib.sha256(sys.argv[1].encode()).hexdigest())' "$1"; }

R=$RANDOM$RANDOM
for n in 1 2 3 4; do
  eval "V$n=$(hex "coal-v$n-$R") CH$n=$(hex "coal-ch$n-$R") EN$n=$(hex "coal-en$n-$R") A$n=$(hex "coal-a$n-$R") P$n=$(hex "coal-p$n-$R")"
done
reg() { # reg n [engine] [provenance] : register vehicle n with its own pseudonym
  local v ch en a p; eval "v=\$V$1 ch=\$CH$1 en=\$EN$1 a=\$A$1 p=\$P$1"
  en="${2:-$en}"
  invoke maj VehicleRegistry:RegisterVehicle "{\"vid\":\"$v\",\"pseudonym\":\"$p\",\"idxChassis\":\"$ch\",\"idxEngine\":\"$en\",\"provenance\":\"${3:-CUSTOMS}\",\"envelopeAnchor\":\"$a\"}"
}
imp() { # imp chassis engine recordId [source]
  invoke maj VehicleRegistry:CreateImportAttestation "{\"recordId\":\"$3\",\"idxChassis\":\"$1\",\"idxEngine\":\"$2\",\"source\":\"${4:-CUSTOMS}\"}"
}

echo "== Goal: register a vehicle Customs never cleared (CIABOC pattern)"
as dmt; reg 1; row "Register a vehicle Customs never cleared (CIABOC)" "DMT officer alone" "1 DMT" refused
as customs1; imp "$CH1" "$EN1" "coal-imp1-$R"; as dmt; reg 1
row "Register a vehicle Customs never cleared (CIABOC)" "Customs officer records an import, then the DMT registers" "1 Customs + 1 DMT" committed

echo "== Goal: register a vehicle with no import record, claiming local assembly"
as dmt; imp "$CH4" "$EN4" "coal-asm-dmt-$R" ASSEMBLER
row "Register a vehicle with no import record by claiming local assembly" "DMT officer records an assembler attestation for itself" "1 DMT" refused
as assembler1; imp "$CH4" "$EN4" "coal-asm-$R" ASSEMBLER; as dmt; reg 4 "" ASSEMBLER
row "Register a vehicle with no import record by claiming local assembly" "A licensed assembler attests the build, then the DMT registers" "1 Assembler + 1 DMT (in the lab the assembler certificate is issued by the DMT organisation's CA)" committed

echo "== Goal: insert a second registration under an existing engine (COPA pattern)"
as customs1; imp "$CH2" "$EN1" "coal-imp2-$R"
row "Insert a second registration under an existing engine (COPA)" "Customs records an import for a new chassis with the registered engine" "1 Customs" committed
as dmt; reg 2 "$EN1"
row "Insert a second registration under an existing engine (COPA)" "DMT then registers it with the attested engine" "1 Customs + 1 DMT" refused
as dmt; reg 2
row "Insert a second registration under an existing engine (COPA)" "DMT registers it with a different engine from the one attested" "1 Customs + 1 DMT" refused

as customs1; imp "$(hex "coal-ch5-$R")" "$(echo "$EN1" | tr a-f A-F)" "coal-imp5-$R"
row "Insert a second registration under an existing engine (COPA)" "Customs records the registered engine in upper case (v1.5)" "1 Customs" refused

echo "== Goal: rewrite or delete a committed record"
as dmt; invoke maj VehicleRegistry:DeleteVehicle "$V1"
row "Rewrite or delete a committed record" "DMT officer calls a delete" "1 DMT" refused

echo "== Goal: block a sale with a false theft report (v1.8)"
as police1; invoke maj VehicleRegistry:SetTheftStatus "{\"recordId\":\"coal-theft0-$R\",\"vid\":\"$V1\",\"reference\":\"PR/COAL/$R\"}" true
row "Block a sale with a false theft report" "One Police officer flags it without the owner's confirmation (v1.8)" "1 Police" refused
as police1; invoke maj VehicleRegistry:AppendTheftReport "{\"recordId\":\"coal-report-$R\",\"vid\":\"$V1\",\"reference\":\"PR/COAL/$R\"}"
as police1; invoke maj VehicleRegistry:ConfirmReporter "{\"recordId\":\"coal-confirm-pol-$R\",\"vid\":\"$V1\",\"reference\":\"PR/COAL/$R\"}"
row "Block a sale with a false theft report" "The Police officer records a report and confirms the owner itself (v1.8)" "1 Police" refused
as dmt; invoke maj VehicleRegistry:ConfirmReporter "{\"recordId\":\"coal-confirm-$R\",\"vid\":\"$V1\",\"reference\":\"PR/COAL/$R\"}"
as police1; invoke maj VehicleRegistry:SetTheftStatus "{\"recordId\":\"coal-theft-$R\",\"vid\":\"$V1\",\"reference\":\"PR/COAL/$R\"}" true
row "Block a sale with a false theft report" "The DMT confirms the owner for the report, then the Police flag (v1.8)" "1 Police + 1 DMT" committed

echo "== Goal: sell a vehicle reported stolen"
as dmt; invoke maj VehicleRegistry:TransferVehicle "{\"recordId\":\"coal-tr1-$R\",\"vid\":\"$V1\",\"pseudonym\":\"$(hex "coal-new1-$R")\",\"envelopeAnchor\":\"$(hex "coal-na1-$R")\"}"
row "Sell a vehicle reported stolen" "DMT officer transfers it" "1 DMT" refused
as police1; invoke maj VehicleRegistry:SetTheftStatus "{\"recordId\":\"coal-clear-$R\",\"vid\":\"$V1\"}" false
as dmt; invoke maj VehicleRegistry:TransferVehicle "{\"recordId\":\"coal-tr2-$R\",\"vid\":\"$V1\",\"pseudonym\":\"$(hex "coal-new2-$R")\",\"envelopeAnchor\":\"$(hex "coal-na2-$R")\"}"
row "Sell a vehicle reported stolen" "One Police officer clears the report, then the DMT transfers" "1 Police + 1 DMT" committed

echo "== Goal: sell a written-off vehicle"
as insurer1; invoke maj VehicleRegistry:SetWriteOffStatus "$V1" true
as dmt; invoke maj VehicleRegistry:TransferVehicle "{\"recordId\":\"coal-tr3-$R\",\"vid\":\"$V1\",\"pseudonym\":\"$(hex "coal-new3-$R")\",\"envelopeAnchor\":\"$(hex "coal-na3-$R")\"}"
row "Sell a written-off vehicle" "DMT officer transfers it" "1 DMT" refused
as insurer1; invoke maj VehicleRegistry:SetWriteOffStatus "$V1" false
as dmt; invoke maj VehicleRegistry:TransferVehicle "{\"recordId\":\"coal-tr4-$R\",\"vid\":\"$V1\",\"pseudonym\":\"$(hex "coal-new4-$R")\",\"envelopeAnchor\":\"$(hex "coal-na4-$R")\"}"
row "Sell a written-off vehicle" "Insurer clears the write-off, then the DMT transfers" "1 Insurer + 1 DMT" committed

echo "== Goal: sell a vehicle still under finance"
as customs1; imp "$CH3" "$EN3" "coal-imp3-$R"; as dmt; reg 3
invoke maj VehicleRegistry:SetEncumbrance "{\"recordId\":\"coal-fin-$R\",\"vid\":\"$V3\",\"reference\":\"AO/$R\"}"
invoke maj VehicleRegistry:TransferVehicle "{\"recordId\":\"coal-tr5-$R\",\"vid\":\"$V3\",\"pseudonym\":\"$(hex "coal-new5-$R")\",\"envelopeAnchor\":\"$(hex "coal-na5-$R")\"}"
row "Sell a vehicle still under finance" "DMT officer transfers it" "1 DMT" refused
invoke maj VehicleRegistry:ReleaseEncumbrance "{\"recordId\":\"coal-rel-$R\",\"vid\":\"$V3\",\"reference\":\"DL/$R\"}"
invoke maj VehicleRegistry:TransferVehicle "{\"recordId\":\"coal-tr6-$R\",\"vid\":\"$V3\",\"pseudonym\":\"$(hex "coal-new6-$R")\",\"envelopeAnchor\":\"$(hex "coal-na6-$R")\"}"
row "Sell a vehicle still under finance" "The same DMT officer records a release, then transfers" "1 DMT (negative result: the lender is not on the network)" committed

echo "== Goal: sell a vehicle the register shows as scrapped"
as dmt; invoke maj VehicleRegistry:SetRetiredStatus "$V4" true
invoke maj VehicleRegistry:TransferVehicle "{\"recordId\":\"coal-tr7-$R\",\"vid\":\"$V4\",\"pseudonym\":\"$(hex "coal-new7-$R")\",\"envelopeAnchor\":\"$(hex "coal-na7-$R")\"}"
row "Sell a vehicle the register shows as scrapped" "DMT officer transfers it" "1 DMT" refused
invoke maj VehicleRegistry:SetRetiredStatus "$V4" false
invoke maj VehicleRegistry:TransferVehicle "{\"recordId\":\"coal-tr8-$R\",\"vid\":\"$V4\",\"pseudonym\":\"$(hex "coal-new8-$R")\",\"envelopeAnchor\":\"$(hex "coal-na8-$R")\"}"
row "Sell a vehicle the register shows as scrapped" "The same DMT officer returns it to the active register, then transfers" "1 DMT (negative result: retirement is a status any DMT identity can reverse)" committed

echo "== Goal: reveal an owner without a second agency"
as police1; invoke maj VehicleRegistry:AppendAccident "{\"recordId\":\"coal-rep-$R\",\"vid\":\"$V3\",\"day\":800}"
as insurer1
invoke maj VehicleRegistry:AppendClaim "{\"recordId\":\"coal-cl1-$R\",\"vid\":\"$V3\",\"day\":805,\"evidenceAnchor\":\"coal-rep-$R\",\"evidenceKind\":\"POLICE_REPORT\",\"insurerId\":\"insurer-a\"}"
invoke maj VehicleRegistry:AppendClaim "{\"recordId\":\"coal-cl2-$R\",\"vid\":\"$V3\",\"day\":809,\"evidenceAnchor\":\"coal-rep-$R\",\"evidenceKind\":\"POLICE_REPORT\",\"insurerId\":\"insurer-a\"}"
CASE="COAL/$R"
as police1; invoke maj IdentityManager:CreateRevealRequest "$CASE" "$V3" F3c COURT_ORDER "MC/$R"
invoke maj IdentityManager:ApproveReveal "$CASE"
row "Reveal an owner without a second agency" "The requesting Police officer approves their own case" "1 Police" refused
as police2; invoke maj IdentityManager:ApproveReveal "$CASE"; APPROVAL="$LAST"
query IdentityManager:GetRevealAudit "$CASE"
STATUS=$(echo "$LAST" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status","?"))' 2>/dev/null); LAST="$APPROVAL"
row "Reveal an owner without a second agency" "A second Police officer approves" "2 Police (case still pending: $STATUS)" committed
as dmt; invoke maj IdentityManager:ApproveReveal "$CASE"
row "Reveal an owner without a second agency" "The DMT approves as well" "2 Police + 1 DMT" committed
as police1; invoke maj IdentityManager:CreateRevealRequest "$CASE/2" "$V3" F3c COURT_ORDER "MC/$R/2"
as dmt; invoke maj IdentityManager:ApproveReveal "$CASE/2"; invoke maj IdentityManager:ApproveReveal "$CASE/2"
row "Re-attest an approved fleet while a case is pending" "The DMT approves the same pending case twice (v1.5)" "1 DMT" refused

echo "== Goal: write another agency's record type"
as vet1; invoke maj VehicleRegistry:SetTheftStatus "{\"recordId\":\"coal-vet-$R\",\"vid\":\"$V3\"}" true
row "Write another agency's record type" "VET centre reports a theft" "1 VET" refused

echo "== Goal: falsify an insurer, a theft record or a reading date (v1.5 gates)"
as insurer1; invoke maj VehicleRegistry:AppendClaim "{\"recordId\":\"coal-cl3-$R\",\"vid\":\"$V3\",\"day\":806,\"insurerId\":\"insurer-b\"}"
row "File a claim in another insurer's name" "insurer-a's certificate files as insurer-b (v1.5)" "1 Insurer" refused
as police1; invoke maj VehicleRegistry:SetTheftStatus "{\"recordId\":\"coal-clr0-$R\",\"vid\":\"$V3\"}" false
row "Clear a theft report that was never filed" "Police clears a vehicle not flagged stolen (v1.5)" "1 Police" refused
as vet1; invoke maj VehicleRegistry:AppendMileage "{\"recordId\":\"coal-m0-$R\",\"vid\":\"$V3\",\"day\":850,\"odometer\":11000,\"vetLaneId\":\"lane-1\"}"
invoke maj VehicleRegistry:AppendMileage "{\"recordId\":\"coal-mb-$R\",\"vid\":\"$V3\",\"day\":849,\"odometer\":900000,\"vetLaneId\":\"lane-1\"}"
row "Hide an implausible reading from F1b" "VET backdates a reading by one day (v1.5)" "1 VET" refused

echo "== Goal: commit with fewer than a majority of organisations"
as vet1; invoke 4 VehicleRegistry:AppendMileage "{\"recordId\":\"coal-m1-$R\",\"vid\":\"$V3\",\"day\":900,\"odometer\":12000,\"vetLaneId\":\"lane-1\"}"
row "Commit any write with fewer than a majority of organisations" "Valid write endorsed by the VET organisation's peer only" "1 organisation" refused
invoke 4,1 VehicleRegistry:AppendMileage "{\"recordId\":\"coal-m1b-$R\",\"vid\":\"$V3\",\"day\":900,\"odometer\":12000,\"vetLaneId\":\"lane-1\"}"
row "Commit any write with fewer than a majority of organisations" "The same write endorsed by the VET and DMT peers" "2 organisations" refused
invoke 4,1,2 VehicleRegistry:AppendMileage "{\"recordId\":\"coal-m2-$R\",\"vid\":\"$V3\",\"day\":901,\"odometer\":12100,\"vetLaneId\":\"lane-1\"}"
row "Commit any write with fewer than a majority of organisations" "The same write endorsed by the VET, DMT and Police peers" "3 organisations (majority of five)" committed

echo "== Goal: act as another agency with a certificate from the wrong organisation (v1.6)"
ROGUE="$(org_dir 1)/users/dmtcustoms1@org1.example.com/msp"
if [[ ! -d "$ROGUE/signcerts" ]]; then
  FABRIC_CA_CLIENT_HOME="$(org_dir 1)" fabric-ca-client register --caname ca-org1 --id.name dmtcustoms1 --id.secret dmtcustoms1pw \
    --id.type client --id.attrs "registry.role=CUSTOMS:ecert" --tls.certfiles "$TN/organizations/fabric-ca/org1/ca-cert.pem" >/dev/null 2>&1
  FABRIC_CA_CLIENT_HOME="$(org_dir 1)" fabric-ca-client enroll -u "https://dmtcustoms1:dmtcustoms1pw@localhost:7054" --caname ca-org1 \
    -M "$ROGUE" --enrollment.attrs registry.role --tls.certfiles "$TN/organizations/fabric-ca/org1/ca-cert.pem" >/dev/null 2>&1
  cp "$(org_dir 1)/msp/config.yaml" "$ROGUE/config.yaml"
fi
as dmtcustoms1; imp "$(hex "coal-ch6-$R")" "$(hex "coal-en6-$R")" "coal-imp6-$R"
row "Clear an import without Customs" "The DMT's CA administrator issues a certificate with the Customs role, which records an import" "1 DMT (CA administrator)" refused

echo "== Access checks"
for who in insurer1 vet1 customs1; do as "$who"; query IdentityManager:GetRevealAudit "$CASE"; access "$who reads a case log" "GetRevealAudit" refused; done
as police1; query IdentityManager:GetRevealAudit "$CASE"; access "Police reads the case log" "GetRevealAudit" read
as dmt; query FraudDetector:GetDisclosure "$V3" P0; access "Any identity asks for a P0 disclosure directly" "FraudDetector:GetDisclosure (removed)" refused
as insurer1; query VehicleRegistry:GetVehicleHistory "$V3"; access "Any identity reads a raw history" "VehicleRegistry:GetVehicleHistory (removed)" refused
as insurer1; invoke maj FraudDetector:EvaluateOwnerPredicate "$V3" F6b; access "An insurer runs the owner-scope predicate" "EvaluateOwnerPredicate" refused

{
  as admin1; VERSION=$(peer lifecycle chaincode querycommitted -C "$CHANNEL" -n "$CC" 2>/dev/null | sed -n 's/.*Version: \([^,]*\),.*/\1/p')
  echo "# Minimum-coalition table (live five-organisation network, chaincode v${VERSION:-?})"
  echo
  echo "Recorded $(date -u +%Y-%m-%dT%H:%MZ) on the five-organisation Fabric network: Org1 DMT (also the licensed assembler identity), Org2 Police (two officers), Org3 Customs, Org4 VET, Org5 insurers (three insurer identities), each with its own peer and CA; three Raft orderers. Endorsement policy: the channel default, a majority of the five organisations, so three. Every write below that is not an endorsement row is endorsed by the caller's own peer plus two others. Full transcripts: \`transcripts.txt\`."
  echo
  echo "**Role versus organisation.** \"1 Customs + 1 DMT\" now means two organisations' officers, but the write is still endorsed by three peers chosen by the caller; the chaincode, not the endorsement policy, enforces who may write which record. All five organisations run on one laboratory host with one operator."
  echo
  echo "| Insider goal | Attempt | Identities involved | Outcome |"
  echo "|---|---|---|---|"
  printf '%s\n' "${ROWS[@]}"
  echo
  echo "## Chaincode access checks"
  echo
  echo "| Check | Function | Outcome |"
  echo "|---|---|---|"
  printf '%s\n' "${ACCESS[@]}"
} > "$OUT/coalition.md"
cat "$OUT/coalition.md"
echo "coalition: $MISMATCH outcome(s) differ from the expected column"
[[ $MISMATCH -eq 0 ]]
