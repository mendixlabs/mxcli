// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func nanoflowViolations(t *testing.T, body string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\ncreate nanoflow M.NF_T()\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("parsing:\n%s\nerrors: %v", body, errs)
	}
	return ValidateNanoflow(prog.Statements[len(prog.Statements)-1].(*ast.CreateNanoflowStmt))
}

func hasErrorRule(vs []linter.Violation, rule string) bool {
	for _, v := range vs {
		if v.RuleID == rule && v.Severity == linter.SeverityError {
			return true
		}
	}
	return false
}

// `mxcli check` reported nothing a nanoflow could not hold: the nanoflow rules
// ran only inside exec's build (validateNanoflow), and the annotation rules
// (MDL059/MDL060) ran only for CREATE MICROFLOW. So `call nanoflow … on error
// continue` (mendixlabs/mxcli#591), `show home page` in a nanoflow, and an
// outcome `@curve(true: …)` (mendixlabs/mxcli#992) all passed check — the
// curve one passed exec too, and was dropped. An IF now takes an outcome curve
// (ako/mxcli#2592); on any other statement it is still refused.
func TestValidateNanoflow_ReportsWhatExecWouldRefuse(t *testing.T) {
	for _, tc := range []struct{ name, body, rule string }{
		{"error handling", "call nanoflow M.NF_Sub() on error continue;", nanoflowActivityRule},
		{"disallowed action", "show home page;", nanoflowActivityRule},
		{"outcome curve off an IF", "@curve(true: (from: (15, 0), to: (0, -30)))\nlog info node 'a' 'b';", "MDL060"},
		{"unknown annotation", "@postion(1, 2)\nlog info node 'a' 'b';", "MDL059"},
		{"inside a handler", "call microflow M.MF() on error without rollback begin\n  @curve(true: (from: (1, 0)))\n  log info node 'a' 'b';\nend error;", "MDL060"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if vs := nanoflowViolations(t, tc.body); !hasErrorRule(vs, tc.rule) {
				t.Errorf("check passed a nanoflow exec refuses or drops; want %s, got %+v", tc.rule, vs)
			}
		})
	}
	// CONTROL: a nanoflow with none of these is clean, an IF's outcome curve
	// included.
	for _, body := range []string{
		"@position(100, 100)\n@curve(from: (0, 30), to: (0, -30))\nlog info node 'a' 'b';",
		"@curve(true: (from: (15, 0), to: (0, -30)))\nif true then\n  log info node 'a' 'b';\nend if;",
	} {
		if vs := nanoflowViolations(t, body); hasErrorRule(vs, nanoflowActivityRule) || hasErrorRule(vs, "MDL060") || hasErrorRule(vs, "MDL059") {
			t.Errorf("a valid nanoflow was reported: %+v", vs)
		}
	}
}

// exec refuses MDL060 and MDL059 on a nanoflow as it does on a microflow.
func TestValidateNanoflowRules_RefusesAnIgnoredCurve(t *testing.T) {
	prog, errs := visitor.Build("mdl 1;\ncreate nanoflow M.NF_T()\nbegin\n@curve(true: (from: (15, 0)))\nlog info node 'a' 'b';\nend;")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if err := validateNanoflowRules(prog.Statements[len(prog.Statements)-1].(*ast.CreateNanoflowStmt)); err == nil {
		t.Fatal("exec accepted a @curve it would drop")
	}
}

// A microflow's annotation rules did not enter an error-handler body either.
func TestValidateMicroflow_ChecksAnnotationsInsideAHandler(t *testing.T) {
	prog, errs := visitor.Build("mdl 1;\ncreate microflow M.MF_T()\nbegin\ncall microflow M.MF() on error without rollback begin\n  @postion(1, 2)\n  log info node 'a' 'b';\nend error;\nend;")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if vs := ValidateMicroflow(prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt)); !hasErrorRule(vs, "MDL059") {
		t.Fatalf("an unknown annotation inside a handler passed: %+v", vs)
	}
}

// mendixlabs/mxcli#992: an @anchor parameter the visitor cannot use was
// skipped, and the edge kept the builder's default sides. Refused at check and
// exec, on a microflow and a nanoflow alike.
func TestAnchorParameterItCannotUseIsRefused(t *testing.T) {
	const body = "@anchor(true: (to: middle))\nif true then\n  log info node 'a' 'b';\nend if;"
	prog, errs := visitor.Build("mdl 1;\ncreate microflow M.MF_T()\nbegin\n" + body + "\nend;\ncreate nanoflow M.NF_T()\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	n := len(prog.Statements)
	mf := prog.Statements[n-2].(*ast.CreateMicroflowStmt)
	nf := prog.Statements[n-1].(*ast.CreateNanoflowStmt)
	if vs := ValidateMicroflow(mf); !hasErrorRule(vs, invalidAnchorRule) {
		t.Errorf("microflow: check passed an @anchor it drops: %+v", vs)
	}
	if vs := ValidateNanoflow(nf); !hasErrorRule(vs, invalidAnchorRule) {
		t.Errorf("nanoflow: check passed an @anchor it drops: %+v", vs)
	}
	if err := validateMicroflowRules(mf); err == nil {
		t.Error("microflow: exec accepted an @anchor it drops")
	}
	if err := validateNanoflowRules(nf); err == nil {
		t.Error("nanoflow: exec accepted an @anchor it drops")
	}
	// CONTROL: the one-sided per-case form is valid.
	prog, _ = visitor.Build("mdl 1;\ncreate nanoflow M.NF_T()\nbegin\n@anchor(true: (to: top))\nif true then\n  log info node 'a' 'b';\nend if;\nend;")
	if vs := ValidateNanoflow(prog.Statements[len(prog.Statements)-1].(*ast.CreateNanoflowStmt)); hasErrorRule(vs, invalidAnchorRule) {
		t.Errorf("a valid per-case @anchor was refused: %+v", vs)
	}
}
