// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The repro from mendixlabs/mxcli#1323, verbatim: two Reference retrieves
// followed from the association's FROM entity, so both $Cursor and $Next are
// single G46.Group objects, then `set $Cursor = $Next`. It passed
// `check --references` and `exec`, and mx check answered
// [CE7247] "Variable 'Cursor' does not have a primitive type."
const setObjectRepro = `create module "G46";
create persistent entity "G46"."Group" ("Name": String(50));
create persistent entity "G46"."Node" ("Name": String(50));
create association "G46"."Node_Group" from "G46"."Node" to "G46"."Group" type Reference;
create microflow "G46"."GET_OtherGroup" ($A: "G46"."Node", $B: "G46"."Node") returns "G46"."Group" as $Cursor
begin
  retrieve $Cursor from $A/G46.Node_Group;
  retrieve $Next from $B/G46.Node_Group;
  set $Cursor = $Next;
  return $Cursor;
end;
/
`

// checkScriptMicroflows runs check --references' per-statement validation over
// every microflow in src and returns the joined errors.
func checkScriptMicroflows(t *testing.T, src string) string {
	t.Helper()
	ctx := assocShapeCtx(t)
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	var out []string
	for _, st := range prog.Statements {
		if _, ok := st.(*ast.CreateMicroflowStmt); !ok {
			continue
		}
		if err := validateWithContext(ctx, st, sc); err != nil {
			out = append(out, err.Error())
		}
	}
	return strings.Join(out, "\n")
}

// check --references refuses the #1323 repro, naming the CE7247 it prevents.
func TestCheckRefusesSetOnAssociationRetrievedObject(t *testing.T) {
	got := checkScriptMicroflows(t, setObjectRepro)
	if !strings.Contains(got, "CE7247") || !strings.Contains(got, "$Cursor") {
		t.Fatalf("check accepted `set` on object variable $Cursor; got %q", got)
	}
}

// Controls for the check path: the shapes where the retrieve yields a LIST
// keep passing, because `set` on a list is a Change list Replace (ako/mxcli#949).
func TestCheckAcceptsSetOnAssociationRetrievedList(t *testing.T) {
	for name, mf := range map[string]string{
		// Reference followed from its TO entity: the reverse is a list.
		"reverse Reference": `create microflow C88.M ($X: C88.B, $Y: C88.B)
begin
  retrieve $L from $X/C88.R_def;
  retrieve $M from $Y/C88.R_def;
  set $L = $M;
end;
/
`,
		// ReferenceSet from the FROM entity: a list.
		"ReferenceSet": `create microflow C88.M ($X: C88.A, $Y: C88.A)
begin
  retrieve $L from $X/C88.RS_def;
  retrieve $M from $Y/C88.RS_def;
  set $L = $M;
end;
/
`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := checkScriptMicroflows(t, mf); got != "" {
				t.Errorf("check refused `set` on a list: %s", got)
			}
		})
	}
}

// The same refusal for a stored association, from the project rather than the
// script.
func TestCheckRefusesSetOnObjectVariable(t *testing.T) {
	mf := `create microflow C88.M ($X: C88.A, $Y: C88.A)
begin
  retrieve $O from $X/C88.R_def;
  retrieve $P from $Y/C88.R_def;
  set $O = $P;
end;
/
`
	if got := checkScriptMicroflows(t, mf); !strings.Contains(got, "CE7247") {
		t.Errorf("check accepted `set` on an object variable; got %q", got)
	}
}

// An object plain check can see (a parameter) is MDL-SET01's to report; the
// reference validator, which check --references runs as well, stays quiet on
// it so the author sees the refusal once.
func TestCheckReferencesLeavesVisibleObjectsToMDLSET01(t *testing.T) {
	mf := `create microflow C88.M ($X: C88.A, $Y: C88.A)
begin
  set $X = $Y;
end;
/
`
	if got := checkScriptMicroflows(t, mf); got != "" {
		t.Errorf("reference validator reported what MDL-SET01 owns: %q", got)
	}
	if hits := setObjectRuleHits(t, mf); len(hits) != 1 {
		t.Errorf("want one %s, got %q", setObjectRule, hits)
	}
}

