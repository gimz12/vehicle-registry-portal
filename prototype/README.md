# Vehicle Registry Prototype

A privacy-configurable, multi-agency vehicle registry for Sri Lanka: Hyperledger
Fabric chaincode, a linkage-ablation experiment, keyed identity primitives,
per-organisation custody services, a signed append-only centralised baseline,
and a Nuxt portal with purpose-bound disclosure.

Every fraud rule returns `PASS`, `FLAGGED`, `GAP_NOTICE` or `SUPPRESSED`, so a
privacy profile cannot turn missing evidence into a clean result: "checks not
performed" is part of every disclosure.

All people, vehicles and numbers in this repository are synthetic.

## Layout

| Path | Contents |
|---|---|
| `rules/` | Twelve rules F1a–F6b, five linkage capabilities, profiles P0–P3, `Decide` and `Measure`. Frozen. |
| `experiment/` | Corpus generator (A), noise model (B), 26 named scenarios (C), view transformer, metric harness, exposure metrics, freeze manifest. Frozen. |
| `identity/` | HMAC vehicle handles and pseudonyms, AES-GCM owner envelopes with split key shares. |
| `baseline/` | Hash-chained, agency-signed event log with cosigned checkpoints, fed to the same rules. |
| `chaincode/registry/` | `VehicleRegistry`, `FraudDetector` and `IdentityManager` contracts; purpose-bound disclosure; dual-approved owner-scope cases with an access log. |
| `services/custody/`, `offchain/custody/` | One custody service and PostgreSQL database per organisation (DMT, Police, Customs, VET, insurers). |
| `offchain/devstore/` | File-store fallback; the store itself (`offchain/dev-store/`) is not committed. |
| `server/`, `app/`, `shared/` | The portal: buyer disclosure, agency desk, Police case page, live network view, results. |
| `network/` | Five-organisation Fabric network and chaincode deployment (see `network/README.md`). |
| `cmd/` | `vlfgen`, `harness`, `sweep`, `freeze`, `disclose`, `chaincode`; identity tools `handle`, `seal`, `ownerindex`; measurements `rates`, `ownerlink`, `diff`. |
| `eval/` | `predictions.md` (frozen), `freeze/<date>/`, `results/<date>/`, and the run, demo and QA scripts. |

## Quick start

With Docker running and the network built once (see "Fabric network" below):

```bash
./eval/demo-up.sh       # network, custody services and portal (about 30 s)
./eval/demo-down.sh     # stop the portal and custody services; --all also stops Fabric
```

The portal runs on `http://localhost:3000`: `/` (buyer), `/desk` (agencies),
`/police/<handle>`, `/live` and `/results`.

## Tests

```bash
GOCACHE=/tmp/rn-bol-go-cache go test ./...   # add -short to skip the 500-vehicle parity control
GOCACHE=/tmp/rn-bol-go-cache go vet ./...
yarn ci:lint
yarn ci:test
```

Against the running network and portal:

```bash
./eval/qa-live.sh          # end-to-end checks through the portal; non-zero on any failure
./eval/custody-verify.sh   # custody databases against the ledger
./eval/coalition.sh        # which coalitions of identities can perform each insider goal
```

`qa-live.sh` and `coalition.sh` write test records to the ledger.

The parity test (`chaincode/registry/parity_test.go`) checks that chaincode,
harness and baseline agree on every vehicle and rule of a 500-vehicle corpus at
every profile. It loads committed state directly, because the write path
refuses the F2, F4 and F5 positives the corpus contains.

## Reproduce the experiment

```bash
./eval/reproduce.sh      # verifies the freeze, runs tests, the sweep and benchmarks
```

By hand:

```bash
go run ./cmd/sweep -seeds 1001,1002,1003 -rates 0.02,0.05,0.08 \
  -frozen eval/freeze/2026-10-03/MANIFEST.sha256 -out eval/results/2026-10-03
```

Outputs: `grid.md` (measured class against prediction, precision and recall
ranges, exposure, disagreements), `frontier.csv`, `scenarios.md`, per-cell JSON
and `latency.txt`.

