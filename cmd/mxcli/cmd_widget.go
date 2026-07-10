// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/sdk/widgets/mpk"
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

var widgetInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Extract definitions for all project widgets",
	Long: `Scan the project's widgets/ directory, extract .def.json for each .mpk,
and generate skill documentation in .claude/skills/widgets/.

This enables CREATE PAGE to use any project widget via the pluggable engine.

Each run compares the generated content against any existing .def.json.
Unchanged files are reported as "up to date"; content drift (e.g. when
mxcli was upgraded and now emits additional fields like objectLists) is
auto-refreshed without needing --force.

Requires --project (-p) to locate the project's widgets/ directory.`,
	Example: `  mxcli widget init -p /path/to/app.mpr
  mxcli widget init -p app.mpr
  mxcli widget init -p app.mpr --force   # rewrite every .def.json even when content matches`,
	RunE: runWidgetInit,
}

var widgetDocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate widget skill documentation",
	Long:  `Generate per-widget markdown documentation in .claude/skills/widgets/ from .mpk definitions.`,
	RunE:  runWidgetDocs,
}

func init() {
	widgetExtractCmd.Flags().String("mpk", "", "Path to .mpk widget package file")
	widgetExtractCmd.Flags().StringP("output", "o", "", "Output directory (default: .mxcli/widgets/)")
	widgetExtractCmd.Flags().String("mdl-name", "", "Override the MDL keyword name (default: derived from widget name)")
	widgetExtractCmd.MarkFlagRequired("mpk")

	widgetInitCmd.Flags().StringP("project", "p", "", "Path to .mpr project file")
	widgetInitCmd.Flags().Bool("force", false, "Rewrite every .def.json unconditionally (default auto-refreshes only when content drifts)")
	widgetInitCmd.MarkFlagRequired("project")

	widgetDocsCmd.Flags().StringP("project", "p", "", "Path to .mpr project file")
	widgetDocsCmd.MarkFlagRequired("project")

	widgetCmd.AddCommand(widgetExtractCmd)
	widgetCmd.AddCommand(widgetListCmd)
	widgetCmd.AddCommand(widgetInitCmd)
	widgetCmd.AddCommand(widgetDocsCmd)
	rootCmd.AddCommand(widgetCmd)
}

func runWidgetExtract(cmd *cobra.Command, args []string) error {
	mpkPath, _ := cmd.Flags().GetString("mpk")
	outputDir, _ := cmd.Flags().GetString("output")
	mdlNameOverride, _ := cmd.Flags().GetString("mdl-name")

	// Parse .mpk — a single package can bundle many widgets (e.g. Charts.mpk).
	mpkDefs, err := mpk.ParseMPKAll(mpkPath)
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

	// The --mdl-name override only makes sense for a single-widget package.
	if mdlNameOverride != "" && len(mpkDefs) > 1 {
		return fmt.Errorf("--mdl-name cannot be used with a bundled .mpk that contains %d widgets", len(mpkDefs))
	}

	for _, mpkDef := range mpkDefs {
		mdlName := mdlNameOverride
		if mdlName == "" {
			mdlName = executor.DeriveMDLName(mpkDef.ID)
		}

		defJSON := executor.GenerateDefJSON(mpkDef, mdlName)
		filename := strings.ToLower(mdlName) + ".def.json"
		outPath := filepath.Join(outputDir, filename)

		data, err := json.MarshalIndent(defJSON, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal definition for %s: %w", mpkDef.ID, err)
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
	}

	return nil
}

// deriveMDLName derives an uppercase MDL keyword from a widget ID.
// e.g. "com.mendix.widget.web.combobox.Combobox" → "COMBOBOX"
// e.g. "com.company.widget.MyCustomWidget" → "MYCUSTOMWIDGET"
// Helpers deriveMDLName / generateDefJSON / makeObjectListMapping /
// deriveObjectListKeyword / operationForType live in
// mdl/executor/widget_defs.go so both the CLI command and the in-executor
// refresh-catalog path share them.

func runWidgetInit(cmd *cobra.Command, args []string) error {
	projectPath, _ := cmd.Flags().GetString("project")
	force, _ := cmd.Flags().GetBool("force")
	return ExtractWidgetDefinitions(projectPath, force, true)
}

// ExtractWidgetDefinitions scans `projectDir/widgets/` for .mpk files and
// generates/refreshes `projectDir/.mxcli/widgets/<name>.def.json` for each.
// Auto-refreshes definitions whose generated content has drifted. When
// `force` is true, every existing def.json is rewritten regardless. When
// `verbose` is false, suppresses per-widget output but keeps the summary.
//
// Delegates core extraction to executor.RefreshWidgetDefinitions; this CLI
// wrapper also generates the widget skill .md docs.
func ExtractWidgetDefinitions(projectPath string, force bool, verbose bool) error {
	var output io.Writer
	if verbose {
		output = os.Stdout
	}
	stats, err := executor.RefreshWidgetDefinitions(projectPath, force, output)
	if err != nil {
		return err
	}

	if stats.Extracted+stats.Refreshed+stats.UpToDate+stats.Skipped == 0 {
		if verbose {
			fmt.Println("No .mpk files found in widgets/ directory.")
		}
		return nil
	}

	if verbose {
		fmt.Printf("\nExtracted: %d new, %d refreshed, %d up to date, %d skipped (built-in or unparseable)\n",
			stats.Extracted, stats.Refreshed, stats.UpToDate, stats.Skipped)
	} else if stats.Extracted > 0 || stats.Refreshed > 0 {
		fmt.Printf("  Widget definitions: %d new, %d refreshed\n", stats.Extracted, stats.Refreshed)
	}

	if verbose {
		fmt.Println("\nGenerating widget documentation...")
	}
	return generateWidgetDocs(projectPath)
}

func runWidgetDocs(cmd *cobra.Command, args []string) error {
	projectPath, _ := cmd.Flags().GetString("project")
	return generateWidgetDocs(projectPath)
}

// generateWidgetDocs is a thin CLI wrapper around executor.RegenerateWidgetDocs.
// The full implementation (including .def.json-driven object-list / child-slot
// documentation) lives in mdl/executor/widget_defs.go so refresh catalog can
// regenerate docs alongside def refresh.
func generateWidgetDocs(projectPath string) error {
	generated, err := executor.RegenerateWidgetDocs(projectPath)
	if err != nil {
		return err
	}
	if generated > 0 {
		projectDir := filepath.Dir(projectPath)
		docsDir := filepath.Join(projectDir, ".claude", "skills", "widgets")
		if _, statErr := os.Stat(filepath.Join(projectDir, ".ai-context")); statErr == nil {
			docsDir = filepath.Join(projectDir, ".ai-context", "skills", "widgets")
		}
		fmt.Printf("Generated %d widget docs in %s\n", generated, docsDir)
	}
	return nil
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

	fmt.Printf("%-16s %-20s %-50s %s\n", "Kind", "MDL Name", "Widget ID", "Template")
	fmt.Printf("%-16s %-20s %-50s %s\n", strings.Repeat("-", 16), strings.Repeat("-", 20), strings.Repeat("-", 50), strings.Repeat("-", 20))
	for _, def := range defs {
		kind := def.WidgetKind
		if kind == "" {
			kind = "pluggable"
		}
		fmt.Printf("%-16s %-20s %-50s %s\n", kind, def.MDLName, def.WidgetID, def.TemplateFile)
	}
	fmt.Printf("\nTotal: %d definitions\n", len(defs))

	return nil
}
