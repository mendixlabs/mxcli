// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

// The System module's Java actions are platform built-ins with no stored unit,
// so a reader that only decodes units reports them as absent. The legacy
// sdk/mpr reader synthesized them and this backend did not, which is a
// difference nothing measured until `project-tree` moved onto the backend and
// silently lost System.VerifyPassword from its output (Phase 4a).
//
// Caught by diffing the command against a pre-port binary, not by a test —
// which is the argument for keeping that baseline around during a port.

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

func TestListJavaActions_IncludesSystemBuiltIns(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	got, err := b.ListJavaActions()
	if err != nil {
		t.Fatalf("ListJavaActions: %v", err)
	}
	names := map[string]bool{}
	for _, ja := range got {
		names[ja.Name] = true
	}
	for _, def := range meta.SystemJavaActions {
		if !names[def.Name] {
			t.Errorf("System.%s missing from ListJavaActions — a platform built-in "+
				"has no stored unit, so it must be synthesized or it vanishes", def.Name)
		}
	}
}

func TestListJavaActionsFull_IncludesSystemBuiltIns(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	got, err := b.ListJavaActionsFull()
	if err != nil {
		t.Fatalf("ListJavaActionsFull: %v", err)
	}
	for _, def := range meta.SystemJavaActions {
		var found bool
		for _, ja := range got {
			if ja.Name != def.Name {
				continue
			}
			found = true
			// Not just the name: the full variant is what the catalog stores, so
			// an entry with no parameters or return type would satisfy a
			// name-only check while being useless to every consumer.
			if len(ja.Parameters) != len(def.Parameters) {
				t.Errorf("System.%s has %d parameters, want %d",
					def.Name, len(ja.Parameters), len(def.Parameters))
			}
			if def.ReturnType != "Void" && ja.ReturnType == nil {
				t.Errorf("System.%s has no return type, want %s", def.Name, def.ReturnType)
			}
		}
		if !found {
			t.Errorf("System.%s missing from ListJavaActionsFull", def.Name)
		}
	}
}

// The control. Both tests above pass just as well against a backend that
// returns ONLY the synthesized System actions and drops every stored one —
// which would be a far worse regression than the one they were written for.
func TestListJavaActions_StillReturnsStoredActions(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	got, err := b.ListJavaActions()
	if err != nil {
		t.Fatalf("ListJavaActions: %v", err)
	}
	var stored int
	for _, ja := range got {
		if string(ja.ContainerID) != meta.SystemModuleID {
			stored++
		}
	}
	if stored == 0 {
		t.Fatalf("every returned Java action is a System built-in (%d total) — "+
			"the stored ones were dropped", len(got))
	}
}
