package registry

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	objectVehicle   = "vehicle"
	objectImport    = "import"
	objectIndex     = "identity"
	objectReveal    = "reveal"
	objectPredicate = "predicate"
)

type FabricRepository struct {
	ctx contractapi.TransactionContextInterface
}

func NewFabricRepository(ctx contractapi.TransactionContextInterface) *FabricRepository {
	return &FabricRepository{ctx: ctx}
}

func (r *FabricRepository) key(objectType string, attrs ...string) (string, error) {
	return r.ctx.GetStub().CreateCompositeKey(objectType, attrs)
}

func (r *FabricRepository) get(objectType string, attrs []string, target any) error {
	key, err := r.key(objectType, attrs...)
	if err != nil {
		return err
	}
	data, err := r.ctx.GetStub().GetState(key)
	if err != nil {
		return err
	}
	if data == nil {
		return ErrNotFound
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("registry: decode %s state: %w", objectType, err)
	}
	return nil
}

func (r *FabricRepository) put(objectType string, attrs []string, value any) error {
	key, err := r.key(objectType, attrs...)
	if err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.ctx.GetStub().PutState(key, data)
}

func (r *FabricRepository) GetVehicle(vid string) (*VehicleRecord, error) {
	var record VehicleRecord
	if err := r.get(objectVehicle, []string{vid}, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *FabricRepository) PutVehicle(record *VehicleRecord) error {
	return r.put(objectVehicle, []string{record.Vehicle.VID}, record)
}

func (r *FabricRepository) ListVehicles() ([]*VehicleRecord, error) {
	iterator, err := r.ctx.GetStub().GetStateByPartialCompositeKey(objectVehicle, nil)
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	var records []*VehicleRecord
	for iterator.HasNext() {
		entry, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var record VehicleRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return nil, err
		}
		records = append(records, &record)
	}
	sortedRecords(records)
	return records, nil
}

func (r *FabricRepository) GetImport(idxChassis string) (*ImportAttestation, error) {
	var attestation ImportAttestation
	if err := r.get(objectImport, []string{idxChassis}, &attestation); err != nil {
		return nil, err
	}
	return &attestation, nil
}

func (r *FabricRepository) PutImport(attestation *ImportAttestation) error {
	return r.put(objectImport, []string{attestation.IdxChassis}, attestation)
}

func (r *FabricRepository) GetIdentityOwner(kind, digest string) (string, error) {
	var owner string
	if err := r.get(objectIndex, []string{kind, digest}, &owner); err != nil {
		return "", err
	}
	return owner, nil
}

func (r *FabricRepository) PutIdentityOwner(kind, digest, vid string) error {
	return r.put(objectIndex, []string{kind, digest}, vid)
}

func (r *FabricRepository) GetReveal(id string) (*RevealRequest, error) {
	var request RevealRequest
	if err := r.get(objectReveal, []string{id}, &request); err != nil {
		return nil, err
	}
	return &request, nil
}

func (r *FabricRepository) PutReveal(request *RevealRequest) error {
	return r.put(objectReveal, []string{request.ID}, request)
}

func (r *FabricRepository) GetPredicate(vid, ruleID string) (*PredicateRecord, error) {
	var record PredicateRecord
	if err := r.get(objectPredicate, []string{vid, ruleID}, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *FabricRepository) PutPredicate(record *PredicateRecord) error {
	return r.put(objectPredicate, []string{record.VID, record.RuleID}, record)
}
