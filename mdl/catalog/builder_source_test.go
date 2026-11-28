// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// #912: buildSource discarded every describe failure —
//
//	text, err := b.describeFunc(...)
//	if err == nil && text != "" { results[idx] = ... }
//
// so a document type the executor could not describe contributed zero rows and
// said nothing about it. On the reporter's project that hid 13 nanoflows and a
// rule: 114 documents collected, 100 inserted, and the 14 missing ones were only
// findable by subtracting the counts by hand.
//
// runDescribes must therefore return what failed, so the caller can report it.
func TestRunDescribes_ReturnsFailuresInsteadOfSwallowingThem(t *testing.T) {
	items := []sourceItem{
		{SourceEntity, "Mod.Customer", "Mod", ""},
		{SourceNanoflow, "Mod.ACT_Save", "Mod", ""},
		{SourceRule, "Mod.RL_IsActive", "Mod", ""},
		{SourceMicroflow, "Mod.IVK_Save", "Mod", ""},
	}

	// Stands in for the executor dispatch before the fix: nanoflows and rules
	// were unreachable, everything else described fine.
	describe := func(objType, qn, _ string) (string, error) {
		switch objType {
		case SourceNanoflow:
			return "", errors.New("nanoflow not found: " + qn)
		case SourceRule:
			return "", errors.New("unsupported object type for describe: " + objType)
		}
		return "create " + strings.ToLower(objType) + " " + qn + ";", nil
	}

	results, failures := runDescribes(items, describe, 2, nil, nil)

	var described int
	for _, r := range results {
		if r.text != "" {
			described++
		}
	}
	if described != 2 {
		t.Errorf("described %d documents, want 2 (the entity and the microflow)", described)
	}

	if len(failures) != 2 {
		t.Fatalf("got %d failures, want 2 — a describe that produces no row must be reported, not dropped", len(failures))
	}

	byType := map[string]string{}
	for _, f := range failures {
		byType[f.item.objType] = f.reason
	}
	for _, objType := range []string{SourceNanoflow, SourceRule} {
		reason, ok := byType[objType]
		if !ok {
			t.Errorf("no failure recorded for %s", objType)
			continue
		}
		if reason == "" {
			t.Errorf("%s failure has an empty reason — the report needs to say why", objType)
		}
	}
}

// A describe that returns no error but also no text still produces no row. That
// is the same silent drop wearing a different mask, so it counts as a failure.
func TestRunDescribes_EmptyOutputCountsAsFailure(t *testing.T) {
	items := []sourceItem{{SourcePage, "Mod.Home", "Mod", ""}}

	_, failures := runDescribes(items, func(string, string, string) (string, error) {
		return "", nil
	}, 1, nil, nil)

	if len(failures) != 1 {
		t.Fatalf("got %d failures, want 1 — empty describe output indexes nothing", len(failures))
	}
	if !strings.Contains(failures[0].reason, "empty") {
		t.Errorf("reason = %q, want it to mention the empty output", failures[0].reason)
	}
}

func TestRunDescribes_AllSucceeding(t *testing.T) {
	items := []sourceItem{
		{SourceEntity, "Mod.A", "Mod", ""},
		{SourceEntity, "Mod.B", "Mod", ""},
	}
	results, failures := runDescribes(items, func(_, qn, _ string) (string, error) {
		return "create entity " + qn + ";", nil
	}, 4, nil, nil)

	if len(failures) != 0 {
		t.Errorf("got %d failures, want 0: %v", len(failures), failures)
	}
	// Results are positional — a worker pool must not reorder them, or a
	// document's source text would be filed under another document's name.
	if results[0].item.qn != "Mod.A" || results[1].item.qn != "Mod.B" {
		t.Errorf("results out of order: %q, %q", results[0].item.qn, results[1].item.qn)
	}
}

func TestRunDescribes_NoItems(t *testing.T) {
	results, failures := runDescribes(nil, func(string, string, string) (string, error) {
		t.Fatal("describe called with no items")
		return "", nil
	}, 4, nil, nil)
	if len(results) != 0 || len(failures) != 0 {
		t.Errorf("got %d results and %d failures, want none", len(results), len(failures))
	}
}

// The exported list is what the executor's dispatch test iterates, so a constant
// that is not in it is invisible to that guard.
func TestSourceObjectTypesListsEveryConstant(t *testing.T) {
	constants := []string{
		SourceEntity, SourceMicroflow, SourceNanoflow, SourceRule,
		SourcePage, SourceSnippet, SourceEnumeration, SourceWorkflow,
		SourceJsonStructure, SourceImportMapping, SourceExportMapping,
	}

	listed := map[string]int{}
	for _, s := range SourceObjectTypes {
		listed[s]++
	}

	for _, c := range constants {
		if listed[c] == 0 {
			t.Errorf("SourceObjectTypes is missing %q", c)
		}
	}
	for s, n := range listed {
		if n > 1 {
			t.Errorf("SourceObjectTypes lists %q %d times", s, n)
		}
	}
	if len(SourceObjectTypes) != len(constants) {
		t.Errorf("SourceObjectTypes has %d entries, %d constants are declared", len(SourceObjectTypes), len(constants))
	}
}

// A describe that never returns hung the whole build with "source 2183/2184"
// on screen and 0% CPU (ako/mxcli#2641). It is given up on after
// describeStallAfter: recorded as a failure, reported, and the rest of the
// queue still runs — even when every worker is stuck.
func TestRunDescribes_StalledDescribeIsGivenUpOn_2641(t *testing.T) {
	old := describeStallAfter
	describeStallAfter = 50 * time.Millisecond
	defer func() { describeStallAfter = old }()

	block := make(chan struct{})
	defer close(block)
	items := []sourceItem{
		{SourcePage, "Mod.Stuck", "Mod", ""},
		{SourcePage, "Mod.A", "Mod", ""},
		{SourcePage, "Mod.B", "Mod", ""},
	}
	var reported []string
	finished := make(chan struct{})
	var results []sourceResult
	var failures []describeFailure
	go func() {
		results, failures = runDescribes(items, func(_, qn, _ string) (string, error) {
			if qn == "Mod.Stuck" {
				<-block
			}
			return "create page " + qn + ";", nil
		}, 1, nil, func(it sourceItem, _ time.Duration) { reported = append(reported, it.qn) })
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("runDescribes still waiting on a describe that never returns")
	}
	if len(failures) != 1 || failures[0].item.qn != "Mod.Stuck" || !strings.Contains(failures[0].reason, "did not finish") {
		t.Errorf("failures %v, want Mod.Stuck given up on", failures)
	}
	if len(reported) != 1 || reported[0] != "Mod.Stuck" {
		t.Errorf("reported %v, want Mod.Stuck", reported)
	}
	// With one worker, stuck first, the others only ran because a fresh
	// worker took its place.
	if results[1].text == "" || results[2].text == "" {
		t.Errorf("the rest of the queue did not run: %+v", results)
	}
}
