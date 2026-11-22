// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// The Datagrid drop-down filter (DatagridDropdownFilter.editorConfig.js, the
// line shipped since 2.x) hides its association properties in a function
// invoked as a ternary's then-branch:
//
//	return "attr"===e.baseType ? t=function(e,t){ hide([… "refEntity" …]),
//	    "auto"===e.attrChoice && (hide("linkedDs"), hide("attr")); …; return t }(e,t)
//	  : hide(["attr","attrChoice","filterOptions","auto"]), …
//
// The first hide was read as having no guard at all (its text ends at the
// function's `{`), so refEntity — required — read as always shown and
// MDL-WIDGET56 reported it on every attribute filter (ako/mxcli#2523). The
// nested attrChoice hides were lifted without the outer condition, claiming
// linkedDs hidden in association mode too.
const iifeThenJS = `function getProperties(e,t){var r=e.filterable&&e.multiSelect;return"attr"===e.baseType?t=function(e,t){A.hidePropertiesIn(t,e,["refEntity","refSearchAttr"]),"auto"===e.attrChoice&&(A.hidePropertyIn(t,{},"linkedDs"),A.hidePropertyIn(t,e,"attr"));return t}(e,t):A.hidePropertiesIn(t,e,["attr","attrChoice"]),t}`

func TestTernaryThenIIFEHidesCarryTheTernaryCondition_2523(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(iifeThenJS)
	byKey := map[string][]string{}
	for _, r := range rules {
		byKey[r.PropertyKey] = append(byKey[r.PropertyKey], iifeRuleText(r))
	}
	want := map[string]string{
		"refEntity":     "baseType eq attr",
		"refSearchAttr": "baseType eq attr",
		"linkedDs":      "attrChoice eq auto & baseType eq attr",
		"attr":          "attrChoice eq auto & baseType eq attr | baseType ne attr",
		"attrChoice":    "baseType ne attr",
	}
	for k, w := range want {
		if got := strings.Join(sortedCopy(byKey[k]), " | "); got != w {
			t.Errorf("%s: rules %q, want %q", k, got, w)
		}
	}
}

func iifeRuleText(r types.WidgetVisibilityRule) string {
	parts := []string{r.HiddenWhen.PropertyKey + " " + r.HiddenWhen.Operator + " " + r.HiddenWhen.Value}
	for _, c := range r.And {
		parts = append(parts, c.PropertyKey+" "+c.Operator+" "+c.Value)
	}
	return strings.Join(parts, " & ")
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
