// SPDX-License-Identifier: Apache-2.0

package mprread_test

import (
	"sync"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// listMicroflowUnits is the listing every microflow lookup in the backend
// starts from.
func listMicroflowUnits(t *testing.T, list func() ([]mprread.Unit[*genMf.Microflow], error)) []mprread.Unit[*genMf.Microflow] {
	t.Helper()
	units, err := list()
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 10 {
		t.Fatalf("control: the fixture has %d microflows; a budget over so few proves nothing", len(units))
	}
	return units
}

// ako/mxcli#2160: a script listed whole document types once per statement, and
// each listing decoded every unit from its bytes. Listing a type again now
// decodes nothing: the first listing remembers each unit, the second keeps its
// decoded form, and every later one is served from there.
//
// The control is the same four listings with the cache off, which decode four
// times over — so the budget below cannot pass against a build without it.
func TestDecodeCache_RepeatedListingDecodesEachUnitTwiceAtMost(t *testing.T) {
	const listings = 4
	decodesOver := func(t *testing.T) (decoded, hits, units int64) {
		r := openTestReader(t)
		list := func() ([]mprread.Unit[*genMf.Microflow], error) {
			return mprread.ListUnitsWithContainer[*genMf.Microflow](r)
		}
		d0, h0 := mprread.UnitsDecoded(), mprread.DecodeCacheHits()
		var n int
		for i := 0; i < listings; i++ {
			n = len(listMicroflowUnits(t, list))
		}
		return mprread.UnitsDecoded() - d0, mprread.DecodeCacheHits() - h0, int64(n)
	}

	decoded, hits, units := decodesOver(t)
	if decoded != 2*units {
		t.Errorf("%d listings of %d microflows decoded %d units, want %d: once to remember, once to keep", listings, units, decoded, 2*units)
	}
	if hits != (listings-2)*units {
		t.Errorf("%d units served from the cache, want %d", hits, (listings-2)*units)
	}

	defer mprread.SetDecodeCacheEnabled(false)()
	decoded, hits, units = decodesOver(t)
	if decoded != listings*units || hits != 0 {
		t.Errorf("control, cache off: decoded %d units with %d hits, want %d and 0", decoded, hits, listings*units)
	}
}

// A command that lists a type once must pay nothing for the cache: nothing is
// kept, so nothing is frozen and no memory is retained.
func TestDecodeCache_ASingleListingIsNotCached(t *testing.T) {
	r := openTestReader(t)
	h0 := mprread.DecodeCacheHits()
	first := listMicroflowUnits(t, func() ([]mprread.Unit[*genMf.Microflow], error) {
		return mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	})
	second := listMicroflowUnits(t, func() ([]mprread.Unit[*genMf.Microflow], error) {
		return mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	})
	if mprread.DecodeCacheHits() != h0 {
		t.Errorf("the second listing was served from the cache: the first one was kept")
	}
	if first[0].Element == second[0].Element {
		t.Error("the first listing's element was handed out again: it was shared without being frozen")
	}
}

// The cache hands the same element to every caller, so a caller that changes
// one must not change what the next caller reads. A changed element is dirty,
// and a dirty entry is decoded again rather than served.
func TestDecodeCache_AnElementACallerChangedIsNotServedAgain(t *testing.T) {
	r := openTestReader(t)
	list := func() ([]mprread.Unit[*genMf.Microflow], error) {
		return mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	}
	listMicroflowUnits(t, list)
	listMicroflowUnits(t, list) // kept from here on
	shared := listMicroflowUnits(t, list)[0].Element
	if again := listMicroflowUnits(t, list)[0].Element; again != shared {
		t.Fatal("control: an unchanged element is not being shared, so the test below would pass without the guard")
	}
	stored := shared.Documentation()

	shared.SetDocumentation(stored + " — changed by one caller")
	if !shared.IsDirty() {
		t.Fatal("control: setting a property did not mark the element dirty")
	}

	next := listMicroflowUnits(t, list)[0].Element
	if next == shared {
		t.Fatal("the changed element was served to the next caller")
	}
	if got := next.Documentation(); got != stored {
		t.Errorf("the next caller read %q, want the stored %q", got, stored)
	}
	if next.IsDirty() {
		t.Error("the next caller was handed a dirty element")
	}
}

// A lookup that decodes one unit shares the entry a full listing keeps, and
// agrees with it.
func TestDecodeCache_ALookupAndAListingShareTheirUnits(t *testing.T) {
	r := openTestReader(t)
	list := func() ([]mprread.Unit[*genMf.Microflow], error) {
		return mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	}
	listMicroflowUnits(t, list)
	want := listMicroflowUnits(t, list)[0]

	d0 := mprread.UnitsDecoded()
	got, ok, err := mprread.UnitByID[*genMf.Microflow](r, model.ID(want.Element.ID()))
	if err != nil || !ok {
		t.Fatalf("UnitByID: ok=%v err=%v", ok, err)
	}
	if mprread.UnitsDecoded() != d0 {
		t.Error("looking a unit up by ID decoded it again after a listing had kept it")
	}
	if got.Element != want.Element || got.ContainerID != want.ContainerID {
		t.Error("the lookup and the listing returned different units for one ID")
	}
}

// A kept element is read by several goroutines at once (the catalog builds
// with a worker pool), which is only sound if reading it writes nothing. Every
// scalar property decodes lazily on its first read, so the cache loads them
// all before it shares the element. Under -race this fails if any is left.
func TestDecodeCache_AKeptElementCanBeReadConcurrently(t *testing.T) {
	r := openTestReader(t)
	list := func() ([]mprread.Unit[*genMf.Microflow], error) {
		return mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	}
	listMicroflowUnits(t, list)
	listMicroflowUnits(t, list)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			units, err := list()
			if err != nil {
				t.Error(err)
				return
			}
			for _, u := range units {
				_ = u.Element.Name()
				_ = u.Element.Documentation()
				// Every property of every element in the tree, as a converter
				// reading the whole document would.
				element.Walk(u.Element, func(e element.Element) bool {
					for _, p := range e.Properties() {
						if wp, ok := p.(element.WritableProperty); ok {
							_ = wp.BSONValue()
						}
					}
					return true
				})
			}
		}()
	}
	wg.Wait()
}
