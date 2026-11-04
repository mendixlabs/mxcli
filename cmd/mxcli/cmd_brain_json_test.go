// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// The types marshalling correctly is only half of it. The other half is that
// nothing else reaches stdout — one stray Println (a "Nothing staged.", a
// progress line, the PoC banner) and the output stops being JSON while still
// looking fine to a person. So this runs the commands as a caller would and
// parses what comes back.
func TestBrainCommandsEmitParseableJSONOnStdout(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	store := brain.NewStore(dir)
	if _, err := store.Init(); err != nil {
		t.Fatal(err)
	}
	queue := brain.NewQueue(dir)
	decision, err := brain.NewEntry("Orders are committed by Finance, not Sales", []string{"@Sales.Order"}, brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Append(decision); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want []string // top-level keys the contract promises
	}{
		{"staged", []string{"brain", "staged", "-p", mpr, "--json"}, []string{"staged", "count"}},
		{"show", []string{"brain", "show", "-p", mpr, "--json"}, []string{"shards"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runBrainForTest(t, tc.args)

			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("`mxcli %v` did not emit JSON on stdout: %v\ngot:\n%s", tc.args, err, out)
			}
			for _, k := range tc.want {
				if _, ok := got[k]; !ok {
					t.Errorf("no %q key in the output of `mxcli %v`: %s", k, tc.args, out)
				}
			}
		})
	}

	// Control: without --json the same commands print for a person, and that
	// output is deliberately NOT JSON. Without this, a change that made every
	// command emit JSON unconditionally would pass the assertions above.
	out := runBrainForTest(t, []string{"brain", "staged", "-p", mpr})
	var any0 map[string]any
	if json.Unmarshal([]byte(out), &any0) == nil {
		t.Errorf("`brain staged` without --json emitted JSON; the human output was replaced rather than added to:\n%s", out)
	}
}

