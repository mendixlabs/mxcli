// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2185 (mendixlabs/mxcli#1838): a parameter `create or modify` adds
// between stored ones came with the position derived from its index, which is
// where the stored parameter after it already sits, so the two icons overlapped
// (and DESCRIBE then carried the overlap into any later rebuild). The added
// parameter now goes to a slot no parameter uses: after the rightmost one.
func TestSetHeader_AddedParameterDoesNotLandOnAStoredOne(t *testing.T) {
	positions := func(m *Mutator) map[string]string {
		out := map[string]string{}
		for _, el := range arrayElements(dGet(dDoc(m.doc, "ObjectCollection"), "Objects")) {
			if d, _ := el.(bson.D); dString(d, "$Type") == parameterType {
				out[dString(d, "Name")] = dString(d, "RelativeMiddlePoint")
			}
		}
		return out
	}

	stored := headerUnit(param("Count", "DataTypes$IntegerType", 200), param("List", "DataTypes$ListType", 300))
	m, _ := newMutator(t, stored)
	if _, err := m.SetHeader(declaredDoc("Hidden", "DataTypes$VoidType",
		freshParam("Count", "DataTypes$IntegerType", 200),
		freshParam("Extra", "DataTypes$StringType", 300),
		freshParam("List", "DataTypes$ListType", 400))); err != nil {
		t.Fatal(err)
	}
	got := positions(m)
	if got["Count"] != "200;53" || got["List"] != "300;53" {
		t.Errorf("stored parameters moved: %v", got)
	}
	if got["Extra"] == got["List"] || got["Extra"] == got["Count"] {
		t.Fatalf("the added parameter shares a position: %v", got)
	}
	if got["Extra"] != "400;53" {
		t.Errorf("Extra at %s, want 400;53 (after the rightmost parameter)", got["Extra"])
	}
	if _, err := m.Bytes(); err != nil {
		t.Fatalf("integrity: %v", err)
	}

	// Control: an added parameter whose slot is free keeps it.
	m, _ = newMutator(t, headerUnit(param("A", "DataTypes$StringType", 200)))
	if _, err := m.SetHeader(declaredDoc("Hidden", "DataTypes$VoidType",
		freshParam("A", "DataTypes$StringType", 200), freshParam("N", "DataTypes$BooleanType", 300))); err != nil {
		t.Fatal(err)
	}
	if got := positions(m); got["N"] != "300;53" {
		t.Errorf("appended parameter moved: %v", got)
	}
}
