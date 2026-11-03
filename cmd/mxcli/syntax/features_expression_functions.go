// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// The function reference is rendered from the type checker's own table rather
// than written by hand. A hand-kept list had drifted: isNew, the add*/subtract*
// family and the trimTo* functions were all accepted by `check` and named
// nowhere in `mxcli syntax` (mendixlabs/mxcli#1708). Rendering it means a row
// added to exprcheck is documented in the same change, with the types the
// checker enforces.
func init() {
	Register(SyntaxFeature{
		Path:    "microflow.expressions.functions",
		Summary: "Every built-in expression function, with argument and result types",
		Keywords: []string{
			"function", "functions", "built-in", "builtin", "expression function",
			"isNew", "addDays", "addMonths", "addYears", "subtractDays", "trimToDays",
			"trimToMonths", "beginOfDay", "endOfMonth", "dateTime", "formatDateTime",
			"parseDateTime", "formatDecimal", "parseInteger", "toString", "getCaption",
			"date functions", "string functions", "math functions",
		},
		Syntax:  renderFunctionReference(exprcheck.Functions()),
		Example: "mdl 1;\ncreate microflow Mod.DueDate ($Order: Mod.Order) returns DateTime\nbegin\n  if isNew($Order) then\n    return addMonths(trimToDays([%CurrentDateTime%]), 1);\n  end if;\n  return endOfMonth($Order/OrderDate);\nend;",
		SeeAlso: []string{"microflow.expressions", "microflow.variables"},
	})
}

var functionGroups = []struct {
	title string
	match func(exprcheck.FunctionSignature) bool
}{
	{"Date and time", func(f exprcheck.FunctionSignature) bool {
		return f.Return == "DateTime" || (len(f.Params) > 0 && f.Params[0] == "DateTime")
	}},
	{"Objects and enumerations", func(f exprcheck.FunctionSignature) bool {
		return len(f.Params) > 0 && f.Params[0] == "Any" && f.Name != "toString"
	}},
	{"Strings and conversion", func(f exprcheck.FunctionSignature) bool {
		return f.Return == "String" || (len(f.Params) > 0 && f.Params[0] == "String")
	}},
	{"Numbers", func(f exprcheck.FunctionSignature) bool { return true }},
}

func renderFunctionReference(fns []exprcheck.FunctionSignature) string {
	var b strings.Builder
	b.WriteString("-- Built-in functions accepted by `mxcli check`, with the types it\n" +
		"-- enforces: a wrong argument kind is E005 (CE0117 at build), a wrong\n" +
		"-- argument count E006, an unknown name MDL044. [x] marks an optional\n" +
		"-- argument; *UTC variants use the UTC calendar instead of the user's.\n" +
		"-- The current time is the token [%CurrentDateTime%], not a function.\n" +
		"-- There are no hour(), dayOfWeek(), year() or month() functions: format the\n" +
		"-- date instead, e.g. parseInteger(formatDateTime($d, 'yyyy')).\n")
	done := map[string]bool{}
	for _, g := range functionGroups {
		var lines []string
		for _, f := range fns {
			if done[f.Name] || !g.match(f) {
				continue
			}
			done[f.Name] = true
			line := "  " + f.String()
			if f.ReturnNote != "" {
				line += "   -- " + f.ReturnNote
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			continue
		}
		b.WriteString("\n-- " + g.title + "\n")
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteByte('\n')
	}
	b.WriteString("\n-- *Between returns the ABSOLUTE difference, as a Decimal (calendar*Between\n" +
		"-- an Integer, millisecondsBetween a Long): see microflow.expressions.\n")
	return b.String()
}
