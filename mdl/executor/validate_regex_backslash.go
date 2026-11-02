// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// regexBackslashRule is the rule ID for a regular expression that ends in a
// lone backslash.
const regexBackslashRule = "MDL-REGEX01"

// checkRegexTrailingBackslash warns about a replaceAll, replaceFirst or isMatch
// whose regular expression ends in a lone backslash (ako/mxcli#1397,
// mendixlabs/mxcli#1515). It builds — mxbuild does not read the pattern — and
// fails every time it runs: measured by calling the Mendix runtime's own
// replaceAll (10.24.24, 11.13.0, 11.15.0), it throws
// java.util.regex.PatternSyntaxException "Unescaped trailing backslash"; the
// nanoflow client's RegExp throws "\ at end of pattern". A warning, since the
// build is not what fails.
//
// The replacement is not judged: it is literal text on both sides (the runtime
// applies Matcher.quoteReplacement, the client escapes `$`), so a backslash in
// it is inserted as written — `replaceAll($s, '"', '\"')` puts a backslash
// before every quote, as intended.
//
// src is the expression as stored, so a script without `mdl 1;` is judged after
// its `\\` became one backslash.
func (v *microflowValidator) checkRegexTrailingBackslash(label, src string) {
	for _, h := range exprcheck.TrailingBackslashRegexes(src) {
		v.addViolation(regexBackslashRule, linter.SeverityWarning,
			fmt.Sprintf("%s: the regular expression of %s, '%s', ends in an unescaped backslash — it builds, and "+
				"the call fails each time it runs (java.util.regex.PatternSyntaxException \"Unescaped trailing "+
				"backslash\"; in a nanoflow, \"\\ at end of pattern\")", label, h.Func, h.Pattern),
			"The second argument is a regular expression: a backslash in it escapes the next character, so match "+
				"one backslash with \\\\ ('\\\\' under `mdl 1;`, '\\\\\\\\' without the header, where \\\\ is one backslash). "+
				"The replacement is literal text — a backslash or $ in it is inserted as written.")
	}
}
