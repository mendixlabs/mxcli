// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// ako/mxcli#2590: `rename module` renamed the qualified names stored on their
// own, and a retrieve by association, but not one inside an XPath constraint
// or an expression: `[OldMod.Contract_Company = $Company]` kept the old module
// and mx check reported CE1613. The expression rewriter took a qualified name
// only, and a module rename hands it a bare module name.
func TestRenameModuleRewritesXPathAndExpressions2590(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = `mdl 1;
create module OldMod;
create persistent entity OldMod.Company (Name: String(200));
create persistent entity OldMod.Contract (Number: Integer, Code: String(20));
create association OldMod.Contract_Company from OldMod.Contract to OldMod.Company;
create constant OldMod.Prefix type String default 'OldMod.';

create microflow OldMod.ACT_contracts_of_company ($Company: OldMod.Company)
begin
  retrieve $Contracts from OldMod.Contract
    where [OldMod.Contract_Company = $Company];
  retrieve $Contract from OldMod.Contract
    where [Number = 1]
    [not(OldMod.Contract_Company = $Company)]
    first;
  declare $Label string = @OldMod.Prefix + 'OldMod.Contract';
  return;
end;`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if err := agreeExec(t, exec, "mdl 1;\nrename module OldMod to NewMod;"); err != nil {
		t.Fatalf("rename: %v\n%s", err, out.String())
	}

	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	err := afRun(t, exec, "describe microflow NewMod.ACT_contracts_of_company;")
	exec.output = saved
	if err != nil {
		t.Fatal(err)
	}
	desc := buf.String()
	for _, want := range []string{
		"NewMod.Contract_Company = $Company",
		"not(NewMod.Contract_Company = $Company)",
		"@NewMod.Prefix",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("after rename module the flow does not hold %q:\n%s", want, desc)
		}
	}
	// A string literal is data: the text 'OldMod.Contract' stays as written.
	if !strings.Contains(desc, "'OldMod.Contract'") {
		t.Errorf("the string literal was rewritten:\n%s", desc)
	}
	if n := strings.Count(desc, "OldMod."); n != 1 {
		t.Errorf("the flow still names the old module %d time(s) outside the literal:\n%s", n-1, desc)
	}
}
