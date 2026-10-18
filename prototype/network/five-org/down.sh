#!/usr/bin/env bash
# Tears down the five-organisation lab network, its volumes, chaincode images and crypto.
# The off-ledger store and the custody databases are not touched.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../lib.sh"

[[ -f "$TN/five-org/compose.yaml" ]] && docker compose -f "$TN/five-org/compose.yaml" down --volumes --remove-orphans
docker ps -aq --filter name='dev-peer' | xargs -r docker rm -f >/dev/null
docker images -q --filter reference='dev-peer*' | xargs -r docker rmi -f >/dev/null
cd "$TN"
rm -rf organizations/peerOrganizations organizations/ordererOrganizations channel-artifacts log.txt
for d in org1 org2 org3 org4 org5 ordererOrg; do
  rm -rf "organizations/fabric-ca/$d/msp" "organizations/fabric-ca/$d/"{tls-cert.pem,ca-cert.pem,IssuerPublicKey,IssuerRevocationPublicKey,fabric-ca-server.db}
done
echo "Five-organisation network removed."
