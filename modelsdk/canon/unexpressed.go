// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CarryUnexpressed returns contents with two values no MDL statement can state
// put back from the stored element of the same $ID, so a describe → create or
// replace round trip that changed nothing writes nothing (ako/mxcli#1897):
//
//   - An Expression that differs from the stored one only in leading or
//     trailing whitespace. MDL writes an expression bare, so a stored "true\n"
//     came back as "true" — measured on TestApp's WorkflowCommons pages, in
//     visibility conditions and page-call parameter mappings.
//   - A property written null where the stored element holds an EMPTY
//     Forms$ClientTemplate — no text in any language and no parameters. Studio
//     Pro stores a widget's unset screen-reader label that way; the writer
//     writes null. Both mean "no label".
//
// Only elements whose $ID the stored document already has take anything, which
// after TransplantIDs means the elements that correspond; a real change — other
// expression text, a label with text — is not touched and still lands.
func CarryUnexpressed(contents, stored []byte) []byte {
	if len(contents) == 0 || len(stored) == 0 {
		return contents
	}
	var storedDoc, newDoc bson.D
	if bson.Unmarshal(stored, &storedDoc) != nil || bson.Unmarshal(contents, &newDoc) != nil {
		return contents
	}
	byID := map[string]bson.D{}
	walkElements(storedDoc, func(d bson.D) {
		if id, ok := docLookup(d, "$ID").(bson.Binary); ok {
			byID[string(id.Data)] = d
		}
	})
	used := elementIDs(newDoc)

	changed := false
	walkElements(newDoc, func(d bson.D) {
		id, ok := docLookup(d, "$ID").(bson.Binary)
		if !ok {
			return
		}
		old, ok := byID[string(id.Data)]
		if !ok {
			return
		}
		for i, e := range d {
			ov := docLookup(old, e.Key)
			switch v := e.Value.(type) {
			case string:
				s, isStr := ov.(string)
				if e.Key == "Expression" && isStr && s != v && strings.TrimSpace(s) == strings.TrimSpace(v) {
					d[i].Value = s
					changed = true
				}
			case nil:
				if t, isDoc := ov.(bson.D); isDoc && isEmptyClientTemplate(t) && !anyIDUsed(t, used) {
					d[i].Value = t
					changed = true
				}
			}
		}
	})
	if !changed {
		return contents
	}
	out, err := bson.Marshal(newDoc)
	if err != nil {
		return contents
	}
	return out
}

// walkElements calls fn on every embedded document, depth first. The bson.D
// values are live: a change through one changes the document.
func walkElements(v any, fn func(bson.D)) {
	switch n := v.(type) {
	case bson.D:
		fn(n)
		for _, e := range n {
			walkElements(e.Value, fn)
		}
	case bson.A:
		for _, e := range n {
			walkElements(e, fn)
		}
	}
}

// isEmptyClientTemplate reports whether t is a Forms$ClientTemplate with no
// parameters and no translation in its Template or Fallback.
func isEmptyClientTemplate(t bson.D) bool {
	if ty, _ := docLookup(t, "$Type").(string); ty != "Forms$ClientTemplate" {
		return false
	}
	if hasItems(docLookup(t, "Parameters")) {
		return false
	}
	for _, k := range []string{"Template", "Fallback"} {
		if txt, ok := docLookup(t, k).(bson.D); ok && hasItems(docLookup(txt, "Items")) {
			return false
		}
	}
	return true
}

// hasItems reports whether a Mendix list holds anything past its leading
// type marker.
func hasItems(v any) bool {
	a, ok := v.(bson.A)
	return ok && len(a) > 1
}

// anyIDUsed reports whether any element id inside d is already in the
// document: carrying it in would make two elements share an identity.
func anyIDUsed(d bson.D, used map[string]bool) bool {
	clash := false
	walkElements(d, func(e bson.D) {
		if id, ok := docLookup(e, "$ID").(bson.Binary); ok && used[string(id.Data)] {
			clash = true
		}
	})
	return clash
}
