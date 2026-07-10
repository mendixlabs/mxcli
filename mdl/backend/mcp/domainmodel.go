// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// domainModelDocType is the PED document type for a module's domain model.
// It is addressed by module name (documentName = module name only).
const domainModelDocType = "DomainModels$DomainModel"

// ---------------------------------------------------------------------------
// PED wire types
// ---------------------------------------------------------------------------

type pedPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type pedAttribute struct {
	SType           string `json:"$Type"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	EnumerationName string `json:"enumerationName,omitempty"`
	Value           any    `json:"value,omitempty"`
}

type pedEntity struct {
	SType          string         `json:"$Type"`
	Name           string         `json:"name"`
	Location       *pedPoint      `json:"location,omitempty"`
	Attributes     []pedAttribute `json:"attributes"`
	Generalization any            `json:"generalization,omitempty"`
	Source         any            `json:"source,omitempty"`
}

const oqlViewEntitySourceType = "DomainModels$OqlViewEntitySource"

type pedOperation struct {
	Type  string `json:"type"`            // "set" | "add" | "remove"
	Value any    `json:"value,omitempty"` // for set/add
	Index *int   `json:"index,omitempty"` // for add (optional) / remove (required)
}

type pedOpEntry struct {
	Path      string       `json:"path"`
	Operation pedOperation `json:"operation"`
}

// ---------------------------------------------------------------------------
// Entity operations
// ---------------------------------------------------------------------------

// CreateEntity adds an entity to a module's domain model via PED.
//
// Choreography (verified against Studio Pro 11.x):
//
//	ped_get_schema       (contract: fetch schema before add)
//	ped_update_document  (add the entity constructor at /entities)
//	ped_check_errors     (contract: validate after the final write)
func (b *Backend) CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	if err := b.gateAttributeDefaults(entity.Attributes); err != nil {
		return err
	}
	value, err := b.buildEntityValue(entity)
	if err != nil {
		return err
	}
	if err := b.ensureSchema("DomainModels$Entity", "DomainModels$Attribute"); err != nil {
		return err
	}
	// verify-on-timeout: an entity add is not idempotent (a blind re-run
	// duplicates the entity — observed live on 11.12), so on a server timeout
	// confirm via a shallow /entities read before failing a write that landed.
	if err := b.pedUpdateVerify(moduleName,
		fmt.Sprintf("entity %s.%s", moduleName, entity.Name),
		func() (bool, error) { return b.domainModelHasEntity(moduleName, entity.Name) },
		pedOpEntry{
			Path:      "/entities",
			Operation: pedOperation{Type: "add", Value: value},
		}); err != nil {
		return err
	}
	// Validation rules (NOT NULL / UNIQUE) are not part of the entity constructor
	// — PED's schema directs you to add them as separate DomainModels$ValidationRule
	// elements once the entity exists.
	if err := b.addValidationRules(moduleName, entity); err != nil {
		return err
	}
	// Default values aren't carried by the constructor either — set them via a
	// path-op now that the entity (and its StoredValues) exist (#641-area; 11.12+).
	entIdx, err := b.entityIndex(moduleName, entity.Name)
	if err != nil {
		return err
	}
	if err := b.applyAttributeDefaults(moduleName, entIdx, entity.Attributes); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// DeleteEntity removes an entity from a module's domain model via PED.
func (b *Backend) DeleteEntity(domainModelID model.ID, entityID model.ID) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	name, err := b.entityNameForID(domainModelID, entityID)
	if err != nil {
		return err
	}
	idx, err := b.entityIndex(moduleName, name)
	if err != nil {
		return err
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      "/entities",
		Operation: pedOperation{Type: "remove", Index: &idx},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// AddAttribute adds an attribute to an existing entity via PED.
func (b *Backend) AddAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	name, err := b.entityNameForID(domainModelID, entityID)
	if err != nil {
		return err
	}
	if err := b.gateAttributeDefaults([]*domainmodel.Attribute{attr}); err != nil {
		return err
	}
	idx, err := b.entityIndex(moduleName, name)
	if err != nil {
		return err
	}
	value, err := b.buildAttributeValue(attr)
	if err != nil {
		return err
	}
	if err := b.ensureSchema("DomainModels$Attribute"); err != nil {
		return err
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      fmt.Sprintf("/entities/%d/attributes", idx),
		Operation: pedOperation{Type: "add", Value: value},
	}); err != nil {
		return err
	}
	if err := b.applyAttributeDefaults(moduleName, idx, []*domainmodel.Attribute{attr}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// UpdateEntity applies an ALTER ENTITY change by diffing the incoming entity
// against the live one (read over MCP). The executor rebuilds the whole entity
// for every ALTER, so the diff is how we recover "what changed". The shape of
// the name-keyed diff selects the operation:
//
//   - adds only / removes only  -> ADD / DROP ATTRIBUTE (granular array ops)
//   - one add + one remove      -> RENAME ATTRIBUTE (set the `name` leaf in place)
//   - no structural change      -> in-place: entity/attribute documentation;
//     an attribute *type* change is rejected (PED cannot reassign a type in
//     place, and recreating the attribute would drop its column data)
//   - many adds + many removes  -> rejected (can't tell renames from drop+add)
//
// Reliable type-change detection relies on the reconstructed entity carrying
// real attribute types (see enrichReconstructedEntities), so a documentation
// edit on a dirty module is not mistaken for a type change.
func (b *Backend) UpdateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	if err := guardUnsupportedEntityFeatures(entity); err != nil {
		return err
	}
	if err := b.gateAttributeDefaults(entity.Attributes); err != nil {
		return err
	}
	entIdx, err := b.entityIndex(moduleName, entity.Name)
	if err != nil {
		return err
	}
	liveNames, err := b.liveAttributeNames(moduleName, entIdx)
	if err != nil {
		return err
	}

	liveSet := toStringSet(liveNames)
	incomingSet := map[string]bool{}
	var toAdd []*domainmodel.Attribute
	for _, a := range entity.Attributes {
		incomingSet[a.Name] = true
		if !liveSet[a.Name] {
			toAdd = append(toAdd, a)
		}
	}
	var toRemove []string
	for _, n := range liveNames {
		if !incomingSet[n] {
			toRemove = append(toRemove, n)
		}
	}

	// Validation rules: a rule on an attribute that is ALREADY live is pre-existing
	// (already in Studio Pro, untouched by an attribute add/rename/drop) — let it
	// pass. A rule on a NOT-yet-live attribute would have to be authored by this
	// ALTER, which the entity slice can't do; reject that specific case rather than
	// blocking every ALTER on a constrained entity (the Issue 6 over-rejection).
	nameByAttrID := make(map[model.ID]string, len(entity.Attributes))
	for _, a := range entity.Attributes {
		nameByAttrID[a.ID] = a.Name
	}
	for _, vr := range entity.ValidationRules {
		if name, ok := nameByAttrID[vr.AttributeID]; ok && !liveSet[name] {
			return unsupportedEntityFeature(entity.Name, "adding NOT NULL / UNIQUE on a new attribute via ALTER")
		}
	}

	switch {
	case len(toAdd) == 1 && len(toRemove) == 1:
		// Exactly one name appeared and one disappeared: a rename. (A type change
		// keeps the name, so it never lands here.) Set the name leaf in place,
		// which preserves the attribute's $ID — and therefore its column data.
		return b.renameAttribute(moduleName, entIdx, liveNames, toRemove[0], toAdd[0].Name)
	case len(toAdd) > 0 && len(toRemove) > 0:
		return fmt.Errorf("entity %q: this change adds and removes several attributes at once, which the MCP backend cannot apply safely (it cannot tell a rename from a drop-and-add); rename one attribute at a time", entity.Name)
	case len(toAdd) == 0 && len(toRemove) == 0:
		// No structural change: documentation edit, a (rejected) type change, or a
		// no-op. Reconcile in place against the live model.
		return b.applyInPlaceEntityChanges(moduleName, entIdx, entity, liveNames)
	}

	if len(toAdd) > 0 {
		if err := b.ensureSchema("DomainModels$Attribute"); err != nil {
			return err
		}
		ops := make([]pedOpEntry, 0, len(toAdd))
		for _, a := range toAdd {
			value, err := b.buildAttributeValue(a)
			if err != nil {
				return err
			}
			ops = append(ops, pedOpEntry{
				Path:      fmt.Sprintf("/entities/%d/attributes", entIdx),
				Operation: pedOperation{Type: "add", Value: value},
			})
		}
		if err := b.pedUpdate(moduleName, ops...); err != nil {
			return err
		}
		// Set defaults on the just-added attributes (constructor can't carry them).
		if err := b.applyAttributeDefaults(moduleName, entIdx, toAdd); err != nil {
			return err
		}
	}

	if len(toRemove) > 0 {
		removeSet := toStringSet(toRemove)
		var idxs []int
		for i, n := range liveNames {
			if removeSet[n] {
				idxs = append(idxs, i)
			}
		}
		// Remove highest index first so earlier indices stay valid.
		ops := make([]pedOpEntry, 0, len(idxs))
		for j := len(idxs) - 1; j >= 0; j-- {
			idx := idxs[j]
			ops = append(ops, pedOpEntry{
				Path:      fmt.Sprintf("/entities/%d/attributes", entIdx),
				Operation: pedOperation{Type: "remove", Index: &idx},
			})
		}
		if err := b.pedUpdate(moduleName, ops...); err != nil {
			return err
		}
	}

	return b.pedCheckErrors(moduleName)
}

// renameAttribute renames an attribute in place by setting its `name` leaf — a
// primitive property, which is one of the few things PED's update API permits
// setting directly. This preserves the attribute element (its $ID), so column
// data survives, unlike a drop-and-add.
func (b *Backend) renameAttribute(moduleName string, entIdx int, liveNames []string, oldName, newName string) error {
	idx := -1
	for i, n := range liveNames {
		if n == oldName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("rename attribute: %q not found on entity (index %d) in module %q", oldName, entIdx, moduleName)
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      fmt.Sprintf("/entities/%d/attributes/%d/name", entIdx, idx),
		Operation: pedOperation{Type: "set", Value: newName},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// liveAttr is the live state of one attribute relevant to an in-place ALTER.
type liveAttr struct {
	typ domainmodel.AttributeType
	doc string
}

// applyInPlaceEntityChanges reconciles an ALTER that neither adds nor drops
// attributes. PED only lets us set primitive/reference properties directly, so
// this supports entity documentation and attribute documentation. A genuine
// attribute *type* change is rejected rather than silently dropped: PED cannot
// reassign an attribute's type element in place, and emulating it via drop-and-add
// would discard the attribute's $ID — and its column data.
func (b *Backend) applyInPlaceEntityChanges(moduleName string, entIdx int, entity *domainmodel.Entity, liveNames []string) error {
	entDoc, attrs, err := b.liveEntityDetails(moduleName, entIdx, len(liveNames))
	if err != nil {
		return err
	}
	nameToIdx := make(map[string]int, len(liveNames))
	for i, n := range liveNames {
		nameToIdx[n] = i
	}

	var ops []pedOpEntry
	if entity.Documentation != entDoc {
		ops = append(ops, pedOpEntry{
			Path:      fmt.Sprintf("/entities/%d/documentation", entIdx),
			Operation: pedOperation{Type: "set", Value: entity.Documentation},
		})
	}
	for _, a := range entity.Attributes {
		idx, ok := nameToIdx[a.Name]
		if !ok {
			continue // no adds in this path; a name we don't know is unexpected
		}
		if !sameAttributeType(a.Type, attrs[idx].typ) {
			return fmt.Errorf("entity %q attribute %q: changing an attribute's type is not possible via the MCP backend — Studio Pro's MCP server cannot reassign a type in place, and recreating the attribute would drop its column data; run this ALTER against a local .mpr instead", entity.Name, a.Name)
		}
		if a.Documentation != attrs[idx].doc {
			ops = append(ops, pedOpEntry{
				Path:      fmt.Sprintf("/entities/%d/attributes/%d/documentation", entIdx, idx),
				Operation: pedOperation{Type: "set", Value: a.Documentation},
			})
		}
	}
	// Attribute default values change in place too (the common case: set a default
	// on an existing attribute). Handled via applyAttributeDefaults, which reads the
	// live defaults and only sets the ones that differ.
	hasDefaults := false
	for _, a := range entity.Attributes {
		if _, ok := pedAttributeDefault(a); ok {
			hasDefaults = true
			break
		}
	}
	if len(ops) == 0 && !hasDefaults {
		return nil // nothing changed (idempotent)
	}
	if len(ops) > 0 {
		if err := b.pedUpdate(moduleName, ops...); err != nil {
			return err
		}
	}
	if err := b.applyAttributeDefaults(moduleName, entIdx, entity.Attributes); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// liveEntityDetails reads an entity's documentation and each attribute's type +
// documentation from PED in a single batched leaf read.
func (b *Backend) liveEntityDetails(moduleName string, entIdx, attrCount int) (string, []liveAttr, error) {
	paths := []string{fmt.Sprintf("/entities/%d/documentation", entIdx)}
	for i := range attrCount {
		paths = append(paths,
			fmt.Sprintf("/entities/%d/attributes/%d/type", entIdx, i),
			fmt.Sprintf("/entities/%d/attributes/%d/documentation", entIdx, i))
	}
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        paths,
	})
	if err != nil {
		return "", nil, err
	}
	if res.IsError {
		return "", nil, fmt.Errorf("ped_read_document %s entity %d: %s", moduleName, entIdx, pedStripReminder(res.Text))
	}
	byPath, err := parsePedResults(pedStripReminder(res.Text))
	if err != nil {
		return "", nil, fmt.Errorf("parse entity %d of %s: %w", entIdx, moduleName, err)
	}
	attrs := make([]liveAttr, attrCount)
	for i := range attrCount {
		attrs[i] = liveAttr{
			typ: attributeTypeFromPED(byPath[fmt.Sprintf("/entities/%d/attributes/%d/type", entIdx, i)]),
			doc: jsonString(byPath[fmt.Sprintf("/entities/%d/attributes/%d/documentation", entIdx, i)]),
		}
	}
	return jsonString(byPath[fmt.Sprintf("/entities/%d/documentation", entIdx)]), attrs, nil
}

// sameAttributeType reports whether two attribute types are equivalent for the
// purpose of detecting an ALTER type change. Unknown types (nil) compare equal
// so a failed type read never manufactures a spurious "type change" rejection.
func sameAttributeType(incoming, live domainmodel.AttributeType) bool {
	if incoming == nil || live == nil {
		return true
	}
	in, e1 := pedAttributeType(incoming.GetTypeName())
	lv, e2 := pedAttributeType(live.GetTypeName())
	if e1 != nil || e2 != nil {
		return true // a type we can't name -> don't manufacture a type-change
	}
	if in != lv {
		return false
	}
	ie, iok := incoming.(*domainmodel.EnumerationAttributeType)
	le, lok := live.(*domainmodel.EnumerationAttributeType)
	if iok && lok {
		return ie.EnumerationRef == le.EnumerationRef
	}
	return true
}

// liveAttributeNames returns the ordered attribute names of an entity from the
// live model (names parsed from $QualifiedName, which is all a PED read of an
// attribute array exposes).
func (b *Backend) liveAttributeNames(moduleName string, entIdx int) ([]string, error) {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        []string{fmt.Sprintf("/entities/%d/attributes", entIdx)},
	})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, fmt.Errorf("ped_read_document %s entity %d attributes: %s", moduleName, entIdx, res.Text)
	}
	var doc struct {
		Results []struct {
			Result []struct {
				QualifiedName string `json:"$QualifiedName"`
			} `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(res.Text), &doc); err != nil {
		return nil, fmt.Errorf("parse attributes of %s entity %d: %w", moduleName, entIdx, err)
	}
	if len(doc.Results) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(doc.Results[0].Result))
	for _, a := range doc.Results[0].Result {
		names = append(names, lastSegment(a.QualifiedName))
	}
	return names, nil
}

func toStringSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// ---------------------------------------------------------------------------
// Association operations
// ---------------------------------------------------------------------------

// pedAssociation is the PED $constructor shape for an association. parentEntity
// and childEntity are entity GUIDs ($ID). For a project Studio Pro has open
// from the same .mpr, those GUIDs equal the local reader's entity IDs — which
// is exactly what the executor passes as assoc.ParentID / assoc.ChildID.
type pedAssociation struct {
	SType        string `json:"$Type"`
	Name         string `json:"name"`
	ParentEntity string `json:"parentEntity"` // FROM / owner (stores the reference)
	ChildEntity  string `json:"childEntity"`  // TO / referenced
	Multiplicity string `json:"multiplicity"`
}

// CreateAssociation adds a within-module association via PED.
//
// Mapping (note Mendix's inverted parent/child naming — see CLAUDE.md):
//   - assoc.ParentID (FROM, FK owner)  -> parentEntity
//   - assoc.ChildID  (TO, referenced)  -> childEntity
//   - Type/Owner                       -> multiplicity
func (b *Backend) CreateAssociation(domainModelID model.ID, assoc *domainmodel.Association) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	if assoc.ParentID == "" || assoc.ChildID == "" {
		return fmt.Errorf("association %q: missing parent/child entity id", assoc.Name)
	}
	if err := guardAssociationFeatures(assoc); err != nil {
		return err
	}
	mult, err := associationMultiplicity(assoc)
	if err != nil {
		return err
	}
	if err := b.ensureSchema("DomainModels$Association"); err != nil {
		return err
	}
	// parentEntity/childEntity are by-id references. PED accepts the $id(/entities/N)
	// path-reference form it also emits on read — use that, since a session-created
	// entity has no on-disk GUID (its model ID is a synthetic mcp~ent~… placeholder).
	parentRef, err := b.pedEntityRef(domainModelID, moduleName, assoc.ParentID)
	if err != nil {
		return fmt.Errorf("association %q: parent entity: %w", assoc.Name, err)
	}
	childRef, err := b.pedEntityRef(domainModelID, moduleName, assoc.ChildID)
	if err != nil {
		return fmt.Errorf("association %q: child entity: %w", assoc.Name, err)
	}
	value := pedAssociation{
		SType:        "DomainModels$Association",
		Name:         assoc.Name,
		ParentEntity: parentRef,
		ChildEntity:  childRef,
		Multiplicity: mult,
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      "/associations",
		Operation: pedOperation{Type: "add", Value: value},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// DeleteAssociation removes a within-module association via PED.
func (b *Backend) DeleteAssociation(domainModelID model.ID, assocID model.ID) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	name, err := b.associationNameForID(domainModelID, assocID)
	if err != nil {
		return err
	}
	idx, err := b.arrayElementIndex(moduleName, "/associations", "association", name)
	if err != nil {
		return err
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      "/associations",
		Operation: pedOperation{Type: "remove", Index: &idx},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// associationMultiplicity maps a domain-model association's Type/Owner onto the
// PED multiplicity enum. The "one" side is always the parent (FROM) entity.
func associationMultiplicity(a *domainmodel.Association) (string, error) {
	switch a.Type {
	case domainmodel.AssociationTypeReferenceSet:
		return "many_to_many", nil
	case domainmodel.AssociationTypeReference:
		if a.Owner == domainmodel.AssociationOwnerBoth {
			return "one_to_one", nil
		}
		return "one_to_many", nil
	default:
		return "", fmt.Errorf("association %q: unsupported type %q for the MCP backend", a.Name, a.Type)
	}
}

// guardAssociationFeatures rejects association settings the PED constructor
// cannot express, rather than silently applying PED defaults. The constructor
// covers name/parent/child/multiplicity only; delete behavior and storage
// format fall back to Studio Pro's defaults (keep-references, column), so a
// non-default request must be refused.
func guardAssociationFeatures(a *domainmodel.Association) error {
	defaultDelete := domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	for _, db := range []*domainmodel.DeleteBehavior{a.ChildDeleteBehavior, a.ParentDeleteBehavior} {
		if db != nil && db.Type != "" && db.Type != defaultDelete {
			return fmt.Errorf("association %q: custom delete behavior (%s) is not yet supported by the MCP backend", a.Name, db.Type)
		}
	}
	if a.Source != "" {
		return fmt.Errorf("association %q: external/OData associations are not supported by the MCP backend", a.Name)
	}
	return nil
}

// associationNameForID resolves an association's name from its ID. Synthetic
// IDs from a reconstructed (dirty) read resolve through the synthetic map;
// saved associations resolve via the local reader.
func (b *Backend) associationNameForID(domainModelID, assocID model.ID) (string, error) {
	if name, ok := b.syntheticName(assocID); ok {
		return name, nil
	}
	dm, err := b.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return "", fmt.Errorf("resolve domain model %s: %w", domainModelID, err)
	}
	for _, a := range dm.Associations {
		if a.ID == assocID {
			return a.Name, nil
		}
	}
	return "", fmt.Errorf("association %s not found in domain model %s", assocID, domainModelID)
}

// ---------------------------------------------------------------------------
// Value builders + feature guards
// ---------------------------------------------------------------------------

// buildEntityValue maps a domain-model Entity onto the PED $constructor shape.
// The constructor is deliberately simple (name, attributes, location,
// generalization). Features it cannot express are rejected with a clear error
// rather than silently dropped — except a Boolean's auto-added `false` default,
// which is Mendix's own default and carries no information.
func (b *Backend) buildEntityValue(entity *domainmodel.Entity) (*pedEntity, error) {
	if err := guardUnsupportedEntityFeatures(entity); err != nil {
		return nil, err
	}

	pe := &pedEntity{
		SType:      "DomainModels$Entity",
		Name:       entity.Name,
		Location:   &pedPoint{X: entity.Location.X, Y: entity.Location.Y},
		Attributes: []pedAttribute{},
	}
	if entity.GeneralizationRef != "" {
		pe.Generalization = entity.GeneralizationRef // by-name reference (e.g. "System.User")
	}
	// View entity: link to its OQL source document (created separately via
	// CreateViewEntitySourceDocument). The reference is by qualified name.
	if entity.Source == oqlViewEntitySourceType {
		if entity.SourceDocumentRef == "" {
			return nil, fmt.Errorf("view entity %q: missing source document reference", entity.Name)
		}
		pe.Source = map[string]any{
			"$Type":          oqlViewEntitySourceType,
			"sourceDocument": entity.SourceDocumentRef,
		}
	}
	isView := entity.Source == oqlViewEntitySourceType
	for _, a := range entity.Attributes {
		pa, err := b.buildAttributeValue(a)
		if err != nil {
			return nil, err
		}
		if isView {
			// A view entity's attributes are sourced from OQL columns; each
			// needs an OqlViewValue whose reference is the column alias (the
			// executor stores it in Value.ViewReference, defaulting to the
			// attribute name). Without it the entity is "out of sync" (CE-6770).
			ref := a.Name
			if a.Value != nil && a.Value.ViewReference != "" {
				ref = a.Value.ViewReference
			}
			pa.Value = map[string]any{"$Type": "DomainModels$OqlViewValue", "reference": ref}
		}
		pe.Attributes = append(pe.Attributes, *pa)
	}
	return pe, nil
}

func (b *Backend) buildAttributeValue(a *domainmodel.Attribute) (*pedAttribute, error) {
	if a.Type == nil {
		return nil, fmt.Errorf("attribute %q: missing type", a.Name)
	}
	typeName := a.Type.GetTypeName()
	pedType, err := pedAttributeType(typeName)
	if err != nil {
		return nil, fmt.Errorf("attribute %q: %w", a.Name, err)
	}
	if err := guardAttributeValue(a); err != nil {
		return nil, err
	}
	pa := &pedAttribute{SType: "DomainModels$Attribute", Name: a.Name, Type: pedType}
	if et, ok := a.Type.(*domainmodel.EnumerationAttributeType); ok {
		pa.EnumerationName = et.EnumerationRef
	}
	return pa, nil
}

// guardAttributeValue rejects attribute value settings the PED constructor
// cannot carry. Calculated attributes are unsupported. Default values ARE
// supported (Studio Pro 11.12+), but the constructor can't carry them — they are
// applied as a follow-up ped_update_document path-op (see applyAttributeDefaults);
// the version gate lives in gateAttributeDefaults, called up-front by each write.
func guardAttributeValue(a *domainmodel.Attribute) error {
	if a.Value == nil {
		return nil
	}
	if a.Value.Type == "CalculatedValue" {
		return fmt.Errorf("attribute %q: calculated attributes are not yet supported by the MCP backend", a.Name)
	}
	return nil
}

// pedAttributeDefault returns the value PED stores for an attribute's default,
// and whether a default should be set at all. PED's DomainModels$StoredValue
// stores the BARE value verbatim — including for enums, where it keeps just the
// value name (e.g. "Draft", not "MES.WorkOrderStatus.Draft"), exactly matching
// what the executor puts in DefaultValue. Verified live on Studio Pro 11.12:
// setting the bare value succeeds, reads back bare, and passes ped_check_errors.
// A Boolean's `false` is treated as no default (it is Mendix's own implicit
// default); calculated/empty values carry no settable default.
func pedAttributeDefault(a *domainmodel.Attribute) (string, bool) {
	if a.Value == nil || a.Value.Type == "CalculatedValue" || a.Value.DefaultValue == "" {
		return "", false
	}
	v := a.Value.DefaultValue
	if _, ok := a.Type.(*domainmodel.BooleanAttributeType); ok && strings.EqualFold(v, "false") {
		return "", false
	}
	return v, true
}

// gateAttributeDefaults rejects (up-front, before any write) setting an attribute
// default on a Studio Pro older than 11.12, whose PED server lacks the
// value/defaultValue path-op. Keeps the failure clean instead of letting the
// path-op error mid-write.
func (b *Backend) gateAttributeDefaults(attrs []*domainmodel.Attribute) error {
	var pv *types.ProjectVersion
	if b.reader != nil {
		pv = b.ProjectVersion()
	}
	if attributeDefaultsSupported(pv) {
		return nil
	}
	for _, a := range attrs {
		if _, ok := pedAttributeDefault(a); ok {
			return fmt.Errorf("attribute %q: setting a default value via the MCP backend requires Studio Pro 11.12+ "+
				"(the PED defaultValue path-op); run this against a local .mpr instead", a.Name)
		}
	}
	return nil
}

// attributeDefaultsSupported reports whether the connected Studio Pro is new
// enough for the PED value/defaultValue path-op (11.12+).
func attributeDefaultsSupported(pv *types.ProjectVersion) bool {
	return pv != nil && pv.IsAtLeast(11, 12)
}

// liveAttributeDefaults reads each attribute's current StoredValue.defaultValue
// (empty string when unset, or for a calculated/view value), indexed by attribute
// position. It reads the whole `value` object — which always exists — and extracts
// defaultValue, rather than the leaf (which may be absent when unset, and a
// missing-leaf read can error).
func (b *Backend) liveAttributeDefaults(moduleName string, entIdx, attrCount int) ([]string, error) {
	if attrCount == 0 {
		return nil, nil
	}
	paths := make([]string, 0, attrCount)
	for i := range attrCount {
		paths = append(paths, fmt.Sprintf("/entities/%d/attributes/%d/value", entIdx, i))
	}
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        paths,
	})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, fmt.Errorf("ped_read_document %s defaults: %s", moduleName, pedStripReminder(res.Text))
	}
	byPath, err := parsePedResults(pedStripReminder(res.Text))
	if err != nil {
		return nil, fmt.Errorf("parse defaults of %s: %w", moduleName, err)
	}
	out := make([]string, attrCount)
	for i := range attrCount {
		var v struct {
			DefaultValue string `json:"defaultValue"`
		}
		_ = json.Unmarshal(byPath[fmt.Sprintf("/entities/%d/attributes/%d/value", entIdx, i)], &v)
		out[i] = v.DefaultValue
	}
	return out, nil
}