// exec: the builder refuses `set` on an object variable rather than writing a
// Change variable action mxbuild rejects with CE7247.
func TestSetOnObjectVariableRefusedByBuilder(t *testing.T) {
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"Cursor": "G46.Group", "Next": "G46.Group"},
		declaredVars: map[string]string{},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.MfSetStmt{Target: "Cursor", Value: &ast.VariableExpr{Name: "Next"}},
	}, nil)
	if got := strings.Join(fb.errors, "\n"); !strings.Contains(got, "CE7247") || !strings.Contains(got, "$Cursor") {
		t.Fatalf("builder accepted `set` on object variable $Cursor; errors = %q", got)
	}
}

// setObjectRuleHits parses src and returns the MDL-SET01 messages plain check
// (ValidateMicroflow / ValidateNanoflow, no project) reports for it.
func setObjectRuleHits(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var hits []string
	for _, st := range prog.Statements {
		var vs []linter.Violation
		switch s := st.(type) {
		case *ast.CreateMicroflowStmt:
			vs = ValidateMicroflow(s)
		case *ast.CreateNanoflowStmt:
			vs = ValidateNanoflow(s)
		}
		for _, v := range vs {
			if v.RuleID == setObjectRule {
				hits = append(hits, v.Message)
			}
		}
	}
	return hits
}

// Plain check, with no project, refuses `set` on a variable it can see is an
// object without one: the object producers that need no association lookup.
func TestPlainCheckRefusesSetOnObjectVariable(t *testing.T) {
	for name, body := range map[string]string{
		"parameter": `set $A = $B;`,
		"create object": `$N = create M.E (Name = 'x');
  set $N = $B;`,
		"retrieve first": `retrieve $F from M.E first;
  set $F = $B;`,
		"loop iterator": `retrieve $L from M.E;
  loop $It in $L begin
    set $It = $B;
  end loop;`,
		"head": `retrieve $L from M.E;
  $H = head($L);
  set $H = $B;`,
	} {
		t.Run(name, func(t *testing.T) {
			src := "create microflow M.MF ($A: M.E, $B: M.E)\nbegin\n  " + body + "\nend;\n"
			hits := setObjectRuleHits(t, src)
			if len(hits) != 1 || !strings.Contains(hits[0], "CE7247") {
				t.Errorf("want one %s naming CE7247, got %q", setObjectRule, hits)
			}
		})
	}
	nf := "create nanoflow M.NF ($A: M.E, $B: M.E)\nbegin\n  set $A = $B;\nend;\n"
	if hits := setObjectRuleHits(t, nf); len(hits) != 1 {
		t.Errorf("nanoflow: want one %s, got %q", setObjectRule, hits)
	}
}

// Controls: what plain check cannot or must not call an object stays accepted
// — a primitive, a list (Change list Replace, ako/mxcli#949), a member path,
// and an association retrieve, whose cardinality needs the project.
func TestPlainCheckAcceptsSetOnNonObject(t *testing.T) {
	for name, body := range map[string]string{
		"primitive": `declare $N Integer = 0;
  set $N = 1;`,
		"list parameter": `set $Ls = $Ls2;`,
		"list retrieve": `retrieve $L from M.E;
  set $L = $Ls;`,
		"member path": `set $A/Name = 'x';`,
		"association retrieve": `retrieve $C from $A/M.E_Other;
  retrieve $D from $B/M.E_Other;
  set $C = $D;`,
	} {
		t.Run(name, func(t *testing.T) {
			src := "create microflow M.MF ($A: M.E, $B: M.E, $Ls: list of M.E, $Ls2: list of M.E)\nbegin\n  " + body + "\nend;\n"
			if hits := setObjectRuleHits(t, src); len(hits) != 0 {
				t.Errorf("refused a set that is not on a known object: %q", hits)
			}
		})
	}
}

// mxbuild words CE7247 differently for a parameter (measured on 11.14.0), and
// the message quotes what the author will see at build time.
func TestPlainCheckQuotesParameterRejection(t *testing.T) {
	hits := setObjectRuleHits(t, "create microflow M.MF ($A: M.E, $B: M.E)\nbegin\n  set $A = $B;\nend;\n")
	if len(hits) != 1 || !strings.Contains(hits[0], `"Parameter 'A' cannot be changed"`) {
		t.Errorf("want the parameter wording of CE7247, got %q", hits)
	}
}

