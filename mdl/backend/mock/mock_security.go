// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

func (m *MockBackend) GetProjectSecurity() (*security.ProjectSecurity, error) {
	if m.GetProjectSecurityFunc != nil {
		return m.GetProjectSecurityFunc()
	}
	return nil, nil
}

func (m *MockBackend) SetProjectSecurityLevel(unitID model.ID, level string) error {
	if m.SetProjectSecurityLevelFunc != nil {
		return m.SetProjectSecurityLevelFunc(unitID, level)
	}
	return nil
}

func (m *MockBackend) SetProjectDemoUsersEnabled(unitID model.ID, enabled bool) error {
	if m.SetProjectDemoUsersEnabledFunc != nil {
		return m.SetProjectDemoUsersEnabledFunc(unitID, enabled)
	}
	return nil
}

func (m *MockBackend) AddUserRole(unitID model.ID, name string, moduleRoles []string, manageAllRoles bool) error {
	if m.AddUserRoleFunc != nil {
		return m.AddUserRoleFunc(unitID, name, moduleRoles, manageAllRoles)
	}
	return nil
}

func (m *MockBackend) AlterUserRoleModuleRoles(unitID model.ID, userRoleName string, add bool, moduleRoles []string) error {
	if m.AlterUserRoleModuleRolesFunc != nil {
		return m.AlterUserRoleModuleRolesFunc(unitID, userRoleName, add, moduleRoles)
	}
	return nil
}

func (m *MockBackend) RemoveUserRole(unitID model.ID, name string) error {
	if m.RemoveUserRoleFunc != nil {
		return m.RemoveUserRoleFunc(unitID, name)
	}
	return nil
}

func (m *MockBackend) AddDemoUser(unitID model.ID, userName, password, entity string, userRoles []string) error {
	if m.AddDemoUserFunc != nil {
		return m.AddDemoUserFunc(unitID, userName, password, entity, userRoles)
	}
	return nil
}

func (m *MockBackend) RemoveDemoUser(unitID model.ID, userName string) error {
	if m.RemoveDemoUserFunc != nil {
		return m.RemoveDemoUserFunc(unitID, userName)
	}
	return nil
}

func (m *MockBackend) ListModuleSecurity() ([]*security.ModuleSecurity, error) {
	if m.ListModuleSecurityFunc != nil {
		return m.ListModuleSecurityFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetModuleSecurity(moduleID model.ID) (*security.ModuleSecurity, error) {
	if m.GetModuleSecurityFunc != nil {
		return m.GetModuleSecurityFunc(moduleID)
	}
	return nil, nil
}

func (m *MockBackend) AddModuleRole(unitID model.ID, roleName, description string) error {
	if m.AddModuleRoleFunc != nil {
		return m.AddModuleRoleFunc(unitID, roleName, description)
	}
	return nil
}

func (m *MockBackend) RemoveModuleRole(unitID model.ID, roleName string) error {
	if m.RemoveModuleRoleFunc != nil {
		return m.RemoveModuleRoleFunc(unitID, roleName)
	}
	return nil
}

func (m *MockBackend) RemoveModuleRoleFromAllUserRoles(unitID model.ID, qualifiedRole string) (int, error) {
	if m.RemoveModuleRoleFromAllUserRolesFunc != nil {
		return m.RemoveModuleRoleFromAllUserRolesFunc(unitID, qualifiedRole)
	}
	return 0, nil
}

func (m *MockBackend) UpdateAllowedRoles(unitID model.ID, roles []string) error {
	if m.UpdateAllowedRolesFunc != nil {
		return m.UpdateAllowedRolesFunc(unitID, roles)
	}
	return nil
}

func (m *MockBackend) UpdatePublishedRestServiceRoles(unitID model.ID, roles []string) error {
	if m.UpdatePublishedRestServiceRolesFunc != nil {
		return m.UpdatePublishedRestServiceRolesFunc(unitID, roles)
	}
	return nil
}

func (m *MockBackend) RemoveFromAllowedRoles(unitID model.ID, roleName string) (bool, error) {
	if m.RemoveFromAllowedRolesFunc != nil {
		return m.RemoveFromAllowedRolesFunc(unitID, roleName)
	}
	return false, nil
}

func (m *MockBackend) AddEntityAccessRule(params backend.EntityAccessRuleParams) error {
	if m.AddEntityAccessRuleFunc != nil {
		return m.AddEntityAccessRuleFunc(params)
	}
	return nil
}

func (m *MockBackend) RemoveEntityAccessRule(unitID model.ID, entityName string, roleNames []string) (int, error) {
	if m.RemoveEntityAccessRuleFunc != nil {
		return m.RemoveEntityAccessRuleFunc(unitID, entityName, roleNames)
	}
	return 0, nil
}

func (m *MockBackend) RevokeEntityMemberAccess(unitID model.ID, entityName string, roleNames []string, revocation types.EntityAccessRevocation) (int, error) {
	if m.RevokeEntityMemberAccessFunc != nil {
		return m.RevokeEntityMemberAccessFunc(unitID, entityName, roleNames, revocation)
	}
	return 0, nil
}

func (m *MockBackend) RemoveRoleFromAllEntities(unitID model.ID, roleName string) (int, error) {
	if m.RemoveRoleFromAllEntitiesFunc != nil {
		return m.RemoveRoleFromAllEntitiesFunc(unitID, roleName)
	}
	return 0, nil
}

func (m *MockBackend) ReconcileMemberAccesses(unitID model.ID, moduleName string) (int, error) {
	if m.ReconcileMemberAccessesFunc != nil {
		return m.ReconcileMemberAccessesFunc(unitID, moduleName)
	}
	return 0, nil
}
