// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#437 (ako/mxcli#1240): `show structure` at depth 2 and 3
// listed a module's documents flat, whatever folders they sit in. They are now
// nested under their folders; documents at the module root come first, and an
// entity (which lives in the domain model, not a folder) stays there.
func TestNestStructureByFolder_Issue1240(t *testing.T) {
	lines := []string{
		"  Entity Shop.Order [Number]",
		"    → Shop.Customer (1)",
		"  Microflow Shop.ACT_Create()",
		"  Microflow Shop.ACT_Delete()",
		"  Microflow Shop.ACT_Root()",
		"  Microflow Shop.SUB_Validate()",
		"  Page Shop.Order_Overview",
		"  Constant Shop.ApiUrl: String",
	}
	folders := map[string]string{
		"Shop.ACT_Create":     "Orders/Actions",
		"Shop.ACT_Delete":     "Orders/Actions",
		"Shop.SUB_Validate":   "Orders",
		"Shop.Order_Overview": "Orders/Pages",
	}
	got := strings.Join(nestStructureByFolder(lines, folders), "\n")
	want := strings.Join([]string{
		"  Entity Shop.Order [Number]",
		"    → Shop.Customer (1)",
		"  Microflow Shop.ACT_Root()",
		"  Constant Shop.ApiUrl: String",
		"  Orders/",
		"    Microflow Shop.SUB_Validate()",
		"    Actions/",
		"      Microflow Shop.ACT_Create()",
		"      Microflow Shop.ACT_Delete()",
		"    Pages/",
		"      Page Shop.Order_Overview",
	}, "\n")
	if got != want {
		t.Errorf("nested structure:\n%s\n--- want\n%s", got, want)
	}
}

// CONTROL: a module with no folders prints exactly as before.
func TestNestStructureByFolder_NoFoldersUnchanged_Issue1240(t *testing.T) {
	lines := []string{"  Entity M.E [A]", "  Microflow M.F()", "  Page M.P"}
	if got := nestStructureByFolder(lines, map[string]string{}); strings.Join(got, "\n") != strings.Join(lines, "\n") {
		t.Errorf("got %q, want the input unchanged", got)
	}
}
