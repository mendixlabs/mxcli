// SPDX-License-Identifier: Apache-2.0

package mprread

import (
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// Unit pairs a decoded gen-typed element with its SQLite ContainerID.
// ContainerID is a DB-level column that is not embedded in the BSON content,
// so callers that need it (e.g. building module-level trees) can obtain it
// here without a second query.
type Unit[T element.Element] struct {
	Element     T
	ContainerID model.ID
}

// bsonTypeName returns the canonical BSON $Type name for the type parameter T
// by looking up the Go concrete type in the codec DefaultRegistry reverse map.
// It panics at startup if T is not registered, which catches configuration
// errors at init time rather than silently returning empty results at runtime.
func bsonTypeName[T element.Element]() string {
	var zero T
	goType := reflect.TypeOf(&zero).Elem()
	name, ok := codec.DefaultRegistry.TypeNameOf(goType)
	if !ok {
		panic(fmt.Sprintf("mprread: type %v not registered in codec.DefaultRegistry — import the gen package", goType))
	}
	return name
}

// ListUnitsByType decodes every unit of type T and returns them as []T.
// The BSON $Type name is inferred from T via the codec registry — no magic
// string needed. Import the relevant gen/* package to trigger registration.
//
// ContainerID is not included; use ListUnitsWithContainer if you need it.
func ListUnitsByType[T element.Element](r *mmpr.Reader) ([]T, error) {
	units, err := ListUnitsWithContainer[T](r)
	if err != nil {
		return nil, err
	}
	out := make([]T, len(units))
	for i, u := range units {
		out[i] = u.Element
	}
	return out, nil
}

// ListUnitsWithContainer decodes every unit of type T and returns each element
// paired with its ContainerID. Uses ref.Contents from the unit index (already
// loaded by ListUnitsByType on the Reader) to avoid a second per-unit read.
// The BSON $Type name is inferred from T via the codec registry.
func ListUnitsWithContainer[T element.Element](r *mmpr.Reader) ([]Unit[T], error) {
	return ListUnitsWithContainerWhere[T](r, nil)
}

// ListUnitsWithContainerWhere is ListUnitsWithContainer for the units whose
// stored document keep accepts (nil keeps all). keep sees the raw bytes, so a
// lookup by name or ID decodes the one document it wants instead of every
// document of the type — the decode was most of a flow lookup's cost, and
// check makes several per statement (mendixlabs/mxcli#1272).
func ListUnitsWithContainerWhere[T element.Element](r *mmpr.Reader, keep func(bson.Raw) bool) ([]Unit[T], error) {
	typeName := bsonTypeName[T]()
	// Shared: the decoder only reads the bytes, and keeps a reference to them.
	refs, err := r.ListUnitsByTypeShared(typeName, nil)
	if err != nil {
		return nil, err
	}
	return decodeUnits[T](r, refs, typeName, keep)
}

// ListUnitsWithContainerIn is ListUnitsWithContainer for the units whose ID
// keepID accepts, and reads no other unit's contents on v2 (ako/mxcli#1804).
func ListUnitsWithContainerIn[T element.Element](r *mmpr.Reader, keepID func(id string) bool) ([]Unit[T], error) {
	typeName := bsonTypeName[T]()
	refs, err := r.ListUnitsByTypeShared(typeName, keepID)
	if err != nil {
		return nil, err
	}
	return decodeUnits[T](r, refs, typeName, nil)
}

// decodeUnits decodes the refs of typeName that keep accepts (nil keeps all),
// serving from r's decode cache the units it has decoded from the same bytes
// before (see decodeCache). An element it returns may be one another caller
// holds too.
func decodeUnits[T element.Element](r *mmpr.Reader, refs []mmpr.UnitRef, typeName string, keep func(bson.Raw) bool) ([]Unit[T], error) {
	dec := codec.NewDecoder(codec.DefaultRegistry)
	cache := decodeCacheFor(r)
	out := make([]Unit[T], 0, len(refs))
	for _, ref := range refs {
		if ref.Type != typeName {
			continue
		}
		if keep != nil && !keep(bson.Raw(ref.Contents)) {
			continue
		}
		var store bool
		if cache != nil {
			var cached element.Element
			if cached, store = cache.lookup(ref.ID, ref.Contents); cached != nil {
				if typed, ok := cached.(T); ok {
					decodeCacheHits.Add(1)
					out = append(out, Unit[T]{Element: typed, ContainerID: model.ID(ref.ContainerID)})
					continue
				}
			}
		}
		unitsDecoded.Add(1)
		elem, err := dec.Decode(bson.Raw(ref.Contents))
		if err != nil {
			return nil, fmt.Errorf("decode unit %s: %w", ref.ID, err)
		}
		typed, ok := elem.(T)
		if !ok {
			return nil, fmt.Errorf("unit %s decoded as %T, want %T", ref.ID, elem, *new(T))
		}
		if store {
			cache.put(ref.ID, ref.Contents, elem)
		}
		out = append(out, Unit[T]{Element: typed, ContainerID: model.ID(ref.ContainerID)})
	}
	return out, nil
}

// UnitByIDAs decodes the unit whose ID is id, whatever storage name it has,
// and returns it if it decodes to a T. It is UnitByID for a Go type registered
// under more than one storage name — a module is `Projects$ModuleImpl` in a
// project and `Projects$Module` in the registry's first entry for the type, so
// UnitByID, which matches the one name the registry gives back, never finds it.
func UnitByIDAs[T element.Element](r *mmpr.Reader, id model.ID) (unit Unit[T], ok bool, err error) {
	ref, found, err := r.UnitByID(string(id))
	if err != nil || !found {
		return Unit[T]{}, false, err
	}
	factory, registered := codec.DefaultRegistry.Lookup(ref.Type)
	if !registered {
		return Unit[T]{}, false, nil
	}
	if _, isT := factory().(T); !isT {
		return Unit[T]{}, false, nil
	}
	units, err := decodeUnits[T](r, []mmpr.UnitRef{ref}, ref.Type, nil)
	if err != nil || len(units) == 0 {
		return Unit[T]{}, false, err
	}
	return units[0], true, nil
}

// ListUnitsInContainer decodes the units of type T whose ContainerID is
// containerID, and reads no other unit: a lookup of one module's domain model
// reads one document, not the project's (mendixlabs/mxcli#1616).
func ListUnitsInContainer[T element.Element](r *mmpr.Reader, containerID model.ID) ([]Unit[T], error) {
	typeName := bsonTypeName[T]()
	refs, err := r.ListUnitsByTypeInContainer(typeName, string(containerID))
	if err != nil {
		return nil, err
	}
	return decodeUnits[T](r, refs, typeName, nil)
}

// UnitByID decodes the unit of type T whose ID is id, reading that unit only.
// ok is false when there is no such unit or it is not a T.
func UnitByID[T element.Element](r *mmpr.Reader, id model.ID) (unit Unit[T], ok bool, err error) {
	typeName := bsonTypeName[T]()
	ref, found, err := r.UnitByID(string(id))
	if err != nil || !found || ref.Type != typeName {
		return Unit[T]{}, false, err
	}
	units, err := decodeUnits[T](r, []mmpr.UnitRef{ref}, typeName, nil)
	if err != nil || len(units) == 0 {
		return Unit[T]{}, false, err
	}
	return units[0], true, nil
}

// ListUnitsNamed decodes the units of type T whose top-level Name is name.
// The names come from the reader's unit index, so on MPR v2 only the matching
// units are opened, where filtering the listing on the stored Name opened
// every unit of the type to read it (ako/mxcli#1791).
func ListUnitsNamed[T element.Element](r *mmpr.Reader, name string) ([]Unit[T], error) {
	hs, err := r.ListUnitHeadersWhere(bsonTypeName[T](), func(n string) bool { return n == name })
	if err != nil {
		return nil, err
	}
	var out []Unit[T]
	for _, h := range hs {
		u, ok, err := UnitByID[T](r, model.ID(h.ID))
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, u)
		}
	}
	return out, nil
}

