// seal seals a synthetic owner record into the development store and prints
// its anchor. Used by network/seed.sh.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/gimz12/vehicle-registry-portal/prototype/offchain/devstore"
)

func main() {
	vid := flag.String("vid", "", "64-hex vehicle handle")
	name := flag.String("name", "", "owner name (synthetic)")
	nic := flag.String("nic", "", "NIC number (synthetic)")
	address := flag.String("address", "", "address (synthetic)")
	store := flag.String("store", "offchain/dev-store", "off-ledger development store")
	flag.Parse()
	if *vid == "" || *name == "" {
		log.Fatal("seal: -vid and -name are required")
	}
	anchor, err := devstore.Seal(*store, *vid, devstore.OwnerPII{Name: *name, NIC: *nic, Address: *address})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(anchor)
}
