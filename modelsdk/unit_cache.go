package modelsdk

import (
	"sync"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
)

// UnitCache caches decoded elements and provides name-based lookup.
type UnitCache struct {
	mu         sync.Mutex
	cache      map[element.ID]element.Element
	cacheGen   int64
	elementGen map[element.ID]int64
	typeGen    map[string]int64
	nameIndex  map[string]element.ID // "type:name" → unit ID
}

func newUnitCache() *UnitCache {
	return &UnitCache{
		cache:      map[element.ID]element.Element{},
		elementGen: map[element.ID]int64{},
		typeGen:    map[string]int64{},
	}
}

// Get returns a cached element by ID.
func (c *UnitCache) Get(id element.ID) (element.Element, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.cache[id]
	return elem, ok
}

// GetWithGen returns a cached element and its generation.
func (c *UnitCache) GetWithGen(id element.ID) (element.Element, int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.cache[id]
	gen := c.elementGen[id]
	return elem, gen, ok
}

// Put stores a decoded element in the cache at the current generation.
func (c *UnitCache) Put(id element.ID, elem element.Element) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[id] = elem
	c.elementGen[id] = c.cacheGen
}

// PutWithGen stores a decoded element at a specific generation.
func (c *UnitCache) PutWithGen(id element.ID, elem element.Element, gen int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[id] = elem
	c.elementGen[id] = gen
}

// Delete removes an element from the cache.
func (c *UnitCache) Delete(id element.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, id)
	delete(c.elementGen, id)
}

// TypeGen returns the current generation for a type.
func (c *UnitCache) TypeGen(typeName string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.typeGen[typeName]
}

// BumpGen increments the global cache generation and the per-type generation.
func (c *UnitCache) BumpGen(typeName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheGen++
	if typeName != "" {
		c.typeGen[typeName]++
	}
}

// DirtyUnits returns all cached elements that have been modified.
func (c *UnitCache) DirtyUnits() map[element.ID]element.Element {
	c.mu.Lock()
	defer c.mu.Unlock()
	dirty := make(map[element.ID]element.Element)
	for id, elem := range c.cache {
		if elem.IsDirty() {
			dirty[id] = elem
		}
	}
	return dirty
}

// RebuildNameIndex rebuilds the name → ID index from unit metadata.
func (c *UnitCache) RebuildNameIndex(units []codec.UnitInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := make(map[string]element.ID, len(units))
	for _, u := range units {
		if u.Type != "" && u.Name != "" {
			idx[u.Type+":"+u.Name] = u.ID
		}
	}
	c.nameIndex = idx
}

// FindByName looks up a unit ID by "type:name" key.
func (c *UnitCache) FindByName(key string) (element.ID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.nameIndex[key]
	return id, ok
}

// AddToNameIndex adds a type:name → ID mapping.
func (c *UnitCache) AddToNameIndex(typeName, name string, id element.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if typeName != "" && name != "" {
		c.nameIndex[typeName+":"+name] = id
	}
}

// RemoveFromNameIndex removes a type:name mapping.
func (c *UnitCache) RemoveFromNameIndex(typeName, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if typeName != "" && name != "" {
		delete(c.nameIndex, typeName+":"+name)
	}
}

// TypeNameOf returns the TypeName of a cached element, or empty string if not cached.
func (c *UnitCache) TypeNameOf(id element.ID) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.cache[id]; ok {
		return elem.TypeName()
	}
	return ""
}
