// Package rules holds the registry's fraud-detection logic as pure functions over
// an event stream. It is compiled twice: into the Fabric chaincode, and into the
// off-chain sweep harness. Both targets must produce identical verdicts, which the
// parity test asserts — so nothing here may depend on chaincode state, wall-clock
// time, or randomness.
package rules

// Scope is the pseudonym linkage a rule needs in order to be decidable at all.
//
// The lattice is ordered by how much linkage the rule demands, weakest first:
//
//	NONE        no pseudonym linkage at all — the rule works off the deterministic
//	            identity index (HMAC of chassis/engine), which every event carries
//	            regardless of how pseudonyms are scoped
//	ASSET       events of one vehicle must link to each other
//	ASSET_EVENT ...and an event must link to its own specific antecedent
//	OWNER       ...and one person's several vehicles must link to each other
//
// A rule declares the linkage it requires; the active profile declares what the
// registry exposes. Because the ordering is by demand, a rule requiring less
// linkage survives to a more private profile — which is why the measured
// collapse is a staircase rather than a single cliff.
type Capability string

const (
	CapabilityIndex    Capability = "INDEX"
	CapabilityRecord   Capability = "RECORD"
	CapabilityAsset    Capability = "ASSET"
	CapabilityIncident Capability = "INCIDENT"
	CapabilityOwner    Capability = "OWNER"
)

// Profile is an operating point of the linkage dial — the independent variable of
// the Chapter 6 sweep. P1 is the design point the chaincode runs at.
type Profile string

const (
	ProfileP0 Profile = "P0" // one pseudonym per owner across every vehicle (full linkage; the NMVTIS-equivalent control)
	ProfileP1 Profile = "P1" // one pseudonym per (owner, vehicle) — this thesis's design
	ProfileP2 Profile = "P2" // one pseudonym per event within a vehicle
	ProfileP3 Profile = "P3" // per event, with the vehicle link itself hidden
)

// AvailableCapabilities reports which joins a profile exposes to the rule engine.
// A rule is evaluable only if its required capability appears here.
//
// Each profile removes one capability. INDEX remains at P3: this is not "no
// linkage"; it is the deliberately retained keyed join for identity/provenance
// records that keeps the control rows flat.
func (p Profile) AvailableCapabilities() []Capability {
	switch p {
	case ProfileP0:
		return []Capability{CapabilityRecord, CapabilityIndex, CapabilityAsset, CapabilityIncident, CapabilityOwner}
	case ProfileP1:
		return []Capability{CapabilityRecord, CapabilityIndex, CapabilityAsset, CapabilityIncident}
	case ProfileP2:
		return []Capability{CapabilityRecord, CapabilityIndex, CapabilityAsset}
	case ProfileP3:
		return []Capability{CapabilityRecord, CapabilityIndex}
	}
	return nil
}

// Exposes reports whether the profile makes capability c available.
func (p Profile) Exposes(c Capability) bool {
	for _, available := range p.AvailableCapabilities() {
		if available == c {
			return true
		}
	}
	return false
}

// Tier grades the evidence an event rests on (Algorithm 8, three-tier form).
// Tiers propagate to a verdict by minimum: a conclusion is only as strong as its
// weakest antecedent.
type Tier int

const (
	// E0 — self-declared, legacy-migrated, or absent. Never supports an alert.
	E0 Tier = iota
	// E1 — attested by one accountable MSP identity (VET lane, insurer assessor, DMT clerk).
	E1
	// E2 — two independent MSPs, or cross-checked against a documentary anchor
	// (Customs declaration, police report).
	E2
)

func (t Tier) String() string {
	switch t {
	case E2:
		return "E2"
	case E1:
		return "E1"
	default:
		return "E0"
	}
}

// Strength is the severity of a verdict. Only ALERT satisfies the reveal and
// owner-scope escalation preconditions.
type Strength string

const (
	StrengthInfo    Strength = "INFO"
	StrengthWarning Strength = "WARNING"
	StrengthAlert   Strength = "ALERT"
)

// Status is the outcome of asking a rule about a vehicle. The three non-evaluated
// statuses are the thesis's declared-suppression contribution: the registry states
// what it did not check instead of returning a clean-looking record.
type Status string

