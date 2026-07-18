// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddSessionStartHook_Empty(t *testing.T) {
	s := map[string]any{}
	if !addSessionStartHook(s, "cmd A") {
		t.Fatal("expected a change on empty settings")
	}
	hooks := s["hooks"].(map[string]any)
	ss := hooks["SessionStart"].([]any)
	if len(ss) != 1 {
		t.Fatalf("SessionStart len = %d, want 1", len(ss))
	}
}

func TestAddSessionStartHook_Idempotent(t *testing.T) {
	s := map[string]any{}
	addSessionStartHook(s, "x run --local --setup y")
	// A second add with the marker present must be a no-op.
	if addSessionStartHook(s, "run --local --setup --ensure-db -p App.mpr") {
		t.Error("expected no change when the marker is already present")
	}
	ss := s["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(ss) != 1 {
		t.Errorf("SessionStart len = %d, want 1 (no duplicate)", len(ss))
	}
}

func TestAddSessionStartHook_PreservesExisting(t *testing.T) {
	// Existing unrelated settings + a different SessionStart hook must survive.
	s := map[string]any{
		"model": "opus",
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo hi"}}},
			},
			"PostToolUse": []any{map[string]any{"hooks": []any{}}},
		},
	}
	if !addSessionStartHook(s, "run --local --setup -p App.mpr") {
		t.Fatal("expected a change")
	}
	if s["model"] != "opus" {
		t.Error("unrelated top-level setting was dropped")
	}
	hooks := s["hooks"].(map[string]any)
	if _, ok := hooks["PostToolUse"]; !ok {
		t.Error("unrelated hook group was dropped")
	}
	ss := hooks["SessionStart"].([]any)
	if len(ss) != 2 {
		t.Errorf("SessionStart len = %d, want 2 (existing + new)", len(ss))
	}
}

func TestEnsureSessionStartHook_WritesFile(t *testing.T) {
	dir := t.TempDir()
	changed, err := ensureSessionStartHook(dir, "App.mpr")
	if err != nil || !changed {
		t.Fatalf("ensureSessionStartHook = (%v,%v), want (true,nil)", changed, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "run --local --setup --ensure-db -p App.mpr") {
		t.Errorf("settings.json missing the hook command:\n%s", data)
	}
	// Valid JSON round-trips.
	var check map[string]any
	if err := json.Unmarshal(data, &check); err != nil {
		t.Errorf("settings.json is not valid JSON: %v", err)
	}
	// Second call is idempotent (no change).
	changed, err = ensureSessionStartHook(dir, "App.mpr")
	if err != nil || changed {
		t.Errorf("second call = (%v,%v), want (false,nil)", changed, err)
	}
}

func TestEnsureSessionStartHook_InvalidJSONUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	_ = os.WriteFile(path, []byte("{ not json"), 0o644)
	changed, err := ensureSessionStartHook(dir, "App.mpr")
	if err == nil {
		t.Error("expected an error for invalid existing settings.json")
	}
	if changed {
		t.Error("must not report a change when leaving invalid JSON untouched")
	}
	// The original content is preserved.
	data, _ := os.ReadFile(path)
	if string(data) != "{ not json" {
		t.Errorf("invalid settings.json was modified: %q", data)
	}
}
