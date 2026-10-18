// rates re-tallies the held-out corpora per vehicle to report coverage, declared
// suppression, structural and evidence gap rates, conditional precision and
// recall, and effective recall. It refuses to run if the freeze manifest no
// longer matches.
//
// Usage: go run ./cmd/rates
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gimz12/vehicle-registry-portal/prototype/experiment"
	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

const manifest = "eval/freeze/2026-10-03/MANIFEST.sha256"

var profiles = []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3}

type observation struct{ detected, evaluated, structural bool }

// tally is one (corpus, run, rule, profile) cell, counted per vehicle.
type tally struct {
	n, evaluated, gapStructural, gapEvidence int
	pos, posEvaluated, tp, fp, fnEvaluated   int
	suppressed                               bool
}

func ratio(a, b int) float64 {
	if b == 0 {
		return math.NaN()
	}
	return float64(a) / float64(b)
}

func (t tally) coverage() float64         { return ratio(t.evaluated, t.n) }
func (t tally) condPrecision() float64    { return ratio(t.tp, t.tp+t.fp) }
func (t tally) condRecall() float64       { return ratio(t.tp, t.posEvaluated) }
func (t tally) effectiveRecall() float64  { return ratio(t.tp, t.pos) }
func (t tally) conditionalRisk() float64  { return ratio(t.fp+t.fnEvaluated, t.evaluated) }
func (t tally) unevaluatedShare() float64 { return ratio(t.pos-t.posEvaluated, t.pos) }

// observe repeats experiment.Run's per-vehicle observation on one view.
func observe(view *experiment.View) map[string]*observation {
	observed := map[string]*observation{}
	for visibleVID, vehicle := range view.Ledger.Vehicles {
		sourceVID := view.VisibleToSource[visibleVID]
		for _, descriptor := range rules.Registry {
			key := sourceVID + ":" + descriptor.ID
			if observed[key] == nil {
				observed[key] = &observation{}
			}
			for _, finding := range rules.Measure(descriptor, view.Ledger, vehicle) {
				switch {
				case finding.Status == rules.StatusFlagged:
					observed[key].detected, observed[key].evaluated = true, true
				case finding.Status == rules.StatusPass:
					observed[key].evaluated = true
				case finding.Status == rules.StatusGapNotice && finding.Structural:
					observed[key].structural = true
				}
			}
		}
	}
	return observed
}

// Reasons a check answers to the asker, through the chaincode gate (Decide):
// performed, withheld by policy, link removed by the view, or no evidence.
var reasonNames = [4]string{"Performed", "Withheld by policy", "Link removed (structural)", "No evidence"}

// reasons classifies each labelled (vehicle, rule) by what Decide returns on the
// view; a vehicle split into fragments (P3) counts once, by its best answer.
func reasons(view *experiment.View) map[string]int {
	best := map[string]int{}
	for visibleVID, vehicle := range view.Ledger.Vehicles {
		sourceVID := view.VisibleToSource[visibleVID]
		for _, descriptor := range rules.Registry {
			reason := 3
			for _, finding := range rules.Decide(descriptor, view.Ledger, vehicle, view.Profile) {
				switch {
				case finding.Status == rules.StatusFlagged || finding.Status == rules.StatusPass:
					reason = 0
				case finding.Status == rules.StatusSuppressed:
					reason = min(reason, 1)
				case finding.Structural:
					reason = min(reason, 2)
				}
			}
			k := sourceVID + ":" + descriptor.ID
			if r, ok := best[k]; !ok || reason < r {
				best[k] = reason
			}
		}
	}
	return best
}

func measure(corpus *experiment.Corpus, profile rules.Profile) (map[string]*tally, [4]int, error) {
	var why [4]int
	view, err := experiment.Transform(corpus, profile)
	if err != nil {
		return nil, why, err
	}
	observed := observe(view)
	best := reasons(view)
	for _, label := range corpus.Labels {
		why[best[label.SourceVID+":"+label.RuleID]]++
	}
	cells := map[string]*tally{}
	for _, label := range corpus.Labels {
		cell := cells[label.RuleID]
		if cell == nil {
			descriptor, _ := rules.Lookup(label.RuleID)
			cell = &tally{suppressed: !profile.Exposes(descriptor.RequiredCapability)}
			cells[label.RuleID] = cell
		}
		o := observed[label.SourceVID+":"+label.RuleID]
		if o == nil {
			o = &observation{}
		}
		cell.n++
		switch {
		case o.evaluated:
			cell.evaluated++
		case o.structural:
			cell.gapStructural++
		default:
			cell.gapEvidence++
		}
		if label.Positive {
			cell.pos++
			if o.evaluated {
				cell.posEvaluated++
			}
			if o.detected {
				cell.tp++
			} else if o.evaluated {
				cell.fnEvaluated++
			}
		} else if o.detected {
			cell.fp++
		}
	}
	return cells, why, nil
}

