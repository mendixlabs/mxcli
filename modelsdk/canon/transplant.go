// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// TransplantIDs returns contents with every element $ID replaced by the $ID the
// structurally corresponding element in stored already carries — and with every
// reference to that element rewritten in the same pass.
//
// # Why
//
// Elision (Reconcile) answers "did anything change?". When something did, the
// whole document is still rebuilt from the MDL, and every sub-element of a
// rebuild gets a freshly random $ID. Measured on the ticket's own model:
// changing one argument of one JavaScript action call rewrote 36 of a nanoflow's
// 37 element identities, and the same one-line change to a microflow rewrote 21
// of 22. Studio Pro's version-control view keys on those identities, so a
// two-line semantic change reads as "the entire document was replaced". Worse,
// it is cumulative: making a change and reverting it lands on a document that is
// semantically identical to where it started and shares none of its ids.
//
// This is the other half of ADR-0008. Elision keeps a no-op write off disk;
// carrying identities keeps a *real* write down to what really changed.
//
// # Why this is safe
//
// The correctness bar is lower than it looks, and it is worth being precise
// about where it sits, because the neighbouring operation — renumbering $IDs —
// is what PR #125 did and it made projects unopenable.
//
//   - **A wrong match is not a wrong document.** If two elements are paired that
//     a person would not have paired, the result is a suboptimal diff, not a
//     broken model: every reference to the element is rewritten with it, so
//     every pointer still names the same node. Only which UUID labels the node
//     changes, and nothing outside the unit remembers it (ADR-0008: of 9,910
//     binary $ID pointers in a real project, 0 cross a unit boundary).
//   - **Duplicating an id is the one real failure**, and it is guarded against
//     explicitly below: the mapping is injective by construction, and an entry
//     whose target is an id some *unmapped* element still holds is dropped.
//   - **References are found without knowing which properties hold them**, the
//     same insight the canonical form is built on: a pointer is a plain binary
//     property, invisible to a containment walk, but any occurrence of one of
//     this document's element ids *is* a reference by definition. So the
//     substitution is over every 16-byte binary in the document, not over a list
//     of pointer property names that would have to be maintained.
//
// The direction also matters. Transplanting moves the written document *towards*
// the ids already on disk, which is exactly what eliding a write does implicitly
// — so it cannot be less safe than the elision that already ships.
//
// # Limits
//
// Only 16-byte binary $IDs participate. The patch is applied in place on a copy,
// which a fixed-width binary makes framing-safe; a string $ID would change the
// document's length and is left alone. Nothing is re-marshalled, so a document
// the codec produced reaches storage exactly as the codec produced it apart from
// these bytes.
//
// Anything that cannot be read — a malformed document on either side — yields no
// mapping and the contents pass through untouched, which is the behaviour that
// existed before this function.
func TransplantIDs(contents, stored []byte) []byte {
	return transplantIDs(contents, parseStored(stored))
}

// transplantIDs is TransplantIDs with the stored document already parsed. It
// only reads the parse.
func transplantIDs(contents []byte, stored parsedStored) []byte {
	if !stored.ok {
		return contents
	}
	m := idMappingWith(contents, stored.doc)
	if len(m) == 0 {
		return contents
	}
	out := append([]byte(nil), contents...)
	if !substituteIDs(out, m, elementIDSetOf) {
		return contents
	}
	return out
}

// transplantIDsInPasses is TransplantIDs as it was: both documents parsed
// here, and the substitution checked by parsing the document before and after.
func transplantIDsInPasses(contents, stored []byte) []byte {
	m := idMapping(contents, stored)
	if len(m) == 0 {
		return contents
	}
	out := append([]byte(nil), contents...)
	if !substituteIDs(out, m, elementIDSetByParsing) {
		return contents
	}
	return out
}

// idMapping pairs the two documents structurally and returns new id → stored id
// for every pair that is safe to apply. Entries that would be no-ops are kept so
// the collision check can see them, and skipped when applied.
func idMapping(contents, stored []byte) map[string][]byte {
	var oldDoc bson.D
	if err := bson.Unmarshal(stored, &oldDoc); err != nil {
		return nil
	}
	return idMappingWith(contents, oldDoc)
}

