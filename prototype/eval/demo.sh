#!/usr/bin/env bash
# Viva demo without Docker or Fabric: generates the Corpus C disclosures through
# the chaincode Service (in-memory state) and starts the portal on them.
# Usage: ./eval/demo.sh            (then open http://localhost:3000)
set -euo pipefail
cd "$(dirname "$0")/.."
export GOCACHE="${GOCACHE:-/tmp/rn-bol-go-cache}"
DEMO="${DEMO_DIR:-$(mktemp -d /tmp/rn-bol-demo.XXXXXX)}"
go run ./cmd/disclose -out "$DEMO"
echo "== demo disclosures in $DEMO; starting portal in demo mode"
NUXT_FABRIC_DEMO_DIR="$DEMO" exec yarn dev
