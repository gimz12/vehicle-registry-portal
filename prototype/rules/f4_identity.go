package rules

import (
	"fmt"
	"sort"
)

// F4 — identity uniqueness (candidate rule R4).
//
// Directly targets the COPA Rs. 1.2 bn typology: inserting the engine/chassis
// numbers of illegally imported vehicles into existing registrations. It is
// possible only because the keyed deterministic index replaced the per-vehicle
// salted hash — a fresh salt yields a different digest for the same chassis, so
// the interim design could never match identifiers across records at all.
//
// This family is the sweep's CONTROL ROW, and its required scope is NONE: the
// rule compares keyed identity indices carried by individual registration
// records, so it needs no pseudonym linkage whatsoever — not even the vehicle
// link. Its recall must therefore be flat across all four profiles, P3 included.
// If Chapter 6 shows it moving, the harness is wrong; that is stated in advance
// as the experiment's own correctness condition.

func init() {
	Register(Descriptor{
		ID:                 "F4",
		Family:             "F4",
		Title:              "Chassis and engine identity uniqueness",
		RequiredCapability: CapabilityIndex,
		Antecedents:        []EventType{EventRegistration},
		Eval:               evalIdentityUnique,
	})
}

func evalIdentityUnique(l *Ledger, v *Vehicle) []Finding {
	regs := v.EventsOfType(EventRegistration)
	if len(regs) == 0 {
		return nil
	}
	mine := regs[0]

	var out []Finding
	otherVIDs := make([]string, 0, len(l.Vehicles))
	for otherVID := range l.Vehicles {
		otherVIDs = append(otherVIDs, otherVID)
	}
	sort.Strings(otherVIDs)
	for _, otherVID := range otherVIDs {
		other := l.Vehicles[otherVID]
		if otherVID == v.VID {
			continue
		}
		otherRegs := other.EventsOfType(EventRegistration)
		if len(otherRegs) == 0 {
			continue
		}
		theirs := otherRegs[0]

		// Report once, from the later registration's point of view, so a duplicate
		// pair yields one finding rather than two.
		if theirs.Order() >= mine.Order() {
			continue
		}
		duplicateChassis := mine.IdxChassis != "" && mine.IdxChassis == theirs.IdxChassis
		duplicateEngine := mine.IdxEngine != "" && mine.IdxEngine == theirs.IdxEngine
		if duplicateChassis || duplicateEngine {
			strength := StrengthFor(StrengthAlert, mine, theirs)
			identity := "engine"
			if duplicateChassis && duplicateEngine {
				identity = "chassis and engine"
			} else if duplicateChassis {
				identity = "chassis"
			}
			detail := fmt.Sprintf("%s identity already registered to another vehicle at commit order %d", identity, theirs.Order())
			if other.Retired {
				detail = fmt.Sprintf("%s identity belongs to a retired vehicle (registered at commit order %d) — resurrection attempt", identity, theirs.Order())
			}
			out = append(out, Finding{
				RuleID:   "F4",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: strength,
				Tier:     MinTier(mine, theirs),
				AtSeq:    mine.Seq,
				Subject:  SubjectSubmitter,
				Detail:   detail,
			})
		}
	}
	return out
}
