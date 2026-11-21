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
)

// ako/mxcli#2497: the hand-written `create external entity … (attrs)` stored
// every attribute as a DomainModels$StoredValue, no key, and TopSupported /
// SkipSupported false. mx check 11.12.6 on a contract the bulk import builds at
// 0 errors: CE6010 "Key cannot be empty", CE6612 on every attribute, and two
// CE6630 ($top, $skip). The attributes now come from the contract, as the bulk
// import makes them.
const salesContract2497 = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Vendor.Sales" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EnumType Name="OrderStatus"><Member Name="Open" Value="0"/><Member Name="Shipped" Value="1"/></EnumType>
      <EntityType Name="Order">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.Int64" Nullable="false"/>
        <Property Name="Title" Type="Edm.String" MaxLength="100"/>
        <Property Name="Status" Type="Vendor.Sales.OrderStatus"/>
      </EntityType>
      <EntityContainer Name="Container"><EntitySet Name="Orders" EntityType="Vendor.Sales.Order"/></EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

type singleImport2497 struct {
	created map[string]*domainmodel.Entity
	updated map[string]*domainmodel.Entity
	enums   []*model.Enumeration
}

func runSingle2497(t *testing.T, src string, existing ...*domainmodel.Entity) (singleImport2497, error) {
	t.Helper()
	mod := mkModule("Shop")
	svc := &model.ConsumedODataService{BaseElement: model.BaseElement{ID: nextID("cos")}, ContainerID: mod.ID,
		Name: "Sales", MetadataUrl: "http://localhost/$metadata", ODataVersion: "4.0", Metadata: salesContract2497}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)
	dm := &domainmodel.DomainModel{Entities: existing}
	dm.ID = nextID("dm")
	r := singleImport2497{created: map[string]*domainmodel.Entity{}, updated: map[string]*domainmodel.Entity{}}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return r.enums, nil },
		CreateEnumerationFunc: func(e *model.Enumeration) error {
			e.ContainerID = mod.ID
			r.enums = append(r.enums, e)
			return nil
		},
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error { r.created[e.Name] = e; return nil },
		UpdateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error { r.updated[e.Name] = e; return nil },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: 11, MinorVersion: 12, ProductVersion: "11.12.6"}
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return r, execCreateExternalEntity(ctx, prog.Statements[0].(*ast.CreateExternalEntityStmt))
}

const singleOrder2497 = `create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders') (
  OrderId: Long,
  Title: String(100),
  Status: Enumeration(Shop.OrderStatus)
);`

func TestSingleExternalEntityMapsTheContract_2497(t *testing.T) {
	r, err := runSingle2497(t, singleOrder2497)
	if err != nil {
		t.Fatal(err)
	}
	e := r.created["Order"]
	if e == nil {
		t.Fatal("not created")
	}
	// CE6010: the key.
	if len(e.RemoteKeyParts) != 1 || e.RemoteKeyParts[0].RemoteName != "Id" || e.RemoteKeyParts[0].Name != "OrderId" {
		t.Errorf("key = %+v", e.RemoteKeyParts)
	}
	// CE6612: each attribute mapped to its property.
	want := map[string]string{"OrderId": "Id/Edm.Int64", "Title": "Title/Edm.String", "Status": "Status/Vendor.Sales.OrderStatus"}
	for _, a := range e.Attributes {
		if got := a.RemoteName + "/" + a.RemoteType; got != want[a.Name] {
			t.Errorf("%s mapped to %q, want %q", a.Name, got, want[a.Name])
		}
		delete(want, a.Name)
	}
	if len(want) != 0 {
		t.Errorf("attributes missing: %v", want)
	}
	// CE6630: the contract's capabilities.
	if !e.TopSupported || !e.SkipSupported || e.RemoteEntityName != "Order" || e.Source != "Rest$ODataRemoteEntitySource" {
		t.Errorf("entity = top %v skip %v remote %q source %q", e.TopSupported, e.SkipSupported, e.RemoteEntityName, e.Source)
	}
	// The enum attribute's enumeration is generated, as the bulk import does it.
	if len(r.enums) != 1 || r.enums[0].RemoteName != "Vendor.Sales.OrderStatus" {
		t.Errorf("enumerations = %+v", r.enums)
	}
}

