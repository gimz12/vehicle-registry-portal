package experiment

import (
	"math/rand"
	"strconv"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

// Noise is the Corpus B robustness model (Table 3.2). Every rate is the
// per-vehicle (or per-event, where stated) probability of one honest-world
// imperfection. The parameters are frozen with the rules: changing them after
// the held-out run invalidates the manifest.
//
// Noise is applied only where the hidden label for the affected rule is
// negative, so every flag it induces is a true false positive; evidence-tier
// downgrades apply to every vehicle, so a positive that becomes undecidable is a
// true false negative. Labels are never changed.
type Noise struct {
	NearThresholdRate   float64 `json:"nearThresholdRate"`   // legitimate high mileage just under the F1b rate
	ClericalMileageRate float64 `json:"clericalMileageRate"` // two digits of the second reading transposed
	ClericalDateRate    float64 `json:"clericalDateRate"`    // second reading dated within a month of the first
	OutOfOrderRate      float64 `json:"outOfOrderRate"`      // later reading committed before the earlier one
	DuplicateRate       float64 `json:"duplicateRate"`       // claim resubmitted under a new record id
	MissingDocumentRate float64 `json:"missingDocumentRate"` // honest claim filed before the police report exists
	RepeatClaimRate     float64 `json:"repeatClaimRate"`     // genuine second accident and claim, other insurer
	CorruptedRefRate    float64 `json:"corruptedRefRate"`    // claim cites a mistyped report number
	E1Rate              float64 `json:"e1Rate"`              // per event: independently anchored -> attested only
	E0Rate              float64 `json:"e0Rate"`              // per vehicle: one record type is self-declared only
}

var DefaultNoise = Noise{
	NearThresholdRate:   0.05,
	ClericalMileageRate: 0.02,
	ClericalDateRate:    0.02,
	OutOfOrderRate:      0.02,
	DuplicateRate:       0.02,
	MissingDocumentRate: 0.05,
	RepeatClaimRate:     0.03,
	CorruptedRefRate:    0.02,
	E1Rate:              0.20,
	E0Rate:              0.03,
}

// applyNoise mutates a generated corpus in place. It draws from its own stream
// so the clean Corpus A event sequence for the same seed is byte-identical.
func applyNoise(c *Corpus, n Noise) {
	rng := rand.New(rand.NewSource(c.Config.Seed ^ 0x6e6f697365))
	labels := map[string]bool{}
	for _, label := range c.Labels {
		labels[label.SourceVID+":"+label.RuleID] = label.Positive
	}
	var commitOrder uint64
	for _, v := range c.Vehicles {
		for _, e := range v.Vehicle.Events {
			if e.CommitOrder > commitOrder {
				commitOrder = e.CommitOrder
			}
		}
	}

	for i := range c.Vehicles {
		v := &c.Vehicles[i].Vehicle
		negative := func(rule string) bool { positive, ok := labels[v.VID+":"+rule]; return ok && !positive }
		add := func(event rules.Event) {
			commitOrder++
			event.CommitOrder = commitOrder
			event.Seq = uint64(len(v.Events))
			event.VID = v.VID
			v.Events = append(v.Events, event)
		}
		ma, mb := -1, -1
		for idx, e := range v.Events {
			if e.Type == rules.EventMileage {
				if ma < 0 {
					ma = idx
				} else if mb < 0 {
					mb = idx
				}
			}
		}
		odometerClean := negative("F1a") && negative("F1b") && negative("F1c")

		// --- odometer ---
		if ma >= 0 && mb >= 0 && odometerClean {
			switch {
			case rng.Float64() < n.NearThresholdRate:
				days := v.Events[mb].Day - v.Events[ma].Day
				v.Events[mb].Odometer = v.Events[ma].Odometer + 999*days
			case rng.Float64() < n.ClericalMileageRate:
				v.Events[mb].Odometer = transposeDigits(rng, v.Events[mb].Odometer)
			case rng.Float64() < n.ClericalDateRate:
				v.Events[mb].Day = v.Events[ma].Day + int64(rng.Intn(30)+1)
			case rng.Float64() < n.OutOfOrderRate:
				seqA, seqB := v.Events[ma].Seq, v.Events[mb].Seq
				coA, coB := v.Events[ma].CommitOrder, v.Events[mb].CommitOrder
				v.Events[ma], v.Events[mb] = v.Events[mb], v.Events[ma]
				v.Events[ma].Seq, v.Events[mb].Seq = seqA, seqB
				v.Events[ma].CommitOrder, v.Events[mb].CommitOrder = coA, coB
			}
		}

		// --- claims ---
		claims := v.EventsOfType(rules.EventClaim)
		if len(claims) > 0 {
			first := claims[0]
			switch {
			case negative("F3c") && first.EvidenceAnchor != "" && rng.Float64() < n.DuplicateRate:
				dup := first
				dup.RecordID = "resubmit-" + first.RecordID
				dup.Day = first.Day + 1
				add(dup)
			case negative("F3a") && first.EvidenceAnchor != "" && rng.Float64() < n.MissingDocumentRate:
				setClaimAnchor(v, first.RecordID, "", "")
			case negative("F3a") && first.EvidenceAnchor != "" && rng.Float64() < n.CorruptedRefRate:
				setClaimAnchor(v, first.RecordID, "accident-"+digest("typo", v.VID)[:12], first.EvidenceKind)
			case negative("F3d") && negative("F3c") && rng.Float64() < n.RepeatClaimRate:
				anchor := "accident-c-" + v.VID[:12]
				add(rules.Event{RecordID: anchor, Type: rules.EventAccident, Day: first.Day + 12, Tier: rules.E2,
					EvidenceAnchor: anchor, EvidenceKind: "POLICE_REPORT", IncidentRef: rules.ReferencePresent})
				add(rules.Event{RecordID: "claim-c-" + v.VID[:12], Type: rules.EventClaim, Day: first.Day + 15, Tier: rules.E2,
					EvidenceAnchor: anchor, EvidenceKind: "POLICE_REPORT", IncidentRef: rules.ReferencePresent,
					InsurerID: "insurer-c", AttestorID: "assessor-c-" + v.VID[:8]})
			}
		}

		// --- evidence tiers (all vehicles) ---
		for idx := range v.Events {
			if v.Events[idx].Tier == rules.E2 && rng.Float64() < n.E1Rate {
				v.Events[idx].Tier = rules.E1
			}
		}
		if rng.Float64() < n.E0Rate {
			target := []rules.EventType{rules.EventMileage, rules.EventClaim, rules.EventTransfer}[rng.Intn(3)]
			for idx := range v.Events {
				if v.Events[idx].Type == target {
					v.Events[idx].Tier = rules.E0
				}
			}
		}
	}
}

func setClaimAnchor(v *rules.Vehicle, recordID, anchor, kind string) {
	for idx := range v.Events {
		if v.Events[idx].RecordID == recordID {
			v.Events[idx].EvidenceAnchor = anchor
			v.Events[idx].EvidenceKind = kind
			v.Events[idx].IncidentRef = refState(anchor)
		}
	}
}

// transposeDigits swaps two adjacent digits, the classic clerical keying error.
func transposeDigits(rng *rand.Rand, value int64) int64 {
	s := []byte(strconv.FormatInt(value, 10))
	if len(s) < 2 {
		return value
	}
	i := rng.Intn(len(s) - 1)
	s[i], s[i+1] = s[i+1], s[i]
	out, err := strconv.ParseInt(string(s), 10, 64)
	if err != nil {
		return value
	}
	return out
}
