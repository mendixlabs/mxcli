// Package modelsdk provides typed, lazy-decoded access to Mendix project files (.mpr).
//
// Usage:
//
//	import _ "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
//
//	m, err := modelsdk.Open("app.mpr")
//	defer m.Close()
//	for _, dm := range m.AllOfType("DomainModels$DomainModel") { ... }
//
//	// Read-write:
//	m, err := modelsdk.OpenForWriting("app.mpr")
//	entity.SetName("NewName")
//	entity.MarkDirty(0)
//	m.Flush()
package modelsdk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Model provides high-level access to a Mendix project.
type Model struct {
	store   *codec.Store
	decoder *codec.Decoder
	encoder *codec.Encoder
	uc      *UnitCache
	units   []codec.UnitInfo
	modules *ModuleResolver
}

// Open opens an MPR file for reading.
func Open(path string) (*Model, error) {
	store, err := codec.Open(path)
	if err != nil {
		return nil, err
	}
	return newModel(store), nil
}

// OpenForWriting opens an MPR file for reading and writing.
func OpenForWriting(path string) (*Model, error) {
	store, err := codec.OpenForWriting(path)
	if err != nil {
		return nil, err
	}
	return newModel(store), nil
}

func newModel(store *codec.Store) *Model {
	m := &Model{
		store:   store,
		decoder: codec.NewDecoder(codec.DefaultRegistry),
		encoder: &codec.Encoder{},
		uc:      newUnitCache(),
	}
	m.units = store.ListUnits()
	m.uc.RebuildNameIndex(m.units)
	m.modules = newModuleResolver(store, func() []codec.UnitInfo { return m.units })
	return m
}

// Close closes the underlying store.
func (m *Model) Close() error {
	return m.store.Close()
}

// IsWritable returns true if the model was opened for writing.
func (m *Model) IsWritable() bool {
	return m.store.IsWritable()
}

// Units returns metadata for all document units in the MPR.
func (m *Model) Units() []codec.UnitInfo {
	return m.units
}

// LoadUnit loads and decodes a single unit by ID. Results are cached.
// Not safe for concurrent mutation — use from a single goroutine.
func (m *Model) LoadUnit(id element.ID) (element.Element, error) {
	if elem, ok := m.uc.Get(id); ok {
		return elem, nil
	}

	raw, err := m.store.LoadUnit(id)
	if err != nil {
		return nil, err
	}
	elem, err := m.decoder.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode unit %s: %w", id, err)
	}

	m.uc.Put(id, elem)
	return elem, nil
}

// AllOfType loads and returns all units whose $Type matches the given type name.
// Uses UnitInfo.Type metadata to pre-filter, avoiding BSON I/O for non-matching units.
// Not safe for concurrent mutation — use from a single goroutine.
func (m *Model) AllOfType(typeName string) []element.Element {
	var result []element.Element
	currentTypeGen := m.uc.TypeGen(typeName)

	for _, u := range m.units {
		// Fast path: skip units whose metadata type doesn't match.
		if u.Type != "" && u.Type != typeName {
			continue
		}

		// Use cached element if available AND it was cached in the current
		// per-type generation. This preserves in-memory mutations made by
		// earlier statements in the same batch (e.g., CREATE ENTITY followed
		// by CREATE ASSOCIATION) while forcing a re-decode only when units of
		// this specific type have changed (InsertUnit/DeleteUnit bumps typeGen
		// for the affected type only).
		cached, cachedGen, hasCached := m.uc.GetWithGen(u.ID)
		if hasCached && cachedGen >= currentTypeGen && u.Type != "" {
			result = append(result, cached)
			continue
		}

		raw, err := m.store.LoadUnit(u.ID)
		if err != nil {
			continue
		}
		// Fallback: if metadata type was empty, check BSON $Type field.
		if u.Type == "" {
			if decodeTypeField(raw) != typeName {
				continue
			}
		}

		// If cached in this generation (but type was empty so we had to verify), return cached version
		if hasCached && cachedGen >= currentTypeGen {
			result = append(result, cached)
			continue
		}

		elem, err := m.decoder.Decode(raw)
		if err != nil {
			continue
		}
		m.uc.PutWithGen(u.ID, elem, currentTypeGen)
		result = append(result, elem)
	}
	return result
}

