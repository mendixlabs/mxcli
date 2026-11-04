// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"regexp"
	"strings"
)

// flowArgsCallRe finds a `microflow M.F(` / `nanoflow M.F(` call on a line: the
// data-source and action forms whose argument lists ANTLR mis-locates.
var flowArgsCallRe = regexp.MustCompile(`(?i)\b(?:microflow|nanoflow)\s+[\w."]+\s*\(`)

var (
	argNoValueRe      = regexp.MustCompile(`^(\$?[\w"]+)\s*[=:]\s*$`)
	argWithValueRe    = regexp.MustCompile(`^\$?[\w"]+\s*[=:]`)
	argMissingEqualRe = regexp.MustCompile(`^[\w"]+\s+\S`)
)

// flowArgumentHint explains a "no viable alternative" that a malformed flow
// argument list caused (ako/mxcli#533). ANTLR predicts a widget property with
// full-LL lookahead, so an error anywhere inside `datasource: microflow
// M.F(Param = )` is reported at `datasource` — a keyword that is correct. The
// grammar cannot move the report without restructuring every widget property,
// so this reads the argument list on the offending line and names the
// argument at fault, with its own line:column. It returns "" when the line has
// no flow call or every argument looks well formed: the error is then
// somewhere else, and a guess would mislead.
func flowArgumentHint(msg, offendingLine string, line int) string {
	if line <= 0 || !strings.Contains(msg, "no viable alternative") {
		return ""
	}
	for _, loc := range flowArgsCallRe.FindAllStringIndex(offendingLine, -1) {
		if col, problem := badFlowArgument(offendingLine, loc[1]); problem != "" {
			return fmt.Sprintf("  The mistake is in the argument list, at line %d:%d: %s.\n"+
				"  Each argument is `Param = <expression>`, separated by commas:\n"+
				"    microflow Module.DS_Orders(Customer = $Customer, Status = 'Open')", line, col, problem)
		}
	}
	return ""
}

// badFlowArgument scans the argument list that opens just before index open
// and returns the column and description of the first malformed argument.
func badFlowArgument(line string, open int) (int, string) {
	depth := 0
	start := open
	var args [][2]int // [start, end) of each argument; end is its delimiter
	closed := false
	inQuote := false
scan:
	for i := open; i < len(line); i++ {
		c := line[i]
		switch {
		case inQuote:
			if c == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
				} else {
					inQuote = false
				}
			}
		case c == '\'':
			inQuote = true
		case c == '(' || c == '[':
			depth++
		case (c == ')' || c == ']') && depth > 0:
			depth--
		case c == ')':
			args = append(args, [2]int{start, i})
			closed = true
			break scan
		case c == ',' && depth == 0:
			args = append(args, [2]int{start, i})
			start = i + 1
		}
	}
	if !closed {
		return 0, "" // runs onto the next line: not judged from one line
	}
	if len(args) == 1 && strings.TrimSpace(line[args[0][0]:args[0][1]]) == "" {
		return 0, "" // `()` — no arguments, which is valid
	}
	for _, a := range args {
		raw := line[a[0]:a[1]]
		text := strings.TrimSpace(raw)
		col := a[0] + (len(raw) - len(strings.TrimLeft(raw, " \t")))
		switch {
		case text == "":
			return a[1], "an empty argument (a stray comma)"
		case argNoValueRe.MatchString(text):
			name := argNoValueRe.FindStringSubmatch(text)[1]
			return col, fmt.Sprintf("`%s` has no value", name)
		case argWithValueRe.MatchString(text):
			continue
		case argMissingEqualRe.MatchString(text):
			return col, fmt.Sprintf("`%s` is missing `=` between the parameter and its value", text)
		}
	}
	return 0, ""
}
