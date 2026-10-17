package experiment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

func testCorpus(t *testing.T) *Corpus {
	t.Helper()
	corpus, err := Generate(Config{Vehicles: 1000, Seed: 42, FraudRate: 0.08})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return corpus
}

func TestCorpusReadRejectsTamperedInput(t *testing.T) {
	corpus := testCorpus(t)
	directory := t.TempDir()
	if _, err := corpus.Write(directory); err != nil {
		t.Fatalf("Write: %v", err)
	}
	path := filepath.Join(directory, "events.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := file.WriteString("{}\n"); err != nil {
		t.Fatalf("tamper corpus: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close corpus: %v", err)
	}
	_, err = Read(directory)
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("Read tampered corpus = %v, want SHA-256 mismatch", err)
	}
}

func TestCorpusRoundTripIsDeterministic(t *testing.T) {
	corpus := testCorpus(t)
	dirA, dirB := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
	manifestA, err := corpus.Write(dirA)
	if err != nil {
		t.Fatalf("Write A: %v", err)
	}
	manifestB, err := corpus.Write(dirB)
	if err != nil {
		t.Fatalf("Write B: %v", err)
	}
	for name, hash := range manifestA.SHA256 {
		if manifestB.SHA256[name] != hash {
			t.Errorf("%s changed across identical writes", name)
		}
	}
	read, err := Read(dirA)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(read.Vehicles) != len(corpus.Vehicles) || len(read.Labels) != len(corpus.Labels) {
		t.Fatal("corpus round trip changed record counts")
	}
}

func TestProfilesChangeStructureNotOnlyMetadata(t *testing.T) {
	corpus := testCorpus(t)
	p0, _ := Transform(corpus, rules.ProfileP0)
	p1, _ := Transform(corpus, rules.ProfileP1)
	p2, _ := Transform(corpus, rules.ProfileP2)
	p3, _ := Transform(corpus, rules.ProfileP3)
	if len(p0.Ledger.OwnerVehicles) == 0 || p1.Ledger.OwnerVehicles != nil {
		t.Fatal("owner grouping was not limited to P0")
	}
	if len(p2.Ledger.Vehicles) != len(corpus.Vehicles) {
		t.Fatal("P2 should retain vehicle grouping")
	}
	if len(p3.Ledger.Vehicles) <= len(corpus.Vehicles) {
		t.Fatal("P3 should split vehicle histories into event-level records")
	}
	for _, vehicle := range p3.Ledger.Vehicles {
		if len(vehicle.Events) != 1 {
			t.Fatal("P3 exposed a multi-event vehicle history")
		}
	}
	redacted := false
	for _, vehicle := range p2.Ledger.Vehicles {
		for _, event := range vehicle.Events {
			if event.Type == rules.EventClaim && event.IncidentRef == rules.ReferenceRedacted {
				redacted = true
			}
		}
	}
	if !redacted {
		t.Fatal("P2 did not structurally redact incident references")
	}
}

func findCell(t *testing.T, report *Report, ruleID string) Cell {
	t.Helper()
	for _, cell := range report.Cells {
		if cell.RuleID == ruleID {
			if cell.Positive == 0 {
				t.Fatalf("generator produced no positive cases for %s", ruleID)
			}
			return cell
		}
	}
	t.Fatalf("no cell for %s", ruleID)
	return Cell{}
}

func TestMeasuredBreakpointsComeFromTransformedViews(t *testing.T) {
	corpus := testCorpus(t)
	reports := map[rules.Profile]*Report{}
	for _, profile := range []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3} {
		report, err := Run(corpus, profile)
		if err != nil {
			t.Fatalf("Run %s: %v", profile, err)
		}
		reports[profile] = report
	}

	for _, ruleID := range []string{"F1a", "F2", "F3d"} {
		if findCell(t, reports[rules.ProfileP2], ruleID).Recall() == 0 {
			t.Errorf("%s unexpectedly collapsed before P3", ruleID)
		}
		if findCell(t, reports[rules.ProfileP3], ruleID).Recall() != 0 {
			t.Errorf("%s retained detection after vehicle histories were split", ruleID)
		}
	}
	for _, ruleID := range []string{"F3c"} {
		if findCell(t, reports[rules.ProfileP1], ruleID).Recall() == 0 {
			t.Errorf("%s unexpectedly failed at P1", ruleID)
		}
		if findCell(t, reports[rules.ProfileP2], ruleID).Recall() != 0 {
			t.Errorf("%s retained detection after incident references were redacted", ruleID)
		}
	}
	if findCell(t, reports[rules.ProfileP3], "F3b").Recall() != 0 {
		t.Error("F3b treated split event records as a complete vehicle history")
	}
	for _, profile := range []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3} {
		if findCell(t, reports[profile], "F3a").Recall() == 0 {
			t.Errorf("F3a lost single-record missing-anchor detection at %s", profile)
		}
	}
	for _, ruleID := range []string{"F4", "F5"} {
		if findCell(t, reports[rules.ProfileP3], ruleID).Recall() == 0 {
			t.Errorf("%s control did not survive INDEX-only P3", ruleID)
		}
	}
	for _, ruleID := range []string{"F6a", "F6b"} {
		if findCell(t, reports[rules.ProfileP0], ruleID).Recall() == 0 {
			t.Errorf("%s was not detected with owner grouping", ruleID)
		}
		if findCell(t, reports[rules.ProfileP1], ruleID).Recall() != 0 {
			t.Errorf("%s remained detectable after owner grouping was removed", ruleID)
		}
	}
}

