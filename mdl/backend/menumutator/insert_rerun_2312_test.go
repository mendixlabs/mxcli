// SPDX-License-Identifier: Apache-2.0

package menumutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#2312: `alter navigation … { insert { menu item 'Helpdesk chat' (…) } }`
// run twice put the item in the menu twice, and the second run reported
// "Altered navigation" — a project script that is not re-runnable. An item the
// list already holds, as the statement builds it, is now already there.

func menuDocWith(items ...bson.D) bson.D {
	arr := bson.A{int32(3)}
	for _, it := range items {
		arr = append(arr, it)
	}
	return bson.D{{Key: "$Type", Value: "Menus$MenuDocument"}, {Key: "ItemCollection", Value: bson.D{
		{Key: "$Type", Value: "Menus$MenuItemCollection"}, {Key: "Items", Value: arr}}}}
}

func TestInsertMenuItems_RerunInsertsNothing_Issue2312(t *testing.T) {
	m, err := NewMenuDocument(menuDocWith(item("Home")), "u", &fakeDeps{})
	if err != nil {
		t.Fatal(err)
	}
	spec := []types.NavMenuItemSpec{{Caption: "Helpdesk chat"}}
	for run := 0; run < 2; run++ {
		if err := m.InsertMenuItems(nil, len(m.MenuTree()), spec); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
	}
	if tree := m.MenuTree(); len(tree) != 2 || tree[1].Caption != "Helpdesk chat" {
		t.Errorf("tree after two runs = %+v, want Home and one Helpdesk chat", captions(tree))
	}
}

// actionDeps builds an item whose action differs from the stored one's.
type actionDeps struct{ fakeDeps }

func (actionDeps) BuildMenuItem(spec types.NavMenuItemSpec) (bson.D, error) {
	it := item(spec.Caption)
	for i, e := range it {
		if e.Key == "Action" {
			it[i].Value = bson.D{{Key: "$Type", Value: "Forms$FormAction"}, {Key: "FormSettings", Value: "Helpdesk.Chat"}}
		}
	}
	return it, nil
}

// An item of the same caption that the statement builds differently is not a
// re-run: inserting it would leave two items of one caption, and skipping it
// would drop the statement's change. It is refused, naming replace.
func TestInsertMenuItems_SameCaptionDifferentItemRefused_Issue2312(t *testing.T) {
	m, err := NewMenuDocument(menuDocWith(item("Helpdesk chat")), "u", &actionDeps{})
	if err != nil {
		t.Fatal(err)
	}
	err = m.InsertMenuItems(nil, 1, []types.NavMenuItemSpec{{Caption: "Helpdesk chat"}})
	if err == nil || !strings.Contains(err.Error(), "replace") {
		t.Errorf("err = %v, want a refusal naming replace", err)
	}
	if tree := m.MenuTree(); len(tree) != 1 {
		t.Errorf("tree = %v, want it unchanged", captions(m.MenuTree()))
	}
}

// CONTROL: an item whose caption is not in the list is inserted, and the same
// caption in another list (a sub-menu) is not this list's.
func TestInsertMenuItems_NewCaptionInserted_Issue2312(t *testing.T) {
	m, err := NewMenuDocument(menuDocWith(item("Home", item("Helpdesk chat"))), "u", &fakeDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.InsertMenuItems(nil, 1, []types.NavMenuItemSpec{{Caption: "Helpdesk chat"}}); err != nil {
		t.Fatal(err)
	}
	if tree := m.MenuTree(); len(tree) != 2 {
		t.Errorf("tree = %v, want the top-level item added", captions(tree))
	}
}

func captions(tree []*backend.MenuTreeNode) []string {
	var out []string
	for _, n := range tree {
		out = append(out, n.Caption)
	}
	return out
}
