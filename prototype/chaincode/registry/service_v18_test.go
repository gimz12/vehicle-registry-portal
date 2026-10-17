package registry

import (
	"strings"
	"testing"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

// Theft: Police report, DMT owner confirmation, then Police flag. Registration
// must reproduce the description committed at import.

const testDesc = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

// reportAndConfirm records a theft report and the DMT's confirmation under ref.
func reportAndConfirm(t *testing.T, s *Service, ref string) {
	t.Helper()
	if _, err := s.AppendEvent(MSPPolice, EventInput{RecordID: "report:" + ref, VID: testVID, Type: EventTheftReport, Reference: ref}); err != nil {
		t.Fatalf("theft report %s: %v", ref, err)
	}
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "confirm:" + ref, VID: testVID, Type: EventOwnerConfirmed, Reference: ref}); err != nil {
		t.Fatalf("owner confirmation %s: %v", ref, err)
	}
}

func flag(s *Service, recordID, ref string) error {
	_, err := s.AppendEvent(MSPPolice, EventInput{RecordID: recordID, VID: testVID, Type: rules.EventTheftFlag, Reference: ref})
	return err
}

func TestV18_TheftFlagNeedsOwnerConfirmation(t *testing.T) {
	s := registeredService(t)
	if err := flag(s, "f-0", ""); err == nil {
		t.Fatal("a theft flag with no report number was accepted")
	}
	if err := flag(s, "f-1", "PR/9"); err == nil || !strings.Contains(err.Error(), "owner confirmation") {
		t.Fatalf("a bare theft flag err = %v", err)
	}
	if _, err := s.AppendEvent(MSPPolice, EventInput{RecordID: "report:PR/9", VID: testVID, Type: EventTheftReport, Reference: "PR/9"}); err != nil {
		t.Fatal(err)
	}
	if err := flag(s, "f-2", "PR/9"); err == nil {
		t.Fatal("a theft flag with a report but no DMT confirmation was accepted")
	}
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "confirm:PR/9", VID: testVID, Type: EventOwnerConfirmed, Reference: "PR/9"}); err != nil {
		t.Fatal(err)
	}
	if err := flag(s, "f-3", "PR/9"); err != nil {
		t.Fatalf("a confirmed theft flag was refused: %v", err)
	}
	if err := flag(s, "f-4", "PR/OTHER"); err == nil {
		t.Fatal("a flag citing another report number was accepted")
	}
	record, _ := s.Repo.GetVehicle(testVID)
	if !record.Vehicle.Stolen {
		t.Fatal("vehicle not stolen after a confirmed flag")
	}
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "t-1", VID: testVID, Type: rules.EventTransfer, Pseudonym: "p2", EnvelopeAnchor: testAnchor2}); err == nil || !strings.Contains(err.Error(), "stolen") {
		t.Fatalf("transfer while stolen err = %v", err)
	}
}

func TestV18_ConfirmationExpiresOnClearAndTransfer(t *testing.T) {
	s := registeredService(t)
	reportAndConfirm(t, s, "PR/1")
	if err := flag(s, "f-1", "PR/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvent(MSPPolice, EventInput{RecordID: "c-1", VID: testVID, Type: rules.EventTheftClear}); err != nil {
		t.Fatal(err)
	}
	if err := flag(s, "f-2", "PR/1"); err == nil {
		t.Fatal("a confirmation was reused after the flag was cleared")
	}
	reportAndConfirm(t, s, "PR/2")
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "t-1", VID: testVID, Type: rules.EventTransfer, Pseudonym: "p2", EnvelopeAnchor: testAnchor2}); err != nil {
		t.Fatalf("transfer after a confirmation but no flag was refused: %v", err)
	}
	if err := flag(s, "f-3", "PR/2"); err == nil {
		t.Fatal("a confirmation for the previous owner licensed a flag after the transfer")
	}
}

func TestV18_OnlyPoliceReportOnlyDMTConfirms(t *testing.T) {
	s := registeredService(t)
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "r-1", VID: testVID, Type: EventTheftReport, Reference: "PR/1"}); err == nil {
		t.Fatal("the DMT recorded a theft report")
	}
	if _, err := s.AppendEvent(MSPPolice, EventInput{RecordID: "r-2", VID: testVID, Type: EventTheftReport}); err == nil {
		t.Fatal("a theft report with no report number was accepted")
	}
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "c-1", VID: testVID, Type: EventOwnerConfirmed, Reference: "PR/1"}); err == nil || !strings.Contains(err.Error(), "no theft report") {
		t.Fatalf("a confirmation with no matching report err = %v", err)
	}
	if _, err := s.AppendEvent(MSPPolice, EventInput{RecordID: "r-3", VID: testVID, Type: EventTheftReport, Reference: "PR/1"}); err != nil {
		t.Fatal(err)
	}
	for _, msp := range []string{MSPPolice, MSPCustoms, MSPInsurer, MSPVET} {
		if _, err := s.AppendEvent(msp, EventInput{RecordID: "c-" + msp, VID: testVID, Type: EventOwnerConfirmed, Reference: "PR/1"}); err == nil {
			t.Fatalf("%s confirmed an owner", msp)
		}
	}
}

