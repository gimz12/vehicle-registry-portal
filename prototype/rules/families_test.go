package rules

import (
	"strings"
	"testing"
)

// TestDeclaredCapabilitiesMatchPrediction encodes the analytical prediction as a
// configuration-regression test. It is deliberately not treated as empirical
// validation; the independent transformed-view harness supplies that evidence.
//
// This is the thesis's headline comparison in miniature. The prediction is
// derived analytically from each rule's logic (what must it compare?), while the
// measurement comes from executing Decide(). If this test ever fails, either the
// prediction or the implementation is wrong — and saying which is a reportable
// result, not a bug to paper over.
//
//	F1 odometer            ASSET       — survives P0/P1/P2, dies at P3
//	F2 stolen-transfer     ASSET       — survives P0/P1/P2, dies at P3
//	F3 accident-insurance  ASSET_EVENT — survives P0/P1, dies at P2
//	F4 identity-uniqueness NONE        — CONTROL, flat across all four profiles
//	F5 provenance          NONE        — survives to P3
//	F6 owner-pattern       OWNER       — decidable only at P0
func TestDeclaredCapabilitiesMatchPrediction(t *testing.T) {
	predicted := map[string]map[Profile]bool{
		"F1a": {ProfileP0: true, ProfileP1: true, ProfileP2: true, ProfileP3: false},
		"F1b": {ProfileP0: true, ProfileP1: true, ProfileP2: true, ProfileP3: false},
		"F1c": {ProfileP0: true, ProfileP1: true, ProfileP2: true, ProfileP3: false},
		"F2":  {ProfileP0: true, ProfileP1: true, ProfileP2: true, ProfileP3: false},
		"F3a": {ProfileP0: true, ProfileP1: true, ProfileP2: true, ProfileP3: true},
		"F3b": {ProfileP0: true, ProfileP1: true, ProfileP2: false, ProfileP3: false},
		"F3c": {ProfileP0: true, ProfileP1: true, ProfileP2: false, ProfileP3: false},
		"F3d": {ProfileP0: true, ProfileP1: true, ProfileP2: true, ProfileP3: false},
		"F4":  {ProfileP0: true, ProfileP1: true, ProfileP2: true, ProfileP3: true},
		"F5":  {ProfileP0: true, ProfileP1: true, ProfileP2: true, ProfileP3: true},
		"F6a": {ProfileP0: true, ProfileP1: false, ProfileP2: false, ProfileP3: false},
		"F6b": {ProfileP0: true, ProfileP1: false, ProfileP2: false, ProfileP3: false},
	}

	for ruleID, byProfile := range predicted {
		d, ok := Lookup(ruleID)
		if !ok {
			t.Fatalf("rule %s not registered", ruleID)
		}
		for profile, wantDecidable := range byProfile {
			gotDecidable := profile.Exposes(d.RequiredCapability)
			if gotDecidable != wantDecidable {
				t.Errorf("%s at %s: decidable = %v, predicted %v (rule requires %s)",
					ruleID, profile, gotDecidable, wantDecidable, d.RequiredCapability)
			}
		}
	}
}

func TestF1DetectsRollbackAndIgnoresHonestRetest(t *testing.T) {
	t.Run("rollback fires at alert strength", func(t *testing.T) {
		v := &Vehicle{VID: "v1", Events: []Event{
			mileage(1, 0, 62_400, E2),
			mileage(2, 300, 78_900, E2),
			mileage(3, 600, 61_000, E2), // the thesis's worked example
		}}
		f := statusOf(t, evalOdometerMonotonic(ledgerOf(v), v), "F1a")
		if f.Strength != StrengthAlert {
			t.Errorf("strength = %s, want ALERT", f.Strength)
		}
	})

	t.Run("fail-and-retest does not fire the rate rule", func(t *testing.T) {
		// A failed emission test and its same-day retest: without collapsing the
		// retest group this reads as a huge km/day jump over a zero-day interval.
		v := &Vehicle{VID: "v1", Events: []Event{
			{Seq: 1, Type: EventMileage, Day: 0, Odometer: 50_000, Tier: E1, VETLaneID: "l1", RetestGroup: "g1"},
			{Seq: 2, Type: EventMileage, Day: 0, Odometer: 50_010, Tier: E1, VETLaneID: "l1", RetestGroup: "g1"},
			{Seq: 3, Type: EventMileage, Day: 365, Odometer: 62_000, Tier: E1, VETLaneID: "l1"},
		}}
		if got := evalOdometerRate(ledgerOf(v), v); len(got) != 0 {
			t.Errorf("rate rule fired on a legitimate retest sequence: %+v", got)
		}
	})

	t.Run("stagnation is flagged", func(t *testing.T) {
		v := &Vehicle{VID: "v1", Events: []Event{
			mileage(1, 0, 50_000, E1),
			mileage(2, 400, 50_000, E1),
		}}
		if got := evalOdometerStagnant(ledgerOf(v), v); len(got) != 1 {
			t.Errorf("expected one stagnation finding, got %d", len(got))
		}
	})
}

