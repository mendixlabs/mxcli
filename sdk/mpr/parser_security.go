// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
	"go.mongodb.org/mongo-driver/bson"
)

// parseProjectSecurity parses a Security$ProjectSecurity BSON document.
func (r *Reader) parseProjectSecurity(unitID, containerID string, contents []byte) (*security.ProjectSecurity, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	ps := &security.ProjectSecurity{}
	ps.ID = model.ID(unitID)
	ps.TypeName = "Security$ProjectSecurity"

	ps.SecurityLevel = extractString(raw["SecurityLevel"])
	ps.AdminUserName = extractString(raw["AdminUserName"])
	ps.AdminPassword = extractString(raw["AdminPassword"])
	ps.AdminUserRole = extractString(raw["AdminUserRole"])
	ps.CheckSecurity = extractBool(raw["CheckSecurity"], false)
	ps.StrictMode = extractBool(raw["StrictMode"], false)
	ps.StrictPageUrlCheck = extractBool(raw["StrictPageUrlCheck"], false)
	ps.EnableDemoUsers = extractBool(raw["EnableDemoUsers"], false)
	ps.EnableGuestAccess = extractBool(raw["EnableGuestAccess"], false)
	ps.GuestUserRole = extractString(raw["GuestUserRole"])

	// Parse user roles
	userRoles := extractBsonArray(raw["UserRoles"])
	for _, ur := range userRoles {
		urMap := toMap(ur)
		if urMap == nil {
			continue
		}
		role := parseUserRole(urMap)
		ps.UserRoles = append(ps.UserRoles, role)
	}

	// Parse demo users
	demoUsers := extractBsonArray(raw["DemoUsers"])
	for _, du := range demoUsers {
		duMap := toMap(du)
		if duMap == nil {
			continue
		}
		user := parseDemoUser(duMap)
		ps.DemoUsers = append(ps.DemoUsers, user)
	}

	// Parse password policy
	if ppRaw, ok := raw["PasswordPolicySettings"]; ok {
		ppMap := toMap(ppRaw)
		if ppMap != nil {
			ps.PasswordPolicy = parsePasswordPolicy(ppMap)
		}
	}

	return ps, nil
}

// parseUserRole parses a Security$UserRole from a BSON map.
func parseUserRole(raw map[string]any) *security.UserRole {
	role := &security.UserRole{}
	role.ID = model.ID(extractBsonID(raw["$ID"]))
	role.TypeName = "Security$UserRole"
	role.Name = extractString(raw["Name"])
	role.Description = extractString(raw["Description"])
	role.ManageAllRoles = extractBool(raw["ManageAllRoles"], false)
	role.ManageUsersWithoutRoles = extractBool(raw["ManageUsersWithoutRoles"], false)
	role.CheckSecurity = extractBool(raw["CheckSecurity"], false)

	// Module roles are BY_NAME references (qualified name strings)
	moduleRoles := extractBsonArray(raw["ModuleRoles"])
	for _, mr := range moduleRoles {
		if name, ok := mr.(string); ok {
			role.ModuleRoles = append(role.ModuleRoles, name)
		}
	}

	// Manageable roles are BY_NAME references
	manageableRoles := extractBsonArray(raw["ManageableRoles"])
	for _, mr := range manageableRoles {
		if name, ok := mr.(string); ok {
			role.ManageableRoles = append(role.ManageableRoles, name)
		}
	}

	return role
}

// parseDemoUser parses a Security$DemoUserImpl from a BSON map.
func parseDemoUser(raw map[string]any) *security.DemoUser {
	user := &security.DemoUser{}
	user.ID = model.ID(extractBsonID(raw["$ID"]))
	user.TypeName = "Security$DemoUserImpl"
	user.UserName = extractString(raw["UserName"])
	user.Password = extractString(raw["Password"])
	user.Entity = extractString(raw["Entity"])

	// User roles are BY_NAME references
	userRoles := extractBsonArray(raw["UserRoles"])
	for _, ur := range userRoles {
		if name, ok := ur.(string); ok {
			user.UserRoles = append(user.UserRoles, name)
		}
	}

	return user
}

// parsePasswordPolicy parses Security$PasswordPolicySettings from a BSON map.
func parsePasswordPolicy(raw map[string]any) *security.PasswordPolicy {
	pp := &security.PasswordPolicy{}
	pp.ID = model.ID(extractBsonID(raw["$ID"]))
	pp.TypeName = "Security$PasswordPolicySettings"
	pp.MinimumLength = extractInt(raw["MinimumLength"])
	pp.RequireDigit = extractBool(raw["RequireDigit"], false)
	pp.RequireMixedCase = extractBool(raw["RequireMixedCase"], false)
	pp.RequireSymbol = extractBool(raw["RequireSymbol"], false)
	return pp
}

// parseModuleSecurity parses a Security$ModuleSecurity BSON document.
func (r *Reader) parseModuleSecurity(unitID, containerID string, contents []byte) (*security.ModuleSecurity, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	ms := &security.ModuleSecurity{}
	ms.ID = model.ID(unitID)
	ms.TypeName = "Security$ModuleSecurity"
	ms.ContainerID = model.ID(containerID)

	// Parse module roles
	roles := extractBsonArray(raw["ModuleRoles"])
	for _, r := range roles {
		rMap := toMap(r)
		if rMap == nil {
			continue
		}
		role := &security.ModuleRole{}
		role.ID = model.ID(extractBsonID(rMap["$ID"]))
		role.TypeName = "Security$ModuleRole"
		role.Name = extractString(rMap["Name"])
		role.Description = extractString(rMap["Description"])
		ms.ModuleRoles = append(ms.ModuleRoles, role)
	}

	return ms, nil
}

// toMap is defined in parser_javaactions.go
