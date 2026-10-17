// handle derives a vehicle's on-ledger identifiers (handle, chassis and engine
// indices, and with a NIC the owner pseudonym). It also produces the fixture
// for the portal's TypeScript derivation.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gimz12/vehicle-registry-portal/prototype/identity"
)

func main() {
	kv := flag.String("kv", "", "asset-scope key k_v, 32 bytes hex")
	kown := flag.String("kown", "", "owner-scope key k_own, 32 bytes hex")
	chassis := flag.String("chassis", "", "chassis number as printed")
	engine := flag.String("engine", "", "engine serial as printed")
	make := flag.String("make", "", "manufacturer")
	nic := flag.String("nic", "", "owner NIC (optional, for the pseudonym)")
	flag.Parse()
	kvBytes, err1 := hex.DecodeString(*kv)
	kownBytes, err2 := hex.DecodeString(*kown)
	if err1 != nil || err2 != nil || len(kvBytes) != 32 || len(kownBytes) != 32 {
		log.Fatal("handle: -kv and -kown must each be 32 bytes of hex")
	}
	keys := &identity.Keys{KV: kvBytes, KOwn: kownBytes}
	out := map[string]string{}
	var err error
	if out["vid"], err = keys.VID(*chassis); err != nil {
		log.Fatal(err)
	}
	if out["idxChassis"], err = keys.IdxChassis(*chassis); err != nil {
		log.Fatal(err)
	}
	if out["idxEngine"], err = keys.IdxEngine(*make, *engine); err != nil {
		log.Fatal(err)
	}
	if *nic != "" {
		if out["pseudonym"], err = keys.Pseudonym(*nic, out["vid"]); err != nil {
			log.Fatal(err)
		}
		// Printed only for the TypeScript fixture; the owner tag never goes on-chain.
		if out["ownerTag"], err = keys.OwnerTag(*nic); err != nil {
			log.Fatal(err)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
