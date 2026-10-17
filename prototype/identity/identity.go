// Package identity derives keyed identifiers (HMAC with k_v and k_own) and seals
// each owner record under a per-record key split between the DMT and the Police.
//
// Do not call this package from chaincode: every endorsing peer would need the
// keys. The chaincode only receives the computed values.
package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Keys holds k_v and k_own. Whoever holds k_own can link an owner's vehicles.
type Keys struct {
	KV   []byte // asset-scope key: chassis and engine indices
	KOwn []byte // owner-scope key: pseudonyms and the service-internal owner tag
}

// NewKeys generates a fresh key pair.
func NewKeys() (*Keys, error) {
	kv, kown := make([]byte, 32), make([]byte, 32)
	if _, err := rand.Read(kv); err != nil {
		return nil, fmt.Errorf("generating k_v: %w", err)
	}
	if _, err := rand.Read(kown); err != nil {
		return nil, fmt.Errorf("generating k_own: %w", err)
	}
	return &Keys{KV: kv, KOwn: kown}, nil
}

func mac(key []byte, parts ...string) string {
	h := hmac.New(sha256.New, key)
	// Length-prefix each part so that ("ab","c") and ("a","bc") cannot collide.
	for _, p := range parts {
		fmt.Fprintf(h, "%d:%s|", len(p), p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// canonical keeps upper-case letters and digits, so spelling variants of one
// identifier give one index.
func canonical(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	ErrInvalidIdentifier = errors.New("identity: identifier is empty after canonicalisation")
	ErrInvalidNIC        = errors.New("identity: NIC must be 12 digits or 9 digits followed by V/X")
)

// CanonicalIdentifier normalises an identifier and rejects null-like values,
// which would otherwise all collide at one index.
func CanonicalIdentifier(s string) (string, error) {
	value := canonical(s)
	switch value {
	case "", "0", "NA", "NONE", "NULL", "UNKNOWN":
		return "", ErrInvalidIdentifier
	}
	return value, nil
}

// CanonicalNIC accepts the two Sri Lankan NIC shapes used by the prototype.
func CanonicalNIC(s string) (string, error) {
	value := canonical(s)
	if len(value) == 12 {
		if _, err := strconv.ParseUint(value, 10, 64); err == nil {
			return value, nil
		}
	}
	if len(value) == 10 && (value[9] == 'V' || value[9] == 'X') {
		if _, err := strconv.ParseUint(value[:9], 10, 64); err == nil {
			return value, nil
		}
	}
	return "", ErrInvalidNIC
}

// VID is the asset handle: HMAC(k_v, canonical(chassis)).
func (k *Keys) VID(chassis string) (string, error) {
	value, err := CanonicalIdentifier(chassis)
	if err != nil {
		return "", err
	}
	return mac(k.KV, "vid", value), nil
}

// IdxChassis is the deterministic cross-record chassis index used by F4/F5.
func (k *Keys) IdxChassis(chassis string) (string, error) {
	value, err := CanonicalIdentifier(chassis)
	if err != nil {
		return "", err
	}
	return mac(k.KV, "chassis", value), nil
}

// IdxEngine is HMAC(k_v, make, engineSerial); serials are only unique per make.
func (k *Keys) IdxEngine(make, engineSerial string) (string, error) {
	canonicalMake, err := CanonicalIdentifier(make)
	if err != nil {
		return "", fmt.Errorf("make: %w", err)
	}
	serial, err := CanonicalIdentifier(engineSerial)
	if err != nil {
		return "", fmt.Errorf("engine serial: %w", err)
	}
	return mac(k.KV, "eng", canonicalMake, serial), nil
}

// Pseudonym is the on-chain owner pseudonym (profile P1): stable for one owner
// and one vehicle, unlinkable across vehicles without k_own.
func (k *Keys) Pseudonym(nic, vid string) (string, error) {
	canonicalOwner, err := CanonicalNIC(nic)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(vid) == "" {
		return "", ErrInvalidIdentifier
	}
	return mac(k.KOwn, "p", canonicalOwner, vid), nil
}

// OwnerTag links all of an owner's vehicles, so it must never go on-chain.
func (k *Keys) OwnerTag(nic string) (string, error) {
	canonicalOwner, err := CanonicalNIC(nic)
	if err != nil {
		return "", err
	}
	return mac(k.KOwn, "owner", canonicalOwner), nil
}

// Profile mirrors rules.Profile without importing the rules package.
type Profile string

const (
	P0 Profile = "P0"
	P1 Profile = "P1"
	P2 Profile = "P2"
	P3 Profile = "P3"
)

// ScopedPseudonym derives the owner pseudonym for a linkage profile:
//
//	P0  one pseudonym per owner across every vehicle   (full linkage)
//	P1  one per (owner, vehicle)                       (the design point)
//	P2  one per (owner, vehicle, event)                (vehicle link survives via vid)
//	P3  one per event, with the vehicle link hidden
func (k *Keys) ScopedPseudonym(p Profile, nic, vid string, seq uint64) (string, error) {
	canonicalOwner, err := CanonicalNIC(nic)
	if err != nil {
		return "", err
	}
	switch p {
	case P0:
		return k.OwnerTag(canonicalOwner)
	case P1:
		return k.Pseudonym(canonicalOwner, vid)
	case P2:
		return mac(k.KOwn, "p2", canonicalOwner, vid, fmt.Sprint(seq)), nil
	case P3:
		return mac(k.KOwn, "p3", canonicalOwner, fmt.Sprint(seq)), nil
	}
	return "", fmt.Errorf("identity: unknown profile %q", p)
}

// Envelope is one vehicle's sealed owner record. The ciphertext and both shares
// live off-chain; only Anchor is committed to the ledger.
type Envelope struct {
	VID        string
	Version    uint8
	Ciphertext []byte
	Nonce      []byte
	ShareDMT   []byte
	SharePol   []byte
	Anchor     string // SHA-256 of the ciphertext, written on-chain
}

const envelopeVersion uint8 = 1

func envelopeAAD(vid string, version uint8) []byte {
	return []byte(fmt.Sprintf("vehicle-owner-envelope:v%d:%s", version, vid))
}

func envelopeAnchor(e *Envelope) string {
	h := sha256.New()
	h.Write([]byte{e.Version})
	h.Write([]byte(e.VID))
	h.Write(e.Nonce)
	h.Write(e.Ciphertext)
	return hex.EncodeToString(h.Sum(nil))
}

// SealOwner encrypts one owner record with AES-GCM under a fresh key and splits
// the key into two XOR shares (DMT, Police). Both shares open one record only.
func SealOwner(vid string, pii []byte) (*Envelope, error) {
	if strings.TrimSpace(vid) == "" {
		return nil, ErrInvalidIdentifier
	}
	recordKey := make([]byte, 32)
	if _, err := rand.Read(recordKey); err != nil {
		return nil, fmt.Errorf("generating record key: %w", err)
	}
	shareDMT := make([]byte, 32)
	if _, err := rand.Read(shareDMT); err != nil {
		return nil, fmt.Errorf("generating DMT share: %w", err)
	}

	// sharePol = key XOR shareDMT; either share alone reveals nothing.
	sharePol := make([]byte, 32)
	for i := range recordKey {
		sharePol[i] = recordKey[i] ^ shareDMT[i]
	}

	block, err := aes.NewCipher(recordKey)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, pii, envelopeAAD(vid, envelopeVersion))

	// Only the shares are kept.
	for i := range recordKey {
		recordKey[i] = 0
	}

	envelope := &Envelope{
		VID:        vid,
		Version:    envelopeVersion,
		Ciphertext: ct,
		Nonce:      nonce,
		ShareDMT:   shareDMT,
		SharePol:   sharePol,
	}
	envelope.Anchor = envelopeAnchor(envelope)
	return envelope, nil
}

// ErrShareMissing is returned when a reveal is attempted without both shares.
var ErrShareMissing = errors.New("identity: both authority shares are required to reveal")

// OpenOwner rebuilds the record key from both shares and decrypts the record.
func OpenOwner(e *Envelope, shareDMT, sharePol []byte) ([]byte, error) {
	if e == nil || !VerifyEnvelopeAnchor(e) {
		return nil, errors.New("identity: owner envelope anchor mismatch")
	}
	if len(shareDMT) != 32 || len(sharePol) != 32 {
		return nil, ErrShareMissing
	}
	recordKey := make([]byte, 32)
	for i := range recordKey {
		recordKey[i] = shareDMT[i] ^ sharePol[i]
	}

	block, err := aes.NewCipher(recordKey)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	pii, err := gcm.Open(nil, e.Nonce, e.Ciphertext, envelopeAAD(e.VID, e.Version))
	if err != nil {
		return nil, fmt.Errorf("decrypting owner record: %w", err)
	}
	return pii, nil
}

// VerifyEnvelopeAnchor checks the stored ciphertext against the on-chain anchor.
func VerifyEnvelopeAnchor(e *Envelope) bool {
	if e == nil {
		return false
	}
	sum, err := hex.DecodeString(envelopeAnchor(e))
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(e.Anchor)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(sum, want) == 1
}
