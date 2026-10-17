package main

import (
	"flag"
	"fmt"
	"log"
	"sort"

	"github.com/gimz12/vehicle-registry-portal/prototype/experiment"
)

func main() {
	vehicles := flag.Int("vehicles", 30000, "number of synthetic vehicles")
	seed := flag.Int64("seed", 20260809, "deterministic generator seed")
	fraudRate := flag.Float64("fraud-rate", 0.01, "per-rule synthetic fraud probability")
	output := flag.String("out", "./data/vlf-sl", "output directory")
	flag.Parse()

	corpus, err := experiment.Generate(experiment.Config{
		Vehicles: *vehicles, Seed: *seed, FraudRate: *fraudRate,
	})
	if err != nil {
		log.Fatal(err)
	}
	manifest, err := corpus.Write(*output)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("generated %d vehicles and %d events in %s\n", manifest.Config.Vehicles, manifest.EventCount, *output)
	names := make([]string, 0, len(manifest.SHA256))
	for name := range manifest.SHA256 {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		hash := manifest.SHA256[name]
		fmt.Printf("%s  %s\n", hash, name)
	}
}
