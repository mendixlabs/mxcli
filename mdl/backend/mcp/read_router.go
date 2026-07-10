// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// markDirty records that a module's live model has diverged from disk. Called
// after every successful PED write so subsequent reads of that module route to
// the live reconstruction instead of the stale local reader.
func (b *Backend) markDirty(moduleName string) {
	if b.dirty == nil {
		b.dirty = map[string]bool{}
	}
	b.dirty[moduleName] = true
}

func synthEntityID(module, name string) model.ID {
	return model.ID("mcp~ent~" + module + "~" + name)
}

func synthAssocID(module, name string) model.ID {
	return model.ID("mcp~assoc~" + module + "~" + name)
}

// syntheticName resolves a synthetic ID handed out by a reconstructed read back
// to its PED-addressable element name.
func (b *Backend) syntheticName(id model.ID) (string, bool) {
	n, ok := b.synthetic[id]
	return n, ok
}

func (b *Backend) registerSynthetic(id model.ID, name string) {
	if b.synthetic == nil {
		b.synthetic = map[model.ID]string{}
	}
	b.synthetic[id] = name
}

// pedIDRef matches a PED path reference like "$id(/entities/3)".
var pedIDRef = regexp.MustCompile(`^\$id\(/entities/(\d+)\)$`)

// reconstructDomainModelFromPED reads a module's live entities/associations from
// Studio Pro and builds a domain model with the given IDs. It is shared by the
// dirty existing-module path (real on-disk IDs) and the session-created-module
// path (synthetic IDs), the latter having no on-disk domain model for the reader.
//
// Fidelity note: PED reads expose attribute NAMES (from $QualifiedName) but not
// their primitive types — recovering a type costs a read per attribute. The
// shallow reconstruction uses placeholder types; enrichReconstructedEntities then
// fills in real types/documentation with one batched leaf read. This is enough
// for the operations routed through it (name-keyed resolution, ALTER add/drop,
// DROP); a DESCRIBE of an in-session-edited entity is faithful after enrichment.
func (b *Backend) reconstructDomainModelFromPED(moduleName string, dmID, containerID model.ID) (*domainmodel.DomainModel, error) {
	dm := &domainmodel.DomainModel{ContainerID: containerID}
	dm.ID = dmID

	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        []string{"/entities", "/associations"},
	})
	if err != nil {
		return nil, err
	}
	// A module created over MCP this session can briefly be unreadable
	// ("ERROR: Module ... not found", reported with isError:false per PED's
	// convention) right after ped_create_module — the same propagation lag the
	// write path retries on. Treat an unreadable module as having no entities yet
	// (empty domain model) rather than failing/parsing the error text as JSON;
	// the entity write itself retries until the module is mutable.
	text := pedStripReminder(res.Text)
	if res.IsError || strings.HasPrefix(strings.TrimSpace(text), "ERROR") {
		return dm, nil
	}

	var doc struct {
		Results []struct {
			Path   string          `json:"path"`
			Result json.RawMessage `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		// Non-JSON, non-ERROR body — treat as empty rather than crash the create.
		return dm, nil
	}

	var entityOrder []string // index -> entity synthetic ID (for $id(/entities/N) refs)

	for _, r := range doc.Results {
		switch r.Path {
		case "/entities":
			ents, order, err := b.reconstructEntities(moduleName, dm.ID, r.Result)
			if err != nil {
				return nil, err
			}
			dm.Entities = ents
			entityOrder = order
		case "/associations":
			assocs, err := b.reconstructAssociations(moduleName, r.Result, entityOrder)
			if err != nil {
				return nil, err
			}
			dm.Associations = assocs
		}
	}
	return dm, nil
}

func (b *Backend) reconstructEntities(moduleName string, dmID model.ID, raw json.RawMessage) ([]*domainmodel.Entity, []string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	var pedEntities []struct {
		Name       string `json:"name"`
		Attributes []struct {
			QualifiedName string `json:"$QualifiedName"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal(raw, &pedEntities); err != nil {
		return nil, nil, fmt.Errorf("reconstruct %s entities: %w", moduleName, err)
	}

	entities := make([]*domainmodel.Entity, 0, len(pedEntities))
	order := make([]string, 0, len(pedEntities))
	for _, pe := range pedEntities {
		id := synthEntityID(moduleName, pe.Name)
		b.registerSynthetic(id, pe.Name)

		e := &domainmodel.Entity{
			ContainerID: dmID,
			Name:        pe.Name,
			Persistable: true, // unknown via PED; default (and the slice only creates persistent)
		}
		e.ID = id
		for _, pa := range pe.Attributes {
			e.Attributes = append(e.Attributes, &domainmodel.Attribute{
				ContainerID: id,
				Name:        lastSegment(pa.QualifiedName),
				// Placeholder type — see fidelity note on reconstructDomainModelFromPED.
				Type: &domainmodel.StringAttributeType{},
			})
		}
		entities = append(entities, e)
		order = append(order, string(id))
	}
	// A shallow array read exposes only attribute names; the real primitive type
	// and documentation live behind per-leaf reads. Enrich them so a dirty/session
	// module reconstructs faithfully (correct DESCRIBE, and a reliable ALTER diff —
	// UpdateEntity must tell a genuine type change from an unchanged attribute).
	b.enrichReconstructedEntities(moduleName, entities)
	return entities, order, nil
}

// enrichReconstructedEntities fills in each attribute's real type + documentation
// and each entity's documentation from PED, using a single batched leaf read.
// Best-effort: on any failure the entities keep their placeholder String types
// (the same fidelity the reconstruction had before this enrichment).
func (b *Backend) enrichReconstructedEntities(moduleName string, entities []*domainmodel.Entity) {
	var paths []string
	for ei, e := range entities {
		paths = append(paths, fmt.Sprintf("/entities/%d/documentation", ei))
		for ai := range e.Attributes {
			paths = append(paths,
				fmt.Sprintf("/entities/%d/attributes/%d/type", ei, ai),
				fmt.Sprintf("/entities/%d/attributes/%d/documentation", ei, ai))
		}
	}
	if len(paths) == 0 || b.client == nil {
		return
	}
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        paths,
	})
	if err != nil || res.IsError {
		return
	}
	byPath, err := parsePedResults(pedStripReminder(res.Text))
	if err != nil {
		return
	}
	for ei, e := range entities {
		e.Documentation = jsonString(byPath[fmt.Sprintf("/entities/%d/documentation", ei)])
		for ai, a := range e.Attributes {
			if t := attributeTypeFromPED(byPath[fmt.Sprintf("/entities/%d/attributes/%d/type", ei, ai)]); t != nil {
				a.Type = t
			}
			a.Documentation = jsonString(byPath[fmt.Sprintf("/entities/%d/attributes/%d/documentation", ei, ai)])
		}
	}
}

