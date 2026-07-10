// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// TestUpdateModuleSettings_RoundTrip reads a module's settings, adds a jar
// dependency with an exclusion, writes, and confirms both round-trip.
func TestUpdateModuleSettings_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	ms, err := b.GetModuleSettings(mod.ID)
	if err != nil {
		t.Fatalf("GetModuleSettings: %v", err)
	}
	ms.JarDependencies = append(ms.JarDependencies, &types.JarDependency{
		GroupID: "org.example", ArtifactID: "lib", Version: "1.2.3", IsIncluded: true,
		Exclusions: []*types.JarDependencyExclusion{{GroupID: "com.bad", ArtifactID: "evil"}},
	})
	if err := b.UpdateModuleSettings(ms); err != nil {
		t.Fatalf("UpdateModuleSettings: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	ms2, err := b2.GetModuleSettings(mod.ID)
	if err != nil {
		t.Fatalf("GetModuleSettings(2): %v", err)
	}
	var found *types.JarDependency
	for _, d := range ms2.JarDependencies {
		if d.GroupID == "org.example" && d.ArtifactID == "lib" {
			found = d
		}
	}
	if found == nil {
		t.Fatalf("jar dependency not round-tripped: %+v", ms2.JarDependencies)
	}
	if found.Version != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", found.Version)
	}
	if len(found.Exclusions) != 1 || found.Exclusions[0].ArtifactID != "evil" {
		t.Errorf("exclusion not round-tripped: %+v", found.Exclusions)
	}
}
