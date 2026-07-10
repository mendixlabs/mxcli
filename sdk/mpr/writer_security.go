// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetRawUnitBytes reads the raw BSON bytes for a unit by ID.
// This returns unprocessed bytes suitable for raw BSON patching.
func (r *Reader) GetRawUnitBytes(id model.ID) ([]byte, error) {
	var contents []byte
	var err error

	if r.version == MPRVersionV2 {
		contents, err = r.readMprContents(string(id))
		if err != nil {
			return nil, fmt.Errorf("failed to read unit contents: %w", err)
		}
	} else {
		unitIDBlob := uuidToBlob(string(id))
		row := r.db.QueryRow("SELECT Contents FROM Unit WHERE UnitID = ?", unitIDBlob)
		err = row.Scan(&contents)
		if err != nil {
			return nil, fmt.Errorf("failed to read unit from database: %w", err)
		}
	}

	contents, err = r.resolveContents(string(id), contents)
	if err != nil {
		return nil, err
	}

	return contents, nil
}

// readPatchWrite is the core helper: reads raw BSON, applies a patch function, writes back.
func (w *Writer) readPatchWrite(unitID model.ID, patchFn func(doc bson.D) (bson.D, error)) error {
	rawBytes, err := w.reader.GetRawUnitBytes(unitID)
	if err != nil {
		return fmt.Errorf("failed to read unit %s: %w", unitID, err)
	}

	var doc bson.D
	if err := bson.Unmarshal(rawBytes, &doc); err != nil {
		return fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	doc, err = patchFn(doc)
	if err != nil {
		return err
	}

	newBytes, err := marshalUnitIDFirst(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal BSON: %w", err)
	}

	return w.updateUnit(string(unitID), newBytes)
}

// setBsonField sets a top-level field in a bson.D, adding it if not found.
func setBsonField(doc bson.D, key string, value any) bson.D {
	for i, elem := range doc {
		if elem.Key == key {
			doc[i].Value = value
			return doc
		}
	}
	return append(doc, bson.E{Key: key, Value: value})
}

// getBsonArray returns the Mendix-style array for a field (skipping the int32 marker).
func getBsonArray(doc bson.D, key string) bson.A {
	for _, elem := range doc {
		if elem.Key == key {
			if arr, ok := elem.Value.(bson.A); ok {
				return arr
			}
		}
	}
	return nil
}

// makeMendixArray builds a Mendix-style array: int32(1) marker followed by items.
func makeMendixArray(items ...any) bson.A {
	arr := bson.A{int32(1)}
	arr = append(arr, items...)
	return arr
}

// makeMendixStringArray builds a Mendix-style array of strings.
func makeMendixStringArray(items []string) bson.A {
	arr := bson.A{int32(1)}
	for _, s := range items {
		arr = append(arr, s)
	}
	return arr
}

// allowedModuleRolesArray builds a Mendix-style AllowedModuleRoles BSON array
// from a slice of model.IDs. Returns the empty array marker if no roles are set.
func allowedModuleRolesArray(roles []model.ID) bson.A {
	arr := bson.A{int32(1)}
	for _, r := range roles {
		arr = append(arr, string(r))
	}
	return arr
}

// ============================================================================
// Microflow/Page: AllowedModuleRoles
// ============================================================================

// UpdateAllowedRoles patches the AllowedModuleRoles BSON field on a unit (microflow or page).
// roles should be qualified name strings like "Module.RoleName".
func (w *Writer) UpdateAllowedRoles(unitID model.ID, roles []string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		return setBsonField(doc, "AllowedModuleRoles", makeMendixStringArray(roles)), nil
	})
}

// UpdatePublishedRestServiceRoles patches the AllowedRoles BSON field on a
// Rest$PublishedRestService unit. Note: REST uses "AllowedRoles" while
// microflows/pages/OData use "AllowedModuleRoles".
func (w *Writer) UpdatePublishedRestServiceRoles(unitID model.ID, roles []string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		return setBsonField(doc, "AllowedRoles", makeMendixStringArray(roles)), nil
	})
}

// RemoveFromAllowedRoles removes a role from the AllowedModuleRoles BSON field on a unit.
// Returns true if the role was found and removed.
func (w *Writer) RemoveFromAllowedRoles(unitID model.ID, roleName string) (bool, error) {
	removed := false
	err := w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		for _, f := range doc {
			if f.Key != "AllowedModuleRoles" {
				continue
			}
			arr, ok := f.Value.(bson.A)
			if !ok {
				return doc, nil
			}
			var remaining bson.A
			for _, item := range arr {
				if s, ok := item.(string); ok && s == roleName {
					removed = true
					continue
				}
				remaining = append(remaining, item)
			}
			if removed {
				return setBsonField(doc, "AllowedModuleRoles", remaining), nil
			}
			return doc, nil
		}
		return doc, nil
	})
	return removed, err
}

// ============================================================================
// Module Roles: CREATE/DROP on Security$ModuleSecurity
// ============================================================================

