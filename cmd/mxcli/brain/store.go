// SPDX-License-Identifier: Apache-2.0

// store.go - the committed side of the brain: docs/brain/.
//
// The store lives under docs/ because it is reviewed in a pull request like any
// other change, and in its own subfolder because a Mendix project's docs/ may
// already be the customer's or Studio Pro's. A labelled subfolder can be added
// to someone else's docs tree; a decisions.md dropped into it cannot
// (PROPOSAL_project_brain.md §4.2).
package brain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// StoreDir is the store's location relative to the project directory.
const StoreDir = "docs/brain"

// Store is the committed half of the brain. The staged queue is deliberately
// not part of it — see staged.go.
type Store struct{ Root string }

// NewStore locates the store for a project directory.
func NewStore(projectDir string) *Store {
	return &Store{Root: filepath.Join(projectDir, filepath.FromSlash(StoreDir))}
}

// ErrForeignStore is returned when docs/brain/ exists but mxcli did not write
// it. Adopting it would mean writing into someone else's notes.
var ErrForeignStore = errors.New("docs/brain/ exists but was not written by mxcli")

// Exists reports whether the store has been initialised.
func (s *Store) Exists() bool {
	_, err := os.Stat(filepath.Join(s.Root, "README.md"))
	return err == nil
}

// Init creates the store, returning the paths written. It refuses a docs/brain/
// that exists without mxcli's marker, and is otherwise idempotent: an existing
// store is left alone rather than overwritten, so a second `init` cannot
// silently discard entries.
func (s *Store) Init() ([]string, error) {
	if info, err := os.Stat(s.Root); err == nil && info.IsDir() {
		readme, err := os.ReadFile(filepath.Join(s.Root, "README.md"))
		if err != nil || !HasMarker(string(readme)) {
			return nil, fmt.Errorf("%w: %s", ErrForeignStore, s.Root)
		}
	}
	for _, sub := range []string{"modules", "plan"} {
		if err := os.MkdirAll(filepath.Join(s.Root, sub), 0755); err != nil {
			return nil, err
		}
	}
	var written []string
	for _, f := range []struct{ path, content string }{
		{filepath.Join(s.Root, "README.md"), readmeContent()},
		{filepath.Join(s.Root, "project.md"), RenderShard(ProjectShard, nil)},
	} {
		if _, err := os.Stat(f.path); err == nil {
			continue // never clobber an existing file
		}
		if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
			return nil, err
		}
		written = append(written, f.path)
	}
	return written, nil
}

// ShardPath is where a shard's Markdown lives.
func (s *Store) ShardPath(shard string) string {
	switch {
	case shard == ProjectShard:
		return filepath.Join(s.Root, "project.md")
	case IsPlanShard(shard):
		return filepath.Join(s.Root, "plan", SliceOf(shard)+".md")
	default:
		return filepath.Join(s.Root, "modules", shard+".md")
	}
}

// LoadShard reads a shard. A shard that does not exist is empty, not an error —
// shards appear on promotion.
func (s *Store) LoadShard(shard string) ([]Entry, []string, error) {
	b, err := os.ReadFile(s.ShardPath(shard))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return ParseShard(shard, string(b))
}

// SaveShard writes a shard, deleting it when it has no entries left. Leaving an
// empty file behind would make the directory accumulate husks that read as
// "this module has decisions" when it has none.
//
// Any YAML frontmatter already on the file is carried across. A shard is
// re-rendered from its parsed entries, which is right for the parts mxcli owns
// — title and preamble are regenerated so they cannot drift — but frontmatter
// is not one of them, and discarding it silently broke every markdown tool that
// keeps per-file metadata there. This is the single write choke point, so
// preserving here covers promote, replace, drop and rename at once
// (guard-don't-drop, ADR-0005).
func (s *Store) SaveShard(shard string, entries []Entry) error {
	return s.saveShardWith(shard, entries, s.Frontmatter(shard))
}

