// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// droppedMemberRefRule is the rule ID for "this script drops an attribute, entity
// or association that a microflow, nanoflow, page or snippet still uses".
const droppedMemberRefRule = "MDL108"

// Dropping an attribute, an entity or an association that a document still uses
// breaks that document. Measured on MxBuild 11.16.1, ONE project per case (CE1613
// is an unresolved reference and hides every other error):
//
//	drop attribute used in a microflow `change`      CE1613 on the microflow
//	  ... in a page, a snippet or a nanoflow         CE1613 on each
//	  ... only in a retrieve's XPath                 CE0161 on the microflow
//	  ... only in an expression (`$P/Code`)          CE0117 on the microflow or snippet
//	drop entity used as a microflow parameter        CE1613
//	drop association used by a retrieve or a page    CE1613
//	drop entity that only an association points at   clean (the association goes with it)
//	drop the attribute, then add it again            clean
//
// `check` passed every rejected case and `exec` applied it, with at most a "may
// still reference" warning — `drop entity` said nothing at all — so the project
// stopped building and only `mx check` said why. The catalog already holds the
// references (`show references to`), so this reads them.
//
// What is NOT reported: a document the script itself drops, creates, replaces or
// alters (it may well fix the reference, and a verdict on a half-known result would
// be a guess); a member the script puts back; a source of a type the build was not
// measured on (a mapping, a workflow, a navigation). The catalog is a full build, so
// it is only opened for a script that drops one of these — a script that does not
// pays nothing.
func (e *Executor) CheckDroppedMemberReferences(prog *ast.Program) []linter.Violation {
	if e == nil || prog == nil || !scriptDropsReferencedKind(prog) {
		return nil
	}
	ctx := e.newExecContext(context.Background())
	if !ctx.Connected() {
		return nil
	}
	if err := ensureCatalog(ctx, true); err != nil {
		return nil
	}
	return CheckDroppedMemberReferences(ctx, prog)
}

// measuredReferenceSources are the document kinds MxBuild was measured to reject
// (CE1613, or CE0161 for an XPath) when something they use is gone.
var measuredReferenceSources = "'MICROFLOW','NANOFLOW','PAGE','SNIPPET'"

func scriptDropsReferencedKind(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.DropEntityStmt, *ast.DropAssociationStmt:
			return true
		case *ast.AlterEntityStmt:
			if s.Operation == ast.AlterEntityDropAttribute {
				return true
			}
		}
	}
	return false
}

// CheckDroppedMemberReferences is the ExecContext-level entry point; it judges
// against the catalog already loaded in ctx and returns nothing without one.
func CheckDroppedMemberReferences(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || ctx.Catalog == nil || prog == nil {
		return nil
	}
	rewritten := rewrittenDocuments(prog)
	var out []linter.Violation
	for i, stmt := range prog.Statements {
		var (
			target, what string
			prefix       bool
			module, doc  string
			restored     func(ast.Statement) bool
		)
		switch s := stmt.(type) {
		case *ast.AlterEntityStmt:
			if s.Operation != ast.AlterEntityDropAttribute {
				continue
			}
			target = s.Name.String() + "." + s.AttributeName
			what = fmt.Sprintf("alter entity %s drop attribute %q", s.Name, s.AttributeName)
			module, doc = s.Name.Module, s.Name.Name
			restored = func(later ast.Statement) bool { return restoresAttribute(later, s.Name, s.AttributeName) }
		case *ast.DropEntityStmt:
			target, prefix = s.Name.String(), true
			what = "drop entity " + s.Name.String()
			module, doc = s.Name.Module, s.Name.Name
			restored = func(later ast.Statement) bool {
				c, ok := later.(*ast.CreateEntityStmt)
				return ok && strings.EqualFold(c.Name.String(), s.Name.String())
			}
		case *ast.DropAssociationStmt:
			target = s.Name.String()
			what = "drop association " + s.Name.String()
			module, doc = s.Name.Module, s.Name.Name
			restored = func(later ast.Statement) bool {
				c, ok := later.(*ast.CreateAssociationStmt)
				return ok && strings.EqualFold(c.Name.String(), s.Name.String())
			}
		default:
			continue
		}
		put := false
		for _, later := range prog.Statements[i+1:] {
			if restored(later) {
				put = true
				break
			}
		}
		if put {
			continue
		}
		users := referencingDocuments(ctx, target, prefix, rewritten)
		if len(users) == 0 {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   droppedMemberRefRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"`%s` leaves %s still using it — mxbuild reports CE1613 \"The selected … no longer exists\" on each "+
					"(CE0117 where it is in an expression, CE0161 in an XPath), and exec applies the drop and only warns",
				what, describeUsers(users)),
			Location:   linter.Location{Module: module, DocumentType: "entity", DocumentName: doc},
			Suggestion: "Change those documents first (an `alter` of each, or drop them in this script), or keep the member. `show references to " + target + "` lists them.",
		})
	}
	return out
}

