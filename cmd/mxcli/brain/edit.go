// SPDX-License-Identifier: Apache-2.0

// edit.go - changing an entry without changing which entry it is.
//
// An entry's id is a handle: `brain promote <id>`, `brain resolve <id>`, prose
// and other entries cite it. It is derived from the content when the entry is
// captured, but once it exists it is NOT re-derived — the rule rename.go and
// Resolve already follow (ako/mxcli#1950). Before this file there was no edit
// path at all, so every correction went drop + capture and orphaned the handle.
//
// The consequence the rest of the package has to absorb: a stored id need not
// match the text it sits beside. So duplicates are found by ContentKey — the
// hash of what the entry says now — never by comparing stored ids, which would
// miss the edited wording captured again and collide with the original wording.
package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
)

// ContentKey is the hash of what the entry currently says: title, slice and
// anchors, exactly what computeID covers. For an entry never edited, resolved
// or re-anchored it equals the id; for one that was, it does not, and that
// difference is the point — the key identifies content, the id identifies the
// entry.
func (e Entry) ContentKey() string { return e.computeID() }

// Edit returns the entry with new text and, when replaceAnchors is set, new
// anchors. Everything else — id, kind, slice, open, date — is kept. It is
// validated as capture validates, so an edit cannot produce an entry capture
// would have refused.
func (e Entry) Edit(text string, anchors []string, replaceAnchors bool) (Entry, error) {
	title, body := splitTitle(text)
	if title == "" {
		return Entry{}, fmt.Errorf("an entry needs at least a title line")
	}
	out := e
	out.Title, out.Body = title, body
	if replaceAnchors {
		out.Anchors = slices.Clone(anchors)
	}
	if _, err := ParseAnchors(out.Anchors); err != nil {
		return Entry{}, err
	}
	if out.EntryKind() == KindRequirement && !out.Open {
		if err := requirementAnchorsArePlannable(out.Anchors); err != nil {
			return Entry{}, err
		}
	}
	return out, nil
}

// MoveToSlice returns a requirement (or a question filed against a slice)
// moved to another slice. A decision is refused: giving one a slice turns it
// into a requirement, whose anchors stop failing when they go stale — a change
// of meaning, not of place.
func (e Entry) MoveToSlice(slice string) (Entry, error) {
	if e.EntryKind() != KindRequirement {
		return Entry{}, fmt.Errorf("%s is a decision, which has no slice; a decision's file follows its anchors (re-anchor it with 'brain edit %s -a @Module.Element')", e.ID, e.ID)
	}
	if !sliceName.MatchString(slice) {
		return Entry{}, fmt.Errorf("slice %q: use letters, digits, '-' and '_' (a leading number orders it: 01-accounts)", slice)
	}
	out := e
	out.Slice = slice
	return out, nil
}

// TakenIDs is every id in use, committed or staged.
func TakenIDs(s *Store, q *Queue) (map[string]bool, error) {
	taken := map[string]bool{}
	staged, err := q.Load()
	if err != nil {
		return nil, err
	}
	for _, e := range staged {
		taken[e.ID] = true
	}
	shards, err := s.ListShards()
	if err != nil {
		return nil, err
	}
	for _, sh := range shards {
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			taken[e.ID] = true
		}
	}
	return taken, nil
}

// AssignFreeID gives a newly captured entry an id no existing entry holds.
//
// Normally that is its content-derived id, unchanged. It differs only when the
// id is already a handle for an entry whose text has since changed — capture
// the ORIGINAL wording of an edited entry and it hashes straight to that
// entry's id. The caller has already established this is not a duplicate (the
// content keys differ), so it is a new fact, and it gets a new id derived from
// the same content plus a counter: deterministic, and still six hex digits.
func AssignFreeID(e Entry, taken map[string]bool) Entry {
	for n := 1; taken[e.ID]; n++ {
		h := sha256.Sum256([]byte(e.ContentKey() + "\x02" + strconv.Itoa(n)))
		e.ID = hex.EncodeToString(h[:])[:6]
	}
	return e
}

