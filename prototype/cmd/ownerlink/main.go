// ownerlink measures the owner-linking path on the held-out corpora: for each
// vehicle and F6a/F6b it asks the DMT predicate, opens a case on each ALERT and
// compares both verdicts with the harness's P0 measurement and the labels.
// Corpus owner ids stand in for the owner index; each owner gets its own
// Service, since at P1 no two vehicles share a pseudonym. It refuses to run if
// the freeze manifest no longer matches.
//
// Usage: go run ./cmd/ownerlink
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gimz12/vehicle-registry-portal/prototype/chaincode/registry"
	"github.com/gimz12/vehicle-registry-portal/prototype/experiment"
	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

const manifest = "eval/freeze/2026-10-03/MANIFEST.sha256"

var ownerRules = []string{"F6a", "F6b"}

type counts struct {
	vehicles, positives                       int
	eligible, eligiblePositives               int
	predicateFlagged, predicateAlert          int
	predicateAgree, predicateTP, predicateFP  int
	alertPositives                            int
	cases, caseAgree                          int
	fleetSizeSum, fleetMax, fleetRecordsSum   int
	predicateHarnessFlagged, harnessPositives int
}

// facts records the corpus properties that bound the numbers above.
type facts struct {
	fleetSizes          map[int]int
	vehicles, assessors int
	mileage             map[rules.Tier]int
}

func (f *facts) add(corpus *experiment.Corpus) {
	owners := map[string]int{}
	for _, c := range corpus.Vehicles {
		owners[c.OwnerID]++
		f.vehicles++
		named := false
		for _, e := range c.Vehicle.Events {
			if e.Type == rules.EventClaim && e.AttestorID != "" {
				named = true
			}
			if e.Type == rules.EventMileage {
				f.mileage[e.Tier]++
			}
		}
		if named {
			f.assessors++
		}
	}
	for _, n := range owners {
		f.fleetSizes[n]++
	}
}

func flagged(findings []rules.Finding) bool {
	for _, f := range findings {
		if f.Flagged() {
			return true
		}
	}
	return false
}

func ratio(a, b int) string {
	if b == 0 {
		return "—"
	}
	return fmt.Sprintf("%.2f", float64(a)/float64(b))
}

