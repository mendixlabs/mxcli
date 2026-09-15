// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `CHANGE $Order ("IsArchived" = true)` — no such attribute — passed
// `check --references`, passed `exec`, and surfaced as CE1613 at the far end of
// a build (mendixlabs/mxcli#1048). Reference checking resolved the entity and
// stopped there.

// memberFixture builds Shop.Order (OrderNo, Status) extending Shop.Base (Code),
// plus an association, plus a second module whose domain model CANNOT be read —
// the backend-cannot-answer case the three-valued resolver exists for.
//
// Two shapes exist for the association resolver specifically, because a fixture
// of flat single-module entities cannot tell a correct resolver from one that
// compares two names: Shop.Base_Region is declared on the GENERALIZATION (so
// Shop.Order has it only by inheritance) and Shop.Order_Bin is CROSS-MODULE (so
// it lives in Shop's CrossAssociations with its far end held by name).
func memberFixture(t *testing.T) *ExecContext {
	t.Helper()
	shop := &model.Module{Name: "Shop"}
	shop.ID = nextID("shop")
	opaque := &model.Module{Name: "Opaque"}
	opaque.ID = nextID("opaque")
	warehouse := &model.Module{Name: "Warehouse"}
	warehouse.ID = nextID("warehouse")

	mkAttr := func(name string) *domainmodel.Attribute {
		a := &domainmodel.Attribute{Name: name, Type: &domainmodel.StringAttributeType{}}
		a.ID = nextID("attr" + name)
		return a
	}

	base := &domainmodel.Entity{Name: "Base", Persistable: true}
	base.ID = nextID("base")
	base.Attributes = []*domainmodel.Attribute{mkAttr("Code")}

	order := &domainmodel.Entity{Name: "Order", Persistable: true, GeneralizationRef: "Shop.Base"}
	order.ID = nextID("order")
	order.Attributes = []*domainmodel.Attribute{mkAttr("OrderNo"), mkAttr("Status")}

	customer := &domainmodel.Entity{Name: "Customer", Persistable: true}
	customer.ID = nextID("cust")
	customer.Attributes = []*domainmodel.Attribute{mkAttr("Name")}

	assoc := &domainmodel.Association{Name: "Order_Customer", ParentID: order.ID, ChildID: customer.ID}
	assoc.ID = nextID("assoc")

	// Declared on the GENERALIZATION. Shop.Order has it, and only through the
	// chain — exactly the shape that made `check` reject every page constraining
	// an Administration.Account on System.UserRoles.
	region := &domainmodel.Entity{Name: "Region", Persistable: true}
	region.ID = nextID("region")
	region.Attributes = []*domainmodel.Attribute{mkAttr("RegionName")}

	inherited := &domainmodel.Association{Name: "Base_Region", ParentID: base.ID, ChildID: region.ID}
	inherited.ID = nextID("inherited")

	// Cross-module: stored in the FROM entity's module, far end BY NAME.
	bin := &domainmodel.Entity{Name: "Bin", Persistable: true}
	bin.ID = nextID("bin")
	bin.Attributes = []*domainmodel.Attribute{mkAttr("BinCode")}

	cross := &domainmodel.CrossModuleAssociation{
		Name: "Order_Bin", ParentID: order.ID, ChildRef: "Warehouse.Bin"}
	cross.ID = nextID("cross")

	// An entity whose generalization lives in a module the backend refuses to
	// answer for. Its own attributes resolve; anything else is unknowable.
	imported := &domainmodel.Entity{Name: "Imported", Persistable: true, GeneralizationRef: "Opaque.Thing"}
	imported.ID = nextID("imported")
	imported.Attributes = []*domainmodel.Attribute{mkAttr("LocalOnly")}

	dm := &domainmodel.DomainModel{ContainerID: shop.ID,
		Entities:          []*domainmodel.Entity{base, order, customer, imported, region},
		Associations:      []*domainmodel.Association{assoc, inherited},
		CrossAssociations: []*domainmodel.CrossModuleAssociation{cross},
	}
	dm.ID = nextID("dm")

	wdm := &domainmodel.DomainModel{ContainerID: warehouse.ID,
		Entities: []*domainmodel.Entity{bin},
	}
	wdm.ID = nextID("wdm")

	h := mkHierarchy(shop, warehouse)
	withContainer(h, dm.ID, shop.ID)
	withContainer(h, wdm.ID, warehouse.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{shop, opaque, warehouse}, nil
		},
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			switch name {
			case "Shop":
				return shop, nil
			case "Opaque":
				return opaque, nil
			case "Warehouse":
				return warehouse, nil
			}
			return nil, fmt.Errorf("no module %q", name)
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{dm, wdm}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			switch id {
			case shop.ID:
				return dm, nil
			case warehouse.ID:
				return wdm, nil
			}
			return nil, fmt.Errorf("domain model %s cannot be read", id)
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func changeStmt(variable string, attrs ...string) *ast.ChangeObjectStmt {
	s := &ast.ChangeObjectStmt{Variable: variable}
	for _, a := range attrs {
		s.Changes = append(s.Changes, ast.ChangeItem{Attribute: a})
	}
	return s
}

func flowProgram(params []ast.MicroflowParam, body ...ast.MicroflowStatement) *ast.Program {
	return &ast.Program{Statements: []ast.Statement{
		&ast.CreateMicroflowStmt{
			Name:       ast.QualifiedName{Module: "Shop", Name: "ACT_Probe"},
			Parameters: params,
			Body:       body,
		},
	}}
}

func orderParam() []ast.MicroflowParam {
	return []ast.MicroflowParam{{
		Name: "Order",
		Type: ast.DataType{EntityRef: &ast.QualifiedName{Module: "Shop", Name: "Order"}},
	}}
}

func TestValidateMemberReferences_ReportsAMissingAttribute(t *testing.T) {
	ctx := memberFixture(t)
	msgs := validateMemberReferences(ctx,
		flowProgram(orderParam(), changeStmt("Order", `"IsArchived"`)), newScriptContext())

	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1: %v", len(msgs), msgs)
	}
	for _, want := range []string{"Shop.Order", "IsArchived", "CE1613"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("message should mention %q: %s", want, msgs[0])
		}
	}
	// A typo is cheap to fix only when the error shows what is there.
	if !strings.Contains(msgs[0], "OrderNo") {
		t.Errorf("message should offer the entity's real members: %s", msgs[0])
	}
}

