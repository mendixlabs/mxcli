// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// ako/mxcli#935: `create java action M.JA_Paint (C: M.Color) returns Boolean`
// with M.Color an enumeration passed check and exec and was written as an
// ENTITY parameter — mxbuild: CE1613 "The selected entity 'M.Color' no longer
// exists". The parser cannot tell an entity from an enumeration for a bare
// `Module.Name` (both arrive as TypeEnumeration without ExplicitEnum), and the
// converter called that an entity unconditionally. A comment promised a
// resolveCodeActionParamType that would settle it against the project; no such
// function existed.

// codeActionProject is a mock project with one module holding an entity
// Customer and an enumeration Color. created collects the Java action and
// JavaScript action writes.
type codeActionProject struct {
	mb   *mock.MockBackend
	h    *ContainerHierarchy
	ja   *javaactions.JavaAction
	jsa  *types.JavaScriptAction
	mod  *model.Module
	enum *model.Enumeration
}

func newCodeActionProject() *codeActionProject {
	p := &codeActionProject{}
	p.mod = mkModule("MyFirstModule")
	p.h = mkHierarchy(p.mod)
	p.enum = mkEnumeration(p.mod.ID, "Color", "Red", "Green")
	dm := mkDomainModel(p.mod.ID, mkEntity(p.mod.ID, "Customer"))
	p.mb = &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListModulesFunc:           func() ([]*model.Module, error) { return []*model.Module{p.mod}, nil },
		ListEnumerationsFunc:      func() ([]*model.Enumeration, error) { return []*model.Enumeration{p.enum}, nil },
		ListDomainModelsFunc:      func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		ListJavaActionsFunc:       func() ([]*types.JavaAction, error) { return nil, nil },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return nil, nil },
		CreateJavaActionFunc: func(ja *javaactions.JavaAction) error {
			p.ja = ja
			return nil
		},
		CreateJavaScriptActionFunc: func(jsa *types.JavaScriptAction) error {
			p.jsa = jsa
			return nil
		},
		WriteJavaSourceFileFunc: func(string, string, string, []*javaactions.JavaActionParameter,
			javaactions.CodeActionReturnType, []string, string, string) error {
			return nil
		},
		WriteJavaScriptSourceFileFunc: func(string, string, string, []*types.JavaActionParameter, types.CodeActionReturnType) error {
			return nil
		},
	}
	return p
}

func TestCreateJavaAction_BareEnumerationNameIsAnEnumeration(t *testing.T) {
	p := newCodeActionProject()
	ctx, _ := newMockCtx(t, withBackend(p.mb), withHierarchy(p.h))

	s := parseOne[*ast.CreateJavaActionStmt](t, `create java action MyFirstModule.JA_Paint (
    C: MyFirstModule.Color,
    Who: MyFirstModule.Customer
) returns MyFirstModule.Color
as $$
return null;
$$;`)
	assertNoError(t, execCreateJavaAction(ctx, s))
	if p.ja == nil || len(p.ja.Parameters) != 2 {
		t.Fatalf("CreateJavaAction not called with two parameters: %+v", p.ja)
	}

	et, ok := p.ja.Parameters[0].ParameterType.(*javaactions.EnumerationType)
	if !ok {
		t.Fatalf("parameter C is %T, want *EnumerationType — an enumeration written as an entity is CE1613",
			p.ja.Parameters[0].ParameterType)
	}
	if et.Enumeration != "MyFirstModule.Color" {
		t.Errorf("parameter C enumeration = %q", et.Enumeration)
	}
	// The other half of the ambiguity: a bare name that IS an entity stays one.
	if en, ok := p.ja.Parameters[1].ParameterType.(*javaactions.EntityType); !ok || en.Entity != "MyFirstModule.Customer" {
		t.Errorf("parameter Who = %#v, want EntityType MyFirstModule.Customer", p.ja.Parameters[1].ParameterType)
	}
	rt, ok := p.ja.ReturnType.(*javaactions.EnumerationType)
	if !ok || rt.Enumeration != "MyFirstModule.Color" {
		t.Errorf("return type = %#v, want EnumerationType MyFirstModule.Color", p.ja.ReturnType)
	}
}

