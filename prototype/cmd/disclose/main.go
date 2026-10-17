// disclose writes each Corpus C vehicle's per-purpose disclosures and histories
// through the chaincode Service over a MemoryRepository. The portal serves them
// when NUXT_FABRIC_DEMO_DIR is set, so the demo needs no running peer.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gimz12/vehicle-registry-portal/prototype/chaincode/registry"
	"github.com/gimz12/vehicle-registry-portal/prototype/experiment"
	"github.com/gimz12/vehicle-registry-portal/prototype/offchain/devstore"
	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

type indexEntry struct {
	VID      string `json:"vid"`
	Scenario string `json:"scenario"`
	Title    string `json:"title"`
	Source   string `json:"source"`
	Vehicle  int    `json:"vehicle"`
}

const demoCase = "DEMO-CASE"

func main() {
	out := flag.String("out", "./demo", "output directory")
	flag.Parse()

	var index []indexEntry
	for _, s := range experiment.Scenarios() {
		// One repository per scenario at P1 pseudonym scope; scenario owner ids stand
		// in for the DMT owner index.
		view, err := experiment.Transform(s.Corpus(), rules.ProfileP1)
		if err != nil {
			log.Fatal(err)
		}
		fleetOf := map[string][]string{}
		for _, canonical := range s.Vehicles {
			fleetOf[canonical.OwnerID] = append(fleetOf[canonical.OwnerID], canonical.Vehicle.VID)
		}
		repo := registry.NewMemoryRepository()
		for _, v := range view.Ledger.Vehicles {
			// A sealed synthetic owner record, so the reveal step works without Fabric.
			anchor, err := devstore.Seal(*out, v.VID, devstore.OwnerPII{
				Name: "Owner " + v.VID[:6], NIC: "19" + v.VID[6:13] + "V", Address: "No. " + v.VID[13:15] + ", Demo Mawatha, Colombo",
			})
			if err != nil {
				log.Fatal(err)
			}
			if err := repo.PutVehicle(&registry.VehicleRecord{Vehicle: *v, EnvelopeAnchor: anchor}); err != nil {
				log.Fatal(err)
			}
		}
		service := registry.NewService(repo)
		for i, canonical := range s.Vehicles {
			vid := canonical.Vehicle.VID
			dir := filepath.Join(*out, vid)
			index = append(index, indexEntry{VID: vid, Scenario: s.ID, Title: s.Title, Source: s.Source, Vehicle: i})

			write := func(name string, value any) {
				if err := writeJSON(filepath.Join(dir, name+".json"), value); err != nil {
					log.Fatal(err)
				}
			}
			for _, purpose := range []struct {
				p     registry.Purpose
				actor string
				party string
			}{
				{registry.PurposeBuyer, "public", ""},
				{registry.PurposeDMT, registry.MSPDMT, ""},
				{registry.PurposeInsurer, registry.MSPInsurer, "insurer-a"},
				{registry.PurposePolice, registry.MSPPolice, ""},
			} {
				d, err := service.DisclosureFor(purpose.actor, purpose.party, purpose.p, vid, "")
				if err != nil {
					log.Fatalf("%s %s: %v", s.ID, purpose.p, err)
				}
				write(string(purpose.p), d)
				if purpose.p == registry.PurposeDMT || purpose.p == registry.PurposePolice {
					record, policy, err := service.HistoryFor(purpose.actor, "", purpose.p, vid, "")
					if err != nil {
						log.Fatal(err)
					}
					write("history-"+string(purpose.p), map[string]any{"record": record, "policy": policy})
				}
			}

			// Owner-scope view under an approved demo case, when a P1 or predicate ALERT exists.
			fleet := fleetOf[canonical.OwnerID]
			police, _ := service.DisclosureFor(registry.MSPPolice, "", registry.PurposePolice, vid, "")
			ground := ""
			for _, f := range police.Failed {
				if f.SatisfiesRevealPrecondition() {
					ground = f.RuleID
					break
				}
			}
			if ground == "" && len(fleet) > 1 {
				for _, rule := range []string{"F6b", "F6a"} {
					result, err := service.EvaluateOwnerPredicate(registry.MSPDMT, "demo-dmt-1", vid, rule, fleet, "demo-tx")
					if err == nil && result.Strength == rules.StrengthAlert {
						ground = rule
						write("PREDICATE-"+rule, result)
						break
					}
				}
			}
			if ground != "" {
				caseID := demoCase + "-" + vid[:8]
				if _, err := service.CreateRevealRequestBy(registry.MSPPolice, "demo-police-1", caseID, vid, ground, "FRAUD_INVESTIGATION", "DEMO/2026/"+vid[:4]); err != nil {
					log.Fatal(err)
				}
				if _, err := service.ApproveRevealBy(registry.MSPDMT, "demo-dmt-1", caseID, true, "", fleet); err != nil {
					log.Fatal(err)
				}
				if _, err := service.ApproveRevealBy(registry.MSPPolice, "demo-police-2", caseID, true, "", nil); err != nil {
					log.Fatal(err)
				}
				owner, err := service.DisclosureForFleet(registry.MSPPolice, "", registry.PurposePolice, vid, caseID, fleet)
				if err != nil {
					log.Fatal(err)
				}
				write("POLICE-OWNER", owner)
			}
		}
	}
	if err := writeJSON(filepath.Join(*out, "index.json"), index); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote demo disclosures for %d vehicles to %s\n", len(index), *out)
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
