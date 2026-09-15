// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"
)

// The measured points: 32 types in 11.6.0, 36 in 11.10.0, 42 in 11.13.0 and
// 11.14.0 (ako/TestApp stores all 42 for a handler named OnAnyEvent).
func TestWorkflowEventTypes_AnySetPerVersion(t *testing.T) {
	cases := []struct {
		major, minor, patch int
		want                int // -1 = not known
	}{
		{10, 24, 0, -1},
		{11, 5, 9, -1},
		{11, 6, 0, 32},
		{11, 9, 3, 32}, // between points: the older point's set, never a guess upward
		{11, 10, 0, 36},
		{11, 12, 4, 36},
		{11, 13, 0, 42},
		{11, 14, 0, 42},
		{12, 0, 0, 42},
	}
	for _, c := range cases {
		types, _, ok := allWorkflowEventTypes(c.major, c.minor, c.patch)
		switch {
		case c.want < 0 && ok:
			t.Errorf("%d.%d.%d: got %d types, want not known", c.major, c.minor, c.patch, len(types))
		case c.want >= 0 && (!ok || len(types) != c.want):
			t.Errorf("%d.%d.%d: got %d types (ok=%v), want %d", c.major, c.minor, c.patch, len(types), ok, c.want)
		}
	}
	all, _, _ := allWorkflowEventTypes(11, 14, 0)
	if !reflect.DeepEqual(all, workflowEventTypeOrder) {
		t.Errorf("11.14 set is not Studio Pro's stored order:\n%v", all)
	}
}

// Every ordered type belongs to exactly one measured point, or `any` would skip
// or double it.
func TestWorkflowEventTypes_TableIsConsistent(t *testing.T) {
	seen := map[string]int{}
	for _, p := range workflowEventTypePoints {
		for _, n := range p.added {
			seen[n]++
		}
	}
	for _, n := range workflowEventTypeOrder {
		if seen[n] != 1 {
			t.Errorf("%s appears in %d points", n, seen[n])
		}
	}
	if len(seen) != len(workflowEventTypeOrder) {
		t.Errorf("points name %d types, order lists %d", len(seen), len(workflowEventTypeOrder))
	}
}

// A type is refused only where it is measured absent. Between the last absence
// and the first sighting the answer is not known, and a type the author named is
// let through.
func TestWorkflowEventTypes_MissingIn(t *testing.T) {
	cases := []struct {
		name                string
		major, minor, patch int
		missing             bool
	}{
		{"NotificationStarted", 11, 13, 0, false},
		{"NotificationStarted", 11, 12, 0, false}, // unknown, not refused
		{"NotificationStarted", 11, 10, 0, true},
		{"NotificationStarted", 11, 6, 0, true},
		{"NotificationStarted", 10, 24, 0, true},
		{"AIAgentTaskStarted", 11, 6, 0, true},
		{"AIAgentTaskStarted", 11, 7, 0, false},
		{"UserTaskStarted", 10, 7, 0, false}, // oldest point: never measured absent
	}
	for _, c := range cases {
		if _, got := workflowEventTypeMissingIn(c.name, c.major, c.minor, c.patch); got != c.missing {
			t.Errorf("%s in %d.%d.%d: missing = %v, want %v", c.name, c.major, c.minor, c.patch, got, c.missing)
		}
	}
}

func TestWorkflowEventTypes_CanonicalAndSorted(t *testing.T) {
	if got, ok := canonicalWorkflowEventType("usertaskstarted"); !ok || got != "UserTaskStarted" {
		t.Errorf("canonical(usertaskstarted) = %q, %v", got, ok)
	}
	if _, ok := canonicalWorkflowEventType("UserTaskStart"); ok {
		t.Error("a misspelt type must not be accepted")
	}
	got := sortWorkflowEventTypes([]string{"UserTaskEnded", "WorkflowCompleted", "UserTaskEnded"})
	if want := []string{"WorkflowCompleted", "UserTaskEnded"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sorted = %v, want %v", got, want)
	}
}
