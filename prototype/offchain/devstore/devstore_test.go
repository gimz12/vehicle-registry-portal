package devstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSealThenOpenRoundTripAndTamperDetection(t *testing.T) {
	store := t.TempDir()
	vid := strings.Repeat("ab", 32)
	anchor, err := Seal(store, vid, OwnerPII{Name: "Test Person", NIC: "900000000V", Address: "1 Test Lane"})
	if err != nil {
		t.Fatal(err)
	}
	if len(anchor) != 64 {
		t.Fatalf("anchor %q is not a sha256 hex", anchor)
	}
	pii, err := Open(store, vid)
	if err != nil {
		t.Fatal(err)
	}
	if pii.Name != "Test Person" || pii.Note != SyntheticNote {
		t.Fatalf("round trip changed the record: %+v", pii)
	}
	// Flip a byte of the stored ciphertext: the anchor check must refuse it.
	path := filepath.Join(store, "owner_record", vid+".json")
	data, _ := os.ReadFile(path)
	tampered := strings.Replace(string(data), `"ciphertext": "`, `"ciphertext": "AA`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(store, vid); err == nil {
		t.Fatal("tampered envelope was opened")
	}
}
