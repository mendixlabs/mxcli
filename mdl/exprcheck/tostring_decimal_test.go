// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

func TestToStringOfDecimal(t *testing.T) {
	vars := map[string]TypeKind{"d": KindDecimal, "i": KindInteger, "a": KindDateTime, "b": KindDateTime}
	for src, want := range map[string]bool{
		"toString(minutesBetween($a, $b))":        true,
		"toString($d)":                            true,
		"toString($i div 3)":                      true,
		"toString(round(minutesBetween($a, $b)))": false, // one-arg round is whole
		"toString($i)":                            false,
		"toString($unknown)":                      false,
		"minutesBetween($a, $b)":                  false,
		"'x' + toString($d)":                      false, // not the whole value
	} {
		if got := ToStringOfDecimal(src, vars); got != want {
			t.Errorf("ToStringOfDecimal(%q) = %v, want %v", src, got, want)
		}
	}
}