// applyAttributeDefaults sets DomainModels$StoredValue.defaultValue for each
// attribute whose desired default differs from the live model, via
// ped_update_document `set` path-ops (the create constructor can't carry it). It
// re-reads the live model to resolve attribute array indices and skip values that
// are already correct, so it is idempotent and works for both freshly-created and
// existing attributes. Requires Studio Pro 11.12+ (gated up-front by
// gateAttributeDefaults).
func (b *Backend) applyAttributeDefaults(moduleName string, entIdx int, attrs []*domainmodel.Attribute) error {
	want := make(map[string]string)
	for _, a := range attrs {
		if v, ok := pedAttributeDefault(a); ok {
			want[a.Name] = v
		}
	}
	if len(want) == 0 {
		return nil
	}
	liveNames, err := b.liveAttributeNames(moduleName, entIdx)
	if err != nil {
		return err
	}
	liveDefaults, err := b.liveAttributeDefaults(moduleName, entIdx, len(liveNames))
	if err != nil {
		return err
	}
	idxByName := make(map[string]int, len(liveNames))
	for i, n := range liveNames {
		idxByName[n] = i
	}
	var ops []pedOpEntry
	for name, v := range want {
		i, ok := idxByName[name]
		if !ok {
			return fmt.Errorf("attribute %q not found in live model when setting its default", name)
		}
		if i < len(liveDefaults) && liveDefaults[i] == v {
			continue // already set
		}
		ops = append(ops, pedOpEntry{
			Path:      fmt.Sprintf("/entities/%d/attributes/%d/value/defaultValue", entIdx, i),
			Operation: pedOperation{Type: "set", Value: v},
		})
	}
	if len(ops) == 0 {
		return nil
	}
	return b.pedUpdate(moduleName, ops...)
}

