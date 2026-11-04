// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"path/filepath"
	"testing"

	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1373: Mendix's merge and diff engine compares two revisions
// of an element by their property NAMES and throws when they differ. Every key
// the writer leaves out (or writes that Mendix does not have) is a document that
// cannot be merged once Studio Pro has saved it. These tests pin each gap the
// `mx convert` measurement found at the layer that writes it.

// TestIssue1373_RewrittenLoopKeepsItsPropertySet is the issue's own microflow
// case, end to end through storage: a Studio Pro loop is rebuilt (moved, so the
// write is not elided), keeps its $ID through the transplant, and must keep
// Documentation — the key whose absence made `mx diff` throw.
func TestIssue1373_RewrittenLoopKeepsItsPropertySet(t *testing.T) {
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../testdata/testapp-views"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	proj := filepath.Join(dst, "TestApp.mpr")
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}

	mf, loop := studioProLoop(t, b)
	storedKeys := elementKeys(t, b, mf.ID, loop.ID)
	if !storedKeys["Documentation"] {
		t.Fatalf("subject: stored loop has no Documentation (%v) — need a Studio Pro-authored loop", storedKeys)
	}
	loop.Position.X += 10 // a real change, so the write lands
	if err := b.UpdateMicroflow(mf); err != nil {
		t.Fatalf("UpdateMicroflow: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	got := elementKeys(t, b2, mf.ID, loop.ID)
	if got == nil {
		t.Fatalf("loop %s lost its $ID on the rewrite — the subject no longer exercises the transplant", loop.ID)
	}
	for k := range storedKeys {
		if !got[k] {
			t.Errorf("rewritten loop lost %q: Mendix's merge engine refuses to compare it with the stored revision", k)
		}
	}
	for k := range got {
		if !storedKeys[k] {
			t.Errorf("rewritten loop gained %q the stored revision does not have", k)
		}
	}
}

func TestIssue1373_LoopWritesDocumentation(t *testing.T) {
	la := &microflows.LoopedActivity{Documentation: "keeps its notes"}
	la.ID = "aaaaaaaa-0000-4000-8000-000000000001"
	m := encodeToMap(t, microflowObjectToGen(la))
	if m["Documentation"] != "keeps its notes" {
		t.Fatalf("Documentation = %#v", m["Documentation"])
	}
	la.Documentation = ""
	if _, ok := encodeToMap(t, microflowObjectToGen(la))["Documentation"]; !ok {
		t.Fatal("an empty Documentation must still be written")
	}
}

// The default tab is stored as DefaultPagePointer; gen bound it as DefaultPage,
// a key Mendix drops, so the chosen default tab never reached the model.
func TestIssue1373_TabControlWritesDefaultPagePointer(t *testing.T) {
	tc := &pages.TabContainer{TabPages: []*pages.TabPage{{Name: "one"}, {Name: "two"}}}
	tc.Name = "tabs"
	el, err := widgetToGen(tc)
	if err != nil {
		t.Fatal(err)
	}
	m := encodeToMap(t, el)
	if _, ok := m["DefaultPage"]; ok {
		t.Error("wrote DefaultPage, a key Forms$TabControl does not have")
	}
	if _, ok := m["DefaultPagePointer"]; !ok {
		t.Errorf("no DefaultPagePointer; keys %v", keysOf(m))
	}
}

// A Title shows its page's title; Forms$Title declares no Caption.
func TestIssue1373_TitleWritesNoCaption(t *testing.T) {
	ti := &pages.Title{Caption: &model.Text{Translations: map[string]string{"en_US": "x"}}}
	ti.Name = "title1"
	el, err := widgetToGen(ti)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := encodeToMap(t, el)["Caption"]; ok {
		t.Error("wrote Caption, a key Forms$Title does not have")
	}
}

// Forms$NavigationListItem declares no Name; Studio Pro stores none.
func TestIssue1373_NavigationListItemWritesNoName(t *testing.T) {
	el, err := navListItemToGen(&pages.NavigationListItem{Name: "item1", Action: &pages.NoClientAction{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := encodeToMap(t, el)["Name"]; ok {
		t.Error("wrote Name, a key Forms$NavigationListItem does not have")
	}
}

// LastSelectedQuery exists from 10.0 to 11.10 (measured with `mx convert`).
func TestIssue1373_DatabaseConnectionLastSelectedQueryByVersion(t *testing.T) {
	conn := &model.DatabaseConnection{Name: "Db"}
	conn.ID = "aaaaaaaa-0000-4000-8000-000000000003"
	if _, ok := encodeToMap(t, databaseConnectionToGen(conn, false, true))["LastSelectedQuery"]; !ok {
		t.Error("a project that declares LastSelectedQuery did not get it")
	}
	if _, ok := encodeToMap(t, databaseConnectionToGen(conn, false, false))["LastSelectedQuery"]; ok {
		t.Error("a project that does not declare LastSelectedQuery got it")
	}
}

// studioProLoop returns a microflow of the Studio Pro-authored fixture and the
// loop in it.
func studioProLoop(t *testing.T, b *Backend) (*microflows.Microflow, *microflows.LoopedActivity) {
	t.Helper()
	list, err := b.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range list {
		mf, err := b.GetMicroflow(l.ID)
		if err != nil || mf.ObjectCollection == nil {
			continue
		}
		for _, o := range mf.ObjectCollection.Objects {
			if la, ok := o.(*microflows.LoopedActivity); ok {
				return mf, la
			}
		}
	}
	t.Fatal("no microflow with a loop in the fixture")
	return nil, nil
}

// elementKeys returns the BSON keys of the element with the given $ID in a
// stored unit, or nil when no element has it.
func elementKeys(t *testing.T, b *Backend, unit, id model.ID) map[string]bool {
	t.Helper()
	raw, err := b.GetRawUnitBytes(unit)
	if err != nil {
		t.Fatal(err)
	}
	var doc bsonv2.D
	if err := bsonv2.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var found map[string]bool
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bsonv2.D:
			keys := map[string]bool{}
			match := false
			for _, e := range x {
				keys[e.Key] = true
				if e.Key == "$ID" {
					if bin, ok := e.Value.(bsonv2.Binary); ok && codec.BinaryToUUID(bin.Data) == string(id) {
						match = true
					}
				}
				walk(e.Value)
			}
			if match {
				found = keys
			}
		case bsonv2.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return found
}

// `Response: none` is written as an empty ContentType. Absent, Mendix fills it
// with application/json — and so does the property-set completion — which read
// back as `Response: json` (TestRoundtripRestClient_* in CI).
func TestIssue1373_ResponseNoneWritesEmptyContentType(t *testing.T) {
	m := encodeToMap(t, restResponseHandlingToGen("NONE"))
	if v, ok := m["ContentType"]; !ok || v != "" {
		t.Fatalf("ContentType = %#v (present %v), want an explicit empty string", v, ok)
	}
	// Control: a declared type keeps its content type.
	if v := encodeToMap(t, restResponseHandlingToGen("JSON"))["ContentType"]; v != "application/json" {
		t.Errorf("json: ContentType = %#v", v)
	}
}
