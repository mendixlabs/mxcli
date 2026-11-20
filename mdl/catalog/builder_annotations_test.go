// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#1300: a domain model annotation was in no catalog table, so a
// Starlark rule looking for "TODO" notes covered every microflow (whose
// annotations are activities) and silently skipped every domain model. Each
// row keeps the full caption, newlines included, and the annotation's place
// on the canvas. Module Empty has a domain model with no annotations: the
// control that a module contributes rows only for annotations it has.
func TestBuildDomainModelAnnotations_Issue1300(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })

	const sales, empty = model.ID("mod-sales"), model.ID("mod-empty")
	note := func(id, caption string, x, y, width int) *domainmodel.Annotation {
		a := &domainmodel.Annotation{Caption: caption, Location: model.Point{X: x, Y: y}, Width: width}
		a.ID = model.ID(id)
		return a
	}
	sdm := &domainmodel.DomainModel{
		ContainerID: sales,
		Annotations: []*domainmodel.Annotation{
			note("an-todo", "TODO split sales\nper region", 60, 40, 400),
			note("an-readme", "This domain model defines the data.", 60, 240, 440),
		},
	}
	sdm.ID = "dm-sales"
	edm := &domainmodel.DomainModel{ContainerID: empty}
	edm.ID = "dm-empty"

	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{
		catalog:  cat,
		snapshot: &Snapshot{ID: "snap"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{sales: true, empty: true},
			moduleNames:     map[model.ID]string{sales: "Sales", empty: "Empty"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		domainModelCache: []*domainmodel.DomainModel{sdm, edm},
		tx:               wrapTx(tx),
	}
	if err := b.buildDomainModelAnnotations(); err != nil {
		t.Fatalf("buildDomainModelAnnotations: %v", err)
	}
	if err := b.tx.Commit(); err != nil {
		t.Fatal(err)
	}

	res, err := cat.Query(`SELECT Id, ModuleName, Caption, X, Y, Width FROM annotations ORDER BY Id`)
	if err != nil {
		t.Fatalf("query: %v -- the annotations view must exist", err)
	}
	var got []string
	for _, r := range res.Rows {
		got = append(got, strings.TrimSuffix(fmt.Sprintln(r...), "\n"))
	}
	want := []string{
		"an-readme Sales This domain model defines the data. 60 240 440",
		"an-todo Sales TODO split sales\nper region 60 40 400",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("annotations:\n%q\nwant:\n%q", got, want)
	}
}
