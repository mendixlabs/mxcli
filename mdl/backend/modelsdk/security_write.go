// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genSec "github.com/mendixlabs/mxcli/modelsdk/gen/security"
)

// Studio Pro writes Security$ProjectSecurity's UserRoles and DemoUsers lists
// with typed-array marker 2, not the encoder's default 3 (measured on PedApp
// 11.13.0, TestApp 11.14.0 and the expr-checker fixture: every document
// agrees). Without these, rewriting a user role or demo user with an unchanged
// definition changed the marker, so running `describe user role` output wrote
// the unit (#731).
func init() {
	codec.RegisterPropertyListMarker("Security$ProjectSecurity", "UserRoles", 2)
	codec.RegisterPropertyListMarker("Security$ProjectSecurity", "DemoUsers", 2)
	// Studio Pro gives every user role a GUID; mxcli wrote new ones without, so
	// Mendix minted one on its next save and the two revisions of the role no
	// longer had the same properties — which Mendix's merge engine refuses to
	// compare (mendixlabs/mxcli#1373). A new role gets GUID = $ID, as a new
	// entity does; a stored role passes through with the GUID it has.
	codec.RegisterTypeDefaults("Security$UserRole", codec.TypeDefaults{EmitGUID: true})
}

// loadModuleSecurityGen decodes a Security$ModuleSecurity unit by ID.
func (b *Backend) loadModuleSecurityGen(unitID model.ID) (*genSec.ModuleSecurity, error) {
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return nil, fmt.Errorf("read module security unit %s: %w", unitID, err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode module security unit %s: %w", unitID, err)
	}
	ms, ok := el.(*genSec.ModuleSecurity)
	if !ok {
		return nil, fmt.Errorf("unit %s is not a ModuleSecurity (%s)", unitID, el.TypeName())
	}
	return ms, nil
}

// AddModuleRole adds (or, case-insensitively, updates) a module role on a module's
// Security$ModuleSecurity document. Mirrors legacy: a case-insensitive duplicate is
// overwritten in place (keeping its ID stable so references stay valid).
func (b *Backend) AddModuleRole(unitID model.ID, roleName, description string) error {
	if b.writer == nil {
		return fmt.Errorf("AddModuleRole: not connected for writing")
	}
	ms, err := b.loadModuleSecurityGen(unitID)
	if err != nil {
		return err
	}
	for _, el := range ms.ModuleRolesItems() {
		if r, ok := el.(*genSec.ModuleRole); ok && strings.EqualFold(r.Name(), roleName) {
			r.SetName(roleName)
			r.SetDescription(description)
			return b.persistUnit(unitID, ms)
		}
	}
	r := genSec.NewModuleRole()
	assignID(r)
	r.SetName(roleName)
	r.SetDescription(description)
	ms.AddModuleRoles(r)
	return b.persistUnit(unitID, ms)
}

// RemoveModuleRole removes a module role (by case-insensitive name) from a module's
// Security$ModuleSecurity document.
func (b *Backend) RemoveModuleRole(unitID model.ID, roleName string) error {
	if b.writer == nil {
		return fmt.Errorf("RemoveModuleRole: not connected for writing")
	}
	ms, err := b.loadModuleSecurityGen(unitID)
	if err != nil {
		return err
	}
	for i, el := range ms.ModuleRolesItems() {
		if r, ok := el.(*genSec.ModuleRole); ok && strings.EqualFold(r.Name(), roleName) {
			ms.RemoveModuleRoles(i)
			return b.persistUnit(unitID, ms)
		}
	}
	return nil // not present — nothing to do
}

// loadProjectSecurityGen decodes a Security$ProjectSecurity unit by ID.
func (b *Backend) loadProjectSecurityGen(unitID model.ID) (*genSec.ProjectSecurity, error) {
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return nil, fmt.Errorf("read project security unit %s: %w", unitID, err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode project security unit %s: %w", unitID, err)
	}
	ps, ok := el.(*genSec.ProjectSecurity)
	if !ok {
		return nil, fmt.Errorf("unit %s is not a ProjectSecurity (%s)", unitID, el.TypeName())
	}
	return ps, nil
}

