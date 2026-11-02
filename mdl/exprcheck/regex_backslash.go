// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "strings"

// regexArgument is, per built-in, the position of its regular-expression
// argument. Only that argument is a regex: the replacement of replaceAll and
// replaceFirst is literal text. The server runtime passes it through
// java.util.regex.Matcher.quoteReplacement before String.replaceAll, and the
// nanoflow client escapes `$` before String.replace — so a backslash or a `$`
// in it is inserted as written (measured on the 10.24.24, 11.13.0 and 11.15.0
// runtimes and the 11.13.0 client, ako/mxcli#1397).
var regexArgument = map[string]int{
	"replaceAll":   1,
	"replaceFirst": 1,
	"isMatch":      1,
}

// TrailingBackslash is a call whose regular expression is a string literal that
// ends in an unescaped backslash.
type TrailingBackslash struct {
	Func    string // replaceAll, replaceFirst or isMatch
	Pattern string // the literal's text, as stored in the expression
	Line    int
	Column  int
}

// TrailingBackslashRegexes returns every call in src whose regular expression
// is a string literal ending in an odd number of backslashes.
//
// mxbuild does not validate the pattern (mx check 11.13.0 builds
// `replaceAll($s, '\', '/')` at 0 errors), and the call fails each time it
// runs: the runtime throws PatternSyntaxException "Unescaped trailing
// backslash", the nanoflow client a SyntaxError "\ at end of pattern". A
// backslash anywhere else escapes the character after it, which is a regex
// matter this does not judge.
func TrailingBackslashRegexes(src string) []TrailingBackslash {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	root, _ := (&parserImpl{}).Parse(src, Context{})
	if root == nil {
		return nil
	}
	var out []TrailingBackslash
	walkCalls(root, func(c *CallExpr) {
		pos, ok := regexArgument[c.Name]
		if !ok || c.Qualified || pos >= len(c.Args) {
			return
		}
		lit, ok := c.Args[pos].(*StringLit)
		if !ok {
			return
		}
		pattern := strings.ReplaceAll(lit.Value, "''", "'")
		n := len(pattern) - len(strings.TrimRight(pattern, `\`))
		if n%2 == 1 {
			out = append(out, TrailingBackslash{Func: c.Name, Pattern: pattern, Line: c.Pos().Line, Column: c.Pos().Column})
		}
	})
	return out
}
