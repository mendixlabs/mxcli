// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// The System module is not stored in the project — its enumerations exist only
// in modelsdk/meta — so a reader that decodes stored units alone reports them as
// absent. That is what made `describe enumeration System.WorkflowActivityType`
// fail, `show enumerations` omit them, and `check --references` reject a valid
// attribute typed against one (mendixlabs/mxcli#1102). Same shape as the System
// Java actions synthesized in java.go.

// valuesOf returns an enumeration's value names.
func valuesOf(e *model.Enumeration) []string {
	out := make([]string, 0, len(e.Values))
	for _, v := range e.Values {
		out = append(out, v.Name)
	}
	return out
}

func hasValue(e *model.Enumeration, name string) bool {
	for _, v := range e.Values {
		if v.Name == name {
			return true
		}
	}
	return false
}

// findSystemEnum returns the synthesized System enumeration of that local name.
func findSystemEnum(enums []*model.Enumeration, name string) *model.Enumeration {
	for _, e := range enums {
		if e.Name == name && string(e.ContainerID) == meta.SystemModuleID {
			return e
		}
	}
	return nil
}

func TestListEnumerations_IncludesSystemModule(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	enums, err := b.ListEnumerations()
	if err != nil {
		t.Fatalf("ListEnumerations: %v", err)
	}

	// The control: the STORED enumerations must still come back. Without it this
	// passes against a build that returned the synthesized ones only.
	stored := 0
	for _, e := range enums {
		if string(e.ContainerID) != meta.SystemModuleID {
			stored++
		}
	}
	if stored != 7 {
		t.Errorf("stored enumerations = %d, want 7 (the fixture's own)", stored)
	}

	// The fixture is Mendix 11.6.6, so only the enumerations its System module
	// has: System.WorkflowSubProcessState arrived in 11.8 and must not be listed.
	if got, want := len(enums)-stored, len(meta.SystemEnumerationsIn(b.systemVersionAtLeast())); got != want {
		t.Errorf("synthesized System enumerations = %d, want %d", got, want)
	}
	if findSystemEnum(enums, "WorkflowSubProcessState") != nil {
		t.Error("System.WorkflowSubProcessState listed for an 11.6.6 project; it was introduced in 11.8")
	}
	if len(meta.SystemEnumerationsIn(b.systemVersionAtLeast())) >= len(meta.SystemEnumerations) {
		t.Error("control: the version filter dropped nothing for 11.6.6")
	}

	activityType := findSystemEnum(enums, "WorkflowActivityType")
	if activityType == nil {
		t.Fatal("System.WorkflowActivityType not returned by ListEnumerations")
	}
	if !hasValue(activityType, "UserTask") {
		t.Errorf("System.WorkflowActivityType values = %v, want UserTask among them", valuesOf(activityType))
	}

	// The reporter's CE1613 was `System.WorkflowActivityExecutionState.Finished`.
	// Finished is real, but on System.WorkflowActivityState — this one has
	// Completed. Pinning both directions is what makes the listing answer the
	// question that was actually being asked.
	execState := findSystemEnum(enums, "WorkflowActivityExecutionState")
	if execState == nil {
		t.Fatal("System.WorkflowActivityExecutionState not returned by ListEnumerations")
	}
	if hasValue(execState, "Finished") {
		t.Error("System.WorkflowActivityExecutionState must not carry Finished (that is WorkflowActivityState)")
	}
	if !hasValue(execState, "Completed") {
		t.Errorf("System.WorkflowActivityExecutionState values = %v, want Completed", valuesOf(execState))
	}
	if activityState := findSystemEnum(enums, "WorkflowActivityState"); activityState == nil {
		t.Error("System.WorkflowActivityState not returned by ListEnumerations")
	} else if !hasValue(activityState, "Finished") {
		t.Errorf("System.WorkflowActivityState values = %v, want Finished", valuesOf(activityState))
	}
}

// TestGetEnumeration_FindsSystemModule keeps the by-ID lookup consistent with
// the listing: a caller holding an ID from ListEnumerations must be able to
// resolve it again.
func TestGetEnumeration_FindsSystemModule(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	enums, err := b.ListEnumerations()
	if err != nil {
		t.Fatalf("ListEnumerations: %v", err)
	}
	want := findSystemEnum(enums, "WorkflowActivityType")
	if want == nil {
		t.Fatal("System.WorkflowActivityType not in ListEnumerations")
	}

	got, err := b.GetEnumeration(want.ID)
	if err != nil {
		t.Fatalf("GetEnumeration(%q): %v", want.ID, err)
	}
	if got == nil {
		t.Fatalf("GetEnumeration(%q) = nil — the listing offers an ID the getter cannot resolve", want.ID)
	}
	if got.Name != "WorkflowActivityType" {
		t.Errorf("GetEnumeration returned %q, want WorkflowActivityType", got.Name)
	}

	// Control: a stored enumeration still resolves by ID.
	for _, e := range enums {
		if string(e.ContainerID) == meta.SystemModuleID {
			continue
		}
		stored, err := b.GetEnumeration(e.ID)
		if err != nil || stored == nil {
			t.Fatalf("GetEnumeration(%q) for stored %s: %v / nil=%v", e.ID, e.Name, err, stored == nil)
		}
		break
	}
}
