// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
)

// # Editing an activity in place (ako/mxcli#1761)
//
// A replace by one activity of the same kind — the same activity $Type and
// the same action $Type — is an edit of the stored activity, the way Studio
// Pro edits one: the activity keeps its $ID, and so does every element under
// it that the new content has at the same place. Studio Pro compares and
// merges by $ID, so a replace that minted new ones showed an edited filter as
// one activity removed and another added, and turned a teammate's edit of the
// same activity on another branch into a conflict — while a decision whose
// condition changed the same way kept its $IDs (SetCondition).
//
// The replacement is serialized from the statement as for any replace and put
// where the stored activity is (swapInPlace). Its elements are then paired
// with the stored ones structurally (carryPairs), never by a shared value:
//
//   - two elements pair when they have the same $Type at the same place: the
//     activity itself, or the same key of two paired elements;
//   - in a list, entries that name what they are about pair by that name — a
//     member change by its attribute or association and whether it sets, adds
//     or removes; a parameter mapping by its parameter; a translation by its
//     language — so a reordered member keeps its $ID, and a removed member's
//     $ID goes onto no other member. A list of unnamed entries pairs by
//     position, and only when both lists hold the same $Types in the same
//     order;
//   - anything else is new: an element whose $Type differs (a find where a
//     filter was, a custom range where a constant one was), and everything
//     under it. Its stored counterpart is removed, so a reference left to it
//     is refused at Save as for any removed element.
//
// The stored $IDs go on at Bytes, not at the replace. Until then the
// replacement carries the $IDs its fragment was built with, which the
// executor still names it by: a stored note linked to it after the replace
// (ako/mxcli#1695), a note the statement states. At Bytes every reference to
// a carried element is rewritten with it, in one pass over every binary in
// the unit, so no pointer is left naming the built $ID (CLAUDE.md rule 1).
//
// What is written is the replacement's content, as before: the statement
// states the activity whole (describe prints every property MDL has a
// spelling for), and the writer completes the property set Studio Pro writes
// (canon.CompletePropertySets). Only which $IDs it carries changes.

// sameKind reports whether d, a serialized replacement, is the same kind of
// activity as stored: both action activities, whose actions have one $Type.
func sameKind(d, stored bson.D) bool {
	const activity = "Microflows$ActionActivity"
	if dString(d, "$Type") != activity || dString(stored, "$Type") != activity {
		return false
	}
	na, sa := dDoc(d, "Action"), dDoc(stored, "Action")
	return na != nil && sa != nil && dString(na, "$Type") != "" && dString(na, "$Type") == dString(sa, "$Type")
}

// swapKeeping is swapInPlace that, when d is the same kind of activity as x
// (sameKind), carries x's identity onto it: x's $ID and those of the elements
// under it that pair up (carryPairs), put on at Bytes.
func (m *Mutator) swapKeeping(g *graph, x *node, oc, d bson.D, id model.ID) error {
	same := sameKind(d, x.doc)
	if err := m.swapInPlace(g, x, oc, d, id); err != nil {
		return err
	}
	if same {
		m.carryUnstated(d, x.doc)
		carryPairs(d, x.doc, m.carried)
		m.edited[x.id] = true
	}
	return nil
}

// unstated are the properties, by element $Type, that MDL has no spelling for:
// describe does not print them, so no statement states them, and the writer puts
// its default where they go. On an edit the stored value is the only one there
// is, and it is kept (ako/mxcli#2169, mendixlabs/mxcli#1830) — a REST call whose
// proxy was set to Override went back to the JVM default when only its timeout
// was edited, and an activity lost its documentation.
//
// A property belongs here only while MDL cannot state it. Once it can, the
// statement is the authority and the entry has to go, or an edit that removes
// the value would be silently undone.
var unstated = map[string][]string{
	"Microflows$ActionActivity": {"Documentation"},
	"Microflows$RestCallAction": {"RequestProxyType", "ProxyConfiguration"},
}

// carryUnstated copies onto d, the serialized replacement of stored, the
// stored value of every unstated property of the activity and of its action.
// A carried element keeps its $IDs: it is the stored element, still in the
// unit, so it is no longer among the removed ones.
func (m *Mutator) carryUnstated(d, stored bson.D) {
	carry := func(nd, sd bson.D) {
		for _, key := range unstated[dString(nd, "$Type")] {
			v := dGet(sd, key)
			if v == nil && !hasKey(sd, key) {
				continue
			}
			if !dSet(nd, key, v) {
				continue
			}
			m.unmarkRemoved(v)
		}
	}
	carry(d, stored)
	if na, sa := dDoc(d, "Action"), dDoc(stored, "Action"); na != nil && sa != nil {
		carry(na, sa)
	}
}

