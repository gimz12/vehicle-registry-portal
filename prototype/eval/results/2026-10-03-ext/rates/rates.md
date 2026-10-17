# Table 3.3 measures on the held-out sweep

Frozen manifest `760f75dc35fb7a62d14f43e8f03944484f1326f10593da08fbff51ee13ac28be`. Seeds [1001 1002 1003] × fraud rates [0.02 0.05 0.08] × 1000 vehicles per run, Corpus A (clean) and Corpus B (noisy); the same corpora as `eval/results/2026-10-03`. Rates are pooled over the nine runs per corpus; precision and recall are also given as the range across runs.

- **Coverage**: share of vehicles the rule could evaluate in this view (PASS or FLAGGED).
- **Gap, structural**: not evaluated because the view itself removed what the rule needs (redacted reference, split history, removed owner link).
- **Gap, evidence**: not evaluated because the records were missing or too weak (E0), with the view intact.
- **Declared**: what the chaincode gate (`Decide`) answers for this profile: *suppressed* when the profile withholds the rule's required capability. The measured columns come from the view (`Measure`), as in the frozen sweep, and are kept apart.
- **Conditional precision / recall**: among evaluated vehicles only. **Effective recall**: detected positives over all positives, including those the view could not evaluate; this is the figure the sweep's `grid.md` reports as recall.
- **Unevaluated positives**: share of positives the view could not evaluate; the difference between conditional and effective recall comes from these.

## Corpus A

