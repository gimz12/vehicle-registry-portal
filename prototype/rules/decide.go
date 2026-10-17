package rules

import (
	"fmt"
	"sort"
)

// Descriptor declares what a rule needs before it can run. RequiredCapability
// drives declared suppression; CanEvaluate drives data-derived gap notices.
type Descriptor struct {
	ID                 string
	Family             string
	Title              string // rendered to the buyer
	RequiredCapability Capability
	Antecedents        []EventType // event types without which the rule cannot be decided
	CanEvaluate        func(l *Ledger, v *Vehicle) Availability
	Eval               func(l *Ledger, v *Vehicle) []Finding
}

// Availability is derived from the data visible to a rule, independently of
// the descriptor's declared capability. The evaluation harness uses this path
// so the prediction table cannot manufacture its own measured breakpoint.
type Availability struct {
	Available bool
	Reason    string
	// Structural marks a gap caused by the active data view (redacted reference,
	// split history, removed owner link) rather than by weak or missing
	// evidence. Only structural gaps change a cell's decidability class.
	Structural bool
}

// Registry is the registry's full rule set, in reporting order.
var Registry = []Descriptor{}

// Register adds a rule to the registry. Called from each family's init().
func Register(d Descriptor) { Registry = append(Registry, d) }

// Lookup returns the descriptor with the given id.
func Lookup(id string) (Descriptor, bool) {
	for _, d := range Registry {
		if d.ID == id {
			return d, true
		}
	}
	return Descriptor{}, false
}

// Decide runs one rule against one vehicle under the active linkage profile —
// Algorithm 6. It is the single gate through which every rule evaluation passes,
// in both the chaincode and the harness.
//
// The three outcomes are the point of the design: a rule that cannot be decided
// says so, rather than returning nothing and letting silence read as "clean".
//
//  1. required scope not exposed by the profile -> SUPPRESSED
//  2. an antecedent event type absent, or present only at E0 -> GAP_NOTICE
//  3. otherwise evaluate
func Decide(d Descriptor, l *Ledger, v *Vehicle, p Profile) []Finding {
	if !p.Exposes(d.RequiredCapability) {
		return []Finding{{
			RuleID:  d.ID,
			VID:     v.VID,
			Status:  StatusSuppressed,
			Tier:    E0,
			Detail:  d.Title,
			Reason:  fmt.Sprintf("requires %s capability, not available under profile %s", d.RequiredCapability, p),
			Subject: SubjectSystem,
		}}
	}
	return Measure(d, l, v)
}

// Measure evaluates one rule using only the supplied ledger view. It deliberately
// does not inspect RequiredCapability or Profile and is therefore suitable for
// the independent prediction-versus-measurement experiment.
func Measure(d Descriptor, l *Ledger, v *Vehicle) []Finding {
	availability := availabilityFor(d, l, v)
	if !availability.Available {
		return []Finding{{
			RuleID:     d.ID,
			VID:        v.VID,
			Status:     StatusGapNotice,
			Tier:       E0,
			Detail:     d.Title,
			Reason:     availability.Reason,
			Subject:    SubjectSystem,
			Structural: availability.Structural,
		}}
	}

	out := d.Eval(l, v)
	if len(out) == 0 {
		return []Finding{{
			RuleID:   d.ID,
			VID:      v.VID,
			Status:   StatusPass,
			Strength: StrengthInfo,
			Tier:     tierOfAntecedents(v, d.Antecedents),
			Detail:   d.Title,
			Subject:  SubjectSystem,
		}}
	}
	return out
}

// DecideAll runs every registered rule against one vehicle.
func DecideAll(l *Ledger, v *Vehicle, p Profile) []Finding {
	var out []Finding
	for _, d := range Registry {
		out = append(out, Decide(d, l, v, p)...)
	}
	sortFindings(out)
	return out
}

func availabilityFor(d Descriptor, l *Ledger, v *Vehicle) Availability {
	if d.CanEvaluate != nil {
		return d.CanEvaluate(l, v)
	}
	for _, want := range d.Antecedents {
		got := v.EventsOfType(want)
		if len(got) == 0 {
			return Availability{Reason: fmt.Sprintf("no %s record exists for this vehicle", want)}
		}
		if bestTier(got) == E0 {
			return Availability{Reason: fmt.Sprintf("%s records are self-declared only (E0); no accountable attestation", want)}
		}
	}
	return Availability{Available: true}
}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.AtSeq != b.AtSeq {
			return a.AtSeq < b.AtSeq
		}
		if a.VID != b.VID {
			return a.VID < b.VID
		}
		return a.Detail < b.Detail
	})
}

func bestTier(events []Event) Tier {
	best := E0
	for _, e := range events {
		if e.Tier > best {
			best = e.Tier
		}
	}
	return best
}

func tierOfAntecedents(v *Vehicle, want []EventType) Tier {
	var all []Event
	for _, t := range want {
		all = append(all, v.EventsOfType(t)...)
	}
	return MinTier(all...)
}

// Disclosure is the buyer-facing view of one vehicle: the three lists the
// pre-purchase portal renders. NotPerformed is the contribution — the checks the
// registry declares it could not make.
type Disclosure struct {
	VID          string
	Profile      Profile
	Passed       []Finding
	Failed       []Finding
	NotPerformed []Finding
}

// Disclose partitions a vehicle's findings into the buyer's three lists.
func Disclose(l *Ledger, v *Vehicle, p Profile) Disclosure {
	d := Disclosure{
		VID: v.VID, Profile: p,
		Passed: []Finding{}, Failed: []Finding{}, NotPerformed: []Finding{},
	}
	for _, f := range DecideAll(l, v, p) {
		switch f.Status {
		case StatusFlagged:
			d.Failed = append(d.Failed, f)
		case StatusPass:
			d.Passed = append(d.Passed, f)
		default:
			d.NotPerformed = append(d.NotPerformed, f)
		}
	}
	return d
}
