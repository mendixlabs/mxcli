// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// TestSetTargetQuotedMemberNormalized guards the "quoted association SET
// corrupts the .mpr" bug: a quoted member/association name on a SET target must
// be unquoted when captured, otherwise the quotes flow verbatim into the Change
// activity's member identifier and produce an invalid AttributeIdentifier that
// passes `mxcli check` but fails to load in MxBuild/Studio Pro.
func TestSetTargetQuotedMemberNormalized(t *testing.T) {
	cases := []struct {
		name string
		set  string
		want string
	}{
		{"quoted association", `set $SkillProfile/BuildScheduling."SkillProfile_Resource" = $Resource;`, "$SkillProfile/BuildScheduling.SkillProfile_Resource"},
		{"unquoted association", `set $SkillProfile/BuildScheduling.SkillProfile_Resource = $Resource;`, "$SkillProfile/BuildScheduling.SkillProfile_Resource"},
		{"quoted bare attribute", `set $Order/"Price" = 5;`, "$Order/Price"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.ACT ($SkillProfile: M.S, $Resource: M.R, $Order: M.O)\nbegin\n  " + tc.set + "\nend;"
			prog, errs := Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			var got string
			for _, s := range prog.Statements {
				cm, ok := s.(*ast.CreateMicroflowStmt)
				if !ok {
					continue
				}
				for _, st := range cm.Body {
					if set, ok := st.(*ast.MfSetStmt); ok {
						got = set.Target
					}
				}
			}
			if got != tc.want {
				t.Errorf("SET target = %q, want %q (quotes must be stripped)", got, tc.want)
			}
		})
	}
}
