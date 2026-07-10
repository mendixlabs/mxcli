// SPDX-License-Identifier: Apache-2.0

// Package modelsdk provides a Go library for reading and modifying Mendix projects.
//
// This library is a Go alternative to the Mendix Model SDK and Mendix Platform SDK,
// allowing direct manipulation of Mendix project files (.mpr) on disk.
//
// # Overview
//
// Mendix projects are stored in .mpr files which are SQLite databases containing
// BSON-encoded model elements. This library provides:
//
//   - Reading and parsing MPR files (both v1 and v2 formats)
//   - Type-safe access to all Mendix model elements
//   - Creating, updating, and deleting model elements
//   - Exporting models to JSON format
//
// # Quick Start
//
//	package main
//
//	import (
//	    "fmt"
//	    "github.com/JordtenBulte-OLC/mxcli"
//	    "github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
//	)
//
//	func main() {
//	    // Open a Mendix project
//	    reader, err := mpr.Open("/path/to/MyApp.mpr")
//	    if err != nil {
//	        panic(err)
//	    }
//	    defer reader.Close()
//
//	    // List all modules
//	    modules, err := reader.ListModules()
//	    if err != nil {
//	        panic(err)
//	    }
//
//	    for _, m := range modules {
//	        fmt.Printf("Module: %s\n", m.Name)
//	    }
//	}
//
// # MPR File Formats
//
// The library supports both MPR v1 (single file) and MPR v2 (with mprcontents folder)
// formats. MPR v2 was introduced in Mendix Studio Pro 10.18.
//
// # Model Structure
//
// The Mendix model is organized hierarchically:
//
//   - Project
//   - Modules
//   - Domain Models (Entities, Attributes, Associations)
//   - Microflows and Nanoflows
//   - Pages, Layouts, and Snippets
//   - Enumerations and Constants
//   - Scheduled Events
//
// Each element has a unique ID and belongs to a container element.
//
// # Reading Models
//
// Use the mpr.Reader to read model elements:
//
//	reader, _ := mpr.Open("MyApp.mpr")
//
//	// Get all domain models
//	domainModels, _ := reader.ListDomainModels()
//
//	// Get a specific module
//	module, _ := reader.GetModuleByName("MyModule")
//
//	// Get the domain model for a module
//	dm, _ := reader.GetDomainModel(module.ID)
//
// # Modifying Models
//
// Use the mpr.Writer to modify model elements:
//
//	writer, _ := mpr.NewWriter("MyApp.mpr")
//	defer writer.Close()
//
//	// Create a new entity
//	entity := &domainmodel.Entity{
//	    Name: "Customer",
//	    Persistable: true,
//	}
//	writer.CreateEntity(domainModelID, entity)
//
//	// Add an attribute
//	attr := &domainmodel.Attribute{
//	    Name: "CustomerName",
//	    Type: &domainmodel.StringAttributeType{Length: 200},
//	}
//	writer.AddAttribute(domainModelID, entity.ID, attr)
//
// # Thread Safety
//
// The Reader is safe for concurrent read access. The Writer should only be used
// from a single goroutine. For concurrent modifications, use transactions.
//
// # Error Handling
//
// All functions that can fail return an error. Errors include:
//
//   - File not found
//   - Invalid MPR format
//   - Element not found
//   - BSON parsing errors
//   - SQLite errors
package modelsdk

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// Version is the library version.
const Version = "0.1.0"

