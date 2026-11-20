// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2054: capture and promote as one act each, so accepting many
// entries cannot stop half way with nobody sure which half.

func batchStore(t *testing.T) (*Store, *Queue) {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s, NewQueue(dir)
}

func items(texts ...string) []BatchItem {
	out := make([]BatchItem, 0, len(texts))
	for _, t := range texts {
		out = append(out, BatchItem{Text: t, Anchors: []string{"@Sales.Order"}})
	}
	return out
}

func queueLen(t *testing.T, q *Queue) int {
	t.Helper()
	es, err := q.Load()
	if err != nil {
		t.Fatal(err)
	}
	return len(es)
}

func TestBuildBatchRefusesTheWholeBatchNamingTheBadItem(t *testing.T) {
	bad := items("One", "Two", "Three")
	bad[1].Anchors = []string{"not an anchor"}
	if _, err := BuildBatch(bad, day, nil); err == nil || !strings.Contains(err.Error(), "items[1]") {
		t.Fatalf("err = %v, want a refusal naming items[1]", err)
	}

	// Control: the same batch without the bad item builds, and queues N-1.
	s, q := batchStore(t)
	good := append(bad[:1:1], bad[2])
	es, err := BuildBatch(good, day, nil)
	if err != nil {
		t.Fatalf("control batch refused: %v", err)
	}
	if _, err := CaptureBatch(s, q, es); err != nil {
		t.Fatal(err)
	}
	if n := queueLen(t, q); n != 2 {
		t.Errorf("queued %d, want 2", n)
	}
}

func TestBuildBatchRefusesAmbiguousOrEmptyItems(t *testing.T) {
	for name, it := range map[string]BatchItem{
		"text and title": {Text: "A", Title: "B"},
		"body only":      {Body: "B"},
		"nothing":        {},
		"bare module requirement": {Title: "Needs a module element", Slice: "01-x",
			Anchors: []string{"@Sales"}},
	} {
		if _, err := BuildBatch([]BatchItem{it}, day, nil); err == nil {
			t.Errorf("%s: accepted, want a refusal", name)
		}
	}
	// title + body is the two-field form of text.
	es, err := BuildBatch([]BatchItem{{Title: "Head", Body: "Prose"}}, day, nil)
	if err != nil || es[0].Title != "Head" || es[0].Body != "Prose" {
		t.Fatalf("title+body = %+v, %v", es, err)
	}
}

func TestParseBatchRefusesUnknownFields(t *testing.T) {
	// "anchor" for "anchors" would otherwise queue an unanchored entry.
	if _, err := ParseBatch(strings.NewReader(`[{"text":"A","anchor":["@Sales.Order"]}]`)); err == nil {
		t.Fatal("a misspelt field was accepted")
	}
	got, err := ParseBatch(strings.NewReader(`[{"text":"A","anchors":["@Sales.Order"]}]`))
	if err != nil || len(got) != 1 || got[0].Anchors[0] != "@Sales.Order" {
		t.Fatalf("control: %+v, %v", got, err)
	}
}

