package experiment

import (
	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

// Scenario is one Corpus C fixture (Table 3.2): a named, publicly documented or
// practitioner-described fraud pattern encoded as committed ledger events. The
// rules.Event struct is the fixture format; Run() consumes the derived Corpus,
// so a scenario is evaluated exactly like a generated vehicle.
type Scenario struct {
	ID       string
	Title    string
	Source   string
	Vehicles []CanonicalVehicle
	// Expect maps a rule id to the indexes (into Vehicles) expected to be
	// flagged at P0. Rules absent from the map are expected negative.
	Expect map[string][]int
}

// Corpus turns the scenario into a labelled corpus. FraudRate is nominal; it is
// only there to satisfy Config validation in the manifest.
func (s Scenario) Corpus() *Corpus {
	c := &Corpus{Config: Config{Vehicles: len(s.Vehicles), Seed: 0, FraudRate: 0.01}, Vehicles: s.Vehicles}
	for idx, vehicle := range s.Vehicles {
		for _, descriptor := range rules.Registry {
			positive := false
			for _, want := range s.Expect[descriptor.ID] {
				if want == idx {
					positive = true
				}
			}
			c.Labels = append(c.Labels, Label{SourceVID: vehicle.Vehicle.VID, RuleID: descriptor.ID, Positive: positive})
		}
	}
	return c
}

// builder assigns commit order across every vehicle in one scenario, so the
// F4/F5 "which record came first" questions have a definite answer.
type builder struct {
	id string
	co uint64
}

func (b *builder) vid(i int) string     { return digest("scenario-vid", b.id, i) }
func (b *builder) chassis(i int) string { return digest("scenario-chassis", b.id, i) }
func (b *builder) engine(i int) string  { return digest("scenario-engine", b.id, i) }

func (b *builder) vehicle(owner string, i int, events ...rules.Event) CanonicalVehicle {
	v := rules.Vehicle{VID: b.vid(i), AssetLink: rules.ReferencePresent}
	for _, e := range events {
		b.co++
		e.CommitOrder = b.co
		e.Seq = uint64(len(v.Events))
		e.VID = v.VID
		v.Events = append(v.Events, e)
	}
	return CanonicalVehicle{OwnerID: owner, Vehicle: v}
}

// Common event shapes.
func imp(chassis, engine string) rules.Event {
	return rules.Event{RecordID: "import-" + chassis[:8], Type: rules.EventImport, Tier: rules.E2, IdxChassis: chassis, IdxEngine: engine, Provenance: "CUSTOMS", AttestorID: "CustomsMSP"}
}
func reg(chassis, engine, provenance string) rules.Event {
	return rules.Event{RecordID: "reg-" + chassis[:8] + "-" + engine[:4], Type: rules.EventRegistration, Tier: rules.E2, IdxChassis: chassis, IdxEngine: engine, Provenance: provenance, AttestorID: "DMTMSP"}
}
func mileage(id string, day, odo int64) rules.Event {
	return rules.Event{RecordID: "mileage-" + id, Type: rules.EventMileage, Tier: rules.E2, Day: day, Odometer: odo, VETLaneID: "lane-1"}
}
func accident(id string, day int64) rules.Event {
	return rules.Event{RecordID: id, Type: rules.EventAccident, Tier: rules.E2, Day: day, EvidenceAnchor: id, EvidenceKind: "POLICE_REPORT", IncidentRef: rules.ReferencePresent, AttestorID: "PoliceMSP"}
}
func claim(id, anchor, insurer, assessor string, day int64) rules.Event {
	return rules.Event{RecordID: id, Type: rules.EventClaim, Day: day, Tier: rules.E2, EvidenceAnchor: anchor, EvidenceKind: "POLICE_REPORT", IncidentRef: refState(anchor), InsurerID: insurer, AttestorID: assessor}
}
func transfer(id string) rules.Event {
	return rules.Event{RecordID: "transfer-" + id, Type: rules.EventTransfer, Tier: rules.E2, AttestorID: "DMTMSP"}
}
func theftFlag(id string) rules.Event {
	return rules.Event{RecordID: "theft-" + id, Type: rules.EventTheftFlag, Tier: rules.E2, AttestorID: "PoliceMSP"}
}
func theftClear(id string) rules.Event {
	return rules.Event{RecordID: "theft-clear-" + id, Type: rules.EventTheftClear, Tier: rules.E2, AttestorID: "PoliceMSP"}
}

// Scenarios returns the Corpus C fixtures. Sources: COPA = Parliament of Sri
// Lanka Committee on Public Accounts (2025); CIABOC = Commission to Investigate
// Allegations of Bribery or Corruption (2024); DMT = practitioner consultation,
// July 2026 (Appendix C); Design = negative or boundary case written to exercise
// a declared rule limit.
func Scenarios() []Scenario {
	var out []Scenario
	add := func(id, title, source string, expect map[string][]int, build func(b *builder) []CanonicalVehicle) {
		b := &builder{id: id}
		out = append(out, Scenario{ID: id, Title: title, Source: source, Vehicles: build(b), Expect: expect})
	}

	add("C01-copa-identity-insertion", "Identity of an illegally imported vehicle inserted into a second registration", "COPA",
		map[string][]int{"F4": {1}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{
				b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), mileage("a", 0, 10000), mileage("b", 365, 22000)),
				// The second registration reuses the first vehicle's identity. F5
				// passes because an import with that engine exists; only F4 sees it.
				b.vehicle("o2", 1, reg(b.chassis(0), b.engine(0), "CUSTOMS"), mileage("c", 0, 5000), mileage("d", 365, 9000)),
			}
		})
	add("C02-ciaboc-no-provenance", "Registration with no Customs or assembler record", "CIABOC",
		map[string][]int{"F5": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, reg(b.chassis(0), b.engine(0), "NONE"), mileage("a", 0, 1000), mileage("b", 365, 9000))}
		})
	add("C03-backdated-import", "Import record committed after the registration it claims to support", "CIABOC",
		map[string][]int{"F5": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, reg(b.chassis(0), b.engine(0), "CUSTOMS"), imp(b.chassis(0), b.engine(0)), mileage("a", 0, 1000), mileage("b", 365, 9000))}
		})
	add("C04-odometer-rollback", "Second VET reading below the first", "DMT",
		map[string][]int{"F1a": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), mileage("a", 0, 120000), mileage("b", 365, 80000))}
		})
	add("C05-mileage-blocker", "Identical reading after more than a year", "DMT",
		map[string][]int{"F1c": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), mileage("a", 0, 90000), mileage("b", 400, 90000))}
		})
	add("C06-implausible-rate", "450,000 km accumulated in one year", "DMT",
		map[string][]int{"F1b": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), mileage("a", 0, 50000), mileage("b", 365, 500000))}
		})
	add("C07-retest-collapse", "Fail-and-retest readings minutes apart at one lane", "Design",
		map[string][]int{}, func(b *builder) []CanonicalVehicle {
			r1 := mileage("a", 0, 50000)
			r2 := mileage("b", 365, 62000)
			r2.RetestGroup = "retest-1"
			r3 := mileage("c", 365, 62010)
			r3.RetestGroup = "retest-1"
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), r1, r2, r3)}
		})
	add("C08-stolen-transfer", "Ownership transfer while the vehicle stands flagged stolen", "DMT",
		map[string][]int{"F2": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), theftFlag("1"), transfer("1"))}
		})
	add("C09-transfer-after-clear", "Transfer after the theft flag was cleared", "Design",
		map[string][]int{}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), theftFlag("1"), theftClear("1"), transfer("1"))}
		})
	add("C10-unanchored-claim", "Claim citing neither police report nor assessor attestation", "DMT",
		map[string][]int{"F3a": {0}, "F3b": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), accident("acc-1", 500), claim("claim-1", "", "insurer-a", "assessor-1", 505))}
		})
	add("C11-assessor-only-claim", "Claim resting on an insurer assessor attestation only", "DMT",
		map[string][]int{"F3a": {0}}, func(b *builder) []CanonicalVehicle {
			c := claim("claim-1", "assessor-note-1", "insurer-a", "assessor-1", 505)
			c.EvidenceKind, c.Tier = "ASSESSOR_ATTESTATION", rules.E1
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), c)}
		})
	add("C12-duplicate-anchor", "Two insurers' claims cite the same police report", "DMT",
		map[string][]int{"F3c": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), accident("acc-1", 500),
				claim("claim-1", "acc-1", "insurer-a", "assessor-1", 505), claim("claim-2", "acc-1", "insurer-b", "assessor-2", 508))}
		})
	add("C13-cross-insurer-window", "Two accidents and two insurers within 30 days", "DMT",
		map[string][]int{"F3d": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), accident("acc-1", 500),
				claim("claim-1", "acc-1", "insurer-a", "assessor-1", 505), accident("acc-2", 512), claim("claim-2", "acc-2", "insurer-b", "assessor-2", 515))}
		})
	add("C14-accident-no-claim", "Police accident report with no insurance claim", "Design",
		map[string][]int{"F3b": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), accident("acc-1", 500))}
		})
	add("C15-serial-rollback", "Three vehicles of one owner, each with a rollback within two years", "DMT",
		map[string][]int{"F1a": {0, 1, 2}, "F6a": {0, 1, 2}}, func(b *builder) []CanonicalVehicle {
			var vs []CanonicalVehicle
			for i := 0; i < 3; i++ {
				vs = append(vs, b.vehicle("serial", i, imp(b.chassis(i), b.engine(i)), reg(b.chassis(i), b.engine(i), "CUSTOMS"), mileage("a", int64(i*100), 100000), mileage("b", int64(i*100+300), 70000)))
			}
			return vs
		})
	add("C16-serial-assessor", "Claims on two of one owner's vehicles share an assessor", "DMT",
		map[string][]int{"F6b": {0, 1}}, func(b *builder) []CanonicalVehicle {
			var vs []CanonicalVehicle
			for i := 0; i < 2; i++ {
				acc := accident("acc-"+string(rune('1'+i)), 500)
				vs = append(vs, b.vehicle("serial", i, imp(b.chassis(i), b.engine(i)), reg(b.chassis(i), b.engine(i), "CUSTOMS"), acc,
					claim("claim-"+string(rune('1'+i)), acc.RecordID, "insurer-a", "shared-assessor", 505)))
			}
			return vs
		})
	add("C17-retired-resurrection", "Chassis of a retired vehicle registered again", "DMT",
		map[string][]int{"F4": {1}}, func(b *builder) []CanonicalVehicle {
			retired := b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"))
			retired.Vehicle.Retired = true
			return []CanonicalVehicle{retired, b.vehicle("o2", 1, imp(b.chassis(0), b.engine(1)), reg(b.chassis(0), b.engine(1), "CUSTOMS"))}
		})
	add("C18-engine-only-duplicate", "Engine index already registered to another vehicle", "COPA",
		map[string][]int{"F4": {1}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{
				b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS")),
				b.vehicle("o2", 1, imp(b.chassis(1), b.engine(0)), reg(b.chassis(1), b.engine(0), "CUSTOMS")),
			}
		})
	add("C19-honest-on-the-spot-claim", "Claim filed before the police report was committed (expected false positive)", "Design",
		map[string][]int{}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"),
				claim("claim-1", "acc-1", "insurer-a", "assessor-1", 500), accident("acc-1", 500))}
		})
	add("C20-legit-high-mileage-taxi", "350,000 km in one year, just under the rate limit", "Design",
		map[string][]int{}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), mileage("a", 0, 50000), mileage("b", 365, 400000))}
		})
	add("C21-e0-legacy-mileage", "Self-declared paper readings only (gap, not pass)", "Design",
		map[string][]int{}, func(b *builder) []CanonicalVehicle {
			r1, r2 := mileage("a", 0, 50000), mileage("b", 365, 40000)
			r1.Tier, r2.Tier = rules.E0, rules.E0
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), r1, r2)}
		})
	add("C22-theft-flag-no-transfer", "Vehicle flagged stolen, never transferred (transfer refused at write)", "DMT",
		map[string][]int{"F2": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), theftFlag("1"))}
		})
	add("C23-assembler-provenance", "Locally assembled vehicle with assembler attestation", "Design",
		map[string][]int{}, func(b *builder) []CanonicalVehicle {
			i := imp(b.chassis(0), b.engine(0))
			i.Provenance, i.AttestorID = "ASSEMBLER", "DMTMSP"
			return []CanonicalVehicle{b.vehicle("o1", 0, i, reg(b.chassis(0), b.engine(0), "ASSEMBLER"), mileage("a", 0, 100), mileage("b", 365, 9000))}
		})
	add("C24-anchored-claim", "Well-evidenced claim (redacted at P2, not missing)", "Design",
		map[string][]int{}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), accident("acc-1", 500), claim("claim-1", "acc-1", "insurer-a", "assessor-1", 505))}
		})
	add("C25-repeated-stolen-transfers", "Two transfers while flagged, then cleared", "DMT",
		map[string][]int{"F2": {0}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{b.vehicle("o1", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), theftFlag("1"), transfer("1"), transfer("2"), theftClear("1"))}
		})
	add("C26-serial-rollback-outside-window", "Two rollbacks by one owner more than two years apart", "Design",
		map[string][]int{"F1a": {0, 1}}, func(b *builder) []CanonicalVehicle {
			return []CanonicalVehicle{
				b.vehicle("serial", 0, imp(b.chassis(0), b.engine(0)), reg(b.chassis(0), b.engine(0), "CUSTOMS"), mileage("a", 0, 100000), mileage("b", 300, 70000)),
				b.vehicle("serial", 1, imp(b.chassis(1), b.engine(1)), reg(b.chassis(1), b.engine(1), "CUSTOMS"), mileage("a", 1200, 100000), mileage("b", 1500, 70000)),
			}
		})
	return out
}
