// SPDX-License-Identifier: Apache-2.0

// Example: Reading a Mendix project
//
// This example demonstrates how to read and explore a Mendix project
// using the modelsdk-go library.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli"
)

// redactSensitiveFields recursively walks a JSON map and replaces values
// for keys containing "Password" or "Secret" with "***".
func redactSensitiveFields(m map[string]any) {
	for key, val := range m {
		lk := strings.ToLower(key)
		if strings.Contains(lk, "password") || strings.Contains(lk, "secret") {
			if _, ok := val.(string); ok {
				m[key] = "***"
			}
			continue
		}
		switch v := val.(type) {
		case map[string]any:
			redactSensitiveFields(v)
		case []any:
			for _, item := range v {
				if nested, ok := item.(map[string]any); ok {
					redactSensitiveFields(nested)
				}
			}
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: read_project <path-to-mpr-file>")
		os.Exit(1)
	}

	mprPath := os.Args[1]

	// Open the MPR file
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		fmt.Printf("Error opening MPR file: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	fmt.Printf("Opened: %s\n", reader.Path())
	fmt.Printf("MPR Version: %d\n", reader.Version())

	// Get Mendix version
	version, err := reader.GetMendixVersion()
	if err != nil {
		fmt.Printf("Warning: Could not get Mendix version: %v\n", err)
	} else {
		fmt.Printf("Mendix Version: %s\n", version)
	}

	fmt.Println("\n=== Modules ===")
	modules, err := reader.ListModules()
	if err != nil {
		fmt.Printf("Error listing modules: %v\n", err)
		os.Exit(1)
	}

	for _, m := range modules {
		fmt.Printf("  - %s (ID: %s)\n", m.Name, m.ID)
		if m.FromAppStore {
			fmt.Println("    (From App Store)")
		}
	}

	fmt.Println("\n=== Domain Models ===")
	domainModels, err := reader.ListDomainModels()
	if err != nil {
		fmt.Printf("Error listing domain models: %v\n", err)
	} else {
		for _, dm := range domainModels {
			fmt.Printf("  Domain Model: %s\n", dm.ID)
			fmt.Printf("    Entities: %d\n", len(dm.Entities))
			for _, e := range dm.Entities {
				fmt.Printf("      - %s", e.Name)
				if !e.Persistable {
					fmt.Print(" (non-persistable)")
				}
				fmt.Printf(" [%d attributes]\n", len(e.Attributes))
				for _, a := range e.Attributes {
					typeName := "unknown"
					if a.Type != nil {
						typeName = a.Type.GetTypeName()
					}
					fmt.Printf("        - %s: %s\n", a.Name, typeName)
				}
			}
			fmt.Printf("    Associations: %d\n", len(dm.Associations))
			for _, a := range dm.Associations {
				fmt.Printf("      - %s (%s)\n", a.Name, a.Type)
			}
		}
	}

	fmt.Println("\n=== Microflows ===")
	microflows, err := reader.ListMicroflows()
	if err != nil {
		fmt.Printf("Error listing microflows: %v\n", err)
	} else {
		fmt.Printf("  Total: %d microflows\n", len(microflows))
		for _, mf := range microflows {
			fmt.Printf("  - %s\n", mf.Name)
			if len(mf.Parameters) > 0 {
				fmt.Printf("    Parameters: %d\n", len(mf.Parameters))
			}
		}
	}

	fmt.Println("\n=== Nanoflows ===")
	nanoflows, err := reader.ListNanoflows()
	if err != nil {
		fmt.Printf("Error listing nanoflows: %v\n", err)
	} else {
		fmt.Printf("  Total: %d nanoflows\n", len(nanoflows))
		for _, nf := range nanoflows {
			fmt.Printf("  - %s\n", nf.Name)
		}
	}

	fmt.Println("\n=== Pages ===")
	pages, err := reader.ListPages()
	if err != nil {
		fmt.Printf("Error listing pages: %v\n", err)
	} else {
		fmt.Printf("  Total: %d pages\n", len(pages))
		for _, p := range pages {
			fmt.Printf("  - %s", p.Name)
			if p.URL != "" {
				fmt.Printf(" (URL: %s)", p.URL)
			}
			fmt.Println()
		}
	}

	fmt.Println("\n=== Layouts ===")
	layouts, err := reader.ListLayouts()
	if err != nil {
		fmt.Printf("Error listing layouts: %v\n", err)
	} else {
		fmt.Printf("  Total: %d layouts\n", len(layouts))
		for _, l := range layouts {
			fmt.Printf("  - %s (%s)\n", l.Name, l.LayoutType)
		}
	}

	fmt.Println("\n=== Enumerations ===")
	enumerations, err := reader.ListEnumerations()
	if err != nil {
		fmt.Printf("Error listing enumerations: %v\n", err)
	} else {
		fmt.Printf("  Total: %d enumerations\n", len(enumerations))
		for _, e := range enumerations {
			fmt.Printf("  - %s [%d values]\n", e.Name, len(e.Values))
			for _, v := range e.Values {
				fmt.Printf("      - %s\n", v.Name)
			}
		}
	}

	fmt.Println("\n=== Constants ===")
	constants, err := reader.ListConstants()
	if err != nil {
		fmt.Printf("Error listing constants: %v\n", err)
	} else {
		fmt.Printf("  Total: %d constants\n", len(constants))
		for _, c := range constants {
			fmt.Printf("  - %s: %s = %s\n", c.Name, c.Type, c.DefaultValue)
		}
	}

	fmt.Println("\n=== Scheduled Events ===")
	events, err := reader.ListScheduledEvents()
	if err != nil {
		fmt.Printf("Error listing scheduled events: %v\n", err)
	} else {
		fmt.Printf("  Total: %d scheduled events\n", len(events))
		for _, e := range events {
			status := "disabled"
			if e.Enabled {
				status = "enabled"
			}
			fmt.Printf("  - %s (%s)\n", e.Name, status)
		}
	}

	fmt.Println("\n=== Building Blocks ===")
	buildingBlocks, err := reader.ListBuildingBlocks()
	if err != nil {
		fmt.Printf("Error listing building blocks: %v\n", err)
	} else {
		fmt.Printf("  Total: %d building blocks\n", len(buildingBlocks))
		for _, bb := range buildingBlocks {
			fmt.Printf("  - %s\n", bb.Name)
		}
	}

	fmt.Println("\n=== Page Templates ===")
	pageTemplates, err := reader.ListPageTemplates()
	if err != nil {
		fmt.Printf("Error listing page templates: %v\n", err)
	} else {
		fmt.Printf("  Total: %d page templates\n", len(pageTemplates))
		for _, pt := range pageTemplates {
			fmt.Printf("  - %s\n", pt.Name)
		}
	}

	fmt.Println("\n=== JavaScript Actions ===")
	jsActions, err := reader.ListJavaScriptActions()
	if err != nil {
		fmt.Printf("Error listing JavaScript actions: %v\n", err)
	} else {
		fmt.Printf("  Total: %d JavaScript actions\n", len(jsActions))
		for _, jsa := range jsActions {
			fmt.Printf("  - %s\n", jsa.Name)
		}
	}

	fmt.Println("\n=== Snippets ===")
	snippets, err := reader.ListSnippets()
	if err != nil {
		fmt.Printf("Error listing snippets: %v\n", err)
	} else {
		fmt.Printf("  Total: %d snippets\n", len(snippets))
		for _, s := range snippets {
			fmt.Printf("  - %s\n", s.Name)
		}
	}

	fmt.Println("\n=== Image Collections ===")
	imageCollections, err := reader.ListImageCollections()
	if err != nil {
		fmt.Printf("Error listing image collections: %v\n", err)
	} else {
		fmt.Printf("  Total: %d image collections\n", len(imageCollections))
		for _, ic := range imageCollections {
			fmt.Printf("  - %s (%d images)\n", ic.Name, len(ic.Images))
		}
	}

	// Export to JSON option
	if len(os.Args) > 2 && os.Args[2] == "--json" {
		fmt.Println("\n=== Exporting to JSON ===")
		jsonData, err := reader.ExportJSON()
		if err != nil {
			fmt.Printf("Error exporting JSON: %v\n", err)
		} else {
			var prettyJSON map[string]any
			json.Unmarshal(jsonData, &prettyJSON)
			redactSensitiveFields(prettyJSON)
			output, _ := json.MarshalIndent(prettyJSON, "", "  ")
			fmt.Println(string(output))
		}
	}
}
