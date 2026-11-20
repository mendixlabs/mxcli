// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#1811 sweep: the document shapes whose storage types (or the
// properties holding them) arrived after 9.0, which the writers produce with
// no older shape and no statement gated. Each floor is the write's: the latest
// introduction among the types and properties the shape writes, measured on
// stamped projects and pinned to the type and property data by
// TestTypeGates_FloorIsTheTypeIntroduction.
func TestCheckVersionGates_DocumentShapes1811(t *testing.T) {
	qn := func(module, name string) ast.QualifiedName { return ast.QualifiedName{Module: module, Name: name} }
	param := []ast.PageParameter{{Name: "Order", EntityType: qn("M", "Order")}}
	variable := []ast.PageVariable{{Name: "Flag", DataType: "Boolean", DefaultValue: "true"}}
	button := func(action *ast.ActionV3) []*ast.WidgetV3 {
		return []*ast.WidgetV3{{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{"Action": action}}}
	}
	iconItem := ast.NavMenuItemDef{Caption: "Home", Icon: "Atlas_Core.Atlas_Filled.home", IconKind: types.MenuIconCollection}
	getOp := &ast.RestOperationDef{Name: "Get", Method: "GET", Path: "/x"}
	addon := "addon"
	queryOp := &ast.RestOperationDef{Name: "Search", Method: "GET", Path: "/s", QueryParameters: []ast.RestParamDef{{Name: "q", DataType: "String"}}}
	checkStatementGateCases(t, []statementGateCase{
		{"create snippet with a parameter", &ast.CreateSnippetStmtV3{Name: qn("M", "S"), Parameters: param},
			[3]int{9, 20, 0}, [3]int{9, 21, 0}, "a snippet parameter"},
		{"alter snippet add a parameter", &ast.AlterPageStmt{ContainerType: "SNIPPET", PageName: qn("M", "S"),
			Operations: []ast.AlterPageOperation{&ast.AddParameterOp{Parameter: param[0]}}},
			[3]int{9, 20, 0}, [3]int{9, 21, 0}, "a snippet parameter"},
		{"alter page add a parameter", &ast.AlterPageStmt{ContainerType: "PAGE", PageName: qn("M", "P"),
			Operations: []ast.AlterPageOperation{&ast.AddParameterOp{Parameter: param[0]}}},
			[3]int{9, 3, 0}, [3]int{9, 4, 0}, "add a page parameter"},
		{"create page with variables", &ast.CreatePageStmtV3{Name: qn("M", "P"), Variables: variable},
			[3]int{10, 19, 0}, [3]int{10, 20, 0}, "a page or snippet variable"},
		{"create snippet with variables", &ast.CreateSnippetStmtV3{Name: qn("M", "S"), Variables: variable},
			[3]int{10, 19, 0}, [3]int{10, 20, 0}, "a page or snippet variable"},
		{"alter page add a variable", &ast.AlterPageStmt{ContainerType: "PAGE", PageName: qn("M", "P"),
			Operations: []ast.AlterPageOperation{&ast.AddVariableOp{Variable: variable[0]}}},
			[3]int{10, 16, 0}, [3]int{10, 20, 0}, "a page or snippet variable"},
		{"a complete task button", &ast.CreatePageStmtV3{Name: qn("M", "P"), Widgets: button(&ast.ActionV3{Type: "completeTask", OutcomeValue: "Approve"})},
			[3]int{9, 18, 0}, [3]int{9, 19, 0}, "action: complete task"},
		{"an open user task button", &ast.CreateSnippetStmtV3{Name: qn("M", "S"), Widgets: button(&ast.ActionV3{Type: "openUserTask"})},
			[3]int{9, 0, 1}, [3]int{9, 0, 2}, "action: open user task / open workflow / call workflow"},
		{"a call workflow button, inserted", &ast.AlterPageStmt{ContainerType: "PAGE", PageName: qn("M", "P"),
			Operations: []ast.AlterPageOperation{&ast.InsertWidgetOp{Widgets: button(&ast.ActionV3{Type: "callWorkflow", Target: "M.WF"})}}},
			[3]int{9, 0, 1}, [3]int{9, 0, 2}, "action: open user task / open workflow / call workflow"},
		{"create menu with an icon", &ast.CreateMenuStmt{Name: qn("M", "Menu"), Items: []ast.NavMenuItemDef{
			{Caption: "More", Items: []ast.NavMenuItemDef{iconItem}}}},
			[3]int{9, 19, 0}, [3]int{9, 20, 0}, "Icon: on a menu item"},
		{"alter navigation set an icon", &ast.AlterNavigationStmt{ProfileName: "Responsive", Operations: []ast.MenuAlterOp{
			&ast.SetMenuItemOp{SetIcon: true, Item: iconItem}}},
			[3]int{9, 19, 0}, [3]int{9, 20, 0}, "Icon: on a menu item"},
		{"progressive web app settings", &ast.CreateNavigationStmt{ProfileName: "PhoneOffline", ProgressiveWebApp: &types.NavPWASpec{}},
			[3]int{9, 0, 2}, [3]int{9, 0, 3}, "progressive web app ( … )"},
		{"a native home page", &ast.CreateNavigationStmt{ProfileName: "NativePhone", HomePages: []ast.NavHomePageDef{{IsPage: true, Target: qn("M", "Home")}}},
			[3]int{9, 3, 0}, [3]int{9, 4, 0}, "home page on a native profile"},
		{"create navigation not found page", &ast.CreateNavigationStmt{ProfileName: "Responsive", NotFoundPage: &ast.QualifiedName{Module: "M", Name: "NotFound"}},
			[3]int{10, 12, 0}, [3]int{10, 13, 0}, "not found page"},
		{"alter navigation set not found page", &ast.AlterNavigationStmt{ProfileName: "Responsive", Operations: []ast.MenuAlterOp{
			&ast.SetNavPageClauseOp{Clause: "notfound", Page: qn("M", "NotFound")}}},
			[3]int{10, 12, 0}, [3]int{10, 13, 0}, "not found page"},
		{"add jar dependency", &ast.AlterModuleJarDepStmt{ModuleName: "M", Actions: []ast.JarDepAction{
			&ast.AddJarDepAction{Group: "g", Artifact: "a", Version: "1"}}},
			[3]int{9, 24, 30}, [3]int{10, 0, 0}, "add jar dependency"},
		{"create an add-on module", &ast.CreateModuleStmt{Name: "M", Settings: ast.ModuleSettingsClause{ModuleType: &addon}},
			[3]int{9, 11, 0}, [3]int{9, 12, 0}, "ModuleType: addon on a module"},
		{"make a module an add-on", &ast.AlterModuleSetStmt{ModuleName: "M", Settings: ast.ModuleSettingsClause{ModuleType: &addon}},
			[3]int{9, 11, 0}, [3]int{9, 12, 0}, "ModuleType: addon on a module"},
		{"add a jar dependency exclusion", &ast.AlterModuleJarDepStmt{ModuleName: "M", Actions: []ast.JarDepAction{
			&ast.AddJarDepExclusionAction{}}},
			[3]int{10, 11, 0}, [3]int{10, 12, 0}, "add exclusion on a jar dependency"},
		{"create scheduled event", &ast.CreateScheduledEventStmt{Name: qn("M", "E"), Microflow: "M.Job", Repeat: "Daily"},
			[3]int{9, 11, 0}, [3]int{9, 12, 0}, "create scheduled event"},
		{"create database connection", &ast.CreateDatabaseConnectionStmt{Name: qn("M", "Db"), DatabaseType: "MSSQL"},
			[3]int{9, 24, 30}, [3]int{10, 0, 0}, "create database connection"},
		{"a database connection with a query", &ast.CreateDatabaseConnectionStmt{Name: qn("M", "Db"), DatabaseType: "MSSQL",
			Queries: []ast.DatabaseQueryDef{{Name: "All", SQL: "SELECT 1"}}},
			[3]int{10, 11, 0}, [3]int{10, 12, 0}, "a query of a database connection"},
		{"create business event service", &ast.CreateBusinessEventServiceStmt{Name: qn("M", "Events")},
			[3]int{9, 23, 0}, [3]int{9, 24, 0}, "create business event service"},
		{"a REST client with basic authentication", &ast.CreateRestClientStmt{Name: qn("M", "R"), Authentication: &ast.RestAuthDef{Scheme: "BASIC"}},
			[3]int{10, 1, 0}, [3]int{10, 2, 0}, "authentication: basic (on a consumed REST service)"},
		{"a REST client with an operation", &ast.CreateRestClientStmt{Name: qn("M", "R"), Operations: []*ast.RestOperationDef{getOp}},
			[3]int{10, 3, 0}, [3]int{10, 4, 0}, "an operation of a consumed REST service"},
		{"alter REST client add an operation", &ast.AlterRestClientStmt{Name: qn("M", "R"), Actions: []ast.RestClientAlterAction{
			&ast.RestClientAddOperationAction{Operation: getOp}}},
			[3]int{10, 3, 0}, [3]int{10, 4, 0}, "an operation of a consumed REST service"},
		{"a REST operation with a query parameter", &ast.CreateRestClientStmt{Name: qn("M", "R"), Operations: []*ast.RestOperationDef{queryOp}},
			[3]int{10, 24, 24}, [3]int{11, 0, 0}, "query: ( … ) on a REST operation"},
		{"a REST client from OpenAPI", &ast.CreateRestClientStmt{Name: qn("M", "R"), OpenApiPath: "spec.json"},
			[3]int{10, 20, 0}, [3]int{10, 21, 0}, "create consumed rest service from an OpenAPI document"},
		// The view entity's floor moves: its source document is 10.21.
		{"create view entity", &ast.CreateViewEntityStmt{Query: ast.OQLQuery{RawQuery: "select s.Code as Code\nfrom M.Sale as s"}},
			[3]int{10, 20, 0}, [3]int{10, 21, 0}, "create view entity"},
	})
}