func TestF2FlagsTransferWhileStolenAndCurrentTheftStatus(t *testing.T) {
	tests := []struct {
		name         string
		events       []Event
		wantTransfer bool // a transfer committed while flagged
		wantCurrent  bool // vehicle currently reported stolen
	}{
		{"transfer while flagged stolen, still flagged", []Event{
			{Seq: 1, Type: EventTheftFlag, Tier: E2},
			{Seq: 2, Type: EventTransfer, Tier: E2},
		}, true, true},
		{"transfer after the flag was cleared", []Event{
			{Seq: 1, Type: EventTheftFlag, Tier: E2},
			{Seq: 2, Type: EventTheftClear, Tier: E2},
			{Seq: 3, Type: EventTransfer, Tier: E2},
		}, false, false},
		{"flag raised after the transfer committed", []Event{
			{Seq: 1, Type: EventTransfer, Tier: E2},
			{Seq: 2, Type: EventTheftFlag, Tier: E2},
		}, false, true},
		{"flagged then cleared, never transferred", []Event{
			{Seq: 1, Type: EventTheftFlag, Tier: E2},
			{Seq: 2, Type: EventTheftClear, Tier: E2},
		}, false, false},
		{"flagged, never transferred (chaincode refused the attempt)", []Event{
			{Seq: 1, Type: EventTheftFlag, Tier: E2},
		}, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &Vehicle{VID: "v1", Events: tc.events}
			var transfer, current bool
			for _, f := range evalTheftTransfer(ledgerOf(v), v) {
				if strings.Contains(f.Detail, "transfer attempted") {
					transfer = true
				}
				if strings.Contains(f.Detail, "currently reported stolen") {
					current = true
					if f.Strength != StrengthAlert || f.Subject != SubjectOwner {
						t.Errorf("current-theft finding must be an owner-attributed ALERT, got %s/%s", f.Strength, f.Subject)
					}
				}
			}
			if transfer != tc.wantTransfer || current != tc.wantCurrent {
				t.Errorf("transfer=%v current=%v, want %v/%v", transfer, current, tc.wantTransfer, tc.wantCurrent)
			}
		})
	}
}

func TestF3TwoTierEvidenceRule(t *testing.T) {
	t.Run("unanchored claim is an alert", func(t *testing.T) {
		v := &Vehicle{VID: "v1", Events: []Event{
			{Seq: 1, Type: EventClaim, Tier: E2, InsurerID: "ins-a"},
		}}
		f := statusOf(t, evalClaimUnanchored(ledgerOf(v), v), "F3a")
		if f.Strength != StrengthAlert {
			t.Errorf("strength = %s, want ALERT for a claim citing no evidence", f.Strength)
		}
	})

	t.Run("police-anchored claim passes cleanly", func(t *testing.T) {
		// The on-the-spot settlement case the July 2026 audit corrected for.
		v := &Vehicle{VID: "v1", Events: []Event{
			{Seq: 1, Type: EventAccident, Tier: E2, EvidenceAnchor: "tx-police-1"},
			{Seq: 2, Type: EventClaim, Tier: E2, EvidenceAnchor: "tx-police-1", InsurerID: "ins-a"},
		}}
		if got := evalClaimUnanchored(ledgerOf(v), v); len(got) != 0 {
			t.Errorf("police-anchored claim should not be flagged: %+v", got)
		}
	})

	t.Run("assessor-only claim is a warning, never an alert", func(t *testing.T) {
		v := &Vehicle{VID: "v1", Events: []Event{
			{Seq: 1, Type: EventClaim, Tier: E1, EvidenceAnchor: "tx-assessor-1", InsurerID: "ins-a"},
		}}
		f := statusOf(t, evalClaimUnanchored(ledgerOf(v), v), "F3a")
		if f.Strength == StrengthAlert {
			t.Error("a self-attested assessor anchor must not reach ALERT strength")
		}
		if f.SatisfiesRevealPrecondition() {
			t.Error("an assessor-only claim must not authorise an identity reveal")
		}
	})

	t.Run("unknown E2 evidence anchor is flagged", func(t *testing.T) {
		v := &Vehicle{VID: "v1", Events: []Event{
			{Seq: 1, Type: EventClaim, Tier: E2, EvidenceAnchor: "fabricated-police-record", InsurerID: "ins-a"},
		}}
		f := statusOf(t, evalClaimUnanchored(ledgerOf(v), v), "F3a")
		if f.Strength != StrengthAlert {
			t.Errorf("strength = %s, want ALERT for a fabricated E2 reference", f.Strength)
		}
	})

	t.Run("duplicate anchor fires", func(t *testing.T) {
		v := &Vehicle{VID: "v1", Events: []Event{
			{Seq: 1, Type: EventClaim, Tier: E2, EvidenceAnchor: "tx-1", InsurerID: "ins-a"},
			{Seq: 2, Type: EventClaim, Tier: E2, EvidenceAnchor: "tx-1", InsurerID: "ins-b"},
		}}
		if got := evalDuplicateAnchor(ledgerOf(v), v); len(got) != 1 {
			t.Errorf("expected one duplicate-anchor finding, got %d", len(got))
		}
	})

	t.Run("cross-insurer window fires without a shared anchor", func(t *testing.T) {
		v := &Vehicle{VID: "v1", Events: []Event{
			{Seq: 1, Type: EventClaim, Day: 10, Tier: E1, EvidenceAnchor: "a1", InsurerID: "ins-a"},
			{Seq: 2, Type: EventClaim, Day: 20, Tier: E1, EvidenceAnchor: "a2", InsurerID: "ins-b"},
		}}
		got := evalCrossInsurerWindow(ledgerOf(v), v)
		if len(got) != 1 {
			t.Fatalf("expected one cross-insurer finding, got %d", len(got))
		}
		if got[0].Strength == StrengthAlert {
			t.Error("cross-insurer window is a suspicion, not proof — must not be ALERT")
		}
	})
}

