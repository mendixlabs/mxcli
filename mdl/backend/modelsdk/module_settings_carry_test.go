// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/TestApp's WorkflowCommons settings (Marketplace 4.11.0, Studio Pro
// 11.16.0): a Checksum, a PackageId and EnableDetailedTroubleshooting false —
// three values that differ from their defaults, and that mxcli does not model.
const workflowCommonsSettingsFixture = "testdata/TestApp.WorkflowCommons.ModuleSettings.11_16.bson"

// #1920: a jar dependency change rebuilt the settings unit from the keys mxcli
// models, so the write path filled the rest with defaults: the PackageId and
// the Marketplace Checksum were wiped and EnableDetailedTroubleshooting turned
// true. Every key but JarDependencies must come back as Studio Pro stored it.
func TestModuleSettingsContents_KeepsWhatItDoesNotModel(t *testing.T) {
	stored, err := os.ReadFile(workflowCommonsSettingsFixture)
	if err != nil {
		t.Fatal(err)
	}
	var before bson.M
	if err := bson.Unmarshal(stored, &before); err != nil {
		t.Fatal(err)
	}
	// The controls: the fixture must hold non-default values, or a rebuild
	// would pass this test too.
	if before["Checksum"] == "" || before["PackageId"] == "" || before["EnableDetailedTroubleshooting"] != false {
		t.Fatalf("fixture lost its non-default values: %v", before)
	}

	ms := &types.ModuleSettings{
		ID:                  model.ID("7a01f032-a89d-46c1-ac0a-9cf136570db5"),
		ExportLevel:         "Source",
		ProtectedModuleType: "AddOn",
		Version:             "4.11.0",
		JarDependencies: []*types.JarDependency{{
			GroupID: "org.example", ArtifactID: "probe", Version: "1.0.0", IsIncluded: true,
		}},
	}
	out, err := moduleSettingsContents(stored, ms)
	if err != nil {
		t.Fatal(err)
	}
	var after bson.M
	if err := bson.Unmarshal(out, &after); err != nil {
		t.Fatal(err)
	}
	for k, v := range before {
		if k == "JarDependencies" {
			continue
		}
		if !reflect.DeepEqual(after[k], v) {
			t.Errorf("%s: stored %v, written %v", k, v, after[k])
		}
	}
	if deps, _ := after["JarDependencies"].(bson.A); len(deps) != 2 {
		t.Errorf("JarDependencies = %v, want the marker and the one dependency", after["JarDependencies"])
	}
	if raw := bson.Raw(out); len(raw.Index(0).Key()) == 0 || raw.Index(0).Key() != "$ID" {
		t.Errorf("first key = %q, want $ID", raw.Index(0).Key())
	}
}

// A statement that does change a modelled key still changes it.
func TestModuleSettingsContents_WritesModelledKeys(t *testing.T) {
	stored, err := os.ReadFile(workflowCommonsSettingsFixture)
	if err != nil {
		t.Fatal(err)
	}
	ms := &types.ModuleSettings{ID: "7a01f032-a89d-46c1-ac0a-9cf136570db5", Version: "4.12.0"}
	out, err := moduleSettingsContents(stored, ms)
	if err != nil {
		t.Fatal(err)
	}
	var after bson.M
	if err := bson.Unmarshal(out, &after); err != nil {
		t.Fatal(err)
	}
	if after["Version"] != "4.12.0" {
		t.Errorf("Version = %v, want 4.12.0", after["Version"])
	}
}