type key struct {
	corpus  string
	rule    string
	profile rules.Profile
}

func span(values []float64) string {
	lo, hi, seen := math.Inf(1), math.Inf(-1), false
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		lo, hi, seen = math.Min(lo, v), math.Max(hi, v), true
	}
	switch {
	case !seen:
		return "—"
	case fmt.Sprintf("%.2f", lo) == fmt.Sprintf("%.2f", hi):
		return fmt.Sprintf("%.2f", lo)
	default:
		return fmt.Sprintf("%.2f–%.2f", lo, hi)
	}
}

func pooled(runs []tally) tally {
	var p tally
	for _, r := range runs {
		p.n += r.n
		p.evaluated += r.evaluated
		p.gapStructural += r.gapStructural
		p.gapEvidence += r.gapEvidence
		p.pos += r.pos
		p.posEvaluated += r.posEvaluated
		p.tp += r.tp
		p.fp += r.fp
		p.fnEvaluated += r.fnEvaluated
		p.suppressed = r.suppressed
	}
	return p
}

func each(runs []tally, f func(tally) float64) []float64 {
	out := make([]float64, len(runs))
	for i, r := range runs {
		out[i] = f(r)
	}
	return out
}

func pct(v float64) string {
	if math.IsNaN(v) {
		return "—"
	}
	return fmt.Sprintf("%.2f", v)
}

