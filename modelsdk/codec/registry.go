package codec

import (
	"reflect"
	"sync"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
)

// TypeRegistry maps BSON $Type strings to element factory functions, and
// maintains a reverse map from Go concrete type to canonical BSON $Type name.
type TypeRegistry struct {
	mu        sync.RWMutex
	factories map[string]func() element.Element
	reverse   map[reflect.Type]string // Go type → canonical BSON $Type
}

// NewRegistry returns an empty TypeRegistry.
func NewRegistry() *TypeRegistry {
	return &TypeRegistry{
		factories: map[string]func() element.Element{},
		reverse:   map[reflect.Type]string{},
	}
}

// Register adds or replaces the factory for the given typeName and records the
// Go→BSON reverse mapping. The first registration for a given Go type wins
// (canonical name), so aliases registered later do not overwrite it.
func (r *TypeRegistry) Register(typeName string, factory func() element.Element) {
	r.mu.Lock()
	r.factories[typeName] = factory
	goType := reflect.TypeOf(factory())
	if _, exists := r.reverse[goType]; !exists {
		r.reverse[goType] = typeName
	}
	r.mu.Unlock()
}

// Lookup returns the factory for typeName, and whether it was found.
func (r *TypeRegistry) Lookup(typeName string) (func() element.Element, bool) {
	r.mu.RLock()
	f, ok := r.factories[typeName]
	r.mu.RUnlock()
	return f, ok
}

// TypeNameOf returns the canonical BSON $Type name for the given Go reflect.Type.
// Returns ("", false) if the type was never registered.
func (r *TypeRegistry) TypeNameOf(goType reflect.Type) (string, bool) {
	r.mu.RLock()
	name, ok := r.reverse[goType]
	r.mu.RUnlock()
	return name, ok
}

// RegisterAlias registers storageName as an alias for qualifiedName.
// When the decoder encounters storageName in $Type, it uses the factory
// registered for qualifiedName. Aliases do NOT update the reverse map;
// TypeNameOf always returns the canonical (first-registered) name.
func (r *TypeRegistry) RegisterAlias(storageName, qualifiedName string) {
	r.mu.RLock()
	f, ok := r.factories[qualifiedName]
	r.mu.RUnlock()
	if ok {
		r.mu.Lock()
		r.factories[storageName] = f // forward only, no reverse update
		r.mu.Unlock()
	}
}

// DefaultRegistry is the package-level registry used by generated types.
var DefaultRegistry = NewRegistry()

// No runtime aliases needed — all storage name mappings are handled at
// codegen time via dual registration in gen/*/types.go init() functions.
// See cmd/modelsdk-codegen/main.go storageAliases and propertyKeyOverrides.
