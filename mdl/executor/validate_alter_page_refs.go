// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// ALTER PAGE / SNIPPET / LAYOUT was resolved down to its document and no
// further (mendixlabs/mxcli#1472). Two kinds of reference inside it passed
// `check -p --references` and were then refused by exec, or written for
// mxbuild to report as CE1613:
//
//   - the widget an INSERT is placed against, a REPLACE replaces, a DROP
//     removes — exec resolves each through the document's AlterTargetResolver
//     before the operation runs, and stops on a miss;
//   - the flows, pages, snippets and entities the inserted or replacing
//     widgets name — CREATE PAGE resolved those (validateWidgetReferences),
//     ALTER never did.
//
// Both are answered by what exec itself runs: the targets through
// resolveAlterPageTargets against the stored document (opened read-only, never
// saved), the widget references through validateWidgetReferences. A SET's
// target is not resolved here — validateAlterSetProperties dry-runs the SET and
// already reports it, and saying it twice reads as two problems.
//
// "Could not establish" stays silent: a document that will not open, a target
// the script itself puts on the document (or renames a widget to), and a grid
// column on a document an EARLIER operation adds widgets to (a column is
// addressed by a derived name, so a name match cannot say whether the script
// adds it). Earlier only: exec resolves an operation's targets before it runs,
// so a REPLACE's own column address is against what is already there. Waiving
// it too passed `replace grid1 column(C) …` on a grid with no column C, which
// exec then refused (found reproducing mendixlabs/mxcli#1512).

// validateAlterPageReferences reports the unresolvable targets and widget
// references of every ALTER PAGE / SNIPPET / LAYOUT in the script.
func validateAlterPageReferences(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	h, _ := getHierarchy(ctx)
	added := widgetsTheScriptAddsTo(ctx, prog)
	grown := map[string]bool{} // documents an operation so far has added widgets to
	// The widgets an operation so far has dropped, per document: this pass
	// reads the stored document, where exec has already applied them.
	dropped := map[string]map[string]bool{}
	opened := map[model.ID]backend.PageMutator{}
	var judge columnAttrJudge

	var errs []error
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterPageStmt)
		if !ok {
			continue
		}
		kind := strings.ToLower(s.ContainerType)
		if kind == "" {
			kind = "page"
		}
		label := fmt.Sprintf("alter %s %s", kind, s.PageName.String())
		doc := s.PageName.String()
		excluded := (kind == "page" || kind == "snippet") && carriedExclusion(ctx, kind, s.PageName, true)

		// The stored document, for the targets and for the entity of a grid
		// the statement inserts columns into. None for a document the script
		// creates, or one that does not resolve (validateAlterTarget's
		// finding) or will not open.
		var mutator backend.PageMutator
		var unitID, containerID model.ID
		if h != nil && !alterTargetComesFromScript(sc, s) {
			if uid, cid, _, err := resolveAlterPageUnit(ctx, s, h); err == nil {
				m, seen := opened[uid]
				if !seen {
					m, _ = ctx.Backend.OpenPageForMutation(uid)
					opened[uid] = m
				}
				mutator, unitID, containerID = m, uid, cid
			}
		}
		// The stored document as describe prints it, for a pluggable REPLACE's
		// baseline — read once, and only when a REPLACE needs it, as exec does.
		var described map[string]*ast.WidgetV3
		describedOnce := false
		storedWidgets := func() map[string]*ast.WidgetV3 {
			if !describedOnce {
				describedOnce = true
				described = describedStoredWidgets(ctx, unitID, kind, s.PageName)
			}
			return described
		}

		// The references the inserted and replacing widgets carry. They need no
		// stored document, so a document the script creates is covered too.
		ws := alterOpWidgets(s)
		if refErrors := validateWidgetReferences(ctx, ws, sc); len(refErrors) > 0 {
			// An excluded document stays excluded through an ALTER, and is
			// relaxed the way CREATE relaxes it.
			if excluded {
				if err := sc.relaxExcludedWidgetRefs(kind, doc, ws, refErrors); err != nil {
					errs = append(errs, err)
				}
			} else {
				errs = append(errs, mdlerrors.NewValidationf("%s has reference errors:\n  - %s",
					label, strings.Join(refErrors, "\n  - ")))
			}
		}

		// DataGrid 2 column attributes: of a grid the statement
		// carries, and of the columns it inserts into a stored grid.
		var colErrors []string
		if len(ws) > 0 {
			if judge == nil {
				judge = newColumnAttrJudge(ctx, sc)
			}
			colErrors = datagridColumnAttrErrors(ws, "", judge)
			for _, op := range s.Operations {
				target, cols := alterColumnInsertion(op)
				if mutator == nil || len(cols) == 0 || added[doc].declares(target.Widget) {
					continue
				}
				colErrors = append(colErrors, datagridColumnAttrErrors(cols, alterColumnsEntity(mutator, target.Widget), judge)...)
			}
		}
		if len(colErrors) > 0 {
			if excluded {
				sc.warnExcluded(kind, doc, colErrors)
			} else {
				errs = append(errs, mdlerrors.NewValidationf("%s has attribute binding errors:\n  - %s",
					label, strings.Join(colErrors, "\n  - ")))
			}
		}

		// The targets, against the stored document, in the order exec
		// resolves them.
		itemSetEarlier := map[string]bool{}
		for _, op := range s.Operations {
			if set, ok := op.(*ast.SetPropertyOp); ok && set.Target.Item != nil {
				itemSetEarlier[set.Target.Widget] = true
			}
			// A SET's target is validateAlterSetProperties' finding.
			if _, isSet := op.(*ast.SetPropertyOp); mutator != nil && !isSet {
				for _, ref := range alterPageOperationTargets(op) {
					if ref.Widget != "" && added[doc].declares(ref.Widget) {
						continue
					}
					if ref.IsColumn() && grown[doc] {
						continue
					}
					if _, err := mutator.ResolveAlterTarget(alterTargetOf(ref)); err != nil {
						errs = append(errs, mdlerrors.NewValidationf("%s: %v", label, err))
					} else if !itemSetEarlier[ref.Widget] {
						// An earlier SET on the item in this statement may be what
						// makes the slot visible (`set (headerRenderMode: 'custom')`,
						// the very step the refusal recommends). This pass reads the
						// stored document, so it cannot see that; exec checks again
						// against the values after the SET.
						if err := refuseHiddenItemSlotFor(ctx, mutator, op, ref); err != nil {
							errs = append(errs, mdlerrors.NewValidationf("%s: %v", label, err))
						}
						if r, isReplace := op.(*ast.ReplaceWidgetOp); isReplace && ref.Widget == r.Target.Widget {
							err := refuseStaleContentReplace(ctx, mutator, r, h.GetModuleName(containerID), containerID, storedWidgets)
							if err != nil && !droppedFirst(err, dropped[doc]) {
								errs = append(errs, mdlerrors.NewValidationf("%s: %v", label, err))
							}
						}
					}
				}
			}
			switch o := op.(type) {
			case *ast.InsertWidgetOp, *ast.ReplaceWidgetOp:
				grown[doc] = true
			case *ast.DropWidgetOp:
				if dropped[doc] == nil {
					dropped[doc] = map[string]bool{}
				}
				for _, t := range o.Targets {
					dropped[doc][t.Widget] = true
				}
			}
		}
	}
	return errs
}

