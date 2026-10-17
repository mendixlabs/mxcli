// SPDX-License-Identifier: Apache-2.0

// Reference validation for the targets a navigation statement names.
//
// A navigation profile points at documents in six places: its home page (and
// one per user role), a home flow, the login page, the not-found page, and
// every menu item at any depth. A standalone menu document carries the same
// menu items. Nothing resolved any of them: the names were written to the model
// as they stood, `check -p` said "Check passed!", and the first sign of a typo
// was MxBuild, one build later:
//
//	[CE1613] "The selected page 'Shop.NoSuchHome' no longer exists."
//
// Measured on a Mendix 11.12.5 app, one such error for the home page and one
// for each menu item whose page or microflow was missing.
//
// A reference-resolution pass added per reference kind leaves every kind it does
// not list silently unchecked, and this was one of them: a page's `show page`
// and `call microflow` actions and an icon name were refused, while the same
// action on a menu item was not.
//
// # Why exec refuses too, and before it writes
//
// CREATE OR REPLACE NAVIGATION rewrites the whole profile — home pages, login
// page, menu tree — so a target that does not exist replaces the stored menu
// with one the build rejects. The handler therefore runs the same check as
// `check -p` before it reads or writes anything (the menu builder stores a
// target by name and never fails on it), and `exec`'s own pre-flight refuses the
// script before any statement is written. `exec --no-check` skips the
// pre-flight, which is why the handler cannot rely on it.
//
// # Script-aware, forward references as for a flow call
//
// A page or flow the script creates BEFORE the navigation resolves. One it
// creates further down resolves too in a run that rolls back, which is what
// plain `mxcli exec` and `mxcli check` are: the target is stored by name, and a
// later failure undoes the whole run, so nothing is left dangling. That is the
// rule a call to a flow created further down already follows
// (mendixlabs/mxcli#1405, forwardFlowCalls), and it keeps a script that writes
// the navigation first working — upstream built and checked such a script clean
// (measured on 11.12.5, 0 errors). A run that cannot roll back
// (--continue-on-error, the REPL, -c) refuses it, from validateForwardDefRefs,
// because a later failure would leave the target dangling; the references are in
// eagerDefRefs for that pass. This pass resolves against the project plus what
// the script creates anywhere, so a forward name is reported once, there.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// Target kinds a navigation statement can name. They are the document kinds the
// definition-order passes use, so one list serves both.
const (
	navPage      = defPage
	navMicroflow = defMicroflow
	navNanoflow  = defNanoflow
	navEntity    = defEntity
)

// navTarget is one reference a navigation statement or menu document makes.
type navTarget struct {
	kind  string // navPage, navMicroflow, navNanoflow or navEntity
	name  string // the qualified name as written
	where string // where it sits, e.g. `menu item 'Admin' > 'Users'`
}

// navigationTargets collects every reference a navigation statement or a menu
// document names, with where it was written. label is "navigation Responsive" or
// "menu Shop.Main_Menu"; ok is false for any other statement.
//
// native reports whether a navigation profile is a native one. It matters for
// exactly one clause: a native profile's `home microflow` is stored as the
// nanoflow it is (see checkProfileClauses), so the name is looked up as one.
func navigationTargets(stmt ast.Statement, native func(profile string) bool) (label string, out []navTarget, ok bool) {
	switch s := stmt.(type) {
	case *ast.AlterNavigationStmt:
		label = "navigation " + s.ProfileName
		for _, hp := range s.HomePages {
			where, kind := "home page", navPage
			switch {
			case hp.IsNanoflow:
				where, kind = "home nanoflow", navNanoflow
			case !hp.IsPage:
				where, kind = "home microflow", navMicroflow
				if native != nil && native(s.ProfileName) {
					kind = navNanoflow
				}
			}
			if hp.ForRole != nil {
				where += " for " + hp.ForRole.Name
			}
			out = append(out, navTarget{kind: kind, name: hp.Target.String(), where: where})
		}
		if s.LoginPage != nil {
			out = append(out, navTarget{kind: navPage, name: s.LoginPage.String(), where: "login page"})
		}
		if s.NotFoundPage != nil {
			out = append(out, navTarget{kind: navPage, name: s.NotFoundPage.String(), where: "not found page"})
		}
		out = append(out, menuItemTargets(s.MenuItems, "")...)
		return label, out, true
	case *ast.CreateMenuStmt:
		return "menu " + s.Name.String(), menuItemTargets(s.Items, ""), true
	}
	return "", nil, false
}

