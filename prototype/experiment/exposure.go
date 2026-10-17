package experiment

import (
	"sort"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

// Exposure is the privacy axis of the frontier (Table 3.3): how much of a
// vehicle's history one reference reaches, and how often an owner's vehicles can
// be joined, under one structural view. Both are measured on the transformed
// ledger, never read from the declared capability list.
type Exposure struct {
	Profile rules.Profile `json:"profile"`

	// Vehicle-history exposure: records reachable from one vehicle reference by
	// following the joins the view exposes (ASSET grouping, then OWNER grouping).
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	P95    int     `json:"p95"`
	Max    int     `json:"max"`

	// ResolvableRefs counts claim records whose evidence anchor resolves to an
	// incident record inside the reachable set. It separates P1 from P2, where
	// the reachable record count is otherwise identical.
	ResolvableRefs int `json:"resolvableRefs"`

	// Cross-vehicle owner linkability against hidden ground truth. Linkable means
	// the two vehicles carry the same non-empty pseudonym somewhere in the view.
	OwnerPairs    int     `json:"ownerPairs"`
	LinkablePairs int     `json:"linkablePairs"`
	Linkability   float64 `json:"linkability"`

	// QuasiLinkablePairs ignores pseudonyms and joins on a shared claim assessor.
	// It is reported because pseudonymisation does not remove quasi-identifiers.
	QuasiLinkablePairs int     `json:"quasiLinkablePairs"`
	QuasiLinkability   float64 `json:"quasiLinkability"`
}

func MeasureExposure(corpus *Corpus, view *View) Exposure {
	out := Exposure{Profile: view.Profile}
	ledger := view.Ledger

	// visible vehicles per source vehicle (one at P0–P2, one per event at P3)
	visibleBySource := map[string][]*rules.Vehicle{}
	for visibleVID, sourceVID := range view.VisibleToSource {
		visibleBySource[sourceVID] = append(visibleBySource[sourceVID], ledger.Vehicles[visibleVID])
	}
	for _, list := range visibleBySource {
		sort.Slice(list, func(i, j int) bool { return list[i].VID < list[j].VID })
	}

	reachable := make([]int, 0, len(corpus.Vehicles))
	pseudonyms := map[string]map[string]bool{}
	assessors := map[string]map[string]bool{}
	for _, canonical := range corpus.Vehicles {
		source := canonical.Vehicle.VID
		visible := visibleBySource[source]
		if len(visible) == 0 {
			continue
		}
		// The reference is the first committed event's visible vehicle.
		start := visible[0]
		for _, v := range visible {
			if len(v.Events) > 0 && v.Events[0].RecordID == canonical.Vehicle.Events[0].RecordID {
				start = v
				break
			}
		}
		set := reachableFrom(ledger, start)
		reachable = append(reachable, countEvents(set))
		out.ResolvableRefs += resolvableRefs(set)

		pseudonyms[source] = map[string]bool{}
		assessors[source] = map[string]bool{}
		for _, v := range visible {
			for _, e := range v.Events {
				if e.P != "" {
					pseudonyms[source][e.P] = true
				}
				if e.Type == rules.EventClaim && e.AttestorID != "" {
					assessors[source][e.AttestorID] = true
				}
			}
		}
	}
	sort.Ints(reachable)
	if n := len(reachable); n > 0 {
		total := 0
		for _, r := range reachable {
			total += r
		}
		out.Mean = float64(total) / float64(n)
		out.Median = float64(reachable[n/2])
		if n%2 == 0 {
			out.Median = float64(reachable[n/2-1]+reachable[n/2]) / 2
		}
		out.P95 = reachable[(n*95-1)/100]
		out.Max = reachable[n-1]
	}

	byOwner := map[string][]string{}
	for _, canonical := range corpus.Vehicles {
		byOwner[canonical.OwnerID] = append(byOwner[canonical.OwnerID], canonical.Vehicle.VID)
	}
	for _, vids := range byOwner {
		sort.Strings(vids)
		for i := 0; i < len(vids); i++ {
			for j := i + 1; j < len(vids); j++ {
				out.OwnerPairs++
				if intersects(pseudonyms[vids[i]], pseudonyms[vids[j]]) {
					out.LinkablePairs++
				}
				if intersects(assessors[vids[i]], assessors[vids[j]]) {
					out.QuasiLinkablePairs++
				}
			}
		}
	}
	if out.OwnerPairs > 0 {
		out.Linkability = float64(out.LinkablePairs) / float64(out.OwnerPairs)
		out.QuasiLinkability = float64(out.QuasiLinkablePairs) / float64(out.OwnerPairs)
	}
	return out
}

// reachableFrom follows the joins the view exposes: the vehicle's own grouped
// events, then every vehicle under the same owner pseudonym when the owner index
// exists. Redacted asset links (P3) stop at the single record.
func reachableFrom(l *rules.Ledger, start *rules.Vehicle) map[string]*rules.Vehicle {
	set := map[string]*rules.Vehicle{start.VID: start}
	if l.OwnerVehicles == nil {
		return set
	}
	for _, e := range start.Events {
		if e.P == "" {
			continue
		}
		for _, vid := range l.OwnerVehicles[e.P] {
			if other, ok := l.Vehicles[vid]; ok {
				set[vid] = other
			}
		}
		break
	}
	return set
}

func countEvents(set map[string]*rules.Vehicle) int {
	n := 0
	for _, v := range set {
		n += len(v.Events)
	}
	return n
}

func resolvableRefs(set map[string]*rules.Vehicle) int {
	incidents := map[string]bool{}
	for _, v := range set {
		for _, e := range v.Events {
			if e.Type == rules.EventAccident {
				incidents[e.RecordID] = true
				if e.EvidenceAnchor != "" {
					incidents[e.EvidenceAnchor] = true
				}
			}
		}
	}
	n := 0
	for _, v := range set {
		for _, e := range v.Events {
			if e.Type == rules.EventClaim && e.ReferenceStatus() == rules.ReferencePresent && incidents[e.EvidenceAnchor] {
				n++
			}
		}
	}
	return n
}

func intersects(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}
