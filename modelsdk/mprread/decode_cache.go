// SPDX-License-Identifier: Apache-2.0

package mprread

import (
	"bytes"
	"os"
	"sync"
	"sync/atomic"

	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// decodeCache keeps the decoded form of the units a reader has decoded more
// than once, so that listing a document type again decodes only the units
// that changed since.
//
// A script costs statements × project without it: the check before an exec
// and the exec itself each list whole document types per statement — every
// microflow to resolve one call, every domain model to resolve one entity —
// and each listing decoded every unit from its bytes (ako/mxcli#2160: about a
// thousand full microflow listings for 600 page statements, where one costs
// 0.11 s).
//
// Three properties make sharing a decoded element safe:
//
//   - An entry is valid for exactly the bytes it was decoded from. It holds
//     them and is used only when a unit's stored bytes are equal, so a write,
//     by this process or another, needs no invalidation: the changed unit no
//     longer matches. This is the rule Reader.Memo asks for.
//   - An element is shared only after freeze has loaded every lazily decoded
//     value in it. A read of a frozen element writes nothing, so two goroutines
//     may read the same one (the catalog builds with a worker pool).
//   - An element a caller has changed is dirty, and a dirty entry is dropped
//     and decoded again rather than handed to the next caller.
//
// A unit is not cached the first time it is decoded, only remembered: a
// command that lists a type once pays nothing — no freeze, no retained memory.
type decodeCache struct {
	mu      sync.Mutex
	entries map[string]*decodedUnit // by unit ID
}

type decodedUnit struct {
	contents []byte
	// elem is nil after the first decode of contents and set by the second.
	elem element.Element
}

type decodeCacheKey struct{}

// decodeCacheOff turns the cache off for the process: MXCLI_DECODE_CACHE=off,
// to rule the cache in or out when a result looks wrong.
var decodeCacheOff atomic.Bool

func init() {
	if os.Getenv("MXCLI_DECODE_CACHE") == "off" {
		decodeCacheOff.Store(true)
	}
}

// SetDecodeCacheEnabled switches the cache for the process and returns the
// function that restores the previous setting. For tests: the control of a
// decode budget is the same pass with the cache off.
func SetDecodeCacheEnabled(on bool) (restore func()) {
	was := decodeCacheOff.Swap(!on)
	return func() { decodeCacheOff.Store(was) }
}

// decodeCacheHits counts the units served from the cache, process-wide.
var decodeCacheHits atomic.Int64

// DecodeCacheHits returns how many units have been served from the cache.
func DecodeCacheHits() int64 { return decodeCacheHits.Load() }

func decodeCacheFor(r *mmpr.Reader) *decodeCache {
	if r == nil || decodeCacheOff.Load() {
		return nil
	}
	c, _ := r.Memo().LoadOrStore(decodeCacheKey{}, &decodeCache{entries: map[string]*decodedUnit{}})
	return c.(*decodeCache)
}

// lookup returns the cached element for the unit when its stored bytes are the
// ones the element was decoded from and nobody has changed it since. Otherwise
// store says whether the caller's fresh decode should be frozen and kept:
// true from the second decode of the same bytes on.
func (c *decodeCache) lookup(id string, contents []byte) (elem element.Element, store bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[id]
	if e == nil || !sameBytes(e.contents, contents) {
		c.entries[id] = &decodedUnit{contents: contents}
		return nil, false
	}
	if e.elem != nil && !e.elem.IsDirty() {
		return e.elem, false
	}
	return nil, true
}

// sameBytes reports whether a and b hold the same bytes. A listing made while
// the reader's content cache is held is given that cache's own slice for each
// unit (Reader.ListUnitsByTypeShared), so an unchanged unit arrives as the very
// slice its entry was decoded from, and is recognised without reading it.
func sameBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 || &a[0] == &b[0] {
		return true
	}
	return bytes.Equal(a, b)
}

func (c *decodeCache) put(id string, contents []byte, elem element.Element) {
	freeze(elem)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = &decodedUnit{contents: contents, elem: elem}
}

// freeze loads every lazily decoded value in the tree. A scalar property
// decodes from the element's bytes on its first read and keeps the result;
// after this walk every such read has happened, so reading the tree no longer
// writes to it. Reading a value does not mark it dirty.
func freeze(root element.Element) {
	element.Walk(root, func(e element.Element) bool {
		for _, prop := range e.Properties() {
			if wp, ok := prop.(element.WritableProperty); ok {
				_ = wp.BSONValue()
			}
		}
		return true
	})
}
