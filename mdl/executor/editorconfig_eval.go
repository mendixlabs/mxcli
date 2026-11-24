// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/editorconfig"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// evaluatedVisibilityRules answers a widget's top-level visibility for ONE
// configuration by running its editorConfig's getProperties (ako/mxcli#1776),
// in the form every consumer already reads: rules.
//
// The rules lifted from the bundle's source are partial — 96 of 170 measured
// widget versions had hide-calls the reader could not state, a widget with its
// own hide helper (ChessClock, ako/mxcli#2507) had none at all, and a guard on
// Studio's `target` was read as if it applied in Studio Pro (TreeNode, Maps,
// the charts). Running the function has none of those gaps, so when it runs,
// its answer replaces every top-level rule:
//
//   - a property it hides keeps the lifted rules about it (they name the
//     condition, which is the better message) plus one that always fires;
//   - a property it shows gets one rule that never fires — a lifted rule saying
//     otherwise is wrong for Studio Pro, and the property stays among the ones
//     the schema treats as conditional.
//
// Nested (object-list item) rules are kept as lifted. When the bundle cannot be
// run — no package installed, a script error — the lifted rules are returned
// unchanged, which is exactly the behaviour before.
//
// values is keyed by property key (any case), in mxcli's string form; a
// property it does not name is taken at its declared default.
func evaluatedVisibilityRules(projectPath, widgetID string, lifted []types.WidgetVisibilityRule, values map[string]string) []types.WidgetVisibilityRule {
	hidden, ok := evaluateHiddenProperties(projectPath, widgetID, values)
	if !ok {
		return lifted
	}
	return mergeEvaluatedRules(lifted, hidden)
}

// mergeEvaluatedRules is evaluatedVisibilityRules once the hidden set is known.
func mergeEvaluatedRules(lifted []types.WidgetVisibilityRule, hidden map[string]bool) []types.WidgetVisibilityRule {
	hiddenLower := map[string]string{}
	for k, h := range hidden {
		if h {
			hiddenLower[strings.ToLower(k)] = k
		}
	}
	var out []types.WidgetVisibilityRule
	shown := map[string]bool{}
	for _, r := range lifted {
		if r.Nested() {
			out = append(out, r)
			continue
		}
		lower := strings.ToLower(r.PropertyKey)
		if _, isHidden := hiddenLower[lower]; isHidden {
			out = append(out, r)
			continue
		}
		if !shown[lower] {
			shown[lower] = true
			out = append(out, types.WidgetVisibilityRule{
				PropertyKey: r.PropertyKey,
				HiddenWhen:  &types.WidgetVisibilityCondition{Operator: types.OperatorNever},
			})
		}
	}
	for _, lower := range sortedHiddenKeys(hiddenLower) {
		out = append(out, types.WidgetVisibilityRule{
			PropertyKey: hiddenLower[lower],
			HiddenWhen:  &types.WidgetVisibilityCondition{Operator: types.OperatorAlways},
		})
	}
	return out
}

func sortedHiddenKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// evaluateHiddenProperties runs the installed package's getProperties for the
// given values. ok is false when there is nothing to run or the run failed —
// the caller then keeps the lifted rules.
func evaluateHiddenProperties(projectPath, widgetID string, values map[string]string) (map[string]bool, bool) {
	if projectPath == "" || widgetID == "" {
		return nil, false
	}
	projectDir := projectPath
	if strings.EqualFold(filepath.Ext(projectDir), ".mpr") {
		projectDir = filepath.Dir(projectDir)
	}
	mpkPath, err := mpk.FindMPK(projectDir, widgetID)
	if err != nil || mpkPath == "" {
		return nil, false
	}
	ev, err := editorconfig.Load(mpkPath, widgetID)
	if err != nil {
		return nil, false
	}
	hidden, err := ev.Hidden(values)
	if err != nil {
		return nil, false
	}
	return hidden, true
}

// sortLiftedFirst orders rules so a lifted rule comes before an evaluated one,
// keeping each group's order.
func sortLiftedFirst(rules []types.WidgetVisibilityRule) {
	sort.SliceStable(rules, func(i, j int) bool { return !rules[i].Evaluated() && rules[j].Evaluated() })
}
