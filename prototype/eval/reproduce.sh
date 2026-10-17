#!/usr/bin/env bash
# Regenerates the result tables from the frozen inputs; fails if any frozen file
# differs from the manifest. Usage: ./eval/reproduce.sh [freeze-date] [out-dir]
set -euo pipefail
cd "$(dirname "$0")/.."
FREEZE="${1:-2026-10-03}"
OUT="${2:-eval/results/${FREEZE}}"
MANIFEST="eval/freeze/${FREEZE}/MANIFEST.sha256"
export GOCACHE="${GOCACHE:-/tmp/rn-bol-go-cache}"

[ -f "$MANIFEST" ] || { echo "no freeze manifest at $MANIFEST" >&2; exit 1; }
echo "== frozen manifest sha256: $(shasum -a 256 "$MANIFEST" | cut -d' ' -f1)"

echo "== conformance: unit tests, parity control, vet"
go vet ./...
go test ./...

echo "== held-out sweep (corpora A and B, Corpus C scenarios)"
go run ./cmd/sweep \
  -seeds 1001,1002,1003 -rates 0.02,0.05,0.08 -vehicles 1000 \
  -frozen "$MANIFEST" -out "$OUT"

echo "== laboratory latency (in-process; Fabric round trip is logged by the portal)"
go test -run '^$' -bench 'Disclosure|Verify' -benchmem ./chaincode/... | tee "$OUT/latency.txt"

echo "== outputs"
ls -1 "$OUT"
