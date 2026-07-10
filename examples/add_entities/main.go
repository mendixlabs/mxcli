// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: add_entities <mpr-path>")
		os.Exit(1)
	}

	mprPath := os.Args[1]

	// Open writer
	fmt.Println("Opening project...")
	writer, err := mpr.NewWriter(mprPath)
	if err != nil {
		fmt.Printf("Error opening writer: %v\n", err)
		os.Exit(1)
	}
	defer writer.Close()

	reader := writer.Reader()

	// Find MyFirstModule
	modules, err := reader.ListModules()
	if err != nil {
		fmt.Printf("Error listing modules: %v\n", err)
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

	fmt.Printf("Found module: %s (ID: %s)\n", myModule.Name, myModule.ID)

	// Get domain model for MyFirstModule
	dm, err := reader.GetDomainModel(myModule.ID)
	if err != nil {
		fmt.Printf("Error getting domain model: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Domain model ID: %s\n", dm.ID)
	fmt.Printf("Existing entities: %d\n", len(dm.Entities))

	// Create Customer entity
	customerEntity := &domainmodel.Entity{
		BaseElement: model.BaseElement{
			TypeName: "DomainModels$Entity",
		},
		Name:        "Customer",
		Persistable: true,
		Location: model.Point{
			X: 100,
			Y: 100,
		},
		Attributes: []*domainmodel.Attribute{
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "FirstName",
				Type: &domainmodel.StringAttributeType{Length: 100},
			},
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "LastName",
				Type: &domainmodel.StringAttributeType{Length: 100},
			},
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "Email",
				Type: &domainmodel.StringAttributeType{Length: 200},
			},
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "Phone",
				Type: &domainmodel.StringAttributeType{Length: 50},
			},
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "Address",
				Type: &domainmodel.StringAttributeType{Length: 500},
			},
		},
	}

	fmt.Println("\nCreating Customer entity...")
	err = writer.CreateEntity(dm.ID, customerEntity)
	if err != nil {
		fmt.Printf("Error creating Customer: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Customer created with ID: %s\n", customerEntity.ID)

	// Create Order entity
	orderEntity := &domainmodel.Entity{
		BaseElement: model.BaseElement{
			TypeName: "DomainModels$Entity",
		},
		Name:        "Order",
		Persistable: true,
		Location: model.Point{
			X: 400,
			Y: 100,
		},
		Attributes: []*domainmodel.Attribute{
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "OrderNumber",
				Type: &domainmodel.StringAttributeType{Length: 50},
			},
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "OrderDate",
				Type: &domainmodel.DateTimeAttributeType{LocalizeDate: true},
			},
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "TotalAmount",
				Type: &domainmodel.DecimalAttributeType{},
			},
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "Status",
				Type: &domainmodel.StringAttributeType{Length: 50},
			},
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "Notes",
				Type: &domainmodel.StringAttributeType{Length: 2000},
			},
		},
	}

	fmt.Println("Creating Order entity...")
	err = writer.CreateEntity(dm.ID, orderEntity)
	if err != nil {
		fmt.Printf("Error creating Order: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Order created with ID: %s\n", orderEntity.ID)

	// Create association: Customer has many Orders
	association := &domainmodel.Association{
		BaseElement: model.BaseElement{
			TypeName: "DomainModels$Association",
		},
		Name:     "Customer_Order",
		ParentID: customerEntity.ID,
		ChildID:  orderEntity.ID,
		Type:     domainmodel.AssociationTypeReferenceSet,
		Owner:    domainmodel.AssociationOwnerDefault,
	}

	fmt.Println("Creating Customer_Order association...")
	err = writer.CreateAssociation(dm.ID, association)
	if err != nil {
		fmt.Printf("Error creating association: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Association created with ID: %s\n", association.ID)

	// Verify by re-reading
	fmt.Println("\n=== Verification ===")
	updatedDM, err := reader.GetDomainModelByID(dm.ID)
	if err != nil {
		fmt.Printf("Error re-reading domain model: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Entities: %d\n", len(updatedDM.Entities))
	for _, e := range updatedDM.Entities {
		fmt.Printf("  - %s [%d attributes]\n", e.Name, len(e.Attributes))
		for _, a := range e.Attributes {
			typeName := "unknown"
			if a.Type != nil {
				typeName = a.Type.GetTypeName()
			}
			fmt.Printf("      - %s: %s\n", a.Name, typeName)
		}
	}

	fmt.Printf("\nAssociations: %d\n", len(updatedDM.Associations))
	for _, a := range updatedDM.Associations {
		fmt.Printf("  - %s (%s)\n", a.Name, a.Type)
	}

	fmt.Println("\nDone! Customer and Order entities added to MyFirstModule.")
}
