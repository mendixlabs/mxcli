// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#1684: `mxcli check -p` warned
//
//	MDL-WIDGET10 … property `attributes` is hidden when `attrChoice` is "auto"
//
// on every text/number/date filter that names attributes, e.g.
// `textfilter tfName (Attributes: [Name])`. False: the builder's
// SetAttributeObjects writes attrChoice = "linked" whenever it fills the list.
// The definitions declared attrChoice as the constant "auto", so the value map
// the check reads disagreed with what exec stores. The drop-down filter had the
// same shape and was fixed under mendixlabs/mxcli#1519 by an `attribute` mode;
// the three sibling filters now carry the same mode.
//
// The warning only appears with a project, because the visibility rule is lifted
// from the installed DataWidgets .mpk (whose editorConfig hides `attributes`
// under "auto"). The rule is supplied directly here, as the #1519 test advises.

// filterDefWithAttributesRule returns a copy of a real filter definition whose
// visibility rules hide `attributes` when attrChoice equals hideWhen.
func filterDefWithAttributesRule(t *testing.T, mdlName, hideWhen string) (*WidgetRegistry, *WidgetDefinition) {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	real, ok := reg.Get(mdlName)
	if !ok {
		t.Fatalf("no %s definition", mdlName)
	}
	def := *real
	def.PropertyVisibility = []types.WidgetVisibilityRule{{
		PropertyKey: "attributes",
		HiddenWhen:  &types.WidgetVisibilityCondition{PropertyKey: "attrChoice", Operator: "eq", Value: hideWhen},
	}}
	return &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{mdlName: &def}}, &def
}

func TestCheck1684_FilterAttributesAreNotHidden(t *testing.T) {
	for _, kw := range []string{"textfilter", "numberfilter", "datefilter"} {
		t.Run(kw, func(t *testing.T) {
			reg, def := filterDefWithAttributesRule(t, strings.ToUpper(kw), "auto")
			w := parseSnippetWidget(t, kw+` f (Attributes: [Name])`)

			values, _ := widgetValueMap(w, def)
			if got := values["attrchoice"]; got != "linked" {
				t.Errorf("attrChoice = %q, want linked (what SetAttributeObjects stores)", got)
			}
			if v := validateWidgetVisibility(w, reg, "page P"); len(v) != 0 {
				t.Errorf("got %d violation(s) %+v, want none — the builder stores attrChoice linked", len(v), v)
			}
		})
	}
}

// A bare filter still builds in the column-bound "auto" choice.
func TestWidgetValueMap1684_BareFilterStaysAuto(t *testing.T) {
	_, def := filterDefWithAttributesRule(t, "TEXTFILTER", "auto")
	values, _ := widgetValueMap(parseSnippetWidget(t, `textfilter f`), def)
	if got := values["attrchoice"]; got != "auto" {
		t.Errorf("attrChoice = %q, want auto", got)
	}
}

// Control: the rule still fires for a property really hidden under the
// configuration the builder writes.
func TestCheck1684_ReallyHiddenAttributesStillWarn(t *testing.T) {
	reg, _ := filterDefWithAttributesRule(t, "TEXTFILTER", "linked")
	w := parseSnippetWidget(t, `textfilter f (Attributes: [Name])`)
	v := validateWidgetVisibility(w, reg, "page P")
	if len(v) != 1 || v[0].RuleID != "MDL-WIDGET10" {
		t.Fatalf("got %d violation(s) %+v, want 1 MDL-WIDGET10", len(v), v)
	}
}
