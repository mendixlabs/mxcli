// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// CREATE OR MODIFY DEMO USER on an existing user removed it and added it back,
// so it moved to the end of the list and every user after it shifted one
// index: `mxcli diff` showed DemoUsers[1].UserName, DemoUsers[2].Entity … as
// changed for users the statement never named (mendixlabs/mxcli#1701).
func TestCreateOrModifyDemoUser_ModifiesInPlace(t *testing.T) {
	ctx, _, ps := demoUserCtx(t,
		&security.DemoUser{UserName: "alice", Password: "Pa55word!", Entity: "Administration.Account", UserRoles: []string{"User"}},
		&security.DemoUser{UserName: "bob", Password: "Pa55word!", Entity: "Administration.Account", UserRoles: []string{"User"}},
		&security.DemoUser{UserName: "carol", Password: "Pa55word!", Entity: "Administration.Account", UserRoles: []string{"User"}},
	)
	alice := ps.DemoUsers[0]

	assertNoError(t, execCreateDemoUser(ctx, &ast.CreateDemoUserStmt{
		UserName: "alice", Password: "Pa55word!", UserRoles: []string{"Administrator"}, CreateOrModify: true,
	}))

	var order []string
	for _, du := range ps.DemoUsers {
		order = append(order, du.UserName)
	}
	if got := strings.Join(order, ","); got != "alice,bob,carol" {
		t.Errorf("modifying alice reordered the demo users: %s (#1701)", got)
	}
	if ps.DemoUsers[0] != alice {
		t.Errorf("alice was replaced by a new element instead of updated in place (#1701)")
	}
	// Control: the modification itself landed.
	if got := strings.Join(alice.UserRoles, ","); got != "User,Administrator" {
		t.Errorf("control: alice's roles = %s, want User,Administrator", got)
	}
}
