// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func TestShowODataClients_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.ConsumedODataService{
		BaseElement:  model.BaseElement{ID: nextID("cos")},
		ContainerID:  mod.ID,
		Name:         "PetStoreClient",
		MetadataUrl:  "https://example.com/$metadata",
		Version:      "1.0",
		ODataVersion: "4.0",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listODataClients(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "QualifiedName")
	assertContainsStr(t, out, "MyModule.PetStoreClient")
}

func TestShowODataServices_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.PublishedODataService{
		BaseElement:  model.BaseElement{ID: nextID("pos")},
		ContainerID:  mod.ID,
		Name:         "CatalogService",
		Path:         "/odata/v1",
		Version:      "1.0",
		ODataVersion: "4.0",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) {
			return []*model.PublishedODataService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listODataServices(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "QualifiedName")
	assertContainsStr(t, out, "MyModule.CatalogService")
}

func TestDescribeODataClient_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.ConsumedODataService{
		BaseElement:  model.BaseElement{ID: nextID("cos")},
		ContainerID:  mod.ID,
		Name:         "PetStoreClient",
		MetadataUrl:  "https://example.com/$metadata",
		Version:      "2.0",
		ODataVersion: "4.0",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeODataClient(ctx, ast.QualifiedName{Module: "MyModule", Name: "PetStoreClient"}))

	out := buf.String()
	assertContainsStr(t, out, "create odata client")
	assertContainsStr(t, out, "MyModule.PetStoreClient")
	assertContainsStr(t, out, "https://example.com/$metadata")
	assertContainsStr(t, out, "2.0")
}

func TestDescribeODataClient_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeODataClient(ctx, ast.QualifiedName{Module: "MyModule", Name: "NoSuch"}))
}

func TestShowODataClients_FilterByModule(t *testing.T) {
	mod1 := mkModule("Alpha")
	mod2 := mkModule("Beta")
	svc1 := &model.ConsumedODataService{
		BaseElement: model.BaseElement{ID: nextID("cos")},
		ContainerID: mod1.ID,
		Name:        "AlphaSvc",
	}
	svc2 := &model.ConsumedODataService{
		BaseElement: model.BaseElement{ID: nextID("cos")},
		ContainerID: mod2.ID,
		Name:        "BetaSvc",
	}
	h := mkHierarchy(mod1, mod2)
	withContainer(h, svc1.ContainerID, mod1.ID)
	withContainer(h, svc2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc1, svc2}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listODataClients(ctx, "Alpha"))

	out := buf.String()
	assertContainsStr(t, out, "Alpha.AlphaSvc")
	assertNotContainsStr(t, out, "Beta.BetaSvc")
}

func TestShowODataServices_FilterByModule(t *testing.T) {
	mod := mkModule("Sales")
	svc := &model.PublishedODataService{
		BaseElement: model.BaseElement{ID: nextID("pos")},
		ContainerID: mod.ID,
		Name:        "SalesSvc",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) {
			return []*model.PublishedODataService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listODataServices(ctx, "Sales"))
	assertContainsStr(t, buf.String(), "Sales.SalesSvc")
}

func TestDescribeODataService_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.PublishedODataService{
		BaseElement:  model.BaseElement{ID: nextID("pos")},
		ContainerID:  mod.ID,
		Name:         "CatalogService",
		Path:         "/odata/v1",
		Version:      "1.0",
		ODataVersion: "4.0",
		Namespace:    "MyApp",
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) {
			return []*model.PublishedODataService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeODataService(ctx, ast.QualifiedName{Module: "MyModule", Name: "CatalogService"}))

	out := buf.String()
	assertContainsStr(t, out, "create odata service")
	assertContainsStr(t, out, "MyModule.CatalogService")
}

func TestDescribeODataService_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeODataService(ctx, ast.QualifiedName{Module: "X", Name: "NoSuch"}))
}

// TestCreateExternalEntity_RejectsNonExistentClient verifies that CREATE EXTERNAL ENTITY
// returns an error when the referenced OData client does not exist (issue #417).
func TestCreateExternalEntity_RejectsNonExistentClient(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: nextID("dm")}, ContainerID: mod.ID}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return nil, nil // no services registered
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateExternalEntityStmt{
		Name:       ast.QualifiedName{Module: "MyModule", Name: "FakeEntity"},
		ServiceRef: ast.QualifiedName{Module: "MyModule", Name: "NonExistentClient"},
		EntitySet:  strPtr("Products"),
	}
	err := execCreateExternalEntity(ctx, stmt)
	assertError(t, err)
	assertContainsStr(t, err.Error(), "odata client not found")
}

