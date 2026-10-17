// ownerindex adds a vehicle to the DMT owner index and prints its P1 pseudonym.
// Go twin of server/utils/ownerIndex.ts, used by network/seed.sh.
//
// Usage: go run ./cmd/ownerindex -store offchain/dev-store -nic 751111111V -vid <64 hex>
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/gimz12/vehicle-registry-portal/prototype/identity"
)

type ownerIndex struct {
	Owners   map[string][]string `json:"owners"`
	Vehicles map[string]string   `json:"vehicles"`
}

func main() {
	store := flag.String("store", "offchain/dev-store", "off-ledger dev store")
	nic := flag.String("nic", "", "owner NIC (synthetic)")
	vid := flag.String("vid", "", "vehicle handle, 64 hex")
	flag.Parse()

	var stored struct{ KV, KOwn string }
	raw, err := os.ReadFile(filepath.Join(*store, "registry-keys.json"))
	if err != nil {
		log.Fatalf("ownerindex: registry keys: %v (run the portal once, or create the key file)", err)
	}
	var keyFile map[string]string
	if err := json.Unmarshal(raw, &keyFile); err != nil {
		log.Fatal(err)
	}
	stored.KV, stored.KOwn = keyFile["kv"], keyFile["kown"]
	kv, err1 := hex.DecodeString(stored.KV)
	kown, err2 := hex.DecodeString(stored.KOwn)
	if err1 != nil || err2 != nil || len(kv) != 32 || len(kown) != 32 {
		log.Fatal("ownerindex: registry-keys.json must hold 32-byte hex kv and kown")
	}
	keys := &identity.Keys{KV: kv, KOwn: kown}
	tag, err := keys.OwnerTag(*nic)
	if err != nil {
		log.Fatal(err)
	}
	pseudonym, err := keys.Pseudonym(*nic, *vid)
	if err != nil {
		log.Fatal(err)
	}

	path := filepath.Join(*store, "custody_dmt", "owner_index.json")
	index := ownerIndex{Owners: map[string][]string{}, Vehicles: map[string]string{}}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &index); err != nil {
			log.Fatal(err)
		}
		if index.Owners == nil {
			index.Owners = map[string][]string{}
		}
		if index.Vehicles == nil {
			index.Vehicles = map[string]string{}
		}
	}
	for owner, vids := range index.Owners {
		kept := vids[:0]
		for _, v := range vids {
			if v != *vid {
				kept = append(kept, v)
			}
		}
		if len(kept) == 0 {
			delete(index.Owners, owner)
		} else {
			index.Owners[owner] = kept
		}
	}
	set := map[string]bool{*vid: true}
	for _, v := range index.Owners[tag] {
		set[v] = true
	}
	vids := make([]string, 0, len(set))
	for v := range set {
		vids = append(vids, v)
	}
	sort.Strings(vids)
	index.Owners[tag] = vids
	index.Vehicles[*vid] = tag
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	data, _ := json.MarshalIndent(index, "", "  ")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		log.Fatal(err)
	}
	fmt.Println(pseudonym)
}
