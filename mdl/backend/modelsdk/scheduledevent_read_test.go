// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	genSched "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/scheduledevents"
)

// TestScheduledEventFromGen guards the gen→semantic mapping the modelsdk read
// relies on. (No committed fixture contains scheduled events, and modelsdk has no
// scheduled-event write path, so the converter is exercised directly; the
// List/Get plumbing reuses the ListUnitsWithContainer pattern covered by the
// page/java reads.) The key mappings: MicroflowID carries the by-name microflow
// reference (BSON "Microflow"), Interval narrows the int32 accessor to int.
func TestScheduledEventFromGen(t *testing.T) {
	g := genSched.NewScheduledEvent()
	g.SetID("evt-1")
	g.SetName("SE_Cleanup")
	g.SetDocumentation("nightly cleanup")
	g.SetMicroflowQualifiedName("MyModule.DoCleanup")
	g.SetInterval(86400)
	g.SetIntervalType("Day")
	g.SetEnabled(true)

	ev := scheduledEventFromGen(g, model.ID("mod-1"))

	if ev.ID != "evt-1" {
		t.Errorf("ID = %q, want evt-1", ev.ID)
	}
	if ev.ContainerID != "mod-1" {
		t.Errorf("ContainerID = %q, want mod-1", ev.ContainerID)
	}
	if ev.TypeName != "ScheduledEvents$ScheduledEvent" {
		t.Errorf("TypeName = %q", ev.TypeName)
	}
	if ev.Name != "SE_Cleanup" {
		t.Errorf("Name = %q, want SE_Cleanup", ev.Name)
	}
	if ev.Documentation != "nightly cleanup" {
		t.Errorf("Documentation = %q", ev.Documentation)
	}
	if ev.MicroflowID != "MyModule.DoCleanup" {
		t.Errorf("MicroflowID = %q, want MyModule.DoCleanup", ev.MicroflowID)
	}
	if ev.Interval != 86400 {
		t.Errorf("Interval = %d, want 86400", ev.Interval)
	}
	if ev.IntervalType != "Day" {
		t.Errorf("IntervalType = %q, want Day", ev.IntervalType)
	}
	if !ev.Enabled {
		t.Error("Enabled = false, want true")
	}
}

// TestListScheduledEvents_Empty confirms the read returns an empty (not error)
// result on a project with no scheduled events — the minimal fixture — so SHOW
// STRUCTURE no longer swallows a not-implemented error.
func TestListScheduledEvents_Empty(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	events, err := b.ListScheduledEvents()
	if err != nil {
		t.Fatalf("ListScheduledEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d scheduled events, want 0 (minimal fixture has none)", len(events))
	}
}
