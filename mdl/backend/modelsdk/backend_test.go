// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
)

const fixture = "../../../testdata/expr-checker/minimal.mpr"

// TestReadSlice_Modules exercises the Phase-1 read path end to end against the
// vendored fixture: connect through the modelsdk engine, enumerate modules, and
// confirm the connection metadata is populated.
func TestReadSlice_Modules(t *testing.T) {
	b := New()
	// Interface conformance is also checked at compile time (var _ ... in backend.go).
	var _ backend.FullBackend = b

	if b.IsConnected() {
		t.Fatal("new backend should not report connected")
	}
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	if !b.IsConnected() {
		t.Fatal("expected IsConnected() true after Connect")
	}
	if b.Path() != fixture {
		t.Errorf("Path() = %q, want %q", b.Path(), fixture)
	}

	mods, err := b.ListModules()
	if err != nil {
		t.Fatalf("ListModules: %v", err)
	}
	if len(mods) == 0 {
		t.Fatal("ListModules returned no modules")
	}

	byName := map[string]bool{}
	for _, m := range mods {
		if m.Name == "" {
			t.Errorf("module with empty name: %+v", m)
		}
		if m.ID == "" {
			t.Errorf("module %q has empty ID", m.Name)
		}
		byName[m.Name] = true
	}
	// System is always present in a Mendix project.
	if !byName["System"] {
		t.Errorf("expected a System module; got %d modules", len(mods))
	}

	// Marketplace metadata (the SHOW MODULES "Source" column) must be enriched
	// from the decoded module unit, not just ID+Name.
	for _, m := range mods {
		if m.Name == "Administration" {
			if !m.FromAppStore || m.AppStoreVersion != "4.3.2" {
				t.Errorf("Administration FromAppStore=%v version=%q, want true/4.3.2", m.FromAppStore, m.AppStoreVersion)
			}
		}
	}

	// GetModuleByName round-trips against the list.
	sys, err := b.GetModuleByName("System")
	if err != nil {
		t.Fatalf("GetModuleByName(System): %v", err)
	}
	if sys == nil || sys.Name != "System" {
		t.Fatalf("GetModuleByName(System) = %+v, want a module named System", sys)
	}
}
