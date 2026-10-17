package rules

import "fmt"

// F2 — stolen-vehicle check (thesis Algorithm 2, step 2).
//
// Two findings, both needing ASSET scope and block commit ordering:
//
//   - a TRANSFER committed while the theft flag stood (the harness case: the
//     centralized baseline and the parity corpus contain such histories);
//   - the vehicle is currently flagged stolen with no later clearance. In the
//     chaincode a transfer is refused at write and therefore never committed, so
//     without this finding a stolen vehicle would read "no evidence" to a buyer.
//     Corrected 3 Oct 2026 after the live walkthrough; re-frozen the same day.

func init() {
	Register(Descriptor{
		ID:                 "F2",
		Family:             "F2",
		Title:              "Stolen-vehicle check at ownership transfer",
		RequiredCapability: CapabilityAsset,
		Antecedents:        []EventType{EventTransfer, EventTheftFlag},
		CanEvaluate:        canEvaluateTransferHistory,
		Eval:               evalTheftTransfer,
	})
}

func canEvaluateTransferHistory(_ *Ledger, v *Vehicle) Availability {
	if !v.HasCompleteAssetHistory() {
		return Availability{Reason: "theft flags and transfers cannot be joined in this data view"}
	}
	transfers := v.EventsOfType(EventTransfer)
	flags := v.EventsOfType(EventTheftFlag)
	if len(transfers) == 0 && len(flags) == 0 {
		return Availability{Reason: "no TRANSFER or theft record exists for this vehicle"}
	}
	if bestTier(append(append([]Event(nil), transfers...), flags...)) == E0 {
		return Availability{Reason: "transfer and theft records are self-declared only (E0); no accountable attestation"}
	}
	return Availability{Available: true}
}

func evalTheftTransfer(_ *Ledger, v *Vehicle) []Finding {
	var out []Finding
	stolen := false
	var flagEvent Event

	// Single pass in commit order — the flag's state at the moment of each transfer
	// is what matters for the transfer finding; the flag's state at the end of the
	// history is what matters for the buyer.
	for _, e := range v.Events {
		switch e.Type {
		case EventTheftFlag:
			stolen, flagEvent = true, e
		case EventTheftClear:
			stolen = false
		case EventTransfer:
			if stolen {
				out = append(out, Finding{
					RuleID:   "F2",
					VID:      v.VID,
					Status:   StatusFlagged,
					Strength: StrengthFor(StrengthAlert, flagEvent, e),
					Tier:     MinTier(flagEvent, e),
					AtSeq:    e.Seq,
					Subject:  SubjectOwner,
					Detail: fmt.Sprintf("ownership transfer attempted while the vehicle was flagged stolen (flag at block %d)",
						flagEvent.Seq),
				})
			}
		}
	}
	if stolen {
		out = append(out, Finding{
			RuleID:   "F2",
			VID:      v.VID,
			Status:   StatusFlagged,
			Strength: StrengthFor(StrengthAlert, flagEvent),
			Tier:     flagEvent.Tier,
			AtSeq:    flagEvent.Seq,
			Subject:  SubjectOwner,
			Detail:   fmt.Sprintf("vehicle is currently reported stolen (flag at block %d); ownership transfers are refused", flagEvent.Seq),
		})
	}
	return out
}
