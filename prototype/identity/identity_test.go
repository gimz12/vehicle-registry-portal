package identity

import (
	"bytes"
	"testing"
)

func testKeys(t *testing.T) *Keys {
	t.Helper()
	k, err := NewKeys()
	if err != nil {
		t.Fatalf("NewKeys: %v", err)
	}
	return k
}

func mustVID(t *testing.T, k *Keys, chassis string) string {
	t.Helper()
	value, err := k.VID(chassis)
	if err != nil {
		t.Fatalf("VID(%q): %v", chassis, err)
	}
	return value
}

func mustEngine(t *testing.T, k *Keys, make, serial string) string {
	t.Helper()
	value, err := k.IdxEngine(make, serial)
	if err != nil {
		t.Fatalf("IdxEngine(%q, %q): %v", make, serial, err)
	}
	return value
}

func mustPseudonym(t *testing.T, k *Keys, nic, vid string) string {
	t.Helper()
	value, err := k.Pseudonym(nic, vid)
	if err != nil {
		t.Fatalf("Pseudonym: %v", err)
	}
	return value
}

func mustOwnerTag(t *testing.T, k *Keys, nic string) string {
	t.Helper()
	value, err := k.OwnerTag(nic)
	if err != nil {
		t.Fatalf("OwnerTag: %v", err)
	}
	return value
}

func mustScoped(t *testing.T, k *Keys, profile Profile, nic, vid string, seq uint64) string {
	t.Helper()
	value, err := k.ScopedPseudonym(profile, nic, vid, seq)
	if err != nil {
		t.Fatalf("ScopedPseudonym: %v", err)
	}
	return value
}

// The central privacy property of Algorithm 1': one owner's events on one vehicle
// link (so the fraud rules can run), while the same owner's different vehicles do
// not (so serial fraud is invisible without k_own).
func TestPseudonymLinksWithinVehicleNotAcross(t *testing.T) {
	k := testKeys(t)
	vidA, vidB := mustVID(t, k, "NHP10-1234567"), mustVID(t, k, "ZVW30-7654321")
	nic := "199512345678"

	if mustPseudonym(t, k, nic, vidA) != mustPseudonym(t, k, nic, vidA) {
		t.Error("pseudonym is not stable for the same owner and vehicle — cross-event rules would break")
	}
	if mustPseudonym(t, k, nic, vidA) == mustPseudonym(t, k, nic, vidB) {
		t.Error("the same owner's two vehicles share a pseudonym — cross-vehicle unlinkability is lost")
	}
	if mustPseudonym(t, k, nic, vidA) == mustPseudonym(t, k, "200087654321", vidA) {
		t.Error("two different owners collide on one vehicle's pseudonym")
	}
}

// The owner tag is the one value that would make a person's vehicles linkable to
// any ledger observer, so it must be distinct from anything written on-chain.
func TestOwnerTagIsNotAnOnChainValue(t *testing.T) {
	k := testKeys(t)
	nic := "199512345678"
	vid := mustVID(t, k, "NHP10-1234567")

	tag := mustOwnerTag(t, k, nic)
	if tag == mustPseudonym(t, k, nic, vid) || tag == vid {
		t.Fatal("the owner tag collides with an on-chain value; it must never be derivable from the ledger")
	}
}

// Identifiers must survive the formatting noise of real paperwork, or F4
// duplicate-identity detection is defeated by a stray hyphen.
func TestIdentifiersAreCanonicalised(t *testing.T) {
	k := testKeys(t)
	tests := []struct{ a, b string }{
		{"NHP10-1234567", "nhp10 1234567"},
		{"NHP10-1234567", " NHP10-1234567 "},
		{"NHP10/1234567", "NHP101234567"},
	}
	for _, tc := range tests {
		if mustVID(t, k, tc.a) != mustVID(t, k, tc.b) {
			t.Errorf("VID(%q) != VID(%q); duplicate detection would miss this pair", tc.a, tc.b)
		}
	}
	if mustVID(t, k, "NHP10-1234567") == mustVID(t, k, "NHP10-1234568") {
		t.Error("distinct chassis numbers collide")
	}
}

func TestInvalidIdentifiersAndNICsAreRejected(t *testing.T) {
	k := testKeys(t)
	for _, value := range []string{"", " ", "N/A", "0", "unknown"} {
		if _, err := k.VID(value); err == nil {
			t.Errorf("VID(%q) accepted a null-like chassis", value)
		}
	}
	for _, nic := range []string{"", "123", "1995-ABC", "1234567890"} {
		if _, err := k.Pseudonym(nic, "vid-1"); err == nil {
			t.Errorf("Pseudonym accepted invalid NIC %q", nic)
		}
	}
	if _, err := k.Pseudonym("199512345678", ""); err == nil {
		t.Error("Pseudonym accepted an empty vehicle handle")
	}
}

// Engine indices are scoped by make, since serials are unique only per manufacturer.
func TestEngineIndexIsScopedByMake(t *testing.T) {
	k := testKeys(t)
	if mustEngine(t, k, "Toyota", "1NZ-FXE-987654") == mustEngine(t, k, "Nissan", "1NZ-FXE-987654") {
		t.Error("the same serial under two makes yields one index — false duplicates would be reported")
	}
}

// The length-prefixed MAC must not let two different field splits collide.
func TestNoConcatenationCollision(t *testing.T) {
	k := testKeys(t)
	if mustEngine(t, k, "AB", "C") == mustEngine(t, k, "A", "BC") {
		t.Error("field boundaries are ambiguous; identifiers can be forged by re-splitting inputs")
	}
}

