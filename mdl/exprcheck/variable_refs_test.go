// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"reflect"
	"testing"
)

func TestVariableReferences(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"$Nope + 1", []string{"Nope"}},
		{"$a + $b * $a", []string{"a", "b", "a"}},
		{"$Order/Status = 'Open'", []string{"Order"}},
		{"if $Flag then $X else $Y", []string{"Flag", "X", "Y"}},
		{"length($S) > $Max", []string{"S", "Max"}},
		{"not($Done)", []string{"Done"}},
		{"($A)", []string{"A"}},
		// No variable at all.
		{"'text'", nil},
		{"1 + 2", nil},
		{"[%CurrentDateTime%]", nil},
		{"Mod.Enum.Value", nil},
		{"", nil},
		// A bare identifier is not a `$` reference: a predicate uses one for an
		// attribute of the item under test.
		{"Status = 'Open'", nil},
	}
	for _, c := range cases {
		var got []string
		for _, r := range VariableReferences(c.src) {
			got = append(got, r.Name)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %v, want %v", c.src, got, c.want)
		}
	}
}