// AddModuleRole adds a new module role to the module's Security$ModuleSecurity unit.
// If a role with the same name (case-insensitive) already exists, the existing role's
// Name is overwritten with the caller-supplied casing and Description is updated.
// Mendix Studio Pro rejects case-insensitive duplicate role names with CE0123, so
// merging into the existing entry matches runtime semantics — and preserves the
// caller's casing for downstream case-sensitive lookups (e.g., GRANT ACCESS TO x.user).
func (w *Writer) AddModuleRole(unitID model.ID, roleName, description string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		// Get existing ModuleRoles array
		existing := getBsonArray(doc, "ModuleRoles")
		if existing == nil {
			existing = bson.A{int32(1)}
		}

		// If a case-insensitive duplicate already exists, overwrite its Name and
		// Description with the caller's values. This keeps the ID stable (any
		// references to it remain valid) while adopting the newly-requested casing.
		for i, item := range existing {
			role, ok := item.(bson.D)
			if !ok {
				continue
			}
			matched := false
			for _, field := range role {
				if field.Key == "Name" {
					if name, ok := field.Value.(string); ok && strings.EqualFold(name, roleName) {
						matched = true
					}
					break
				}
			}
			if !matched {
				continue
			}
			for j, field := range role {
				switch field.Key {
				case "Name":
					role[j].Value = roleName
				case "Description":
					if description != "" {
						role[j].Value = description
					}
				}
			}
			existing[i] = role
			return setBsonField(doc, "ModuleRoles", existing), nil
		}

		// Build the new role BSON document
		newRole := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Security$ModuleRole"},
			{Key: "Name", Value: roleName},
			{Key: "Description", Value: description},
		}

		existing = append(existing, newRole)
		return setBsonField(doc, "ModuleRoles", existing), nil
	})
}

// RemoveModuleRole removes a module role by name from the module's Security$ModuleSecurity unit.
func (w *Writer) RemoveModuleRole(unitID model.ID, roleName string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		existing := getBsonArray(doc, "ModuleRoles")
		if existing == nil {
			return doc, nil
		}

		var filtered bson.A
		for _, item := range existing {
			// Keep the int32 marker
			if _, ok := item.(int32); ok {
				filtered = append(filtered, item)
				continue
			}
			// Check if this role matches the name
			if roleDoc, ok := item.(bson.D); ok {
				name := ""
				for _, f := range roleDoc {
					if f.Key == "Name" {
						name = bsonutil.String(f.Value, "Name")
						break
					}
				}
				if name == roleName {
					continue // Skip this role (remove it)
				}
			}
			filtered = append(filtered, item)
		}

		return setBsonField(doc, "ModuleRoles", filtered), nil
	})
}

// ============================================================================
// Project Security: ALTER, User Roles, Demo Users
// ============================================================================

// SetProjectSecurityLevel patches the SecurityLevel field on Security$ProjectSecurity.
func (w *Writer) SetProjectSecurityLevel(unitID model.ID, level string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		return setBsonField(doc, "SecurityLevel", level), nil
	})
}

// SetProjectDemoUsersEnabled patches the EnableDemoUsers field on Security$ProjectSecurity.
func (w *Writer) SetProjectDemoUsersEnabled(unitID model.ID, enabled bool) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		return setBsonField(doc, "EnableDemoUsers", enabled), nil
	})
}

// AddUserRole adds a new user role to Security$ProjectSecurity.
func (w *Writer) AddUserRole(unitID model.ID, name string, moduleRoles []string, manageAllRoles bool) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		newRole := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Security$UserRole"},
			{Key: "Name", Value: name},
			{Key: "Description", Value: ""},
			{Key: "ModuleRoles", Value: makeMendixStringArray(moduleRoles)},
			{Key: "ManageAllRoles", Value: manageAllRoles},
			{Key: "ManageUsersWithoutRoles", Value: false},
			{Key: "ManageableRoles", Value: makeMendixArray()},
			{Key: "CheckSecurity", Value: false},
		}

		existing := getBsonArray(doc, "UserRoles")
		if existing == nil {
			existing = bson.A{int32(1)}
		}
		existing = append(existing, newRole)
		return setBsonField(doc, "UserRoles", existing), nil
	})
}

// AlterUserRoleModuleRoles adds or removes module roles from a user role in Security$ProjectSecurity.
func (w *Writer) AlterUserRoleModuleRoles(unitID model.ID, userRoleName string, add bool, moduleRoles []string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		existing := getBsonArray(doc, "UserRoles")
		if existing == nil {
			return doc, fmt.Errorf("no UserRoles array found")
		}

		found := false
		for i, item := range existing {
			roleDoc, ok := item.(bson.D)
			if !ok {
				continue
			}
			name := ""
			for _, f := range roleDoc {
				if f.Key == "Name" {
					name = bsonutil.String(f.Value, "Name")
					break
				}
			}
			if name != userRoleName {
				continue
			}
			found = true

			// Get current module roles
			var currentRoles []string
			for _, f := range roleDoc {
				if f.Key == "ModuleRoles" {
					if arr, ok := f.Value.(bson.A); ok {
						for _, r := range arr {
							if s, ok := r.(string); ok {
								currentRoles = append(currentRoles, s)
							}
						}
					}
					break
				}
			}

			if add {
				// Add new roles, skip duplicates
				existingSet := make(map[string]bool)
				for _, r := range currentRoles {
					existingSet[r] = true
				}
				for _, r := range moduleRoles {
					if !existingSet[r] {
						currentRoles = append(currentRoles, r)
					}
				}
			} else {
				// Remove specified roles
				removeSet := make(map[string]bool)
				for _, r := range moduleRoles {
					removeSet[r] = true
				}
				var filtered []string
				for _, r := range currentRoles {
					if !removeSet[r] {
						filtered = append(filtered, r)
					}
				}
				currentRoles = filtered
			}

			// Update the ModuleRoles field in the role document
			for j, f := range roleDoc {
				if f.Key == "ModuleRoles" {
					roleDoc[j].Value = makeMendixStringArray(currentRoles)
					break
				}
			}
			existing[i] = roleDoc
			break
		}

		if !found {
			return doc, fmt.Errorf("user role not found: %s", userRoleName)
		}

		return setBsonField(doc, "UserRoles", existing), nil
	})
}

