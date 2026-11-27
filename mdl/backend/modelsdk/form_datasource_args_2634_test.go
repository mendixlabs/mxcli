// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The page mutator wrote a microflow data source with empty ParameterMappings
// and FormValidations All, so `alter page … set (DataSource: microflow
// M.F(Dag = …))` failed CE1571 (ako/mxcli#2634). It now writes the source as
// create page does.
func TestSerializeFormDataSourceKeepsMappings_2634(t *testing.T) {
	ds := &pages.MicroflowSource{Microflow: "M.DS_Account", FormValidations: "None",
		ParameterMappings: []*pages.MicroflowParameterMapping{{ParameterName: "Dag", Expression: "[%BeginOfCurrentDay%]"}}}
	d := codecPageDeps{}.SerializeFormDataSource(ds, false)
	if d == nil {
		t.Fatal("no data source written")
	}
	raw, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	r := bson.Raw(raw)
	if fv, _ := r.Lookup("MicroflowSettings", "FormValidations").StringValueOK(); fv != "None" {
		t.Errorf("FormValidations %q, want None", fv)
	}
	if p, _ := r.Lookup("MicroflowSettings", "ParameterMappings", "1", "Parameter").StringValueOK(); p != "M.DS_Account.Dag" {
		t.Errorf("mapping Parameter %q, want M.DS_Account.Dag", p)
	}
	if e, _ := r.Lookup("MicroflowSettings", "ParameterMappings", "1", "Expression").StringValueOK(); e != "[%BeginOfCurrentDay%]" {
		t.Errorf("mapping Expression %q", e)
	}
}
