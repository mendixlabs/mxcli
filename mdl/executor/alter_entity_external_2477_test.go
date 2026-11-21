// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#2477: ADD ATTRIBUTE on an external entity was accepted by check and
// exec, and stored a DomainModels$StoredValue where an external entity's
// attribute is a Rest$ODataMappedValue. mx check 11.12.6: CE6612 "Attribute
// 'Status' of external entity 'Order' is not supported." for String(unlimited)
// and for Enumeration(...) alike, and with AllowCreateChangeLocally too.
func externalAlterTestCtx(t *testing.T) (*ExecContext, *bool) {
	t.Helper()
	mod := mkModule("Shop")
	dmID := nextID("dm")
	order := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")}, ContainerID: dmID, Name: "Order",
		Persistable: true, Source: "Rest$ODataRemoteEntitySource", RemoteServiceName: "Shop.Sales",
		Attributes: []*domainmodel.Attribute{{Name: "Title"}},
	}
	local := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")}, ContainerID: dmID, Name: "Note",
		Persistable: true, Attributes: []*domainmodel.Attribute{{Name: "Text"}},
	}
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: dmID}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{order, local}}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	updated := false
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateEntityFunc:     func(dmID model.ID, e *domainmodel.Entity) error { updated = true; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &updated
}

func addAttr2477(entity string) *ast.AlterEntityStmt {
	return &ast.AlterEntityStmt{Name: ast.QualifiedName{Module: "Shop", Name: entity}, Operation: ast.AlterEntityAddAttribute,
		Attribute: &ast.Attribute{Name: "Status", Type: ast.DataType{Kind: ast.TypeString}}}
}

func TestAlterEntity_RefusesAddAttributeOnExternalEntity_2477(t *testing.T) {
	ctx, updated := externalAlterTestCtx(t)
	err := execAlterEntity(ctx, addAttr2477("Order"))
	if err == nil {
		t.Fatal("ADD ATTRIBUTE on an external entity was accepted — mxbuild reports CE6612")
	}
	if *updated {
		t.Error("the entity was written despite the refusal")
	}
	for _, want := range []string{"CE6612", "create or modify external entities from Shop.Sales"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q:\n%s", want, err)
		}
	}
	// check says it too, before exec would.
	ctx2, _ := externalAlterTestCtx(t)
	if err := validateWithContext(ctx2, addAttr2477("Order"), newScriptContext()); err == nil || !strings.Contains(err.Error(), "CE6612") {
		t.Errorf("check: %v, want the CE6612 refusal", err)
	}
}

// Control: the same statement on a local entity goes through.
func TestAlterEntity_AddAttributeOnLocalEntityStillWorks_2477(t *testing.T) {
	ctx, updated := externalAlterTestCtx(t)
	if err := execAlterEntity(ctx, addAttr2477("Note")); err != nil {
		t.Fatalf("local entity: %v", err)
	}
	if !*updated {
		t.Error("the local entity was not written")
	}
	ctx2, _ := externalAlterTestCtx(t)
	if err := validateWithContext(ctx2, addAttr2477("Note"), newScriptContext()); err != nil {
		t.Errorf("check refused a local entity: %v", err)
	}
}
