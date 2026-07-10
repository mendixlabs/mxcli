// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
)

// minimalReader is a test double for LintReader that returns empty/nil for
// everything except ListScheduledEvents, which is configurable via a func field.
type minimalReader struct {
	listScheduledEvents func() ([]*model.ScheduledEvent, error)
}

func (m *minimalReader) GetMicroflow(_ model.ID) (*microflows.Microflow, error) {
	return nil, nil
}
func (m *minimalReader) ListMicroflows() ([]*microflows.Microflow, error)       { return nil, nil }
func (m *minimalReader) GetProjectSecurity() (*security.ProjectSecurity, error) { return nil, nil }
func (m *minimalReader) GetNavigation() (*types.NavigationDocument, error)      { return nil, nil }
func (m *minimalReader) ListPages() ([]*pages.Page, error)                      { return nil, nil }
func (m *minimalReader) ListModules() ([]*model.Module, error)                  { return nil, nil }
func (m *minimalReader) ListFolders() ([]*types.FolderInfo, error)              { return nil, nil }
func (m *minimalReader) GetRawUnit(_ model.ID) (map[string]any, error)          { return nil, nil }
func (m *minimalReader) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	if m.listScheduledEvents != nil {
		return m.listScheduledEvents()
	}
	return nil, nil
}

// TestIntervalToSeconds is a white-box test; we call it via the exported
// ScheduledEvents iterator rather than calling the unexported helper directly.
// The expected IntervalSeconds values verify all multipliers and the unknown-type fallback.
func TestIntervalToSeconds(t *testing.T) {
	tests := []struct {
		interval     int
		intervalType string
		want         int
	}{
		{1, "Second", 1},
		{2, "Minute", 120},
		{3, "Hour", 10800},
		{1, "Day", 86400},
		{1, "Week", 604800},
		{1, "Month", 2592000},
		{1, "Year", 31536000},
		{5, "Unknown", 0}, // unrecognised type → 0
		{5, "", 0},        // empty type → 0
	}

	containerID := model.ID("mod-1")
	for _, tt := range tests {
		reader := &minimalReader{
			listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
				return []*model.ScheduledEvent{{
					ContainerID:  containerID,
					Name:         "SE",
					Interval:     tt.interval,
					IntervalType: tt.intervalType,
					Enabled:      true,
				}}, nil
			},
		}

		cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
		if err != nil {
			t.Fatalf("NewFromFile: %v", err)
		}
		db := cat.CatalogDB()
		if _, err := db.Exec(
			`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
			string(containerID), "MyModule", "default", "s1",
		); err != nil {
			t.Fatalf("insert module: %v", err)
		}
		cat.Close()

		ctx := linter.NewLintContext(cat, reader)
		var got int
		for se := range ctx.ScheduledEvents() {
			got = se.IntervalSeconds
		}
		if got != tt.want {
			t.Errorf("interval=%d type=%q: IntervalSeconds=%d, want %d", tt.interval, tt.intervalType, got, tt.want)
		}
	}
}

func TestScheduledEvents_MicroflowNameResolution(t *testing.T) {
	containerID := model.ID("mod-uuid")
	mfID := model.ID("mf-uuid-1234")

	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{
				{
					ContainerID:  containerID,
					Name:         "SEWithCatalog",
					MicroflowID:  mfID,
					Interval:     1,
					IntervalType: "Hour",
					Enabled:      true,
				},
				{
					ContainerID:  containerID,
					Name:         "SEWithoutCatalog",
					MicroflowID:  model.ID("unknown-uuid"),
					Interval:     1,
					IntervalType: "Hour",
					Enabled:      false,
				},
			}, nil
		},
	}

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		string(containerID), "MyModule", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?)`,
		string(mfID), "ACT_DoSomething", "MyModule.ACT_DoSomething", "MyModule", "Microflow", "default", "s1",
	); err != nil {
		t.Fatalf("insert microflow: %v", err)
	}

	ctx := linter.NewLintContext(cat, reader)
	events := make(map[string]linter.ScheduledEvent)
	for se := range ctx.ScheduledEvents() {
		events[se.Name] = se
	}

	// When the microflow ID is in the catalog, MicroflowName must be the qualified name.
	if got := events["SEWithCatalog"].MicroflowName; got != "MyModule.ACT_DoSomething" {
		t.Errorf("SEWithCatalog.MicroflowName = %q, want %q", got, "MyModule.ACT_DoSomething")
	}

	// When the microflow ID is not in the catalog, fall back to the raw UUID.
	if got := events["SEWithoutCatalog"].MicroflowName; got != "unknown-uuid" {
		t.Errorf("SEWithoutCatalog.MicroflowName = %q, want raw UUID %q", got, "unknown-uuid")
	}
}

