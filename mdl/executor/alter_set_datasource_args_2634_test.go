// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `alter page … set (DataSource: microflow M.F(Dag = [%BeginOfCurrentDay%]))`
// replaced the source and dropped its arguments, so mx check failed CE1571
// "No argument has been selected for parameter 'Dag'" (ako/mxcli#2634,
// upstream mendixlabs/mxcli#1969). create page maps them.
func TestAlterSetDataSourceKeepsArguments_2634(t *testing.T) {
	pb := &pageBuilder{}
	ds := &ast.DataSourceV3{Type: "microflow", Reference: "M.DS_Account", FormValidations: "None",
		Args: []ast.FlowArgV3{{Name: "Dag", Value: "[%BeginOfCurrentDay%]"}}}
	got, err := convertASTDataSource(ds, pb)
	if err != nil {
		t.Fatal(err)
	}
	ms, ok := got.(*pages.MicroflowSource)
	if !ok {
		t.Fatalf("got %T, want a microflow source", got)
	}
	if len(ms.ParameterMappings) != 1 || ms.ParameterMappings[0].ParameterName != "Dag" ||
		ms.ParameterMappings[0].Expression != "[%BeginOfCurrentDay%]" {
		t.Errorf("mappings %+v, want Dag = [%%BeginOfCurrentDay%%]", ms.ParameterMappings)
	}
	if ms.FormValidations != "None" {
		t.Errorf("FormValidations %q, want None", ms.FormValidations)
	}

	nds := &ast.DataSourceV3{Type: "nanoflow", Reference: "M.NF", Args: []ast.FlowArgV3{{Name: "P", Value: "'x'"}}}
	got, err = convertASTDataSource(nds, pb)
	if err != nil {
		t.Fatal(err)
	}
	if nf := got.(*pages.NanoflowSource); len(nf.ParameterMappings) != 1 || nf.ParameterMappings[0].ParameterName != "P" {
		t.Errorf("nanoflow mappings %+v, want P", nf.ParameterMappings)
	}

	// Control: a source without arguments has none.
	got, _ = convertASTDataSource(&ast.DataSourceV3{Type: "microflow", Reference: "M.F"}, pb)
	if n := len(got.(*pages.MicroflowSource).ParameterMappings); n != 0 {
		t.Errorf("a source without arguments got %d mappings", n)
	}
}
