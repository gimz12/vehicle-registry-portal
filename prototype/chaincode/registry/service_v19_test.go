package registry

import (
	"strings"
	"testing"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

// A report must postdate the last transfer or clearance to be confirmed; an
// unconfirmed report can be withdrawn and does not survive a sale.

func report(t *testing.T, s *Service, recordID, ref string) {
	t.Helper()
	if _, err := s.AppendEvent(MSPPolice, EventInput{RecordID: recordID, VID: testVID, Type: EventTheftReport, Reference: ref}); err != nil {
		t.Fatal(err)
	}
}

func clearTheft(s *Service, recordID string) error {
	_, err := s.AppendEvent(MSPPolice, EventInput{RecordID: recordID, VID: testVID, Type: rules.EventTheftClear})
	return err
}

func buyerStatus(t *testing.T, s *Service) VehicleStatus {
	t.Helper()
	d, err := s.DisclosureFor("anyone", "", PurposeBuyer, testVID, "")
	if err != nil {
		t.Fatal(err)
	}
	return d.Status
}

func TestV19_OldReportCannotBeConfirmedAfterTransfer(t *testing.T) {
	s := registeredService(t)
	report(t, s, "r-1", "PR/OLD")
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "t-1", VID: testVID, Type: rules.EventTransfer, Pseudonym: "p2", EnvelopeAnchor: testAnchor2}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "c-1", VID: testVID, Type: EventOwnerConfirmed, Reference: "PR/OLD"}); err == nil || !strings.Contains(err.Error(), "since its last transfer") {
		t.Fatalf("a report from before the sale was confirmed: err = %v", err)
	}
	if err := flag(s, "f-1", "PR/OLD"); err == nil {
		t.Fatal("a report from before the sale led to a flag")
	}
}

func TestV19_UnconfirmedReportIsWithdrawnOrEndsAtTransfer(t *testing.T) {
	s := registeredService(t)
	if err := clearTheft(s, "c-0"); err == nil {
		t.Fatal("a clearance with no flag and no report was accepted")
	}
	report(t, s, "r-1", "PR/1")
	if !buyerStatus(t, s).TheftReportPending {
		t.Fatal("report not shown as pending")
	}
	if err := clearTheft(s, "c-1"); err != nil {
		t.Fatalf("an unconfirmed report could not be withdrawn: %v", err)
	}
	if st := buyerStatus(t, s); st.TheftReportPending || st.TheftCleared || st.Stolen {
		t.Fatalf("status after a withdrawn report = %+v (a withdrawal is not a cleared theft)", st)
	}
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "c-2", VID: testVID, Type: EventOwnerConfirmed, Reference: "PR/1"}); err == nil {
		t.Fatal("a withdrawn report was confirmed")
	}
	report(t, s, "r-2", "PR/2")
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "t-1", VID: testVID, Type: rules.EventTransfer, Pseudonym: "p2", EnvelopeAnchor: testAnchor2}); err != nil {
		t.Fatal(err)
	}
	if buyerStatus(t, s).TheftReportPending {
		t.Fatal("a report against the previous owner still shows after the sale")
	}
}

func TestV19_FlaggedThenClearedStillShownAfterReconfirmation(t *testing.T) {
	s := registeredService(t)
	reportAndConfirm(t, s, "PR/1")
	if err := flag(s, "f-1", "PR/1"); err != nil {
		t.Fatal(err)
	}
	if err := clearTheft(s, "c-1"); err != nil {
		t.Fatal(err)
	}
	// The same report number can be reported, confirmed and flagged again later.
	report(t, s, "r-again", "PR/1")
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "confirm-again", VID: testVID, Type: EventOwnerConfirmed, Reference: "PR/1"}); err != nil {
		t.Fatalf("a fresh report could not be confirmed: %v", err)
	}
	if err := flag(s, "f-2", "PR/1"); err != nil {
		t.Fatalf("a freshly confirmed report could not be flagged: %v", err)
	}
}

func TestV19_TheftCaseNeedsCurrentOwnerAnchor(t *testing.T) {
	s := registeredService(t)
	if err := s.Repo.PutReveal(&RevealRequest{ID: "case-no-anchor", VID: testVID, GroundCode: "THEFT_INVESTIGATION", Status: RevealApproved}); err != nil {
		t.Fatal(err)
	}
	if err := flag(s, "f-1", "case-no-anchor"); err == nil {
		t.Fatal("a case with no owner anchor licensed a flag")
	}
}

func TestV19_TransferNeedsNewSealedRecord(t *testing.T) {
	s := registeredService(t)
	if _, err := s.AppendEvent(MSPDMT, EventInput{RecordID: "t-1", VID: testVID, Type: rules.EventTransfer, Pseudonym: "p2", EnvelopeAnchor: testAnchor}); err == nil || !strings.Contains(err.Error(), "new sealed owner record") {
		t.Fatalf("a transfer reusing the current anchor err = %v", err)
	}
}