// An empty queue must still be an object with a count, not the words "Nothing
// staged." A dispatcher's whole question is whether a slice staged anything,
// and it has to be able to ask that when the answer is no — which is the case
// where a human-shaped message would break the parse.
func TestBrainStagedJSONIsStillJSONWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	out := runBrainForTest(t, []string{"brain", "staged", "-p", mpr, "--json"})

	var got struct {
		Count  int   `json:"count"`
		Staged []any `json:"staged"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("an empty queue did not emit JSON: %v\ngot:\n%s", err, out)
	}
	if got.Count != 0 || len(got.Staged) != 0 {
		t.Errorf("empty queue reported %d staged: %s", got.Count, out)
	}
}

// runBrainForTest executes a command through the real root, so the persistent
// --json flag and PersistentPreRun are exercised exactly as a caller gets them.
//
// rootCmd is a package global and cobra keeps flag values between runs, so
// --json is reset first: without that, every run after a --json one inherits it
// and the control below (human output is NOT JSON) passes for the wrong reason.
func runBrainForTest(t *testing.T, args []string) string {
	t.Helper()
	resetBrainFlagsForTest(t)

	out, err := captureStdout(t, func() error {
		rootCmd.SetArgs(args)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("`mxcli %v` failed: %v\n%s", args, err, out)
	}
	return out
}

// resetBrainFlagsForTest clears what cobra carries between runs: --json, and
// -p / -C on every brain command. A -C left over from one run makes the next
// run's -p look like both were passed, which is refused.
func resetBrainFlagsForTest(t *testing.T) {
	t.Helper()
	if err := rootCmd.PersistentFlags().Set("json", "false"); err != nil {
		t.Fatal(err)
	}
	globalJSONFlag = false
	for _, c := range brainLocatedCommands() {
		for _, name := range []string{"project", "dir"} {
			if err := c.Flags().Set(name, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func brainTestDay() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }

// The dispatcher's loop, end to end: note the queue's last id, run the slice,
// ask what it staged. The two things that make it usable are asserted here
// because neither is obvious from the filter alone.
//
//  1. last_id comes from the UNFILTERED queue and is reported even when nothing
//     matched. A slice that staged nothing must still hand the next slice a
//     boundary, or the next one re-reports this one's captures.
//  2. count 0 is qualified by "filtered", because the number alone cannot say
//     whether the queue is empty or the filter matched nothing — and only one
//     of those means the slice did not do its job.
func TestStagedSinceGivesADispatcherItsSliceBoundary(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	queue := brain.NewQueue(dir)
	stage := func(text string) brain.Entry {
		t.Helper()
		e, err := brain.NewEntry(text, nil, brainTestDay())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := queue.Append(e); err != nil {
			t.Fatal(err)
		}
		return e
	}

	boundary := stage("a decision from the slice before this one")

	type staged struct {
		Count    int    `json:"count"`
		LastID   string `json:"last_id"`
		Filtered bool   `json:"filtered"`
	}
	decode := func(out string) staged {
		t.Helper()
		var s staged
		if err := json.Unmarshal([]byte(out), &s); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
		return s
	}

	// The slice staged nothing.
	got := decode(runBrainForTest(t, []string{"brain", "staged", "-p", mpr, "--since", boundary.ID, "--json"}))
	if got.Count != 0 {
		t.Errorf("count %d, want 0", got.Count)
	}
	if !got.Filtered {
		t.Error("a zero count was not marked as filtered; a caller cannot tell it from an empty queue")
	}
	if got.LastID != boundary.ID {
		t.Errorf("last_id is %q, want %q — a slice that staged nothing must still pass the boundary on",
			got.LastID, boundary.ID)
	}

	// Now it stages something, including a decision, which carries no slice.
	found := stage("a decision found while building the slice")
	got = decode(runBrainForTest(t, []string{"brain", "staged", "-p", mpr, "--since", boundary.ID, "--json"}))
	if got.Count != 1 {
		t.Errorf("count %d, want 1", got.Count)
	}
	if got.LastID != found.ID {
		t.Errorf("last_id is %q, want the newest entry %q", got.LastID, found.ID)
	}
}

// The brief has to be pipeable: the pack on stdout, the commentary on stderr.
// If the size line landed on stdout, every use of `brain brief | ...` would
// feed a session a line about token counts as though it were a decision.
func TestBriefPutsThePackOnStdoutAndTheSizeOnStderr(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	store := brain.NewStore(dir)
	if _, err := store.Init(); err != nil {
		t.Fatal(err)
	}
	for _, e := range []brain.Entry{
		mustTestEntry(t, "the whole app is single-tenant"),
		mustTestEntry(t, "planning uses a snapshot", "@Planning.Snapshot"),
		mustTestEntry(t, "billing runs nightly", "@Billing.ACT_Post"),
		mustTestRequirement(t, "roll up by cost centre", "07-planning", "@Planning.ACT_Rollup"),
	} {
		if err := store.Promote(e, e.Shard()); err != nil {
			t.Fatal(err)
		}
	}

	out := runBrainForTest(t, []string{"brain", "brief", "--slice", "07-planning", "-p", mpr})

	for _, want := range []string{"single-tenant", "snapshot", "roll up by cost centre"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout does not contain %q", want)
		}
	}
	if strings.Contains(out, "billing runs nightly") {
		t.Error("a module the slice does not touch is in the pack")
	}
	if strings.Contains(out, "whole store is") {
		t.Error("the size line is on stdout; `brain brief | ...` would feed it to the session as content")
	}
}

func mustTestEntry(t *testing.T, text string, anchors ...string) brain.Entry {
	t.Helper()
	e, err := brain.NewEntry(text, anchors, brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func mustTestRequirement(t *testing.T, text, slice string, anchors ...string) brain.Entry {
	t.Helper()
	e, err := brain.NewRequirement(text, anchors, slice, brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	return e
}
