"""Coverage against conditional risk, one point per rule and profile (Table 3.3 figure).

Reads rates.csv written by `go run ./cmd/rates`. Hollow markers are cells the
chaincode gate declares suppressed for that profile; their position is what the
view alone would allow. Usage: python3 cmd/rates/plot.py
"""
import csv
import sys

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

src = sys.argv[1] if len(sys.argv) > 1 else "eval/results/2026-10-03-ext/rates/rates.csv"
dst = sys.argv[2] if len(sys.argv) > 2 else src.replace("rates.csv", "coverage-vs-conditional-risk.png")
rows = [r for r in csv.DictReader(open(src)) if r["conditional_risk"] != "—"]
colours = {"P0": "#1b4965", "P1": "#5fa8d3", "P2": "#e07a5f", "P3": "#81b29a"}

fig, axes = plt.subplots(1, 2, figsize=(10, 4.2), sharey=True)
for ax, corpus in zip(axes, ["A", "B"]):
    labels = {}
    for r in (r for r in rows if r["corpus"] == corpus):
        x, y = float(r["coverage"]), float(r["conditional_risk"])
        declared = r["suppressed"] == "true"
        ax.scatter(x, y, s=46, facecolors="none" if declared else colours[r["profile"]],
                   edgecolors=colours[r["profile"]], linewidths=1.4, zorder=3)
        if y > 0.02 or x < 0.95:
            labels.setdefault((round(x, 2), round(y, 2)), []).append(f'{r["rule"]} {r["profile"]}')
    right = 0
    for (x, y), names in sorted(labels.items(), key=lambda item: item[0][1]):
        if x > 0.5:
            right += 1
        ax.annotate(", ".join(names), (x, y), fontsize=7, xytext=(-6, 11 * right - 8) if x > 0.5 else (5, 4),
                    textcoords="offset points", ha="right" if x > 0.5 else "left",
                    arrowprops={"arrowstyle": "-", "lw": 0.5, "color": "grey"} if x > 0.5 else None)
    ax.set_title(f"Corpus {corpus} ({'clean' if corpus == 'A' else 'noisy'})", fontsize=10)
    ax.set_xlabel("Coverage (share of vehicles evaluated)")
    ax.set_xlim(-0.03, 1.08)
    ax.grid(alpha=0.3)
axes[0].set_ylabel("Conditional risk (errors among evaluated)")
handles = [plt.Line2D([], [], marker="o", ls="", color=c, label=p) for p, c in colours.items()]
handles.append(plt.Line2D([], [], marker="o", ls="", mfc="none", mec="grey", label="declared suppressed"))
fig.legend(handles=handles, loc="lower center", ncol=5, frameon=False, fontsize=8)
fig.tight_layout(rect=(0, 0.07, 1, 1))
fig.savefig(dst, dpi=300)
print("written", dst)