const (
	StatusPass       Status = "PASS"       // rule evaluated, nothing wrong found
	StatusFlagged    Status = "FLAGGED"    // rule evaluated and fired
	StatusSuppressed Status = "SUPPRESSED" // undecidable under the active linkage profile
	StatusGapNotice  Status = "GAP_NOTICE" // antecedent evidence absent or self-declared only
)

// EventType enumerates the seven lifecycle events plus the Customs import record
// that first registration is checked against.
type EventType string

const (
	EventImport       EventType = "IMPORT"       // Customs declaration / local-assembler attestation
	EventRegistration EventType = "REGISTRATION" // DMT first registration
	EventTransfer     EventType = "TRANSFER"     // DMT ownership transfer
	EventMileage      EventType = "MILEAGE"      // odometer reading at a VET lane
	EventAccident     EventType = "ACCIDENT"     // Police accident report
	EventClaim        EventType = "CLAIM"        // insurer claim
	EventEmission     EventType = "EMISSION"     // VET certificate
	EventTheftFlag    EventType = "THEFT_FLAG"   // Police theft flag set
	EventTheftClear   EventType = "THEFT_CLEAR"  // Police theft flag cleared
)

// Event is one lifecycle record as committed to the ledger.
//
// Ordering is by Seq — the block commit sequence — never by a client-set
// timestamp, which Fabric does not validate. Day exists only for the arithmetic
// the odometer rules need (km/day, stagnation windows) and is a submitted value,
// so it is never used to order two events.
type Event struct {
	Seq         uint64 // chaincode-assigned logical order for this vehicle
	CommitOrder uint64 // block/transaction order assigned by the committed-data adapter
	RecordID    string // immutable transaction-local record identifier
	Type        EventType
	VID         string // asset handle: HMAC(k_v, chassis)
	P           string // owner pseudonym at this event, scoped by the active profile

	Day  int64 // days since epoch, submitted; for interval arithmetic only
	Tier Tier  // evidence grade of this event

	AttestorID string // organisational actor: VET lane, assessor, DMT office

	// Mileage
	Odometer    int64
	VETLaneID   string
	RetestGroup string // fail-and-retest sequences collapse to one reading

	// Claim / accident
	EvidenceAnchor string         // record ID of the police report or assessor attestation a claim cites
	EvidenceKind   string         // POLICE_REPORT or ASSESSOR_ATTESTATION
	IncidentRef    ReferenceState // source absence and privacy redaction are different facts
	InsurerID      string
	ClaimType      string

	// Identity / provenance
	IdxChassis string // HMAC(k_v, canonical chassis)
	IdxEngine  string // HMAC(k_v, make||engineSerial)
	Provenance string // CUSTOMS | ASSEMBLER | NONE
}

// Order returns the cross-record commit order where available. Hand-written
// unit fixtures fall back to Seq; production/off-chain indexed events populate
// CommitOrder from block number and transaction position.
func (e Event) Order() uint64 {
	if e.CommitOrder != 0 {
		return e.CommitOrder
	}
	return e.Seq
}

type ReferenceState string

const (
	ReferencePresent  ReferenceState = "PRESENT"
	ReferenceMissing  ReferenceState = "MISSING_AT_SOURCE"
	ReferenceRedacted ReferenceState = "REDACTED_BY_PROFILE"
)

// ReferenceStatus preserves the meaning of older fixtures while allowing a
// transformed privacy view to declare that a source reference was redacted.
func (e Event) ReferenceStatus() ReferenceState {
	if e.IncidentRef != "" {
		return e.IncidentRef
	}
	if e.EvidenceAnchor != "" {
		return ReferencePresent
	}
	return ReferenceMissing
}

// RedactIncidentReference applies the INCIDENT-capability withdrawal to one
// claim: the anchor is removed and the reference marked as redacted by the
// view, which rules must treat as a gap, never as a missing-at-source claim.
// Shared by the experiment transformer and the chaincode history view.
func RedactIncidentReference(event *Event) {
	if event.Type != EventClaim || event.ReferenceStatus() != ReferencePresent {
		return
	}
	event.EvidenceAnchor = ""
	event.EvidenceKind = ""
	event.IncidentRef = ReferenceRedacted
}

