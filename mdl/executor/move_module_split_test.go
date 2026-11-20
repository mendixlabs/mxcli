// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#1836 (mendixlabs/mxcli#1687): splitting a module with MOVE left
// three things naming the old module, all of which mx check reports:
//   - the access-rule XPath `[Src.Order_Customer/Src.Customer/Name = 'x']` —
//     path steps, which the reference sweep left alone;
//   - the entity access rule's module role (an entity's rules may only use its
//     own module's roles);
//   - a nanoflow's grant, which was not mapped to the target's same-named role
//     as a microflow's is (CE0148).
//
// Enumeration and constant references inside expressions, also in the report,
// were already rewritten (#1671, #1674); the test keeps them covered.
func TestMoveModuleSplit_RewritesXPathsRolesAndNanoflowGrants(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	setup := `create module Src1836;
create module Dst1836;
create module role Src1836.User;
create module role Dst1836.User;
create enumeration Src1836.Kind (A 'A', B 'B');
create constant Src1836.Limit ( Type: Integer, DefaultValue: 5 );
create persistent entity Src1836.Customer (Name: String(50));
create persistent entity Src1836.Order (Number: String(20), K: enum Src1836.Kind);
create association Src1836.Order_Customer from Src1836.Order to Src1836.Customer;
grant read * on entity Src1836.Order to Src1836.User where [Src1836.Order_Customer/Src1836.Customer/Name = 'x'];
grant read * on entity Src1836.Customer to Src1836.User;
create microflow Src1836.MF ($O: Src1836.Order) returns Boolean
begin
  if $O/K = Src1836.Kind.A and @Src1836.Limit > 3 then
    return true;
  end if;
  return false;
end;
grant execute on microflow Src1836.MF to Src1836.User;
create nanoflow Src1836.NF () returns Boolean
begin
  return @Src1836.Limit > 1;
end;
grant execute on nanoflow Src1836.NF to Src1836.User;`
	if err := afRun(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	out.Reset()
	moves := `move entity Src1836.Customer to Dst1836;
move entity Src1836.Order to Dst1836;
move enumeration Src1836.Kind to Dst1836;
move constant Src1836.Limit to Dst1836;
move microflow Src1836.MF to Dst1836;
move nanoflow Src1836.NF to Dst1836;`
	if err := afRun(t, exec, moves); err != nil {
		t.Fatalf("move: %v\n%s", err, out.String())
	}

	describe := func(stmt string) string {
		t.Helper()
		out.Reset()
		if err := afRun(t, exec, stmt); err != nil {
			t.Fatalf("%s: %v\n%s", stmt, err, out.String())
		}
		return out.String()
	}

	order := describe("describe entity Dst1836.Order;")
	if !strings.Contains(order, "where [Dst1836.Order_Customer/Dst1836.Customer/Name = 'x']") {
		t.Errorf("the access rule's XPath still names the old module:\n%s", order)
	}
	if !strings.Contains(order, "to Dst1836.User") || strings.Contains(order, "Src1836.User") {
		t.Errorf("the access rule keeps the old module's role:\n%s", order)
	}

	nf := describe("describe nanoflow Dst1836.NF;")
	if !strings.Contains(nf, "grant execute on nanoflow Dst1836.NF to Dst1836.User") {
		t.Errorf("the nanoflow's grant was not mapped to the target's role:\n%s", nf)
	}

	// Controls: the microflow's grant was always mapped, and its expressions
	// rewritten.
	mf := describe("describe microflow Dst1836.MF;")
	for _, want := range []string{"grant execute on microflow Dst1836.MF to Dst1836.User", "Dst1836.Kind.A", "@Dst1836.Limit"} {
		if !strings.Contains(mf, want) {
			t.Errorf("control: microflow lacks %q:\n%s", want, mf)
		}
	}
}

// A role with no same-named role in the target module is kept, and said: an
// access rule that silently lost its role would hide the entity from it.
func TestMoveEntity_WarnsOfARoleTheTargetLacks(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	setup := `create module SrcW1836;
create module DstW1836;
create module role SrcW1836.Clerk;
create persistent entity SrcW1836.Note (Text: String(50));
grant read * on entity SrcW1836.Note to SrcW1836.Clerk;`
	if err := afRun(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := afRun(t, exec, "move entity SrcW1836.Note to DstW1836;"); err != nil {
		t.Fatalf("move: %v\n%s", err, out.String())
	}
	if s := out.String(); !strings.Contains(s, "SrcW1836.Clerk") || !strings.Contains(s, "DstW1836") || !strings.Contains(strings.ToLower(s), "warning") {
		t.Errorf("no warning about the role the target lacks:\n%s", s)
	}
}