// Re-export commonly used types for convenience.
type (
	// ID is a unique identifier for model elements.
	ID = model.ID

	// Module represents a Mendix module.
	Module = model.Module

	// Project represents a Mendix project.
	Project = model.Project

	// Enumeration represents an enumeration type.
	Enumeration = model.Enumeration

	// Constant represents a constant value.
	Constant = model.Constant

	// ConstantDataType represents the data type of a constant.
	ConstantDataType = model.ConstantDataType

	// ScheduledEvent represents a scheduled event.
	ScheduledEvent = model.ScheduledEvent

	// DomainModel represents a module's domain model.
	DomainModel = domainmodel.DomainModel

	// Entity represents an entity in a domain model.
	Entity = domainmodel.Entity

	// Attribute represents an attribute of an entity.
	Attribute = domainmodel.Attribute

	// Association represents an association between entities.
	Association = domainmodel.Association

	// Microflow represents a microflow.
	Microflow = microflows.Microflow

	// Nanoflow represents a nanoflow.
	Nanoflow = microflows.Nanoflow

	// Page represents a page.
	Page = pages.Page

	// Layout represents a layout.
	Layout = pages.Layout

	// Snippet represents a page snippet.
	Snippet = pages.Snippet

	// Reader provides methods to read Mendix project files.
	Reader = mpr.Reader

	// Writer provides methods to write Mendix project files.
	Writer = mpr.Writer
)

// Open opens an MPR file for reading.
func Open(path string) (*Reader, error) {
	return mpr.Open(path)
}

// OpenForWriting opens an MPR file for reading and writing.
func OpenForWriting(path string) (*Writer, error) {
	return mpr.NewWriter(path)
}

// NewStringAttribute creates a new string attribute.
func NewStringAttribute(name string, length int) *Attribute {
	return &Attribute{
		Name: name,
		Type: &domainmodel.StringAttributeType{Length: length},
	}
}

// NewIntegerAttribute creates a new integer attribute.
func NewIntegerAttribute(name string) *Attribute {
	return &Attribute{
		Name: name,
		Type: &domainmodel.IntegerAttributeType{},
	}
}

// NewDecimalAttribute creates a new decimal attribute.
func NewDecimalAttribute(name string) *Attribute {
	return &Attribute{
		Name: name,
		Type: &domainmodel.DecimalAttributeType{},
	}
}

// NewBooleanAttribute creates a new boolean attribute.
func NewBooleanAttribute(name string) *Attribute {
	return &Attribute{
		Name: name,
		Type: &domainmodel.BooleanAttributeType{},
	}
}

// NewDateTimeAttribute creates a new date/time attribute.
func NewDateTimeAttribute(name string, localize bool) *Attribute {
	return &Attribute{
		Name: name,
		Type: &domainmodel.DateTimeAttributeType{LocalizeDate: localize},
	}
}

// NewEnumerationAttribute creates a new enumeration attribute.
func NewEnumerationAttribute(name string, enumerationID ID) *Attribute {
	return &Attribute{
		Name: name,
		Type: &domainmodel.EnumerationAttributeType{EnumerationID: enumerationID},
	}
}

// NewEntity creates a new persistable entity.
func NewEntity(name string) *Entity {
	return &Entity{
		Name:        name,
		Persistable: true,
	}
}

// NewNonPersistableEntity creates a new non-persistable entity.
func NewNonPersistableEntity(name string) *Entity {
	return &Entity{
		Name:        name,
		Persistable: false,
	}
}

// NewAssociation creates a new reference association.
func NewAssociation(name string, parentID, childID ID) *Association {
	return &Association{
		Name:     name,
		ParentID: parentID,
		ChildID:  childID,
		Type:     domainmodel.AssociationTypeReference,
		Owner:    domainmodel.AssociationOwnerDefault,
	}
}

// NewReferenceSetAssociation creates a new reference set association.
func NewReferenceSetAssociation(name string, parentID, childID ID) *Association {
	return &Association{
		Name:     name,
		ParentID: parentID,
		ChildID:  childID,
		Type:     domainmodel.AssociationTypeReferenceSet,
		Owner:    domainmodel.AssociationOwnerDefault,
	}
}

// NewMicroflow creates a new microflow.
func NewMicroflow(name string) *Microflow {
	return &Microflow{
		Name: name,
	}
}

// NewNanoflow creates a new nanoflow.
func NewNanoflow(name string) *Nanoflow {
	return &Nanoflow{
		Name: name,
	}
}

// NewPage creates a new page.
func NewPage(name string) *Page {
	return &Page{
		Name: name,
	}
}

// GenerateID generates a new unique ID for model elements.
func GenerateID() ID {
	return ID(mpr.GenerateID())
}
