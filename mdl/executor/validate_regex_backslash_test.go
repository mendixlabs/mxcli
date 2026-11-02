// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#1397 (mendixlabs/mxcli#1515, item 3): a replaceAll whose regular
// expression ends in a lone backslash passed check, exec and mx check (11.13.0:
// 0 errors) and failed at run time — the runtime's own replaceAll throws
// PatternSyntaxException "Unescaped trailing backslash". It is a warning: the
// build accepts it. What is judged is the STORED text, so the same literal is
// '\' under `mdl 1;` and '\\' without the header (where `\\` is one backslash).
func TestRegexTrailingBackslash(t *testing.T) {
	for _, c := range []struct {
		name, src string
		warn      bool
	}{
		{"mdl 1, lone backslash pattern",
			"mdl 1;\ncreate microflow M.P ($s: String) returns String\nbegin\n  return replaceAll($s, '\\', '/');\nend;\n", true},
		{"mdl 0, '\\\\' is one stored backslash",
			"create microflow M.P ($s: String) returns String\nbegin\n  return replaceAll($s, '\\\\', '/');\nend;\n", true},
		{"nanoflow, isMatch in a declare",
			"mdl 1;\ncreate nanoflow M.P ($s: String) returns Boolean\nbegin\n  declare $ok Boolean = isMatch($s, 'a\\');\n  return $ok;\nend;\n", true},
		// controls
		{"mdl 1, an escaped backslash",
			"mdl 1;\ncreate microflow M.P ($s: String) returns String\nbegin\n  return replaceAll($s, '\\\\', '\\\\');\nend;\n", false},
		{"mdl 1, the replacement is literal text",
			"mdl 1;\ncreate microflow M.P ($s: String) returns String\nbegin\n  return replaceAll($s, '\"', '\\');\nend;\n", false},
		{"the issue's JSON escape builds and runs",
			"mdl 1;\ncreate microflow M.P ($s: String) returns String\nbegin\n  return replaceAll($s, '\"', '\\\"');\nend;\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []linter.Violation
			for _, v := range ValidateProgram(parseMDL(t, c.src), "") {
				if v.RuleID == regexBackslashRule {
					got = append(got, v)
				}
			}
			if (len(got) > 0) != c.warn {
				t.Fatalf("%s reported = %v, want %v: %v", regexBackslashRule, len(got) > 0, c.warn, got)
			}
			if c.warn {
				if got[0].Severity != linter.SeverityWarning {
					t.Errorf("severity = %v, want a warning: the build accepts it", got[0].Severity)
				}
				if !strings.Contains(got[0].Message, "Unescaped trailing backslash") {
					t.Errorf("message should name the runtime failure: %s", got[0].Message)
				}
			}
		})
	}
}
