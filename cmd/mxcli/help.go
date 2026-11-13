// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/syntax"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/migration"
	"github.com/spf13/cobra"
)

var syntaxCmd = &cobra.Command{
	Use:   "syntax [topic [subtopic...]]",
	Short: "Show MDL syntax reference",
	Long: `Show MDL syntax reference from the feature registry.

Use --json for machine-readable output (optimized for LLM consumption).
Drill down with multiple arguments: mxcli syntax workflow user-task targeting
The topic may be given as separate words, as one quoted string, or dotted —
all three reach the same page, as does the plain-word spelling ("user task").

Top-level topics:
  domain-model    - Entities, associations, enumerations, constants, keywords, types
  microflow       - Microflow/nanoflow creation and activities
  page            - Pages, snippets, fragments, widgets
  layout          - Layouts: regions, navigation, placeholders, repointing pages
  security        - Roles, access control, demo users
  workflow        - Workflows, user tasks, decisions, parallel splits
  navigation      - Navigation profiles, menus, home pages
  settings        - Project settings
  integration     - OData, REST, SQL, OQL, XPath, Java actions, business events
  agents          - AI agent documents (Model, KB, Consumed MCP Service, Agent)
  errors          - Common validation errors and fixes
  structure       - SHOW STRUCTURE command
  move            - MOVE command for relocating documents
  search          - Full-text SEARCH command
  features        - LIST FEATURES: what a Mendix version supports, and since when

Examples:
  mxcli syntax --json                          # Full index (LLM: cache this)
  mxcli syntax workflow --json                 # All workflow features
  mxcli syntax workflow user-task targeting     # Drill down to targeting
  mxcli syntax security entity-access           # Entity access rules
  mxcli syntax workflow user task               # Plain words resolve too
  mxcli syntax entity                           # Legacy alias → domain-model.entity
  mxcli syntax page --deprecated                # Old spellings for pages, and their new form
  mxcli syntax --deprecated                     # Every old spelling
`,
	Run: func(cmd *cobra.Command, args []string) {
		jsonFlag, _ := cmd.Flags().GetBool("json")
		out := cmd.OutOrStdout()

		if deprecated, _ := cmd.Flags().GetBool("deprecated"); deprecated {
			runSyntaxDeprecated(cmd, out, args, jsonFlag)
			return
		}

		// No args: show full index (JSON) or help text
		if len(args) == 0 {
			if jsonFlag {
				syntax.WriteJSON(out, syntax.All())
				return
			}
			cmd.Help()
			return
		}

		// One resolver for both surfaces — see syntax.Lookup. The topic may
		// arrive as separate words, as one quoted string, or dotted.
		m := syntax.Lookup(args)
		if len(m.Features) > 0 {
			if jsonFlag {
				syntax.WriteJSON(out, m.Features)
				return
			}
			fmt.Fprint(out, m.Heading())
			m.WriteText(out)
			return
		}

		fmt.Fprintf(out, "Unknown topic: %s\n\n", m.Path)
		cmd.Help()
	},
}

func init() {
	syntaxCmd.Flags().Bool("deprecated", false, "list the old spellings for the topic (or every topic) from the deprecation registry")
	rootCmd.AddCommand(syntaxCmd)
}

// runSyntaxDeprecated is `mxcli syntax [topic] --deprecated`: the deprecated
// spellings filed under the topic, with their new form, read from the
// deprecation registry (ako/mxcli#714 decision 3). The topic resolves as it
// does without the flag, so every spelling of a topic reaches the same list.
func runSyntaxDeprecated(cmd *cobra.Command, out io.Writer, args []string, jsonFlag bool) {
	var entries []deprecation.Entry
	label := "every topic"
	if len(args) == 0 {
		entries = deprecation.ForTopic("")
	} else {
		m := syntax.Lookup(args)
		if len(m.Features) == 0 {
			fmt.Fprintf(out, "Unknown topic: %s\n\n", m.Path)
			cmd.Help()
			return
		}
		entries = deprecatedForFeatures(m)
		label = m.Path
	}
	if jsonFlag {
		writeDeprecatedJSON(out, entries)
		return
	}
	if len(entries) == 0 {
		fmt.Fprintf(out, "No deprecated spellings for %s.\n", label)
		return
	}
	fmt.Fprintf(out, "Deprecated spellings for %s (%d). Each still runs and warns; "+
		"`mxcli fmt --upgrade` rewrites those marked so.\n\n", label, len(entries))
	for _, e := range entries {
		fmt.Fprintf(out, "%s  refused from mdl %d\n", e.Code, e.RemovedIn)
		fmt.Fprintf(out, "  old:     %s\n", e.Old)
		fmt.Fprintf(out, "  new:     %s\n", e.Canonical)
		fmt.Fprintf(out, "  rewrite: %s\n\n", migration.DeprecationRewrite(e))
	}
	fmt.Fprintln(out, "Details: mxcli help <code>")
}

// deprecatedForFeatures returns the entries filed under the matched topic, or
// under any matched feature's path when the topic came from a segment match.
func deprecatedForFeatures(m syntax.Match) []deprecation.Entry {
	paths := []string{m.Path}
	if !m.Exact {
		paths = paths[:0]
		for _, f := range m.Features {
			paths = append(paths, f.Path)
		}
	}
	seen := map[string]bool{}
	var out []deprecation.Entry
	for _, p := range paths {
		for _, e := range deprecation.ForTopic(strings.ToLower(p)) {
			if !seen[e.Code] {
				seen[e.Code] = true
				out = append(out, e)
			}
		}
	}
	return out
}

func writeDeprecatedJSON(out io.Writer, entries []deprecation.Entry) {
	type row struct {
		Code        string   `json:"code"`
		Old         string   `json:"old"`
		New         string   `json:"new"`
		Rewrite     string   `json:"rewrite"`
		RefusedFrom int      `json:"refused_from"`
		Topics      []string `json:"topics"`
	}
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, row{e.Code, e.Old, e.Canonical, migration.DeprecationRewrite(e), e.RemovedIn, e.Topics()})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(rows)
}
