// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr/version"
)

func TestParseVersionDirective(t *testing.T) {
	tests := []struct {
		line    string
		wantNil bool
		min     [2]int // {major, minor}, ignored if wantNil
		max     [2]int // {major, minor}, -1 means no limit
	}{
		{"-- @version: any", true, [2]int{}, [2]int{}},
		{"-- @version: 11.0+", false, [2]int{11, 0}, [2]int{-1, -1}},
		{"-- @version: 10.18+", false, [2]int{10, 18}, [2]int{-1, -1}},
		{"-- @version: 10.6..10.24", false, [2]int{10, 6}, [2]int{10, 24}},
		{"-- @version: ..10.24", false, [2]int{-1, -1}, [2]int{10, 24}},
		{"-- @version:   11.0+  ", false, [2]int{11, 0}, [2]int{-1, -1}}, // extra whitespace
	}

	for _, tt := range tests {
		vc := parseVersionDirective(tt.line)
		if tt.wantNil {
			if vc != nil {
				t.Errorf("parseVersionDirective(%q) = %v, want nil", tt.line, vc)
			}
			continue
		}
		if vc == nil {
			t.Errorf("parseVersionDirective(%q) = nil, want non-nil", tt.line)
			continue
		}
		if vc.minMajor != tt.min[0] || vc.minMinor != tt.min[1] {
			t.Errorf("parseVersionDirective(%q) min = %d.%d, want %d.%d", tt.line, vc.minMajor, vc.minMinor, tt.min[0], tt.min[1])
		}
		if vc.maxMajor != tt.max[0] || vc.maxMinor != tt.max[1] {
			t.Errorf("parseVersionDirective(%q) max = %d.%d, want %d.%d", tt.line, vc.maxMajor, vc.maxMinor, tt.max[0], tt.max[1])
		}
	}
}

func TestVersionConstraintMatches(t *testing.T) {
	mx1024 := &version.ProjectVersion{MajorVersion: 10, MinorVersion: 24}
	mx110 := &version.ProjectVersion{MajorVersion: 11, MinorVersion: 0}
	mx116 := &version.ProjectVersion{MajorVersion: 11, MinorVersion: 6}

	tests := []struct {
		constraint string
		pv         *version.ProjectVersion
		want       bool
	}{
		// min only: 11.0+
		{"-- @version: 11.0+", mx1024, false},
		{"-- @version: 11.0+", mx110, true},
		{"-- @version: 11.0+", mx116, true},
		// min only: 10.18+
		{"-- @version: 10.18+", mx1024, true},
		{"-- @version: 10.18+", mx110, true},
		// range: 10.6..10.24
		{"-- @version: 10.6..10.24", mx1024, true},
		{"-- @version: 10.6..10.24", mx110, false},
		{"-- @version: 10.6..10.24", mx116, false},
		// max only: ..10.24
		{"-- @version: ..10.24", mx1024, true},
		{"-- @version: ..10.24", mx110, false},
	}

	for _, tt := range tests {
		vc := parseVersionDirective(tt.constraint)
		if vc == nil {
			t.Fatalf("parseVersionDirective(%q) returned nil", tt.constraint)
		}
		got := vc.matches(tt.pv)
		if got != tt.want {
			t.Errorf("constraint %q matches(%d.%d) = %v, want %v",
				tt.constraint, tt.pv.MajorVersion, tt.pv.MinorVersion, got, tt.want)
		}
	}
}

func TestFilterByVersion(t *testing.T) {
	content := `-- setup
create module Test;
-- @version: 11.0+
create view entity Test.MyView (...);
-- @version: any
create entity Test.Universal (...);
`

	mx1024 := &version.ProjectVersion{MajorVersion: 10, MinorVersion: 24, ProductVersion: "10.24.0"}
	mx116 := &version.ProjectVersion{MajorVersion: 11, MinorVersion: 6, ProductVersion: "11.6.0"}

	// On 10.24: VIEW ENTITY line should be stripped
	filtered1024, skipped1024 := filterByVersion(content, mx1024)
	if skipped1024 == 0 {
		t.Error("Expected skipped lines on 10.24")
	}
	if containsNonComment(filtered1024, "create view entity") {
		t.Error("view entity should be stripped on 10.24")
	}
	if !containsNonComment(filtered1024, "create module") {
		t.Error("create module should be kept on 10.24")
	}
	if !containsNonComment(filtered1024, "create entity") {
		t.Error("create entity should be kept on 10.24 (after @version: any)")
	}

	// On 11.6: everything should be kept
	filtered116, skipped116 := filterByVersion(content, mx116)
	if skipped116 != 0 {
		t.Errorf("Expected 0 skipped lines on 11.6, got %d", skipped116)
	}
	if !containsNonComment(filtered116, "create view entity") {
		t.Error("view entity should be kept on 11.6")
	}
}

func containsNonComment(s, substr string) bool {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, substr) && !strings.HasPrefix(trimmed, "--") {
			return true
		}
	}
	return false
}
