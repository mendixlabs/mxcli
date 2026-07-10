// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"sort"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// CreatePage creates a new page.
func (w *Writer) CreatePage(page *pages.Page) error {
	if page.ID == "" {
		page.ID = model.ID(generateUUID())
	}
	page.TypeName = "Forms$Page"

	contents, err := w.serializePage(page)
	if err != nil {
		return fmt.Errorf("failed to serialize page: %w", err)
	}

	return w.insertUnit(string(page.ID), string(page.ContainerID), "Documents", "Forms$Page", contents)
}

// UpdatePage updates an existing page.
func (w *Writer) UpdatePage(page *pages.Page) error {
	contents, err := w.serializePage(page)
	if err != nil {
		return fmt.Errorf("failed to serialize page: %w", err)
	}

	return w.updateUnit(string(page.ID), contents)
}

// DeletePage deletes a page.
func (w *Writer) DeletePage(id model.ID) error {
	return w.deleteUnit(string(id))
}

// MovePage moves a page to a new container (folder or module).
// Only updates the ContainerID in the database, preserving all BSON content as-is.
func (w *Writer) MovePage(page *pages.Page) error {
	return w.moveUnitByID(string(page.ID), string(page.ContainerID))
}

// CreateLayout creates a new layout.
func (w *Writer) CreateLayout(layout *pages.Layout) error {
	if layout.ID == "" {
		layout.ID = model.ID(generateUUID())
	}
	layout.TypeName = "Forms$Layout"

	contents, err := w.serializeLayout(layout)
	if err != nil {
		return fmt.Errorf("failed to serialize layout: %w", err)
	}

	return w.insertUnit(string(layout.ID), string(layout.ContainerID), "Documents", "Forms$Layout", contents)
}

// UpdateLayout updates an existing layout.
func (w *Writer) UpdateLayout(layout *pages.Layout) error {
	contents, err := w.serializeLayout(layout)
	if err != nil {
		return fmt.Errorf("failed to serialize layout: %w", err)
	}

	return w.updateUnit(string(layout.ID), contents)
}

// DeleteLayout deletes a layout.
func (w *Writer) DeleteLayout(id model.ID) error {
	return w.deleteUnit(string(id))
}

// CreateSnippet creates a new snippet.
func (w *Writer) CreateSnippet(snippet *pages.Snippet) error {
	if snippet.ID == "" {
		snippet.ID = model.ID(generateUUID())
	}
	snippet.TypeName = "Forms$Snippet"

	contents, err := w.serializeSnippet(snippet)
	if err != nil {
		return fmt.Errorf("failed to serialize snippet: %w", err)
	}

	return w.insertUnit(string(snippet.ID), string(snippet.ContainerID), "Documents", "Forms$Snippet", contents)
}

// DeleteSnippet deletes a snippet.
func (w *Writer) DeleteSnippet(id model.ID) error {
	return w.deleteUnit(string(id))
}

// UpdateSnippet updates an existing snippet.
func (w *Writer) UpdateSnippet(snippet *pages.Snippet) error {
	contents, err := w.serializeSnippet(snippet)
	if err != nil {
		return fmt.Errorf("failed to serialize snippet: %w", err)
	}

	return w.updateUnit(string(snippet.ID), contents)
}

// MoveSnippet moves a snippet to a new container (folder or module).
// Only updates the ContainerID in the database, preserving all BSON content as-is.
func (w *Writer) MoveSnippet(snippet *pages.Snippet) error {
	return w.moveUnitByID(string(snippet.ID), string(snippet.ContainerID))
}

// popupDimension returns the pop-up width/height as the int64 BSON value Studio
// Pro uses. Studio Pro's own default is 0 (auto-size), so 0 is a valid value and
// is written through verbatim (issue #713); only a stray negative is clamped to 0.
func popupDimension(n int) int64 {
	if n < 0 {
		return 0
	}
	return int64(n)
}