// CONTROL: everything that legitimately resolves must stay silent. Each of
// these is a shape a real script uses, and each would be a false error that
// blocks a script which builds cleanly.
func TestValidateMemberReferences_AcceptsWhatResolves(t *testing.T) {
	ctx := memberFixture(t)

	for _, tc := range []struct {
		name string
		prog *ast.Program
		sc   *scriptContext
	}{
		{
			name: "an attribute the entity declares",
			prog: flowProgram(orderParam(), changeStmt("Order", `"Status"`)),
			sc:   newScriptContext(),
		},
		{
			// The walk has to follow GeneralizationRef, or every specialisation
			// reports its inherited members as missing.
			name: "an attribute INHERITED from the generalization",
			prog: flowProgram(orderParam(), changeStmt("Order", `"Code"`)),
			sc:   newScriptContext(),
		},
		{
			// An association is a legal member of a CREATE/CHANGE, and it is not
			// in the entity's attribute list.
			name: "an association as a member",
			prog: flowProgram(orderParam(), changeStmt("Order", "Order_Customer")),
			sc:   newScriptContext(),
		},
		{
			// exec already refuses the one-qualifier form that cannot be an
			// attribute (FINDINGS #51); this check leaves qualified names alone
			// rather than second-guessing that.
			name: "a qualified member",
			prog: flowProgram(orderParam(), changeStmt("Order", "Shop.Order_Customer")),
			sc:   newScriptContext(),
		},
		{
			name: "a variable this walk cannot type",
			prog: flowProgram(nil, changeStmt("Mystery", `"Whatever"`)),
			sc:   newScriptContext(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if msgs := validateMemberReferences(ctx, tc.prog, tc.sc); len(msgs) != 0 {
				t.Errorf("reported a member that resolves: %v", msgs)
			}
		})
	}
}

