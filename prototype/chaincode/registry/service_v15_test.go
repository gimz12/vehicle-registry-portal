package registry

// Input canonicalisation, approval, claim and role-attribute gates.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/cid"
	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"google.golang.org/protobuf/proto"
)

func vpFind(d *rules.Disclosure, id string) []rules.Finding {
	var out []rules.Finding
	for _, list := range [][]rules.Finding{d.Failed, d.Passed, d.NotPerformed} {
		for _, f := range list {
			if f.RuleID == id {
				out = append(out, f)
			}
		}
	}
	return out
}

// Case variants of an index are the same identity.
func TestV15_UppercaseDigestsRefused(t *testing.T) {
	service := registeredService(t)
	if _, err := service.CreateImportAttestation(MSPCustoms, ImportAttestation{
		RecordID: "imp-dup", IdxChassis: strings.ToUpper(testChassis), IdxEngine: strings.ToUpper(testEngine), Source: "CUSTOMS",
	}); err == nil {
		t.Errorf("re-import of an imported chassis accepted under an uppercase digest")
	}
	// COPA: new chassis, registered engine in uppercase.
	const ch2, v2 = "2222222222222222222222222222222222222222222222222222222222222222", "1111111111111111111111111111111111111111111111111111111111111111"
	_, impErr := service.CreateImportAttestation(MSPCustoms, ImportAttestation{RecordID: "imp2", IdxChassis: ch2, IdxEngine: strings.ToUpper(testEngine), Source: "CUSTOMS"})
	_, regErr := service.RegisterVehicle(MSPDMT, RegistrationInput{VID: v2, Pseudonym: "p2", IdxChassis: ch2, IdxEngine: strings.ToUpper(testEngine), Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor})
	if impErr == nil && regErr == nil {
		for _, p := range []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3} {
			d, _ := service.Disclosure(v2, p)
			t.Errorf("COPA committed with 1 Customs + 1 DMT; F4 at %s = %s", p, vpFind(d, "F4")[0].Status)
		}
	}
	// An uppercase VID is refused at registration, not accepted and then stranded.
	service2 := NewService(NewMemoryRepository())
	if _, err := service2.CreateImportAttestation(MSPCustoms, ImportAttestation{RecordID: "i", IdxChassis: testChassis, IdxEngine: testEngine, Source: "CUSTOMS"}); err != nil {
		t.Fatal(err)
	}
	up := strings.ToUpper(testVID)
	if _, err := service2.RegisterVehicle(MSPDMT, RegistrationInput{VID: up, Pseudonym: "p", IdxChassis: testChassis, IdxEngine: testEngine, Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor}); err == nil {
		_, fleetErr := service2.normaliseFleet(up, nil)
		t.Errorf("uppercase VID registered; its own fleet then fails: %v", fleetErr)
	}
}

// One side approves once.
func TestV15_SecondApprovalBySameSideRefused(t *testing.T) {
	service, second := twoVehicleFleet(t)
	req := f3cCase(t, service, "police-1", "c-over")
	a, err := service.ApproveRevealBy(MSPDMT, "dmt-1", req.ID, true, "", []string{testVID})
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.ApproveRevealBy(MSPDMT, "dmt-2", req.ID, true, "", []string{testVID, second})
	if err == nil {
		t.Errorf("second DMT approval accepted: fleet size %d -> %d, approver hash replaced: %v", a.FleetSize, b.FleetSize, a.DMTApproverIDHash != b.DMTApproverIDHash)
	}
}

// Identical input, identical refusal text.
func TestV15_RegisterRefusalDeterministic(t *testing.T) {
	service := NewService(NewMemoryRepository())
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		_, err := service.RegisterVehicle(MSPDMT, RegistrationInput{VID: "x", IdxChassis: "y", IdxEngine: "z", EnvelopeAnchor: "w", Pseudonym: "p"})
		seen[err.Error()] = true
	}
	if len(seen) > 1 {
		t.Errorf("%d distinct refusal texts for one input", len(seen))
	}
}