func TestCreateExternalEntity_AcceptsExistingClient(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	svc := &model.ConsumedODataService{
		BaseElement: model.BaseElement{ID: nextID("cos")},
		ContainerID: mod.ID,
		Name:        "ProductsClient",
	}
	withContainer(h, svc.ContainerID, mod.ID)
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: nextID("dm")}, ContainerID: mod.ID}

	var created *domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
		CreateEntityFunc: func(dmID model.ID, entity *domainmodel.Entity) error {
			created = entity
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateExternalEntityStmt{
		Name:       ast.QualifiedName{Module: "MyModule", Name: "Product"},
		ServiceRef: ast.QualifiedName{Module: "MyModule", Name: "ProductsClient"},
		EntitySet:  strPtr("Products"),
	}
	assertNoError(t, execCreateExternalEntity(ctx, stmt))
	if created == nil {
		t.Error("expected CreateEntity to be called")
	}
}

// TestCreateExternalEntity_AllowCreateChangeLocally_Issue534 verifies that
// AllowCreateChangeLocally is passed through to the new entity.
func TestCreateExternalEntity_AllowCreateChangeLocally_Issue534(t *testing.T) {
	mod := mkModule("TripPin")
	h := mkHierarchy(mod)
	svc := &model.ConsumedODataService{
		BaseElement: model.BaseElement{ID: nextID("cos")},
		ContainerID: mod.ID,
		Name:        "TripPinClient",
	}
	withContainer(h, svc.ContainerID, mod.ID)
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: nextID("dm")}, ContainerID: mod.ID}

	var created *domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
		CreateEntityFunc: func(dmID model.ID, entity *domainmodel.Entity) error {
			created = entity
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateExternalEntityStmt{
		Name:                     ast.QualifiedName{Module: "TripPin", Name: "People"},
		ServiceRef:               ast.QualifiedName{Module: "TripPin", Name: "TripPinClient"},
		EntitySet:                strPtr("People"),
		AllowCreateChangeLocally: boolPtr(true),
	}
	assertNoError(t, execCreateExternalEntity(ctx, stmt))
	if created == nil {
		t.Fatal("expected CreateEntity to be called")
	}
	if !created.CreateChangeLocally {
		t.Errorf("expected CreateChangeLocally = true, got false")
	}
}

