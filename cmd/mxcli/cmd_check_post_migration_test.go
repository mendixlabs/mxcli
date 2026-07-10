// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestWalkForLegacyWidgets verifies the reflection-based walker finds a
// DataGrid embedded at arbitrary depth in a parsed page widget tree.
func TestWalkForLegacyWidgets(t *testing.T) {
	dg := &pages.DataGrid{}
	dg.Name = "GridA"

	// Build a tree: outer DataView → controlBarWidgets list with a DataGrid.
	outer := &pages.DataView{
		Widgets: []pages.Widget{dg},
	}

	var hits []string
	walkForLegacyWidgets(reflect.ValueOf(outer), "11.0.0", func(entry *executor.LegacyWidget, name string) {
		hits = append(hits, entry.GoTypeName+":"+name)
	})

	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d (%v)", len(hits), hits)
	}
	if hits[0] != "DataGrid:GridA" {
		t.Errorf("expected DataGrid:GridA, got %s", hits[0])
	}
}

// TestWalkForLegacyWidgets_NoFalsePositives confirms that a widget tree
// without legacy widgets yields no hits.
func TestWalkForLegacyWidgets_NoFalsePositives(t *testing.T) {
	outer := &pages.DataView{
		Widgets: []pages.Widget{},
	}
	var hits int
	walkForLegacyWidgets(reflect.ValueOf(outer), "11.0.0", func(*executor.LegacyWidget, string) {
		hits++
	})
	if hits != 0 {
		t.Errorf("expected 0 hits on empty DataView, got %d", hits)
	}
}

// TestWalkForLegacyWidgets_VersionGate confirms the scanner skips legacy
// widgets when the project version is BEFORE the deprecation cut-off.
func TestWalkForLegacyWidgets_VersionGate(t *testing.T) {
	dg := &pages.DataGrid{}
	dg.Name = "GridA"
	outer := &pages.DataView{Widgets: []pages.Widget{dg}}

	var hits int
	walkForLegacyWidgets(reflect.ValueOf(outer), "10.18.0", func(*executor.LegacyWidget, string) {
		hits++
	})
	if hits != 0 {
		t.Errorf("expected DataGrid to be allowed on Mendix 10.18; got %d hits", hits)
	}
}