// menuItemTargets walks a menu's items at any depth. path is the captions of the
// groups an item sits in, so a message names the item the way the author wrote
// it: `menu item 'Admin' > 'Users'`.
func menuItemTargets(items []ast.NavMenuItemDef, path string) []navTarget {
	var out []navTarget
	for _, it := range items {
		where := "menu item " + path + "'" + it.Caption + "'"
		add := func(kind, name string) {
			if name = unquoteQualifiedName(strings.TrimSpace(name)); name != "" {
				out = append(out, navTarget{kind: kind, name: name, where: where})
			}
		}
		// The item's action is the whole truth (every kind a menu item stores);
		// Page and Microflow are the old clause spelling of the same two, set only
		// for a hand-built AST or alongside Action.
		if it.Action == nil {
			if it.Page != nil {
				add(navPage, it.Page.String())
			}
			if it.Microflow != nil {
				add(navMicroflow, it.Microflow.String())
			}
		}
		for a := it.Action; a != nil; a = a.ThenAction {
			switch a.Type {
			case "showPage":
				add(navPage, a.Target)
			case "microflow":
				add(navMicroflow, a.Target)
			case "nanoflow":
				add(navNanoflow, a.Target)
			case "create":
				add(navEntity, a.Target)
			}
		}
		out = append(out, menuItemTargets(it.Items, path+"'"+it.Caption+"' > ")...)
	}
	return out
}

// navProfileIsNative reports whether the stored profile of that name is a native
// one. A project that cannot be read has no native profile as far as this goes.
func navProfileIsNative(ctx *ExecContext, profile string) bool {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil || nav == nil {
		return false
	}
	for _, p := range nav.Profiles {
		if strings.EqualFold(p.Name, profile) {
			return p.IsNative
		}
	}
	return false
}

