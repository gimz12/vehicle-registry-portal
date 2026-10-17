#!/usr/bin/env bash
# NOTE: JSON bodies are single-quoted with '"$VAR"' splices on purpose: macOS bash 3.2
# mangles escaped quotes inside $(...), which silently turns JSON into brace expansions.
# Live checks against the running portal and Fabric network; non-zero on any failure.
# Usage: ./eval/qa-live.sh [base-url]
set -uo pipefail
BASE="${1:-http://localhost:3000}"
PASS=0; FAIL=0
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STORE="${OFFCHAIN_DIR:-$ROOT/offchain/dev-store}"

post() { curl -s --max-time 180 -H 'content-type: application/json' -X POST "$BASE$1" -d "$2"; }
get()  { curl -s --max-time 180 "$BASE$1"; }
field() { python3 -c "import json,sys; d=json.load(sys.stdin); v=d
for k in sys.argv[1].split('.'):
    v = v[int(k)] if isinstance(v, list) else v.get(k) if isinstance(v, dict) else None
print('' if v is None else v)" "$1"; }
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 180 -H 'content-type: application/json' "$@"; }
check() { # check "name" "expected" "actual"
  if [[ "$2" == "$3" ]]; then PASS=$((PASS+1)); printf '  ok   %s\n' "$1"
  else FAIL=$((FAIL+1)); printf '  FAIL %s\n       expected: %s\n       actual:   %s\n' "$1" "$2" "$3"; fi
}
contains() { # contains "name" "needle" "haystack"
  if [[ "$3" == *"$2"* ]]; then PASS=$((PASS+1)); printf '  ok   %s\n' "$1"
  else FAIL=$((FAIL+1)); printf '  FAIL %s\n       wanted: ...%s...\n       got:    %s\n' "$1" "$2" "$3"; fi
}
ids() { # ids <list> [rule]: rule ids in a disclosure list; ERROR when the body is not a disclosure
  python3 -c "import json,sys
try: d=json.load(sys.stdin); print(','.join(f['ruleId'] for f in d[sys.argv[1]] if len(sys.argv)<3 or f['ruleId']==sys.argv[2]))
except Exception: print('ERROR')" "$@"; }
urlenc() { python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "$1"; }

# Where the off-ledger records live: per-organisation custody databases (services) or the fallback file store.
MODE=$(get /api/health | field custody.mode 2>/dev/null) || { echo "The portal at $BASE does not answer /api/health; start it first." >&2; exit 2; }
MODE="${MODE:-files}"
echo "== Custody mode: $MODE"
dmtsql() { docker exec custody-dmt-db psql -U dmt -d dmt -tAc "$1"; }
polsql() { docker exec custody-police-db psql -U police -d police -tAc "$1"; }
has_record() { if [[ $MODE == services ]]; then dmtsql "select count(*) from owner_record where vid = '$1'"; else ls "$STORE/owner_record/" 2>/dev/null | grep -c "$1"; fi; }
share_count() {
  if [[ $MODE == services ]]; then echo $(( $(dmtsql "select count(*) from key_share where vid = '$1'") + $(polsql "select count(*) from key_share where vid = '$1'") ))
  else echo $(( $(ls "$STORE/custody_dmt/" | grep -c "$1") + $(ls "$STORE/custody_police/" | grep -c "$1") )); fi
}
# The anchor is a hash of the whole envelope, so it changes exactly when the stored record does.
env_fingerprint() { if [[ $MODE == services ]]; then dmtsql "select anchor_sha256 from owner_record where vid = '$1'"; else shasum -a 256 "$STORE/owner_record/$1.json" | cut -d' ' -f1; fi; }
flip_first_byte() { # flips one ciphertext bit; running it twice restores the record
  if [[ $MODE == services ]]; then dmtsql "update owner_record set ciphertext = set_byte(ciphertext, 0, get_byte(ciphertext, 0) # 1) where vid = '$1'" >/dev/null
  else python3 - "$STORE/owner_record/$1.json" <<'EOF'
import base64,json,sys; p=sys.argv[1]; d=json.load(open(p)); c=bytearray(base64.b64decode(d["ciphertext"])); c[0]^=1; d["ciphertext"]=base64.b64encode(bytes(c)).decode(); json.dump(d,open(p,"w"))
EOF
  fi
}
fleet_size() {
  if [[ $MODE == services ]]; then dmtsql "select count(*) from owner_index where owner_tag = (select owner_tag from owner_index where vid = '$1')"
  else python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(len(d['owners'][d['vehicles'][sys.argv[2]]]))" "$STORE/custody_dmt/owner_index.json" "$1"; fi
}

R=$RANDOM$RANDOM
CH="QA-CH-$R"; EN="QA-EN-$R"; MK="Toyota"
echo "== Input validation (no ledger round trip)"
check "desk: unknown agency -> 400" 400 "$(code -X POST "$BASE/api/desk" -d '{"agency":"TAXI","action":"import"}')"
check "desk: VET cannot report theft -> 400" 400 "$(code -X POST "$BASE/api/desk" -d '{"agency":"VET","action":"theft","vid":"'$(printf 'a%.0s' {1..64})'","recordId":"x"}')"
check "desk: register with bad NIC -> 400" 400 "$(code -X POST "$BASE/api/desk" -d '{"agency":"DMT","action":"register","chassisNumber":"'$CH'","engineNumber":"'$EN'","make":"'$MK'","owner":{"name":"A","nic":"ABC","address":"x"}}')"
check "desk: register with null-like chassis -> 400" 400 "$(code -X POST "$BASE/api/desk" -d '{"agency":"DMT","action":"register","chassisNumber":"N/A","engineNumber":"'$EN'","make":"'$MK'","owner":{"name":"A","nic":"751234567V","address":"x"}}')"
check "desk: mileage without odometer -> 400" 400 "$(code -X POST "$BASE/api/desk" -d '{"agency":"VET","action":"mileage","vid":"'$(printf 'a%.0s' {1..64})'","recordId":"m","day":1}')"
check "handle: empty chassis -> 400" 400 "$(code -X POST "$BASE/api/handle" -d '{"chassis":""}')"
check "handle: null-like chassis -> 400" 400 "$(code -X POST "$BASE/api/handle" -d '{"chassis":"NONE"}')"
check "vehicle: bad handle -> 400" 400 "$(code "$BASE/api/vehicle/not-a-handle")"
check "vehicle: unknown purpose -> 400" 400 "$(code "$BASE/api/vehicle/$(printf 'a%.0s' {1..64})?as=AUDITOR")"
check "history: buyer refused -> 403" 403 "$(code "$BASE/api/vehicle/$(printf 'a%.0s' {1..64})/history?as=BUYER")"
check "escalation: reject without reason -> 400" 400 "$(code -X POST "$BASE/api/escalation" -d '{"action":"reject","as":"DMT","caseId":"X"}')"
check "escalation: create without legal reference -> 400" 400 "$(code -X POST "$BASE/api/escalation" -d '{"action":"create","as":"POLICE","caseId":"X","vid":"'$(printf 'a%.0s' {1..64})'","ruleId":"F3c","ground":"COURT_ORDER","groundRef":""}')"
check "reveal: malformed body -> 400" 400 "$(code -X POST "$BASE/api/reveal" -d '{"caseId":"","vid":"x"}')"

echo "== Handle derivation"
H1=$(post /api/handle '{"chassis":"'"$CH"'"}' | field vid)
H2=$(post /api/handle "{\"chassis\":\"$(echo "$CH" | tr 'A-Z-' 'a-z ')\"}" | field vid)
check "same chassis, different spelling -> same handle" "$H1" "$H2"
check "handle is 64 hex" 64 "${#H1}"
VID="$H1"

echo "== Provenance and identity gates (F5, F4)"
contains "register before import -> refused (F5)" "prior Customs or assembler" "$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH"'","engineNumber":"'"$EN"'","make":"'"$MK"'","owner":{"name":"Owner A (synthetic)","nic":"751234567V","address":"1 A St"}}' | field statusMessage)"
contains "customs import committed" "Import clearance recorded" "$(post /api/desk '{"agency":"CUSTOMS","action":"import","recordId":"cusdec-'"$R"'","chassisNumber":"'"$CH"'","engineNumber":"'"$EN"'","make":"'"$MK"'","internal":{"declarant":"QA Importers (synthetic)","cusdecNo":"CUSDEC/QA/'"$R"'","valuationLkr":4250000}}' | field message)"
contains "same chassis imported twice -> refused" "already has an import attestation" "$(post /api/desk '{"agency":"CUSTOMS","action":"import","recordId":"cusdec-'"$R"'-dup","chassisNumber":"'"$CH"'","engineNumber":"'"$EN"'","make":"'"$MK"'"}' | field statusMessage)"
contains "register with wrong engine for that chassis -> refused (F5, does not match the import)" "does not match the prior CUSTOMS attestation" "$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH"'","engineNumber":"OTHER-'"$R"'","make":"'"$MK"'","owner":{"name":"A","nic":"751234567V","address":"x"}}' | field statusMessage)"
contains "register committed" "owner record sealed" "$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH"'","engineNumber":"'"$EN"'","make":"'"$MK"'","owner":{"name":"Owner A (synthetic)","nic":"751234567V","address":"1 A St"}}' | field message)"
check "owner record exists off-ledger" 1 "$(has_record "$VID")"
check "both custody shares exist" 2 "$(share_count "$VID")"
contains "register same chassis again -> refused (F4)" "already assigned" "$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH"'","engineNumber":"'"$EN"'","make":"'"$MK"'","owner":{"name":"B","nic":"197512345678","address":"y"}}' | field statusMessage)"
contains "register other chassis with same engine -> refused (F4 engine)" "already assigned" "$(post /api/desk '{"agency":"CUSTOMS","action":"import","recordId":"cusdec-'"$R"'-b","chassisNumber":"'"$CH"'-B","engineNumber":"'"$EN"'","make":"'"$MK"'"}' >/dev/null; post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH"'-B","engineNumber":"'"$EN"'","make":"'"$MK"'","owner":{"name":"B","nic":"197512345678","address":"y"}}' | field statusMessage)"

