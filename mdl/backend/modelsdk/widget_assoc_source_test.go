// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Bug 4 — the modelsdk engine refused *pages.AssociationSource for DataView and
// ListView (only pluggable widgets handled it), forcing MXCLI_ENGINE=legacy.
// Both now emit a Forms$AssociationSource with an IndirectEntityRef of steps,
// matching the canonical legacy serializeAssociationSource.

func assertAssociationSource(t *testing.T, src bsonv1.D, wantAssoc, wantDest string, wantSourceVar bool) {
	t.Helper()
	assertAssociationSourceTyped(t, src, "Forms$AssociationSource", wantAssoc, wantDest, wantSourceVar)
}

// assertAssociationSourceTyped checks the shared IndirectEntityRef structure under
// a given wrapper $Type — Forms$AssociationSource (list widgets) or
// Forms$DataViewSource (a DataView's "data from context over association").
func assertAssociationSourceTyped(t *testing.T, src bsonv1.D, wantType, wantAssoc, wantDest string, wantSourceVar bool) {
	t.Helper()
	if got := docGet(src, "$Type"); got != wantType {
		t.Fatalf("$Type = %v, want %v", got, wantType)
	}
	if got := docGet(src, "ForceFullObjects"); got != false {
		t.Errorf("ForceFullObjects = %v, want false", got)
	}
	if wantSourceVar {
		if docGet(src, "SourceVariable") == nil {
			t.Errorf("SourceVariable is nil, want a Forms$PageVariable")
		}
	} else if got := docGet(src, "SourceVariable"); got != nil {
		t.Errorf("SourceVariable = %v, want nil ($currentObject)", got)
	}
	entityRef, ok := docGet(src, "EntityRef").(bsonv1.D)
	if !ok {
		t.Fatalf("EntityRef not a document (got %T) — this is the legacy null-EntityRef bug", docGet(src, "EntityRef"))
	}
	if got := docGet(entityRef, "$Type"); got != "DomainModels$IndirectEntityRef" {
		t.Errorf("EntityRef.$Type = %v, want DomainModels$IndirectEntityRef", got)
	}
	step := firstListItem(t, docGet(entityRef, "Steps"))
	if got := docGet(step, "Association"); got != wantAssoc {
		t.Errorf("step.Association = %v, want %v", got, wantAssoc)
	}
	if got := docGet(step, "DestinationEntity"); got != wantDest {
		t.Errorf("step.DestinationEntity = %v, want %v", got, wantDest)
	}
}

// Bug 5 (reclassified) — a DataView over a to-one association is "data from
// context": a Forms$DataViewSource whose EntityRef is an IndirectEntityRef, NOT a
// Forms$AssociationSource (which MxBuild rejects on a DataView: CE6705).
func TestDataViewAssociationSource_Serialized(t *testing.T) {
	dv := &pages.DataView{
		DataSource: &pages.AssociationSource{
			BaseElement: model.BaseElement{TypeName: "Forms$AssociationSource"},
			EntityPath:  "Sales.Order_Customer/Sales.Customer",
			// ContextVariable empty → $currentObject
		},
	}
	dv.Name = "dvCust"
	doc := encodeWidget(t, dv)
	src, ok := docGet(doc, "DataSource").(bsonv1.D)
	if !ok {
		t.Fatalf("DataSource not serialized (got %T)", docGet(doc, "DataSource"))
	}
	assertAssociationSourceTyped(t, src, "Forms$DataViewSource", "Sales.Order_Customer", "Sales.Customer", false)
}

func TestListViewAssociationSource_Serialized(t *testing.T) {
	lv := &pages.ListView{
		DataSource: &pages.AssociationSource{
			BaseElement:     model.BaseElement{TypeName: "Forms$AssociationSource"},
			EntityPath:      "Sales.Order_Lines/Sales.OrderLine",
			ContextVariable: "Order", // page-parameter rooted
		},
	}
	lv.Name = "lvLines"
	doc := encodeWidget(t, lv)
	src, ok := docGet(doc, "DataSource").(bsonv1.D)
	if !ok {
		t.Fatalf("DataSource not serialized (got %T)", docGet(doc, "DataSource"))
	}
	assertAssociationSource(t, src, "Sales.Order_Lines", "Sales.OrderLine", true)
}
