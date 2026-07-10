// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Entity access rules over MCP.
//
// PED does not expose the security *documents* (Security$ProjectSecurity,
// Security$ModuleSecurity report "Unknown document type"), so module roles,
// user roles, and project security settings cannot be authored over MCP. But an
// entity's access rules are NOT in the security document — they live on the
// domain model (DomainModels$Entity.accessRules), which PED already authors.
// Verified live on 11.12: the rule and its member accesses map 1:1 onto the
// executor's params (moduleRoles, attribute/association refs, and access rights
// are the same qualified names mxcli already builds).
//
// One hard PED limit shapes everything here: a DomainModels$AccessRule and a
// DomainModels$MemberAccess can be ADDED and their leaves SET, but they can
// NEVER be removed ("Element of type … cannot be removed"). So the MCP backend
// can author a new rule, but cannot replace an existing one in place (that would
// need member removal) or revoke a rule. Those paths are rejected, not faked.

// AddEntityAccessRule adds an entity access rule to the domain model via PED.
// The role(s) referenced must already exist (PED cannot create module roles).
// An existing rule for the same role set is rejected: PED cannot remove the old
// rule or its member accesses to replace it cleanly.
func (b *Backend) AddEntityAccessRule(params backend.EntityAccessRuleParams) error {
	moduleName, err := b.moduleNameForDomainModel(params.UnitID)
	if err != nil {
		return err
	}
	entIdx, err := b.entityIndex(moduleName, params.EntityName)
	if err != nil {
		return err
	}

	existing, err := b.entityAccessRuleRoleSets(moduleName, entIdx)
	if err != nil {
		return err
	}
	want := normalizeRoleSet(params.RoleNames)
	for _, rs := range existing {
		if roleSetsEqual(rs, want) {
			return fmt.Errorf("an entity access rule for role(s) %s already exists on %s.%s; "+
				"the MCP backend cannot replace an access rule in place (PED cannot remove access rules or "+
				"member accesses) — edit it in Studio Pro, or run without --mcp against a local .mpr",
				strings.Join(params.RoleNames, ", "), moduleName, params.EntityName)
		}
	}

	if err := b.ensureSchema("DomainModels$AccessRule", "DomainModels$MemberAccess"); err != nil {
		return err
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      fmt.Sprintf("/entities/%d/accessRules", entIdx),
		Operation: pedOperation{Type: "add", Value: buildAccessRuleValue(params)},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// RemoveEntityAccessRule is unsupported over MCP: PED refuses to remove a
// DomainModels$AccessRule.
func (b *Backend) RemoveEntityAccessRule(_ model.ID, entityName string, roleNames []string) (int, error) {
	return 0, fmt.Errorf("cannot revoke entity access over MCP: PED cannot remove access rules "+
		"(role(s) %s on %s) — do it in Studio Pro or run without --mcp against a local .mpr",
		strings.Join(roleNames, ", "), entityName)
}

// RevokeEntityMemberAccess is unsupported over MCP: revoking a member's access
// would require removing a DomainModels$MemberAccess, which PED refuses.
func (b *Backend) RevokeEntityMemberAccess(_ model.ID, entityName string, _ []string, _ types.EntityAccessRevocation) (int, error) {
	return 0, fmt.Errorf("cannot revoke member access on %s over MCP: PED cannot remove member accesses "+
		"— do it in Studio Pro or run without --mcp against a local .mpr", entityName)
}

// buildAccessRuleValue builds a DomainModels$AccessRule constructor for
// ped_update_document. moduleRoles, attribute and association references are the
// qualified names the executor already produces (verified to match PED's form).
func buildAccessRuleValue(params backend.EntityAccessRuleParams) map[string]any {
	rule := map[string]any{
		"$Type":                     "DomainModels$AccessRule",
		"moduleRoles":               params.RoleNames,
		"allowCreate":               params.AllowCreate,
		"allowDelete":               params.AllowDelete,
		"defaultMemberAccessRights": orDefault(params.DefaultMemberAccess, "None"),
	}
	if params.XPathConstraint != "" {
		rule["xPathConstraint"] = params.XPathConstraint
	}
	if len(params.MemberAccesses) > 0 {
		mas := make([]any, 0, len(params.MemberAccesses))
		for _, ma := range params.MemberAccesses {
			m := map[string]any{
				"$Type":        "DomainModels$MemberAccess",
				"accessRights": orDefault(ma.AccessRights, "None"),
			}
			// Send only the populated reference. PED rejects an empty string as an
			// invalid reference; the unused side defaults to empty on its own.
			if ma.AttributeRef != "" {
				m["attribute"] = ma.AttributeRef
			}
			if ma.AssociationRef != "" {
				m["association"] = ma.AssociationRef
			}
			mas = append(mas, m)
		}
		rule["memberAccesses"] = mas
	}
	return rule
}

// entityAccessRuleRoleSets reads the module's domain model and returns the
// moduleRoles set of each existing access rule on the entity at entIdx.
func (b *Backend) entityAccessRuleRoleSets(moduleName string, entIdx int) ([][]string, error) {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        []string{fmt.Sprintf("/entities/%d/accessRules", entIdx)},
	})
	if err != nil {
		return nil, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError || strings.HasPrefix(strings.TrimSpace(text), "ERROR") {
		return nil, fmt.Errorf("read access rules for %s entity #%d: %s", moduleName, entIdx, text)
	}
	byPath, err := parsePedResults(text)
	if err != nil {
		return nil, err
	}
	raw := byPath[fmt.Sprintf("/entities/%d/accessRules", entIdx)]
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var rules []struct {
		ModuleRoles []string `json:"moduleRoles"`
	}
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, fmt.Errorf("parse access rules for %s entity #%d: %w", moduleName, entIdx, err)
	}
	out := make([][]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, normalizeRoleSet(r.ModuleRoles))
	}
	return out, nil
}

// normalizeRoleSet returns a sorted copy of the role names for set comparison.
func normalizeRoleSet(roles []string) []string {
	cp := append([]string(nil), roles...)
	sort.Strings(cp)
	return cp
}

// roleSetsEqual reports whether two normalized role sets are identical.
func roleSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