// RemoveModuleRoleFromAllUserRoles removes a qualified module role (e.g., "Module.RoleName")
// from every user role's ModuleRoles list in Security$ProjectSecurity.
// Returns the number of user roles that were modified.
func (w *Writer) RemoveModuleRoleFromAllUserRoles(unitID model.ID, qualifiedRole string) (int, error) {
	modified := 0
	err := w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		existing := getBsonArray(doc, "UserRoles")
		if existing == nil {
			return doc, nil
		}

		for i, item := range existing {
			roleDoc, ok := item.(bson.D)
			if !ok {
				continue
			}

			// Find and filter ModuleRoles
			for j, f := range roleDoc {
				if f.Key != "ModuleRoles" {
					continue
				}
				arr, ok := f.Value.(bson.A)
				if !ok {
					break
				}
				var filtered bson.A
				found := false
				for _, r := range arr {
					if s, ok := r.(string); ok && s == qualifiedRole {
						found = true
						continue // Remove this role
					}
					filtered = append(filtered, r)
				}
				if found {
					if len(filtered) == 0 {
						roleDoc[j].Value = bson.A{int32(1)} // Empty Mendix array
					} else {
						roleDoc[j].Value = makeMendixStringArray(bsonAToStrings(filtered))
					}
					existing[i] = roleDoc
					modified++
				}
				break
			}
		}

		return setBsonField(doc, "UserRoles", existing), nil
	})
	return modified, err
}

// bsonAToStrings converts a bson.A of strings to []string.
func bsonAToStrings(a bson.A) []string {
	var result []string
	for _, v := range a {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// RemoveUserRole removes a user role by name from Security$ProjectSecurity.
func (w *Writer) RemoveUserRole(unitID model.ID, name string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		existing := getBsonArray(doc, "UserRoles")
		if existing == nil {
			return doc, nil
		}

		var filtered bson.A
		for _, item := range existing {
			if _, ok := item.(int32); ok {
				filtered = append(filtered, item)
				continue
			}
			if roleDoc, ok := item.(bson.D); ok {
				roleName := ""
				for _, f := range roleDoc {
					if f.Key == "Name" {
						roleName = bsonutil.String(f.Value, "Name")
						break
					}
				}
				if roleName == name {
					continue // Remove this one
				}
			}
			filtered = append(filtered, item)
		}

		return setBsonField(doc, "UserRoles", filtered), nil
	})
}

// AddDemoUser adds a new demo user to Security$ProjectSecurity.
func (w *Writer) AddDemoUser(unitID model.ID, userName, password, entity string, userRoles []string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		newUser := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Security$DemoUserImpl"},
			{Key: "UserName", Value: userName},
			{Key: "Password", Value: password},
			{Key: "Entity", Value: entity},
			{Key: "UserRoles", Value: makeMendixStringArray(userRoles)},
		}

		existing := getBsonArray(doc, "DemoUsers")
		if existing == nil {
			existing = bson.A{int32(1)}
		}
		existing = append(existing, newUser)
		return setBsonField(doc, "DemoUsers", existing), nil
	})
}

// RemoveDemoUser removes a demo user by name from Security$ProjectSecurity.
func (w *Writer) RemoveDemoUser(unitID model.ID, userName string) error {
	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		existing := getBsonArray(doc, "DemoUsers")
		if existing == nil {
			return doc, nil
		}

		var filtered bson.A
		for _, item := range existing {
			if _, ok := item.(int32); ok {
				filtered = append(filtered, item)
				continue
			}
			if userDoc, ok := item.(bson.D); ok {
				name := ""
				for _, f := range userDoc {
					if f.Key == "UserName" {
						name = bsonutil.String(f.Value, "UserName")
						break
					}
				}
				if name == userName {
					continue // Remove
				}
			}
			filtered = append(filtered, item)
		}

		return setBsonField(doc, "DemoUsers", filtered), nil
	})
}

// ============================================================================
// Entity Access: GRANT/REVOKE on DomainModels$DomainModel
// ============================================================================

// EntityMemberAccess describes per-member access rights for an access rule.
type EntityMemberAccess struct {
	AttributeRef   string // "Module.Entity.AttrName" or ""
	AssociationRef string // "Module.AssocName" or ""
	AccessRights   string // "None", "ReadOnly", "ReadWrite"
}

