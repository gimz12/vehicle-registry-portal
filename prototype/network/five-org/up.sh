#!/usr/bin/env bash
# Brings up the five-organisation lab network from nothing (CAs, crypto, channel, orderers, peers).
# Then run ../deploy.sh, ../register-roles.sh and ../seed.sh.
#
# Usage: ./network/five-org/up.sh        (tear down first with ./network/five-org/down.sh)
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../lib.sh"
CFG="$TN/five-org"
COMPOSE=(docker compose -f "$CFG/compose.yaml")
ORDERERS=(orderer:7053 orderer2:7055 orderer3:7057)

if [[ -d "$TN/organizations/peerOrganizations" ]]; then
  echo "Crypto material already exists under $TN/organizations; run ./network/five-org/down.sh first." >&2
  exit 1
fi

mkdir -p "$CFG" "$TN/channel-artifacts"
cp "$HERE/configtx.yaml" "$HERE/compose.yaml" "$CFG/"
# CA server configs: org1, org2 and ordererOrg ship with fabric-samples; org3–5 are derived from org1's.
for n in 3 4 5; do
  mkdir -p "$TN/organizations/fabric-ca/org$n"
  sed -e "s/Org1CA/Org${n}CA/; s/org1\.example\.com/org$n.example.com/g" \
    "$TN/organizations/fabric-ca/org1/fabric-ca-server-config.yaml" > "$TN/organizations/fabric-ca/org$n/fabric-ca-server-config.yaml"
done

echo "== Certificate authorities"
"${COMPOSE[@]}" up -d ca_org1 ca_org2 ca_org3 ca_org4 ca_org5 ca_orderer
for d in org1 org2 org3 org4 org5 ordererOrg; do
  for _ in $(seq 1 30); do [[ -f "$TN/organizations/fabric-ca/$d/tls-cert.pem" ]] && break; sleep 1; done
  [[ -f "$TN/organizations/fabric-ca/$d/tls-cert.pem" ]] || { echo "CA $d did not start" >&2; exit 1; }
done
sleep 2

nodeous() { # nodeous <cacert file name> <output config.yaml>
  local c="cacerts/$1"
  printf 'NodeOUs:\n  Enable: true\n  ClientOUIdentifier: {Certificate: %s, OrganizationalUnitIdentifier: client}\n  PeerOUIdentifier: {Certificate: %s, OrganizationalUnitIdentifier: peer}\n  AdminOUIdentifier: {Certificate: %s, OrganizationalUnitIdentifier: admin}\n  OrdererOUIdentifier: {Certificate: %s, OrganizationalUnitIdentifier: orderer}\n' "$c" "$c" "$c" "$c" > "$2"
}

enrol_org() { # enrol_org N : CA admin, peer0 (msp + tls), User1, Admin
  local n="$1" port; port=$(ca_port "$n")
  local dir; dir=$(org_dir "$n")
  local cert="$TN/organizations/fabric-ca/org$n/ca-cert.pem" ca="ca-org$n"
  local tls=(--caname "$ca" --tls.certfiles "$cert")
  mkdir -p "$dir"
  export FABRIC_CA_CLIENT_HOME="$dir"
  fabric-ca-client enroll -u "https://admin:adminpw@localhost:$port" "${tls[@]}" >/dev/null 2>&1
  nodeous "localhost-$port-$ca.pem" "$dir/msp/config.yaml"
  mkdir -p "$dir/msp/tlscacerts" "$dir/tlsca" "$dir/ca"
  cp "$cert" "$dir/msp/tlscacerts/ca.crt"
  cp "$cert" "$dir/tlsca/tlsca.org$n.example.com-cert.pem"
  cp "$cert" "$dir/ca/ca.org$n.example.com-cert.pem"
  fabric-ca-client register --id.name peer0 --id.secret peer0pw --id.type peer "${tls[@]}" >/dev/null 2>&1
  fabric-ca-client register --id.name user1 --id.secret user1pw --id.type client "${tls[@]}" >/dev/null 2>&1
  fabric-ca-client register --id.name "org${n}admin" --id.secret "org${n}adminpw" --id.type admin "${tls[@]}" >/dev/null 2>&1
  local p="$dir/peers/peer0.org$n.example.com"
  fabric-ca-client enroll -u "https://peer0:peer0pw@localhost:$port" -M "$p/msp" "${tls[@]}" >/dev/null 2>&1
  cp "$dir/msp/config.yaml" "$p/msp/config.yaml"
  fabric-ca-client enroll -u "https://peer0:peer0pw@localhost:$port" -M "$p/tls" --enrollment.profile tls \
    --csr.hosts "peer0.org$n.example.com" --csr.hosts localhost "${tls[@]}" >/dev/null 2>&1
  cp "$p/tls/tlscacerts/"* "$p/tls/ca.crt"; cp "$p/tls/signcerts/"* "$p/tls/server.crt"; cp "$p/tls/keystore/"* "$p/tls/server.key"
  fabric-ca-client enroll -u "https://user1:user1pw@localhost:$port" -M "$dir/users/User1@org$n.example.com/msp" "${tls[@]}" >/dev/null 2>&1
  cp "$dir/msp/config.yaml" "$dir/users/User1@org$n.example.com/msp/config.yaml"
  fabric-ca-client enroll -u "https://org${n}admin:org${n}adminpw@localhost:$port" -M "$dir/users/Admin@org$n.example.com/msp" "${tls[@]}" >/dev/null 2>&1
  cp "$dir/msp/config.yaml" "$dir/users/Admin@org$n.example.com/msp/config.yaml"
  echo "  org$n: peer0, User1, Admin"
}

