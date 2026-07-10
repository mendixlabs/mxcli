// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: clear_domain_model <mpr-path>")
		os.Exit(1)
	}

	mprPath := os.Args[1]

	writer, err := mpr.NewWriter(mprPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer writer.Close()

	reader := writer.Reader()

	// Find MyFirstModule
	modules, err := reader.ListModules()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	var myModule *model.Module
	for _, m := range modules {
		if m.Name == "MyFirstModule" {
			myModule = m
			break
		}
	}

	if myModule == nil {
		fmt.Println("MyFirstModule not found")
		os.Exit(1)
	}

	// Get domain model
	dm, err := reader.GetDomainModel(myModule.ID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d entities, %d associations\n", len(dm.Entities), len(dm.Associations))
	fmt.Println("Clearing domain model...")

	// Delete all entities
	for len(dm.Entities) > 0 {
		err = writer.DeleteEntity(dm.ID, dm.Entities[0].ID)
		if err != nil {
			fmt.Printf("Error deleting entity: %v\n", err)
			break
		}
		dm, _ = reader.GetDomainModelByID(dm.ID)
	}

	// Delete all associations
	for len(dm.Associations) > 0 {
		err = writer.DeleteAssociation(dm.ID, dm.Associations[0].ID)
		if err != nil {
			fmt.Printf("Error deleting association: %v\n", err)
			break
		}
		dm, _ = reader.GetDomainModelByID(dm.ID)
	}

	// Verify
	dm, _ = reader.GetDomainModelByID(dm.ID)
	fmt.Printf("After clear: %d entities, %d associations\n", len(dm.Entities), len(dm.Associations))
}
