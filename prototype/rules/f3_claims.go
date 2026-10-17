package rules

import "fmt"

// F3 — accident/insurance cross-reference (thesis Algorithm 5, two-tier evidence).
//
// The July 2026 capability audit revised check 1: Sri Lankan insurers routinely
// settle on-the-spot claims with no police report at all (police involvement is
// mandatory only for injury or third-party dispute), so requiring a police report
// as the sole valid anchor would flag every legitimate on-the-spot claim as fraud.
// A claim must therefore cite either a police report (E2, independent) or an
// insurer assessor attestation (E1, self-attested) — and the tier it rests on is
// carried into the verdict, so an assessor-anchored claim cannot by itself reach
// ALERT or authorise a reveal.
//
// F3 is intentionally not assigned one family-wide breakpoint. F3a-c need an
// explicit incident reference, while F3d only compares claims on one vehicle.

const claimWindowDays = 30

func init() {
	Register(Descriptor{
		ID:                 "F3a",
		Family:             "F3",
		Title:              "Insurance claim supported by accident evidence",
		RequiredCapability: CapabilityRecord,
		Antecedents:        []EventType{EventClaim},
		Eval:               evalClaimUnanchored,
	})
	Register(Descriptor{
		ID:                 "F3b",
		Family:             "F3",
		Title:              "Accident reported without a corresponding claim",
		RequiredCapability: CapabilityIncident,
		Antecedents:        []EventType{EventAccident},
		CanEvaluate:        canEvaluateAccidentReferences,
		Eval:               evalAccidentNoClaim,
	})
	Register(Descriptor{
		ID:                 "F3c",
		Family:             "F3",
		Title:              "Duplicate claims on one evidence anchor",
		RequiredCapability: CapabilityIncident,
		Antecedents:        []EventType{EventClaim},
		CanEvaluate:        canEvaluateClaimReferences,
		Eval:               evalDuplicateAnchor,
	})
	Register(Descriptor{
		ID:                 "F3d",
		Family:             "F3",
		Title:              "Cross-insurer claims in an overlapping window",
		RequiredCapability: CapabilityAsset,
		Antecedents:        []EventType{EventClaim},
		CanEvaluate:        canEvaluateClaimWindow,
		Eval:               evalCrossInsurerWindow,
	})
}

func canEvaluateClaimReferences(_ *Ledger, v *Vehicle) Availability {
	claims := v.EventsOfType(EventClaim)
	if len(claims) == 0 {
		return Availability{Reason: "no CLAIM record exists for this vehicle"}
	}
	for _, claim := range claims {
		if claim.ReferenceStatus() == ReferenceRedacted {
			return Availability{Reason: "incident references were removed by the active data view", Structural: true}
		}
	}
	if bestTier(claims) == E0 {
		return Availability{Reason: "CLAIM records are self-declared only (E0); no accountable attestation"}
	}
	return Availability{Available: true}
}

func canEvaluateAccidentReferences(_ *Ledger, v *Vehicle) Availability {
	if !v.HasCompleteAssetHistory() {
		return Availability{Reason: "claims and accidents cannot be joined because vehicle history was split", Structural: true}
	}
	accidents := v.EventsOfType(EventAccident)
	if len(accidents) == 0 {
		return Availability{Reason: "no ACCIDENT record exists for this vehicle"}
	}
	for _, claim := range v.EventsOfType(EventClaim) {
		if claim.ReferenceStatus() == ReferenceRedacted {
			return Availability{Reason: "claim-to-incident references were removed by the active data view", Structural: true}
		}
	}
	if bestTier(accidents) == E0 {
		return Availability{Reason: "ACCIDENT records are self-declared only (E0); no accountable attestation"}
	}
	return Availability{Available: true}
}

func canEvaluateClaimWindow(_ *Ledger, v *Vehicle) Availability {
	if !v.HasCompleteAssetHistory() {
		return Availability{Reason: "claims cannot be compared because vehicle history was split", Structural: true}
	}
	claims := v.EventsOfType(EventClaim)
	if len(claims) == 0 {
		return Availability{Reason: "no CLAIM record exists for this vehicle"}
	}
	if bestTier(claims) == E0 {
		return Availability{Reason: "CLAIM records are self-declared only (E0); no accountable attestation"}
	}
	return Availability{Available: true}
}