func TestV18_ApprovedTheftCaseLicensesFlag(t *testing.T) {
	s := registeredService(t)
	put := func(id, ground string, status RevealStatus, anchor string) {
		if err := s.Repo.PutReveal(&RevealRequest{ID: id, VID: testVID, GroundCode: ground, Status: status, OwnerAnchor: anchor}); err != nil {
			t.Fatal(err)
		}
	}
	put("case-fraud", "FRAUD_INVESTIGATION", RevealApproved, testAnchor)
	put("case-pending", "THEFT_INVESTIGATION", RevealPending, testAnchor)
	put("case-old-owner", "THEFT_INVESTIGATION", RevealApproved, testAnchor2)
	put("case-theft", "THEFT_INVESTIGATION", RevealApproved, testAnchor)
	for _, id := range []string{"case-fraud", "case-pending", "case-old-owner"} {
		if err := flag(s, "f-"+id, id); err == nil {
			t.Fatalf("%s licensed a theft flag", id)
		}
	}
	if err := flag(s, "f-theft", "case-theft"); err != nil {
		t.Fatalf("an approved theft case did not license a flag: %v", err)
	}
}

func TestV18_ReportAloneBlocksNothingAndShowsPending(t *testing.T) {
	s := registeredService(t)
	if _, err := s.AppendEvent(MSPPolice, EventInput{RecordID: "r-1", VID: testVID, Type: EventTheftReport, Reference: "PR/1"}); err != nil {
		t.Fatal(err)
	}
	disclosure, err := s.DisclosureFor("anyone", "", PurposeBuyer, testVID, "")
	if err != nil {
		t.Fatal(err)
	}
	if disclosure.Status.Stolen || !disclosure.Status.TheftReportPending {
		t.Fatalf("status after a report alone = %+v", disclosure.Status)
	}
	for _, f := range disclosure.Failed {
		if f.RuleID == "F2" {
			t.Fatalf("a theft report alone raised F2: %+v", f)
		}
	}
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "t-1", VID: testVID, Type: rules.EventTransfer, Pseudonym: "p2", EnvelopeAnchor: testAnchor2}); err != nil {
		t.Fatalf("a theft report alone blocked a transfer: %v", err)
	}
}

func TestV18_ClearedTheftIsShownToTheBuyer(t *testing.T) {
	s := registeredService(t)
	reportAndConfirm(t, s, "PR/1")
	if err := flag(s, "f-1", "PR/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvent(MSPPolice, EventInput{RecordID: "c-1", VID: testVID, Type: rules.EventTheftClear}); err != nil {
		t.Fatal(err)
	}
	disclosure, err := s.DisclosureFor("anyone", "", PurposeBuyer, testVID, "")
	if err != nil {
		t.Fatal(err)
	}
	if disclosure.Status.Stolen || !disclosure.Status.TheftCleared || disclosure.Status.TheftReportPending {
		t.Fatalf("status after flag and clear = %+v", disclosure.Status)
	}
}

func TestV18_RegistrationMustReproduceImportDescription(t *testing.T) {
	register := func(importDesc, regDesc string) error {
		s := NewService(NewMemoryRepository())
		if _, err := s.CreateImportAttestation(MSPCustoms, ImportAttestation{RecordID: "i-1", IdxChassis: testChassis, IdxEngine: testEngine, Source: "CUSTOMS", DescAnchor: importDesc}); err != nil {
			return err
		}
		_, err := s.RegisterVehicle(MSPDMT, RegistrationInput{VID: testVID, Pseudonym: testPseudo, IdxChassis: testChassis, IdxEngine: testEngine, Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor, DescAnchor: regDesc})
		return err
	}
	if err := register(testDesc, testAnchor2); err == nil || !strings.Contains(err.Error(), "description committed at import") {
		t.Fatalf("a registration with a different description err = %v", err)
	}
	if err := register(testDesc, ""); err == nil {
		t.Fatal("a registration that omits the committed description was accepted")
	}
	if err := register(testDesc, testDesc); err != nil {
		t.Fatalf("a matching description was refused: %v", err)
	}
	if err := register("", ""); err != nil {
		t.Fatalf("an import with no description no longer registers (backward compatibility): %v", err)
	}
	if err := register("", testDesc); err != nil {
		t.Fatalf("a registration description with no import description was refused: %v", err)
	}
	if err := register("NOT-HEX", ""); err == nil {
		t.Fatal("a malformed description anchor was accepted at import")
	}
}
