// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// storedUses reads flow references from a document's bytes, where the form
// before it decoded the whole document. It must find what that form finds, in
// its order: compared on every page, snippet, layout, nanoflow, menu and the
// navigation document of the fixture, after a script has added a page that
// calls a microflow and a nanoflow, and an excluded page that calls one, so
// that the comparison is not only of documents with nothing in them.
func TestStoredUses_FindsWhatDecodingFinds(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = `mdl 1;
create microflow MyFirstModule.UsesProbe_Mf () begin end;
create nanoflow MyFirstModule.UsesProbe_Nf () begin end;
create page MyFirstModule.UsesProbe_Page (title: 'Probe', layout: Atlas_Core.Atlas_Default) {
  container c1 {
    actionbutton b1 (caption: 'Run', action: microflow MyFirstModule.UsesProbe_Mf)
    actionbutton b2 (caption: 'Run', action: nanoflow MyFirstModule.UsesProbe_Nf)
  }
};
@excluded
create page MyFirstModule.UsesProbe_Excluded (title: 'Probe', layout: Atlas_Core.Atlas_Default) {
  actionbutton b1 (caption: 'Run', action: microflow MyFirstModule.UsesProbe_Mf)
};
`
	if err := runScript(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	docs, withRefs, refs, profiles, excluded := 0, 0, 0, 0, 0
	kinds := map[string]int{}
	for _, typ := range flowRefSourceTypes {
		units, err := exec.Backend().ListRawUnitsByType(typ)
		if err != nil {
			t.Fatalf("ListRawUnitsByType(%s): %v", typ, err)
		}
		for _, u := range units {
			if u.Type != typ || len(u.Contents) == 0 {
				continue
			}
			docs++
			got, want := storedUses(typ, u.Contents), storedUsesDecoded(typ, u.Contents)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s:\n got  %+v\n want %+v", typ, u.ID, got, want)
			}
			if len(want) == 0 {
				excluded++
			}
			for _, use := range want {
				if use.navigation {
					profiles++
				}
				if len(use.refs) > 0 {
					withRefs++
				}
				for _, r := range use.refs {
					refs++
					kinds[r.kind]++
				}
			}
		}
	}
	// A comparison of nothing with nothing passes too.
	if docs < 50 || withRefs < 3 || profiles == 0 || excluded == 0 || kinds["microflow"] == 0 || kinds["nanoflow"] == 0 {
		t.Fatalf("the fixture did not exercise the comparison: %d documents, %d with references, %d references %v, %d navigation profiles, %d excluded",
			docs, withRefs, refs, kinds, profiles, excluded)
	}
}

// The shapes the fixture may not hold: a reference inside an array of
// documents, a key of the right name holding something that is not a
// qualified name, and bytes that are not a document.
func TestStoredUses_ShapesTheFixtureMayNotHold(t *testing.T) {
	doc := bson.D{
		{Key: "Name", Value: "P"},
		{Key: "Microflow", Value: ""},
		{Key: "Nanoflow", Value: "NotQualified"},
		{Key: "Microflow", Value: bson.D{{Key: "Microflow", Value: "A.Inner"}}},
		{Key: "Widgets", Value: bson.A{
			int32(3),
			bson.D{{Key: "Action", Value: bson.D{{Key: "Nanoflow", Value: "A.Nf"}}}},
			bson.A{bson.D{{Key: "Microflow", Value: "A.Deep"}}},
			"A.NotUnderAKey",
		}},
		{Key: "Microflows", Value: "A.OtherKey"},
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := storedUses("Forms$Page", raw)
	want := []storedUse{{label: "P", refs: []flowRef{{"microflow", "A.Inner"}, {"nanoflow", "A.Nf"}, {"microflow", "A.Deep"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if dec := storedUsesDecoded("Forms$Page", raw); !reflect.DeepEqual(dec, want) {
		t.Fatalf("the decoding form disagrees: %+v", dec)
	}
	for _, bad := range [][]byte{nil, {1, 2, 3}, raw[:len(raw)-7]} {
		if got := storedUses("Forms$Page", bad); got != nil {
			t.Errorf("bytes that are not a document gave %+v", got)
		}
	}
}

// What the change is for: reading a page's references must not cost what
// decoding the page costs. Allocations, because they do not depend on the
// machine.
func TestStoredUses_DoesNotDecodeTheDocument(t *testing.T) {
	var widgets bson.A
	for i := 0; i < 400; i++ {
		widgets = append(widgets, bson.D{
			{Key: "$Type", Value: "Forms$ActionButton"},
			{Key: "Name", Value: "b"},
			{Key: "Appearance", Value: bson.D{{Key: "Class", Value: ""}, {Key: "Style", Value: ""}}},
			{Key: "Action", Value: bson.D{{Key: "Microflow", Value: "A.Flow"}}},
		})
	}
	raw, err := bson.Marshal(bson.D{{Key: "Name", Value: "P"}, {Key: "Widgets", Value: widgets}})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(storedUses("Forms$Page", raw)[0].refs); n != 400 {
		t.Fatalf("%d references, want 400", n)
	}
	scan := testing.AllocsPerRun(5, func() { storedUses("Forms$Page", raw) })
	decode := testing.AllocsPerRun(5, func() { storedUsesDecoded("Forms$Page", raw) })
	if scan*20 > decode {
		t.Fatalf("reading the references allocated %.0f times, decoding %.0f: the document is being decoded", scan, decode)
	}
}
