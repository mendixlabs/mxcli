// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"crypto/sha256"
	"sort"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CarryTranslations copies onto a rebuilt document the translations the rebuild
// could not express, taken from the stored document.
//
// # Why anything is lost
//
// A Mendix text is a `Texts$Text` holding one `Texts$Translation` child per
// language. MDL carries ONE string — `Title: 'My Account'` — so a rebuilt
// document has exactly one child, and every other language the stored document
// held is gone. Measured on 11.13.0: describing `Administration.MyAccount` and
// re-executing that description dropped "Mijn account" from the entire project,
// taking the project's nl_NL count from 17 to 16. mxbuild reports 0 errors
// before and after, because a model missing a translation is a valid model — so
// nothing downstream catches it.
//
// This is the same shape as the other carries in Reconcile: the rebuild is
// authoritative about what the statement said, and silent about everything the
// statement had no way to say.
//
// # How elements are paired
//
// Two strategies, and which one applies is decided by evidence rather than by
// preference.
//
// **By position, when the shape is provably unchanged.** Each text is addressed
// by its containment path ("Title", "Widgets/2/Caption"). If the two documents
// have the *same set of paths*, then no text was inserted, removed or moved, so
// path pairing is exact — and it is the only strategy that can carry a text
// whose source string the statement CHANGED, which is the common edit.
//
// Positional pairing is used only under that precondition, because a mis-pair
// here is not benign the way TransplantIDs' is: that function rewrites every
// reference along with the id, so a wrong pairing costs a worse diff and nothing
// else. Copying a translation onto the wrong text would inject a translation of
// some other string — silent, and plausible-looking. So if a single path differs,
// positional pairing is abandoned entirely rather than applied where it happens
// to fit.
//
// **By owning element, when that is exact.** A widget's Name is unique within
// its document, so a text can be addressed as (element $Type, element Name, path
// from that element) — "Forms$ActionButton actionButton1 /CaptionTemplate/
// Template". That address survives the whole-document shape changes that rule
// out positional pairing, and it tells apart texts the source pairing cannot:
// two captions with the same English, or the many empty ones (Studio Pro stores
// en_US "" on every empty caption and message, so (en_US, "") is ambiguous on
// any real page, and a rebuilt empty text has no translation to look up by at
// all — ako/mxcli#705). It is used only where it is exact: the address must
// occur once in each document, and the path from the element must contain no
// list index — a rebuilt pluggable widget reorders its Properties list, and
// pairing across that would move a translation onto the wrong property.
//
// **By source string otherwise.** For each stored text, every (language, text)
// pair it carries keys that text's full translation set; a rebuilt text is
// looked up by the one pair it has. No project default language is needed —
// whichever language the rebuild wrote is the key. This is exact in practice:
// across a real project's 388 units, 34 carry translations and in NONE of them
// does one source string map to two different translations in one language.
// Where it is not exact, nothing is carried for that string.
//
// TransplantIDs' own element mapping is not reusable here: it deliberately does
// not re-marshal, patching fixed-width binaries in place, so it is unavailable to
// a pass that must change the document's length.
//
// # What it will not do
//
//   - **Overwrite a language the rebuild wrote.** The statement is the authority
//     for what it says.
//   - **Guess on an ambiguous source.** Two stored texts sharing a source string
//     but disagreeing on its translation are not resolvable this way, so that
//     string carries nothing. Guessing would silently move a translation from one
//     element to another, which is worse than leaving one behind.
//   - **Drop the other languages when the source changed.** Editing the English
//     in Studio Pro leaves the sibling translations in place, stale though they
//     become; deciding on the author's behalf that a translation is now wrong is
//     not this function's call. (`CREATE OR REPLACE TRANSLATIONS` is where a
//     stale translation is meant to be removed, deliberately and visibly.)
//
// Anything that cannot be read on either side returns the contents untouched,
// which is the behaviour that existed before this function.
func CarryTranslations(contents, stored []byte) []byte {
	if len(contents) == 0 || len(stored) == 0 {
		return contents
	}
	var storedDoc, newDoc bson.D
	if bson.Unmarshal(stored, &storedDoc) != nil {
		return contents
	}
	if bson.Unmarshal(contents, &newDoc) != nil {
		return contents
	}

	storedByPath := textsByPath(storedDoc)
	newByPath := textsByPath(newDoc)
	byPath := samePathSet(storedByPath, newByPath)

	_, storedByElement := textsByElement(storedDoc)
	newElementOf, _ := textsByElement(newDoc)

	sets := storedTranslationSets(storedDoc)
	if len(sets) == 0 && !byPath && len(storedByElement) == 0 {
		return contents
	}

	// Every element id already in the rebuilt document, so a carried element that
	// would collide with one gets a fresh id instead. See reuseSafeID.
	used := elementIDs(newDoc)

	changed := false
	// Sorted, not map order: which text is visited first decides which one keeps
	// a stored element's id, and a run-to-run difference there would change the
	// bytes and stop no-op elision from ever firing.
	for _, path := range sortedPaths(newByPath) {
		text := newByPath[path]
		var want map[string]bson.D
		var order []string
		if byPath {
			want, order = textTranslationElems(storedByPath[path]), textLanguageOrder(storedByPath[path])
			if dropUnmatchedEmpty(text, want) {
				changed = true
			}
		} else if stored, ok := storedByElement[newElementOf[path]]; ok {
			want, order = textTranslationElems(stored), textLanguageOrder(stored)
			if dropUnmatchedEmpty(text, want) {
				changed = true
			}
		} else {
			want, order = matchBySource(textTranslations(text), sets)
		}
		if len(want) == 0 {
			continue
		}
		if mergeText(text, want, order, path, used) {
			changed = true
		}
	}
	if !changed {
		return contents
	}
	out, err := bson.Marshal(newDoc)
	if err != nil {
		return contents
	}
	return out
}

