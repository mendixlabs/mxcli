// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// createGuardCase is one guarded create per document family, and how to make
// the mock project already hold the element it names (ako/mxcli#731).
//
// Most families are found through a Backend.List* method; lister names the
// MockBackend field, and the test fills it by reflection with one element of
// the list's own element type, named after the statement. That also proves the
// production probe reads the real type's ContainerID and Name.
type createGuardCase struct {
	src    string
	lister string                                        // MockBackend List*Func field, or ""
	seed   func(mb *mock.MockBackend, mod *model.Module) // for the families without one
}

var createGuardCases = map[string]createGuardCase{
	"microflow":        {src: "create microflow if not exists M.X () begin return; end;", lister: "ListMicroflowsFunc"},
	"nanoflow":         {src: "create nanoflow if not exists M.X () begin return; end;", lister: "ListNanoflowsFunc"},
	"rule":             {src: "create rule if not exists M.X ($c: M.C) returns Boolean begin return true; end;", lister: "ListRulesFunc"},
	"javaaction":       {src: "create java action if not exists M.X() returns String as $$return \"\";$$;", lister: "ListJavaActionsFunc"},
	"javascriptaction": {src: "create javascript action if not exists M.X() returns Boolean platform Web as $$return true;$$;", lister: "ListJavaScriptActionsFunc"},
	"page":             {src: "create page if not exists M.X (Title: 'X', Layout: Atlas_Core.Atlas_Default) { dynamictext t (Content: 'x') };", lister: "ListPagesFunc"},
	"snippet":          {src: "create snippet if not exists M.X { dynamictext t (Content: 'x') };", lister: "ListSnippetsFunc"},
	"layout":           {src: "create layout if not exists M.X (layouttype: 'Responsive') { placeholder Main };", lister: "ListLayoutsFunc"},
	"enumeration":      {src: "create enumeration if not exists M.X (Red 'Red');", lister: "ListEnumerationsFunc"},
	"constant":         {src: "create constant if not exists M.X type String default 'a';", lister: "ListConstantsFunc"},
	"databaseconnection": {src: "create database connection if not exists M.X type 'PostgreSQL' connection string @M.DbUrl username @M.DbUser password @M.DbPass;",
		lister: "ListDatabaseConnectionsFunc"},
	"restclient":           {src: "create consumed rest service if not exists M.X (BaseUrl: 'https://x.example.com', Authentication: NONE) { };", lister: "ListConsumedRestServicesFunc"},
	"publishedrestservice": {src: "create published rest service if not exists M.X (Path: 'rest/x/v1', Version: '1.0.0', ServiceName: 'X') { };", lister: "ListPublishedRestServicesFunc"},
	"odataclient":          {src: "create consumed odata service if not exists M.X (Version: '1.0', ODataVersion: OData4, MetadataUrl: 'https://x.example.com/$metadata');", lister: "ListConsumedODataServicesFunc"},
	"odataservice": {src: "create published odata service if not exists M.X (path: 'odata/x/', version: '1.0.0', ODataVersion: OData4, namespace: 'M.X') authentication basic { };",
		lister: "ListPublishedODataServicesFunc"},
	"businesseventservice": {src: "create business event service if not exists M.X (ServiceName: 'X', EventNamePrefix: '') { message E (Id: Long) publish entity M.PBE; };",
		lister: "ListBusinessEventServicesFunc"},
	"workflow":                    {src: "create workflow if not exists M.X parameter $Context: M.C begin end workflow;", lister: "ListWorkflowsFunc"},
	"imagecollection":             {src: "create image collection if not exists M.X;", lister: "ListImageCollectionsFunc"},
	"queue":                       {src: "create task queue if not exists M.X (Parallelism: 3);", lister: "ListQueuesFunc"},
	"scheduledevent":              {src: "create scheduled event if not exists M.X (Microflow: M.SE, Repeat: Daily, HourOfDay: 4, MinuteOfHour: 0, TimeZone: Server, Enabled: true);", lister: "ListScheduledEventsFunc"},
	"regularexpression":           {src: "create regular expression if not exists M.X (Expression: '.+');", lister: "ListRegularExpressionsFunc"},
	"jsonstructure":               {src: "create json structure if not exists M.X snippet '{\"id\": 1}';", lister: "ListJsonStructuresFunc"},
	"messagedefinitioncollection": {src: "create message definition collection if not exists M.X {definition D for M.C as 'Cs' {Id}};", lister: "ListMessageDefinitionCollectionsFunc"},
	"messagedefinition":           {src: "create message definition if not exists M.X for M.C as 'Cs' {Id};", lister: "ListMessageDefinitionDocumentsFunc"},
	"importmapping":               {src: "create import mapping if not exists M.X with json structure M.J { create M.C { Id = id } };", lister: "ListImportMappingsFunc"},
	"exportmapping":               {src: "create export mapping if not exists M.X with json structure M.J { M.C { id = Id } };", lister: "ListExportMappingsFunc"},
	"datatransformer":             {src: "create data transformer if not exists M.X source json '{\"id\": 1}' { jslt '{\"id\": .id}'; };", lister: "ListDataTransformersFunc"},
	"model":                       {src: "create model if not exists M.X (Provider: MxCloudGenAI, Key: @M.K);", lister: "ListAgentEditorModelsFunc"},
	"consumedmcpservice":          {src: "create consumed mcp service if not exists M.X (ProtocolVersion: v2025_03_26, Version: '1.0');", lister: "ListAgentEditorConsumedMCPServicesFunc"},
	"publishedmcpservice":         {src: "create published mcp service if not exists M.X (EndpointPath: 'x', Version: '1.0', ProtocolVersion: v2025_03_26);", lister: "ListAgentEditorPublishedMCPServicesFunc"},
	"knowledgebase":               {src: "create knowledge base if not exists M.X (Provider: MxCloudGenAI, Key: @M.K);", lister: "ListAgentEditorKnowledgeBasesFunc"},
	"agent":                       {src: "create agent if not exists M.X (UsageType: Task, Model: M.GPT4, SystemPrompt: 'S.', UserPrompt: 'U.');", lister: "ListAgentEditorAgentsFunc"},
	"menu":                        {src: "create menu if not exists M.X (menu item 'Plain';);", lister: "ListMenuDocumentsFunc"},
	"module": {src: "create module if not exists X;", seed: func(mb *mock.MockBackend, mod *model.Module) {
		x := mkModule("X")
		mb.ListModulesFunc = func() ([]*model.Module, error) { return []*model.Module{mod, x}, nil }
	}},
	"modulerole": {src: "create module role if not exists M.X description 'x';", seed: func(mb *mock.MockBackend, mod *model.Module) {
		mb.GetModuleSecurityFunc = func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ContainerID: mod.ID, ModuleRoles: []*security.ModuleRole{{Name: "x"}}}, nil
		}
	}},
	"userrole": {src: "create user role if not exists X (M.User);", seed: func(mb *mock.MockBackend, _ *model.Module) {
		mb.GetProjectSecurityFunc = func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{UserRoles: []*security.UserRole{{Name: "X"}}}, nil
		}
	}},
	"demouser": {src: "create demo user if not exists 'X' password 'Password1!' (Clerk);", seed: func(mb *mock.MockBackend, _ *model.Module) {
		mb.GetProjectSecurityFunc = func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{DemoUsers: []*security.DemoUser{{UserName: "X"}}}, nil
		}
	}},
	"configuration": {src: "create configuration if not exists 'X';", seed: func(mb *mock.MockBackend, _ *model.Module) {
		mb.GetProjectSettingsFunc = func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{Configuration: &model.ConfigurationSettings{
				Configurations: []*model.ServerConfiguration{{Name: "x"}}}}, nil
		}
	}},
	"viewentity": {src: "create view entity if not exists M.X (Name: String(100)) as (select c.Name as Name from M.C as c);", seed: seedEntityX},
	"externalentity": {src: "create external entity if not exists M.X from consumed odata service M.Api (EntitySet: 'Xs', RemoteName: 'X');",
		seed: seedEntityX},
}