// hasKey reports whether d has key, whatever its value (null included).
func hasKey(d bson.D, key string) bool {
	for _, e := range d {
		if e.Key == key {
			return true
		}
	}
	return false
}

// unmarkRemoved is markRemoved undone, for an element that stays in the unit.
func (m *Mutator) unmarkRemoved(v any) {
	switch t := v.(type) {
	case bson.D:
		for _, e := range t {
			if e.Key == "$ID" {
				if b, ok := e.Value.(bson.Binary); ok {
					delete(m.removed, string(b.Data))
				}
				continue
			}
			m.unmarkRemoved(e.Value)
		}
	case bson.A:
		for _, el := range t {
			m.unmarkRemoved(el)
		}
	}
}

// EditedInPlace reports whether a Replace of target edited the stored
// activity, keeping its identity, rather than putting new elements in its
// place.
func (m *Mutator) EditedInPlace(target model.ID) bool { return m.edited[string(target)] }

// editInPlace replaces the top-level activity x by frag's one activity when
// the two are the same kind, and reports whether it did. The replacement is
// written where x is in the collection, drawn where the script states — or,
// when it states nothing, where x is — at x's size (a Studio Pro activity may
// have been resized), and takes x's identity (swapKeeping). The notes frag
// states for the activity come with it, drawn where they are relative to it.
// Anything else — more than one activity, or another kind — is left to the
// replace, with nothing changed.
func (m *Mutator) editInPlace(g *graph, x *node, frag *backend.MicroflowFragment) (bool, error) {
	rest, notes := withoutNotes(frag)
	if !singleActivity(rest) {
		return false, nil
	}
	obj := rest.Objects[0]
	d, err := m.deps.SerializeObject(obj)
	if err != nil {
		return false, fmt.Errorf("serialize %T: %w", obj, err)
	}
	if d == nil || !sameKind(d, x.doc) {
		return false, nil
	}
	at := x.pos
	if frag.Placed {
		p := obj.GetPosition()
		at = point{p.X, p.Y}
	}
	obj.SetPosition(model.Point{X: at.X, Y: at.Y})
	dSet(d, "RelativeMiddlePoint", at.String())
	dSet(d, "Size", x.size.String())
	if err := m.swapKeeping(g, x, dDoc(m.doc, "ObjectCollection"), d, rest.Entry); err != nil {
		return true, err
	}
	return true, m.addNotesAt(notes, at)
}

// addNotesAt adds the notes a one-activity fragment attaches to its activity,
// now drawn at at: a note drawn relative to the activity moves with it, from
// where the builder drew the activity; one whose canvas position the script
// states stays there (mendixlabs/mxcli#1598).
func (m *Mutator) addNotesAt(notes *fragmentNotes, at point) error {
	if notes == nil {
		return nil
	}
	for _, n := range notes.frag.Objects {
		if !notes.frag.Pinned[n.GetID()] {
			p := n.GetPosition()
			n.SetPosition(model.Point{X: p.X + at.X - notes.entryAt.X, Y: p.Y + at.Y - notes.entryAt.Y})
		}
	}
	return m.addFragment(notes.frag)
}

// carryPairs records in into, for d and every element under it, the $ID of
// the element of stored it stands for: built $ID (its 16 bytes as a string) →
// stored $ID. d stands for stored when the two have one $Type; see the
// package comment above for how their children and list entries pair.
func carryPairs(d, stored bson.D, into map[string][]byte) {
	if t := dString(d, "$Type"); t == "" || t != dString(stored, "$Type") {
		return
	}
	if n, s := elementID(d), elementID(stored); n != nil && s != nil {
		into[string(n)] = bytes.Clone(s)
	}
	for _, e := range d {
		if e.Key == "$ID" || e.Key == "$Type" {
			continue
		}
		switch nv := e.Value.(type) {
		case bson.D:
			if sd, ok := dGet(stored, e.Key).(bson.D); ok {
				carryPairs(nv, sd, into)
			}
		case bson.A:
			if sa, ok := dGet(stored, e.Key).(bson.A); ok {
				for _, p := range pairEntries(nv, sa) {
					carryPairs(p[0], p[1], into)
				}
			}
		}
	}
}

// elementID is d's 16-byte binary $ID, or nil.
func elementID(d bson.D) []byte {
	if b, ok := dGet(d, "$ID").(bson.Binary); ok && len(b.Data) == 16 {
		return b.Data
	}
	return nil
}

