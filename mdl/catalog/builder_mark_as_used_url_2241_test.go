// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2241 (mendixlabs/mxcli#1855): a document's "Mark as used" flag
// and a microflow's URL had no catalog column, so a catalog question about
// unused documents could not leave out the ones Studio Pro treats as used.
func TestCatalogMicroflowsMarkAsUsedAndURL_2241(t *testing.T) {
	mf := &microflows.Microflow{Name: "ACT_Report", MarkAsUsed: true, URL: "reports"}
	mf.ID, mf.ContainerID = "mf1", loopTestModule
	plain := &microflows.Microflow{Name: "ACT_Plain"}
	plain.ID, plain.ContainerID = "mf2", loopTestModule
	nf := &microflows.Nanoflow{Name: "NF_Kept", MarkAsUsed: true}
	nf.ID, nf.ContainerID = "nf1", model.ID(loopTestModule)
	cat := buildFlowsForTest(t, []*microflows.Microflow{mf, plain}, []*microflows.Nanoflow{nf}, nil)

	res, err := cat.Query("select QualifiedName, MarkAsUsed, URL from microflows_data order by QualifiedName")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]string{}
	for _, r := range res.Rows {
		got[toStr(r[0])] = [2]string{toStr(r[1]), toStr(r[2])}
	}
	for qn, want := range map[string][2]string{
		"Loops.ACT_Report": {"1", "reports"},
		"Loops.ACT_Plain":  {"0", ""},
		"Loops.NF_Kept":    {"1", ""},
	} {
		if got[qn] != want {
			t.Errorf("%s: MarkAsUsed, URL = %v, want %v", qn, got[qn], want)
		}
	}
}

func toStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return "0"
	case int64:
		if x != 0 {
			return "1"
		}
		return "0"
	}
	return ""
}
