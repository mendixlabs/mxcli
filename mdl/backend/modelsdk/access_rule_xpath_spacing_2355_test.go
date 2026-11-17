// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#2355: a grant matched the stored rule by role set and XPath
// constraint compared byte for byte, so the same constraint written over two
// lines — or stored on one line by an older mxcli or by Studio Pro — added a
// second, identical rule instead of merging, and a re-run of the script
// changed the project. Constraints that differ only in whitespace outside
// string literals are one constraint.
func TestGrant_XPathDifferingOnlyInWhitespaceMerges_Issue2355(t *testing.T) {
	b, mod, dm := grantFixture(t)
	oneLine := "[Name = 'a' or Name = 'b']"
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "ReadOnly", oneLine, nil)); err != nil {
		t.Fatal(err)
	}
	wrapped := "[Name = 'a'\n         or Name = 'b']"
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "ReadOnly", wrapped, nil)); err != nil {
		t.Fatal(err)
	}
	rule := onlyRule(t, b, mod.ID)
	// The stored spelling is kept, so a re-run in the other spelling writes
	// nothing.
	if rule.XPathConstraint != oneLine {
		t.Errorf("constraint = %q, want the stored %q kept", rule.XPathConstraint, oneLine)
	}
}

// CONTROL: whitespace inside a string literal is part of the constraint, so a
// different literal is a different rule.
func TestGrant_XPathLiteralWhitespaceIsDifferent_Issue2355(t *testing.T) {
	b, mod, dm := grantFixture(t)
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "ReadOnly", "[Name = 'a b']", nil)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "ReadOnly", "[Name = 'ab']", nil)); err != nil {
		t.Fatal(err)
	}
	if n := len(rulesOf(t, b, mod.ID)); n != 2 {
		t.Errorf("rules = %d, want 2 (the literals differ)", n)
	}
}

func TestSameXPathConstraint_Issue2355(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"[a = 'x' or b = 'y']", "[a = 'x'\n  or b = 'y']", true},
		{"", "", true},
		{"[a = 'x y']", "[a = 'xy']", false},
		{"", "[a = 1]", false},
	} {
		if got := types.SameXPathConstraint(c.a, c.b); got != c.same {
			t.Errorf("SameXPathConstraint(%q, %q) = %v, want %v", c.a, c.b, got, c.same)
		}
	}
}