// idMappingWith is idMapping with the stored document already parsed.
func idMappingWith(contents []byte, oldDoc bson.D) map[string][]byte {
	var newDoc bson.D
	if err := bson.Unmarshal(contents, &newDoc); err != nil {
		return nil
	}
	w := &pairer{pairs: map[string][]byte{}, claimed: map[string]bool{}, storedNames: namedElementIDs(oldDoc)}
	w.pairValue(newDoc, oldDoc)
	if len(w.pairs) == 0 {
		return nil
	}
	return dropCollisions(w.pairs, collectElementIDs(newDoc))
}

// pairer accumulates the correspondence. claimed enforces injectivity: a stored
// id can be handed out at most once, so two elements can never end up sharing
// one however the alignment behaves.
type pairer struct {
	pairs   map[string][]byte
	claimed map[string]bool
	// storedNames maps the stored document's named elements' ids to their
	// names. A written element that carries one of those ids under the same
	// name IS that stored element — moved to another list, or retyped from an
	// association to a cross-association — and keeps its id rather than take
	// the id of whatever the alignment put opposite it (ako/mxcli#2322: a
	// converted cross-association took its neighbour's $ID, and with it the
	// GUID mxcli had minted equal to it, and mx check reported CE9208).
	storedNames map[string]string
}

func (p *pairer) record(newID string, oldID string, oldData []byte) {
	if _, seen := p.pairs[newID]; seen {
		return
	}
	if p.claimed[oldID] {
		return
	}
	p.claimed[oldID] = true
	p.pairs[newID] = oldData
}

func (p *pairer) pairValue(newV, oldV any) {
	if nd, ok := asDoc(newV); ok {
		od, ok := asDoc(oldV)
		if !ok {
			return
		}
		p.pairDoc(nd, od)
		return
	}
	if news, ok := asSlice(newV); ok {
		olds, ok := asSlice(oldV)
		if !ok {
			return
		}
		for _, pair := range alignSlices(news, olds) {
			p.pairValue(news[pair[0]], olds[pair[1]])
		}
	}
}

// pairDoc corresponds two elements. A differing $Type means these are not the
// same element — inheriting the identity there would claim a replacement was an
// edit — so neither they nor anything under them is paired.
func (p *pairer) pairDoc(nd, od map[string]any) {
	if typeName(nd) != typeName(od) {
		return
	}
	if nid, ndata, ok := binaryElementID(nd); ok {
		if oid, odata, ok := binaryElementID(od); ok && len(ndata) == len(odata) {
			// Not the element opposite it: neither it nor anything under it
			// takes that element's ids (its delete behaviour took the
			// neighbour's, the second duplicate of #2322).
			if p.keepsOwnID(nid, oid, nd) {
				return
			}
			p.record(nid, oid, odata)
		}
	}
	for _, k := range sortedKeys(nd) {
		if k == "$ID" {
			continue
		}
		if ov, ok := od[k]; ok {
			p.pairValue(nd[k], ov)
		}
	}
}

func typeName(d map[string]any) string {
	s, _ := d["$Type"].(string)
	return s
}

// binaryElementID reads a 16-byte binary $ID. A string $ID is deliberately not
// accepted: substituting one would change the document's length, and the patch
// is in place.
func binaryElementID(d map[string]any) (uuid string, data []byte, ok bool) {
	b, isBin := d["$ID"].(bson.Binary)
	if !isBin || len(b.Data) != 16 {
		return "", nil, false
	}
	return blobToUUID(b.Data), b.Data, true
}

// alignSlices decides which entry of the new list corresponds to which entry of
// the stored one, returning [newIndex, storedIndex] pairs in order.
//
// Positional alignment alone would be enough for the ticket's case (one value
// changed, shape untouched), but it is wrong for the change people actually make
// next: inserting an activity would shift every element after it onto its
// neighbour's identity and churn the rest of the flow — the very symptom being
// fixed. So entries are anchored on a match key first, longest-common-
// subsequence style, and only the gaps between anchors fall back to position.
func alignSlices(news, olds []any) [][2]int {
	if len(news) == 0 || len(olds) == 0 {
		return nil
	}
	// A quadratic table on a pathologically long list is not worth it; the
	// positional fallback is what alignment degrades to anyway.
	const lcsLimit = 1024
	if len(news) > lcsLimit || len(olds) > lcsLimit {
		return positionalPairs(0, 0, len(news), len(olds))
	}

	anchors := lcsPairsPlaced(matchKeys(news), matchKeys(olds), places(news), places(olds))

	var out [][2]int
	ni, oi := 0, 0
	for _, a := range anchors {
		out = append(out, positionalPairs(ni, oi, a[0], a[1])...)
		out = append(out, a)
		ni, oi = a[0]+1, a[1]+1
	}
	out = append(out, positionalPairs(ni, oi, len(news), len(olds))...)
	return pairLeftovers(news, olds, out)
}

