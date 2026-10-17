package rules

// F5 — provenance / prerequisite registration (candidate rule R1).
//
// Targets the CIABOC finding of ~400 vehicles fraudulently entered into the DMT
// database without Customs clearance. A first registration must match either a
// Customs-signed import record or a registered local assembler's build
// attestation; the two-path form is required because locally assembled vehicles
// clear Customs as parts, never per chassis.
//
// Required scope is NONE. The check matches a registration against a prior import
// record through the keyed engine index, which is independent of how pseudonyms
// are scoped, so the rule survives all the way to P3 — the analytical prediction
// this implementation must satisfy.
//
// Honest limitation, carried into Chapter 8: this attributes and prevents a
// single-organisation fraud. A DMT insider colluding with a Customs insider can
// still produce a well-formed prerequisite record, and no rule here detects that.

func init() {
	Register(Descriptor{
		ID:                 "F5",
		Family:             "F5",
		Title:              "Customs or assembler provenance for first registration",
		RequiredCapability: CapabilityIndex,
		Antecedents:        []EventType{EventRegistration},
		Eval:               evalProvenance,
	})
}

func evalProvenance(l *Ledger, v *Vehicle) []Finding {
	regs := v.EventsOfType(EventRegistration)
	if len(regs) == 0 {
		return nil
	}
	reg := regs[0]

	switch reg.Provenance {
	case "CUSTOMS", "ASSEMBLER":
		// Search the whole ledger by keyed engine index rather than within this
		// vehicle's own history: the match is what makes the rule independent of
		// pseudonym linkage, and hence decidable at every profile.
		//
		// Ordering is by commit sequence, not by the client-set date, so a
		// backdated import record cannot retrospectively legitimise a
		// registration that was committed before it.
		if importPrecedes(l, reg) {
			return nil
		}
		return []Finding{{
			RuleID:   "F5",
			VID:      v.VID,
			Status:   StatusFlagged,
			Strength: StrengthFor(StrengthAlert, reg),
			Tier:     reg.Tier,
			AtSeq:    reg.Seq,
			Subject:  SubjectSubmitter,
			Detail:   "registration declares " + reg.Provenance + " provenance, but no matching prior import record exists on the ledger",
		}}
	}

	return []Finding{{
		RuleID:   "F5",
		VID:      v.VID,
		Status:   StatusFlagged,
		Strength: StrengthFor(StrengthAlert, reg),
		Tier:     reg.Tier,
		AtSeq:    reg.Seq,
		Subject:  SubjectSubmitter,
		Detail:   "registered with no Customs import record and no local-assembler attestation",
	}}
}

// importPrecedes reports whether any import record anywhere on the ledger carries
// the registration's engine index and was committed before it.
func importPrecedes(l *Ledger, reg Event) bool {
	if reg.IdxEngine == "" || l == nil {
		return false
	}
	for _, other := range l.Vehicles {
		for _, imp := range other.EventsOfType(EventImport) {
			if imp.Order() < reg.Order() && imp.IdxEngine == reg.IdxEngine {
				return true
			}
		}
	}
	return false
}
