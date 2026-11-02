// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1264 (mendixlabs/mxcli#675) — the codec wrote Mendix "Integer"
// properties as BSON int32 where Studio Pro writes int64. Reported on a fresh
// 11.11.0 project:
//
//	Field                       Studio Pro / legacy writer   modelsdk codec
//	CanvasWidth / CanvasHeight  int64                        int32
//	PopupWidth / PopupHeight    int64                        int32
//
// and, within one page, CREATE (codec, int32) disagreed with ALTER (the page
// mutator, which writes int64). mx check accepts both, so nothing failed — the
// difference is only visible in the BSON, which is where these tests look.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

var pageIntegerHeaderKeys = []string{"CanvasWidth", "CanvasHeight", "PopupWidth", "PopupHeight"}

func rawUnit1264(t *testing.T, b *Backend, id model.ID) bson.Raw {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	return bson.Raw(raw)
}

// A page created through the codec stores its integer header at the width a
// Studio Pro page in the same project does. The reference is not a constant
// written into this test: it is read off the fixture's own pages, which
// `mx create-project` wrote (16 of 16 carry all four keys as int64).
func TestCreatePage_IntegerHeaderMatchesStudioProWidth(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	// The reference widths, before mxcli writes anything.
	all, err := b.ListPages()
	if err != nil || len(all) == 0 {
		t.Fatalf("ListPages = %d pages, %v", len(all), err)
	}
	want := map[string]bson.Type{}
	for _, key := range pageIntegerHeaderKeys {
		for _, p := range all {
			v, err := rawUnit1264(t, b, p.ID).LookupErr(key)
			if err != nil {
				continue
			}
			if prev, ok := want[key]; ok && prev != v.Type {
				t.Fatalf("Studio Pro pages disagree on %s: %s and %s", key, prev, v.Type)
			}
			want[key] = v.Type
		}
		if _, ok := want[key]; !ok {
			t.Fatalf("no Studio Pro page in the fixture carries %s — no reference to compare against", key)
		}
	}

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	page := &pages.Page{ContainerID: mod.ID, Name: "ZzPopup1264", PopupWidth: 640, PopupHeight: 480}
	if err := b.CreatePage(page); err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	raw := rawUnit1264(t, b, page.ID)
	for _, key := range pageIntegerHeaderKeys {
		v, err := raw.LookupErr(key)
		if err != nil {
			t.Errorf("%s: not written", key)
			continue
		}
		if v.Type != want[key] {
			t.Errorf("created page stores %s as BSON %s; Studio Pro's pages store it as %s", key, v.Type, want[key])
		}
	}
	// Control: the values are the ones authored, so the width check above is
	// not passing on a document that lost them.
	if got := raw.Lookup("PopupWidth").AsInt64(); got != 640 {
		t.Errorf("PopupWidth = %d, want 640", got)
	}
	if got := raw.Lookup("PopupHeight").AsInt64(); got != 480 {
		t.Errorf("PopupHeight = %d, want 480", got)
	}
}

// CREATE (the codec) and ALTER (the page mutator, raw BSON) write the pop-up
// dimensions at the same width. Before the fix CREATE wrote int32 and ALTER
// int64, so one page's two dimensions could differ in type depending on which
// statement last touched each.
func TestPopupDimensions_CreateAndAlterAgreeOnWidth(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	page := &pages.Page{ContainerID: mod.ID, Name: "ZzPopupAlter1264", PopupWidth: 640, PopupHeight: 480}
	if err := b.CreatePage(page); err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	created := rawUnit1264(t, b, page.ID).Lookup("PopupHeight").Type

	m, err := b.OpenPageForMutation(page.ID)
	if err != nil {
		t.Fatalf("OpenPageForMutation: %v", err)
	}
	if err := m.SetWidgetProperty("", "PopupWidth", 700); err != nil {
		t.Fatalf("set PopupWidth: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw := rawUnit1264(t, b, page.ID)
	altered := raw.Lookup("PopupWidth")
	if got := altered.AsInt64(); got != 700 {
		t.Fatalf("PopupWidth after ALTER = %d, want 700", got)
	}
	// PopupHeight was written by CREATE and left alone by ALTER; PopupWidth was
	// rewritten by ALTER. Same document, same kind of property, one width.
	if altered.Type != created || raw.Lookup("PopupHeight").Type != created {
		t.Errorf("CREATE wrote PopupHeight as %s, ALTER wrote PopupWidth as %s — the two write paths disagree",
			created, altered.Type)
	}
	if created != bson.TypeInt64 {
		t.Errorf("CREATE wrote PopupHeight as %s, want int64 (what Studio Pro stores)", created)
	}
}

// customHandlerFromRaw read LevelOfParent with Int32OK alone, so the parameter
// of a custom handler bound to an ancestor came back as level -1 for every
// Studio Pro document (120 of 120 LevelOfParent values in the census are
// int64) — and, once the codec writes int64 too, for mxcli's own documents.
func TestCustomHandlerFromRaw_ReadsLevelOfParentAtEitherWidth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		level any
	}{
		{"int64 (what Studio Pro stores)", int64(2)},
		{"int32 (what older mxcli builds stored)", int32(2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := bson.D{
				{Key: "$Type", Value: "Mappings$MappingMicroflowCallImpl"},
				{Key: "Microflow", Value: "M.Handler"},
				{Key: "ParameterMappings", Value: bson.A{int32(3), bson.D{
					{Key: "$Type", Value: "Mappings$MicroflowCallParameterMappingImpl"},
					{Key: "Parameter", Value: "M.Handler.Ancestor"},
					{Key: "LevelOfParent", Value: tc.level},
					{Key: "JsonValueElementPath", Value: ""},
					{Key: "XmlValueElementPath", Value: ""},
				}}},
			}
			raw, err := bson.Marshal(bson.D{{Key: "CustomHandlerCall", Value: call}})
			if err != nil {
				t.Fatal(err)
			}
			got := customHandlerFromRaw(raw)
			if got == nil || len(got.Parameters) != 1 {
				t.Fatalf("customHandlerFromRaw = %+v, want one parameter", got)
			}
			p := got.Parameters[0]
			if p.LevelOfParent != 2 || p.Source != "ancestor" {
				t.Errorf("parameter = {Source: %q, LevelOfParent: %d}, want {ancestor, 2}", p.Source, p.LevelOfParent)
			}
		})
	}
}

// The writer and the reader of a custom handler agree: what customHandlerToGen
// encodes, customHandlerFromRaw reads back — whatever width the encoder picks.
func TestCustomHandler_AncestorLevelRoundTrips(t *testing.T) {
	in := &model.MappingMicroflowCall{
		Microflow: "M.Handler",
		Parameters: []*model.MappingMicroflowParameter{
			{Parameter: "M.Handler.Ancestor", Source: "ancestor", LevelOfParent: 2},
		},
	}
	out, err := (&codec.Encoder{}).Encode(customHandlerToGen(in))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw, err := bson.Marshal(bson.D{{Key: "CustomHandlerCall", Value: bson.Raw(out)}})
	if err != nil {
		t.Fatal(err)
	}
	got := customHandlerFromRaw(raw)
	if got == nil || len(got.Parameters) != 1 {
		t.Fatalf("customHandlerFromRaw = %+v, want one parameter", got)
	}
	if p := got.Parameters[0]; p.Source != "ancestor" || p.LevelOfParent != 2 {
		t.Errorf("round trip gave {Source: %q, LevelOfParent: %d}, want {ancestor, 2}", p.Source, p.LevelOfParent)
	}
}
