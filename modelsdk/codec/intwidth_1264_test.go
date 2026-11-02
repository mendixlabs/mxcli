// SPDX-License-Identifier: Apache-2.0

package codec_test

// ako/mxcli#1264 (mendixlabs/mxcli#675): the codec wrote every Mendix "Integer"
// property — canvas and pop-up dimensions, flow connection indices, tab
// indices, layout-grid weights, … — as BSON int32, because modelsdk/gen binds
// reflection Integer as Primitive[int32] and Primitive.BSONValue returned the Go
// value unchanged. Studio Pro writes int64.
//
// Measured, not assumed: 15,354 units from nine Studio Pro-authored projects
// (a blank 11.13.0 app from `mx create-project`, and the .mpr inside eight app
// packages spanning Mendix 10.6.4 to 11.13.0) hold 208,038 integer property
// values, typed-array markers excluded: 208,038 int64, 0 int32. (The only
// other numbers are 154 doubles, all Projects$ModuleImpl.NewSortIndex, which
// gen binds float64.) 137 of the 331 Integer bindings the walk below reaches
// occur there, and every one is int64-only, so no exception list is needed.
// mx check accepts either width, which is why this survived: the mismatch is
// visible only in the BSON.

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/modelsdk/property"

	// Register every gen package, so the walk below reaches every type.
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/appservices"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/authentication"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/businessevents"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/changedatacapture"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/client"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/codeactions"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/connectorkit"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/constants"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/customblobdocuments"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/customicons"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/customwidgets"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/databaseconnector"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/datasets"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/datatransformers"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/documenttemplates"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/enumerations"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/exceldataimporter"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/exportmappings"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/expressions"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/images"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/importmappings"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/integrationoverview"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/javaactions"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/javascriptactions"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/jsonstructures"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/kafka"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/mappings"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/menus"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/messagedefinitions"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/metamodelversion"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/mlmappings"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/nanoflows"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/nativepages"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/navigation"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/odatapublish"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/projects"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/queues"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/regularexpressions"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/reports"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/rest"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/scheduledevents"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/security"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/services"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/settings"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/url"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/webservices"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/xmlschemas"
)

// Every Integer property of every registered gen type encodes as BSON int64.
// The walk is over the registry rather than a hand-picked list, so a type
// added by a re-vendored gen is covered without anyone remembering it.
func TestEncoder_EveryIntegerPropertyIsInt64(t *testing.T) {
	seen := map[string]bool{}
	checked, types := 0, 0
	for _, name := range codec.DefaultRegistry.TypeNames() {
		factory, _ := codec.DefaultRegistry.Lookup(name)
		el := factory()
		if seen[el.TypeName()] {
			continue // an alias of a type already walked
		}
		seen[el.TypeName()] = true

		var keys []string
		for _, p := range el.Properties() {
			ip, ok := p.(*property.Primitive[int32])
			if !ok {
				continue
			}
			ip.Set(7)
			keys = append(keys, p.Name())
		}
		if len(keys) == 0 {
			continue
		}
		types++
		out, err := (&codec.Encoder{}).Encode(el)
		if err != nil {
			t.Fatalf("encode %s: %v", el.TypeName(), err)
		}
		raw := bson.Raw(out)
		for _, k := range keys {
			v, err := raw.LookupErr(k)
			if err != nil {
				t.Errorf("%s.%s: set but not encoded", el.TypeName(), k)
				continue
			}
			checked++
			if v.Type != bson.TypeInt64 {
				t.Errorf("%s.%s encoded as BSON %s, want int64 — Studio Pro stores Integer properties as int64",
					el.TypeName(), k, v.Type)
			}
		}
	}
	// Guard against a vacuous pass: the walk reaches 331 Integer properties on
	// 173 types today. If it stops reaching them (a package dropped from the
	// imports above, a rename of the property type), say so.
	t.Logf("checked %d Integer properties on %d gen types", checked, types)
	if checked < 300 {
		t.Fatalf("checked %d Integer properties, want >= 300 — the registry walk is not reaching gen", checked)
	}
}

// A rewrite of a Studio Pro document must not narrow it. The stored page holds
// its canvas at int64; editing the width re-encodes the dirty key, and before
// the fix that key came back int32 while its untouched sibling stayed int64 —
// one document, two widths for the same kind of property.
func TestEncoder_RewriteKeepsStudioProIntegerWidth(t *testing.T) {
	stored, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: "P"},
		{Key: "CanvasWidth", Value: int64(800)},
		{Key: "CanvasHeight", Value: int64(600)},
		{Key: "PopupWidth", Value: int64(0)},
		{Key: "PopupHeight", Value: int64(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(stored)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	pg, ok := el.(*genPg.Page)
	if !ok {
		t.Fatalf("decoded %T, want *pages.Page", el)
	}
	pg.SetCanvasWidth(1198)
	pg.SetPopupWidth(640)

	out, err := (&codec.Encoder{}).Encode(pg)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw := bson.Raw(out)
	for key, want := range map[string]int64{"CanvasWidth": 1198, "CanvasHeight": 600, "PopupWidth": 640, "PopupHeight": 0} {
		v := raw.Lookup(key)
		if v.Type != bson.TypeInt64 {
			t.Errorf("%s encoded as BSON %s, want int64", key, v.Type)
			continue
		}
		if got := v.Int64(); got != want {
			t.Errorf("%s = %d, want %d", key, got, want)
		}
	}
}