// pairLeftovers adds to pairs the entries the alignment left with nothing
// opposite them, paired by matchKey where that key occurs exactly once among
// the leftovers on each side (ako/mxcli#2329, mendixlabs/mxcli#1872).
//
// The alignment is ordered, and some lists are not: Studio Pro stores a flow's
// objects in the order they were drawn, and mxcli writes a flow's parameters
// and start event first. Anchored on the longest common run of activities,
// the parameter and the start event fell in a gap with nothing — or an element
// of another $Type, which pairDoc refuses — opposite them, so a rebuild of a
// Studio Pro flow re-minted their $IDs and every carry keyed on the $ID missed
// them: the parameter's documentation was lost on `drop` + `create`.
//
// A positional pair of two elements of different $Types pairs nothing
// (pairDoc), so it is released here rather than holding both entries. A key
// that occurs twice among the leftovers on either side is ambiguous, and those
// entries stay unpaired rather than pair by guess.
func pairLeftovers(news, olds []any, pairs [][2]int) [][2]int {
	usedNew, usedOld := map[int]bool{}, map[int]bool{}
	kept := pairs[:0:0]
	for _, p := range pairs {
		nd, nok := asDoc(news[p[0]])
		od, ook := asDoc(olds[p[1]])
		if nok && ook && typeName(nd) != typeName(od) {
			continue
		}
		kept = append(kept, p)
		usedNew[p[0]], usedOld[p[1]] = true, true
	}
	leftovers := func(vs []any, used map[int]bool) map[string][]int {
		out := map[string][]int{}
		for i, v := range vs {
			if used[i] {
				continue
			}
			if _, ok := asDoc(v); !ok {
				continue
			}
			k := matchKey(v)
			out[k] = append(out[k], i)
		}
		return out
	}
	newLeft, oldLeft := leftovers(news, usedNew), leftovers(olds, usedOld)
	var extra [][2]int
	for k, ns := range newLeft {
		if os := oldLeft[k]; len(ns) == 1 && len(os) == 1 {
			extra = append(extra, [2]int{ns[0], os[0]})
		}
	}
	if len(extra) == 0 {
		return kept
	}
	// In list order, so the pairing does not depend on map iteration.
	sort.Slice(extra, func(i, j int) bool { return extra[i][0] < extra[j][0] })
	return append(kept, extra...)
}

// positionalPairs pairs [nStart,nEnd) with [oStart,oEnd) by offset. Used only
// inside a gap between anchors, where the entries did not match by key — a
// renamed element lands here, and pairDoc still refuses if the types differ.
func positionalPairs(nStart, oStart, nEnd, oEnd int) [][2]int {
	var out [][2]int
	for i := 0; nStart+i < nEnd && oStart+i < oEnd; i++ {
		out = append(out, [2]int{nStart + i, oStart + i})
	}
	return out
}

// matchKey identifies a list entry well enough to line two lists up, without
// being so specific that editing the entry stops it matching itself — the whole
// point is that an *edited* element keeps its identity.
//
// $Type plus Name is the "by position, or by name where there is one" the report
// asks for, but on its own it is too blunt for a flow: every activity in a
// microflow is a `Microflows$ActionActivity` with no name, so inserting one at
// the front made LCS pair the *wrapper* correctly and hand the newcomer the old
// activity's whole action subtree. Adding the $Type of each immediate child
// element — the shape one level down, `Action=Microflows$LogMessageAction` — is
// what tells two otherwise identical wrappers apart. Measured on the ticket's
// microflow, inserting a log activity: 17 of 22 identities kept before, 22 of 22
// after.
//
// Deliberately no other property values. A key built from content would stop an
// element matching itself the moment someone edited it, which is the case this
// whole file exists to preserve.
func matchKey(v any) string {
	d, ok := asDoc(v)
	if !ok {
		return "\x00scalar"
	}
	key := typeName(d)
	if name, ok := d["Name"].(string); ok {
		key += "\x00" + name
	}
	for _, k := range sortedKeys(d) {
		child, ok := asDoc(d[k])
		if !ok {
			continue
		}
		if t := typeName(child); t != "" {
			key += "\x01" + k + "=" + t
		}
	}
	return key
}

