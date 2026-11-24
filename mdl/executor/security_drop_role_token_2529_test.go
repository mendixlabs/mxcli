// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#2529 (mendixlabs/mxcli#1924): a user role named in an XPath by its
// token, `'[%UserRole_Spare%]'`, was invisible to `drop user role`: check and
// exec passed, and mx check then reported CE1613 "The selected user role
// 'Spare' no longer exists." at the constraint that names it.
func TestDropUserRole_RefusesARoleAnXPathTokenNames_2529(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const setup = "mdl 1;\ncreate user role Spare ( ModuleRoles: (MyFirstModule.User) );\n" +
		"create microflow MyFirstModule.ACT_Spares ()\nbegin\n" +
		"  retrieve $L from System.User where System.UserRoles = '[%UserRole_Spare%]';\nend;\n"
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, "mdl 1;\ndrop user role Spare;", "MyFirstModule.ACT_Spares")
	if !userRoleExists(t, exec, "Spare") {
		t.Error("the refused drop removed the role anyway")
	}
	// The clean order: remove the constraint's document, then the role.
	assertAgree(t, exec, dir, "mdl 1;\ndrop microflow MyFirstModule.ACT_Spares;\ndrop user role Spare;", "")
	if userRoleExists(t, exec, "Spare") {
		t.Error("the drop after the microflow's did not happen")
	}
}

// Control: a role no token names drops; a token naming a role with a longer
// name that shares its prefix does not hold it.
func TestDropUserRole_TokenOfAnotherRoleDoesNotHold_2529(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const setup = "mdl 1;\ncreate user role Spare ( ModuleRoles: (MyFirstModule.User) );\n" +
		"create user role SpareTwo ( ModuleRoles: (MyFirstModule.User) );\n" +
		"create microflow MyFirstModule.ACT_Spares ()\nbegin\n" +
		"  retrieve $L from System.User where System.UserRoles = '[%UserRole_SpareTwo%]';\nend;\n"
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, "mdl 1;\ndrop user role Spare;", "")
}