func seedEntityX(mb *mock.MockBackend, mod *model.Module) {
	mb.ListDomainModelsFunc = func() ([]*domainmodel.DomainModel, error) {
		return []*domainmodel.DomainModel{{ContainerID: mod.ID, Entities: []*domainmodel.Entity{{Name: "X"}}}}, nil
	}
}

// createGuardCtx is a project holding module M and nothing else; with exists,
// it also holds the element the case's statement names.
func createGuardCtx(t *testing.T, c createGuardCase, exists bool) (*ExecContext, *strings.Builder) {
	t.Helper()
	mod := mkModule("M")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{ContainerID: mod.ID}}, nil
		},
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ContainerID: mod.ID}, nil
		},
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return &security.ProjectSecurity{}, nil },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{Configuration: &model.ConfigurationSettings{}}, nil
		},
	}
	if c.lister != "" {
		f := reflect.ValueOf(mb).Elem().FieldByName(c.lister)
		if !f.IsValid() {
			t.Fatalf("MockBackend has no field %s", c.lister)
		}
		var items []reflect.Value
		sliceType := f.Type().Out(0)
		if exists {
			elem := reflect.New(sliceType.Elem().Elem())
			elem.Elem().FieldByName("ContainerID").Set(reflect.ValueOf(mod.ID))
			elem.Elem().FieldByName("Name").SetString("X")
			items = append(items, elem)
		}
		slice := reflect.MakeSlice(sliceType, 0, len(items))
		slice = reflect.Append(slice, items...)
		f.Set(reflect.MakeFunc(f.Type(), func([]reflect.Value) []reflect.Value {
			return []reflect.Value{slice, reflect.Zero(f.Type().Out(1))}
		}))
	} else if exists {
		c.seed(mb, mod)
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	var out strings.Builder
	ctx.Output = &out
	return ctx, &out
}

