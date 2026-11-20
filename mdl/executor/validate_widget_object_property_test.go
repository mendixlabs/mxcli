// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// fixtureProjectWithDefs copies the fixture to a temp dir and DERIVES its widget
// definitions from the tracked `.mpk` files.
//
// `testdata/expr-checker/.mxcli/` is gitignored: the `.def.json` files are
// generated, and a developer who has ever run `mxcli widget docs` against the
// fixture has them while CI never does. Reading them ambiently makes a test pass
// locally and fail on the runner — which is exactly how the three tests below
// first shipped red. Deriving them here depends only on tracked inputs, and the
// copy keeps the generated files out of the fixture other tests copy.
func fixtureProjectWithDefs(t *testing.T) string {
	t.Helper()
	src := filepath.Dir(fixtureProject(t))
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, src); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	// Start from no definitions whatever the developer's tree holds, so the
	// test sees the same inputs here and on the runner.
	if err := os.RemoveAll(filepath.Join(dst, ".mxcli")); err != nil {
		t.Fatalf("clear derived definitions: %v", err)
	}
	proj := filepath.Join(dst, "minimal.mpr")
	if _, err := RefreshWidgetDefinitions(proj, true, nil); err != nil {
		t.Fatalf("derive widget definitions from the tracked .mpk files: %v", err)
	}
	return proj
}

func widget27(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing: %v", errs)
	}
	registry := LoadWidgetRegistry(fixtureProjectWithDefs(t))
	if registry == nil {
		t.Fatal("no registry")
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		for _, v := range ValidateWidgetPropertiesForStatement(stmt, registry) {
			if v.RuleID == "MDL-WIDGET27" {
				out = append(out, v)
			}
		}
	}
	return out
}

const page27 = `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  htmlelement frame ( tagName: 'div', %s )
}`

// mendixlabs/mxcli#999. A repeatable widget property written as a property
// VALUE had two failure modes, and the dangerous one looked like success:
//
//	[(configMode: simple)]              parsed as a list of expressions,
//	                                    checked CLEAN, exec'd, and the property
//	                                    vanished from storage
//	[(configMode: simple, x: y)]        died as `missing ')' at ','`
//
// The reporter's own framing: "check/exec-pass-then-discard". On FileUploader
// the dropped property was `allowedFileFormats`, which restricts uploads — so a
// silent drop is a correctness AND a mild security problem in the built app.
func TestObjectEntryProperty_IsReported(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s", `attributes: [(attributeName: 'data-x')]`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27, want 1: %+v", len(got), got)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error — a warning would still let exec write the "+
			"page and discard the entries, which is the bug", got[0].Severity)
	}
	// The message has to name the container keyword, or it says "wrong" without
	// saying what right looks like. Assert the KEYWORD IN ITS REMEDY, not a bare
	// "attribute": the property is called `attributes`, so a substring check for
	// "attribute" is satisfied by the property name and passes with no definition
	// loaded at all — which is how the definition-dependent tests below shipped
	// green locally and red in CI.
	if !strings.Contains(got[0].Message, "`attribute <name> (…)` blocks") {
		t.Errorf("message does not name the container keyword: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "attributes") {
		t.Errorf("message does not name the property: %q", got[0].Message)
	}
}

// The multi-key shape is the one that would not parse at all. It now reaches the
// same diagnostic instead of `missing ')' at ','`, which named a paren and left
// the author to guess.
func TestObjectEntryProperty_MultiKeyReachesTheSameError(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s",
		`attributes: [(attributeName: 'data-x', attributeValueType: 'expression')]`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27, want 1: %+v", len(got), got)
	}
}

// Several entries, which is what a real allowedFileFormats list looks like.
func TestObjectEntryProperty_SeveralEntries(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s",
		`attributes: [(attributeName: 'a'), (attributeName: 'b')]`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27, want 1 per property: %+v", len(got), got)
	}
}

// THE CONTROL. The container form is the spelling that works, and it must stay
// silent — otherwise the rule is "report every object list" and the assertions
// above prove nothing about the property form specifically.
func TestObjectEntryProperty_ContainerFormIsSilent(t *testing.T) {
	src := `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  htmlelement frame ( tagName: 'div' ) {
    attribute a1 (attributeName: 'data-x', attributeValueType: 'expression')
  }
}`
	if got := widget27(t, src); len(got) != 0 {
		t.Errorf("the working container form was reported: %+v", got)
	}
}

