// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// TestListJavaScriptActions reads every JS action from the vendored fixture and
// confirms the codec-native list is populated (the modelsdk engine previously
// errored "ListJavaScriptActions not implemented yet" — Issue 7 follow-up).
func TestListJavaScriptActions(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	jsas, err := b.ListJavaScriptActions()
	if err != nil {
		t.Fatalf("ListJavaScriptActions: %v", err)
	}
	if len(jsas) == 0 {
		t.Fatal("ListJavaScriptActions returned no actions; fixture has bundled JS actions")
	}
	// Every action must carry at least a name and container (the half-shell the
	// gen-key mismatch would have produced still has these; the deeper fields are
	// asserted by the DESCRIBE round-trip below).
	for _, jsa := range jsas {
		if jsa.Name == "" {
			t.Errorf("JS action with empty name (id=%s)", jsa.ID)
		}
		if jsa.ContainerID == "" {
			t.Errorf("JS action %q has empty container", jsa.Name)
		}
	}
}

// TestReadJavaScriptActionByName guards the four children the gen codec decodes
// under the wrong storage names (JavaReturnType / Parameters / TypeParameters /
// MicroflowActionInfo). A gen-accessor converter would return them empty — the
// Issue 7 "half-shell". NanoflowCommons.Geocode exercises all of them, plus the
// enum-in-BasicParameterType recovery that legacy drops to bare "Object".
func TestReadJavaScriptActionByName(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	jsa, err := b.ReadJavaScriptActionByName("NanoflowCommons.Geocode")
	if err != nil {
		t.Fatalf("ReadJavaScriptActionByName: %v", err)
	}

	if jsa.Documentation == "" {
		t.Error("Documentation empty; expected the Geocode doc comment")
	}
	if jsa.Platform != "All" {
		t.Errorf("Platform = %q, want All", jsa.Platform)
	}
	if jsa.ActionDefaultReturnName != "ReturnValueName" {
		t.Errorf("ActionDefaultReturnName = %q, want ReturnValueName", jsa.ActionDefaultReturnName)
	}

	// Parameters (stored under "Parameters", not the gen "ActionParameters").
	if len(jsa.Parameters) != 3 {
		t.Fatalf("got %d parameters, want 3", len(jsa.Parameters))
	}
	byName := map[string]*types.JavaActionParameter{}
	for _, p := range jsa.Parameters {
		byName[p.Name] = p
	}
	addr := byName["Address"]
	if addr == nil || addr.ParameterType == nil || addr.ParameterType.TypeString() != "String" {
		t.Errorf("Address param type = %v, want String", addr)
	}
	// The enum parameter is wrapped in a BasicParameterType; legacy renders it as
	// "Object", modelsdk recovers the real enumeration.
	prov := byName["GeocodingProvider"]
	if prov == nil {
		t.Fatal("GeocodingProvider parameter missing")
	}
	enum, ok := prov.ParameterType.(*types.EnumerationType)
	if !ok {
		t.Fatalf("GeocodingProvider type = %T, want *types.EnumerationType", prov.ParameterType)
	}
	if enum.Enumeration != "NanoflowCommons.GeocodingProvider" {
		t.Errorf("GeocodingProvider enum = %q, want NanoflowCommons.GeocodingProvider", enum.Enumeration)
	}

	// Return type (stored under "JavaReturnType", not the gen "ActionReturnType").
	if jsa.ReturnType == nil {
		t.Fatal("ReturnType nil; want entity NanoflowCommons.Position")
	}
	if got := jsa.ReturnType.TypeString(); got != "NanoflowCommons.Position" {
		t.Errorf("ReturnType = %q, want NanoflowCommons.Position", got)
	}

	// Exposed-as info (stored under "MicroflowActionInfo", not "ModelerActionInfo").
	if jsa.MicroflowActionInfo == nil {
		t.Fatal("MicroflowActionInfo nil; want exposed-as Geocode/Geolocation")
	}
	if jsa.MicroflowActionInfo.Caption != "Geocode" {
		t.Errorf("exposed-as caption = %q, want Geocode", jsa.MicroflowActionInfo.Caption)
	}
	if jsa.MicroflowActionInfo.Category != "Geolocation" {
		t.Errorf("exposed-as category = %q, want Geolocation", jsa.MicroflowActionInfo.Category)
	}
}

// TestReadJavaScriptActionByName_NotFound confirms a clear error (not a nil,nil
// shell) for an unknown action.
func TestReadJavaScriptActionByName_NotFound(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	if _, err := b.ReadJavaScriptActionByName("NanoflowCommons.NoSuchAction"); err == nil {
		t.Fatal("expected error for unknown JS action, got nil")
	}
}
