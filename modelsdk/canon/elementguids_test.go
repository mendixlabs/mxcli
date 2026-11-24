// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"os"
	"testing"
)

// ElementGUIDs is what a CDC writer reads the baseline's identities from, so it
// is checked against a real Studio Pro-authored domain model rather than
// something this repo encoded: an element mxcli created has GUID == $ID from
// birth, and a reader that returned the $ID would look correct on it.
//
// The fixture is CdcPublish from ako/TestApp at 619864b (Mendix 11.16.1), the
// module whose GUIDs the CDC fixture's baseline references.
func TestElementGUIDs_ReadsStudioProDomainModel(t *testing.T) {
	raw, err := os.ReadFile("testdata/domainmodel-CdcPublish.mxunit")
	if err != nil {
		t.Fatal(err)
	}
	got := ElementGUIDs(raw)
	if len(got) == 0 {
		t.Fatal("no GUIDs read from a unit that has 13 of them")
	}

	// The values the CDC baseline references, verified independently against
	// the fixture JSON.
	want := map[string]bool{
		"057964e1-fd78-4e74-bd66-c74a06131511": false, // entity Order
		"bae3f172-aebb-49a6-ad92-783f53cf5014": false, // entity Customer
		"f3bcaa9f-4ed8-46de-9b7d-dad81425bc33": false, // attribute Order.OrderId
		"4057dfad-680c-44cb-bfd2-97706b919d3e": false, // association Order_Customer
	}
	for id, guid := range got {
		if _, interesting := want[guid]; interesting {
			want[guid] = true
		}
		if id == guid {
			t.Errorf("element %s: GUID equals the $ID, so this fixture cannot detect a reader that returns the $ID", id)
		}
	}
	for guid, found := range want {
		if !found {
			t.Errorf("GUID %s not read; got %d GUIDs", guid, len(got))
		}
	}
}

// A unit that does not unmarshal gives nil, not a panic: the caller has to be
// able to tell "could not read" from "nothing to read".
func TestElementGUIDs_GarbageIsNil(t *testing.T) {
	if got := ElementGUIDs([]byte("not bson")); got != nil {
		t.Errorf("want nil for unparseable input, got %v", got)
	}
}
