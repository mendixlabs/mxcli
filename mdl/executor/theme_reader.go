// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// ThemeProperty represents a single design property definition from design-properties.json.
type ThemeProperty struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"` // "Toggle", "Dropdown", "ColorPicker", "ToggleButtonGroup"
	Description string        `json:"description"`
	Class       string        `json:"class"`   // For Toggle type: the CSS class toggled
	Options     []ThemeOption `json:"options"` // For Dropdown/ColorPicker/ToggleButtonGroup
}

// ThemeOption represents a single option within a dropdown/picker design property.
type ThemeOption struct {
	Name  string `json:"name"`
	Class string `json:"class"`
}

// ThemeRegistry holds all design property definitions loaded from the project's themesource.
type ThemeRegistry struct {
	// WidgetProperties maps design-properties.json widget type key to its properties.
	// Keys: "Widget", "DivContainer", "Button", "DataGrid", pluggable widget IDs, etc.
	WidgetProperties map[string][]ThemeProperty
}

// loadThemeRegistry reads and merges all design-properties.json files from the project's
// themesource directories (themesource/*/web/design-properties.json).
func loadThemeRegistry(projectDir string) (*ThemeRegistry, error) {
	registry := &ThemeRegistry{
		WidgetProperties: make(map[string][]ThemeProperty),
	}

	themesourceDir := filepath.Join(projectDir, "themesource")
	if _, err := os.Stat(themesourceDir); os.IsNotExist(err) {
		return registry, nil // No themesource directory — return empty registry
	}

	// Walk themesource/*/web/design-properties.json
	entries, err := os.ReadDir(themesourceDir)
	if err != nil {
		return nil, mdlerrors.NewBackend("read themesource directory", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dpPath := filepath.Join(themesourceDir, entry.Name(), "web", "design-properties.json")
		if _, err := os.Stat(dpPath); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(dpPath)
		if err != nil {
			continue // Skip unreadable files
		}

		var fileProps map[string][]ThemeProperty
		if err := json.Unmarshal(data, &fileProps); err != nil {
			continue // Skip malformed files
		}

		// Merge into registry
		for widgetType, props := range fileProps {
			registry.WidgetProperties[widgetType] = append(registry.WidgetProperties[widgetType], props...)
		}
	}

	return registry, nil
}

// GetPropertiesForWidget returns properties applicable to a widget type,
// including inherited "Widget" properties that apply to all widget types.
func (r *ThemeRegistry) GetPropertiesForWidget(widgetTypeKey string) []ThemeProperty {
	var result []ThemeProperty

	// Add inherited "Widget" properties first (apply to all widgets)
	if widgetProps, ok := r.WidgetProperties["Widget"]; ok {
		result = append(result, widgetProps...)
	}

	// Add type-specific properties
	if widgetTypeKey != "Widget" {
		if typeProps, ok := r.WidgetProperties[widgetTypeKey]; ok {
			result = append(result, typeProps...)
		}
	}

	return result
}

// mdlKeywordToDesignPropsKey maps MDL widget type keywords (uppercase) to
// the keys used in design-properties.json.
var mdlKeywordToDesignPropsKey = map[string]string{
	"container":         "DivContainer",
	"customcontainer":   "DivContainer",
	"actionbutton":      "Button",
	"linkbutton":        "Button",
	"textbox":           "TextBox",
	"textarea":          "TextArea",
	"datepicker":        "DatePicker",
	"checkbox":          "CheckBox",
	"radiobuttons":      "RadioButtons",
	"combobox":          "ReferenceSelector",
	"dropdown":          "DropDown",
	"referenceselector": "ReferenceSelector",
	"datagrid":          "DataGrid",
	"dataview":          "DataView",
	"listview":          "ListView",
	"gallery":           "Gallery",
	"layoutgrid":        "LayoutGrid",
	"dynamictext":       "DynamicText",
	"statictext":        "Label",
	"image":             "Image",
	"staticimage":       "StaticImageViewer",
	"dynamicimage":      "DynamicImageViewer",
	"navigationlist":    "NavigationList",
	"snippetcall":       "SnippetCall",
	"header":            "Header",
	"footer":            "Footer",
}

// resolveDesignPropsKey converts an MDL widget type keyword (e.g., "CONTAINER")
// to the design-properties.json key (e.g., "DivContainer"). Falls back to the input as-is
// for unrecognized types (e.g., pluggable widget identifiers).
func resolveDesignPropsKey(mdlKeyword string) string {
	upper := strings.ToUpper(mdlKeyword)
	if key, ok := mdlKeywordToDesignPropsKey[upper]; ok {
		return key
	}
	return mdlKeyword
}

// bsonTypeToDesignPropsKey maps BSON $Type values to design-properties.json keys.
var bsonTypeToDesignPropsKey = map[string]string{
	"Forms$DivContainer":       "DivContainer",
	"Pages$DivContainer":       "DivContainer",
	"Forms$ActionButton":       "Button",
	"Pages$ActionButton":       "Button",
	"Forms$TextBox":            "TextBox",
	"Pages$TextBox":            "TextBox",
	"Forms$TextArea":           "TextArea",
	"Pages$TextArea":           "TextArea",
	"Forms$DatePicker":         "DatePicker",
	"Pages$DatePicker":         "DatePicker",
	"Forms$CheckBox":           "CheckBox",
	"Pages$CheckBox":           "CheckBox",
	"Forms$RadioButtons":       "RadioButtons",
	"Pages$RadioButtons":       "RadioButtons",
	"Forms$ReferenceSelector":  "ReferenceSelector",
	"Pages$ReferenceSelector":  "ReferenceSelector",
	"Forms$DropDown":           "DropDown",
	"Pages$DropDown":           "DropDown",
	"Forms$DataGrid":           "DataGrid",
	"Pages$DataGrid":           "DataGrid",
	"Forms$DataView":           "DataView",
	"Pages$DataView":           "DataView",
	"Forms$ListView":           "ListView",
	"Pages$ListView":           "ListView",
	"Forms$LayoutGrid":         "LayoutGrid",
	"Pages$LayoutGrid":         "LayoutGrid",
	"Forms$DynamicText":        "DynamicText",
	"Pages$DynamicText":        "DynamicText",
	"Forms$Label":              "Label",
	"Pages$Label":              "Label",
	"Forms$StaticImageViewer":  "StaticImageViewer",
	"Pages$StaticImageViewer":  "StaticImageViewer",
	"Forms$DynamicImageViewer": "DynamicImageViewer",
	"Pages$DynamicImageViewer": "DynamicImageViewer",
	"Forms$Gallery":            "Gallery",
	"Pages$Gallery":            "Gallery",
	"Forms$NavigationList":     "NavigationList",
	"Pages$NavigationList":     "NavigationList",
}

// widgetTypeDisplayName maps BSON $Type to a short display name for output.
var widgetTypeDisplayName = map[string]string{
	"Forms$DivContainer":         "Container",
	"Pages$DivContainer":         "Container",
	"Forms$ActionButton":         "ActionButton",
	"Pages$ActionButton":         "ActionButton",
	"Forms$TextBox":              "TextBox",
	"Pages$TextBox":              "TextBox",
	"Forms$TextArea":             "TextArea",
	"Pages$TextArea":             "TextArea",
	"Forms$DatePicker":           "DatePicker",
	"Pages$DatePicker":           "DatePicker",
	"Forms$CheckBox":             "CheckBox",
	"Pages$CheckBox":             "CheckBox",
	"Forms$RadioButtons":         "RadioButtons",
	"Pages$RadioButtons":         "RadioButtons",
	"Forms$ReferenceSelector":    "ReferenceSelector",
	"Pages$ReferenceSelector":    "ReferenceSelector",
	"Forms$DropDown":             "DropDown",
	"Pages$DropDown":             "DropDown",
	"Forms$DataGrid":             "DataGrid",
	"Pages$DataGrid":             "DataGrid",
	"Forms$DataView":             "DataView",
	"Pages$DataView":             "DataView",
	"Forms$ListView":             "ListView",
	"Pages$ListView":             "ListView",
	"Forms$LayoutGrid":           "LayoutGrid",
	"Pages$LayoutGrid":           "LayoutGrid",
	"Forms$DynamicText":          "DynamicText",
	"Pages$DynamicText":          "DynamicText",
	"Forms$Title":                "Title",
	"Pages$Title":                "Title",
	"Forms$Text":                 "StaticText",
	"Pages$Text":                 "StaticText",
	"Forms$Label":                "Label",
	"Pages$Label":                "Label",
	"Forms$StaticImageViewer":    "StaticImage",
	"Pages$StaticImageViewer":    "StaticImage",
	"Forms$DynamicImageViewer":   "DynamicImage",
	"Pages$DynamicImageViewer":   "DynamicImage",
	"Forms$Gallery":              "Gallery",
	"Pages$Gallery":              "Gallery",
	"Forms$NavigationList":       "NavigationList",
	"Pages$NavigationList":       "NavigationList",
	"Forms$SnippetCallWidget":    "SnippetCall",
	"Pages$SnippetCallWidget":    "SnippetCall",
	"CustomWidgets$CustomWidget": "CustomWidget",
}

// getWidgetDisplayName returns a short display name for a BSON widget $Type.
func getWidgetDisplayName(bsonType string) string {
	if name, ok := widgetTypeDisplayName[bsonType]; ok {
		return name
	}
	return bsonType
}