// AddUserRole adds (or, by name, updates) a project user role with its module-role
// mappings and manage-all flag.
func (b *Backend) AddUserRole(unitID model.ID, name string, moduleRoles []string, manageAllRoles bool) error {
	if b.writer == nil {
		return fmt.Errorf("AddUserRole: not connected for writing")
	}
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	for _, el := range ps.UserRolesItems() {
		if r, ok := el.(*genSec.UserRole); ok && strings.EqualFold(r.Name(), name) {
			r.SetName(name)
			r.SetModuleRolesQualifiedNames(moduleRoles)
			r.SetManageAllRoles(manageAllRoles)
			return b.persistUnit(unitID, ps)
		}
	}
	r := genSec.NewUserRole()
	assignID(r)
	r.SetName(name)
	r.SetModuleRolesQualifiedNames(moduleRoles)
	r.SetManageAllRoles(manageAllRoles)
	ps.AddUserRoles(r)
	return b.persistUnit(unitID, ps)
}

// RemoveUserRole removes a project user role by (case-insensitive) name.
func (b *Backend) RemoveUserRole(unitID model.ID, name string) error {
	if b.writer == nil {
		return fmt.Errorf("RemoveUserRole: not connected for writing")
	}
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	for i, el := range ps.UserRolesItems() {
		if r, ok := el.(*genSec.UserRole); ok && strings.EqualFold(r.Name(), name) {
			ps.RemoveUserRoles(i)
			return b.persistUnit(unitID, ps)
		}
	}
	return nil
}

// AddDemoUser adds (or, by username, updates) a demo user (sample user bound to an
// entity with a set of user roles).
func (b *Backend) AddDemoUser(unitID model.ID, userName, password, entity string, userRoles []string) error {
	if b.writer == nil {
		return fmt.Errorf("AddDemoUser: not connected for writing")
	}
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	for _, el := range ps.DemoUsersItems() {
		if d, ok := el.(*genSec.DemoUser); ok && strings.EqualFold(d.UserName(), userName) {
			d.SetPassword(password)
			d.SetEntityQualifiedName(entity)
			d.SetUserRolesQualifiedNames(userRoles)
			return b.persistUnit(unitID, ps)
		}
	}
	d := genSec.NewDemoUser()
	assignID(d)
	d.SetUserName(userName)
	d.SetPassword(password)
	d.SetEntityQualifiedName(entity)
	d.SetUserRolesQualifiedNames(userRoles)
	ps.AddDemoUsers(d)
	return b.persistUnit(unitID, ps)
}

// SetProjectSecurityLevel sets the project's security level (e.g. "CheckEverything").
func (b *Backend) SetProjectSecurityLevel(unitID model.ID, level string) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	ps.SetSecurityLevel(level)
	return b.persistUnit(unitID, ps)
}

// SetProjectDemoUsersEnabled toggles whether demo users are enabled.
func (b *Backend) SetProjectDemoUsersEnabled(unitID model.ID, enabled bool) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	ps.SetEnableDemoUsers(enabled)
	return b.persistUnit(unitID, ps)
}

// SetProjectStrictMode toggles security strict mode.
//
// StrictMode is a plain bool declared by BOTH generated sources — gen binds it
// as property.NewPrimitive[bool]("StrictMode") and generated/metamodel declares
// it on ProjectSecurity — and mxcli already reads it back from real projects,
// so this writes a property Studio Pro knows rather than one gen merely offers.
func (b *Backend) SetProjectStrictMode(unitID model.ID, enabled bool) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	ps.SetStrictMode(enabled)
	return b.persistUnit(unitID, ps)
}

// SetProjectAdminUserName renames the built-in administrator account
// (Security$ProjectSecurity.AdminUserName, MxAdmin by default). The password
// is left as stored.
func (b *Backend) SetProjectAdminUserName(unitID model.ID, name string) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	ps.SetAdminUserName(name)
	return b.persistUnit(unitID, ps)
}

// SetProjectAdminUserRole sets the built-in administrator's user role
// (Security$ProjectSecurity.AdminUserRole), stored by name like the guest user
// role (ako/mxcli#2015).
func (b *Backend) SetProjectAdminUserRole(unitID model.ID, role string) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	ps.SetAdminUserRoleName(role)
	return b.persistUnit(unitID, ps)
}