| Rule | Profile | Declared | Coverage | Gap, structural | Gap, evidence | Positives (evaluated) | Unevaluated positives | Cond. precision | Cond. recall | Effective recall |
|---|---|---|---|---|---|---|---|---|---|---|
| F1a | P0 | evaluated | 1.00 | 0.00 | 0.00 | 805 (805) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1a | P1 | evaluated | 1.00 | 0.00 | 0.00 | 805 (805) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1a | P2 | evaluated | 1.00 | 0.00 | 0.00 | 805 (805) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1a | P3 | suppressed | 0.00 | 1.00 | 0.00 | 805 (0) | 1.00 | — | — | 0.00 |
| F1b | P0 | evaluated | 1.00 | 0.00 | 0.00 | 369 (369) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1b | P1 | evaluated | 1.00 | 0.00 | 0.00 | 369 (369) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1b | P2 | evaluated | 1.00 | 0.00 | 0.00 | 369 (369) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1b | P3 | suppressed | 0.00 | 1.00 | 0.00 | 369 (0) | 1.00 | — | — | 0.00 |
| F1c | P0 | evaluated | 1.00 | 0.00 | 0.00 | 373 (373) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1c | P1 | evaluated | 1.00 | 0.00 | 0.00 | 373 (373) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1c | P2 | evaluated | 1.00 | 0.00 | 0.00 | 373 (373) | 0.00 | 1.00 | 1.00 | 1.00 |
| F1c | P3 | suppressed | 0.00 | 1.00 | 0.00 | 373 (0) | 1.00 | — | — | 0.00 |
| F2 | P0 | evaluated | 1.00 | 0.00 | 0.00 | 468 (468) | 0.00 | 1.00 | 1.00 | 1.00 |
| F2 | P1 | evaluated | 1.00 | 0.00 | 0.00 | 468 (468) | 0.00 | 1.00 | 1.00 | 1.00 |
| F2 | P2 | evaluated | 1.00 | 0.00 | 0.00 | 468 (468) | 0.00 | 1.00 | 1.00 | 1.00 |
| F2 | P3 | suppressed | 0.00 | 0.00 | 1.00 | 468 (0) | 1.00 | — | — | 0.00 |
| F3a | P0 | evaluated | 1.00 | 0.00 | 0.00 | 475 (475) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3a | P1 | evaluated | 1.00 | 0.00 | 0.00 | 475 (475) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3a | P2 | evaluated | 0.05 | 0.95 | 0.00 | 475 (475) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3a | P3 | evaluated | 0.05 | 0.95 | 0.00 | 475 (475) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3b | P0 | evaluated | 1.00 | 0.00 | 0.00 | 910 (910) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3b | P1 | evaluated | 1.00 | 0.00 | 0.00 | 910 (910) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3b | P2 | suppressed | 0.05 | 0.95 | 0.00 | 910 (475) | 0.48 | 1.00 | 1.00 | 0.45–0.70 |
| F3b | P3 | suppressed | 0.00 | 1.00 | 0.00 | 910 (0) | 1.00 | — | — | 0.00 |
| F3c | P0 | evaluated | 1.00 | 0.00 | 0.00 | 438 (438) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3c | P1 | evaluated | 1.00 | 0.00 | 0.00 | 438 (438) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3c | P2 | suppressed | 0.05 | 0.95 | 0.00 | 438 (0) | 1.00 | — | — | 0.00 |
| F3c | P3 | suppressed | 0.05 | 0.95 | 0.00 | 438 (0) | 1.00 | — | — | 0.00 |
| F3d | P0 | evaluated | 1.00 | 0.00 | 0.00 | 395 (395) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3d | P1 | evaluated | 1.00 | 0.00 | 0.00 | 395 (395) | 0.00 | 1.00 | 1.00 | 1.00 |
| F3d | P2 | evaluated | 1.00 | 0.00 | 0.00 | 395 (395) | 0.00 | 0.38–0.69 | 1.00 | 1.00 |
| F3d | P3 | suppressed | 0.00 | 1.00 | 0.00 | 395 (0) | 1.00 | — | — | 0.00 |
| F4 | P0 | evaluated | 1.00 | 0.00 | 0.00 | 482 (482) | 0.00 | 1.00 | 1.00 | 1.00 |
| F4 | P1 | evaluated | 1.00 | 0.00 | 0.00 | 482 (482) | 0.00 | 1.00 | 1.00 | 1.00 |
| F4 | P2 | evaluated | 1.00 | 0.00 | 0.00 | 482 (482) | 0.00 | 1.00 | 1.00 | 1.00 |
| F4 | P3 | evaluated | 1.00 | 0.00 | 0.00 | 482 (482) | 0.00 | 1.00 | 1.00 | 1.00 |
| F5 | P0 | evaluated | 1.00 | 0.00 | 0.00 | 434 (434) | 0.00 | 1.00 | 1.00 | 1.00 |
| F5 | P1 | evaluated | 1.00 | 0.00 | 0.00 | 434 (434) | 0.00 | 1.00 | 1.00 | 1.00 |
| F5 | P2 | evaluated | 1.00 | 0.00 | 0.00 | 434 (434) | 0.00 | 1.00 | 1.00 | 1.00 |
| F5 | P3 | evaluated | 1.00 | 0.00 | 0.00 | 434 (434) | 0.00 | 1.00 | 1.00 | 1.00 |
| F6a | P0 | evaluated | 1.00 | 0.00 | 0.00 | 406 (406) | 0.00 | 1.00 | 1.00 | 1.00 |
| F6a | P1 | suppressed | 0.00 | 1.00 | 0.00 | 406 (0) | 1.00 | — | — | 0.00 |
| F6a | P2 | suppressed | 0.00 | 1.00 | 0.00 | 406 (0) | 1.00 | — | — | 0.00 |
| F6a | P3 | suppressed | 0.00 | 1.00 | 0.00 | 406 (0) | 1.00 | — | — | 0.00 |
| F6b | P0 | evaluated | 1.00 | 0.00 | 0.00 | 416 (416) | 0.00 | 1.00 | 1.00 | 1.00 |
| F6b | P1 | suppressed | 0.00 | 1.00 | 0.00 | 416 (0) | 1.00 | — | — | 0.00 |
| F6b | P2 | suppressed | 0.00 | 1.00 | 0.00 | 416 (0) | 1.00 | — | — | 0.00 |
| F6b | P3 | suppressed | 0.00 | 1.00 | 0.00 | 416 (0) | 1.00 | — | — | 0.00 |

## Corpus B

