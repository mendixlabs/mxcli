// SPDX-License-Identifier: Apache-2.0

// CE1571 on a navigation menu item: "No argument has been selected for
// parameter 'X' and no default is available. Please select an argument
// manually." at Menu item.
//
// A menu item has no context object, so a flow or page it calls gets an
// argument only from the item's own argument list. validateFlowArguments checks
// the same fault on a page's widgets; nothing checked it on a menu item, so
// `check -p` passed a menu that the build rejects. Measured on a fresh Mendix
// 11.14.0 project, one `alter navigation … insert { menu item … }` per copy,
// then `mx check`:
//
//	call microflow, object parameter, no argument       → CE1571 at Menu item
//	call microflow, String parameter, no argument       → CE1571 at Menu item
//	call nanoflow, object parameter, no argument        → CE1571 at Menu item
//	show page, required object parameter, no argument   → CE1571 at Menu item
//	call microflow, no parameters                       → no error
//	call microflow(Code = 'abc'), String parameter      → no error
//	show page, optional parameter, no argument          → no error
//
// The rule reports a parameter no argument names. A page parameter that is
// optional (Mendix 11.5+) or has a default is not reported, and neither is
// anything this check cannot see: a flow or page that does not resolve (that is
// validateNavigationTargets' report), the page a `create object … then show
// page` opens (the created object fills it), or an argument's value (the menu
// builder judges that, MDL-PAGEARG01).
//
// Only what a statement states is judged. An ALTER NAVIGATION or ALTER MENU is
// checked on the items it inserts or replaces and the actions it sets, through
// menuAlterView; the stored items it does not name are not looked at.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// menuCallParams answers, for a menu item's call, the parameters an argument
// must fill. kind is "microflow", "nanoflow" or "showPage"; target is the
// qualified name as written. ok is false when the target is unknown.
type menuCallParams func(kind, target string) (required []string, primitive map[string]bool, ok bool)

// validateMenuItemArguments reports, per navigation or menu statement, the menu
// items whose call leaves a parameter without an argument (`check -p` and
// exec's pre-flight). A menu document stored as excluded is warned about
// instead: Mendix does not validate an excluded document.
func validateMenuItemArguments(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if !ctx.Connected() {
		return nil
	}
	lookup := newMenuCallLookup(ctx, sc)
	var errs []error
	for i, stmt := range prog.Statements {
		label, items, ok := menuStatementItems(stmt)
		if !ok {
			continue
		}
		found := menuItemArgErrors(items, "", lookup)
		if len(found) == 0 {
			continue
		}
		if m, isMenu := stmt.(*ast.CreateMenuStmt); isMenu && storedMenuExcluded(ctx, m.Name) {
			sc.warnExcluded("menu", m.Name.String(), found)
			continue
		}
		errs = append(errs, fmt.Errorf("statement %d: %w", i+1, mdlerrors.NewValidationf(
			"%s has argument errors:\n  - %s", label, strings.Join(found, "\n  - "))))
	}
	return errs
}

// menuStatementItems returns the menu items a statement states, with the label
// its errors carry; ok is false for a statement that states none.
func menuStatementItems(stmt ast.Statement) (label string, items []ast.NavMenuItemDef, ok bool) {
	if v := menuAlterView(stmt); v != nil {
		stmt = v
	}
	switch s := stmt.(type) {
	case *ast.CreateNavigationStmt:
		return "navigation " + s.ProfileName, s.MenuItems, true
	case *ast.CreateMenuStmt:
		return "menu " + s.Name.String(), s.Items, true
	}
	return "", nil, false
}

// menuItemArgErrors walks the items at any depth. path is the captions of the
// groups an item sits in, as menuItemTargets writes it.
func menuItemArgErrors(items []ast.NavMenuItemDef, path string, params menuCallParams) []string {
	var out []string
	for _, it := range items {
		if msg := menuItemArgError(it, path, params); msg != "" {
			out = append(out, msg)
		}
		out = append(out, menuItemArgErrors(it.Items, path+"'"+it.Caption+"' > ", params)...)
	}
	return out
}