// textsByPath addresses every Texts$Text in a document by its containment path.
// The bson.D values are the live sub-documents, so mutating one mutates the
// document it came from.
func textsByPath(doc bson.D) map[string]bson.D {
	out := map[string]bson.D{}
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch n := v.(type) {
		case bson.D:
			if ty, _ := docLookup(n, "$Type").(string); ty == "Texts$Text" {
				out[path] = n
				return
			}
			for _, e := range n {
				walk(e.Value, path+"/"+e.Key)
			}
		case bson.A:
			for i, e := range n {
				walk(e, path+"/"+strconv.Itoa(i))
			}
		}
	}
	walk(doc, "")
	return out
}

// textsByElement addresses the texts of a document by the named element that
// owns them. It returns the address of each text by its containment path (as
// textsByPath spells it), and the text at each address — both restricted to the
// addresses that are exact: owned by an element with a $Type and a Name, reached
// from it without passing through a list, and occurring once in the document.
func textsByElement(doc bson.D) (elementOf map[string]string, byElement map[string]bson.D) {
	elementOf = map[string]string{}
	byElement = map[string]bson.D{}
	count := map[string]int{}
	var walk func(v any, path, element, rel string, indexed bool)
	walk = func(v any, path, element, rel string, indexed bool) {
		switch n := v.(type) {
		case bson.D:
			ty, _ := docLookup(n, "$Type").(string)
			if ty == "Texts$Text" {
				if element != "" && !indexed {
					key := element + "\x00" + rel
					count[key]++
					elementOf[path] = key
					byElement[key] = n
				}
				return
			}
			if name, _ := docLookup(n, "Name").(string); name != "" && ty != "" {
				element, rel, indexed = ty+"\x00"+name, "", false
			}
			for _, e := range n {
				walk(e.Value, path+"/"+e.Key, element, rel+"/"+e.Key, indexed)
			}
		case bson.A:
			for i, e := range n {
				walk(e, path+"/"+strconv.Itoa(i), element, rel+"/"+strconv.Itoa(i), true)
			}
		}
	}
	walk(doc, "", "", "", false)
	for path, key := range elementOf {
		if count[key] > 1 {
			delete(elementOf, path)
			delete(byElement, key)
		}
	}
	return elementOf, byElement
}

// samePathSet reports whether the two documents hold texts at exactly the same
// paths — the precondition that makes positional pairing exact.
func samePathSet(a, b map[string]bson.D) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			return false
		}
	}
	return true
}

// matchBySource finds the stored translation set for a rebuilt text, by whichever
// (language, text) pair the rebuild wrote, with its stored language order.
// Returns nil when nothing matches or the match is ambiguous.
func matchBySource(have map[string]string, sets map[string]*translationSet) (map[string]bson.D, []string) {
	for lang, txt := range have {
		if s, ok := sets[lang+"\x00"+txt]; ok {
			if s.ambiguous {
				return nil, nil
			}
			return s.byLang, s.order
		}
	}
	return nil, nil
}

// ambiguous marks a key whose stored translation sets disagree.
type translationSet struct {
	byLang map[string]bson.D
	// order is the stored text's language order, which the carry keeps.
	order []string
	// text is the language→string view, used only to decide whether two stored
	// sets sharing a source key actually agree.
	text      map[string]string
	ambiguous bool
}