// TestCreateOrModifyExternalEntity_PreservesOmittedFields_Issue594 verifies that
// CREATE OR MODIFY EXTERNAL ENTITY preserves the existing value of any field
// that is omitted from the MDL statement, rather than overwriting it with the
// zero value. Previously the executor unconditionally wrote `existingEntity.X = s.X`
// for every field, so omitting `RemoteName` on `or modify` wiped the BSON
// `RemoteName` to "" and triggered Studio Pro's
// ODataRemoteEntitySource.get_RemoteId() NRE in the Integration pane.
func TestCreateOrModifyExternalEntity_PreservesOmittedFields_Issue594(t *testing.T) {
	mod := mkModule("OdTest")
	h := mkHierarchy(mod)
	svc := &model.ConsumedODataService{
		BaseElement: model.BaseElement{ID: nextID("cos")},
		ContainerID: mod.ID,
		Name:        "SalesforceAPI",
	}
	withContainer(h, svc.ContainerID, mod.ID)

	// Pre-existing external entity with non-default values for every field
	// that the bug could clobber.
	existing := &domainmodel.Entity{
		BaseElement:         model.BaseElement{ID: nextID("ent")},
		Name:                "RemoteAccount",
		Source:              "Rest$ODataRemoteEntitySource",
		RemoteServiceName:   "OdTest.SalesforceAPI",
		RemoteEntitySet:     "Accounts",
		RemoteEntityName:    "Account", // <-- the field that #594 wiped
		Countable:           true,
		Creatable:           true,
		Deletable:           true,
		Updatable:           true,
		CreateChangeLocally: false,
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{existing},
	}

	var updated *domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
		UpdateEntityFunc: func(dmID model.ID, entity *domainmodel.Entity) error {
			updated = entity
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	// Only mention EntitySet + AllowCreateChangeLocally — every other
	// scalar should be preserved from the existing entity, not zeroed.
	stmt := &ast.CreateExternalEntityStmt{
		Name:                     ast.QualifiedName{Module: "OdTest", Name: "RemoteAccount"},
		ServiceRef:               ast.QualifiedName{Module: "OdTest", Name: "SalesforceAPI"},
		EntitySet:                strPtr("Accounts"),
		AllowCreateChangeLocally: boolPtr(true),
		CreateOrModify:           true,
	}
	assertNoError(t, execCreateExternalEntity(ctx, stmt))

	if updated == nil {
		t.Fatal("expected UpdateEntity to be called")
	}
	if updated.RemoteEntityName != "Account" {
		t.Errorf("RemoteEntityName = %q, want \"Account\" (omitted RemoteName must preserve existing value)", updated.RemoteEntityName)
	}
	if !updated.Countable {
		t.Errorf("Countable = false, want true (omitted Countable must preserve existing value)")
	}
	if !updated.Creatable {
		t.Errorf("Creatable = false, want true (omitted Creatable must preserve existing value)")
	}
	if !updated.Deletable {
		t.Errorf("Deletable = false, want true (omitted Deletable must preserve existing value)")
	}
	if !updated.Updatable {
		t.Errorf("Updatable = false, want true (omitted Updatable must preserve existing value)")
	}
	if !updated.CreateChangeLocally {
		t.Errorf("CreateChangeLocally = false, want true (explicitly set value)")
	}
}

// TestDescribeODataService_ExposeRoundtrip verifies that DESCRIBE ODATA SERVICE
// output for entities with key/filterable/sortable members is valid MDL that
// the parser can re-parse (issue #400).
func TestDescribeODataService_ExposeRoundtrip(t *testing.T) {
	mod := mkModule("MyModule")
	svc := &model.PublishedODataService{
		BaseElement:  model.BaseElement{ID: nextID("pos")},
		ContainerID:  mod.ID,
		Name:         "CatalogService",
		Path:         "/odata/v1",
		Version:      "1.0",
		ODataVersion: "4.0",
		EntityTypes: []*model.PublishedEntityType{
			{
				Entity:      "MyModule.Order",
				ExposedName: "Orders",
				Members: []*model.PublishedMember{
					{Name: "Id", ExposedName: "Id", IsPartOfKey: true},
					{Name: "Name", ExposedName: "Name", Filterable: true, Sortable: true},
				},
			},
		},
		EntitySets: []*model.PublishedEntitySet{
			{
				ExposedName:    "Orders",
				EntityTypeName: "MyModule.Order",
				ReadMode:       "Readable",
				InsertMode:     "NotSupported",
			},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) {
			return []*model.PublishedODataService{svc}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeODataService(ctx, ast.QualifiedName{Module: "MyModule", Name: "CatalogService"}))

	out := buf.String()
	assertContainsStr(t, out, "IsPartOfKey")

	_, errs := visitor.Build(out)
	if len(errs) > 0 {
		t.Errorf("DESCRIBE output failed to parse (roundtrip broken):\n%s\nErrors:", out)
		for _, e := range errs {
			t.Errorf("  %v", e)
		}
	}
}

func TestCreateODataClient_InvalidMetadataURL(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	stmt := &ast.CreateODataClientStmt{
		Name:        ast.QualifiedName{Module: "MyModule", Name: "BadClient"},
		MetadataUrl: "not-a-url",
	}
	err := createODataClient(ctx, stmt)
	assertError(t, err)
	assertContainsStr(t, err.Error(), "MetadataUrl")
}

func TestCreateODataClient_ValidMetadataURLs(t *testing.T) {
	for _, validURL := range []string{
		"https://example.com/odata/$metadata",
		"http://localhost:8080/$metadata",
		"file:///tmp/metadata.xml",
		"./metadata.xml",
		"../service/metadata.xml",
		"/abs/path/metadata.xml",
	} {
		err := validateMetadataURL(validURL)
		if err != nil {
			t.Errorf("expected %q to be valid, got error: %v", validURL, err)
		}
	}
}

func TestValidateMetadataURL_RejectsBarWords(t *testing.T) {
	for _, bad := range []string{"not-a-url", "justword", "no-scheme-no-dots"} {
		err := validateMetadataURL(bad)
		if err == nil {
			t.Errorf("expected %q to be rejected, but got nil error", bad)
		}
	}
}

// TestCreateODataClient_StripsMicroflowPrefix_Issue573 verifies that the
// "microflow " keyword prefix the visitor emits in front of a qualified name
// is stripped before the value reaches BSON.
//
// The visitor at mdl/visitor/visitor_odata.go emits uppercase "MICROFLOW "
// for `microflow Module.Name` property values. extractMicroflowRef used to
// strip only lowercase "microflow ", so the prefix survived all the way to
// BSON and Mendix tried to resolve a microflow whose qualified name was
// literally "MICROFLOW Module.Name" — see issue #573.
func TestCreateODataClient_StripsMicroflowPrefix_Issue573(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	var captured *model.ConsumedODataService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return nil, nil
		},
		CreateConsumedODataServiceFunc: func(svc *model.ConsumedODataService) error {
			captured = svc
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	stmt := &ast.CreateODataClientStmt{
		Name:                   ast.QualifiedName{Module: "MyModule", Name: "MyService"},
		ODataVersion:           "OData4",
		MetadataUrl:            "https://example.com/odata/$metadata",
		ConfigurationMicroflow: "MICROFLOW MyModule.ConfigureRequest",
		ErrorHandlingMicroflow: "MICROFLOW MyModule.HandleError",
	}
	assertNoError(t, createODataClient(ctx, stmt))

	if captured == nil {
		t.Fatal("CreateConsumedODataService was not called")
	}
	if captured.ConfigurationMicroflow != "MyModule.ConfigureRequest" {
		t.Errorf("ConfigurationMicroflow = %q, want %q (uppercase \"MICROFLOW \" prefix not stripped)",
			captured.ConfigurationMicroflow, "MyModule.ConfigureRequest")
	}
	if captured.ErrorHandlingMicroflow != "MyModule.HandleError" {
		t.Errorf("ErrorHandlingMicroflow = %q, want %q (uppercase \"MICROFLOW \" prefix not stripped)",
			captured.ErrorHandlingMicroflow, "MyModule.HandleError")
	}
}

// TestCreateODataClient_VisitorRoundtrip_Issue573 is the user-facing scenario
// for issue #573: parse the exact MDL the user wrote, run the executor, and
// confirm the value handed to the backend is the bare qualified name.
func TestCreateODataClient_VisitorRoundtrip_Issue573(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	var captured *model.ConsumedODataService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return nil, nil
		},
		CreateConsumedODataServiceFunc: func(svc *model.ConsumedODataService) error {
			captured = svc
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	const script = `CREATE ODATA CLIENT MyModule.MyService (
		ODataVersion: OData4,
		MetadataUrl: 'https://example.com/odata/$metadata',
		Timeout: 300,
		ConfigurationMicroflow: microflow MyModule.ConfigureRequest,
		ErrorHandlingMicroflow: microflow MyModule.HandleError
	);`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateODataClientStmt)
	if !ok {
		t.Fatalf("expected *CreateODataClientStmt, got %T", prog.Statements[0])
	}
	assertNoError(t, createODataClient(ctx, stmt))

	if captured == nil {
		t.Fatal("CreateConsumedODataService was not called")
	}
	if captured.ConfigurationMicroflow != "MyModule.ConfigureRequest" {
		t.Errorf("ConfigurationMicroflow = %q, want %q", captured.ConfigurationMicroflow, "MyModule.ConfigureRequest")
	}
	if captured.ErrorHandlingMicroflow != "MyModule.HandleError" {
		t.Errorf("ErrorHandlingMicroflow = %q, want %q", captured.ErrorHandlingMicroflow, "MyModule.HandleError")
	}
}

// TestCreateODataClient_HeadersMicroflow verifies the "Headers microflow"
// dropdown option in Studio Pro's "Configuration source". Both the
// "Configuration microflow" and "Headers microflow" options write to the
// same BSON field `ConfigurationMicroflow`; Studio Pro picks the dropdown
// label from the referenced microflow's return type (list of
// System.HttpHeader -> "Headers microflow"). The MDL `HeadersMicroflow:`
// keyword is therefore an alias for `ConfigurationMicroflow:` — both
// routes flow into the same model field.
func TestCreateODataClient_HeadersMicroflow(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	var captured *model.ConsumedODataService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return nil, nil
		},
		CreateConsumedODataServiceFunc: func(svc *model.ConsumedODataService) error {
			captured = svc
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	const script = `CREATE ODATA CLIENT MyModule.MyService (
		ODataVersion: OData4,
		MetadataUrl: 'https://example.com/odata/$metadata',
		Timeout: 300,
		HeadersMicroflow: microflow MyModule.SetHeaders,
		ErrorHandlingMicroflow: microflow MyModule.HandleError
	);`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateODataClientStmt)
	if !ok {
		t.Fatalf("expected *CreateODataClientStmt, got %T", prog.Statements[0])
	}
	assertNoError(t, createODataClient(ctx, stmt))

	if captured == nil {
		t.Fatal("CreateConsumedODataService was not called")
	}
	// The `HeadersMicroflow` keyword maps to the distinct HeadersMicroflow
	// model field (Studio Pro's HeaderListMicroflow slot on 11.10+), NOT the
	// configuration slot — conflating them triggers CE6808 (issue #728).
	if captured.HeadersMicroflow != "MyModule.SetHeaders" {
		t.Errorf("HeadersMicroflow = %q, want %q", captured.HeadersMicroflow, "MyModule.SetHeaders")
	}
	if captured.ConfigurationMicroflow != "" {
		t.Errorf("ConfigurationMicroflow = %q, want empty (headers keyword must not populate the config slot)", captured.ConfigurationMicroflow)
	}
}
