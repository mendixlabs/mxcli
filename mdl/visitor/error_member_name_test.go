// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A member named `Error` followed by the `message` keyword lexed as the one
// compound ERROR_MESSAGE token, so `validation feedback $E/Error message 'x'`
// — exactly what describe writes for such an attribute — failed with "missing
// MESSAGE" (mendixlabs/mxcli#1720). After a `/` or `.` member separator the
// word is a name, never the start of the `error message` clause.
func TestValidationFeedbackOnMemberNamedError(t *testing.T) {
	cases := map[string]struct {
		target   string
		wantLast string
	}{
		"attribute Error":           {"$E/Error", "Error"},
		"attribute error":           {"$E/error", "error"},
		"attribute ERROR":           {"$E/ERROR", "ERROR"},
		"after an association":      {"$E/M.E_Other/Error", "Error"},
		"spaced separator":          {"$E / Error", "Error"},
		"qualified with dot":        {"$E/M.Other.Error", "M.Other.Error"},
		"attribute ErrorMessage":    {"$E/ErrorMessage", "ErrorMessage"},
		"attribute Error_Message":   {"$E/Error_Message", "Error_Message"},
		"control: unrelated member": {"$E/Code", "Code"},
	}
	for name, tc := range cases {
		for _, header := range []string{"", "mdl 1;\n"} {
			t.Run(name+"/"+strings.TrimSpace(header), func(t *testing.T) {
				src := header + "create or modify microflow M.ACT ($E: M.E)\nbegin\n" +
					"  validation feedback " + tc.target + " message 'not good';\n" +
					"  return;\nend;"
				prog, errs := Build(src)
				if len(errs) > 0 {
					t.Fatalf("must parse: %v\n%s", errs, src)
				}
				mf, ok := prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt)
				if !ok {
					t.Fatalf("want a microflow, got %T", prog.Statements[len(prog.Statements)-1])
				}
				vf, ok := mf.Body[0].(*ast.ValidationFeedbackStmt)
				if !ok {
					t.Fatalf("want validation feedback, got %T", mf.Body[0])
				}
				segs := vf.AttributePath.Segments
				if len(segs) == 0 || segs[len(segs)-1].Name != tc.wantLast {
					t.Fatalf("last segment = %v, want %q", segs, tc.wantLast)
				}
				if vf.Message == nil {
					t.Fatal("the message was lost")
				}
			})
		}
	}
}

// The `error message` clause itself must keep lexing as ERROR_MESSAGE wherever
// it follows something other than a member separator — including a block
// comment, whose closing `*/` ends in a slash.
func TestErrorMessageClauseStillLexes(t *testing.T) {
	for name, src := range map[string]string{
		"attribute validation":   "create entity M.E (Name: String(100) not null error message 'Required');",
		"on delete restrict":     "create association M.A_B from M.A to M.B on delete restrict error message 'In use';",
		"after a block comment":  "create association M.A_B from M.A to M.B on delete restrict /* why */ error message 'In use';",
		"after a spaced comment": "create association M.A_B from M.A to M.B on delete restrict /* why */\n  error   message 'In use';",
		"after a line comment":   "create association M.A_B from M.A to M.B on delete restrict -- see the docs.\n  error message 'In use';",
		"after a path comment":   "create association M.A_B from M.A to M.B on delete restrict -- see a/b/\n  error message 'In use';",
	} {
		t.Run(name, func(t *testing.T) {
			if _, errs := Build(src); len(errs) > 0 {
				t.Fatalf("must parse: %v\n%s", errs, src)
			}
		})
	}
}