// The migration shape — add the column, then populate it — is the change most
// likely to name a member the STORED project does not have yet. Reporting it
// would break the scripts this check is supposed to help.
func TestValidateMemberReferences_CountsMembersTheScriptAuthors(t *testing.T) {
	ctx := memberFixture(t)

	prog := &ast.Program{Statements: []ast.Statement{
		&ast.AlterEntityStmt{
			Name:      ast.QualifiedName{Module: "Shop", Name: "Order"},
			Operation: ast.AlterEntityAddAttribute,
			Attribute: &ast.Attribute{Name: "Archived"},
		},
		&ast.CreateMicroflowStmt{
			Name:       ast.QualifiedName{Module: "Shop", Name: "ACT_Probe"},
			Parameters: orderParam(),
			Body:       []ast.MicroflowStatement{changeStmt("Order", `"Archived"`)},
		},
	}}
	if msgs := validateMemberReferences(ctx, prog, newScriptContext()); len(msgs) != 0 {
		t.Errorf("reported an attribute the script adds earlier: %v", msgs)
	}

	// CONTROL: the guard is scoped to what the script actually authors — a
	// different name on the same entity is still reported.
	prog.Statements[1].(*ast.CreateMicroflowStmt).Body =
		[]ast.MicroflowStatement{changeStmt("Order", `"Archivd"`)}
	if msgs := validateMemberReferences(ctx, prog, newScriptContext()); len(msgs) != 1 {
		t.Errorf("a typo beside an authored attribute went unreported: %v", msgs)
	}
}

// An entity the script creates is not in the stored project, so the member is
// unknowable rather than missing — and that falls out of the resolver rather
// than needing a guard of its own. An explicit "skip entities the script
// creates" guard used to sit here and cost the check most of its reach: a
// self-contained script defines its own entities, so the guard skipped the whole
// file, including the example written to demonstrate the check.
func TestValidateMemberReferences_SkipsAnEntityTheScriptCreates(t *testing.T) {
	ctx := memberFixture(t)
	sc := newScriptContext()
	sc.entities["Shop.Fresh"] = true

	prog := flowProgram(nil, &ast.CreateObjectStmt{
		Variable:   "F",
		EntityType: ast.QualifiedName{Module: "Shop", Name: "Fresh"},
		Changes:    []ast.ChangeItem{{Attribute: `"Anything"`}},
	})
	if msgs := validateMemberReferences(ctx, prog, sc); len(msgs) != 0 {
		t.Errorf("reported a member on an entity the script creates: %v", msgs)
	}
	if got := resolveMemberOnEntity(ctx, "Shop.Fresh", "Anything"); got != memberUnknown {
		t.Errorf("an entity absent from the project = %v, want memberUnknown", got)
	}

	// CONTROL: an entity the script creates that DOES already exist in the
	// project is checked normally — the script declaring it is not a licence to
	// stop resolving its members.
	prog2 := flowProgram(nil, &ast.CreateObjectStmt{
		Variable:   "O",
		EntityType: ast.QualifiedName{Module: "Shop", Name: "Order"},
		Changes:    []ast.ChangeItem{{Attribute: `"Nonsense"`}},
	})
	sc2 := newScriptContext()
	sc2.entities["Shop.Order"] = true
	if msgs := validateMemberReferences(ctx, prog2, sc2); len(msgs) != 1 {
		t.Errorf("a stored entity restated by the script went unchecked: %v", msgs)
	}
}

// THE CONTROL the package comment names. A backend that cannot answer must
// produce silence, not a report — mxcli has more than one backend and every
// lookup in the interface may return an error. Shop.Imported's generalization
// lives in a module whose domain model the fixture refuses to read, so a name
// that is not among Imported's own attributes is unknowable, not missing.
//
// Collapse memberUnknown into memberMissing (a two-valued resolver, which is
// what exec uses) and this reports.
func TestResolveMemberOnEntity_SilentWhenTheModelCannotBeRead(t *testing.T) {
	ctx := memberFixture(t)

	if got := resolveMemberOnEntity(ctx, "Shop.Imported", "LocalOnly"); got != memberFound {
		t.Errorf("an attribute the entity itself declares = %v, want memberFound", got)
	}
	if got := resolveMemberOnEntity(ctx, "Shop.Imported", "CouldBeInherited"); got != memberUnknown {
		t.Errorf("a name past an unreadable generalization = %v, want memberUnknown", got)
	}
	// And nothing is reported for it.
	prog := flowProgram([]ast.MicroflowParam{{
		Name: "Imp",
		Type: ast.DataType{EntityRef: &ast.QualifiedName{Module: "Shop", Name: "Imported"}},
	}}, changeStmt("Imp", `"CouldBeInherited"`))
	if msgs := validateMemberReferences(ctx, prog, newScriptContext()); len(msgs) != 0 {
		t.Errorf("reported a member the backend could not rule out: %v", msgs)
	}
}