// H5: both exposure metrics are non-increasing from P0 to P3, and the
// resolvable-reference count separates P1 from P2.
func TestExposureMonotone(t *testing.T) {
	corpus := testCorpus(t)
	var previous *Exposure
	for _, profile := range []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3} {
		view, err := Transform(corpus, profile)
		if err != nil {
			t.Fatalf("Transform %s: %v", profile, err)
		}
		exposure := MeasureExposure(corpus, view)
		if exposure.OwnerPairs == 0 {
			t.Fatalf("%s: no owner pairs in ground truth", profile)
		}
		if previous != nil {
			if exposure.Mean > previous.Mean || exposure.Max > previous.Max {
				t.Errorf("%s: history exposure increased (%.2f > %.2f)", profile, exposure.Mean, previous.Mean)
			}
			if exposure.Linkability > previous.Linkability {
				t.Errorf("%s: owner linkability increased", profile)
			}
			if exposure.ResolvableRefs > previous.ResolvableRefs {
				t.Errorf("%s: resolvable references increased", profile)
			}
		}
		switch profile {
		case rules.ProfileP0:
			if exposure.Linkability != 1 {
				t.Errorf("P0 linkability = %.3f, want 1", exposure.Linkability)
			}
		case rules.ProfileP1:
			if exposure.Linkability != 0 || exposure.ResolvableRefs == 0 {
				t.Errorf("P1 linkability=%.3f resolvable=%d", exposure.Linkability, exposure.ResolvableRefs)
			}
		case rules.ProfileP2:
			if exposure.ResolvableRefs != 0 {
				t.Errorf("P2 still resolves %d incident references", exposure.ResolvableRefs)
			}
		case rules.ProfileP3:
			if exposure.Mean != 1 || exposure.Max != 1 {
				t.Errorf("P3 exposure mean=%.2f max=%d, want 1", exposure.Mean, exposure.Max)
			}
		}
		if exposure.QuasiLinkability == 0 {
			t.Errorf("%s: quasi-identifier linkability should be non-zero (serial assessors)", profile)
		}
		e := exposure
		previous = &e
	}
}

// Measured decidability class comes from the transformed view, not the
// declared capability: F3b is partial at P2, F3c is suppressed there.
func TestMeasuredClassPerCell(t *testing.T) {
	corpus := testCorpus(t)
	report, err := Run(corpus, rules.ProfileP2)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := findCell(t, report, "F3b").Class(); got != ClassPartial {
		t.Errorf("F3b@P2 class = %s, want partial", got)
	}
	// F3c at P2: every referenced claim is redacted, so detection collapses
	// (recall 0), but vehicles whose claims never carried an anchor are still
	// trivially decidable. Measured class: partial (corrected in the
	// prediction table before the freeze, 2 Oct 2026).
	if f3c := findCell(t, report, "F3c"); f3c.Recall() != 0 || f3c.Class() != ClassPartial {
		t.Errorf("F3c@P2 recall=%.3f class=%s, want recall 0 and class partial", f3c.Recall(), f3c.Class())
	}
	if got := findCell(t, report, "F3a").Class(); got != ClassPartial {
		t.Errorf("F3a@P2 class = %s, want partial (redacted references are gaps)", got)
	}
	if got := findCell(t, report, "F1a").Class(); got != ClassAvailable {
		t.Errorf("F1a@P2 class = %s, want available", got)
	}
	f6 := findCell(t, report, "F6a")
	if f6.Suppressed != f6.Positive+f6.Negative {
		t.Errorf("F6a@P2 declared-suppressed count = %d, want every vehicle", f6.Suppressed)
	}
	if findCell(t, report, "F1a").Suppressed != 0 {
		t.Error("F1a@P2 should not be declared suppressed")
	}
}

func TestVerifyFrozenDetectsEdit(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"rules", "experiment", "eval"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("rules/a.go", "package rules\n")
	write("rules/a_test.go", "ignored\n")
	write("experiment/b.go", "package experiment\n")
	write("eval/predictions.md", "| F1a | ASSET | ✓ |\n")
	hashes, err := HashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 3 {
		t.Fatalf("hashed %d files, want 3 (tests excluded)", len(hashes))
	}
	manifest := filepath.Join(root, "MANIFEST.sha256")
	if err := WriteManifest(manifest, hashes); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFrozen(root, manifest); err != nil {
		t.Fatalf("fresh manifest failed verification: %v", err)
	}
	write("eval/predictions.md", "| F1a | ASSET | ✗ |\n")
	if err := VerifyFrozen(root, manifest); err == nil || !strings.Contains(err.Error(), "modified eval/predictions.md") {
		t.Fatalf("edit not detected: %v", err)
	}
	write("eval/predictions.md", "| F1a | ASSET | ✓ |\n")
	write("rules/c.go", "package rules\n")
	if err := VerifyFrozen(root, manifest); err == nil || !strings.Contains(err.Error(), "added rules/c.go") {
		t.Fatalf("addition not detected: %v", err)
	}
}

