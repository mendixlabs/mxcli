// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

func TestDomainModelSizeRule_NoViolation(t *testing.T) {
	var entities [][]any
	for i := 0; i < 10; i++ {
		entities = append(entities, []any{
			fmt.Sprintf("id%d", i), fmt.Sprintf("Entity%d", i),
			fmt.Sprintf("MyModule.Entity%d", i), "MyModule", "",
			"PERSISTENT", "", "", 5, 1, 0, 0, 0,
		})
	}

	db := setupEntitiesDB(t, entities)
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewDomainModelSizeRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for 10 entities, got %d", len(violations))
	}
}

func TestDomainModelSizeRule_ExceedsThreshold(t *testing.T) {
	var entities [][]any
	for i := 0; i < 20; i++ {
		entities = append(entities, []any{
			fmt.Sprintf("id%d", i), fmt.Sprintf("Entity%d", i),
			fmt.Sprintf("BigModule.Entity%d", i), "BigModule", "",
			"PERSISTENT", "", "", 3, 1, 0, 0, 0,
		})
	}

	db := setupEntitiesDB(t, entities)
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewDomainModelSizeRule()
	violations := rule.Check(ctx)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "MPR003" {
		t.Errorf("expected rule ID MPR003, got %s", violations[0].RuleID)
	}
}

func TestDomainModelSizeRule_NonPersistentIgnored(t *testing.T) {
	var entities [][]any
	for i := 0; i < 20; i++ {
		entities = append(entities, []any{
			fmt.Sprintf("id%d", i), fmt.Sprintf("Entity%d", i),
			fmt.Sprintf("MyModule.Entity%d", i), "MyModule", "",
			"NON_PERSISTENT", "", "", 3, 0, 0, 0, 0,
		})
	}

	db := setupEntitiesDB(t, entities)
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewDomainModelSizeRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for non-persistent entities, got %d", len(violations))
	}
}

func TestDomainModelSizeRule_ExactlyAtThreshold(t *testing.T) {
	var entities [][]any
	for i := 0; i < DefaultMaxPersistentEntities; i++ {
		entities = append(entities, []any{
			fmt.Sprintf("id%d", i), fmt.Sprintf("Entity%d", i),
			fmt.Sprintf("MyModule.Entity%d", i), "MyModule", "",
			"PERSISTENT", "", "", 3, 1, 0, 0, 0,
		})
	}

	db := setupEntitiesDB(t, entities)
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewDomainModelSizeRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations at threshold (%d entities), got %d", DefaultMaxPersistentEntities, len(violations))
	}
}

func TestDomainModelSizeRule_OneOverThreshold(t *testing.T) {
	count := DefaultMaxPersistentEntities + 1
	var entities [][]any
	for i := 0; i < count; i++ {
		entities = append(entities, []any{
			fmt.Sprintf("id%d", i), fmt.Sprintf("Entity%d", i),
			fmt.Sprintf("MyModule.Entity%d", i), "MyModule", "",
			"PERSISTENT", "", "", 3, 1, 0, 0, 0,
		})
	}

	db := setupEntitiesDB(t, entities)
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewDomainModelSizeRule()
	violations := rule.Check(ctx)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation at %d entities, got %d", count, len(violations))
	}
}

func TestDomainModelSizeRule_Configure_MaxEntitiesInt(t *testing.T) {
	rule := NewDomainModelSizeRule()
	rule.Configure(map[string]any{"max_entities": 25})
	if rule.MaxPersistentEntities != 25 {
		t.Errorf("MaxPersistentEntities = %d, want 25", rule.MaxPersistentEntities)
	}
}

func TestDomainModelSizeRule_Configure_MaxEntitiesFloat(t *testing.T) {
	rule := NewDomainModelSizeRule()
	// YAML may unmarshal integers as float64.
	rule.Configure(map[string]any{"max_entities": float64(30)})
	if rule.MaxPersistentEntities != 30 {
		t.Errorf("MaxPersistentEntities = %d, want 30", rule.MaxPersistentEntities)
	}
}

func TestDomainModelSizeRule_Configure_UnknownKeyIgnored(t *testing.T) {
	rule := NewDomainModelSizeRule()
	rule.Configure(map[string]any{"unknown_key": "value"})
	if rule.MaxPersistentEntities != DefaultMaxPersistentEntities {
		t.Errorf("unexpected change: MaxPersistentEntities = %d", rule.MaxPersistentEntities)
	}
}

func TestDomainModelSizeRule_Configure_RaisesThreshold(t *testing.T) {
	// With 20 entities and default threshold 15 there is a violation;
	// raising threshold to 25 should eliminate it.
	var entities [][]any
	for i := 0; i < 20; i++ {
		entities = append(entities, []any{
			fmt.Sprintf("id%d", i), fmt.Sprintf("Entity%d", i),
			fmt.Sprintf("BigModule.Entity%d", i), "BigModule", "",
			"PERSISTENT", "", "", 3, 1, 0, 0, 0,
		})
	}
	db := setupEntitiesDB(t, entities)
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewDomainModelSizeRule()
	rule.Configure(map[string]any{"max_entities": 25})
	violations := rule.Check(ctx)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations with raised threshold, got %d", len(violations))
	}
}

func TestDomainModelSizeRule_Metadata(t *testing.T) {
	r := NewDomainModelSizeRule()
	if r.ID() != "MPR003" {
		t.Errorf("ID = %q, want MPR003", r.ID())
	}
	if r.Category() != "design" {
		t.Errorf("Category = %q, want design", r.Category())
	}
	if r.MaxPersistentEntities != DefaultMaxPersistentEntities {
		t.Errorf("MaxPersistentEntities = %d, want %d", r.MaxPersistentEntities, DefaultMaxPersistentEntities)
	}
}