// FindByQualifiedName searches for a unit of the given type whose BSON
// "Name" field matches name. Note: Mendix stores the module-local simple
// name (e.g. "ACT_GetUser"), not the dot-qualified name ("MyModule.ACT_GetUser").
// Uses the name index for O(1) lookup, falling back to a linear scan if the index misses.
func (m *Model) FindByQualifiedName(typeName, name string) (element.Element, error) {
	// Fast path: index lookup.
	if id, ok := m.uc.FindByName(typeName + ":" + name); ok {
		return m.LoadUnit(id)
	}

	// Fallback: linear scan (covers units whose Type/Name metadata was empty).
	for _, u := range m.units {
		if u.Type != "" && u.Type != typeName {
			continue
		}
		raw, err := m.store.LoadUnit(u.ID)
		if err != nil {
			continue
		}
		if u.Type == "" && decodeTypeField(raw) != typeName {
			continue
		}
		bsonName, _ := raw.LookupErr("Name")
		if s, ok := bsonName.StringValueOK(); ok && s == name {
			elem, err := m.decoder.Decode(raw)
			if err != nil {
				return nil, fmt.Errorf("decode unit %s: %w", u.ID, err)
			}
			m.uc.Put(u.ID, elem)
			return elem, nil
		}
	}
	return nil, fmt.Errorf("element %s with name %q not found", typeName, name)
}

// Encode serializes an element to BSON bytes.
func (m *Model) Encode(elem element.Element) ([]byte, error) {
	return m.encoder.Encode(elem)
}

// Flush encodes and saves all dirty cached units back to the MPR.
// Returns the number of units written and an error if the model is read-only.
func (m *Model) Flush() (int, error) {
	if !m.store.IsWritable() {
		return 0, fmt.Errorf("model is read-only — use OpenForWriting")
	}

	dirtyElems := m.uc.DirtyUnits()
	dirty := map[element.ID][]byte{}
	for id, elem := range dirtyElems {
		data, err := m.encoder.Encode(elem)
		if err != nil {
			return 0, fmt.Errorf("encode unit %s: %w", id, err)
		}
		dirty[id] = data
	}

	if len(dirty) == 0 {
		return 0, nil // nothing to flush
	}

	return len(dirty), m.store.FlushUnits(dirty)
}

// ModuleMap returns a mapping from module unit ID to module name.
func (m *Model) ModuleMap() map[element.ID]string {
	return m.modules.ModuleMap()
}

// ResolveModuleName finds the module name for a unit by walking the container hierarchy upward.
func (m *Model) ResolveModuleName(containerID element.ID) string {
	return m.modules.ResolveModuleName(containerID)
}

// GetProjectRootID returns the ID of the project root unit.
func (m *Model) GetProjectRootID() (string, error) {
	return m.store.GetProjectRootID()
}

// Store returns the underlying codec.Store for low-level unit access.
func (m *Model) Store() *codec.Store { return m.store }

// PatchEncodedField sets a top-level field on already-encoded BSON bytes.
// Use this only when the SDK type's setter has a different type than the
// BSON storage format (e.g., SDK uses Part[element.Element] but BSON stores
// a plain string).
// TODO: remove when generated setters cover all BSON storage type mismatches.
func (m *Model) PatchEncodedField(data []byte, key string, value any) ([]byte, error) {
	return codec.PatchBSONField(data, key, value)
}

// PatchEncodedNestedField sets a field within a nested document in encoded BSON.
func (m *Model) PatchEncodedNestedField(data []byte, parentKey, childKey string, value any) ([]byte, error) {
	return codec.PatchNestedBSONField(data, parentKey, childKey, value)
}

// PatchEncodedVersionedArrayElements patches fields on elements of a versioned
// BSON array by index. patches maps element index (0-based, skipping the
// version marker) to a map of field-name -> value.
func (m *Model) PatchEncodedVersionedArrayElements(data []byte, arrayKey string, patches map[int]map[string]any) ([]byte, error) {
	return codec.PatchVersionedArrayElements(data, arrayKey, patches)
}

