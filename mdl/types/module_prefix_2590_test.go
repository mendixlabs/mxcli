// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// ako/mxcli#2590: a module rename rewrites the module of each qualified
// reference inside an expression, XPath or OQL, and nothing else.
func TestRewriteModulePrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[OldMod.Contract_Company = $Company]", "[NewMod.Contract_Company = $Company]"},
		{"[Number = 1][not(OldMod.Contract_Company = $Company)]", "[Number = 1][not(NewMod.Contract_Company = $Company)]"},
		{"@OldMod.Prefix + 'x'", "@NewMod.Prefix + 'x'"},
		{"$O/OldMod.Order_Customer/Name", "$O/NewMod.Order_Customer/Name"},
		{"$S = OldMod.Status.Open", "$S = NewMod.Status.Open"},
		{"SELECT c.Name FROM OldMod.Company AS c", "SELECT c.Name FROM NewMod.Company AS c"},
		// Controls.
		{"'OldMod.Contract'", "'OldMod.Contract'"},       // a string literal is data
		{"'it''s OldMod.X'", "'it''s OldMod.X'"},         // with an escaped quote
		{"[XOldMod.Assoc = $C]", "[XOldMod.Assoc = $C]"}, // a longer module name
		{"$OldMod.Name", "$OldMod.Name"},                 // a variable of that name
		{"Other.OldMod.X", "Other.OldMod.X"},             // a member, not a module
		{"OldMod.", "OldMod."},                           // no name after the dot
	}
	for _, c := range cases {
		got, n := RewriteModulePrefix(c.in, "OldMod", "NewMod")
		if got != c.want {
			t.Errorf("%q → %q, want %q", c.in, got, c.want)
		}
		if (n > 0) != (c.in != c.want) {
			t.Errorf("%q: count %d", c.in, n)
		}
	}
	// A qualified name is RewriteExpressionReferences' job, not this one's.
	if got, n := RewriteModulePrefix("[OldMod.A = 1]", "OldMod.A", "OldMod.B"); n != 0 || got != "[OldMod.A = 1]" {
		t.Errorf("a qualified old name was handled: %q %d", got, n)
	}
}
