package modelsdk

import (
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ModuleResolver caches and resolves module names from the unit hierarchy.
type ModuleResolver struct {
	store *codec.Store
	units func() []codec.UnitInfo // accessor to get current unit list
	cache map[element.ID]string
}

func newModuleResolver(store *codec.Store, units func() []codec.UnitInfo) *ModuleResolver {
	return &ModuleResolver{store: store, units: units}
}

// ModuleMap returns a mapping from module unit ID to module name.
func (r *ModuleResolver) ModuleMap() map[element.ID]string {
	if r.cache != nil {
		return r.cache
	}
	result := make(map[element.ID]string)
	for _, u := range r.units() {
		raw, err := r.store.LoadUnit(u.ID)
		if err != nil {
			continue
		}
		tn := decodeTypeField(raw)
		if tn != "Projects$ModuleImpl" {
			continue
		}
		name, err := bson.Raw(raw).LookupErr("Name")
		if err != nil {
			continue
		}
		if s, ok := name.StringValueOK(); ok {
			result[u.ID] = s
		}
	}
	r.cache = result
	return result
}

// ResolveModuleName finds the module name for a unit by walking the container hierarchy.
func (r *ModuleResolver) ResolveModuleName(containerID element.ID) string {
	moduleMap := r.ModuleMap()
	parentMap := make(map[element.ID]element.ID)
	for _, u := range r.units() {
		parentMap[u.ID] = u.ContainerID
	}
	current := containerID
	for range 20 {
		if name, ok := moduleMap[current]; ok {
			return name
		}
		parent, ok := parentMap[current]
		if !ok || parent == current {
			break
		}
		current = parent
	}
	return ""
}

// Invalidate clears the cached module map.
func (r *ModuleResolver) Invalidate() {
	r.cache = nil
}
