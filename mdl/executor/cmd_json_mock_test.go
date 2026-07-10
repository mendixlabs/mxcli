// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

func TestShowEnumerations_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	enum := mkEnumeration(mod.ID, "Status", "Active", "Inactive")
	withContainer(h, enum.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listEnumerations(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Status")
}

func TestShowConstants_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	c := mkConstant(mod.ID, "Timeout", "Integer", "30")
	withContainer(h, c.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListConstantsFunc: func() ([]*model.Constant, error) { return []*model.Constant{c}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listConstants(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Timeout")
}

func TestShowMicroflows_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	mf := mkMicroflow(mod.ID, "ACT_DoStuff")
	withContainer(h, mf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listMicroflows(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "ACT_DoStuff")
}

func TestShowNanoflows_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	nf := mkNanoflow(mod.ID, "NF_Validate")
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listNanoflows(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "NF_Validate")
}

func TestShowPages_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	pg := mkPage(mod.ID, "Page_Home")
	withContainer(h, pg.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listPages(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Page_Home")
}

func TestShowSnippets_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	snp := mkSnippet(mod.ID, "Snippet_Header")
	withContainer(h, snp.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listSnippets(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Snippet_Header")
}

func TestShowLayouts_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	lay := mkLayout(mod.ID, "Layout_Main")
	withContainer(h, lay.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{lay}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listLayouts(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Layout_Main")
}

func TestShowWorkflows_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	wf := mkWorkflow(mod.ID, "WF_Approve")
	withContainer(h, wf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listWorkflows(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "WF_Approve")
}

func TestShowODataClients_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	svc := &model.ConsumedODataService{
		BaseElement: model.BaseElement{ID: nextID("cos")},
		ContainerID: mod.ID,
		Name:        "ExtService",
	}
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:               func() bool { return true },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) { return []*model.ConsumedODataService{svc}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listODataClients(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "ExtService")
}

func TestShowODataServices_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	svc := &model.PublishedODataService{
		BaseElement: model.BaseElement{ID: nextID("pos")},
		ContainerID: mod.ID,
		Name:        "PubOData",
	}
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:                func() bool { return true },
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) { return []*model.PublishedODataService{svc}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listODataServices(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "PubOData")
}

func TestShowRestClients_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: nextID("crs")},
		ContainerID: mod.ID,
		Name:        "RestClient1",
	}
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:              func() bool { return true },
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) { return []*model.ConsumedRestService{svc}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listRestClients(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "RestClient1")
}

func TestShowPublishedRestServices_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	svc := &model.PublishedRestService{
		BaseElement: model.BaseElement{ID: nextID("prs")},
		ContainerID: mod.ID,
		Name:        "PubRest1",
	}
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:               func() bool { return true },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) { return []*model.PublishedRestService{svc}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listPublishedRestServices(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "PubRest1")
}

func TestShowJavaActions_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	ja := &types.JavaAction{
		BaseElement: model.BaseElement{ID: nextID("ja")},
		ContainerID: mod.ID,
		Name:        "MyJavaAction",
	}
	withContainer(h, ja.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) { return []*types.JavaAction{ja}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listJavaActions(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "MyJavaAction")
}

func TestShowJavaScriptActions_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	jsa := &types.JavaScriptAction{
		BaseElement: model.BaseElement{ID: nextID("jsa")},
		ContainerID: mod.ID,
		Name:        "MyJSAction",
	}
	withContainer(h, jsa.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return []*types.JavaScriptAction{jsa}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listJavaScriptActions(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "MyJSAction")
}

func TestShowDatabaseConnections_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	dc := &model.DatabaseConnection{
		BaseElement: model.BaseElement{ID: nextID("dc")},
		ContainerID: mod.ID,
		Name:        "MyDB",
	}
	withContainer(h, dc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListDatabaseConnectionsFunc: func() ([]*model.DatabaseConnection, error) { return []*model.DatabaseConnection{dc}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listDatabaseConnections(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "MyDB")
}

func TestShowImageCollections_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	ic := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "Icons",
	}
	withContainer(h, ic.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listImageCollections(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Icons")
}

func TestShowJsonStructures_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	js := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod.ID,
		Name:        "OrderSchema",
	}
	withContainer(h, js.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listJsonStructures(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "OrderSchema")
}

func TestShowUserRoles_Mock_JSON(t *testing.T) {
	ps := &security.ProjectSecurity{
		BaseElement: model.BaseElement{ID: nextID("ps")},
		UserRoles: []*security.UserRole{
			{Name: "Administrator"},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return ps, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON))
	assertNoError(t, listUserRoles(ctx))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Administrator")
}

