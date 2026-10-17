package registry

import (
	"encoding/json"
	"errors"
	"sort"
)

// MemoryRepository backs tests and the parity harness. JSON round-trips stop
// callers mutating stored state through shared pointers.
type MemoryRepository struct {
	vehicles   map[string][]byte
	imports    map[string][]byte
	identity   map[string]string
	reveals    map[string][]byte
	predicates map[string][]byte
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		vehicles: map[string][]byte{}, imports: map[string][]byte{},
		identity: map[string]string{}, reveals: map[string][]byte{},
		predicates: map[string][]byte{},
	}
}

func marshal(value any) ([]byte, error) { return json.Marshal(value) }

func unmarshal[T any](data []byte) (*T, error) {
	if data == nil {
		return nil, ErrNotFound
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *MemoryRepository) GetVehicle(vid string) (*VehicleRecord, error) {
	return unmarshal[VehicleRecord](r.vehicles[vid])
}

func (r *MemoryRepository) PutVehicle(record *VehicleRecord) error {
	data, err := marshal(record)
	if err == nil {
		r.vehicles[record.Vehicle.VID] = data
	}
	return err
}

func (r *MemoryRepository) ListVehicles() ([]*VehicleRecord, error) {
	ids := make([]string, 0, len(r.vehicles))
	for id := range r.vehicles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*VehicleRecord, 0, len(ids))
	for _, id := range ids {
		record, err := r.GetVehicle(id)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *MemoryRepository) GetImport(idxChassis string) (*ImportAttestation, error) {
	return unmarshal[ImportAttestation](r.imports[idxChassis])
}

func (r *MemoryRepository) PutImport(attestation *ImportAttestation) error {
	data, err := marshal(attestation)
	if err == nil {
		r.imports[attestation.IdxChassis] = data
	}
	return err
}

func (r *MemoryRepository) GetIdentityOwner(kind, digest string) (string, error) {
	owner, ok := r.identity[kind+":"+digest]
	if !ok {
		return "", ErrNotFound
	}
	return owner, nil
}

func (r *MemoryRepository) PutIdentityOwner(kind, digest, vid string) error {
	key := kind + ":" + digest
	if owner, ok := r.identity[key]; ok && owner != vid {
		return errors.New("registry: identity index already exists")
	}
	r.identity[key] = vid
	return nil
}

func (r *MemoryRepository) GetReveal(id string) (*RevealRequest, error) {
	return unmarshal[RevealRequest](r.reveals[id])
}

func (r *MemoryRepository) PutReveal(request *RevealRequest) error {
	data, err := marshal(request)
	if err == nil {
		r.reveals[request.ID] = data
	}
	return err
}

func (r *MemoryRepository) GetPredicate(vid, ruleID string) (*PredicateRecord, error) {
	return unmarshal[PredicateRecord](r.predicates[vid+"|"+ruleID])
}

func (r *MemoryRepository) PutPredicate(record *PredicateRecord) error {
	data, err := marshal(record)
	if err == nil {
		r.predicates[record.VID+"|"+record.RuleID] = data
	}
	return err
}