// evalClaimUnanchored is check 1: a claim citing neither evidence tier.
func evalClaimUnanchored(_ *Ledger, v *Vehicle) []Finding {
	anchors := map[string]Event{}
	for _, a := range v.EventsOfType(EventAccident) {
		if a.EvidenceAnchor != "" {
			anchors[a.EvidenceAnchor] = a
		}
	}

	var out []Finding
	for _, c := range v.EventsOfType(EventClaim) {
		if c.ReferenceStatus() == ReferenceRedacted {
			out = append(out, Finding{
				RuleID: "F3a", VID: v.VID, Status: StatusGapNotice, Tier: E0,
				AtSeq: c.Seq, Subject: SubjectSystem,
				Detail:     "Insurance claim supported by accident evidence",
				Reason:     "the claim's incident reference was removed by the active data view",
				Structural: true,
			})
			continue
		}
		if c.EvidenceAnchor == "" {
			out = append(out, Finding{
				RuleID:   "F3a",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: StrengthFor(StrengthAlert, c),
				Tier:     c.Tier,
				AtSeq:    c.Seq,
				Subject:  SubjectOwner,
				Detail:   "claim cites neither a police accident report nor an assessor scene attestation",
			})
			continue
		}
		if anchor, ok := anchors[c.EvidenceAnchor]; ok && anchor.Seq < c.Seq {
			// Anchored to a police report: strongest evidence, nothing to report.
			_ = anchor
			continue
		}
		// Anchored to an assessor attestation only (self-attested, E1).
		if c.Tier <= E1 {
			out = append(out, Finding{
				RuleID:   "F3a",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: StrengthFor(StrengthWarning, c),
				Tier:     c.Tier,
				AtSeq:    c.Seq,
				Subject:  SubjectOwner,
				Detail:   "claim rests on an insurer assessor attestation only; no independent police report",
			})
			continue
		}
		out = append(out, Finding{
			RuleID: "F3a", VID: v.VID, Status: StatusFlagged,
			Strength: StrengthFor(StrengthAlert, c), Tier: c.Tier, AtSeq: c.Seq,
			Subject: SubjectOwner,
			Detail:  "claim cites an independent evidence record that does not exist or did not precede the claim",
		})
	}
	return out
}

// evalAccidentNoClaim is check 2: information for the buyer, not a fraud signal.
func evalAccidentNoClaim(_ *Ledger, v *Vehicle) []Finding {
	claims := v.EventsOfType(EventClaim)
	claimed := map[string]bool{}
	for _, c := range claims {
		claimed[c.EvidenceAnchor] = true
	}

	var out []Finding
	for _, a := range v.EventsOfType(EventAccident) {
		if !claimed[a.EvidenceAnchor] {
			out = append(out, Finding{
				RuleID:   "F3b",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: StrengthInfo,
				Tier:     a.Tier,
				AtSeq:    a.Seq,
				Subject:  SubjectSystem,
				Detail:   "an accident was reported to police with no insurance claim filed — repair history may be off-record",
			})
		}
	}
	return out
}

// evalDuplicateAnchor is check 3: deterministic only where both claims share an
// anchor, which is why the window rule below exists at warning strength.
func evalDuplicateAnchor(_ *Ledger, v *Vehicle) []Finding {
	seen := map[string]Event{}
	var out []Finding
	for _, c := range v.EventsOfType(EventClaim) {
		if c.EvidenceAnchor == "" {
			continue
		}
		if first, ok := seen[c.EvidenceAnchor]; ok {
			out = append(out, Finding{
				RuleID:   "F3c",
				VID:      v.VID,
				Status:   StatusFlagged,
				Strength: StrengthFor(StrengthAlert, first, c),
				Tier:     MinTier(first, c),
				AtSeq:    c.Seq,
				Subject:  SubjectOwner,
				Detail: fmt.Sprintf("a second claim cites the same evidence anchor as the claim at block %d",
					first.Seq),
			})
			continue
		}
		seen[c.EvidenceAnchor] = c
	}
	return out
}

// evalCrossInsurerWindow is check 4: different insurers, same vehicle, overlapping
// dates, no shared anchor. Warning strength — this is a suspicion, not a proof.
func evalCrossInsurerWindow(_ *Ledger, v *Vehicle) []Finding {
	claims := v.EventsOfType(EventClaim)
	var out []Finding
	for i := 0; i < len(claims); i++ {
		for j := i + 1; j < len(claims); j++ {
			a, b := claims[i], claims[j]
			if a.InsurerID == b.InsurerID || a.InsurerID == "" || b.InsurerID == "" {
				continue
			}
			if a.EvidenceAnchor != "" && a.EvidenceAnchor == b.EvidenceAnchor {
				continue // F3c already owns this pair
			}
			if abs64(b.Day-a.Day) <= claimWindowDays {
				out = append(out, Finding{
					RuleID:   "F3d",
					VID:      v.VID,
					Status:   StatusFlagged,
					Strength: StrengthFor(StrengthWarning, a, b),
					Tier:     MinTier(a, b),
					AtSeq:    b.Seq,
					Subject:  SubjectOwner,
					Detail: fmt.Sprintf("claims filed with two different insurers %d days apart with no shared evidence anchor",
						abs64(b.Day-a.Day)),
				})
			}
		}
	}
	return out
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
