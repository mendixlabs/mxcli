// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2322: moving Src.A2 to Dst converts Src.A3_A2 (FROM A3, TO A2)
// into a cross-module association that stays in Src, keeping its $ID. The
// rewrite of Src's domain model then paired it, by position, with the
// cross-association already in that list — Src.A2_A1, from moving A1 — and
// gave it A2_A1's $ID (and, on an mxcli-created model where GUID == $ID, its
// GUID). A3_A2's own identity was gone, the two associations shared one, and
// mx check reported CE9208 ("map to the same database table"); the next move
// refused the write as two elements with one $ID.
//
// An element that already carries a $ID the stored document holds is that
// element, whatever list it now sits in, and keeps it.
func assocDoc(id byte, typ, name string) bson.D {
	return bson.D{{Key: "$ID", Value: id16(id)}, {Key: "$Type", Value: typ},
		{Key: "DeleteBehavior", Value: bson.D{{Key: "$ID", Value: id16(id + 1)}, {Key: "$Type", Value: "DomainModels$DeleteBehavior"}}},
		{Key: "GUID", Value: id16(id)}, {Key: "Name", Value: name}}
}

func domainModelWith(assocs, cross []bson.D) []byte {
	a, c := bson.A{int32(3)}, bson.A{int32(3)}
	for _, x := range assocs {
		a = append(a, x)
	}
	for _, x := range cross {
		c = append(c, x)
	}
	b, _ := bson.Marshal(bson.D{{Key: "$ID", Value: id16(0xD0)}, {Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "Associations", Value: a}, {Key: "CrossAssociations", Value: c}})
	return b
}

func TestTransplant_ElementWithAStoredIDKeepsIt_Issue2322(t *testing.T) {
	stored := domainModelWith(
		[]bson.D{assocDoc(0x32, "DomainModels$Association", "A3_A2"), assocDoc(0x43, "DomainModels$Association", "A4_A3")},
		[]bson.D{assocDoc(0x21, "DomainModels$CrossAssociation", "A2_A1")})
	written := domainModelWith(
		[]bson.D{assocDoc(0x43, "DomainModels$Association", "A4_A3")},
		[]bson.D{assocDoc(0x32, "DomainModels$CrossAssociation", "A3_A2")})
	out, _ := Reconcile(written, stored)
	var d bson.D
	if err := bson.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	walkElements(d, func(e bson.D) {
		if docLookup(e, "Name") != "A3_A2" {
			return
		}
		id, _ := docLookup(e, "$ID").(bson.Binary)
		guid, _ := docLookup(e, "GUID").(bson.Binary)
		if string(id.Data) != string(id16(0x32).Data) || string(guid.Data) != string(id16(0x32).Data) {
			t.Errorf("A3_A2 $ID/GUID = %x/%x, want its own %x", id.Data, guid.Data, id16(0x32).Data)
		}
		db, _ := docLookup(e, "DeleteBehavior").(bson.D)
		if dbID, _ := docLookup(db, "$ID").(bson.Binary); string(dbID.Data) != string(id16(0x33).Data) {
			t.Errorf("A3_A2's delete behaviour $ID = %x, want its own %x", dbID.Data, id16(0x33).Data)
		}
	})
}

// CONTROL: a rebuilt element with a fresh $ID still takes the stored one of
// the element it stands for — the transplant's purpose.
func TestTransplant_FreshIDStillTakesTheStoredOne_Issue2322(t *testing.T) {
	stored := domainModelWith(nil, []bson.D{assocDoc(0x21, "DomainModels$CrossAssociation", "A2_A1")})
	written := domainModelWith(nil, []bson.D{assocDoc(0x99, "DomainModels$CrossAssociation", "A2_A1")})
	var d bson.D
	_ = bson.Unmarshal(TransplantIDs(written, stored), &d)
	walkElements(d, func(e bson.D) {
		if docLookup(e, "Name") == "A2_A1" {
			if id, _ := docLookup(e, "$ID").(bson.Binary); string(id.Data) != string(id16(0x21).Data) {
				t.Errorf("A2_A1 $ID = %x, want the stored %x", id.Data, id16(0x21).Data)
			}
		}
	})
}
