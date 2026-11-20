// SPDX-License-Identifier: Apache-2.0

// CE1571 on a menu item (ako/mxcli#2098, mendixlabs/mxcli#1799).
//
// A navigation profile's or menu document's item that calls a microflow or
// nanoflow with a parameter, or shows a page with a required parameter, passed
// `check -p` and exec, and mx check then reported
//
//	[CE1571] "No argument has been selected for parameter 'Thing' and no
//	default is available. Please select an argument manually." at Menu item
//
// validateFlowArguments judges the same fault for a widget's data source and
// actions, but walks widgets only; a menu item is not one. The rule here is the
// same argDiff, judged in an EMPTY data context: a menu has no context object
// to fill anything in.
//
// Measured on mxbuild 11.14.0, one project per variant, one `mx check` each:
//
//	navigation item, call microflow M.F($Thing: M.Thing)        → CE1571
//	navigation item in a group, call nanoflow N($Text: String)  → CE1571
//	navigation item, show page P($Thing) required               → CE1571
//	navigation item, show page P($Text: String) required        → CE1571
//	menu document item, call microflow M.F($Thing)              → CE1571
//	alter navigation insert, call microflow M.F($Text: String)  → CE1571
//	(6 errors, one per item)
//
//	parameterless microflow / page                              → 0 errors
//	show page P($Thing optional), Mendix 11.5+                  → 0 errors
//	call microflow M.F(Text = 'hello') — a literal argument      → 0 errors
//
// So an optional page parameter is not reported, and an argument the item
// states fills its parameter whatever the expression is.
//
// A `create object E then show page P` item hands the new object to the page,
// so the chain is not judged. A target that does not resolve is the not-found
// check's to report (validateNavigationTargets); here it stays silent rather
// than guess at a signature.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// menuItemCall is one menu item's flow call or show page, as written.
type menuItemCall struct {
	where  string // `menu item 'Group' > 'Item'`
	kind   string // "microflow", "nanoflow" or "page"
	target string // qualified name as written
	args   []ast.FlowArgV3
}

// menuItemCalls walks a menu's items at any depth, with menuItemTargets'
// wording for where each sits.
func menuItemCalls(items []ast.NavMenuItemDef, path string) []menuItemCall {
	var out []menuItemCall
	for _, it := range items {
		where := "menu item " + path + "'" + it.Caption + "'"
		a := it.Action
		if a == nil {
			// The old clause spelling, or a hand-built AST.
			switch {
			case it.Page != nil:
				a = &ast.ActionV3{Type: "showPage", Target: it.Page.String()}
			case it.Microflow != nil:
				a = &ast.ActionV3{Type: "microflow", Target: it.Microflow.String()}
			}
		}
		// Only the item's own action: a chained show page follows a create
		// object, which passes the new object to it.
		if a != nil {
			name := unquoteQualifiedName(strings.TrimSpace(a.Target))
			switch a.Type {
			case "microflow", "nanoflow":
				out = append(out, menuItemCall{where: where, kind: a.Type, target: name, args: a.Args})
			case "showPage":
				out = append(out, menuItemCall{where: where, kind: "page", target: name, args: a.Args})
			}
		}
		out = append(out, menuItemCalls(it.Items, path+"'"+it.Caption+"' > ")...)
	}
	return out
}

// menuStatementCalls returns the label and calls of a navigation, menu document
// or an alter of either; ok is false for any other statement.
func menuStatementCalls(stmt ast.Statement) (label string, calls []menuItemCall, ok bool) {
	if v := menuAlterView(stmt); v != nil {
		stmt = v
	}
	switch s := stmt.(type) {
	case *ast.CreateNavigationStmt:
		return "navigation " + s.ProfileName, menuItemCalls(s.MenuItems, ""), true
	case *ast.CreateMenuStmt:
		return "menu " + s.Name.String(), menuItemCalls(s.Items, ""), true
	}
	return "", nil, false
}

