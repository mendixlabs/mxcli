// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"reflect"
	"strings"
	"testing"
)

// ListUnitHeadersWhere filters on the name inside the listing, so that a
// lookup of one document does not copy the header of every document of the
// type (ako/mxcli#2160). It must give what filtering the whole listing gives,
// in its order.
func TestListUnitHeadersWhere_IsTheListingFiltered(t *testing.T) {
	const n = 40
	p, _ := seedProject(t, n)
	r := p.reader()
	all, err := r.ListUnitHeaders("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n {
		t.Fatalf("listed %d headers, want %d", len(all), n)
	}
	one, upper := all[n/2].Name, strings.ToUpper(all[n/3].Name)
	keeps := map[string]func(string) bool{
		"one name":      func(name string) bool { return name == one },
		"a folded name": func(name string) bool { return strings.EqualFold(name, upper) },
		"half of them":  func(name string) bool { return len(name) > 0 && name[len(name)-1]%2 == 0 },
		"none":          func(string) bool { return false },
		"all":           func(string) bool { return true },
	}
	matched := map[string]int{}
	for label, keep := range keeps {
		var want []UnitHeaderRef
		for _, h := range all {
			if keep(h.Name) {
				want = append(want, h)
			}
		}
		got, err := r.ListUnitHeadersWhere("Microflows$Microflow", keep)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %d headers %+v, want %d %+v", label, len(got), got, len(want), want)
		}
		matched[label] = len(want)
	}
	// The filters were not all "everything" or all "nothing".
	if matched["one name"] != 1 || matched["a folded name"] != 1 || matched["none"] != 0 || matched["all"] != n ||
		matched["half of them"] == 0 || matched["half of them"] == n {
		t.Fatalf("the filters did not divide the listing: %v", matched)
	}
	// nil keeps all, and another type is still another type.
	if got, _ := r.ListUnitHeadersWhere("Microflows$Microflow", nil); !reflect.DeepEqual(got, all) {
		t.Error("a nil filter is not the whole listing")
	}
	if got, _ := r.ListUnitHeadersWhere("Projects$Folder", keeps["all"]); len(got) != 0 {
		t.Errorf("%d headers of a type the project has none of", len(got))
	}
}