// pedAttributeType maps a domain-model attribute type name onto the PED
// constructor's `type` enum.
func pedAttributeType(name string) (string, error) {
	switch name {
	case "String", "Integer", "Long", "Decimal", "Boolean", "DateTime",
		"AutoNumber", "Binary", "HashedString", "Enumeration":
		return name, nil
	case "Date":
		return "DateTime", nil // Mendix stores Date as a DateTime
	default:
		return "", fmt.Errorf("attribute type %q is not supported by the MCP backend", name)
	}
}

// guardUnsupportedEntityFeatures rejects entity-level constructs the PED entity
// path cannot express. Shared by create (buildEntityValue) and the ALTER diff
// (UpdateEntity) so both refuse the same things instead of silently dropping.
func guardUnsupportedEntityFeatures(entity *domainmodel.Entity) error {
	if !entity.Persistable {
		return unsupportedEntityFeature(entity.Name, "non-persistent entities")
	}
	if len(entity.Indexes) > 0 {
		return unsupportedEntityFeature(entity.Name, "indexes")
	}
	if len(entity.EventHandlers) > 0 {
		return unsupportedEntityFeature(entity.Name, "event handlers")
	}
	if entity.HasOwner || entity.HasChangedBy || entity.HasCreatedDate || entity.HasChangedDate {
		return unsupportedEntityFeature(entity.Name, "system members (owner/changedBy/createdDate/changedDate)")
	}
	return nil
}

