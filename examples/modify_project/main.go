// SPDX-License-Identifier: Apache-2.0

// Example: Modifying a Mendix project
//
// This example demonstrates how to create and modify entities,
// attributes, and associations in a Mendix project.
package main

import (
	"fmt"
	"os"

	"github.com/JordtenBulte-OLC/mxcli"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: modify_project <path-to-mpr-file>")
		fmt.Println()
		fmt.Println("WARNING: This will modify the MPR file! Make a backup first.")
		os.Exit(1)
	}

	mprPath := os.Args[1]

	// Open the MPR file for writing
	writer, err := modelsdk.OpenForWriting(mprPath)
	if err != nil {
		fmt.Printf("Error opening MPR file: %v\n", err)
		os.Exit(1)
	}
	defer writer.Close()

	reader := writer.Reader()

	fmt.Printf("Opened: %s\n", reader.Path())

	// Get the first module's domain model
	modules, err := reader.ListModules()
	if err != nil || len(modules) == 0 {
		fmt.Println("No modules found")
		os.Exit(1)
	}

	targetModule := modules[0]
	fmt.Printf("Working with module: %s\n", targetModule.Name)

	dm, err := reader.GetDomainModel(targetModule.ID)
	if err != nil {
		fmt.Printf("Error getting domain model: %v\n", err)
		os.Exit(1)
	}

	// Create a new entity
	fmt.Println("\nCreating new entity: Customer")
	customerEntity := modelsdk.NewEntity("Customer")
	customerEntity.Documentation = "Represents a customer in the system"
	customerEntity.Location = model.Point{X: 100, Y: 100}

	err = writer.CreateEntity(dm.ID, customerEntity)
	if err != nil {
		fmt.Printf("Error creating entity: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created entity with ID: %s\n", customerEntity.ID)

	// Add attributes to the entity
	fmt.Println("\nAdding attributes...")

	// Customer name
	nameAttr := modelsdk.NewStringAttribute("CustomerName", 200)
	nameAttr.Documentation = "The full name of the customer"
	err = writer.AddAttribute(dm.ID, customerEntity.ID, nameAttr)
	if err != nil {
		fmt.Printf("Error adding CustomerName attribute: %v\n", err)
	} else {
		fmt.Println("  Added: CustomerName (String)")
	}

	// Email
	emailAttr := modelsdk.NewStringAttribute("Email", 254)
	emailAttr.Documentation = "Customer email address"
	err = writer.AddAttribute(dm.ID, customerEntity.ID, emailAttr)
	if err != nil {
		fmt.Printf("Error adding Email attribute: %v\n", err)
	} else {
		fmt.Println("  Added: Email (String)")
	}

	// Phone number
	phoneAttr := modelsdk.NewStringAttribute("PhoneNumber", 20)
	err = writer.AddAttribute(dm.ID, customerEntity.ID, phoneAttr)
	if err != nil {
		fmt.Printf("Error adding PhoneNumber attribute: %v\n", err)
	} else {
		fmt.Println("  Added: PhoneNumber (String)")
	}

	// Registration date
	regDateAttr := modelsdk.NewDateTimeAttribute("RegistrationDate", true)
	err = writer.AddAttribute(dm.ID, customerEntity.ID, regDateAttr)
	if err != nil {
		fmt.Printf("Error adding RegistrationDate attribute: %v\n", err)
	} else {
		fmt.Println("  Added: RegistrationDate (DateTime)")
	}

	// Active status
	activeAttr := modelsdk.NewBooleanAttribute("IsActive")
	err = writer.AddAttribute(dm.ID, customerEntity.ID, activeAttr)
	if err != nil {
		fmt.Printf("Error adding IsActive attribute: %v\n", err)
	} else {
		fmt.Println("  Added: IsActive (Boolean)")
	}

	// Credit balance
	balanceAttr := modelsdk.NewDecimalAttribute("CreditBalance")
	err = writer.AddAttribute(dm.ID, customerEntity.ID, balanceAttr)
	if err != nil {
		fmt.Printf("Error adding CreditBalance attribute: %v\n", err)
	} else {
		fmt.Println("  Added: CreditBalance (Decimal)")
	}

	// Create an Order entity
	fmt.Println("\nCreating new entity: Order")
	orderEntity := modelsdk.NewEntity("Order")
	orderEntity.Documentation = "Represents a customer order"
	orderEntity.Location = model.Point{X: 300, Y: 100}

	err = writer.CreateEntity(dm.ID, orderEntity)
	if err != nil {
		fmt.Printf("Error creating entity: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created entity with ID: %s\n", orderEntity.ID)

	// Add Order attributes
	orderNumberAttr := modelsdk.NewIntegerAttribute("OrderNumber")
	err = writer.AddAttribute(dm.ID, orderEntity.ID, orderNumberAttr)
	if err != nil {
		fmt.Printf("Error adding OrderNumber attribute: %v\n", err)
	} else {
		fmt.Println("  Added: OrderNumber (Integer)")
	}

	orderDateAttr := modelsdk.NewDateTimeAttribute("OrderDate", true)
	err = writer.AddAttribute(dm.ID, orderEntity.ID, orderDateAttr)
	if err != nil {
		fmt.Printf("Error adding OrderDate attribute: %v\n", err)
	} else {
		fmt.Println("  Added: OrderDate (DateTime)")
	}

	totalAttr := modelsdk.NewDecimalAttribute("TotalAmount")
	err = writer.AddAttribute(dm.ID, orderEntity.ID, totalAttr)
	if err != nil {
		fmt.Printf("Error adding TotalAmount attribute: %v\n", err)
	} else {
		fmt.Println("  Added: TotalAmount (Decimal)")
	}

	// Create an association between Customer and Order
	fmt.Println("\nCreating association: Customer_Order")
	customerOrderAssoc := modelsdk.NewAssociation("Customer_Order", customerEntity.ID, orderEntity.ID)
	customerOrderAssoc.Documentation = "Links orders to customers"

	err = writer.CreateAssociation(dm.ID, customerOrderAssoc)
	if err != nil {
		fmt.Printf("Error creating association: %v\n", err)
	} else {
		fmt.Printf("Created association with ID: %s\n", customerOrderAssoc.ID)
	}

	// Create a non-persistable entity (for search/filter)
	fmt.Println("\nCreating non-persistable entity: CustomerSearchCriteria")
	searchEntity := modelsdk.NewNonPersistableEntity("CustomerSearchCriteria")
	searchEntity.Documentation = "Used for customer search functionality"
	searchEntity.Location = model.Point{X: 100, Y: 250}

	err = writer.CreateEntity(dm.ID, searchEntity)
	if err != nil {
		fmt.Printf("Error creating entity: %v\n", err)
	} else {
		fmt.Printf("Created entity with ID: %s\n", searchEntity.ID)
	}

	// Add search criteria attributes
	searchNameAttr := modelsdk.NewStringAttribute("SearchName", 200)
	err = writer.AddAttribute(dm.ID, searchEntity.ID, searchNameAttr)
	if err != nil {
		fmt.Printf("Error adding SearchName attribute: %v\n", err)
	} else {
		fmt.Println("  Added: SearchName (String)")
	}

	searchActiveOnlyAttr := modelsdk.NewBooleanAttribute("ActiveOnly")
	err = writer.AddAttribute(dm.ID, searchEntity.ID, searchActiveOnlyAttr)
	if err != nil {
		fmt.Printf("Error adding ActiveOnly attribute: %v\n", err)
	} else {
		fmt.Println("  Added: ActiveOnly (Boolean)")
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("Created entities:")
	fmt.Printf("  - Customer (ID: %s)\n", customerEntity.ID)
	fmt.Printf("  - Order (ID: %s)\n", orderEntity.ID)
	fmt.Printf("  - CustomerSearchCriteria (ID: %s) - non-persistable\n", searchEntity.ID)
	fmt.Println("Created associations:")
	fmt.Printf("  - Customer_Order (ID: %s)\n", customerOrderAssoc.ID)
	fmt.Println("\nChanges have been saved to the MPR file.")
}

// ExampleBuildComplexDomainModel demonstrates building a more complex domain model
func ExampleBuildComplexDomainModel(writer *modelsdk.Writer, domainModelID modelsdk.ID) {
	// Create Product entity
	product := modelsdk.NewEntity("Product")
	product.Attributes = []*modelsdk.Attribute{
		modelsdk.NewStringAttribute("ProductCode", 50),
		modelsdk.NewStringAttribute("ProductName", 200),
		modelsdk.NewStringAttribute("Description", 2000),
		modelsdk.NewDecimalAttribute("UnitPrice"),
		modelsdk.NewIntegerAttribute("StockQuantity"),
		modelsdk.NewBooleanAttribute("IsAvailable"),
	}

	// Create Category entity
	category := modelsdk.NewEntity("Category")
	category.Attributes = []*modelsdk.Attribute{
		modelsdk.NewStringAttribute("CategoryName", 100),
		modelsdk.NewStringAttribute("Description", 500),
	}

	// Create OrderLine entity
	orderLine := modelsdk.NewEntity("OrderLine")
	orderLine.Attributes = []*modelsdk.Attribute{
		modelsdk.NewIntegerAttribute("Quantity"),
		modelsdk.NewDecimalAttribute("UnitPrice"),
		modelsdk.NewDecimalAttribute("LineTotal"),
	}

	// Create entities
	writer.CreateEntity(domainModelID, product)
	writer.CreateEntity(domainModelID, category)
	writer.CreateEntity(domainModelID, orderLine)

	// Create associations
	// Category has many Products
	writer.CreateAssociation(domainModelID, modelsdk.NewAssociation(
		"Category_Product", category.ID, product.ID,
	))

	// Product has many OrderLines
	writer.CreateAssociation(domainModelID, modelsdk.NewAssociation(
		"Product_OrderLine", product.ID, orderLine.ID,
	))
}

// ExampleCreateEnumeration demonstrates creating an enumeration
func ExampleCreateEnumeration(writer *modelsdk.Writer, moduleID modelsdk.ID) {
	orderStatus := &modelsdk.Enumeration{
		Name:          "OrderStatus",
		Documentation: "Possible statuses for an order",
		ContainerID:   moduleID,
		Values: []model.EnumerationValue{
			{Name: "Draft"},
			{Name: "Submitted"},
			{Name: "Processing"},
			{Name: "Shipped"},
			{Name: "Delivered"},
			{Name: "Cancelled"},
		},
	}

	writer.CreateEnumeration(orderStatus)
}

// ExampleCreateConstant demonstrates creating a constant
func ExampleCreateConstant(writer *modelsdk.Writer, moduleID modelsdk.ID) {
	maxOrdersPerDay := &modelsdk.Constant{
		Name:            "MaxOrdersPerDay",
		Documentation:   "Maximum number of orders a customer can place per day",
		ContainerID:     moduleID,
		Type:            modelsdk.ConstantDataType{Kind: "Integer"},
		DefaultValue:    "10",
		ExposedToClient: false,
	}

	writer.CreateConstant(maxOrdersPerDay)
}
