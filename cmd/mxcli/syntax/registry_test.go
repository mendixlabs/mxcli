// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestAll_ReturnsSortedFeatures(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("expected registered features, got none")
	}
	for i := 1; i < len(all); i++ {
		if all[i].Path < all[i-1].Path {
			t.Errorf("features not sorted: %q before %q", all[i-1].Path, all[i].Path)
		}
	}
}

func TestByPrefix_ReturnsMatchingFeatures(t *testing.T) {
	tests := []struct {
		prefix    string
		wantMin   int
		wantPaths []string // paths that must be present
	}{
		{"workflow", 5, []string{"workflow", "workflow.user-task", "workflow.user-task.targeting"}},
		{"workflow.user-task", 2, []string{"workflow.user-task", "workflow.user-task.targeting"}},
		{"security", 5, []string{"security", "security.module-role", "security.entity-access"}},
		{"nonexistent", 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			got := ByPrefix(tt.prefix)
			if len(got) < tt.wantMin {
				t.Errorf("ByPrefix(%q): got %d features, want >= %d", tt.prefix, len(got), tt.wantMin)
			}
			for _, wantPath := range tt.wantPaths {
				found := false
				for _, f := range got {
					if f.Path == wantPath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ByPrefix(%q): missing expected path %q", tt.prefix, wantPath)
				}
			}
		})
	}
}

func TestByPath_ExactMatch(t *testing.T) {
	f := ByPath("workflow.user-task.targeting")
	if f == nil {
		t.Fatal("expected feature for workflow.user-task.targeting, got nil")
	}
	if f.Summary == "" {
		t.Error("feature summary is empty")
	}
	if f.MinVersion == "" {
		t.Error("expected MinVersion to be set for targeting")
	}
}

func TestByPath_NotFound(t *testing.T) {
	f := ByPath("does.not.exist")
	if f != nil {
		t.Errorf("expected nil for nonexistent path, got %v", f)
	}
}

func TestHasPrefix(t *testing.T) {
	if !HasPrefix("workflow") {
		t.Error("expected HasPrefix(workflow) = true")
	}
	if !HasPrefix("security") {
		t.Error("expected HasPrefix(security) = true")
	}
	if HasPrefix("nonexistent") {
		t.Error("expected HasPrefix(nonexistent) = false")
	}
}

func TestFeatureFieldsPopulated(t *testing.T) {
	for _, f := range All() {
		t.Run(f.Path, func(t *testing.T) {
			if f.Path == "" {
				t.Error("empty path")
			}
			if f.Summary == "" {
				t.Error("empty summary")
			}
			if len(f.Keywords) == 0 {
				t.Error("no keywords")
			}
			if f.Syntax == "" {
				t.Error("empty syntax")
			}
			if f.Example == "" {
				t.Error("empty example")
			}
		})
	}
}

func TestWriteJSON_ValidOutput(t *testing.T) {
	features := ByPrefix("workflow.user-task")
	var buf bytes.Buffer
	if err := WriteJSON(&buf, features); err != nil {
		t.Fatal(err)
	}
	var parsed []SyntaxFeature
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != len(features) {
		t.Errorf("got %d features in JSON, want %d", len(parsed), len(features))
	}
}

func TestWriteText_SingleFeature(t *testing.T) {
	f := ByPath("workflow.user-task.targeting")
	if f == nil {
		t.Fatal("feature not found")
	}
	var buf bytes.Buffer
	WriteText(&buf, []SyntaxFeature{*f})
	out := buf.String()
	if !strings.Contains(out, "workflow.user-task.targeting") {
		t.Error("output missing feature path")
	}
	if !strings.Contains(out, "Syntax:") {
		t.Error("output missing Syntax section")
	}
	if !strings.Contains(out, "Example:") {
		t.Error("output missing Example section")
	}
	// From the version registry (#1367): a user task is 9.19.0, group
	// targeting 11.2.0. The hand-written 9.0.0 this used to assert was wrong.
	if !strings.Contains(out, "Mendix 9.19.0+") || !strings.Contains(out, "Mendix 11.2.0+") {
		t.Errorf("output missing the registry versions:\n%s", out)
	}
}