// SetProjectAdminPassword sets the built-in administrator's password
// (Security$ProjectSecurity.AdminPassword), which Studio Pro stores in the
// model as entered. The user name is left as stored (mendixlabs/mxcli#624).
func (b *Backend) SetProjectAdminPassword(unitID model.ID, password string) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	ps.SetAdminPassword(password)
	return b.persistUnit(unitID, ps)
}

// SetProjectGuestAccess toggles anonymous (guest) access. An empty
// guestUserRole leaves the stored role alone, so turning access off and back on
// does not lose it.
func (b *Backend) SetProjectGuestAccess(unitID model.ID, enabled bool, guestUserRole string) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	ps.SetEnableGuestAccess(enabled)
	if guestUserRole != "" {
		ps.SetGuestUserRoleName(guestUserRole)
	}
	return b.persistUnit(unitID, ps)
}

// AlterUserRoleModuleRoles adds or removes module-role mappings on a project user
// role (by name). add=true unions, add=false subtracts.
func (b *Backend) AlterUserRoleModuleRoles(unitID model.ID, userRoleName string, add bool, moduleRoles []string) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	for _, el := range ps.UserRolesItems() {
		r, ok := el.(*genSec.UserRole)
		if !ok || !strings.EqualFold(r.Name(), userRoleName) {
			continue
		}
		cur := r.ModuleRolesQualifiedNames()
		if add {
			have := map[string]bool{}
			for _, m := range cur {
				have[m] = true
			}
			for _, m := range moduleRoles {
				if !have[m] {
					cur = append(cur, m)
					have[m] = true
				}
			}
		} else {
			drop := map[string]bool{}
			for _, m := range moduleRoles {
				drop[m] = true
			}
			kept := cur[:0:0]
			for _, m := range cur {
				if !drop[m] {
					kept = append(kept, m)
				}
			}
			cur = kept
		}
		r.SetModuleRolesQualifiedNames(cur)
		return b.persistUnit(unitID, ps)
	}
	return fmt.Errorf("AlterUserRoleModuleRoles: user role not found: %s", userRoleName)
}

// SetUserRoleProperties sets the properties props states on a project user
// role (by name); a nil property is left as stored.
func (b *Backend) SetUserRoleProperties(unitID model.ID, userRoleName string, props backend.UserRoleProperties) error {
	if b.writer == nil {
		return fmt.Errorf("SetUserRoleProperties: not connected for writing")
	}
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	for _, el := range ps.UserRolesItems() {
		r, ok := el.(*genSec.UserRole)
		if !ok || !strings.EqualFold(r.Name(), userRoleName) {
			continue
		}
		if props.Description != nil {
			r.SetDescription(*props.Description)
		}
		if props.ManageAllRoles != nil {
			r.SetManageAllRoles(*props.ManageAllRoles)
		}
		if props.ManageUsersWithoutRoles != nil {
			r.SetManageUsersWithoutRoles(*props.ManageUsersWithoutRoles)
		}
		if props.CheckSecurity != nil {
			r.SetCheckSecurity(*props.CheckSecurity)
		}
		if props.ManageableRoles != nil {
			r.SetManageableRolesQualifiedNames(props.ManageableRoles)
		}
		return b.persistUnit(unitID, ps)
	}
	return fmt.Errorf("SetUserRoleProperties: user role not found: %s", userRoleName)
}

// RemoveDemoUser removes a demo user by (case-insensitive) username.
func (b *Backend) RemoveDemoUser(unitID model.ID, userName string) error {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return err
	}
	for i, el := range ps.DemoUsersItems() {
		if d, ok := el.(*genSec.DemoUser); ok && strings.EqualFold(d.UserName(), userName) {
			ps.RemoveDemoUsers(i)
			return b.persistUnit(unitID, ps)
		}
	}
	return nil
}

// RemoveModuleRoleFromAllUserRoles removes a module-role qualified name from every
// project user role's mappings. Returns the number of user roles modified.
func (b *Backend) RemoveModuleRoleFromAllUserRoles(unitID model.ID, qualifiedRole string) (int, error) {
	ps, err := b.loadProjectSecurityGen(unitID)
	if err != nil {
		return 0, err
	}
	modified := 0
	for _, el := range ps.UserRolesItems() {
		r, ok := el.(*genSec.UserRole)
		if !ok {
			continue
		}
		cur := r.ModuleRolesQualifiedNames()
		kept := make([]string, 0, len(cur))
		for _, m := range cur {
			if m != qualifiedRole {
				kept = append(kept, m)
			}
		}
		if len(kept) != len(cur) {
			r.SetModuleRolesQualifiedNames(kept)
			modified++
		}
	}
	if modified > 0 {
		if err := b.persistUnit(unitID, ps); err != nil {
			return 0, err
		}
	}
	return modified, nil
}

