package experiment

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

type Config struct {
	Vehicles  int     `json:"vehicles"`
	Seed      int64   `json:"seed"`
	FraudRate float64 `json:"fraudRate"`
	// Noise is nil for Corpus A (conformance). Corpus B sets it; the manifest
	// then records the exact parameters because Manifest embeds Config.
	Noise *Noise `json:"noise,omitempty"`
}

type CanonicalVehicle struct {
	OwnerID string
	Vehicle rules.Vehicle
}

type Label struct {
	SourceVID string `json:"sourceVid"`
	RuleID    string `json:"ruleId"`
	Positive  bool   `json:"positive"`
}

type Manifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	Config        Config            `json:"config"`
	EventCount    int               `json:"eventCount"`
	RuleCount     int               `json:"ruleCount"`
	SHA256        map[string]string `json:"sha256"`
}

type Corpus struct {
	Config   Config
	Vehicles []CanonicalVehicle
	Labels   []Label
}

func digest(parts ...any) string {
	h := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(h, "%v|", part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func Generate(config Config) (*Corpus, error) {
	if config.Vehicles <= 0 {
		return nil, fmt.Errorf("experiment: vehicles must be positive")
	}
	if config.FraudRate <= 0 || config.FraudRate >= 0.5 {
		return nil, fmt.Errorf("experiment: fraudRate must be greater than 0 and below 0.5")
	}
	rng := rand.New(rand.NewSource(config.Seed))
	corpus := &Corpus{Config: config}
	var commitOrder uint64
	ownerF6a := map[string]bool{}
	ownerF6b := map[string]bool{}
	var previousChassis, previousEngine string

	for i := 0; i < config.Vehicles; i++ {
		ownerID := fmt.Sprintf("owner-%06d", i/2)
		if _, ok := ownerF6a[ownerID]; !ok {
			ownerF6a[ownerID] = rng.Float64() < config.FraudRate
			ownerF6b[ownerID] = rng.Float64() < config.FraudRate
		}
		vid := digest("vid", config.Seed, i)
		idxChassis := digest("chassis", config.Seed, i)
		idxEngine := digest("engine", config.Seed, i)
		positives := map[string]bool{}

		f4 := i > 0 && rng.Float64() < config.FraudRate
		if f4 {
			idxChassis, idxEngine = previousChassis, previousEngine
			positives["F4"] = true
		}
		f5 := rng.Float64() < config.FraudRate
		f1a := rng.Float64() < config.FraudRate || ownerF6a[ownerID]
		f1b := !f1a && rng.Float64() < config.FraudRate
		f1c := !f1a && !f1b && rng.Float64() < config.FraudRate
		f2 := rng.Float64() < config.FraudRate
		f3a := rng.Float64() < config.FraudRate
		f3b := !f3a && rng.Float64() < config.FraudRate
		f3c := !f3a && rng.Float64() < config.FraudRate
		f3d := !f3a && !f3c && rng.Float64() < config.FraudRate

		vehicle := rules.Vehicle{VID: vid}
		add := func(event rules.Event) {
			commitOrder++
			event.CommitOrder = commitOrder
			event.Seq = uint64(len(vehicle.Events))
			event.VID = vid
			vehicle.Events = append(vehicle.Events, event)
		}
		if !f5 {
			add(rules.Event{
				RecordID: "import-" + vid[:12], Type: rules.EventImport, Tier: rules.E2,
				IdxChassis: idxChassis, IdxEngine: idxEngine, Provenance: "CUSTOMS", AttestorID: "CustomsMSP",
			})
		} else {
			positives["F5"] = true
		}
		add(rules.Event{
			RecordID: "registration-" + vid[:12], Type: rules.EventRegistration, Tier: rules.E2,
			IdxChassis: idxChassis, IdxEngine: idxEngine, Provenance: map[bool]string{true: "NONE", false: "CUSTOMS"}[f5],
			AttestorID: "DMTMSP",
		})

		secondMileage := int64(62_000)
		if f1a {
			secondMileage = 45_000
			positives["F1a"] = true
		} else if f1b {
			secondMileage = 500_000
			positives["F1b"] = true
		} else if f1c {
			secondMileage = 50_000
			positives["F1c"] = true
		}
		add(rules.Event{RecordID: "mileage-a-" + vid[:12], Type: rules.EventMileage, Day: 0, Odometer: 50_000, Tier: rules.E2, VETLaneID: "lane-1"})
		add(rules.Event{RecordID: "mileage-b-" + vid[:12], Type: rules.EventMileage, Day: 365, Odometer: secondMileage, Tier: rules.E2, VETLaneID: "lane-1"})

		if f2 {
			add(rules.Event{RecordID: "theft-" + vid[:12], Type: rules.EventTheftFlag, Tier: rules.E2, AttestorID: "PoliceMSP"})
			positives["F2"] = true
		}
		add(rules.Event{RecordID: "transfer-" + vid[:12], Type: rules.EventTransfer, Tier: rules.E2, AttestorID: "DMTMSP"})
		if f2 {
			add(rules.Event{RecordID: "theft-clear-" + vid[:12], Type: rules.EventTheftClear, Tier: rules.E2, AttestorID: "PoliceMSP"})
		}

		accidentID := "accident-" + vid[:12]
		add(rules.Event{
			RecordID: accidentID, Type: rules.EventAccident, Day: 500, Tier: rules.E2,
			EvidenceAnchor: accidentID, EvidenceKind: "POLICE_REPORT", IncidentRef: rules.ReferencePresent,
		})
		if f3b {
			unclaimedID := "unclaimed-accident-" + vid[:12]
			add(rules.Event{
				RecordID: unclaimedID, Type: rules.EventAccident, Day: 501, Tier: rules.E2,
				EvidenceAnchor: unclaimedID, EvidenceKind: "POLICE_REPORT", IncidentRef: rules.ReferencePresent,
			})
			positives["F3b"] = true
		}
		claimAnchor, claimKind := accidentID, "POLICE_REPORT"
		if f3a {
			claimAnchor, claimKind = "", ""
			positives["F3a"] = true
			positives["F3b"] = true // the recorded accident also has no corresponding claim
		}
		sharedAttestor := "assessor-" + vid[:8]
		if ownerF6b[ownerID] {
			sharedAttestor = "serial-assessor-" + ownerID
			positives["F6b"] = true
		}
		add(rules.Event{
			RecordID: "claim-a-" + vid[:12], Type: rules.EventClaim, Day: 505, Tier: rules.E2,
			EvidenceAnchor: claimAnchor, EvidenceKind: claimKind, IncidentRef: refState(claimAnchor),
			InsurerID: "insurer-a", AttestorID: sharedAttestor,
		})
		if f3c {
			add(rules.Event{
				RecordID: "claim-b-" + vid[:12], Type: rules.EventClaim, Day: 510, Tier: rules.E2,
				EvidenceAnchor: accidentID, EvidenceKind: "POLICE_REPORT", IncidentRef: rules.ReferencePresent,
				InsurerID: "insurer-b", AttestorID: sharedAttestor,
			})
			positives["F3c"] = true
		}
		if f3d {
			secondAccident := "accident-b-" + vid[:12]
			add(rules.Event{RecordID: secondAccident, Type: rules.EventAccident, Day: 512, Tier: rules.E2, EvidenceAnchor: secondAccident, IncidentRef: rules.ReferencePresent})
			add(rules.Event{
				RecordID: "claim-b-" + vid[:12], Type: rules.EventClaim, Day: 515, Tier: rules.E2,
				EvidenceAnchor: secondAccident, EvidenceKind: "POLICE_REPORT", IncidentRef: rules.ReferencePresent,
				InsurerID: "insurer-b", AttestorID: sharedAttestor,
			})
			positives["F3d"] = true
		}
		if ownerF6a[ownerID] {
			positives["F6a"] = true
		}

		previousChassis, previousEngine = idxChassis, idxEngine
		corpus.Vehicles = append(corpus.Vehicles, CanonicalVehicle{OwnerID: ownerID, Vehicle: vehicle})
		for _, descriptor := range rules.Registry {
			corpus.Labels = append(corpus.Labels, Label{SourceVID: vid, RuleID: descriptor.ID, Positive: positives[descriptor.ID]})
		}
	}
	// Independent per-vehicle injections can themselves form a genuine owner-level
	// pattern. Promote those pairs in the hidden label track so they are not
	// incorrectly counted as F6 false positives.
	labelsByKey := map[string]*Label{}
	for i := range corpus.Labels {
		label := &corpus.Labels[i]
		labelsByKey[label.SourceVID+":"+label.RuleID] = label
	}
	byOwner := map[string][]string{}
	for _, vehicle := range corpus.Vehicles {
		byOwner[vehicle.OwnerID] = append(byOwner[vehicle.OwnerID], vehicle.Vehicle.VID)
	}
	for _, vids := range byOwner {
		var rollbackCount int
		for _, vid := range vids {
			if labelsByKey[vid+":F1a"].Positive {
				rollbackCount++
			}
		}
		if rollbackCount >= 2 {
			for _, vid := range vids {
				labelsByKey[vid+":F6a"].Positive = true
			}
		}
	}
	if config.Noise != nil {
		applyNoise(corpus, *config.Noise)
	}
	return corpus, nil
}

func refState(anchor string) rules.ReferenceState {
	if anchor == "" {
		return rules.ReferenceMissing
	}
	return rules.ReferencePresent
}

type publicVehicle struct {
	Vehicle rules.Vehicle `json:"vehicle"`
}

type ownerRecord struct {
	SourceVID string `json:"sourceVid"`
	OwnerID   string `json:"ownerId"`
}

func encodeJSONLines(values any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	switch typed := values.(type) {
	case []CanonicalVehicle:
		for _, value := range typed {
			if err := encoder.Encode(publicVehicle{Vehicle: value.Vehicle}); err != nil {
				return nil, err
			}
		}
	case []ownerRecord:
		for _, value := range typed {
			if err := encoder.Encode(value); err != nil {
				return nil, err
			}
		}
	case []Label:
		for _, value := range typed {
			if err := encoder.Encode(value); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("experiment: unsupported JSONL value %T", values)
	}
	return buffer.Bytes(), nil
}

func (c *Corpus) Write(directory string) (*Manifest, error) {
	vehicles := append([]CanonicalVehicle(nil), c.Vehicles...)
	sort.Slice(vehicles, func(i, j int) bool { return vehicles[i].Vehicle.VID < vehicles[j].Vehicle.VID })
	labels := append([]Label(nil), c.Labels...)
	sort.Slice(labels, func(i, j int) bool {
		if labels[i].SourceVID != labels[j].SourceVID {
			return labels[i].SourceVID < labels[j].SourceVID
		}
		return labels[i].RuleID < labels[j].RuleID
	})
	owners := make([]ownerRecord, 0, len(vehicles))
	eventCount := 0
	for _, vehicle := range vehicles {
		owners = append(owners, ownerRecord{SourceVID: vehicle.Vehicle.VID, OwnerID: vehicle.OwnerID})
		eventCount += len(vehicle.Vehicle.Events)
	}
	files := map[string][]byte{}
	var err error
	if files["events.jsonl"], err = encodeJSONLines(vehicles); err != nil {
		return nil, err
	}
	if files["owners.jsonl"], err = encodeJSONLines(owners); err != nil {
		return nil, err
	}
	if files["labels.jsonl"], err = encodeJSONLines(labels); err != nil {
		return nil, err
	}
	manifest := &Manifest{
		SchemaVersion: 1, Config: c.Config, EventCount: eventCount,
		RuleCount: len(rules.Registry), SHA256: map[string]string{},
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, err
	}
	for _, name := range []string{"events.jsonl", "owners.jsonl", "labels.jsonl"} {
		sum := sha256.Sum256(files[name])
		manifest.SHA256[name] = hex.EncodeToString(sum[:])
		if err := os.WriteFile(filepath.Join(directory, name), files[name], 0o644); err != nil {
			return nil, err
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0o644); err != nil {
		return nil, err
	}
	return manifest, nil
}

func readJSONLines[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var values []T
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var value T
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, scanner.Err()
}

func Read(directory string) (*Corpus, error) {
	manifestData, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != 1 {
		return nil, fmt.Errorf("experiment: unsupported corpus schema version %d", manifest.SchemaVersion)
	}
	for _, name := range []string{"events.jsonl", "owners.jsonl", "labels.jsonl"} {
		expected, ok := manifest.SHA256[name]
		if !ok {
			return nil, fmt.Errorf("experiment: manifest has no SHA-256 for %s", name)
		}
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return nil, err
		}
		actual := sha256.Sum256(data)
		if hex.EncodeToString(actual[:]) != expected {
			return nil, fmt.Errorf("experiment: SHA-256 mismatch for %s", name)
		}
	}
	public, err := readJSONLines[publicVehicle](filepath.Join(directory, "events.jsonl"))
	if err != nil {
		return nil, err
	}
	owners, err := readJSONLines[ownerRecord](filepath.Join(directory, "owners.jsonl"))
	if err != nil {
		return nil, err
	}
	labels, err := readJSONLines[Label](filepath.Join(directory, "labels.jsonl"))
	if err != nil {
		return nil, err
	}
	ownerByVID := map[string]string{}
	for _, owner := range owners {
		ownerByVID[owner.SourceVID] = owner.OwnerID
	}
	corpus := &Corpus{Config: manifest.Config, Labels: labels}
	for _, item := range public {
		owner, ok := ownerByVID[item.Vehicle.VID]
		if !ok {
			return nil, fmt.Errorf("experiment: missing owner truth for %s", item.Vehicle.VID)
		}
		corpus.Vehicles = append(corpus.Vehicles, CanonicalVehicle{OwnerID: owner, Vehicle: item.Vehicle})
	}
	if len(corpus.Vehicles) != manifest.Config.Vehicles {
		return nil, fmt.Errorf("experiment: manifest declares %d vehicles, read %d", manifest.Config.Vehicles, len(corpus.Vehicles))
	}
	if len(corpus.Labels) != manifest.Config.Vehicles*manifest.RuleCount {
		return nil, fmt.Errorf("experiment: manifest implies %d labels, read %d", manifest.Config.Vehicles*manifest.RuleCount, len(corpus.Labels))
	}
	return corpus, nil
}