// saveShardWith writes a shard beneath an explicitly supplied frontmatter
// block. Only the module-rename move needs it: that writes to a path with no
// existing file, so there is nothing there to read the block back off.
func (s *Store) saveShardWith(shard string, entries []Entry, frontmatter string) error {
	path := s.ShardPath(shard)
	if len(entries) == 0 && shard != ProjectShard {
		// The file goes, frontmatter included. A shard with no decisions left
		// is not a shard, and keeping a husk alive for its metadata is the
		// accumulation this branch exists to prevent.
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Atomically: a crash mid-write must not leave a half-written shard, which
	// would read back as a shorter one and silently lose the tail (#2054).
	return writeFileAtomic(path, []byte(RenderShardWithFrontmatter(shard, entries, frontmatter)))
}

// Frontmatter returns the YAML block at the top of a shard, or "" if the shard
// has none or does not exist yet.
func (s *Store) Frontmatter(shard string) string {
	b, err := os.ReadFile(s.ShardPath(shard))
	if err != nil {
		// An unreadable shard is not this function's problem to report: the
		// caller is about to read or write it and will surface the error there.
		// Returning "" only means "no frontmatter to carry".
		return ""
	}
	return extractFrontmatter(string(b))
}

// ListShards returns every shard that exists, project first and modules sorted.
func (s *Store) ListShards() ([]string, error) {
	var shards []string
	if _, err := os.Stat(s.ShardPath(ProjectShard)); err == nil {
		shards = append(shards, ProjectShard)
	}
	mods, err := s.namesIn("modules", "")
	if err != nil {
		return nil, err
	}
	slices, err := s.namesIn("plan", PlanPrefix)
	if err != nil {
		return nil, err
	}
	shards = append(shards, mods...)
	return append(shards, slices...), nil
}

// ListSlices returns the plan shards, in name order — which is what gives a
// roadmap its order, since a slice is sorted by name and a numeric prefix is
// the user's way of sequencing it.
func (s *Store) ListSlices() ([]string, error) { return s.namesIn("plan", PlanPrefix) }

func (s *Store) namesIn(sub, prefix string) ([]string, error) {
	ents, err := os.ReadDir(filepath.Join(s.Root, sub))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, prefix+strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(out)
	return out, nil
}

// ErrCapExceeded reports a promotion that would push a shard past its budget.
type ErrCapExceeded struct {
	Shard      string
	Would, Cap int
}

func (e *ErrCapExceeded) Error() string {
	return fmt.Sprintf("promoting into %s would take it to %d lines, past its %d-line cap; "+
		"drop or condense an entry there first (mxcli brain show %s)", e.Shard, e.Would, e.Cap, e.Shard)
}

// Promote appends an entry to a shard. This is the only writer of a committed
// file, and it is the point where the cap bites — refusing with the shard named
// and its occupancy, rather than letting the store grow past what a session can
// afford to load.
func (s *Store) Promote(e Entry, shard string) error {
	entries, _, err := s.LoadShard(shard)
	if err != nil {
		return err
	}
	for _, existing := range entries {
		if existing.ID == e.ID {
			return fmt.Errorf("%s already carries entry %s (%q)", shard, e.ID, existing.Title)
		}
		// By content as well: an edited entry keeps an id its text no longer
		// hashes to, so the same words captured again arrive with a new id.
		if existing.ContentKey() == e.ContentKey() {
			return fmt.Errorf("%s already says this, as entry %s (%q)", shard, existing.ID, existing.Title)
		}
	}
	next := append(entries, e)
	// Counted WITH frontmatter: the cap is what a session pays to load the
	// shard, `brain show` measures the file on disk, and the two disagreeing
	// would show up exactly when a shard is near its limit.
	rendered := RenderShardWithFrontmatter(shard, next, s.Frontmatter(shard))
	if lines, limit := CountLines(rendered), CapFor(shard); lines > limit {
		return &ErrCapExceeded{Shard: shard, Would: lines, Cap: limit}
	}
	return s.SaveShard(shard, next)
}

// Replace swaps an entry for a new one with the same id, in place. Used by
// resolution, where a question becomes the decision it was heading towards and
// should not move, jump to the end of the file, or change identity.
func (s *Store) Replace(shard string, e Entry) error {
	entries, _, err := s.LoadShard(shard)
	if err != nil {
		return err
	}
	found := false
	for i := range entries {
		if entries[i].ID == e.ID {
			entries[i], found = e, true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s does not carry entry %s", shard, e.ID)
	}
	return s.SaveShard(shard, entries)
}

// Find returns the entry with the given id and the shard holding it.
func (s *Store) Find(id string) (Entry, string, error) {
	shards, err := s.ListShards()
	if err != nil {
		return Entry{}, "", err
	}
	for _, sh := range shards {
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return Entry{}, "", err
		}
		for _, e := range entries {
			if e.ID == id {
				return e, sh, nil
			}
		}
	}
	return Entry{}, "", nil
}

// Drop removes an entry by id, reporting which shard it came from and whether
// that emptied the shard.
func (s *Store) Drop(id string) (shard string, deletedFile bool, err error) {
	shards, err := s.ListShards()
	if err != nil {
		return "", false, err
	}
	for _, sh := range shards {
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return "", false, err
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
			continue
		}
		if err := s.SaveShard(sh, kept); err != nil {
			return "", false, err
		}
		return sh, len(kept) == 0 && sh != ProjectShard, nil
	}
	return "", false, nil
}

