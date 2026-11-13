// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"fmt"
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
// and, measured on FactoryManagement's pages (ako/mxcli#2106):
//
//   - A DynamicClasses expression or an XPathConstraint that differs only in
//     whitespace outside its string literals. Studio Pro keeps the author's
//     line breaks and spacing; MDL prints and rebuilds them normalised.
//   - An enumeration condition's values in another order, each with the same
//     flag. The writer lists them in enumeration order, Studio Pro in the
//     order they were added.
//
// and, measured on TestApp's WorkflowCommons snippets (ako/mxcli#2128):
//
//   - A screen-reader label whose translations all have empty text.
//   - A condition list with values mxcli's metadata lacks, the known ones
//     carrying the same flags (storedConditionsExtend).
//
// and, from a drop + create of a flow (ako/mxcli#2179):
//
//   - A loop's Size. MDL has no way to state it, so a rebuild wrote the size
//     mxcli computes and a loop drawn larger in Studio Pro overlapped its
//     neighbours (carryLoopSize).
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
		if carryUnstatedFlowProperties(d, old, used) {
			changed = true
		}
		if carryLoopSize(d, old) {
			changed = true
		}
		for i, e := range d {
			ov := docLookup(old, e.Key)
			switch v := e.Value.(type) {
			case string:
				s, isStr := ov.(string)
				if isStr && s != v && whitespaceInsensitive[e.Key] && withoutWhitespace(s) == withoutWhitespace(v) {
					d[i].Value = s
					changed = true
				}
			case bson.A:
				stored, isList := ov.(bson.A)
				if !isList {
					continue
				}
				if e.Key == "Conditions" && (sameConditionsReordered(v, stored) || storedConditionsExtend(v, stored)) &&
					!anyIDUsedOutside(stored, used, v) {
					d[i].Value = stored
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

// carryLoopSize keeps a loop's stored Size, per dimension never smaller than
// the size written: a box drawn larger in Studio Pro stays as drawn, and a body
// that grew past it still gets the room the writer computed for it, so the
// loop's own activities never end up outside its box.
func carryLoopSize(d, old bson.D) bool {
	if docLookup(d, "$Type") != "Microflows$LoopedActivity" {
		return false
	}
	ws, _ := docLookup(d, "Size").(string)
	ss, _ := docLookup(old, "Size").(string)
	if ws == "" || ss == "" || ws == ss {
		return false
	}
	ww, wh, ok1 := parseSize(ws)
	sw, sh, ok2 := parseSize(ss)
	if !ok1 || !ok2 {
		return false
	}
	size := fmt.Sprintf("%d;%d", max(ww, sw), max(wh, sh))
	if size == ws {
		return false
	}
	for i := range d {
		if d[i].Key == "Size" {
			d[i].Value = size
		}
	}
	return true
}

// parseSize reads a stored "width;height".
func parseSize(s string) (int, int, bool) {
	w, h, ok := strings.Cut(s, ";")
	if !ok {
		return 0, 0, false
	}
	var a, b int
	if _, err := fmt.Sscanf(w+" "+h, "%d %d", &a, &b); err != nil {
		return 0, 0, false
	}
	return a, b, true
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
		if txt, ok := docLookup(t, k).(bson.D); ok && hasText(docLookup(txt, "Items")) {
			return false
		}
	}
	return true
}

// hasText reports whether a Texts$Text item list holds a translation with any
// text. Studio Pro also stores an unset label as one translation of "" (TestApp
// WorkflowCommons.Snip_Assignee_Search, ako/mxcli#2128); that is no label too.
func hasText(v any) bool {
	a, ok := v.(bson.A)
	if !ok {
		return false
	}
	for _, item := range a[min(1, len(a)):] {
		tr, ok := item.(bson.D)
		if !ok {
			return true
		}
		if txt, _ := docLookup(tr, "Text").(string); txt != "" {
			return true
		}
	}
	return false
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

// whitespaceInsensitive names the expression-valued properties whose stored
// whitespace MDL does not reproduce.
var whitespaceInsensitive = map[string]bool{"Expression": true, "DynamicClasses": true, "XPathConstraint": true}

// withoutWhitespace drops whitespace outside single-quoted string literals, so
// two spellings of one expression compare equal while a changed literal does
// not. A doubled quote inside a literal closes and reopens it, which leaves
// the literal's text intact.
func withoutWhitespace(expr string) string {
	var b strings.Builder
	inLiteral := false
	for _, r := range expr {
		if r == '\'' {
			inLiteral = !inLiteral
		}
		if !inLiteral && (r == ' ' || r == '\t' || r == '\n' || r == '\r') {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sameConditionsReordered reports whether two Enumerations$Condition lists hold
// the same values with the same flags in a different order.
func sameConditionsReordered(written, stored bson.A) bool {
	key := func(a bson.A) ([]string, bool) {
		var out []string
		for _, item := range a[min(1, len(a)):] {
			c, ok := item.(bson.D)
			if !ok || docLookup(c, "$Type") != "Enumerations$Condition" {
				return nil, false
			}
			out = append(out, fmt.Sprint(docLookup(c, "AttributeValue"), "|", docLookup(c, "EditableVisible")))
		}
		return out, true
	}
	w, okW := key(written)
	s, okS := key(stored)
	if !okW || !okS || len(w) != len(s) || len(w) == 0 || strings.Join(w, ",") == strings.Join(s, ",") {
		return false
	}
	count := map[string]int{}
	for _, k := range w {
		count[k]++
	}
	for _, k := range s {
		count[k]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}

// storedConditionsExtend reports whether the stored Enumerations$Condition
// list holds every written condition with the same flag, plus conditions for
// values the written list does not name at all. Those are values mxcli's
// metadata does not have — a System enumeration a newer Mendix extended
// (WorkflowActivityType on 11.14: 25 stored, 17 written; ako/mxcli#2128) — so
// MDL cannot state them, and writing the shorter list would drop them.
func storedConditionsExtend(written, stored bson.A) bool {
	conds := func(a bson.A) (map[string]any, bool) {
		out := map[string]any{}
		for _, item := range a[min(1, len(a)):] {
			c, ok := item.(bson.D)
			if !ok || docLookup(c, "$Type") != "Enumerations$Condition" {
				return nil, false
			}
			v, _ := docLookup(c, "AttributeValue").(string)
			if _, dup := out[v]; dup {
				return nil, false
			}
			out[v] = docLookup(c, "EditableVisible")
		}
		return out, true
	}
	w, okW := conds(written)
	s, okS := conds(stored)
	if !okW || !okS || len(w) == 0 || len(s) <= len(w) {
		return false
	}
	for v, flag := range w {
		if sf, ok := s[v]; !ok || sf != flag {
			return false
		}
	}
	return true
}

// anyIDUsedOutside reports whether an element id inside items is in the
// document anywhere other than inside replaced, the list items take the place
// of: carrying it in would make two elements share an identity.
func anyIDUsedOutside(items bson.A, used map[string]bool, replaced bson.A) bool {
	own := map[string]bool{}
	walkElements(replaced, func(e bson.D) {
		if id, ok := docLookup(e, "$ID").(bson.Binary); ok {
			own[string(id.Data)] = true
		}
	})
	clash := false
	walkElements(items, func(e bson.D) {
		if id, ok := docLookup(e, "$ID").(bson.Binary); ok && used[string(id.Data)] && !own[string(id.Data)] {
			clash = true
		}
	})
	return clash
}