| Rule | Profile | Declared | Coverage | Gap, structural | Gap, evidence | Positives (evaluated) | Unevaluated positives | Cond. precision | Cond. recall | Effective recall |
|---|---|---|---|---|---|---|---|---|---|---|
| F1a | P0 | evaluated | 0.99 | 0.00 | 0.01 | 805 (794) | 0.01 | 0.62–0.93 | 1.00 | 0.96–1.00 |
| F1a | P1 | evaluated | 0.99 | 0.00 | 0.01 | 805 (794) | 0.01 | 0.62–0.93 | 1.00 | 0.96–1.00 |
| F1a | P2 | evaluated | 0.99 | 0.00 | 0.01 | 805 (794) | 0.01 | 0.62–0.93 | 1.00 | 0.96–1.00 |
| F1a | P3 | suppressed | 0.00 | 1.00 | 0.00 | 805 (0) | 1.00 | — | — | 0.00 |
| F1b | P0 | evaluated | 0.99 | 0.00 | 0.01 | 369 (367) | 0.01 | 0.63–0.99 | 1.00 | 0.98–1.00 |
| F1b | P1 | evaluated | 0.99 | 0.00 | 0.01 | 369 (367) | 0.01 | 0.63–0.99 | 1.00 | 0.98–1.00 |
| F1b | P2 | evaluated | 0.99 | 0.00 | 0.01 | 369 (367) | 0.01 | 0.63–0.99 | 1.00 | 0.98–1.00 |
| F1b | P3 | suppressed | 0.00 | 1.00 | 0.00 | 369 (0) | 1.00 | — | — | 0.00 |
| F1c | P0 | evaluated | 0.99 | 0.00 | 0.01 | 373 (369) | 0.01 | 1.00 | 1.00 | 0.97–1.00 |
| F1c | P1 | evaluated | 0.99 | 0.00 | 0.01 | 373 (369) | 0.01 | 1.00 | 1.00 | 0.97–1.00 |
| F1c | P2 | evaluated | 0.99 | 0.00 | 0.01 | 373 (369) | 0.01 | 1.00 | 1.00 | 0.97–1.00 |
| F1c | P3 | suppressed | 0.00 | 1.00 | 0.00 | 373 (0) | 1.00 | — | — | 0.00 |
| F2 | P0 | evaluated | 0.99 | 0.00 | 0.01 | 468 (468) | 0.00 | 1.00 | 1.00 | 1.00 |
| F2 | P1 | evaluated | 0.99 | 0.00 | 0.01 | 468 (468) | 0.00 | 1.00 | 1.00 | 1.00 |
| F2 | P2 | evaluated | 0.99 | 0.00 | 0.01 | 468 (468) | 0.00 | 1.00 | 1.00 | 1.00 |
| F2 | P3 | suppressed | 0.00 | 0.00 | 1.00 | 468 (0) | 1.00 | — | — | 0.00 |
| F3a | P0 | evaluated | 0.99 | 0.00 | 0.01 | 475 (469) | 0.01 | 0.22–0.66 | 1.00 | 0.93–1.00 |
| F3a | P1 | evaluated | 0.99 | 0.00 | 0.01 | 475 (469) | 0.01 | 0.22–0.66 | 1.00 | 0.93–1.00 |
| F3a | P2 | evaluated | 0.11 | 0.89 | 0.01 | 475 (469) | 0.01 | 0.27–0.72 | 1.00 | 0.93–1.00 |
| F3a | P3 | evaluated | 0.11 | 0.89 | 0.01 | 475 (469) | 0.01 | 0.27–0.72 | 1.00 | 0.93–1.00 |
| F3b | P0 | evaluated | 1.00 | 0.00 | 0.00 | 910 (910) | 0.00 | 0.29–0.82 | 1.00 | 1.00 |
| F3b | P1 | evaluated | 1.00 | 0.00 | 0.00 | 910 (910) | 0.00 | 0.29–0.82 | 1.00 | 1.00 |
| F3b | P2 | suppressed | 0.10 | 0.90 | 0.00 | 910 (484) | 0.47 | 0.28–0.78 | 1.00 | 0.47–0.70 |
| F3b | P3 | suppressed | 0.00 | 1.00 | 0.00 | 910 (0) | 1.00 | — | — | 0.00 |
| F3c | P0 | evaluated | 0.99 | 0.00 | 0.01 | 438 (434) | 0.01 | 0.43–0.85 | 0.91–1.00 | 0.89–1.00 |
| F3c | P1 | evaluated | 0.99 | 0.00 | 0.01 | 438 (434) | 0.01 | 0.43–0.85 | 0.91–1.00 | 0.89–1.00 |
| F3c | P2 | suppressed | 0.10 | 0.90 | 0.00 | 438 (0) | 1.00 | — | — | 0.00 |
| F3c | P3 | suppressed | 0.11 | 0.89 | 0.00 | 438 (22) | 0.95 | — | 0.00 | 0.00 |
| F3d | P0 | evaluated | 0.99 | 0.00 | 0.01 | 395 (394) | 0.00 | 0.33–0.72 | 1.00 | 0.98–1.00 |
| F3d | P1 | evaluated | 0.99 | 0.00 | 0.01 | 395 (394) | 0.00 | 0.33–0.72 | 1.00 | 0.98–1.00 |
| F3d | P2 | evaluated | 0.99 | 0.00 | 0.01 | 395 (394) | 0.00 | 0.22–0.50 | 1.00 | 0.98–1.00 |
| F3d | P3 | suppressed | 0.00 | 1.00 | 0.00 | 395 (0) | 1.00 | — | — | 0.00 |
| F4 | P0 | evaluated | 1.00 | 0.00 | 0.00 | 482 (482) | 0.00 | 1.00 | 1.00 | 1.00 |
| F4 | P1 | evaluated | 1.00 | 0.00 | 0.00 | 482 (482) | 0.00 | 1.00 | 1.00 | 1.00 |
| F4 | P2 | evaluated | 1.00 | 0.00 | 0.00 | 482 (482) | 0.00 | 1.00 | 1.00 | 1.00 |
| F4 | P3 | evaluated | 1.00 | 0.00 | 0.00 | 482 (482) | 0.00 | 1.00 | 1.00 | 1.00 |
| F5 | P0 | evaluated | 1.00 | 0.00 | 0.00 | 434 (434) | 0.00 | 1.00 | 1.00 | 1.00 |
| F5 | P1 | evaluated | 1.00 | 0.00 | 0.00 | 434 (434) | 0.00 | 1.00 | 1.00 | 1.00 |
| F5 | P2 | evaluated | 1.00 | 0.00 | 0.00 | 434 (434) | 0.00 | 1.00 | 1.00 | 1.00 |
| F5 | P3 | evaluated | 1.00 | 0.00 | 0.00 | 434 (434) | 0.00 | 1.00 | 1.00 | 1.00 |
| F6a | P0 | evaluated | 0.99 | 0.00 | 0.01 | 406 (403) | 0.01 | 0.82–1.00 | 1.00 | 0.97–1.00 |
| F6a | P1 | suppressed | 0.00 | 1.00 | 0.00 | 406 (0) | 1.00 | — | — | 0.00 |
| F6a | P2 | suppressed | 0.00 | 1.00 | 0.00 | 406 (0) | 1.00 | — | — | 0.00 |
| F6a | P3 | suppressed | 0.00 | 1.00 | 0.00 | 406 (0) | 1.00 | — | — | 0.00 |
| F6b | P0 | evaluated | 1.00 | 0.00 | 0.00 | 416 (416) | 0.00 | 1.00 | 1.00 | 1.00 |
| F6b | P1 | suppressed | 0.00 | 1.00 | 0.00 | 416 (0) | 1.00 | — | — | 0.00 |
| F6b | P2 | suppressed | 0.00 | 1.00 | 0.00 | 416 (0) | 1.00 | — | — | 0.00 |
| F6b | P3 | suppressed | 0.00 | 1.00 | 0.00 | 416 (0) | 1.00 | — | — | 0.00 |

