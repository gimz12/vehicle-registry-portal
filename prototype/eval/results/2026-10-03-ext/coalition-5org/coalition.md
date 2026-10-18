# Minimum-coalition table (live five-organisation network, chaincode v1.9)

Recorded 2026-10-10T17:48Z on the five-organisation Fabric network: Org1 DMT (also the licensed assembler identity), Org2 Police (two officers), Org3 Customs, Org4 VET, Org5 insurers (three insurer identities), each with its own peer and CA; three Raft orderers. Endorsement policy: the channel default, a majority of the five organisations, so three. Every write below that is not an endorsement row is endorsed by the caller's own peer plus two others. Full transcripts: `transcripts.txt`.

**Role versus organisation.** "1 Customs + 1 DMT" now means two organisations' officers, but the write is still endorsed by three peers chosen by the caller; the chaincode, not the endorsement policy, enforces who may write which record. All five organisations run on one laboratory host with one operator.

| Insider goal | Attempt | Identities involved | Outcome |
|---|---|---|---|
| Register a vehicle Customs never cleared (CIABOC) | DMT officer alone | 1 DMT | refused: registry: registration requires a prior Customs or assembler attestation |
| Register a vehicle Customs never cleared (CIABOC) | Customs officer records an import, then the DMT registers | 1 Customs + 1 DMT | committed |
| Register a vehicle with no import record by claiming local assembly | DMT officer records an assembler attestation for itself | 1 DMT | refused: registry: MSP DMTMSP is not authorised; requires one of [AssemblerMSP] |
| Register a vehicle with no import record by claiming local assembly | A licensed assembler attests the build, then the DMT registers | 1 Assembler + 1 DMT (in the lab the assembler certificate is issued by the DMT organisation's CA) | committed |
| Insert a second registration under an existing engine (COPA) | Customs records an import for a new chassis with the registered engine | 1 Customs | committed |
| Insert a second registration under an existing engine (COPA) | DMT then registers it with the attested engine | 1 Customs + 1 DMT | refused: registry: engine identity is already assigned to vehicle 7709ea8c7108a4c0d19d79e441ccc59876b7b21865159419e272b7cff9b6e656 |
| Insert a second registration under an existing engine (COPA) | DMT registers it with a different engine from the one attested | 1 Customs + 1 DMT | refused: registry: registration does not match the prior CUSTOMS attestation |
| Insert a second registration under an existing engine (COPA) | Customs records the registered engine in upper case (v1.5) | 1 Customs | refused: registry: idxEngine must be a 32-byte lowercase hexadecimal digest |
| Rewrite or delete a committed record | DMT officer calls a delete | 1 DMT | refused: function does not exist |
| Block a sale with a false theft report | One Police officer flags it without the owner's confirmation (v1.8) | 1 Police | refused: registry: a theft flag needs the DMT's owner confirmation for report PR/COAL/58851806, or an approved theft case |
| Block a sale with a false theft report | The Police officer records a report and confirms the owner itself (v1.8) | 1 Police | refused: registry: MSP PoliceMSP is not authorised; requires one of [DMTMSP] |
| Block a sale with a false theft report | The DMT confirms the owner for the report, then the Police flag (v1.8) | 1 Police + 1 DMT | committed |
| Sell a vehicle reported stolen | DMT officer transfers it | 1 DMT | refused: registry: ownership transfer blocked while vehicle is stolen |
| Sell a vehicle reported stolen | One Police officer clears the report, then the DMT transfers | 1 Police + 1 DMT | committed |
| Sell a written-off vehicle | DMT officer transfers it | 1 DMT | refused: registry: ownership transfer blocked while write-off is unresolved |
| Sell a written-off vehicle | Insurer clears the write-off, then the DMT transfers | 1 Insurer + 1 DMT | committed |
| Sell a vehicle still under finance | DMT officer transfers it | 1 DMT | refused: registry: ownership transfer blocked: the vehicle is under finance until the absolute owner's release is recorded |
| Sell a vehicle still under finance | The same DMT officer records a release, then transfers | 1 DMT (negative result: the lender is not on the network) | committed |
| Sell a vehicle the register shows as scrapped | DMT officer transfers it | 1 DMT | refused: registry: ownership transfer blocked: vehicle is retired from the register |
| Sell a vehicle the register shows as scrapped | The same DMT officer returns it to the active register, then transfers | 1 DMT (negative result: retirement is a status any DMT identity can reverse) | committed |
| Reveal an owner without a second agency | The requesting Police officer approves their own case | 1 Police | refused: registry: the officer who requested case COAL/58851806 cannot approve it; a second officer must |
| Reveal an owner without a second agency | A second Police officer approves | 2 Police (case still pending: PENDING) | committed |
| Reveal an owner without a second agency | The DMT approves as well | 2 Police + 1 DMT | committed |
| Re-attest an approved fleet while a case is pending | The DMT approves the same pending case twice (v1.5) | 1 DMT | refused: registry: DMTMSP has already approved case COAL/58851806/2 |
| Write another agency's record type | VET centre reports a theft | 1 VET | refused: registry: MSP VETMSP is not authorised; requires one of [PoliceMSP] |
| File a claim in another insurer's name | insurer-a's certificate files as insurer-b (v1.5) | 1 Insurer | refused: registry: this certificate files claims only as insurer insurer-a, not insurer-b |
| Clear a theft report that was never filed | Police clears a vehicle not flagged stolen (v1.5) | 1 Police | refused: registry: the vehicle is not flagged stolen and has no open theft report |
| Hide an implausible reading from F1b | VET backdates a reading by one day (v1.5) | 1 VET | refused: registry: a mileage reading cannot be dated before the previous reading (day 850) |
| Commit any write with fewer than a majority of organisations | Valid write endorsed by the VET organisation's peer only | 1 organisation | refused: endorsement policy failure |
| Commit any write with fewer than a majority of organisations | The same write endorsed by the VET and DMT peers | 2 organisations | refused: endorsement policy failure |
| Commit any write with fewer than a majority of organisations | The same write endorsed by the VET, DMT and Police peers | 3 organisations (majority of five) | committed |
| Clear an import without Customs | The DMT's CA administrator issues a certificate with the Customs role, which records an import | 1 DMT (CA administrator) | refused: registry: certificate role CUSTOMS may not be issued by Org1MSP |

## Chaincode access checks

| Check | Function | Outcome |
|---|---|---|
| insurer1 reads a case log | GetRevealAudit | refused: registry: MSP InsurerMSP is not authorised; requires one of [DMTMSP PoliceMSP] |
| vet1 reads a case log | GetRevealAudit | refused: registry: MSP VETMSP is not authorised; requires one of [DMTMSP PoliceMSP] |
| customs1 reads a case log | GetRevealAudit | refused: registry: MSP CustomsMSP is not authorised; requires one of [DMTMSP PoliceMSP] |
| Police reads the case log | GetRevealAudit | read |
| Any identity asks for a P0 disclosure directly | FraudDetector:GetDisclosure (removed) | refused: function does not exist |
| Any identity reads a raw history | VehicleRegistry:GetVehicleHistory (removed) | refused: function does not exist |
| An insurer runs the owner-scope predicate | EvaluateOwnerPredicate | refused: registry: MSP InsurerMSP is not authorised; requires one of [DMTMSP] |