func unsupportedEntityFeature(entityName, feature string) error {
	return fmt.Errorf("entity %q: %s are not yet supported by the MCP backend (entity slice); create it against a local .mpr instead", entityName, feature)
}

// pedValidationRule is a DomainModels$ValidationRule $element added to an entity's
// /validationRules. PED's entity constructor cannot carry rules (its schema says
// to add them separately), and the target attribute is referenced by qualified
// name (Module.Entity.Attribute), not by ID.
type pedValidationRule struct {
	SType        string       `json:"$Type"`
	Attribute    string       `json:"attribute"`
	ErrorMessage *pedText     `json:"errorMessage,omitempty"`
	RuleInfo     *pedRuleInfo `json:"ruleInfo"`
}

type pedText struct {
	SType string `json:"$Type"`
	Text  string `json:"text"`
}

type pedRuleInfo struct {
	SType string `json:"$Type"`
}

// addValidationRules adds NOT NULL (Required) and UNIQUE validation rules to a
// just-created entity. They are separate $element adds to /entities/<idx>/
// validationRules, applied after the entity exists (the freshly created entity's
// rule array starts empty, so an index-less add appends).
func (b *Backend) addValidationRules(moduleName string, entity *domainmodel.Entity) error {
	if len(entity.ValidationRules) == 0 {
		return nil
	}
	if err := b.ensureSchema("DomainModels$ValidationRule"); err != nil {
		return err
	}
	nameByID := make(map[model.ID]string, len(entity.Attributes))
	for _, a := range entity.Attributes {
		nameByID[a.ID] = a.Name
	}
	entIdx, err := b.entityIndex(moduleName, entity.Name)
	if err != nil {
		return err
	}
	ops := make([]pedOpEntry, 0, len(entity.ValidationRules))
	for _, vr := range entity.ValidationRules {
		attrName, ok := nameByID[vr.AttributeID]
		if !ok {
			return fmt.Errorf("entity %q: validation rule references unknown attribute %s", entity.Name, vr.AttributeID)
		}
		ruleInfoType, err := pedRuleInfoType(vr.Type)
		if err != nil {
			return fmt.Errorf("entity %q attribute %q: %w", entity.Name, attrName, err)
		}
		rule := &pedValidationRule{
			SType:     "DomainModels$ValidationRule",
			Attribute: moduleName + "." + entity.Name + "." + attrName,
			RuleInfo:  &pedRuleInfo{SType: ruleInfoType},
		}
		if msg := validationErrorText(vr.ErrorMessage); msg != "" {
			rule.ErrorMessage = &pedText{SType: "Texts$Text", Text: msg}
		}
		ops = append(ops, pedOpEntry{
			Path:      fmt.Sprintf("/entities/%d/validationRules", entIdx),
			Operation: pedOperation{Type: "add", Value: rule},
		})
	}
	return b.pedUpdate(moduleName, ops...)
}

