// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
)

// --- SEC001: NoEntityAccessRules ---

// NoEntityAccessRulesRule flags persistent entities with no access rules defined.
type NoEntityAccessRulesRule struct{}

func NewNoEntityAccessRulesRule() *NoEntityAccessRulesRule { return &NoEntityAccessRulesRule{} }

func (r *NoEntityAccessRulesRule) ID() string                       { return "SEC001" }
func (r *NoEntityAccessRulesRule) Name() string                     { return "NoEntityAccessRules" }
func (r *NoEntityAccessRulesRule) Category() string                 { return "security" }
func (r *NoEntityAccessRulesRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *NoEntityAccessRulesRule) Description() string {
	return "Checks that persistent entities have at least one access rule"
}

func (r *NoEntityAccessRulesRule) Check(ctx *linter.LintContext) []linter.Violation {
	var violations []linter.Violation

	for e := range ctx.Entities() {
		if e.EntityType != "Persistent" || e.IsExternal || e.AccessRuleCount > 0 {
			continue
		}
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message:  fmt.Sprintf("Persistent entity '%s' has no access rules", e.QualifiedName),
			Location: linter.Location{
				Module:       e.ModuleName,
				DocumentType: "entity",
				DocumentName: e.Name,
				DocumentID:   e.ID,
			},
			Suggestion: fmt.Sprintf("GRANT <Role> ON %s (READ *)", e.QualifiedName),
		})
	}

	return violations
}

// --- SEC002: WeakPasswordPolicy ---

// WeakPasswordPolicyRule flags projects where password minimum length is below 8.
type WeakPasswordPolicyRule struct{}

func NewWeakPasswordPolicyRule() *WeakPasswordPolicyRule { return &WeakPasswordPolicyRule{} }

func (r *WeakPasswordPolicyRule) ID() string                       { return "SEC002" }
func (r *WeakPasswordPolicyRule) Name() string                     { return "WeakPasswordPolicy" }
func (r *WeakPasswordPolicyRule) Category() string                 { return "security" }
func (r *WeakPasswordPolicyRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *WeakPasswordPolicyRule) Description() string {
	return "Checks that the password policy requires at least 8 characters"
}

func (r *WeakPasswordPolicyRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil {
		return nil
	}

	if ps.PasswordPolicy == nil || ps.PasswordPolicy.MinimumLength >= 8 {
		return nil
	}

	return []linter.Violation{{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message: fmt.Sprintf("Password policy minimum length is %d (recommended: 8 or more)",
			ps.PasswordPolicy.MinimumLength),
		Location: linter.Location{
			DocumentType: "security",
			DocumentName: "ProjectSecurity",
		},
		Suggestion: "ALTER PROJECT SECURITY PASSWORD POLICY MINIMUM LENGTH 8",
	}}
}

// --- SEC003: DemoUsersActive ---

// DemoUsersActiveRule flags projects that have demo users enabled at production security level.
type DemoUsersActiveRule struct{}

func NewDemoUsersActiveRule() *DemoUsersActiveRule { return &DemoUsersActiveRule{} }

func (r *DemoUsersActiveRule) ID() string                       { return "SEC003" }
func (r *DemoUsersActiveRule) Name() string                     { return "DemoUsersActive" }
func (r *DemoUsersActiveRule) Category() string                 { return "security" }
func (r *DemoUsersActiveRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *DemoUsersActiveRule) Description() string {
	return "Checks that demo users are disabled when security level is Production"
}

func (r *DemoUsersActiveRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil {
		return nil
	}

	if !ps.EnableDemoUsers || ps.SecurityLevel != security.SecurityLevelProduction {
		return nil
	}

	return []linter.Violation{{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message:  "Demo users are enabled at Production security level",
		Location: linter.Location{
			DocumentType: "security",
			DocumentName: "ProjectSecurity",
		},
		Suggestion: "ALTER PROJECT SECURITY DEMO USERS OFF",
	}}
}
