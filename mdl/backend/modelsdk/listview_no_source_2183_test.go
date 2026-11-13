// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// studioProUnsetListViewSourceKeys is the key set of the DataSource of every
// Studio Pro-authored List View whose source is not yet configured: 13 of them
// in the page templates of testdata/testapp, 13 more in testdata/pedapp's (e.g.
// testapp mprcontents/30/12/30127bfe-68f5-40ca-ba13-a73943dd6285.mxunit,
// listView1), all identical —
//
//	{$ID, $Type: Forms$ListViewXPathSource, EntityRef: null,
//	 ForceFullObjects: false, Search: {SearchRefs: [2]},
//	 SortBar: {SortItems: [2]}, SourceVariable: null, XPathConstraint: ""}
var studioProUnsetListViewSourceKeys = []string{
	"$ID", "$Type", "EntityRef", "ForceFullObjects", "Search", "SortBar", "SourceVariable", "XPathConstraint",
}

// TestListViewWithoutSourceWritesStudioProShape is ako/mxcli#2183
// (mendixlabs/mxcli#1836): `use building block PlatformDesignSystem.Avatar_Group_SM`
// failed with "ListView source <nil> is not supported by either engine" — the
// block's list view has no data source configured, which Studio Pro stores as
// an XPath source naming no entity. Written that way, `mx check` asks the author
// to pick an entity, instead of mxcli refusing the whole block.
func TestListViewWithoutSourceWritesStudioProShape(t *testing.T) {
	lv, err := widgetToGen(&pages.ListView{PageSize: 10})
	if err != nil {
		t.Fatalf("list view without a data source: %v", err)
	}
	ds, ok := lookup(encodeD(t, lv), "DataSource").(bson.D)
	if !ok {
		t.Fatalf("DataSource missing or not a document")
	}
	if got := lookup(ds, "$Type"); got != "Forms$ListViewXPathSource" {
		t.Errorf("DataSource $Type = %v, want Forms$ListViewXPathSource", got)
	}
	keys := dKeys(ds)
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, studioProUnsetListViewSourceKeys) {
		t.Errorf("DataSource keys = %v, want Studio Pro's %v", keys, studioProUnsetListViewSourceKeys)
	}
	for _, k := range []string{"EntityRef", "SourceVariable"} {
		if v := lookup(ds, k); v != nil {
			t.Errorf("%s = %v, want null (no entity configured)", k, v)
		}
	}
	if v := lookup(ds, "XPathConstraint"); v != "" {
		t.Errorf("XPathConstraint = %v, want empty", v)
	}
	if v := lookup(ds, "ForceFullObjects"); v != false {
		t.Errorf("ForceFullObjects = %v, want false", v)
	}
	if m := lvListMarker(t, lookup(ds, "Search").(bson.D), "SearchRefs"); m != int32(2) {
		t.Errorf("Search.SearchRefs marker = %v, want 2", m)
	}
	if m := lvListMarker(t, lookup(ds, "SortBar").(bson.D), "SortItems"); m != int32(2) {
		t.Errorf("SortBar.SortItems marker = %v, want 2", m)
	}
}

// Control: a source type the writer does not know is still refused loudly —
// only the absent source changed meaning.
func TestListViewUnknownSourceStillRefused(t *testing.T) {
	_, err := listViewSourceToGen(&pages.ListenToWidgetSource{})
	if err == nil || !strings.Contains(err.Error(), "is not supported by either engine") {
		t.Errorf("want the unsupported-source refusal, got %v", err)
	}
}
