package baseline

import (
	"strings"
	"testing"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

func sampleLog(t *testing.T) *Log {
	t.Helper()
	l := New("DMT", Agencies, 2)
	events := []rules.Event{
		{RecordID: "import-1", Type: rules.EventImport, VID: "v1", IdxChassis: "c1", IdxEngine: "e1", Provenance: "CUSTOMS", Tier: rules.E2},
		{RecordID: "reg-1", Type: rules.EventRegistration, VID: "v1", P: "owner-a", IdxChassis: "c1", IdxEngine: "e1", Provenance: "CUSTOMS", Tier: rules.E2},
		{RecordID: "mileage-1", Type: rules.EventMileage, VID: "v1", P: "owner-a", Odometer: 50000, Day: 0, Tier: rules.E2},
		{RecordID: "mileage-2", Type: rules.EventMileage, VID: "v1", P: "owner-a", Odometer: 40000, Day: 365, Tier: rules.E2},
	}
	for i, e := range events {
		e.Seq, e.CommitOrder = uint64(i), uint64(i+1)
		if _, err := l.Append(AgencyFor(e.Type), e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	return l
}

func TestLogVerifiesAndSharesRules(t *testing.T) {
	l := sampleLog(t)
	if err := l.Verify(); err != nil {
		t.Fatalf("fresh log failed verification: %v", err)
	}
	if len(l.Checkpoints) != 2 {
		t.Fatalf("checkpoints = %d, want 2", len(l.Checkpoints))
	}
	ledger := l.Ledger(rules.ProfileP1)
	findings := rules.DecideAll(ledger, ledger.Vehicles["v1"], rules.ProfileP1)
	flagged := false
	for _, f := range findings {
		if f.RuleID == "F1a" && f.Flagged() {
			flagged = true
		}
	}
	if !flagged {
		t.Fatal("rule package did not flag the rollback over the baseline log")
	}
	if _, err := l.Append("Hacker", rules.Event{Type: rules.EventTransfer}); err == nil {
		t.Fatal("unknown agency was allowed to write")
	}
}

func TestLogDetectsRewriteAndMissingQuorum(t *testing.T) {
	l := sampleLog(t)
	l.Entries[3].Event.Odometer = 60000 // the operator "corrects" the rollback
	if err := l.Verify(); err == nil || !strings.Contains(err.Error(), "does not match its hash") {
		t.Fatalf("rewrite not detected: %v", err)
	}

	l = sampleLog(t)
	l.Entries = l.Entries[:2] // operator presents a shorter history than the cosigned head
	if err := l.Verify(); err == nil || !strings.Contains(err.Error(), "split view or rewrite") {
		t.Fatalf("truncation against cosigned checkpoint not detected: %v", err)
	}

	l = sampleLog(t)
	cp := &l.Checkpoints[1]
	for _, agency := range []string{"Police", "VET", "Customs"} {
		delete(cp.Cosigs, agency)
	}
	if err := l.Verify(); err == nil || !strings.Contains(err.Error(), "quorum") {
		t.Fatalf("missing quorum not detected: %v", err)
	}
}
