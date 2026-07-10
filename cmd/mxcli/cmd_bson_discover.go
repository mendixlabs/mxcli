package main

import (
	"fmt"
	"os"

	bsondiscover "github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/bson"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/bson"
)

// discoverCmd is the "bson discover" subcommand for field coverage analysis.
// The parent bsonCmd (cmd_bson.go) adds this via bsonCmd.AddCommand(discoverCmd).
var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Analyze BSON field coverage against MDL output",
	Long: `Discover which BSON fields are covered by MDL DESCRIBE output.

Scans all objects of the given type from the MPR, collects field names and values,
and checks which fields appear in MDL text output. Reports per-$Type coverage.

Examples:
  # Scan all workflows
  mxcli bson discover -p app.mpr --type workflow

  # Scan a specific workflow
  mxcli bson discover -p app.mpr --type workflow --object "Module.WfName"
`,
	Run: runDiscover,
}

func init() {
	discoverCmd.Flags().StringP("type", "t", "workflow", "Object type: workflow, microflow, page, nanoflow, enumeration")
	discoverCmd.Flags().StringP("object", "o", "", "Specific object qualified name (e.g., Module.WfName)")
}

func runDiscover(cmd *cobra.Command, args []string) {
	projectPath, _ := cmd.Flags().GetString("project")
	objectType, _ := cmd.Flags().GetString("type")
	objectName, _ := cmd.Flags().GetString("object")

	if projectPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
		os.Exit(1)
	}

	reader, err := mpr.Open(projectPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening project: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	var rawUnits []bsondiscover.RawUnit

	if objectName != "" {
		unit, err := reader.GetRawUnitByName(objectType, objectName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting %s: %v\n", objectName, err)
			os.Exit(1)
		}
		parsed, err := unmarshalBsonFields(unit.Contents)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing BSON for %s: %v\n", objectName, err)
			os.Exit(1)
		}
		rawUnits = collectNestedUnits(unit.QualifiedName, parsed)
	} else {
		units, err := reader.ListRawUnits(objectType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing %s units: %v\n", objectType, err)
			os.Exit(1)
		}
		for _, unit := range units {
			parsed, err := unmarshalBsonFields(unit.Contents)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse BSON for %s: %v\n", unit.QualifiedName, err)
				continue
			}
			nested := collectNestedUnits(unit.QualifiedName, parsed)
			rawUnits = append(rawUnits, nested...)
		}
	}

	if len(rawUnits) == 0 {
		fmt.Fprintln(os.Stderr, "No objects found")
		os.Exit(1)
	}

	// Run discover with empty MDL text (pure field inventory mode).
	dr := bsondiscover.RunDiscover(rawUnits, "")
	fmt.Print(bsondiscover.FormatResult(dr))
}

// unmarshalBsonFields parses raw BSON bytes into a map.
func unmarshalBsonFields(contents []byte) (map[string]any, error) {
	var fields map[string]any
	if err := bson.Unmarshal(contents, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// collectNestedUnits extracts the top-level unit and all nested typed objects
// (activities, outcomes, etc.) from a BSON document tree.
func collectNestedUnits(qualifiedName string, fields map[string]any) []bsondiscover.RawUnit {
	bsonType, _ := fields["$Type"].(string)
	if bsonType == "" {
		bsonType = "Unknown"
	}

	var units []bsondiscover.RawUnit
	units = append(units, bsondiscover.RawUnit{
		QualifiedName: qualifiedName,
		BsonType:      bsonType,
		Fields:        fields,
	})

	// Recurse into nested objects and arrays to find typed sub-objects.
	for _, v := range fields {
		switch val := v.(type) {
		case map[string]any:
			if _, hasType := val["$Type"]; hasType {
				nested := collectNestedUnits(qualifiedName, val)
				units = append(units, nested...)
			}
		case []any:
			for _, elem := range val {
				if m, ok := elem.(map[string]any); ok {
					if _, hasType := m["$Type"]; hasType {
						nested := collectNestedUnits(qualifiedName, m)
						units = append(units, nested...)
					}
				}
			}
		}
	}

	return units
}
