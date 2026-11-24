// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/spf13/cobra"
)

// cmd_layout.go arranges a module's domain model from its association graph.
//
// Positions are presentation, and MDL treats them that way: `@Position` exists
// but almost no script writes one, so entities take the default slot. The
// default is deliberately dumb — it cannot be anything else, because the first
// entity of a script is placed before the last one is known. Laying the model
// out properly needs the whole graph, which only exists once the script has run,
// so it is a separate operation rather than a side effect of CREATE.
//
// It is opt-in for a second reason: a domain model somebody arranged by hand in
// Studio Pro is not improved by being rearranged. Nothing here runs unless
// asked, --dry-run shows the moves first, and Marketplace modules are skipped
// outright — mxcli does not rearrange a module the next update replaces.

var (
	layoutModules            []string
	layoutDryRun             bool
	layoutIncludeMarketplace bool
	layoutOnlyOverlapping    bool
)

var layoutCmd = &cobra.Command{
	Use:   "layout",
	Short: "Arrange a module's domain model from its association graph",
	Long: `Arrange the entities of a domain model so related ones sit together.

Entities are layered on the association graph: an entity sits one column past
the furthest thing it references, on the row of the entity it is most linked
to, and a lookup (one that references nothing else in the module) sits one
column left of its nearest referrer, on the row of what references it where
there is room. Association lines then mostly run one way, and straight across,
instead of crossing the diagram. The gap between two columns is wide enough for
the longest association name drawn across it. An entity with a cross-module
association (at either end) is placed where Studio Pro's line for it, which
always runs from its left side straight to the left edge, passes no other box.
Entities with no association at
all — non-persistent helpers, mostly — go in a band underneath rather than being
mixed in with the lookups. Every association line is then re-attached: on the
sides of its two boxes that face each other, straight across where the boxes
share a span and nothing is in the way, the others spread along their side.

Positions are a function of the model alone, so running this twice in a row
changes nothing the second time and re-running after adding an entity moves only
what the new relationships require.

This REPLACES the positions of every entity in the modules it touches, including
any you arranged by hand. Use --dry-run to see the moves first. Marketplace
modules and System are never touched.

--only-overlapping repairs instead of rearranging: an entity whose box does not
collide with another entity or an annotation keeps its position, and of two that
collide the first in reading order (top to bottom, then left to right) stays put
while the other moves to the nearest free spot. Annotations never move, and
association lines keep the attachment points they have. Box
sizes are estimated from the entity's name and attribute count — Studio Pro
does not store them — so the estimate errs on the generous side.

To re-arrange microflows and nanoflows, use 'mxcli layout flows'.`,
	Example: `  mxcli layout -p app.mpr
  mxcli layout -p app.mpr --module CapTrack
  mxcli layout -p app.mpr --dry-run
  mxcli layout -p app.mpr --only-overlapping --dry-run`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runLayout(cmd)
	},
}

func init() {
	layoutCmd.Flags().StringSliceVar(&layoutModules, "module", nil,
		"module to lay out (repeatable; default: every module the project owns)")
	layoutCmd.Flags().BoolVar(&layoutDryRun, "dry-run", false,
		"report the moves without writing")
	layoutCmd.Flags().BoolVar(&layoutIncludeMarketplace, "include-marketplace", false,
		"also lay out Marketplace modules (a module update replaces them, so this is normally pointless)")
	layoutCmd.Flags().BoolVar(&layoutOnlyOverlapping, "only-overlapping", false,
		"move only entities that overlap another entity or an annotation, keeping every other position")
	rootCmd.AddCommand(layoutCmd)
}

