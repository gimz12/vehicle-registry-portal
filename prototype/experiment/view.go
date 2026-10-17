package experiment

import (
	"fmt"
	"sort"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

type View struct {
	Profile         rules.Profile
	Ledger          *rules.Ledger
	VisibleToSource map[string]string
}

func Transform(corpus *Corpus, profile rules.Profile) (*View, error) {
	if corpus == nil || len(profile.AvailableCapabilities()) == 0 {
		return nil, fmt.Errorf("experiment: invalid corpus or profile %q", profile)
	}
	view := &View{
		Profile:         profile,
		Ledger:          &rules.Ledger{Vehicles: map[string]*rules.Vehicle{}},
		VisibleToSource: map[string]string{},
	}
	if profile == rules.ProfileP0 {
		view.Ledger.OwnerVehicles = map[string][]string{}
		view.Ledger.OwnerLink = rules.ReferencePresent
	} else {
		view.Ledger.OwnerLink = rules.ReferenceRedacted
	}
	for _, canonical := range corpus.Vehicles {
		sourceVID := canonical.Vehicle.VID
		if profile == rules.ProfileP3 {
			for _, sourceEvent := range canonical.Vehicle.Events {
				event := sourceEvent
				visibleVID := digest("p3-event", sourceEvent.RecordID, sourceEvent.CommitOrder)
				event.VID = visibleVID
				event.P = digest("p3-pseudonym", canonical.OwnerID, sourceEvent.RecordID)
				rules.RedactIncidentReference(&event)
				view.Ledger.Vehicles[visibleVID] = &rules.Vehicle{VID: visibleVID, Events: []rules.Event{event}, AssetLink: rules.ReferenceRedacted}
				view.VisibleToSource[visibleVID] = sourceVID
			}
			continue
		}

		vehicle := canonical.Vehicle
		vehicle.Events = append([]rules.Event(nil), canonical.Vehicle.Events...)
		vehicle.AssetLink = rules.ReferencePresent
		var ownerPseudonym string
		switch profile {
		case rules.ProfileP0:
			ownerPseudonym = digest("p0-owner", canonical.OwnerID)
		case rules.ProfileP1:
			ownerPseudonym = digest("p1-owner-vehicle", canonical.OwnerID, sourceVID)
		}
		for i := range vehicle.Events {
			event := &vehicle.Events[i]
			event.VID = sourceVID
			switch profile {
			case rules.ProfileP0, rules.ProfileP1:
				event.P = ownerPseudonym
			case rules.ProfileP2:
				event.P = digest("p2-event", canonical.OwnerID, sourceVID, event.RecordID)
				rules.RedactIncidentReference(event)
			}
		}
		view.Ledger.Vehicles[sourceVID] = &vehicle
		view.VisibleToSource[sourceVID] = sourceVID
		if profile == rules.ProfileP0 {
			view.Ledger.OwnerVehicles[ownerPseudonym] = append(view.Ledger.OwnerVehicles[ownerPseudonym], sourceVID)
		}
	}
	for owner := range view.Ledger.OwnerVehicles {
		sort.Strings(view.Ledger.OwnerVehicles[owner])
	}
	return view, nil
}