// Each profile must produce a genuinely different linkage structure, or the
// Chapter 6 sweep would be measuring nothing.
func TestScopedPseudonymRealisesEachProfile(t *testing.T) {
	k := testKeys(t)
	nic := "199512345678"
	vidA, vidB := mustVID(t, k, "chassis-a"), mustVID(t, k, "chassis-b")

	t.Run("P0 links every vehicle of one owner", func(t *testing.T) {
		if mustScoped(t, k, P0, nic, vidA, 1) != mustScoped(t, k, P0, nic, vidB, 9) {
			t.Error("P0 must expose owner-level linkage")
		}
	})
	t.Run("P1 links events of one vehicle only", func(t *testing.T) {
		if mustScoped(t, k, P1, nic, vidA, 1) != mustScoped(t, k, P1, nic, vidA, 9) {
			t.Error("P1 must link a vehicle's events to each other")
		}
		if mustScoped(t, k, P1, nic, vidA, 1) == mustScoped(t, k, P1, nic, vidB, 1) {
			t.Error("P1 must not link two vehicles of one owner")
		}
	})
	t.Run("P2 gives each event its own pseudonym", func(t *testing.T) {
		if mustScoped(t, k, P2, nic, vidA, 1) == mustScoped(t, k, P2, nic, vidA, 2) {
			t.Error("P2 must not link two events of one vehicle through the pseudonym")
		}
	})
	t.Run("P3 hides the vehicle link too", func(t *testing.T) {
		if mustScoped(t, k, P3, nic, vidA, 1) == mustScoped(t, k, P3, nic, vidA, 2) {
			t.Error("P3 must not link any two events")
		}
	})
}

// A reveal requires both authorities. This is the property that makes the
// governed reveal meaningful rather than decorative.
func TestRevealRequiresBothShares(t *testing.T) {
	pii := []byte(`{"nic":"199512345678","name":"K. Perera","address":"Colombo 05"}`)
	env, err := SealOwner("vid-alice", pii)
	if err != nil {
		t.Fatalf("SealOwner: %v", err)
	}

	t.Run("both shares recover the record", func(t *testing.T) {
		got, err := OpenOwner(env, env.ShareDMT, env.SharePol)
		if err != nil {
			t.Fatalf("OpenOwner with both shares: %v", err)
		}
		if !bytes.Equal(got, pii) {
			t.Error("recovered PII does not match the sealed value")
		}
	})

	t.Run("one share alone recovers nothing", func(t *testing.T) {
		zero := make([]byte, 32)
		if _, err := OpenOwner(env, env.ShareDMT, zero); err == nil {
			t.Error("DMT alone decrypted the record — the threshold guarantee is void")
		}
		if _, err := OpenOwner(env, zero, env.SharePol); err == nil {
			t.Error("Police alone decrypted the record — the threshold guarantee is void")
		}
	})

	t.Run("a missing share is refused outright", func(t *testing.T) {
		if _, err := OpenOwner(env, env.ShareDMT, nil); err != ErrShareMissing {
			t.Errorf("err = %v, want ErrShareMissing", err)
		}
	})
}

// The headline containment property: combining both shares of one record yields
// that record only. There is no population-wide key to steal.
func TestSharesAreScopedToOneRecordOnly(t *testing.T) {
	alice, err := SealOwner("vid-alice", []byte("alice-pii"))
	if err != nil {
		t.Fatalf("SealOwner: %v", err)
	}
	bob, err := SealOwner("vid-bob", []byte("bob-pii"))
	if err != nil {
		t.Fatalf("SealOwner: %v", err)
	}

	if _, err := OpenOwner(bob, alice.ShareDMT, alice.SharePol); err == nil {
		t.Fatal("one vehicle's reconstructed key opened another vehicle's record — the envelope is not per-record")
	}
}

// The on-chain anchor is what makes off-chain tampering detectable.
func TestAnchorDetectsOffChainTampering(t *testing.T) {
	env, err := SealOwner("vid-owner", []byte("owner-pii"))
	if err != nil {
		t.Fatalf("SealOwner: %v", err)
	}
	if !VerifyEnvelopeAnchor(env) {
		t.Error("an untampered ciphertext failed its anchor check")
	}

	tampered := *env
	tampered.Ciphertext = append([]byte(nil), env.Ciphertext...)
	tampered.Ciphertext[0] ^= 0xFF
	if VerifyEnvelopeAnchor(&tampered) {
		t.Error("a tampered ciphertext passed its anchor check")
	}

	tamperedVID := *env
	tamperedVID.VID = "different-vehicle"
	if VerifyEnvelopeAnchor(&tamperedVID) {
		t.Error("an envelope rebound to another vehicle passed its anchor check")
	}
}

// Erasure under PDPA s.16 is performed by destroying a share: the ledger keeps
// its append-only record, while the personal data becomes unrecoverable.
func TestShareDestructionRendersRecordUnrecoverable(t *testing.T) {
	env, err := SealOwner("vid-owner", []byte("owner-pii"))
	if err != nil {
		t.Fatalf("SealOwner: %v", err)
	}
	for i := range env.ShareDMT {
		env.ShareDMT[i] = 0
	}
	if _, err := OpenOwner(env, env.ShareDMT, env.SharePol); err == nil {
		t.Error("the record was still recoverable after its DMT share was destroyed")
	}
}