// menuItemArgError is the one item's report, "" when its call is complete.
func menuItemArgError(it ast.NavMenuItemDef, path string, params menuCallParams) string {
	act := it.Action
	if act == nil {
		// The old clause spelling, as the menu builder reads it.
		switch {
		case it.Page != nil:
			act = &ast.ActionV3{Type: "showPage", Target: it.Page.String()}
		case it.Microflow != nil:
			act = &ast.ActionV3{Type: "microflow", Target: it.Microflow.String()}
		default:
			return ""
		}
	}
	// Only the item's own call: the page a create-object action opens gets the
	// created object.
	if act.Type != "microflow" && act.Type != "nanoflow" && act.Type != "showPage" {
		return ""
	}
	target := unquoteQualifiedName(strings.TrimSpace(act.Target))
	required, primitive, ok := params(act.Type, target)
	if !ok {
		return "" // resolution is validateNavigationTargets' job
	}
	given := make(map[string]bool, len(act.Args))
	for _, a := range act.Args {
		given[strings.ToLower(a.Name)] = true
	}
	var missing []string
	firstPrimitive := ""
	for _, p := range required {
		if given[strings.ToLower(p)] {
			continue
		}
		missing = append(missing, p)
		if firstPrimitive == "" && primitive[strings.ToLower(p)] {
			firstPrimitive = p
		}
	}
	if len(missing) == 0 {
		return ""
	}

	what := "show page " + target
	remedy := "Open it from a microflow without parameters that retrieves or creates what the page needs and shows it"
	if act.Type != "showPage" {
		what = "call " + act.Type + " " + target
		remedy = "Call a " + act.Type + " without parameters that retrieves or creates what it needs"
		if firstPrimitive != "" {
			remedy += fmt.Sprintf(", or pass a value: `call %s %s(%s = '…')`", act.Type, target, firstPrimitive)
		}
	}
	return fmt.Sprintf(
		"menu item %s'%s': %s has no argument for %s %s — Mendix rejects this with CE1571: "+
			"a menu item has no context object to supply it. %s",
		path, it.Caption, what, plural(len(missing), "parameter", "parameters"), quoteJoin(missing), remedy)
}

// menuPageParams is a page's parameters as a menu item's check needs them.
type menuPageParams struct {
	names    []string
	optional map[string]bool // lower-cased; optional or with a default
}

// newMenuCallLookup resolves menu item calls against the project plus what the
// script creates — a script-defined flow or page wins on a name clash, as for
// validateFlowArguments — loading the project's signatures on first use.
func newMenuCallLookup(ctx *ExecContext, sc *scriptContext) menuCallParams {
	var flows map[string]*flowSignature
	var pages map[string]menuPageParams
	return func(kind, target string) ([]string, map[string]bool, bool) {
		key := strings.ToLower(target)
		if kind == "showPage" {
			p, ok := menuScriptPage(sc, key)
			if !ok {
				if pages == nil {
					pages = buildMenuPageParams(ctx)
				}
				p, ok = pages[key]
			}
			if !ok {
				return nil, nil, false
			}
			p = applyMenuPageParamEdits(p, sc.pageParamEdits[key])
			var required []string
			for _, n := range p.names {
				if !p.optional[strings.ToLower(n)] {
					required = append(required, n)
				}
			}
			return required, nil, true
		}
		sig, ok := sc.flowParams[key]
		if !ok {
			if flows == nil {
				flows = buildFlowSignatures(ctx)
			}
			sig, ok = flows[key]
		}
		if !ok || sig == nil {
			return nil, nil, false
		}
		primitive := map[string]bool{}
		for _, p := range sig.Params {
			if !p.Object {
				primitive[strings.ToLower(p.Name)] = true
			}
		}
		return sig.paramNames(), primitive, true
	}
}

// menuScriptPage is a page the script creates, if it does.
func menuScriptPage(sc *scriptContext, key string) (menuPageParams, bool) {
	names, ok := sc.pageParamNames[key]
	if !ok {
		return menuPageParams{}, false
	}
	return menuPageParams{names: names, optional: sc.pageOptionalParams[key]}, true
}

// buildMenuPageParams maps each stored page (lower-cased qualified name) to its
// parameters. A parameter stored as not required, or with a default value, is
// one Mendix does not ask an argument for.
func buildMenuPageParams(ctx *ExecContext) map[string]menuPageParams {
	out := map[string]menuPageParams{}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return out
	}
	all, err := ctx.Backend.ListPages()
	if err != nil {
		return out
	}
	for _, p := range liveFirst(all, pageExcluded) {
		if p == nil {
			continue
		}
		key := strings.ToLower(h.GetQualifiedName(p.ContainerID, p.Name))
		if _, seen := out[key]; seen {
			continue // an excluded twin of a live page
		}
		pp := menuPageParams{names: []string{}, optional: map[string]bool{}}
		for _, param := range p.Parameters {
			if param == nil {
				continue
			}
			pp.names = append(pp.names, param.Name)
			if !param.IsRequired || strings.TrimSpace(param.DefaultValue) != "" {
				pp.optional[strings.ToLower(param.Name)] = true
			}
		}
		out[key] = pp
	}
	return out
}

// applyMenuPageParamEdits is applyPageParamEdits keeping which parameters are
// optional: an `alter page … add parameters $X: T optional` adds one a menu
// item may leave out.
func applyMenuPageParamEdits(p menuPageParams, edits []ast.AlterPageOperation) menuPageParams {
	if len(edits) == 0 {
		return p
	}
	optional := map[string]bool{}
	for k, v := range p.optional {
		optional[k] = v
	}
	for _, op := range edits {
		if o, ok := op.(*ast.AddParameterOp); ok {
			optional[strings.ToLower(o.Parameter.Name)] = o.Parameter.Optional
		}
	}
	return menuPageParams{names: applyPageParamEdits(p.names, edits), optional: optional}
}