// entryNames are the properties by which a list entry names what it is about,
// in the order they are tried: a member change names its attribute or its
// association (one of the two is empty), a parameter mapping its parameter, a
// translation its language.
var entryNames = []string{"Attribute", "Association", "Parameter", "LanguageCode", "Name", "Key"}

// entryKey is what a list entry is about — its $Type, the first of
// entryNames it states, and its Type where it has one (a member change that
// sets an association is not one that adds to it) — or "" for an entry that
// names nothing.
func entryKey(d bson.D) string {
	for _, k := range entryNames {
		if s := dString(d, k); s != "" {
			key := dString(d, "$Type") + "\x00" + k + "=" + s
			if t := dString(d, "Type"); t != "" {
				key += "\x00" + t
			}
			return key
		}
	}
	return ""
}

// pairEntries pairs the entries of two lists of elements: by entryKey when
// every entry on both sides names what it is about (the n-th entry with a key
// with the n-th stored one with that key), by position when none does and the
// two lists hold the same $Types in the same order, and not at all otherwise.
func pairEntries(news, olds bson.A) [][2]bson.D {
	nd, ok1 := entryDocs(news)
	od, ok2 := entryDocs(olds)
	if !ok1 || !ok2 || len(nd) == 0 || len(od) == 0 {
		return nil
	}
	named, unnamed := 0, 0
	for _, d := range append(append([]bson.D(nil), nd...), od...) {
		if entryKey(d) == "" {
			unnamed++
		} else {
			named++
		}
	}
	var out [][2]bson.D
	switch {
	case unnamed == 0:
		byKey := map[string][]bson.D{}
		for _, d := range od {
			k := entryKey(d)
			byKey[k] = append(byKey[k], d)
		}
		for _, d := range nd {
			k := entryKey(d)
			if q := byKey[k]; len(q) > 0 {
				out = append(out, [2]bson.D{d, q[0]})
				byKey[k] = q[1:]
			}
		}
	case named == 0:
		if len(nd) != len(od) {
			return nil
		}
		for i := range nd {
			if dString(nd[i], "$Type") != dString(od[i], "$Type") {
				return nil
			}
		}
		for i := range nd {
			out = append(out, [2]bson.D{nd[i], od[i]})
		}
	}
	return out
}

// entryDocs returns the elements of a Mendix list, without its marker; false
// when it holds anything but elements (strings, pointers).
func entryDocs(a bson.A) ([]bson.D, bool) {
	var out []bson.D
	for i, v := range a {
		switch t := v.(type) {
		case bson.D:
			if dString(t, "$Type") == "" {
				return nil, false
			}
			out = append(out, t)
		case int32:
			if i != 0 {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return out, true
}

// applyCarried puts on each carried element the stored $ID it takes over, and
// rewrites every reference to it in the same pass: every 16-byte binary in the
// unit that holds the built $ID. A pair applies only while the element is in
// the unit and no element holds the stored $ID — an operation after the
// replace that took the replacement out again leaves the stored $ID removed.
// The stored $IDs that go back on are no longer removed elements.
func (m *Mutator) applyCarried() {
	if len(m.carried) == 0 {
		return
	}
	held := map[string]bool{}
	collectIDs(m.doc, held)
	sub := map[string][]byte{}
	taken := map[string]bool{}
	for n, s := range m.carried {
		if held[n] && !held[string(s)] && !taken[string(s)] {
			sub[n] = s
			taken[string(s)] = true
		}
	}
	substituteIDs(m.doc, sub)
	for _, s := range sub {
		delete(m.removed, string(s))
	}
	m.carried = map[string][]byte{}
}

// substituteIDs rewrites, in place, every 16-byte binary in v that sub maps.
func substituteIDs(v any, sub map[string][]byte) {
	repl := func(x any) (any, bool) {
		b, ok := x.(bson.Binary)
		if !ok || len(b.Data) != 16 {
			return nil, false
		}
		if s, ok := sub[string(b.Data)]; ok {
			return bson.Binary{Subtype: b.Subtype, Data: bytes.Clone(s)}, true
		}
		return nil, true
	}
	switch t := v.(type) {
	case bson.D:
		for i := range t {
			if r, isBin := repl(t[i].Value); isBin {
				if r != nil {
					t[i].Value = r
				}
				continue
			}
			substituteIDs(t[i].Value, sub)
		}
	case bson.A:
		for i := range t {
			if r, isBin := repl(t[i]); isBin {
				if r != nil {
					t[i] = r
				}
				continue
			}
			substituteIDs(t[i], sub)
		}
	}
}
