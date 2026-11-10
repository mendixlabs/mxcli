// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func compileOne(t *testing.T, src string) (*execProgram, string) {
	t.Helper()
	tests, err := parseMDLTests(src, "t.test.mdl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("got %d tests, want 1", len(tests))
	}
	return compileForExec(tests[0])
}

// The supported statements compile to steps in order, expressions in Mendix's
// grammar, and the @expect lines with their actual-value expressions.
func TestCompileForExec_SupportedBody(t *testing.T) {
	p, reason := compileOne(t, `mdl 1;
/**
 * @test Paying an invoice marks it paid
 * @expect $inv/Status = Spike.Status.PAID
 * @expect $n = 1
 */
$cust = create Spike.Customer (Name = 'Acme');
$inv = create Spike.Invoice (InvoiceNo = 'INV-1', Amount = 250.5, Spike.Invoice_Customer = $cust);
commit $inv;
declare $n Integer = 0;
call microflow Spike.Invoice_Pay(Invoice = $inv);
retrieve $paid from Spike.Invoice where Status = Spike.Status.PAID and InvoiceNo = $inv/InvoiceNo;
set $n = 1;
/
`)
	if reason != "" {
		t.Fatalf("not interpretable: %s", reason)
	}
	var ops []string
	for _, s := range p.Steps {
		ops = append(ops, s.Op)
	}
	if want := []string{"create", "create", "commit", "declare", "call", "retrieve", "set"}; !reflect.DeepEqual(ops, want) {
		t.Fatalf("ops = %v, want %v", ops, want)
	}
	inv := p.Steps[1]
	if inv.Entity != "Spike.Invoice" || len(inv.Members) != 3 || inv.Members[2].Name != "Spike.Invoice_Customer" ||
		inv.Members[2].Expr != "$cust" || inv.Members[0].Expr != "'INV-1'" {
		t.Errorf("create step = %+v", inv)
	}
	call := p.Steps[4]
	if call.Flow != "Spike.Invoice_Pay" || len(call.Args) != 1 || call.Args[0].Name != "Invoice" || call.Args[0].Expr != "$inv" {
		t.Errorf("call step = %+v", call)
	}
	ret := p.Steps[5]
	if !strings.HasPrefix(ret.XPath, "//Spike.Invoice[") || !strings.Contains(ret.XPath, "$inv/InvoiceNo") {
		t.Errorf("retrieve xpath = %q", ret.XPath)
	}
	if len(p.Expects) != 2 || p.Expects[0].Condition != "$inv/Status = Spike.Status.PAID" || p.Expects[0].Actual == "" {
		t.Errorf("expects = %+v", p.Expects)
	}
	if !p.Rollback || p.Throws != nil || p.V != execProtocolVersion {
		t.Errorf("program flags = %+v", p)
	}
	if _, err := json.Marshal(p); err != nil {
		t.Fatal(err)
	}
}

// A test outside the supported set takes the microflow route, with a reason
// that names what kept it there.
func TestCompileForExec_RoutesTheRestToMicroflows(t *testing.T) {
	for _, c := range []struct{ body, reason string }{
		{"if 1 = 1 then\n  declare $x Integer = 1;\nend if;", "the statement if"},
		{"declare $list List of M.E = empty;\nloop $i in $list begin\nend loop;", "the statement loop"},
		{"$r = call microflow M.F() on error continue;", "ON ERROR"},
		{"retrieve $l from M.E sort by Name asc;", "sort"},
		{"$o = create M.E;\nretrieve $l from $o/M.Assoc where Name = 'x';", "over an association with where"},
		{"retrieve $l from M.E;\n$f = find $l by Name = 'x';", "list operation (find)"},
		{"$b = call java action M.J();", "the statement call java action"},
		{"set $x = 1;", "$x is read before anything declares it"},
		{"$r = call microflow M.F(A = $missing);", "$missing is read before"},
	} {
		_, reason := compileOne(t, "mdl 1;\n/**\n * @test t\n * @expect true\n */\n"+c.body+"\n/\n")
		if reason == "" || !strings.Contains(reason, c.reason) {
			t.Errorf("%q: reason = %q, want it to mention %q", c.body, reason, c.reason)
		}
	}
}

// An @expect that reads an undeclared variable is refused here: the runtime's
// evaluator would read it as empty and could report a pass.
func TestCompileForExec_ExpectReadingAnUndeclaredVariable(t *testing.T) {
	_, reason := compileOne(t, "mdl 1;\n/**\n * @test t\n * @expect $reslt = 3\n */\n$result = call microflow M.F();\n/\n")
	if !strings.Contains(reason, "$reslt") {
		t.Errorf("reason = %q, want it to name $reslt", reason)
	}
}

func TestCompileForExec_ThrowsSetupAndCleanup(t *testing.T) {
	p, reason := compileOne(t, `mdl 1;
/**
 * @test Negative amount is refused
 * @setup M.Seed
 * @throws 'must be positive'
 * @cleanup none
 */
$r = call microflow M.Apply(Amount = -5.0);
/
`)
	if reason != "" {
		t.Fatalf("not interpretable: %s", reason)
	}
	if p.Throws == nil || p.Throws.Want != "must be positive" || len(p.Expects) != 0 {
		t.Errorf("throws = %+v, expects = %+v", p.Throws, p.Expects)
	}
	if !reflect.DeepEqual(p.Setups, []string{"M.Seed"}) || p.Rollback {
		t.Errorf("setups = %v, rollback = %v", p.Setups, p.Rollback)
	}
}

