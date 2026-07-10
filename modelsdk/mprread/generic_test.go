// SPDX-License-Identifier: Apache-2.0

package mprread_test

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
)

// TestListUnitsByType_Microflows verifies the generic helper returns non-empty
// results for Microflows$Microflow and that every decoded element carries the
// correct $Type name.
func TestListUnitsByType_Microflows(t *testing.T) {
	r := openTestReader(t)
	mfs, err := mprread.ListUnitsByType[*genMf.Microflow](r)
	if err != nil {
		t.Fatalf("ListUnitsByType[Microflow]: %v", err)
	}
	if len(mfs) == 0 {
		t.Fatal("ListUnitsByType[Microflow]: got 0, expected non-zero")
	}
	for _, mf := range mfs {
		if mf == nil {
			t.Fatal("ListUnitsByType[Microflow]: nil entry in result")
		}
		if mf.TypeName() != "Microflows$Microflow" {
			t.Errorf("TypeName = %q, want %q", mf.TypeName(), "Microflows$Microflow")
		}
		if mf.ID() == "" {
			t.Errorf("microflow %q has empty ID", mf.Name())
		}
		var _ element.Element = mf
	}
}

// TestListUnitsByType_Nanoflows verifies the generic helper succeeds for
// Microflows$Nanoflow. The fixture may have zero nanoflows, so we only check
// entries when present.
func TestListUnitsByType_Nanoflows(t *testing.T) {
	r := openTestReader(t)
	nfs, err := mprread.ListUnitsByType[*genMf.Nanoflow](r)
	if err != nil {
		t.Fatalf("ListUnitsByType[Nanoflow]: %v", err)
	}
	for _, nf := range nfs {
		if nf == nil {
			t.Fatal("ListUnitsByType[Nanoflow]: nil entry in result")
		}
		if nf.TypeName() != "Microflows$Nanoflow" {
			t.Errorf("TypeName = %q, want %q", nf.TypeName(), "Microflows$Nanoflow")
		}
		if nf.ID() == "" {
			t.Errorf("nanoflow %q has empty ID", nf.Name())
		}
		var _ element.Element = nf
	}
}
