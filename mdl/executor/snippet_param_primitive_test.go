// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Primitive snippet parameters are a Mendix 11.17 feature ("Primitive
// Parameters Support for Snippets"). Below 11.17 mxbuild rejects every one with
// CE0046 "Invalid data type '<caption>'." — measured on 11.13.0 and 11.16.0 —
// and from 11.17.0 the same bytes check clean, as does Studio Pro 11.17's own
// primitive snippet (ako/TestApp Snippet.SnippetPrimitiveParameter). The floor
// is pages/snippet_parameter_primitive; the measurements are in
// types.SnippetParameterTypeRule.
//
// History: mendixlabs/mxcli#1028. The documented spelling
//
//	CREATE SNIPPET Test.SNIPPET_Label ( Params: { $Label: string } )
//
// failed at exec with "failed to resolve entity string: entity not found:
// string", naming a type nobody spelled, because the snippet's Params clause
// never typed a primitive and the executor took the source text for an entity
// name. It was then refused outright (CE0046). On 11.17 it is now written —
// and written typed: a primitive parameter left untyped serializes as a
// DataTypes$ObjectType naming no entity, which mx 11.17 cannot even load
// ("An error occurred when trying to set the 'Entity' property of a Object
// type in a Snippet").

// buildSnippetFromMDL parses one CREATE SNIPPET statement and builds it against
// a backend that knows no entities, so an attempt to resolve a primitive as an
// entity surfaces rather than being masked by a lucky name collision. version
// is the project's Mendix version ("11.16.0"); the gate reads it.
func buildSnippetFromMDL(t *testing.T, src, version string) (*pages.Snippet, error) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var stmt *ast.CreateSnippetStmtV3
	for _, s := range prog.Statements {
		if cs, ok := s.(*ast.CreateSnippetStmtV3); ok {
			stmt = cs
			break
		}
	}
	if stmt == nil {
		t.Fatal("no CREATE SNIPPET statement built")
	}

	mod := mkModule("Test")
	h := mkHierarchy(mod)
	pv := projectVersionOf(t, version)
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListSnippetsFunc:   func() ([]*pages.Snippet, error) { return nil, nil },
		ProjectVersionFunc: func() *types.ProjectVersion { return pv },
	}
	pb := newPageBuilder(mb, h, "Test")
	pb.ctx = &ExecContext{Context: context.Background(), Backend: mb}
	return pb.buildSnippetV3(stmt)
}

func projectVersionOf(t *testing.T, v string) *types.ProjectVersion {
	t.Helper()
	var major, minor, patch int
	if n, err := fmt.Sscanf(v, "%d.%d.%d", &major, &minor, &patch); n != 3 || err != nil {
		t.Fatalf("bad version %q", v)
	}
	return &types.ProjectVersion{ProductVersion: v, MajorVersion: major, MinorVersion: minor, PatchVersion: patch}
}

// The reported repro, below the floor. The refusal must name the CE the author
// would otherwise meet a whole build later and the version that lifts it, and
// must not be the old "entity not found" — which pointed at a lower-cased word
// that appears nowhere in the script and offered no way to act.
func TestBuildSnippetV3_PrimitiveParameterRefusedBelow1117(t *testing.T) {
	_, err := buildSnippetFromMDL(t, `CREATE SNIPPET Test.SNIPPET_Label (
  Params: { $Label: string }
) {
  DYNAMICTEXT dt (Content: 'x')
}`, "11.16.0")
	if err == nil {
		t.Fatal("a primitive snippet parameter was accepted on 11.16; mxbuild rejects it with CE0046")
	}
	msg := err.Error()
	for _, want := range []string{"$Label", "String", "CE0046", "11.17"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "entity not found") {
		t.Errorf("still the #1028 message, which names a type nobody spelled: %s", msg)
	}
}

