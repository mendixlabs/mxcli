// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1436: the catalog held no folders, so a rule could only derive a
// folder from the `folder` of the documents in it, and an empty folder did not
// exist. "Every module has the folders Objects and UI" then reported a folder
// that is there, empty, as missing.
//
// Module Sales holds:
//
//	Objects/            one document, one subfolder
//	Objects/Order/      empty
//	UI/                 empty
//	UI/Pages/Snippets   one folder whose name holds a slash, as DeepLink's does:
//	                    depth 2, though its path has two slashes
//	(module root)       one document, the control: it is in no folder
func TestBuildFolders_Issue1436(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })

	const sales = model.ID("mod-sales")
	folders := []*types.FolderInfo{
		{ID: "f-objects", ContainerID: sales, Name: "Objects"},
		{ID: "f-order", ContainerID: "f-objects", Name: "Order"},
		{ID: "f-ui", ContainerID: sales, Name: "UI"},
		{ID: "f-slash", ContainerID: "f-ui", Name: "Pages/Snippets"},
	}
	h := &hierarchy{
		moduleIDs:       map[model.ID]bool{sales: true},
		moduleNames:     map[model.ID]string{sales: "Sales"},
		containerParent: map[model.ID]model.ID{},
		folderNames:     map[model.ID]string{},
		unitTypes:       map[model.ID]string{},
	}
	for _, f := range folders {
		h.folderNames[f.ID] = f.Name
		h.containerParent[f.ID] = f.ContainerID
	}
	for id, parent := range map[model.ID]model.ID{"u-flow": "f-objects", "u-dm": sales} {
		h.containerParent[id] = parent
		h.unitTypes[id] = "Microflows$Microflow"
	}

	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{
		catalog:   cat,
		snapshot:  &Snapshot{ID: "snap"},
		hierarchy: h,
		tx:        wrapTx(tx),
	}
	if err := b.buildFolders(); err != nil {
		t.Fatalf("buildFolders: %v", err)
	}
	if err := b.tx.Commit(); err != nil {
		t.Fatal(err)
	}

	res, err := cat.Query(`SELECT Id, ModuleName, Name, Path, ParentPath, Depth, DocumentCount, FolderCount
		FROM folders ORDER BY Path`)
	if err != nil {
		t.Fatalf("query: %v -- the folders view must exist", err)
	}
	var got []string
	for _, r := range res.Rows {
		got = append(got, strings.TrimSuffix(fmt.Sprintln(r...), "\n"))
	}
	want := []string{
		"f-objects Sales Objects Objects  1 1 1",
		"f-order Sales Order Objects/Order Objects 2 0 0",
		"f-ui Sales UI UI  1 0 1",
		"f-slash Sales Pages/Snippets UI/Pages/Snippets UI 2 0 0",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("folders:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