echo "== Clean vehicle views"
check "buyer gets P2" P2 "$(get "/api/vehicle/$VID?as=BUYER" | field profile)"
check "buyer sees no status banner" "False" "$(get "/api/vehicle/$VID?as=BUYER" | field status.stolen)"
check "insurer without a claim gets P2" P2 "$(get "/api/vehicle/$VID?as=INSURER" | field profile)"
check "police without a case gets P1" P1 "$(get "/api/vehicle/$VID?as=POLICE" | field profile)"
check "police with unknown case -> 403" 403 "$(code "$BASE/api/vehicle/$VID?as=POLICE&case=NOPE")"
check "DMT history visible" "200" "$(code "$BASE/api/vehicle/$VID/history?as=DMT")"

echo "== Evidence and rules on committed data"
post /api/desk '{"agency":"VET","action":"mileage","vid":"'"$VID"'","recordId":"vet-'"$R"'-1","day":0,"odometer":90000}' >/dev/null
contains "second reading below first is accepted, not rejected" "recorded" "$(post /api/desk '{"agency":"VET","action":"mileage","vid":"'"$VID"'","recordId":"vet-'"$R"'-2","day":300,"odometer":70000}' | field message)"
contains "backdated reading -> refused (v1.5, F1b cannot be dodged)" "cannot be dated before" "$(post /api/desk '{"agency":"VET","action":"mileage","vid":"'"$VID"'","recordId":"vet-'"$R"'-b","day":299,"odometer":900000}' | field statusMessage)"
contains "buyer sees F1a as a warning (VET evidence is E1)" "F1a/WARNING" "$(get "/api/vehicle/$VID?as=BUYER" | python3 -c "import json,sys; print(','.join(f['ruleId']+'/'+f['strength'] for f in json.load(sys.stdin)['failed']))")"
contains "DMT emission certificate recorded" "Emission certificate recorded" "$(post /api/desk '{"agency":"VET","action":"emission","vid":"'"$VID"'","recordId":"vet-'"$R"'-e","day":301}' | field message)"
post /api/desk '{"agency":"POLICE","action":"accident","vid":"'"$VID"'","recordId":"police-report-'"$R"'","day":400}' >/dev/null
contains "insurer claim without evidence -> F3a (claim on insurer-c)" "Claim filed" "$(post /api/desk '{"agency":"INSURER","action":"claim","vid":"'"$VID"'","recordId":"claim-'"$R"'-0","day":402,"insurerId":"insurer-c"}' | field message)"
post /api/desk '{"agency":"INSURER","action":"claim","vid":"'"$VID"'","recordId":"claim-'"$R"'-1","day":405,"insurerId":"insurer-a","evidenceAnchor":"police-report-'"$R"'"}' >/dev/null
post /api/desk '{"agency":"INSURER","action":"claim","vid":"'"$VID"'","recordId":"claim-'"$R"'-2","day":409,"insurerId":"insurer-b","evidenceAnchor":"police-report-'"$R"'"}' >/dev/null
contains "claim under another insurer's name -> refused (v1.5, insurer bound to registry.party)" "files claims only as insurer insurer-a" "$(post /api/desk '{"agency":"INSURER","action":"claim","vid":"'"$VID"'","recordId":"claim-'"$R"'-z","day":406,"insurerId":"insurer-z"}' | field statusMessage)"
POLICE_FAILED=$(get "/api/vehicle/$VID?as=POLICE" | python3 -c "import json,sys; print(','.join(sorted(f['ruleId']+'/'+f['strength'] for f in json.load(sys.stdin)['failed'])))")
contains "police sees F3c duplicate-anchor alert" "F3c/ALERT" "$POLICE_FAILED"
contains "police sees F3a unanchored claim" "F3a/" "$POLICE_FAILED"
check "insurer-a (holds a claim) gets P1" P1 "$(get "/api/vehicle/$VID?as=INSURER" | field profile)"
check "buyer at P2 does not see F3c (incident link withheld)" "" "$(get "/api/vehicle/$VID?as=BUYER" | ids failed F3c)"
contains "buyer: F3c appears under not performed as a structural gap" "F3c" "$(get "/api/vehicle/$VID?as=BUYER" | python3 -c "import json,sys; print(','.join(f['ruleId'] for f in json.load(sys.stdin)['notPerformed']))")"
check "DMT history redacts the claim's incident reference" "2" "$(get "/api/vehicle/$VID/history?as=DMT" | python3 -c "import json,sys; print(sum(1 for e in json.load(sys.stdin)['events'] if e['redacted']))")"
check "police history keeps it" "0" "$(get "/api/vehicle/$VID/history?as=POLICE" | python3 -c "import json,sys; print(sum(1 for e in json.load(sys.stdin)['events'] if e['redacted']))")"