func run(corpus *experiment.Corpus, tag string, totals map[string]*counts) {
	p1, err := experiment.Transform(corpus, rules.ProfileP1)
	if err != nil {
		log.Fatal(err)
	}
	p0, err := experiment.Transform(corpus, rules.ProfileP0)
	if err != nil {
		log.Fatal(err)
	}
	fleetOf := map[string][]string{}
	ownerOf := map[string]string{}
	for _, c := range corpus.Vehicles {
		fleetOf[c.OwnerID] = append(fleetOf[c.OwnerID], c.Vehicle.VID)
		ownerOf[c.Vehicle.VID] = c.OwnerID
	}
	positive := map[string]bool{}
	for _, l := range corpus.Labels {
		if l.Positive {
			positive[l.SourceVID+":"+l.RuleID] = true
		}
	}
	vids := make([]string, 0, len(corpus.Vehicles))
	for _, c := range corpus.Vehicles {
		vids = append(vids, c.Vehicle.VID)
	}
	sort.Strings(vids)
	services := map[string]*registry.Service{}
	for owner, fleet := range fleetOf {
		repo := registry.NewMemoryRepository()
		for _, vid := range fleet {
			if err := repo.PutVehicle(&registry.VehicleRecord{Vehicle: *p1.Ledger.Vehicles[vid]}); err != nil {
				log.Fatal(err)
			}
		}
		services[owner] = registry.NewService(repo)
	}

	for _, rule := range ownerRules {
		descriptor, _ := rules.Lookup(rule)
		t := totals[rule]
		for _, vid := range vids {
			fleet := fleetOf[ownerOf[vid]]
			service := services[ownerOf[vid]]
			isPositive := positive[vid+":"+rule]
			harness := flagged(rules.Measure(descriptor, p0.Ledger, p0.Ledger.Vehicles[vid]))
			t.vehicles++
			if isPositive {
				t.positives++
			}
			if harness {
				t.harnessPositives++
			}
			result, err := service.EvaluateOwnerPredicate(registry.MSPDMT, "dmt-officer", vid, rule, fleet, tag)
			if err != nil {
				if !strings.Contains(err.Error(), "first") {
					log.Fatalf("%s %s: %v", rule, vid, err)
				}
				continue // refused by the eligibility gate
			}
			t.eligible++
			t.fleetSizeSum += len(fleet)
			t.fleetMax = max(t.fleetMax, len(fleet))
			for _, other := range fleet {
				if other != vid {
					t.fleetRecordsSum += len(p1.Ledger.Vehicles[other].Events)
				}
			}
			if isPositive {
				t.eligiblePositives++
			}
			got := result.Status == rules.StatusFlagged
			if got {
				t.predicateFlagged++
				if isPositive {
					t.predicateTP++
				} else {
					t.predicateFP++
				}
			}
			if got == harness {
				t.predicateAgree++
			}
			if harness {
				t.predicateHarnessFlagged++
			}
			if result.Strength != rules.StrengthAlert {
				continue
			}
			t.predicateAlert++
			if isPositive {
				t.alertPositives++
			}
			// A predicate ALERT grounds a dual-approved case; read the case-time P0.
			caseID := fmt.Sprintf("%s-%s-%s", tag, rule, vid[:12])
			if _, err := service.CreateRevealRequestBy(registry.MSPPolice, "police-officer-1", caseID, vid, rule, "FRAUD_INVESTIGATION", "OL/"+vid[:6]); err != nil {
				log.Fatalf("case %s: %v", caseID, err)
			}
			if _, err := service.ApproveRevealBy(registry.MSPDMT, "dmt-officer-2", caseID, true, "", fleet); err != nil {
				log.Fatalf("approve %s: %v", caseID, err)
			}
			if _, err := service.ApproveRevealBy(registry.MSPPolice, "police-officer-2", caseID, true, "", nil); err != nil {
				log.Fatalf("approve %s: %v", caseID, err)
			}
			d, err := service.DisclosureForFleet(registry.MSPPolice, "", registry.PurposePolice, vid, caseID, fleet)
			if err != nil {
				log.Fatalf("read %s: %v", caseID, err)
			}
			t.cases++
			caseFlagged := false
			for _, f := range d.Failed {
				if f.RuleID == rule {
					caseFlagged = true
				}
			}
			if caseFlagged == harness {
				t.caseAgree++
			}
		}
	}
}

