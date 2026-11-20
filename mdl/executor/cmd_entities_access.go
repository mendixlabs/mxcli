// SPDX-License-Identifier: Apache-2.0

// Package executor - Entity access control (GRANT/REVOKE output and resolution)
package executor

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
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

		fmt.Fprintln(ctx.Output, "\n"+entityGrantMDL(ctx, rightsStr, moduleName+"."+entityName, roleStrs, rule.XPathConstraint))
	}
}

// entityGrantMDL is one access rule as the canonical grant (R5, ako/mxcli#753):
// rights first, the roles after `to`, and the XPath constraint in [ ] as
// stored, so no quote inside it is doubled; under mdl 0 a backslash in a
// string is (describeXPath, ako/mxcli#825).
//
// A long constraint is stored broken across lines so it can be read in Studio
// Pro's editor (upstream #979); MDL keeps that layout on one line, and the
// executor re-derives it on write. A layout its author gave it is written as
// stored, which is what re-executing the grant stores (mendixlabs/mxcli#1542).
//
// A stored constraint the bracketed grammar does not read (one Studio Pro
// stored without brackets, or with syntax the XPath rules do not cover) is
// written in the deprecated quoted form instead, which carries any string:
// describe must stay re-executable over whatever a project holds, and a
// deprecation warning is better than output that does not parse.
func entityGrantMDL(ctx *ExecContext, rights, entity string, roles []string, xpath string) string {
	x := visitor.DescribeXPathConstraintLayout(xpath)
	if x == "" || visitor.IsBracketedXPath(x) {
		line := fmt.Sprintf("grant %s on entity %s to %s", rights, entity, strings.Join(roles, ", "))
		if x != "" {
			line += " where " + describeXPath(ctx, x)
		}
		return line + ";"
	}
	return fmt.Sprintf("grant %s on %s (%s) where %s;",
		strings.Join(roles, ", "), entity, rights, mdlQuote(ctx, x))
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

// With anyXPath false (GRANT) the rule is the one the backend upserted: exactly
// roleNames as a set, and the same xpath — Mendix allows one rule per constraint,
// and several rules may name one role. Matching on any overlap reported a shared
// rule (FabUser, Coordinator, Engineer) after a GRANT that wrote a separate rule
// for Coordinator alone. anyXPath takes the first rule naming any of the roles,
// whatever its constraint, which is what REVOKE wants since it narrows every rule
// the roles appear in.
//
// formatAccessRuleResult re-reads the entity and formats the resulting access state
// for the given roles. Returns a string like "  Result: CREATE, READ (Name, Price)\n".
func formatAccessRuleResult(ctx *ExecContext, moduleName, entityName string, roleNames []string, xpath string, anyXPath bool) string {
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
		// A role may hold one rule per XPath constraint (#936), and the backend
		// keys the rule by role set plus constraint — echoing any other match
		// would report a different rule's rights back to the user.
		if !anyXPath && (!types.SameXPathConstraint(rule.XPathConstraint, xpath) || !sameRoleSet(rule.ModuleRoleNames, roleNames)) {
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

// sameRoleSet reports whether a and b hold the same role names, order-insensitive.
// It is the comparison AddEntityAccessRule upserts by (sameStringSet in
// mdl/backend/modelsdk), so the GRANT echo finds the rule that was written.
func sameRoleSet(a, b []string) bool {
	ac := slices.Clone(a)
	bc := slices.Clone(b)
	slices.Sort(ac)
	slices.Sort(bc)
	return slices.Equal(ac, bc)
}

// --- Executor method wrappers for callers not yet migrated ---