echo "== Theft (v1.8): report, owner confirmed by the DMT, flag, block, buyer visibility, clear"
contains "clearing a theft that was never reported -> refused (v1.5)" "not flagged stolen" "$(post /api/desk '{"agency":"POLICE","action":"clear","vid":"'"$VID"'","recordId":"clear-'"$R"'-0"}' | field statusMessage)"
contains "a theft flag with no owner confirmation -> refused by the chaincode (v1.8)" "owner confirmation" "$(post /api/desk '{"agency":"POLICE","action":"theft","vid":"'"$VID"'","recordId":"theft-'"$R"'-0","reportRef":"PR/QA/'"$R"'"}' | field statusMessage)"
contains "owner check before the report is on the ledger -> refused" "theft report" "$(post /api/desk '{"agency":"POLICE","action":"theft","vid":"'"$VID"'","recordId":"theft-'"$R"'-1","reportRef":"PR/QA/'"$R"'","reporterNic":"751234567V"}' | field statusMessage)"
contains "Police record the theft report" "Theft report PR/QA/" "$(post /api/desk '{"agency":"POLICE","action":"report","vid":"'"$VID"'","recordId":"theft-report-'"$R"'","reportRef":"PR/QA/'"$R"'"}' | field message)"
check "a report alone does not flag the vehicle" "False" "$(get "/api/vehicle/$VID?as=BUYER" | field status.stolen)"
check "the buyer sees that a theft report is being verified" "True" "$(get "/api/vehicle/$VID?as=BUYER" | field status.theftReportPending)"
check "a report alone raises no F2 alert" "" "$(get "/api/vehicle/$VID?as=BUYER" | ids failed F2)"
check "a reporter who is not the owner -> no flag (403)" 403 "$(code -X POST "$BASE/api/desk" -d '{"agency":"POLICE","action":"theft","vid":"'"$VID"'","recordId":"theft-'"$R"'-2","reportRef":"PR/QA/'"$R"'","reporterNic":"200012345678"}')"
check "still not stolen after the refused reporter" "False" "$(get "/api/vehicle/$VID?as=BUYER" | field status.stolen)"
contains "the owner's NIC -> the DMT confirms and the flag is committed" "confirmed the reporter as the owner" "$(post /api/desk '{"agency":"POLICE","action":"theft","vid":"'"$VID"'","recordId":"theft-'"$R"'","reportRef":"PR/QA/'"$R"'","reporterNic":"751234567V"}' | field message)"
contains "the confirmation is on the ledger, written by the DMT" "OWNER_CONFIRMED" "$(get "/api/vehicle/$VID/history?as=POLICE" | python3 -c "import json,sys; print(','.join(e['type'] for e in json.load(sys.stdin)['events']))")"
check "buyer sees stolen status" "True" "$(get "/api/vehicle/$VID?as=BUYER" | field status.stolen)"
contains "buyer sees F2 current-theft alert" "F2/ALERT" "$(get "/api/vehicle/$VID?as=BUYER" | python3 -c "import json,sys; print(','.join(f['ruleId']+'/'+f['strength'] for f in json.load(sys.stdin)['failed']))")"
ENV_BEFORE=$(env_fingerprint "$VID")
contains "transfer while stolen -> refused (F2 prevention)" "blocked while vehicle is stolen" "$(post /api/desk '{"agency":"DMT","action":"transfer","vid":"'"$VID"'","recordId":"tr-'"$R"'-1","owner":{"name":"B","nic":"197512345678","address":"2 B Rd"}}' | field statusMessage)"
check "refused transfer leaves the stored owner record untouched" "$ENV_BEFORE" "$(env_fingerprint "$VID")"
contains "portal refuses: DMT has no theft-clear action" "may only" "$(post /api/desk '{"agency":"DMT","action":"clear","vid":"'"$VID"'","recordId":"x"}' | field statusMessage)"
post /api/desk '{"agency":"POLICE","action":"clear","vid":"'"$VID"'","recordId":"clear-'"$R"'"}' >/dev/null
check "status cleared" "False" "$(get "/api/vehicle/$VID?as=BUYER" | field status.stolen)"
check "the buyer is told the vehicle was reported stolen and cleared (v1.8)" "True" "$(get "/api/vehicle/$VID?as=BUYER" | field status.theftCleared)"
contains "the old confirmation cannot flag the vehicle again after the clearance" "owner confirmation" "$(post /api/desk '{"agency":"POLICE","action":"theft","vid":"'"$VID"'","recordId":"theft-'"$R"'-again","reportRef":"PR/QA/'"$R"'"}' | field statusMessage)"
echo "== v1.9: an unconfirmed theft report can be withdrawn"
post /api/desk '{"agency":"POLICE","action":"report","vid":"'"$VID"'","recordId":"theft-report-'"$R"'-w","reportRef":"PR/QA/'"$R"'-W"}' >/dev/null
check "a new report shows as being verified" "True" "$(get "/api/vehicle/$VID?as=BUYER" | field status.theftReportPending)"
contains "the Police withdraw the unconfirmed report with a clearance (v1.9)" "cleared" "$(post /api/desk '{"agency":"POLICE","action":"clear","vid":"'"$VID"'","recordId":"clear-'"$R"'-w"}' | field message)"
check "the buyer no longer sees a report being verified" "False" "$(get "/api/vehicle/$VID?as=BUYER" | python3 -c "import json,sys; print(bool(json.load(sys.stdin)['status'].get('theftReportPending')))")"
contains "the withdrawn report cannot be confirmed later" "theft report" "$(post /api/desk '{"agency":"POLICE","action":"theft","vid":"'"$VID"'","recordId":"theft-'"$R"'-w","reportRef":"PR/QA/'"$R"'-W","reporterNic":"751234567V"}' | field statusMessage)"
check "F2 no longer flagged after clearance" "" "$(get "/api/vehicle/$VID?as=BUYER" | ids failed F2)"

