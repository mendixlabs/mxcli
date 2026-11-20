// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// documentTypeGates are the gates of the document shapes whose storage
// $Types (or the properties that hold them) Mendix added after 9.0 and the
// writers produce with no older shape (ako/mxcli#1811). Each floor is the
// latest introduction among its types and properties in mendixmodelsdk
// 4.117.0, the version below which the write refuses the statement
// (canon.UndeclaredTypeError, canon.StripUndeclaredProperties) — pinned by
// TestTypeGates_FloorIsTheTypeIntroduction. The workflow shapes are
// workflowShapeGates.
var documentTypeGates = []statementTypeGate{
	{
		// create page has the same row in its own case; an alter adds one
		// to a page as surely (ako/mxcli#1121's floor, pinned here).
		typeGate: typeGate{
			versionGate: versionGate{"pages", "page_parameters", "add a page parameter",
				"pass data via a non-persistent entity or microflow parameter instead"},
			types:      []string{"Forms$PageParameter"},
			properties: []string{"Forms$Page.Parameters"},
		},
		applies: func(stmt ast.Statement) bool {
			s, ok := stmt.(*ast.AlterPageStmt)
			return ok && !strings.EqualFold(s.ContainerType, "SNIPPET") && alterPageAddsParameter(s)
		},
	},
	{
		// `optional` writes Forms$PageParameter.IsRequired = false, a property
		// below whose introduction every page parameter is required — the
		// writer leaves it out there, so the word would be dropped
		// (mendixlabs/mxcli#1640).
		typeGate: typeGate{
			versionGate: versionGate{"pages", "page_parameter_optional", "an optional page parameter",
				"below 11.5 every page parameter is required: remove `optional` and pass an argument wherever the page is opened, or upgrade the project"},
			types:      []string{"Forms$PageParameter"},
			properties: []string{"Forms$PageParameter.IsRequired"},
		},
		applies: func(stmt ast.Statement) bool {
			switch s := stmt.(type) {
			case *ast.CreatePageStmtV3:
				for _, p := range s.Parameters {
					if p.Optional {
						return true
					}
				}
			case *ast.AlterPageStmt:
				if strings.EqualFold(s.ContainerType, "SNIPPET") {
					return false // refused for a snippet whatever the version
				}
				for _, op := range s.Operations {
					if add, ok := op.(*ast.AddParameterOp); ok && add.Parameter.Optional {
						return true
					}
				}
			}
			return false
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"pages", "snippet_parameters", "a snippet parameter",
				"this version stores no snippet parameters (Forms$SnippetParameter): pass the object through the data context, or upgrade the project"},
			types:      []string{"Forms$SnippetParameter"},
			properties: []string{"Forms$Snippet.Parameters"},
		},
		applies: func(stmt ast.Statement) bool {
			switch s := stmt.(type) {
			case *ast.CreateSnippetStmtV3:
				return len(s.Parameters) > 0
			case *ast.AlterPageStmt:
				return strings.EqualFold(s.ContainerType, "SNIPPET") && alterPageAddsParameter(s)
			}
			return false
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"pages", "page_variables", "a page or snippet variable",
				"this version stores no page variables (Forms$LocalVariable): keep the value in a non-persistent object, or upgrade the project"},
			types: []string{"Forms$LocalVariable"},
			// MDL writes every variable with its default expression, and the
			// DefaultValue property is three minors later than the type.
			properties: []string{"Forms$Page.Variables", "Forms$LocalVariable.DefaultValue"},
		},
		applies: func(stmt ast.Statement) bool {
			switch s := stmt.(type) {
			case *ast.CreatePageStmtV3:
				return len(s.Variables) > 0
			case *ast.CreateSnippetStmtV3:
				return len(s.Variables) > 0
			case *ast.AlterPageStmt:
				for _, op := range s.Operations {
					if _, ok := op.(*ast.AddVariableOp); ok {
						return true
					}
				}
			}
			return false
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"pages", "complete_task_action", "action: complete task",
				"this version stores no outcome on a page's complete task action (Forms$SetTaskOutcomeClientAction.OutcomeValue): set the outcome from a microflow, or upgrade the project"},
			types:      []string{"Forms$SetTaskOutcomeClientAction"},
			properties: []string{"Forms$SetTaskOutcomeClientAction.OutcomeValue"},
		},
		applies: func(stmt ast.Statement) bool { return pageActionTypes(stmt)["completeTask"] },
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"pages", "workflow_page_actions", "action: open user task / open workflow / call workflow",
				"this version has no workflow page actions (Forms$OpenUserTaskClientAction, OpenWorkflowClientAction, CallWorkflowClientAction): upgrade the project"},
			types: []string{"Forms$OpenUserTaskClientAction", "Forms$OpenWorkflowClientAction", "Forms$CallWorkflowClientAction"},
		},
		applies: func(stmt ast.Statement) bool {
			t := pageActionTypes(stmt)
			return t["openUserTask"] || t["openWorkflow"] || t["callWorkflow"]
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"navigation", "menu_item_icons", "Icon: on a menu item",
				"this version has no icon collections (Forms$IconCollectionIcon): leave the icon out, or upgrade the project"},
			types: []string{"Forms$IconCollectionIcon"},
		},
		applies: statesMenuIcon,
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"navigation", "progressive_web_app", "progressive web app ( … )",
				"this version has no progressive web app settings (Navigation$ProgressiveWebAppSettings): leave the clause out, or upgrade the project"},
			types: []string{"Navigation$ProgressiveWebAppSettings"},
		},
		applies: func(stmt ast.Statement) bool {
			s, ok := stmt.(*ast.CreateNavigationStmt)
			return ok && s.ProgressiveWebApp != nil && !s.ProgressiveWebApp.Off
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"navigation", "native_home_page", "home page on a native profile",
				"this version stores a native profile's home page differently (Navigation$NativeHomePage), which mxcli does not write: set it in Studio Pro, or upgrade the project"},
			types: []string{"Navigation$NativeHomePage"},
		},
		applies: setsNativeHomePage,
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"navigation", "not_found_page", "not found page",
				"this version has no not-found page (Navigation$NotFoundHomePage): leave the clause out, or upgrade the project"},
			types:      []string{"Navigation$NotFoundHomePage"},
			properties: []string{"Navigation$NavigationProfile.NotFoundHomepage"},
		},
		applies: func(stmt ast.Statement) bool {
			switch s := stmt.(type) {
			case *ast.CreateNavigationStmt:
				return s.NotFoundPage != nil
			case *ast.AlterNavigationStmt:
				for _, op := range s.Operations {
					if o, ok := op.(*ast.SetNavPageClauseOp); ok && o.Clause == "notfound" {
						return true
					}
				}
			}
			return false
		},
	},
	{
		// An add-on module is ExportLevel Protected with ProtectedModuleType
		// AddOn; below 9.12 there is no ProtectedModuleType to write
		// (ako/mxcli#1921).
		typeGate: typeGate{
			versionGate: versionGate{"modules", "module_type", "ModuleType: addon on a module",
				"this version has no add-on modules (Projects$ModuleSettings.ProtectedModuleType): leave the module an app module, or upgrade the project"},
			types:      []string{"Projects$ModuleSettings"},
			properties: []string{"Projects$ModuleSettings.ProtectedModuleType"},
		},
		applies: func(stmt ast.Statement) bool {
			var c ast.ModuleSettingsClause
			switch s := stmt.(type) {
			case *ast.CreateModuleStmt:
				c = s.Settings
			case *ast.AlterModuleSetStmt:
				c = s.Settings
			}
			return c.ModuleType != nil && *c.ModuleType == "addon"
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"modules", "jar_dependencies", "add jar dependency",
				"this version stores no JAR dependencies on a module (Projects$JarDependency): put the JAR in userlib/, or upgrade the project"},
			types:      []string{"Projects$JarDependency"},
			properties: []string{"Projects$ModuleSettings.JarDependencies"},
		},
		applies: func(stmt ast.Statement) bool {
			return jarDepActions(stmt, func(a ast.JarDepAction) bool { _, ok := a.(*ast.AddJarDepAction); return ok })
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"modules", "jar_dependency_exclusions", "add exclusion on a jar dependency",
				"this version has no JAR dependency exclusions (Projects$JarDependencyExclusion): upgrade the project"},
			types: []string{"Projects$JarDependencyExclusion"},
		},
		applies: func(stmt ast.Statement) bool {
			return jarDepActions(stmt, func(a ast.JarDepAction) bool { _, ok := a.(*ast.AddJarDepExclusionAction); return ok })
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"scheduled_events", "schedules", "create scheduled event",
				"this version stores a scheduled event's interval, not a schedule (ScheduledEvents$*Schedule), which mxcli does not write: create it in Studio Pro, or upgrade the project"},
			types: []string{"ScheduledEvents$MinuteSchedule", "ScheduledEvents$HourSchedule", "ScheduledEvents$DaySchedule",
				"ScheduledEvents$WeekSchedule", "ScheduledEvents$MonthDateSchedule", "ScheduledEvents$MonthWeekdaySchedule",
				"ScheduledEvents$YearDateSchedule", "ScheduledEvents$YearWeekdaySchedule"},
			properties: []string{"ScheduledEvents$ScheduledEvent.Schedule", "ScheduledEvents$ScheduledEvent.OnOverlap"},
		},
		applies: func(stmt ast.Statement) bool {
			s, ok := stmt.(*ast.CreateScheduledEventStmt)
			return ok && s.Repeat != ""
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"integration", "database_connector_basic", "create database connection",
				"this version stores a database connection's connection input differently (DatabaseConnector$ConnectionString), which mxcli does not write: create it in Studio Pro, or upgrade the project"},
			types:      []string{"DatabaseConnector$DatabaseConnection", "DatabaseConnector$ConnectionString"},
			properties: []string{"DatabaseConnector$DatabaseConnection.ConnectionInput"},
		},
		applies: func(stmt ast.Statement) bool { _, ok := stmt.(*ast.CreateDatabaseConnectionStmt); return ok },
	},
	{
		// Measured on stamped projects: any query is refused below 10.12, on
		// its QueryType; one returning an entity on its TableMappings too.
		typeGate: typeGate{
			versionGate: versionGate{"integration", "database_connector_queries", "a query of a database connection",
				"this version stores a database query differently (DatabaseConnector$DatabaseQuery.QueryType), which mxcli does not write: create the connection without queries and add them in Studio Pro, or upgrade the project"},
			types:      []string{"DatabaseConnector$DatabaseQuery"},
			properties: []string{"DatabaseConnector$DatabaseQuery.QueryType", "DatabaseConnector$DatabaseQuery.TableMappings"},
		},
		applies: func(stmt ast.Statement) bool {
			s, ok := stmt.(*ast.CreateDatabaseConnectionStmt)
			return ok && len(s.Queries) > 0
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"integration", "business_events", "create business event service",
				"this version has no business event services (BusinessEvents$BusinessEventService): upgrade the project"},
			types: []string{"BusinessEvents$BusinessEventService", "BusinessEvents$BusinessEventDefinition",
				"BusinessEvents$Channel", "BusinessEvents$Message", "BusinessEvents$MessageAttribute", "BusinessEvents$ServiceOperation"},
		},
		applies: func(stmt ast.Statement) bool { _, ok := stmt.(*ast.CreateBusinessEventServiceStmt); return ok },
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"integration", "rest_client_authentication", "authentication: basic (on a consumed REST service)",
				"this version has no authentication scheme on a consumed REST service (Rest$BasicAuthenticationScheme): set the header yourself, or upgrade the project"},
			types:      []string{"Rest$BasicAuthenticationScheme"},
			properties: []string{"Rest$ConsumedRestService.AuthenticationScheme"},
		},
		applies: func(stmt ast.Statement) bool {
			switch s := stmt.(type) {
			case *ast.CreateRestClientStmt:
				return s.Authentication != nil
			case *ast.AlterRestClientStmt:
				for _, a := range s.Actions {
					if set, ok := a.(*ast.RestClientSetAction); ok && set.AuthenticationSet && set.Authentication != nil {
						return true
					}
				}
			}
			return false
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"integration", "rest_client_operations", "an operation of a consumed REST service",
				"this version stores a REST operation's method and response differently (Rest$RestOperationMethodWithBody / WithoutBody, Rest$NoResponseHandling), which mxcli does not write: create it in Studio Pro, or upgrade the project"},
			types: []string{"Rest$RestOperationMethodWithBody", "Rest$RestOperationMethodWithoutBody",
				"Rest$NoResponseHandling", "Rest$StringBody"},
			properties: []string{"Rest$RestOperation.Method", "Rest$RestOperation.ResponseHandling"},
		},
		applies: func(stmt ast.Statement) bool {
			return len(restClientOperations(stmt)) > 0 || openAPIRestClient(stmt)
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"integration", "rest_client_openapi", "create consumed rest service from an OpenAPI document",
				"this version stores no OpenAPI document on a consumed REST service (Rest$OpenApiFile): declare the operations, or upgrade the project"},
			types: []string{"Rest$OpenApiFile"},
		},
		applies: openAPIRestClient,
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"integration", "rest_client_query_params", "query: ( … ) on a REST operation",
				"this version has no query parameters on a REST operation (Rest$QueryParameter): put them in the path, or upgrade the project"},
			types:      []string{"Rest$QueryParameter", "Rest$RequiredQueryParameterUsage"},
			properties: []string{"Rest$RestOperation.QueryParameters"},
		},
		applies: func(stmt ast.Statement) bool {
			for _, op := range restClientOperations(stmt) {
				if len(op.QueryParameters) > 0 {
					return true
				}
			}
			return false
		},
	},
}

