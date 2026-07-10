// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestGetPage_SessionVisible: a page created this session is resolvable by ID
// within the same run (mirrors ListPages / GetMicroflow), not only after save.
// No reader is set, so a hit must come from the session cache.
func TestGetPage_SessionVisible(t *testing.T) {
	p := &pages.Page{}
	p.ID = "mcp~page~M~P"
	p.Name = "P"
	b := &Backend{sessionPages: []*pages.Page{p}}

	got, err := b.GetPage("mcp~page~M~P")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got != p {
		t.Errorf("expected the session-created page, got %v", got)
	}
}