// pedRuleInfoType maps a domain-model validation rule type onto PED's RuleInfo
// element type. Only NOT NULL (Required) and UNIQUE are authorable today.
func pedRuleInfoType(ruleType string) (string, error) {
	switch ruleType {
	case "Required":
		return "DomainModels$RequiredRuleInfo", nil
	case "Unique":
		return "DomainModels$UniqueRuleInfo", nil
	default:
		return "", fmt.Errorf("validation rule %q is not yet supported by the MCP backend (only NOT NULL / UNIQUE)", ruleType)
	}
}

// validationErrorText extracts a validation rule's error message, preferring the
// en_US translation and otherwise the lexicographically first (deterministic).
func validationErrorText(t *model.Text) string {
	if t == nil {
		return ""
	}
	if v, ok := t.Translations["en_US"]; ok {
		return v
	}
	keys := make([]string, 0, len(t.Translations))
	for k := range t.Translations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return t.Translations[keys[0]]
	}
	return ""
}

// ---------------------------------------------------------------------------
// PED helpers
// ---------------------------------------------------------------------------

// moduleNameForDomainModel resolves the PED documentName (the module name) for
// a domain-model ID using the local reader.
func (b *Backend) moduleNameForDomainModel(domainModelID model.ID) (string, error) {
	// Synthetic domain model for a session-created module (see GetDomainModel):
	// the module name is encoded in the ID.
	if name, ok := strings.CutPrefix(string(domainModelID), sessionDMPrefix); ok {
		return name, nil
	}
	dm, err := b.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return "", fmt.Errorf("resolve domain model %s: %w", domainModelID, err)
	}
	mod, err := b.reader.GetModule(dm.ContainerID)
	if err != nil {
		return "", fmt.Errorf("resolve module for domain model %s: %w", domainModelID, err)
	}
	return mod.Name, nil
}

