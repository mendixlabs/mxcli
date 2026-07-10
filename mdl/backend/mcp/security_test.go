// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"os"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestBuildAccessRuleValue(t *testing.T) {
	v := buildAccessRuleValue(backend.EntityAccessRuleParams{
		EntityName:          "Expense",
		RoleNames:           []string{"ExpenseApproval.Manager"},
		AllowCreate:         true,
		AllowDelete:         true,
		DefaultMemberAccess: "ReadWrite",
		MemberAccesses: []types.EntityMemberAccess{
			{AttributeRef: "ExpenseApproval.Expense.Title", AccessRights: "ReadWrite"},
			{AssociationRef: "ExpenseApproval.Expense_Employee", AccessRights: "ReadOnly"},
		},
	})

	if v["$Type"] != "DomainModels$AccessRule" {
		t.Fatalf("$Type = %v", v["$Type"])
	}
	if v["defaultMemberAccessRights"] != "ReadWrite" || v["allowCreate"] != true || v["allowDelete"] != true {
		t.Fatalf("rule leaves = %#v", v)
	}
	if _, hasXPath := v["xPathConstraint"]; hasXPath {
		t.Fatal("empty xPathConstraint should be omitted")
	}
	mas := v["memberAccesses"].([]any)
	if len(mas) != 2 {
		t.Fatalf("memberAccesses len = %d", len(mas))
	}
	attr := mas[0].(map[string]any)
	if attr["attribute"] != "ExpenseApproval.Expense.Title" || attr["accessRights"] != "ReadWrite" {
		t.Fatalf("attr member = %#v", attr)
	}
	if _, hasAssoc := attr["association"]; hasAssoc {
		t.Fatal("empty association ref must be omitted (PED rejects empty references)")
	}
	assoc := mas[1].(map[string]any)
	if assoc["association"] != "ExpenseApproval.Expense_Employee" {
		t.Fatalf("assoc member = %#v", assoc)
	}
	if _, hasAttr := assoc["attribute"]; hasAttr {
		t.Fatal("empty attribute ref must be omitted")
	}
}

func TestBuildAccessRuleValue_Defaults(t *testing.T) {
	v := buildAccessRuleValue(backend.EntityAccessRuleParams{RoleNames: []string{"M.R"}})
	if v["defaultMemberAccessRights"] != "None" {
		t.Fatalf("default rights should fall back to None, got %v", v["defaultMemberAccessRights"])
	}
	if _, has := v["memberAccesses"]; has {
		t.Fatal("no member accesses -> key omitted")
	}
}

func TestRoleSetsEqual(t *testing.T) {
	if !roleSetsEqual(normalizeRoleSet([]string{"A.x", "B.y"}), normalizeRoleSet([]string{"B.y", "A.x"})) {
		t.Fatal("order-independent equality failed")
	}
	if roleSetsEqual(normalizeRoleSet([]string{"A.x"}), normalizeRoleSet([]string{"A.x", "B.y"})) {
		t.Fatal("different sizes must not be equal")
	}
}

// TestLive_EntityAccessRuleReject exercises the live entity-access read + dedup
// path WITHOUT writing: it grants a role that already exists on an entity, which
// must hit the "already exists" rejection (PED is add-only for access rules, so a
// real write here would not be removable). Skipped unless MXCLI_MCP_URL is set.
//
//	MXCLI_MCP_URL=http://localhost/mcp MXCLI_MCP_DIAL=host.docker.internal:7784 \
//	MXCLI_MCP_MODULE=ExpenseApproval MXCLI_MCP_ENTITY=Expense MXCLI_MCP_ROLE=ExpenseApproval.Manager \
//	go test ./mdl/backend/mcp/ -run TestLive_EntityAccessRuleReject -v
func TestLive_EntityAccessRuleReject(t *testing.T) {
	url := os.Getenv("MXCLI_MCP_URL")
	if url == "" {
		t.Skip("set MXCLI_MCP_URL to run the live MCP integration test")
	}
	module := envOr("MXCLI_MCP_MODULE", "ExpenseApproval")
	entity := envOr("MXCLI_MCP_ENTITY", "Expense")
	role := envOr("MXCLI_MCP_ROLE", "ExpenseApproval.Manager")

	c, err := NewClient(ClientOptions{URL: url, Dial: os.Getenv("MXCLI_MCP_DIAL")})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	b := &Backend{client: c}

	idx, err := b.entityIndex(module, entity)
	if err != nil {
		t.Fatalf("entityIndex: %v", err)
	}
	sets, err := b.entityAccessRuleRoleSets(module, idx)
	if err != nil {
		t.Fatalf("entityAccessRuleRoleSets: %v", err)
	}
	t.Logf("%s.%s has %d access rule(s)", module, entity, len(sets))

	// Grant a role that already exists -> must be rejected, with no write.
	err = b.AddEntityAccessRule(backend.EntityAccessRuleParams{
		UnitID:              model.ID(sessionDMPrefix + module),
		EntityName:          entity,
		RoleNames:           []string{role},
		DefaultMemberAccess: "ReadOnly",
	})
	if err == nil {
		t.Fatal("expected AddEntityAccessRule to reject an existing role set, got nil (a rule may have been written and PED cannot remove it)")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected an 'already exists' rejection, got: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
