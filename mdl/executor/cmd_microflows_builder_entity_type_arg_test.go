// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// parsedFlowBody parses src (one CREATE MICROFLOW/NANOFLOW statement) and
// returns its body. These tests go through the parser on purpose: the defect
// lives in the text the visitor preserves for an argument, and a hand-built
// ast.IdentifierExpr — which is what the #1137 tests use — carries none of it.
func parsedFlowBody(t *testing.T, src string) []ast.MicroflowStatement {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			return s.Body
		case *ast.CreateNanoflowStmt:
			return s.Body
		}
	}
	t.Fatalf("no microflow or nanoflow in %q", src)
	return nil
}

func refreshEntityBackend() *mock.MockBackend {
	return &mock.MockBackend{
		ReadJavaScriptActionByNameFunc: func(string) (*types.JavaScriptAction, error) {
			return &types.JavaScriptAction{
				Name: "RefreshEntity",
				Parameters: []*types.JavaActionParameter{
					{Name: "EntityToRefresh", ParameterType: &types.EntityTypeParameterType{}},
				},
			}, nil
		},
	}
}

func jsEntityArgument(t *testing.T, fb *flowBuilder, src string) string {
	t.Helper()
	for _, stmt := range parsedFlowBody(t, src) {
		call, ok := stmt.(*ast.CallJavaScriptActionStmt)
		if !ok {
			continue
		}
		action := jsActionActivity(t, fb, call)
		value, ok := action.ParameterMappings[0].Value.(*microflows.EntityTypeCodeActionParameterValue)
		if !ok {
			t.Fatalf("value = %T, want *EntityTypeCodeActionParameterValue", action.ParameterMappings[0].Value)
		}
		return value.Entity
	}
	t.Fatal("no call javascript action in body")
	return ""
}

// TestBuildJavaScriptAction_EntityTypeArgumentOnItsOwnLine reproduces
// mendixlabs/mxcli#1171, verbatim in shape: the argument on its own line and
// the closing parenthesis on the next.
//
//	$Var = call javascript action NanoflowCommons.RefreshEntity(
//	EntityToRefresh = ReferenceData.Dimension
//	);
//
// The visitor keeps an argument's trailing whitespace so an *expression*
// round-trips as written. The #1137 fix then took that same text as the
// entity's qualified name and stored `Entity: "ReferenceData.Dimension\n"` —
// the right $Type, naming an entity that does not exist. mxbuild reports it
// as the same CE0115 #1137 fixed ("The arguments that are passed to
// JavaScript action 'NanoflowCommons.RefreshEntity' do not match the expected
// parameters and need to be refreshed"), while `mxcli check --references`
// passes and DESCRIBE prints the stray newline as harmless layout.
func TestBuildJavaScriptAction_EntityTypeArgumentOnItsOwnLine(t *testing.T) {
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, backend: refreshEntityBackend()}
	got := jsEntityArgument(t, fb, `create or modify nanoflow CustomModule.ACT_Example ()
begin
$Var = call javascript action NanoflowCommons.RefreshEntity(
EntityToRefresh = ReferenceData.Dimension
);
return;
end;`)
	if got != "ReferenceData.Dimension" {
		t.Errorf("Entity = %q, want %q (issue #1171: a stored name with trailing whitespace is CE0115)", got, "ReferenceData.Dimension")
	}
}

// TestBuildJavaScriptAction_EntityTypeArgumentOneLine is the control: the
// single-line spelling #1137's example uses. It passed before the #1171 fix,
// which is what shows the failure above is the layout and not the value.
func TestBuildJavaScriptAction_EntityTypeArgumentOneLine(t *testing.T) {
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, backend: refreshEntityBackend()}
	got := jsEntityArgument(t, fb, `create nanoflow CustomModule.ACT_Example ()
begin
  call javascript action NanoflowCommons.RefreshEntity(EntityToRefresh = ReferenceData.Dimension);
end;`)
	if got != "ReferenceData.Dimension" {
		t.Errorf("Entity = %q, want ReferenceData.Dimension", got)
	}
}

