// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
)

// TestIssue2323_RefusedTargetWritesNothing guards ako/mxcli#2323: MoveEntity
// writes two units, the source without the entity and the target with it. When
// the target write was refused, the source had already been written, so the
// entity was in neither module. A script that stops is rolled back as a whole,
// but `exec --continue-on-error` goes on to the next statement — and on a
// generated project lost 9 of 12 moved entities.
//
// The refusal is provoked the way #2322 provoked it: the target unit already
// holds an element with the moved entity's $ID, so the target with the entity
// added holds that $ID twice.
func TestIssue2323_RefusedTargetWritesNothing(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	srcID, srcMod, _, _, childID, _ := associationSubject(t, b)
	dstID, dstMod := otherDomainModel(t, b, srcID)
	if dstID == "" {
		t.Skip("fixture has no second domain model to move into")
	}
	ent := entityByID(t, b, srcID, childID)
	plantElementWithID(t, b, dstID, childID)

	_, err := b.MoveEntity(ent, srcID, dstID, srcMod, dstMod)
	if err == nil {
		t.Fatal("MoveEntity succeeded; the planted duplicate should have refused the target")
	}
	if !strings.Contains(err.Error(), "nothing was written") {
		t.Errorf("error does not say nothing was written: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	dm, err := b2.loadDomainModelGen(srcID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range dm.EntitiesItems() {
		if string(e.ID()) == string(childID) {
			return
		}
	}
	t.Errorf("entity %s is gone from %s after a move whose target was refused", ent.Name, srcMod)
}

// plantElementWithID adds to the domain model dmID an annotation carrying id
// as its $ID.
func plantElementWithID(t *testing.T, b *Backend, dmID, id model.ID) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(dmID))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	ann := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(string(id))},
		{Key: "$Type", Value: "DomainModels$Annotation"},
		{Key: "Caption", Value: "planted"},
		{Key: "Location", Value: "0;0"},
		{Key: "Width", Value: int32(100)},
	}
	planted := false
	for i, e := range d {
		if e.Key == "Annotations" {
			list, _ := e.Value.(bson.A)
			if len(list) == 0 {
				list = bson.A{int32(3)}
			}
			d[i].Value = append(list, ann)
			planted = true
		}
	}
	if !planted {
		t.Fatal("domain model has no Annotations list")
	}
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.writer.UpdateRawUnit(string(dmID), out); err != nil {
		t.Fatalf("plant: %v", err)
	}
}
