// SPDX-License-Identifier: Apache-2.0

package texttemplate

import (
	"reflect"
	"strings"
	"testing"
)

// Every row was measured with `mx check -j` on mxbuild 10.24.24, 11.13.0 and
// 11.15.0 (ako/mxcli#1397, mendixlabs/mxcli#1515): one document per text, in
// each of the slots that store one — log message (microflow and nanoflow),
// show message, validation feedback, REST call URL and custom body, a Java
// action's StringTemplate argument, send email subject and body (11.13+),
// workflow display (11.x), dynamic text content and action button caption.
// Every slot gave the same answer for every row on every version, which is
// what makes one grammar for all of them sound. want is "" where mxbuild
// reports nothing.
var measured = []struct {
	name   string
	text   string
	params int
	want   string
}{
	// --- the reported shapes -------------------------------------------------
	{"a name in braces (the log report)", "Item {number} not found", 0, "CE0712"},
	{"an empty pair (the REST body report)", "a {} b", 0, "CE0712"},
	{"a letter", "{x}", 0, "CE0712"},
	{"an unescaped JSON object", `{"a": {1}}`, 1, "CE0712"},
	// --- what a brace may be followed by ---------------------------------------
	{"a space before the index", "{ 1 }", 1, "CE0712"},
	{"a space", "{ }", 0, "CE0712"},
	{"a sign", "{-1}", 1, "CE0712"},
	{"a plus sign", "{+1}", 1, "CE0712"},
	{"a tab", "{\t1}", 1, "CE0712"},
	{"a closing brace", "{}}", 0, "CE0712"},
	{"three opening braces", "a {{{ b", 0, "CE0712"},
	{"two bad placeholders: one error", "{x} {y}", 0, "CE0712"},
	{"digits and a space", "{1 }", 1, "CE0714"},
	{"digits and a letter", "{1x}", 1, "CE0714"},
	{"digits and a line break", "{1\n}", 1, "CE0714"},
	{"digits at the end", "{1", 1, "CE0713"},
	{"index zero", "{0}", 1, "CE0716"},
	{"index zero, two digits", "{00}", 1, "CE0716"},
	{"past int32", "{2147483648}", 1, "CE0717"},
	{"far past int32", "{99999999999}", 1, "CE0717"},
	{"a non-ASCII digit", "{١}", 1, "CE0717"},
	// --- placeholders against parameters ---------------------------------------
	{"no parameter", "{1}", 0, "CE0720"},
	{"beyond the parameters", "{1} {3}", 2, "CE0720"},
	{"a large index", "{1000}", 1, "CE0720"},
	{"int32 max", "{2147483647}", 1, "CE0720"},
	{"int32 max - 1", "{2147483646}", 1, "CE0720"},
	// --- the first problem in the text wins; counts are judged after -----------
	{"over, then bad", "{2} {x}", 1, "CE0712"},
	{"bad, then over", "{x} {2}", 1, "CE0712"},
	{"zero, then over", "{0} {2}", 1, "CE0716"},
	{"over, then zero", "{2} {0}", 1, "CE0716"},
	{"zero, then bad", "{0} {x}", 1, "CE0716"},
	{"bad, then zero", "{x} {0}", 1, "CE0712"},
	{"too big, then bad", "{99999999999} {x}", 1, "CE0717"},
	{"bad, then too big", "{x} {99999999999}", 1, "CE0712"},
	{"too big, then zero", "{99999999999} {0}", 1, "CE0717"},
	{"zero, then too big", "{0} {99999999999}", 1, "CE0716"},
	{"bad, then unterminated", "{x} {1", 1, "CE0712"},
	// --- accepted ----------------------------------------------------------------
	{"filled", "{1}", 1, ""},
	{"a lone closing brace", "a } b", 0, ""},
	{"an escaped opening brace", "a {{ b", 0, ""},
	{"two closing braces", "a }} b", 0, ""},
	{"an opening brace at the very end", "abc {", 0, ""},
	{"an escaped placeholder", "{{1}}", 0, ""},
	{"a leading zero", "{01}", 1, ""},
	{"an escaped JSON object", `{{"a": {1}}}`, 1, ""},
	{"the JSON body the REST docs show", `{{"a": "{1}"}`, 1, ""},
	{"a placeholder in escaped braces", "{{{1}}}", 1, ""},
	{"out of order", "{2} {1}", 2, ""},
	{"an unused parameter", "{2}", 2, ""},
	{"close, then open", "} {", 0, ""},
	{"a placeholder, then a closing brace", "{1}}", 1, ""},
	{"an escaped brace, then a closing one", "{{}", 0, ""},
	{"a placeholder, then a brace at the end", "{1}{", 1, ""},
	{"three opening braces at the end", "a{{{", 0, ""},
	{"no braces", "plain text", 0, ""},
	{"empty", "", 0, ""},
}

