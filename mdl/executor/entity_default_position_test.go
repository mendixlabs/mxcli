// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// CREATE ENTITY places an entity without an @Position on dmlayout.GridSlot(n),
// n being the number of entities already in the domain model. View entities,
// external entities and entities imported from a contract used to keep the
// pre-grid formula `X: 100 + n*150, Y: 100`: one row, and 150px is narrower
// than an entity box, so neighbouring boxes touched.

// assertGridSlots checks that created[i] sits on GridSlot(first+i) and that no
// two of them share a slot.
func assertGridSlots(t *testing.T, kind string, first int, created []*domainmodel.Entity) {
	t.Helper()
	seen := map[model.Point]string{}
	for i, e := range created {
		want := dmlayout.GridSlot(first + i)
		if e.Location != want {
			t.Errorf("%s %s (#%d in the domain model) is at %+v, want grid slot %+v", kind, e.Name, first+i, e.Location, want)
		}
		if prev, dup := seen[e.Location]; dup {
			t.Errorf("%s %s and %s share the position %+v", kind, prev, e.Name, e.Location)
		}
		seen[e.Location] = e.Name
	}
	// Eight entities on top of two existing ones wrap the six-column grid: the
	// old formula kept every one of them at y=100.
	if len(created) > dmlayout.GridColumns && created[len(created)-1].Location.Y == created[0].Location.Y {
		t.Errorf("%s: %d entities are still on one row (y=%d)", kind, len(created), created[0].Location.Y)
	}
}

// domainModelWith returns a domain model that already holds n entities.
func domainModelWith(mod *model.Module, n int) *domainmodel.DomainModel {
	dm := &domainmodel.DomainModel{}
	dm.ID = nextID("dm")
	dm.ContainerID = mod.ID
	for i := 0; i < n; i++ {
		dm.Entities = append(dm.Entities, mkEntity(mod.ID, fmt.Sprintf("Existing%d", i)))
	}
	return dm
}

func TestCreateViewEntity_DefaultPositionIsTheGridSlot(t *testing.T) {
	const existing, views = 2, 8
	mod := mkModule("Shop")
	dm := domainModelWith(mod, existing)
	var created []*domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			created = append(created, e)
			dm.Entities = append(dm.Entities, e)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	for i := 0; i < views; i++ {
		s := parseViewStmt(t, fmt.Sprintf("create view entity Shop.View%d (\n  A: Integer\n) as (\n  select 1 as A\n);", i))
		if err := execCreateViewEntity(ctx, s); err != nil {
			t.Fatalf("view %d: %v", i, err)
		}
	}
	if len(created) != views {
		t.Fatalf("created %d view entities, want %d", len(created), views)
	}
	assertGridSlots(t, "view entity", existing, created)
}

// A view entity given an @Position keeps it: the grid slot is only a default.
func TestCreateViewEntity_ExplicitPositionIsKept(t *testing.T) {
	mod := mkModule("Shop")
	dm := domainModelWith(mod, 3)
	var created []*domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			created = append(created, e)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	s := parseViewStmt(t, "create view entity Shop.Pinned (\n  A: Integer\n) as (\n  select 1 as A\n);")
	s.Position = &ast.Position{X: 777, Y: 333}
	if err := execCreateViewEntity(ctx, s); err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || created[0].Location != (model.Point{X: 777, Y: 333}) {
		t.Fatalf("explicit @Position was not kept: %+v", created)
	}
}

func TestCreateExternalEntity_DefaultPositionIsTheGridSlot(t *testing.T) {
	const existing, externals = 2, 8
	mod := mkModule("Clients")
	h := mkHierarchy(mod)
	svc := &model.ConsumedODataService{BaseElement: model.BaseElement{ID: nextID("cos")}, ContainerID: mod.ID, Name: "OrderODataClient"}
	withContainer(h, svc.ContainerID, mod.ID)
	dm := domainModelWith(mod, existing)
	var created []*domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc:               func() bool { return true },
		ListModulesFunc:               func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc:            func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) { return []*model.ConsumedODataService{svc}, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			created = append(created, e)
			dm.Entities = append(dm.Entities, e)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	for i := 0; i < externals; i++ {
		set := fmt.Sprintf("Orders%d", i)
		s := &ast.CreateExternalEntityStmt{
			Name:       ast.QualifiedName{Module: "Clients", Name: set},
			ServiceRef: ast.QualifiedName{Module: "Clients", Name: "OrderODataClient"},
			EntitySet:  &set,
			Attributes: []ast.Attribute{{Name: "OrderId", Type: ast.DataType{Kind: ast.TypeLong}}},
		}
		assertNoError(t, execCreateExternalEntity(ctx, s))
	}
	if len(created) != externals {
		t.Fatalf("created %d external entities, want %d", len(created), externals)
	}
	assertGridSlots(t, "external entity", existing, created)
}

// manyTypesMetadata is a contract with n entity types and no associations.
func manyTypesMetadata(n int) string {
	var entityTypes, sets strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&entityTypes, `<EntityType Name="Type%d"><Key><PropertyRef Name="Id"/></Key><Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/></EntityType>`, i)
		fmt.Fprintf(&sets, `<EntitySet Name="Type%d" EntityType="App.Model.Type%d"/>`, i, i)
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx"><edmx:DataServices>
<Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">` + entityTypes.String() +
		`<EntityContainer Name="Container">` + sets.String() + `</EntityContainer></Schema></edmx:DataServices></edmx:Edmx>`
}

// Entities imported from a contract go on the grid after the entities the
// module already holds — the running count is the module's, not just the
// import's, or the first imported entity would land on top of an existing one.
func TestCreateExternalEntities_DefaultPositionIsTheGridSlot(t *testing.T) {
	const existing, imported = 2, 8
	mod := mkModule("CustomModule")
	svc := &model.ConsumedODataService{
		BaseElement:  model.BaseElement{ID: nextID("cos")},
		ContainerID:  mod.ID,
		Name:         "App",
		MetadataUrl:  "https://example.com/$metadata",
		ODataVersion: "4.0",
		Metadata:     manyTypesMetadata(imported),
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)
	dm := domainModelWith(mod, existing)

	var created []*domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			created = append(created, e)
			dm.Entities = append(dm.Entities, e)
			return nil
		},
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: 11, MinorVersion: 12, ProductVersion: "11.12.0"}
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateExternalEntitiesStmt{
		ServiceRef:     ast.QualifiedName{Module: "CustomModule", Name: "App"},
		TargetModule:   "CustomModule",
		CreateOrModify: true,
	}
	assertNoError(t, createExternalEntities(ctx, stmt))
	if len(created) != imported {
		t.Fatalf("imported %d entities, want %d", len(created), imported)
	}
	assertGridSlots(t, "contract entity", existing, created)
}
