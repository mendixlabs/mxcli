// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// An ALTER PAGE / SNIPPET on a document the SAME script creates was never
// resolved: the stored-document passes (validateAlterPageReferences,
// validateAlterSetProperties) skip it, since there is nothing stored to open,
// and nothing took their place. So
//
//	create page M.P (…) { placeholder Main { use building block B.Alert as a_ } };
//	alter page M.P { set (Class: 'x') on a_container99 };
//
// passed `check -p` and exec then stopped on `widget "a_container99" not found`
// with the create already applied (ako/mxcli#2182, mendixlabs/mxcli#1835). The
// same held for a plain misspelt widget; the building block is only the form in
// which the name is hardest to get right, because the page source never spells
// it — exec expands the block and prefixes its widgets.
//
// The names a script-created document holds are knowable from the script: the
// tree its CREATE writes, expanded as exec expands it (expandUseRefsForCheck,
// the shared pre-pass — not a second expander), then each ALTER's inserts and
// renames in statement order. This pass replays that and refuses a target that
// is not among them.
//
// It never guesses in the direction that blocks a working script:
//
//   - a `use building block` / `use fragment` it cannot expand — no project, an
//     unknown block, a fragment the script itself defines — makes every name
//     under its prefix possible, and an unprefixed one every name at all;
//   - a document holding widgets whose items are addressed by DERIVED names (a
//     DataGrid 2 column by its attribute, an object-list item) accepts every
//     name, since those are not the names the script writes;
//   - only a plain name is judged: a column, item, cell, caption or @n address
//     is left to exec;
//   - `create … if not exists` may leave the stored document in place, and a
//     rename, move or drop ends what the script can know about it.
//
// So without a project, a script whose blocks cannot be expanded behaves as
// before for the names under the block's prefix, while a misspelt name of the
// page's own widgets is reported either way.

// knownWidgetNames is what check can know about the widget names one document
// holds at a point in the script.
type knownWidgetNames struct {
	names map[string]bool
	// prefixes are those of use references check could not expand: any name
	// beginning with one may exist.
	prefixes []string
	// unexpanded means an UNPREFIXED use reference could not be expanded, so
	// any name may be one of its widgets.
	unexpanded bool
	// derived means the widgets hold something addressed by a name the script
	// does not write (hasDerivedAddresses), so any name may address it.
	derived bool
}

func newKnownWidgetNames() *knownWidgetNames {
	return &knownWidgetNames{names: map[string]bool{}}
}

// allows reports whether name may address something on the document. A nil
// set knows nothing, so it allows everything.
func (n *knownWidgetNames) allows(name string) bool {
	return n == nil || n.derived || n.declares(name)
}

// declares reports whether name may be a widget name the widgets put on the
// document — written, renamed to, or produced by an expansion — leaving
// derived addresses aside.
func (n *knownWidgetNames) declares(name string) bool {
	if n == nil {
		return false
	}
	if n.unexpanded || n.names[name] {
		return true
	}
	for _, p := range n.prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// add records the names the widgets put on the document, after the expansion
// exec runs. ctx may be nil (no project): every use reference is then
// unexpanded.
func (n *knownWidgetNames) add(ctx *ExecContext, widgets []*ast.WidgetV3) {
	built := expandUseRefsForCheckReporting(ctx, widgets, func(prefix string) {
		if prefix == "" {
			n.unexpanded = true
			return
		}
		n.prefixes = append(n.prefixes, prefix)
	})
	registry := pageContextWidgetRegistry(ctx)
	var walk func(ws []*ast.WidgetV3, parentType string)
	walk = func(ws []*ast.WidgetV3, parentType string) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			if w.Name != "" {
				n.names[w.Name] = true
			}
			if hasDerivedAddresses(registry, w, parentType) {
				n.derived = true
			}
			walk(w.Children, strings.ToLower(w.Type))
		}
	}
	walk(built, "")
}

// hasDerivedAddresses reports whether w puts on the page something a target
// may name by a name the script does not write: a grid column (a layout grid's
// column sits in a row, and is not one), an object-list item, or anything
// inside a widget whose definition check cannot see.
func hasDerivedAddresses(registry *WidgetRegistry, w *ast.WidgetV3, parentType string) bool {
	switch strings.ToLower(w.Type) {
	case "column":
		return parentType != "row"
	case "pluggablewidget", "customwidget":
		return len(w.Children) > 0
	}
	return len(w.Children) > 0 && len(unstoredContainerKinds(registry, w)) > 0
}

// rename records a SET that renames a widget.
func (n *knownWidgetNames) rename(op *ast.SetPropertyOp) {
	for k, v := range op.Properties {
		if s, ok := v.(string); ok && strings.EqualFold(k, "Name") {
			n.names[s] = true
		}
	}
}

// suggestion names the closest known name, for the refusal.
func (n *knownWidgetNames) suggestion(name string) string {
	known := make([]string, 0, len(n.names))
	for k := range n.names {
		known = append(known, k)
	}
	sort.Strings(known)
	if near := nearestKey(name, known); near != "" {
		return fmt.Sprintf(" — did you mean `%s`?", near)
	}
	return ""
}

