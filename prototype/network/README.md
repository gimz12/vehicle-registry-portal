# Fabric network

The Fabric binaries, images and `fabric-samples` live at `~/fabric-registry/`,
outside this repository. The repository path contains a space and the
`fabric-samples` scripts are not space-safe, so they run from a space-free path.

## Install (once)

```bash
mkdir -p ~/fabric-registry && cd ~/fabric-registry
curl -sSL https://raw.githubusercontent.com/hyperledger/fabric/main/scripts/install-fabric.sh -o install-fabric.sh
chmod +x install-fabric.sh
./install-fabric.sh --fabric-version 2.5.13 docker samples binary
```

Requires Docker and Go 1.22 or later.

## Organisations

One peer organisation per agency, each with its own peer and certificate
authority, and three Raft orderers. Definitions are in `five-org/`
(`configtx.yaml`, `compose.yaml`); shared helpers in `lib.sh`.

| Organisation | MSP | Agency | Peer | CA | Operations |
|---|---|---|---|---|---|
| Org1 | Org1MSP | DMT (also the licensed assembler identity, `assembler1`) | peer0.org1 :7051 | :7054 | :9444 |
| Org2 | Org2MSP | Sri Lanka Police (`User1`, `police2`) | peer0.org2 :9051 | :8054 | :9445 |
| Org3 | Org3MSP | Sri Lanka Customs (`customs1`) | peer0.org3 :11051 | :11054 | :9449 |
| Org4 | Org4MSP | Vehicle emission test centres (`vet1`) | peer0.org4 :13051 | :13054 | :9450 |
| Org5 | Org5MSP | Insurers (`insurer1`/`2`/`3` = insurer-a/-b/-c) | peer0.org5 :15051 | :15054 | :9451 |
| Orderer org | OrdererMSP | orderer :7050, orderer2 :7052, orderer3 :7056 (Raft) | | :9054 | :9443, :9446, :9447 |

Endorsement uses the channel default, `MAJORITY Endorsement`: any three of the
five organisations' peers. Anchor peers are set in the genesis block.

Laboratory simplifications: the three orderers belong to one ordering
organisation; the CAs keep the samples' bootstrap password; TLS is server-side
only; every port is published on 127.0.0.1 only; everything runs on one host.

## Build it

From the prototype directory:

```bash
./network/five-org/up.sh                    # CAs, crypto, genesis block, 3 orderers, 5 peers joined
CHAINCODE_VERSION=1.9 ./network/deploy.sh   # install on 5 peers, approve as 5 orgs, commit
./network/register-roles.sh                 # role identities, each on its own organisation's CA
./network/seed.sh                           # smoke fleet
```

`up.sh` copies `five-org/` into the fabric-samples test-network directory and
runs there. It refuses to run over existing crypto material.

```bash
./network/five-org/down.sh                  # removes containers, ledgers, crypto and CA databases
```

Day to day, `./eval/demo-up.sh` starts the stopped network, the custody services
and the portal; `./eval/demo-down.sh --all` stops them without deleting anything.

## Deploy or upgrade the chaincode

`deploy.sh` stages the Go module under `/tmp`, vendors and packages it, installs
it on all five peers, approves it as each organisation and commits it. The new
sequence is the committed one plus one; the script stops on any failure.

The chaincode has three contracts: `VehicleRegistry`, `FraudDetector` and
`IdentityManager`. A plain client of Org*N*MSP acts as that organisation's
agency. A `registry.role` certificate attribute is honoured only from the role's
own organisation: CUSTOMS from Org3, VET from Org4, INSURER from Org5, POLICE
from Org2, DMT and ASSEMBLER from Org1.

## Identities

`register-roles.sh` enrols each identity on its own organisation's CA. Insurers
carry `registry.party`, and a claim is filed under the certificate's own party.
The portal maps each purpose to its agency's organisation and peer
(`identityFor` in `server/utils/fabric.ts`): BUYER and DMT use Org1 User1,
POLICE Org2 User1 (approvals `police2`), CUSTOMS `customs1@org3`, VET
`vet1@org4`, INSURER `insurer1`–`3@org5`.