// AddEntityAccessRule adds or updates an access rule for the given roles on an entity.
// If an existing rule with the same AllowedModuleRoles is found, it is updated in place.
// If memberAccesses is non-nil, explicit per-member access entries are created;
// otherwise an empty MemberAccesses array is used (DefaultMemberAccessRights applies to all).
// Note: Mendix does not have AllowRead/AllowWrite properties on AccessRule — read/write
// access is determined entirely by DefaultMemberAccessRights and MemberAccesses.
func (w *Writer) AddEntityAccessRule(unitID model.ID, entityName string, roleNames []string,
	allowCreate, allowDelete bool,
	defaultMemberAccess string, xpathConstraint string,
	memberAccesses []EntityMemberAccess) error {

	return w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		entitiesArr := getBsonArray(doc, "Entities")
		if entitiesArr == nil {
			return doc, fmt.Errorf("no Entities array found in domain model")
		}

		found := false
		for i, item := range entitiesArr {
			entityDoc, ok := item.(bson.D)
			if !ok {
				continue
			}
			name := ""
			for _, f := range entityDoc {
				if f.Key == "Name" {
					name = bsonutil.String(f.Value, "Name")
					break
				}
			}
			if name != entityName {
				continue
			}
			found = true

			// Build MemberAccesses BSON
			var memberAccessesBson bson.A
			if len(memberAccesses) > 0 {
				memberAccessesBson = bson.A{int32(3)} // storageListType 3
				for _, ma := range memberAccesses {
					maDoc := bson.D{
						{Key: "$ID", Value: idToBsonBinary(generateUUID())},
						{Key: "$Type", Value: "DomainModels$MemberAccess"},
						{Key: "AccessRights", Value: ma.AccessRights},
					}
					if ma.AttributeRef != "" {
						maDoc = append(maDoc, bson.E{Key: "Attribute", Value: ma.AttributeRef})
					}
					if ma.AssociationRef != "" {
						maDoc = append(maDoc, bson.E{Key: "Association", Value: ma.AssociationRef})
					}
					memberAccessesBson = append(memberAccessesBson, maDoc)
				}
			} else {
				memberAccessesBson = bson.A{int32(3)} // empty — DefaultMemberAccessRights applies
			}

			// Get existing AccessRules
			var accessRules bson.A
			accessRulesIdx := -1
			for j, f := range entityDoc {
				if f.Key == "AccessRules" {
					if arr, ok := f.Value.(bson.A); ok {
						accessRules = arr
					}
					accessRulesIdx = j
					break
				}
			}
			if accessRules == nil {
				accessRules = bson.A{int32(3)} // storageListType 3
			}

			// Check for existing rule with same AllowedModuleRoles — upsert
			existingIdx := -1
			existingID := ""
			for ri, ruleItem := range accessRules {
				ruleDoc, ok := ruleItem.(bson.D)
				if !ok {
					continue
				}
				if rolesMatch(ruleDoc, roleNames) {
					existingIdx = ri
					// Preserve the existing rule's $ID
					for _, rf := range ruleDoc {
						if rf.Key == "$ID" {
							existingID = extractBsonIDValue(rf.Value)
							break
						}
					}
					break
				}
			}

			// Build the rule
			ruleID := generateUUID()
			if existingID != "" {
				ruleID = existingID
			}
			newRule := bson.D{
				{Key: "$ID", Value: idToBsonBinary(ruleID)},
				{Key: "$Type", Value: "DomainModels$AccessRule"},
				{Key: "AllowedModuleRoles", Value: makeMendixStringArray(roleNames)},
				{Key: "AllowCreate", Value: allowCreate},
				{Key: "AllowDelete", Value: allowDelete},
				{Key: "DefaultMemberAccessRights", Value: defaultMemberAccess},
				{Key: "XPathConstraint", Value: xpathConstraint},
				{Key: "XPathConstraintCaption", Value: ""},
				{Key: "Documentation", Value: ""},
				{Key: "MemberAccesses", Value: memberAccessesBson},
			}

			if existingIdx >= 0 {
				// Merge additively: keep the higher access level for each member
				// and OR the structural permissions (Create/Delete).
				existingRule, _ := accessRules[existingIdx].(bson.D)
				newRule = mergeAccessRule(existingRule, newRule)
				accessRules[existingIdx] = newRule
			} else {
				// Append new rule
				accessRules = append(accessRules, newRule)
			}

			if accessRulesIdx >= 0 {
				entityDoc[accessRulesIdx].Value = accessRules
			} else {
				entityDoc = append(entityDoc, bson.E{Key: "AccessRules", Value: accessRules})
			}
			entitiesArr[i] = entityDoc
			break
		}

		if !found {
			return doc, fmt.Errorf("entity not found: %s", entityName)
		}

		return setBsonField(doc, "Entities", entitiesArr), nil
	})
}

// rolesMatch checks if a rule's AllowedModuleRoles matches the given role names (order-independent).
func rolesMatch(ruleDoc bson.D, roleNames []string) bool {
	for _, f := range ruleDoc {
		if f.Key != "AllowedModuleRoles" {
			continue
		}
		arr, ok := f.Value.(bson.A)
		if !ok {
			return false
		}
		// Extract role strings from the Mendix array (skip int32 markers)
		var existing []string
		for _, item := range arr {
			if s, ok := item.(string); ok {
				existing = append(existing, s)
			}
		}
		if len(existing) != len(roleNames) {
			return false
		}
		// Build set for comparison
		set := make(map[string]bool, len(existing))
		for _, s := range existing {
			set[s] = true
		}
		for _, rn := range roleNames {
			if !set[rn] {
				return false
			}
		}
		return true
	}
	return false
}

// accessRightsLevel returns a numeric level for access rights comparison.
// None=0 < ReadOnly=1 < ReadWrite=2.
func accessRightsLevel(s string) int {
	switch s {
	case "ReadWrite":
		return 2
	case "ReadOnly":
		return 1
	default:
		return 0
	}
}

