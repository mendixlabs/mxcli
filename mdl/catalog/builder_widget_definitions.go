// SPDX-License-Identifier: Apache-2.0

// Builder for the widget_definitions + widget_definition_properties tables.
// Sources:
//   - Project widgets/*.mpk files (mpkDef.ID, Version, properties from XML)
//   - Project .mxcli/widgets/*.def.json files (MDL keyword routing —
//     childSlots, objectLists)
//   - Built-in definitions supplied by the caller via SetBuiltinWidgetMetas
//     (the hand-crafted COMBOBOX, GALLERY, DATAGRID, filter widgets that
//     ship inside the mxcli binary)
package catalog

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/sdk/widgets/mpk"
)

// WidgetDefinitionMeta is the catalog-level shape of a widget definition.
// Callers (cmd_catalog.go) adapt the executor's WidgetDefinition into this
// to avoid the mdl/catalog → mdl/executor import cycle.
type WidgetDefinitionMeta struct {
	WidgetId    string
	MdlName     string
	DisplayName string
	WidgetKind  string // "pluggable" | "custom" | "builtin"
	Version     string
	Properties  []WidgetDefinitionPropMeta
	ChildSlots  []WidgetDefinitionSlotMeta
	ObjectLists []WidgetDefinitionSlotMeta
}

// WidgetDefinitionPropMeta describes one scalar / datasource / attribute
// property of a widget definition.
type WidgetDefinitionPropMeta struct {
	PropertyKey  string
	Type         string // boolean, enumeration, datasource, attribute, ...
	Required     bool
	DefaultValue string
	Description  string
}

// WidgetDefinitionSlotMeta describes one child slot or object list (i.e. a
// container that takes an MDL keyword block). MdlKeyword is the singular
// uppercase form (TRIGGER, GROUP, ITEM, SERIES, …).
type WidgetDefinitionSlotMeta struct {
	PropertyKey string
	MdlKeyword  string
}

// SetBuiltinWidgetMetas registers the hand-crafted built-in widget
// definitions (COMBOBOX, GALLERY, DATAGRID, filters) that ship in the
// mxcli binary itself. The catalog builder writes one widget_definitions
// row per built-in alongside the project's .mpk-extracted widgets.
func (b *Builder) SetBuiltinWidgetMetas(metas []WidgetDefinitionMeta) {
	b.builtinWidgetMetas = metas
}

