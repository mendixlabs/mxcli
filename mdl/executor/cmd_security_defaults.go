// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
)

const (
	autoDocumentRoleName        = "User"
	autoDocumentRoleDescription = "Auto-created default role for mxcli document access"
)

// defaultDocumentAccessRoles returns a conservative fallback role set for newly
// created pages/microflows when the target module has no module roles at all.
//
// Mendix accepts document access only when it references a role from the same
// module; using an existing role from another module causes CE0148 on freshly
// created documents. To keep mx-check green, auto-create a local `User` module
// role only for modules that currently have zero roles. Modules that already
// manage their own roles keep the existing "no access by default" behavior.
func defaultDocumentAccessRoles(ctx *ExecContext, module *model.Module) []model.ID {
	if module == nil {
		return nil
	}

	ms, err := ctx.Backend.GetModuleSecurity(module.ID)
	if err != nil || ms == nil {
		return nil
	}
	if moduleUsesAutoDocumentRole(ms) {
		return []model.ID{model.ID(module.Name + "." + autoDocumentRoleName)}
	}
	if len(ms.ModuleRoles) > 0 {
		return nil
	}

	if err := ctx.Backend.AddModuleRole(ms.ID, autoDocumentRoleName, autoDocumentRoleDescription); err != nil {
		return nil
	}
	return []model.ID{model.ID(module.Name + "." + autoDocumentRoleName)}
}

func moduleUsesAutoDocumentRole(ms *security.ModuleSecurity) bool {
	if ms == nil {
		return false
	}
	return len(ms.ModuleRoles) == 1 &&
		ms.ModuleRoles[0].Name == autoDocumentRoleName &&
		ms.ModuleRoles[0].Description == autoDocumentRoleDescription
}

func remapDocumentAccessRoles(ctx *ExecContext, targetModule *model.Module, currentRoles []model.ID) []model.ID {
	if targetModule == nil {
		return nil
	}

	ms, err := ctx.Backend.GetModuleSecurity(targetModule.ID)
	if err != nil || ms == nil {
		return nil
	}
	if len(ms.ModuleRoles) == 0 || moduleUsesAutoDocumentRole(ms) {
		return defaultDocumentAccessRoles(ctx, targetModule)
	}

	targetRoleNames := make(map[string]bool, len(ms.ModuleRoles))
	for _, role := range ms.ModuleRoles {
		targetRoleNames[role.Name] = true
	}

	var remapped []model.ID
	seen := make(map[string]bool)
	for _, qualifiedRole := range currentRoles {
		roleName := string(qualifiedRole)
		if idx := strings.LastIndex(roleName, "."); idx >= 0 {
			roleName = roleName[idx+1:]
		}
		if !targetRoleNames[roleName] {
			continue
		}
		targetQualifiedRole := targetModule.Name + "." + roleName
		if seen[targetQualifiedRole] {
			continue
		}
		seen[targetQualifiedRole] = true
		remapped = append(remapped, model.ID(targetQualifiedRole))
	}

	return remapped
}

func documentRoleStrings(roles []model.ID) []string {
	values := make([]string, 0, len(roles))
	for _, role := range roles {
		values = append(values, string(role))
	}
	return values
}

func cloneRoleIDs(roles []model.ID) []model.ID {
	if len(roles) == 0 {
		return nil
	}
	cloned := make([]model.ID, len(roles))
	copy(cloned, roles)
	return cloned
}

// pruneInvalidUserRoles removes user roles that no longer have any non-System
// module role assignments. Mendix rejects those roles with CE0157.
func pruneInvalidUserRoles(ctx *ExecContext, ps *security.ProjectSecurity) error {
	if latest, err := ctx.Backend.GetProjectSecurity(); err == nil {
		ps = latest
	} else if ps == nil {
		return err
	}

	for _, userRole := range ps.UserRoles {
		hasNonSystemRole := false
		for _, moduleRole := range userRole.ModuleRoles {
			if !strings.HasPrefix(moduleRole, "System.") {
				hasNonSystemRole = true
				break
			}
		}
		if hasNonSystemRole {
			continue
		}
		if err := ctx.Backend.RemoveUserRole(ps.ID, userRole.Name); err != nil {
			return err
		}
		if !ctx.Quiet {
			fmt.Fprintf(ctx.Output, "Dropped invalid user role: %s\n", userRole.Name)
		}
	}

	return nil
}