// mergeAccessRule merges a new access rule into an existing one additively.
// AllowCreate/AllowDelete are OR'd. MemberAccesses keep the higher access level.
// XPathConstraint is replaced only if the new rule specifies one.
func mergeAccessRule(existing, newRule bson.D) bson.D {
	// Extract existing MemberAccesses keyed by attribute/association ref
	existingMembers := make(map[string]string) // ref -> access rights
	for _, f := range existing {
		if f.Key != "MemberAccesses" {
			continue
		}
		arr, ok := f.Value.(bson.A)
		if !ok {
			break
		}
		for _, item := range arr {
			maDoc, ok := item.(bson.D)
			if !ok {
				continue
			}
			var ref, rights string
			for _, mf := range maDoc {
				switch mf.Key {
				case "Attribute":
					ref = bsonutil.String(mf.Value, "Attribute")
				case "Association":
					ref = bsonutil.String(mf.Value, "Association")
				case "AccessRights":
					rights = bsonutil.String(mf.Value, "AccessRights")
				}
			}
			if ref != "" {
				existingMembers[ref] = rights
			}
		}
	}

	// Extract existing AllowCreate/AllowDelete and DefaultMemberAccessRights
	var existCreate, existDelete bool
	var existDefault, existXPath string
	for _, f := range existing {
		switch f.Key {
		case "AllowCreate":
			existCreate = bsonutil.Bool(f.Value, "AllowCreate")
		case "AllowDelete":
			existDelete = bsonutil.Bool(f.Value, "AllowDelete")
		case "DefaultMemberAccessRights":
			existDefault = bsonutil.String(f.Value, "DefaultMemberAccessRights")
		case "XPathConstraint":
			existXPath = bsonutil.String(f.Value, "XPathConstraint")
		}
	}

	// Merge into newRule
	for i, f := range newRule {
		switch f.Key {
		case "AllowCreate":
			newVal := bsonutil.Bool(f.Value, "AllowCreate")
			newRule[i].Value = newVal || existCreate
		case "AllowDelete":
			newVal := bsonutil.Bool(f.Value, "AllowDelete")
			newRule[i].Value = newVal || existDelete
		case "DefaultMemberAccessRights":
			newVal := bsonutil.String(f.Value, "DefaultMemberAccessRights")
			if accessRightsLevel(existDefault) > accessRightsLevel(newVal) {
				newRule[i].Value = existDefault
			}
		case "XPathConstraint":
			newVal := bsonutil.String(f.Value, "XPathConstraint")
			if newVal == "" && existXPath != "" {
				newRule[i].Value = existXPath
			}
		case "MemberAccesses":
			arr, ok := f.Value.(bson.A)
			if !ok {
				break
			}
			for j, item := range arr {
				maDoc, ok := item.(bson.D)
				if !ok {
					continue
				}
				var ref, newRights string
				for _, mf := range maDoc {
					switch mf.Key {
					case "Attribute":
						ref = bsonutil.String(mf.Value, "Attribute")
					case "Association":
						ref = bsonutil.String(mf.Value, "Association")
					case "AccessRights":
						newRights = bsonutil.String(mf.Value, "AccessRights")
					}
				}
				if ref == "" {
					continue
				}
				if existRights, ok := existingMembers[ref]; ok {
					if accessRightsLevel(existRights) > accessRightsLevel(newRights) {
						// Upgrade to existing higher level
						for k, mf := range maDoc {
							if mf.Key == "AccessRights" {
								maDoc[k].Value = existRights
								break
							}
						}
						arr[j] = maDoc
					}
				}
			}
			newRule[i].Value = arr
		}
	}

	return newRule
}

// RemoveEntityAccessRule removes the given roles from access rules on an entity.
// For multi-role rules, only the specified roles are removed from the rule's role list.
// If a rule has no remaining roles after removal, the entire rule is deleted.
// Returns the number of rules that were modified or removed.
func (w *Writer) RemoveEntityAccessRule(unitID model.ID, entityName string, roleNames []string) (int, error) {
	modified := 0
	err := w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		entitiesArr := getBsonArray(doc, "Entities")
		if entitiesArr == nil {
			return doc, fmt.Errorf("no Entities array found in domain model")
		}

		removeRoles := make(map[string]bool)
		for _, r := range roleNames {
			removeRoles[r] = true
		}

		found := false
		for i, item := range entitiesArr {
			entityDoc, ok := item.(bson.D)
			if !ok {
				continue
			}
			name := ""
			for _, f := range entityDoc {
				if f.Key == "Name" {
					name = bsonutil.String(f.Value, "Name")
					break
				}
			}
			if name != entityName {
				continue
			}
			found = true

			for j, f := range entityDoc {
				if f.Key != "AccessRules" {
					continue
				}
				arr, ok := f.Value.(bson.A)
				if !ok {
					break
				}

				var filtered bson.A
				for _, ruleItem := range arr {
					if _, ok := ruleItem.(int32); ok {
						filtered = append(filtered, ruleItem)
						continue
					}
					ruleDoc, ok := ruleItem.(bson.D)
					if !ok {
						filtered = append(filtered, ruleItem)
						continue
					}

					keepRule, wasModified := removeRolesFromAccessRule(ruleDoc, removeRoles)
					if wasModified {
						modified++
					}
					if keepRule {
						filtered = append(filtered, ruleDoc)
					}
				}

				entityDoc[j].Value = filtered
				break
			}

			entitiesArr[i] = entityDoc
			break
		}

		if !found {
			return doc, fmt.Errorf("entity not found: %s", entityName)
		}

		return setBsonField(doc, "Entities", entitiesArr), nil
	})
	return modified, err
}

// removeRolesFromAccessRule removes the specified roles from a rule's AllowedModuleRoles.
// Returns (keepRule, wasModified). keepRule is false if no roles remain (rule should be deleted).
func removeRolesFromAccessRule(ruleDoc bson.D, removeRoles map[string]bool) (bool, bool) {
	for k, rf := range ruleDoc {
		if rf.Key != "AllowedModuleRoles" {
			continue
		}
		rolesArr, ok := rf.Value.(bson.A)
		if !ok {
			return true, false
		}

		var remaining bson.A
		removed := false
		roleCount := 0
		for _, rr := range rolesArr {
			if _, ok := rr.(int32); ok {
				remaining = append(remaining, rr) // keep array marker
				continue
			}
			if s, ok := rr.(string); ok {
				if removeRoles[s] {
					removed = true
				} else {
					remaining = append(remaining, rr)
					roleCount++
				}
			}
		}

		if !removed {
			return true, false // no change
		}
		if roleCount == 0 {
			return false, true // delete entire rule
		}
		ruleDoc[k].Value = remaining
		return true, true // keep rule with fewer roles
	}
	return true, false
}

// EntityAccessRevocation describes what to revoke from an entity access rule.
type EntityAccessRevocation struct {
	RevokeCreate bool
	RevokeDelete bool
	// Members to fully revoke (set to None)
	RevokeReadMembers []string // attribute/association refs to set to None
	// Members to downgrade from ReadWrite to ReadOnly
	RevokeWriteMembers []string // attribute/association refs to downgrade
	// Revoke all read/write access
	RevokeReadAll  bool
	RevokeWriteAll bool
}

