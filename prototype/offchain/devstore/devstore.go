// Package devstore is a file-backed development owner store: the sealed
// envelope plus one key share each in custody_dmt and custody_police.
// Limitation: two local directories; a deployment uses separately administered services.
package devstore

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/gimz12/vehicle-registry-portal/prototype/identity"
)

type OwnerPII struct {
	Name    string `json:"name"`
	NIC     string `json:"nic"`
	Address string `json:"address"`
	Note    string `json:"note"`
}

// StoredEnvelope is the owner_record row: ciphertext and anchor, never a share.
type StoredEnvelope struct {
	VID        string `json:"vid"`
	Version    uint8  `json:"version"`
	Ciphertext []byte `json:"ciphertext"`
	Nonce      []byte `json:"nonce"`
	Anchor     string `json:"anchor"`
}

type StoredShare struct {
	VID   string `json:"vid"`
	Share []byte `json:"share"`
}

const SyntheticNote = "SYNTHETIC RECORD. No real person is described."

// Seal encrypts and stores one owner record and returns the anchor to commit on-chain.
func Seal(store, vid string, pii OwnerPII) (string, error) {
	if pii.Note == "" {
		pii.Note = SyntheticNote
	}
	payload, err := json.Marshal(pii)
	if err != nil {
		return "", err
	}
	envelope, err := identity.SealOwner(vid, payload)
	if err != nil {
		return "", err
	}
	writes := map[string]any{
		filepath.Join(store, "owner_record", vid+".json"):   StoredEnvelope{VID: vid, Version: envelope.Version, Ciphertext: envelope.Ciphertext, Nonce: envelope.Nonce, Anchor: envelope.Anchor},
		filepath.Join(store, "custody_dmt", vid+".json"):    StoredShare{VID: vid, Share: envelope.ShareDMT},
		filepath.Join(store, "custody_police", vid+".json"): StoredShare{VID: vid, Share: envelope.SharePol},
	}
	for path, value := range writes {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
			return "", err
		}
	}
	return envelope.Anchor, nil
}

// Open reads the envelope and both shares and decrypts.
func Open(store, vid string) (OwnerPII, error) {
	var env StoredEnvelope
	var dmt, pol StoredShare
	for path, target := range map[string]any{
		filepath.Join(store, "owner_record", vid+".json"):   &env,
		filepath.Join(store, "custody_dmt", vid+".json"):    &dmt,
		filepath.Join(store, "custody_police", vid+".json"): &pol,
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			return OwnerPII{}, err
		}
		if err := json.Unmarshal(data, target); err != nil {
			return OwnerPII{}, err
		}
	}
	plain, err := identity.OpenOwner(&identity.Envelope{VID: env.VID, Version: env.Version, Ciphertext: env.Ciphertext, Nonce: env.Nonce, Anchor: env.Anchor}, dmt.Share, pol.Share)
	if err != nil {
		return OwnerPII{}, err
	}
	var pii OwnerPII
	return pii, json.Unmarshal(plain, &pii)
}
