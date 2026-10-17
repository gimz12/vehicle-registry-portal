package rules

import (
	"encoding/json"
	"testing"
)

// ledgerOf builds a single-vehicle ledger with no owner index (i.e. P1-style
// linkage) unless the caller adds one.
func ledgerOf(v *Vehicle) *Ledger {
	return &Ledger{Vehicles: map[string]*Vehicle{v.VID: v}}
}

func mileage(seq uint64, day, odo int64, tier Tier) Event {
	return Event{Seq: seq, Type: EventMileage, VID: "v1", Day: day, Odometer: odo, Tier: tier, VETLaneID: "lane-1"}
}

func statusOf(t *testing.T, findings []Finding, ruleID string) Finding {
	t.Helper()
	for _, f := range findings {
		if f.RuleID == ruleID {
			return f
		}
	}
	t.Fatalf("no finding for rule %s in %+v", ruleID, findings)
	return Finding{}
}

// Decide has exactly three outcomes; each must be reachable and correctly chosen.
func TestDecideThreeOutcomes(t *testing.T) {
	rollback := &Vehicle{VID: "v1", Events: []Event{
		mileage(1, 0, 50_000, E2),
		mileage(2, 400, 40_000, E2), // below the previous reading
	}}

	tests := []struct {
		name    string
		rule    string
		vehicle *Vehicle
		profile Profile
		want    Status
	}{
		{"evaluates and fires at the design profile", "F1a", rollback, ProfileP1, StatusFlagged},
		{"suppressed once the vehicle link is hidden", "F1a", rollback, ProfileP3, StatusSuppressed},
		{"owner-scope rule suppressed at the design profile", "F6a", rollback, ProfileP1, StatusSuppressed},
		{"gap notice when the antecedent is absent", "F3a", rollback, ProfileP1, StatusGapNotice},
		{"passes when nothing is wrong", "F1a", &Vehicle{VID: "v1", Events: []Event{
			mileage(1, 0, 50_000, E2),
			mileage(2, 400, 62_000, E2),
		}}, ProfileP1, StatusPass},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := Lookup(tc.rule)
			if !ok {
				t.Fatalf("rule %s not registered", tc.rule)
			}
			got := Decide(d, ledgerOf(tc.vehicle), tc.vehicle, tc.profile)
			if len(got) == 0 {
				t.Fatal("Decide returned no finding; it must always account for the rule")
			}
			if got[0].Status != tc.want {
				t.Errorf("status = %s, want %s (reason: %s)", got[0].Status, tc.want, got[0].Reason)
			}
		})
	}
}

// A suppressed finding must name the scope it lacked — that string is what the
// buyer-facing portal renders, so an empty reason is a silent failure.
func TestSuppressionStatesItsReason(t *testing.T) {
	v := &Vehicle{VID: "v1", Events: []Event{mileage(1, 0, 10, E2), mileage(2, 400, 5, E2)}}
	d, _ := Lookup("F6a")
	got := Decide(d, ledgerOf(v), v, ProfileP1)
	if got[0].Reason == "" {
		t.Fatal("suppressed finding carries no reason")
	}
	if got[0].Status != StatusSuppressed {
		t.Fatalf("status = %s, want SUPPRESSED", got[0].Status)
	}
}

// An E0-only antecedent must yield a gap notice rather than a clean pass: a
// self-declared reading is not evidence the registry may vouch for.
func TestE0AntecedentYieldsGapNotice(t *testing.T) {
	v := &Vehicle{VID: "v1", Events: []Event{
		mileage(1, 0, 50_000, E0),
		mileage(2, 400, 62_000, E0),
	}}
	d, _ := Lookup("F1a")
	got := Decide(d, ledgerOf(v), v, ProfileP1)
	if got[0].Status != StatusGapNotice {
		t.Fatalf("status = %s, want GAP_NOTICE for E0-only evidence", got[0].Status)
	}
}

// Evidence tiers must cap severity: an alert cannot rest on weak evidence.
func TestStrengthIsCappedByEvidenceTier(t *testing.T) {
	tests := []struct {
		name     string
		proposed Strength
		events   []Event
		want     Strength
	}{
		{"E2 pair supports an alert", StrengthAlert,
			[]Event{{Tier: E2}, {Tier: E2}}, StrengthAlert},
		{"E1 only downgrades an alert to warning", StrengthAlert,
			[]Event{{Tier: E1}, {Tier: E1}}, StrengthWarning},
		{"any E0 antecedent caps at info", StrengthAlert,
			[]Event{{Tier: E2}, {Tier: E0}}, StrengthInfo},
		{"mixed E1/E2 still supports an alert", StrengthAlert,
			[]Event{{Tier: E1}, {Tier: E2}}, StrengthAlert},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := StrengthFor(tc.proposed, tc.events...); got != tc.want {
				t.Errorf("StrengthFor = %s, want %s", got, tc.want)
			}
		})
	}
}

// A submitter-side measurement flag must never unlock an owner's identity: a
// tester's negligence is not evidence against the owner.
func TestSubmitterSideFlagCannotTriggerReveal(t *testing.T) {
	ownerFraud := Finding{RuleID: "F1a", Status: StatusFlagged, Strength: StrengthAlert, Subject: SubjectOwner}
	if !ownerFraud.SatisfiesRevealPrecondition() {
		t.Error("an ALERT-strength rollback should satisfy the reveal precondition")
	}

	measurement := Finding{RuleID: "F1c", Status: StatusFlagged, Strength: StrengthAlert, Subject: SubjectSubmitter}
	if measurement.SatisfiesRevealPrecondition() {
		t.Error("a submitter-side measurement flag must not satisfy the reveal precondition")
	}

	weak := Finding{RuleID: "F1a", Status: StatusFlagged, Strength: StrengthWarning, Subject: SubjectOwner}
	if weak.SatisfiesRevealPrecondition() {
		t.Error("a WARNING-strength finding must not satisfy the reveal precondition")
	}
}

// The buyer's view must account for every registered rule — nothing may vanish.
func TestDisclosureAccountsForEveryRule(t *testing.T) {
	v := &Vehicle{VID: "v1", Events: []Event{mileage(1, 0, 50_000, E2)}}
	d := Disclose(ledgerOf(v), v, ProfileP1)
	total := len(d.Passed) + len(d.Failed) + len(d.NotPerformed)
	if total != len(Registry) {
		t.Fatalf("disclosure covers %d rules, registry has %d — a check went missing", total, len(Registry))
	}
	if len(d.NotPerformed) == 0 {
		t.Error("expected at least one declared not-performed check for a sparse vehicle")
	}
}

func TestDisclosureSerialisesEmptyListsAsArrays(t *testing.T) {
	disclosure := Disclosure{
		VID: "v1", Profile: ProfileP1,
		Passed: []Finding{}, Failed: []Finding{}, NotPerformed: []Finding{},
	}
	data, err := json.Marshal(disclosure)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || containsJSONNull(data) {
		t.Fatalf("empty disclosure slices must be arrays, got %s", data)
	}
}

func containsJSONNull(data []byte) bool {
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return true
	}
	return decoded["Passed"] == nil || decoded["Failed"] == nil || decoded["NotPerformed"] == nil
}
