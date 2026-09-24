// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `drop demo user` and `drop user role` had no IF EXISTS form, so a one-time
// cleanup of what `mxcli new` ships either broke every later run of its slice
// script or had to be commented out — which is what a real project did
// (ako/CapTrackV4 R5, 024). Re-running every mdlsource/*.mdl in order is what a
// fresh clone does, so a statement that only works once is a script that only
// works for whoever wrote it.
//
// The spelling is the one ALTER ENTITY already uses (DROP ATTRIBUTE IF EXISTS),
// so the grammar's own ifExists rule is reused rather than a second spelling
// invented.

func TestDropUserRole_IfExistsIsParsed(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"drop user role if exists Admin;", true},
		{"drop user role Admin;", false},
		{"drop user role if exists 'Admin';", true},
	} {
		prog, errs := Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.src, errs[0])
		}
		if len(prog.Statements) != 1 {
			t.Fatalf("%s: got %d statements", tc.src, len(prog.Statements))
		}
		s, ok := prog.Statements[0].(*ast.DropUserRoleStmt)
		if !ok {
			t.Fatalf("%s: got %T", tc.src, prog.Statements[0])
		}
		if s.IfExists != tc.want {
			t.Errorf("%s: IfExists = %v, want %v", tc.src, s.IfExists, tc.want)
		}
		if s.Name != "Admin" {
			t.Errorf("%s: Name = %q, want Admin — IF EXISTS must not be eaten as the name",
				tc.src, s.Name)
		}
	}
}

func TestDropDemoUser_IfExistsIsParsed(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"drop demo user if exists 'demo';", true},
		{"drop demo user 'demo';", false},
	} {
		prog, errs := Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.src, errs[0])
		}
		s, ok := prog.Statements[0].(*ast.DropDemoUserStmt)
		if !ok {
			t.Fatalf("%s: got %T", tc.src, prog.Statements[0])
		}
		if s.IfExists != tc.want {
			t.Errorf("%s: IfExists = %v, want %v", tc.src, s.IfExists, tc.want)
		}
		if s.UserName != "demo" {
			t.Errorf("%s: UserName = %q, want demo", tc.src, s.UserName)
		}
	}
}

// Document DROPs take IF EXISTS too, so a script that removes a layout or a page
// can run a second time (mendixlabs/mxcli#1190). One case per kind: the flag is
// set, and IF EXISTS is not read as part of the name.
func TestDropDocument_IfExistsIsParsed(t *testing.T) {
	for _, kind := range []string{
		"entity", "association", "enumeration", "constant", "microflow", "nanoflow",
		"page", "layout", "snippet", "menu", "java action", "image collection",
	} {
		for _, withIf := range []bool{true, false} {
			src := "drop " + kind + " M.Doc;"
			if withIf {
				src = "drop " + kind + " if exists M.Doc;"
			}
			prog, errs := Build(src)
			if len(errs) > 0 {
				t.Fatalf("%s: parse: %v", src, errs[0])
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("%s: got %d statements", src, len(prog.Statements))
			}
			skipper, ok := prog.Statements[0].(ast.MissingSkipper)
			if !ok {
				t.Fatalf("%s: %T does not take IF EXISTS", src, prog.Statements[0])
			}
			if skipper.SkipsMissing() != withIf {
				t.Errorf("%s: SkipsMissing = %v, want %v", src, skipper.SkipsMissing(), withIf)
			}
		}
	}
}