`go run ./cmd/freeze` writes a SHA-256 manifest over `rules/`, `experiment/` and
`eval/predictions.md`, plus `frozen.tar.gz`; `cmd/sweep -frozen` refuses to run
if any of those files changed. A freeze is never overwritten. The current freeze
is `eval/freeze/2026-10-03` (manifest SHA-256 `760f75dc…`); it supersedes
`2026-10-02` after the F2 correction recorded in `eval/predictions.md`, and both
result sets are kept. Seed 42 is for development only.

Decidability is measured on the transformed view: *suppressed* when no vehicle
could be evaluated, *partial* when the view made some vehicles undecidable,
*available* otherwise. Evidence gaps reduce coverage but not the class.

Post-freeze measurements verify the manifest first and write to
`eval/results/2026-10-03-ext/`:

```bash
go run ./cmd/rates && python3 cmd/rates/plot.py
go run ./cmd/ownerlink
go run ./cmd/diff -label after-v1.3
go test ./chaincode/... -run ParityRaw -v
```

Development corpus only:

```bash
go run ./cmd/vlfgen -vehicles 1000 -seed 42 -fraud-rate 0.08 -out /tmp/rn-bol-vlf
go run ./cmd/harness -corpus /tmp/rn-bol-vlf -out /tmp/rn-bol-vlf/report.json
```

## Fabric network

Install Fabric under a space-free path (`network/README.md`), then from this
directory:

```bash
./network/five-org/up.sh
CHAINCODE_VERSION=1.9 ./network/deploy.sh
./network/register-roles.sh
./network/seed.sh
./offchain/custody/setup-env.sh             # once: service tokens, database passwords, the HMAC keys
docker compose -f offchain/custody/docker-compose.yml up -d --build --wait
python3 offchain/custody/import.py dmt offchain/dev-store | docker exec -i custody-dmt-db psql -q -v ON_ERROR_STOP=1 -U dmt -d dmt
python3 offchain/custody/import.py police offchain/dev-store | docker exec -i custody-police-db psql -q -v ON_ERROR_STOP=1 -U police -d police
python3 eval/demo-live.py                   # demo vehicles
```