// Vehicle is one asset's committed history, in block-commit order.
type Vehicle struct {
	VID       string
	Events    []Event
	AssetLink ReferenceState // REDACTED when sibling events may exist but cannot be joined

	Stolen     bool // latest theft flag state
	WrittenOff bool
	Retired    bool
}

func (v Vehicle) HasCompleteAssetHistory() bool {
	return v.AssetLink != ReferenceRedacted
}

// EventsOfType returns this vehicle's events of the given type, in commit order.
func (v Vehicle) EventsOfType(t EventType) []Event {
	var out []Event
	for _, e := range v.Events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

// Has reports whether any event of the given type exists for this vehicle.
func (v Vehicle) Has(t EventType) bool {
	for _, e := range v.Events {
		if e.Type == t {
			return true
		}
	}
	return false
}

// Ledger is the corpus a rule sees: every vehicle, keyed by asset handle, plus the
// owner-scope index. OwnerVehicles is populated only when the active profile
// exposes CapabilityOwner — at P1 and below it is empty by construction, which is what
// makes the owner-pattern family undecidable there rather than merely unlucky.
type Ledger struct {
	Vehicles      map[string]*Vehicle
	OwnerVehicles map[string][]string // owner pseudonym -> asset handles
	OwnerLink     ReferenceState      // REDACTED when cross-vehicle owner joins are hidden
}

// Finding is one rule's answer about one vehicle. Suppressed and gap-notice
// findings carry the reason so the buyer-facing portal can render "checks not
// performed" rather than silently omitting them.
type Finding struct {
	RuleID   string
	VID      string
	Status   Status
	Strength Strength
	Tier     Tier   // min-propagated tier of the antecedents this rests on
	Detail   string // human-readable, rendered to the buyer
	Reason   string
	// Structural is set on a GAP_NOTICE caused by the active data view, as
	// opposed to one caused by missing or self-declared evidence.
	Structural bool    // why suppressed / what evidence is missing
	AtSeq      uint64  // commit sequence of the event that triggered it
	Subject    Subject // party the finding concerns; controls reveal eligibility
}

// Subject identifies who a finding concerns. Only OWNER findings may support a
// governed identity reveal; evidence about a submitter or a system/data gap must
// never expose an owner.
type Subject string

const (
	SubjectOwner     Subject = "OWNER"
	SubjectSubmitter Subject = "SUBMITTER"
	SubjectSystem    Subject = "SYSTEM"
)

// Flagged reports whether the finding is an actual detection.
func (f Finding) Flagged() bool { return f.Status == StatusFlagged }

// SatisfiesRevealPrecondition reports whether this finding may serve as the
// machine-checked trigger for an identity reveal or owner-scope escalation.
//
// Two gates, both deliberate. A finding must be ALERT strength (Algorithm 3',
// step 3), and it must not be a submitter-side measurement flag: a VET tester's
// negligence must never become the key that de-anonymises an innocent owner.
func (f Finding) SatisfiesRevealPrecondition() bool {
	if !f.Flagged() || f.Strength != StrengthAlert {
		return false
	}
	return f.Subject == SubjectOwner
}

// MinTier returns the weakest tier among the given events — the tier a verdict
// resting on all of them inherits.
func MinTier(events ...Event) Tier {
	if len(events) == 0 {
		return E0
	}
	min := E2
	for _, e := range events {
		if e.Tier < min {
			min = e.Tier
		}
	}
	return min
}

// StrengthFor maps a proposed severity down to what its evidence can support:
// an ALERT requires every antecedent at E1 or better and at least one at E2.
// Anything resting on an E0 antecedent cannot exceed INFO.
func StrengthFor(proposed Strength, events ...Event) Strength {
	if len(events) == 0 {
		return StrengthInfo
	}
	min := MinTier(events...)
	if min == E0 {
		return StrengthInfo
	}
	if proposed != StrengthAlert {
		return proposed
	}
	for _, e := range events {
		if e.Tier == E2 {
			return StrengthAlert
		}
	}
	return StrengthWarning
}
