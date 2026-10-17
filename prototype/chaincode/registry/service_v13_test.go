package registry

import (
	"strings"
	"testing"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

// Tests for self-approval, fleet scoping, the owner predicate and the finance flag.

func f3cCase(t *testing.T, service *Service, requesterID, caseID string) *RevealRequest {
	t.Helper()
	request, err := service.CreateRevealRequestBy(MSPPolice, requesterID, caseID, testVID, "F3c", "FRAUD_INVESTIGATION", "CID/2026/"+caseID)
	if err != nil {
		t.Fatalf("CreateRevealRequestBy: %v", err)
	}
	return request
}

// The officer who asks cannot also approve.
func TestRequesterCannotApproveOwnCase(t *testing.T) {
	service, second := twoVehicleFleet(t)
	request := f3cCase(t, service, "police-officer-1", "c1")
	if request.RequesterIDHash == "" || strings.Contains(request.RequesterIDHash, "police-officer-1") {
		t.Fatalf("requester stored as %q, want a hash of the identity", request.RequesterIDHash)
	}
	if _, err := service.ApproveRevealBy(MSPPolice, "police-officer-1", request.ID, true, "", nil); err == nil || !strings.Contains(err.Error(), "cannot approve") {
		t.Fatalf("self-approval err = %v", err)
	}
	if _, err := service.ApproveRevealBy(MSPDMT, "dmt-officer-1", request.ID, true, "", []string{testVID, second}); err != nil {
		t.Fatalf("DMT approval: %v", err)
	}
	approved, err := service.ApproveRevealBy(MSPPolice, "police-officer-2", request.ID, true, "", nil)
	if err != nil || approved.Status != RevealApproved {
		t.Fatalf("second-officer approval status=%v err=%v", approved, err)
	}
	if approved.FleetSize != 2 || approved.FleetHash != FleetHash(testVID, []string{second, testVID}) {
		t.Fatalf("fleet commitment = %d %q", approved.FleetSize, approved.FleetHash)
	}
	// Rejecting one's own request is a withdrawal and stays allowed.
	withdrawn := f3cCase(t, service, "police-officer-1", "c2")
	if _, err := service.ApproveRevealBy(MSPPolice, "police-officer-1", withdrawn.ID, false, "withdrawn", nil); err != nil {
		t.Fatalf("withdrawal refused: %v", err)
	}
}

// Per-vehicle pseudonyms never group; the attested fleet does, under a case only.
func TestFleetGroupsOwnersVehiclesOnlyUnderApprovedCase(t *testing.T) {
	service, second := twoVehicleFleet(t)
	// With per-vehicle pseudonyms the committed P0 view cannot see the pattern.
	plain, err := service.Disclosure(testVID, rules.ProfileP0)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range plain.Failed {
		if f.RuleID == "F6a" {
			t.Fatal("F6a fired from committed pseudonyms alone; owner linkage leaked on-chain")
		}
	}
	request := f3cCase(t, service, "p1", "c3")
	fleet := []string{second, testVID}
	if _, err := service.ApproveRevealBy(MSPDMT, "d1", request.ID, true, "", fleet); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveRevealBy(MSPPolice, "p2", request.ID, true, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DisclosureForFleet(MSPPolice, "", PurposePolice, testVID, request.ID, []string{testVID}); err == nil {
		t.Fatal("a narrower fleet than the attested one was accepted")
	}
	if _, err := service.DisclosureForFleet(MSPPolice, "", PurposePolice, testVID, request.ID, []string{testVID, "9999999999999999999999999999999999999999999999999999999999999999"}); err == nil {
		t.Fatal("a fleet naming an unknown vehicle was accepted")
	}
	owner, err := service.DisclosureForFleet(MSPPolice, "", PurposePolice, testVID, request.ID, fleet)
	if err != nil {
		t.Fatal(err)
	}
	var f6a *rules.Finding
	for i, f := range owner.Failed {
		if f.RuleID == "F6a" {
			f6a = &owner.Failed[i]
		}
	}
	if f6a == nil {
		t.Fatal("F6a did not fire over the attested fleet")
	}
	// Live mileage is E1, so the serial pattern is a WARNING.
	if f6a.Strength != rules.StrengthWarning {
		t.Fatalf("F6a strength = %s; live VET readings are E1, so WARNING is expected", f6a.Strength)
	}
	// Nothing about the fleet was written to the vehicle records.
	for _, vid := range fleet {
		record, _ := service.Repo.GetVehicle(vid)
		for _, e := range record.Vehicle.Events {
			if strings.HasPrefix(e.P, "case:") {
				t.Fatalf("view-time owner tag was persisted on %s", vid[:8])
			}
		}
	}
}

// The DMT predicate lets serial claim fraud ground a case at P1, verdict only.
func TestOwnerPredicateGroundsCaseOnlyAtAlert(t *testing.T) {
	service, second := twoVehicleFleet(t)
	fleet := []string{testVID, second}
	// Eligibility: F6b needs a claim with a named assessor; the fleet's claims have none yet.
	if _, err := service.EvaluateOwnerPredicate(MSPDMT, "d1", testVID, "F6b", fleet, "tx-0"); err == nil || !strings.Contains(err.Error(), "named assessor") {
		t.Fatalf("predicate without an assessor claim: err = %v", err)
	}
	if _, err := service.EvaluateOwnerPredicate(MSPPolice, "p1", testVID, "F6a", fleet, "tx-0"); err == nil {
		t.Fatal("the Police evaluated the DMT custodian's predicate")
	}
	// The same assessor on both vehicles; one claim cites a police report (E2).
	for _, vid := range fleet {
		if _, err := service.AppendEvent(MSPInsurer, EventInput{
			RecordID: vid[:4] + "-assessed", VID: vid, Type: rules.EventClaim, Day: 600,
			EvidenceAnchor: vid[:4] + "-acc", EvidenceKind: "POLICE_REPORT", InsurerID: "insurer-a", AttestorID: "assessor-77",
		}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.EvaluateOwnerPredicate(MSPDMT, "d1", testVID, "F6b", fleet, "tx-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != rules.StatusFlagged || result.Strength != rules.StrengthAlert {
		t.Fatalf("F6b predicate = %s/%s, want FLAGGED/ALERT", result.Status, result.Strength)
	}
	stored, _ := service.Repo.GetPredicate(testVID, "F6b")
	if stored.Finding.Detail != "" || stored.FleetHash != FleetHash(testVID, fleet) {
		t.Fatalf("predicate record leaks detail or lost its fleet commitment: %+v", stored)
	}
	request, err := service.CreateRevealRequestBy(MSPPolice, "p1", "c-f6b", testVID, "F6b", "FRAUD_INVESTIGATION", "CID/2026/77")
	if err != nil {
		t.Fatalf("F6b case refused despite an ALERT predicate: %v", err)
	}
	if _, err := service.ApproveRevealBy(MSPDMT, "d1", request.ID, true, "", []string{testVID}); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("DMT approved a fleet other than the predicate's: err = %v", err)
	}
	if _, err := service.ApproveRevealBy(MSPDMT, "d1", request.ID, true, "", fleet); err != nil {
		t.Fatal(err)
	}

	// F6a: eligible (F1a rollback), but the pattern rests on E1 readings, so it
	// stays WARNING and cannot ground a case.
	f6a, err := service.EvaluateOwnerPredicate(MSPDMT, "d1", testVID, "F6a", fleet, "tx-2")
	if err != nil {
		t.Fatal(err)
	}
	if f6a.Status != rules.StatusFlagged || f6a.Strength != rules.StrengthWarning {
		t.Fatalf("F6a predicate = %s/%s, want FLAGGED/WARNING", f6a.Status, f6a.Strength)
	}
	if _, err := service.CreateRevealRequestBy(MSPPolice, "p1", "c-f6a", testVID, "F6a", "FRAUD_INVESTIGATION", "CID/2026/78"); err == nil || !strings.Contains(err.Error(), "did not return an owner-attributed ALERT") {
		t.Fatalf("F6a WARNING grounded a case: err = %v", err)
	}
	if _, err := service.CreateRevealRequestBy(MSPPolice, "p1", "c-x", second, "F6b", "FRAUD_INVESTIGATION", "CID/2026/79"); err == nil || !strings.Contains(err.Error(), "predicate") {
		t.Fatalf("F6b case without a predicate on that vehicle: err = %v", err)
	}
}

// Finance flag: DMT-only events, transfer refused while unreleased, status visible.
func TestEncumbranceBlocksTransferUntilReleased(t *testing.T) {
	service := registeredService(t)
	set := EventInput{RecordID: "lease-1", VID: testVID, Reference: "AO/LOLC/2026/115"}
	set.Type = EventEncumbranceSet
	if _, err := service.AppendEvent(MSPInsurer, set); err == nil {
		t.Fatal("an insurer recorded an encumbrance")
	}
	if _, err := service.AppendEvent(MSPDMT, EventInput{RecordID: "lease-0", VID: testVID, Type: EventEncumbranceSet}); err == nil {
		t.Fatal("an encumbrance without a reference was accepted")
	}
	if _, err := service.AppendEvent(MSPDMT, set); err != nil {
		t.Fatal(err)
	}
	disclosure, err := service.DisclosureFor("anyone", "", PurposeBuyer, testVID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !disclosure.Status.Encumbered {
		t.Fatal("buyer disclosure does not show the vehicle is under finance")
	}
	transfer := EventInput{RecordID: "transfer-1", VID: testVID, Type: rules.EventTransfer, Pseudonym: "next-owner", EnvelopeAnchor: testAnchor2}
	if _, err := service.AppendEvent(MSPDMT, transfer); err == nil || !strings.Contains(err.Error(), "under finance") {
		t.Fatalf("transfer of an encumbered vehicle: err = %v", err)
	}
	release := EventInput{RecordID: "lease-1-release", VID: testVID, Type: EventEncumbranceReleased, Reference: "DL/LOLC/2026/88"}
	if _, err := service.AppendEvent(MSPDMT, release); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendEvent(MSPDMT, EventInput{RecordID: "lease-2-release", VID: testVID, Type: EventEncumbranceReleased, Reference: "DL/x"}); err == nil {
		t.Fatal("a release of an unencumbered vehicle was accepted")
	}
	if _, err := service.AppendEvent(MSPDMT, transfer); err != nil {
		t.Fatalf("transfer after release: %v", err)
	}
	// The history keeps both events: the flag cannot be cleared without trace.
	record, _ := service.Repo.GetVehicle(testVID)
	count := 0
	for _, e := range record.Vehicle.Events {
		if e.Type == EventEncumbranceSet || e.Type == EventEncumbranceReleased {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("encumbrance history has %d events, want 2", count)
	}
}

// A claim without an assessor carries no attestor, so F6b cannot read it as shared.
func TestClaimWithoutAssessorHasNoAttestor(t *testing.T) {
	service := registeredService(t)
	claim, err := service.AppendEvent(MSPInsurer, EventInput{RecordID: "claim-x", VID: testVID, Type: rules.EventClaim, InsurerID: "insurer-a"})
	if err != nil {
		t.Fatal(err)
	}
	if claim.AttestorID != "" {
		t.Fatalf("assessor-less claim attestor = %q, want empty", claim.AttestorID)
	}
	mileage, err := service.AppendEvent(MSPVET, EventInput{RecordID: "m-x", VID: testVID, Type: rules.EventMileage, Odometer: 1, VETLaneID: "lane-1"})
	if err != nil {
		t.Fatal(err)
	}
	if mileage.AttestorID != MSPVET {
		t.Fatalf("non-claim attestor = %q, want the writer's MSP", mileage.AttestorID)
	}
}

// The buyer's P2 view does not evaluate claim-to-incident links, so an anchored
// claim is a structural gap.
func TestBuyerDisclosureRedactsIncidentLinks(t *testing.T) {
	service := registeredService(t)
	if _, err := service.AppendEvent(MSPPolice, EventInput{RecordID: "report-1", VID: testVID, Type: rules.EventAccident}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendEvent(MSPInsurer, EventInput{RecordID: "claim-1", VID: testVID, Type: rules.EventClaim,
		EvidenceAnchor: "report-1", EvidenceKind: "POLICE_REPORT", InsurerID: "insurer-a"}); err != nil {
		t.Fatal(err)
	}
	buyer, err := service.DisclosureFor("anyone", "", PurposeBuyer, testVID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range buyer.NotPerformed {
		if f.RuleID == "F3a" {
			if !f.Structural {
				t.Fatalf("F3a gap at P2 is not marked structural: %+v", f)
			}
			police, err := service.DisclosureFor(MSPPolice, "", PurposePolice, testVID, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range police.Passed {
				if g.RuleID == "F3a" {
					return // P1 still checks the anchored claim
				}
			}
			t.Fatal("F3a not evaluated at P1")
		}
	}
	t.Fatalf("F3a was evaluated for the buyer on an unredacted incident link: %+v", buyer)
}

// The DMT that registers cannot also attest as assembler (F5 bypass).
func TestDMTCannotSelfAttestLocalAssembly(t *testing.T) {
	service := NewService(NewMemoryRepository())
	attestation := ImportAttestation{RecordID: "asm-1", IdxChassis: testAnchor2, IdxEngine: testAnchor2, Source: "ASSEMBLER"}
	if _, err := service.CreateImportAttestation(MSPDMT, attestation); err == nil || !strings.Contains(err.Error(), MSPAssembler) {
		t.Fatalf("DMT assembler attestation: err = %v", err)
	}
	if _, err := service.CreateImportAttestation(MSPAssembler, attestation); err != nil {
		t.Fatalf("assembler attestation: %v", err)
	}
}
