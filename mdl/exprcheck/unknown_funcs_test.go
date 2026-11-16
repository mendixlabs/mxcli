// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

func TestUnknownFunctionCalls(t *testing.T) {
	cases := []struct {
		src        string
		wantName   string // "" = expect no unknown funcs
		wantSuggat string // expected suggestion (if any)
	}{
		{"randomInt(9)", "randomInt", "random"},
		{"round(random() * 8)", "", ""},    // all known
		{"toUpperCase($x)", "", ""},        // known
		{"secondsBetween($a, $b)", "", ""}, // known
		{"$a + length($s)", "", ""},        // known nested
		{"if $a then floor($b) else ceil($c)", "", ""},
		{"totallyMadeUpFn($x)", "totallyMadeUpFn", ""}, // no close match
		// replace() is not a Mendix function: the string replacers are replaceAll
		// and replaceFirst. MxBuild reports CE0117 and Mendix CLI 2 CE7811
		// "Unknown function 'replace'" (measured on 11.16.1).
		{"replace($s, 'a', 'b')", "replace", "replaceAll"},
		{"replaceAll($s, 'a', 'b')", "", ""},
		{"replaceFirst($s, 'a', 'b')", "", ""},
	}
	for _, c := range cases {
		got := UnknownFunctionCalls(c.src)
		if c.wantName == "" {
			if len(got) != 0 {
				t.Errorf("%q: expected no unknown funcs, got %+v", c.src, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Name != c.wantName {
			t.Fatalf("%q: expected unknown func %q, got %+v", c.src, c.wantName, got)
		}
		if got[0].Suggestion != c.wantSuggat {
			t.Errorf("%q: expected suggestion %q, got %q", c.src, c.wantSuggat, got[0].Suggestion)
		}
	}
}

func TestSourceRejectedForIntegerTarget(t *testing.T) {
	vars := map[string]TypeKind{"a": KindInteger, "b": KindInteger, "d1": KindDateTime, "d2": KindDateTime}
	cases := []struct {
		src  string
		want bool
	}{
		{"$a div $b", true},                        // arithmetic Decimal
		{"secondsBetween($d1, $d2)", true},         // Decimal-returning func
		{"random()", true},                         // Decimal-returning func
		{"round(random() * 8)", false},             // rounding → accepted
		{"floor($a div $b)", false},                // rounding → accepted
		{"$a + $b", false},                         // Integer arithmetic
		{"length($s)", false},                      // Integer-returning func
		{"calendarMonthsBetween($d1, $d2)", false}, // Integer-returning func
		{"", false},
	}
	for _, c := range cases {
		if got := SourceRejectedForIntegerTarget(c.src, vars); got != c.want {
			t.Errorf("%q: got %v, want %v", c.src, got, c.want)
		}
	}
}