// The full chain must be walked before anything is reported: a name on neither
// the entity nor its generalization is the real defect.
func TestResolveMemberOnEntity_WalksTheWholeChain(t *testing.T) {
	ctx := memberFixture(t)
	if got := resolveMemberOnEntity(ctx, "Shop.Order", "Code"); got != memberFound {
		t.Errorf("inherited attribute = %v, want memberFound", got)
	}
	if got := resolveMemberOnEntity(ctx, "Shop.Order", "Nonsense"); got != memberMissing {
		t.Errorf("absent attribute = %v, want memberMissing", got)
	}
}

// A CHANGE on a loop iterator resolves through the list it iterates; without
// that the commonest bulk-update shape goes unchecked.
func TestValidateMemberReferences_TypesALoopIterator(t *testing.T) {
	ctx := memberFixture(t)
	prog := flowProgram(nil,
		&ast.RetrieveStmt{Variable: "Orders",
			Source: ast.QualifiedName{Module: "Shop", Name: "Order"}},
		&ast.LoopStmt{LoopVariable: "O", ListVariable: "Orders",
			Body: []ast.MicroflowStatement{changeStmt("O", `"Bogus"`)}},
	)
	msgs := validateMemberReferences(ctx, prog, newScriptContext())
	if len(msgs) != 1 || !strings.Contains(msgs[0], "Bogus") {
		t.Errorf("a CHANGE on a loop iterator went unchecked: %v", msgs)
	}
}

// An association retrieve feeding a LOOP is the ordinary bulk-update shape, and
// leaving it untyped made the check silent on the commonest place a member is
// named. Both directions resolve, because a retrieve may traverse from either
// end (ParentID is the FROM entity, ChildID the TO — CLAUDE.md).
func TestValidateMemberReferences_TypesAnAssociationRetrieve(t *testing.T) {
	ctx := memberFixture(t)

	// From the Customer end: Order_Customer traversed from Customer gives Orders.
	prog := flowProgram([]ast.MicroflowParam{{
		Name: "Cust",
		Type: ast.DataType{EntityRef: &ast.QualifiedName{Module: "Shop", Name: "Customer"}},
	}},
		&ast.RetrieveStmt{Variable: "Orders", StartVariable: "Cust",
			Source: ast.QualifiedName{Module: "Shop", Name: "Order_Customer"}},
		&ast.LoopStmt{LoopVariable: "O", ListVariable: "Orders",
			Body: []ast.MicroflowStatement{changeStmt("O", `"Bogus"`)}},
	)
	msgs := validateMemberReferences(ctx, prog, newScriptContext())
	if len(msgs) != 1 || !strings.Contains(msgs[0], "Shop.Order") {
		t.Errorf("an association retrieve from the TO end went unchecked: %v", msgs)
	}

	// CONTROL: a real member of the traversed-to entity stays silent, so the
	// direction was resolved rather than merely producing some entity.
	prog.Statements[0].(*ast.CreateMicroflowStmt).Body[1].(*ast.LoopStmt).Body =
		[]ast.MicroflowStatement{changeStmt("O", `"OrderNo"`)}
	if msgs := validateMemberReferences(ctx, prog, newScriptContext()); len(msgs) != 0 {
		t.Errorf("resolved the wrong end of the association: %v", msgs)
	}
}

// CONTROL for the coverage boundary. A variable bound by an activity this walk
// does not model — `send rest request`, `response: file as`, an association
// retrieve from an untyped start — stays untyped, and an untyped target is
// UNCHECKED rather than wrongly checked. Measured on ako/mxcli-rest: every
// CHANGE in its 14 scripts is bound that way, so the check fires there only on
// CREATE OBJECT (which names its entity and is therefore always typed).
//
// This is a real limit, not a bug: widening it means typing more binders, and
// each one is only safe once its output entity is known for certain.
func TestValidateMemberReferences_UntypedTargetIsUnchecked(t *testing.T) {
	ctx := memberFixture(t)
	prog := flowProgram(nil,
		// StartVariable is never bound, so the retrieve yields nothing typed.
		&ast.RetrieveStmt{Variable: "Kids", StartVariable: "FromNowhere",
			Source: ast.QualifiedName{Module: "Shop", Name: "Order_Customer"}},
		&ast.LoopStmt{LoopVariable: "K", ListVariable: "Kids",
			Body: []ast.MicroflowStatement{changeStmt("K", `"DefinitelyNotAMember"`)}},
	)
	if msgs := validateMemberReferences(ctx, prog, newScriptContext()); len(msgs) != 0 {
		t.Errorf("checked a member against an entity it could not establish: %v", msgs)
	}
}
