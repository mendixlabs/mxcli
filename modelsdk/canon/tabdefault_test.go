// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1897: a describe → create or replace round trip rewrote a page
// although nothing changed. Measured on TestApp's Administration.Account_Overview:
// the one difference was its tab container's DefaultPagePointer — stored null
// (Studio Pro's "no default set"), written as the first tab page. MDL has no
// syntax for the default tab and DESCRIBE prints none, so a stored default
// other than the first was reset on every round trip too. The stored pointer is
// carried while it still means something: null, or a tab page the rebuilt
// control still has.

func id16(b byte) bson.Binary {
	d := bytes.Repeat([]byte{b}, 16)
	return bson.Binary{Subtype: 0, Data: d}
}

var zero16 = bson.Binary{Subtype: 0, Data: make([]byte, 16)}

func tabControl(id bson.Binary, def bson.Binary, tabs ...bson.Binary) bson.D {
	var pages bson.A
	pages = append(pages, int32(2))
	for _, t := range tabs {
		pages = append(pages, bson.D{{Key: "$ID", Value: t}, {Key: "$Type", Value: "Forms$TabPage"}})
	}
	return bson.D{
		{Key: "$ID", Value: id},
		{Key: "$Type", Value: "Forms$TabControl"},
		{Key: "DefaultPagePointer", Value: def},
		{Key: "TabPages", Value: pages},
	}
}

func pageWith(t *testing.T, ctl bson.D) []byte {
	return mustMarshal(t, bson.D{
		{Key: "$ID", Value: id16(0xAA)},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Widgets", Value: bson.A{int32(2), ctl}},
	})
}

func defaultPointerOf(t *testing.T, doc []byte) []byte {
	t.Helper()
	var d struct {
		Widgets []any `bson:"Widgets"`
	}
	if err := bson.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	ctl := d.Widgets[1].(bson.D)
	for _, e := range ctl {
		if e.Key == "DefaultPagePointer" {
			return e.Value.(bson.Binary).Data
		}
	}
	t.Fatal("no DefaultPagePointer")
	return nil
}

func TestCarryDefaultTab_NullStaysNullAndTheWriteIsElided(t *testing.T) {
	stored := pageWith(t, tabControl(id16(1), zero16, id16(2), id16(3)))
	written := pageWith(t, tabControl(id16(1), id16(2), id16(2), id16(3)))

	// Control: without the carry the two differ — the reported rewrite.
	if eq, _ := Equal(written, stored); eq {
		t.Fatal("control: the documents should differ before the carry")
	}
	out, unchanged := Reconcile(written, stored)
	if !unchanged {
		t.Errorf("a round trip that changed nothing was not elided; pointer now %x", defaultPointerOf(t, out))
	}
}

func TestCarryDefaultTab_KeepsAStoredSecondTab(t *testing.T) {
	stored := pageWith(t, tabControl(id16(1), id16(3), id16(2), id16(3)))
	written := pageWith(t, tabControl(id16(1), id16(2), id16(2), id16(3)))
	out := CarryDefaultTabPages(written, stored)
	if got := defaultPointerOf(t, out); !bytes.Equal(got, id16(3).Data) {
		t.Errorf("default tab = %x, want the stored second tab %x", got, id16(3).Data)
	}
}

// A stored default naming a tab the rebuild no longer has is not carried: it
// would dangle.
func TestCarryDefaultTab_DoesNotCarryARemovedTab(t *testing.T) {
	stored := pageWith(t, tabControl(id16(1), id16(3), id16(2), id16(3)))
	written := pageWith(t, tabControl(id16(1), id16(2), id16(2)))
	out := CarryDefaultTabPages(written, stored)
	if got := defaultPointerOf(t, out); !bytes.Equal(got, id16(2).Data) {
		t.Errorf("default tab = %x, want the written first tab kept", got)
	}
}

// A tab control that is not the stored one (another $ID) gets nothing.
func TestCarryDefaultTab_OnlyTheSameControl(t *testing.T) {
	stored := pageWith(t, tabControl(id16(1), zero16, id16(2)))
	written := pageWith(t, tabControl(id16(9), id16(2), id16(2)))
	out := CarryDefaultTabPages(written, stored)
	if got := defaultPointerOf(t, out); !bytes.Equal(got, id16(2).Data) {
		t.Errorf("a different control took the stored default: %x", got)
	}
}
