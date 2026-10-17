package registry

import (
	"fmt"
	"sort"
	"testing"

	"github.com/gimz12/vehicle-registry-portal/prototype/baseline"
	"github.com/gimz12/vehicle-registry-portal/prototype/experiment"
	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

// Implementation parity: chaincode, harness and centralised baseline must agree
// on the 500-vehicle corpus. Vehicles bypass the Service write path, which would
// reject the F2/F4/F5 positives.

const parityVehicles = 500

func parityCorpus(tb testing.TB) *experiment.Corpus {
	tb.Helper()
	corpus, err := experiment.Generate(experiment.Config{Vehicles: parityVehicles, Seed: 777, FraudRate: 0.08})
	if err != nil {
		tb.Fatalf("Generate: %v", err)
	}
	return corpus
}

type loaded struct {
	view    *experiment.View
	service *Service
	log     *baseline.Log
}

func loadProfile(tb testing.TB, corpus *experiment.Corpus, profile rules.Profile) loaded {
	tb.Helper()
	view, err := experiment.Transform(corpus, profile)
	if err != nil {
		tb.Fatalf("Transform %s: %v", profile, err)
	}
	repo := NewMemoryRepository()
	var events []rules.Event
	for _, v := range view.Ledger.Vehicles {
		if err := repo.PutVehicle(&VehicleRecord{Vehicle: *v}); err != nil {
			tb.Fatalf("PutVehicle: %v", err)
		}
		events = append(events, v.Events...)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CommitOrder < events[j].CommitOrder })
	log := baseline.New("DMT", baseline.Agencies, 100)
	for _, e := range events {
		if _, err := log.Append(baseline.AgencyFor(e.Type), e); err != nil {
			tb.Fatalf("baseline Append: %v", err)
		}
	}
	return loaded{view: view, service: NewService(repo), log: log}
}

func key(f rules.Finding) string {
	return fmt.Sprintf("%s|%d|%s|%s|%s|%s", f.RuleID, f.AtSeq, f.Status, f.Strength, f.Tier, f.Detail)
}

func keys(findings []rules.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, key(f))
	}
	sort.Strings(out)
	return out
}

func equalKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParityChaincodeHarnessBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("parity over 500 vehicles × 4 profiles takes about a minute")
	}
	corpus := parityCorpus(t)
	for _, profile := range []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3} {
		l := loadProfile(t, corpus, profile)
		if err := l.log.Verify(); err != nil {
			t.Fatalf("%s: baseline log failed verification: %v", profile, err)
		}
		baseLedger := l.log.Ledger(profile)

		report, err := experiment.Run(corpus, profile)
		if err != nil {
			t.Fatal(err)
		}
		declared := 0
		for _, cell := range report.Cells {
			declared += cell.Suppressed
		}

		vids := make([]string, 0, len(l.view.Ledger.Vehicles))
		for vid := range l.view.Ledger.Vehicles {
			vids = append(vids, vid)
		}
		sort.Strings(vids)
		suppressed, mismatches := 0, 0
		for _, vid := range vids {
			disclosure, err := l.service.Disclosure(vid, profile)
			if err != nil {
				t.Fatalf("%s Disclosure %s: %v", profile, vid[:8], err)
			}
			var chain []rules.Finding
			chain = append(chain, disclosure.Failed...)
			chain = append(chain, disclosure.Passed...)
			chain = append(chain, disclosure.NotPerformed...)

			base := rules.DecideAll(baseLedger, baseLedger.Vehicles[vid], profile)
			if !equalKeys(keys(chain), keys(base)) {
				mismatches++
				if mismatches <= 3 {
					t.Errorf("%s %s: chaincode and baseline disagree\n  chain=%v\n  base =%v", profile, vid[:8], keys(chain), keys(base))
				}
			}

			byRule := map[string][]rules.Finding{}
			for _, f := range chain {
				byRule[f.RuleID] = append(byRule[f.RuleID], f)
			}
			vehicle := l.view.Ledger.Vehicles[vid]
			for _, d := range rules.Registry {
				got := byRule[d.ID]
				if len(got) == 1 && got[0].Status == rules.StatusSuppressed {
					suppressed++
					if profile.Exposes(d.RequiredCapability) {
						t.Errorf("%s %s: %s suppressed although capability is exposed", profile, vid[:8], d.ID)
					}
					continue
				}
				measured := rules.Measure(d, l.view.Ledger, vehicle)
				if !equalKeys(keys(got), keys(measured)) {
					mismatches++
					if mismatches <= 3 {
						t.Errorf("%s %s %s: chaincode and harness disagree\n  chain   =%v\n  measured=%v", profile, vid[:8], d.ID, keys(got), keys(measured))
					}
				}
			}
		}
		if mismatches > 0 {
			t.Errorf("%s: %d parity mismatches", profile, mismatches)
		}
		// The parity corpus is labelled only for the source vehicles; at P3 the
		// view has one visible vehicle per event, so compare per visible vehicle.
		expected := declared
		if profile == rules.ProfileP3 {
			expected = 0
			for _, d := range rules.Registry {
				if !profile.Exposes(d.RequiredCapability) {
					expected += len(vids)
				}
			}
		}
		if suppressed != expected {
			t.Errorf("%s: chaincode emitted %d SUPPRESSED findings, harness declared %d", profile, suppressed, expected)
		}
	}
}

func BenchmarkDisclosureChaincodeMemory(b *testing.B) {
	corpus := parityCorpus(b)
	l := loadProfile(b, corpus, rules.ProfileP1)
	vid := corpus.Vehicles[0].Vehicle.VID
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := l.service.Disclosure(vid, rules.ProfileP1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDisclosureBaseline(b *testing.B) {
	corpus := parityCorpus(b)
	l := loadProfile(b, corpus, rules.ProfileP1)
	vid := corpus.Vehicles[0].Vehicle.VID
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ledger := l.log.Ledger(rules.ProfileP1)
		rules.Disclose(ledger, ledger.Vehicles[vid], rules.ProfileP1)
	}
}

func BenchmarkBaselineVerify(b *testing.B) {
	corpus := parityCorpus(b)
	l := loadProfile(b, corpus, rules.ProfileP1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := l.log.Verify(); err != nil {
			b.Fatal(err)
		}
	}
}