// RevokeEntityMemberAccess performs a partial revoke on an existing access rule.
// It downgrades or removes specific rights without deleting the entire rule.
// Returns the number of rules modified.
func (w *Writer) RevokeEntityMemberAccess(unitID model.ID, entityName string, roleNames []string, revocation EntityAccessRevocation) (int, error) {
	modified := 0
	err := w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		entitiesArr := getBsonArray(doc, "Entities")
		if entitiesArr == nil {
			return doc, fmt.Errorf("no Entities array found in domain model")
		}

		found := false
		for i, item := range entitiesArr {
			entityDoc, ok := item.(bson.D)
			if !ok {
				continue
			}
			name := ""
			for _, f := range entityDoc {
				if f.Key == "Name" {
					name = bsonutil.String(f.Value, "Name")
					break
				}
			}
			if name != entityName {
				continue
			}
			found = true

			for j, f := range entityDoc {
				if f.Key != "AccessRules" {
					continue
				}
				arr, ok := f.Value.(bson.A)
				if !ok {
					break
				}

				for ri, ruleItem := range arr {
					ruleDoc, ok := ruleItem.(bson.D)
					if !ok {
						continue
					}
					if !rolesMatch(ruleDoc, roleNames) {
						continue
					}

					// Found matching rule — apply revocations
					ruleModified := false

					// Build sets for quick lookup
					revokeReadSet := make(map[string]bool)
					for _, ref := range revocation.RevokeReadMembers {
						revokeReadSet[ref] = true
					}
					revokeWriteSet := make(map[string]bool)
					for _, ref := range revocation.RevokeWriteMembers {
						revokeWriteSet[ref] = true
					}

					for k, rf := range ruleDoc {
						switch rf.Key {
						case "AllowCreate":
							if revocation.RevokeCreate {
								ruleDoc[k].Value = false
								ruleModified = true
							}
						case "AllowDelete":
							if revocation.RevokeDelete {
								ruleDoc[k].Value = false
								ruleModified = true
							}
						case "DefaultMemberAccessRights":
							if revocation.RevokeReadAll {
								ruleDoc[k].Value = "None"
								ruleModified = true
							} else if revocation.RevokeWriteAll {
								cur := bsonutil.String(rf.Value, "DefaultMemberAccessRights")
								if cur == "ReadWrite" {
									ruleDoc[k].Value = "ReadOnly"
									ruleModified = true
								}
							}
						case "MemberAccesses":
							maArr, ok := rf.Value.(bson.A)
							if !ok {
								break
							}
							for mi, maItem := range maArr {
								maDoc, ok := maItem.(bson.D)
								if !ok {
									continue
								}
								var ref, rights string
								for _, mf := range maDoc {
									switch mf.Key {
									case "Attribute":
										ref = bsonutil.String(mf.Value, "Attribute")
									case "Association":
										ref = bsonutil.String(mf.Value, "Association")
									case "AccessRights":
										rights = bsonutil.String(mf.Value, "AccessRights")
									}
								}
								if ref == "" {
									continue
								}

								newRights := rights
								if revocation.RevokeReadAll || revokeReadSet[ref] {
									newRights = "None"
								} else if revocation.RevokeWriteAll || revokeWriteSet[ref] {
									if rights == "ReadWrite" {
										newRights = "ReadOnly"
									}
								}

								if newRights != rights {
									for mk, mf := range maDoc {
										if mf.Key == "AccessRights" {
											maDoc[mk].Value = newRights
											break
										}
									}
									maArr[mi] = maDoc
									ruleModified = true
								}
							}
							ruleDoc[k].Value = maArr
						}
					}

					if ruleModified {
						arr[ri] = ruleDoc
						modified++
					}
					break
				}

				entityDoc[j].Value = arr
				break
			}

			entitiesArr[i] = entityDoc
			break
		}

		if !found {
			return doc, fmt.Errorf("entity not found: %s", entityName)
		}

		return setBsonField(doc, "Entities", entitiesArr), nil
	})
	return modified, err
}

// RemoveRoleFromAllEntities removes the given role from all entity access rules in a domain model.
// Used by DROP MODULE ROLE cascade. Returns the number of rules modified/removed.
func (w *Writer) RemoveRoleFromAllEntities(unitID model.ID, roleName string) (int, error) {
	modified := 0
	err := w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		entitiesArr := getBsonArray(doc, "Entities")
		if entitiesArr == nil {
			return doc, nil // no entities, nothing to do
		}

		removeRoles := map[string]bool{roleName: true}

		for i, item := range entitiesArr {
			entityDoc, ok := item.(bson.D)
			if !ok {
				continue
			}

			for j, f := range entityDoc {
				if f.Key != "AccessRules" {
					continue
				}
				arr, ok := f.Value.(bson.A)
				if !ok {
					break
				}

				var filtered bson.A
				for _, ruleItem := range arr {
					if _, ok := ruleItem.(int32); ok {
						filtered = append(filtered, ruleItem)
						continue
					}
					ruleDoc, ok := ruleItem.(bson.D)
					if !ok {
						filtered = append(filtered, ruleItem)
						continue
					}

					keepRule, wasModified := removeRolesFromAccessRule(ruleDoc, removeRoles)
					if wasModified {
						modified++
					}
					if keepRule {
						filtered = append(filtered, ruleDoc)
					}
				}

				entityDoc[j].Value = filtered
				break
			}

			entitiesArr[i] = entityDoc
		}

		return setBsonField(doc, "Entities", entitiesArr), nil
	})
	return modified, err
}