func TestWriteText_MultipleFeatures(t *testing.T) {
	features := ByPrefix("security")
	var buf bytes.Buffer
	WriteText(&buf, features)
	out := buf.String()
	if !strings.Contains(out, "Security") {
		t.Error("output missing group header")
	}
	if !strings.Contains(out, "security.entity-access") {
		t.Error("output missing security.entity-access")
	}
}

func TestResolveAlias(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"entity", "domain-model.entity"},
		{"entities", "domain-model.entity"},
		{"enum", "domain-model.enumeration"},
		{"association", "domain-model.association"},
		{"nav", "navigation"},
		{"be", "business-events"},
		{"validation", "errors"},
		{"testing", "test"},
		{"tests", "test"},
		// Non-alias passes through unchanged
		{"workflow", "workflow"},
		{"security", "security"},
		{"nonexistent", "nonexistent"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveAlias(tt.input)
			if got != tt.want {
				t.Errorf("ResolveAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAliasTargetsExist(t *testing.T) {
	all := All()
	pathSet := make(map[string]bool, len(all))
	for _, f := range all {
		pathSet[f.Path] = true
	}
	for alias, target := range topicAliases {
		if !HasPrefix(target) {
			t.Errorf("alias %q -> %q: target path has no matching features", alias, target)
		}
	}
}

func TestSeeAlsoRefsExist(t *testing.T) {
	all := All()
	pathSet := make(map[string]bool, len(all))
	for _, f := range all {
		pathSet[f.Path] = true
	}
	for _, f := range all {
		for _, ref := range f.SeeAlso {
			if !pathSet[ref] {
				t.Errorf("%s: see_also references %q which is not registered", f.Path, ref)
			}
		}
	}
}

// #955: `mxcli syntax rule` reported "Unknown topic: rule" while both
// microflow.rule and validation-rule were registered. Registry lookup is
// anchored at the left, so a topic is reachable only by its first segment or
// via a hand-written alias — which left 60 of the registry's leaf names with no
// spelling that finds them.
func TestBySegmentMatch_FindsNonLeadingSegments(t *testing.T) {
	tests := []struct {
		query     string
		wantPaths []string
	}{
		// The reported case, and the reason an alias would have been the worse
		// fix: "rule" names two unrelated topics and both are relevant.
		{"rule", []string{"microflow.rule", "validation-rule"}},
		{"user-task", []string{"workflow.user-task", "workflow.user-task.targeting"}},
		// A segment is often a compound of the word the reader has in mind, so
		// the match is on substring-within-segment rather than equality.
		{"task", []string{"workflow.user-task"}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := BySegmentMatch(tt.query)
			for _, want := range tt.wantPaths {
				found := false
				for _, f := range got {
					if f.Path == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("BySegmentMatch(%q) did not return %q", tt.query, want)
				}
			}
		})
	}
}

// The fallback only runs when the anchored lookup finds nothing, but it must
// also not invent matches: a query naming no segment stays unknown, which is
// what keeps "Unknown topic" meaningful.
func TestBySegmentMatch_UnknownStaysUnknown(t *testing.T) {
	if got := BySegmentMatch("zzzznope"); len(got) != 0 {
		t.Errorf("expected no matches, got %d", len(got))
	}
	if got := BySegmentMatch(""); len(got) != 0 {
		t.Errorf("empty query must not match everything, got %d", len(got))
	}
}

// Every leaf segment in the registry must be reachable by some spelling. This
// is the property #955 reported the absence of; asserting it over the whole
// registry means a newly added nested topic cannot silently become
// unreachable the way `rule` did.
func TestEverySegmentIsReachable(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range All() {
		for _, seg := range strings.Split(f.Path, ".") {
			seen[seg] = true
		}
	}
	for seg := range seen {
		if HasPrefix(seg) || ResolveAlias(seg) != seg {
			continue // reachable as a top-level path, or via an alias
		}
		if len(BySegmentMatch(seg)) == 0 {
			t.Errorf("topic segment %q is not reachable by any spelling", seg)
		}
	}
}
