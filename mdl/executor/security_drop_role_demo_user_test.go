// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// mendixlabs/mxcli#1775 (follow-up to ako/mxcli#2015): `drop user role X`
// while a demo user still holds X passed check and exec, and left the demo user
// naming a role that does not exist — `mx check` then reports CE1613 "The
// selected user role 'X' no longer exists." at the demo user (measured on
// 11.12.6). A demo user's roles only grow (`create or modify demo user` adds),
// so the way out is to drop the demo user first; check follows that order
// within the script.

func TestDropUserRole_RefusesARoleADemoUserHolds(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	// PedApp's demo_user holds User.
	assertAgree(t, exec, dir, "mdl 1;\ndrop user role User;", "held by demo user 'demo_user'")
	if !userRoleExists(t, exec, "User") {
		t.Error("the refused drop removed the role anyway")
	}

	// The clean order: drop the demo user, then the role.
	assertAgree(t, exec, dir, "mdl 1;\ndrop demo user 'demo_user';\ndrop user role User;", "")
	if userRoleExists(t, exec, "User") {
		t.Error("the drop after the demo user's did not happen")
	}
}

// A demo user the script itself creates holds its roles for the statements
// after it; dropping it again in the script releases them.
func TestDropUserRole_FollowsTheScriptsDemoUsers(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	const setup = "mdl 1;\ncreate user role Spare ( ModuleRoles: (MyFirstModule.User) );\n" +
		"create demo user 'demo_spare' ( Password: 'Spare-Passw0rd-1', UserRoles: (Spare) );\n"
	assertAgree(t, exec, dir, setup+"drop user role Spare;", "held by demo user 'demo_spare'")

	exec, _, dir = openPedAppCopy(t)
	assertAgree(t, exec, dir, setup+"drop demo user 'demo_spare';\ndrop user role Spare;", "")
	if userRoleExists(t, exec, "Spare") {
		t.Error("the drop after the demo user's did not happen")
	}
}
