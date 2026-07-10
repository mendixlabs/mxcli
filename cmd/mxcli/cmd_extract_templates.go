// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var extractTemplatesCmd = &cobra.Command{
	Use:   "extract-templates",
	Short: "Extract widget templates from a Mendix project",
	Long: `Extract pluggable widget type definitions from a Mendix project
and save them as JSON templates for use in mxcli.

This command searches for CustomWidgets in the project and extracts
their type definitions, which can then be embedded in mxcli for
consistent widget creation across projects.

Example:
  mxcli extract-templates -p app.mpr -o templates/mendix-11.6/`,
	RunE: runExtractTemplates,
}

func init() {
	extractTemplatesCmd.Flags().StringP("project", "p", "", "Path to the Mendix project (.mpr file)")
	extractTemplatesCmd.Flags().StringP("output", "o", "", "Output directory for templates")
	extractTemplatesCmd.MarkFlagRequired("project")
	extractTemplatesCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(extractTemplatesCmd)
}

// WidgetTemplate is the JSON structure for a widget template file.
type WidgetTemplate struct {
	WidgetID      string         `json:"widgetId"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	ExtractedFrom string         `json:"extractedFrom"`
	Type          map[string]any `json:"type"`
	Object        map[string]any `json:"object,omitempty"`
}

func runExtractTemplates(cmd *cobra.Command, args []string) error {
	projectPath, _ := cmd.Flags().GetString("project")
	outputDir, _ := cmd.Flags().GetString("output")

	// Open the project
	reader, err := mpr.Open(projectPath)
	if err != nil {
		return fmt.Errorf("failed to open project: %w", err)
	}
	defer reader.Close()

	// Get Mendix version
	version, _ := reader.GetMendixVersion()
	fmt.Printf("Extracting templates from Mendix %s project\n", version)

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Widget IDs to extract
	widgetIDs := []struct {
		id       string
		filename string
		name     string
	}{
		{"com.mendix.widget.web.combobox.Combobox", "combobox.json", "Combo box"},
		{"com.mendix.widget.web.gallery.Gallery", "gallery.json", "Gallery"},
		{"com.mendix.widget.web.datagrid.Datagrid", "datagrid.json", "Data grid 2"},
		{"com.mendix.widget.web.datagridtextfilter.DatagridTextFilter", "datagrid-text-filter.json", "Text filter"},
		{"com.mendix.widget.web.datagriddatefilter.DatagridDateFilter", "datagrid-date-filter.json", "Date filter"},
		{"com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter", "datagrid-dropdown-filter.json", "Dropdown filter"},
		{"com.mendix.widget.web.datagridnumberfilter.DatagridNumberFilter", "datagrid-number-filter.json", "Number filter"},
		{"com.mendix.widget.web.image.Image", "image.json", "Image"},
	}

	extracted := 0
	for _, w := range widgetIDs {
		rawWidget, err := reader.FindCustomWidgetType(w.id)
		if err != nil {
			fmt.Printf("  [SKIP] %s: %v\n", w.name, err)
			continue
		}
		if rawWidget == nil {
			fmt.Printf("  [SKIP] %s: not found in project\n", w.name)
			continue
		}

		// Convert BSON to JSON-compatible map
		typeMap, err := bsonDToMap(rawWidget.RawType)
		if err != nil {
			fmt.Printf("  [SKIP] %s: failed to convert type BSON: %v\n", w.name, err)
			continue
		}

		var objectMap map[string]any
		if rawWidget.RawObject != nil {
			objectMap, err = bsonDToMap(rawWidget.RawObject)
			if err != nil {
				fmt.Printf("  [SKIP] %s: failed to convert object BSON: %v\n", w.name, err)
				continue
			}
		}

		template := WidgetTemplate{
			WidgetID:      w.id,
			Name:          w.name,
			Version:       version,
			ExtractedFrom: rawWidget.UnitID,
			Type:          typeMap,
			Object:        objectMap,
		}

		// Write to file
		outPath := filepath.Join(outputDir, w.filename)
		data, err := json.MarshalIndent(template, "", "  ")
		if err != nil {
			fmt.Printf("  [SKIP] %s: failed to marshal JSON: %v\n", w.name, err)
			continue
		}

		if err := os.WriteFile(outPath, data, 0644); err != nil {
			fmt.Printf("  [SKIP] %s: failed to write file: %v\n", w.name, err)
			continue
		}

		fmt.Printf("  [OK] %s -> %s\n", w.name, w.filename)
		extracted++
	}

	fmt.Printf("\nExtracted %d widget templates to %s\n", extracted, outputDir)
	return nil
}

// bsonDToMap converts a bson.D to a JSON-compatible map.
func bsonDToMap(doc bson.D) (map[string]any, error) {
	result := make(map[string]any)
	for _, elem := range doc {
		result[elem.Key] = convertBsonValue(elem.Value)
	}
	return result, nil
}

// convertBsonValue converts BSON values to JSON-compatible types.
func convertBsonValue(v any) any {
	switch val := v.(type) {
	case bson.D:
		m := make(map[string]any)
		for _, elem := range val {
			m[elem.Key] = convertBsonValue(elem.Value)
		}
		return m
	case bson.A:
		arr := make([]any, len(val))
		for i, item := range val {
			arr[i] = convertBsonValue(item)
		}
		return arr
	case primitive.Binary:
		// Convert binary IDs to hex strings
		return fmt.Sprintf("%x", val.Data)
	case []byte:
		return fmt.Sprintf("%x", val)
	default:
		return val
	}
}

// filenameFromWidgetID generates a filename from a widget ID.
func filenameFromWidgetID(widgetID string) string {
	// Extract the last part after the last dot
	parts := strings.Split(widgetID, ".")
	name := parts[len(parts)-1]
	// Convert camelCase to kebab-case
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('-')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String()) + ".json"
}
