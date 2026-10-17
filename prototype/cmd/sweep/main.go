// sweep runs the pre-registered linkage-ablation experiment (corpora A and B,
// held-out seeds, fraud rates, P0 to P3, and Corpus C) and writes per-cell
// JSON, the grid against eval/predictions.md and the summary tables. With
// -frozen it refuses to run if any frozen input differs from the manifest.
package main

import (
	"bufio"
	"encoding/json"
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

var profiles = []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3}

type runKey struct {
	Corpus  string        `json:"corpus"`
	Seed    int64         `json:"seed"`
	Rate    float64       `json:"rate"`
	Profile rules.Profile `json:"profile"`
}

type runResult struct {
	runKey
	Report *experiment.Report `json:"report"`
}

type scenarioRow struct {
	ID      string                   `json:"id"`
	Title   string                   `json:"title"`
	Source  string                   `json:"source"`
	Rule    string                   `json:"rule"`
	Outcome map[rules.Profile]string `json:"outcome"` // DETECTED / MISSED / GAP / SUPPRESSED / FALSE_POSITIVE / PASS
}

func main() {
	seedsFlag := flag.String("seeds", "1001,1002,1003", "comma-separated held-out seeds")
	ratesFlag := flag.String("rates", "0.02,0.05,0.08", "comma-separated fraud rates")
	vehicles := flag.Int("vehicles", 1000, "vehicles per run")
	noise := flag.Bool("noise", true, "also run Corpus B with experiment.DefaultNoise")
	scenarios := flag.Bool("scenarios", true, "also run the Corpus C scenarios")
	frozen := flag.String("frozen", "", "path to MANIFEST.sha256; refuse to run if frozen inputs changed")
	predictions := flag.String("predictions", "eval/predictions.md", "prediction table")
	out := flag.String("out", "./out", "output directory")
	flag.Parse()

	if *frozen != "" {
		if err := experiment.VerifyFrozen(".", *frozen); err != nil {
			log.Fatal(err)
		}
		digest, err := experiment.ManifestDigest(*frozen)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("frozen inputs verified against %s (sha256 %s)\n", *frozen, digest)
	}
	seeds := parseInts(*seedsFlag)
	rates := parseFloats(*ratesFlag)
	predicted, err := readPredictions(*predictions)
	if err != nil {
		log.Fatal(err)
	}

	var results []runResult
	corpora := []string{"A"}
	if *noise {
		corpora = append(corpora, "B")
	}
	for _, corpusName := range corpora {
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
					report, err := experiment.Run(corpus, profile)
					if err != nil {
						log.Fatal(err)
					}
					r := runResult{runKey{corpusName, seed, rate, profile}, report}
					results = append(results, r)
					dir := filepath.Join(*out, corpusName, strconv.FormatInt(seed, 10), fmt.Sprintf("%.2f", rate))
					if err := writeJSON(filepath.Join(dir, string(profile)+".json"), r); err != nil {
						log.Fatal(err)
					}
				}
				fmt.Printf("corpus %s seed %d rate %.2f done\n", corpusName, seed, rate)
			}
		}
	}

	var grid strings.Builder
	grid.WriteString("# Linkage-ablation results\n\n")
	fmt.Fprintf(&grid, "Seeds %v · fraud rates %v · %d vehicles per run · corpora %v\n\n", seeds, rates, *vehicles, corpora)
	for _, corpusName := range corpora {
		writeGrid(&grid, corpusName, results, predicted)
	}
	writeExposure(&grid, results)
	if err := os.WriteFile(filepath.Join(*out, "grid.md"), []byte(grid.String()), 0o644); err != nil {
		log.Fatal(err)
	}
	if err := writeFrontier(filepath.Join(*out, "frontier.csv"), results); err != nil {
		log.Fatal(err)
	}
	if *scenarios {
		rows := runScenarios()
		if err := writeJSON(filepath.Join(*out, "scenarios.json"), rows); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*out, "scenarios.md"), []byte(scenarioTable(rows)), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Print(grid.String())
	fmt.Printf("\nwritten to %s\n", *out)
}

// ---- prediction table ----

func readPredictions(path string) (map[string][4]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := map[string][4]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "| F") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 8 {
			continue
		}
		rule := strings.Fields(strings.TrimSpace(fields[1]))[0]
		var glyphs [4]string
		for i := 0; i < 4; i++ {
			glyphs[i] = strings.TrimSpace(fields[3+i])
		}
		out[rule] = glyphs
	}
	if len(out) != len(rules.Registry) {
		return nil, fmt.Errorf("sweep: parsed %d prediction rows, registry has %d rules", len(out), len(rules.Registry))
	}
	return out, scanner.Err()
}

