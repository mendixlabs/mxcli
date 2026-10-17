// SPDX-License-Identifier: Apache-2.0

package pages

import "testing"

// Editable and Visible share a grammar slot, so a value must mean the same
// thing to both consumers' helpers: a string that is not the property's plain
// value is its condition.
func TestStaticEditability(t *testing.T) {
	cases := []struct {
		in   any
		enum string
		expr string
		ok   bool
	}{
		{"Always", "Always", "", true},
		{"never", "Never", "", true},
		{" NEVER ", "Never", "", true},
		{"Conditional", "Conditional", "", true},
		{true, "Always", "", true},
		{false, "Never", "", true},
		{"true", "Always", "", true},
		{"False", "Never", "", true},
		{"$currentObject/Locked = false", "", "$currentObject/Locked = false", true},
		{"  $currentObject/Code != 'x' ", "", "$currentObject/Code != 'x'", true},
		// Not an enum word: an expression, and the caller's check decides if it is a sensible one.
		{"Neever", "", "Neever", true},
		{"", "", "", false},
		{"   ", "", "", false},
		{nil, "", "", false},
		{42, "", "", false},
		{[]string{"x"}, "", "", false},
	}
	for _, c := range cases {
		enum, expr, ok := StaticEditability(c.in)
		if enum != c.enum || expr != c.expr || ok != c.ok {
			t.Errorf("StaticEditability(%#v) = (%q, %q, %v), want (%q, %q, %v)", c.in, enum, expr, ok, c.enum, c.expr, c.ok)
		}
	}
}

// Visible and Editable agree on what a quoted expression is: the string, verbatim.
func TestStaticEditability_ExpressionMatchesStaticVisibleExpression(t *testing.T) {
	const e = "$currentObject/Locked = false"
	ve, vok := StaticVisibleExpression(e)
	_, ee, eok := StaticEditability(e)
	if ve != ee || !vok || !eok {
		t.Errorf("Visible reads %q (%v), Editable reads %q (%v)", ve, vok, ee, eok)
	}
}
