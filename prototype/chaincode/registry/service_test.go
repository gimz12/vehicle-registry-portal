package registry

import (
	"strings"
	"testing"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

const (
	testVID     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testChassis = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testEngine  = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	testAnchor  = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	testPseudo  = "pseudonym-vehicle-1"
	testAnchor2 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
)

func registeredService(t *testing.T) *Service {
	t.Helper()
	service := NewService(NewMemoryRepository())
	_, err := service.CreateImportAttestation(MSPCustoms, ImportAttestation{
		RecordID: "import-1", IdxChassis: testChassis, IdxEngine: testEngine, Source: "CUSTOMS",
	})
	if err != nil {
		t.Fatalf("CreateImportAttestation: %v", err)
	}
	_, err = service.RegisterVehicle(MSPDMT, RegistrationInput{
		VID: testVID, Pseudonym: testPseudo, IdxChassis: testChassis,
		IdxEngine: testEngine, Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor,
	})
	if err != nil {
		t.Fatalf("RegisterVehicle: %v", err)
	}
	return service
}

func TestRegistrationRequiresRoleProvenanceAndUniqueIdentity(t *testing.T) {
	service := NewService(NewMemoryRepository())
	input := ImportAttestation{RecordID: "import-1", IdxChassis: testChassis, IdxEngine: testEngine, Source: "CUSTOMS"}
	if _, err := service.CreateImportAttestation(MSPDMT, input); err == nil {
		t.Fatal("DMT created a Customs attestation")
	}
	if _, err := service.CreateImportAttestation(MSPCustoms, input); err != nil {
		t.Fatalf("Customs attestation rejected: %v", err)
	}
	registration := RegistrationInput{
		VID: testVID, Pseudonym: testPseudo, IdxChassis: testChassis,
		IdxEngine: testEngine, Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor,
	}
	if _, err := service.RegisterVehicle(MSPPolice, registration); err == nil {
		t.Fatal("Police registered a vehicle")
	}
	if _, err := service.RegisterVehicle("Org1MSP", registration); err != nil {
		t.Fatalf("mapped DMT test-network MSP was rejected: %v", err)
	}
	if _, err := service.RegisterVehicle(MSPDMT, registration); err == nil {
		t.Fatal("duplicate registration was accepted")
	}
}

func TestEventRBACDerivedEvidenceAndTransferGates(t *testing.T) {
	service := registeredService(t)
	if _, err := service.AppendEvent(MSPDMT, EventInput{
		RecordID: "mileage-1", VID: testVID, Type: rules.EventMileage,
		Odometer: 50000, VETLaneID: "lane-1",
	}); err == nil {
		t.Fatal("DMT submitted a VET mileage event")
	}
	mileage, err := service.AppendEvent(MSPVET, EventInput{
		RecordID: "mileage-1", VID: testVID, Type: rules.EventMileage,
		Odometer: 50000, VETLaneID: "lane-1",
	})
	if err != nil {
		t.Fatalf("AppendEvent mileage: %v", err)
	}
	if mileage.Tier != rules.E1 {
		t.Fatalf("VET mileage tier = %s, want E1", mileage.Tier)
	}
	reportAndConfirm(t, service, "PR/1")
	if _, err := service.AppendEvent(MSPPolice, EventInput{
		RecordID: "stolen-1", VID: testVID, Type: rules.EventTheftFlag, Reference: "PR/1",
	}); err != nil {
		t.Fatalf("Set theft flag: %v", err)
	}
	if _, err := service.AppendEvent(MSPDMT, EventInput{
		RecordID: "transfer-1", VID: testVID, Type: rules.EventTransfer,
		Pseudonym: "new-owner-pseudonym", EnvelopeAnchor: testAnchor2,
	}); err == nil || !strings.Contains(err.Error(), "stolen") {
		t.Fatalf("transfer while stolen err = %v", err)
	}
	if _, err := service.AppendEvent(MSPPolice, EventInput{
		RecordID: "clear-1", VID: testVID, Type: rules.EventTheftClear,
	}); err != nil {
		t.Fatalf("Clear theft flag: %v", err)
	}
	if _, err := service.AppendEvent(MSPDMT, EventInput{
		RecordID: "transfer-1", VID: testVID, Type: rules.EventTransfer,
		Pseudonym: "new-owner-pseudonym",
	}); err == nil || !strings.Contains(err.Error(), "sealed record") {
		t.Fatalf("transfer without the new owner's envelope anchor err = %v", err)
	}
	if _, err := service.AppendEvent(MSPDMT, EventInput{
		RecordID: "transfer-1", VID: testVID, Type: rules.EventTransfer,
		Pseudonym: "new-owner-pseudonym", EnvelopeAnchor: testAnchor2,
	}); err != nil {
		t.Fatalf("transfer after clearance: %v", err)
	}
	if record, _ := service.Repo.GetVehicle(testVID); record.EnvelopeAnchor != testAnchor2 {
		t.Fatalf("transfer did not replace the owner envelope anchor: %s", record.EnvelopeAnchor)
	}
}

func TestClaimEvidenceAndDualApprovalReveal(t *testing.T) {
	service := registeredService(t)
	if _, err := service.AppendEvent(MSPPolice, EventInput{
		RecordID: "police-report-1", VID: testVID, Type: rules.EventAccident,
	}); err != nil {
		t.Fatalf("Append accident: %v", err)
	}
	for _, id := range []string{"claim-1", "claim-2"} {
		event, err := service.AppendEvent(MSPInsurer, EventInput{
			RecordID: id, VID: testVID, Type: rules.EventClaim,
			EvidenceAnchor: "police-report-1", EvidenceKind: "POLICE_REPORT", InsurerID: id,
		})
		if err != nil {
			t.Fatalf("Append claim %s: %v", id, err)
		}
		if event.Tier != rules.E2 {
			t.Fatalf("claim tier = %s, want E2 from police anchor", event.Tier)
		}
	}
	if _, err := service.CreateRevealRequest(MSPDMT, "reveal-0", testVID, "F3c", "FRAUD_INVESTIGATION", ""); err == nil {
		t.Fatal("reveal request without a legal-basis reference was accepted")
	}
	request, err := service.CreateRevealRequest(MSPDMT, "reveal-1", testVID, "F3c", "FRAUD_INVESTIGATION", "CID/2026/0412")
	if err != nil {
		t.Fatalf("CreateRevealRequest: %v", err)
	}
	if request.GroundRef != "CID/2026/0412" {
		t.Fatalf("GroundRef = %q, not carried into the request", request.GroundRef)
	}
	if request.Status != RevealPending {
		t.Fatalf("status = %s, want PENDING", request.Status)
	}
	request, err = service.ApproveReveal(MSPDMT, request.ID, true, "")
	if err != nil || request.Status != RevealPending {
		t.Fatalf("unilateral approval status=%s err=%v", request.Status, err)
	}
	request, err = service.ApproveReveal(MSPPolice, request.ID, true, "")
	if err != nil || request.Status != RevealApproved {
		t.Fatalf("dual approval status=%s err=%v", request.Status, err)
	}
}

func TestFabricDisclosureUsesSharedRules(t *testing.T) {
	service := registeredService(t)
	for i, odo := range []int64{80000, 60000} {
		_, err := service.AppendEvent(MSPVET, EventInput{
			RecordID: "mileage-" + string(rune('1'+i)), VID: testVID,
			Type: rules.EventMileage, Day: int64(i * 200), Odometer: odo, VETLaneID: "lane-1",
		})
		if err != nil {
			t.Fatalf("Append mileage: %v", err)
		}
	}
	disclosure, err := service.Disclosure(testVID, rules.ProfileP1)
	if err != nil {
		t.Fatalf("Disclosure: %v", err)
	}
	found := false
	for _, finding := range disclosure.Failed {
		if finding.RuleID == "F1a" {
			found = true
		}
	}
	if !found {
		t.Fatal("shared F1 rule did not appear in chaincode service disclosure")
	}
}

// The view must carry the owner index under OWNER, or F6 passes on an empty fleet.
func TestDisclosureBuildsOwnerIndexOnlyWhenProfileExposesOwner(t *testing.T) {
	service := registeredService(t)
	const (
		secondVID     = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
		secondChassis = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		secondEngine  = "0000000000000000000000000000000000000000000000000000000000000000"
	)
	if _, err := service.CreateImportAttestation(MSPCustoms, ImportAttestation{
		RecordID: "import-2", IdxChassis: secondChassis, IdxEngine: secondEngine, Source: "CUSTOMS",
	}); err != nil {
		t.Fatalf("second import: %v", err)
	}
	if _, err := service.RegisterVehicle(MSPDMT, RegistrationInput{
		VID: secondVID, Pseudonym: testPseudo, IdxChassis: secondChassis,
		IdxEngine: secondEngine, Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor,
	}); err != nil {
		t.Fatalf("second registration: %v", err)
	}
	for _, vid := range []string{testVID, secondVID} {
		for i, odo := range []int64{80000, 60000} {
			if _, err := service.AppendEvent(MSPVET, EventInput{
				RecordID: vid[:4] + "-mileage-" + string(rune('1'+i)), VID: vid,
				Type: rules.EventMileage, Day: int64(i * 200), Odometer: odo, VETLaneID: "lane-1",
			}); err != nil {
				t.Fatalf("Append mileage: %v", err)
			}
		}
	}
	statusOf := func(profile rules.Profile) rules.Status {
		disclosure, err := service.Disclosure(testVID, profile)
		if err != nil {
			t.Fatalf("Disclosure %s: %v", profile, err)
		}
		for _, group := range [][]rules.Finding{disclosure.Failed, disclosure.Passed, disclosure.NotPerformed} {
			for _, finding := range group {
				if finding.RuleID == "F6a" {
					return finding.Status
				}
			}
		}
		t.Fatalf("F6a missing from disclosure at %s", profile)
		return ""
	}
	if got := statusOf(rules.ProfileP0); got != rules.StatusFlagged {
		t.Fatalf("F6a at P0 = %s, want FLAGGED (owner index not built from committed pseudonyms)", got)
	}
	if got := statusOf(rules.ProfileP1); got != rules.StatusSuppressed {
		t.Fatalf("F6a at P1 = %s, want SUPPRESSED", got)
	}
}

// twoVehicleFleet registers a second vehicle of the same owner, each with its
// own pseudonym, plus a rollback and an anchored insurer-a claim on both.
func twoVehicleFleet(t *testing.T) (*Service, string) {
	t.Helper()
	service := registeredService(t)
	const (
		secondVID     = "1111111111111111111111111111111111111111111111111111111111111111"
		secondChassis = "2222222222222222222222222222222222222222222222222222222222222222"
		secondEngine  = "3333333333333333333333333333333333333333333333333333333333333333"
	)
	if _, err := service.CreateImportAttestation(MSPCustoms, ImportAttestation{
		RecordID: "import-2", IdxChassis: secondChassis, IdxEngine: secondEngine, Source: "CUSTOMS",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RegisterVehicle(MSPDMT, RegistrationInput{
		VID: secondVID, Pseudonym: "pseudonym-vehicle-2", IdxChassis: secondChassis,
		IdxEngine: secondEngine, Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor,
	}); err != nil {
		t.Fatal(err)
	}
	for _, vid := range []string{testVID, secondVID} {
		for i, odo := range []int64{80000, 60000} {
			if _, err := service.AppendEvent(MSPVET, EventInput{
				RecordID: vid[:4] + "-m-" + string(rune('1'+i)), VID: vid,
				Type: rules.EventMileage, Day: int64(i * 200), Odometer: odo, VETLaneID: "lane-1",
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := service.AppendEvent(MSPPolice, EventInput{RecordID: vid[:4] + "-acc", VID: vid, Type: rules.EventAccident, Day: 500}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.AppendEvent(MSPInsurer, EventInput{
			RecordID: vid[:4] + "-claim", VID: vid, Type: rules.EventClaim, Day: 505,
			EvidenceAnchor: vid[:4] + "-acc", EvidenceKind: "POLICE_REPORT", InsurerID: "insurer-a",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A second claim on the same police anchor: an E2 ALERT (F3c) that can ground a reveal.
	if _, err := service.AppendEvent(MSPInsurer, EventInput{
		RecordID: "dup-claim", VID: testVID, Type: rules.EventClaim, Day: 509,
		EvidenceAnchor: testVID[:4] + "-acc", EvidenceKind: "POLICE_REPORT", InsurerID: "insurer-b",
	}); err != nil {
		t.Fatal(err)
	}
	return service, secondVID
}

func TestPurposeBoundDisclosure(t *testing.T) {
	service, second := twoVehicleFleet(t)
	fleet := []string{testVID, second}
	cases := []struct {
		name    string
		actor   string
		party   string
		purpose Purpose
		caseID  string
		want    rules.Profile
		wantErr bool
	}{
		{"buyer", "anyone", "", PurposeBuyer, "", rules.ProfileP2, false},
		{"dmt", "Org1MSP", "", PurposeDMT, "", rules.ProfileP2, false},
		{"dmt via wrong msp", MSPPolice, "", PurposeDMT, "", "", true},
		{"insurer with claim", MSPInsurer, "insurer-a", PurposeInsurer, "", rules.ProfileP1, false},
		{"insurer without claim", MSPInsurer, "insurer-z", PurposeInsurer, "", rules.ProfileP2, false},
		{"police no case", "Org2MSP", "", PurposePolice, "", rules.ProfileP1, false},
		{"police unknown case", "Org2MSP", "", PurposePolice, "nope", "", true},
		{"unknown purpose", MSPDMT, "", Purpose("AUDITOR"), "", "", true},
	}
	for _, tc := range cases {
		got, err := service.DisclosureFor(tc.actor, tc.party, tc.purpose, testVID, tc.caseID)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected refusal", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.Policy.Profile != tc.want || got.Profile != tc.want {
			t.Errorf("%s: profile %s, want %s", tc.name, got.Policy.Profile, tc.want)
		}
		if got.Policy.Reason == "" {
			t.Errorf("%s: policy reason is empty", tc.name)
		}
	}

	// Owner scope only after dual approval, and every use is logged on the case.
	request, err := service.CreateRevealRequest(MSPPolice, "case-7", testVID, "F3c", "FRAUD_INVESTIGATION", "CID/2026/0007")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DisclosureFor(MSPPolice, "", PurposePolice, testVID, request.ID); err == nil {
		t.Fatal("owner scope granted on a pending case")
	}
	if _, err := service.ApproveRevealBy(MSPDMT, "", request.ID, true, "", fleet); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveReveal(MSPPolice, request.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	// The read must present the fleet the DMT attested; anything else is refused.
	if _, err := service.DisclosureFor(MSPPolice, "", PurposePolice, testVID, request.ID); err == nil || !strings.Contains(err.Error(), "fleet has changed") {
		t.Fatalf("owner-scope read without the attested fleet: err = %v", err)
	}
	owner, err := service.DisclosureForFleet(MSPPolice, "", PurposePolice, testVID, request.ID, fleet)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Profile != rules.ProfileP0 {
		t.Fatalf("approved case profile = %s, want P0", owner.Profile)
	}
	f6 := false
	for _, f := range owner.Failed {
		if f.RuleID == "F6a" {
			f6 = true
		}
	}
	if !f6 {
		t.Fatal("owner-scope disclosure did not evaluate the serial-odometer rule")
	}
	if _, err := service.DisclosureFor(MSPDMT, "", PurposeDMT, testVID, request.ID); err != nil {
		t.Fatal(err)
	}
	audit, err := service.Repo.GetReveal(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.AccessLog) != 1 || audit.AccessLog[0] != "OWNER_SCOPE:"+testVID {
		t.Fatalf("access log = %v, want exactly one owner-scope entry", audit.AccessLog)
	}
}

func TestHistoryRedactsByPurpose(t *testing.T) {
	service, _ := twoVehicleFleet(t)
	if _, _, err := service.HistoryFor("anyone", "", PurposeBuyer, testVID, ""); err == nil {
		t.Fatal("buyer received lifecycle history")
	}
	dmt, policy, err := service.HistoryFor(MSPDMT, "", PurposeDMT, testVID, "")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Profile != rules.ProfileP2 {
		t.Fatalf("DMT history profile = %s", policy.Profile)
	}
	for _, e := range dmt.Vehicle.Events {
		if e.Type == rules.EventClaim && (e.EvidenceAnchor != "" || e.IncidentRef != rules.ReferenceRedacted) {
			t.Fatalf("DMT history exposes an incident reference: %+v", e)
		}
	}
	police, _, err := service.HistoryFor(MSPPolice, "", PurposePolice, testVID, "")
	if err != nil {
		t.Fatal(err)
	}
	anchored := false
	for _, e := range police.Vehicle.Events {
		if e.Type == rules.EventClaim && e.EvidenceAnchor != "" {
			anchored = true
		}
	}
	if !anchored {
		t.Fatal("Police history lost the incident reference")
	}
	stored, _ := service.Repo.GetVehicle(testVID)
	for _, e := range stored.Vehicle.Events {
		if e.Type == rules.EventClaim && e.EvidenceAnchor == "" {
			t.Fatal("redaction leaked into committed state")
		}
	}
}

func TestRetiredVehicleBlocksTransferAndIdentityReuse(t *testing.T) {
	service := registeredService(t)
	if _, err := service.SetRetiredStatus(MSPPolice, testVID, true); err == nil {
		t.Fatal("Police retired a vehicle")
	}
	if _, err := service.SetRetiredStatus(MSPDMT, testVID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendEvent(MSPDMT, EventInput{RecordID: "t-1", VID: testVID, Type: rules.EventTransfer, Pseudonym: "p2", EnvelopeAnchor: testAnchor2}); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("transfer of a retired vehicle err = %v", err)
	}
	// Re-registering the retired vehicle's identity is refused at write (F4 prevention).
	if _, err := service.RegisterVehicle(MSPDMT, RegistrationInput{
		VID: "9999999999999999999999999999999999999999999999999999999999999999", Pseudonym: "p3",
		IdxChassis: testChassis, IdxEngine: testEngine, Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor,
	}); err == nil {
		t.Fatal("identity of a retired vehicle was registered again")
	}
}

func TestIdentityAccessRequiresApprovedCase(t *testing.T) {
	service, _ := twoVehicleFleet(t)
	if _, err := service.RecordIdentityAccess(MSPPolice, "nope", testVID); err == nil {
		t.Fatal("identity access recorded on an unknown case")
	}
	request, err := service.CreateRevealRequest(MSPPolice, "case-9", testVID, "F3c", "COURT_ORDER", "B/9/2026")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordIdentityAccess(MSPPolice, request.ID, testVID); err == nil {
		t.Fatal("identity access recorded on a pending case")
	}
	if _, err := service.ApproveReveal(MSPDMT, request.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveReveal(MSPPolice, request.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordIdentityAccess(MSPDMT, request.ID, testVID); err == nil {
		t.Fatal("DMT recorded an identity access")
	}
	audit, err := service.RecordIdentityAccess(MSPPolice, request.ID, testVID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.AccessLog) != 1 || audit.AccessLog[0] != "IDENTITY_REVEAL:"+testVID+":"+audit.OwnerAnchor || audit.OwnerAnchor == "" {
		t.Fatalf("access log = %v, owner anchor %q", audit.AccessLog, audit.OwnerAnchor)
	}
	// After a transfer the case no longer opens the vehicle's owner record.
	if _, err := service.AppendEvent(MSPDMT, EventInput{RecordID: "t-9", VID: testVID, Type: rules.EventTransfer, Pseudonym: "p9", EnvelopeAnchor: testAnchor2}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordIdentityAccess(MSPPolice, request.ID, testVID); err == nil || !strings.Contains(err.Error(), "changed since case") {
		t.Fatalf("reveal after transfer err = %v; want refusal", err)
	}
}