func main() {
	vehicles := flag.Int("vehicles", 1000, "vehicles per run (the sweep's value)")
	out := flag.String("out", "eval/results/2026-10-03-ext/ownerlink", "output directory")
	flag.Parse()
	if err := experiment.VerifyFrozen(".", manifest); err != nil {
		log.Fatal(err)
	}
	digest, _ := experiment.ManifestDigest(manifest)
	fmt.Printf("frozen inputs verified (sha256 %s)\n", digest)

	var md strings.Builder
	fmt.Fprintf(&md, "# Owner linking through the DMT owner index (chaincode v1.3)\n\n")
	fmt.Fprintf(&md, "Frozen manifest `%s`. Seeds 1001–1003 × fraud rates 0.02/0.05/0.08 × %d vehicles, pooled per corpus. ", digest, *vehicles)
	md.WriteString("Vehicles are loaded at P1 pseudonym scope into the chaincode Service; the corpus owner ids stand in for the DMT owner index. ")
	md.WriteString("The *harness* column is the frozen sweep's P0 measurement of the same rule on the same vehicle.\n\n")
	md.WriteString("Eligibility gate: F6a needs an F1a rollback finding at P1 on this vehicle; F6b needs a claim on this vehicle with a named assessor.\n\n")

	for _, corpusName := range []string{"A", "B"} {
		totals := map[string]*counts{"F6a": {}, "F6b": {}}
		corpusFacts := &facts{fleetSizes: map[int]int{}, mileage: map[rules.Tier]int{}}
		for _, seed := range []int64{1001, 1002, 1003} {
			for _, rate := range []float64{0.02, 0.05, 0.08} {
				config := experiment.Config{Vehicles: *vehicles, Seed: seed, FraudRate: rate}
				if corpusName == "B" {
					n := experiment.DefaultNoise
					config.Noise = &n
				}
				corpus, err := experiment.Generate(config)
				if err != nil {
					log.Fatal(err)
				}
				run(corpus, fmt.Sprintf("%s-%d-%.2f", corpusName, seed, rate), totals)
				corpusFacts.add(corpus)
				fmt.Printf("corpus %s seed %d rate %.2f done\n", corpusName, seed, rate)
			}
		}
		fmt.Fprintf(&md, "## Corpus %s\n\n", corpusName)
		md.WriteString("| Measure | F6a | F6b |\n|---|---|---|\n")
		row := func(name string, f func(c *counts) string) {
			fmt.Fprintf(&md, "| %s | %s | %s |\n", name, f(totals["F6a"]), f(totals["F6b"]))
		}
		row("Vehicles", func(c *counts) string { return fmt.Sprint(c.vehicles) })
		row("Labelled positives", func(c *counts) string { return fmt.Sprint(c.positives) })
		row("Predicate calls the gate admits, per 1,000 vehicles", func(c *counts) string {
			return fmt.Sprintf("%.1f", 1000*float64(c.eligible)/float64(c.vehicles))
		})
		row("Positives the gate can reach (eligible / all positives)", func(c *counts) string { return ratio(c.eligiblePositives, c.positives) })
		row("Predicate agrees with the harness P0 verdict (eligible calls)", func(c *counts) string { return ratio(c.predicateAgree, c.eligible) })
		row("Predicate precision against labels", func(c *counts) string { return ratio(c.predicateTP, c.predicateTP+c.predicateFP) })
		row("Predicate recall against all positives", func(c *counts) string { return ratio(c.predicateTP, c.positives) })
		row("Predicate answers at ALERT (can ground a case), per 1,000 vehicles", func(c *counts) string {
			return fmt.Sprintf("%.1f", 1000*float64(c.predicateAlert)/float64(c.vehicles))
		})
		row("Positives that reach ALERT (case-grounding recall)", func(c *counts) string { return ratio(c.alertPositives, c.positives) })
		row("Cases opened and read at case-time P0", func(c *counts) string { return fmt.Sprint(c.cases) })
		row("Case-time verdict agrees with the harness P0", func(c *counts) string { return ratio(c.caseAgree, c.cases) })
		row("Mean fleet size per admitted call (max)", func(c *counts) string {
			if c.eligible == 0 {
				return "—"
			}
			return fmt.Sprintf("%.2f (%d)", float64(c.fleetSizeSum)/float64(c.eligible), c.fleetMax)
		})
		row("Other vehicles' records a case-time P0 read joins, mean per call", func(c *counts) string {
			if c.eligible == 0 {
				return "—"
			}
			return fmt.Sprintf("%.1f", float64(c.fleetRecordsSum)/float64(c.eligible))
		})
		fmt.Fprintf(&md, "\nCorpus properties that bound these numbers: owners by fleet size %v; vehicles with a claim naming an assessor %d of %d; odometer readings by evidence grade %v.\n\n",
			corpusFacts.fleetSizes, corpusFacts.assessors, corpusFacts.vehicles, corpusFacts.mileage)
	}
	md.WriteString("**What these numbers can and cannot show.** The generator gives every owner exactly two vehicles and every vehicle a claim naming an assessor, so the F6b gate admits every vehicle here and the fleet size never varies; on a real register the gate admits only vehicles with an assessor-named claim and fleets vary. ")
	md.WriteString("The generator grades odometer readings E2 (Corpus B noise downgrades some), so F6a reaches ALERT here; the live network grades desk-entered readings E1, so live F6a stays a WARNING and cannot ground a case (disclosed limitation S4). ")
	md.WriteString("Agreement with the harness is by construction when the owner index is complete; it shows the chaincode path regroups the fleet exactly as the frozen P0 view does, not that the index is correct.\n\n")
	md.WriteString("**Exposure per answer.** A predicate call returns one verdict (status and strength) for one vehicle and is stored on the ledger with the fleet hash and the hashed caller id; no record of the owner's other vehicles leaves the chaincode. ")
	md.WriteString("A case-time P0 read joins the other vehicles' records listed above, and only after two agencies approve.\n")

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "ownerlink.md"), []byte(md.String()), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Print(md.String())
}
