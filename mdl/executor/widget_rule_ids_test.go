// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// MDL-WIDGET rule numbers are handed out by hand, from no registry, by grepping
// for the highest one in use. That has now failed twice:
//
//   - once at authoring time, picking 27 for a rule that already existed;
//   - once at MERGE time, where two branches independently took 29 — one for a
//     retired-spelling rule, one for an unroutable child — and each was correct
//     in isolation. Nothing compared them until a rebase put both in the tree.
//
// The second is the one that matters, because no amount of care at authoring
// time prevents it. This test does the comparison, so the collision fails on the
// merge that creates it rather than shipping as two rules answering to one id.
//
// The invariant is NOT "one file per id": a rule legitimately raised from two
// places stays one rule. So known pairs are listed, and anything new fails —
// which is exactly the moment to check whether it is one rule or two.
var widgetRuleIDsRaisedFromSeveralFiles = map[string][]string{
	// One rule about a design-property KEY the theme does not declare, asked of
	// the widget trees a statement carries and of ALTER STYLING, which names a
	// stored widget instead (ako/mxcli#509).
	"MDL-WIDGET11": {"validate_alter_styling.go", "validate_design_properties.go"},
	// One rule about a design-property VALUE the theme does not allow — an
	// off-list option, or a single value where the property takes a set
	// (ako/mxcli#511). Same two paths.
	"MDL-WIDGET12": {"validate_alter_styling.go", "validate_design_properties.go"},
	// One rule about a property the builder silently drops on write — a key no
	// builder reads, or DesignProperties on a slot of the parent (a layoutgrid
	// row, a row's column, a dataview footer) that never reaches
	// applyWidgetAppearance, or Visible: on a row's column, which has no
	// visibility slot (mendixlabs/mxcli#1461).
	"MDL-WIDGET07": {"validate_design_properties.go", "validate_layoutgrid_column_visible.go", "validate_widgets.go"},
	// One rule about an enumeration value the widget declares no key for: an
	// object-list item's sub-property, and — gated on the language version,
	// MDL-V1-WIDGETENUM under mdl 0 — a widget's own property (ako/mxcli#854).
	"MDL-WIDGET08": {"validate_widget_language.go", "validate_widgets.go"},
}

// Matched as a QUOTED literal rather than after `RuleID:`, because an id is
// also declared as a named constant (imageSourceRule = "MDL-WIDGET22") — the
// first version of this guard missed that one and reported a gap that was not
// there. Quoting is what separates a declaration from a doc comment mentioning
// a neighbouring rule.
var widgetRuleIDRe = regexp.MustCompile(`"(MDL-WIDGET\d+)"`)

func TestWidgetRuleIDsAreNotReused(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range widgetRuleIDRe.FindAllStringSubmatch(string(src), -1) {
			if byID[m[1]] == nil {
				byID[m[1]] = map[string]bool{}
			}
			byID[m[1]][f] = true
		}
	}
	if len(byID) == 0 {
		t.Fatal("no MDL-WIDGET rule ids found — this guard would pass vacuously")
	}

	for id, files := range byID {
		if len(files) < 2 {
			continue
		}
		var got []string
		for f := range files {
			got = append(got, f)
		}
		sort.Strings(got)
		want := append([]string(nil), widgetRuleIDsRaisedFromSeveralFiles[id]...)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s is raised from %v.\n"+
				"If those are TWO different rules, one of them needs a new number — take the next "+
				"free one after the highest in use, and check again after rebasing, because another "+
				"branch may have taken it meanwhile.\n"+
				"If it is ONE rule raised from two places, add it to "+
				"widgetRuleIDsRaisedFromSeveralFiles.", id, got)
		}
	}
}

// The numbers are also expected to be dense, so "the next free one" is a
// question with an answer. A gap means a rule was removed without its number
// being reused, which is fine but worth stating deliberately rather than
// leaving the next author to guess.
func TestWidgetRuleIDsHaveNoGaps(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	seen := map[int]bool{}
	max := 0
	numRe := regexp.MustCompile(`MDL-WIDGET(\d+)`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range widgetRuleIDRe.FindAllStringSubmatch(string(src), -1) {
			n := 0
			for _, d := range numRe.FindStringSubmatch(m[1])[1] {
				n = n*10 + int(d-'0')
			}
			seen[n] = true
			if n > max {
				max = n
			}
		}
	}
	// Retired numbers, never reused: a report or script naming one must not
	// start meaning a different rule.
	retired := map[int]string{
		3:  "an inline style on a dynamictext; the \"crashes MxBuild\" claim does not reproduce on 11.12.1, so it is allowed (ako/mxcli#2183 follow-up)",
		14: "an expression in contentparams/captionparams; stored as the parameter's Expression since ako/mxcli#823 option C",
		24: "a `$variable/` template parameter; stored as the Mendix expression it is since mendixlabs/mxcli#1406",
	}
	var missing []int
	for i := 1; i < max; i++ {
		if _, gone := retired[i]; gone {
			continue
		}
		if !seen[i] {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		t.Errorf("MDL-WIDGET numbers are not dense: %v missing below %d. "+
			"Reuse the lowest free number, or note here why it is retired.", missing, max)
	}
}