// A Change variable cannot target ANY parameter: measured on 11.14.0, `set` on
// an Integer or String parameter is CE7247 "Parameter 'N' cannot be changed."
// in a microflow, a nanoflow and a rule. A LIST parameter is not refused —
// `set $L = $M` is a Change list Replace, and mxbuild accepts it.
func TestCheckRefusesSetOnPrimitiveParameter(t *testing.T) {
	for name, src := range map[string]string{
		"microflow Integer": "create microflow M.MF ($N: Integer)\nbegin\n  set $N = 1;\nend;\n",
		"microflow String":  "create microflow M.MF ($S: String)\nbegin\n  set $S = 'x';\nend;\n",
		"nanoflow Integer":  "create nanoflow M.NF ($N: Integer)\nbegin\n  set $N = 1;\nend;\n",
	} {
		t.Run(name, func(t *testing.T) {
			hits := setObjectRuleHits(t, src)
			if len(hits) != 1 || !strings.Contains(hits[0], "cannot be changed") {
				t.Errorf("want one %s quoting \"Parameter '…' cannot be changed\", got %q", setObjectRule, hits)
			}
		})
	}
}

// Controls: the parameter shapes mxbuild accepts.
func TestCheckAcceptsSetOnListParameterAndMembers(t *testing.T) {
	src := `create microflow M.MF ($L: list of M.E, $M: list of M.E, $O: M.E, $N: Integer)
begin
  set $L = $M;
  set $O/Name = 'x';
  declare $Local Integer = $N;
  set $Local = 2;
end;
`
	if hits := setObjectRuleHits(t, src); len(hits) != 0 {
		t.Errorf("refused a set mxbuild accepts: %q", hits)
	}
}

// A rule never goes through ValidateMicroflow: plain check reports MDL-SET01
// from ValidateProgram, and exec refuses it through validateRuleSetTargets.
func TestRuleRefusesSetOnParameter(t *testing.T) {
	src := "create rule M.R ($N: Integer) returns Boolean\nbegin\n  set $N = 1;\n  return true;\nend;\n"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	r := prog.Statements[0].(*ast.CreateRuleStmt)
	if err := validateRuleSetTargets(r.Name.String(), r.Parameters, r.Body); err == nil ||
		!strings.Contains(err.Error(), "cannot be changed") {
		t.Errorf("exec's rule gate accepted `set` on a rule parameter: %v", err)
	}
	var plain []string
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == setObjectRule {
			plain = append(plain, v.Message)
		}
	}
	if len(plain) != 1 {
		t.Errorf("plain check: want one %s for the rule, got %q", setObjectRule, plain)
	}
}

// exec refuses it too: MDL-SET01 is exec-enforced, so a script that skips
// check does not write the Change variable mxbuild rejects.
func TestExecEnforcesSetOnParameter(t *testing.T) {
	prog, errs := visitor.Build("create microflow M.MF ($N: Integer)\nbegin\n  set $N = 1;\nend;\n")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if err := validateMicroflowRules(prog.Statements[0].(*ast.CreateMicroflowStmt)); err == nil ||
		!strings.Contains(err.Error(), setObjectRule) {
		t.Errorf("exec's rule gate let `set` on a parameter through: %v", err)
	}
}

// execSetRefusals builds src's single microflow the way exec does — the flow
// builder, its variable types seeded from the parameters — and returns the
// "cannot set object variable" refusals it reports.
func execSetRefusals(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var mf *ast.CreateMicroflowStmt
	for _, st := range prog.Statements {
		if m, ok := st.(*ast.CreateMicroflowStmt); ok {
			mf = m
		}
	}
	if mf == nil {
		t.Fatalf("no microflow in %q", src)
	}
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
	}
	for _, p := range mf.Parameters {
		switch {
		case p.Type.EntityRef != nil && p.Type.Kind == ast.TypeListOf:
			fb.varTypes[p.Name] = "List of " + p.Type.EntityRef.String()
		case p.Type.EntityRef != nil:
			fb.varTypes[p.Name] = p.Type.EntityRef.String()
		default:
			fb.declaredVars[p.Name] = p.Type.Kind.String()
		}
	}
	fb.buildFlowGraph(mf.Body, nil)
	var out []string
	for _, e := range fb.errors {
		if strings.Contains(e, "cannot set object variable") {
			out = append(out, e)
		}
	}
	return out
}