func main() {
	vehicles := flag.Int("vehicles", 1000, "vehicles per run (the sweep's value)")
	out := flag.String("out", "eval/results/2026-10-03-ext/rates", "output directory")
	flag.Parse()
	if err := experiment.VerifyFrozen(".", manifest); err != nil {
		log.Fatal(err)
	}
	digest, _ := experiment.ManifestDigest(manifest)
	fmt.Printf("frozen inputs verified (sha256 %s)\n", digest)

	seeds := []int64{1001, 1002, 1003}
	rates := []float64{0.02, 0.05, 0.08}
	runs := map[key][]tally{}
	why := map[string]*[4]int{}
	for _, corpusName := range []string{"A", "B"} {
		for _, seed := range seeds {
			for _, rate := range rates {
				config := experiment.Config{Vehicles: *vehicles, Seed: seed, FraudRate: rate}
				if corpusName == "B" {
					n := experiment.DefaultNoise
					config.Noise = &n
				}
				corpus, err := experiment.Generate(config)
				if err != nil {
					log.Fatal(err)
				}
				for _, profile := range profiles {
					cells, counts, err := measure(corpus, profile)
					if err != nil {
						log.Fatal(err)
					}
					wk := corpusName + string(profile)
					if why[wk] == nil {
						why[wk] = &[4]int{}
					}
					for i, c := range counts {
						why[wk][i] += c
					}
					for rule, cell := range cells {
						k := key{corpusName, rule, profile}
						runs[k] = append(runs[k], *cell)
					}
				}
			}
		}
	}
	ruleIDs := make([]string, 0, len(rules.Registry))
	for _, d := range rules.Registry {
		ruleIDs = append(ruleIDs, d.ID)
	}
	sort.Strings(ruleIDs)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	csvFile, err := os.Create(filepath.Join(*out, "rates.csv"))
	if err != nil {
		log.Fatal(err)
	}
	w := csv.NewWriter(csvFile)
	_ = w.Write([]string{"corpus", "rule", "profile", "vehicles", "suppressed", "evaluated", "coverage", "gap_structural", "gap_evidence",
		"positives", "positives_evaluated", "tp", "fp", "fn_evaluated", "cond_precision", "cond_recall", "effective_recall", "conditional_risk"})

	var md strings.Builder
	fmt.Fprintf(&md, "# Table 3.3 measures on the held-out sweep\n\n")
	fmt.Fprintf(&md, "Frozen manifest `%s`. Seeds %v × fraud rates %v × %d vehicles per run, Corpus A (clean) and Corpus B (noisy); the same corpora as `eval/results/2026-10-03`. ", digest, seeds, rates, *vehicles)
	md.WriteString("Rates are pooled over the nine runs per corpus; precision and recall are also given as the range across runs.\n\n")
	md.WriteString("- **Coverage**: share of vehicles the rule could evaluate in this view (PASS or FLAGGED).\n")
	md.WriteString("- **Gap, structural**: not evaluated because the view itself removed what the rule needs (redacted reference, split history, removed owner link).\n")
	md.WriteString("- **Gap, evidence**: not evaluated because the records were missing or too weak (E0), with the view intact.\n")
	md.WriteString("- **Declared**: what the chaincode gate (`Decide`) answers for this profile: *suppressed* when the profile withholds the rule's required capability. The measured columns come from the view (`Measure`), as in the frozen sweep, and are kept apart.\n")
	md.WriteString("- **Conditional precision / recall**: among evaluated vehicles only. **Effective recall**: detected positives over all positives, including those the view could not evaluate; this is the figure the sweep's `grid.md` reports as recall.\n")
	md.WriteString("- **Unevaluated positives**: share of positives the view could not evaluate; the difference between conditional and effective recall comes from these.\n\n")

	for _, corpusName := range []string{"A", "B"} {
		fmt.Fprintf(&md, "## Corpus %s\n\n", corpusName)
		md.WriteString("| Rule | Profile | Declared | Coverage | Gap, structural | Gap, evidence | Positives (evaluated) | Unevaluated positives | Cond. precision | Cond. recall | Effective recall |\n")
		md.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
		for _, rule := range ruleIDs {
			for _, profile := range profiles {
				rs := runs[key{corpusName, rule, profile}]
				p := pooled(rs)
				declared := "evaluated"
				if p.suppressed {
					declared = "suppressed"
				}
				fmt.Fprintf(&md, "| %s | %s | %s | %s | %s | %s | %d (%d) | %s | %s | %s | %s |\n",
					rule, profile, declared, pct(p.coverage()), pct(ratio(p.gapStructural, p.n)), pct(ratio(p.gapEvidence, p.n)),
					p.pos, p.posEvaluated, pct(p.unevaluatedShare()),
					span(each(rs, tally.condPrecision)), span(each(rs, tally.condRecall)), span(each(rs, tally.effectiveRecall)))
				_ = w.Write([]string{corpusName, rule, string(profile), strconv.Itoa(p.n), strconv.FormatBool(p.suppressed), strconv.Itoa(p.evaluated),
					pct(p.coverage()), pct(ratio(p.gapStructural, p.n)), pct(ratio(p.gapEvidence, p.n)),
					strconv.Itoa(p.pos), strconv.Itoa(p.posEvaluated), strconv.Itoa(p.tp), strconv.Itoa(p.fp), strconv.Itoa(p.fnEvaluated),
					pct(p.condPrecision()), pct(p.condRecall()), pct(p.effectiveRecall()), pct(p.conditionalRisk())})
			}
		}
		md.WriteString("\n")
	}

	// Positives per rule per run behind each precision and recall figure.
	md.WriteString("## Positives per rule per run\n\nMinimum–maximum positive vehicles per run (nine runs per corpus); every precision and recall range above rests on these counts.\n\n| Rule | Corpus A | Corpus B |\n|---|---|---|\n")
	for _, rule := range ruleIDs {
		row := []string{rule}
		for _, corpusName := range []string{"A", "B"} {
			lo, hi := math.MaxInt, 0
			for _, r := range runs[key{corpusName, rule, rules.ProfileP0}] {
				lo, hi = min(lo, r.pos), max(hi, r.pos)
			}
			row = append(row, fmt.Sprintf("%d–%d", lo, hi))
		}
		fmt.Fprintf(&md, "| %s |\n", strings.Join(row, " | "))
	}

	md.WriteString("\n## Why a check was not performed, as the asker sees it\n\n")
	md.WriteString("Every labelled (vehicle, rule) answer through the chaincode gate (`Decide`, the path the portal shows), pooled over nine runs. ")
	md.WriteString("The buyer page labels these *Withheld*, *Link removed* and *No evidence*. A vehicle split by P3 counts once, by its best answer.\n\n")
	md.WriteString("| Corpus | Profile | " + strings.Join(reasonNames[:], " | ") + " |\n|---|---|---|---|---|---|\n")
	for _, corpusName := range []string{"A", "B"} {
		for _, profile := range profiles {
			c := why[corpusName+string(profile)]
			total := c[0] + c[1] + c[2] + c[3]
			fmt.Fprintf(&md, "| %s | %s | %s | %s | %s | %s |\n", corpusName, profile,
				pct(ratio(c[0], total)), pct(ratio(c[1], total)), pct(ratio(c[2], total)), pct(ratio(c[3], total)))
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
	if err := csvFile.Close(); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "rates.md"), []byte(md.String()), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("written to %s\n", *out)
}
