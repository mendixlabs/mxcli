// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2015 (mendixlabs/mxcli#1775): `drop user role Administrator` on a
// fresh app dropped the role Security$ProjectSecurity.AdminUserRole names.
// check, exec and `mx check` all passed, and every build then crashed with a
// NullReferenceException in mxbuild's metadata writer. MDL had no way to move
// the administrator to another role first, and nothing printed which role it
// had. Measured by the reporter on 11.12.6: the dangling name is the whole
// cause; moving AdminUserRole to another role, then dropping, builds.

func adminUserRoleOf(t *testing.T, exec *Executor) string {
	t.Helper()
	ps, err := exec.newExecContext(t.Context()).Backend.GetProjectSecurity()
	if err != nil {
		t.Fatal(err)
	}
	return ps.AdminUserRole
}

func userRoleExists(t *testing.T, exec *Executor, name string) bool {
	t.Helper()
	ps, err := exec.newExecContext(t.Context()).Backend.GetProjectSecurity()
	if err != nil {
		t.Fatal(err)
	}
	for _, ur := range ps.UserRoles {
		if ur.Name == name {
			return true
		}
	}
	return false
}

func TestDropUserRole_RefusesTheAdministratorsRole(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	if got := adminUserRoleOf(t, exec); got != "Administrator" {
		t.Fatalf("control: PedApp's AdminUserRole = %q, want Administrator", got)
	}
	assertAgree(t, exec, dir, "mdl 1;\ndrop user role Administrator;", "administrator's user role")
	if !userRoleExists(t, exec, "Administrator") {
		t.Error("the refused drop removed the role anyway")
	}

	// Control: a role nobody's administrator role names drops as before.
	assertAgree(t, exec, dir, "mdl 1;\ncreate user role Spare (MyFirstModule.User);\ndrop user role Spare;", "")
}

// Moving the administrator first is the clean order, in one script: check
// follows the move, so it does not refuse the drop after it.
func TestAlterAppSecurity_AdminUserRoleThenDrop(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	src := "mdl 1;\ncreate user role AppAdmin (MyFirstModule.User);\n" +
		"alter app security ( AdminUserRole: AppAdmin );\n" +
		"drop demo user 'demo_administrator';\n" +
		"drop user role Administrator;"
	assertAgree(t, exec, dir, src, "")
	if got := adminUserRoleOf(t, exec); got != "AppAdmin" {
		t.Errorf("stored AdminUserRole = %q, want AppAdmin\n%s", got, out.String())
	}
	if userRoleExists(t, exec, "Administrator") {
		t.Error("the drop after the move did not happen")
	}

	// The role it names is printed.
	out.Reset()
	if err := afRun(t, exec, "describe app security;"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Admin User Role: AppAdmin") {
		t.Errorf("describe app security does not print the admin user role:\n%s", out.String())
	}
}

// A role that does not exist is refused: mxbuild crashes on a name that does
// not resolve instead of reporting it.
func TestAlterAppSecurity_AdminUserRoleMustExist(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, "mdl 1;\nalter app security ( AdminUserRole: NoSuchRole );", "NoSuchRole")
	if got := adminUserRoleOf(t, exec); got != "Administrator" {
		t.Errorf("the refused alter changed AdminUserRole to %q", got)
	}
	// Stored under the role's declared casing.
	assertAgree(t, exec, dir, "mdl 1;\nalter app security ( AdminUserRole: user );", "")
	if got := adminUserRoleOf(t, exec); got != "User" {
		t.Errorf("AdminUserRole = %q, want the declared casing User", got)
	}
}