func runLayout(cmd *cobra.Command) error {
	projectPath, _ := cmd.Flags().GetString("project")
	if projectPath == "" {
		return fmt.Errorf("no project given: pass -p <project.mpr>")
	}
	if _, err := os.Stat(projectPath); err != nil {
		return fmt.Errorf("project not found: %s", projectPath)
	}

	b := newBackendFactory()()
	if err := b.Connect(projectPath); err != nil {
		return err
	}
	defer func() { _ = b.Disconnect() }()

	modules, err := b.ListModules()
	if err != nil {
		return err
	}

	// Keyed lower-cased because Mendix resolves module names case-insensitively,
	// but the value keeps the spelling the author used so a "not found" quotes
	// their typo back rather than a normalised version of it.
	wanted := map[string]string{}
	for _, m := range layoutModules {
		if t := strings.TrimSpace(m); t != "" {
			wanted[strings.ToLower(t)] = t
		}
	}

	targets, err := layoutTargets(modules, wanted)
	if err != nil {
		return err
	}

	// Cross-module lines are drawn in BOTH modules' diagrams, so placing a
	// module needs the other modules' cross-module associations too.
	allDMs, err := b.ListDomainModels()
	if err != nil {
		return fmt.Errorf("read domain models: %w", err)
	}

	movedTotal, unchangedTotal, reanchoredTotal := 0, 0, 0
	for _, m := range targets {
		dm, err := b.GetDomainModel(m.ID)
		if err != nil {
			return fmt.Errorf("read domain model of %s: %w", m.Name, err)
		}
		if dm == nil || len(dm.Entities) == 0 {
			continue
		}

		var plan map[model.ID]model.Point
		if layoutOnlyOverlapping {
			plan = dmlayout.ResolveOverlaps(dm)
		} else {
			plan = dmlayout.PlanWith(dm, dmlayout.CrossLinesFor(m.Name, dm, allDMs))
		}
		moved := 0
		var moves []string
		for _, e := range dm.Entities {
			p, ok := plan[e.ID]
			if !ok || p == e.Location {
				continue
			}
			moves = append(moves, fmt.Sprintf("  %s.%s  (%d, %d) -> (%d, %d)",
				m.Name, e.Name, e.Location.X, e.Location.Y, p.X, p.Y))
			if !layoutDryRun {
				e.Location = p
			}
			moved++
		}
		sort.Strings(moves)

		// A full layout also places every line: the stored connection points
		// were chosen for the old positions, and Studio Pro's default (both
		// mid-height) slants a line between boxes of different heights.
		// --only-overlapping keeps them, as it keeps every other hand-placed
		// thing it does not have to move.
		reanchored := 0
		if !layoutOnlyOverlapping {
			reanchored = applyAnchors(dm, plan, !layoutDryRun)
		}

		if moved == 0 && reanchored == 0 {
			unchangedTotal += len(dm.Entities)
			if layoutOnlyOverlapping {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: no overlapping entities (%d entities)\n", m.Name, len(dm.Entities))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: already laid out (%d entities)\n", m.Name, len(dm.Entities))
			}
			continue
		}
		for _, line := range moves {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		if reanchored > 0 {
			verb := "re-anchored"
			if layoutDryRun {
				verb = "would be re-anchored"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %d association line(s) %s\n", m.Name, reanchored, verb)
		}
		if layoutDryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d of %d entities would move\n", m.Name, moved, len(dm.Entities))
		} else {
			if err := b.UpdateDomainModel(dm); err != nil {
				return fmt.Errorf("write domain model of %s: %w", m.Name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: moved %d of %d entities\n", m.Name, moved, len(dm.Entities))
		}
		movedTotal += moved
		reanchoredTotal += reanchored
	}

	switch {
	case movedTotal == 0 && unchangedTotal == 0 && reanchoredTotal == 0:
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to lay out.")
	case layoutDryRun:
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run: %d entities would move. Re-run without --dry-run to apply.\n", movedTotal)
	}
	return nil
}

// applyAnchors sets each association's connection points to where
// dmlayout.Anchors places them for the planned positions (an entity the plan
// does not move keeps its own), and returns how many associations change. With
// write false it only counts.
func applyAnchors(dm *domainmodel.DomainModel, plan map[model.ID]model.Point, write bool) int {
	pos := make(map[model.ID]model.Point, len(dm.Entities))
	for _, e := range dm.Entities {
		if e == nil {
			continue
		}
		pos[e.ID] = e.Location
		if p, ok := plan[e.ID]; ok {
			pos[e.ID] = p
		}
	}
	anchors := dmlayout.Anchors(dm, pos)
	changed := 0
	for _, a := range dm.Associations {
		pair, ok := anchors[a.ID]
		if !ok {
			continue
		}
		if a.ParentConnection != nil && a.ChildConnection != nil &&
			*a.ParentConnection == pair.Parent && *a.ChildConnection == pair.Child {
			continue
		}
		changed++
		if write {
			parent, child := pair.Parent, pair.Child
			a.ParentConnection, a.ChildConnection = &parent, &child
		}
	}
	return changed
}

// layoutTargets picks the modules to touch.
//
// A Marketplace module is excluded by default for the same reason CREATE LAYOUT
// refuses to write into one: an update replaces the module wholesale, so any
// arrangement is thrown away. System is excluded because it is not the user's to
// arrange. A --module the project does not have is an error rather than a silent
// no-op — a typo there would otherwise report success having done nothing.
func layoutTargets(modules []*model.Module, wanted map[string]string) ([]*model.Module, error) {
	var out []*model.Module
	matched := map[string]bool{}
	for _, m := range modules {
		if m == nil {
			continue
		}
		if _, ok := wanted[strings.ToLower(m.Name)]; len(wanted) > 0 && !ok {
			continue
		}
		matched[strings.ToLower(m.Name)] = true
		if m.Name == "System" {
			if len(wanted) > 0 {
				return nil, fmt.Errorf("the System module cannot be laid out")
			}
			continue
		}
		fromMarketplace := m.FromAppStore || strings.TrimSpace(m.AppStoreGuid) != ""
		if fromMarketplace && !layoutIncludeMarketplace {
			if len(wanted) > 0 {
				return nil, fmt.Errorf("%s comes from the Marketplace, and a module update replaces it — "+
					"pass --include-marketplace to lay it out anyway", m.Name)
			}
			continue
		}
		out = append(out, m)
	}

	var missing []string
	for key, spelled := range wanted {
		if !matched[key] {
			missing = append(missing, spelled)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("module not found: %s", strings.Join(missing, ", "))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
