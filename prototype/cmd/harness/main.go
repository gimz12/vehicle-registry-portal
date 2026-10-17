package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gimz12/vehicle-registry-portal/prototype/experiment"
	"github.com/gimz12/vehicle-registry-portal/prototype/rules"
)

type sweepReport struct {
	Reports []*experiment.Report `json:"reports"`
}

func main() {
	corpusDir := flag.String("corpus", "./data/vlf-sl", "generated corpus directory")
	output := flag.String("out", "", "optional JSON report path")
	flag.Parse()

	corpus, err := experiment.Read(*corpusDir)
	if err != nil {
		log.Fatal(err)
	}
	sweep := sweepReport{}
	for _, profile := range []rules.Profile{rules.ProfileP0, rules.ProfileP1, rules.ProfileP2, rules.ProfileP3} {
		report, err := experiment.Run(corpus, profile)
		if err != nil {
			log.Fatal(err)
		}
		sweep.Reports = append(sweep.Reports, report)
		fmt.Printf("\n%s\n", profile)
		fmt.Println("RULE  POS   TP   FP   FN   PRECISION  RECALL  F1")
		for _, cell := range report.Cells {
			fmt.Printf("%-4s %4d %4d %4d %4d %9.3f %7.3f %5.3f\n",
				cell.RuleID, cell.Positive, cell.TP, cell.FP, cell.FN,
				cell.Precision(), cell.Recall(), cell.F1())
		}
	}
	if *output != "" {
		data, err := json.MarshalIndent(sweep, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(*output, data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