// The second control: an ordinary array property must keep working. `[…]` is a
// real value shape (DesignProperties, ContentParams), and a rule that fired on
// every bracket would break scripts that are correct today.
func TestObjectEntryProperty_OrdinaryArrayIsSilent(t *testing.T) {
	src := `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: '{1}', ContentParams: [{1} = Name])
}`
	if got := widget27(t, src); len(got) != 0 {
		t.Errorf("an ordinary array property was reported: %+v", got)
	}
}

// A property the widget does not declare as an object list still gets the error:
// the SHAPE is wrong regardless, and staying quiet would put us back to writing
// a page that silently discards it.
func TestObjectEntryProperty_ReportedEvenWhenTheKeyIsUnknown(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s", `notARealProperty: [(a: 'b')]`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27, want 1: %+v", len(got), got)
	}
}

// And with no project at all — `make check-mdl` runs that way, so a rule that
// needed a definition would be inert in CI.
func TestObjectEntryProperty_FiresWithoutAProject(t *testing.T) {
	src := strings.Replace(page27, "%s", `attributes: [(attributeName: 'data-x')]`, 1)
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing: %v", errs)
	}
	registry := LoadWidgetRegistry("")
	var n int
	for _, stmt := range prog.Statements {
		for _, v := range ValidateWidgetPropertiesForStatement(stmt, registry) {
			if v.RuleID == "MDL-WIDGET27" {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("got %d MDL-WIDGET27 with no project, want 1", n)
	}
}

// The AST shape, asserted directly — the #1036 lesson is that a corpus diff of
// check output is blind to a construct that parses into the wrong shape, and
// this construct's whole problem was that it parsed into a []string nobody
// claimed.
func TestObjectEntryProperty_ParsesToItsOwnType(t *testing.T) {
	prog, errs := visitor.Build(strings.Replace(page27, "%s",
		`attributes: [(attributeName: 'data-x', attributeValueType: 'expression')]`, 1))
	if len(errs) > 0 {
		t.Fatalf("parsing: %v", errs)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	oel, ok := w.Properties["attributes"].(*ast.ObjectEntryListV3)
	if !ok {
		t.Fatalf("attributes is %T, want *ast.ObjectEntryListV3", w.Properties["attributes"])
	}
	if len(oel.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(oel.Entries))
	}
	if oel.Entries[0]["attributeName"] != "data-x" {
		t.Errorf("entry = %+v, want attributeName data-x", oel.Entries[0])
	}
}

// The shape control for the grammar change. The new alternative is ordered
// BEFORE the expression array, so the risk is that it swallows an ordinary
// `[…]` value and changes its AST type — which a corpus diff of check output
// cannot see, because a value read as the wrong type still produces no
// diagnostic (the slice 2-3 lesson from mendixlabs/mxcli#1036).
//
// Measured alongside this: 0 of 532 example scripts changed their check output.
// That is necessary and not sufficient; this is the sufficient half.
func TestOrdinaryArrayKeepsItsAstShape(t *testing.T) {
	prog, errs := visitor.Build(`create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: '{1}', ContentParams: [{1} = Name])
}`)
	if len(errs) > 0 {
		t.Fatalf("parsing: %v", errs)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	got := w.Properties["ContentParams"]
	if _, wrong := got.(*ast.ObjectEntryListV3); wrong {
		t.Fatalf("an ordinary array was captured by the object-entry alternative: %#v", got)
	}
	if _, ok := got.([]ast.ParamAssignmentV3); !ok {
		t.Errorf("ContentParams is %T, want []ast.ParamAssignmentV3 — the ordinary array "+
			"shape is unchanged", got)
	}

	// A plain literal array too, which is the shape closest to `[(…)]` and so the
	// likeliest to be captured by mistake.
	prog2, errs2 := visitor.Build(`create page M.P2 (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: 'x', DesignProperties: ['Spacing top': 'Large'])
}`)
	if len(errs2) > 0 {
		t.Fatalf("parsing the design-property array: %v", errs2)
	}
	w2 := prog2.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	if _, wrong := w2.Properties["DesignProperties"].(*ast.ObjectEntryListV3); wrong {
		t.Errorf("a design-property array was captured by the object-entry alternative: %#v",
			w2.Properties["DesignProperties"])
	}
}

// mendixlabs/mxcli#1056. #999's fix keyed on the `[(k: v)]` shape, so it needed
// at least one parenthesised entry. The reporter of #1056 reached for the EMPTY
// form instead — `customAllSelected: []` — after the slot syntax their own
// generated docs showed was rejected by the parser (that half is fixed; the
// grammar took generic container names in bca5466e). The empty form fell
// through to the generic `[expr, …]` branch, became a nil []string no writer
// claims, and reproduced #999 exactly: check clean, exec successful, nothing
// written — then 3x CE0642 at build time because the slots are required.
func TestEmptyListProperty_IsReported(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s", `attributes: []`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27 for `attributes: []`, want 1: %+v", len(got), got)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error — a warning lets exec write the page "+
			"with the property discarded, which is the whole defect", got[0].Severity)
	}
	if !strings.Contains(got[0].Suggestion, "attribute") {
		t.Errorf("suggestion must name the container keyword, got %q", got[0].Suggestion)
	}
}

