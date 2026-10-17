# Development hypotheses — linkage capabilities and rule behaviour

**Status:** FROZEN on 3 October 2026 (supersedes the 2 October freeze, which is
kept unchanged in `eval/freeze/2026-10-02/`). The SHA-256 manifest over
`rules/`, `experiment/` and this file is in `eval/freeze/2026-10-03/MANIFEST.sha256`;
`cmd/sweep -frozen` refuses to run if any of those inputs differ from it. Development seed `42` was used for every
correction recorded below and is never reported as a held-out result. Held-out
seeds: 1001, 1002, 1003; fraud rates 0.02, 0.05, 0.08; corpora A and B.

## Capability model

The profiles vary several kinds of linkage, not just the spelling of an owner
pseudonym:

| Capability | Meaning |
|---|---|
| `RECORD` | A rule can inspect fields on one record without joining it to another record. |
| `INDEX` | Selected identity/provenance records retain keyed chassis and engine joins. |
| `ASSET` | Events belonging to one vehicle can be grouped and ordered. |
| `INCIDENT` | A claim can be joined to the specific accident/evidence record it cites. |
| `OWNER` | Different vehicles belonging to one owner can be grouped. |

`INDEX` is retained linkage. P3 must therefore be described as hiding general
vehicle histories, not as eliminating every possible cross-record correlation.

| Profile | Capabilities exposed |
|---|---|
| P0 | RECORD, INDEX, ASSET, INCIDENT, OWNER |
| P1 | RECORD, INDEX, ASSET, INCIDENT |
| P2 | RECORD, INDEX, ASSET |
| P3 | RECORD, INDEX |

## Rule-level declarations — 48 configuration cells

`✓` means fully declared available, `△` means partially informative but expected
to lose accuracy for some subtypes, and `✗` means declared suppressed.

| Rule | Capability | P0 | P1 | P2 | P3 | Development hypothesis |
|---|---|:--:|:--:|:--:|:--:|---|
| F1a rollback | ASSET | ✓ | ✓ | ✓ | ✗ | Recall collapses only when vehicle histories split. |
| F1b implausible rate | ASSET | ✓ | ✓ | ✓ | ✗ | Same breakpoint as F1a. |
| F1c stagnation | ASSET | ✓ | ✓ | ✓ | ✗ | Same breakpoint as F1a. |
| F2 stolen-vehicle check | ASSET | ✓ | ✓ | ✓ | ✗ | Theft flag and transfer cannot be joined at P3. Since 3 Oct 2026 the rule also reports a vehicle that is currently flagged stolen, because the chaincode refuses the transfer at write and no transfer record can exist; the breakpoint is unchanged. |
| F3a missing/invalid evidence | RECORD | ✓ | ✓ | △ | △ | A genuinely missing reference remains visible on the claim itself at every profile; a redacted reference (P2, P3) produces a structural gap, not a clean pass, so the cell is partial there. |
| F3b accident without claim | INCIDENT | ✓ | ✓ | △ | ✗ | At P2, genuinely unanchored claims remain informative but referenced claims cannot be matched. |
| F3c duplicate evidence anchor | INCIDENT | ✓ | ✓ | △ | △ | Detection collapses (recall 0) when references are redacted; claims that never carried an anchor remain trivially decidable, so the cell is partial rather than fully suppressed. |
| F3d cross-insurer window | ASSET | ✓ | ✓ | △ | ✗ | Recall survives P2, but precision is expected to fall because shared-anchor pairs can no longer be excluded. |
| F4 identity uniqueness | INDEX | ✓ | ✓ | ✓ | ✓ | Control row: recall and precision should remain flat. |
| F5 provenance prerequisite | INDEX | ✓ | ✓ | ✓ | ✓ | Control row: recall and precision should remain flat. |
| F6a serial odometer pattern | OWNER | ✓ | ✗ | ✗ | ✗ | Requires grouping an owner's vehicles. |
| F6b serial claim pattern | OWNER | ✓ | ✗ | ✗ | ✗ | Requires grouping an owner's vehicles. |

## Why this replaces the earlier table

The earlier family-level table contained three problems caught before the final
evaluation:

1. Its test read `RequiredScope` and compared it with `Decide()`, which used the
   same field. That is a configuration regression test, not independent evidence.
2. F3d needs the vehicle history, not a specific incident join; it therefore does
   not share F3c's breakpoint.
3. A claim whose evidence reference was absent at source remains detectable even
   after incident links are removed. Privacy redaction and source absence must be
   represented as different states.

The implementation now separates `Measure()` from declared suppression and
constructs P0–P3 as structurally different ledger views. The final headline must
report recall and precision changes, including partial cells, rather than forcing
every rule into a binary staircase.

## Corrections made before the freeze (2 October 2026, development seed 42)

Two glyphs were corrected after the decidability class was computed from the
transformed view with structural gaps separated from evidence gaps:

1. **F3a at P2/P3: ✓ → △.** The row's own note already said a redacted reference
   "produces a gap, not a clean pass"; the glyph did not reflect that.
2. **F3c at P2/P3: ✗ → △.** Recall is 0 as predicted, but vehicles whose claims
   carry no anchor at source are still decidable (no anchor, no duplicate).

Both corrections are structural facts of the rule definitions, visible on the
development seed only. No threshold, rule or transformation was changed.

Decidability class is measured as: *suppressed* if no vehicle could be
evaluated; *partial* if at least one vehicle received a gap caused by the data
view (redaction, split history, removed owner link); *available* otherwise. Gaps
caused by weak evidence (E0 records) reduce coverage but do not change the
class, which is what lets H4 be tested.

## Correction made on 3 October 2026 (re-freeze)

The live walkthrough showed that a vehicle flagged stolen read "no evidence" for
F2 in the portal: the chaincode refuses a transfer while flagged, so the committed
history never contains the transfer the rule looked for. F2 now also reports the
current theft status (an owner-attributed alert). Its required capability and its
P3 breakpoint are unchanged; the generated corpora contain no flag-only vehicles,
so no cell of the grid moves. Scenario C22 changed from negative to F2-positive.
The 2 October results are retained; the 3 October sweep is the one reported.

## Final-evaluation freeze conditions

Before the thesis sweep:

- freeze the generator, transformer, label consistency checks, this table, and
  the final seeds;
- record file hashes and a version identifier without modifying them after the
  first held-out run;
- reserve seed 42 for development only;
- run three fraud rates and at least three untouched seeds;
- report every disagreement and partial cell without retuning the prediction;
- require F4/F5 flatness and 500-vehicle chaincode/harness parity as controls.