// A subset of the properties is kept as stated; a property the statement does
// not name is not imported, and its enumeration is not generated.
func TestSingleExternalEntityKeepsTheStatedSubset_2497(t *testing.T) {
	r, err := runSingle2497(t, `create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders') (
  OrderId: Long
);`)
	if err != nil {
		t.Fatal(err)
	}
	if e := r.created["Order"]; e == nil || len(e.Attributes) != 1 || e.Attributes[0].Name != "OrderId" {
		t.Fatalf("created = %+v", e)
	}
	if len(r.enums) != 0 {
		t.Errorf("an enumeration was generated for a property not imported: %+v", r.enums)
	}
}

func TestSingleExternalEntityRefusesWhatCannotBuild_2497(t *testing.T) {
	for name, c := range map[string]struct{ src, want string }{
		"not in the contract": {`create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders') (
  OrderId: Long, Note: String(50));`, "Note"},
		"type contradicts the contract": {`create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders') (
  OrderId: Long, Title: Integer);`, "String(100)"},
		"key left out": {`create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders') (
  Title: String(100));`, "OrderId"},
		"capability contradicts the contract": {`create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders', Creatable: true) (
  OrderId: Long);`, "CE6630"},
		"unknown entity set": {`create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Ordrs') (
  OrderId: Long);`, "Orders"},
		"plain enumeration": {`create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders') (
  OrderId: Long, Status: Enumeration(Shop.LocalStatus));`, "Shop.OrderStatus"},
	} {
		t.Run(name, func(t *testing.T) {
			r, err := runSingle2497(t, c.src)
			if err == nil {
				t.Fatalf("accepted; created %+v", r.created["Order"])
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error does not mention %q:\n%s", c.want, err)
			}
			if len(r.created) != 0 {
				t.Error("written despite the refusal")
			}
		})
	}
}

// With no attribute list the entity gets every property the contract has.
func TestSingleExternalEntityWithoutAttributesTakesTheContract_2497(t *testing.T) {
	r, err := runSingle2497(t, `create external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders');`)
	if err != nil {
		t.Fatal(err)
	}
	if e := r.created["Order"]; e == nil || len(e.Attributes) != 3 || len(e.RemoteKeyParts) != 1 {
		t.Fatalf("created = %+v", e)
	}
}

// A stored entity whose attribute maps a property under another local name —
// an older import named the key `_Id`, today's names it `OrderId` — is
// rewritten by its describe output: the stated name is matched by the stored
// mapping, and keeps its name (the TestApp round trip, Odata.Devices).
func TestSingleExternalEntityRewriteMatchesStoredMapping_2497(t *testing.T) {
	stored := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Order", Source: "Rest$ODataRemoteEntitySource",
		RemoteServiceName: "Shop.Sales", RemoteEntitySet: "Orders", RemoteEntityName: "Order",
		Attributes: []*domainmodel.Attribute{
			{Name: "_Id", RemoteName: "Id", RemoteType: "Edm.Int64", Type: &domainmodel.LongAttributeType{}},
			{Name: "Title", RemoteName: "Title", RemoteType: "Edm.String", Type: &domainmodel.StringAttributeType{Length: 100}},
		},
		RemoteKeyParts: []*domainmodel.RemoteKeyPart{{Name: "_Id", RemoteName: "Id", RemoteType: "Edm.Int64"}},
	}
	r, err := runSingle2497(t, `create or modify external entity Shop.Order from consumed odata service Shop.Sales (EntitySet: 'Orders', RemoteName: 'Order') (
  _Id: Long,
  Title: String(100)
);`, stored)
	if err != nil {
		t.Fatal(err)
	}
	e := r.updated["Order"]
	if e == nil || len(e.Attributes) != 2 || e.Attributes[0].Name != "_Id" || e.Attributes[0].RemoteName != "Id" {
		t.Fatalf("updated = %+v", e)
	}
	if len(e.RemoteKeyParts) != 1 || e.RemoteKeyParts[0].Name != "_Id" {
		t.Errorf("key = %+v, want the key part named as the stored attribute", e.RemoteKeyParts)
	}
}