## Positives per rule per run

Minimum–maximum positive vehicles per run (nine runs per corpus); every precision and recall range above rests on these counts.

| Rule | Corpus A | Corpus B |
|---|---|---|
| F1a | 32–156 | 32–156 |
| F1b | 12–66 | 12–66 |
| F1c | 14–68 | 14–68 |
| F2 | 22–85 | 22–85 |
| F3a | 21–102 | 21–102 |
| F3b | 30–183 | 30–183 |
| F3c | 15–81 | 15–81 |
| F3d | 13–85 | 13–85 |
| F4 | 22–94 | 22–94 |
| F5 | 16–80 | 16–80 |
| F6a | 18–86 | 18–86 |
| F6b | 10–84 | 10–84 |

## Why a check was not performed, as the asker sees it

Every labelled (vehicle, rule) answer through the chaincode gate (`Decide`, the path the portal shows), pooled over nine runs. The buyer page labels these *Withheld*, *Link removed* and *No evidence*. A vehicle split by P3 counts once, by its best answer.

| Corpus | Profile | Performed | Withheld by policy | Link removed (structural) | No evidence |
|---|---|---|---|---|---|
| A | P0 | 1.00 | 0.00 | 0.00 | 0.00 |
| A | P1 | 0.83 | 0.17 | 0.00 | 0.00 |
| A | P2 | 0.59 | 0.33 | 0.08 | 0.00 |
| A | P3 | 0.17 | 0.75 | 0.08 | 0.00 |
| B | P0 | 0.99 | 0.00 | 0.00 | 0.01 |
| B | P1 | 0.83 | 0.17 | 0.00 | 0.01 |
| B | P2 | 0.59 | 0.33 | 0.07 | 0.00 |
| B | P3 | 0.18 | 0.75 | 0.07 | 0.00 |
