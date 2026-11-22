// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#2518: the Data grid Drop-down filter (DatagridDropdownFilter 3.4.0)
// hides emptyOptionCaption behind a disjunction,
//
//	(e.filterable||e.multiSelect)&&A.hidePropertyIn(t,e,"emptyOptionCaption")
//
// and the extractor lifted no rule for it, so mxcli never nulled the text
// template while the property was hidden — CE0463 on every multi-select filter.
// `mx update-widgets` stores null when either is set and the declared default
// translations when neither is (measured on mx 11.14.0, 7 filters, 2 pages).

// hiddenBy reports whether any rule for key fires on values, and whether the
// rules could decide it at all.
func hiddenBy(rules []types.WidgetVisibilityRule, key string, values map[string]string) (hidden, decided bool) {
	for _, r := range rules {
		if r.PropertyKey != key || r.Nested() {
			continue
		}
		fires, det := r.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
			v, ok := values[c.PropertyKey]
			return v, ok
		})
		if det {
			decided = true
		}
		if det && fires {
			hidden = true
		}
	}
	return hidden, decided
}

// The drop-down filter's getProperties, as the 3.4.0 package ships it.
const dropdownFilterGetProperties = `getProperties=function(e,t){var r=e.filterable&&e.multiSelect,n=r&&"boxes"===e.selectedItemsStyle;return"attr"===e.baseType?t=function(e,t){A.hidePropertiesIn(t,e,["fetchOptionsLazy","refCaption","refCaptionExp","refCaptionSource","refEntity","refOptions","refSearchAttr"]),"auto"===e.attrChoice&&(A.hidePropertyIn(t,{},"linkedDs"),A.hidePropertyIn(t,e,"attr"));e.auto&&A.hidePropertyIn(t,e,"filterOptions");return t}(e,t):A.hidePropertiesIn(t,e,["attr","attrChoice","filterOptions","auto"]),(e.filterable||e.multiSelect)&&A.hidePropertyIn(t,e,"emptyOptionCaption"),e.filterable?A.hidePropertyIn(t,e,"clearable"):(A.hidePropertyIn(t,e,"refSearchAttr"),A.hidePropertyIn(t,e,"filterInputPlaceholderCaption")),"attr"===e.refCaptionSource?A.hidePropertiesIn(t,e,["refCaptionExp","refSearchAttr"]):A.hidePropertyIn(t,e,"refCaption"),r||A.hidePropertyIn(t,e,"selectedItemsStyle"),n||A.hidePropertyIn(t,e,"selectionMethod"),t};`

func TestExtractVisibility_DisjunctiveGuard(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(dropdownFilterGetProperties)
	for _, c := range []struct {
		filterable, multiSelect string
		hidden                  bool
	}{
		{"false", "false", false},
		{"true", "false", true},
		{"false", "true", true},
		{"true", "true", true},
	} {
		got, decided := hiddenBy(rules, "emptyOptionCaption", map[string]string{"filterable": c.filterable, "multiSelect": c.multiSelect})
		if !decided || got != c.hidden {
			t.Errorf("filterable=%s multiSelect=%s: emptyOptionCaption hidden=%v (decided %v), want %v",
				c.filterable, c.multiSelect, got, decided, c.hidden)
		}
	}
}

// A disjunction is lifted only where it is the whole condition: inside a guarded
// group, or as one conjunct of a longer chain, a rule per disjunct would hide the
// property in configurations the editor shows.
func TestExtractVisibility_DisjunctionOnlyAtStatementLevel(t *testing.T) {
	for _, js := range []string{
		`getProperties=function(e,t){return e.g?((e.a||e.b)&&A.hidePropertyIn(t,e,"p"),t):t};`,
		`getProperties=function(e,t){return e.c&&(e.a||e.b)&&A.hidePropertyIn(t,e,"p"),t};`,
		`getProperties=function(e,t){return(e.a||e.b)&&e.c&&A.hidePropertyIn(t,e,"p"),t};`,
		`getProperties=function(e,t){return(e.a||e.b&&e.c)&&A.hidePropertyIn(t,e,"p"),t};`,
	} {
		rules, _ := extractVisibilityRulesFromJS(js)
		if hidden, _ := hiddenBy(rules, "p", map[string]string{"a": "true", "b": "false", "c": "false", "g": "false"}); hidden {
			t.Errorf("hides p where the editor shows it:\n%s\n%+v", js, rules)
		}
	}
	// Control: the same disjunction at statement level, with a comparison term.
	rules, _ := extractVisibilityRulesFromJS(`getProperties=function(e,t){return("x"===e.k||e.b)&&A.hidePropertiesIn(t,e,["p","q"]),t};`)
	for _, key := range []string{"p", "q"} {
		if hidden, _ := hiddenBy(rules, key, map[string]string{"k": "x", "b": "false"}); !hidden {
			t.Errorf("%s: not hidden when k is \"x\": %+v", key, rules)
		}
		if hidden, _ := hiddenBy(rules, key, map[string]string{"k": "y", "b": "false"}); hidden {
			t.Errorf("%s: hidden when neither disjunct holds: %+v", key, rules)
		}
	}
}