// Moved reports where an Update took an entry from and to; the same shard when
// it stayed put.
type Moved struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Update changes a committed entry in place, keeping its id.
//
// It stays at its position in its shard unless the change moves it: a
// requirement follows its slice, and a decision follows its anchors — but only
// if it was where its anchors put it. One placed elsewhere on purpose (promote
// --to project) stays where it was put.
//
// A change that would make the entry say the same as another committed entry
// is refused, naming that entry: two copies of one fact is what the store
// exists to prevent.
func (s *Store) Update(id string, change func(Entry) (Entry, error)) (Moved, error) {
	old, from, err := s.Find(id)
	if err != nil {
		return Moved{}, err
	}
	if from == "" {
		return Moved{}, fmt.Errorf("no committed entry with id %s", id)
	}
	next, err := change(old)
	if err != nil {
		return Moved{}, err
	}
	next.ID = old.ID // a handle: whatever the change did, this does not move

	if dup, err := s.findContentExcept(next.ContentKey(), id); err != nil {
		return Moved{}, err
	} else if dup.ID != "" {
		return Moved{}, fmt.Errorf("that would make %s say the same as %s (%q)", id, dup.ID, dup.Title)
	}

	to := from
	switch {
	case next.EntryKind() == KindRequirement:
		to = next.Shard()
	case from == old.Shard():
		to = next.Shard()
	}
	if to == from {
		return Moved{From: from, To: to}, s.Replace(from, next)
	}
	// Into the new shard first, then out of the old: a failure in between
	// leaves the entry in both files, visible to `check`, rather than in none.
	if err := s.Promote(next, to); err != nil {
		return Moved{}, err
	}
	return Moved{From: from, To: to}, s.removeFrom(from, id)
}

// FindContent returns the committed entry whose content key is key, if any —
// the duplicate check for a capture, which has no id of its own to compare yet.
func (s *Store) FindContent(key string) (Entry, error) { return s.findContentExcept(key, "") }

// findContentExcept returns a committed entry, other than except, whose content
// key is key.
func (s *Store) findContentExcept(key, except string) (Entry, error) {
	shards, err := s.ListShards()
	if err != nil {
		return Entry{}, err
	}
	for _, sh := range shards {
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return Entry{}, err
		}
		for _, e := range entries {
			if e.ID != except && e.ContentKey() == key {
				return e, nil
			}
		}
	}
	return Entry{}, nil
}

// removeFrom drops one entry from one named shard. Drop searches every shard
// for the id, which during a move would find whichever copy comes first.
func (s *Store) removeFrom(shard, id string) error {
	entries, _, err := s.LoadShard(shard)
	if err != nil {
		return err
	}
	return s.SaveShard(shard, slices.DeleteFunc(entries, func(e Entry) bool { return e.ID == id }))
}

// Update changes a staged entry in place, keeping its id and its position in
// the queue — which matters, because the queue's order is the timeline
// `staged --since` reads.
func (q *Queue) Update(id string, change func(Entry) (Entry, error)) (bool, error) {
	release, err := acquireQueueLock(q.Path)
	if err != nil {
		return false, err
	}
	defer release()

	entries, err := q.Load()
	if err != nil {
		return false, err
	}
	at := slices.IndexFunc(entries, func(e Entry) bool { return e.ID == id })
	if at < 0 {
		return false, nil
	}
	next, err := change(entries[at])
	if err != nil {
		return false, err
	}
	next.ID = id
	for i, e := range entries {
		if i != at && e.ContentKey() == next.ContentKey() {
			return false, fmt.Errorf("that would make %s say the same as staged %s (%q)", id, e.ID, e.Title)
		}
	}
	entries[at] = next
	return true, q.write(entries)
}