// storedTranslationSets indexes every stored text by each (language, text) pair
// it carries, so a rebuilt text can be found by whichever single language the
// rebuild happened to write.
func storedTranslationSets(doc bson.D) map[string]*translationSet {
	out := map[string]*translationSet{}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if ty, _ := docLookup(n, "$Type").(string); ty == "Texts$Text" {
				byText := textTranslations(n)
				if len(byText) > 1 {
					set := &translationSet{byLang: textTranslationElems(n), order: textLanguageOrder(n), text: byText}
					for lang, text := range byText {
						k := lang + "\x00" + text
						prev, seen := out[k]
						if !seen {
							out[k] = set
							continue
						}
						if !sameTranslations(prev.text, byText) {
							prev.ambiguous = true
						}
					}
				}
			}
			for _, e := range n {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

// dropUnmatchedEmpty removes from a rebuilt text each empty translation in a
// language its stored counterpart has no translation in, reporting whether it
// removed any.
//
// A text the statement states as empty is written as one empty translation in
// the authoring language, so that this carry leaves it empty instead of adding
// the stored text back (a data grid column's `Caption: ”` could not clear a
// stored caption). Studio Pro, though, stores an empty pluggable caption with no
// translation at all, and keeping the rebuild's ("en_US", "") over it would make
// every rewrite of such a page a write. Where the stored text has nothing in
// that language, the empty translation says nothing it does not, so it goes.
func dropUnmatchedEmpty(text bson.D, stored map[string]bson.D) bool {
	items, ok := docLookup(text, "Items").(bson.A)
	if !ok {
		return false
	}
	kept := make(bson.A, 0, len(items))
	dropped := false
	for _, it := range items {
		if d, ok := it.(bson.D); ok {
			lang, _ := docLookup(d, "LanguageCode").(string)
			txt, _ := docLookup(d, "Text").(string)
			if _, has := stored[lang]; lang != "" && txt == "" && !has {
				dropped = true
				continue
			}
		}
		kept = append(kept, it)
	}
	if !dropped {
		return false
	}
	for i, e := range text {
		if e.Key == "Items" {
			text[i].Value = kept
		}
	}
	return true
}

// mergeText appends the languages a rebuilt text is missing, reporting whether
// anything was added. A language the rebuild wrote is never overwritten: the
// statement is the authority for what it says.
//
// The result follows the stored text's language order. The rebuild writes the
// one language MDL carries, so appending the others after it — in sorted order,
// as this did — turned a text Studio Pro stored as [nl_NL, en_US] into [en_US,
// nl_NL]. The canonical form compares lists in order, so the reorder alone
// made an unchanged page a write (ako/mxcli#1704).
//
// used carries every element id already in the document and is updated as ids
// are taken, so the same stored element handed to two texts does not put one id
// on both of them (see reuseSafeID).
func mergeText(text bson.D, want map[string]bson.D, order []string, path string, used map[string]bool) bool {
	have := textTranslations(text)
	items, ok := docLookup(text, "Items").(bson.A)
	if !ok {
		return false
	}
	added := false
	for _, lang := range order {
		if _, exists := have[lang]; exists {
			continue
		}
		elem, ok := want[lang]
		if !ok {
			continue
		}
		items = append(items, reuseSafeID(elem, path, lang, used))
		have[lang] = ""
		added = true
	}
	if !added {
		return false
	}
	items = inLanguageOrder(items, order)
	for i, e := range text {
		if e.Key == "Items" {
			text[i].Value = items
		}
	}
	return true
}

// inLanguageOrder arranges a text's translation elements in the given language
// order. The list marker stays first; a language the order does not name keeps
// its relative place after the named ones.
func inLanguageOrder(items bson.A, order []string) bson.A {
	rank := make(map[string]int, len(order))
	for i, lang := range order {
		if _, seen := rank[lang]; !seen {
			rank[lang] = i
		}
	}
	rankOf := func(d bson.D) int {
		lang, _ := docLookup(d, "LanguageCode").(string)
		if r, ok := rank[lang]; ok {
			return r
		}
		return len(order)
	}
	out := make(bson.A, 0, len(items))
	var elems []bson.D
	for _, it := range items {
		if d, ok := it.(bson.D); ok {
			elems = append(elems, d)
		} else {
			out = append(out, it)
		}
	}
	sort.SliceStable(elems, func(i, j int) bool { return rankOf(elems[i]) < rankOf(elems[j]) })
	for _, d := range elems {
		out = append(out, d)
	}
	return out
}

// textLanguageOrder reads a Texts$Text's languages in stored order.
func textLanguageOrder(text bson.D) []string {
	items, ok := docLookup(text, "Items").(bson.A)
	if !ok {
		return nil
	}
	var out []string
	for _, it := range items {
		if d, ok := it.(bson.D); ok {
			if lang, _ := docLookup(d, "LanguageCode").(string); lang != "" {
				out = append(out, lang)
			}
		}
	}
	return out
}

// textTranslationElems reads a Texts$Text's language→translation-element map.
// The whole sub-document is kept, not just the string: appending it verbatim
// carries its $ID too, so a carried translation keeps the identity it had on
// disk and the canonical form sees the same element it saw before. Appending a
// freshly built child without an $ID makes an otherwise identical document
// compare unequal, and no-op elision could never fire.
func textTranslationElems(text bson.D) map[string]bson.D {
	items, ok := docLookup(text, "Items").(bson.A)
	if !ok {
		return nil
	}
	out := map[string]bson.D{}
	for _, it := range items {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		if lang, _ := docLookup(d, "LanguageCode").(string); lang != "" {
			out[lang] = d
		}
	}
	return out
}

// textTranslations reads a Texts$Text's language→text map.
func textTranslations(text bson.D) map[string]string {
	items, ok := docLookup(text, "Items").(bson.A)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for _, it := range items {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		lang, _ := docLookup(d, "LanguageCode").(string)
		if lang == "" {
			continue
		}
		txt, _ := docLookup(d, "Text").(string)
		out[lang] = txt
	}
	return out
}

func sameTranslations(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func docLookup(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// reuseSafeID returns the stored translation element to append, with a fresh
// element id if the stored one is already taken in this document.
//
// The by-source branch can hand ONE stored element to several rebuilt texts —
// eight copies of the literal '{1}' on a page is entirely ordinary — and
// appending it verbatim each time put the same $ID on all eight. A unit whose
// document holds two elements with the same $ID cannot be opened: Studio Pro
// refuses the whole project, and mxcli's own write-time guard (duplicates.go)
// refuses the write. That guard is what surfaced this, and its comment records
// that the cause could not be established at the time — this is it, for the
// Texts$Translation case it was reported against (CapTrackV2 §30/§17).
//
// Re-identifying a copy is safe HERE in a way that deduplicating ids in general
// is not, and the distinction is the whole argument for doing it:
//
//   - An $ID is a pointer target, and rewriting one means finding every
//     reference to it (ADR-0008). Nothing references a Texts$Translation: it is
//     a leaf child of a Texts$Text, holding four keys and no identity of its own
//     that anything resolves by. So there are no references to miss.
//   - The FIRST use keeps the stored id, so an unchanged document still compares
//     equal and no-op elision fires. Only the copies — elements that never
//     existed on disk under that id — are re-identified.
//
// The fresh id is derived rather than random, so the same inputs give the same
// bytes on every run; a random id would make the document differ from itself and
// write on every execution.
func reuseSafeID(elem bson.D, path, lang string, used map[string]bool) bson.D {
	id, ok := docLookup(elem, "$ID").(bson.Binary)
	if !ok {
		return elem // no id to collide; leave it exactly as it was
	}
	key := string(id.Data)
	if !used[key] {
		used[key] = true
		return elem
	}
	next := derivedID(id, path, lang, used)
	out := make(bson.D, len(elem))
	copy(out, elem)
	for i := range out {
		if out[i].Key == "$ID" {
			out[i].Value = next
		}
	}
	used[string(next.Data)] = true
	return out
}

// derivedID builds a deterministic 16-byte id from the element being copied and
// where the copy is going. Two different destinations therefore get two
// different ids, and the same run twice gets the same ones.
//
// The loop is a collision guard rather than an expectation: a SHA-256 prefix
// colliding with an id already in the document is not a case anyone will see,
// but the failure it would cause is precisely the one this function exists to
// remove, so it is cheaper to rule out than to reason about.
func derivedID(from bson.Binary, path, lang string, used map[string]bool) bson.Binary {
	for n := 0; ; n++ {
		h := sha256.New()
		h.Write(from.Data)
		h.Write([]byte(path))
		h.Write([]byte(lang))
		h.Write([]byte(strconv.Itoa(n)))
		out := bson.Binary{Subtype: from.Subtype, Data: h.Sum(nil)[:16]}
		if !used[string(out.Data)] {
			return out
		}
	}
}

// elementIDs collects every $ID in a document, so a carried element can be told
// whether the id it wants is already spoken for.
func elementIDs(doc bson.D) map[string]bool {
	out := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if b, ok := docLookup(n, "$ID").(bson.Binary); ok {
				out[string(b.Data)] = true
			}
			for _, e := range n {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

// sortedPaths keeps the visit order stable across runs.
func sortedPaths(m map[string]bson.D) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
