// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1914, against the real backend: a create or modify that restates
// a stored flow takes the SPLICE path, which rebuilt the call external action
// and dropped its Associations to include — the #1922 carry only covered the
// full rebuild. Measured on PedApp with the reporter's service shape (a
// command DTO with a Quantity sub-DTO, and a Unit under it).
func TestIncludeAssociations_SpliceKeepsTheStoredTree(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	metadata, err := filepath.Abs(filepath.Join("testdata", "bug1914_metadata.xml"))
	if err != nil {
		t.Fatal(err)
	}
	setup := `mdl 1;
create constant MyFirstModule.SvcUrl1914 ( Type: String, DefaultValue: 'https://example.com/odata/' );
create consumed odata service MyFirstModule.Bug1914 ( Version: '1.0', ODataVersion: OData4,
  MetadataUrl: '` + metadata + `', Timeout: 300, ServiceUrl: MyFirstModule.SvcUrl1914 );
create external entities from MyFirstModule.Bug1914 into MyFirstModule;`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	flow := func(clause string) string {
		return `mdl 1;
create or modify microflow MyFirstModule.ACT_Buffer1914 ($Cmd: MyFirstModule.CreateBufferCommand)
begin
  call external action MyFirstModule.Bug1914.CreateBuffer(command = $Cmd` + clause + `);
end;`
	}
	describe := func() string {
		t.Helper()
		out.Reset()
		if err := agreeExec(t, exec, "describe microflow MyFirstModule.ACT_Buffer1914;"); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	const tree = " include associations (MyFirstModule.Quantity (MyFirstModule.Unit_2))"
	if err := agreeExec(t, exec, flow(tree)); err != nil {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	if d := describe(); !strings.Contains(d, "command = $Cmd"+tree+")") {
		t.Fatalf("the authored tree does not describe back:\n%s", d)
	}

	out.Reset()
	if err := agreeExec(t, exec, flow("")); err != nil {
		t.Fatalf("restate without the clause: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), carriedIncludesRule) || !strings.Contains(out.String(), "spliced") {
		t.Errorf("the splice did not warn about the carried tree:\n%s", out.String())
	}
	if d := describe(); !strings.Contains(d, "command = $Cmd"+tree+")") {
		t.Errorf("the splice dropped the stored tree:\n%s", d)
	}

	out.Reset()
	if err := agreeExec(t, exec, flow(" include associations ()")); err != nil {
		t.Fatalf("clear: %v\n%s", err, out.String())
	}
	if d := describe(); strings.Contains(d, "include associations") || strings.Contains(out.String(), carriedIncludesRule) {
		t.Errorf("include associations () did not clear the tree quietly:\n%s", d)
	}
}