func TestCreateJavaAction_BareNameThatIsNeitherIsRefused(t *testing.T) {
	p := newCodeActionProject()
	ctx, _ := newMockCtx(t, withBackend(p.mb), withHierarchy(p.h))

	s := parseOne[*ast.CreateJavaActionStmt](t, `create java action MyFirstModule.JA_Ghost (
    G: MyFirstModule.NoSuchThing
) returns Boolean
as $$
return true;
$$;`)
	err := execCreateJavaAction(ctx, s)
	if err == nil {
		t.Fatal("expected a refusal for a type that is neither an entity nor an enumeration")
	}
	if !strings.Contains(err.Error(), "MyFirstModule.NoSuchThing") {
		t.Errorf("refusal does not name the type: %v", err)
	}
	if p.ja != nil {
		t.Error("the action was written despite the refusal")
	}
}

func TestCreateJavaScriptAction_BareEnumerationNameIsAnEnumeration(t *testing.T) {
	p := newCodeActionProject()
	ctx, _ := newMockCtx(t, withBackend(p.mb), withHierarchy(p.h))

	s := parseOne[*ast.CreateJavaScriptActionStmt](t, `create javascript action MyFirstModule.JSA_Paint (
    C: MyFirstModule.Color
) returns MyFirstModule.Color
as $$
return null;
$$;`)
	assertNoError(t, execCreateJavaScriptAction(ctx, s))
	if p.jsa == nil || len(p.jsa.Parameters) != 1 {
		t.Fatalf("CreateJavaScriptAction not called with one parameter: %+v", p.jsa)
	}
	if et, ok := p.jsa.Parameters[0].ParameterType.(*types.EnumerationType); !ok || et.Enumeration != "MyFirstModule.Color" {
		t.Errorf("parameter C = %#v, want EnumerationType MyFirstModule.Color", p.jsa.Parameters[0].ParameterType)
	}
	if et, ok := p.jsa.ReturnType.(*types.EnumerationType); !ok || et.Enumeration != "MyFirstModule.Color" {
		t.Errorf("return type = %#v, want EnumerationType MyFirstModule.Color", p.jsa.ReturnType)
	}
}

func TestCreateJavaScriptAction_BareNameThatIsNeitherIsRefused(t *testing.T) {
	p := newCodeActionProject()
	ctx, _ := newMockCtx(t, withBackend(p.mb), withHierarchy(p.h))

	s := parseOne[*ast.CreateJavaScriptActionStmt](t, `create javascript action MyFirstModule.JSA_Ghost ()
returns MyFirstModule.NoSuchThing
as $$
return null;
$$;`)
	if err := execCreateJavaScriptAction(ctx, s); err == nil || !strings.Contains(err.Error(), "MyFirstModule.NoSuchThing") {
		t.Fatalf("expected a refusal naming MyFirstModule.NoSuchThing, got %v", err)
	}
	if p.jsa != nil {
		t.Error("the action was written despite the refusal")
	}
}

// check --references gives exec's verdict: the name resolves when it is an
// entity or an enumeration — of the project or created earlier in the script —
// and is refused otherwise.
func TestValidate_CodeActionSignatureTypes(t *testing.T) {
	p := newCodeActionProject()
	ctx, _ := newMockCtx(t, withBackend(p.mb), withHierarchy(p.h))

	prog, errs := visitor.Build(`create enumeration MyFirstModule.Shade (Light 'Light');
create java action MyFirstModule.JA_Ok (
    C: MyFirstModule.Color, S: MyFirstModule.Shade, Who: MyFirstModule.Customer
) returns MyFirstModule.Color as $$ return null; $$;
create java action MyFirstModule.JA_Ghost (G: MyFirstModule.NoSuchThing) returns Boolean as $$ return true; $$;
create javascript action MyFirstModule.JSA_Ghost () returns MyFirstModule.Nope as $$ return null; $$;`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	sc := newScriptContext()
	sc.collectDefinitions(prog)

	if err := validateWithContext(ctx, prog.Statements[1], sc); err != nil {
		t.Errorf("JA_Ok: unexpected reference error: %v", err)
	}
	if err := validateWithContext(ctx, prog.Statements[2], sc); err == nil || !strings.Contains(err.Error(), "MyFirstModule.NoSuchThing") {
		t.Errorf("JA_Ghost: want a reference error naming MyFirstModule.NoSuchThing, got %v", err)
	}
	if err := validateWithContext(ctx, prog.Statements[3], sc); err == nil || !strings.Contains(err.Error(), "MyFirstModule.Nope") {
		t.Errorf("JSA_Ghost: want a reference error naming MyFirstModule.Nope, got %v", err)
	}
}
