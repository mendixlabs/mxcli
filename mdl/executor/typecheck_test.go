// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// typeCheckFixture copies the shared fixture project into a temp dir, connects an
// executor to it for writing, and seeds an enumeration plus an entity that uses
// it.
//
// A real project rather than a mock: the whole point of this path is that the
// catalog answers questions about a model on disk, and every one of the three
// defects found while building it (a missing column, an absent table, an
// expression whose source text the walk never saw) would have passed a mocked
// test.
func typeCheckFixture(t *testing.T) *Executor {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../testdata/expr-checker"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	proj := filepath.Join(dst, "minimal.mpr")

	exec := New(&bytes.Buffer{})
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })

	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(proj)+"'")
	run(t, exec, `CREATE ENUMERATION MyFirstModule.OrderStatus (Open 'Open', Closed 'Closed');`)
	run(t, exec, `CREATE PERSISTENT ENTITY MyFirstModule.Ticket (
		Title: String(100),
		Status: Enumeration(MyFirstModule.OrderStatus)
	);`)
	run(t, exec, `CREATE PERSISTENT ENTITY MyFirstModule.Reporter (Email: String(200));`)
	run(t, exec, `CREATE ASSOCIATION MyFirstModule.Ticket_Reporter
		FROM MyFirstModule.Ticket TO MyFirstModule.Reporter;`)
	return exec
}

func run(t *testing.T, exec *Executor, mdl string) {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", mdl, errs)
	}
	for _, stmt := range prog.Statements {
		if err := exec.Execute(stmt); err != nil {
			t.Fatalf("executing %q: %v", mdl, err)
		}
	}
}

func typeCheck(t *testing.T, exec *Executor, mdl string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return exec.TypeCheckProgram(prog)
}

// TestTypeCheckProgramCatchesEnumStringLiteral is the end-to-end proof that the
// checker now checks something. Before the CatalogReader seam had an
// implementation, this ran with a nil Catalog and every semantic rule was
// skipped — a green result that meant nothing.
func TestTypeCheckProgramCatchesEnumStringLiteral(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Bug ()
BEGIN
  $T = CREATE MyFirstModule.Ticket (Title = 'x', Status = 'Open');
END;
`)

	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	if got[0].RuleID != "E001" {
		t.Errorf("rule is %q, want exprcheck's own E001 — the codes are kept, not remapped", got[0].RuleID)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity is %v, want error", got[0].Severity)
	}
	// The fix must name the enum the catalog resolved, which is the part that
	// only works because AttributeEnumQN and EnumCases have data behind them.
	if !strings.Contains(got[0].Suggestion, "MyFirstModule.OrderStatus.Open") {
		t.Errorf("suggestion is %q, want the qualified enum value", got[0].Suggestion)
	}
}

// TestTypeCheckProgramAcceptsTheCorrectedForm is the control. Without it the
// test above would pass against a checker that flagged everything.
func TestTypeCheckProgramAcceptsTheCorrectedForm(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Fixed ()
BEGIN
  $T = CREATE MyFirstModule.Ticket (Title = 'x', Status = MyFirstModule.OrderStatus.Open);
END;
`)

	if len(got) != 0 {
		t.Errorf("the corrected form was flagged: %+v", got)
	}
}

// TestTypeCheckProgramSeesChangeAsWellAsCreate pins the fix for the second
// wiring defect. The adapter's default source function reads only
// ast.SourceExpr, and the visitor attaches one to a CREATE's value but not to a
// CHANGE's — so half the enum mistakes in one microflow were invisible until the
// executor supplied a source function that can render either.
func TestTypeCheckProgramSeesChangeAsWellAsCreate(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Both ()
BEGIN
  $T = CREATE MyFirstModule.Ticket (Status = 'Open');
  CHANGE $T (Status = 'Closed');
END;
`)

	if len(got) != 2 {
		t.Fatalf("got %d violations, want one for the CREATE and one for the CHANGE: %+v", len(got), got)
	}
	var sawOpen, sawClosed bool
	for _, v := range got {
		sawOpen = sawOpen || strings.Contains(v.Suggestion, "OrderStatus.Open")
		sawClosed = sawClosed || strings.Contains(v.Suggestion, "OrderStatus.Closed")
	}
	if !sawOpen || !sawClosed {
		t.Errorf("expected both values reported, got %+v", got)
	}
}

// TestTypeCheckProgramLeavesNonEnumAttributesAlone pins that the rule keys off
// the attribute's actual type, not off any string literal in a member slot.
func TestTypeCheckProgramLeavesNonEnumAttributesAlone(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Strings ()
BEGIN
  $T = CREATE MyFirstModule.Ticket (Title = 'Open');
END;
`)

	if len(got) != 0 {
		t.Errorf("a String attribute assigned a string literal was flagged: %+v", got)
	}
}