// A widgets-typed slot is the same mistake wearing a different type: the
// property holds CHILD WIDGETS, so no value written in the property list can
// ever reach storage. This is the exact property from #1056.
func TestEmptyListOnWidgetSlot_IsReported(t *testing.T) {
	src := `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.selectionhelper.SelectionHelper' sh (
    renderStyle: 'custom', customAllSelected: []
  )
}`
	got := widget27(t, src)
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27 for `customAllSelected: []`, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Suggestion, "customallselected") {
		t.Errorf("suggestion must name the slot's container keyword, got %q", got[0].Suggestion)
	}
}

// A scalar cannot reach a container-typed property either, and this one is only
// knowable from the definition — which is why it is reported ONLY when the
// widget resolves. Without that, `p: 'x'` is the ordinary property form and
// flagging it would be a guess.
func TestScalarOnContainerProperty_IsReported(t *testing.T) {
	src := `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.selectionhelper.SelectionHelper' sh (
    renderStyle: 'custom', customAllSelected: 'something'
  )
}`
	got := widget27(t, src)
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27 for `customAllSelected: 'something'`, want 1: %+v", len(got), got)
	}
}

// The bracket form is how MDL writes a conditional expression and a filter's
// attribute list. Reporting those would break the corpus, so the rule must key
// on EMPTINESS, never on the brackets.
func TestNonEmptyBracketValuesAreNotReported(t *testing.T) {
	for _, prop := range []string{
		`visible: [$currentObject/Name != '']`,
		`editable: [true]`,
	} {
		if got := widget27(t, strings.Replace(page27, "%s", prop, 1)); len(got) != 0 {
			t.Errorf("%s: got %d MDL-WIDGET27, want 0: %+v", prop, len(got), got)
		}
	}
	filter := `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  datagrid2 dg (DataSource: database, Entity: MyFirstModule.Expense) {
    column c1 (Attribute: Name) { textfilter tf (attributes: [Name]) }
  }
}`
	if got := widget27(t, filter); len(got) != 0 {
		t.Errorf("filter attribute list: got %d MDL-WIDGET27, want 0: %+v", len(got), got)
	}
}

// The other half of mendixlabs/mxcli#1056: the slot syntax the widget docs
// generate must PARSE. It did not on the reporter's build (`mismatched input
// 'customallselected' expecting '}'`), which is why they reached for
// `customAllSelected: []` at all. bca5466e made generic container names parse;
// this pins it, so the two halves cannot regress independently — a parser that
// rejects the right spelling turns MDL-WIDGET27 into a dead end.
func TestDocumentedSlotSyntaxParses(t *testing.T) {
	src := `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.selectionhelper.SelectionHelper' sh (
    renderStyle: 'custom'
  ) {
    customallselected s1 { dynamictext d1 (Content: 'All') }
    customsomeselected s2 { dynamictext d2 (Content: 'Some') }
    customnoneselected s3 { dynamictext d3 (Content: 'None') }
  }
}`
	if _, errs := visitor.Build(src); len(errs) > 0 {
		t.Fatalf("the slot form `mxcli widget docs` emits must parse, got: %v", errs)
	}
	if got := widget27(t, src); len(got) != 0 {
		t.Errorf("the correct form must not be reported, got %d: %+v", len(got), got)
	}
}
