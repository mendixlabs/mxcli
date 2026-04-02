// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
	"github.com/spf13/cobra"
)

var widgetCmd = &cobra.Command{
	Use:   "widget",
	Short: "Widget management commands",
}

var widgetExtractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract widget definition from an .mpk file",
	Long: `Extract a pluggable widget definition from a Mendix .mpk package file
and generate a skeleton .def.json for use with the pluggable widget engine.

The command parses the widget XML inside the .mpk to discover properties,
infers the appropriate operation for each property based on its type,
and writes the result to the project's .mxcli/widgets/ directory.

Examples:
  mxcli widget extract --mpk widgets/MyWidget.mpk
  mxcli widget extract --mpk widgets/MyWidget.mpk --output .mxcli/widgets/
  mxcli widget extract --mpk widgets/MyWidget.mpk --mdl-name MYWIDGET`,
	RunE: runWidgetExtract,
}

var widgetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered widget definitions",
	Long:  `List all widget definitions available in the pluggable widget engine registry.`,
	RunE:  runWidgetList,
}

func init() {
	widgetExtractCmd.Flags().String("mpk", "", "Path to .mpk widget package file")
	widgetExtractCmd.Flags().StringP("output", "o", "", "Output directory (default: .mxcli/widgets/)")
	widgetExtractCmd.Flags().String("mdl-name", "", "Override the MDL keyword name (default: derived from widget name)")
	widgetExtractCmd.MarkFlagRequired("mpk")

	widgetCmd.AddCommand(widgetExtractCmd)
	widgetCmd.AddCommand(widgetListCmd)
	rootCmd.AddCommand(widgetCmd)
}

func runWidgetExtract(cmd *cobra.Command, args []string) error {
	mpkPath, _ := cmd.Flags().GetString("mpk")
	outputDir, _ := cmd.Flags().GetString("output")
	mdlNameOverride, _ := cmd.Flags().GetString("mdl-name")

	// Parse .mpk
	mpkDef, err := mpk.ParseMPK(mpkPath)
	if err != nil {
		return fmt.Errorf("failed to parse .mpk: %w", err)
	}

	// Determine output directory
	if outputDir == "" {
		outputDir = filepath.Join(".mxcli", "widgets")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Determine MDL name
	mdlName := mdlNameOverride
	if mdlName == "" {
		mdlName = deriveMDLName(mpkDef.ID)
	}

	// Generate .def.json
	defJSON := generateDefJSON(mpkDef, mdlName)

	// Determine output filename
	filename := strings.ToLower(mdlName) + ".def.json"
	outPath := filepath.Join(outputDir, filename)

	data, err := json.MarshalIndent(defJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal definition: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Extracted widget definition:\n")
	fmt.Printf("  Widget ID:  %s\n", mpkDef.ID)
	fmt.Printf("  MDL name:   %s\n", mdlName)
	fmt.Printf("  Properties: %d\n", len(mpkDef.Properties))
	fmt.Printf("  Output:     %s\n", outPath)

	return nil
}

// deriveMDLName derives an uppercase MDL keyword from a widget ID.
// e.g. "com.mendix.widget.web.combobox.Combobox" → "COMBOBOX"
// e.g. "com.company.widget.MyCustomWidget" → "MYCUSTOMWIDGET"
func deriveMDLName(widgetID string) string {
	parts := strings.Split(widgetID, ".")
	name := parts[len(parts)-1]
	return strings.ToUpper(name)
}

// generateDefJSON creates a WidgetDefinition from an mpk.WidgetDefinition.
func generateDefJSON(mpkDef *mpk.WidgetDefinition, mdlName string) *executor.WidgetDefinition {
	def := &executor.WidgetDefinition{
		WidgetID:        mpkDef.ID,
		MDLName:         mdlName,
		TemplateFile:    strings.ToLower(mdlName) + ".json",
		DefaultEditable: "Always",
	}

	// Build property mappings by inferring operations from XML types
	var mappings []executor.PropertyMapping
	var childSlots []executor.ChildSlotMapping

	for _, prop := range mpkDef.Properties {
		normalizedType := mpk.NormalizeType(prop.Type)

		switch normalizedType {
		case "attribute":
			mappings = append(mappings, executor.PropertyMapping{
				PropertyKey: prop.Key,
				Source:      "Attribute",
				Operation:   "attribute",
			})
		case "association":
			mappings = append(mappings, executor.PropertyMapping{
				PropertyKey: prop.Key,
				Source:      "Association",
				Operation:   "association",
			})
		case "datasource":
			mappings = append(mappings, executor.PropertyMapping{
				PropertyKey: prop.Key,
				Source:      "DataSource",
				Operation:   "datasource",
			})
		case "widgets":
			// Widgets properties become child slots
			containerName := strings.ToUpper(prop.Key)
			if containerName == "CONTENT" {
				containerName = "TEMPLATE"
			}
			childSlots = append(childSlots, executor.ChildSlotMapping{
				PropertyKey:  prop.Key,
				MDLContainer: containerName,
				Operation:    "widgets",
			})
		case "selection":
			mappings = append(mappings, executor.PropertyMapping{
				PropertyKey: prop.Key,
				Source:      "Selection",
				Operation:   "selection",
				Default:     prop.DefaultValue,
			})
		case "boolean", "string", "enumeration", "integer", "decimal":
			mapping := executor.PropertyMapping{
				PropertyKey: prop.Key,
				Operation:   "primitive",
			}
			if prop.DefaultValue != "" {
				mapping.Value = prop.DefaultValue
			}
			mappings = append(mappings, mapping)
			// Skip action, expression, textTemplate, object, icon, image, file — too complex for auto-mapping
		}
	}

	def.PropertyMappings = mappings
	def.ChildSlots = childSlots

	return def
}

func runWidgetList(cmd *cobra.Command, args []string) error {
	registry, err := executor.NewWidgetRegistry()
	if err != nil {
		return fmt.Errorf("failed to create widget registry: %w", err)
	}

	// Load user definitions if project path available
	projectPath, _ := cmd.Flags().GetString("project")
	if projectPath != "" {
		if err := registry.LoadUserDefinitions(projectPath); err != nil {
			log.Printf("warning: loading user widget definitions: %v", err)
		}
	}

	defs := registry.All()
	if len(defs) == 0 {
		fmt.Println("No widget definitions registered.")
		return nil
	}

	fmt.Printf("%-20s %-50s %s\n", "MDL Name", "Widget ID", "Template")
	fmt.Printf("%-20s %-50s %s\n", strings.Repeat("-", 20), strings.Repeat("-", 50), strings.Repeat("-", 20))
	for _, def := range defs {
		fmt.Printf("%-20s %-50s %s\n", def.MDLName, def.WidgetID, def.TemplateFile)
	}
	fmt.Printf("\nTotal: %d definitions\n", len(defs))

	return nil
}
