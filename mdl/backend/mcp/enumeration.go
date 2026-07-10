// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

const enumerationDocType = "Enumerations$Enumeration"

// CreateEnumeration creates an enumeration document via ped_create_document and
// registers it in the session so it is visible to ListEnumerations this run
// (e.g. a subsequent CREATE ENTITY with an attribute of this enum type).
func (b *Backend) CreateEnumeration(enum *model.Enumeration) error {
	moduleName, folderPath, err := b.resolveDocContainer(enum.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve container for enumeration %q: %w", enum.Name, err)
	}
	if err := b.ensureSchema(enumerationDocType); err != nil {
		return err
	}
	if err := b.pedCreateDocument(moduleName, enumerationDocType, enum.Name, buildEnumContent(enum), folderPath); err != nil {
		return err
	}
	if enum.ID == "" {
		enum.ID = model.ID("mcp~enum~" + moduleName + "~" + enum.Name)
	}
	b.sessionEnums = append(b.sessionEnums, enum)
	return b.pedCheckDocument(enumerationDocType, moduleName+"."+enum.Name)
}

// UpdateEnumeration (CREATE OR MODIFY on an existing enumeration) is not yet
// supported by the MCP backend.
func (b *Backend) UpdateEnumeration(enum *model.Enumeration) error {
	return fmt.Errorf("modifying enumeration %q is not yet supported by the MCP backend (create a new one, or edit it in Studio Pro)", enum.Name)
}

// DeleteEnumeration drops an enumeration via Concord's delete_document (PED has
// no delete tool). Requires --mcp-concord; errors clearly otherwise.
func (b *Backend) DeleteEnumeration(id model.ID) error {
	enum, err := b.GetEnumeration(id)
	if err != nil {
		return fmt.Errorf("resolve enumeration %s for DROP: %w", id, err)
	}
	modName, err := b.moduleNameForContainer(enum.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for enumeration %q: %w", enum.Name, err)
	}
	if err := b.concordDeleteDocument(modName, enum.Name); err != nil {
		return err
	}
	b.forgetSessionEnum(modName, enum.Name)
	return nil
}

// forgetSessionEnum removes a session-created enumeration from the registry after
// it has been dropped, so later reads this run no longer surface it.
func (b *Backend) forgetSessionEnum(moduleName, name string) {
	out := b.sessionEnums[:0]
	for _, e := range b.sessionEnums {
		if mod, err := b.GetModule(e.ContainerID); err == nil && mod.Name == moduleName && e.Name == name {
			continue
		}
		out = append(out, e)
	}
	b.sessionEnums = out
}

// ListEnumerations returns enumerations from the local reader merged with any
// created over MCP this session (session entries take precedence by module+name).
func (b *Backend) ListEnumerations() ([]*model.Enumeration, error) {
	local, err := b.reader.ListEnumerations()
	if err != nil {
		return nil, err
	}
	if len(b.sessionEnums) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionEnums))
	out := make([]*model.Enumeration, 0, len(local)+len(b.sessionEnums))
	for _, e := range b.sessionEnums {
		seen[enumKey(e)] = true
		out = append(out, e)
	}
	for _, e := range local {
		if !seen[enumKey(e)] {
			out = append(out, e)
		}
	}
	return out, nil
}

// GetEnumeration resolves by ID, preferring session-created enumerations.
func (b *Backend) GetEnumeration(id model.ID) (*model.Enumeration, error) {
	for _, e := range b.sessionEnums {
		if e.ID == id {
			return e, nil
		}
	}
	return b.reader.GetEnumeration(id)
}

func enumKey(e *model.Enumeration) string {
	return string(e.ContainerID) + "." + e.Name
}

// buildEnumContent maps a model.Enumeration onto the PED enum $constructor
// ({name, values:[{name, caption}]}).
func buildEnumContent(enum *model.Enumeration) map[string]any {
	values := make([]map[string]any, 0, len(enum.Values))
	for _, v := range enum.Values {
		values = append(values, map[string]any{
			"name":    v.Name,
			"caption": enumCaption(v),
		})
	}
	return map[string]any{"name": enum.Name, "values": values}
}

// enumCaption returns the en_US caption, falling back to the value name.
func enumCaption(v model.EnumerationValue) string {
	if v.Caption != nil {
		if c := v.Caption.Translations["en_US"]; c != "" {
			return c
		}
	}
	return v.Name
}
