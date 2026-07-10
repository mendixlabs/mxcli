// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// stmtTypeName returns the short type name of a statement (without package prefix).
func stmtTypeName(stmt ast.Statement) string {
	t := fmt.Sprintf("%T", stmt)
	// Remove "*ast." prefix
	if i := strings.LastIndex(t, "."); i >= 0 {
		return t[i+1:]
	}
	return t
}

// stmtSummary returns a safe one-line summary of a statement for logging.
func stmtSummary(stmt ast.Statement) string {
	switch s := stmt.(type) {
	// Connection
	case *ast.ConnectStmt:
		return fmt.Sprintf("connect local '%s'", s.Path)
	case *ast.DisconnectStmt:
		return "disconnect"
	case *ast.StatusStmt:
		return "status"

	// Module
	case *ast.CreateModuleStmt:
		return fmt.Sprintf("create module %s", s.Name)
	case *ast.DropModuleStmt:
		return fmt.Sprintf("drop module %s", s.Name)
	case *ast.AlterModuleJarDepStmt:
		return fmt.Sprintf("alter module %s jar dep (%d actions)", s.ModuleName, len(s.Actions))

	// Entity
	case *ast.CreateEntityStmt:
		return fmt.Sprintf("create entity %s", s.Name)
	case *ast.CreateViewEntityStmt:
		return fmt.Sprintf("create view entity %s", s.Name)
	case *ast.DropEntityStmt:
		return fmt.Sprintf("drop entity %s", s.Name)

	// Association
	case *ast.CreateAssociationStmt:
		return fmt.Sprintf("create association %s", s.Name)
	case *ast.DropAssociationStmt:
		return fmt.Sprintf("drop association %s", s.Name)

	// Enumeration
	case *ast.CreateEnumerationStmt:
		return fmt.Sprintf("create enumeration %s", s.Name)
	case *ast.AlterEnumerationStmt:
		return fmt.Sprintf("alter enumeration %s", s.Name)
	case *ast.DropEnumerationStmt:
		return fmt.Sprintf("drop enumeration %s", s.Name)

	// Microflow
	case *ast.CreateMicroflowStmt:
		return fmt.Sprintf("create microflow %s", s.Name)
	case *ast.DropMicroflowStmt:
		return fmt.Sprintf("drop microflow %s", s.Name)

	// Page
	case *ast.CreatePageStmtV3:
		return fmt.Sprintf("create page %s", s.Name)
	case *ast.DropPageStmt:
		return fmt.Sprintf("drop page %s", s.Name)
	case *ast.CreateSnippetStmtV3:
		return fmt.Sprintf("create snippet %s", s.Name)
	case *ast.DropSnippetStmt:
		return fmt.Sprintf("drop snippet %s", s.Name)

	// Java actions
	case *ast.CreateJavaActionStmt:
		return fmt.Sprintf("create java action %s", s.Name)
	case *ast.DropJavaActionStmt:
		return fmt.Sprintf("drop java action %s", s.Name)
	case *ast.CreateJavaScriptActionStmt:
		return fmt.Sprintf("create javascript action %s", s.Name)
	case *ast.DropJavaScriptActionStmt:
		return fmt.Sprintf("drop javascript action %s", s.Name)

	// Move
	case *ast.MoveStmt:
		return fmt.Sprintf("move %s %s", s.DocumentType, s.Name)

	// Security
	case *ast.CreateModuleRoleStmt:
		return fmt.Sprintf("create module role %s", s.Name)
	case *ast.DropModuleRoleStmt:
		return fmt.Sprintf("drop module role %s", s.Name)
	case *ast.CreateUserRoleStmt:
		return fmt.Sprintf("create user role %s", s.Name)
	case *ast.DropUserRoleStmt:
		return fmt.Sprintf("drop user role %s", s.Name)
	case *ast.GrantMicroflowAccessStmt:
		return fmt.Sprintf("grant execute on microflow %s", s.Microflow)
	case *ast.RevokeMicroflowAccessStmt:
		return fmt.Sprintf("revoke execute on microflow %s", s.Microflow)
	case *ast.GrantNanoflowAccessStmt:
		return fmt.Sprintf("grant execute on nanoflow %s", s.Nanoflow)
	case *ast.RevokeNanoflowAccessStmt:
		return fmt.Sprintf("revoke execute on nanoflow %s", s.Nanoflow)
	case *ast.GrantPageAccessStmt:
		return fmt.Sprintf("grant view on page %s", s.Page)
	case *ast.RevokePageAccessStmt:
		return fmt.Sprintf("revoke view on page %s", s.Page)
	case *ast.GrantWorkflowAccessStmt:
		return fmt.Sprintf("grant execute on workflow %s", s.Workflow)
	case *ast.RevokeWorkflowAccessStmt:
		return fmt.Sprintf("revoke execute on workflow %s", s.Workflow)
	case *ast.GrantEntityAccessStmt:
		return fmt.Sprintf("grant on entity %s", s.Entity)
	case *ast.RevokeEntityAccessStmt:
		return fmt.Sprintf("revoke on entity %s", s.Entity)
	case *ast.AlterProjectSecurityStmt:
		return "alter project security"
	case *ast.CreateDemoUserStmt:
		return fmt.Sprintf("create demo user %s", s.UserName)
	case *ast.DropDemoUserStmt:
		return fmt.Sprintf("drop demo user %s", s.UserName)
	case *ast.CreateExternalEntityStmt:
		return fmt.Sprintf("create external entity %s", s.Name)
	case *ast.GrantODataServiceAccessStmt:
		return fmt.Sprintf("grant access on odata service %s", s.Service)
	case *ast.RevokeODataServiceAccessStmt:
		return fmt.Sprintf("revoke access on odata service %s", s.Service)
	case *ast.GrantPublishedRestServiceAccessStmt:
		return fmt.Sprintf("grant access on published rest service %s", s.Service)
	case *ast.RevokePublishedRestServiceAccessStmt:
		return fmt.Sprintf("revoke access on published rest service %s", s.Service)

	// Image Collection
	case *ast.CreateImageCollectionStmt:
		return fmt.Sprintf("create image collection %s", s.Name)
	case *ast.DropImageCollectionStmt:
		return fmt.Sprintf("drop image collection %s", s.Name)

	// Database Connection
	case *ast.CreateDatabaseConnectionStmt:
		return fmt.Sprintf("create database connection %s", s.Name)

	// Business Events
	case *ast.CreateBusinessEventServiceStmt:
		return fmt.Sprintf("create business event service %s", s.Name)
	case *ast.DropBusinessEventServiceStmt:
		return fmt.Sprintf("drop business event service %s", s.Name)

	// Settings
	case *ast.AlterSettingsStmt:
		return fmt.Sprintf("alter settings %s", s.Section)

	// Navigation
	case *ast.AlterNavigationStmt:
		return fmt.Sprintf("create navigation %s", s.ProfileName)

	// Query
	case *ast.ShowStmt:
		summary := fmt.Sprintf("show %s", s.ObjectType)
		if s.Name != nil {
			summary += " " + s.Name.String()
		}
		if s.InModule != "" {
			summary += " in " + s.InModule
		}
		return summary
	case *ast.DescribeStmt:
		return fmt.Sprintf("describe %v %s", s.ObjectType, s.Name)
	case *ast.SelectStmt:
		return "select ..."
	case *ast.SearchStmt:
		return fmt.Sprintf("search '%s'", s.Query)
	case *ast.ShowWidgetsStmt:
		return "show widgets"
	case *ast.UpdateWidgetsStmt:
		return "update widgets"
	case *ast.ShowDesignPropertiesStmt:
		if s.WidgetType != "" {
			return fmt.Sprintf("show design properties for %s", s.WidgetType)
		}
		return "show design properties"
	case *ast.DescribeStylingStmt:
		summary := fmt.Sprintf("describe styling on %s %s", s.ContainerType, s.ContainerName)
		if s.WidgetName != "" {
			summary += " widget " + s.WidgetName
		}
		return summary
	case *ast.AlterStylingStmt:
		return fmt.Sprintf("alter styling on %s %s widget %s", s.ContainerType, s.ContainerName, s.WidgetName)

	// ALTER PAGE / ALTER SNIPPET
	case *ast.AlterPageStmt:
		ct := s.ContainerType
		if ct == "" {
			ct = "page"
		}
		return fmt.Sprintf("alter %s %s", ct, s.PageName)

	// Fragments
	case *ast.DefineFragmentStmt:
		return fmt.Sprintf("define fragment %s", s.Name)
	case *ast.DescribeFragmentFromStmt:
		return fmt.Sprintf("describe fragment from %s %s widget %s", s.ContainerType, s.ContainerName, s.WidgetName)

	// SQL
	case *ast.SQLConnectStmt:
		return fmt.Sprintf("sql connect %s as %s", s.Driver, s.Alias)
	case *ast.SQLDisconnectStmt:
		return fmt.Sprintf("sql disconnect %s", s.Alias)
	case *ast.SQLConnectionsStmt:
		return "sql connections"
	case *ast.SQLQueryStmt:
		q := s.Query
		if len(q) > 40 {
			q = q[:40] + "..."
		}
		return fmt.Sprintf("sql %s %s", s.Alias, q)
	case *ast.SQLShowTablesStmt:
		return fmt.Sprintf("sql %s show tables", s.Alias)
	case *ast.SQLShowViewsStmt:
		return fmt.Sprintf("sql %s show views", s.Alias)
	case *ast.SQLShowFunctionsStmt:
		return fmt.Sprintf("sql %s show FUNCTIONS", s.Alias)
	case *ast.SQLDescribeTableStmt:
		return fmt.Sprintf("sql %s describe %s", s.Alias, s.Table)
	case *ast.SQLGenerateConnectorStmt:
		return fmt.Sprintf("sql %s generate connector into %s", s.Alias, s.Module)

	// Import
	case *ast.ImportStmt:
		summary := fmt.Sprintf("import from %s into %s (%d mappings", s.SourceAlias, s.TargetEntity, len(s.Mappings))
		if len(s.Links) > 0 {
			summary += fmt.Sprintf(", %d links", len(s.Links))
		}
		return summary + ")"

	// Repository
	case *ast.RefreshCatalogStmt:
		return "refresh catalog"
	case *ast.RefreshStmt:
		return "refresh"
	// Session
	case *ast.ExitStmt:
		return "EXIT"
	case *ast.HelpStmt:
		return "HELP"
	case *ast.ExecuteScriptStmt:
		return fmt.Sprintf("execute '%s'", s.Path)
	case *ast.LintStmt:
		return "lint"

	default:
		return stmtTypeName(stmt)
	}
}
