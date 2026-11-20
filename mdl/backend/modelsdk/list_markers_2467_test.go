// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genSec "github.com/mendixlabs/mxcli/modelsdk/gen/security"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Studio Pro writes a JavaScript action call's ParameterMappings with marker 2
// (25/25 lists in a Studio Pro-made 11.x project, 106/106 ParameterMappings
// lists across 10 owner types in the reporter's) and a module security's
// ModuleRoles with marker 3, empty or not (8/8). mxcli wrote 3 and 1, so Studio
// Pro rewrote both on first open (ako/mxcli#2467).

func marker2467(t *testing.T, doc bson.D, key string) int32 {
	t.Helper()
	for _, e := range doc {
		if e.Key != key {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok || len(arr) == 0 {
			t.Fatalf("%s = %#v, want a marked list", key, e.Value)
		}
		m, ok := arr[0].(int32)
		if !ok {
			t.Fatalf("%s[0] = %#v, want an int32 marker", key, arr[0])
		}
		return m
	}
	t.Fatalf("%s missing", key)
	return 0
}

func unmarshal2467(t *testing.T, raw []byte) bson.D {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestJavaScriptActionCallParameterMappingsMarker_2467(t *testing.T) {
	g := microflowActionToGen(&microflows.JavaScriptActionCallAction{
		JavaScriptAction: "Sim.JS_Echo",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{Parameter: "Sim.JS_Echo.Msg", Value: &microflows.ExpressionBasedCodeActionParameterValue{Expression: "'hi'"}},
		},
	})
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatal(err)
	}
	if m := marker2467(t, unmarshal2467(t, raw), "ParameterMappings"); m != 2 {
		t.Errorf("ParameterMappings marker = %d, want 2 (Studio Pro's)", m)
	}
}

func TestModuleSecurityModuleRolesMarker_2467(t *testing.T) {
	raw, err := (&codec.Encoder{}).Encode(genSec.NewModuleSecurity())
	if err != nil {
		t.Fatal(err)
	}
	if m := marker2467(t, unmarshal2467(t, raw), "ModuleRoles"); m != 3 {
		t.Errorf("empty ModuleRoles marker = %d, want 3 (Studio Pro's)", m)
	}
}

// A unit an older mxcli wrote keeps its wrong marker through every rewrite that
// does not rebuild the list, unless the write path normalises it. The control is
// a list whose marker is already right, which must come back unchanged.
func TestStoredWrongMarkersNormalised_2467(t *testing.T) {
	sec, _ := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Security$ModuleSecurity"},
		{Key: "ModuleRoles", Value: bson.A{int32(1)}},
	})
	if m := marker2467(t, unmarshal2467(t, canon.NormalizeListMarkers(sec)), "ModuleRoles"); m != 3 {
		t.Errorf("stored ModuleRoles marker = %d after normalising, want 3", m)
	}
	call, _ := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Microflows$JavaScriptActionCallAction"},
		{Key: "ParameterMappings", Value: bson.A{int32(3)}},
	})
	if m := marker2467(t, unmarshal2467(t, canon.NormalizeListMarkers(call)), "ParameterMappings"); m != 2 {
		t.Errorf("stored ParameterMappings marker = %d after normalising, want 2", m)
	}
	// Control: UserRole.ModuleRoles is a by-name list, marker 1, and stays so.
	ur, _ := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Security$UserRole"},
		{Key: "ModuleRoles", Value: bson.A{int32(1), "Sim.User"}},
	})
	if m := marker2467(t, unmarshal2467(t, canon.NormalizeListMarkers(ur)), "ModuleRoles"); m != 1 {
		t.Errorf("UserRole.ModuleRoles marker = %d, want 1 (untouched)", m)
	}
}