// ReconcileMemberAccesses reconciles MemberAccesses on all AccessRules within a domain model
// to match the current entity structure. It adds entries for new attributes/associations and
// removes entries for deleted ones. Returns the number of rules modified.
func (w *Writer) ReconcileMemberAccesses(unitID model.ID, moduleName string) (int, error) {
	modified := 0

	err := w.readPatchWrite(unitID, func(doc bson.D) (bson.D, error) {
		entitiesArr := getBsonArray(doc, "Entities")
		if entitiesArr == nil {
			return doc, nil
		}

		// Collect all association names in this module (from Associations + CrossAssociations)
		assocNames := map[string]bool{}
		assocArr := getBsonArray(doc, "Associations")
		for _, item := range assocArr {
			assocDoc, ok := item.(bson.D)
			if !ok {
				continue
			}
			for _, f := range assocDoc {
				if f.Key == "Name" {
					if name, ok := f.Value.(string); ok {
						assocNames[name] = true
					}
					break
				}
			}
		}
		crossArr := getBsonArray(doc, "CrossAssociations")
		for _, item := range crossArr {
			crossDoc, ok := item.(bson.D)
			if !ok {
				continue
			}
			for _, f := range crossDoc {
				if f.Key == "Name" {
					if name, ok := f.Value.(string); ok {
						assocNames[name] = true
					}
					break
				}
			}
		}

		for i, item := range entitiesArr {
			entityDoc, ok := item.(bson.D)
			if !ok {
				continue
			}

			// Get entity name
			entityName := ""
			for _, f := range entityDoc {
				if f.Key == "Name" {
					entityName = bsonutil.String(f.Value, "Name")
					break
				}
			}
			if entityName == "" {
				continue
			}

			// Collect current attribute names and track calculated attributes
			attrNames := map[string]bool{}
			calculatedAttrs := map[string]bool{}
			attrsArr := getBsonArray(entityDoc, "Attributes")
			for _, attrItem := range attrsArr {
				attrDoc, ok := attrItem.(bson.D)
				if !ok {
					continue
				}
				attrName := ""
				isCalculated := false
				for _, f := range attrDoc {
					if f.Key == "Name" {
						attrName = bsonutil.String(f.Value, "Name")
					}
					if f.Key == "Value" {
						if valueDoc, ok := f.Value.(bson.D); ok {
							for _, vf := range valueDoc {
								if vf.Key == "$Type" {
									if vt, ok := vf.Value.(string); ok && vt == "DomainModels$CalculatedValue" {
										isCalculated = true
									}
								}
							}
						}
					}
				}
				if attrName != "" {
					attrNames[attrName] = true
					if isCalculated {
						calculatedAttrs[attrName] = true
					}
				}
			}

			// Collect associations where this entity is the FROM entity (ParentPointer).
			// In Mendix BSON, ParentPointer = FROM entity (FK owner), ChildPointer = TO entity.
			// MemberAccess for associations is only required on the FROM (owner) side.
			entityID := ""
			for _, f := range entityDoc {
				if f.Key == "$ID" {
					entityID = extractBsonIDValue(f.Value)
					break
				}
			}
			entityAssocNames := map[string]bool{}

			// Check for system associations (HasOwner, HasChangedBy) in NoGeneralization.
			// These add implicit System.owner / System.changedBy associations that
			// require MemberAccess entries. Stored as full refs (e.g., "System.owner").
			systemAssocRefs := map[string]bool{}
			for _, f := range entityDoc {
				if f.Key == "Generalization" || f.Key == "MaybeGeneralization" {
					if genDoc, ok := f.Value.(bson.D); ok {
						for _, gf := range genDoc {
							if gf.Key == "$Type" {
								if gt, ok := gf.Value.(string); ok && gt == "DomainModels$NoGeneralization" {
									for _, ngf := range genDoc {
										switch ngf.Key {
										case "HasOwner":
											if v, ok := ngf.Value.(bool); ok && v {
												systemAssocRefs["System.owner"] = true
											}
										case "HasChangedBy":
											if v, ok := ngf.Value.(bool); ok && v {
												systemAssocRefs["System.changedBy"] = true
											}
										}
									}
								}
							}
						}
					}
					break
				}
			}
			for _, aItem := range assocArr {
				aDoc, ok := aItem.(bson.D)
				if !ok {
					continue
				}
				aParentID := ""
				aName := ""
				for _, f := range aDoc {
					switch f.Key {
					case "ParentPointer":
						aParentID = extractBsonIDValue(f.Value)
					case "Name":
						aName = bsonutil.String(f.Value, "Name")
					}
				}
				if aParentID == entityID && aName != "" {
					entityAssocNames[aName] = true
				}
			}
			for _, caItem := range crossArr {
				caDoc, ok := caItem.(bson.D)
				if !ok {
					continue
				}
				parentID := ""
				caName := ""
				for _, f := range caDoc {
					if f.Key == "ParentPointer" {
						parentID = extractBsonIDValue(f.Value)
					}
					if f.Key == "Name" {
						caName = bsonutil.String(f.Value, "Name")
					}
				}
				if parentID == entityID && caName != "" {
					entityAssocNames[caName] = true
				}
			}

			// Process AccessRules
			for j, f := range entityDoc {
				if f.Key != "AccessRules" {
					continue
				}
				rulesArr, ok := f.Value.(bson.A)
				if !ok {
					break
				}

				for k, ruleItem := range rulesArr {
					ruleDoc, ok := ruleItem.(bson.D)
					if !ok {
						continue
					}

					// Strip invalid properties (AllowRead, AllowWrite) that
					// old mxcli versions wrote. These crash Studio Pro with
					// "Sequence contains no matching element" in MprProperty..ctor.
					ruleDoc, stripped := stripInvalidAccessRuleProps(ruleDoc)
					if stripped {
						rulesArr[k] = ruleDoc
						modified++
					}

					// Find MemberAccesses
					for m, rf := range ruleDoc {
						if rf.Key != "MemberAccesses" {
							continue
						}
						maArr, ok := rf.Value.(bson.A)
						if !ok {
							break
						}

						// If empty (just the storage marker), skip
						if len(maArr) <= 1 {
							break
						}

						// Get DefaultMemberAccessRights for new entries
						defaultRights := "ReadWrite"
						for _, drf := range ruleDoc {
							if drf.Key == "DefaultMemberAccessRights" {
								if dr, ok := drf.Value.(string); ok {
									defaultRights = dr
								}
								break
							}
						}

						// Build set of covered attributes and associations
						coveredAttrs := map[string]bool{}
						coveredAssocs := map[string]bool{}
						changed := false
						var filtered bson.A
						// Preserve the storage marker
						if len(maArr) > 0 {
							filtered = bson.A{maArr[0]}
						}

						coveredSystemAssocs := map[string]bool{}
						for _, maItem := range maArr[1:] {
							maDoc, ok := maItem.(bson.D)
							if !ok {
								continue
							}
							attrRef := ""
							assocRef := ""
							for _, mf := range maDoc {
								if mf.Key == "Attribute" {
									attrRef = bsonutil.String(mf.Value, "Attribute")
								}
								if mf.Key == "Association" {
									assocRef = bsonutil.String(mf.Value, "Association")
								}
							}

							if attrRef != "" {
								// Extract attribute name from Module.Entity.AttrName
								parts := splitQualifiedRef(attrRef)
								if parts != "" && attrNames[parts] {
									coveredAttrs[parts] = true
									// Downgrade write rights on calculated attributes (CE6592)
									if calculatedAttrs[parts] {
										maDoc = downgradeCalculatedAttrRights(maDoc)
									}
									filtered = append(filtered, maDoc)
								} else {
									changed = true // stale attribute entry removed
								}
							} else if assocRef != "" {
								// Check if it's a system association (e.g., "System.owner")
								if systemAssocRefs[assocRef] {
									coveredSystemAssocs[assocRef] = true
									filtered = append(filtered, maItem)
								} else {
									// Extract association name from Module.AssocName
									parts := splitAssocRef(assocRef)
									if parts != "" && entityAssocNames[parts] {
										coveredAssocs[parts] = true
										filtered = append(filtered, maItem)
									} else {
										changed = true // stale association entry removed
									}
								}
							} else {
								filtered = append(filtered, maItem)
							}
						}

						// Add missing attributes
						for attrName := range attrNames {
							if !coveredAttrs[attrName] {
								rights := defaultRights
								// Calculated attributes cannot have write rights (CE6592)
								if calculatedAttrs[attrName] && (rights == "ReadWrite" || rights == "WriteOnly") {
									rights = "ReadOnly"
								}
								newMA := bson.D{
									{Key: "$ID", Value: idToBsonBinary(generateUUID())},
									{Key: "$Type", Value: "DomainModels$MemberAccess"},
									{Key: "AccessRights", Value: rights},
									{Key: "Attribute", Value: moduleName + "." + entityName + "." + attrName},
								}
								filtered = append(filtered, newMA)
								changed = true
							}
						}

						// Add missing module associations
						for aName := range entityAssocNames {
							if !coveredAssocs[aName] {
								newMA := bson.D{
									{Key: "$ID", Value: idToBsonBinary(generateUUID())},
									{Key: "$Type", Value: "DomainModels$MemberAccess"},
									{Key: "AccessRights", Value: defaultRights},
									{Key: "Association", Value: moduleName + "." + aName},
								}
								filtered = append(filtered, newMA)
								changed = true
							}
						}

						// Add missing system associations (e.g., System.owner)
						for sysRef := range systemAssocRefs {
							if !coveredSystemAssocs[sysRef] {
								newMA := bson.D{
									{Key: "$ID", Value: idToBsonBinary(generateUUID())},
									{Key: "$Type", Value: "DomainModels$MemberAccess"},
									{Key: "AccessRights", Value: defaultRights},
									{Key: "Association", Value: sysRef},
								}
								filtered = append(filtered, newMA)
								changed = true
							}
						}

						if changed {
							ruleDoc[m].Value = filtered
							rulesArr[k] = ruleDoc
							modified++
						}

						break
					}
				}

				entityDoc[j].Value = rulesArr
				break
			}

			entitiesArr[i] = entityDoc
		}

		return setBsonField(doc, "Entities", entitiesArr), nil
	})

	return modified, err
}

