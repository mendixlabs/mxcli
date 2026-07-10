// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// DefaultMaxPersistentEntities is the default maximum number of persistent entities per module.
const DefaultMaxPersistentEntities = 15

// DomainModelSizeRule checks that modules don't have too many persistent entities.
type DomainModelSizeRule struct {
	MaxPersistentEntities int
}

// NewDomainModelSizeRule creates a new domain model size rule with default thresholds.
func NewDomainModelSizeRule() *DomainModelSizeRule {
	return &DomainModelSizeRule{
		MaxPersistentEntities: DefaultMaxPersistentEntities,
	}
}

func (r *DomainModelSizeRule) ID() string                       { return "MPR003" }
func (r *DomainModelSizeRule) Name() string                     { return "DomainModelSize" }
func (r *DomainModelSizeRule) Category() string                 { return "design" }
func (r *DomainModelSizeRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *DomainModelSizeRule) Description() string {
	return fmt.Sprintf("Checks that modules have no more than %d persistent entities", r.MaxPersistentEntities)
}

// Configure applies options from the lint config file.
// Supported option: max_entities (int) — override DefaultMaxPersistentEntities.
func (r *DomainModelSizeRule) Configure(options map[string]any) {
	if v, ok := options["max_entities"]; ok {
		switch n := v.(type) {
		case int:
			r.MaxPersistentEntities = n
		case float64:
			r.MaxPersistentEntities = int(n)
		}
	}
}

// Check counts persistent entities per module and flags those exceeding the limit.
func (r *DomainModelSizeRule) Check(ctx *linter.LintContext) []linter.Violation {
	// Count persistent entities per module
	counts := make(map[string]int)
	for entity := range ctx.Entities() {
		if entity.EntityType == "Persistent" {
			counts[entity.ModuleName]++
		}
	}

	var violations []linter.Violation
	for moduleName, count := range counts {
		if count > r.MaxPersistentEntities {
			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message: fmt.Sprintf("Module '%s' has %d persistent entities (max %d). Consider splitting into smaller modules.",
					moduleName, count, r.MaxPersistentEntities),
				Location: linter.Location{
					Module:       moduleName,
					DocumentType: "domainmodel",
					DocumentName: "DomainModel",
				},
				Suggestion: fmt.Sprintf("Split module '%s' into smaller modules with focused domain models", moduleName),
			})
		}
	}

	return violations
}