// TestTypeCheckProgramWithoutAConnectionIsSilent pins the advisory contract: a
// caller that cannot consult a project gets no violations rather than an error.
func TestTypeCheckProgramWithoutAConnectionIsSilent(t *testing.T) {
	exec := New(&bytes.Buffer{})
	defer exec.Close()

	prog, errs := visitor.Build(`CREATE MICROFLOW M.A () BEGIN LOG 'x'; END;`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if got := exec.TypeCheckProgram(prog); got != nil {
		t.Errorf("an unconnected executor reported %+v", got)
	}
	if got := exec.TypeCheckProgram(nil); got != nil {
		t.Errorf("a nil program reported %+v", got)
	}
}

// TestTypeCheckProgramResolvesAttributePaths is the Tier-2 case: `$T/Status`
// has no slot naming its attribute, so the only way to know it is an
// enumeration is to type the variable and look the attribute up. inferKind
// returned KindUnknown for every attribute path until this landed, which is why
// the shape the proposal opens with went uncaught.
func TestTypeCheckProgramResolvesAttributePaths(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Path ($T: MyFirstModule.Ticket)
BEGIN
  IF $T/Status = 'Open' THEN
    LOG 'open';
  END IF;
END;
`)

	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	if got[0].RuleID != "E001" {
		t.Errorf("rule is %q, want E001", got[0].RuleID)
	}
	if !strings.Contains(got[0].Suggestion, "MyFirstModule.OrderStatus.Open") {
		t.Errorf("suggestion is %q, want the qualified enum value", got[0].Suggestion)
	}
}

// TestTypeCheckProgramTypesAParameter pins the half of the scope that was
// missing entirely: buildVarEntityScope walks only the body, so a variable the
// microflow takes as a parameter — the ordinary case — was never typed.
func TestTypeCheckProgramTypesAParameter(t *testing.T) {
	exec := typeCheckFixture(t)

	// The variable is introduced by RETRIEVE rather than by a parameter, which
	// buildVarEntityScope already covered; this is the control for the pair.
	fromBody := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Body ()
BEGIN
  RETRIEVE $T FROM MyFirstModule.Ticket;
  IF $T/Status = 'Open' THEN
    LOG 'x';
  END IF;
END;
`)
	if len(fromBody) != 1 {
		t.Errorf("a RETRIEVE-introduced variable was not typed: %+v", fromBody)
	}

	fromParam := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Param ($T: MyFirstModule.Ticket)
BEGIN
  IF $T/Status = 'Open' THEN
    LOG 'x';
  END IF;
END;
`)
	if len(fromParam) != 1 {
		t.Errorf("a parameter-introduced variable was not typed: %+v", fromParam)
	}
}

// TestTypeCheckProgramFollowsAnAssociation pins the multi-hop path. A Mendix
// expression does not name the intermediate entity the way XPath does, so each
// hop has to be resolved rather than read off the path.
func TestTypeCheckProgramFollowsAnAssociation(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Hop ($T: MyFirstModule.Ticket)
BEGIN
  DECLARE $Label String = 'to: ' + $T/MyFirstModule.Ticket_Reporter/Email;
  DECLARE $Bad String = 'status: ' + $T/Status;
END;
`)

	// Email is a String, so concatenating it is fine and must stay quiet; Status
	// is an Enumeration, which Mendix will not concatenate.
	if len(got) != 1 {
		t.Fatalf("got %d violations, want only the Enumeration concat: %+v", len(got), got)
	}
	if got[0].RuleID != "E004" {
		t.Errorf("rule is %q, want E004", got[0].RuleID)
	}
}

