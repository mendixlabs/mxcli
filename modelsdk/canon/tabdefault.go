// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"

	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// tabControlType is the storage name of a page's tab container.
const tabControlType = "Forms$TabControl"

// CarryDefaultTabPages returns contents with each tab container's
// DefaultPagePointer set to the value the stored container of the same $ID
// holds, when that value still means something in the written document: null
// (Studio Pro's "no default set"), or the $ID of a tab page the written
// container still has.
//
// MDL has no syntax for a tab container's default tab and DESCRIBE prints none,
// so the writer points it at the first tab page. A describe → create or
// replace round trip therefore rewrote a page whose stored pointer was null —
// the write elision saw a difference (ako/mxcli#1897) — and reset a stored
// default other than the first on every pass. Run after TransplantIDs, which is
// what makes the written container and its tab pages carry their stored $IDs.
//
// The patch is in place over a fixed-width 16-byte binary, like the $ID
// transplant; nothing is re-marshalled. A document that cannot be read passes
// through untouched.
func CarryDefaultTabPages(contents, stored []byte) []byte {
	want := storedDefaultTabs(bsoncore.Document(stored))
	if len(want) == 0 {
		return contents
	}
	out := append([]byte(nil), contents...)
	if !carryDefaultTabs(bsoncore.Document(out), want) {
		return contents
	}
	return out
}

// storedDefaultTabs maps each stored tab container's $ID to its
// DefaultPagePointer.
func storedDefaultTabs(doc bsoncore.Document) map[string][]byte {
	m := map[string][]byte{}
	walkDocs(doc, func(d bsoncore.Document) {
		if docType(d) != tabControlType {
			return
		}
		id, ok := binaryField(d, "$ID")
		if !ok {
			return
		}
		if def, ok := binaryField(d, "DefaultPagePointer"); ok {
			m[string(id)] = append([]byte(nil), def...)
		}
	})
	return m
}

// carryDefaultTabs patches each written tab container that has a stored
// counterpart; false when the document cannot be read.
func carryDefaultTabs(doc bsoncore.Document, want map[string][]byte) bool {
	if _, err := doc.Elements(); err != nil {
		return false
	}
	walkDocs(doc, func(d bsoncore.Document) {
		if docType(d) != tabControlType {
			return
		}
		id, found := binaryField(d, "$ID")
		if !found {
			return
		}
		stored, found := want[string(id)]
		if !found {
			return
		}
		def, found := binaryField(d, "DefaultPagePointer")
		if !found || bytes.Equal(def, stored) {
			return
		}
		if !isNullID(stored) && !hasTabPage(d, stored) {
			return // the stored default is gone; carrying it would dangle
		}
		copy(def, stored) // aliases the buffer: an in-place patch
	})
	return true
}

func isNullID(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// hasTabPage reports whether the container lists a tab page with this $ID.
func hasTabPage(ctl bsoncore.Document, id []byte) bool {
	v, err := ctl.LookupErr("TabPages")
	if err != nil {
		return false
	}
	arr, ok := v.ArrayOK()
	if !ok {
		return false
	}
	vals, err := arr.Values()
	if err != nil {
		return false
	}
	for _, e := range vals {
		if sub, ok := e.DocumentOK(); ok {
			if pid, ok := binaryField(sub, "$ID"); ok && bytes.Equal(pid, id) {
				return true
			}
		}
	}
	return false
}

// walkDocs calls fn on doc and on every document nested in it, depth first.
func walkDocs(doc bsoncore.Document, fn func(bsoncore.Document)) {
	fn(doc)
	elems, err := doc.Elements()
	if err != nil {
		return
	}
	for _, e := range elems {
		v := e.Value()
		switch v.Type {
		case bsoncore.TypeEmbeddedDocument:
			if sub, ok := v.DocumentOK(); ok {
				walkDocs(sub, fn)
			}
		case bsoncore.TypeArray:
			if arr, ok := v.ArrayOK(); ok {
				walkDocs(bsoncore.Document(arr), fn)
			}
		}
	}
}

func docType(d bsoncore.Document) string {
	v, err := d.LookupErr("$Type")
	if err != nil {
		return ""
	}
	s, _ := v.StringValueOK()
	return s
}

// binaryField returns a 16-byte binary property's bytes, aliasing the
// document's buffer so a caller can patch it in place.
func binaryField(d bsoncore.Document, key string) ([]byte, bool) {
	v, err := d.LookupErr(key)
	if err != nil {
		return nil, false
	}
	_, data, ok := v.BinaryOK()
	if !ok || len(data) != 16 {
		return nil, false
	}
	return data, true
}