func TestCheck_MeasuredAgainstMxbuild(t *testing.T) {
	for _, c := range measured {
		t.Run(c.name, func(t *testing.T) {
			p := Check(c.text, c.params)
			got := ""
			if p != nil {
				got = p.Code
			}
			if got != c.want {
				t.Errorf("Check(%q, %d) = %q, want %q (%+v)", c.text, c.params, got, c.want, p)
			}
		})
	}
}

// The message is mxbuild's own wording, so a user who has seen the build error
// recognises it, and the index and count are filled in as mxbuild fills them.
func TestCheck_MessagesAreMxbuilds(t *testing.T) {
	for _, c := range []struct {
		text   string
		params int
		want   string
	}{
		{"{x}", 0, "Brace should be followed by a number of digits or another brace."},
		{"{1", 1, "Missing close brace at end of string."},
		{"{1x}", 1, "Placeholder should end in closing brace."},
		{"{0}", 1, "Place holder indices start at one (1)."},
		{"{2147483648}", 1, "Place holder index 2147483648 too big."},
		{"{1} {3}", 2, "Place holder index 3 is greater than 2, the number of parameter(s)."},
	} {
		p := Check(c.text, c.params)
		if p == nil || p.Message != c.want {
			t.Errorf("Check(%q, %d) = %+v, want message %q", c.text, c.params, p, c.want)
		}
	}
}

// At is the text from the offending brace, so the message can show which brace
// it means in a long body.
func TestCheck_PointsAtTheBrace(t *testing.T) {
	p := Check(`{{"id": 1, "name": {name}}}`, 0)
	if p == nil || !strings.HasPrefix(p.At, "{name}") {
		t.Fatalf("At = %+v, want it to start at {name}", p)
	}
	if p := Check(`{"a": "{1}"}`, 1); p == nil || p.At != `{"a": "…` {
		t.Errorf("a JSON object's brace should be cut before the placeholder inside it, got %+v", p)
	}
	if p := Check("{1} {2} {3}", 1); p == nil || p.At != "{3}" {
		t.Errorf("CE0720 should point at the highest placeholder, got %+v", p)
	}
}

// Placeholders is what the CE0720 checks of other slots count: an escaped brace
// is not a placeholder, which a `\{(\d+)\}` regexp gets wrong — `{{1}}` is the
// literal text {1}, and mxbuild accepts it with no parameters.
func TestPlaceholders(t *testing.T) {
	for _, c := range []struct {
		text string
		want []int
	}{
		{"{1} and {2}", []int{1, 2}},
		{"{{1}}", nil},
		{"{{{1}}}", []int{1}},
		{"{01} {1}", []int{1, 1}},
		{"{x} {1}", nil}, // not a template mxbuild accepts: no placeholders are claimed
	} {
		got, _ := Placeholders(c.text)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Placeholders(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestMaxPlaceholder(t *testing.T) {
	if got := MaxPlaceholder("{{1}} {3} {2}"); got != 3 {
		t.Errorf("MaxPlaceholder = %d, want 3", got)
	}
	if got := MaxPlaceholder("{{7}}"); got != 0 {
		t.Errorf("MaxPlaceholder of an escaped index = %d, want 0", got)
	}
}
