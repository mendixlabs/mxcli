// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/spf13/cobra"
)

var brainMilestoneCmd = &cobra.Command{
	Use:   "milestone [<name>] [--slice <slice>]… [--requirement <id>]…",
	Short: "Group slices into the specification's milestones",
	Long: `Put slices — or single requirements — in a milestone of the specification.

A milestone is part of the slice it groups: the slice file's frontmatter gets
'milestone: "<name>"', and a requirement placed apart from its slice gets a
"Milestone: ` + "`<name>`" + `" line. 'brain plan' shows them, and
'report traceability' and 'report map' group by them.

Milestones are ordered by where they first appear, walking the slices in name
order (01-…, 02-…).

With no name, lists the milestones. --clear takes the slices or requirements
out of their milestone. --import reads the traceability.json the traceability
report used to take, writes it into the plan, and removes the file.`,
	Example: `  mxcli brain milestone "M1 — accounts" --slice 01-accounts --slice 02-profiles
  mxcli brain milestone "M2 — approvals" --requirement a1b2c3
  mxcli brain milestone --clear --slice 02-profiles
  mxcli brain milestone --import docs/brain/plan/traceability.json
  mxcli brain milestone`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		store := brain.NewStore(brainProjectDir(cmd))
		if !store.Exists() {
			brainFatal(fmt.Errorf("no brain here; 'mxcli brain init' creates one"))
		}
		slices, _ := cmd.Flags().GetStringArray("slice")
		reqs, _ := cmd.Flags().GetStringArray("requirement")
		clear, _ := cmd.Flags().GetBool("clear")
		imp, _ := cmd.Flags().GetString("import")

		switch {
		case imp != "":
			if len(args) > 0 || len(slices) > 0 || len(reqs) > 0 || clear {
				brainFatal(fmt.Errorf("--import takes no name, --slice, --requirement or --clear"))
			}
			runMilestoneImport(store, imp)
			return
		case len(args) == 0 && !clear:
			if len(slices) > 0 || len(reqs) > 0 {
				brainFatal(fmt.Errorf("name the milestone, or pass --clear"))
			}
			ms, err := store.Milestones()
			if err != nil {
				brainFatal(err)
			}
			if globalJSONFlag {
				brainJSON(map[string]any{"milestones": ms})
				return
			}
			if len(ms) == 0 {
				fmt.Println("No milestones. Group slices with:")
				fmt.Println("  mxcli brain milestone \"M1 — <name>\" --slice 01-<slice>")
				return
			}
			for _, m := range ms {
				fmt.Println(m.Name)
				for _, sl := range m.Slices {
					fmt.Printf("  slice %s\n", sl)
				}
				for _, id := range m.Requirements {
					fmt.Printf("  requirement %s\n", id)
				}
			}
			return
		}
		if len(slices) == 0 && len(reqs) == 0 {
			brainFatal(fmt.Errorf("say what goes in it: --slice <slice> or --requirement <id>"))
		}
		name := ""
		if !clear {
			name = args[0]
		} else if len(args) > 0 {
			brainFatal(fmt.Errorf("--clear takes no milestone name"))
		}
		for _, sl := range slices {
			if err := store.SetSliceMilestone(sl, name); err != nil {
				brainFatal(err)
			}
			reportMilestoneChange("slice "+sl, name)
		}
		for _, id := range reqs {
			if err := store.SetRequirementMilestone(id, name); err != nil {
				brainFatal(err)
			}
			reportMilestoneChange("requirement "+id, name)
		}
	},
}

func reportMilestoneChange(what, name string) {
	if globalJSONFlag {
		return
	}
	if name == "" {
		fmt.Printf("Took %s out of its milestone\n", what)
		return
	}
	fmt.Printf("Put %s in milestone %q\n", what, name)
}

func runMilestoneImport(store *brain.Store, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		brainFatal(err)
	}
	var file struct {
		Milestones []brain.Milestone `json:"milestones"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		brainFatal(fmt.Errorf("%s: %w", path, err))
	}
	skipped, err := store.ImportMilestones(file.Milestones)
	if err != nil {
		brainFatal(err)
	}
	removed := false
	if len(skipped) == 0 {
		removed = os.Remove(path) == nil
	}
	if globalJSONFlag {
		brainJSON(map[string]any{"imported": len(file.Milestones), "skipped": skipped, "removed": removed})
		return
	}
	fmt.Printf("Imported %d milestone(s) from %s into the plan\n", len(file.Milestones), filepath.Base(path))
	for _, s := range skipped {
		fmt.Printf("  not placed: %s\n", s)
	}
	if removed {
		fmt.Printf("Removed %s: the slices carry their milestones now\n", path)
	} else if len(skipped) > 0 {
		fmt.Printf("Kept %s, since not everything in it was placed; delete it once the rest is settled\n", path)
	}
}

func init() {
	brainMilestoneCmd.Flags().StringArray("slice", nil, "A slice to put in the milestone; repeatable")
	brainMilestoneCmd.Flags().StringArray("requirement", nil, "A requirement placed in the milestone apart from its slice; repeatable")
	brainMilestoneCmd.Flags().Bool("clear", false, "Take the slices or requirements out of their milestone")
	brainMilestoneCmd.Flags().String("import", "", "Write a traceability.json's milestones into the plan, then remove the file")
	brainCmd.AddCommand(brainMilestoneCmd)
}
