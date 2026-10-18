# Derived-disclosure leak test (before-v1.2)

Frozen manifest `760f75dc35fb7a62d14f43e8f03944484f1326f10593da08fbff51ee13ac28be`. 26 named scenarios, live-style ledger state (P1 pseudonyms, incident links present), one-event perturbations, disclosure recomputed through the chaincode Service per purpose.

A **leak** is any flip caused by perturbing a capability the granted profile does not expose.

| Purpose | Profile | Perturbation | Capability exposed? | Vehicles | With any flip | Leaks | Fields that flipped |
|---|---|---|---|---|---|---|---|
| BUYER | P2 | add accident | ASSET yes | 33 | 0 | 0 |  |
| BUYER | P2 | add claim | ASSET yes | 33 | 33 | 0 | outcome 30, strength 30, evidence tier 32, detail 30, reason 25, event seq 30, list counts 33 |
| BUYER | P2 | add theft flag | ASSET yes | 33 | 33 | 0 | status bits 33, outcome 30, strength 30, evidence tier 29, detail 32, reason 29, event seq 32, list counts 31 |
| BUYER | P2 | change owner pseudonym | OWNER **no** | 33 | 0 | 0 |  |
| BUYER | P2 | re-anchor a claim | INCIDENT **no** | 7 | 5 | 5 | outcome 5, strength 5, detail 5, event seq 5, list counts 5 |
| BUYER | P2 | remove accident | ASSET yes | 8 | 5 | 0 | outcome 5, strength 5, detail 5, event seq 5, list counts 5 |
| BUYER | P2 | shift last reading | ASSET yes | 16 | 14 | 0 | outcome 7, strength 7, detail 14, event seq 7, list counts 6 |
| DMT | P2 | add accident | ASSET yes | 33 | 0 | 0 |  |
| DMT | P2 | add claim | ASSET yes | 33 | 33 | 0 | outcome 30, strength 30, evidence tier 32, detail 30, reason 25, event seq 30, list counts 33 |
| DMT | P2 | add theft flag | ASSET yes | 33 | 33 | 0 | status bits 33, outcome 30, strength 30, evidence tier 29, detail 32, reason 29, event seq 32, list counts 31 |
| DMT | P2 | change owner pseudonym | OWNER **no** | 33 | 0 | 0 |  |
| DMT | P2 | re-anchor a claim | INCIDENT **no** | 7 | 5 | 5 | outcome 5, strength 5, detail 5, event seq 5, list counts 5 |
| DMT | P2 | remove accident | ASSET yes | 8 | 5 | 0 | outcome 5, strength 5, detail 5, event seq 5, list counts 5 |
| DMT | P2 | shift last reading | ASSET yes | 16 | 14 | 0 | outcome 7, strength 7, detail 14, event seq 7, list counts 6 |
| INSURER | P1 | add accident | ASSET yes | 8 | 8 | 0 | outcome 7, strength 1, evidence tier 1, detail 7, reason 1, event seq 7, list counts 8 |
| INSURER | P1 | add claim | ASSET yes | 8 | 8 | 0 | outcome 5, strength 5, evidence tier 7, detail 5, event seq 5, list counts 8 |
| INSURER | P1 | add theft flag | ASSET yes | 8 | 8 | 0 | status bits 8, outcome 8, strength 8, evidence tier 8, detail 8, reason 8, event seq 8, list counts 8 |
| INSURER | P1 | change owner pseudonym | OWNER **no** | 8 | 0 | 0 |  |
| INSURER | P1 | re-anchor a claim | INCIDENT yes | 7 | 6 | 0 | outcome 6, strength 5, detail 6, event seq 6, list counts 6 |
| INSURER | P1 | remove accident | ASSET yes | 7 | 7 | 0 | outcome 7, strength 7, evidence tier 6, detail 6, reason 6, event seq 6, list counts 7 |
| INSURER | P2 | add accident | ASSET yes | 25 | 0 | 0 |  |
| INSURER | P2 | add claim | ASSET yes | 25 | 25 | 0 | outcome 25, strength 25, evidence tier 25, detail 25, reason 25, event seq 25, list counts 25 |
| INSURER | P2 | add theft flag | ASSET yes | 25 | 25 | 0 | status bits 25, outcome 22, strength 22, evidence tier 21, detail 24, reason 21, event seq 24, list counts 23 |
| INSURER | P2 | change owner pseudonym | OWNER **no** | 25 | 0 | 0 |  |
| INSURER | P2 | remove accident | ASSET yes | 1 | 0 | 0 |  |
| INSURER | P2 | shift last reading | ASSET yes | 16 | 14 | 0 | outcome 7, strength 7, detail 14, event seq 7, list counts 6 |
| POLICE | P1 | add accident | ASSET yes | 33 | 33 | 0 | outcome 31, strength 25, evidence tier 25, detail 31, reason 25, event seq 31, list counts 33 |
| POLICE | P1 | add claim | ASSET yes | 33 | 33 | 0 | outcome 30, strength 30, evidence tier 32, detail 30, reason 25, event seq 30, list counts 33 |
| POLICE | P1 | add theft flag | ASSET yes | 33 | 33 | 0 | status bits 33, outcome 30, strength 30, evidence tier 29, detail 32, reason 29, event seq 32, list counts 31 |
| POLICE | P1 | change owner pseudonym | OWNER **no** | 33 | 0 | 0 |  |
| POLICE | P1 | re-anchor a claim | INCIDENT yes | 7 | 6 | 0 | outcome 6, strength 5, detail 6, event seq 6, list counts 6 |
| POLICE | P1 | remove accident | ASSET yes | 8 | 8 | 0 | outcome 8, strength 8, evidence tier 7, detail 7, reason 7, event seq 7, list counts 8 |
| POLICE | P1 | shift last reading | ASSET yes | 16 | 14 | 0 | outcome 7, strength 7, detail 14, event seq 7, list counts 6 |

**Total leaks: 10**