// restoresAttribute reports whether a later statement puts the attribute back.
func restoresAttribute(stmt ast.Statement, entity ast.QualifiedName, attr string) bool {
	switch s := stmt.(type) {
	case *ast.AlterEntityStmt:
		return s.Operation == ast.AlterEntityAddAttribute && s.Attribute != nil &&
			strings.EqualFold(s.Name.String(), entity.String()) && strings.EqualFold(s.Attribute.Name, attr)
	case *ast.CreateEntityStmt:
		if !strings.EqualFold(s.Name.String(), entity.String()) {
			return false
		}
		for _, a := range s.Attributes {
			if strings.EqualFold(a.Name, attr) {
				return true
			}
		}
	}
	return false
}

type referenceUser struct{ kind, name string }

// referencingDocuments is every measured document that refers to target (or, for
// an entity, to one of its members) and that the script does not touch.
func referencingDocuments(ctx *ExecContext, target string, prefix bool, rewritten map[string]bool) []referenceUser {
	esc := escapeSQLString(target)
	where := fmt.Sprintf("lower(TargetName) = lower('%s')", esc)
	if prefix {
		where = fmt.Sprintf(`(%s or lower(TargetName) like lower('%s.%%') escape '\')`, where, escapeSQLString(escapeSQLLike(target)))
	}
	res, err := ctx.Catalog.Query(`select distinct SourceType, SourceName from refs where ` + where +
		` and SourceType in (` + measuredReferenceSources + `) order by SourceType, SourceName`)
	if err != nil || res == nil {
		return nil
	}
	var out []referenceUser
	for _, row := range res.Rows {
		if len(row) < 2 {
			continue
		}
		kind, name := strings.ToLower(fmt.Sprint(row[0])), fmt.Sprint(row[1])
		if rewritten[strings.ToLower(name)] {
			continue
		}
		out = append(out, referenceUser{kind: kind, name: name})
	}
	return out
}

func describeUsers(users []referenceUser) string {
	const show = 6
	parts := make([]string, 0, show)
	for i, u := range users {
		if i == show {
			break
		}
		parts = append(parts, u.kind+" "+u.name)
	}
	s := strings.Join(parts, ", ")
	if len(users) > show {
		s += fmt.Sprintf(" and %d more", len(users)-show)
	}
	return s
}

// rewrittenDocuments is the lower-cased qualified names of the documents the
// script drops, creates, replaces or alters.
func rewrittenDocuments(prog *ast.Program) map[string]bool {
	out := map[string]bool{}
	add := func(q ast.QualifiedName) { out[strings.ToLower(q.String())] = true }
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.DropMicroflowStmt:
			add(s.Name)
		case *ast.DropNanoflowStmt:
			add(s.Name)
		case *ast.DropPageStmt:
			add(s.Name)
		case *ast.DropSnippetStmt:
			add(s.Name)
		case *ast.CreateMicroflowStmt:
			add(s.Name)
		case *ast.CreateNanoflowStmt:
			add(s.Name)
		case *ast.CreatePageStmtV3:
			add(s.Name)
		case *ast.CreateSnippetStmtV3:
			add(s.Name)
		case *ast.AlterFlowStmt:
			add(s.Name)
		case *ast.AlterPageStmt:
			add(s.PageName)
		}
	}
	return out
}