echo "== Transfer replaces the owner record"
contains "transfer committed" "previous record replaced" "$(post /api/desk '{"agency":"DMT","action":"transfer","vid":"'"$VID"'","recordId":"tr-'"$R"'-2","owner":{"name":"Owner B (synthetic)","nic":"197512345678","address":"2 B Rd"}}' | field message)"
check "transfer replaced the sealed owner record" "changed" "$([[ "$(env_fingerprint "$VID")" != "$ENV_BEFORE" ]] && echo changed || echo unchanged)"

echo "== Escalation edge cases"
CASE="QA/$R"; CASEQ=$(urlenc "$CASE")
check "reveal on a case that does not exist -> 404" "404" "$(code -X POST "$BASE/api/reveal" -d '{"caseId":"'"$CASE"'","vid":"'"$VID"'"}')"
contains "portal refuses: VET cannot create a case" "as must be" "$(post /api/escalation '{"action":"create","as":"VET","caseId":"'"$CASE"'","vid":"'"$VID"'","ruleId":"F3c","ground":"COURT_ORDER","groundRef":"x"}' | field statusMessage)"
contains "case on a warning-only rule is refused (F1a is not an alert)" "no live owner-attributed ALERT" "$(post /api/escalation '{"action":"create","as":"POLICE","caseId":"'"$CASE"'-w","vid":"'"$VID"'","ruleId":"F1a","ground":"COURT_ORDER","groundRef":"x"}' | field statusMessage)"
check "case created on F3c" PENDING "$(post /api/escalation '{"action":"create","as":"POLICE","caseId":"'"$CASE"'","vid":"'"$VID"'","ruleId":"F3c","ground":"COURT_ORDER","groundRef":"MC/'"$R"'"}' | field status)"
contains "same case id again -> refused" "already exists" "$(post /api/escalation '{"action":"create","as":"POLICE","caseId":"'"$CASE"'","vid":"'"$VID"'","ruleId":"F3c","ground":"COURT_ORDER","groundRef":"MC/'"$R"'"}' | field statusMessage)"
check "owner scope on pending case -> 403" 403 "$(code "$BASE/api/vehicle/$VID?as=POLICE&case=$CASEQ")"
check "reveal on pending case -> 403" 403 "$(code -X POST "$BASE/api/reveal" -d '{"caseId":"'"$CASE"'","vid":"'"$VID"'"}')"
check "one approval leaves it pending" PENDING "$(post /api/escalation '{"action":"approve","as":"DMT","caseId":"'"$CASE"'"}' | field status)"
contains "approving twice as DMT -> refused (v1.5, one approval per side)" "already approved" "$(post /api/escalation '{"action":"approve","as":"DMT","caseId":"'"$CASE"'"}' | field statusMessage)"
check "second agency approves -> approved" APPROVED "$(post /api/escalation '{"action":"approve","as":"POLICE","caseId":"'"$CASE"'"}' | field status)"
contains "approving an approved case -> refused" "already" "$(post /api/escalation '{"action":"approve","as":"POLICE","caseId":"'"$CASE"'"}' | field statusMessage)"
check "owner scope under approved case -> P0" P0 "$(get "/api/vehicle/$VID?as=POLICE&case=$CASEQ" | field profile)"
OTHER=$(printf 'b%.0s' {1..64})
check "approved case used on another vehicle -> 403" 403 "$(code "$BASE/api/vehicle/$OTHER?as=POLICE&case=$CASEQ")"
check "DMT presenting the case still gets only P2" P2 "$(get "/api/vehicle/$VID?as=DMT&case=$CASEQ" | field profile)"
REVEAL=$(post /api/reveal '{"caseId":"'"$CASE"'","vid":"'"$VID"'"}')
check "reveal returns the CURRENT owner after transfer" "Owner B (synthetic)" "$(echo "$REVEAL" | field owner.name)"
contains "reveal is marked synthetic" "SYNTHETIC" "$(echo "$REVEAL" | field owner.note)"
contains "access log records owner scope" "OWNER_SCOPE:$VID" "$(echo "$REVEAL" | field accessLog)"
contains "access log records identity reveal" "IDENTITY_REVEAL:$VID" "$(echo "$REVEAL" | field accessLog)"
flip_first_byte "$VID"
check "tampered off-ledger record -> reveal refused (409)" 409 "$(code -X POST "$BASE/api/reveal" -d '{"caseId":"'"$CASE"'","vid":"'"$VID"'"}')"
flip_first_byte "$VID"
check "restored record reveals again" 200 "$(code -X POST "$BASE/api/reveal" -d '{"caseId":"'"$CASE"'","vid":"'"$VID"'"}')"
CASE2="QA/$R/2";
post /api/escalation '{"action":"create","as":"POLICE","caseId":"'"$CASE2"'","vid":"'"$VID"'","ruleId":"F3c","ground":"THEFT_INVESTIGATION","groundRef":"CID/'"$R"'"}' >/dev/null
check "rejected case" REJECTED "$(post /api/escalation '{"action":"reject","as":"DMT","caseId":"'"$CASE2"'","reason":"Reference does not match an open investigation"}' | field status)"
contains "approving a rejected case -> refused" "already" "$(post /api/escalation '{"action":"approve","as":"POLICE","caseId":"'"$CASE2"'"}' | field statusMessage)"
check "reveal on rejected case -> 403" 403 "$(code -X POST "$BASE/api/reveal" -d '{"caseId":"'"$CASE2"'","vid":"'"$VID"'"}')"