// The repro from ako/mxcli#2176: an object from a list `find`, reassigned with
// `set` inside an if. exec refused it ("cannot set object variable") and rolled
// the script back; check said "Check passed!" because the text-only typing
// MDL-SET01 runs on did not type a find's output, so the rule never fired.
const setOnFoundObjectRepro = `create module "M";
create or modify non-persistent entity "M"."Line" ( "Model": String(200) );
create or modify microflow "M"."SUB_Repro" ($Lines: List of "M"."Line", $Model: String)
returns String as $R
begin
  $Line = find $Lines by Model = $Model;
  if $Line = empty then
    $New = create "M"."Line" ("Model" = $Model);
    add $New to $Lines;
    set $Line = $New;
  end if;
  return $Line/Model;
end;
`

func TestCheckRefusesSetOnFoundObject_Issue2176(t *testing.T) {
	hits := setObjectRuleHits(t, setOnFoundObjectRepro)
	if len(hits) != 1 || !strings.Contains(hits[0], "cannot set object variable '$Line' (M.Line)") {
		t.Fatalf("plain check: want one %s on $Line, got %q", setObjectRule, hits)
	}
	// check --references runs MDL-SET01 too, and its own validator stays quiet
	// so the author reads the refusal once.
	if got := checkScriptMicroflows(t, setOnFoundObjectRepro); strings.Contains(got, "cannot set object variable") {
		t.Errorf("reference validator reported what MDL-SET01 owns: %q", got)
	}
}

// check and exec answer the same question — is this `set` target an object? —
// for every producer whose type the text states. Each case is run through
// both, and they must agree; the control (no `set`) must pass both.
func TestCheckAndExecAgreeOnSetOnObject_Issue2176(t *testing.T) {
	const head = "create microflow M.MF ($A: M.E, $B: M.E, $Ls: list of M.E, $S: String)\nbegin\n  "
	refused := map[string]string{
		"find by":        "$F = find $Ls by Name = $S;\n  set $F = $B;",
		"find where":     "$F = find $Ls where Name = $S;\n  set $F = $B;",
		"find call":      "$F = find($Ls, Name = $S);\n  set $F = $B;",
		"find in if":     "$F = find $Ls by Name = $S;\n  if $F = empty then\n    set $F = $B;\n  end if;",
		"find of filter": "$G = filter($Ls, Name = $S);\n  $F = find $G by Name = $S;\n  set $F = $B;",
		"retrieve first": "retrieve $F from M.E first;\n  set $F = $B;",
		"parameter":      "set $A = $B;",
		"create object":  "$F = create M.E (Name = $S);\n  set $F = $B;",
		"head":           "$F = head($Ls);\n  set $F = $B;",
	}
	for name, body := range refused {
		t.Run(name, func(t *testing.T) {
			src := head + body + "\nend;\n"
			check := setObjectRuleHits(t, src)
			exec := execSetRefusals(t, src)
			if len(exec) != 1 {
				t.Fatalf("exec: want one refusal, got %q", exec)
			}
			if len(check) != 1 {
				t.Errorf("check and exec disagree: exec refused (%q), check reported %q", exec[0], check)
			}
		})
	}
	accepted := map[string]string{
		"find, no set":     "$F = find $Ls by Name = $S;\n  change $F (Name = 'x');",
		"string find":      "declare $I Integer = 0;\n  $I = find($S, 'x');",
		"list from filter": "$G = filter($Ls, Name = $S);\n  set $G = $Ls;",
	}
	for name, body := range accepted {
		t.Run(name, func(t *testing.T) {
			src := head + body + "\nend;\n"
			if check := setObjectRuleHits(t, src); len(check) != 0 {
				t.Errorf("check refused: %q", check)
			}
			if exec := execSetRefusals(t, src); len(exec) != 0 {
				t.Errorf("exec refused: %q", exec)
			}
		})
	}
}
