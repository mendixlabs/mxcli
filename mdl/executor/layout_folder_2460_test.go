// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2460 (mendixlabs/mxcli#1905): `create layout … (folder: '…')` passed
// check, and exec refused it as "unknown property folder" — a layout header
// took only layouttype, class, style, layout and showbottombar. A layout is
// filed in the folder as a page or snippet is.
func TestBuildLayoutV3_Folder_Issue2460(t *testing.T) {
	pb := newPopupPageBuilder()
	pb.foldersCache = []*types.FolderInfo{{ID: "f1", ContainerID: "mod1", Name: "Layouts"}}
	l, err := pb.buildLayoutV3(layoutStmt(map[string]any{"LayoutType": "Responsive", "Folder": "Layouts"}, scrollWithMain()))
	if err != nil {
		t.Fatalf("folder refused: %v", err)
	}
	if l.ContainerID != model.ID("f1") {
		t.Errorf("ContainerID = %q, want the Layouts folder f1", l.ContainerID)
	}
	// CONTROL: no folder files it in the module.
	l, err = newPopupPageBuilder().buildLayoutV3(layoutStmt(map[string]any{"LayoutType": "Responsive"}, scrollWithMain()))
	if err != nil || l.ContainerID != model.ID("mod1") {
		t.Errorf("no folder: %v, ContainerID = %q", err, l.ContainerID)
	}
}