// documentGates are the documentTypeGates stmt states, in table order.
func documentGates(stmt ast.Statement) []versionGate {
	var gates []versionGate
	for _, g := range documentTypeGates {
		if g.applies(stmt) {
			gates = append(gates, g.versionGate)
		}
	}
	return gates
}

func alterPageAddsParameter(s *ast.AlterPageStmt) bool {
	for _, op := range s.Operations {
		if _, ok := op.(*ast.AddParameterOp); ok {
			return true
		}
	}
	return false
}

// pageActionTypes are the action types (ast.ActionV3.Type) the widgets a page,
// snippet or alter page statement writes carry, then-actions included.
func pageActionTypes(stmt ast.Statement) map[string]bool {
	out := map[string]bool{}
	visit := func(v any) {
		for a, _ := v.(*ast.ActionV3); a != nil; a = a.ThenAction {
			out[a.Type] = true
		}
	}
	forEachWidget(stmt, func(w *ast.WidgetV3, _ string) {
		for _, v := range w.Properties {
			visit(v)
		}
	})
	if s, ok := stmt.(*ast.AlterPageStmt); ok {
		for _, op := range s.Operations {
			if o, ok := op.(*ast.SetPropertyOp); ok {
				for _, v := range o.Properties {
					visit(v)
				}
			}
		}
	}
	return out
}

