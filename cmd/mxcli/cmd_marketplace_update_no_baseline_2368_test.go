// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
)

// ako/mxcli#2368: under --no-baseline the update deleted an association another
// module owns without naming it. The edits to the module itself cannot be known
// without the baseline, but the associations reaching into it can, and they are
// gated like any other loss: refused without --force, named with it.
func TestNoBaselineGate_RefusesWhenAnAssociationReachesIn(t *testing.T) {
	deps := []marketplace.Element{{
		Key: marketplace.ElementKey{Type: "ASSOCIATION", Name: "MyFirstModule.Request_Configuration"},
		MDL: "create or modify association MyFirstModule.Request_Configuration\nfrom MyFirstModule.Request to WorkflowCommons.Configuration;",
	}}
	var out bytes.Buffer
	drift, err := noBaselineGate(&out, "WorkflowCommons", deps, false, "")
	if err == nil {
		t.Fatal("update allowed without --force although it removes MyFirstModule.Request_Configuration")
	}
	if !strings.Contains(err.Error(), "MyFirstModule.Request_Configuration") {
		t.Errorf("refusal does not name the association: %v", err)
	}

	out.Reset()
	drift, err = noBaselineGate(&out, "WorkflowCommons", deps, true, "")
	if err != nil {
		t.Fatalf("--force must proceed: %v", err)
	}
	if len(drift.DependentNames()) != 1 {
		t.Errorf("the report after the update would not name the removal: %v", drift.DependentNames())
	}

	// Control: with nothing reaching in, --no-baseline proceeds as before.
	if _, err := noBaselineGate(&out, "WorkflowCommons", nil, false, ""); err != nil {
		t.Fatalf("control: no dependents must pass: %v", err)
	}
}

func TestNoBaselineGate_SavesTheAssociation(t *testing.T) {
	deps := []marketplace.Element{{
		Key: marketplace.ElementKey{Type: "ASSOCIATION", Name: "MyFirstModule.Request_Configuration"},
		MDL: "create or modify association MyFirstModule.Request_Configuration\nfrom MyFirstModule.Request to WorkflowCommons.Configuration;",
	}}
	var out bytes.Buffer
	dir := t.TempDir()
	if _, err := noBaselineGate(&out, "WorkflowCommons", deps, false, dir); err == nil {
		t.Fatal("saving does not lift the refusal; --force does")
	}
	if !strings.Contains(out.String(), "Saved 1") {
		t.Errorf("--save-edits did not save the association:\n%s", out.String())
	}
}
