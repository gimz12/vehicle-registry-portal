package main

import (
	"log"

	"github.com/gimz12/vehicle-registry-portal/prototype/chaincode/registry"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

func main() {
	chaincode, err := contractapi.NewChaincode(
		&registry.VehicleRegistry{},
		&registry.FraudDetector{},
		&registry.IdentityManager{},
	)
	if err != nil {
		log.Fatalf("create registry chaincode: %v", err)
	}
	if err := chaincode.Start(); err != nil {
		log.Fatalf("start registry chaincode: %v", err)
	}
}
