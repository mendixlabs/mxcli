package main

import (
	"fmt"
	"os"

	bsondebug "github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/bson"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/bson"
)

var bsonCompareCmd = &cobra.Command{
	Use:   "compare [name1] [name2]",
	Short: "Compare two BSON objects for differences",
	Long: `Compare two BSON objects from Mendix projects and display a structured diff.

Supports same-project and cross-project comparison. By default, structural
and layout fields ($ID, PersistentId, RelativeMiddlePoint, Size) are skipped.

Examples:
  # Compare two workflows in the same project
  mxcli bson compare -p app.mpr --type workflow WF1 WF2

  # Compare same workflow across two MPR files
  mxcli bson compare -p app.mpr -p2 other.mpr --type workflow MyWorkflow

  # Include structural fields in comparison
  mxcli bson compare -p app.mpr --type workflow --all WF1 WF2
`,
	Args: cobra.RangeArgs(1, 2),
	Run:  runBsonCompare,
}

func init() {
	bsonCompareCmd.Flags().StringP("project", "p", "", "Path to first MPR project (required)")
	bsonCompareCmd.Flags().String("p2", "", "Path to second MPR project (for cross-MPR comparison)")
	bsonCompareCmd.Flags().String("type", "workflow", "Object type: workflow, page, microflow, nanoflow, enumeration, snippet, layout")
	bsonCompareCmd.Flags().Bool("all", false, "Include structural/layout fields ($ID, PersistentId, etc.)")
	bsonCompareCmd.Flags().String("format", "diff", "Output format: diff, ndsl")
}

func runBsonCompare(cmd *cobra.Command, args []string) {
	projectPath, _ := cmd.Flags().GetString("project")
	secondProject, _ := cmd.Flags().GetString("p2")
	objectType, _ := cmd.Flags().GetString("type")
	includeAll, _ := cmd.Flags().GetBool("all")

	if projectPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
		os.Exit(1)
	}

	reader1, err := mpr.Open(projectPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening project: %v\n", err)
		os.Exit(1)
	}
	defer reader1.Close()

	var leftName, rightName string
	var reader2 *mpr.Reader

	switch len(args) {
	case 2:
		// Two names in the same (or different) project
		leftName = args[0]
		rightName = args[1]
		if secondProject != "" {
			reader2, err = mpr.Open(secondProject)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error opening second project: %v\n", err)
				os.Exit(1)
			}
			defer reader2.Close()
		}
	case 1:
		// One name, must have -p2 for cross-MPR comparison
		if secondProject == "" {
			fmt.Fprintln(os.Stderr, "Error: provide two names, or one name with -p2 for cross-MPR comparison")
			os.Exit(1)
		}
		leftName = args[0]
		rightName = args[0]
		reader2, err = mpr.Open(secondProject)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening second project: %v\n", err)
			os.Exit(1)
		}
		defer reader2.Close()
	}

	// If no second reader, use the first for both
	rightReader := reader1
	if reader2 != nil {
		rightReader = reader2
	}

	// Fetch raw BSON
	leftUnit, err := reader1.GetRawUnitByName(objectType, leftName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting %s: %v\n", leftName, err)
		os.Exit(1)
	}

	rightUnit, err := rightReader.GetRawUnitByName(objectType, rightName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting %s: %v\n", rightName, err)
		os.Exit(1)
	}

	// NDSL format: render both sides as normalized DSL for LLM-friendly comparison
	format, _ := cmd.Flags().GetString("format")
	if format == "ndsl" {
		var leftDocD, rightDocD bson.D
		if err := bson.Unmarshal(leftUnit.Contents, &leftDocD); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing BSON for %s: %v\n", leftName, err)
			os.Exit(1)
		}
		if err := bson.Unmarshal(rightUnit.Contents, &rightDocD); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing BSON for %s: %v\n", rightName, err)
			os.Exit(1)
		}
		fmt.Printf("=== LEFT: %s ===\n%s\n\n=== RIGHT: %s ===\n%s\n",
			leftName, bsondebug.Render(leftDocD, 0),
			rightName, bsondebug.Render(rightDocD, 0))
		return
	}

	// Unmarshal to map[string]any
	var leftDoc, rightDoc bson.M
	if err := bson.Unmarshal(leftUnit.Contents, &leftDoc); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing BSON for %s: %v\n", leftName, err)
		os.Exit(1)
	}
	if err := bson.Unmarshal(rightUnit.Contents, &rightDoc); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing BSON for %s: %v\n", rightName, err)
		os.Exit(1)
	}

	// Compare
	opts := bsondebug.CompareOptions{IncludeAll: includeAll}
	diffs := bsondebug.Compare(leftDoc, rightDoc, opts)

	// Print header
	typeName := leftDoc["$Type"]
	if typeName != nil {
		fmt.Println(typeName)
	}
	if leftName == rightName {
		fmt.Printf("  Comparing: %s (across two MPRs)\n\n", leftName)
	} else {
		fmt.Printf("  Comparing: %s vs %s\n\n", leftName, rightName)
	}

	// Print formatted output
	fmt.Println(bsondebug.FormatDiffs(diffs))
}