echo "== Write-off and retirement"
contains "insurer writes off" "written off" "$(post /api/desk '{"agency":"INSURER","action":"writeoff","vid":"'"$VID"'"}' | field message)"
check "buyer sees written-off status" "True" "$(get "/api/vehicle/$VID?as=BUYER" | field status.writtenOff)"
contains "transfer while written off -> refused" "write-off is unresolved" "$(post /api/desk '{"agency":"DMT","action":"transfer","vid":"'"$VID"'","recordId":"tr-'"$R"'-3","owner":{"name":"C","nic":"751234567V","address":"z"}}' | field statusMessage)"
contains "insurer resolves write-off" "resolved" "$(post /api/desk '{"agency":"INSURER","action":"writeoff","vid":"'"$VID"'","flag":false}' | field message)"
contains "portal refuses: Police has no retire action" "may only" "$(post /api/desk '{"agency":"POLICE","action":"retire","vid":"'"$VID"'"}' | field statusMessage)"
contains "DMT retires" "retired" "$(post /api/desk '{"agency":"DMT","action":"retire","vid":"'"$VID"'"}' | field message)"
check "buyer sees retired status" "True" "$(get "/api/vehicle/$VID?as=BUYER" | field status.retired)"
contains "transfer of retired vehicle -> refused" "retired" "$(post /api/desk '{"agency":"DMT","action":"transfer","vid":"'"$VID"'","recordId":"tr-'"$R"'-4","owner":{"name":"C","nic":"751234567V","address":"z"}}' | field statusMessage)"
contains "re-registering a retired chassis -> refused (F4 resurrection)" "already assigned" "$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH"'","engineNumber":"'"$EN"'","make":"'"$MK"'","owner":{"name":"C","nic":"751234567V","address":"z"}}' | field statusMessage)"

echo "== Assembler path"
CH3="QA-ASM-$R"; EN3="QA-AE-$R"
contains "portal refuses: Customs has no assembler action" "may only" "$(post /api/desk '{"agency":"CUSTOMS","action":"attest","recordId":"a-'"$R"'","chassisNumber":"'"$CH3"'","engineNumber":"'"$EN3"'","make":"'"$MK"'"}' | field statusMessage)"
contains "portal refuses: DMT has no assembler action (the chaincode gate is a coalition.sh row)" "may only" "$(post /api/desk '{"agency":"DMT","action":"attest","recordId":"asm-'"$R"'","chassisNumber":"'"$CH3"'","engineNumber":"'"$EN3"'","make":"'"$MK"'"}' | field statusMessage)"
contains "assembler records its attestation" "Assembler attestation recorded" "$(post /api/desk '{"agency":"ASSEMBLER","action":"attest","recordId":"asm-'"$R"'","chassisNumber":"'"$CH3"'","engineNumber":"'"$EN3"'","make":"'"$MK"'"}' | field message)"
contains "register with CUSTOMS provenance against an assembler attestation -> refused" "does not match the prior ASSEMBLER attestation" "$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH3"'","engineNumber":"'"$EN3"'","make":"'"$MK"'","provenance":"CUSTOMS","owner":{"name":"D","nic":"751234567V","address":"q"}}' | field statusMessage)"
contains "register with ASSEMBLER provenance -> committed" "owner record sealed" "$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH3"'","engineNumber":"'"$EN3"'","make":"'"$MK"'","provenance":"ASSEMBLER","owner":{"name":"D","nic":"751234567V","address":"q"}}' | field message)"

echo "== v1.3: no ownership oracle"
check "handle never returns an owner pseudonym, even when sent a NIC" "idxChassis,vid" "$(post /api/handle '{"chassis":"'"$CH"'","nic":"751234567V"}' | python3 -c "import json,sys; print(','.join(sorted(json.load(sys.stdin))))" 2>/dev/null)"

echo "== v1.3: owner linking through the DMT owner index"
NICF="19$(printf '%010d' "${R:0:10}")"
VA=""; VB=""
for tag in A B; do
  C="QA-F$tag-$R"; E="QA-F${tag}E-$R"
  post /api/desk '{"agency":"CUSTOMS","action":"import","recordId":"cusdec-'"$C"'","chassisNumber":"'"$C"'","engineNumber":"'"$E"'","make":"'"$MK"'"}' >/dev/null
  V=$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$C"'","engineNumber":"'"$E"'","make":"'"$MK"'","owner":{"name":"Owner F (synthetic)","nic":"'"$NICF"'","address":"9 F Lane"}}' | field vid)
  if [[ $tag == A ]]; then VA=$V; else VB=$V; fi
done
check "owner index (DMT custody) holds the owner's two vehicles" "2" "$(fleet_size "$VA")"
PA=$(get "/api/vehicle/$VA/history?as=POLICE" | python3 -c "import json,sys; print(json.load(sys.stdin)['events'][0]['pseudonym'])")
PB=$(get "/api/vehicle/$VB/history?as=POLICE" | python3 -c "import json,sys; print(json.load(sys.stdin)['events'][0]['pseudonym'])")
check "the two vehicles carry different on-chain pseudonyms" "different" "$([[ -n "$PA" && "$PA" != "$PB" ]] && echo different || echo "same: $PA")"
for V in "$VA" "$VB"; do
  post /api/desk '{"agency":"POLICE","action":"accident","vid":"'"$V"'","recordId":"rep-'"${V:0:8}"'","day":700}' >/dev/null
  post /api/desk '{"agency":"INSURER","action":"claim","vid":"'"$V"'","recordId":"cl-'"${V:0:8}"'","day":705,"insurerId":"insurer-a","evidenceAnchor":"rep-'"${V:0:8}"'","assessorId":"assessor-'"$R"'"}' >/dev/null
