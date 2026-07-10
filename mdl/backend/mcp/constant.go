// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

const constantDocType = "Constants$Constant"

// CreateConstant creates a constant document via ped_create_document and registers
// it in the session so ListConstants/GetConstant see it this run (CREATE OR MODIFY
// duplicate detection, and DROP resolving a just-created constant).
//
// PED's Constants$Constant constructor carries name/type/defaultValue/
// exposedToClient only — there is no documentation field, and the type enum is
// limited (see pedConstantType).
func (b *Backend) CreateConstant(c *model.Constant) error {
	moduleName, folderPath, err := b.resolveDocContainer(c.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve container for constant %q: %w", c.Name, err)
	}
	content, err := buildConstantContent(c)
	if err != nil {
		return err
	}
	if err := b.ensureSchema(constantDocType); err != nil {
		return err
	}
	if err := b.pedCreateDocument(moduleName, constantDocType, c.Name, content, folderPath); err != nil {
		return err
	}
	if c.ID == "" {
		c.ID = model.ID("mcp~const~" + moduleName + "~" + c.Name)
	}
	b.sessionConstants = append(b.sessionConstants, c)
	return b.pedCheckDocument(constantDocType, moduleName+"."+c.Name)
}

// UpdateConstant (CREATE OR MODIFY on an existing constant) sets the value and
// client exposure in place. The constant's `type`, despite being a plain enum in
// the *constructor*, is a nested `DataTypes$*Type` element in the model — PED can't
// set it directly (same as an attribute's type), so a type change is rejected
// rather than silently ignored.
func (b *Backend) UpdateConstant(c *model.Constant) error {
	mod, err := b.GetModule(c.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for constant %q: %w", c.Name, err)
	}
	pedType, err := pedConstantType(c.Type.Kind)
	if err != nil {
		return err
	}
	qn := mod.Name + "." + c.Name
	if live, err := b.liveConstantType(qn); err == nil && live != "" && live != pedType {
		return fmt.Errorf("constant %q: changing its type (%s -> %s) is not supported by the MCP backend — PED can't set the type element in place; drop and recreate, or edit in Studio Pro", c.Name, live, pedType)
	}
	ops := []pedOpEntry{
		{Path: "/defaultValue", Operation: pedOperation{Type: "set", Value: c.DefaultValue}},
		{Path: "/exposedToClient", Operation: pedOperation{Type: "set", Value: c.ExposedToClient}},
	}
	if err := b.pedUpdateDoc(constantDocType, qn, ops...); err != nil {
		return err
	}
	b.markDirty(mod.Name)
	b.upsertSessionConstant(c)
	return b.pedCheckDocument(constantDocType, qn)
}

// liveConstantType reads a constant's current type and maps the nested
// DataTypes$*Type element back to the constant type name ("" if unreadable, so a
// flaky read never manufactures a spurious type-change rejection).
func (b *Backend) liveConstantType(qn string) (string, error) {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": constantDocType,
		"documentName": qn,
		"paths":        []string{"/type"},
	})
	if err != nil {
		return "", err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return "", nil
	}
	var doc struct {
		Results []struct {
			Result struct {
				SType string `json:"$Type"`
			} `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil || len(doc.Results) == 0 {
		return "", nil
	}
	// "DataTypes$BooleanType" -> "Boolean"
	k := strings.TrimSuffix(strings.TrimPrefix(doc.Results[0].Result.SType, "DataTypes$"), "Type")
	return k, nil
}

// DeleteConstant drops a constant via Concord's delete_document (PED has no delete
// tool). Requires --mcp-concord; errors clearly otherwise.
func (b *Backend) DeleteConstant(id model.ID) error {
	c, err := b.GetConstant(id)
	if err != nil {
		return fmt.Errorf("resolve constant %s for DROP: %w", id, err)
	}
	modName, err := b.moduleNameForContainer(c.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for constant %q: %w", c.Name, err)
	}
	if err := b.concordDeleteDocument(modName, c.Name); err != nil {
		return err
	}
	b.forgetSessionConstant(modName, c.Name)
	return nil
}

// ListConstants returns constants from the local reader merged with any created
// over MCP this session (session entries take precedence by module+name).
func (b *Backend) ListConstants() ([]*model.Constant, error) {
	local, err := b.reader.ListConstants()
	if err != nil {
		return nil, err
	}
	if len(b.sessionConstants) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionConstants))
	out := make([]*model.Constant, 0, len(local)+len(b.sessionConstants))
	for _, c := range b.sessionConstants {
		seen[constKey(c)] = true
		out = append(out, c)
	}
	for _, c := range local {
		if !seen[constKey(c)] {
			out = append(out, c)
		}
	}
	return out, nil
}

// GetConstant resolves by ID, preferring session-created constants.
func (b *Backend) GetConstant(id model.ID) (*model.Constant, error) {
	for _, c := range b.sessionConstants {
		if c.ID == id {
			return c, nil
		}
	}
	return b.reader.GetConstant(id)
}

func (b *Backend) upsertSessionConstant(c *model.Constant) {
	for i, existing := range b.sessionConstants {
		if existing.ID == c.ID || constKey(existing) == constKey(c) {
			b.sessionConstants[i] = c
			return
		}
	}
	b.sessionConstants = append(b.sessionConstants, c)
}

func (b *Backend) forgetSessionConstant(moduleName, name string) {
	out := b.sessionConstants[:0]
	for _, c := range b.sessionConstants {
		if mod, err := b.GetModule(c.ContainerID); err == nil && mod.Name == moduleName && c.Name == name {
			continue
		}
		out = append(out, c)
	}
	b.sessionConstants = out
}

func constKey(c *model.Constant) string {
	return string(c.ContainerID) + "." + c.Name
}

// buildConstantContent maps a model.Constant onto the PED Constants$Constant
// $constructor ({name, type, defaultValue, exposedToClient}).
func buildConstantContent(c *model.Constant) (map[string]any, error) {
	pedType, err := pedConstantType(c.Type.Kind)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"name":            c.Name,
		"type":            pedType,
		"defaultValue":    c.DefaultValue,
		"exposedToClient": c.ExposedToClient,
	}, nil
}

// pedConstantType maps a constant's data-type kind onto PED's constant type enum.
// PED supports only these five primitives; Long/Enumeration/Binary (and anything
// else) are rejected rather than silently coerced.
func pedConstantType(kind string) (string, error) {
	switch kind {
	case "String", "Integer", "Decimal", "Boolean", "DateTime":
		return kind, nil
	case "Date":
		return "DateTime", nil // Mendix stores Date as DateTime
	default:
		return "", fmt.Errorf("constant type %q is not supported by the MCP backend (PED's constant type is one of String/Integer/Decimal/Boolean/DateTime)", kind)
	}
}
