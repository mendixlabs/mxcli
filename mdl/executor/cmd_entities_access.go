// SPDX-License-Identifier: Apache-2.0

// Package executor - Entity access control (GRANT/REVOKE output and resolution)
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// outputEntityAccessGrants outputs GRANT statements for entity access rules.
func outputEntityAccessGrants(ctx *ExecContext, entity *domainmodel.Entity, moduleName, entityName string) {
	if len(entity.AccessRules) == 0 {
		return
	}

	// Build attribute name map for resolving member accesses
	attrNames := make(map[string]string)
	for _, attr := range entity.Attributes {
		attrNames[string(attr.ID)] = attr.Name
	}

	for _, rule := range entity.AccessRules {
		// Get role names
		var roleStrs []string
		for _, rn := range rule.ModuleRoleNames {
			roleStrs = append(roleStrs, rn)
		}
		if len(roleStrs) == 0 {
			for _, rid := range rule.ModuleRoles {
				roleStrs = append(roleStrs, string(rid))
			}
		}
		if len(roleStrs) == 0 {
			continue
		}

		rightsStr := formatAccessRuleRights(ctx, rule, attrNames)
		if rightsStr == "" {
			continue
		}

		grantLine := fmt.Sprintf("\ngrant %s on %s.%s (%s)",
			strings.Join(roleStrs, ", "), moduleName, entityName, rightsStr)

		if rule.XPathConstraint != "" {
			escaped := strings.ReplaceAll(rule.XPathConstraint, "'", "''")
			grantLine += fmt.Sprintf(" where '%s'", escaped)
		}
		grantLine += ";"

		fmt.Fprintln(ctx.Output, grantLine)
	}
}

// resolveEntityMemberAccess determines per-member READ/WRITE access.
// Returns nil slices for "all members" (*), or specific member name lists.
func resolveEntityMemberAccess(_ *ExecContext, rule *domainmodel.AccessRule, attrNames map[string]string) (readMembers []string, writeMembers []string) {
	if len(rule.MemberAccesses) == 0 {
		// No per-member overrides: use default
		return nil, nil
	}

	// Check if all member accesses match the default — if so, treat as "*"
	allMatchDefault := true
	for _, ma := range rule.MemberAccesses {
		if ma.AccessRights != rule.DefaultMemberAccessRights {
			allMatchDefault = false
			break
		}
	}
	if allMatchDefault {
		return nil, nil
	}

	// Collect members by access level
	var readOnly, readWrite []string
	for _, ma := range rule.MemberAccesses {
		memberName := ma.AttributeName
		if memberName == "" {
			memberName = ma.AssociationName
		}
		if memberName == "" {
			if an, ok := attrNames[string(ma.AttributeID)]; ok {
				memberName = an
			} else {
				memberName = string(ma.AttributeID)
			}
		}

		// BSON stores BY_NAME references fully qualified ("Module.Entity.Attr" for
		// attributes, "Module.Assoc" for associations), but the grant grammar accepts
		// a bare member IDENTIFIER only. Strip to the last segment so DESCRIBE output
		// re-parses (issue #633).
		if idx := strings.LastIndex(memberName, "."); idx >= 0 {
			memberName = memberName[idx+1:]
		}

		switch ma.AccessRights {
		case domainmodel.MemberAccessRightsReadWrite:
			readWrite = append(readWrite, memberName)
		case domainmodel.MemberAccessRightsReadOnly:
			readOnly = append(readOnly, memberName)
		}
	}

	// If there are overrides, list specific members for READ and WRITE
	// READ includes both ReadOnly and ReadWrite members
	allReadable := append(readOnly, readWrite...)
	if len(allReadable) == 0 {
		readMembers = nil // all via default
	} else {
		readMembers = allReadable
	}

	if len(readWrite) == 0 {
		writeMembers = []string{} // no write members
	} else {
		writeMembers = readWrite
	}

	return readMembers, writeMembers
}

// formatAccessRuleRights formats the rights portion of an access rule as a string.
// Returns a string like "CREATE, DELETE, READ (Name, Price), WRITE (Price)" or empty if no rights.
func formatAccessRuleRights(ctx *ExecContext, rule *domainmodel.AccessRule, attrNames map[string]string) string {
	var rights []string
	if rule.AllowCreate {
		rights = append(rights, "create")
	}
	if rule.AllowDelete {
		rights = append(rights, "delete")
	}

	hasRead := rule.DefaultMemberAccessRights == domainmodel.MemberAccessRightsReadOnly ||
		rule.DefaultMemberAccessRights == domainmodel.MemberAccessRightsReadWrite
	hasWrite := rule.DefaultMemberAccessRights == domainmodel.MemberAccessRightsReadWrite
	if !hasRead || !hasWrite {
		for _, ma := range rule.MemberAccesses {
			if ma.AccessRights == domainmodel.MemberAccessRightsReadOnly ||
				ma.AccessRights == domainmodel.MemberAccessRightsReadWrite {
				hasRead = true
			}
			if ma.AccessRights == domainmodel.MemberAccessRightsReadWrite {
				hasWrite = true
			}
		}
	}

	readMembers, writeMembers := resolveEntityMemberAccess(ctx, rule, attrNames)

	if hasRead {
		if readMembers == nil {
			rights = append(rights, "read *")
		} else {
			rights = append(rights, fmt.Sprintf("read (%s)", strings.Join(quoteMembers(readMembers), ", ")))
		}
	}
	if hasWrite {
		if writeMembers == nil {
			rights = append(rights, "write *")
		} else if len(writeMembers) > 0 {
			rights = append(rights, fmt.Sprintf("write (%s)", strings.Join(quoteMembers(writeMembers), ", ")))
		}
	}

	return strings.Join(rights, ", ")
}

// quoteMembers quotes any member name that is a reserved word (or otherwise does
// not lex as a bare identifier) so the emitted READ/WRITE list re-parses. The
// grammar accepts a QUOTED_IDENTIFIER in member positions.
func quoteMembers(members []string) []string {
	out := make([]string, len(members))
	for i, m := range members {
		out[i] = mdlIdent(m)
	}
	return out
}

// formatAccessRuleResult re-reads the entity and formats the resulting access state
// for the given roles. Returns a string like "  Result: CREATE, READ (Name, Price)\n".
func formatAccessRuleResult(ctx *ExecContext, moduleName, entityName string, roleNames []string) string {
	invalidateDomainModelsCache(ctx)

	module, err := findModule(ctx, moduleName)
	if err != nil {
		return ""
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return ""
	}

	entity := dm.FindEntityByName(entityName)
	if entity == nil {
		return ""
	}

	attrNames := make(map[string]string)
	for _, attr := range entity.Attributes {
		attrNames[string(attr.ID)] = attr.Name
	}

	// Build role set for matching
	roleSet := make(map[string]bool)
	for _, rn := range roleNames {
		roleSet[rn] = true
	}

	for _, rule := range entity.AccessRules {
		// Check if this rule matches the given roles
		matchCount := 0
		for _, rn := range rule.ModuleRoleNames {
			if roleSet[rn] {
				matchCount++
			}
		}
		if matchCount == 0 {
			continue
		}
		// Found a matching rule
		rightsStr := formatAccessRuleRights(ctx, rule, attrNames)
		if rightsStr == "" {
			return "  Result: (no access)\n"
		}
		return fmt.Sprintf("  Result: %s\n", rightsStr)
	}

	return "  Result: (no access)\n"
}

// --- Executor method wrappers for callers not yet migrated ---
