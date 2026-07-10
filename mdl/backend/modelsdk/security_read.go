// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	genSec "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/security"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
)

// ListModuleSecurity reads every module's security document (its module roles)
// through the codec engine. Mirrors the legacy reader.ListModuleSecurity.
func (b *Backend) ListModuleSecurity() ([]*security.ModuleSecurity, error) {
	units, err := mprread.ListUnitsWithContainer[*genSec.ModuleSecurity](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*security.ModuleSecurity, 0, len(units))
	for _, u := range units {
		out = append(out, moduleSecurityFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

// GetModuleSecurity returns the module-security document whose container is
// moduleID (its module roles).
func (b *Backend) GetModuleSecurity(moduleID model.ID) (*security.ModuleSecurity, error) {
	units, err := mprread.ListUnitsWithContainer[*genSec.ModuleSecurity](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if u.ContainerID == moduleID {
			return moduleSecurityFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, fmt.Errorf("module security not found for module: %s", moduleID)
}

// GetProjectSecurity reads the project's Security$ProjectSecurity document (security
// level, admin/guest roles, user roles, demo users, password policy).
func (b *Backend) GetProjectSecurity() (*security.ProjectSecurity, error) {
	g, err := mprread.GetProjectSecurity(b.reader)
	if err != nil {
		return nil, err
	}
	ps := &security.ProjectSecurity{
		SecurityLevel:      g.SecurityLevel(),
		AdminUserName:      g.AdminUserName(),
		AdminPassword:      g.AdminPassword(),
		AdminUserRole:      g.AdminUserRoleName(),
		CheckSecurity:      g.CheckSecurity(),
		StrictMode:         g.StrictMode(),
		StrictPageUrlCheck: g.StrictPageUrlCheck(),
		EnableDemoUsers:    g.EnableDemoUsers(),
		EnableGuestAccess:  g.EnableGuestAccess(),
		GuestUserRole:      g.GuestUserRoleName(),
	}
	ps.ID = model.ID(g.ID())
	for _, el := range g.UserRolesItems() {
		r, ok := el.(*genSec.UserRole)
		if !ok {
			continue
		}
		ur := &security.UserRole{
			Name:                    r.Name(),
			Description:             r.Description(),
			ModuleRoles:             r.ModuleRolesQualifiedNames(),
			ManageAllRoles:          r.ManageAllRoles(),
			ManageUsersWithoutRoles: r.ManageUsersWithoutRoles(),
			ManageableRoles:         r.ManageableRolesQualifiedNames(),
			CheckSecurity:           r.CheckSecurity(),
		}
		ur.ID = model.ID(r.ID())
		ps.UserRoles = append(ps.UserRoles, ur)
	}
	for _, el := range g.DemoUsersItems() {
		d, ok := el.(*genSec.DemoUser)
		if !ok {
			continue
		}
		du := &security.DemoUser{
			UserName:  d.UserName(),
			Password:  d.Password(),
			Entity:    d.EntityQualifiedName(),
			UserRoles: d.UserRolesQualifiedNames(),
		}
		du.ID = model.ID(d.ID())
		ps.DemoUsers = append(ps.DemoUsers, du)
	}
	if pp, ok := g.PasswordPolicySettings().(*genSec.PasswordPolicySettings); ok && pp != nil {
		ps.PasswordPolicy = &security.PasswordPolicy{
			MinimumLength:    int(pp.MinimumLength()),
			RequireDigit:     pp.RequireDigit(),
			RequireMixedCase: pp.RequireMixedCase(),
			RequireSymbol:    pp.RequireSymbol(),
		}
	}
	return ps, nil
}

// moduleSecurityFromGen converts a gen ModuleSecurity element to the semantic type.
func moduleSecurityFromGen(ms *genSec.ModuleSecurity, containerID model.ID) *security.ModuleSecurity {
	out := &security.ModuleSecurity{ContainerID: containerID}
	out.ID = model.ID(ms.ID())
	for _, el := range ms.ModuleRolesItems() {
		r, ok := el.(*genSec.ModuleRole)
		if !ok {
			continue
		}
		mr := &security.ModuleRole{Name: r.Name(), Description: r.Description()}
		mr.ID = model.ID(r.ID())
		out.ModuleRoles = append(out.ModuleRoles, mr)
	}
	return out
}
