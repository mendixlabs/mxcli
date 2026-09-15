// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// RoleGrants records which of a module's roles each application user role
// grants: user role name → module role names, unqualified.
//
// These are the second thing an update must carry, alongside the `GUID`s. They
// live in the project's security document rather than in the module, so removing
// the module takes them with it — measured on a blank 11.12.1 app, where
// dropping Administration left Administrator with 2 module roles instead of 3
// and User with 3 instead of 4. Restoring the module does not bring them back,
// and the loss is quiet: the app builds, and users simply lose access.
type RoleGrants map[string][]string

// UserRoles returns the recorded user role names in a stable order.
func (g RoleGrants) UserRoles() []string {
	out := make([]string, 0, len(g))
	for r := range g {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// CaptureRoleGrants records every grant of one module's roles.
//
// Grants of *other* modules' roles are deliberately not recorded: an update
// touches one module, and restoring a grant this function never removed would
// be a write nobody asked for.
func CaptureRoleGrants(mprPath, moduleName string) (RoleGrants, error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Disconnect()

	sec, err := reader.GetProjectSecurity()
	if err != nil {
		return nil, fmt.Errorf("read project security: %w", err)
	}
	if sec == nil {
		return RoleGrants{}, nil
	}

	prefix := strings.ToLower(moduleName) + "."
	grants := RoleGrants{}
	for _, ur := range sec.UserRoles {
		var mine []string
		for _, mr := range ur.ModuleRoles {
			if strings.HasPrefix(strings.ToLower(mr), prefix) {
				mine = append(mine, mr[len(prefix):])
			}
		}
		if len(mine) > 0 {
			sort.Strings(mine)
			grants[ur.Name] = mine
		}
	}
	return grants, nil
}

// RestoreRoleGrants re-grants recorded module roles, and reports the ones it
// could not.
//
// A recorded role the new version no longer defines is returned in `dropped`
// rather than silently skipped: someone had that access and now cannot, which
// is a change the operator has to see. Restoring goes through MDL so it uses
// the same validated write path a user would, rather than a second
// hand-rolled security writer.
func RestoreRoleGrants(mprPath, moduleName string, grants RoleGrants, newBackend func() backend.FullBackend) (restored int, dropped []string, err error) {
	if len(grants) == 0 {
		return 0, nil, nil
	}

	available, err := moduleRoleNames(mprPath, moduleName)
	if err != nil {
		return 0, nil, err
	}

	var stmts []string
	for _, userRole := range grants.UserRoles() {
		var keep []string
		for _, role := range grants[userRole] {
			if available[strings.ToLower(role)] {
				keep = append(keep, moduleName+"."+role)
				continue
			}
			dropped = append(dropped, fmt.Sprintf("%s: %s.%s", userRole, moduleName, role))
		}
		if len(keep) == 0 {
			continue
		}
		stmts = append(stmts, fmt.Sprintf("ALTER USER ROLE %s ADD MODULE ROLES (%s);",
			userRole, strings.Join(keep, ", ")))
		restored += len(keep)
	}
	sort.Strings(dropped)

	if len(stmts) == 0 {
		return 0, dropped, nil
	}
	if err := execStatements(mprPath, strings.Join(stmts, "\n"), newBackend); err != nil {
		return 0, dropped, err
	}
	return restored, dropped, nil
}

// moduleRoleNames returns the roles the module currently defines, lowercased.
func moduleRoleNames(mprPath, moduleName string) (map[string]bool, error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Disconnect()

	mods, err := reader.ListModules()
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		if !strings.EqualFold(m.Name, moduleName) {
			continue
		}
		sec, serr := reader.GetModuleSecurity(m.ID)
		if serr != nil || sec == nil {
			return map[string]bool{}, nil //nolint:nilerr // no security document means no roles
		}
		out := make(map[string]bool, len(sec.ModuleRoles))
		for _, r := range sec.ModuleRoles {
			out[strings.ToLower(r.Name)] = true
		}
		return out, nil
	}
	return nil, fmt.Errorf("module %q not found", moduleName)
}

// execStatements runs MDL against a project through the normal executor.
func execStatements(mprPath, mdl string, newBackend func() backend.FullBackend) error {
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		return fmt.Errorf("build restore statements: %v", errs)
	}
	var sink bytes.Buffer
	ex := executor.New(&sink)
	defer ex.Close()
	if newBackend != nil {
		ex.SetBackendFactory(newBackend)
	}
	if err := ex.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		return fmt.Errorf("connect %s: %w", mprPath, err)
	}
	for _, s := range prog.Statements {
		if err := ex.Execute(s); err != nil {
			return fmt.Errorf("%w\noutput: %s", err, sink.String())
		}
	}
	return nil
}