// TestTypeCheckProgramLeavesUnresolvableVariablesAlone pins the failure
// direction end to end. $currentUser is a platform variable the scope does not
// know, and a real project is full of them.
func TestTypeCheckProgramLeavesUnresolvableVariablesAlone(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Unknown ()
BEGIN
  IF $currentUser/Name = 'Ada' THEN
    LOG 'x';
  END IF;
END;
`)

	if len(got) != 0 {
		t.Errorf("an untypeable variable produced %+v", got)
	}
}

// TestTypeCheckProgramTypesLoopVariables pins mendixlabs/mxcli#1100.
//
// The report's title says the checker is skipped inside a LOOP body. It is not:
// the body is walked, and the same expression written against a PARAMETER
// inside the loop was refused before the fix — that control is the third case
// below, and it distinguishes "the walk does not reach here" from "the variable
// resolves to nothing". It was the second: `LOOP $r IN $reqs` never recorded
// `$r`, so `$r/Status` inferred Unknown, and Unknown is tolerated by every rule
// by design.
func TestTypeCheckProgramTypesLoopVariables(t *testing.T) {
	exec := typeCheckFixture(t)

	// The reported script, verbatim in shape. Before the fix: Check passed!,
	// exec wrote it, mxbuild reported CE0117 at the Change variable activity.
	loopVar := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_LoopNormal () RETURNS String
BEGIN
  DECLARE $out String = '';
  RETRIEVE $reqs FROM MyFirstModule.Ticket;
  LOOP $r IN $reqs BEGIN
    $out = $out + $r/Status;
  END LOOP;
  RETURN $out;
END;
`)
	if len(loopVar) != 1 || loopVar[0].RuleID != "E004" {
		t.Errorf("an Enumeration concatenated inside a LOOP produced %+v, want one E004", loopVar)
	}

	// The report's case A, which already worked and must keep working.
	param := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_Param ($Req: MyFirstModule.Ticket) RETURNS String
BEGIN
  DECLARE $out String = '';
  $out = 'status=' + $Req/Status;
  RETURN $out;
END;
`)
	if len(param) != 1 || param[0].RuleID != "E004" {
		t.Errorf("the parameter control produced %+v, want one E004", param)
	}

	// The control that says the LOOP BODY was never the problem: a parameter
	// referenced one line deeper is checked, and was before the fix too.
	paramInLoop := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_ParamInLoop ($Req: MyFirstModule.Ticket)
BEGIN
  RETRIEVE $reqs FROM MyFirstModule.Ticket;
  LOOP $r IN $reqs BEGIN
    LOG 'x {1}' WITH ({1} = 'status=' + $Req/Status);
  END LOOP;
END;
`)
	if len(paramInLoop) != 1 || paramInLoop[0].RuleID != "E004" {
		t.Errorf("a parameter inside a LOOP produced %+v, want one E004", paramInLoop)
	}

	// Every rule was off for a loop variable, not just E004.
	enumCompare := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_LoopEnumCompare ()
BEGIN
  RETRIEVE $reqs FROM MyFirstModule.Ticket;
  LOOP $r IN $reqs BEGIN
    IF $r/Status = 'Open' THEN
      LOG 'x';
    END IF;
  END LOOP;
END;
`)
	if len(enumCompare) != 1 || enumCompare[0].RuleID != "E001" {
		t.Errorf("an enum compared to a string inside a LOOP produced %+v, want one E001", enumCompare)
	}

	// The failure direction. A correct loop must stay silent — a checker that
	// reports the fixed form is worse than one that reported nothing.
	clean := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_LoopClean () RETURNS String
BEGIN
  DECLARE $out String = '';
  RETRIEVE $reqs FROM MyFirstModule.Ticket;
  LOOP $r IN $reqs BEGIN
    $out = $out + toString($r/Status) + $r/Title;
    IF $r/Status = MyFirstModule.OrderStatus.Open THEN
      LOG 'open';
    END IF;
  END LOOP;
  RETURN $out;
END;
`)
	if len(clean) != 0 {
		t.Errorf("a correct loop produced %+v, want none", clean)
	}
}