// runRecorded runs src through a registry whose handler for the built
// statement only records that it ran, so the test sees exactly what the guard
// decides, independent of what each handler needs from a mock.
func runRecorded(t *testing.T, ctx *ExecContext, src string) (bool, error) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	stmt := prog.Statements[0]
	ran := false
	r := NewRegistry()
	r.handlers[reflect.TypeOf(stmt)] = func(*ExecContext, ast.Statement) error {
		ran = true
		return nil
	}
	err := r.Dispatch(ctx, stmt)
	return ran, err
}

// An element that already exists is left alone: the handler — which would
// refuse the create, or rewrite the element — never runs, and the skip is
// reported.
func TestCreateIfNotExists_ExistingElementIsLeftAlone(t *testing.T) {
	for name, c := range createGuardCases {
		t.Run(name, func(t *testing.T) {
			ctx, out := createGuardCtx(t, c, true)
			ran, err := runRecorded(t, ctx, c.src)
			if err != nil {
				t.Fatalf("errored: %v", err)
			}
			if ran {
				t.Error("the create handler ran on an element that already exists")
			}
			if !strings.Contains(out.String(), "skipped (if not exists)") {
				t.Errorf("a skipped create should say so; output: %q", out.String())
			}
		})
	}
}

// CONTROL: an absent element is created — the handler runs. Without this the
// test above passes against a guard that skips everything.
func TestCreateIfNotExists_AbsentElementIsCreated(t *testing.T) {
	for name, c := range createGuardCases {
		t.Run(name, func(t *testing.T) {
			ctx, out := createGuardCtx(t, c, false)
			ran, err := runRecorded(t, ctx, c.src)
			if err != nil {
				t.Fatalf("errored: %v", err)
			}
			if !ran {
				t.Errorf("the create handler did not run on an absent element; output: %q", out.String())
			}
		})
	}
}

// CONTROL: without the guard, an existing element still reaches the handler —
// the skip is the guard's doing, not the probe's.
func TestCreateIfNotExists_UnguardedCreateReachesHandler(t *testing.T) {
	for name, c := range createGuardCases {
		t.Run(name, func(t *testing.T) {
			ctx, _ := createGuardCtx(t, c, true)
			ran, err := runRecorded(t, ctx, strings.Replace(c.src, " if not exists", "", 1))
			if err != nil {
				t.Fatalf("errored: %v", err)
			}
			if !ran {
				t.Error("an unguarded create was skipped")
			}
		})
	}
}

// Every statement type that carries the guard has an existence probe. The
// visitor test proves every create kind builds a guarded statement; this closes
// the loop to the executor, so the fallback "no existence check" error is
// unreachable from a script.
func TestCreateGuardTargetCoversEveryGuardedStatement(t *testing.T) {
	var guarded []ast.Statement
	for _, c := range createGuardCases {
		prog, errs := visitor.Build(c.src)
		if len(errs) > 0 {
			t.Fatalf("parse %q: %v", c.src, errs)
		}
		guarded = append(guarded, prog.Statements[0])
	}
	seen := map[reflect.Type]bool{}
	for _, s := range guarded {
		seen[reflect.TypeOf(s)] = true
	}
	// Every AST type that embeds CreateGuard must be among the cases, except
	// the two whose handlers test the guard themselves.
	for _, s := range allKnownStatements() {
		if _, ok := s.(ast.IfNotExistsCreate); !ok {
			continue
		}
		switch s.(type) {
		case *ast.CreateEntityStmt, *ast.CreateAssociationStmt:
			continue
		}
		if !seen[reflect.TypeOf(s)] {
			t.Errorf("%T carries the if-not-exists guard but has no case in createGuardCases", s)
		}
		if tgt, ok := createGuardTargetOf(s); ok && strings.HasPrefix(tgt.kind, "*ast.") {
			t.Errorf("%T has no existence probe in createGuardTargetOf", s)
		}
	}
}
