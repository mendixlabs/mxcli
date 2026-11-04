// SPDX-License-Identifier: Apache-2.0

// list.go - the committed store as data.
//
// `brief --json` returns each shard's rendered markdown, which is right for a
// prompt and wrong for a UI: rendering a tree from it means parsing a format
// mxcli owns, a second parser of the same thing that drifts the first time the
// format changes (ako/mxcli#1952). List hands out the parsed entries instead.
package brain

import (
	"fmt"
	"slices"
)

// ListFilter narrows a listing. The zero value lists everything.
type ListFilter struct {
	// Slice keeps one slice's entries: its requirements and the questions
	// filed against it.
	Slice string
	// Kind keeps decisions or requirements. An open question is a decision
	// unless it was filed against a slice.
	Kind Kind
	// Open, when set, keeps only open questions (true) or only settled
	// entries (false).
	Open *bool
}

// ListedEntry is one committed entry with the shard it is in — the same shape
// `staged --json` gives a queued entry, so a consumer reads both sides alike.
type ListedEntry struct {
	Entry
	Shard string `json:"shard"`
}

// ListResult is a listing plus the blocks that could not be read. Those are
// reported rather than dropped: a UI that silently shows fewer entries than
// the file holds is the failure a structured read exists to prevent.
type ListResult struct {
	Entries   []ListedEntry `json:"entries"`
	Malformed []string      `json:"malformed"`
}

// List returns the committed entries matching f, in store order: project.md,
// then modules, then slices, and file order within each.
func (s *Store) List(f ListFilter) (ListResult, error) {
	out := ListResult{Entries: []ListedEntry{}, Malformed: []string{}}
	switch f.Kind {
	case "", KindDecision, KindRequirement:
	default:
		return out, fmt.Errorf("kind %q: use %q or %q", f.Kind, KindDecision, KindRequirement)
	}
	if f.Slice != "" && f.Kind == KindDecision {
		return out, fmt.Errorf("a decision has no slice, so --slice with --kind %s matches nothing; drop one of them", KindDecision)
	}

	shards, err := s.ListShards()
	if err != nil {
		return out, err
	}
	if f.Slice != "" {
		want := PlanShard(f.Slice)
		shards = slices.DeleteFunc(shards, func(sh string) bool { return sh != want })
	}
	for _, sh := range shards {
		entries, malformed, err := s.LoadShard(sh)
		if err != nil {
			return out, err
		}
		for _, m := range malformed {
			out.Malformed = append(out.Malformed, sh+": "+m)
		}
		for _, e := range entries {
			// Explicit, so a consumer never has to know that an empty
			// kind means a decision.
			e.Kind = e.EntryKind()
			if f.Kind != "" && e.Kind != f.Kind {
				continue
			}
			if f.Open != nil && e.Open != *f.Open {
				continue
			}
			out.Entries = append(out.Entries, ListedEntry{Entry: e, Shard: sh})
		}
	}
	return out, nil
}
