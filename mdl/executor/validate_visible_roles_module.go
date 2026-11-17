// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// visibleRolesModuleRule is MDL-GRANT04: `Visible: roles (…)` naming a module role
// of another module than the document the widget is in. It is MDL-GRANT01's
// sibling — CE0148 for a document grant, CE0149 for a widget's visibility.
const visibleRolesModuleRule = "MDL-GRANT04"

// ValidateVisibleRolesModule reports a widget whose `Visible: roles (…)` names a
// role of a module other than its page's, snippet's or layout's own.
//
// Mendix stores a widget's role-based visibility as references to module roles of
// the document's own module only, so a role of another module builds with CE0149
// "Visibility needs to be updated, reselect roles in properties". Measured on
// MxBuild 11.16.1 over 17 documents, one `mx check -j`: a container, a dynamic
// text, a layout-grid row, a navigation-list item, a tab page, a snippet
// container, a layout container and a widget inserted by `alter page` are each
// CE0149 for Administration.Administrator or a role of a user-created module, and
// clean for a role of their own module; a list mixing an own and a foreign role is
// CE0149. Whether the page may be VIEWED by the role does not matter: only the
// role's module does.
//
// `check` passed it and `exec` wrote it, so the first sign was the build. Like
// MDL-GRANT01 it needs no project — both module names are in the script, the
// document's from its own statement — so it runs in the pre-flight pass `check`
// and `exec` share, not only under --references.
//
// Names compare without regard to letter case: whether Mendix distinguishes
// `Wd` from `wd` is unmeasured, and treating them as one module can only silence a
// report. A role written without a module is left to the reference check, which
// cannot resolve it either.
func ValidateVisibleRolesModule(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		module, kind, ok := visibleRolesDocument(stmt)
		if !ok {
			continue
		}
		forEachWidget(stmt, func(w *ast.WidgetV3, where string) {
			var foreign []string
			for _, r := range widgetVisibleRoles(w) {
				if qn, ok := splitQualified(r); ok && !strings.EqualFold(qn.Module, module) {
					foreign = append(foreign, r)
				}
			}
			if len(foreign) == 0 {
				return
			}
			out = append(out, linter.Violation{
				RuleID:   visibleRolesModuleRule,
				Severity: linter.SeverityError,
				Message: fmt.Sprintf("%s: Visible: roles names %s, a role of another module than the %s's own module %q — "+
					"a widget can only be shown to its own module's roles; Mendix reports CE0149 \"Visibility needs to be "+
					"updated, reselect roles in properties\"", where, strings.Join(foreign, ", "), kind, module),
				Suggestion: fmt.Sprintf("Use a role of module %s (e.g. %s.<Role>) and map the user role to it "+
					"(`alter user role <UserRole> add %s.<Role>`).", module, module, module),
				Location: linter.Location{Module: module, DocumentType: kind},
			})
		})
	}
	return out
}

// visibleRolesDocument is the module and kind of the document a statement's
// widgets belong to, for the statements forEachWidget walks. The module is taken
// from the statement's own qualified name, so the check works before the document
// exists — it may be created earlier in the same script.
func visibleRolesDocument(stmt ast.Statement) (module, kind string, ok bool) {
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		return s.Name.Module, "page", true
	case *ast.CreateSnippetStmtV3:
		return s.Name.Module, "snippet", true
	case *ast.CreateLayoutStmt:
		return s.Name.Module, "layout", true
	case *ast.AlterPageStmt:
		return s.PageName.Module, strings.ToLower(s.ContainerType), true
	}
	return "", "", false
}