// plainWidgetTarget reports whether ref addresses a widget by its name alone —
// the one form this pass judges.
func plainWidgetTarget(ref ast.WidgetRef) bool {
	return ref.Widget != "" && ref.Column == "" && ref.Caption == "" && ref.Ordinal == 0 &&
		ref.ColumnAttribute == "" && ref.ColumnCaption == "" && ref.Item == nil && ref.Cell == nil
}

// ValidateScriptWidgetTargets is the ValidateProgram entry: the project is
// opened only when a block or fragment has to be expanded.
func ValidateScriptWidgetTargets(prog *ast.Program, project func() backend.FullBackend) []linter.Violation {
	var ctx *ExecContext
	opened := false
	ctxFor := func() *ExecContext {
		if !opened {
			opened = true
			if project != nil {
				if b := project(); b != nil {
					ctx = &ExecContext{Backend: b}
				}
			}
		}
		return ctx
	}
	return validateScriptWidgetTargets(prog, ctxFor)
}

// validateScriptWidgetTargets replays the documents the script creates and
// reports each ALTER target none of them holds at that point.
func validateScriptWidgetTargets(prog *ast.Program, ctxFor func() *ExecContext) []linter.Violation {
	if prog == nil || !altersAScriptDocument(prog) {
		return nil
	}
	ctxIfNeeded := func(ws []*ast.WidgetV3) *ExecContext {
		if !hasUseRef(ws) {
			return nil
		}
		return ctxFor()
	}
	docs := map[string]*knownWidgetNames{}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			key := "page " + s.Name.String()
			if s.Name.Module == "" || s.CreateIfNotExists() {
				delete(docs, key)
				continue
			}
			ws := allPageWidgets(s)
			set := newKnownWidgetNames()
			set.add(ctxIfNeeded(ws), ws)
			docs[key] = set
		case *ast.CreateSnippetStmtV3:
			key := "snippet " + s.Name.String()
			if s.Name.Module == "" || s.CreateIfNotExists() {
				delete(docs, key)
				continue
			}
			set := newKnownWidgetNames()
			set.add(ctxIfNeeded(s.Widgets), s.Widgets)
			docs[key] = set
		case *ast.DropPageStmt:
			delete(docs, "page "+s.Name.String())
		case *ast.DropSnippetStmt:
			delete(docs, "snippet "+s.Name.String())
		case *ast.RenameStmt:
			delete(docs, strings.ToLower(s.ObjectType)+" "+s.Name.String())
		case *ast.MoveStmt:
			delete(docs, moveTargetKind(s.DocumentType)+" "+s.Name.String())
		case *ast.AlterPageStmt:
			kind := strings.ToLower(s.ContainerType)
			if kind == "" {
				kind = "page"
			}
			set := docs[kind+" "+s.PageName.String()]
			if set == nil {
				continue
			}
			for _, op := range s.Operations {
				// exec resolves an operation's targets before it runs it.
				for _, ref := range alterPageOperationTargets(op) {
					if !plainWidgetTarget(ref) || set.allows(ref.Widget) {
						continue
					}
					out = append(out, linter.Violation{
						RuleID:   "MDL-ALTER02",
						Severity: linter.SeverityError,
						Message: fmt.Sprintf("alter %s %s: widget %q not found (the %s is created earlier in this script "+
							"and holds no widget of that name here; a building block's widgets carry its `as` prefix)%s",
							kind, s.PageName.String(), ref.Widget, kind, set.suggestion(ref.Widget)),
						Location: linter.Location{
							Module:       s.PageName.Module,
							DocumentType: kind,
							DocumentName: s.PageName.Name,
						},
					})
				}
				switch o := op.(type) {
				case *ast.InsertWidgetOp:
					set.add(ctxIfNeeded(o.Widgets), o.Widgets)
				case *ast.ReplaceWidgetOp:
					set.add(ctxIfNeeded(o.NewWidgets), o.NewWidgets)
				case *ast.SetPropertyOp:
					set.rename(o)
				}
			}
		}
	}
	return out
}

// altersAScriptDocument reports whether any ALTER PAGE / SNIPPET follows a
// CREATE of the same document — the only case this pass has anything to say
// about, so every other script costs one walk of the statement list.
func altersAScriptDocument(prog *ast.Program) bool {
	created := map[string]bool{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			created["page "+s.Name.String()] = true
		case *ast.CreateSnippetStmtV3:
			created["snippet "+s.Name.String()] = true
		case *ast.AlterPageStmt:
			kind := strings.ToLower(s.ContainerType)
			if kind == "" {
				kind = "page"
			}
			if created[kind+" "+s.PageName.String()] {
				return true
			}
		}
	}
	return false
}

// hasUseRef reports whether the tree holds a `use building block` or
// `use fragment` reference — the only thing that needs the project to name.
func hasUseRef(ws []*ast.WidgetV3) bool {
	for _, w := range ws {
		if w == nil {
			continue
		}
		if w.Type == "USE_BUILDING_BLOCK" || w.Type == "USE_FRAGMENT" || hasUseRef(w.Children) {
			return true
		}
	}
	return false
}
