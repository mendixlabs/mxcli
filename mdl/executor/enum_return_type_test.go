// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A bare `Module.Name` return type is ambiguous: the parser cannot tell an
// entity from an enumeration and records an entity reference. exec already
// resolved that for PARAMETERS (entity first, then enumeration); the return
// types of a microflow, a nanoflow and a rule did not, so a script that
// `check` accepted failed at exec with "entity 'Shop.Variant' not found for
// return type".

// enumReturnCapture records what the create handlers hand to the backend.
type enumReturnCapture struct {
	microflow *microflows.Microflow
	nanoflow  *microflows.Nanoflow
	rule      *microflows.Rule
}

// enumReturnCtx builds a mock project with module Shop, an enumeration
// Shop.Variant, an entity Shop.Customer and, when twin is set, an entity that
// shares the enumeration's name.
func enumReturnCtx(t *testing.T, twin bool) (*ExecContext, *enumReturnCapture) {
	t.Helper()
	mod := mkModule("Shop")
	enum := mkEnumeration(mod.ID, "Variant", "Original", "Released")
	ents := []*domainmodel.Entity{mkEntity(mod.ID, "Customer")}
	if twin {
		ents = append(ents, mkEntity(mod.ID, "Variant"))
	}
	dm := mkDomainModel(mod.ID, ents...)
	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	got := &enumReturnCapture{}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListRulesFunc:        func() ([]*microflows.Rule, error) { return nil, nil },
		CreateMicroflowFunc:  func(mf *microflows.Microflow) error { got.microflow = mf; return nil },
		CreateNanoflowFunc:   func(nf *microflows.Nanoflow) error { got.nanoflow = nf; return nil },
		CreateRuleFunc:       func(r *microflows.Rule) error { got.rule = r; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, got
}

// enumReturnFlows are the three declaration sites, each with the return type
// spelled as %s.
var enumReturnFlows = map[string]string{
	"microflow": "create or modify microflow Shop.SUB_Pick () returns %s as $Variant begin return Shop.Variant.Original; end;",
	"nanoflow":  "create or modify nanoflow Shop.NF_Pick () returns %s as $Variant begin return Shop.Variant.Original; end;",
	"rule":      "create or modify rule Shop.RL_Pick ($c: Shop.Customer) returns %s as $Variant begin return Shop.Variant.Original; end;",
}

// execEnumReturn parses and executes one flow declaration.
func execEnumReturn(t *testing.T, ctx *ExecContext, kind, returnSpelling string) error {
	t.Helper()
	src := "mdl 1;\n" + fmt.Sprintf(enumReturnFlows[kind], returnSpelling)
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	switch s := prog.Statements[0].(type) {
	case *ast.CreateMicroflowStmt:
		return execCreateMicroflow(ctx, s)
	case *ast.CreateNanoflowStmt:
		return execCreateNanoflow(ctx, s)
	case *ast.CreateRuleStmt:
		return execCreateRule(ctx, s)
	default:
		t.Fatalf("unexpected statement %T", s)
		return nil
	}
}

func (c *enumReturnCapture) returnType(kind string) microflows.DataType {
	switch kind {
	case "microflow":
		return c.microflow.ReturnType
	case "nanoflow":
		return c.nanoflow.ReturnType
	default:
		return c.rule.ReturnType
	}
}

func TestBareEnumReturnType_StoredAsEnumeration(t *testing.T) {
	for kind := range enumReturnFlows {
		t.Run(kind, func(t *testing.T) {
			ctx, got := enumReturnCtx(t, false)
			if err := execEnumReturn(t, ctx, kind, "Shop.Variant"); err != nil {
				t.Fatalf("bare enumeration return type must be accepted: %v", err)
			}
			et, ok := got.returnType(kind).(*microflows.EnumerationType)
			if !ok {
				t.Fatalf("return type = %T, want *EnumerationType", got.returnType(kind))
			}
			if et.EnumerationQualifiedName != "Shop.Variant" {
				t.Errorf("enumeration = %q, want Shop.Variant", et.EnumerationQualifiedName)
			}
		})
	}
}

// The bare spelling must store exactly what the explicit spellings store.
func TestBareEnumReturnType_EqualsExplicitSpellings(t *testing.T) {
	for kind := range enumReturnFlows {
		t.Run(kind, func(t *testing.T) {
			var stored []*microflows.EnumerationType
			for _, spelling := range []string{"enum Shop.Variant", "Enumeration(Shop.Variant)", "Shop.Variant"} {
				ctx, got := enumReturnCtx(t, false)
				if err := execEnumReturn(t, ctx, kind, spelling); err != nil {
					t.Fatalf("%q: %v", spelling, err)
				}
				et, ok := got.returnType(kind).(*microflows.EnumerationType)
				if !ok {
					t.Fatalf("%q: return type = %T, want *EnumerationType", spelling, got.returnType(kind))
				}
				stored = append(stored, et)
			}
			for i := 1; i < len(stored); i++ {
				if stored[i].EnumerationQualifiedName != stored[0].EnumerationQualifiedName {
					t.Errorf("spelling %d stored %+v, explicit spelling stored %+v", i, stored[i], stored[0])
				}
			}
		})
	}
}

// An unknown name names both candidates, so the user is not told to look for an
// entity when they may have meant an enumeration.
func TestBareEnumReturnType_UnknownName(t *testing.T) {
	for kind := range enumReturnFlows {
		t.Run(kind, func(t *testing.T) {
			ctx, _ := enumReturnCtx(t, false)
			err := execEnumReturn(t, ctx, kind, "Shop.Nope")
			assertError(t, err)
			assertContainsStr(t, err.Error(), "entity or enumeration 'Shop.Nope' not found for return type")
		})
	}
}

// A bare name that is an entity stays an entity, and wins when an entity and an
// enumeration share the name — the order the parameter path already has.
func TestBareEnumReturnType_EntityStaysEntity(t *testing.T) {
	for _, kind := range []string{"microflow", "nanoflow"} {
		t.Run(kind, func(t *testing.T) {
			// Shop.Customer is an entity only.
			ctx, got := enumReturnCtx(t, false)
			if err := execEnumReturn(t, ctx, kind, "Shop.Customer"); err != nil {
				t.Fatalf("entity return type: %v", err)
			}
			ot, ok := got.returnType(kind).(*microflows.ObjectType)
			if !ok || ot.EntityQualifiedName != "Shop.Customer" {
				t.Fatalf("return type = %#v, want ObjectType Shop.Customer", got.returnType(kind))
			}

			// Shop.Variant is both an entity and an enumeration: the entity wins.
			ctx, got = enumReturnCtx(t, true)
			if err := execEnumReturn(t, ctx, kind, "Shop.Variant"); err != nil {
				t.Fatalf("twin return type: %v", err)
			}
			if ot, ok := got.returnType(kind).(*microflows.ObjectType); !ok || ot.EntityQualifiedName != "Shop.Variant" {
				t.Errorf("return type = %#v, want ObjectType Shop.Variant (entity wins)", got.returnType(kind))
			}
		})
	}
}

// The parameter path is the model for the return path: the same bare name must
// resolve the same way in either position.
func TestBareEnumParameter_StillResolves(t *testing.T) {
	ctx, got := enumReturnCtx(t, false)
	src := "mdl 1;\ncreate or modify microflow Shop.SUB_Take ($v: Shop.Variant) begin return; end;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if err := execCreateMicroflow(ctx, prog.Statements[0].(*ast.CreateMicroflowStmt)); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.microflow.Parameters[0].Type.(*microflows.EnumerationType); !ok {
		t.Fatalf("parameter type = %T, want *EnumerationType", got.microflow.Parameters[0].Type)
	}
}