func (w *Writer) serializePage(page *pages.Page) ([]byte, error) {
	// Build document with Mendix 10+ format (Forms$Page)
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(page.ID))},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "AllowedModuleRoles", Value: allowedModuleRolesArray(page.AllowedRoles)},
		{Key: "Appearance", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$Appearance"},
			{Key: "Class", Value: page.Class},
			{Key: "DesignProperties", Value: bson.A{int32(3)}},
			{Key: "DynamicClasses", Value: ""},
			{Key: "Style", Value: page.Style},
		}},
		{Key: "Autofocus", Value: "DesktopOnly"},
		{Key: "CanvasHeight", Value: int64(600)},
		{Key: "CanvasWidth", Value: int64(1200)},
		{Key: "Documentation", Value: page.Documentation},
		{Key: "Excluded", Value: page.Excluded},
		{Key: "ExportLevel", Value: "Hidden"},
	}

	// Add FormCall (LayoutCall) if present
	if page.LayoutCall != nil {
		// Build arguments array
		// Format: [3] for empty, [2, {arg1}, {arg2}...] for non-empty
		// Each argument is a bson.D document
		args := bson.A{int32(3)} // Start with empty marker
		hasItems := false
		for _, arg := range page.LayoutCall.Arguments {
			// Parameter uses a qualified name string (e.g., "Atlas_Core.Atlas_TopBar.Main")
			// not a binary ID
			argDoc := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(arg.ID))},
				{Key: "$Type", Value: "Forms$FormCallArgument"},
				{Key: "Parameter", Value: string(arg.ParameterID)}, // Qualified name string
			}
			// Add widgets if present
			if arg.Widget != nil {
				argDoc = append(argDoc, bson.E{Key: "Widgets", Value: serializeWidgetArray([]pages.Widget{arg.Widget})})
			} else {
				argDoc = append(argDoc, bson.E{Key: "Widgets", Value: bson.A{int32(3)}})
			}
			if !hasItems {
				// First item: change version marker from 3 to 2
				args = bson.A{int32(2)}
				hasItems = true
			}
			// Append the argument document directly
			args = append(args, argDoc)
		}

		formCall := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(page.LayoutCall.ID))},
			{Key: "$Type", Value: "Forms$LayoutCall"},
			{Key: "Arguments", Value: args},
			{Key: "Form", Value: page.LayoutCall.LayoutName}, // Qualified name string, not binary ID
		}
		doc = append(doc, bson.E{Key: "FormCall", Value: formCall})
	}

	doc = append(doc, bson.E{Key: "MarkAsUsed", Value: page.MarkAsUsed})
	doc = append(doc, bson.E{Key: "Name", Value: page.Name})

	// Add Parameters array
	// Format: [3] for empty, [3, {param1}, {param2}...] for non-empty
	// Each parameter is a bson.D document (which serializes as array of key-value pairs)
	params := bson.A{int32(3)} // Start with version marker
	for _, p := range page.Parameters {
		paramID := string(p.ID)
		if paramID == "" {
			paramID = generateUUID()
		}

		// Build ParameterType — entity params use DataTypes$ObjectType,
		// primitive params use DataTypes$StringType, DataTypes$IntegerType, etc.
		paramTypeID := generateUUID()
		var paramType bson.D
		if p.TypeName != "" {
			// Primitive type parameter
			paramType = bson.D{
				{Key: "$ID", Value: idToBsonBinary(paramTypeID)},
				{Key: "$Type", Value: p.TypeName},
			}
		} else {
			// Entity type parameter (default)
			paramType = bson.D{
				{Key: "$ID", Value: idToBsonBinary(paramTypeID)},
				{Key: "$Type", Value: "DataTypes$ObjectType"},
				{Key: "Entity", Value: p.EntityName},
			}
		}

		paramDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(paramID)},
			{Key: "$Type", Value: "Forms$PageParameter"},
			{Key: "DefaultValue", Value: p.DefaultValue},
			{Key: "IsRequired", Value: p.IsRequired},
			{Key: "Name", Value: p.Name},
			{Key: "ParameterType", Value: paramType},
		}
		// Append the parameter document directly (bson.D serializes as array of {Key, Value})
		params = append(params, paramDoc)
	}
	doc = append(doc, bson.E{Key: "Parameters", Value: params})

	doc = append(doc, bson.E{Key: "PopupCloseAction", Value: ""})
	doc = append(doc, bson.E{Key: "PopupHeight", Value: popupDimension(page.PopupHeight)})
	doc = append(doc, bson.E{Key: "PopupResizable", Value: page.PopupResizable})
	doc = append(doc, bson.E{Key: "PopupWidth", Value: popupDimension(page.PopupWidth)})

	// Add Title
	// Mendix uses [3] for empty arrays, [2, item1, item2, ...] for non-empty arrays
	// Items go directly after the version marker, NOT nested in another array
	if page.Title != nil {
		titleItems := bson.A{int32(3)} // Start with empty marker
		if len(page.Title.Translations) > 0 {
			titleItems = bson.A{int32(2)} // version 2 for non-empty
			langs := make([]string, 0, len(page.Title.Translations))
			for lang := range page.Title.Translations {
				langs = append(langs, lang)
			}
			sort.Strings(langs)
			for _, langCode := range langs {
				titleItems = append(titleItems, bson.D{
					{Key: "$ID", Value: idToBsonBinary(generateUUID())},
					{Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: langCode},
					{Key: "Text", Value: page.Title.Translations[langCode]},
				})
			}
		}
		titleDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(page.Title.ID))},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: titleItems},
		}
		doc = append(doc, bson.E{Key: "Title", Value: titleDoc})
	} else {
		// Empty title
		doc = append(doc, bson.E{Key: "Title", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}})
	}

	doc = append(doc, bson.E{Key: "Url", Value: page.URL})
	doc = append(doc, bson.E{Key: "Variables", Value: serializeLocalVariables(page.Variables)})

	return bson.Marshal(doc)
}

