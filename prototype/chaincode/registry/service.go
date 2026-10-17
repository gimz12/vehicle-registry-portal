package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

type Service struct {
	Repo Repository
}

func NewService(repo Repository) *Service { return &Service{Repo: repo} }

func canonicalMSP(msp string) string {
	switch msp {
	case "Org1MSP":
		return MSPDMT
	case "Org2MSP":
		return MSPPolice
	case "Org3MSP":
		return MSPCustoms
	case "Org4MSP":
		return MSPVET
	case "Org5MSP":
		return MSPInsurer
	default:
		return msp
	}
}

func requireMSP(actual string, allowed ...string) error {
	actual = canonicalMSP(actual)
	for _, want := range allowed {
		if actual == want {
			return nil
		}
	}
	return fmt.Errorf("registry: MSP %s is not authorised; requires one of %v", actual, allowed)
}

// validDigest accepts only the canonical lowercase form: indices are compared
// as strings, so "ABC…" and "abc…" would otherwise be two identities.
func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func requireDigest(name, value string) error {
	if !validDigest(value) {
		return fmt.Errorf("registry: %s must be a 32-byte lowercase hexadecimal digest", name)
	}
	return nil
}

func (s *Service) CreateImportAttestation(actor string, input ImportAttestation) (*ImportAttestation, error) {
	input.Source = strings.ToUpper(strings.TrimSpace(input.Source))
	switch input.Source {
	case "CUSTOMS":
		if err := requireMSP(actor, MSPCustoms); err != nil {
			return nil, err
		}
	case "ASSEMBLER":
		if err := requireMSP(actor, MSPAssembler); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("registry: provenance source must be CUSTOMS or ASSEMBLER")
	}
	if strings.TrimSpace(input.RecordID) == "" {
		return nil, fmt.Errorf("registry: import recordId is required")
	}
	if err := requireDigest("idxChassis", input.IdxChassis); err != nil {
		return nil, err
	}
	if err := requireDigest("idxEngine", input.IdxEngine); err != nil {
		return nil, err
	}
	if input.DescAnchor != "" {
		if err := requireDigest("descAnchor", input.DescAnchor); err != nil {
			return nil, err
		}
	}
	if existing, err := s.Repo.GetImport(input.IdxChassis); err == nil && existing != nil {
		return nil, fmt.Errorf("registry: chassis already has an import attestation")
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	input.Tier = rules.E2
	input.AttestorMSP = canonicalMSP(actor)
	if err := s.Repo.PutImport(&input); err != nil {
		return nil, err
	}
	return &input, nil
}

func (s *Service) RegisterVehicle(actor string, input RegistrationInput) (*VehicleRecord, error) {
	if err := requireMSP(actor, MSPDMT); err != nil {
		return nil, err
	}
	for _, d := range [][2]string{ // a slice, not a map: every endorser must return the same refusal
		{"vid", input.VID}, {"idxChassis", input.IdxChassis},
		{"idxEngine", input.IdxEngine}, {"envelopeAnchor", input.EnvelopeAnchor},
	} {
		if err := requireDigest(d[0], d[1]); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(input.Pseudonym) == "" {
		return nil, fmt.Errorf("registry: pseudonym is required")
	}
	input.Provenance = strings.ToUpper(strings.TrimSpace(input.Provenance))
	attestation, err := s.Repo.GetImport(input.IdxChassis)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("registry: registration requires a prior Customs or assembler attestation")
		}
		return nil, err
	}
	if attestation.IdxEngine != input.IdxEngine || attestation.Source != input.Provenance {
		return nil, fmt.Errorf("registry: registration does not match the prior %s attestation", attestation.Source)
	}
	// A description committed at import must be reproduced at registration.
	if attestation.DescAnchor != "" && input.DescAnchor != attestation.DescAnchor {
		return nil, fmt.Errorf("registry: registration does not match the description committed at import")
	}
	if input.DescAnchor != "" {
		if err := requireDigest("descAnchor", input.DescAnchor); err != nil {
			return nil, err
		}
	}
	if err := s.ensureIdentityAvailable("chassis", input.IdxChassis); err != nil {
		return nil, err
	}
	if err := s.ensureIdentityAvailable("engine", input.IdxEngine); err != nil {
		return nil, err
	}
	if _, err := s.Repo.GetVehicle(input.VID); err == nil {
		return nil, fmt.Errorf("registry: vehicle already registered")
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	vehicle := rules.Vehicle{VID: input.VID, Events: []rules.Event{
		{
			Seq: 0, RecordID: attestation.RecordID, Type: rules.EventImport,
			VID: input.VID, P: input.Pseudonym, Tier: rules.E2,
			AttestorID: attestation.AttestorMSP, IdxChassis: input.IdxChassis,
			IdxEngine: input.IdxEngine, Provenance: input.Provenance,
		},
		{
			Seq: 1, RecordID: "registration:" + input.VID, Type: rules.EventRegistration,
			VID: input.VID, P: input.Pseudonym, Tier: rules.E2,
			AttestorID: MSPDMT, IdxChassis: input.IdxChassis,
			IdxEngine: input.IdxEngine, Provenance: input.Provenance,
		},
	}}
	record := &VehicleRecord{Vehicle: vehicle, EnvelopeAnchor: input.EnvelopeAnchor, DescAnchor: attestation.DescAnchor}
	if err := s.Repo.PutVehicle(record); err != nil {
		return nil, err
	}
	if err := s.Repo.PutIdentityOwner("chassis", input.IdxChassis, input.VID); err != nil {
		return nil, err
	}
	if err := s.Repo.PutIdentityOwner("engine", input.IdxEngine, input.VID); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) ensureIdentityAvailable(kind, digest string) error {
	owner, err := s.Repo.GetIdentityOwner(kind, digest)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("registry: %s identity is already assigned to vehicle %s", kind, owner)
}

// AppendEventBy is the contract entry point. A claim is filed under the
// writer's own registry.party so one insurer cannot file as another.
func (s *Service) AppendEventBy(actor, party string, input EventInput) (*rules.Event, error) {
	if input.Type == rules.EventClaim {
		if party == "" {
			return nil, fmt.Errorf("registry: an insurer certificate needs a registry.party attribute to file a claim")
		}
		if input.InsurerID != "" && input.InsurerID != party {
			return nil, fmt.Errorf("registry: this certificate files claims only as insurer %s, not %s", party, input.InsurerID)
		}
		input.InsurerID = party
	}
	return s.AppendEvent(actor, input)
}

func (s *Service) AppendEvent(actor string, input EventInput) (*rules.Event, error) {
	record, err := s.Repo.GetVehicle(input.VID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.RecordID) == "" {
		return nil, fmt.Errorf("registry: event recordId is required")
	}
	for _, existing := range record.Vehicle.Events {
		if existing.RecordID == input.RecordID {
			return nil, fmt.Errorf("registry: duplicate event recordId %s", input.RecordID)
		}
	}
	event := rules.Event{
		Seq: nextSequence(&record.Vehicle), RecordID: input.RecordID, Type: input.Type,
		VID: input.VID, P: input.Pseudonym, Day: input.Day,
		Odometer: input.Odometer, VETLaneID: input.VETLaneID, RetestGroup: input.RetestGroup,
		EvidenceAnchor: input.EvidenceAnchor, EvidenceKind: strings.ToUpper(input.EvidenceKind),
		InsurerID: input.InsurerID, ClaimType: input.ClaimType, AttestorID: input.AttestorID,
	}
	switch event.Type {
	case EventEncumbranceSet, EventEncumbranceReleased, EventTheftReport, EventOwnerConfirmed, rules.EventTheftFlag:
		event.EvidenceAnchor = strings.TrimSpace(input.Reference)
	}
	if event.P == "" {
		for i := len(record.Vehicle.Events) - 1; i >= 0; i-- {
			if record.Vehicle.Events[i].P != "" {
				event.P = record.Vehicle.Events[i].P
				break
			}
		}
	}
	if err := s.authoriseAndGradeEvent(actor, &record.Vehicle, &event); err != nil {
		return nil, err
	}
	if event.Type == rules.EventTheftFlag {
		// A flag needs a current DMT owner confirmation or an approved theft case.
		if !confirmedSince(&record.Vehicle, event.EvidenceAnchor) && !s.approvedTheftCase(record, event.EvidenceAnchor) {
			return nil, fmt.Errorf("registry: a theft flag needs the DMT's owner confirmation for report %s, or an approved theft case", event.EvidenceAnchor)
		}
	}
	if event.Type == rules.EventTransfer {
		// The sealed owner record changes with ownership.
		if err := requireDigest("envelopeAnchor", input.EnvelopeAnchor); err != nil {
			return nil, fmt.Errorf("registry: a transfer needs the new owner's sealed record: %w", err)
		}
		if input.EnvelopeAnchor == record.EnvelopeAnchor {
			return nil, fmt.Errorf("registry: a transfer needs a new sealed owner record, not the current one")
		}
		if strings.TrimSpace(input.Pseudonym) == "" {
			return nil, fmt.Errorf("registry: a transfer needs the new owner's pseudonym")
		}
		record.EnvelopeAnchor = input.EnvelopeAnchor
	}
	record.Vehicle.Events = append(record.Vehicle.Events, event)
	if err := s.Repo.PutVehicle(record); err != nil {
		return nil, err
	}
	return &event, nil
}

func nextSequence(v *rules.Vehicle) uint64 {
	var max uint64
	for _, event := range v.Events {
		if event.Seq > max {
			max = event.Seq
		}
	}
	return max + 1
}

func (s *Service) authoriseAndGradeEvent(actor string, vehicle *rules.Vehicle, event *rules.Event) error {
	actor = canonicalMSP(actor)
	switch event.Type {
	case rules.EventTransfer:
		if err := requireMSP(actor, MSPDMT); err != nil {
			return err
		}
		if vehicle.Stolen {
			return fmt.Errorf("registry: ownership transfer blocked while vehicle is stolen")
		}
		if vehicle.WrittenOff {
			return fmt.Errorf("registry: ownership transfer blocked while write-off is unresolved")
		}
		if vehicle.Retired {
			return fmt.Errorf("registry: ownership transfer blocked: vehicle is retired from the register")
		}
		if encumbered(vehicle) {
			return fmt.Errorf("registry: ownership transfer blocked: the vehicle is under finance until the absolute owner's release is recorded")
		}
		event.Tier = rules.E2
	case rules.EventMileage, rules.EventEmission:
		if err := requireMSP(actor, MSPVET); err != nil {
			return err
		}
		if event.Type == rules.EventMileage && (event.Odometer < 0 || event.VETLaneID == "") {
			return fmt.Errorf("registry: mileage requires a non-negative reading and VET lane ID")
		}
		for _, e := range vehicle.Events { // a backdated reading would make F1b skip the pair
			if event.Type == rules.EventMileage && e.Type == rules.EventMileage && e.Day > event.Day {
				return fmt.Errorf("registry: a mileage reading cannot be dated before the previous reading (day %d)", e.Day)
			}
		}
		event.Tier = rules.E1
	case rules.EventAccident:
		if err := requireMSP(actor, MSPPolice); err != nil {
			return err
		}
		event.Tier = rules.E2
		event.EvidenceAnchor = event.RecordID
		event.EvidenceKind = "POLICE_REPORT"
		event.IncidentRef = rules.ReferencePresent
	case rules.EventClaim:
		if err := requireMSP(actor, MSPInsurer); err != nil {
			return err
		}
		event.Tier = rules.E1
		if event.EvidenceAnchor == "" {
			event.IncidentRef = rules.ReferenceMissing
		} else {
			event.IncidentRef = rules.ReferencePresent
		}
		if event.EvidenceKind == "POLICE_REPORT" {
			if !priorRecordExists(vehicle, event.EvidenceAnchor, rules.EventAccident, event.Seq) {
				return fmt.Errorf("registry: claim cites a missing or non-prior police report")
			}
			event.Tier = rules.E2
		} else if event.EvidenceKind != "" && event.EvidenceKind != "ASSESSOR_ATTESTATION" {
			return fmt.Errorf("registry: evidenceKind must be POLICE_REPORT or ASSESSOR_ATTESTATION")
		}
	case rules.EventTheftFlag, rules.EventTheftClear:
		if err := requireMSP(actor, MSPPolice); err != nil {
			return err
		}
		// A clearance also withdraws an unconfirmed report, so a false report
		// cannot leave a buyer warning standing.
		if event.Type == rules.EventTheftClear && !vehicle.Stolen && !reportPending(vehicle) {
			return fmt.Errorf("registry: the vehicle is not flagged stolen and has no open theft report")
		}
		if event.Type == rules.EventTheftFlag && !validReference(event.EvidenceAnchor) {
			return fmt.Errorf("registry: a theft flag needs the Police report number or an approved theft case id")
		}
		event.Tier = rules.E2
		vehicle.Stolen = event.Type == rules.EventTheftFlag
	case EventTheftReport:
		if err := requireMSP(actor, MSPPolice); err != nil {
			return err
		}
		if !validReference(event.EvidenceAnchor) {
			return fmt.Errorf("registry: a theft report needs the Police report number")
		}
		event.EvidenceKind = "POLICE_REPORT"
		event.Tier = rules.E1
	case EventOwnerConfirmed:
		if err := requireMSP(actor, MSPDMT); err != nil {
			return err
		}
		if !validReference(event.EvidenceAnchor) {
			return fmt.Errorf("registry: an owner confirmation needs the Police report number")
		}
		if !reportedSince(vehicle, event.EvidenceAnchor) {
			return fmt.Errorf("registry: no theft report %s on this vehicle since its last transfer or clearance", event.EvidenceAnchor)
		}
		event.EvidenceKind = "OWNER_CONFIRMATION"
		event.Tier = rules.E2
	case EventEncumbranceSet, EventEncumbranceReleased:
		if err := requireMSP(actor, MSPDMT); err != nil {
			return err
		}
		if event.EvidenceAnchor == "" {
			return fmt.Errorf("registry: an encumbrance needs the absolute-owner endorsement or deletion-letter reference")
		}
		if event.Type == EventEncumbranceSet && encumbered(vehicle) {
			return fmt.Errorf("registry: the vehicle is already recorded as under finance")
		}
		if event.Type == EventEncumbranceReleased && !encumbered(vehicle) {
			return fmt.Errorf("registry: the vehicle is not recorded as under finance")
		}
		event.EvidenceKind = "FINANCE_REFERENCE"
		event.Tier = rules.E2
	default:
		return fmt.Errorf("registry: event type %s is not appendable through this transaction", event.Type)
	}
	// Do not default the attestor to the insurer's MSP: F6b would read every
	// assessor-less claim as a shared assessor.
	if event.AttestorID == "" && event.Type != rules.EventClaim {
		event.AttestorID = actor
	}
	return nil
}

// encumbered reports whether the latest encumbrance event is unreleased.
func encumbered(v *rules.Vehicle) bool {
	state := false
	for _, e := range v.Events {
		switch e.Type {
		case EventEncumbranceSet:
			state = true
		case EventEncumbranceReleased:
			state = false
		}
	}
	return state
}

// validReference accepts 1 to 64 letters, digits, / _ or -.
func validReference(ref string) bool {
	if len(ref) == 0 || len(ref) > 64 {
		return false
	}
	for _, r := range ref {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '/' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// reportedSince reports whether a THEFT_REPORT with this number was recorded
// after the last transfer or theft clearance.
func reportedSince(v *rules.Vehicle, ref string) bool {
	reported := false
	for _, e := range v.Events {
		switch {
		case e.Type == rules.EventTransfer || e.Type == rules.EventTheftClear:
			reported = false
		case e.Type == EventTheftReport && e.EvidenceAnchor == ref:
			reported = true
		}
	}
	return reported
}

// reportPending: a theft report since the last transfer, flag or clearance.
func reportPending(v *rules.Vehicle) bool {
	pending, _ := theftStatus(v)
	return pending
}

// confirmedSince reports whether the DMT confirmed this report after the last
// transfer or clearance, so a confirmation cannot be reused.
func confirmedSince(v *rules.Vehicle, ref string) bool {
	if ref == "" {
		return false
	}
	confirmed := false
	for _, e := range v.Events {
		switch {
		case e.Type == rules.EventTransfer || e.Type == rules.EventTheftClear:
			confirmed = false
		case e.Type == EventOwnerConfirmed && e.EvidenceAnchor == ref:
			confirmed = true
		}
	}
	return confirmed
}

// approvedTheftCase: the reference names an approved theft case on this
// vehicle with no transfer since approval.
func (s *Service) approvedTheftCase(record *VehicleRecord, ref string) bool {
	request, err := s.Repo.GetReveal(ref)
	if err != nil || request == nil {
		return false
	}
	return request.Status == RevealApproved && request.VID == record.Vehicle.VID &&
		request.GroundCode == "THEFT_INVESTIGATION" &&
		request.OwnerAnchor != "" && request.OwnerAnchor == record.EnvelopeAnchor
}

// theftStatus derives the buyer-visible theft facts from history. Clearing an
// unconfirmed report is a withdrawal, not a cleared theft.
func theftStatus(v *rules.Vehicle) (pending, cleared bool) {
	flagged := false
	for _, e := range v.Events {
		switch e.Type {
		case EventTheftReport:
			pending = true
		case rules.EventTransfer:
			pending = false
		case rules.EventTheftFlag:
			pending, flagged = false, true
		case rules.EventTheftClear:
			pending = false
			if flagged {
				cleared, flagged = true, false
			}
		}
	}
	return pending, cleared
}

func statusOf(record *VehicleRecord) VehicleStatus {
	v := record.Vehicle
	pending, cleared := theftStatus(&v)
	return VehicleStatus{
		Stolen: v.Stolen, WrittenOff: v.WrittenOff, Retired: v.Retired, Encumbered: encumbered(&v),
		TheftReportPending: pending && !v.Stolen, TheftCleared: cleared && !v.Stolen, DescCommitted: record.DescAnchor != "",
	}
}

func priorRecordExists(v *rules.Vehicle, recordID string, eventType rules.EventType, before uint64) bool {
	if recordID == "" {
		return false
	}
	for _, event := range v.Events {
		if event.RecordID == recordID && event.Type == eventType && event.Seq < before {
			return true
		}
	}
	return false
}

func (s *Service) SetWriteOffStatus(actor, vid string, writtenOff bool) (*VehicleRecord, error) {
	if err := requireMSP(actor, MSPInsurer, MSPPolice); err != nil {
		return nil, err
	}
	record, err := s.Repo.GetVehicle(vid)
	if err != nil {
		return nil, err
	}
	record.Vehicle.WrittenOff = writtenOff
	if err := s.Repo.PutVehicle(record); err != nil {
		return nil, err
	}
	return record, nil
}

// SetRetiredStatus retires a vehicle (DMT only). The record stays so F4 can
// catch reuse of its identity.
func (s *Service) SetRetiredStatus(actor, vid string, retired bool) (*VehicleRecord, error) {
	if err := requireMSP(actor, MSPDMT); err != nil {
		return nil, err
	}
	record, err := s.Repo.GetVehicle(vid)
	if err != nil {
		return nil, err
	}
	record.Vehicle.Retired = retired
	if err := s.Repo.PutVehicle(record); err != nil {
		return nil, err
	}
	return record, nil
}

// RecordIdentityAccess logs an off-ledger identity reveal on an approved
// case (Police only).
func (s *Service) RecordIdentityAccess(actor, caseID, vid string) (*RevealRequest, error) {
	if err := requireMSP(actor, MSPPolice); err != nil {
		return nil, err
	}
	request, err := s.Repo.GetReveal(caseID)
	if err != nil {
		return nil, fmt.Errorf("registry: case %s: %w", caseID, err)
	}
	if request.Status != RevealApproved || request.VID != vid {
		return nil, fmt.Errorf("registry: case %s is not an approved reveal for this vehicle", caseID)
	}
	record, err := s.Repo.GetVehicle(vid)
	if err != nil {
		return nil, err
	}
	if request.OwnerAnchor != "" && record.EnvelopeAnchor != request.OwnerAnchor {
		return nil, fmt.Errorf("registry: the owner record of this vehicle has changed since case %s was approved; a new case is needed", caseID)
	}
	// The anchor names which owner record was opened.
	request.AccessLog = append(request.AccessLog, "IDENTITY_REVEAL:"+vid+":"+record.EnvelopeAnchor)
	if err := s.Repo.PutReveal(request); err != nil {
		return nil, err
	}
	return request, nil
}

func (s *Service) Disclosure(vid string, profile rules.Profile) (*rules.Disclosure, error) {
	return s.disclosureScoped(vid, profile, nil, "")
}

func (s *Service) disclosureScoped(vid string, profile rules.Profile, fleet []string, scope string) (*rules.Disclosure, error) {
	records, err := s.Repo.ListVehicles()
	if err != nil {
		return nil, err
	}
	ledger := buildLedgerScoped(records, profile, fleet, scope)
	vehicle, ok := ledger.Vehicles[vid]
	if !ok {
		return nil, ErrNotFound
	}
	disclosure := rules.Disclose(ledger, vehicle, profile)
	return &disclosure, nil
}

// buildLedger builds the rule-engine view under a profile, matching the
// experiment transformer. Without OWNER there is no owner index, so F6 reports
// a gap rather than passing on an empty fleet.
func buildLedger(records []*VehicleRecord, profile rules.Profile) *rules.Ledger {
	return buildLedgerScoped(records, profile, nil, "")
}

// buildLedgerScoped also groups the DMT-asserted fleet under an in-memory tag,
// since on-chain pseudonyms are per vehicle and never group.
func buildLedgerScoped(records []*VehicleRecord, profile rules.Profile, fleet []string, scope string) *rules.Ledger {
	ledger := &rules.Ledger{Vehicles: make(map[string]*rules.Vehicle), OwnerLink: rules.ReferenceRedacted}
	if profile.Exposes(rules.CapabilityOwner) {
		ledger.OwnerVehicles = map[string][]string{}
		ledger.OwnerLink = rules.ReferencePresent
	}
	inFleet := map[string]bool{}
	for _, vid := range fleet {
		inFleet[vid] = true
	}
	for _, record := range records {
		vehicle := record.Vehicle
		vehicle.Events = append([]rules.Event(nil), record.Vehicle.Events...)
		if !profile.Exposes(rules.CapabilityIncident) {
			for i := range vehicle.Events {
				rules.RedactIncidentReference(&vehicle.Events[i])
			}
		}
		if !profile.Exposes(rules.CapabilityAsset) {
			vehicle.AssetLink = rules.ReferenceRedacted
		}
		if ledger.OwnerVehicles != nil && inFleet[vehicle.VID] {
			for i := range vehicle.Events {
				vehicle.Events[i].P = scope
			}
		}
		ledger.Vehicles[vehicle.VID] = &vehicle
		if ledger.OwnerVehicles == nil {
			continue
		}
		for _, e := range vehicle.Events {
			if e.P != "" {
				// Pseudonyms are written by the DMT; one equal to the view-time
				// tag must not join the attested fleet.
				if e.P != scope || inFleet[vehicle.VID] {
					ledger.OwnerVehicles[e.P] = append(ledger.OwnerVehicles[e.P], vehicle.VID)
				}
				break
			}
		}
	}
	for owner := range ledger.OwnerVehicles {
		sort.Strings(ledger.OwnerVehicles[owner])
	}
	return ledger
}

// FleetHash binds an owner's fleet to one vehicle; only the hash is stored.
func FleetHash(vid string, fleet []string) string {
	sorted := append([]string(nil), fleet...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte("fleet|" + vid + "|" + strings.Join(sorted, ",")))
	return hex.EncodeToString(sum[:])
}

func hashID(id string) string {
	if id == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("caller|" + id))
	return hex.EncodeToString(sum[:])
}

func short(vid string) string {
	if len(vid) > 8 {
		return vid[:8]
	}
	return vid
}

// normaliseFleet validates an asserted fleet. An empty fleet means the vehicle
// alone.
func (s *Service) normaliseFleet(vid string, fleet []string) ([]string, error) {
	if len(fleet) == 0 {
		fleet = []string{vid}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(fleet))
	for _, f := range fleet {
		f = strings.ToLower(strings.TrimSpace(f))
		if seen[f] {
			return nil, fmt.Errorf("registry: the fleet lists vehicle %s twice", short(f))
		}
		seen[f] = true
		if _, err := s.Repo.GetVehicle(f); err != nil {
			return nil, fmt.Errorf("registry: fleet vehicle %s: %w", short(f), err)
		}
		out = append(out, f)
	}
	if !seen[vid] {
		return nil, fmt.Errorf("registry: the fleet must include the vehicle under evaluation")
	}
	sort.Strings(out)
	return out, nil
}

var mspIDs = map[string]bool{MSPDMT: true, MSPPolice: true, MSPVET: true, MSPCustoms: true, MSPInsurer: true, MSPAssembler: true, "Org1MSP": true, "Org2MSP": true, "Org3MSP": true, "Org4MSP": true, "Org5MSP": true}

// EvaluateOwnerPredicate lets the DMT test F6a or F6b over an owner's fleet and
// record the verdict only. The eligibility gate (F1a for F6a, a named assessor
// for F6b) stops it being a free query.
func (s *Service) EvaluateOwnerPredicate(actor, actorID, vid, ruleID string, fleet []string, txID string) (*PredicateResult, error) {
	if err := requireMSP(actor, MSPDMT); err != nil {
		return nil, err
	}
	descriptor, ok := rules.Lookup(ruleID)
	if !ok || descriptor.RequiredCapability != rules.CapabilityOwner {
		return nil, fmt.Errorf("registry: %q is not an owner-pattern rule", ruleID)
	}
	record, err := s.Repo.GetVehicle(vid)
	if err != nil {
		return nil, err
	}
	switch ruleID {
	case "F6a":
		p1, err := s.Disclosure(vid, rules.ProfileP1)
		if err != nil {
			return nil, err
		}
		eligible := false
		for _, f := range p1.Failed {
			if f.RuleID == "F1a" {
				eligible = true
				break
			}
		}
		if !eligible {
			return nil, fmt.Errorf("registry: the F6a predicate needs an odometer rollback finding (F1a) on this vehicle first")
		}
	case "F6b":
		eligible := false
		for _, e := range record.Vehicle.Events {
			if e.Type == rules.EventClaim && e.AttestorID != "" && !mspIDs[e.AttestorID] {
				eligible = true
				break
			}
		}
		if !eligible {
			return nil, fmt.Errorf("registry: the F6b predicate needs a claim with a named assessor on this vehicle first")
		}
	}
	normalised, err := s.normaliseFleet(vid, fleet)
	if err != nil {
		return nil, err
	}
	records, err := s.Repo.ListVehicles()
	if err != nil {
		return nil, err
	}
	ledger := buildLedgerScoped(records, rules.ProfileP0, normalised, "predicate:"+vid)
	findings := rules.Decide(descriptor, ledger, ledger.Vehicles[vid], rules.ProfileP0)
	finding := findings[0]
	for _, f := range findings {
		if f.Flagged() {
			finding = f
			break
		}
	}
	stored := finding
	stored.Detail, stored.Reason = "", ""
	if err := s.Repo.PutPredicate(&PredicateRecord{
		VID: vid, RuleID: ruleID, Finding: stored, FleetHash: FleetHash(vid, normalised),
		RequesterIDHash: hashID(actorID), TxID: txID,
	}); err != nil {
		return nil, err
	}
	return &PredicateResult{VID: vid, RuleID: ruleID, Status: finding.Status, Strength: finding.Strength}, nil
}

// ResolvePolicy grants the minimum profile for a purpose and refuses callers
// whose role does not match it. party identifies the insurer.
func (s *Service) ResolvePolicy(actor, party string, purpose Purpose, vid, caseID string) (Policy, *RevealRequest, error) {
	policy := Policy{Purpose: purpose, CaseID: caseID}
	switch purpose {
	case PurposeBuyer:
		policy.Profile, policy.Reason = rules.ProfileP2, "buyer receives the derived disclosure: vehicle history without incident links or owner linkage"
	case PurposeDMT:
		if err := requireMSP(actor, MSPDMT); err != nil {
			return policy, nil, err
		}
		policy.Profile, policy.Reason = rules.ProfileP2, "DMT sees the vehicle history needed for the transaction; incident and owner linkage are not required"
	case PurposeInsurer:
		if err := requireMSP(actor, MSPInsurer); err != nil {
			return policy, nil, err
		}
		policy.Profile, policy.Reason = rules.ProfileP2, "insurer has no claim on this vehicle; vehicle history only"
		record, err := s.Repo.GetVehicle(vid)
		if err != nil {
			return policy, nil, err
		}
		for _, e := range record.Vehicle.Events {
			if e.Type == rules.EventClaim && party != "" && e.InsurerID == party {
				policy.Profile, policy.Reason = rules.ProfileP1, "insurer holds a claim on this vehicle: claim-to-incident links are exposed for every claim on it"
				break
			}
		}
	case PurposePolice:
		if err := requireMSP(actor, MSPPolice); err != nil {
			return policy, nil, err
		}
		policy.Profile, policy.Reason = rules.ProfileP1, "Police investigation: full vehicle and incident history; owner linkage requires an approved case"
		if caseID != "" {
			request, err := s.Repo.GetReveal(caseID)
			if err != nil {
				return policy, nil, fmt.Errorf("registry: case %s: %w", caseID, err)
			}
			if request.Status != RevealApproved || request.VID != vid {
				return policy, nil, fmt.Errorf("registry: case %s is not an approved reveal for this vehicle", caseID)
			}
			policy.Profile, policy.Reason = rules.ProfileP0, "owner-scope linkage granted under dual-approved case "+caseID+" ("+request.GroundCode+" "+request.GroundRef+")"
			return policy, request, nil
		}
	default:
		return policy, nil, fmt.Errorf("registry: unknown purpose %q", purpose)
	}
	return policy, nil, nil
}

// DisclosureFor evaluates the rules under the granted profile. Owner-scope
// access is logged before the result is returned.
func (s *Service) DisclosureFor(actor, party string, purpose Purpose, vid, caseID string) (*PurposeDisclosure, error) {
	return s.DisclosureForFleet(actor, party, purpose, vid, caseID, nil)
}

// DisclosureForFleet is DisclosureFor with a DMT-supplied fleet, which must
// match the fleet committed at approval.
func (s *Service) DisclosureForFleet(actor, party string, purpose Purpose, vid, caseID string, fleet []string) (*PurposeDisclosure, error) {
	policy, request, err := s.ResolvePolicy(actor, party, purpose, vid, caseID)
	if err != nil {
		return nil, err
	}
	var scoped []string
	if request != nil {
		normalised, err := s.normaliseFleet(vid, fleet)
		if err != nil {
			return nil, err
		}
		committed := request.FleetHash
		if committed == "" {
			committed = FleetHash(vid, []string{vid})
		}
		if FleetHash(vid, normalised) != committed {
			return nil, fmt.Errorf("registry: the owner's fleet has changed since case %s was approved; renew the case", request.ID)
		}
		scoped = normalised
		request.AccessLog = append(request.AccessLog, "OWNER_SCOPE:"+vid)
		if err := s.Repo.PutReveal(request); err != nil {
			return nil, err
		}
	}
	disclosure, err := s.disclosureScoped(vid, policy.Profile, scoped, "case:"+caseID)
	if err != nil {
		return nil, err
	}
	record, err := s.Repo.GetVehicle(vid)
	if err != nil {
		return nil, err
	}
	return &PurposeDisclosure{Disclosure: *disclosure, Policy: policy, Status: statusOf(record)}, nil
}

// HistoryFor returns the events a purpose may see. Buyers and insurers get
// derived disclosures only; incident references are redacted without INCIDENT.
func (s *Service) HistoryFor(actor, party string, purpose Purpose, vid, caseID string) (*VehicleRecord, *Policy, error) {
	if purpose == PurposeBuyer || purpose == PurposeInsurer {
		return nil, nil, fmt.Errorf("registry: purpose %s receives the derived disclosure, not the lifecycle history", purpose)
	}
	policy, _, err := s.ResolvePolicy(actor, party, purpose, vid, caseID)
	if err != nil {
		return nil, nil, err
	}
	record, err := s.Repo.GetVehicle(vid)
	if err != nil {
		return nil, nil, err
	}
	copied := *record
	copied.Vehicle.Events = append([]rules.Event(nil), record.Vehicle.Events...)
	if !policy.Profile.Exposes(rules.CapabilityIncident) {
		for i := range copied.Vehicle.Events {
			rules.RedactIncidentReference(&copied.Vehicle.Events[i])
		}
	}
	return &copied, &policy, nil
}

var allowedGrounds = map[string]bool{
	"FRAUD_INVESTIGATION":  true,
	"THEFT_INVESTIGATION":  true,
	"COURT_ORDER":          true,
	"DATA_SUBJECT_CONSENT": true,
}

func (s *Service) CreateRevealRequest(actor, id, vid, ruleID, ground, groundRef string) (*RevealRequest, error) {
	return s.CreateRevealRequestBy(actor, "", id, vid, ruleID, ground, groundRef)
}

// CreateRevealRequestBy records the requester's identity hash. F6a and F6b
// qualify only through a DMT predicate.
func (s *Service) CreateRevealRequestBy(actor, actorID, id, vid, ruleID, ground, groundRef string) (*RevealRequest, error) {
	if err := requireMSP(actor, MSPDMT, MSPPolice); err != nil {
		return nil, err
	}
	ground = strings.ToUpper(strings.TrimSpace(ground))
	if !allowedGrounds[ground] {
		return nil, fmt.Errorf("registry: invalid reveal ground %q", ground)
	}
	groundRef = strings.TrimSpace(groundRef)
	if groundRef == "" {
		return nil, fmt.Errorf("registry: a legal-basis reference (case, court order or consent record) is required")
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("registry: reveal request ID is required")
	}
	predicateFleet := ""
	if d, ok := rules.Lookup(ruleID); ok && d.RequiredCapability == rules.CapabilityOwner {
		record, err := s.Repo.GetPredicate(vid, ruleID)
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("registry: %s is undecidable at P1; the DMT must first evaluate its owner-scope predicate", ruleID)
		} else if err != nil {
			return nil, err
		}
		if !record.Finding.SatisfiesRevealPrecondition() {
			return nil, fmt.Errorf("registry: the %s predicate did not return an owner-attributed ALERT", ruleID)
		}
		predicateFleet = record.FleetHash
	} else {
		disclosure, err := s.Disclosure(vid, rules.ProfileP1)
		if err != nil {
			return nil, err
		}
		eligible := false
		for _, finding := range disclosure.Failed {
			if finding.RuleID == ruleID && finding.SatisfiesRevealPrecondition() {
				eligible = true
				break
			}
		}
		if !eligible {
			return nil, fmt.Errorf("registry: no live owner-attributed ALERT for rule %s", ruleID)
		}
	}
	if _, err := s.Repo.GetReveal(id); err == nil {
		return nil, fmt.Errorf("registry: reveal request already exists")
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	request := &RevealRequest{
		ID: id, VID: vid, RuleID: ruleID, GroundCode: ground, GroundRef: groundRef,
		RequestedBy: canonicalMSP(actor), Status: RevealPending,
		RequesterIDHash: hashID(actorID), PredicateFleetHash: predicateFleet,
	}
	if err := s.Repo.PutReveal(request); err != nil {
		return nil, err
	}
	return request, nil
}

func (s *Service) ApproveReveal(actor, id string, approve bool, reason string) (*RevealRequest, error) {
	return s.ApproveRevealBy(actor, "", id, approve, reason, nil)
}

// ApproveRevealBy refuses self-approval. The DMT approval stores the fleet
// hash that later owner-scope reads must match.
func (s *Service) ApproveRevealBy(actor, actorID, id string, approve bool, reason string, fleet []string) (*RevealRequest, error) {
	if err := requireMSP(actor, MSPDMT, MSPPolice); err != nil {
		return nil, err
	}
	request, err := s.Repo.GetReveal(id)
	if err != nil {
		return nil, err
	}
	if request.Status != RevealPending {
		return nil, fmt.Errorf("registry: reveal request is already %s", request.Status)
	}
	if approve && actorID != "" && request.RequesterIDHash != "" && hashID(actorID) == request.RequesterIDHash {
		return nil, fmt.Errorf("registry: the officer who requested case %s cannot approve it; a second officer must", id)
	}
	if side := canonicalMSP(actor); approve && (side == MSPDMT && request.DMTApproved || side == MSPPolice && request.PoliceApproved) {
		return nil, fmt.Errorf("registry: %s has already approved case %s", side, id)
	}
	if !approve {
		request.Status = RevealRejected
		request.Reason = strings.TrimSpace(reason)
	} else if canonicalMSP(actor) == MSPDMT {
		normalised, err := s.normaliseFleet(request.VID, fleet)
		if err != nil {
			return nil, err
		}
		hash := FleetHash(request.VID, normalised)
		if request.PredicateFleetHash != "" && hash != request.PredicateFleetHash {
			return nil, fmt.Errorf("registry: the fleet attested at approval differs from the fleet the %s predicate was evaluated on", request.RuleID)
		}
		request.DMTApproved = true
		request.DMTApproverIDHash = hashID(actorID)
		request.FleetHash, request.FleetSize = hash, len(normalised)
	} else {
		request.PoliceApproved = true
		request.PoliceApproverIDHash = hashID(actorID)
	}
	if request.DMTApproved && request.PoliceApproved {
		request.Status = RevealApproved
		// Bind the case to the current owner record.
		record, err := s.Repo.GetVehicle(request.VID)
		if err != nil {
			return nil, err
		}
		request.OwnerAnchor = record.EnvelopeAnchor
	}
	if err := s.Repo.PutReveal(request); err != nil {
		return nil, err
	}
	return request, nil
}

func sortedRecords(records []*VehicleRecord) {
	sort.Slice(records, func(i, j int) bool { return records[i].Vehicle.VID < records[j].Vehicle.VID })
}