// droppedFirst reports whether err is a stale-content refusal whose widgets an
// earlier DROP removes — the remedy the refusal itself gives, which exec sees
// applied and this pass, reading the stored document, does not.
func droppedFirst(err error, dropped map[string]bool) bool {
	var stale *backend.StaleWidgetContentError
	if !errors.As(err, &stale) || len(stale.Widgets) == 0 {
		return false
	}
	for _, w := range stale.Widgets {
		if w == "" || !dropped[w] {
			return false
		}
	}
	return true
}

// alterColumnInsertion returns the grid target and the columns of an INSERT or
// REPLACE that exec routes down its DataGrid 2 column path, or no columns.
func alterColumnInsertion(op ast.AlterPageOperation) (ast.WidgetRef, []*ast.WidgetV3) {
	switch o := op.(type) {
	case *ast.InsertWidgetOp:
		if o.Target.IsColumn() && allColumns(o.Widgets) {
			return o.Target, o.Widgets
		}
	case *ast.ReplaceWidgetOp:
		if o.Target.IsColumn() && allColumns(o.NewWidgets) {
			return o.Target, o.NewWidgets
		}
	}
	return ast.WidgetRef{}, nil
}

// alterOpWidgets returns the widgets the INSERT and REPLACE operations of one
// statement carry.
func alterOpWidgets(s *ast.AlterPageStmt) []*ast.WidgetV3 {
	var ws []*ast.WidgetV3
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.InsertWidgetOp:
			ws = append(ws, o.Widgets...)
		case *ast.ReplaceWidgetOp:
			ws = append(ws, o.NewWidgets...)
		}
	}
	return ws
}

// widgetsTheScriptAddsTo names, per document, every widget an INSERT or
// REPLACE anywhere in the script puts on it (nested widgets included), and
// every name a SET renames a widget to. Not ordered, for the reason
// documentsTheScriptAddsWidgetsTo gives: a false refusal blocks a script that
// works, a missed one costs one unreported error exec still reports.
//
// The names are those exec writes: a `use building block X as p_` adds the
// block's widgets under p_, not a widget named X — read the other way, a
// later `insert after p_container2` on a stored page was refused although exec
// ran it (ako/mxcli#2182). A reference it cannot expand allows every name
// under its prefix.
func widgetsTheScriptAddsTo(ctx *ExecContext, prog *ast.Program) map[string]*knownWidgetNames {
	out := map[string]*knownWidgetNames{}
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterPageStmt)
		if !ok {
			continue
		}
		doc := s.PageName.String()
		names := out[doc]
		if names == nil {
			names = newKnownWidgetNames()
			out[doc] = names
		}
		names.add(ctx, alterOpWidgets(s))
		for _, op := range s.Operations {
			if set, ok := op.(*ast.SetPropertyOp); ok {
				names.rename(set)
			}
		}
	}
	return out
}
