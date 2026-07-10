// SPDX-License-Identifier: Apache-2.0

// Example: Creating a Page in a Mendix Project
//
// This example demonstrates how to create a page with various widgets
// using the modelsdk-go library. It shows the current low-level API
// for building pages programmatically.
package main

import (
	"fmt"
	"os"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: create_page <path-to-mpr-file>")
		fmt.Println()
		fmt.Println("This example creates a Customer edit page in the first module.")
		fmt.Println("WARNING: This will modify the MPR file! Make a backup first.")
		os.Exit(1)
	}

	mprPath := os.Args[1]

	// Open the MPR file for writing
	writer, err := mpr.NewWriter(mprPath)
	if err != nil {
		fmt.Printf("Error opening MPR file: %v\n", err)
		os.Exit(1)
	}
	defer writer.Close()

	reader := writer.Reader()
	fmt.Printf("Opened: %s (Mendix %s)\n", reader.Path(), reader.ProjectVersion())

	// Find the target module
	modules, err := reader.ListModules()
	if err != nil || len(modules) == 0 {
		fmt.Println("No modules found")
		os.Exit(1)
	}

	targetModule := modules[0]
	fmt.Printf("Target module: %s\n", targetModule.Name)

	// Find a layout to use (typically Atlas_Default or similar)
	layouts, err := reader.ListLayouts()
	if err != nil {
		fmt.Printf("Error listing layouts: %v\n", err)
		os.Exit(1)
	}

	var targetLayout *pages.Layout
	for _, l := range layouts {
		if l.Name == "Atlas_Default" || l.Name == "PopupLayout" {
			targetLayout = l
			break
		}
	}
	if targetLayout == nil && len(layouts) > 0 {
		targetLayout = layouts[0]
	}
	if targetLayout == nil {
		fmt.Println("No layouts found")
		os.Exit(1)
	}
	fmt.Printf("Using layout: %s\n", targetLayout.Name)

	// Build layout qualified name (Module.LayoutName)
	// We need to find the layout's module
	hierarchy, err := executor.NewContainerHierarchy(reader)
	if err != nil {
		fmt.Printf("Error building hierarchy: %v\n", err)
		os.Exit(1)
	}
	layoutModuleID := hierarchy.FindModuleID(targetLayout.ContainerID)
	layoutModuleName := hierarchy.GetModuleName(layoutModuleID)
	layoutQualifiedName := layoutModuleName + "." + targetLayout.Name

	// =========================================================================
	// Create the page structure
	// =========================================================================
	fmt.Println("\nCreating Customer_Edit page...")

	// Create the page
	page := &pages.Page{
		BaseElement: model.BaseElement{
			ID:       model.ID(mpr.GenerateID()),
			TypeName: "Pages$Page",
		},
		Name:          "Customer_Edit",
		ContainerID:   targetModule.ID,
		Documentation: "Page for editing customer information. Created via modelsdk-go.",
		URL:           "customer-edit/{Customer}",
		Title: &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{
				"en_US": "Edit Customer",
			},
		},
	}

	// Add a page parameter for the Customer entity
	page.Parameters = []*pages.PageParameter{
		{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$PageParameter",
			},
			Name:       "Customer",
			EntityName: targetModule.Name + ".Customer", // Assumes Customer entity exists
			IsRequired: true,
		},
	}

	// Create the layout call (connects page to layout)
	page.LayoutCall = &pages.LayoutCall{
		BaseElement: model.BaseElement{
			ID:       model.ID(mpr.GenerateID()),
			TypeName: "Forms$LayoutCall",
		},
		LayoutName: layoutQualifiedName,
	}

	// =========================================================================
	// Build the widget tree
	// =========================================================================

	// Create a LayoutGrid as the root container
	layoutGrid := &pages.LayoutGrid{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$LayoutGrid",
			},
			Name: "mainGrid",
		},
	}

	// Create a row with a single column
	row := &pages.LayoutGridRow{
		BaseElement: model.BaseElement{
			ID:       model.ID(mpr.GenerateID()),
			TypeName: "Forms$LayoutGridRow",
		},
	}

	column := &pages.LayoutGridColumn{
		BaseElement: model.BaseElement{
			ID:       model.ID(mpr.GenerateID()),
			TypeName: "Forms$LayoutGridColumn",
		},
		Weight: 12, // Full width
	}

	// Create a DataView to show/edit the Customer
	dataView := &pages.DataView{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$DataView",
			},
			Name: "customerDataView",
		},
		DataSource: &pages.DataViewSource{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$DataViewSource",
			},
			EntityName:    targetModule.Name + ".Customer",
			ParameterName: "Customer",
		},
	}

	// Create input widgets for the DataView
	// TextBox for Name
	nameTextBox := &pages.TextBox{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$TextBox",
			},
			Name: "nameTextBox",
		},
		Label:         "Name",
		AttributePath: targetModule.Name + ".Customer.Name",
	}

	// TextBox for Email
	emailTextBox := &pages.TextBox{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$TextBox",
			},
			Name: "emailTextBox",
		},
		Label:         "Email",
		AttributePath: targetModule.Name + ".Customer.Email",
	}

	// DatePicker for BirthDate (if exists)
	birthDatePicker := &pages.DatePicker{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$DatePicker",
			},
			Name: "birthDatePicker",
		},
		Label:         "Birth Date",
		AttributePath: targetModule.Name + ".Customer.BirthDate",
	}

	// CheckBox for IsActive
	activeCheckBox := &pages.CheckBox{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$CheckBox",
			},
			Name: "activeCheckBox",
		},
		Label:         "Is Active",
		AttributePath: targetModule.Name + ".Customer.IsActive",
	}

	// Add widgets to the DataView
	dataView.Widgets = []pages.Widget{
		nameTextBox,
		emailTextBox,
		birthDatePicker,
		activeCheckBox,
	}

	// Create footer with Save and Cancel buttons
	saveButton := &pages.ActionButton{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$ActionButton",
			},
			Name: "saveButton",
		},
		Caption: &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": "Save"},
		},
		ButtonStyle: pages.ButtonStylePrimary,
		Action: &pages.SaveChangesClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$SaveChangesClientAction",
			},
			ClosePage: true,
		},
	}

	cancelButton := &pages.ActionButton{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$ActionButton",
			},
			Name: "cancelButton",
		},
		Caption: &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": "Cancel"},
		},
		ButtonStyle: pages.ButtonStyleDefault,
		Action: &pages.CancelChangesClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$CancelChangesClientAction",
			},
			ClosePage: true,
		},
	}

	dataView.FooterWidgets = []pages.Widget{saveButton, cancelButton}

	// Assemble the widget tree
	column.Widgets = []pages.Widget{dataView}
	row.Columns = []*pages.LayoutGridColumn{column}
	layoutGrid.Rows = []*pages.LayoutGridRow{row}

	// Create a conditional visibility wrapper (required for Atlas layouts)
	wrapper := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: "conditionalVisibilityWidget1",
		},
		Widgets: []pages.Widget{layoutGrid},
	}

	// Connect the wrapper to the layout's Main placeholder
	page.LayoutCall.Arguments = []*pages.LayoutCallArgument{
		{
			BaseElement: model.BaseElement{
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Forms$FormCallArgument",
			},
			ParameterID: model.ID(layoutQualifiedName + ".Main"),
			Widget:      wrapper,
		},
	}

	// =========================================================================
	// Create the page
	// =========================================================================
	err = writer.CreatePage(page)
	if err != nil {
		fmt.Printf("Error creating page: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created page: %s.%s (ID: %s)\n", targetModule.Name, page.Name, page.ID)

	// =========================================================================
	// Summary
	// =========================================================================
	fmt.Println("\n=== Summary ===")
	fmt.Println("Created Customer_Edit page with:")
	fmt.Println("  - Page parameter: Customer")
	fmt.Println("  - DataView bound to Customer entity")
	fmt.Println("  - Input widgets: Name, Email, BirthDate, IsActive")
	fmt.Println("  - Footer buttons: Save, Cancel")
	fmt.Println()
	fmt.Println("Note: This page expects a Customer entity with the following attributes:")
	fmt.Println("  - Name (String)")
	fmt.Println("  - Email (String)")
	fmt.Println("  - BirthDate (DateTime)")
	fmt.Println("  - IsActive (Boolean)")
	fmt.Println()
	fmt.Println("Changes have been saved to the MPR file.")
}
