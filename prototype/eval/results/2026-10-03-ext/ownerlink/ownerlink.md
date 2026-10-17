# Owner linking through the DMT owner index (chaincode v1.3)

Frozen manifest `760f75dc35fb7a62d14f43e8f03944484f1326f10593da08fbff51ee13ac28be`. Seeds 1001–1003 × fraud rates 0.02/0.05/0.08 × 1000 vehicles, pooled per corpus. Vehicles are loaded at P1 pseudonym scope into the chaincode Service; the corpus owner ids stand in for the DMT owner index. The *harness* column is the frozen sweep's P0 measurement of the same rule on the same vehicle.

Eligibility gate: F6a needs an F1a rollback finding at P1 on this vehicle; F6b needs a claim on this vehicle with a named assessor.

## Corpus A

| Measure | F6a | F6b |
|---|---|---|
| Vehicles | 9000 | 9000 |
| Labelled positives | 406 | 416 |
| Predicate calls the gate admits, per 1,000 vehicles | 89.4 | 1000.0 |
| Positives the gate can reach (eligible / all positives) | 1.00 | 1.00 |
| Predicate agrees with the harness P0 verdict (eligible calls) | 1.00 | 1.00 |
| Predicate precision against labels | 1.00 | 1.00 |
| Predicate recall against all positives | 1.00 | 1.00 |
| Predicate answers at ALERT (can ground a case), per 1,000 vehicles | 45.1 | 46.2 |
| Positives that reach ALERT (case-grounding recall) | 1.00 | 1.00 |
| Cases opened and read at case-time P0 | 406 | 416 |
| Case-time verdict agrees with the harness P0 | 1.00 | 1.00 |
| Mean fleet size per admitted call (max) | 2.00 (2) | 2.00 (2) |
| Other vehicles' records a case-time P0 read joins, mean per call | 7.3 | 7.2 |

Corpus properties that bound these numbers: owners by fleet size map[2:4500]; vehicles with a claim naming an assessor 9000 of 9000; odometer readings by evidence grade map[E2:18000].

## Corpus B

| Measure | F6a | F6b |
|---|---|---|
| Vehicles | 9000 | 9000 |
| Labelled positives | 406 | 416 |
| Predicate calls the gate admits, per 1,000 vehicles | 108.2 | 1000.0 |
| Positives the gate can reach (eligible / all positives) | 0.99 | 1.00 |
| Predicate agrees with the harness P0 verdict (eligible calls) | 1.00 | 1.00 |
| Predicate precision against labels | 0.96 | 1.00 |
| Predicate recall against all positives | 0.99 | 1.00 |
| Predicate answers at ALERT (can ground a case), per 1,000 vehicles | 42.7 | 42.9 |
| Positives that reach ALERT (case-grounding recall) | 0.90 | 0.93 |
| Cases opened and read at case-time P0 | 384 | 386 |
| Case-time verdict agrees with the harness P0 | 1.00 | 1.00 |
| Mean fleet size per admitted call (max) | 2.00 (2) | 2.00 (2) |
| Other vehicles' records a case-time P0 read joins, mean per call | 7.4 | 7.3 |

Corpus properties that bound these numbers: owners by fleet size map[2:4500]; vehicles with a claim naming an assessor 9000 of 9000; odometer readings by evidence grade map[E0:184 E1:3578 E2:14238].

**What these numbers can and cannot show.** The generator gives every owner exactly two vehicles and every vehicle a claim naming an assessor, so the F6b gate admits every vehicle here and the fleet size never varies; on a real register the gate admits only vehicles with an assessor-named claim and fleets vary. The generator grades odometer readings E2 (Corpus B noise downgrades some), so F6a reaches ALERT here; the live network grades desk-entered readings E1, so live F6a stays a WARNING and cannot ground a case (disclosed limitation S4). Agreement with the harness is by construction when the owner index is complete; it shows the chaincode path regroups the fleet exactly as the frozen P0 view does, not that the index is correct.

**Exposure per answer.** A predicate call returns one verdict (status and strength) for one vehicle and is stored on the ledger with the fleet hash and the hashed caller id; no record of the owner's other vehicles leaves the chaincode. A case-time P0 read joins the other vehicles' records listed above, and only after two agencies approve.