// statesMenuIcon reports a menu item the statement writes with an icon from an
// icon collection: the only icon `Icon:` can state.
func statesMenuIcon(stmt ast.Statement) bool {
	var items []ast.NavMenuItemDef
	var ops []ast.MenuAlterOp
	switch s := stmt.(type) {
	case *ast.CreateMenuStmt:
		items = s.Items
	case *ast.CreateNavigationStmt:
		items = s.MenuItems
	case *ast.AlterMenuStmt:
		ops = s.Operations
	case *ast.AlterNavigationStmt:
		ops = s.Operations
	}
	for _, op := range ops {
		switch o := op.(type) {
		case *ast.InsertMenuItemsOp:
			items = append(items, o.Items...)
		case *ast.ReplaceMenuItemOp:
			items = append(items, o.Items...)
		case *ast.SetMenuItemOp:
			if o.SetIcon {
				items = append(items, o.Item)
			}
		}
	}
	var walk func([]ast.NavMenuItemDef) bool
	walk = func(items []ast.NavMenuItemDef) bool {
		for _, it := range items {
			if it.IconKind == types.MenuIconCollection || walk(it.Items) {
				return true
			}
		}
		return false
	}
	return walk(items)
}

// setsNativeHomePage reports a default home page set on the native profile.
// The writer picks the native shape from the stored profile's type; the native
// profile is the one Studio Pro names NativePhone, which is how a statement
// addresses it.
func setsNativeHomePage(stmt ast.Statement) bool {
	native := func(profile string) bool { return strings.HasPrefix(strings.ToLower(profile), "native") }
	switch s := stmt.(type) {
	case *ast.CreateNavigationStmt:
		if !native(s.ProfileName) {
			return false
		}
		for _, hp := range s.HomePages {
			if hp.ForRole == nil {
				return true
			}
		}
	case *ast.AlterNavigationStmt:
		if !native(s.ProfileName) {
			return false
		}
		for _, op := range s.Operations {
			if o, ok := op.(*ast.SetNavPageClauseOp); ok && o.Clause == "home" && o.Home.ForRole == nil {
				return true
			}
		}
	}
	return false
}

func jarDepActions(stmt ast.Statement, match func(ast.JarDepAction) bool) bool {
	s, ok := stmt.(*ast.AlterModuleJarDepStmt)
	if !ok {
		return false
	}
	for _, a := range s.Actions {
		if match(a) {
			return true
		}
	}
	return false
}

// restClientOperations are the operations a create or alter of a consumed REST
// service declares (added or rewritten).
func restClientOperations(stmt ast.Statement) []*ast.RestOperationDef {
	switch s := stmt.(type) {
	case *ast.CreateRestClientStmt:
		return s.Operations
	case *ast.AlterRestClientStmt:
		var ops []*ast.RestOperationDef
		for _, a := range s.Actions {
			switch x := a.(type) {
			case *ast.RestClientAddOperationAction:
				ops = append(ops, x.Operation)
			case *ast.RestClientModifyOperationAction:
				ops = append(ops, x.Operation)
			}
		}
		return ops
	}
	return nil
}

// openAPIRestClient reports a consumed REST service created from an OpenAPI
// document: its operations come from the document, and the document is stored.
func openAPIRestClient(stmt ast.Statement) bool {
	s, ok := stmt.(*ast.CreateRestClientStmt)
	return ok && s.OpenApiPath != ""
}