// validateMenuItemArguments reports every menu item whose flow or page has a
// parameter nothing fills — CE1571, since a menu has no context object.
func validateMenuItemArguments(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	var flows map[string]*flowSignature
	var pageSigs map[string]*flowSignature
	var errs []error
	for i, stmt := range prog.Statements {
		label, calls, ok := menuStatementCalls(stmt)
		if !ok || len(calls) == 0 {
			continue
		}
		if flows == nil {
			flows = buildFlowSignatures(ctx)
			if sc != nil {
				for name, sig := range sc.flowParams {
					flows[name] = sig
				}
			}
			pageSigs = menuPageSignatures(ctx, sc)
		}
		var msgs []string
		for _, c := range calls {
			sig := flows[strings.ToLower(c.target)]
			if c.kind == "page" {
				sig = pageSigs[strings.ToLower(c.target)]
			}
			if sig == nil {
				continue // unresolved: the not-found check reports it
			}
			missing, _, _ := argDiff(c.args, sig, dataContext{}, nil)
			if len(missing) == 0 {
				continue
			}
			msgs = append(msgs, menuItemArgMessage(c, missing, sig))
		}
		if len(msgs) == 0 {
			continue
		}
		if m, isMenu := stmt.(*ast.CreateMenuStmt); isMenu && storedMenuExcluded(ctx, m.Name) {
			sc.warnExcluded("menu", m.Name.String(), msgs)
			continue
		}
		errs = append(errs, fmt.Errorf("statement %d: %w", i+1,
			mdlerrors.NewValidationf("%s has argument errors:\n  - %s", label, strings.Join(msgs, "\n  - "))))
	}
	return errs
}

// menuItemArgMessage words one item's report, with a remedy that was measured
// to build: a literal argument for a primitive flow parameter, an optional page
// parameter (11.5+), or a parameterless microflow that finds the object.
func menuItemArgMessage(c menuItemCall, missing []string, sig *flowSignature) string {
	msg := fmt.Sprintf("%s: %s %s has no argument for %s %s, and a menu item has no context object to fill it in — "+
		"Mendix rejects this with CE1571",
		c.where, c.kind, c.target, plural(len(missing), "parameter", "parameters"), quoteJoin(missing))
	if c.kind == "page" {
		return msg + fmt.Sprintf(". Open the page from a parameterless microflow that supplies the argument, "+
			"or declare the parameter optional (`$%s: … optional`, Mendix 11.5+)", missing[0])
	}
	object := false
	for _, p := range sig.Params {
		if strings.EqualFold(p.Name, missing[0]) {
			object = p.Object
		}
	}
	if object {
		return msg + ". Call a parameterless microflow that retrieves the object and calls this one"
	}
	return msg + fmt.Sprintf(". Pass a value: `call %s %s(%s = <expression>)`", c.kind, c.target, missing[0])
}

// menuPageSignatures maps each page (lower-cased qualified name) to its REQUIRED
// parameters as a signature argDiff can judge: the stored pages, overlaid with
// what the script declares and with its alter page add/drop parameter edits.
func menuPageSignatures(ctx *ExecContext, sc *scriptContext) map[string]*flowSignature {
	type param struct {
		name     string
		required bool
	}
	decls := map[string][]param{}
	if h, err := getHierarchy(ctx); err == nil && h != nil {
		if all, err := projectPages(ctx); err == nil {
			for _, p := range liveFirst(all, pageExcluded) {
				if p == nil {
					continue
				}
				key := strings.ToLower(h.GetQualifiedName(p.ContainerID, p.Name))
				if _, seen := decls[key]; seen {
					continue
				}
				ps := []param{}
				for _, pp := range p.Parameters {
					if pp != nil {
						ps = append(ps, param{pp.Name, pp.IsRequired})
					}
				}
				decls[key] = ps
			}
		}
	}
	if sc != nil {
		for key, params := range sc.pageParamDecls {
			ps := []param{}
			for _, p := range params {
				ps = append(ps, param{p.Name, !p.Optional})
			}
			decls[key] = ps
		}
		for key, edits := range sc.pageParamEdits {
			ps, ok := decls[key]
			if !ok {
				continue
			}
			for _, op := range edits {
				switch o := op.(type) {
				case *ast.AddParameterOp:
					ps = append(ps, param{o.Parameter.Name, !o.Parameter.Optional})
				case *ast.DropParameterOp:
					kept := ps[:0:0]
					for _, p := range ps {
						if !strings.EqualFold(p.name, o.ParameterName) {
							kept = append(kept, p)
						}
					}
					ps = kept
				}
			}
			decls[key] = ps
		}
	}
	out := make(map[string]*flowSignature, len(decls))
	for key, ps := range decls {
		sig := &flowSignature{}
		for _, p := range ps {
			if p.required {
				sig.Params = append(sig.Params, flowParam{Name: p.name})
			}
		}
		out[key] = sig
	}
	return out
}