// A statement that states none of the shapes is not gated by them on 9.0.2:
// a workflow with no parameter, a page and snippet without parameters or
// variables, a menu item without an icon, a REST client with no operation.
func TestCheckVersionGates_PlainShapesAreNotGated(t *testing.T) {
	qn := func(module, name string) ast.QualifiedName { return ast.QualifiedName{Module: module, Name: name} }
	app, version := "app", "2.0.0"
	for _, stmt := range []ast.Statement{
		&ast.CreateWorkflowStmt{Name: qn("M", "WF"), Activities: []ast.WorkflowActivityNode{&ast.WorkflowCallMicroflowNode{Microflow: qn("M", "Act")}}},
		&ast.CreateSnippetStmtV3{Name: qn("M", "S")},
		&ast.CreatePageStmtV3{Name: qn("M", "P"), Widgets: []*ast.WidgetV3{{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{"Action": &ast.ActionV3{Type: "close"}}}}},
		&ast.CreateMenuStmt{Name: qn("M", "Menu"), Items: []ast.NavMenuItemDef{{Caption: "Home"}}},
		&ast.CreateNavigationStmt{ProfileName: "Responsive", ProgressiveWebApp: &types.NavPWASpec{Off: true}},
		&ast.CreateNavigationStmt{ProfileName: "Responsive", HomePages: []ast.NavHomePageDef{{IsPage: true, Target: qn("M", "Home")}}},
		&ast.AlterModuleJarDepStmt{ModuleName: "M", Actions: []ast.JarDepAction{&ast.DropJarDepAction{}}},
		// An app module and a version are in every version's metamodel.
		&ast.AlterModuleSetStmt{ModuleName: "M", Settings: ast.ModuleSettingsClause{ModuleType: &app, Version: &version}},
	} {
		prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, stmt}}
		if vs := versionGateViolations(versionCtx(t, 9, 0, 2), prog); len(vs) != 0 {
			t.Errorf("%T reported on 9.0.2: %+v", stmt, vs)
		}
	}
	// A REST client is 10.1 (rest_client_basic): one with no operation, no
	// authentication and no OpenAPI document is reported for that alone.
	prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, &ast.CreateRestClientStmt{Name: qn("M", "R")}}}
	if vs := versionGateViolations(versionCtx(t, 10, 1, 0), prog); len(vs) != 0 {
		t.Errorf("a REST client with no operation reported on 10.1: %+v", vs)
	}
}