done
contains "buyer at P2: the anchored claim is a structural gap, not evaluated" "F3a" "$(get "/api/vehicle/$VA?as=BUYER" | python3 -c "import json,sys; print(','.join(f['ruleId'] for f in json.load(sys.stdin)['notPerformed'] if f['structural']))")"
contains "police at P1 evaluates the anchored claim" "F3a" "$(get "/api/vehicle/$VA?as=POLICE" | python3 -c "import json,sys; print(','.join(f['ruleId'] for f in json.load(sys.stdin)['passed']))")"
contains "portal refuses: Police cannot run the owner-scope predicate" "only the DMT" "$(post /api/escalation '{"action":"predicate","as":"POLICE","vid":"'"$VA"'","ruleId":"F6b"}' | field statusMessage)"
contains "F6a predicate refused without a rollback on the vehicle" "needs an odometer rollback" "$(post /api/escalation '{"action":"predicate","as":"DMT","vid":"'"$VA"'","ruleId":"F6a"}' | field statusMessage)"
PRED=$(post /api/escalation '{"action":"predicate","as":"DMT","vid":"'"$VA"'","ruleId":"F6b"}')
check "F6b predicate over the owner's fleet: flagged" FLAGGED "$(echo "$PRED" | field status)"
check "F6b predicate at alert strength (claims cite police reports)" ALERT "$(echo "$PRED" | field strength)"
check "the predicate returns the verdict only" "" "$(echo "$PRED" | field detail)"
CASEF="QA/$R/F6"; CASEFQ=$(urlenc "$CASEF")
check "case grounded on the F6b predicate" PENDING "$(post /api/escalation '{"action":"create","as":"POLICE","caseId":"'"$CASEF"'","vid":"'"$VA"'","ruleId":"F6b","ground":"FRAUD_INVESTIGATION","groundRef":"CID/'"$R"'/F6"}' | field status)"
post /api/escalation '{"action":"approve","as":"DMT","caseId":"'"$CASEF"'"}' >/dev/null
APPROVED=$(post /api/escalation '{"action":"approve","as":"POLICE","caseId":"'"$CASEF"'"}')
check "approved by the DMT and a second Police officer" APPROVED "$(echo "$APPROVED" | field status)"
check "the DMT attested a two-vehicle fleet" 2 "$(echo "$APPROVED" | field fleetSize)"
contains "owner scope under the case groups the desk-registered fleet (F6b fires)" "F6b" "$(get "/api/vehicle/$VA?as=POLICE&case=$CASEFQ" | python3 -c "import json,sys; print(','.join(f['ruleId'] for f in json.load(sys.stdin)['failed']))")"

echo "== v1.3: finance flag"
contains "DMT records finance" "under finance" "$(post /api/desk '{"agency":"DMT","action":"encumber","vid":"'"$VB"'","recordId":"fin-'"$R"'","reference":"AO/'"$R"'"}' | field message)"
check "buyer sees the vehicle is under finance" "True" "$(get "/api/vehicle/$VB?as=BUYER" | field status.encumbered)"
contains "portal refuses: insurer has no finance action" "may only" "$(post /api/desk '{"agency":"INSURER","action":"encumber","vid":"'"$VB"'","recordId":"fin-x","reference":"x"}' | field statusMessage)"
contains "transfer of a financed vehicle -> refused" "under finance" "$(post /api/desk '{"agency":"DMT","action":"transfer","vid":"'"$VB"'","recordId":"tr-'"$R"'-f1","owner":{"name":"G","nic":"751234567V","address":"g"}}' | field statusMessage)"
contains "DMT records the release" "release recorded" "$(post /api/desk '{"agency":"DMT","action":"release","vid":"'"$VB"'","recordId":"fin-'"$R"'-r","reference":"DL/'"$R"'"}' | field message)"
contains "transfer after release -> committed" "previous record replaced" "$(post /api/desk '{"agency":"DMT","action":"transfer","vid":"'"$VB"'","recordId":"tr-'"$R"'-f2","owner":{"name":"Owner G (synthetic)","nic":"751234567V","address":"g"}}' | field message)"
check "the fleet changed after approval -> owner-scope read refused (403)" 403 "$(code "$BASE/api/vehicle/$VA?as=POLICE&case=$CASEFQ")"
contains "the refusal names the changed fleet" "fleet has changed" "$(get "/api/vehicle/$VA?as=POLICE&case=$CASEFQ" | field statusMessage)"