func TestScheduledEvents_ExcludedModules(t *testing.T) {
	containerID := model.ID("mod-excl")

	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{{
				ContainerID:  containerID,
				Name:         "ExcludedSE",
				Interval:     1,
				IntervalType: "Day",
				Enabled:      true,
			}}, nil
		},
	}

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		string(containerID), "SystemModule", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}

	ctx := linter.NewLintContext(cat, reader)
	ctx.SetExcludedModules([]string{"SystemModule"})

	var count int
	for range ctx.ScheduledEvents() {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 events after exclusion, got %d", count)
	}
}

func TestScheduledEvents_IncludedModules(t *testing.T) {
	modA := model.ID("mod-a")
	modB := model.ID("mod-b")

	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{
				{ContainerID: modA, Name: "SE_A", Interval: 1, IntervalType: "Hour", Enabled: true},
				{ContainerID: modB, Name: "SE_B", Interval: 1, IntervalType: "Hour", Enabled: true},
			}, nil
		},
	}

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	for _, row := range []struct{ id, name string }{{string(modA), "ModA"}, {string(modB), "ModB"}} {
		if _, err := db.Exec(
			`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
			row.id, row.name, "default", "s1",
		); err != nil {
			t.Fatalf("insert module %s: %v", row.name, err)
		}
	}

	ctx := linter.NewLintContext(cat, reader)
	ctx.SetIncludedModules([]string{"ModA"}) // only ModA is in scope

	var names []string
	for se := range ctx.ScheduledEvents() {
		names = append(names, se.ModuleName)
	}
	if len(names) != 1 || names[0] != "ModA" {
		t.Errorf("expected [ModA], got %v", names)
	}
}

func TestScheduledEvents_NilReader(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()

	ctx := linter.NewLintContext(cat, nil)
	var count int
	for range ctx.ScheduledEvents() {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 events with nil reader, got %d", count)
	}
}

// TestStarlarkScheduledEventsBuiltin exercises the scheduled_events() Starlark builtin.
const scheduledEventsRule = `
RULE_ID = "TEST_SE001"
RULE_NAME = "scheduled events builtin"
DESCRIPTION = "exercises the scheduled_events() builtin"
CATEGORY = "test"
SEVERITY = "info"

def check():
    out = []
    for se in scheduled_events():
        out.append(violation(message = "se %s mf %s secs %d enabled %s" % (
            se.qualified_name,
            se.microflow_name,
            se.interval_seconds,
            "yes" if se.enabled else "no",
        )))
    return out
`

func TestStarlarkScheduledEventsBuiltin(t *testing.T) {
	containerID := model.ID("mod-starlark")
	mfID := model.ID("mf-starlark-uuid")

	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{{
				ContainerID:  containerID,
				Name:         "DailySE",
				MicroflowID:  mfID,
				Interval:     2,
				IntervalType: "Day",
				Enabled:      true,
			}}, nil
		},
	}

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		string(containerID), "Billing", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?)`,
		string(mfID), "SUB_DailyJob", "Billing.SUB_DailyJob", "Billing", "Microflow", "default", "s1",
	); err != nil {
		t.Fatalf("insert microflow: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "se_test.star")
	if err := os.WriteFile(path, []byte(scheduledEventsRule), 0644); err != nil {
		t.Fatal(err)
	}

	rule, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}

	ctx := linter.NewLintContext(cat, reader)
	violations := rule.Check(ctx)

	var msgs []string
	for _, v := range violations {
		msgs = append(msgs, v.Message)
	}
	joined := strings.Join(msgs, "\n")

	for _, want := range []string{
		"se Billing.DailySE",
		"mf Billing.SUB_DailyJob",
		"secs 172800", // 2 * 86400
		"enabled yes",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in violations:\n%s", want, joined)
		}
	}
}