// buildWidgetDefinitions populates widget_definitions and
// widget_definition_properties from the project's widgets/ folder + matching
// .def.json files + caller-supplied built-in definitions.
func (b *Builder) buildWidgetDefinitions() error {
	defStmt, err := b.tx.Prepare(`
		INSERT INTO widget_definitions_data (
			WidgetId, MdlName, DisplayName, WidgetKind, Version,
			MpkPath, DefPath, PropertyCount, ChildSlotCount, ObjectListCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer defStmt.Close()

	propStmt, err := b.tx.Prepare(`
		INSERT INTO widget_definition_properties_data (
			Id, WidgetId, PropertyKey, Kind, Type, MdlKeyword,
			Required, DefaultValue, Description,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer propStmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	// Track widget IDs so a built-in def doesn't shadow a project .mpk and
	// vice-versa. Project widgets win when both exist (the user shipped one).
	seen := make(map[string]bool)
	var count int

	// 1. Project widgets — scan widgets/ for .mpk files, pair with .def.json
	if path := b.reader.Path(); path != "" {
		projectDir := filepath.Dir(path)
		widgetsDir := filepath.Join(projectDir, "widgets")
		defsDir := filepath.Join(projectDir, ".mxcli", "widgets")

		matches, err := filepath.Glob(filepath.Join(widgetsDir, "*.mpk"))
		if err != nil {
			return fmt.Errorf("scan widgets directory: %w", err)
		}
		for _, mpkPath := range matches {
			mpkDefs, err := mpk.ParseMPKAll(mpkPath)
			if err != nil {
				continue
			}
			// A single .mpk can bundle many widgets (e.g. Charts.mpk).
			for _, mpkDef := range mpkDefs {
				mdlName := strings.ToUpper(lastSegment(mpkDef.ID))
				defPath := filepath.Join(defsDir, strings.ToLower(mdlName)+".def.json")
				defFields := loadDefFields(defPath)
				kind := "custom"
				if mpkDef.IsPluggable {
					kind = "pluggable"
				}
				if err := insertWidgetDef(
					defStmt, propStmt,
					mpkDef.ID, mdlName, mpkDef.Name, kind, mpkDef.Version,
					relPath(projectDir, mpkPath), relPath(projectDir, defPath),
					mpkProperties(mpkDef), defFields.ChildSlots, defFields.ObjectLists,
					projectID, snapshotID,
				); err != nil {
					return err
				}
				seen[mpkDef.ID] = true
				count++
			}
		}
	}

	// 2. Built-in widget definitions supplied by the caller
	for _, meta := range b.builtinWidgetMetas {
		if seen[meta.WidgetId] {
			continue
		}
		if err := insertWidgetDef(
			defStmt, propStmt,
			meta.WidgetId, meta.MdlName, meta.DisplayName, "builtin", meta.Version,
			"", "",
			meta.Properties, meta.ChildSlots, meta.ObjectLists,
			projectID, snapshotID,
		); err != nil {
			return err
		}
		seen[meta.WidgetId] = true
		count++
	}

	if b.progress != nil {
		b.progress("widget_definitions", count)
	}
	return nil
}

// defFields holds the fields we care about from a .def.json file — just the
// MDL keyword routing, no operation/source detail.
type defFields struct {
	ChildSlots  []WidgetDefinitionSlotMeta
	ObjectLists []WidgetDefinitionSlotMeta
}

// loadDefFields reads a .def.json file and extracts its childSlots and
// objectLists. Returns empty values if the file is missing or malformed.
func loadDefFields(path string) defFields {
	var out defFields
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var parsed struct {
		ChildSlots []struct {
			PropertyKey  string `json:"propertyKey"`
			MDLContainer string `json:"mdlContainer"`
		} `json:"childSlots"`
		ObjectLists []struct {
			PropertyKey  string `json:"propertyKey"`
			MDLContainer string `json:"mdlContainer"`
		} `json:"objectLists"`
	}
	if jsonErr := json.Unmarshal(data, &parsed); jsonErr != nil {
		return out
	}
	for _, s := range parsed.ChildSlots {
		out.ChildSlots = append(out.ChildSlots, WidgetDefinitionSlotMeta{
			PropertyKey: s.PropertyKey,
			MdlKeyword:  s.MDLContainer,
		})
	}
	for _, ol := range parsed.ObjectLists {
		out.ObjectLists = append(out.ObjectLists, WidgetDefinitionSlotMeta{
			PropertyKey: ol.PropertyKey,
			MdlKeyword:  ol.MDLContainer,
		})
	}
	return out
}

// mpkProperties translates mpk.PropertyDef into the catalog's prop meta.
// Skips system properties (Mendix-internal).
func mpkProperties(mpkDef *mpk.WidgetDefinition) []WidgetDefinitionPropMeta {
	var out []WidgetDefinitionPropMeta
	for _, p := range mpkDef.Properties {
		if p.IsSystem {
			continue
		}
		out = append(out, WidgetDefinitionPropMeta{
			PropertyKey:  p.Key,
			Type:         p.Type,
			Required:     p.Required,
			DefaultValue: p.DefaultValue,
			Description:  p.Description,
		})
	}
	return out
}

// insertWidgetDef writes one widget_definitions row plus the matching
// widget_definition_properties rows for its properties, child slots, and
// object lists.
func insertWidgetDef(
	defStmt, propStmt *sql.Stmt,
	widgetID, mdlName, displayName, kind, version, mpkPath, defPath string,
	properties []WidgetDefinitionPropMeta,
	childSlots []WidgetDefinitionSlotMeta,
	objectLists []WidgetDefinitionSlotMeta,
	projectID, snapshotID string,
) error {
	if _, err := defStmt.Exec(
		widgetID, mdlName, displayName, kind, version,
		mpkPath, defPath, len(properties), len(childSlots), len(objectLists),
		projectID, snapshotID,
	); err != nil {
		return fmt.Errorf("insert widget_definitions: %w", err)
	}

	for _, p := range properties {
		req := 0
		if p.Required {
			req = 1
		}
		id := widgetPropID(widgetID, "property", p.PropertyKey)
		if _, err := propStmt.Exec(
			id, widgetID, p.PropertyKey, "property", p.Type, "",
			req, p.DefaultValue, p.Description,
			projectID, snapshotID,
		); err != nil {
			return fmt.Errorf("insert widget_definition_properties (property): %w", err)
		}
	}
	for _, s := range childSlots {
		id := widgetPropID(widgetID, "childSlot", s.PropertyKey)
		if _, err := propStmt.Exec(
			id, widgetID, s.PropertyKey, "childSlot", "widgets", s.MdlKeyword,
			0, "", "",
			projectID, snapshotID,
		); err != nil {
			return fmt.Errorf("insert widget_definition_properties (childSlot): %w", err)
		}
	}
	for _, ol := range objectLists {
		id := widgetPropID(widgetID, "objectList", ol.PropertyKey)
		if _, err := propStmt.Exec(
			id, widgetID, ol.PropertyKey, "objectList", "object", ol.MdlKeyword,
			0, "", "",
			projectID, snapshotID,
		); err != nil {
			return fmt.Errorf("insert widget_definition_properties (objectList): %w", err)
		}
	}
	return nil
}

// widgetPropID generates a deterministic primary key for a property row,
// derived from the widget ID + kind + property key so re-builds produce the
// same IDs (lets downstream tools track rows across catalog refreshes).
func widgetPropID(widgetID, kind, key string) string {
	h := sha1.Sum([]byte(widgetID + "/" + kind + "/" + key))
	return hex.EncodeToString(h[:])
}

func lastSegment(id string) string {
	parts := strings.Split(id, ".")
	return parts[len(parts)-1]
}

// relPath returns the path relative to base, or the original path if it
// doesn't share a common prefix. Empty input returns empty output.
func relPath(base, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
