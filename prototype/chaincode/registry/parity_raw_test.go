package registry

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/gimz12/vehicle-registry-portal/prototype/experiment"
	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

// Raw-state parity: the repository holds the untransformed P1 view, so the
// chaincode must apply P1 and P2 itself and agree with the harness.
func TestParityRawStateChaincodeAppliesProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("raw-state parity over 500 vehicles takes a few seconds")
	}
	corpus := parityCorpus(t)
	state, err := experiment.Transform(corpus, rules.ProfileP1)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewMemoryRepository()
	for _, v := range state.Ledger.Vehicles {
		if err := repo.PutVehicle(&VehicleRecord{Vehicle: *v}); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(repo)
	vids := make([]string, 0, len(state.Ledger.Vehicles))
	for vid := range state.Ledger.Vehicles {
		vids = append(vids, vid)
	}
	sort.Strings(vids)

	total := 0
	for _, profile := range []rules.Profile{rules.ProfileP1, rules.ProfileP2} {
		view, err := experiment.Transform(corpus, profile)
		if err != nil {
			t.Fatal(err)
		}
		perRule := map[string]int{}
		vehiclesHit := map[string]bool{}
		for _, vid := range vids {
			disclosure, err := service.Disclosure(vid, profile)
			if err != nil {
				t.Fatalf("%s Disclosure %s: %v", profile, vid[:8], err)
			}
			byRule := map[string][]rules.Finding{}
			for _, list := range [][]rules.Finding{disclosure.Failed, disclosure.Passed, disclosure.NotPerformed} {
				for _, f := range list {
					byRule[f.RuleID] = append(byRule[f.RuleID], f)
				}
			}
			for _, d := range rules.Registry {
				got := byRule[d.ID]
				if len(got) == 1 && got[0].Status == rules.StatusSuppressed {
					continue // declared by the profile gate; nothing to compare
				}
				measured := rules.Measure(d, view.Ledger, view.Ledger.Vehicles[vid])
				if !equalKeys(keys(got), keys(measured)) {
					perRule[d.ID]++
					vehiclesHit[vid] = true
				}
			}
		}
		ids := make([]string, 0, len(perRule))
		for id := range perRule {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		var parts []string
		for _, id := range ids {
			parts = append(parts, fmt.Sprintf("%s=%d", id, perRule[id]))
			total += perRule[id]
		}
		if len(parts) == 0 {
			parts = []string{"none"}
		}
		t.Logf("raw-state %s: %d of %d vehicles disagree; disagreements per rule: %s",
			profile, len(vehiclesHit), len(vids), strings.Join(parts, ", "))
	}
	if total > 0 {
		t.Errorf("chaincode on raw ledger state disagrees with the harness in %d (vehicle, rule) cells", total)
	}
}
