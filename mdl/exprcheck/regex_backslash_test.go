// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// ako/mxcli#1397 (mendixlabs/mxcli#1515, item 3): a regular expression that
// ends in a lone backslash builds (mx check 11.13.0: 0 errors) and fails when
// it runs. Measured by calling the Mendix runtime's own StringFunctions
// (10.24.24, 11.13.0, 11.15.0): replaceAll($s, '\', '/') throws
// PatternSyntaxException "Unescaped trailing backslash", as do replaceFirst and
// isMatch; the nanoflow client's `new RegExp('\\')` throws SyntaxError "\ at end
// of pattern". The replacement is NOT a regex on either side — the runtime
// passes it through Matcher.quoteReplacement and the client escapes `$` — so a
// lone backslash there is inserted as written and is not reported.
func TestTrailingBackslashRegexes(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string // the function reported, "" for none
	}{
		{`replaceAll($s, '\', '/')`, "replaceAll"},
		{`replaceAll($s, 'x\', '/')`, "replaceAll"},
		{`replaceAll($s, '\\\', '/')`, "replaceAll"},
		{`replaceFirst($s, '\', '')`, "replaceFirst"},
		{`isMatch($s, 'a\')`, "isMatch"},
		{`toUpperCase(replaceAll($s, '\', '/'))`, "replaceAll"},
		// controls
		{`replaceAll($s, '\\', '/')`, ""},
		{`replaceAll($s, '\\\\', '\\\\')`, ""},
		{`replaceAll($s, '"', '\')`, ""},  // the replacement is literal text
		{`replaceAll($s, '"', '\"')`, ""}, // the issue's JSON escape: builds and runs
		{`replaceAll($s, $Pattern, '/')`, ""},
		{`find($s, '\')`, ""}, // not a regular expression
		{`isMatch($s, '\d+')`, ""},
	} {
		got := TrailingBackslashRegexes(c.src)
		switch {
		case c.want == "" && len(got) > 0:
			t.Errorf("%s: reported %+v, want nothing", c.src, got)
		case c.want != "" && (len(got) != 1 || got[0].Func != c.want):
			t.Errorf("%s: got %+v, want one %s", c.src, got, c.want)
		}
	}
}
