// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/settingsoverlay"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// duplicateFirstConfiguration appends a copy of the fixture's first server
// configuration with the same Name and a fresh $ID, the shape a merge leaves
// behind (ako/mxcli#2184).
func duplicateFirstConfiguration(t *testing.T, proj string) {
	t.Helper()
	r, err := mmpr.OpenWithOptions(proj, mmpr.OpenOptions{ReadOnly: false})
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer r.Close()
	refs, err := r.ListUnitsByType("Settings$ProjectSettings")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ListUnitsByType = %v, %v", refs, err)
	}
	raw, err := r.GetRawUnitBytes(refs[0].ID)
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var doc bson.M
	if err := bsoncompat.UnmarshalDocM(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	done := false
	for _, part := range settingsoverlay.ArrayElements(doc["Settings"]) {
		if part["$Type"] != "Settings$ConfigurationSettings" {
			continue
		}
		arr := part["Configurations"].(bson.A)
		var src bson.M
		for _, el := range arr[1:] {
			src = el.(bson.M)
			break
		}
		cp, err := bson.Marshal(src)
		if err != nil {
			t.Fatalf("marshal copy: %v", err)
		}
		var dup bson.M
		if err := bsoncompat.UnmarshalDocM(cp, &dup); err != nil {
			t.Fatalf("unmarshal copy: %v", err)
		}
		freshIDs(dup)
		part["Configurations"] = append(arr, dup)
		done = true
	}
	if !done {
		t.Fatal("fixture has no Settings$ConfigurationSettings part")
	}
	contents, err := bson.Marshal(bsonutil.OrderStorageValue(doc))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := mmpr.NewWriterWithReader(r).UpdateRawUnit(refs[0].ID, contents); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
}

func freshIDs(v any) {
	switch x := v.(type) {
	case bson.M:
		if _, ok := x["$ID"]; ok {
			x["$ID"] = bsonutil.NewIDBsonBinary()
		}
		for _, c := range x {
			freshIDs(c)
		}
	case bson.A:
		for _, c := range x {
			freshIDs(c)
		}
	}
}

// A settings write that has nothing to do with configurations failed on a
// project holding two configurations with the same name: both were written onto
// the second stored document and the duplicate-id guard refused the unit.
func TestUpdateProjectSettings_DuplicateConfigurationNames(t *testing.T) {
	proj := copyFixture(t)
	duplicateFirstConfiguration(t, proj)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ps, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	cfgs := ps.Configuration.Configurations
	if len(cfgs) < 2 || cfgs[0].Name != cfgs[len(cfgs)-1].Name {
		t.Fatalf("precondition: want two configurations sharing a name, got %d", len(cfgs))
	}
	if ps.Model == nil {
		t.Fatal("fixture has no model settings")
	}
	ps.Model.HashAlgorithm = "BCrypt"
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("UpdateProjectSettings: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
}
