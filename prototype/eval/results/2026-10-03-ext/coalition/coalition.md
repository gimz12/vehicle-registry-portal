# Minimum-coalition table (live network, chaincode v1.5)

Recorded 2026-10-08T07:54Z on the two-organisation Fabric test network (Org1 = DMT, also hosting the Customs, assembler, VET and insurer identities by certificate attribute; Org2 = Police, two officers). Default endorsement policy: majority of the channel's organisations, which with two organisations means both. Full transcripts: `transcripts.txt`.

**Role versus organisation.** "1 Customs + 1 DMT" is two registry roles but, in this lab, one organisation. In production each role would be its own organisation, and every write would need the endorsement policy chosen for it.

| Insider goal | Attempt | Identities involved | Outcome |
|---|---|---|---|
| Register a vehicle Customs never cleared (CIABOC) | DMT officer alone | 1 DMT | refused: registry: registration requires a prior Customs or assembler attestation |
| Register a vehicle Customs never cleared (CIABOC) | Customs officer records an import, then the DMT registers | 1 Customs + 1 DMT | committed |
| Register a vehicle with no import record by claiming local assembly | DMT officer records an assembler attestation for itself | 1 DMT | refused: registry: MSP DMTMSP is not authorised; requires one of [AssemblerMSP] |
| Register a vehicle with no import record by claiming local assembly | A licensed assembler attests the build, then the DMT registers | 1 Assembler + 1 DMT | committed |
| Insert a second registration under an existing engine (COPA) | Customs records an import for a new chassis with the registered engine | 1 Customs | committed |
| Insert a second registration under an existing engine (COPA) | DMT then registers it with the attested engine | 1 Customs + 1 DMT | refused: registry: engine identity is already assigned to vehicle 45f5469c6ad0f58c2ff0bc722db5a5e1bebca1a59923eec83d69a2d77b3643c7 |
| Insert a second registration under an existing engine (COPA) | DMT registers it with a different engine from the one attested | 1 Customs + 1 DMT | refused: registry: registration does not match the prior CUSTOMS attestation |
| Insert a second registration under an existing engine (COPA) | Customs records the registered engine in upper case (v1.5) | 1 Customs | refused: registry: idxEngine must be a 32-byte lowercase hexadecimal digest |
| Rewrite or delete a committed record | DMT officer calls a delete | 1 DMT | refused: function does not exist |
| Sell a vehicle reported stolen | DMT officer transfers it | 1 DMT | refused: registry: ownership transfer blocked while vehicle is stolen |
| Sell a vehicle reported stolen | One Police officer clears the report, then the DMT transfers | 1 Police + 1 DMT | committed |
| Sell a written-off vehicle | DMT officer transfers it | 1 DMT | refused: registry: ownership transfer blocked while write-off is unresolved |
| Sell a written-off vehicle | Insurer clears the write-off, then the DMT transfers | 1 Insurer + 1 DMT | committed |
| Sell a vehicle still under finance | DMT officer transfers it | 1 DMT | refused: registry: ownership transfer blocked: the vehicle is under finance until the absolute owner's release is recorded |
| Sell a vehicle still under finance | The same DMT officer records a release, then transfers | 1 DMT (negative result: the lender is not on the network) | committed |
| Sell a vehicle the register shows as scrapped | DMT officer transfers it | 1 DMT | refused: registry: ownership transfer blocked: vehicle is retired from the register |
| Sell a vehicle the register shows as scrapped | The same DMT officer returns it to the active register, then transfers | 1 DMT (negative result: retirement is a status any DMT identity can reverse) | committed |
| Reveal an owner without a second agency | The requesting Police officer approves their own case | 1 Police | refused: registry: the officer who requested case COAL/181202222 cannot approve it; a second officer must |
| Reveal an owner without a second agency | A second Police officer approves | 2 Police (case still pending: PENDING) | committed |
| Reveal an owner without a second agency | The DMT approves as well | 2 Police + 1 DMT | committed |
| Re-attest an approved fleet while a case is pending | The DMT approves the same pending case twice (v1.5) | 1 DMT | refused: registry: DMTMSP has already approved case COAL/181202222/2 |
| Write another agency's record type | VET centre reports a theft | 1 VET | refused: registry: MSP VETMSP is not authorised; requires one of [PoliceMSP] |
| File a claim in another insurer's name | insurer-a's certificate files as insurer-b (v1.5) | 1 Insurer | refused: registry: this certificate files claims only as insurer insurer-a, not insurer-b |
| Clear a theft report that was never filed | Police clears a vehicle not flagged stolen (v1.5) | 1 Police | refused: registry: the vehicle is not flagged stolen |
| Hide an implausible reading from F1b | VET backdates a reading by one day (v1.5) | 1 VET | refused: registry: a mileage reading cannot be dated before the previous reading (day 850) |
| Commit any write with one organisation's endorsement | Valid write endorsed by Org1's peer only | 1 organisation | refused: endorsement policy failure |
| Commit any write with one organisation's endorsement | The same write endorsed by both organisations' peers | 2 organisations | committed |

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
