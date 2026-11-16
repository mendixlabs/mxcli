// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/spf13/cobra"
)

// The traceability report reads milestones from the plan. The old
// traceability.json is read only when the plan names none, and either way the
// report says how to move it into the plan.
func TestTraceabilityMilestonesComeFromThePlan(t *testing.T) {
	dir := t.TempDir()
	store := brain.NewStore(dir)
	if _, err := store.Init(); err != nil {
		t.Fatal(err)
	}
	r, err := brain.NewRequirement("Users can log in", []string{"@Accounts.ACT_Login"}, "01-accounts", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Promote(r, r.Shard()); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.Flags().String("milestones", "", "")
	read := func() (string, []string) {
		t.Helper()
		ms, warnings, err := traceabilityMilestones(cmd, store, dir)
		if err != nil {
			t.Fatal(err)
		}
		if ms == nil {
			return "", warnings
		}
		return ms.Milestones[0].Name, warnings
	}
	if name, w := read(); name != "" || len(w) > 0 {
		t.Fatalf("no milestones anywhere: got %q %v", name, w)
	}
	old := filepath.Join(dir, "docs", "brain", "plan", "traceability.json")
	if err := os.WriteFile(old, []byte(`{"milestones":[{"name":"From file","slices":["01-accounts"]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if name, w := read(); name != "From file" || len(w) != 1 || !strings.Contains(w[0], "brain milestone --import") {
		t.Errorf("old file only: got %q %v", name, w)
	}
	if err := store.SetSliceMilestone("01-accounts", "From plan"); err != nil {
		t.Fatal(err)
	}
	if name, w := read(); name != "From plan" || len(w) != 1 || !strings.Contains(w[0], "Ignored") {
		t.Errorf("plan and old file: the plan wins and the file is called out: got %q %v", name, w)
	}
	_ = cmd.Flags().Set("milestones", old)
	if name, _ := read(); name != "From file" {
		t.Errorf("an explicit --milestones file still wins: got %q", name)
	}
}
