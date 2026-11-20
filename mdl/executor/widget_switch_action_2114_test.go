// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#2114: describe printed a stored Switch's action (`action: call
// microflow …`), and check refused it as MDL-WIDGET37 ("has no action slot
// `onClick:` writes to"). The Switch's one action slot is keyed plain
// `action`; trimmed as a suffix it became the empty name and took no source,
// so MDL's `action:` keyword routed nowhere.
func TestActionSourceForKey_PlainActionIsTheWidgetsAction(t *testing.T) {
	if got := actionSourceForKey("action"); got != "OnClick" {
		t.Errorf(`actionSourceForKey("action") = %q, want OnClick`, got)
	}
	// Controls: a named slot is still addressed by its own key, not by the
	// keyword (File Uploader, #956), and the event-suffixed rules hold.
	for key, want := range map[string]string{
		"createFileAction": "",
		"onClickAction":    "OnClick",
		"onChangeAction":   "OnChange",
	} {
		if got := actionSourceForKey(key); got != want {
			t.Errorf("actionSourceForKey(%q) = %q, want %q", key, got, want)
		}
	}
}

// A definition cached on disk by an older extraction carries the slot with no
// source; check read that cache and refused what exec wrote. Loading fills it.
func TestFillActionSources_RoutesACachedUnroutedSlot(t *testing.T) {
	def := &WidgetDefinition{
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "action", Operation: "action"},
			{PropertyKey: "createFileAction", Operation: "action"},
			{PropertyKey: "onClick", Operation: "action", Source: "OnClick"},
			{PropertyKey: "booleanAttribute", Operation: "attribute", Source: "Attribute"},
		},
		Modes: []WidgetMode{{PropertyMappings: []PropertyMapping{{PropertyKey: "action", Operation: "action"}}}},
	}
	fillActionSources(def)
	want := []string{"OnClick", "", "OnClick", "Attribute"}
	for i, m := range def.PropertyMappings {
		if m.Source != want[i] {
			t.Errorf("%s: Source = %q, want %q", m.PropertyKey, m.Source, want[i])
		}
	}
	if got := def.Modes[0].PropertyMappings[0].Source; got != "OnClick" {
		t.Errorf("mode mapping Source = %q, want OnClick", got)
	}
	if !defRoutesActionSource(def, "OnClick") {
		t.Error("the definition still routes no `action:`, so check would refuse it")
	}
}
