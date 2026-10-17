#!/usr/bin/env bash
set -euo pipefail

# Packages the chaincode and runs the Fabric lifecycle on all five organisations.
# Staged under /tmp because the repository path contains a space.
#
# Usage: CHAINCODE_VERSION=1.9 ./network/deploy.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
. "$SCRIPT_DIR/lib.sh"
VERSION="${CHAINCODE_VERSION:-1.9}"
LABEL="${CC}_${VERSION}"

STAGE_DIR="$(mktemp -d /tmp/vehicle-registry-chaincode.XXXXXX)"
trap 'rm -rf "$STAGE_DIR"' EXIT
cp "$PROJECT_ROOT/go.mod" "$PROJECT_ROOT/go.sum" "$STAGE_DIR/"
cp "$PROJECT_ROOT/cmd/chaincode/main.go" "$STAGE_DIR/main.go"
cp -R "$PROJECT_ROOT/chaincode" "$PROJECT_ROOT/rules" "$STAGE_DIR/"
# Tests import packages outside the chaincode module; drop them for the peer's build.
find "$STAGE_DIR" -name '*_test.go' -delete
(cd "$STAGE_DIR" && GO111MODULE=on GOFLAGS=-mod=mod go mod vendor)

PKG="$STAGE_DIR/$LABEL.tar.gz"
echo "== Packaging $LABEL"
peer lifecycle chaincode package "$PKG" --path "$STAGE_DIR" --lang golang --label "$LABEL"
PACKAGE_ID=$(peer lifecycle chaincode calculatepackageid "$PKG")

echo "== Installing on five peers (each builds its own chaincode container)"
for n in "${ALL_ORGS[@]}"; do
  as "admin$n"
  if peer lifecycle chaincode queryinstalled 2>/dev/null | grep -q "$PACKAGE_ID"; then
    echo "  peer0.org$n: already installed"
  else
    out=$(peer lifecycle chaincode install "$PKG" 2>&1) || { echo "$out" >&2; echo "install failed on peer0.org$n" >&2; exit 1; }
    echo "  peer0.org$n: installed"
  fi
done

as admin1
# "not found" means a first deployment; any other failure (a peer down) must stop the script.
if QC=$(peer lifecycle chaincode querycommitted --channelID "$CHANNEL" --name "$CC" 2>&1); then
  COMMITTED=$(echo "$QC" | sed -n 's/.*Sequence: \([0-9]*\),.*/\1/p')
elif [[ $QC == *"not found"* ]]; then
  COMMITTED=0
else
  echo "$QC" >&2; echo "could not read the committed definition" >&2; exit 1
fi
SEQUENCE=$(( ${COMMITTED:-0} + 1 ))
echo "== Approving $LABEL, sequence $SEQUENCE, as each organisation"
for n in "${ALL_ORGS[@]}"; do
  as "admin$n"
  out=$(peer lifecycle chaincode approveformyorg "${ORDERER_ARGS[@]}" --channelID "$CHANNEL" --name "$CC" \
    --version "$VERSION" --package-id "$PACKAGE_ID" --sequence "$SEQUENCE" 2>&1) || { echo "$out" >&2; exit 1; }
  echo "  Org${n}MSP approved"
done

as admin1
peer lifecycle chaincode checkcommitreadiness --channelID "$CHANNEL" --name "$CC" --version "$VERSION" --sequence "$SEQUENCE" --output json \
  | tr -d '\n ' | sed 's/^/  readiness: /'; echo
peer_args "${ALL_ORGS[@]}"
echo "== Committing"
peer lifecycle chaincode commit "${ORDERER_ARGS[@]}" --channelID "$CHANNEL" --name "$CC" \
  --version "$VERSION" --sequence "$SEQUENCE" "${PEER_ARGS[@]}" 2>&1 | grep -E "committed with status|Error" || true
FINAL=$(peer lifecycle chaincode querycommitted --channelID "$CHANNEL" --name "$CC")
echo "$FINAL"
[[ $FINAL == *"Version: $VERSION, Sequence: $SEQUENCE,"* ]] || { echo "the commit did not take effect" >&2; exit 1; }
