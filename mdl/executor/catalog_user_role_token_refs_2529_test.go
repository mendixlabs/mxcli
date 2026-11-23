// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2529: `refs` / `impact` on a user role found nothing for the
// documents that name it by its XPath token, so the role read as unused.
// Each XPath constraint holding '[%UserRole_X%]' — a retrieve, an access rule
// — is now an `xpath` edge to USER_ROLE X; a token naming no role is not.
func TestCatalogRefsUserRoleTokens_2529(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = "mdl 1;\ncreate module T2529;\ncreate module role T2529.User;\n" +
		"create user role Spare ( ModuleRoles: (T2529.User) );\n" +
		"create persistent entity T2529.Account extends System.User ( Nick: String(50) );\n" +
		"grant T2529.User on T2529.Account (read *) where '[System.UserRoles = ''[%UserRole_Spare%]'']';\n" +
		"create microflow T2529.ACT_Spares ()\nbegin\n" +
		"  retrieve $L from System.User where System.UserRoles = '[%UserRole_Spare%]';\nend;\n"
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if err := exec.Execute(&ast.RefreshCatalogStmt{Full: true, Force: true}); err != nil {
		t.Fatalf("refresh catalog: %v", err)
	}
	res, err := exec.catalog.Query("select SourceType, SourceName, RefKind from refs where TargetType = 'USER_ROLE' and TargetName = 'Spare'")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range res.Rows {
		got = append(got, strings.Join([]string{refCell2529(r[0]), refCell2529(r[1]), refCell2529(r[2])}, " "))
	}
	sort.Strings(got)
	want := "ENTITY T2529.Account xpath|MICROFLOW T2529.ACT_Spares xpath" // an access rule is its entity's
	if strings.Join(got, "|") != want {
		t.Errorf("user role edges: %q, want %q", strings.Join(got, "|"), want)
	}
}

func refCell2529(v any) string {
	s, _ := v.(string)
	return s
}
