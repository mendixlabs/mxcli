// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// namedItemList is a stored list of named elements: the document holding it
// and the key it is under, the pair findInWidgetArray takes.
type namedItemList struct {
	doc bson.D
	key string
}

// legacyGridItemLists returns the named-item lists of a legacy (Dojo)
// Forms$DataGrid other than its control bar, which the finder already walks:
// the grid's Columns (Forms$GridColumn) and its source's SearchBar.Items
// (Forms$*SearchField). Each item carries a stored Name, so `drop <name>` and
// `set (Caption: …) on <name>` address them like any widget. Without this both
// were unreachable and the only remedy was replacing the grid
// (mendixlabs/mxcli#1459).
//
// Gated on the $Type: a LayoutGrid row also has a `Columns` key, holding
// layout columns rather than named grid columns.
func legacyGridItemLists(wDoc bson.D) []namedItemList {
	if bsonnav.DGetString(wDoc, "$Type") != "Forms$DataGrid" {
		return nil
	}
	lists := []namedItemList{{wDoc, "Columns"}}
	if ds := bsonnav.DGetDoc(wDoc, "DataSource"); ds != nil {
		if sb := bsonnav.DGetDoc(ds, "SearchBar"); sb != nil {
			for _, key := range controlBarItemKeys {
				lists = append(lists, namedItemList{sb, key})
			}
		}
	}
	return lists
}

// controlBarItemKeys are the keys a control bar or search bar holds its
// buttons and fields under. Forms$GridControlBar.Items and
// Forms$SearchBar.Items are STORED as `NewButtons` (the storage name, read from
// OIDC SSO 2.3.2's Snip_Configuration; modelsdk/gen/keyaudit_test.go lists
// both). Walking only `Items` left every real control-bar button and search
// field "not found" (ako/mxcli#2382); `Items` stays for any shape that uses the
// metamodel name.
var controlBarItemKeys = []string{"NewButtons", "Items"}
