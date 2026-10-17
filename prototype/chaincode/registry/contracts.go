package registry

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/cid"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Certificate attributes: registry.role overrides the MSP-to-role map;
// registry.party names the insurer for INSURER callers.
const (
	attrRole  = "registry.role"
	attrParty = "registry.party"
)

// roleMSP maps a certificate role to the MSP it acts as and the only
// organisation whose CA may issue it, so one CA cannot mint another agency's role.
var roleMSP = map[string]struct{ msp, issuer string }{
	"DMT": {MSPDMT, "Org1MSP"}, "ASSEMBLER": {MSPAssembler, "Org1MSP"}, "POLICE": {MSPPolice, "Org2MSP"},
	"CUSTOMS": {MSPCustoms, "Org3MSP"}, "VET": {MSPVET, "Org4MSP"}, "INSURER": {MSPInsurer, "Org5MSP"},
}

type VehicleRegistry struct{ contractapi.Contract }
type FraudDetector struct{ contractapi.Contract }
type IdentityManager struct{ contractapi.Contract }

func actorMSP(ctx contractapi.TransactionContextInterface) (string, error) {
	msp, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("registry: resolve caller MSP: %w", err)
	}
	if role, ok, err := cid.GetAttributeValue(ctx.GetStub(), attrRole); err == nil && ok {
		r, known := roleMSP[strings.ToUpper(strings.TrimSpace(role))]
		if !known {
			return "", fmt.Errorf("registry: unknown certificate role %q", role)
		}
		if r.issuer != msp {
			return "", fmt.Errorf("registry: certificate role %s may not be issued by %s", role, msp)
		}
		return r.msp, nil
	}
	return msp, nil
}

func actorParty(ctx contractapi.TransactionContextInterface) string {
	party, ok, err := cid.GetAttributeValue(ctx.GetStub(), attrParty)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(party)
}

func parsePurpose(raw string) (Purpose, error) {
	p := Purpose(strings.ToUpper(strings.TrimSpace(raw)))
	switch p {
	case PurposeBuyer, PurposeDMT, PurposeInsurer, PurposePolice:
		return p, nil
	}
	return "", fmt.Errorf("registry: unknown purpose %q", raw)
}

func decodeJSON[T any](raw string) (T, error) {
	var value T
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("registry: invalid JSON input: %w", err)
	}
	return value, nil
}

func serviceFor(ctx contractapi.TransactionContextInterface) *Service {
	return NewService(NewFabricRepository(ctx))
}

// caseEvent is the whole case-event payload: every organisation receives
// chaincode events, so it carries only the case id and state.
type caseEvent struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// callerID is the caller's enrolment identity; only its hash is stored.
func callerID(ctx contractapi.TransactionContextInterface) string {
	id, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return ""
	}
	return id
}

// transientFleet reads the DMT-resolved owner fleet from transient data, which
// reaches endorsing peers but is never written to a block.
func transientFleet(ctx contractapi.TransactionContextInterface) ([]string, error) {
	data, err := ctx.GetStub().GetTransient()
	if err != nil {
		return nil, fmt.Errorf("registry: read transient data: %w", err)
	}
	raw, ok := data["fleet"]
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	var fleet []string
	if err := json.Unmarshal(raw, &fleet); err != nil {
		return nil, fmt.Errorf("registry: transient fleet must be a JSON array of vehicle handles: %w", err)
	}
	return fleet, nil
}

func emit(ctx contractapi.TransactionContextInterface, name string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return ctx.GetStub().SetEvent(name, payload)
}

func (c *VehicleRegistry) CreateImportAttestation(ctx contractapi.TransactionContextInterface, raw string) (*ImportAttestation, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	input, err := decodeJSON[ImportAttestation](raw)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).CreateImportAttestation(actor, input)
	if err == nil {
		err = emit(ctx, "ImportAttestationCreated", result)
	}
	return result, err
}

func (c *VehicleRegistry) RegisterVehicle(ctx contractapi.TransactionContextInterface, raw string) (*VehicleRecord, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	input, err := decodeJSON[RegistrationInput](raw)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).RegisterVehicle(actor, input)
	if err == nil {
		err = emit(ctx, "VehicleRegistered", result)
	}
	return result, err
}

func appendTypedEvent(ctx contractapi.TransactionContextInterface, raw string, eventType rules.EventType) (*rules.Event, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	input, err := decodeJSON[EventInput](raw)
	if err != nil {
		return nil, err
	}
	input.Type = eventType
	result, err := serviceFor(ctx).AppendEventBy(actor, actorParty(ctx), input)
	if err == nil {
		err = emit(ctx, "LifecycleEventAppended", result)
	}
	return result, err
}

func (c *VehicleRegistry) TransferVehicle(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, rules.EventTransfer)
}

func (c *VehicleRegistry) AppendMileage(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, rules.EventMileage)
}

func (c *VehicleRegistry) AppendAccident(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, rules.EventAccident)
}

func (c *VehicleRegistry) AppendClaim(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, rules.EventClaim)
}

func (c *VehicleRegistry) AppendEmission(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, rules.EventEmission)
}

func (c *VehicleRegistry) SetTheftStatus(ctx contractapi.TransactionContextInterface, raw string, stolen bool) (*rules.Event, error) {
	if stolen {
		return appendTypedEvent(ctx, raw, rules.EventTheftFlag)
	}
	return appendTypedEvent(ctx, raw, rules.EventTheftClear)
}

func (c *VehicleRegistry) SetWriteOffStatus(ctx contractapi.TransactionContextInterface, vid string, writtenOff bool) (*VehicleRecord, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).SetWriteOffStatus(actor, vid, writtenOff)
	if err == nil {
		err = emit(ctx, "WriteOffStatusChanged", result)
	}
	return result, err
}