// unresolvedNavigationTargets resolves the targets against the project and what
// the script creates (sc), and returns one line per target that resolves to
// nothing, in the order they were written.
//
// The lookups are the ones every other page, microflow, nanoflow and entity
// reference is resolved with (validateWidgetReferences), and so are the words:
// "page not found: Shop.X". sc may be empty; exec passes an empty one because by
// the time a statement runs, what the script created earlier is in the project.
//
// A listing that comes back EMPTY is a backend that could not answer, as often
// as a project with none, and reporting every reference on that basis is the
// false error validateForwardDefRefs also declines to make: that kind is skipped.
func unresolvedNavigationTargets(ctx *ExecContext, targets []navTarget, sc *scriptContext) []string {
	if len(targets) == 0 || !ctx.Connected() {
		return nil
	}
	if sc == nil {
		sc = newScriptContext()
	}
	stored := map[string]map[string]bool{}
	storedOf := func(kind string) map[string]bool {
		if m, done := stored[kind]; done {
			return m
		}
		var m map[string]bool
		switch kind {
		case navPage:
			m = buildPageQualifiedNames(ctx)
		case navMicroflow:
			m = buildMicroflowQualifiedNames(ctx)
		case navNanoflow:
			m = buildNanoflowQualifiedNames(ctx)
		case navEntity:
			m = buildEntityQualifiedNames(ctx)
		}
		stored[kind] = m
		return m
	}
	inScript := func(kind, name string) bool {
		switch kind {
		case navPage:
			return sc.pages[name]
		case navMicroflow:
			return sc.microflows[name]
		case navNanoflow:
			return sc.nanoflows[name]
		case navEntity:
			return sc.entities[name]
		}
		return false
	}

	var out []string
	for _, t := range targets {
		known := storedOf(t.kind)
		if len(known) == 0 || known[t.name] || inScript(t.kind, t.name) {
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s not found: %s", t.where, t.kind, t.name))
	}
	return out
}

// navigationTargetError is the one error a statement's unresolved targets make.
func navigationTargetError(label string, unresolved []string) error {
	return mdlerrors.NewValidationf("%s has reference errors:\n  - %s", label, strings.Join(unresolved, "\n  - "))
}

// validateNavigationTargets resolves the navigation targets of every statement
// in the program (`check -p`, and exec's pre-flight). A menu document that is
// stored as excluded keeps its dangling targets and reports them as warnings:
// Mendix does not validate an excluded document.
func validateNavigationTargets(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if !ctx.Connected() {
		return nil
	}
	native := func(profile string) bool { return navProfileIsNative(ctx, profile) }

	var errs []error
	for i, stmt := range prog.Statements {
		label, targets, ok := navigationTargets(stmt, native)
		if !ok {
			continue
		}
		unresolved := unresolvedNavigationTargets(ctx, targets, sc)
		if len(unresolved) == 0 {
			continue
		}
		if m, isMenu := stmt.(*ast.CreateMenuStmt); isMenu && storedMenuExcluded(ctx, m.Name) {
			sc.warnExcluded("menu", m.Name.String(), unresolved)
			continue
		}
		errs = append(errs, fmt.Errorf("statement %d: %w", i+1, navigationTargetError(label, unresolved)))
	}
	return errs
}

// validateNavigationTargetsForExec is the same check when the statement runs.
// The project holds everything the script created before it. In a run that
// rolls back (forwardFlowCalls) a target the script creates further down counts
// too, as a flow call's callee does (mendixlabs/mxcli#1405): it is stored by
// name, and a later failure undoes the whole run, so nothing dangles.
func validateNavigationTargetsForExec(ctx *ExecContext, stmt ast.Statement) error {
	label, targets, ok := navigationTargets(stmt, func(profile string) bool { return navProfileIsNative(ctx, profile) })
	if !ok {
		return nil
	}
	var sc *scriptContext
	if ctx.forwardFlowCalls && len(ctx.scriptTargets) > 0 {
		sc = newScriptContext()
		for key := range ctx.scriptTargets {
			switch key.kind {
			case defPage:
				sc.pages[key.name] = true
			case defMicroflow:
				sc.microflows[key.name] = true
			case defNanoflow:
				sc.nanoflows[key.name] = true
			}
		}
	}
	if unresolved := unresolvedNavigationTargets(ctx, targets, sc); len(unresolved) > 0 {
		return navigationTargetError(label, unresolved)
	}
	return nil
}

// storedMenuExcluded reports whether the project already holds the menu
// document as excluded, which exec keeps on a rewrite.
func storedMenuExcluded(ctx *ExecContext, name ast.QualifiedName) bool {
	md, err := ctx.Backend.GetMenuDocumentByQualifiedName(name.Module, name.Name)
	return err == nil && md != nil && md.Excluded
}

// navigationDefRefs are the references a navigation statement or a menu document
// looks up when it runs, for the definition-order passes. A page or flow is
// stored by name (byName), so a run that rolls back accepts one created further
// down; a create-object action's entity is resolved by the page builder and is
// not.
func navigationDefRefs(stmt ast.Statement) []defRef {
	label, targets, ok := navigationTargets(stmt, nil)
	if !ok {
		return nil
	}
	var out []defRef
	for _, t := range targets {
		if !strings.Contains(t.name, ".") {
			continue // an unqualified name cannot be placed in a module
		}
		// A create-object action's entity is resolved by the page builder when it
		// runs; a page or flow is only stored by name.
		out = append(out, defRef{name: t.name, kinds: []string{t.kind}, site: label + ": " + t.where, byName: t.kind != navEntity})
	}
	return out
}