// places returns each entry's RelativeMiddlePoint, "" where it has none.
func places(vs []any) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		if d, ok := asDoc(v); ok {
			out[i], _ = d["RelativeMiddlePoint"].(string)
		}
	}
	return out
}

func matchKeys(vs []any) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = matchKey(v)
	}
	return out
}

// lcsPairs returns a longest common subsequence of the two key sequences as
// index pairs.
func lcsPairs(a, b []string) [][2]int {
	return lcsPairsPlaced(a, b, nil, nil)
}

// lcsPairsPlaced is lcsPairs that, among the alignments matching as many keys,
// favours entries drawn at the same place: a flow object's RelativeMiddlePoint,
// in pa and pb ("" for an entry with none).
//
// Every activity of one kind has one key, so where Studio Pro's order of the
// objects differs from mxcli's, two show messages could cross: the rebuilt one
// took the identity, and with it the documentation and translations, of the
// other (ako/mxcli#2329). A describe states every object's position, so a
// rebuild of it puts each one where it was, and an element of one kind at one
// point is that element. A match scores 2, and 12 at the same place, so an
// alignment keeps the same-place pairs over a longer run of guesses; what the
// order then leaves out pairs by key afterwards (pairLeftovers). Where nothing
// is at its old place — an insertion that moved everything after it — every
// match scores the same and the alignment is the plain one.
func lcsPairsPlaced(a, b, pa, pb []string) [][2]int {
	n, m := len(a), len(b)
	score := func(i, j int) int {
		if a[i] != b[j] {
			return -1
		}
		if pa != nil && pb != nil && pa[i] != "" && pa[i] == pb[j] {
			return 12
		}
		return 2
	}
	table := make([][]int, n+1)
	for i := range table {
		table[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			best := table[i+1][j]
			if table[i][j+1] > best {
				best = table[i][j+1]
			}
			if w := score(i, j); w > 0 && w+table[i+1][j+1] > best {
				best = w + table[i+1][j+1]
			}
			table[i][j] = best
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case score(i, j) > 0 && table[i][j] == score(i, j)+table[i+1][j+1]:
			out = append(out, [2]int{i, j})
			i, j = i+1, j+1
		case table[i+1][j] >= table[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// dropCollisions removes any entry that would leave two elements sharing an id.
//
// Applying the mapping renames the ids in its domain and leaves every other id
// alone, so the only way to collide is to rename x onto an id y that some
// element still holds — that is, y is an id of the new document and is not
// itself being renamed away. Dropping an entry can invalidate another (its
// target stops being renamed away), so this runs to a fixed point.
//
// In practice the domain is either all-fresh random ids, where no target is ever
// present in the new document, or an ALTER-style rewrite where most pairs are
// x → x and trivially safe. The guard is here for the mixed case.
func dropCollisions(pairs map[string][]byte, newIDs map[string]int) map[string][]byte {
	for {
		var doomed []string
		for k, v := range pairs {
			target := blobToUUID(v)
			if _, held := newIDs[target]; !held {
				continue
			}
			if _, renamedAway := pairs[target]; !renamedAway {
				doomed = append(doomed, k)
			}
		}
		if len(doomed) == 0 {
			return pairs
		}
		for _, k := range doomed {
			delete(pairs, k)
		}
	}
}

// substituteIDs rewrites, in place, every 16-byte binary in raw whose value is a
// key of m. It reports whether the document was walked cleanly.
//
// Every slot is visited exactly once and its replacement chosen from the value
// it held on entry, so a mapping that swaps two ids applies correctly rather
// than renaming one onto the other and back.
//
// The patch relies on bsoncore's looked-up values aliasing raw rather than
// copying out of it — a property of the library, not of this package. Rather
// than assume it, the result is read back and checked against the id set the
// substitution was supposed to produce.
func substituteIDs(raw []byte, m map[string][]byte, idsOf func([]byte) (map[string]bool, bool)) bool {
	before, ok := idsOf(raw)
	if !ok {
		return false
	}
	// What the document's ids must be afterwards: renamed where the mapping says
	// so, untouched otherwise. Deriving this up front is what makes the check
	// survive a mapping that swaps two ids, where an id being both a source and a
	// target is correct rather than evidence of a half-applied patch.
	want := make(map[string]bool, len(before))
	for id := range before {
		if repl, renamed := m[id]; renamed {
			want[blobToUUID(repl)] = true
		} else {
			want[id] = true
		}
	}

	if !patchDocument(bsoncore.Document(raw), m) {
		return false
	}

	after, ok := idsOf(raw)
	if !ok || len(after) != len(want) {
		return false
	}
	for id := range after {
		if !want[id] {
			return false
		}
	}
	return true
}

// elementIDSetByParsing is the set of element ids of a document, read by
// parsing it: what elementIDSetOf is compared with.
func elementIDSetByParsing(raw []byte) (map[string]bool, bool) {
	ids, ok := elementIDsOf(raw)
	if !ok {
		return nil, false
	}
	out := make(map[string]bool, len(ids))
	for id := range ids {
		out[id] = true
	}
	return out, true
}

// elementIDSetOf is the set of element ids of a document, read where they lie:
// every $ID of the document and of every document under it, a 16-byte binary
// as its UUID and a string as itself (elementID). ok is false for bytes that
// are not a document.
//
// The substitution check reads the ids before and after it patches. Parsing
// the whole document for each was two of the twelve parses a reconcile made
// (ako/mxcli#2160); the ids are all it needs, and they can be read without
// building the tree.
func elementIDSetOf(raw []byte) (map[string]bool, bool) {
	if bson.Raw(raw).Validate() != nil {
		return nil, false
	}
	out := map[string]bool{}
	var walkDoc func(doc bsoncore.Document, isArray bool)
	walkDoc = func(doc bsoncore.Document, isArray bool) {
		elems, err := doc.Elements()
		if err != nil {
			return
		}
		id, hasID := "", false
		for _, e := range elems {
			v := e.Value()
			switch v.Type {
			case bsoncore.TypeEmbeddedDocument:
				if d, ok := v.DocumentOK(); ok {
					walkDoc(d, false)
				}
			case bsoncore.TypeArray:
				if a, ok := v.ArrayOK(); ok {
					walkDoc(bsoncore.Document(a), true)
				}
			}
			if isArray || e.Key() != "$ID" {
				continue
			}
			// The last $ID of a document wins, as it does in the map a parsed
			// document becomes.
			switch v.Type {
			case bsoncore.TypeBinary:
				if _, data, ok := v.BinaryOK(); ok && len(data) == 16 {
					id, hasID = blobToUUID(data), true
				} else {
					hasID = false
				}
			case bsoncore.TypeString:
				if str, ok := v.StringValueOK(); ok {
					id, hasID = str, true
				}
			default:
				hasID = false
			}
		}
		if hasID {
			out[id] = true
		}
	}
	walkDoc(bsoncore.Document(raw), false)
	return out, true
}

func elementIDsOf(raw []byte) (map[string]int, bool) {
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, false
	}
	return collectElementIDs(d), true
}

func patchDocument(doc bsoncore.Document, m map[string][]byte) bool {
	elems, err := doc.Elements()
	if err != nil {
		return false
	}
	for _, e := range elems {
		v := e.Value()
		switch v.Type {
		case bsoncore.TypeBinary:
			_, data, ok := v.BinaryOK()
			if !ok || len(data) != 16 {
				continue
			}
			if repl, found := m[blobToUUID(data)]; found {
				copy(data, repl)
			}
		case bsoncore.TypeEmbeddedDocument:
			sub, ok := v.DocumentOK()
			if !ok || !patchDocument(sub, m) {
				return false
			}
		case bsoncore.TypeArray:
			arr, ok := v.ArrayOK()
			if !ok || !patchDocument(bsoncore.Document(arr), m) {
				return false
			}
		}
	}
	return true
}

// keepsOwnID reports whether nd, a written element with id nid paired with the
// stored element oid, is instead the stored element that already holds nid:
// one with the same name.
func (p *pairer) keepsOwnID(nid, oid string, nd map[string]any) bool {
	if nid == oid {
		return false
	}
	name, held := p.storedNames[nid]
	return held && name != "" && name == nd["Name"]
}

// namedElementIDs maps the id of every element of doc that has a Name to it.
func namedElementIDs(doc bson.D) map[string]string {
	out := map[string]string{}
	var walk func(v any)
	walk = func(v any) {
		if d, ok := asDoc(v); ok {
			if id, _, ok := binaryElementID(d); ok {
				if name, ok := d["Name"].(string); ok && name != "" {
					out[id] = name
				}
			}
			for _, k := range sortedKeys(d) {
				walk(d[k])
			}
			return
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}