func glyphClass(g string) string {
	switch g {
	case "✓":
		return experiment.ClassAvailable
	case "△":
		return experiment.ClassPartial
	case "✗":
		return experiment.ClassSuppressed
	}
	return "?"
}

// ---- aggregation ----

type agg struct {
	classes                            map[string]bool
	pMin, pMax, rMin, rMax, fMin, fMax float64
	n                                  int
}

func aggregate(results []runResult, corpusName string) map[string]map[rules.Profile]*agg {
	out := map[string]map[rules.Profile]*agg{}
	for _, r := range results {
		if r.Corpus != corpusName {
			continue
		}
		for _, cell := range r.Report.Cells {
			if out[cell.RuleID] == nil {
				out[cell.RuleID] = map[rules.Profile]*agg{}
			}
			a := out[cell.RuleID][r.Profile]
			if a == nil {
				a = &agg{classes: map[string]bool{}, pMin: 1, rMin: 1, fMin: 1}
				out[cell.RuleID][r.Profile] = a
			}
			a.classes[cell.Class()] = true
			a.pMin, a.pMax = math.Min(a.pMin, cell.Precision()), math.Max(a.pMax, cell.Precision())
			a.rMin, a.rMax = math.Min(a.rMin, cell.Recall()), math.Max(a.rMax, cell.Recall())
			a.fMin, a.fMax = math.Min(a.fMin, cell.F1()), math.Max(a.fMax, cell.F1())
			a.n++
		}
	}
	return out
}

func (a *agg) class() string {
	if len(a.classes) != 1 {
		return "unstable"
	}
	for c := range a.classes {
		return c
	}
	return "?"
}

func rng(lo, hi float64) string {
	if math.Abs(hi-lo) < 0.0005 {
		return fmt.Sprintf("%.2f", lo)
	}
	return fmt.Sprintf("%.2f–%.2f", lo, hi)
}