Five organisations (DMT, Police, Customs, VET, insurers), each with its own peer
and CA, and three Raft orderers; a write needs three of the five peers.
`register-roles.sh` creates the role identities: the licensed assembler
(`assembler1`, issued by the DMT's CA), insurers `insurer1`–`3`, and a second
Police officer (`police2`), because the officer who requests a case cannot
approve it.

## Portal

```bash
yarn install
yarn dev
```

Runtime settings are `NUXT_FABRIC_*` environment variables (`nuxt.config.ts`).

### Who sees what

| Purpose | Identity | Profile | Scope |
|---|---|---|---|
| BUYER | Org1 User1 | P2 | derived disclosure; no incident links, no owner linkage |
| DMT | Org1 User1 | P2 | vehicle history for the transaction |
| INSURER | Org5 `insurer1`–`3` | P1 where it holds a claim, else P2 | claim-to-incident links on that vehicle |
| POLICE | Org2 User1 | P1; P0 only under an approved case | owner scope needs dual approval |

Each purpose connects through its own organisation's peer. DMT and Police also
see the lifecycle timeline and odometer chart, with incident references
redacted for the DMT.

### Agency desk (`/desk`)

Each agency writes the records its role allows, under its own certificate, and
the chaincode's refusals come back verbatim:

- **Customs:** import clearances, optionally with the vehicle description; the
  ledger keeps only a keyed fingerprint of it.
- **Licensed assembler:** build attestations for locally assembled vehicles.
- **DMT:** registrations (refused without a prior import or attestation, for a
  reused chassis or engine, or for a description that differs from the one
  committed at import), transfers, finance and its release, retirement.
- **VET:** odometer readings (a lower reading is accepted and flagged) and
  emission certificates.
- **Police:** theft reports, theft flags, clearances, accident reports, write-offs.
- **Insurers:** claims and write-offs.

A transfer is refused while the vehicle is flagged stolen, written off, retired
or under finance.

Officers enter identifiers as printed. The DMT custody service holds the HMAC
keys and derives the handle, chassis and engine indices and the owner
pseudonym; spelling variants collapse to one value, so F4 and F5 match records
across agencies. Desks can look up a handle by chassis number or by plate; the
plate stays in the DMT's database and never reaches the ledger. Buyers cannot
look up a vehicle by chassis or plate.

**Theft in three steps.** The Police record a theft report under its report
number; it blocks nothing. Flag stolen sends the reporter's NIC to the DMT
custody service, which checks the report on its own peer, compares the NIC's
owner tag with the vehicle's owner, and on a match writes `OWNER_CONFIRMED`
itself. Only then does the chaincode accept the flag, or under an approved
theft-investigation case. Each report allows three wrong NICs. The Police can
withdraw an unconfirmed report with a clearance.

Registration seals the synthetic owner details (AES-256-GCM, key split into a
DMT half and a Police half) and commits only the anchor. After the commit the
DMT custody service checks the anchor on its own peer, hands the Police half to
the Police custody service directly, and stores its own rows. A transfer
replaces the sealed record; a case approved for a former owner does not open
the new owner's record.

### Buyer page

The buyer page shows the vehicle's description and register status, every
check made, and every check that could not be made. A buyer can type the
chassis number stamped on the car and get one answer: "Matches this record" or
"Does not match". Plate lookups, owner checks and chassis matches are rate
limited per address.

### Police cases (`/police/<handle>`)

A case needs a live owner-attributed alert. The DMT can run an owner-scope
check (F6a, F6b) over the owner's vehicles and return one verdict. A request
names the alert, a legal ground and its reference; it needs approval by the DMT
and by a second Police officer. Every owner-scope read under the case is logged
on the ledger. "Reveal owner identity" asks the Police custody service, which
reads the case on the ledger itself, fetches the DMT's part directly from the
DMT custody service (which also checks the case), decrypts, and logs the reveal
on the ledger before returning the record.

### Live network view (`/live`)

A 3D view of the network that moves a packet for every event the portal or the
ledger reports: endorsement, ordering and commit of each write, reads, custody
checks and releases, and the ledger's block and chaincode event streams. It
draws nothing without an event.

### Results (`/results`)

Renders the frozen sweep output (`NUXT_RESULTS_DIR`, default
`eval/results/2026-10-03`) without recomputing anything.

### Demo mode (no Docker, no Fabric)

```bash
./eval/demo.sh
```

Serves the 26 Corpus C scenarios through the chaincode service in memory. Writes
are disabled.

## Custody services

```bash
./offchain/custody/setup-env.sh                                      # once
docker compose -f offchain/custody/docker-compose.yml up -d --build --wait
./eval/custody-verify.sh
```

Five containers run the same program (`services/custody/main.ts`, `ORG=`), each
with its own PostgreSQL 16 database (`services/custody/sql/<org>.sql`) and
token, on 127.0.0.1:8101–8105 (databases :5441–5445).

| Service | Holds | Releases |
|---|---|---|
| DMT | sealed owner records, the DMT key half, the owner index, the HMAC keys, descriptions and plates | to the Police service only, under a case its own ledger read shows approved; handles to every desk; a yes or no for a theft owner check |
| Police | the Police key half | nothing as a half: it opens the record itself under an approved case and logs the reveal first |
| Customs, VET, insurers | the agency's own internal fields for a ledger record | to that agency only |

Every token names its caller, and each route admits only the callers it lists.
The DMT and Police services call each other directly, so key material never
passes through the portal. A stored key half cannot be replaced by different
bytes.

`NUXT_FABRIC_CUSTODY_MODE=files` makes the portal use the file store
`offchain/dev-store` if the containers are down; `/live` then says so in red.
Plate lookup and the buyer description and match need the services.

## Limitations

- One portal acts for every agency and holds every role's identity; against a
  compromised portal the custody split gives an audit trail, not prevention.
- The services authenticate each other with shared tokens, not mutual TLS.
- All containers run on one host with one operator.
- The owner check proves knowledge of the owner's NIC, not identity; the chassis
  match trusts the stamping on the car.
