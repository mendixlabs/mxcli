// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// WriteJSON writes features as a JSON array.
func WriteJSON(w io.Writer, features []SyntaxFeature) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(features)
}

// WriteText writes features in a human-readable grouped format.
func WriteText(w io.Writer, features []SyntaxFeature) {
	if len(features) == 0 {
		return
	}

	// Single feature — show full detail
	if len(features) == 1 {
		writeFeatureDetail(w, features[0])
		return
	}

	// Multiple features — group by top-level domain
	groups := groupByDomain(features)
	for i, g := range groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s\n", g.title)
		fmt.Fprintf(w, "%s\n\n", strings.Repeat("─", len(g.title)))
		for _, f := range g.features {
			fmt.Fprintf(w, "  %-40s %s\n", f.Path, f.Summary)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Use 'mxcli syntax <path> --json' for machine-readable detail.\n")
	fmt.Fprintf(w, "Use 'mxcli syntax <path>' to drill down (e.g., 'mxcli syntax workflow user-task').\n")
}

func writeFeatureDetail(w io.Writer, f SyntaxFeature) {
	fmt.Fprintf(w, "%s\n", f.Path)
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("═", len(f.Path)))
	fmt.Fprintf(w, "%s\n\n", f.Summary)

	switch {
	case len(f.Requires) == 1:
		fmt.Fprintf(w, "Requires: Mendix %s\n\n", f.Requires[0].versionRange())
	case len(f.Requires) > 1:
		fmt.Fprintln(w, "Requires:")
		width := 9
		for _, r := range f.Requires {
			width = max(width, len(r.versionRange()))
		}
		for _, r := range f.Requires {
			fmt.Fprintf(w, "  Mendix %-*s %s\n", width, r.versionRange(), r.Feature)
		}
		fmt.Fprintln(w)
	}

	if len(f.Keywords) > 0 {
		fmt.Fprintf(w, "Keywords: %s\n\n", strings.Join(f.Keywords, ", "))
	}

	if f.Syntax != "" {
		fmt.Fprintln(w, "Syntax:")
		for _, line := range strings.Split(f.Syntax, "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
		fmt.Fprintln(w)
	}

	if f.Example != "" {
		fmt.Fprintln(w, "Example:")
		for _, line := range strings.Split(f.Example, "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
		fmt.Fprintln(w)
	}

	if len(f.SeeAlso) > 0 {
		fmt.Fprintf(w, "See also: %s\n", strings.Join(f.SeeAlso, ", "))
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

type featureGroup struct {
	title    string
	features []SyntaxFeature
}

func groupByDomain(features []SyntaxFeature) []featureGroup {
	order := []string{}
	m := map[string][]SyntaxFeature{}
	for _, f := range features {
		domain := f.Path
		if idx := strings.IndexByte(f.Path, '.'); idx > 0 {
			domain = f.Path[:idx]
		}
		if _, ok := m[domain]; !ok {
			order = append(order, domain)
		}
		m[domain] = append(m[domain], f)
	}
	groups := make([]featureGroup, len(order))
	for i, d := range order {
		groups[i] = featureGroup{title: capitalize(d), features: m[d]}
	}
	return groups
}

// versionRange is "10.0.0+" for a feature Mendix still has, and
// "10.0.0 to 11.14.99" for one it removed.
func (r Requirement) versionRange() string {
	if r.Until != "" {
		return r.Since + " to " + r.Until
	}
	return r.Since + "+"
}

// WriteText renders a lookup's answer. A nested topic that has subtopics of
// its own (workflow.user-task, with workflow.user-task.targeting under it)
// prints its detail and then lists them; it used to print only the index, so
// `mxcli syntax workflow user-task` never showed the user task syntax
// (ako/mxcli#2238). A top-level area (`workflow`, `microflow`) stays an index
// of its topics, and every other answer is WriteText's.
func (m Match) WriteText(w io.Writer) {
	if own := ByPath(m.Path); m.Exact && own != nil && strings.Contains(m.Path, ".") && len(m.Features) > 1 {
		writeFeatureDetail(w, *own)
		fmt.Fprintln(w, "Subtopics:")
		for _, f := range m.Features {
			if f.Path != own.Path {
				fmt.Fprintf(w, "  %-40s %s\n", f.Path, f.Summary)
			}
		}
		return
	}
	WriteText(w, m.Features)
}