// TestTypeCheckProgramTypesDeclaredVariables pins the second half of #1100.
//
// Typing the loop variable alone does NOT make the reported script report
// anything: E004 needs both operands known, and the accumulator `$out` was
// Unknown too. That is also why the same mistake was silent with no loop in
// sight — `$out = $out + $Req/Status` on a parameter was unreported before the
// fix, which the report's own case A hides by using a string literal.
func TestTypeCheckProgramTypesDeclaredVariables(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_Accumulate ($Req: MyFirstModule.Ticket) RETURNS String
BEGIN
  DECLARE $out String = '';
  $out = $out + $Req/Status;
  RETURN $out;
END;
`)
	if len(got) != 1 || got[0].RuleID != "E004" {
		t.Errorf("a declared String accumulator produced %+v, want one E004", got)
	}

	// Mendix auto-converts a numeric operand in a String concat, so a declared
	// Integer must not be reported. This is the boundary the rule already had;
	// typing the variable is what puts it in reach of being crossed.
	numeric := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_Numeric () RETURNS String
BEGIN
  DECLARE $n Integer = 1;
  DECLARE $out String = 'n=' + $n;
  RETURN $out;
END;
`)
	if len(numeric) != 0 {
		t.Errorf("a numeric concat produced %+v, want none (Mendix auto-converts)", numeric)
	}
}

// TestTypeCheckProgramTypesDerivedLists pins the list sources a LOOP can
// iterate. The iterator is only as typed as the list it walks, so a retrieve
// over an association and a list operation have to carry their element type or
// the fix above covers one spelling of the same loop.
func TestTypeCheckProgramTypesDerivedLists(t *testing.T) {
	exec := typeCheckFixture(t)

	// An association retrieve names the association, not the entity, so the far
	// end is resolved through the association index.
	assoc := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_LoopAssoc ($T: MyFirstModule.Ticket)
BEGIN
  RETRIEVE $reps FROM $T/MyFirstModule.Ticket_Reporter;
  LOOP $r IN $reps BEGIN
    LOG 'x {1}' WITH ({1} = $r);
  END LOOP;
END;
`)
	if len(assoc) != 1 || assoc[0].RuleID != "E009" {
		t.Errorf("a loop over an association retrieve produced %+v, want one E009", assoc)
	}

	// FILTER carries the input's element type through.
	filtered := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_LoopFiltered ()
BEGIN
  RETRIEVE $reqs FROM MyFirstModule.Ticket;
  $open = FILTER($reqs, $currentObject/Title != '');
  LOOP $r IN $open BEGIN
    LOG 'x {1}' WITH ({1} = 'status=' + $r/Status);
  END LOOP;
END;
`)
	if len(filtered) != 1 || filtered[0].RuleID != "E004" {
		t.Errorf("a loop over a FILTER result produced %+v, want one E004", filtered)
	}
}

// TestTypeCheckProgramChecksBlockScopedBodies covers the two other block-scoped
// positions the report asked about: an ON ERROR handler's body, which was not
// walked at all, and a FIND/FILTER predicate, where $currentObject is now bound
// to the element type of the list under test.
func TestTypeCheckProgramChecksBlockScopedBodies(t *testing.T) {
	exec := typeCheckFixture(t)

	handler := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_Handler ($T: MyFirstModule.Ticket) RETURNS String
BEGIN
  DECLARE $out String = '';
  RETRIEVE $reqs FROM MyFirstModule.Ticket
    ON ERROR {
      $out = $out + $T/Status;
    };
  RETURN $out;
END;
`)
	if len(handler) != 1 || handler[0].RuleID != "E004" {
		t.Errorf("an ON ERROR handler body produced %+v, want one E004", handler)
	}

	predicate := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_Predicate ()
BEGIN
  RETRIEVE $reqs FROM MyFirstModule.Ticket;
  $open = FILTER($reqs, $currentObject/Status = 'Open');
END;
`)
	if len(predicate) != 1 || predicate[0].RuleID != "E001" {
		t.Errorf("a FILTER predicate produced %+v, want one E001", predicate)
	}

	// The control: the same predicate written correctly stays silent, and so
	// does the bare-attribute spelling the skills recommend, which resolves to
	// nothing rather than to a wrong answer.
	clean := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_PredicateClean ()