func TestShowModuleRoles_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	ms := &security.ModuleSecurity{
		BaseElement: model.BaseElement{ID: nextID("ms")},
		ContainerID: mod.ID,
		ModuleRoles: []*security.ModuleRole{
			{Name: "User"},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListModuleSecurityFunc: func() ([]*security.ModuleSecurity, error) { return []*security.ModuleSecurity{ms}, nil },
	}

	h := mkHierarchy(mod)
	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listModuleRoles(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "User")
}

func TestShowDemoUsers_Mock_JSON(t *testing.T) {
	ps := &security.ProjectSecurity{
		BaseElement:     model.BaseElement{ID: nextID("ps")},
		EnableDemoUsers: true,
		DemoUsers: []*security.DemoUser{
			{UserName: "demo_admin"},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return ps, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON))
	assertNoError(t, listDemoUsers(ctx))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "demo_admin")
}

func TestShowBusinessEventServices_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	svc := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		ContainerID: mod.ID,
		Name:        "OrderEvents",
	}
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:               func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) { return []*model.BusinessEventService{svc}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listBusinessEventServices(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "OrderEvents")
}

func TestShowAgentEditorModels_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	m1 := &agenteditor.Model{
		BaseElement: model.BaseElement{ID: nextID("aem")},
		ContainerID: mod.ID,
		Name:        "GPT4o",
	}
	withContainer(h, m1.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorModelsFunc: func() ([]*agenteditor.Model, error) { return []*agenteditor.Model{m1}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listAgentEditorModels(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "GPT4o")
}

func TestShowAgentEditorAgents_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	a1 := &agenteditor.Agent{
		BaseElement: model.BaseElement{ID: nextID("aea")},
		ContainerID: mod.ID,
		Name:        "Helper",
	}
	withContainer(h, a1.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) { return []*agenteditor.Agent{a1}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listAgentEditorAgents(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Helper")
}

func TestShowAgentEditorKnowledgeBases_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	kb := &agenteditor.KnowledgeBase{
		BaseElement: model.BaseElement{ID: nextID("aek")},
		ContainerID: mod.ID,
		Name:        "FAQ",
	}
	withContainer(h, kb.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:                   func() bool { return true },
		ListAgentEditorKnowledgeBasesFunc: func() ([]*agenteditor.KnowledgeBase, error) { return []*agenteditor.KnowledgeBase{kb}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listAgentEditorKnowledgeBases(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "FAQ")
}

func TestShowAgentEditorMCPServices_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	svc := &agenteditor.ConsumedMCPService{
		BaseElement: model.BaseElement{ID: nextID("aes")},
		ContainerID: mod.ID,
		Name:        "ToolSvc",
	}
	withContainer(h, svc.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:                        func() bool { return true },
		ListAgentEditorConsumedMCPServicesFunc: func() ([]*agenteditor.ConsumedMCPService, error) { return []*agenteditor.ConsumedMCPService{svc}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listAgentEditorConsumedMCPServices(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "ToolSvc")
}

func TestListDataTransformers_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	dt := &model.DataTransformer{
		BaseElement: model.BaseElement{ID: nextID("dt")},
		ContainerID: mod.ID,
		Name:        "Transform1",
	}
	withContainer(h, dt.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListDataTransformersFunc: func() ([]*model.DataTransformer, error) { return []*model.DataTransformer{dt}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listDataTransformers(ctx, ""))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "Transform1")
}

func TestShowAccessOnMicroflow_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	mf := mkMicroflow(mod.ID, "ACT_DoStuff")
	mf.AllowedModuleRoles = []model.ID{"MyModule.User", "MyModule.Admin"}
	withContainer(h, mf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}

	name := &ast.QualifiedName{Module: "MyModule", Name: "ACT_DoStuff"}
	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listAccessOnMicroflow(ctx, name))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "User")
}

func TestShowAccessOnPage_Mock_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	pg := mkPage(mod.ID, "Page_Home")
	pg.AllowedRoles = []model.ID{"MyModule.User"}
	withContainer(h, pg.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
	}

	name := &ast.QualifiedName{Module: "MyModule", Name: "Page_Home"}
	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listAccessOnPage(ctx, name))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), "User")
}

// TestShowConstants_Mock_JSON_EmptyResult verifies that an empty result still
// produces valid JSON (not the "No ... found." plain-text message).
func TestShowConstants_Mock_JSON_EmptyResult(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListConstantsFunc: func() ([]*model.Constant, error) { return nil, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listConstants(ctx, ""))
	assertValidJSON(t, buf.String())
	assertNotContainsStr(t, buf.String(), "No constants found")
}

// TestShowPublishedRestServices_Mock_JSON_EmptyResult verifies that an empty
// result still produces valid JSON in JSON mode.
func TestShowPublishedRestServices_Mock_JSON_EmptyResult(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:               func() bool { return true },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) { return nil, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON), withHierarchy(h))
	assertNoError(t, listPublishedRestServices(ctx, ""))
	assertValidJSON(t, buf.String())
	assertNotContainsStr(t, buf.String(), "No published rest services found")
}
