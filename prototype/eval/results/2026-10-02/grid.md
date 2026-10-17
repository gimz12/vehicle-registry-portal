# Linkage-ablation results

Seeds [1001 1002 1003] · fraud rates [0.02 0.05 0.08] · 1000 vehicles per run · corpora [A B]

## Corpus A — measured class vs prediction (P / R / F1 as range across seeds × rates)

| Rule | P0 | P1 | P2 | P3 |
|---|---|---|---|---|
| F1a | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F1b | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F1c | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F2 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F3a | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | △ ✔ partial<br>P 1.00 R 1.00 F1 1.00 | △ ✔ partial<br>P 1.00 R 1.00 F1 1.00 |
| F3b | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | △ ✔ partial<br>P 1.00 R 0.45–0.70 F1 0.62–0.82 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F3c | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | △ ✔ partial<br>P 0.00 R 0.00 F1 0.00 | △ ✔ partial<br>P 0.00 R 0.00 F1 0.00 |
| F3d | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | △ ✔ available<br>P 0.38–0.69 R 1.00 F1 0.55–0.81 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F4 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 |
| F5 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 |
| F6a | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F6b | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |

48 of 48 cells agree with the frozen prediction.

## Corpus B — measured class vs prediction (P / R / F1 as range across seeds × rates)

| Rule | P0 | P1 | P2 | P3 |
|---|---|---|---|---|
| F1a | ✓ ✔ available<br>P 0.62–0.93 R 0.96–1.00 F1 0.76–0.97 | ✓ ✔ available<br>P 0.62–0.93 R 0.96–1.00 F1 0.76–0.97 | ✓ ✔ available<br>P 0.62–0.93 R 0.96–1.00 F1 0.76–0.97 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F1b | ✓ ✔ available<br>P 0.63–0.99 R 0.98–1.00 F1 0.77–0.99 | ✓ ✔ available<br>P 0.63–0.99 R 0.98–1.00 F1 0.77–0.99 | ✓ ✔ available<br>P 0.63–0.99 R 0.98–1.00 F1 0.77–0.99 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F1c | ✓ ✔ available<br>P 1.00 R 0.97–1.00 F1 0.98–1.00 | ✓ ✔ available<br>P 1.00 R 0.97–1.00 F1 0.98–1.00 | ✓ ✔ available<br>P 1.00 R 0.97–1.00 F1 0.98–1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F2 | ✓ ✔ available<br>P 1.00 R 0.96–1.00 F1 0.98–1.00 | ✓ ✔ available<br>P 1.00 R 0.96–1.00 F1 0.98–1.00 | ✓ ✔ available<br>P 1.00 R 0.96–1.00 F1 0.98–1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F3a | ✓ ✔ available<br>P 0.22–0.66 R 0.93–1.00 F1 0.37–0.79 | ✓ ✔ available<br>P 0.22–0.66 R 0.93–1.00 F1 0.37–0.79 | △ ✔ partial<br>P 0.27–0.72 R 0.93–1.00 F1 0.42–0.83 | △ ✔ partial<br>P 0.27–0.72 R 0.93–1.00 F1 0.42–0.83 |
| F3b | ✓ ✔ available<br>P 0.29–0.82 R 1.00 F1 0.45–0.90 | ✓ ✔ available<br>P 0.29–0.82 R 1.00 F1 0.45–0.90 | △ ✔ partial<br>P 0.28–0.78 R 0.47–0.70 F1 0.37–0.65 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F3c | ✓ ✔ available<br>P 0.43–0.85 R 0.89–1.00 F1 0.59–0.88 | ✓ ✔ available<br>P 0.43–0.85 R 0.89–1.00 F1 0.59–0.88 | △ ✔ partial<br>P 0.00 R 0.00 F1 0.00 | △ ✔ partial<br>P 0.00 R 0.00 F1 0.00 |
| F3d | ✓ ✔ available<br>P 0.33–0.72 R 0.98–1.00 F1 0.49–0.84 | ✓ ✔ available<br>P 0.33–0.72 R 0.98–1.00 F1 0.49–0.84 | △ ✔ available<br>P 0.22–0.50 R 0.98–1.00 F1 0.36–0.67 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F4 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 |
| F5 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 |
| F6a | ✓ ✔ available<br>P 0.82–1.00 R 0.97–1.00 F1 0.90–1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |
| F6b | ✓ ✔ available<br>P 1.00 R 1.00 F1 1.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 | ✗ ✔ suppressed<br>P 0.00 R 0.00 F1 0.00 |

48 of 48 cells agree with the frozen prediction.

## Exposure per profile (range across seeds × rates, Corpus A)

| Profile | Mean records | Median | p95 | Max | Resolvable refs | Owner linkability | Quasi-identifier linkability |
|---|---|---|---|---|---|---|---|
| P0 | 14.17–14.81 | 14.0 | 16–17 | 17–20 | 2022–2176 | 1.000 | 0.010–0.084 |
| P1 | 7.08–7.41 | 7.0 | 8–9 | 10–12 | 1011–1088 | 0.000 | 0.010–0.084 |
| P2 | 7.08–7.41 | 7.0 | 8–9 | 10–12 | 0 | 0.000 | 0.010–0.084 |
| P3 | 1.00 | 1.0 | 1 | 1 | 0 | 0.000 | 0.010–0.084 |

