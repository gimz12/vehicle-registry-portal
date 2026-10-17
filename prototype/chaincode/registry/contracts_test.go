package registry

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Metadata generation catches unsupported public method signatures early.
func TestFabricContractsProduceMetadata(t *testing.T) {
	if _, err := contractapi.NewChaincode(
		&VehicleRegistry{}, &FraudDetector{}, &IdentityManager{},
	); err != nil {
		t.Fatalf("NewChaincode: %v", err)
	}
}

// contractapi treats every field as required unless tagged metadata:",optional",
// so an omitempty field must also be optional or the peer rejects the response.
func TestOmittedResponseFieldsAreOptionalInMetadata(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Policy{}), reflect.TypeOf(RevealRequest{}), reflect.TypeOf(PurposeDisclosure{}),
		reflect.TypeOf(HistoryView{}), reflect.TypeOf(VehicleRecord{}), reflect.TypeOf(ImportAttestation{}),
		reflect.TypeOf(PredicateResult{}), reflect.TypeOf(VehicleStatus{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if strings.Contains(field.Tag.Get("json"), "omitempty") && !strings.Contains(field.Tag.Get("metadata"), "optional") {
				t.Errorf("%s.%s is omitempty but not metadata optional", typ.Name(), field.Name)
			}
		}
	}
}
