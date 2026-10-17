# Corpus C — named scenarios, outcome per profile

| Scenario | Source | Rule | P0 | P1 | P2 | P3 |
|---|---|---|---|---|---|---|
| C01-copa-identity-insertion — Identity of an illegally imported vehicle inserted into a second registration | COPA | F4 | DETECTED | DETECTED | DETECTED | DETECTED |
| C02-ciaboc-no-provenance — Registration with no Customs or assembler record | CIABOC | F5 | DETECTED | DETECTED | DETECTED | DETECTED |
| C03-backdated-import — Import record committed after the registration it claims to support | CIABOC | F5 | DETECTED | DETECTED | DETECTED | DETECTED |
| C04-odometer-rollback — Second VET reading below the first | DMT | F1a | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C05-mileage-blocker — Identical reading after more than a year | DMT | F1c | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C06-implausible-rate — 450,000 km accumulated in one year | DMT | F1b | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C08-stolen-transfer — Ownership transfer while the vehicle stands flagged stolen | DMT | F2 | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C10-unanchored-claim — Claim citing neither police report nor assessor attestation | DMT | F3a | DETECTED | DETECTED | DETECTED | DETECTED |
| C10-unanchored-claim — Claim citing neither police report nor assessor attestation | DMT | F3b | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C11-assessor-only-claim — Claim resting on an insurer assessor attestation only | DMT | F3a | DETECTED | DETECTED | GAP | GAP |
| C12-duplicate-anchor — Two insurers' claims cite the same police report | DMT | F3c | DETECTED | DETECTED | SUPPRESSED | SUPPRESSED |
| C12-duplicate-anchor — Two insurers' claims cite the same police report | DMT | F3d | PASS | PASS | FALSE_POSITIVE | SUPPRESSED |
| C13-cross-insurer-window — Two accidents and two insurers within 30 days | DMT | F3d | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C14-accident-no-claim — Police accident report with no insurance claim | Design | F3b | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C15-serial-rollback — Three vehicles of one owner, each with a rollback within two years | DMT | F1a | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C15-serial-rollback — Three vehicles of one owner, each with a rollback within two years | DMT | F6a | DETECTED | SUPPRESSED | SUPPRESSED | SUPPRESSED |
| C16-serial-assessor — Claims on two of one owner's vehicles share an assessor | DMT | F6b | DETECTED | SUPPRESSED | SUPPRESSED | SUPPRESSED |
| C17-retired-resurrection — Chassis of a retired vehicle registered again | DMT | F4 | DETECTED | DETECTED | DETECTED | DETECTED |
| C18-engine-only-duplicate — Engine index already registered to another vehicle | COPA | F4 | DETECTED | DETECTED | DETECTED | DETECTED |
| C19-honest-on-the-spot-claim — Claim filed before the police report was committed (expected false positive) | Design | F3a | FALSE_POSITIVE | FALSE_POSITIVE | GAP | GAP |
| C25-repeated-stolen-transfers — Two transfers while flagged, then cleared | DMT | F2 | DETECTED | DETECTED | DETECTED | SUPPRESSED |
| C26-serial-rollback-outside-window — Two rollbacks by one owner more than two years apart | Design | F1a | DETECTED | DETECTED | DETECTED | SUPPRESSED |
