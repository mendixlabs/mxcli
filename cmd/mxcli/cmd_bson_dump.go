// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	bsondebug "github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/bson"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/bson"
)

var bsonDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Dump raw BSON data from Mendix project objects",
	Long: `Dump raw BSON data from Mendix project objects as JSON or NDSL.

Use this to inspect the internal BSON structure of pages, microflows, workflows,
and other model elements. You can compare two objects side-by-side to identify
field differences, structural mismatches, or array marker issues.

Object Types:
  page         Dump a page
  microflow    Dump a microflow
  nanoflow     Dump a nanoflow
  enumeration  Dump an enumeration
  snippet      Dump a snippet
  layout       Dump a layout
  constant     Dump a constant

Examples:
  # List all pages
  mxcli bson dump -p app.mpr --type page --list

  # Dump a specific page
  mxcli bson dump -p app.mpr --type page --object "PgTest.MyPage"

  # Compare two objects (outputs both as JSON for diff)
  mxcli bson dump -p app.mpr --type page --compare "PgTest.Broken" "PgTest.Fixed"

  # Save dump to file
  mxcli bson dump -p app.mpr --type page --object "PgTest.MyPage" > mypage.json

  # Extract raw BSON baseline for roundtrip testing
  mxcli bson dump -p app.mpr --type page --object "PgTest.MyPage" --format bson > mypage.mxunit
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		objectType, _ := cmd.Flags().GetString("type")
		objectName, _ := cmd.Flags().GetString("object")
		listObjects, _ := cmd.Flags().GetBool("list")
		compareFlag, _ := cmd.Flags().GetStringSlice("compare")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Open the project
		reader, err := mpr.Open(projectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening project: %v\n", err)
			os.Exit(1)
		}
		defer reader.Close()

		// List objects
		if listObjects {
			units, err := reader.ListRawUnits(objectType)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Objects of type '%s':\n", objectType)
			for _, u := range units {
				fmt.Printf("  %s (%s)\n", u.QualifiedName, u.Type)
			}
			return
		}

		format, _ := cmd.Flags().GetString("format")

		// Compare two objects
		if len(compareFlag) == 2 {
			obj1, err := reader.GetRawUnitByName(objectType, compareFlag[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting %s: %v\n", compareFlag[0], err)
				os.Exit(1)
			}

			obj2, err := reader.GetRawUnitByName(objectType, compareFlag[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting %s: %v\n", compareFlag[1], err)
				os.Exit(1)
			}

			// Parse BSON to bson.D to preserve key order
			var raw1, raw2 bson.D
			if err := bson.Unmarshal(obj1.Contents, &raw1); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing BSON for %s: %v\n", compareFlag[0], err)
				os.Exit(1)
			}
			if err := bson.Unmarshal(obj2.Contents, &raw2); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing BSON for %s: %v\n", compareFlag[1], err)
				os.Exit(1)
			}

			if format == "ndsl" {
				fmt.Printf("=== LEFT: %s ===\n%s\n\n=== RIGHT: %s ===\n%s\n",
					compareFlag[0], bsondebug.Render(raw1, 0),
					compareFlag[1], bsondebug.Render(raw2, 0))
				return
			}

			// Print diff report
			fmt.Printf("=== BSON DIFF: %s vs %s ===\n\n", compareFlag[0], compareFlag[1])
			diffs := compareBsonDocs(raw1, raw2, "")
			if len(diffs) == 0 {
				fmt.Println("No differences found (documents are identical)")
			} else {
				fmt.Printf("Found %d difference(s):\n\n", len(diffs))
				for _, d := range diffs {
					fmt.Println(d)
				}
			}
			return
		}

		// Dump single object
		if objectName != "" {
			obj, err := reader.GetRawUnitByName(objectType, objectName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if format == "bson" {
				// Write raw BSON bytes to stdout (for baseline extraction)
				os.Stdout.Write(obj.Contents)
				return
			}

			if format == "ndsl" {
				var doc bson.D
				if err := bson.Unmarshal(obj.Contents, &doc); err != nil {
					fmt.Fprintf(os.Stderr, "Error parsing BSON: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(bsondebug.Render(doc, 0))
				return
			}

			// Parse BSON and output as JSON
			var raw any
			if err := bson.Unmarshal(obj.Contents, &raw); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing BSON: %v\n", err)
				os.Exit(1)
			}

			jsonBytes, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error converting to JSON: %v\n", err)
				os.Exit(1)
			}

			fmt.Println(string(jsonBytes))
			return
		}

		// No action specified
		fmt.Fprintln(os.Stderr, "Error: specify --list, --object, or --compare")
		os.Exit(1)
	},
}

// compareBsonDocs compares two BSON documents and returns a list of differences.
// It compares top-level keys and their values, recursing into nested documents.
func compareBsonDocs(doc1, doc2 bson.D, path string) []string {
	var diffs []string

	// Build maps for easier lookup
	map1 := make(map[string]any)
	map2 := make(map[string]any)
	for _, e := range doc1 {
		map1[e.Key] = e.Value
	}
	for _, e := range doc2 {
		map2[e.Key] = e.Value
	}

	// Check for keys in doc1 that are missing or different in doc2
	for _, e := range doc1 {
		key := e.Key
		fullPath := key
		if path != "" {
			fullPath = path + "." + key
		}

		val2, exists := map2[key]
		if !exists {
			diffs = append(diffs, fmt.Sprintf("- %s: only in first document\n  Value: %s", fullPath, formatValue(e.Value)))
			continue
		}

		// Compare values
		valueDiffs := compareValues(e.Value, val2, fullPath)
		diffs = append(diffs, valueDiffs...)
	}

	// Check for keys in doc2 that are missing in doc1
	for _, e := range doc2 {
		key := e.Key
		fullPath := key
		if path != "" {
			fullPath = path + "." + key
		}

		if _, exists := map1[key]; !exists {
			diffs = append(diffs, fmt.Sprintf("+ %s: only in second document\n  Value: %s", fullPath, formatValue(e.Value)))
		}
	}

	return diffs
}

// compareValues compares two values and returns differences.
func compareValues(val1, val2 any, path string) []string {
	var diffs []string

	// Skip $ID comparisons (they're always different)
	if strings.HasSuffix(path, ".$ID") {
		return nil
	}

	// Handle nil values
	if val1 == nil && val2 == nil {
		return nil
	}
	if val1 == nil || val2 == nil {
		diffs = append(diffs, fmt.Sprintf("~ %s: value differs\n  First:  %s\n  Second: %s", path, formatValue(val1), formatValue(val2)))
		return diffs
	}

	// Compare based on type
	switch v1 := val1.(type) {
	case bson.D:
		if v2, ok := val2.(bson.D); ok {
			return compareBsonDocs(v1, v2, path)
		}
		diffs = append(diffs, fmt.Sprintf("~ %s: type differs (document vs %T)", path, val2))

	case bson.A:
		if v2, ok := val2.(bson.A); ok {
			return compareBsonArrays(v1, v2, path)
		}
		diffs = append(diffs, fmt.Sprintf("~ %s: type differs (array vs %T)", path, val2))

	case string:
		if v2, ok := val2.(string); ok {
			if v1 != v2 {
				diffs = append(diffs, fmt.Sprintf("~ %s: string differs\n  First:  %q\n  Second: %q", path, v1, v2))
			}
		} else {
			diffs = append(diffs, fmt.Sprintf("~ %s: type differs (string vs %T)", path, val2))
		}

	case int32, int64, float64, bool:
		if fmt.Sprintf("%v", val1) != fmt.Sprintf("%v", val2) {
			diffs = append(diffs, fmt.Sprintf("~ %s: value differs\n  First:  %v\n  Second: %v", path, val1, val2))
		}

	default:
		// For other types, compare string representations
		s1 := fmt.Sprintf("%v", val1)
		s2 := fmt.Sprintf("%v", val2)
		if s1 != s2 {
			diffs = append(diffs, fmt.Sprintf("~ %s: value differs\n  First:  %s\n  Second: %s", path, s1, s2))
		}
	}

	return diffs
}

// compareBsonArrays compares two BSON arrays.
func compareBsonArrays(arr1, arr2 bson.A, path string) []string {
	var diffs []string

	// Check length difference
	if len(arr1) != len(arr2) {
		diffs = append(diffs, fmt.Sprintf("~ %s: array length differs (first: %d, second: %d)", path, len(arr1), len(arr2)))
	}

	// Compare elements up to the shorter length
	minLen := min(len(arr2), len(arr1))

	for i := range minLen {
		elemPath := fmt.Sprintf("%s[%d]", path, i)
		elemDiffs := compareValues(arr1[i], arr2[i], elemPath)
		diffs = append(diffs, elemDiffs...)
	}

	// Report extra elements
	if len(arr1) > minLen {
		for i := minLen; i < len(arr1); i++ {
			diffs = append(diffs, fmt.Sprintf("- %s[%d]: only in first document\n  Value: %s", path, i, formatValue(arr1[i])))
		}
	}
	if len(arr2) > minLen {
		for i := minLen; i < len(arr2); i++ {
			diffs = append(diffs, fmt.Sprintf("+ %s[%d]: only in second document\n  Value: %s", path, i, formatValue(arr2[i])))
		}
	}

	return diffs
}

// formatValue formats a value for display, truncating long output.
func formatValue(val any) string {
	switch v := val.(type) {
	case nil:
		return "null"
	case string:
		if len(v) > 100 {
			return fmt.Sprintf("%q... (truncated)", v[:100])
		}
		return fmt.Sprintf("%q", v)
	case bson.D:
		keys := make([]string, len(v))
		for i, e := range v {
			keys[i] = e.Key
		}
		return fmt.Sprintf("{document with keys: %v}", keys)
	case bson.A:
		return fmt.Sprintf("[array with %d elements]", len(v))
	default:
		s := fmt.Sprintf("%v", val)
		if len(s) > 100 {
			return s[:100] + "... (truncated)"
		}
		return s
	}
}

func init() {
	bsonDumpCmd.Flags().StringP("type", "t", "page", "Object type: page, microflow, nanoflow, enumeration, snippet, layout, constant")
	bsonDumpCmd.Flags().StringP("object", "o", "", "Object qualified name to dump (e.g., Module.PageName)")
	bsonDumpCmd.Flags().BoolP("list", "l", false, "List all objects of the specified type")
	bsonDumpCmd.Flags().StringSliceP("compare", "c", nil, "Compare two objects: --compare Obj1,Obj2")
	bsonDumpCmd.Flags().String("format", "json", "Output format: json, ndsl, bson (raw bytes)")
}