// downgradeCalculatedAttrRights changes ReadWrite/WriteOnly to ReadOnly on a MemberAccess doc.
func downgradeCalculatedAttrRights(doc bson.D) bson.D {
	for i, f := range doc {
		if f.Key == "AccessRights" {
			if rights, ok := f.Value.(string); ok && (rights == "ReadWrite" || rights == "WriteOnly") {
				doc[i].Value = "ReadOnly"
			}
		}
	}
	return doc
}

// extractBsonIDValue extracts a string ID from various BSON ID representations.
func extractBsonIDValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case primitive.Binary:
		return blobToUUID(val.Data)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// splitQualifiedRef extracts the last component from "Module.Entity.AttrName".
func splitQualifiedRef(ref string) string {
	parts := splitByDot(ref)
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return ""
}

// splitAssocRef extracts the association name from "Module.AssocName".
func splitAssocRef(ref string) string {
	parts := splitByDot(ref)
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

// splitByDot splits a string by "." - simple helper to avoid importing strings.
func splitByDot(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// invalidAccessRuleProps lists BSON keys that are NOT valid Mendix metamodel
// properties on DomainModels$AccessRule. Old mxcli versions wrote these;
// Studio Pro crashes with "Sequence contains no matching element" if present.
var invalidAccessRuleProps = map[string]bool{
	"AllowRead":  true,
	"AllowWrite": true,
}

// stripInvalidAccessRuleProps removes invalid properties from an AccessRule BSON document.
// Returns the cleaned document and true if any properties were removed.
func stripInvalidAccessRuleProps(doc bson.D) (bson.D, bool) {
	cleaned := make(bson.D, 0, len(doc))
	stripped := false
	for _, f := range doc {
		if invalidAccessRuleProps[f.Key] {
			stripped = true
			continue
		}
		cleaned = append(cleaned, f)
	}
	return cleaned, stripped
}

// ensure primitive import is used
var _ = primitive.Binary{}