// F4 is the control row: its verdict must not move as linkage changes.
func TestF4IsFlatAcrossProfiles(t *testing.T) {
	donor := &Vehicle{VID: "v1", Events: []Event{
		{Seq: 1, Type: EventRegistration, Tier: E2, IdxEngine: "eng-shared"},
	}}
	clone := &Vehicle{VID: "v2", Events: []Event{
		{Seq: 2, Type: EventRegistration, Tier: E2, IdxEngine: "eng-shared"},
	}}
	l := &Ledger{Vehicles: map[string]*Vehicle{"v1": donor, "v2": clone}}

	d, _ := Lookup("F4")
	// The control must hold at every profile, P3 included: it reads keyed identity
	// indices, which survive even when all pseudonym linkage is stripped.
	for _, p := range []Profile{ProfileP0, ProfileP1, ProfileP2, ProfileP3} {
		got := Decide(d, l, clone, p)
		if got[0].Status != StatusFlagged {
			t.Errorf("F4 at %s: status = %s, want FLAGGED — the control row moved", p, got[0].Status)
		}
	}
	if got := Decide(d, l, donor, ProfileP1); got[0].Status != StatusPass {
		t.Errorf("the earlier registration should pass, got %s", got[0].Status)
	}
}

func TestF5RequiresPriorImportRecord(t *testing.T) {
	tests := []struct {
		name     string
		events   []Event
		wantFire bool
	}{
		{"customs record precedes registration", []Event{
			{Seq: 1, Type: EventImport, Tier: E2, IdxEngine: "eng-1"},
			{Seq: 2, Type: EventRegistration, Tier: E2, IdxEngine: "eng-1", Provenance: "CUSTOMS"},
		}, false},
		{"claimed customs provenance with no such record", []Event{
			{Seq: 1, Type: EventRegistration, Tier: E2, IdxEngine: "eng-1", Provenance: "CUSTOMS"},
		}, true},
		{"no provenance at all (the CIABOC case)", []Event{
			{Seq: 1, Type: EventRegistration, Tier: E2, IdxEngine: "eng-1", Provenance: "NONE"},
		}, true},
		{"import record committed after the registration cannot back it", []Event{
			{Seq: 1, Type: EventRegistration, Tier: E2, IdxEngine: "eng-1", Provenance: "CUSTOMS"},
			{Seq: 2, Type: EventImport, Tier: E2, IdxEngine: "eng-1"},
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &Vehicle{VID: "v1", Events: tc.events}
			got := evalProvenance(ledgerOf(v), v)
			if fired := len(got) > 0; fired != tc.wantFire {
				t.Errorf("fired = %v, want %v", fired, tc.wantFire)
			}
		})
	}
}

// F6 is the privacy cost made concrete: the same fraud is visible at P0 and
// invisible at P1, and the registry declares the difference rather than hiding it.
func TestF6VisibleAtP0AndSuppressedAtP1(t *testing.T) {
	rollback := func(vid string) *Vehicle {
		return &Vehicle{VID: vid, Events: []Event{
			{Seq: 1, Type: EventMileage, VID: vid, P: "owner-1", Day: 0, Odometer: 80_000, Tier: E2},
			{Seq: 2, Type: EventMileage, VID: vid, P: "owner-1", Day: 200, Odometer: 60_000, Tier: E2},
		}}
	}
	a, b := rollback("v1"), rollback("v2")
	l := &Ledger{
		Vehicles:      map[string]*Vehicle{"v1": a, "v2": b},
		OwnerVehicles: map[string][]string{"owner-1": {"v1", "v2"}},
	}

	d, _ := Lookup("F6a")

	atP0 := Decide(d, l, a, ProfileP0)
	if atP0[0].Status != StatusFlagged {
		t.Fatalf("serial rollback should be detected at P0, got %s", atP0[0].Status)
	}

	atP1 := Decide(d, l, a, ProfileP1)
	if atP1[0].Status != StatusSuppressed {
		t.Fatalf("serial rollback must be SUPPRESSED at P1, got %s", atP1[0].Status)
	}
	if atP1[0].Reason == "" {
		t.Error("the privacy cost must be declared to the buyer, not silently dropped")
	}
}
