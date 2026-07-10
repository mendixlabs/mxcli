// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestResolveDocContainer(t *testing.T) {
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	b := &Backend{sessionModules: []*model.Module{mod}}
	// Folders: M / Processing / Archive
	b.sessionFolders = []*types.FolderInfo{
		{ID: "f1", ContainerID: "mod1", Name: "Processing"},
		{ID: "f2", ContainerID: "f1", Name: "Archive"},
	}

	cases := []struct {
		container          model.ID
		wantModule, wantFP string
	}{
		{"mod1", "M", ""},                 // module root
		{"f1", "M", "Processing"},         // one level
		{"f2", "M", "Processing/Archive"}, // nested
	}
	for _, c := range cases {
		mn, fp, err := b.resolveDocContainer(c.container)
		if err != nil || mn != c.wantModule || fp != c.wantFP {
			t.Errorf("resolve(%s) = (%q,%q,%v), want (%q,%q,nil)", c.container, mn, fp, err, c.wantModule, c.wantFP)
		}
	}
	// Unknown container errors rather than panicking.
	if _, _, err := b.resolveDocContainer("nope"); err == nil {
		t.Error("unknown container should error")
	}
	// CreateFolder records a pending folder that ListFolders surfaces.
	f := &model.Folder{ContainerID: "mod1", Name: "New"}
	f.ID = "f3"
	if err := b.CreateFolder(f); err != nil {
		t.Fatal(err)
	}
	_, fp, _ := b.resolveDocContainer("f3")
	if fp != "New" {
		t.Errorf("after CreateFolder, resolve(f3) folderPath = %q, want New", fp)
	}
	// DROP / MOVE folder are rejected.
	if b.DeleteFolder("f1") == nil || b.MoveFolder("f1", "mod1") == nil {
		t.Error("DeleteFolder/MoveFolder should be rejected")
	}
}