func writeGrid(w *strings.Builder, corpusName string, results []runResult, predicted map[string][4]string) {
	cells := aggregate(results, corpusName)
	fmt.Fprintf(w, "## Corpus %s — measured class vs prediction (P / R / F1 as range across seeds × rates)\n\n", corpusName)
	w.WriteString("| Rule | P0 | P1 | P2 | P3 |\n|---|---|---|---|---|\n")
	var disagreements []string
	ruleIDs := make([]string, 0, len(cells))
	for id := range cells {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	for _, id := range ruleIDs {
		fmt.Fprintf(w, "| %s |", id)
		var prevF1 float64 = -1
		for i, profile := range profiles {
			a := cells[id][profile]
			measured := a.class()
			glyph := predicted[id][i]
			agree := glyphClass(glyph) == measured
			if glyph == "△" && !agree && measured == experiment.ClassAvailable && prevF1 >= 0 && a.fMin < prevF1 {
				agree = true // partial realised as an accuracy drop inside a decidable cell
			}
			mark := "✔"
			if !agree {
				mark = "✘"
				disagreements = append(disagreements, fmt.Sprintf("%s@%s predicted %s (%s), measured %s (P %s, R %s)", id, profile, glyph, glyphClass(glyph), measured, rng(a.pMin, a.pMax), rng(a.rMin, a.rMax)))
			}
			fmt.Fprintf(w, " %s %s %s<br>P %s R %s F1 %s |", glyph, mark, measured, rng(a.pMin, a.pMax), rng(a.rMin, a.rMax), rng(a.fMin, a.fMax))
			prevF1 = a.fMin
		}
		w.WriteString("\n")
	}
	fmt.Fprintf(w, "\n%d of %d cells agree with the frozen prediction.\n", len(ruleIDs)*4-len(disagreements), len(ruleIDs)*4)
	if len(disagreements) > 0 {
		w.WriteString("\nDisagreements (reported, not corrected):\n\n")
		for _, d := range disagreements {
			fmt.Fprintf(w, "- %s\n", d)
		}
	}
	w.WriteString("\n")
}

func writeExposure(w *strings.Builder, results []runResult) {
	w.WriteString("## Exposure per profile (range across seeds × rates, Corpus A)\n\n")
	w.WriteString("| Profile | Mean records | Median | p95 | Max | Resolvable refs | Owner linkability | Quasi-identifier linkability |\n|---|---|---|---|---|---|---|---|\n")
	for _, profile := range profiles {
		var mean, med, p95, max, res, link, quasi []float64
		for _, r := range results {
			if r.Corpus != "A" || r.Profile != profile {
				continue
			}
			e := r.Report.Exposure
			mean = append(mean, e.Mean)
			med = append(med, e.Median)
			p95 = append(p95, float64(e.P95))
			max = append(max, float64(e.Max))
			res = append(res, float64(e.ResolvableRefs))
			link = append(link, e.Linkability)
			quasi = append(quasi, e.QuasiLinkability)
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", profile,
			span(mean, 2), span(med, 1), span(p95, 0), span(max, 0), span(res, 0), span(link, 3), span(quasi, 3))
	}
	w.WriteString("\n")
}

func span(values []float64, decimals int) string {
	if len(values) == 0 {
		return "–"
	}
	lo, hi := values[0], values[0]
	for _, v := range values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	format := fmt.Sprintf("%%.%df", decimals)
	if hi-lo < math.Pow(10, -float64(decimals))/2 {
		return fmt.Sprintf(format, lo)
	}
	return fmt.Sprintf(format+"–"+format, lo, hi)
}

func writeFrontier(path string, results []runResult) error {
	var b strings.Builder
	b.WriteString("corpus,seed,rate,profile,meanExposure,resolvableRefs,ownerLinkability,quasiLinkability,macroF1,decidableCells\n")
	for _, r := range results {
		var f1 float64
		decidable := 0
		for _, cell := range r.Report.Cells {
			if cell.Class() == experiment.ClassSuppressed {
				continue
			}
			f1 += cell.F1()
			decidable++
		}
		if decidable > 0 {
			f1 /= float64(decidable)
		}
		e := r.Report.Exposure
		fmt.Fprintf(&b, "%s,%d,%.2f,%s,%.3f,%d,%.4f,%.4f,%.4f,%d\n", r.Corpus, r.Seed, r.Rate, r.Profile, e.Mean, e.ResolvableRefs, e.Linkability, e.QuasiLinkability, f1, decidable)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ---- scenarios ----

func runScenarios() []scenarioRow {
	var rows []scenarioRow
	for _, s := range experiment.Scenarios() {
		corpus := s.Corpus()
		reports := map[rules.Profile]*experiment.Report{}
		for _, profile := range profiles {
			report, err := experiment.Run(corpus, profile)
			if err != nil {
				log.Fatal(err)
			}
			reports[profile] = report
		}
		ruleIDs := make([]string, 0, len(rules.Registry))
		for _, d := range rules.Registry {
			ruleIDs = append(ruleIDs, d.ID)
		}
		for _, ruleID := range ruleIDs {
			_, expected := s.Expect[ruleID]
			row := scenarioRow{ID: s.ID, Title: s.Title, Source: s.Source, Rule: ruleID, Outcome: map[rules.Profile]string{}}
			interesting := expected
			for _, profile := range profiles {
				var cell experiment.Cell
				for _, c := range reports[profile].Cells {
					if c.RuleID == ruleID {
						cell = c
					}
				}
				outcome := "PASS"
				switch {
				case cell.Suppressed > 0 && cell.Evaluated == 0:
					outcome = "SUPPRESSED"
				case cell.TP > 0:
					outcome = "DETECTED"
				case cell.FN > 0 && cell.Evaluated == 0:
					outcome = "GAP"
				case cell.FN > 0:
					outcome = "MISSED"
				case cell.FP > 0:
					outcome = "FALSE_POSITIVE"
					interesting = true
				case cell.Evaluated == 0:
					outcome = "GAP"
				}
				row.Outcome[profile] = outcome
			}
			if interesting {
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func scenarioTable(rows []scenarioRow) string {
	var b strings.Builder
	b.WriteString("# Corpus C — named scenarios, outcome per profile\n\n| Scenario | Source | Rule | P0 | P1 | P2 | P3 |\n|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s — %s | %s | %s | %s | %s | %s | %s |\n", r.ID, r.Title, r.Source, r.Rule,
			r.Outcome[rules.ProfileP0], r.Outcome[rules.ProfileP1], r.Outcome[rules.ProfileP2], r.Outcome[rules.ProfileP3])
	}
	return b.String()
}

// ---- helpers ----

func parseInts(s string) []int64 {
	var out []int64
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			log.Fatalf("bad seed %q", part)
		}
		out = append(out, v)
	}
	return out
}

func parseFloats(s string) []float64 {
	var out []float64
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			log.Fatalf("bad rate %q", part)
		}
		out = append(out, v)
	}
	return out
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
