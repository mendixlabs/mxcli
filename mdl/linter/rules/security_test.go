// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

func TestNoEntityAccessRulesRule_NoViolation(t *testing.T) {
	db := setupEntitiesDB(t, [][]any{
		{"id1", "Customer", "MyModule.Customer", "MyModule", "", "PERSISTENT", "", "", 5, 2, 0, 0, 0},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewNoEntityAccessRulesRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestNoEntityAccessRulesRule_DetectsMissing(t *testing.T) {
	db := setupEntitiesDB(t, [][]any{
		{"id1", "Customer", "MyModule.Customer", "MyModule", "", "PERSISTENT", "", "", 5, 0, 0, 0, 0},
		{"id2", "Order", "MyModule.Order", "MyModule", "", "PERSISTENT", "", "", 3, 1, 0, 0, 0},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewNoEntityAccessRulesRule()
	violations := rule.Check(ctx)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "SEC001" {
		t.Errorf("expected rule ID SEC001, got %s", violations[0].RuleID)
	}
	if violations[0].Location.DocumentName != "Customer" {
		t.Errorf("expected document Customer, got %s", violations[0].Location.DocumentName)
	}
}

func TestNoEntityAccessRulesRule_NonPersistentIgnored(t *testing.T) {
	db := setupEntitiesDB(t, [][]any{
		{"id1", "TempObj", "MyModule.TempObj", "MyModule", "", "NON_PERSISTENT", "", "", 2, 0, 0, 0, 0},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewNoEntityAccessRulesRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for non-persistent entity, got %d", len(violations))
	}
}

func TestNoEntityAccessRulesRule_ExternalIgnored(t *testing.T) {
	db := setupEntitiesDB(t, [][]any{
		{"id1", "ExtEntity", "MyModule.ExtEntity", "MyModule", "", "PERSISTENT", "", "", 2, 0, 0, 0, 1},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewNoEntityAccessRulesRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for external entity, got %d", len(violations))
	}
}

func TestNoEntityAccessRulesRule_Metadata(t *testing.T) {
	r := NewNoEntityAccessRulesRule()
	if r.ID() != "SEC001" {
		t.Errorf("ID = %q, want SEC001", r.ID())
	}
	if r.Category() != "security" {
		t.Errorf("Category = %q, want security", r.Category())
	}
}

// NOTE: SEC002 and SEC003 require ctx.Reader() → *mpr.Reader to call GetProjectSecurity().
// Without a real MPR file, we can only test the nil-reader early return and metadata.
// Full behavioral coverage requires integration tests with a real .mpr project.

func TestWeakPasswordPolicyRule_NilReader(t *testing.T) {
	ctx := linter.NewLintContextFromDB(nil)
	rule := NewWeakPasswordPolicyRule()
	violations := rule.Check(ctx)

	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

// SEC003: Demo users still active in production
// Without a real MPR file, we can only test the nil-reader early return and metadata.

func TestDemoUsersActiveRule_NilReader(t *testing.T) {
	r := NewDemoUsersActiveRule()
	ctx := linter.NewLintContextFromDB(nil)
	violations := r.Check(ctx)
	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

func TestDemoUsersActiveRule_Metadata(t *testing.T) {
	r := NewDemoUsersActiveRule()
	if r.ID() != "SEC003" {
		t.Errorf("ID = %q, want SEC003", r.ID())
	}
	if r.Category() != "security" {
		t.Errorf("Category = %q, want security", r.Category())
	}
}