// Every primitive, in the reporter's own casing and in the documented casing —
// MDL type keywords are case-insensitive. Below 11.17 each is refused; from
// 11.17 each is written with its DataTypes $Type. The 11.17 half is the
// control that the version decides, not the primitive: a builder that still
// refused everything, or wrote no type (the unloadable ObjectType), fails it.
func TestBuildSnippetV3_EveryPrimitiveParameterAcrossTheFloor(t *testing.T) {
	for spelling, want := range map[string]string{
		"string": "DataTypes$StringType", "String": "DataTypes$StringType", "STRING": "DataTypes$StringType",
		"Integer": "DataTypes$IntegerType", "Long": "DataTypes$IntegerType",
		"Decimal": "DataTypes$DecimalType", "Boolean": "DataTypes$BooleanType",
		"DateTime": "DataTypes$DateTimeType",
	} {
		t.Run(spelling, func(t *testing.T) {
			src := `CREATE SNIPPET Test.S (
  Params: { $P: ` + spelling + ` }
) { DYNAMICTEXT dt (Content: 'x') }`
			if _, err := buildSnippetFromMDL(t, src, "11.16.0"); err == nil || !strings.Contains(err.Error(), "CE0046") {
				t.Errorf("11.16: err = %v, want the CE0046 refusal", err)
			}
			snp, err := buildSnippetFromMDL(t, src, "11.17.0")
			if err != nil {
				t.Fatalf("11.17: %v", err)
			}
			if len(snp.Parameters) != 1 {
				t.Fatalf("11.17: %d parameters, want 1", len(snp.Parameters))
			}
			if p := snp.Parameters[0]; p.Type != want || p.EntityName != "" {
				t.Errorf("11.17: $P typed (Type %q, Entity %q), want (%q, \"\")", p.Type, p.EntityName, want)
			}
		})
	}
}

// CONTROL: an entity-typed parameter is still built, still resolved, and still
// fails loudly when the entity does not exist — on 11.17 as before.
func TestBuildSnippetV3_EntityParameterStillResolves(t *testing.T) {
	_, err := buildSnippetFromMDL(t, `CREATE SNIPPET Test.S (
  Params: { $C: Test.NoSuchEntity }
) { DYNAMICTEXT dt (Content: 'x') }`, "11.17.0")
	if err == nil {
		t.Fatal("an unknown entity-typed parameter was accepted; want a resolve failure")
	}
	if !strings.Contains(err.Error(), "Test.NoSuchEntity") {
		t.Errorf("error = %v, want it to name Test.NoSuchEntity", err)
	}
	if strings.Contains(err.Error(), "CE0046") {
		t.Errorf("entity parameter hit the primitive refusal: %v", err)
	}
}

// Inside a snippet, a primitive parameter binds an input and a text template in
// the SnippetParameter slot, with no attribute — Studio Pro 11.17's own storage
// (TestApp Snippet.SnippetPrimitiveParameter: SourceVariable {SnippetParameter:
// "par2"}, AttributeRef null). The input used to be refused ("not a page
// variable"); the template went to the PageParameter slot, naming a page
// parameter the snippet does not have — mx 11.17 CE0402 + CE7005.
func TestBuildSnippetV3_PrimitiveParameterBindsInTheSnippetSlot(t *testing.T) {
	snp, err := buildSnippetFromMDL(t, `CREATE SNIPPET Test.S (
  Params: ( $Label: String )
) {
  TEXTBOX tb (Label: 'Label', Attribute: $Label)
  DYNAMICTEXT dt (Content: 'Hello {1}', ContentParams: ({1} = $Label))
}`, "11.17.0")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var tb *pages.TextBox
	var dt *pages.DynamicText
	for _, w := range snp.Widgets {
		switch x := w.(type) {
		case *pages.TextBox:
			tb = x
		case *pages.DynamicText:
			dt = x
		}
	}
	if tb == nil || dt == nil {
		t.Fatalf("widgets built: %+v", snp.Widgets)
	}
	if sv := tb.SourceVariable; sv == nil || sv.Variable != "Label" || sv.Kind != "snippet" || sv.Widget != "" {
		t.Errorf("textbox SourceVariable = %+v, want {Variable: Label, Kind: snippet}", sv)
	}
	if tb.AttributePath != "" {
		t.Errorf("textbox AttributePath = %q, want none: the parameter is the value", tb.AttributePath)
	}
	if dt.Content == nil || len(dt.Content.Parameters) != 1 {
		t.Fatalf("dynamictext template parameters: %+v", dt.Content)
	}
	if p := dt.Content.Parameters[0]; p.SourceVariable != "Label" || p.SourceVariableKind != "snippet" {
		t.Errorf("template parameter (Variable, Kind) = (%q, %q), want (Label, snippet)", p.SourceVariable, p.SourceVariableKind)
	}
}

