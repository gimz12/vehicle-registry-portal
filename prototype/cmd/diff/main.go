// diff is the derived-disclosure leak tester. It applies one-event perturbations
// to each scenario vehicle's P1 state and records which disclosure fields flip.
// A flip caused by a capability the granted profile does not expose is a leak.
// It refuses to run if the freeze manifest no longer matches.
//
// Usage: go run ./cmd/diff -label after-v1.3
package main

import (
	"encoding/json"
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

type perturbation struct {
	Name       string
	Capability rules.Capability
	Apply      func(v *rules.Vehicle) bool // false when not applicable to this vehicle
}

func nextSeq(v *rules.Vehicle) uint64 {
	var max uint64
	for _, e := range v.Events {
		if e.Seq > max {
			max = e.Seq
		}
	}
	return max + 1
}

func firstOf(v *rules.Vehicle, t rules.EventType, pred func(rules.Event) bool) int {
	for i, e := range v.Events {
		if e.Type == t && (pred == nil || pred(e)) {
			return i
		}
	}
	return -1
}

var perturbations = []perturbation{
	{"re-anchor a claim", rules.CapabilityIncident, func(v *rules.Vehicle) bool {
		i := firstOf(v, rules.EventClaim, func(e rules.Event) bool { return e.ReferenceStatus() == rules.ReferencePresent })
		if i < 0 {
			return false
		}
		v.Events[i].EvidenceAnchor = "report-that-does-not-exist"
		return true
	}},
	{"change owner pseudonym", rules.CapabilityOwner, func(v *rules.Vehicle) bool {
		for i := range v.Events {
			if v.Events[i].P != "" {
				v.Events[i].P = "a-different-owner-pseudonym"
			}
		}
		return true
	}},
	{"add accident", rules.CapabilityAsset, func(v *rules.Vehicle) bool {
		id := "perturbation-accident"
		v.Events = append(v.Events, rules.Event{Seq: nextSeq(v), CommitOrder: 1 << 40, RecordID: id, Type: rules.EventAccident,
			VID: v.VID, Day: 900, Tier: rules.E2, EvidenceAnchor: id, EvidenceKind: "POLICE_REPORT", IncidentRef: rules.ReferencePresent, AttestorID: registry.MSPPolice})
		return true
	}},
	{"remove accident", rules.CapabilityAsset, func(v *rules.Vehicle) bool {
		i := firstOf(v, rules.EventAccident, nil)
		if i < 0 {
			return false
		}
		v.Events = append(v.Events[:i], v.Events[i+1:]...)
		return true
	}},
	{"add claim", rules.CapabilityAsset, func(v *rules.Vehicle) bool {
		v.Events = append(v.Events, rules.Event{Seq: nextSeq(v), CommitOrder: 1<<40 + 1, RecordID: "perturbation-claim", Type: rules.EventClaim,
			VID: v.VID, Day: 905, Tier: rules.E1, EvidenceKind: "ASSESSOR_ATTESTATION", EvidenceAnchor: "assessor-note-x",
			IncidentRef: rules.ReferencePresent, InsurerID: "insurer-b", AttestorID: "assessor-x"})
		return true
	}},
	{"add theft flag", rules.CapabilityAsset, func(v *rules.Vehicle) bool {
		v.Events = append(v.Events, rules.Event{Seq: nextSeq(v), CommitOrder: 1<<40 + 2, RecordID: "perturbation-theft", Type: rules.EventTheftFlag,
			VID: v.VID, Tier: rules.E2, AttestorID: registry.MSPPolice})
		v.Stolen = true
		return true
	}},
	{"shift last reading", rules.CapabilityAsset, func(v *rules.Vehicle) bool {
		last := -1
		for i, e := range v.Events {
			if e.Type == rules.EventMileage {
				last = i
			}
		}
		if last < 0 {
			return false
		}
		v.Events[last].Odometer -= 20_000
		return true
	}},
}

type caller struct {
	Purpose registry.Purpose
	Actor   string
	Party   string
}

var callers = []caller{
	{registry.PurposeBuyer, "public", ""},
	{registry.PurposeDMT, registry.MSPDMT, ""},
	{registry.PurposeInsurer, registry.MSPInsurer, "insurer-a"},
	{registry.PurposePolice, registry.MSPPolice, ""},
}

var fields = []string{"profile", "status bits", "outcome", "strength", "evidence tier", "detail", "reason", "event seq", "structural", "list counts"}

func service(vehicles []*rules.Vehicle) *registry.Service {
	repo := registry.NewMemoryRepository()
	for _, v := range vehicles {
		if err := repo.PutVehicle(&registry.VehicleRecord{Vehicle: *v}); err != nil {
			log.Fatal(err)
		}
	}
	return registry.NewService(repo)
}

func clone(v *rules.Vehicle) *rules.Vehicle {
	c := *v
	c.Events = append([]rules.Event(nil), v.Events...)
	return &c
}

func byRule(d *registry.PurposeDisclosure) map[string]rules.Finding {
	out := map[string]rules.Finding{}
	for _, list := range [][]rules.Finding{d.Failed, d.Passed, d.NotPerformed} {
		for _, f := range list {
			if _, seen := out[f.RuleID]; !seen {
				out[f.RuleID] = f
			}
		}
	}
	return out
}

func flipped(a, b *registry.PurposeDisclosure) map[string]bool {
	out := map[string]bool{}
	if a.Policy.Profile != b.Policy.Profile {
		out["profile"] = true
	}
	if a.Status != b.Status {
		out["status bits"] = true
	}
	if len(a.Passed) != len(b.Passed) || len(a.Failed) != len(b.Failed) || len(a.NotPerformed) != len(b.NotPerformed) {
		out["list counts"] = true
	}
	fa, fb := byRule(a), byRule(b)
	for _, d := range rules.Registry {
		x, y := fa[d.ID], fb[d.ID]
		if x.Status != y.Status {
			out["outcome"] = true
		}
		if x.Strength != y.Strength {
			out["strength"] = true
		}
		if x.Tier != y.Tier {
			out["evidence tier"] = true
		}
		if x.Detail != y.Detail {
			out["detail"] = true
		}
		if x.Reason != y.Reason {
			out["reason"] = true
		}
		if x.AtSeq != y.AtSeq {
			out["event seq"] = true
		}
		if x.Structural != y.Structural {
			out["structural"] = true
		}
	}
	return out
}

type cell struct {
	Purpose      string         `json:"purpose"`
	Profile      string         `json:"profile"`
	Perturbation string         `json:"perturbation"`
	Capability   string         `json:"capability"`
	Exposed      bool           `json:"capabilityExposed"`
	Applied      int            `json:"vehiclesPerturbed"`
	WithFlip     int            `json:"vehiclesWithAnyFlip"`
	Flips        map[string]int `json:"flipsByField"`
	Leaks        int            `json:"leaks"`
}

func main() {
	label := flag.String("label", "run", "output label, e.g. before-v1.2 or after-v1.3")
	out := flag.String("out", "eval/results/2026-10-03-ext/diff", "output directory")
	flag.Parse()
	if err := experiment.VerifyFrozen(".", manifest); err != nil {
		log.Fatalf("diff: frozen inputs changed: %v", err)
	}
	digest, _ := experiment.ManifestDigest(manifest)
	fmt.Printf("frozen manifest %s\n", digest)

	cells := map[string]*cell{}
	for _, s := range experiment.Scenarios() {
		state, err := experiment.Transform(s.Corpus(), rules.ProfileP1)
		if err != nil {
			log.Fatal(err)
		}
		var base []*rules.Vehicle
		for _, v := range state.Ledger.Vehicles {
			base = append(base, v)
		}
		sort.Slice(base, func(i, j int) bool { return base[i].VID < base[j].VID })
		baseSvc := service(base)
		for _, canonical := range s.Vehicles {
			vid := canonical.Vehicle.VID
			for _, p := range perturbations {
				var changed []*rules.Vehicle
				applied := false
				for _, v := range base {
					c := clone(v)
					if v.VID == vid {
						applied = p.Apply(c)
					}
					changed = append(changed, c)
				}
				if !applied {
					continue
				}
				perturbedSvc := service(changed)
				for _, c := range callers {
					a, errA := baseSvc.DisclosureFor(c.Actor, c.Party, c.Purpose, vid, "")
					b, errB := perturbedSvc.DisclosureFor(c.Actor, c.Party, c.Purpose, vid, "")
					if errA != nil || errB != nil {
						log.Fatalf("%s %s %s: %v %v", s.ID, p.Name, c.Purpose, errA, errB)
					}
					profile := a.Policy.Profile
					k := fmt.Sprintf("%s|%s|%s", c.Purpose, profile, p.Name)
					entry := cells[k]
					if entry == nil {
						entry = &cell{Purpose: string(c.Purpose), Profile: string(profile), Perturbation: p.Name,
							Capability: string(p.Capability), Exposed: profile.Exposes(p.Capability), Flips: map[string]int{}}
						cells[k] = entry
					}
					entry.Applied++
					f := flipped(a, b)
					if len(f) > 0 {
						entry.WithFlip++
						if !entry.Exposed {
							entry.Leaks++
						}
					}
					for name := range f {
						entry.Flips[name]++
					}
				}
			}
		}
	}

	keys := make([]string, 0, len(cells))
	for k := range cells {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var list []*cell
	totalLeaks := 0
	var md strings.Builder
	fmt.Fprintf(&md, "# Derived-disclosure leak test (%s)\n\nFrozen manifest `%s`. 26 named scenarios, live-style ledger state (P1 pseudonyms, incident links present), one-event perturbations, disclosure recomputed through the chaincode Service per purpose.\n\nA **leak** is any flip caused by perturbing a capability the granted profile does not expose.\n\n", *label, digest)
	fmt.Fprintf(&md, "| Purpose | Profile | Perturbation | Capability exposed? | Vehicles | With any flip | Leaks | Fields that flipped |\n|---|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		e := cells[k]
		list = append(list, e)
		totalLeaks += e.Leaks
		var fs []string
		for _, name := range fields {
			if n := e.Flips[name]; n > 0 {
				fs = append(fs, fmt.Sprintf("%s %d", name, n))
			}
		}
		exposed := "yes"
		if !e.Exposed {
			exposed = "**no**"
		}
		fmt.Fprintf(&md, "| %s | %s | %s | %s %s | %d | %d | %d | %s |\n", e.Purpose, e.Profile, e.Perturbation, e.Capability, exposed, e.Applied, e.WithFlip, e.Leaks, strings.Join(fs, ", "))
	}
	fmt.Fprintf(&md, "\n**Total leaks: %d**\n", totalLeaks)

	dir := filepath.Join(*out, *label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	data, _ := json.MarshalIndent(map[string]any{"label": *label, "manifest": digest, "cells": list, "totalLeaks": totalLeaks}, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "diff.json"), append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diff.md"), []byte(md.String()), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("total leaks: %d  (written to %s)\n", totalLeaks, dir)
}