func noisyCorpus(t *testing.T) *Corpus {
	t.Helper()
	noise := DefaultNoise
	corpus, err := Generate(Config{Vehicles: 1000, Seed: 42, FraudRate: 0.08, Noise: &noise})
	if err != nil {
		t.Fatalf("Generate noisy: %v", err)
	}
	return corpus
}

// Corpus B must keep Corpus A's hidden labels and manifest-record the noise.
func TestNoiseKeepsLabelsAndRecordsParameters(t *testing.T) {
	clean, noisy := testCorpus(t), noisyCorpus(t)
	if len(clean.Labels) != len(noisy.Labels) {
		t.Fatal("noise changed the label count")
	}
	for i := range clean.Labels {
		if clean.Labels[i] != noisy.Labels[i] {
			t.Fatalf("noise changed label %+v -> %+v", clean.Labels[i], noisy.Labels[i])
		}
	}
	changed := 0
	for i := range clean.Vehicles {
		a, b := clean.Vehicles[i].Vehicle, noisy.Vehicles[i].Vehicle
		if len(a.Events) != len(b.Events) {
			changed++
			continue
		}
		for j := range a.Events {
			if a.Events[j] != b.Events[j] {
				changed++
				break
			}
		}
	}
	if changed < 200 {
		t.Fatalf("noise touched only %d of 1000 vehicles", changed)
	}
	manifest, err := noisy.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Config.Noise == nil || *manifest.Config.Noise != DefaultNoise {
		t.Fatal("manifest does not record the noise parameters")
	}
	cleanManifest, err := clean.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cleanManifest.Config.Noise != nil {
		t.Fatal("Corpus A manifest must not carry noise parameters")
	}
}

// H4: noise lowers precision and/or recall inside decidable cells, but the
// structurally predicted suppression points do not move.
func TestNoiseLowersPrecisionNotSuppression(t *testing.T) {
	clean, noisy := testCorpus(t), noisyCorpus(t)
	degraded := false
	for _, profile := range []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3} {
		a, err := Run(clean, profile)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Run(noisy, profile)
		if err != nil {
			t.Fatal(err)
		}
		for i := range a.Cells {
			ca, cb := a.Cells[i], b.Cells[i]
			if ca.RuleID != cb.RuleID {
				t.Fatalf("cell order differs: %s vs %s", ca.RuleID, cb.RuleID)
			}
			if ca.Class() != cb.Class() {
				t.Errorf("%s@%s decidability class moved under noise: %s -> %s", ca.RuleID, profile, ca.Class(), cb.Class())
			}
			if cb.Precision() < ca.Precision() || cb.Recall() < ca.Recall() {
				degraded = true
			}
		}
	}
	if !degraded {
		t.Fatal("noise did not lower precision or recall anywhere")
	}
	b, _ := Run(noisy, rules.ProfileP1)
	if findCell(t, b, "F1a").Precision() >= 1 {
		t.Error("out-of-order and clerical readings should produce F1a false positives at P1")
	}
}

// Corpus C: every expected rule fires on the expected vehicle at P0, and no
// scenario is labelled positive for a rule that does not fire there.
func TestScenariosFireAtP0(t *testing.T) {
	scenarios := Scenarios()
	if len(scenarios) < 20 {
		t.Fatalf("only %d scenarios", len(scenarios))
	}
	seen := map[string]bool{}
	for _, s := range scenarios {
		if seen[s.ID] {
			t.Fatalf("duplicate scenario id %s", s.ID)
		}
		seen[s.ID] = true
		report, err := Run(s.Corpus(), rules.ProfileP0)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		for _, cell := range report.Cells {
			if cell.FN > 0 {
				t.Errorf("%s: %s expected on %d vehicle(s), missed %d", s.ID, cell.RuleID, cell.Positive, cell.FN)
			}
		}
	}
	// C19 is the documented false positive; C21 is the documented gap.
	for _, s := range scenarios {
		report, _ := Run(s.Corpus(), rules.ProfileP0)
		switch s.ID {
		case "C19-honest-on-the-spot-claim":
			if findCellAny(report, "F3a").FP != 1 {
				t.Error("C19 should record exactly one F3a false positive")
			}
		case "C21-e0-legacy-mileage":
			if findCellAny(report, "F1a").GapOnly != 1 {
				t.Error("C21 should record an F1a gap, not a pass")
			}
		}
	}
}

func findCellAny(report *Report, ruleID string) Cell {
	for _, cell := range report.Cells {
		if cell.RuleID == ruleID {
			return cell
		}
	}
	return Cell{}
}