// CONTROL: a PAGE parameter of the same primitive is accepted and typed — on
// every version that has page parameters — and an input binds it in the
// PageParameter slot (mx 11.16.0 and 11.17.0: 0 errors).
func TestBuildPageV3_PrimitiveParameterStillAccepted(t *testing.T) {
	// No layout: this asserts what the parameter clause builds, and a layout
	// the mock backend does not have would fail the page for an unrelated
	// reason.
	prog, errs := visitor.Build(`CREATE PAGE Test.P (
  Title: 'P', Params: { $Label: String, $Count: Long }
) { }`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var stmt *ast.CreatePageStmtV3
	for _, s := range prog.Statements {
		if cp, ok := s.(*ast.CreatePageStmtV3); ok {
			stmt = cp
			break
		}
	}
	if stmt == nil {
		t.Fatal("no CREATE PAGE statement built")
	}
	mod := mkModule("Test")
	pb := newPageBuilder(&mock.MockBackend{IsConnectedFunc: func() bool { return true }},
		mkHierarchy(mod), "Test")
	page, err := pb.buildPageV3(stmt)
	if err != nil {
		t.Fatalf("buildPageV3: %v", err)
	}
	want := []string{"DataTypes$StringType", "DataTypes$IntegerType"}
	for i, p := range page.Parameters {
		if p.TypeName != want[i] {
			t.Errorf("page param %s = %q, want %q", p.Name, p.TypeName, want[i])
		}
	}
	sv := pb.pageVariableInputBinding(&ast.WidgetV3{Type: "textbox", Name: "tb", Properties: map[string]any{
		"Attribute": &ast.DataSourceV3{Reference: "$Label"},
	}})
	if sv == nil || sv.Variable != "Label" || sv.Kind != "" {
		t.Errorf("page input binding = %+v, want {Variable: Label, Kind: \"\"} (the PageParameter slot)", sv)
	}
}

// A page variable passed to a PRIMITIVE snippet parameter is the 11.17 "pass a
// local variable directly to a snippet": written in the LocalVariable slot,
// which mx 11.17.0 checks clean. To an entity parameter it stays the #833
// CE0115 refusal (TestSnippetCallLocalVariableArgument), and an omitted
// primitive parameter is refused — no data context supplies one.
func TestSnippetCall_PageVariableToPrimitiveParameter(t *testing.T) {
	pb := scopedPB(false)
	pb.localVariables = map[string]bool{"Caption": true}
	pb.backend = &mock.MockBackend{ListSnippetsFunc: func() ([]*pages.Snippet, error) {
		return []*pages.Snippet{{Name: "Snip", Parameters: []*pages.SnippetParameter{{Name: "Label", Type: "DataTypes$StringType"}}}}, nil
	}}
	sc := &pages.SnippetCallWidget{}
	if err := pb.buildSnippetCallParams(sc, "M.Snip", []ast.SnippetCallParam{{ParamName: "Label", Variable: "$Caption"}}); err != nil {
		t.Fatalf("page variable to a primitive parameter: %v", err)
	}
	if len(sc.ParameterMappings) != 1 || !sc.ParameterMappings[0].IsLocalVariable || sc.ParameterMappings[0].Argument != "$Caption" {
		t.Errorf("mappings = %+v, want one LocalVariable mapping of $Caption", sc.ParameterMappings)
	}

	err := pb.buildSnippetCallParams(&pages.SnippetCallWidget{}, "M.Snip", nil)
	if err == nil || !strings.Contains(err.Error(), "requires parameter $Label") {
		t.Errorf("omitted primitive parameter: err = %v, want it refused", err)
	}
}

// The gate is a statement gate, so `mxcli check -p` and exec's pre-flight
// report it before anything is written, for create and for alter add.
func TestStatementVersionGates_PrimitiveSnippetParameter(t *testing.T) {
	str := ast.PageParameter{Name: "Label", Type: ast.DataType{Kind: ast.TypeString}}
	ent := ast.PageParameter{Name: "C", EntityType: ast.QualifiedName{Module: "M", Name: "Customer"}}
	has := func(stmt ast.Statement) bool {
		for _, g := range statementVersionGates(stmt) {
			if g.area == "pages" && g.name == "snippet_parameter_primitive" {
				return true
			}
		}
		return false
	}
	for name, c := range map[string]struct {
		stmt ast.Statement
		want bool
	}{
		"create snippet, primitive": {&ast.CreateSnippetStmtV3{Parameters: []ast.PageParameter{ent, str}}, true},
		"create snippet, entity":    {&ast.CreateSnippetStmtV3{Parameters: []ast.PageParameter{ent}}, false},
		"alter snippet add primitive": {&ast.AlterPageStmt{ContainerType: "SNIPPET",
			Operations: []ast.AlterPageOperation{&ast.AddParameterOp{Parameter: str}}}, true},
		"alter page add primitive": {&ast.AlterPageStmt{ContainerType: "PAGE",
			Operations: []ast.AlterPageOperation{&ast.AddParameterOp{Parameter: str}}}, false},
		"create page, primitive": {&ast.CreatePageStmtV3{Parameters: []ast.PageParameter{str}}, false},
	} {
		if got := has(c.stmt); got != c.want {
			t.Errorf("%s: gated %v, want %v", name, got, c.want)
		}
	}
}
