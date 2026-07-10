// SPDX-License-Identifier: Apache-2.0

// Bug 10b: a widget `dynamicclasses` (lowercase) was silently dropped on write.
// Generic widget properties are stored under the user's original casing, but the
// builder reads appearance props via GetStringProp("DynamicClasses") (canonical
// case). MDL property names are case-insensitive, so the lookup must match
// regardless of casing.
package ast

import "testing"

func TestWidgetV3_GetStringProp_CaseInsensitive(t *testing.T) {
	w := &WidgetV3{Properties: map[string]any{
		"dynamicclasses": "if 1 = 1 then 'a' else 'b'",
	}}

	// Exact-case lookup still works, and the canonical-case getter resolves the
	// lowercased stored key.
	if got := w.GetStringProp("dynamicclasses"); got == "" {
		t.Fatal("exact-case lookup returned empty")
	}
	if got := w.GetDynamicClasses(); got == "" {
		t.Errorf("GetDynamicClasses() dropped a lowercased property; got empty")
	}
	if got := w.GetStringProp("DYNAMICCLASSES"); got == "" {
		t.Errorf("upper-case lookup returned empty")
	}
}

func TestWidgetV3_GetStringProp_ExactMatchWins(t *testing.T) {
	// When both an exact and a differently-cased key exist, the exact match is
	// preferred (fast path), never a case-folded sibling.
	w := &WidgetV3{Properties: map[string]any{
		"Class": "exact",
		"class": "folded",
	}}
	if got := w.GetStringProp("Class"); got != "exact" {
		t.Errorf("GetStringProp(Class) = %q, want exact", got)
	}
}