BEGIN
  RETRIEVE $reqs FROM MyFirstModule.Ticket;
  $open = FILTER($reqs, $currentObject/Status = MyFirstModule.OrderStatus.Open);
  $named = FILTER($reqs, "Title" != '');
END;
`)
	if len(clean) != 0 {
		t.Errorf("a correct FILTER predicate produced %+v, want none", clean)
	}

	// Mendix's STRING find(haystack, needle) still arrives as a
	// ListOperationStmt — the visitor does not disambiguate it, the flow
	// builder does, by looking at whether the input is a declared String
	// (mdl-examples/bug-tests/ledger-63-string-find.mdl). Checking its second
	// argument as a Boolean predicate reported the needle on a script that
	// builds at 0 errors, which the corpus sweep caught and this pins.
	stringFind := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.SUB_StringFind ($Hay: String, $Needle: String) RETURNS Integer
BEGIN
  DECLARE $At Integer = 0;
  SET $At = find($Hay, $Needle);
  RETURN $At;
END;
`)
	if len(stringFind) != 0 {
		t.Errorf("Mendix's string find() produced %+v, want none", stringFind)
	}
}

// TestTypeCheckProgramSeesScriptCreatedEnum covers the side finding of
// ako/mxcli#969 item 1: the enumeration and the attribute are created in the
// same script as the microflow, so the catalog (built from the stored project)
// knows neither, and the genuine E001 used to vanish.
func TestTypeCheckProgramSeesScriptCreatedEnum(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE ENUMERATION MyFirstModule.WindEnum (N 'North', NW 'North west');
CREATE PERSISTENT ENTITY MyFirstModule.Log (Dir: String(10), Wind: Enumeration(MyFirstModule.WindEnum));
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Wind ($Log: MyFirstModule.Log)
BEGIN
  CHANGE $Log (Wind = 'NW');
END;
`)
	if len(got) != 1 || got[0].RuleID != "E001" {
		t.Fatalf("got %+v, want one E001 for the quoted value", got)
	}
	if !strings.Contains(got[0].Suggestion, "MyFirstModule.WindEnum.NW") {
		t.Errorf("suggestion is %q, want the script-declared enum value", got[0].Suggestion)
	}
}

// TestTypeCheckProgramEnumSlotOperandsAreNotValues is item 1 itself: literals
// compared with a String, passed to find(), or used in an if-condition are not
// the value assigned to the enumeration attribute.
func TestTypeCheckProgramEnumSlotOperandsAreNotValues(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Status ($T: MyFirstModule.Ticket, $Dir: String)
BEGIN
  CHANGE $T (Status = if $Dir = 'NW' then MyFirstModule.OrderStatus.Open else MyFirstModule.OrderStatus.Closed);
  CHANGE $T (Status = if find('|NW|NORTHWEST|', '|' + $Dir + '|') >= 0 then MyFirstModule.OrderStatus.Open else MyFirstModule.OrderStatus.Closed);
END;
`)
	if len(got) != 0 {
		t.Errorf("operands outside value position were flagged: %+v", got)
	}
	// Control: a quoted value in a then-branch is still the value.
	got = typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Status2 ($T: MyFirstModule.Ticket, $Dir: String)
BEGIN
  CHANGE $T (Status = if $Dir = 'NW' then 'Open' else MyFirstModule.OrderStatus.Closed);
END;
`)
	if len(got) != 1 || got[0].RuleID != "E001" {
		t.Errorf("then-branch 'Open' must be E001, got %+v", got)
	}
}
