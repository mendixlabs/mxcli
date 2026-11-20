// SPDX-License-Identifier: Apache-2.0

// staged.go - the queue an agent writes to.
//
// The queue is deliberately NOT sharded. Staging is a queue, not a store, and
// routing it would force the shard decision before a human has looked at the
// entry — which is exactly the decision promotion exists to make. It lives
// under .mxcli/, which `mxcli init` git-ignores, so nothing reaches a pull
// request until someone promotes it.
package brain

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StagedPath is the queue's location relative to the project directory.
const StagedPath = ".mxcli/brain/staged.jsonl"

// Queue is the staged half of the brain.
type Queue struct{ Path string }

// NewQueue locates the queue for a project directory.
func NewQueue(projectDir string) *Queue {
	return &Queue{Path: filepath.Join(projectDir, filepath.FromSlash(StagedPath))}
}

// Load reads the queue. A missing file is an empty queue, not an error.
func (q *Queue) Load() ([]Entry, error) {
	f, err := os.Open(q.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(text), &e); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", q.Path, line, err)
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// Append queues an entry, refusing one whose id is already present.
//
// The id is content-derived, so this duplicate check costs nothing and needs no
// prose comparison: capturing the same fact twice produces the same id. It is
// cheap insurance rather than a load-bearing guard — the duplicate flood that
// motivated it in mxcli's own findings store was a many-parallel-writers
// problem, and one developer on one project has little exposure to it (A5).
func (q *Queue) Append(e Entry) (added bool, err error) {
	// The write itself is a short O_APPEND and needs no protection. The lock is
	// held for the duplicate check, and — more importantly — so that a promote
	// cannot be rebuilding the file from a snapshot taken before this line.
	release, err := acquireQueueLock(q.Path)
	if err != nil {
		return false, err
	}
	defer release()

	entries, err := q.Load()
	if err != nil {
		return false, err
	}
	// By content, not by stored id: an edited entry keeps an id its text no
	// longer hashes to (edit.go).
	for _, existing := range entries {
		if existing.ID == e.ID || existing.ContentKey() == e.ContentKey() {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(q.Path), 0755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(q.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return false, err
	}
	return true, nil
}

// Drop removes an entry from the queue by id.
//
// This is the read-modify-write half of a promote, so the lock spans the load
// and the rewrite: without it a capture landing between the two is overwritten
// by a queue rebuilt from before it existed.
func (q *Queue) Drop(id string) (bool, error) {
	release, err := acquireQueueLock(q.Path)
	if err != nil {
		return false, err
	}
	defer release()

	entries, err := q.Load()
	if err != nil {
		return false, err
	}
	kept := make([]Entry, 0, len(entries))
	found := false
	for _, e := range entries {
		if e.ID == id {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return false, nil
	}
	return true, q.write(kept)
}

func (q *Queue) write(entries []Entry) error {
	if len(entries) == 0 {
		err := os.Remove(q.Path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var b strings.Builder
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return writeFileAtomic(q.Path, []byte(b.String()))
}

// Get returns the queued entry with the given id.
func (q *Queue) Get(id string) (Entry, bool, error) {
	entries, err := q.Load()
	if err != nil {
		return Entry{}, false, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e, true, nil
		}
	}
	return Entry{}, false, nil
}

// StagedFilter narrows the queue to what a caller actually wants to see.
//
// The queue is a flat, append-only list of everything ever staged, which is the
// right shape for a person reviewing before a promote and the wrong one for a
// dispatcher asking what a single slice recorded.
type StagedFilter struct {
	// SinceID is the id of the last entry that was already in the queue.
	// Everything after it — exclusively — is what has been staged since.
	//
	// This is the honest slice boundary. The queue is append-only, so its own
	// order IS the timeline; Entry.Date is a day, so every capture in a session
	// shares one value and cannot separate anything.
	SinceID string
	// Slice matches requirements of one slice. Note this is NOT "what slice 07
	// staged": `capture --slice` is what makes an entry a requirement, so a
	// decision found while building a slice carries no slice at all. Use
	// SinceID for that question and this one for queued scope.
	Slice string
}

// Empty reports whether the filter would return the queue unchanged.
func (f StagedFilter) Empty() bool { return f.SinceID == "" && f.Slice == "" }

// FilterStaged applies f to entries, preserving queue order.
//
// An unknown SinceID is an error rather than an empty result. Empty is a
// meaningful answer here — "this slice recorded nothing", which a dispatcher
// acts on — so producing it from a typo or an id that has since been promoted
// would make the caller abort a slice that had in fact done its job.
func FilterStaged(entries []Entry, f StagedFilter) ([]Entry, error) {
	out := entries
	if f.SinceID != "" {
		at := -1
		for i, e := range out {
			if e.ID == f.SinceID {
				at = i
				break
			}
		}
		if at < 0 {
			return nil, fmt.Errorf("no staged entry with id %s; it may have been promoted or dropped "+
				"since it was noted (an empty result means the slice staged nothing, so this cannot "+
				"be reported as one)", f.SinceID)
		}
		out = out[at+1:]
	}
	if f.Slice != "" {
		kept := make([]Entry, 0, len(out))
		for _, e := range out {
			if e.Slice == f.Slice {
				kept = append(kept, e)
			}
		}
		out = kept
	}
	return out, nil
}