// UnitHeader is what a name listing needs of a unit, read without decoding it.
type UnitHeader struct {
	ID          model.ID
	ContainerID model.ID
	Name        string
	Excluded    bool
}

// ListUnitHeaders lists the ID, container, Name and Excluded of every unit of
// type T without decoding any of them: for callers that build a name set or an
// ID-to-name map and would otherwise decode the whole flow of every document.
func ListUnitHeaders[T element.Element](r *mmpr.Reader) ([]UnitHeader, error) {
	return ListUnitHeadersWhere[T](r, nil)
}

// ListUnitHeadersWhere is ListUnitHeaders for the units whose Name keep
// accepts (nil accepts all): a lookup of one name copies one header, not the
// header of every unit of the type.
func ListUnitHeadersWhere[T element.Element](r *mmpr.Reader, keep func(name string) bool) ([]UnitHeader, error) {
	hs, err := r.ListUnitHeadersWhere(bsonTypeName[T](), keep)
	if err != nil {
		return nil, err
	}
	out := make([]UnitHeader, 0, len(hs))
	for _, h := range hs {
		out = append(out, UnitHeader{
			ID:          model.ID(h.ID),
			ContainerID: model.ID(h.ContainerID),
			Name:        h.Name,
			Excluded:    h.Excluded,
		})
	}
	return out, nil
}

// HasName keeps a document whose top-level Name is name.
func HasName(name string) func(bson.Raw) bool {
	return func(raw bson.Raw) bool {
		n, ok := raw.Lookup("Name").StringValueOK()
		return ok && n == name
	}
}

// HasID keeps the document whose $ID is id.
func HasID(id model.ID) func(bson.Raw) bool {
	return func(raw bson.Raw) bool { return model.ID(codec.PeekID(raw)) == id }
}
