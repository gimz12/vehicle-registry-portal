package experiment

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

type Cell struct {
	RuleID    string        `json:"ruleId"`
	Profile   rules.Profile `json:"profile"`
	Positive  int           `json:"positive"`
	Negative  int           `json:"negative"`
	TP        int           `json:"truePositive"`
	FP        int           `json:"falsePositive"`
	TN        int           `json:"trueNegative"`
	FN        int           `json:"falseNegative"`
	Evaluated int           `json:"evaluated"`
	GapOnly   int           `json:"gapOnly"`
	// GapStructural counts vehicles with at least one gap caused by the data
	// view itself (redaction, split history, removed owner link).
	GapStructural int `json:"gapStructural"`
	// Suppressed is the declared count: vehicles the chaincode gate (Decide)
	// would answer SUPPRESSED because the profile withholds the rule's required
	// capability. It is kept apart from the measured columns above.
	Suppressed int `json:"suppressed"`
}

const (
	ClassAvailable  = "available"
	ClassPartial    = "partial"
	ClassSuppressed = "suppressed"
)

// Class is the measured decidability class, derived from the transformed view
// only: suppressed when no vehicle could be evaluated, partial when the view
// itself made some vehicles undecidable, available otherwise. Gaps caused by
// weak evidence (E0 records, missing antecedents) lower Coverage but do not
// change the class, so Corpus B noise cannot move a breakpoint.
func (c Cell) Class() string {
	switch {
	case c.Evaluated == 0:
		return ClassSuppressed
	case c.GapStructural > 0:
		return ClassPartial
	default:
		return ClassAvailable
	}
}

// Coverage is the share of rule evaluations that returned PASS or FLAGGED.
func (c Cell) Coverage() float64 {
	if c.Evaluated+c.GapOnly == 0 {
		return 0
	}
	return float64(c.Evaluated) / float64(c.Evaluated+c.GapOnly)
}

func (c Cell) Precision() float64 {
	if c.TP+c.FP == 0 {
		return 0
	}
	return float64(c.TP) / float64(c.TP+c.FP)
}

func (c Cell) Recall() float64 {
	if c.TP+c.FN == 0 {
		return 0
	}
	return float64(c.TP) / float64(c.TP+c.FN)
}

func (c Cell) F1() float64 {
	p, r := c.Precision(), c.Recall()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

type Report struct {
	Profile  rules.Profile `json:"profile"`
	Cells    []Cell        `json:"cells"`
	Exposure Exposure      `json:"exposure"`
}

type observation struct {
	detected   bool
	evaluated  bool
	structural bool
}

func Run(corpus *Corpus, profile rules.Profile) (*Report, error) {
	view, err := Transform(corpus, profile)
	if err != nil {
		return nil, err
	}
	observed := map[string]*observation{}
	visibleIDs := make([]string, 0, len(view.Ledger.Vehicles))
	for vid := range view.Ledger.Vehicles {
		visibleIDs = append(visibleIDs, vid)
	}
	sort.Strings(visibleIDs)
	for _, visibleVID := range visibleIDs {
		vehicle := view.Ledger.Vehicles[visibleVID]
		sourceVID := view.VisibleToSource[visibleVID]
		for _, descriptor := range rules.Registry {
			key := sourceVID + ":" + descriptor.ID
			if observed[key] == nil {
				observed[key] = &observation{}
			}
			for _, finding := range rules.Measure(descriptor, view.Ledger, vehicle) {
				if finding.Status == rules.StatusFlagged {
					observed[key].detected = true
				}
				if finding.Status == rules.StatusFlagged || finding.Status == rules.StatusPass {
					observed[key].evaluated = true
				}
				if finding.Status == rules.StatusGapNotice && finding.Structural {
					observed[key].structural = true
				}
			}
		}
	}
	labels := map[string]bool{}
	for _, label := range corpus.Labels {
		labels[label.SourceVID+":"+label.RuleID] = label.Positive
	}
	if len(labels) == 0 {
		return nil, fmt.Errorf("experiment: corpus contains no labels")
	}
	cells := map[string]*Cell{}
	for key, positive := range labels {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("experiment: invalid label key %q", key)
		}
		ruleID := parts[1]
		cell := cells[ruleID]
		if cell == nil {
			cell = &Cell{RuleID: ruleID, Profile: profile}
			cells[ruleID] = cell
		}
		result := observed[key]
		if result == nil {
			result = &observation{}
		}
		if result.evaluated {
			cell.Evaluated++
		} else {
			cell.GapOnly++
		}
		if result.structural {
			cell.GapStructural++
		}
		if positive {
			cell.Positive++
			if result.detected {
				cell.TP++
			} else {
				cell.FN++
			}
		} else {
			cell.Negative++
			if result.detected {
				cell.FP++
			} else {
				cell.TN++
			}
		}
	}
	report := &Report{Profile: profile, Exposure: MeasureExposure(corpus, view)}
	for _, cell := range cells {
		if descriptor, ok := rules.Lookup(cell.RuleID); ok && !profile.Exposes(descriptor.RequiredCapability) {
			cell.Suppressed = cell.Positive + cell.Negative
		}
		report.Cells = append(report.Cells, *cell)
	}
	sort.Slice(report.Cells, func(i, j int) bool { return report.Cells[i].RuleID < report.Cells[j].RuleID })
	return report, nil
}
