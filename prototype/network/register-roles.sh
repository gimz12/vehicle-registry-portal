#!/usr/bin/env bash
# Registers the lab role identities, each on its own organisation's CA:
#   Org1 (DMT) assembler1         registry.role=ASSEMBLER (a DMT-licensed assembler)
#   Org2 (Police) police2         second officer: the requester of a case cannot approve it
#   Org3 (Customs) customs1       registry.role=CUSTOMS
#   Org4 (VET) vet1               registry.role=VET
#   Org5 (Insurers) insurer1/2/3  registry.role=INSURER, registry.party=insurer-a/-b/-c
# A role is honoured only from its own organisation's CA.
# Idempotent: re-running skips identities that already exist.
set -euo pipefail
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

register() { # register <name> [role] [party]
  local name="$1" role="${2:-}" party="${3:-}"
  local n; n=$(org_of "$name")
  local dir; dir=$(org_dir "$n")
  local cert="$TN/organizations/fabric-ca/org$n/ca-cert.pem" port; port=$(ca_port "$n")
  local msp="$dir/users/$name@org$n.example.com/msp" attrs=() enrol=()
  if [[ -d "$msp/signcerts" ]]; then echo "$name already enrolled (org$n)"; return; fi
  if [[ -n "$role" ]]; then
    local a="registry.role=$role:ecert"; [[ -n "$party" ]] && a="$a,registry.party=$party:ecert"
    attrs=(--id.attrs "$a"); enrol=(--enrollment.attrs "registry.role${party:+,registry.party}")
  fi
  FABRIC_CA_CLIENT_HOME="$dir" fabric-ca-client register --caname "ca-org$n" --id.name "$name" --id.secret "${name}pw" \
    --id.type client ${attrs[@]+"${attrs[@]}"} --tls.certfiles "$cert" >/dev/null 2>&1
  FABRIC_CA_CLIENT_HOME="$dir" fabric-ca-client enroll -u "https://$name:${name}pw@localhost:$port" --caname "ca-org$n" \
    -M "$msp" ${enrol[@]+"${enrol[@]}"} --tls.certfiles "$cert" >/dev/null 2>&1
  cp "$dir/msp/config.yaml" "$msp/config.yaml"
  echo "enrolled $name on Org${n}MSP${role:+ as $role}${party:+ ($party)}"
}

register assembler1 ASSEMBLER
register police2
register customs1 CUSTOMS
register vet1 VET
register insurer1 INSURER insurer-a
register insurer2 INSURER insurer-b
register insurer3 INSURER insurer-c
