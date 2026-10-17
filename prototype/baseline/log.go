// Package baseline runs the same rule package over a signed, append-only
// centralised log. Agencies sign their entries and cosign periodic checkpoints,
// which stops the operator showing different histories to different readers.
//
// Limitation: hash chain with cosigned heads; a Merkle tree would add
// third-party inclusion proofs.
package baseline

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

type Entry struct {
	Index  uint64      `json:"index"`
	Prev   string      `json:"prev"`
	Hash   string      `json:"hash"`
	Agency string      `json:"agency"`
	Sig    string      `json:"sig"`
	Event  rules.Event `json:"event"`
}

type Checkpoint struct {
	Size   uint64            `json:"size"`
	Head   string            `json:"head"`
	LogSig string            `json:"logSig"`
	Cosigs map[string]string `json:"cosigs"`
}

type Log struct {
	Operator        string
	Agencies        []string
	Quorum          int
	CheckpointEvery int
	Entries         []Entry
	Checkpoints     []Checkpoint

	keys map[string]ed25519.PrivateKey
}

// New builds a log with agency keys seeded from the agency name, so output is reproducible.
func New(operator string, agencies []string, every int) *Log {
	l := &Log{Operator: operator, Agencies: append([]string(nil), agencies...), CheckpointEvery: every,
		Quorum: len(agencies)/2 + 1, keys: map[string]ed25519.PrivateKey{}}
	for _, name := range append([]string{operator}, agencies...) {
		seed := sha256.Sum256([]byte("baseline-key|" + name))
		l.keys[name] = ed25519.NewKeyFromSeed(seed[:])
	}
	return l
}

func (l *Log) head() string {
	if len(l.Entries) == 0 {
		return hex.EncodeToString(make([]byte, 32))
	}
	return l.Entries[len(l.Entries)-1].Hash
}

func entryHash(prev string, event rules.Event) (string, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write([]byte("|"))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func checkpointMessage(size uint64, head string) []byte {
	return []byte(fmt.Sprintf("checkpoint|%d|%s", size, head))
}

// Append commits one event signed by the writing agency and, at the checkpoint
// interval, publishes a head cosigned by every agency.
func (l *Log) Append(agency string, event rules.Event) (Entry, error) {
	key, ok := l.keys[agency]
	if !ok || !contains(l.Agencies, agency) {
		return Entry{}, fmt.Errorf("baseline: unknown writing agency %q", agency)
	}
	hash, err := entryHash(l.head(), event)
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{Index: uint64(len(l.Entries)), Prev: l.head(), Hash: hash, Agency: agency,
		Sig: hex.EncodeToString(ed25519.Sign(key, []byte(hash))), Event: event}
	l.Entries = append(l.Entries, entry)
	if l.CheckpointEvery > 0 && len(l.Entries)%l.CheckpointEvery == 0 {
		l.Checkpoints = append(l.Checkpoints, l.checkpoint())
	}
	return entry, nil
}

func (l *Log) checkpoint() Checkpoint {
	size, head := uint64(len(l.Entries)), l.head()
	msg := checkpointMessage(size, head)
	cp := Checkpoint{Size: size, Head: head, LogSig: hex.EncodeToString(ed25519.Sign(l.keys[l.Operator], msg)), Cosigs: map[string]string{}}
	for _, agency := range l.Agencies {
		cp.Cosigs[agency] = hex.EncodeToString(ed25519.Sign(l.keys[agency], msg))
	}
	return cp
}

// Verify recomputes the chain, checks every entry signature, and requires each
// checkpoint to match the chain and carry a quorum of valid cosignatures.
func (l *Log) Verify() error {
	prev := hex.EncodeToString(make([]byte, 32))
	heads := map[uint64]string{}
	for i, e := range l.Entries {
		if e.Index != uint64(i) || e.Prev != prev {
			return fmt.Errorf("baseline: entry %d breaks the chain", i)
		}
		hash, err := entryHash(prev, e.Event)
		if err != nil {
			return err
		}
		if hash != e.Hash {
			return fmt.Errorf("baseline: entry %d content does not match its hash", i)
		}
		key, ok := l.keys[e.Agency]
		sig, decodeErr := hex.DecodeString(e.Sig)
		if !ok || decodeErr != nil || !ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(e.Hash), sig) {
			return fmt.Errorf("baseline: entry %d carries an invalid %s signature", i, e.Agency)
		}
		prev = e.Hash
		heads[uint64(i+1)] = e.Hash
	}
	for _, cp := range l.Checkpoints {
		if heads[cp.Size] != cp.Head {
			return fmt.Errorf("baseline: checkpoint at size %d does not match the chain (split view or rewrite)", cp.Size)
		}
		msg := checkpointMessage(cp.Size, cp.Head)
		valid := 0
		for agency, sigHex := range cp.Cosigs {
			key, ok := l.keys[agency]
			sig, err := hex.DecodeString(sigHex)
			if ok && err == nil && ed25519.Verify(key.Public().(ed25519.PublicKey), msg, sig) {
				valid++
			}
		}
		if valid < l.Quorum {
			return fmt.Errorf("baseline: checkpoint at size %d has %d valid cosignatures, quorum is %d", cp.Size, valid, l.Quorum)
		}
	}
	return nil
}

// Ledger regroups the flat log into the rule engine's view under a profile, as
// the chaincode's buildLedger does.
// Limitation: write-off and retirement have no event here, so those flags stay false.
func (l *Log) Ledger(profile rules.Profile) *rules.Ledger {
	ledger := &rules.Ledger{Vehicles: map[string]*rules.Vehicle{}, OwnerLink: rules.ReferenceRedacted}
	if profile.Exposes(rules.CapabilityOwner) {
		ledger.OwnerVehicles = map[string][]string{}
		ledger.OwnerLink = rules.ReferencePresent
	}
	assetLink := rules.ReferencePresent
	if !profile.Exposes(rules.CapabilityAsset) {
		assetLink = rules.ReferenceRedacted
	}
	for _, e := range l.Entries {
		v, ok := ledger.Vehicles[e.Event.VID]
		if !ok {
			v = &rules.Vehicle{VID: e.Event.VID, AssetLink: assetLink}
			ledger.Vehicles[e.Event.VID] = v
		}
		switch e.Event.Type {
		case rules.EventTheftFlag:
			v.Stolen = true
		case rules.EventTheftClear:
			v.Stolen = false
		}
		v.Events = append(v.Events, e.Event)
	}
	if ledger.OwnerVehicles != nil {
		for vid, v := range ledger.Vehicles {
			for _, e := range v.Events {
				if e.P != "" {
					ledger.OwnerVehicles[e.P] = append(ledger.OwnerVehicles[e.P], vid)
					break
				}
			}
		}
		for owner := range ledger.OwnerVehicles {
			sort.Strings(ledger.OwnerVehicles[owner])
		}
	}
	return ledger
}

// AgencyFor maps an event type to its writing agency, as authoriseAndGradeEvent does.
func AgencyFor(t rules.EventType) string {
	switch t {
	case rules.EventImport:
		return "Customs"
	case rules.EventRegistration, rules.EventTransfer:
		return "DMT"
	case rules.EventMileage, rules.EventEmission:
		return "VET"
	case rules.EventAccident, rules.EventTheftFlag, rules.EventTheftClear:
		return "Police"
	case rules.EventClaim:
		return "Insurer"
	}
	return "DMT"
}

var Agencies = []string{"DMT", "Police", "VET", "Customs", "Insurer"}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
