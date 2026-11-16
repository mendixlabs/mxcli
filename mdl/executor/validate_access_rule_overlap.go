// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// accessRuleOverlapRule is MDL-GRANT03 (ako/mxcli#2320): a role holds a rule
// with no XPath constraint on an entity and another one with a constraint.
//
// Access rules add up. A GRANT with a WHERE writes a rule of its own beside the
// role's existing rules — a rule is keyed by its roles and its constraint
// (#936) — so a constrained grant meant to narrow an unconstrained one leaves
// the unconstrained rule in place, and the role still reads every row. exec
// reported success, check passed and lint said nothing; only a diff showed it.
// The same holds the other way round: an unconstrained grant makes the role's
// constrained rules restrict nothing.
//
// It is a warning, not an error: a rule with a constraint beside one without is
// a valid model, and a role may hold both on purpose (a constraint granting
// write on its own rows beside a read on every row). The defect was saying
// nothing.
const accessRuleOverlapRule = "MDL-GRANT03"

// accessRuleOrigin says where a role's rule on an entity comes from, so the
// check without a project reports what the script does to itself and the check
// with one only what it does to the project's rules — each finding once.
type accessRuleOrigin int

const (
	ruleNone accessRuleOrigin = iota
	ruleStored
	ruleScript
)

// roleRules is what one role holds on one entity: an unconstrained rule and a
// constrained one, each by where it came from.
type roleRules struct {
	open, constrained accessRuleOrigin
}

// storedAccessRules answers, for an entity, the roles holding a rule with no
// constraint and those holding one with a constraint; ok is false where the
// project cannot say (no project, or no such entity).
type storedAccessRules func(entity string) (open, constrained map[string]bool, ok bool)

// ValidateAccessRuleOverlap reports MDL-GRANT03 for the overlaps the script
// makes with itself. It needs no project.
func ValidateAccessRuleOverlap(prog *ast.Program) []linter.Violation {
	return accessRuleOverlaps(prog, nil)
}

// CheckAccessRuleOverlap reports MDL-GRANT03 for the overlaps the script makes
// with the rules the project already holds.
func (e *Executor) CheckAccessRuleOverlap(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	ctx := e.newExecContext(context.Background())
	if !ctx.Connected() {
		return nil
	}
	return accessRuleOverlaps(prog, func(entity string) (map[string]bool, map[string]bool, bool) {
		e := lookupEntityForAccess(ctx, entity)
		if e == nil {
			return nil, nil, false
		}
		open, constrained := map[string]bool{}, map[string]bool{}
		for _, r := range e.AccessRules {
			for _, role := range r.ModuleRoleNames {
				if strings.TrimSpace(r.XPathConstraint) == "" {
					open[role] = true
				} else {
					constrained[role] = true
				}
			}
		}
		return open, constrained, true
	})
}

// accessRuleOverlaps walks the program in order, tracking per entity and role
// which rules are held and where they came from. stored is nil for the check
// without a project, which then reports script-made overlaps only; with it,
// only the overlaps a stored rule takes part in are reported.
func accessRuleOverlaps(prog *ast.Program, stored storedAccessRules) []linter.Violation {
	if prog == nil {
		return nil
	}
	state := map[string]map[string]*roleRules{}
	entityState := func(entity string) map[string]*roleRules {
		if m, ok := state[entity]; ok {
			return m
		}
		m := map[string]*roleRules{}
		if stored != nil {
			if open, constrained, ok := stored(entity); ok {
				for role := range open {
					rr := m[role]
					if rr == nil {
						rr = &roleRules{}
						m[role] = rr
					}
					rr.open = ruleStored
				}
				for role := range constrained {
					rr := m[role]
					if rr == nil {
						rr = &roleRules{}
						m[role] = rr
					}
					rr.constrained = ruleStored
				}
			}
		}
		state[entity] = m
		return m
	}
	report := func(a, b accessRuleOrigin) bool {
		if stored == nil {
			return a == ruleScript && b == ruleScript
		}
		return a == ruleStored || b == ruleStored
	}

	var out []linter.Violation
	for i, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.GrantEntityAccessStmt:
			entity := s.Entity.String()
			m := entityState(entity)
			constrained := strings.TrimSpace(s.XPathConstraint) != ""
			var hit []string
			for _, r := range s.Roles {
				role := r.String()
				rr := m[role]
				if rr == nil {
					rr = &roleRules{}
					m[role] = rr
				}
				if constrained {
					if rr.open != ruleNone && report(rr.open, ruleScript) {
						hit = append(hit, role)
					}
					rr.constrained = ruleScript
				} else {
					if rr.constrained != ruleNone && report(rr.constrained, ruleScript) {
						hit = append(hit, role)
					}
					rr.open = ruleScript
				}
			}
			if len(hit) > 0 {
				out = append(out, accessRuleOverlapViolation(i+1, entity, hit, constrained))
			}
		case *ast.RevokeEntityAccessStmt:
			if len(s.Rights) > 0 {
				continue // a partial revoke changes rights, not which rules there are
			}
			m := entityState(s.Entity.String())
			for _, r := range s.Roles {
				delete(m, r.String())
			}
		case *ast.DropEntityStmt:
			state[s.Name.String()] = map[string]*roleRules{}
		}
	}
	return out
}

func accessRuleOverlapViolation(stmt int, entity string, roles []string, constrained bool) linter.Violation {
	sort.Strings(roles)
	who := strings.Join(roles, ", ")
	if constrained {
		return linter.Violation{
			RuleID:   accessRuleOverlapRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("statement %d: %s already has an access rule on %s with no constraint; access rules add up, "+
				"so this grant's where clause narrows nothing — the role still reaches every %s", stmt, who, entity, entity),
			Suggestion: fmt.Sprintf("To narrow the access, remove the unconstrained rule first: "+
				"`revoke all on entity %s from %s;` before this grant.", entity, who),
		}
	}
	return linter.Violation{
		RuleID:   accessRuleOverlapRule,
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf("statement %d: %s also has an access rule on %s with a constraint; access rules add up, "+
			"so with this unconstrained grant that constraint restricts nothing", stmt, who, entity),
		Suggestion: fmt.Sprintf("If the constraint is meant to limit the role, grant with the where clause instead, "+
			"after `revoke all on entity %s from %s;`.", entity, who),
	}
}

// lookupEntityForAccess is the stored entity Module.Name, or nil where the
// project has no such module or entity.
func lookupEntityForAccess(ctx *ExecContext, qn string) *domainmodel.Entity {
	mod, name, ok := strings.Cut(qn, ".")
	if !ok {
		return nil
	}
	module, err := findModule(ctx, mod)
	if err != nil || module == nil {
		return nil
	}
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil || dm == nil {
		return nil
	}
	return dm.FindEntityByName(name)
}