enrol_orderers() {
  local dir="$TN/organizations/ordererOrganizations/example.com"
  local cert="$TN/organizations/fabric-ca/ordererOrg/ca-cert.pem"
  local tls=(--caname ca-orderer --tls.certfiles "$cert")
  mkdir -p "$dir"
  export FABRIC_CA_CLIENT_HOME="$dir"
  fabric-ca-client enroll -u https://admin:adminpw@localhost:9054 "${tls[@]}" >/dev/null 2>&1
  nodeous localhost-9054-ca-orderer.pem "$dir/msp/config.yaml"
  mkdir -p "$dir/msp/tlscacerts" "$dir/tlsca"
  cp "$cert" "$dir/msp/tlscacerts/tlsca.example.com-cert.pem"
  cp "$cert" "$dir/tlsca/tlsca.example.com-cert.pem"
  for o in orderer orderer2 orderer3; do
    local od="$dir/orderers/$o.example.com"
    fabric-ca-client register --id.name "$o" --id.secret "${o}pw" --id.type orderer "${tls[@]}" >/dev/null 2>&1
    fabric-ca-client enroll -u "https://$o:${o}pw@localhost:9054" -M "$od/msp" "${tls[@]}" >/dev/null 2>&1
    cp "$dir/msp/config.yaml" "$od/msp/config.yaml"
    mv "$od/msp/signcerts/cert.pem" "$od/msp/signcerts/$o.example.com-cert.pem"
    fabric-ca-client enroll -u "https://$o:${o}pw@localhost:9054" -M "$od/tls" --enrollment.profile tls \
      --csr.hosts "$o.example.com" --csr.hosts localhost "${tls[@]}" >/dev/null 2>&1
    cp "$od/tls/tlscacerts/"* "$od/tls/ca.crt"; cp "$od/tls/signcerts/"* "$od/tls/server.crt"; cp "$od/tls/keystore/"* "$od/tls/server.key"
    mkdir -p "$od/msp/tlscacerts"; cp "$od/tls/tlscacerts/"* "$od/msp/tlscacerts/tlsca.example.com-cert.pem"
  done
  fabric-ca-client register --id.name ordererAdmin --id.secret ordererAdminpw --id.type admin "${tls[@]}" >/dev/null 2>&1
  fabric-ca-client enroll -u https://ordererAdmin:ordererAdminpw@localhost:9054 -M "$dir/users/Admin@example.com/msp" "${tls[@]}" >/dev/null 2>&1
  cp "$dir/msp/config.yaml" "$dir/users/Admin@example.com/msp/config.yaml"
  echo "  orderer org: orderer, orderer2, orderer3, Admin"
}

echo "== Enrolling identities"
for n in "${ALL_ORGS[@]}"; do enrol_org "$n"; done
enrol_orderers

echo "== Channel genesis block ($CHANNEL)"
BLOCK="$TN/channel-artifacts/$CHANNEL.block"
(cd "$CFG" && FABRIC_CFG_PATH="$CFG" configtxgen -profile FiveOrgChannel -outputBlock "$BLOCK" -channelID "$CHANNEL" 2>&1 | tail -1)

echo "== Orderers and peers"
DOCKER_SOCK="${DOCKER_HOST:-/var/run/docker.sock}"; export DOCKER_SOCK="${DOCKER_SOCK##unix://}"
"${COMPOSE[@]}" up -d
sleep 3

for o in "${ORDERERS[@]}"; do
  name="${o%%:*}" port="${o##*:}" od="$TN/organizations/ordererOrganizations/example.com/orderers/${o%%:*}.example.com/tls"
  for i in 1 2 3 4 5; do
    osnadmin channel join --channelID "$CHANNEL" --config-block "$BLOCK" -o "localhost:$port" \
      --ca-file "$ORDERER_CA" --client-cert "$od/server.crt" --client-key "$od/server.key" >/dev/null 2>&1 && break
    [[ $i == 5 ]] && { echo "$name could not join $CHANNEL" >&2; exit 1; }
    sleep 2
  done
  echo "  $name joined $CHANNEL"
done

for n in "${ALL_ORGS[@]}"; do
  as "admin$n"
  for i in 1 2 3 4 5 6; do
    peer channel join -b "$BLOCK" >/dev/null 2>&1 && break
    [[ $i == 6 ]] && { echo "peer0.org$n could not join $CHANNEL" >&2; exit 1; }
    sleep 3
  done
  echo "  peer0.org$n joined $CHANNEL (anchor peer from the genesis block)"
done

sleep 3
echo "== Ledger heights"
for ops in 9444 9445 9449 9450 9451; do
  printf '  :%s %s\n' "$ops" "$(curl -s "localhost:$ops/metrics" | grep "^ledger_blockchain_height{channel=\"$CHANNEL\"}" || echo 'not yet reporting')"
done
echo "Network up. Next: ./network/deploy.sh, ./network/register-roles.sh, ./network/seed.sh"