// ScanUnitStrings scans a unit's raw BSON for string values without
// decoding into SDK types. fn receives (fieldPath, value) for each
// string field. Return false to stop scanning.
// Used by search — avoids the cost of full SDK decode.
func (m *Model) ScanUnitStrings(id element.ID, fn func(fieldPath, value string) bool) {
	raw, err := m.store.LoadUnit(id)
	if err != nil {
		return
	}
	codec.ScanBSONStrings(raw, fn)
}

// InsertUnit creates a new document unit in the MPR.
func (m *Model) InsertUnit(id element.ID, containerID element.ID, containmentName, typeName string, data []byte) error {
	if err := m.store.InsertUnit(string(id), string(containerID), containmentName, typeName, data); err != nil {
		return err
	}

	// Extract Name from BSON for the name index (single field decode, not full scan).
	name := decodeBSONNameField(data)

	m.uc.BumpGen(typeName)
	// Incremental update: append to units slice + add to name index.
	m.units = append(m.units, codec.UnitInfo{
		ID:          id,
		ContainerID: containerID,
		Type:        typeName,
		Name:        name,
	})
	m.uc.AddToNameIndex(typeName, name, id)
	return nil
}

// DeleteUnit removes a document unit from the MPR.
func (m *Model) DeleteUnit(id element.ID) error {
	// Find the unit metadata before deleting (for incremental index update).
	var deletedType, deletedName string
	deletedType = m.uc.TypeNameOf(id)
	for i, u := range m.units {
		if u.ID == id {
			if deletedType == "" {
				deletedType = u.Type
			}
			deletedName = u.Name
			// Remove from slice in-place.
			m.units = append(m.units[:i], m.units[i+1:]...)
			break
		}
	}

	if err := m.store.DeleteUnit(string(id)); err != nil {
		return err
	}
	m.uc.Delete(id)
	m.uc.BumpGen(deletedType)
	m.uc.RemoveFromNameIndex(deletedType, deletedName)
	return nil
}

// DeleteModuleWithCleanup deletes a module, all its child units, and its themesource directory.
// Uses a single full-reload after all deletes (bulk operation, not incremental).
func (m *Model) DeleteModuleWithCleanup(moduleID element.ID, moduleName string) error {
	if !m.store.IsWritable() {
		return fmt.Errorf("model is read-only — use OpenForWriting")
	}

	// Recursively delete child units in the store (bypasses Model cache).
	if err := m.store.DeleteChildUnits(string(moduleID)); err != nil {
		return fmt.Errorf("delete child units: %w", err)
	}

	// Delete the module unit itself in the store.
	if err := m.store.DeleteUnit(string(moduleID)); err != nil {
		return fmt.Errorf("delete module unit: %w", err)
	}

	// Bulk cleanup: clear cache entries for deleted units and do a single full reload.
	m.uc.Delete(moduleID)
	m.uc.BumpGen("")

	// Full reload is appropriate here — module deletion is rare and removes many units.
	m.units = m.store.ListUnits()
	m.uc.RebuildNameIndex(m.units)
	m.modules.Invalidate()

	// Clean up themesource directory.
	projectDir := filepath.Dir(m.store.Path())
	themesourceBase := filepath.Join(projectDir, "themesource")
	themesourceDir := filepath.Clean(filepath.Join(themesourceBase, strings.ToLower(moduleName)))
	// Guard against path traversal: the resolved path must be under themesource/.
	if strings.HasPrefix(themesourceDir, themesourceBase+string(filepath.Separator)) {
		if stat, err := os.Stat(themesourceDir); err == nil && stat.IsDir() {
			os.RemoveAll(themesourceDir)
		}
	}

	return nil
}

// InvalidateCache removes a cached element so it will be reloaded on next access.
func (m *Model) InvalidateCache(id element.ID) {
	m.uc.Delete(id)
}

func decodeTypeField(raw bson.Raw) string {
	val, err := raw.LookupErr("$Type")
	if err != nil {
		return ""
	}
	s, _ := val.StringValueOK()
	return s
}

// decodeBSONNameField extracts the "Name" string from raw BSON data.
// Returns "" if the field is missing or not a string.
func decodeBSONNameField(data []byte) string {
	val, err := bson.Raw(data).LookupErr("Name")
	if err != nil {
		return ""
	}
	s, _ := val.StringValueOK()
	return s
}