// entityNameForID resolves an entity's name from its ID. Synthetic IDs from a
// reconstructed (dirty) read resolve through the synthetic map; saved entities
// resolve via the local reader.
func (b *Backend) entityNameForID(domainModelID, entityID model.ID) (string, error) {
	if name, ok := b.syntheticName(entityID); ok {
		return name, nil
	}
	dm, err := b.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return "", fmt.Errorf("resolve domain model %s: %w", domainModelID, err)
	}
	for _, e := range dm.Entities {
		if e.ID == entityID {
			return e.Name, nil
		}
	}
	return "", fmt.Errorf("entity %s not found in domain model %s", entityID, domainModelID)
}

// entityIndex finds the position of an entity within the live /entities array.
func (b *Backend) entityIndex(moduleName, entityName string) (int, error) {
	return b.arrayElementIndex(moduleName, "/entities", "entity", entityName)
}

// pedEntityRef resolves an entity ID to the $id(/entities/N) path-reference PED
// uses for by-id entity references (an association's parentEntity/childEntity).
// It maps the ID to the entity name (handling synthetic session IDs and on-disk
// IDs) and then to the entity's live index. PED accepts this form on write — the
// same form it emits on read — which sidesteps needing a real GUID for an entity
// created earlier in this session.
func (b *Backend) pedEntityRef(domainModelID model.ID, moduleName string, entityID model.ID) (string, error) {
	name, err := b.entityNameForID(domainModelID, entityID)
	if err != nil {
		return "", err
	}
	idx, err := b.entityIndex(moduleName, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$id(/entities/%d)", idx), nil
}

// arrayElementIndex reads a named-element array (e.g. /entities, /associations)
// over MCP and returns the index of the element with the given name. Reading
// over MCP means the index reflects Studio Pro's in-memory order, which is what
// a subsequent remove-by-index needs.
func (b *Backend) arrayElementIndex(moduleName, jsonPath, kind, name string) (int, error) {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        []string{jsonPath},
	})
	if err != nil {
		return 0, err
	}
	if res.IsError {
		return 0, fmt.Errorf("ped_read_document %s%s: %s", moduleName, jsonPath, res.Text)
	}
	var doc struct {
		Results []struct {
			Result []struct {
				Name string `json:"name"`
			} `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(res.Text), &doc); err != nil {
		return 0, fmt.Errorf("parse %s of %s: %w", jsonPath, moduleName, err)
	}
	if len(doc.Results) == 0 {
		return 0, fmt.Errorf("%s %q not found in module %q", kind, name, moduleName)
	}
	for i, e := range doc.Results[0].Result {
		if e.Name == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%s %q not found in module %q", kind, name, moduleName)
}

// ensureSchema fetches schemas for the given element types once per session.
func (b *Backend) ensureSchema(elementTypes ...string) error {
	var needed []string
	for _, t := range elementTypes {
		if !b.schemaFetched[t] {
			needed = append(needed, t)
		}
	}
	if len(needed) == 0 {
		return nil
	}
	res, err := b.client.CallTool("ped_get_schema", map[string]any{"elementTypes": needed})
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("ped_get_schema %v: %s", needed, res.Text)
	}
	if b.schemaFetched == nil {
		b.schemaFetched = map[string]bool{}
	}
	for _, t := range needed {
		b.schemaFetched[t] = true
	}
	return nil
}

// pedUpdate applies operations to a module's domain model and marks it dirty.
func (b *Backend) pedUpdate(moduleName string, ops ...pedOpEntry) error {
	if err := b.pedUpdateDoc(domainModelDocType, moduleName, ops...); err != nil {
		return err
	}
	// The write applied to Studio Pro's in-memory model; the on-disk .mpr is now
	// stale for this module, so route its reads through reconstruction.
	b.markDirty(moduleName)
	return nil
}

// pedUpdateDoc applies operations to any document (domain model, view-entity
// source doc, …). Callers that change a module's domain model use pedUpdate,
// which also marks the module dirty.
func (b *Backend) pedUpdateDoc(docType, docName string, ops ...pedOpEntry) error {
	// A module created via ped_create_module this session lags briefly before
	// ped_update_document can mutate it ("Module ... not found"), even though the
	// create flushed to disk. Retry on that transient with a short backoff.
	const maxAttempts = 6
	for attempt := 0; ; attempt++ {
		res, err := b.client.CallTool("ped_update_document", map[string]any{
			"documentType": docType,
			"documentName": docName,
			"operations":   ops,
		})
		if err != nil {
			return err
		}
		opErr := pedOpError("ped_update_document", docName, res)
		if opErr == nil {
			return nil
		}
		if attempt < maxAttempts-1 && strings.Contains(opErr.Error(), "not found") {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		return opErr
	}
}

// pedStripReminder removes the trailing <system-reminder>…</system-reminder>
// block the PED server appends to many results.
func pedStripReminder(text string) string {
	if i := strings.Index(text, "<system-reminder>"); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

// pedOpError turns a ped_create_document / ped_update_document result into an
// error. CRITICAL: these tools report failures in the result TEXT, frequently
// with isError=false (e.g. "Creating documents failed (1 of 1): ERROR …"). A
// successful op's text begins with "SUCCESS"; anything else is a failure.
func pedOpError(tool, target string, res *ToolResult) error {
	if res.IsError || !strings.HasPrefix(strings.TrimSpace(res.Text), "SUCCESS") {
		return fmt.Errorf("%s %s: %s", tool, target, pedStripReminder(res.Text))
	}
	return nil
}

// pedCheckErrors validates a module's domain model and surfaces any errors.
func (b *Backend) pedCheckErrors(moduleName string) error {
	return b.pedCheckDocument(domainModelDocType, moduleName)
}

// pedCheckDocument validates an arbitrary document and surfaces any errors.
// ped_check_errors reports a clean document as "No errors found." (with
// isError=false); any other text is the validation error(s).
func (b *Backend) pedCheckDocument(docType, docName string) error {
	res, err := b.client.CallTool("ped_check_errors", map[string]any{
		"documents": []map[string]any{
			{"documentType": docType, "documentName": docName},
		},
	})
	if err != nil {
		return err
	}
	text := pedStripReminder(res.Text)
	if res.IsError || !strings.Contains(text, "No errors found") {
		return fmt.Errorf("validation failed for %s: %s", docName, text)
	}
	return nil
}

// pedCreateDocument creates a standalone document (enumeration, microflow, …)
// via ped_create_document. documentContent is the type's $constructor body.
func (b *Backend) pedCreateDocument(moduleName, docType, docName string, content any, folderPath string) error {
	doc := map[string]any{
		"documentType":    docType,
		"moduleName":      moduleName,
		"documentName":    docName,
		"documentContent": content,
	}
	// A non-empty folderPath places the document in that folder, auto-creating the
	// whole path (PED has no empty-folder create); "" leaves it at the module root.
	if folderPath != "" {
		doc["folderPath"] = folderPath
	}
	res, err := b.client.CallTool("ped_create_document", map[string]any{
		"documents": []map[string]any{doc},
	})
	if isTimeoutErr(err) {
		// The create is not idempotent (a re-run would fail or duplicate), but
		// its outcome is verifiable: Studio Pro frequently applies the create
		// even though the response timed out, so confirm before failing.
		time.Sleep(timeoutVerifyDelay)
		if exists, verr := b.pedDocumentExists(moduleName, docType, docName); verr == nil && exists {
			timeoutNotice(fmt.Sprintf("%s %s.%s", docType, moduleName, docName))
			b.markDirty(moduleName)
			return nil
		}
		return timeoutUnverified(err, fmt.Sprintf("%s %s.%s", docType, moduleName, docName))
	}
	if err != nil {
		return err
	}
	if e := pedOpError("ped_create_document", moduleName+"."+docName, res); e != nil {
		return e
	}
	b.markDirty(moduleName)
	return nil
}
