package registry

import (
	"errors"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

const (
	MSPDMT     = "DMTMSP"
	MSPPolice  = "PoliceMSP"
	MSPVET     = "VETMSP"
	MSPCustoms = "CustomsMSP"
	MSPInsurer = "InsurerMSP"
	// MSPAssembler's build attestation stands in for Customs clearance, so it
	// must not come from the registering DMT.
	MSPAssembler = "AssemblerMSP"
)

var ErrNotFound = errors.New("registry: record not found")

// Encumbrance events (DMT only): a finance company recorded as absolute owner
// blocks transfer until its deletion letter is lodged.
const (
	EventEncumbranceSet      rules.EventType = "ENCUMBRANCE_SET"
	EventEncumbranceReleased rules.EventType = "ENCUMBRANCE_RELEASED"
)

// Theft reporting: THEFT_REPORT (Police), then OWNER_CONFIRMED (DMT) under the
// same report number, before a THEFT_FLAG is accepted. The rule package ignores
// both types, so a report alone neither raises F2 nor blocks a transfer.
const (
	EventTheftReport    rules.EventType = "THEFT_REPORT"
	EventOwnerConfirmed rules.EventType = "OWNER_CONFIRMED"
)

type ImportAttestation struct {
	RecordID    string     `json:"recordId"`
	IdxChassis  string     `json:"idxChassis"`
	IdxEngine   string     `json:"idxEngine"`
	Source      string     `json:"source"`
	Tier        rules.Tier `json:"tier"`
	AttestorMSP string     `json:"attestorMsp"`
	// DescAnchor is a keyed HMAC of the declared description; when present the
	// registration must carry the same value.
	DescAnchor string `json:"descAnchor,omitempty" metadata:",optional"`
}

type RegistrationInput struct {
	VID            string `json:"vid"`
	Pseudonym      string `json:"pseudonym"`
	IdxChassis     string `json:"idxChassis"`
	IdxEngine      string `json:"idxEngine"`
	Provenance     string `json:"provenance"`
	EnvelopeAnchor string `json:"envelopeAnchor"`
	DescAnchor     string `json:"descAnchor,omitempty" metadata:",optional"`
}

type EventInput struct {
	RecordID       string          `json:"recordId"`
	VID            string          `json:"vid"`
	Type           rules.EventType `json:"type"`
	Pseudonym      string          `json:"pseudonym"`
	Day            int64           `json:"day"`
	Odometer       int64           `json:"odometer,omitempty"`
	VETLaneID      string          `json:"vetLaneId,omitempty"`
	RetestGroup    string          `json:"retestGroup,omitempty"`
	EvidenceAnchor string          `json:"evidenceAnchor,omitempty"`
	EvidenceKind   string          `json:"evidenceKind,omitempty"`
	InsurerID      string          `json:"insurerId,omitempty"`
	ClaimType      string          `json:"claimType,omitempty"`
	AttestorID     string          `json:"attestorId,omitempty"`
	// EnvelopeAnchor is required on a TRANSFER and replaces the previous owner's
	// anchor, so a later reveal opens the current owner.
	EnvelopeAnchor string `json:"envelopeAnchor,omitempty"`
	// Reference: endorsement or deletion-letter number on encumbrance events;
	// Police report number (or approved case id) on theft events.
	Reference string `json:"reference,omitempty"`
}

type VehicleRecord struct {
	Vehicle        rules.Vehicle `json:"vehicle"`
	EnvelopeAnchor string        `json:"envelopeAnchor"`
	DescAnchor     string        `json:"descAnchor,omitempty" metadata:",optional"`
}

type RevealStatus string

const (
	RevealPending  RevealStatus = "PENDING"
	RevealApproved RevealStatus = "APPROVED"
	RevealRejected RevealStatus = "REJECTED"
)

type RevealRequest struct {
	ID             string       `json:"id"`
	VID            string       `json:"vid"`
	RuleID         string       `json:"ruleId"`
	GroundCode     string       `json:"groundCode"`
	GroundRef      string       `json:"groundRef"` // case number, court order number or consent record id
	RequestedBy    string       `json:"requestedBy"`
	DMTApproved    bool         `json:"dmtApproved"`
	PoliceApproved bool         `json:"policeApproved"`
	Status         RevealStatus `json:"status"`
	Reason         string       `json:"reason,omitempty" metadata:",optional"`
	// AccessLog records every owner-scope evaluation under this case.
	AccessLog []string `json:"accessLog,omitempty" metadata:",optional"`
	// Identity hashes, never certificate subjects; used to refuse self-approval.
	RequesterIDHash      string `json:"requesterIdHash,omitempty" metadata:",optional"`
	DMTApproverIDHash    string `json:"dmtApproverIdHash,omitempty" metadata:",optional"`
	PoliceApproverIDHash string `json:"policeApproverIdHash,omitempty" metadata:",optional"`
	// FleetHash commits to the owner's fleet as the DMT asserted it; the fleet
	// itself never reaches the ledger.
	FleetHash string `json:"fleetHash,omitempty" metadata:",optional"`
	FleetSize int    `json:"fleetSize,omitempty" metadata:",optional"`
	// PredicateFleetHash: for F6a/F6b cases the approving DMT must attest the
	// same fleet.
	PredicateFleetHash string `json:"predicateFleetHash,omitempty" metadata:",optional"`
	// OwnerAnchor binds the case to the owner record at approval; a reveal after
	// a transfer is refused.
	OwnerAnchor string `json:"ownerAnchor,omitempty" metadata:",optional"`
}

// PredicateRecord is the stored verdict of an owner-scope rule over a fleet,
// without detail or reason; the fleet appears only as its hash.
type PredicateRecord struct {
	VID             string        `json:"vid"`
	RuleID          string        `json:"ruleId"`
	Finding         rules.Finding `json:"finding"`
	FleetHash       string        `json:"fleetHash"`
	RequesterIDHash string        `json:"requesterIdHash"`
	TxID            string        `json:"txId"`
}

// PredicateResult is all the caller learns: the verdict, not the fleet.
type PredicateResult struct {
	VID      string         `json:"vid"`
	RuleID   string         `json:"ruleId"`
	Status   rules.Status   `json:"status"`
	Strength rules.Strength `json:"strength"`
}

// Purpose is who is asking and why; policy maps it to the minimum linkage
// profile. Callers never choose a profile.
type Purpose string

const (
	PurposeBuyer   Purpose = "BUYER"   // pre-purchase derived disclosure
	PurposeDMT     Purpose = "DMT"     // vehicle history for a transaction
	PurposeInsurer Purpose = "INSURER" // P1 for a vehicle on which the insurer holds any claim
	PurposePolice  Purpose = "POLICE"  // investigation; OWNER only under an approved case
)

// Policy is the resolved decision, returned so the portal can show why.
type Policy struct {
	Purpose Purpose       `json:"purpose"`
	Profile rules.Profile `json:"profile"`
	Reason  string        `json:"reason"`
	CaseID  string        `json:"caseId,omitempty" metadata:",optional"`
}

// VehicleStatus is the register state every purpose may see (asset facts only).
type VehicleStatus struct {
	Stolen     bool `json:"stolen"`
	WrittenOff bool `json:"writtenOff"`
	Retired    bool `json:"retired"`
	// Encumbered never names the lender.
	Encumbered bool `json:"encumbered"`
	// TheftReportPending does not block transfers.
	TheftReportPending bool `json:"theftReportPending,omitempty" metadata:",optional"`
	// TheftCleared: flagged stolen earlier, flag since cleared.
	TheftCleared bool `json:"theftCleared,omitempty" metadata:",optional"`
	// DescCommitted: the record carries the description committed at import.
	DescCommitted bool `json:"descCommitted,omitempty" metadata:",optional"`
}

// PurposeDisclosure is a disclosure with its policy and register status.
type PurposeDisclosure struct {
	rules.Disclosure
	Policy Policy        `json:"Policy"`
	Status VehicleStatus `json:"Status"`
}

type Repository interface {
	GetVehicle(vid string) (*VehicleRecord, error)
	PutVehicle(record *VehicleRecord) error
	ListVehicles() ([]*VehicleRecord, error)
	GetImport(idxChassis string) (*ImportAttestation, error)
	PutImport(attestation *ImportAttestation) error
	GetIdentityOwner(kind, digest string) (string, error)
	PutIdentityOwner(kind, digest, vid string) error
	GetReveal(id string) (*RevealRequest, error)
	PutReveal(request *RevealRequest) error
	GetPredicate(vid, ruleID string) (*PredicateRecord, error)
	PutPredicate(record *PredicateRecord) error
}