// parsePedResults decodes a ped_read_document response body into a path->result map.
func parsePedResults(text string) (map[string]json.RawMessage, error) {
	var doc struct {
		Results []struct {
			Path   string          `json:"path"`
			Result json.RawMessage `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(doc.Results))
	for _, r := range doc.Results {
		out[r.Path] = r.Result
	}
	return out, nil
}

// jsonString decodes a JSON string value, returning "" for absent/non-string.
func jsonString(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// attributeTypeFromPED maps a PED attribute `type` leaf (a polymorphic
// DomainModels$*AttributeType constructor) back to a domainmodel attribute type.
// Returns nil for an absent or unrecognised constructor (caller keeps its default).
func attributeTypeFromPED(raw json.RawMessage) domainmodel.AttributeType {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var t struct {
		SType           string `json:"$Type"`
		Length          int    `json:"length"`
		Enumeration     string `json:"enumeration"`
		EnumerationName string `json:"enumerationName"`
	}
	if json.Unmarshal(raw, &t) != nil {
		return nil
	}
	switch t.SType {
	case "DomainModels$StringAttributeType":
		return &domainmodel.StringAttributeType{Length: t.Length}
	case "DomainModels$IntegerAttributeType":
		return &domainmodel.IntegerAttributeType{}
	case "DomainModels$LongAttributeType":
		return &domainmodel.LongAttributeType{}
	case "DomainModels$DecimalAttributeType":
		return &domainmodel.DecimalAttributeType{}
	case "DomainModels$BooleanAttributeType":
		return &domainmodel.BooleanAttributeType{}
	case "DomainModels$DateTimeAttributeType":
		return &domainmodel.DateTimeAttributeType{}
	case "DomainModels$AutoNumberAttributeType":
		return &domainmodel.AutoNumberAttributeType{}
	case "DomainModels$BinaryAttributeType":
		return &domainmodel.BinaryAttributeType{}
	case "DomainModels$HashedStringAttributeType":
		return &domainmodel.HashedStringAttributeType{}
	case "DomainModels$EnumerationAttributeType":
		ref := t.Enumeration
		if ref == "" {
			ref = t.EnumerationName
		}
		return &domainmodel.EnumerationAttributeType{EnumerationRef: ref}
	default:
		return nil
	}
}

func (b *Backend) reconstructAssociations(moduleName string, raw json.RawMessage, entityOrder []string) ([]*domainmodel.Association, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var pedAssocs []struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Owner  string `json:"owner"`
		Parent string `json:"parent"`
		Child  string `json:"child"`
	}
	if err := json.Unmarshal(raw, &pedAssocs); err != nil {
		return nil, fmt.Errorf("reconstruct %s associations: %w", moduleName, err)
	}

	assocs := make([]*domainmodel.Association, 0, len(pedAssocs))
	for _, pa := range pedAssocs {
		id := synthAssocID(moduleName, pa.Name)
		b.registerSynthetic(id, pa.Name)

		a := &domainmodel.Association{
			Name:     pa.Name,
			Type:     domainmodel.AssociationType(pa.Type),
			Owner:    domainmodel.AssociationOwner(pa.Owner),
			ParentID: resolveEntityRef(pa.Parent, entityOrder),
			ChildID:  resolveEntityRef(pa.Child, entityOrder),
		}
		a.ID = id
		assocs = append(assocs, a)
	}
	return assocs, nil
}

// resolveEntityRef turns a PED "$id(/entities/N)" reference into the synthetic
// ID of the N-th reconstructed entity.
func resolveEntityRef(ref string, entityOrder []string) model.ID {
	m := pedIDRef.FindStringSubmatch(ref)
	if m == nil {
		return ""
	}
	idx, err := strconv.Atoi(m[1])
	if err != nil || idx < 0 || idx >= len(entityOrder) {
		return ""
	}
	return model.ID(entityOrder[idx])
}

func lastSegment(qualifiedName string) string {
	if i := strings.LastIndex(qualifiedName, "."); i >= 0 {
		return qualifiedName[i+1:]
	}
	return qualifiedName
}