func TestCaptureBatchSkipsDuplicatesAndIsIdempotent(t *testing.T) {
	s, q := batchStore(t)
	committed := mustEntry(t, "Already decided", "@Sales.Order")
	if err := s.Promote(committed, committed.Shard()); err != nil {
		t.Fatal(err)
	}

	es, err := BuildBatch(items("Fresh one", "Already decided", "Fresh two", "Fresh one"), day, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := CaptureBatch(s, q, es)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{StatusQueued, StatusAlreadyCommitted, StatusQueued, StatusAlreadyQueued}
	for i, r := range res {
		if r.Status != want[i] || r.Added != (want[i] == StatusQueued) {
			t.Errorf("item %d: status %s added %v, want %s", i, r.Status, r.Added, want[i])
		}
	}
	if res[1].Entry.ID != committed.ID {
		t.Errorf("committed duplicate reported as %s, want the committed id %s", res[1].Entry.ID, committed.ID)
	}
	if res[3].Entry.ID != res[0].Entry.ID {
		t.Errorf("in-batch duplicate reported as %s, want its twin's id %s", res[3].Entry.ID, res[0].Entry.ID)
	}
	if n := queueLen(t, q); n != 2 {
		t.Fatalf("queued %d, want 2", n)
	}

	// The same batch again changes nothing.
	before, _ := os.ReadFile(q.Path)
	again, err := CaptureBatch(s, q, es)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range again {
		if r.Added {
			t.Errorf("re-run item %d added again", i)
		}
	}
	after, _ := os.ReadFile(q.Path)
	if string(before) != string(after) {
		t.Errorf("re-running the batch changed the queue:\n%s\n---\n%s", before, after)
	}
}

// Two items whose ids collide only with each other must get different handles:
// the second has to see the id the first was given in this batch.
func TestCaptureBatchAssignsIDsAgainstEarlierItems(t *testing.T) {
	s, q := batchStore(t)
	a, b := mustEntry(t, "Alpha", "@Sales.Order"), mustEntry(t, "Beta", "@Sales.Order")
	b.ID = a.ID // a collision nothing stored shares
	res, err := CaptureBatch(s, q, []Entry{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if !res[0].Added || !res[1].Added {
		t.Fatalf("both are different facts and should be queued: %+v", res)
	}
	if res[0].Entry.ID == res[1].Entry.ID {
		t.Fatalf("both items got id %s", res[0].Entry.ID)
	}
}

func stage(t *testing.T, s *Store, q *Queue, texts ...string) []string {
	t.Helper()
	es, err := BuildBatch(items(texts...), day, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := CaptureBatch(s, q, es)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(res))
	for _, r := range res {
		ids = append(ids, r.Entry.ID)
	}
	return ids
}

// The cap is checked with every addition of the batch in the shard. Checked one
// by one, each addition fits and the shard still overruns.
func TestPromoteBatchChecksTheCapWithTheWholeBatch(t *testing.T) {
	s, q := batchStore(t)
	const shard = "Sales"
	// Find how many fillers leave room for exactly one more entry.
	var fill []Entry
	for i := 0; ; i++ {
		fill = append(fill, mustEntry(t, fmt.Sprintf("Filler %03d", i), "@Sales.Order"))
		next := append(append([]Entry{}, fill...), mustEntry(t, "Probe one", "@Sales.Order"), mustEntry(t, "Probe two", "@Sales.Order"))
		oneMore := next[:len(next)-1]
		if CountLines(RenderShard(shard, oneMore)) <= CapFor(shard) &&
			CountLines(RenderShard(shard, next)) > CapFor(shard) {
			break
		}
		if i > 1000 {
			t.Fatal("never found the boundary")
		}
	}
	if err := s.SaveShard(shard, fill); err != nil {
		t.Fatal(err)
	}
	ids := stage(t, s, q, "Probe one", "Probe two")
	shardBefore, _ := os.ReadFile(s.ShardPath(shard))
	queueBefore, _ := os.ReadFile(q.Path)

	_, err := PromoteBatch(s, q, ids, "")
	var capErr *ErrCapExceeded
	if !errors.As(err, &capErr) || capErr.Shard != shard {
		t.Fatalf("err = %v, want the cap refused for %s", err, shard)
	}
	shardAfter, _ := os.ReadFile(s.ShardPath(shard))
	queueAfter, _ := os.ReadFile(q.Path)
	if string(shardBefore) != string(shardAfter) || string(queueBefore) != string(queueAfter) {
		t.Fatal("a refused batch changed the shard or the queue")
	}

	// Control: either one alone fits.
	res, err := PromoteBatch(s, q, ids[:1], "")
	if err != nil || len(res) != 1 || res[0].Status != StatusPromoted {
		t.Fatalf("a batch that fits was refused: %+v, %v", res, err)
	}
}

func TestPromoteBatchUnknownIDPromotesNothing(t *testing.T) {
	s, q := batchStore(t)
	ids := stage(t, s, q, "One", "Two")
	if _, err := PromoteBatch(s, q, append(ids, "ffffff"), ""); err == nil {
		t.Fatal("an unknown id was accepted")
	}
	if n := queueLen(t, q); n != 2 {
		t.Errorf("queue has %d, want both still staged", n)
	}
	if es, _, _ := s.LoadShard("Sales"); len(es) != 0 {
		t.Errorf("shard has %d entries, want none", len(es))
	}
}

// The seam a multi-file write cannot close: shards written, queue not. A re-run
// of the same promote completes it, each entry committed exactly once.
func TestPromoteBatchRetryAfterInterruptedQueueRewrite(t *testing.T) {
	s, q := batchStore(t)
	ids := stage(t, s, q, "One", "Two", "Three")

	afterShardWrites = func() error { return errors.New("killed") }
	_, err := PromoteBatch(s, q, ids, "")
	afterShardWrites = nil
	if err == nil {
		t.Fatal("the seam did not fire")
	}
	if es, _, _ := s.LoadShard("Sales"); len(es) != 3 {
		t.Fatalf("after the interruption the shard has %d entries, want 3", len(es))
	}
	if n := queueLen(t, q); n != 3 {
		t.Fatalf("after the interruption the queue has %d, want 3 (committed and still queued)", n)
	}

	res, err := PromoteBatch(s, q, ids, "")
	if err != nil {
		t.Fatalf("re-run refused: %v", err)
	}
	for _, r := range res {
		if r.Status != StatusAlreadyPromoted {
			t.Errorf("%s: status %s, want %s", r.Entry.ID, r.Status, StatusAlreadyPromoted)
		}
	}
	es, _, _ := s.LoadShard("Sales")
	if len(es) != 3 {
		t.Errorf("shard has %d entries after the retry, want each exactly once (3)", len(es))
	}
	if n := queueLen(t, q); n != 0 {
		t.Errorf("queue has %d after the retry, want 0", n)
	}

	// And once more, with the queue already empty: still done, not an error.
	if _, err := PromoteBatch(s, q, ids, ""); err != nil {
		t.Errorf("a completed batch re-run failed: %v", err)
	}
}

func TestPromoteBatchToAppliesToEveryID(t *testing.T) {
	s, q := batchStore(t)
	ids := stage(t, s, q, "One", "Two")
	res, err := PromoteBatch(s, q, ids, ProjectShard)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Shard != ProjectShard {
			t.Errorf("%s went to %s, want %s", r.Entry.ID, r.Shard, ProjectShard)
		}
	}
}

// Shards are written through writeFileAtomic, which leaves no temporary file
// behind and replaces rather than truncates.
func TestShardWritesLeaveNoTemporaries(t *testing.T) {
	s, q := batchStore(t)
	ids := stage(t, s, q, "One", "Two")
	if _, err := PromoteBatch(s, q, ids, ""); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(s.ShardPath("Sales")), ".*tmp-*"))
	if len(matches) != 0 {
		t.Errorf("temporary files left behind: %v", matches)
	}
}