func (w *Writer) serializeLayout(layout *pages.Layout) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: string(layout.ID)},
		{Key: "$Type", Value: layout.TypeName},
		{Key: "Name", Value: layout.Name},
		{Key: "Documentation", Value: layout.Documentation},
		{Key: "LayoutType", Value: string(layout.LayoutType)},
	}
	return bson.Marshal(doc)
}

func (w *Writer) serializeSnippet(snippet *pages.Snippet) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(snippet.ID))},
		{Key: "$Type", Value: "Forms$Snippet"},
		{Key: "CanvasHeight", Value: int64(600)},
		{Key: "CanvasWidth", Value: int64(800)},
		{Key: "Documentation", Value: snippet.Documentation},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Name", Value: snippet.Name},
	}

	// Add parameters
	params := bson.A{int32(3)} // Version prefix
	for _, param := range snippet.Parameters {
		paramDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(param.ID))},
			{Key: "$Type", Value: "Forms$SnippetParameter"},
			{Key: "Name", Value: param.Name},
		}
		if param.EntityName != "" {
			paramDoc = append(paramDoc, bson.E{Key: "ParameterType", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DataTypes$ObjectType"},
				{Key: "Entity", Value: param.EntityName},
			}})
		}
		params = append(params, paramDoc)
	}
	doc = append(doc, bson.E{Key: "Parameters", Value: params})

	// Add fields to match Studio Pro format
	doc = append(doc, bson.E{Key: "Type", Value: ""})
	doc = append(doc, bson.E{Key: "Variables", Value: serializeLocalVariables(snippet.Variables)})
	doc = append(doc, bson.E{Key: "Excluded", Value: false})

	// Use "Widgets" (plural) array, matching Studio Pro format
	doc = append(doc, bson.E{Key: "Widgets", Value: serializeWidgetArray(snippet.Widgets)})

	return bson.Marshal(doc)
}

// serializeLocalVariables serializes page/snippet local variables to BSON array format.
// Returns [3] for empty, [3, {var1}, {var2}...] for non-empty.
func serializeLocalVariables(vars []*pages.LocalVariable) bson.A {
	result := bson.A{int32(3)} // Version marker
	for _, v := range vars {
		varID := string(v.ID)
		if varID == "" {
			varID = generateUUID()
		}

		varTypeID := generateUUID()
		varType := bson.D{
			{Key: "$ID", Value: idToBsonBinary(varTypeID)},
			{Key: "$Type", Value: v.VariableType},
		}
		// For ObjectType, include the Entity field
		if v.VariableType == "DataTypes$ObjectType" {
			varType = append(varType, bson.E{Key: "Entity", Value: v.Name})
		}

		varDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(varID)},
			{Key: "$Type", Value: "Forms$LocalVariable"},
			{Key: "DefaultValue", Value: v.DefaultValue},
			{Key: "Name", Value: v.Name},
			{Key: "VariableType", Value: varType},
		}
		result = append(result, varDoc)
	}
	return result
}
