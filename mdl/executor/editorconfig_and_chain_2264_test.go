// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#2264 (mendixlabs/mxcli#1861): Combo box 2.10 hides noOptionsText
// under a two-condition guard,
//
//	"none"===t.filterType&&"rowclick"!==t.selectionMethod&&x.hidePropertiesIn(e,t,["noOptionsText"])
//
// and the reader lifted no rule for an `&&` chain, so mxcli wrote a text
// template where Studio Pro stores null; mx check 11.12.6 reported CE0463 on
// the combo box with filterType 'none' (and 0 errors with this fix).
func TestEditorConfig_AndChainGuard(t *testing.T) {
	rule := func(js string) (types.WidgetVisibilityRule, bool) {
		rules, _ := extractVisibilityRulesFromJS(js)
		for _, r := range rules {
			if r.PropertyKey == "noOptionsText" {
				return r, true
			}
		}
		return types.WidgetVisibilityRule{}, false
	}

	r, ok := rule(`function g(e,t){return t.a.forEach((function(o,r){x.hideNestedPropertiesIn(e,t,"q",r,["w"])})),"none"===t.filterType&&"rowclick"!==t.selectionMethod&&x.hidePropertiesIn(e,t,["noOptionsText"]),e}`)
	if !ok {
		t.Fatal("the reported two-condition guard lifted no rule")
	}
	if r.HiddenWhen == nil || r.HiddenWhen.PropertyKey != "selectionMethod" || r.HiddenWhen.Value != "rowclick" {
		t.Errorf("HiddenWhen = %+v, want selectionMethod ≠ rowclick", r.HiddenWhen)
	}
	if len(r.And) != 1 || r.And[0].PropertyKey != "filterType" || r.And[0].Value != "none" {
		t.Errorf("And = %+v, want [filterType = none]", r.And)
	}

	// Three conjuncts.
	if r, ok := rule(`function g(e,t){"a"===t.p&&"b"===t.q&&"c"!==t.r&&x.hidePropertiesIn(e,t,["noOptionsText"])}`); !ok || len(r.And) != 2 {
		t.Errorf("three conjuncts: ok=%v And=%+v, want two earlier terms", ok, r.And)
	}

	// Controls — no rule rather than a wrong one:
	for name, js := range map[string]string{
		// a conjunct the guard vocabulary cannot read
		"unreadable conjunct": `function g(e,t){f(t.p)&&"b"===t.q&&x.hidePropertiesIn(e,t,["noOptionsText"])}`,
		// the chain sits inside a group with its own condition
		"inside a guarded group": `function g(e,t){t.z?("a"===t.p&&"b"===t.q&&x.hidePropertiesIn(e,t,["noOptionsText"])):0}`,
		// mixed with ||
		"behind an ||": `function g(e,t){"a"===t.p||"b"===t.q&&x.hidePropertiesIn(e,t,["noOptionsText"])}`,
	} {
		if r, ok := rule(js); ok && len(r.And) > 0 {
			t.Errorf("%s: lifted %+v And=%+v, want no conjunctive rule", name, r.HiddenWhen, r.And)
		}
	}
}