if [[ $MODE == services ]]; then
  echo "== Custody services: each organisation's own container, database and token"
  set -a; . "$ROOT/offchain/custody/.env"; set +a
  DMT=http://127.0.0.1:8101; POL=http://127.0.0.1:8102; CUS=http://127.0.0.1:8103
  auth() { curl -s --max-time 30 -H "Authorization: Bearer $1" "${@:2}"; }
  acode() { curl -s -o /dev/null -w '%{http_code}' --max-time 30 -H "Authorization: Bearer $1" "${@:2}"; }
  CASE3="QA/$R/3"
  post /api/escalation '{"action":"create","as":"POLICE","caseId":"'"$CASE3"'","vid":"'"$VID"'","ruleId":"F3c","ground":"COURT_ORDER","groundRef":"MC/'"$R"'/3"}' >/dev/null
  check "DMT service without a token -> 401" 401 "$(code "$DMT/records/$VID?case=$CASEQ")"
  check "DMT service with a wrong token -> 401" 401 "$(acode wrong "$DMT/records/$VID?case=$CASEQ")"
  check "Police service with the DMT's token -> 401" 401 "$(acode "$CUSTODY_TOKEN_DMT" "$POL/shares/$VID?case=$CASEQ")"
  # The DMT's record is fetched by the Police service (its own credential), never by the portal.
  PSVC="$CUSTODY_TOKEN_POLICE_TO_DMT"
  check "DMT service refuses the record under a pending case (its own ledger check) -> 403" 403 "$(acode "$PSVC" "$DMT/records/$VID?case=$(urlenc "$CASE3")")"
  check "Police service refuses its half under a pending case -> 403" 403 "$(acode "$CUSTODY_TOKEN_POLICE" "$POL/shares/$VID?case=$(urlenc "$CASE3")")"
  check "Police service refuses its half under a rejected case -> 403" 403 "$(acode "$CUSTODY_TOKEN_POLICE" "$POL/shares/$VID?case=$(urlenc "$CASE2")")"
  check "DMT service refuses an approved case presented for another vehicle -> 403" 403 "$(acode "$PSVC" "$DMT/records/$OTHER?case=$CASEQ")"
  check "DMT service releases to the Police service under the approved case after reading it on the ledger" APPROVED "$(auth "$PSVC" "$DMT/records/$VID?case=$CASEQ" | field ledgerCheck.0.status)"
  contains "Police service read the case on the Police organisation's own peer" "peer0.org2" "$(auth "$CUSTODY_TOKEN_POLICE" "$POL/shares/$VID?case=$CASEQ" | field ledgerCheck.0.peer)"
  check "every refusal is in the DMT service's release log" 1 "$(dmtsql "select count(*) from release_log where vid = '$VID' and case_id = '$CASE3' and outcome like 'refused%'")"
  check "this run's vehicle never reached the portal host's file store" 0 "$(ls "$STORE/owner_record/" 2>/dev/null | grep -c "$VID")"
  check "Customs keeps its internal import record in its own service" "QA Importers (synthetic)" "$(auth "$CUSTODY_TOKEN_CUSTOMS" "$CUS/records/cusdec-$R" | field fields.declarant)"
  check "the internal fields appear in no ledger view" 0 "$( (get "/api/vehicle/$VID/history?as=POLICE"; get "/api/vehicle/$VID?as=POLICE&case=$CASEQ") | grep -c 'QA Importers')"
  check "all five custody services healthy" 5 "$(get /api/health | field custody.up)"
  VET=http://127.0.0.1:8104; INS=http://127.0.0.1:8105
  post /api/desk '{"agency":"VET","action":"mileage","vid":"'"$VID"'","recordId":"qa-vet-int-'"$R"'","day":5000,"odometer":990000,"internal":{"technician":"QA Technician (synthetic)","certificateNo":"QA/CERT/'"$R"'"}}' >/dev/null
  check "the emission centre keeps its internal record in its own service" "QA Technician (synthetic)" "$(auth "$CUSTODY_TOKEN_VET" "$VET/records/qa-vet-int-$R" | field fields.technician)"
  post /api/desk '{"agency":"INSURER","action":"claim","vid":"'"$VID"'","recordId":"qa-ins-int-'"$R"'","day":5001,"insurerId":"insurer-a","internal":{"policyNo":"QA/POL/'"$R"'","reserveLkr":125000}}' >/dev/null
  check "the insurer keeps its internal claim record in its own service" "125000" "$(auth "$CUSTODY_TOKEN_INSURER" "$INS/records/qa-ins-int-$R" | field fields.reserveLkr)"
  echo "== Organisation to organisation: the custodians hand key material to each other, not through the portal"
  check "the DMT's own token cannot fetch the sealed record, even under an approved case -> 403" 403 "$(acode "$CUSTODY_TOKEN_DMT" "$DMT/records/$VID?case=$CASEQ")"
  contains "the refusal names the only caller allowed" "police-service" "$(auth "$CUSTODY_TOKEN_DMT" "$DMT/records/$VID?case=$CASEQ" | field message)"
  check "the Police service's credential cannot store a record at the DMT -> 403" 403 "$(acode "$PSVC" -X PUT "$DMT/records/$VID" -d '{}')"
  check "the Police service's credential cannot fetch a fleet for a DMT approval -> 403" 403 "$(acode "$PSVC" "$DMT/records/$VID/fleet?case=$(urlenc "$CASE3")&purpose=approve")"
  check "the portal's Police token cannot store a Police half (only the DMT service may) -> 403" 403 "$(acode "$CUSTODY_TOKEN_POLICE" -X PUT -H 'content-type: application/json' "$POL/shares/$VID" -d '{"share":"","anchor":""}')"
  ANCHOR=$(dmtsql "select anchor_sha256 from owner_record where vid = '$VID'")
  check "a different Police half for the same owner record is refused (write-once) -> 409" 409 "$(acode "$CUSTODY_TOKEN_DMT_TO_POLICE" -X PUT -H 'content-type: application/json' "$POL/shares/$VID" -d '{"share":"'"$(openssl rand -base64 32)"'","anchor":"'"$ANCHOR"'"}')"
  check "the DMT service handed the Police half over itself at registration" 1 "$(dmtsql "select least(count(*), 1) from release_log where vid = '$VID' and outcome = 'handed over to the Police custody service'")"
  check "the Police service stored that half from the DMT service" 1 "$(polsql "select least(count(*), 1) from release_log where vid = '$VID' and outcome = 'stored, from the DMT custody service'")"
  check "the reveal was opened at the Police service" 1 "$(polsql "select least(count(*), 1) from release_log where vid = '$VID' and case_id = '$CASE' and outcome = 'opened to police'")"
  check "the DMT service released the record to the Police service, and logged who asked" 1 "$(dmtsql "select least(count(*), 1) from release_log where vid = '$VID' and case_id = '$CASE' and outcome = 'released to police-service'")"
  check "a desk keying credential gets a handle from a chassis" "$VID" "$(auth "$CUSTODY_TOKEN_CUSTOMS_KEYING" -X POST -H 'content-type: application/json' "$DMT/handles" -d '{"chassis":"'"$CH"'"}' | field vid)"
  check "a desk keying credential cannot turn a NIC into a pseudonym -> 403" 403 "$(acode "$CUSTODY_TOKEN_CUSTOMS_KEYING" -X POST -H 'content-type: application/json' "$DMT/handles" -d '{"vid":"'"$VID"'","nic":"199012345678"}')"
  post /api/handle '{"chassis":"'"$CH"'","agency":"VET"}' >/dev/null
  check "a desk lookup at the portal is keyed with that desk's own credential" 1 "$(dmtsql "select least(count(*), 1) from release_log where vid = '$VID' and outcome = 'derived to vet-desk'")"
  check "the portal's token file holds no service-to-service credential" 0 "$(grep -c '_TO_' "$ROOT/offchain/custody/portal.env")"
  check "an amount typed with commas is refused before the ledger write" 400 "$(code -X POST "$BASE/api/desk" -d '{"agency":"CUSTOMS","action":"import","recordId":"qa-bad-'"$R"'","chassisNumber":"QA-BAD-'"$R"'","engineNumber":"QA-BADE-'"$R"'","make":"Toyota","internal":{"valuationLkr":"6,850,000"}}')"
fi