// TestBuildJavaScriptAction_EntityTypeVariableArgumentOnItsOwnLine: a variable
// argument is resolved to the entity it holds through varTypes, keyed by the
// bare name. Trailing whitespace made that lookup miss too, so the stored
// value became the literal text "$Dim\n".
func TestBuildJavaScriptAction_EntityTypeVariableArgumentOnItsOwnLine(t *testing.T) {
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		backend:  refreshEntityBackend(),
		varTypes: map[string]string{"Dim": "ReferenceData.Dimension"},
	}
	got := jsEntityArgument(t, fb, `create nanoflow CustomModule.ACT_Example ($Dim: ReferenceData.Dimension)
begin
  call javascript action NanoflowCommons.RefreshEntity(
    EntityToRefresh = $Dim
  );
end;`)
	if got != "ReferenceData.Dimension" {
		t.Errorf("Entity = %q, want ReferenceData.Dimension", got)
	}
}

// TestBuildJavaAction_EntityTypeArgumentOnItsOwnLine: the Java-action builder
// is the twin #1137 copied, and has the same hole.
func TestBuildJavaAction_EntityTypeArgumentOnItsOwnLine(t *testing.T) {
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		backend: &mock.MockBackend{
			ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) {
				return &javaactions.JavaAction{
					Name: "Export",
					Parameters: []*javaactions.JavaActionParameter{
						{Name: "EntityType", ParameterType: &javaactions.EntityTypeParameterType{}},
					},
				}, nil
			},
		},
	}
	body := parsedFlowBody(t, `create microflow CustomModule.ACT_Export ()
begin
  call java action CustomModule.Export(
    EntityType = ReferenceData.Dimension
  );
end;`)
	for _, stmt := range body {
		call, ok := stmt.(*ast.CallJavaActionStmt)
		if !ok {
			continue
		}
		id := fb.addCallJavaActionAction(call)
		for _, obj := range fb.objects {
			if obj.GetID() != id {
				continue
			}
			action := obj.(*microflows.ActionActivity).Action.(*microflows.JavaActionCallAction)
			value, ok := action.ParameterMappings[0].Value.(*microflows.EntityTypeCodeActionParameterValue)
			if !ok {
				t.Fatalf("value = %T, want *EntityTypeCodeActionParameterValue", action.ParameterMappings[0].Value)
			}
			if value.Entity != "ReferenceData.Dimension" {
				t.Errorf("Entity = %q, want ReferenceData.Dimension", value.Entity)
			}
			return
		}
	}
	t.Fatal("no call java action in body")
}

// TestBuildJavaScriptAction_QuotedEntityTypeArgument is mendixlabs/mxcli#1785
// (ako/mxcli#2057) verbatim: quoted identifiers, which the report said were
// stored with their MDL quotes (`"CustomModule"."NPActivity"`), giving CE1613
// at mxbuild on mxcli 0.26.0. Measured on main: the stored Entity is the bare
// qualified name and `mx check` (11.14, TestApp) reports 0 errors. This pins
// it — a quoted name must never reach storage with its quotes.
func TestBuildJavaScriptAction_QuotedEntityTypeArgument(t *testing.T) {
	for name, src := range map[string]string{
		"reporter's MDL": `CREATE OR MODIFY NANOFLOW "CustomModule"."TEST_RefreshEntity_NF" (
  $Obj: "CustomModule"."NPActivity"
)
BEGIN
  $r = CALL JAVASCRIPT ACTION "NanoflowCommons"."RefreshEntity" (
    EntityToRefresh = "CustomModule"."NPActivity"
  );
  RETURN;
END;`,
		"quoted, one line": `create nanoflow CustomModule.ACT_Example ()
begin
  call javascript action NanoflowCommons.RefreshEntity(EntityToRefresh = "CustomModule"."NPActivity");
end;`,
	} {
		t.Run(name, func(t *testing.T) {
			fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, backend: refreshEntityBackend()}
			if got := jsEntityArgument(t, fb, src); got != "CustomModule.NPActivity" {
				t.Errorf("Entity = %q, want CustomModule.NPActivity", got)
			}
		})
	}
}
