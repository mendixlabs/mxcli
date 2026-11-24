// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// # A bottom bar item no user role may see (MDL-NATIVEBAR02)
//
// mxbuild gives each NativePhone bottom bar item the roles that may view its
// page (`visibleForRoles` in the generated deployment/native/index.js), and
// the native client keeps an item only when the session has one of them; with
// no item left it does not show the bottom bar at all (ako/mxcli#2565,
// measured on 11.16.1 with Native Mobile Resources 12.7.0 and native-template
// 20.0.2). check, exec and mx check all pass such a profile — the normal case
// for pages just created in a new module with no `grant view` — and the app
// opens its home page with no bar.
//
// A page is viewable by a user role when one of the page's allowed module
// roles is one of the user role's: both as the project stores them and as the
// script changes them (grant/revoke view on page, create user role, alter
// user role add/remove). A warning, not an error: the model is valid, and a
// project may grant access in a later script. Only at security Prototype or
// Production, where page access is checked.

// CheckNativeBottomBarRoles reports MDL-NATIVEBAR02 for the script.
func (e *Executor) CheckNativeBottomBarRoles(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	return CheckNativeBottomBarRoles(e.newExecContext(context.Background()), prog)
}

// CheckNativeBottomBarRoles warns about each NativePhone bottom bar item whose
// page no user role may view, or once for the bar when that is every item.
func CheckNativeBottomBarRoles(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || prog == nil || !ctx.Connected() {
		return nil
	}
	var navs []*ast.CreateNavigationStmt
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateNavigationStmt); ok && s.HasMenuBlock && isNativeProfileName(s.ProfileName) && len(s.MenuItems) > 0 {
			navs = append(navs, s)
		}
	}
	if len(navs) == 0 {
		return nil
	}
	acc := newBarAccess(ctx, prog)
	if !acc.checked {
		return nil
	}
	var out []linter.Violation
	for _, s := range navs {
		var hidden []string
		visible := 0
		for _, it := range s.MenuItems {
			page := barItemPage(it)
			if page == "" {
				continue
			}
			if acc.viewable(page) {
				visible++
			} else {
				hidden = append(hidden, fmt.Sprintf("%q (%s)", it.Caption, page))
			}
		}
		if len(hidden) == 0 {
			continue
		}
		where := "navigation " + s.ProfileName
		fix := "grant view on page <page> to <Module.Role>, with the module role in a user role " +
			"(alter user role <Role> add module roles (<Module.Role>))"
		if visible == 0 {
			out = append(out, linter.Violation{
				RuleID:   "MDL-NATIVEBAR02",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf("%s: no user role may view the page of any bottom bar item (%s), so every item is hidden and "+
					"the bottom bar is not shown; check, exec and mx check do not see it", where, strings.Join(hidden, ", ")),
				Location:   linter.Location{DocumentType: "navigation", DocumentName: s.ProfileName},
				Suggestion: fix,
			})
			continue
		}
		for _, h := range hidden {
			out = append(out, linter.Violation{
				RuleID:     "MDL-NATIVEBAR02",
				Severity:   linter.SeverityWarning,
				Message:    fmt.Sprintf("%s: no user role may view the page of bottom bar item %s, so the item is hidden for everyone", where, h),
				Location:   linter.Location{DocumentType: "navigation", DocumentName: s.ProfileName},
				Suggestion: fix,
			})
		}
	}
	return out
}

// barAccess is page view access and user roles, as the project stores them
// and the script changes them. Names are lower-cased.
type barAccess struct {
	checked   bool                       // security is Prototype or Production
	pageRoles map[string]map[string]bool // page -> module roles allowed to view it
	userRoles map[string]map[string]bool // user role -> its module roles
	ctx       *ExecContext
}

func newBarAccess(ctx *ExecContext, prog *ast.Program) *barAccess {
	a := &barAccess{pageRoles: map[string]map[string]bool{}, userRoles: map[string]map[string]bool{}, ctx: ctx}
	level := ""
	if ps, err := ctx.Backend.GetProjectSecurity(); err == nil && ps != nil {
		level = ps.SecurityLevel
		for _, ur := range ps.UserRoles {
			set := map[string]bool{}
			for _, mr := range ur.ModuleRoles {
				set[strings.ToLower(mr)] = true
			}
			a.userRoles[strings.ToLower(ur.Name)] = set
		}
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.AlterProjectSecurityStmt:
			switch strings.ToLower(s.SecurityLevel) {
			case "production":
				level = security.SecurityLevelProduction
			case "prototype":
				level = security.SecurityLevelPrototype
			case "off":
				level = security.SecurityLevelOff
			}
		case *ast.GrantPageAccessStmt:
			set := a.rolesOf(s.Page.String())
			for _, r := range s.Roles {
				set[strings.ToLower(r.String())] = true
			}
		case *ast.RevokePageAccessStmt:
			set := a.rolesOf(s.Page.String())
			for _, r := range s.Roles {
				delete(set, strings.ToLower(r.String()))
			}
		case *ast.CreateUserRoleStmt:
			key := strings.ToLower(s.Name)
			if a.userRoles[key] == nil || !s.CreateOrModify {
				a.userRoles[key] = map[string]bool{}
			}
			for _, mr := range s.ModuleRoles {
				a.userRoles[key][strings.ToLower(mr.String())] = true
			}
		case *ast.AlterUserRoleStmt:
			key := strings.ToLower(s.Name)
			if a.userRoles[key] == nil {
				a.userRoles[key] = map[string]bool{}
			}
			for _, mr := range s.ModuleRoles {
				if s.Add {
					a.userRoles[key][strings.ToLower(mr.String())] = true
				} else {
					delete(a.userRoles[key], strings.ToLower(mr.String()))
				}
			}
		}
	}
	a.checked = level == security.SecurityLevelPrototype || level == security.SecurityLevelProduction
	return a
}

// rolesOf is the module roles allowed to view page, loaded from the project on
// first use.
func (a *barAccess) rolesOf(page string) map[string]bool {
	key := strings.ToLower(page)
	if set, ok := a.pageRoles[key]; ok {
		return set
	}
	set := map[string]bool{}
	if mod, name, ok := strings.Cut(page, "."); ok {
		if h, err := getHierarchy(a.ctx); err == nil {
			if p, err := findPageByName(a.ctx, ast.QualifiedName{Module: mod, Name: name}, h); err == nil && p != nil {
				for _, r := range p.AllowedRoles {
					set[strings.ToLower(string(r))] = true
				}
			}
		}
	}
	a.pageRoles[key] = set
	return set
}

// viewable reports whether some user role holds a module role allowed to view
// page.
func (a *barAccess) viewable(page string) bool {
	for mr := range a.rolesOf(page) {
		for _, roles := range a.userRoles {
			if roles[mr] {
				return true
			}
		}
	}
	return false
}
