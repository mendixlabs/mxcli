// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestSessionModuleResolution verifies that a module registered this session is
// resolvable by ID and name (so a later "create enumeration NewMod.X" finds it),
// without needing the local reader.
func TestSessionModuleResolution(t *testing.T) {
	b := &Backend{}
	mod := &model.Module{Name: "NewMod"}
	mod.ID = model.ID("mcp~module~NewMod")
	b.sessionModules = append(b.sessionModules, mod)

	got, err := b.GetModuleByName("NewMod")
	if err != nil || got.Name != "NewMod" {
		t.Fatalf("GetModuleByName(NewMod) = %+v / %v", got, err)
	}
	got, err = b.GetModule("mcp~module~NewMod")
	if err != nil || got.ID != "mcp~module~NewMod" {
		t.Fatalf("GetModule(by id) = %+v / %v", got, err)
	}

	// The synthetic domain-model ID handed out for a session module round-trips
	// through moduleNameForDomainModel back to the module name — so a CREATE
	// ENTITY in the same run resolves to the right module. (GetDomainModel itself
	// reconstructs the module's live entities from PED, which needs a live client,
	// so it's covered by the live tests rather than here.)
	name, err := b.moduleNameForDomainModel(model.ID(sessionDMPrefix + "NewMod"))
	if err != nil || name != "NewMod" {
		t.Fatalf("moduleNameForDomainModel = %q / %v", name, err)
	}
}