if [[ $MODE == services ]]; then
  echo "== Vehicle identity (v1.8): plate at the DMT, description committed at import, the buyer's match"
  CH4="QA-ID-$R"; EN4="QA-IDE-$R"; PL4="QA$R"
  post /api/desk '{"agency":"CUSTOMS","action":"import","recordId":"cusdec-id-'"$R"'","chassisNumber":"'"$CH4"'","engineNumber":"'"$EN4"'","make":"Toyota","model":"Axio","year":2016,"engineCc":1496,"fuel":"PETROL"}' >/dev/null
  REG4='"agency":"DMT","action":"register","chassisNumber":"'"$CH4"'","engineNumber":"'"$EN4"'","make":"Toyota","engineCc":1496,"fuel":"PETROL","plate":"'"$PL4"'","colour":"White","owner":{"name":"Owner D (synthetic)","nic":"198011112222","address":"4 D Rd"}'
  contains "a registration with a different year from the import -> refused" "description committed at import" "$(post /api/desk '{'"$REG4"',"model":"Axio","year":2012}' | field statusMessage)"
  contains "a registration that omits the committed description -> refused" "description committed at import" "$(post /api/desk '{"agency":"DMT","action":"register","chassisNumber":"'"$CH4"'","engineNumber":"'"$EN4"'","make":"Toyota","owner":{"name":"Owner D (synthetic)","nic":"198011112222","address":"4 D Rd"}}' | field statusMessage)"
  contains "the matching registration commits, and the DMT keeps description and plate" "description and plate kept" "$(post /api/desk '{'"$REG4"',"model":"Axio","year":2016}' | field message)"
  V4=$(post /api/handle '{"chassis":"'"$CH4"'","agency":"POLICE"}' | field vid)
  check "a Police desk finds the vehicle by its plate" "$V4" "$(post /api/handle '{"plate":"'"$PL4"'","agency":"POLICE"}' | field vid)"
  check "an insurer desk finds it too" "$V4" "$(post /api/handle '{"plate":"'"$PL4"'","agency":"INSURER"}' | field vid)"
  check "a buyer cannot look up by plate (no BUYER desk)" 400 "$(code -X POST "$BASE/api/handle" -d '{"plate":"'"$PL4"'","agency":"BUYER"}')"
  check "the buyer page's own credential is refused the plate lookup at the DMT" 403 "$(acode "$CUSTODY_TOKEN_BUYER_PORTAL" -X POST -H 'content-type: application/json' "$DMT/plates/lookup" -d '{"plate":"'"$PL4"'"}')"
  check "a second live vehicle on the same plate -> refused before the ledger write" 409 "$(code -X POST "$BASE/api/desk" -d '{"agency":"DMT","action":"register","chassisNumber":"'"$CH4"'-X","engineNumber":"'"$EN4"'-X","make":"Toyota","plate":"'"$PL4"'","owner":{"name":"X","nic":"198011112222","address":"x"}}')"
  check "the buyer sees the description, committed at import" "Axio/2016/True" "$(get "/api/vehicle/$V4/description" | python3 -c "import json,sys; d=json.load(sys.stdin); print(f\"{d['model']}/{d['year']}/{d['committed']}\")")"
  check "the description never carries the plate" 0 "$(get "/api/vehicle/$V4/description" | grep -c -i "plate\|$PL4")"
  check "the buyer's disclosure says the description is committed" "True" "$(get "/api/vehicle/$V4?as=BUYER" | field status.descCommitted)"
  check "chassis match: the stamped chassis -> true" "True" "$(post "/api/vehicle/$V4/match" '{"chassis":"'"$CH4"'"}' | field match)"
  check "chassis match: another car's chassis -> false" "False" "$(post "/api/vehicle/$V4/match" '{"chassis":"'"$CH"'"}' | field match)"
  check "the match answer carries no code that could look up a car" 0 "$(post "/api/vehicle/$V4/match" '{"chassis":"'"$CH4"'"}' | grep -c -E '[0-9a-f]{64}')"
  check "the DMT log names the desk that found a car by plate" 1 "$(dmtsql "select least(count(*), 1) from release_log where vid = '$V4' and outcome = 'derived to police-desk' and item = 'handle from a plate'")"
  check "the DMT log names the buyer page for each match" 1 "$(dmtsql "select least(count(*), 1) from release_log where vid = '$V4' and item = 'chassis match' and outcome = 'matched to buyer-portal'")"
  check "the DMT log has the owner confirmation for the QA theft report" 1 "$(dmtsql "select least(count(*), 1) from release_log where vid = '$VID' and item = 'owner confirmation' and outcome = 'confirmed to police-desk'")"
  check "the DMT log has the refused reporter, without the NIC" 1 "$(dmtsql "select least(count(*), 1) from release_log where vid = '$VID' and item = 'owner confirmation' and outcome like 'refused: not the owner%' and outcome not like '%2000%'")"
  check "the plate never appears in the DMT service's request log" 0 "$(docker logs custody-dmt 2>&1 | grep -c "$PL4")"
  check "the plate is stored only in the DMT database" 1 "$(dmtsql "select count(*) from plate_history where plate = '$PL4' and to_day is null")"
  echo "== v1.9: three wrong NICs per report, and an edited description loses its mark"
  post /api/desk '{"agency":"POLICE","action":"report","vid":"'"$V4"'","recordId":"theft-report-'"$R"'-l","reportRef":"PR/QA/'"$R"'-L"}' >/dev/null
  for n in 1 2 3; do
    code -X POST "$BASE/api/desk" -d '{"agency":"POLICE","action":"theft","vid":"'"$V4"'","recordId":"theft-'"$R"'-l'"$n"'","reportRef":"PR/QA/'"$R"'-L","reporterNic":"20001234567'"$n"'"}' >/dev/null
  done
  check "a fourth NIC on the same report -> refused before any comparison (429)" 429 "$(code -X POST "$BASE/api/desk" -d '{"agency":"POLICE","action":"theft","vid":"'"$V4"'","recordId":"theft-'"$R"'-l4","reportRef":"PR/QA/'"$R"'-L","reporterNic":"198011112222"}')"
  check "the refusals are counted in the DMT log, without the NICs" 3 "$(dmtsql "select count(*) from release_log where vid = '$V4' and case_id = 'PR/QA/$R-L' and outcome like 'refused: not the owner%' and outcome not like '%2000%'")"
  post /api/desk '{"agency":"POLICE","action":"clear","vid":"'"$V4"'","recordId":"clear-'"$R"'-l"}' >/dev/null
  dmtsql "update vehicle_description set model = 'Premio' where vid = '$V4'" >/dev/null
  check "a description edited in the DMT database loses 'committed at import'" "False" "$(get "/api/vehicle/$V4/description" | field committed)"
  dmtsql "update vehicle_description set model = 'Axio' where vid = '$V4'" >/dev/null
  check "restored, it is committed again" "True" "$(get "/api/vehicle/$V4/description" | field committed)"
fi

echo "== v1.7: an approved case opens only the owner record it was approved for"
post /api/desk '{"agency":"DMT","action":"retire","vid":"'"$VID"'","flag":false}' >/dev/null   # back on the register (retired above)
contains "the vehicle is transferred after the case was approved" "previous record replaced" "$(post /api/desk '{"agency":"DMT","action":"transfer","vid":"'"$VID"'","recordId":"tr-'"$R"'-3","owner":{"name":"Owner C (synthetic)","nic":"198812345678","address":"3 C Rd"}}' | field message)"
check "reveal under the old case after the transfer -> refused (409)" 409 "$(code -X POST "$BASE/api/reveal" -d '{"caseId":"'"$CASE"'","vid":"'"$VID"'"}')"
contains "the refusal says the owner record changed since approval" "changed since" "$(post /api/reveal '{"caseId":"'"$CASE"'","vid":"'"$VID"'"}' | field statusMessage)"

echo
echo "QA: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