// SetEncumbrance records a finance company as absolute owner (DMT only).
func (c *VehicleRegistry) SetEncumbrance(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, EventEncumbranceSet)
}

// ReleaseEncumbrance records the finance company's deletion letter (DMT only).
func (c *VehicleRegistry) ReleaseEncumbrance(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, EventEncumbranceReleased)
}

// AppendTheftReport records a Police theft report; it neither flags the
// vehicle nor blocks a transfer.
func (c *VehicleRegistry) AppendTheftReport(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, EventTheftReport)
}

// ConfirmReporter records the DMT's off-ledger match of reporter to owner (DMT
// only). No NIC or pseudonym is written.
func (c *VehicleRegistry) ConfirmReporter(ctx contractapi.TransactionContextInterface, raw string) (*rules.Event, error) {
	return appendTypedEvent(ctx, raw, EventOwnerConfirmed)
}

func (c *VehicleRegistry) SetRetiredStatus(ctx contractapi.TransactionContextInterface, vid string, retired bool) (*VehicleRecord, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).SetRetiredStatus(actor, vid, retired)
	if err == nil {
		err = emit(ctx, "RetiredStatusChanged", result)
	}
	return result, err
}

// HistoryView is a purpose-filtered lifecycle history with the policy applied.
type HistoryView struct {
	Record *VehicleRecord `json:"record"`
	Policy Policy         `json:"policy"`
}

func (c *VehicleRegistry) GetVehicleHistoryFor(ctx contractapi.TransactionContextInterface, purpose, vid, caseID string) (*HistoryView, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	p, err := parsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	record, policy, err := serviceFor(ctx).HistoryFor(actor, actorParty(ctx), p, vid, caseID)
	if err != nil {
		return nil, err
	}
	return &HistoryView{Record: record, Policy: *policy}, nil
}

// GetDisclosureFor returns the minimum-profile disclosure for a purpose.
// Owner-scope access is recorded on the case.
func (c *FraudDetector) GetDisclosureFor(ctx contractapi.TransactionContextInterface, purpose, vid, caseID string) (*PurposeDisclosure, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	p, err := parsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	fleet, err := transientFleet(ctx)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).DisclosureForFleet(actor, actorParty(ctx), p, vid, caseID, fleet)
	if err == nil && result.Policy.Profile == rules.ProfileP0 {
		err = emit(ctx, "OwnerScopeAccessed", caseEvent{ID: caseID, Status: "OWNER_SCOPE"})
	}
	return result, err
}

// EvaluateOwnerPredicate runs one owner-pattern rule over the transient fleet
// and records the verdict only.
func (c *FraudDetector) EvaluateOwnerPredicate(ctx contractapi.TransactionContextInterface, vid, ruleID string) (*PredicateResult, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	fleet, err := transientFleet(ctx)
	if err != nil {
		return nil, err
	}
	return serviceFor(ctx).EvaluateOwnerPredicate(actor, callerID(ctx), vid, ruleID, fleet, ctx.GetStub().GetTxID())
}

// EvaluateVehicle is kept for the original portal route; it is the BUYER purpose.
func (c *FraudDetector) EvaluateVehicle(ctx contractapi.TransactionContextInterface, vid string) (*PurposeDisclosure, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	return serviceFor(ctx).DisclosureFor(actor, "", PurposeBuyer, vid, "")
}

func (c *IdentityManager) CreateRevealRequest(ctx contractapi.TransactionContextInterface, id, vid, ruleID, ground, groundRef string) (*RevealRequest, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).CreateRevealRequestBy(actor, callerID(ctx), id, vid, ruleID, ground, groundRef)
	if err == nil {
		err = emit(ctx, "RevealRequested", caseEvent{ID: result.ID, Status: string(result.Status)})
	}
	return result, err
}

func (c *IdentityManager) ApproveReveal(ctx contractapi.TransactionContextInterface, id string) (*RevealRequest, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	fleet, err := transientFleet(ctx)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).ApproveRevealBy(actor, callerID(ctx), id, true, "", fleet)
	if err == nil {
		err = emit(ctx, "RevealDecision", caseEvent{ID: result.ID, Status: string(result.Status)})
	}
	return result, err
}

func (c *IdentityManager) RejectReveal(ctx contractapi.TransactionContextInterface, id, reason string) (*RevealRequest, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).ApproveRevealBy(actor, callerID(ctx), id, false, reason, nil)
	if err == nil {
		err = emit(ctx, "RevealDecision", caseEvent{ID: result.ID, Status: string(result.Status)})
	}
	return result, err
}

// RecordIdentityAccess logs a Police owner-envelope opening under an approved case.
func (c *IdentityManager) RecordIdentityAccess(ctx contractapi.TransactionContextInterface, caseID, vid string) (*RevealRequest, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	result, err := serviceFor(ctx).RecordIdentityAccess(actor, caseID, vid)
	if err == nil {
		err = emit(ctx, "IdentityRevealed", caseEvent{ID: caseID, Status: "IDENTITY_REVEAL"})
	}
	return result, err
}

// GetRevealAudit returns a case and its access log to the DMT and Police only,
// since the log names the vehicles under investigation.
func (c *IdentityManager) GetRevealAudit(ctx contractapi.TransactionContextInterface, id string) (*RevealRequest, error) {
	actor, err := actorMSP(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireMSP(actor, MSPDMT, MSPPolice); err != nil {
		return nil, err
	}
	return NewFabricRepository(ctx).GetReveal(id)
}
