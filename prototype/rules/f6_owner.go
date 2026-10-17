package rules

import (
	"fmt"
	"sort"
)

// F6 — owner-pattern rules (serial fraud across one person's vehicles).
//
// This family is the whole point of the thesis's measurement. It requires OWNER
// scope, so under the registry's own design (P1, one pseudonym per vehicle) it is
// undecidable by construction — a person's cars are deliberately unlinkable, and
// any fraud that repeats across them is invisible. The registry does not fail
// silently there: Decide() emits a suppression notice naming the missing scope.
//
// The expected Chapter 6 result is therefore recall 0.00 at P1/P2/P3 and non-zero
// only at P0 (and, once built, under governed owner-scope escalation P1+E, which
// sits below the cut line and is out of this prototype's scope).

const serialWindowDays = 730 // 24 months

func init() {
	Register(Descriptor{
		ID:                 "F6a",
		Family:             "F6",
		Title:              "Serial odometer fraud across the owner's vehicles",
		RequiredCapability: CapabilityOwner,
		Antecedents:        []EventType{EventMileage},
		CanEvaluate:        canEvaluateOwnerMileage,
		Eval:               evalSerialOdometer,
	})
	Register(Descriptor{
		ID:                 "F6b",
		Family:             "F6",
		Title:              "Serial insurance claims across the owner's vehicles",
		RequiredCapability: CapabilityOwner,
		Antecedents:        []EventType{EventClaim},
		CanEvaluate:        canEvaluateOwnerClaims,
		Eval:               evalSerialClaims,
	})
}

func canEvaluateOwnerMileage(l *Ledger, v *Vehicle) Availability {
	if l == nil || l.OwnerLink == ReferenceRedacted {
		return Availability{Reason: "cross-vehicle owner linkage was removed by the active data view", Structural: true}
	}
	return canEvaluateMileagePair(l, v)
}

func canEvaluateOwnerClaims(l *Ledger, v *Vehicle) Availability {
	if l == nil || l.OwnerLink == ReferenceRedacted {
		return Availability{Reason: "cross-vehicle owner linkage was removed by the active data view", Structural: true}
	}
	claims := v.EventsOfType(EventClaim)
	if len(claims) == 0 {
		return Availability{Reason: "no CLAIM record exists for this vehicle"}
	}
	return Availability{Available: true}
}

// ownerFleet returns the other vehicles sharing this vehicle's owner pseudonym.
// At any profile below P0 the index is empty, so the fleet is just this vehicle —
// but Decide() suppresses these rules before they are ever reached there.
func ownerFleet(l *Ledger, v *Vehicle) []*Vehicle {
	var owner string
	for _, e := range v.Events {
		if e.P != "" {
			owner = e.P
			break
		}
	}
	if owner == "" || l.OwnerVehicles == nil {
		return []*Vehicle{v}
	}
	var out []*Vehicle
	for _, vid := range l.OwnerVehicles[owner] {
		if other, ok := l.Vehicles[vid]; ok {
			out = append(out, other)
		}
	}
	if len(out) == 0 {
		return []*Vehicle{v}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VID < out[j].VID })
	return out
}

func evalSerialOdometer(l *Ledger, v *Vehicle) []Finding {
	fleet := ownerFleet(l, v)
	if len(fleet) < 2 {
		return nil
	}

	// Count vehicles in the fleet carrying an odometer alert, and take the window
	// from the alerts themselves.
	var hits int
	var first, last int64 = 1<<62 - 1, -1
	var support []Event
	for _, f := range fleet {
		for _, finding := range evalOdometerMonotonic(l, f) {
			if !finding.Flagged() {
				continue
			}
			hits++
			support = append(support, Event{Tier: finding.Tier})
			for _, e := range f.EventsOfType(EventMileage) {
				if e.Seq == finding.AtSeq {
					if e.Day < first {
						first = e.Day
					}
					if e.Day > last {
						last = e.Day
					}
				}
			}
			break // one vehicle counts once
		}
	}

	if hits >= 2 && last-first <= serialWindowDays {
		return []Finding{{
			RuleID:   "F6a",
			VID:      v.VID,
			Status:   StatusFlagged,
			Strength: StrengthFor(StrengthAlert, support...),
			Tier:     MinTier(support...),
			Subject:  SubjectOwner,
			Detail: fmt.Sprintf("the same owner holds %d vehicles carrying odometer rollback alerts within %d days",
				hits, last-first),
		}}
	}
	return nil
}

func evalSerialClaims(l *Ledger, v *Vehicle) []Finding {
	fleet := ownerFleet(l, v)
	if len(fleet) < 2 {
		return nil
	}

	// Claims across the fleet that share an assessor or garage node — the actor
	// graph signal, restricted here to one owner's vehicles.
	byAttestor := map[string][]Event{}
	for _, f := range fleet {
		seen := map[string]bool{}
		for _, c := range f.EventsOfType(EventClaim) {
			if c.AttestorID == "" || seen[c.AttestorID] {
				continue
			}
			seen[c.AttestorID] = true
			byAttestor[c.AttestorID] = append(byAttestor[c.AttestorID], c)
		}
	}

	attestors := make([]string, 0, len(byAttestor))
	for attestor := range byAttestor {
		attestors = append(attestors, attestor)
	}
	sort.Strings(attestors)
	for _, attestor := range attestors {
		events := byAttestor[attestor]
		if len(events) >= 2 {
			return []Finding{{
				RuleID:   "F6b",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: StrengthFor(StrengthAlert, events...),
				Tier:     MinTier(events...),
				Subject:  SubjectOwner,
				Detail: fmt.Sprintf("claims on %d of this owner's vehicles share the same assessor (%s)",
					len(events), attestor),
			}}
		}
	}
	return nil
}