// RemoveFromAllowedRoles removes a single role from a document's allowed-roles list
// (microflow/nanoflow use AllowedModuleRoles; page/snippet use AllowedRoles). Returns
// whether it was present.
func (b *Backend) RemoveFromAllowedRoles(unitID model.ID, roleName string) (bool, error) {
	if b.writer == nil {
		return false, fmt.Errorf("RemoveFromAllowedRoles: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return false, err
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return false, err
	}
	filter := func(cur []string) ([]string, bool) {
		kept := make([]string, 0, len(cur))
		for _, r := range cur {
			if r != roleName {
				kept = append(kept, r)
			}
		}
		return kept, len(kept) != len(cur)
	}
	var removed bool
	switch s := el.(type) {
	case interface {
		AllowedModuleRolesQualifiedNames() []string
		SetAllowedModuleRolesQualifiedNames([]string)
	}:
		kept, changed := filter(s.AllowedModuleRolesQualifiedNames())
		if changed {
			s.SetAllowedModuleRolesQualifiedNames(kept)
			removed = true
		}
	case interface {
		AllowedRolesQualifiedNames() []string
		SetAllowedRolesQualifiedNames([]string)
	}:
		kept, changed := filter(s.AllowedRolesQualifiedNames())
		if changed {
			s.SetAllowedRolesQualifiedNames(kept)
			removed = true
		}
	default:
		return false, fmt.Errorf("RemoveFromAllowedRoles: unit %s (%s) has no allowed-roles list", unitID, el.TypeName())
	}
	if !removed {
		return false, nil
	}
	contents, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return false, err
	}
	if err := b.writer.UpdateRawUnit(string(unitID), contents); err != nil {
		return false, err
	}
	return true, nil
}

// persistUnit re-encodes a mutated unit element and writes it back.
func (b *Backend) persistUnit(unitID model.ID, el element.Element) error {
	contents, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return fmt.Errorf("encode unit %s: %w", unitID, err)
	}
	if err := b.writer.UpdateRawUnit(string(unitID), contents); err != nil {
		return fmt.Errorf("update unit %s: %w", unitID, err)
	}
	return nil
}

// UpdateAllowedRoles sets the AllowedModuleRoles of a document unit (which module
// roles may run a microflow / open a page, etc.) to the given role qualified
// names. It decodes the unit, sets just that property, and re-encodes — the codec
// passes the rest of the document through unchanged, so this is a surgical patch
// regardless of the unit's document type.
func (b *Backend) UpdateAllowedRoles(unitID model.ID, roles []string) error {
	if b.writer == nil {
		return fmt.Errorf("UpdateAllowedRoles: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return fmt.Errorf("UpdateAllowedRoles: read unit %s: %w", unitID, err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return fmt.Errorf("UpdateAllowedRoles: decode unit %s: %w", unitID, err)
	}
	// Microflows/nanoflows expose SetAllowedModuleRolesQualifiedNames; pages/
	// snippets use SetAllowedRolesQualifiedNames (different gen Go name, same
	// AllowedModuleRoles storage key).
	switch s := el.(type) {
	case interface{ SetAllowedModuleRolesQualifiedNames([]string) }:
		s.SetAllowedModuleRolesQualifiedNames(roles)
	case interface{ SetAllowedRolesQualifiedNames([]string) }:
		s.SetAllowedRolesQualifiedNames(roles)
	default:
		return fmt.Errorf("UpdateAllowedRoles: unit %s (%s) has no allowed-roles list", unitID, el.TypeName())
	}
	contents, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return fmt.Errorf("UpdateAllowedRoles: encode unit %s: %w", unitID, err)
	}
	if err := b.writer.UpdateRawUnit(string(unitID), contents); err != nil {
		return fmt.Errorf("UpdateAllowedRoles: update unit %s: %w", unitID, err)
	}
	return nil
}