// The claim's insurer is the writer's registry.party.
func TestV15_ClaimInsurerBoundToParty(t *testing.T) {
	service := registeredService(t)
	_, err := service.AppendEventBy(MSPInsurer, "insurer-a", EventInput{RecordID: "cl-b", VID: testVID, Type: rules.EventClaim, Day: 10, InsurerID: "insurer-b"})
	if err == nil {
		t.Errorf("insurer-a's certificate filed a claim as insurer-b")
	}
	ev, err := service.AppendEventBy(MSPInsurer, "insurer-a", EventInput{RecordID: "cl-a", VID: testVID, Type: rules.EventClaim, Day: 11})
	if err != nil || ev.InsurerID != "insurer-a" {
		t.Errorf("claim without insurerId: err=%v insurer=%v, want insurer-a", err, ev)
	}
	if _, err := service.AppendEventBy(MSPInsurer, "", EventInput{RecordID: "cl-x", VID: testVID, Type: rules.EventClaim, Day: 12, InsurerID: "insurer-z"}); err == nil {
		t.Errorf("claim accepted from an insurer certificate with no registry.party")
	}
}

// A pseudonym written to the ledger cannot join the view-time case scope.
func TestV15_ScopeTagCannotBeWrittenIntoCase(t *testing.T) {
	service, _ := twoVehicleFleet(t)
	req := f3cCase(t, service, "p1", "CID-10")
	fleet := []string{testVID}
	if _, err := service.ApproveRevealBy(MSPDMT, "d1", req.ID, true, "", fleet); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveRevealBy(MSPPolice, "p2", req.ID, true, "", nil); err != nil {
		t.Fatal(err)
	}
	const v3, c3, e3 = "7777777777777777777777777777777777777777777777777777777777777777", "6666666666666666666666666666666666666666666666666666666666666666", "5555555555555555555555555555555555555555555555555555555555555555"
	if _, err := service.CreateImportAttestation(MSPCustoms, ImportAttestation{RecordID: "i3", IdxChassis: c3, IdxEngine: e3, Source: "CUSTOMS"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RegisterVehicle(MSPDMT, RegistrationInput{VID: v3, Pseudonym: "case:" + req.ID, IdxChassis: c3, IdxEngine: e3, Provenance: "CUSTOMS", EnvelopeAnchor: testAnchor}); err != nil {
		return // refused at write: also a valid fix
	}
	for i, odo := range []int64{50000, 10000} {
		if _, err := service.AppendEvent(MSPVET, EventInput{RecordID: "v3m" + string(rune('0'+i)), VID: v3, Type: rules.EventMileage, Odometer: odo, VETLaneID: "l", Day: int64(i * 10)}); err != nil {
			t.Fatal(err)
		}
	}
	d, err := service.DisclosureForFleet(MSPPolice, "", PurposePolice, testVID, req.ID, fleet)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range d.Failed {
		if f.RuleID == "F6a" {
			t.Errorf("approved fleet size 1, but a vehicle registered later with pseudonym case:%s joined the scope: %s", req.ID, f.Detail)
		}
	}
}

// A mileage reading may not be dated before the previous one.
func TestV15_BackdatedReadingRefused(t *testing.T) {
	service := registeredService(t)
	if _, err := service.AppendEvent(MSPVET, EventInput{RecordID: "b0", VID: testVID, Type: rules.EventMileage, Odometer: 10000, VETLaneID: "l", Day: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendEvent(MSPVET, EventInput{RecordID: "b1", VID: testVID, Type: rules.EventMileage, Odometer: 900000, VETLaneID: "l", Day: 99}); err == nil {
		d, _ := service.Disclosure(testVID, rules.ProfileP1)
		t.Errorf("backdated 890,000 km reading accepted; F1b = %s", vpFind(d, "F1b")[0].Status)
	}
	// Same-day retest stays allowed.
	if _, err := service.AppendEvent(MSPVET, EventInput{RecordID: "b2", VID: testVID, Type: rules.EventMileage, Odometer: 10050, VETLaneID: "l", Day: 100}); err != nil {
		t.Errorf("same-day reading refused: %v", err)
	}
}

// Clearing a theft report that does not exist.
func TestV15_TheftClearNeedsFlag(t *testing.T) {
	service := registeredService(t)
	if _, err := service.AppendEvent(MSPPolice, EventInput{RecordID: "clr", VID: testVID, Type: rules.EventTheftClear}); err == nil {
		t.Errorf("theft clearance accepted on a vehicle never flagged stolen")
	}
}

// Role attributes and the issuing organisation.

type creatorStub struct {
	shim.ChaincodeStubInterface
	creator []byte
}

func (s creatorStub) GetCreator() ([]byte, error) { return s.creator, nil }

func ctxWith(t *testing.T, mspID string, attrs string) contractapi.TransactionContextInterface {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "vp-test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	if attrs != "" {
		tmpl.ExtraExtensions = []pkix.Extension{{Id: []int{1, 2, 3, 4, 5, 6, 7, 8, 1}, Value: []byte(attrs)}}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := proto.Marshal(&msp.SerializedIdentity{Mspid: mspID, IdBytes: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})})
	stub := creatorStub{creator: sid}
	ci, err := cid.New(stub)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &contractapi.TransactionContext{}
	ctx.SetStub(stub)
	ctx.SetClientIdentity(ci)
	return ctx
}

func TestV15_RoleAttributeOnlyFromHostOrganisation(t *testing.T) {
	cases := []struct {
		msp, attrs, want string // want "" = refused
	}{
		{"Org1MSP", `{"attrs":{"registry.role":"POLICE"}}`, ""},  // Org1 CA mints a Police officer
		{"Org2MSP", `{"attrs":{"registry.role":"DMT"}}`, ""},     // Org2 CA mints a DMT officer
		{"Org2MSP", `{"attrs":{"registry.role":"CUSTOMS"}}`, ""}, // Org2 CA mints Customs
		// The DMT's CA may not issue other agencies' roles.
		{"Org1MSP", `{"attrs":{"registry.role":"CUSTOMS"}}`, ""},
		{"Org1MSP", `{"attrs":{"registry.role":"INSURER","registry.party":"insurer-a"}}`, ""},
		{"Org1MSP", `{"attrs":{"registry.role":"VET"}}`, ""},
		{"Org3MSP", `{"attrs":{"registry.role":"INSURER"}}`, ""},   // Customs CA mints an insurer
		{"Org5MSP", `{"attrs":{"registry.role":"CUSTOMS"}}`, ""},   // insurers' CA mints Customs
		{"Org4MSP", `{"attrs":{"registry.role":"ASSEMBLER"}}`, ""}, // VET CA mints an assembler
		{"Org3MSP", `{"attrs":{"registry.role":"CUSTOMS"}}`, MSPCustoms},
		{"Org4MSP", `{"attrs":{"registry.role":"VET"}}`, MSPVET},
		{"Org5MSP", `{"attrs":{"registry.role":"INSURER","registry.party":"insurer-a"}}`, MSPInsurer},
		{"Org1MSP", `{"attrs":{"registry.role":"ASSEMBLER"}}`, MSPAssembler},
		{"Org1MSP", `{"attrs":{"hf.Type":"client"}}`, "Org1MSP"}, // User1@org1 (DMT by MSP)
		{"Org2MSP", `{"attrs":{"hf.Type":"client"}}`, "Org2MSP"}, // User1@org2, police2
		{"Org3MSP", `{"attrs":{"hf.Type":"client"}}`, "Org3MSP"}, // User1@org3: Customs by MSP (canonicalMSP)
	}
	for _, c := range cases {
		got, err := actorMSP(ctxWith(t, c.msp, c.attrs))
		if c.want == "" && err == nil {
			t.Errorf("%s cert with %s resolved to %s; want refusal", c.msp, c.attrs, got)
		}
		if c.want != "" && (err != nil || got != c.want) {
			t.Errorf("%s cert with %s: got %q err %v; want %s", c.msp, c.attrs, got, err, c.want)
		}
	}
}

func TestV16_PlainAgencyOrganisationsMapToTheirRole(t *testing.T) {
	for msp, want := range map[string]string{"Org1MSP": MSPDMT, "Org2MSP": MSPPolice, "Org3MSP": MSPCustoms, "Org4MSP": MSPVET, "Org5MSP": MSPInsurer} {
		if got := canonicalMSP(msp); got != want {
			t.Errorf("canonicalMSP(%s) = %s; want %s", msp, got, want)
		}
	}
	if err := requireMSP("Org3MSP", MSPCustoms); err != nil {
		t.Errorf("a plain Customs-organisation client is refused the Customs role: %v", err)
	}
	if err := requireMSP("Org3MSP", MSPDMT); err == nil {
		t.Errorf("a Customs-organisation client passed a DMT-only gate")
	}
}
