package rules

import "fmt"

// F1 — odometer rules (thesis Algorithm 4).
//
// Two deviations from the interim text, both deliberate:
//
//   - Accept-and-flag, never reject. A rejected non-monotonic write destroys the
//     only evidence the registry could hold, so the reading is committed and
//     flagged instead.
//   - Readings are ordered by block commit sequence, and a fail-and-retest
//     sequence at one lane collapses to its last reading before the rate rule
//     runs, so a legitimate retest cannot fire the km/day check.
//
// Required scope is ASSET: the rule compares two readings of the same vehicle and
// never needs to know who owns it. That is why it survives every profile except
// P3, where the vehicle link itself is gone.

const maxKmPerDay = 1000

func init() {
	Register(Descriptor{
		ID:                 "F1a",
		Family:             "F1",
		Title:              "Odometer monotonicity",
		RequiredCapability: CapabilityAsset,
		Antecedents:        []EventType{EventMileage},
		CanEvaluate:        canEvaluateMileagePair,
		Eval:               evalOdometerMonotonic,
	})
	Register(Descriptor{
		ID:                 "F1b",
		Family:             "F1",
		Title:              "Odometer rate of change",
		RequiredCapability: CapabilityAsset,
		Antecedents:        []EventType{EventMileage},
		CanEvaluate:        canEvaluateMileagePair,
		Eval:               evalOdometerRate,
	})
	Register(Descriptor{
		ID:                 "F1c",
		Family:             "F1",
		Title:              "Odometer stagnation",
		RequiredCapability: CapabilityAsset,
		Antecedents:        []EventType{EventMileage},
		CanEvaluate:        canEvaluateMileagePair,
		Eval:               evalOdometerStagnant,
	})
}

func canEvaluateMileagePair(_ *Ledger, v *Vehicle) Availability {
	if !v.HasCompleteAssetHistory() {
		return Availability{Reason: "vehicle history was split by the active data view", Structural: true}
	}
	readings := v.EventsOfType(EventMileage)
	if len(readings) < 2 {
		return Availability{Reason: "at least two mileage records are required"}
	}
	if bestTier(readings) == E0 {
		return Availability{Reason: "mileage records are self-declared only (E0); no accountable attestation"}
	}
	return Availability{Available: true}
}

// collapseRetests reduces each (lane, retest group) run to its final reading.
// A vehicle that fails an emission test and returns the same week produces
// several readings minutes apart; without this the rate rule sees a ~0-day
// interval and fires on honest data.
func collapseRetests(events []Event) []Event {
	var out []Event
	for _, e := range events {
		if e.RetestGroup != "" && len(out) > 0 {
			prev := out[len(out)-1]
			if prev.RetestGroup == e.RetestGroup && prev.VETLaneID == e.VETLaneID {
				out[len(out)-1] = e // later reading in the same retest run supersedes
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

func evalOdometerMonotonic(_ *Ledger, v *Vehicle) []Finding {
	readings := collapseRetests(v.EventsOfType(EventMileage))
	var out []Finding
	for i := 1; i < len(readings); i++ {
		prev, cur := readings[i-1], readings[i]
		if cur.Odometer < prev.Odometer {
			out = append(out, Finding{
				RuleID:   "F1a",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: StrengthFor(StrengthAlert, prev, cur),
				Tier:     MinTier(prev, cur),
				AtSeq:    cur.Seq,
				Subject:  SubjectOwner,
				Detail: fmt.Sprintf("reading of %d km is below the previous recorded reading of %d km",
					cur.Odometer, prev.Odometer),
			})
		}
	}
	return out
}

func evalOdometerRate(_ *Ledger, v *Vehicle) []Finding {
	readings := collapseRetests(v.EventsOfType(EventMileage))
	var out []Finding
	for i := 1; i < len(readings); i++ {
		prev, cur := readings[i-1], readings[i]
		days := cur.Day - prev.Day
		if days <= 0 || cur.Odometer < prev.Odometer {
			continue // same-day pair, or a rollback F1a already owns
		}
		if perDay := (cur.Odometer - prev.Odometer) / days; perDay > maxKmPerDay {
			out = append(out, Finding{
				RuleID:   "F1b",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: StrengthFor(StrengthWarning, prev, cur),
				Tier:     MinTier(prev, cur),
				AtSeq:    cur.Seq,
				Subject:  SubjectOwner,
				Detail: fmt.Sprintf("%d km accumulated over %d days (%d km/day) exceeds the plausible rate",
					cur.Odometer-prev.Odometer, days, perDay),
			})
		}
	}
	return out
}

func evalOdometerStagnant(_ *Ledger, v *Vehicle) []Finding {
	readings := collapseRetests(v.EventsOfType(EventMileage))
	var out []Finding
	for i := 1; i < len(readings); i++ {
		prev, cur := readings[i-1], readings[i]
		if cur.Odometer == prev.Odometer && cur.Day-prev.Day >= 365 {
			out = append(out, Finding{
				RuleID:   "F1c",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: StrengthFor(StrengthWarning, prev, cur),
				Tier:     MinTier(prev, cur),
				AtSeq:    cur.Seq,
				Subject:  SubjectSubmitter,
				Detail: fmt.Sprintf("identical reading of %d km after %d days — possible mileage blocker or recycled reading",
					cur.Odometer, cur.Day-prev.Day),
			})
		}
	}
	return out
}