// OpenQuestions lists the unanswered questions across the store. It needs no
// resolver and no catalog — just the files — so a caller that only wants to
// nag about them pays a few small reads rather than a model load.
func (s *Store) OpenQuestions() ([]OpenQuestion, error) {
	shards, err := s.ListShards()
	if err != nil {
		return nil, err
	}
	var out []OpenQuestion
	for _, sh := range shards {
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Open {
				out = append(out, OpenQuestion{Shard: sh, EntryID: e.ID, Title: e.Title})
			}
		}
	}
	return out, nil
}

// Usage computes size and headroom for every shard. Nothing here is cached or
// written down (A6).
func (s *Store) Usage() ([]Usage, error) {
	shards, err := s.ListShards()
	if err != nil {
		return nil, err
	}
	out := make([]Usage, 0, len(shards))
	for _, sh := range shards {
		b, err := os.ReadFile(s.ShardPath(sh))
		if err != nil {
			return nil, err
		}
		entries, _, err := ParseShard(sh, string(b))
		if err != nil {
			return nil, err
		}
		out = append(out, Usage{Shard: sh, Entries: len(entries), Lines: CountLines(string(b)), Cap: CapFor(sh)})
	}
	return out, nil
}

func readmeContent() string {
	return `# Project brain

` + shardMarker + `

Project knowledge that mxcli cannot compute: why a pattern was chosen here,
which marketplace version broke what, what a recurring mxbuild error means in
this app.

**Anything mxcli can answer does not belong here.** Entities, microflows, pages,
bindings and references are all queryable — a note that transcribes them is a
note that will disagree with the project.

## Layout

    project.md           cross-cutting decisions; loaded every session
    modules/<Module>.md  decisions anchored to one module; loaded when it is in play

An entry's anchors decide its file: ` + "`@Sales.Order`" + ` puts it in
` + "`modules/Sales.md`" + `. An entry with no anchor is cross-cutting and lives in
` + "`project.md`" + `.

## Working with it

    mxcli brain capture "<text>" --anchor @Module.Element   queue something
    mxcli brain staged                                      review the queue
    mxcli brain promote <id>                                write it into a shard
    mxcli brain check                                       anchors still resolve?
    mxcli brain show                                        size and headroom

Promotion is a human step on purpose: an agent queues, a person decides what is
worth committing.

Sizes and headroom are computed by ` + "`mxcli brain show`" + ` and are deliberately
not written down anywhere, including in this file — a figure in prose is stale
the next time anyone promotes.
`
}
