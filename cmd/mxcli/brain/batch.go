// SPDX-License-Identifier: Apache-2.0

// batch.go - capturing and promoting many entries as one act (ako/mxcli#2054).
//
// The point is the transaction boundary, not speed. Fifty single captures or
// promotes are fifty places to stop half way — a cap refusal, a bad anchor or a
// killed process at entry 23 leaves 22 done, 28 not, and nobody sure which. So
// a batch validates every item before it writes anything, and a write that
// cannot be atomic (a promote touches several shards and the queue) is ordered
// so that a crash leaves a state a plain re-run completes.
package brain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BatchItem is one entry of `brain capture --from-json`. The fields are the
// input half of what `staged --json` and `list --json` print, so a caller can
// feed a list straight back in.
type BatchItem struct {
	// Text is the single-argument form of capture: first line title, the rest
	// body. Give it or Title (with an optional Body), not both.
	Text    string   `json:"text,omitempty"`
	Title   string   `json:"title,omitempty"`
	Body    string   `json:"body,omitempty"`
	Anchors []string `json:"anchors,omitempty"`
	Slice   string   `json:"slice,omitempty"`
	Open    bool     `json:"open,omitempty"`
	Source  string   `json:"source,omitempty"`
}

// ParseBatch reads a JSON array of items. Unknown fields are refused: a
// misspelt "anchor" would otherwise capture an unanchored entry in silence,
// which is the kind of half-success a batch exists to rule out.
func ParseBatch(r io.Reader) ([]BatchItem, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var items []BatchItem
	if err := dec.Decode(&items); err != nil {
		return nil, fmt.Errorf("--from-json expects a JSON array of entries: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("--from-json expects one JSON array; found more after it")
	}
	return items, nil
}

// BuildBatch turns items into entries, validating every one before the caller
// writes anything. The first invalid item fails the whole batch, named by its
// index in the array, so nothing is queued from a list that is partly wrong.
//
// normalizeSource resolves an item's source the way --source does on the
// command line; nil leaves sources as given.
func BuildBatch(items []BatchItem, now time.Time, normalizeSource func(string) (string, error)) ([]Entry, error) {
	out := make([]Entry, 0, len(items))
	for i, it := range items {
		e, err := it.entry(now)
		if err == nil && it.Source != "" && normalizeSource != nil {
			e.Source, err = normalizeSource(it.Source)
		} else if err == nil {
			e.Source = it.Source
		}
		if err != nil {
			return nil, fmt.Errorf("items[%d]: %w; nothing was queued", i, err)
		}
		out = append(out, e)
	}
	return out, nil
}

func (it BatchItem) entry(now time.Time) (Entry, error) {
	text := it.Text
	switch {
	case it.Text != "" && (it.Title != "" || it.Body != ""):
		return Entry{}, fmt.Errorf("give text, or title and body, not both")
	case it.Text == "" && it.Title == "" && it.Body != "":
		return Entry{}, fmt.Errorf("a body needs a title")
	case it.Text == "":
		text = it.Title
		if it.Body != "" {
			text += "\n" + it.Body
		}
	}
	switch {
	case it.Open:
		return NewQuestion(text, it.Anchors, it.Slice, now)
	case it.Slice != "":
		return NewRequirement(text, it.Anchors, it.Slice, now)
	default:
		return NewEntry(text, it.Anchors, now)
	}
}

// Capture statuses, shared with the single form's --json report.
const (
	StatusQueued           = "queued"
	StatusAlreadyQueued    = "already_queued"
	StatusAlreadyCommitted = "already_committed"
)

// CaptureResult is one item's outcome. A duplicate carries the id of the entry
// it duplicates, so a caller can promote by it without a second lookup.
type CaptureResult struct {
	Entry  Entry
	Added  bool
	Status string
}

// CaptureBatch queues entries as one act.
//
// Duplicates are not errors: an entry matching something committed, something
// queued, or an earlier item of this batch is reported with the twin's id and
// skipped, so accepting the same list twice is a no-op rather than a failure.
// Ids are assigned against the stored ones AND those given earlier in the
// batch, or two items colliding only with each other would share a handle.
//
// The queue lock is held from the duplicate check to the single append, so a
// promote cannot rebuild the file between them.
func CaptureBatch(s *Store, q *Queue, entries []Entry) ([]CaptureResult, error) {
	release, err := acquireQueueLock(q.Path)
	if err != nil {
		return nil, err
	}
	defer release()

	staged, err := q.Load()
	if err != nil {
		return nil, err
	}
	taken, err := TakenIDs(s, q)
	if err != nil {
		return nil, err
	}
	committed := map[string]Entry{}
	if s.Exists() {
		shards, err := s.ListShards()
		if err != nil {
			return nil, err
		}
		for _, sh := range shards {
			es, _, err := s.LoadShard(sh)
			if err != nil {
				return nil, err
			}
			for _, e := range es {
				if _, ok := committed[e.ContentKey()]; !ok {
					committed[e.ContentKey()] = e
				}
			}
		}
	}
	queued := map[string]Entry{}
	for _, e := range staged {
		if _, ok := queued[e.ContentKey()]; !ok {
			queued[e.ContentKey()] = e
		}
	}

	results := make([]CaptureResult, 0, len(entries))
	var fresh []Entry
	for _, e := range entries {
		key := e.ContentKey()
		if dup, ok := committed[key]; ok {
			results = append(results, CaptureResult{Entry: dup, Status: StatusAlreadyCommitted})
			continue
		}
		// An earlier item of this batch is in `queued` by now, so an in-batch
		// twin reads as already queued — which, once the append lands, it is.
		if dup, ok := queued[key]; ok {
			results = append(results, CaptureResult{Entry: dup, Status: StatusAlreadyQueued})
			continue
		}
		e = AssignFreeID(e, taken)
		taken[e.ID] = true
		queued[key] = e
		fresh = append(fresh, e)
		results = append(results, CaptureResult{Entry: e, Added: true, Status: StatusQueued})
	}
	if len(fresh) == 0 {
		return results, nil
	}
	if err := q.appendLocked(fresh); err != nil {
		return nil, err
	}
	return results, nil
}

// appendLocked writes entries to the end of the queue in one write. The
// caller holds the queue lock.
func (q *Queue) appendLocked(entries []Entry) error {
	var buf bytes.Buffer
	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(q.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(q.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Promote statuses.
const (
	StatusPromoted = "promoted"
	// StatusAlreadyPromoted is an id whose entry is already in its shard with
	// the same content: what a promote interrupted after its shard write and
	// before its queue rewrite leaves behind, or a batch run twice.
	StatusAlreadyPromoted = "already_promoted"
)

// PromoteResult is one id's outcome.
type PromoteResult struct {
	Entry  Entry
	Shard  string
	Status string
}

// afterShardWrites runs between the shard writes and the queue rewrite. It is
// a test seam for the one interruption a multi-file write cannot rule out.
var afterShardWrites func() error

// PromoteBatch commits staged entries as one act. to, when set, is the shard
// for every id; otherwise each entry goes where its anchors route it.
//
// Everything is validated before anything is written: every id must resolve,
// and each target shard's size is computed with ALL of this batch's additions
// to it — checked entry by entry, each passes and the shard still overruns.
//
// A multi-file write cannot be atomic, so it is ordered instead: every shard
// first (each atomically), then one queue rewrite. A crash between the two
// leaves entries committed and still queued — visible, and completed by
// re-running the same promote, which counts an id already in its shard with
// the same content as promoted. The reverse order could lose an entry from
// both places; this one never can.
func PromoteBatch(s *Store, q *Queue, ids []string, to string) ([]PromoteResult, error) {
	if !s.Exists() {
		return nil, fmt.Errorf("no store yet — run 'mxcli brain init' first")
	}
	release, err := acquireQueueLock(q.Path)
	if err != nil {
		return nil, err
	}
	defer release()

	staged, err := q.Load()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Entry, len(staged))
	for _, e := range staged {
		byID[e.ID] = e
	}

	type shardPlan struct {
		entries []Entry
		changed bool
	}
	plans := map[string]*shardPlan{}
	var order []string
	load := func(shard string) (*shardPlan, error) {
		if p, ok := plans[shard]; ok {
			return p, nil
		}
		es, _, err := s.LoadShard(shard)
		if err != nil {
			return nil, err
		}
		p := &shardPlan{entries: es}
		plans[shard] = p
		order = append(order, shard)
		return p, nil
	}

	seen := map[string]bool{}
	results := make([]PromoteResult, 0, len(ids))
	drop := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		e, queuedHere := byID[id]
		if !queuedHere {
			// Not queued, but perhaps already promoted: a batch re-run after it
			// completed must read as done, not fail on its first id.
			if found, shard, err := s.Find(id); err != nil {
				return nil, err
			} else if found.ID != "" {
				results = append(results, PromoteResult{Entry: found, Shard: shard, Status: StatusAlreadyPromoted})
				continue
			}
			return nil, fmt.Errorf("no staged entry with id %s; nothing was promoted", id)
		}
		shard := e.Shard()
		if to != "" {
			shard = to
		}
		p, err := load(shard)
		if err != nil {
			return nil, err
		}
		status := StatusPromoted
		for _, existing := range p.entries {
			if existing.ID == e.ID {
				if existing.ContentKey() != e.ContentKey() {
					return nil, fmt.Errorf("%s already carries entry %s (%q); nothing was promoted", shard, e.ID, existing.Title)
				}
				status = StatusAlreadyPromoted
				break
			}
			if existing.ContentKey() == e.ContentKey() {
				return nil, fmt.Errorf("%s already says this, as entry %s (%q); nothing was promoted", shard, existing.ID, existing.Title)
			}
		}
		if status == StatusPromoted {
			p.entries = append(p.entries, e)
			p.changed = true
		}
		drop[e.ID] = true
		results = append(results, PromoteResult{Entry: e, Shard: shard, Status: status})
	}

	// The cap, per shard, with the whole batch in it. Counted with frontmatter,
	// as the single promote counts it and `brain show` measures it.
	rendered := map[string]string{}
	for _, shard := range order {
		p := plans[shard]
		if !p.changed {
			continue
		}
		r := RenderShardWithFrontmatter(shard, p.entries, s.Frontmatter(shard))
		if lines, limit := CountLines(r), CapFor(shard); lines > limit {
			return nil, &ErrCapExceeded{Shard: shard, Would: lines, Cap: limit}
		}
		rendered[shard] = r
	}

	for _, shard := range order {
		r, ok := rendered[shard]
		if !ok {
			continue
		}
		if err := writeFileAtomic(s.ShardPath(shard), []byte(r)); err != nil {
			return nil, err
		}
	}
	if afterShardWrites != nil {
		if err := afterShardWrites(); err != nil {
			return nil, err
		}
	}
	if len(drop) > 0 {
		kept := make([]Entry, 0, len(staged))
		for _, e := range staged {
			if !drop[e.ID] {
				kept = append(kept, e)
			}
		}
		if err := q.write(kept); err != nil {
			return nil, err
		}
	}
	return results, nil
}