// A count(...) in an @expect becomes a count step after the body, as the
// generated microflow's aggregate activity does.
func TestCompileForExec_CountAggregate(t *testing.T) {
	p, reason := compileOne(t, "mdl 1;\n/**\n * @test t\n * @expect count($l) = 2\n */\nretrieve $l from M.E;\n/\n")
	if reason != "" {
		t.Fatalf("not interpretable: %s", reason)
	}
	last := p.Steps[len(p.Steps)-1]
	if last.Op != "count" || last.Of != "l" || last.Var == "" || !strings.Contains(p.Expects[0].Condition, "$"+last.Var) {
		t.Errorf("count step = %+v, condition = %q", last, p.Expects[0].Condition)
	}
}

func TestExpressionVariables_SkipsStringLiterals(t *testing.T) {
	got := expressionVariables("$a/Name + '$notavar it''s $x' + $b_2 + toString($c)")
	if want := []string{"a", "b_2", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A watching dev loop applies a model change itself, so the hint is to wait,
// not to restart; a loop that is not watching has to be restarted.
func TestStaleModelError(t *testing.T) {
	built := time.Unix(1000, 0)
	if err := staleModelError("/p/App.mpr", built, built, true); err != nil {
		t.Errorf("an unchanged model was refused: %v", err)
	}
	err := staleModelError("/p/App.mpr", built.Add(5*time.Second), built, true)
	if err == nil || !strings.Contains(err.Error(), "mxcli run wait -p /p/App.mpr") || strings.Contains(err.Error(), "restart") {
		t.Errorf("watching: %v", err)
	}
	err = staleModelError("/p/App.mpr", built.Add(5*time.Second), built, false)
	if err == nil || !strings.Contains(err.Error(), "mxcli run restart -p /p/App.mpr") {
		t.Errorf("not watching: %v", err)
	}
}

// The executor's Java is embedded in an MDL dollar-quoted literal, which ends
// at the first doubled dollar sign; and it reads the step format version this
// compiler writes.
func TestEndpointExecJava(t *testing.T) {
	if strings.Contains(endpointJava, "$$") {
		t.Error("the endpoint Java contains $$, which ends the MDL literal it is embedded in")
	}
	if execProtocolVersionJava != strconv.Itoa(execProtocolVersion) {
		t.Errorf("Java reads step format %s, the compiler writes %d", execProtocolVersionJava, execProtocolVersion)
	}
	for _, want := range []string{`"` + endpointExecRoute + `".equals(path)`, endpointAllowExecEnv, "evaluateExpression"} {
		if !strings.Contains(GenerateEndpointMDL(""), want) {
			t.Errorf("the endpoint script does not contain %q", want)
		}
	}
}

// Aggregates written as statements and a retrieve over an association compile,
// so a suite of ordinary assertion helpers stays on the executor.
func TestCompileForExec_AggregatesAndAssociationRetrieve(t *testing.T) {
	p, reason := compileOne(t, `mdl 1;
/**
 * @test t
 * @expect $n = 2 and $total > 0
 */
retrieve $rows from M.Line;
$n = count $rows;
$total = sum $rows by Amount;
$o = create M.Order;
retrieve $lines from $o/M.Line_Order;
/
`)
	if reason != "" {
		t.Fatalf("not interpretable: %s", reason)
	}
	n, total, lines := p.Steps[1], p.Steps[2], p.Steps[4]
	if n.Op != "aggregate" || n.Agg != "count" || n.Of != "rows" || n.Var != "n" {
		t.Errorf("count step = %+v", n)
	}
	if total.Op != "aggregate" || total.Agg != "sum" || total.Attr != "Amount" {
		t.Errorf("sum step = %+v", total)
	}
	if lines.Op != "retrieve" || lines.Assoc != "M.Line_Order" || lines.From != "o" || lines.XPath != "" {
		t.Errorf("association retrieve step = %+v", lines)
	}
}

// One test the executor cannot run sends only that test through inject and
// build; the others still run in the app.
func TestCompileSuiteForExec_RoutesPerTest(t *testing.T) {
	tests, err := parseMDLTests(`mdl 1;
/**
 * @test fast
 * @expect $n = 1
 */
declare $n Integer = 1;
/

/**
 * @test slow
 * @expect $n = 1
 */
declare $n Integer = 0;
if true then
  set $n = 1;
end if;
/
`, "t.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	suite := &TestSuite{Name: "s", Tests: tests}
	programs, needBuild := compileSuiteForExec(suite)
	if len(programs) != 1 || programs[tests[0].ID] == nil {
		t.Errorf("programs = %v, want only %s", programs, tests[0].ID)
	}
	if !strings.Contains(needBuild[tests[1].ID], "the statement if") || len(needBuild) != 1 {
		t.Errorf("needBuild = %v", needBuild)
	}
	inject := suiteWith(suite, func(tc TestCase) bool { return programs[tc.ID] == nil })
	if len(inject.Tests) != 1 || inject.Tests[0].Name != "slow" || len(suite.Tests) != 2 {
		t.Errorf("injected %d test(s) (%v), suite now holds %d", len(inject.Tests), inject.Tests, len(suite.Tests))
	}
}
