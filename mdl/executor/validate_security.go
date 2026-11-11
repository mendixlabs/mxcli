// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for security statements. The executor
// refuses these at write time; without a matching check-time rule the same
// script passes `mxcli check` and only fails once it is run against a project,
// which is exactly the round-trip `check` exists to avoid.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// auditMemberNames are the members Mendix stores as entity flags rather than
// attributes. They are reserved — mxcli rejects declaring an ordinary attribute
// under these names — so a GRANT naming one always means the audit member, and
// no project is needed to recognise it.
var auditMemberNames = map[string]bool{
	"createddate": true,
	"changeddate": true,
	"owner":       true,
	"changedby":   true,
}

// ValidateGrantEntityAccess checks a GRANT for member rights Mendix cannot
// store, without requiring a project connection.
//
//   - MDL-SEC01: an audit member (createdDate/changedDate/owner/changedBy) given
//     per-member rights that differ from the rule's default. Mendix keeps no
//     MemberAccess for these — an entity storing them checks clean with none,
//     and a rule that carries one fails the build with CE0066 — so their access
//     can only come from the rule's default. (issuetracker #20)
func ValidateGrantEntityAccess(stmt *ast.GrantEntityAccessStmt) []linter.Violation {
	if stmt == nil {
		return nil
	}
	loc := linter.Location{
		Module:       stmt.Entity.Module,
		DocumentType: "entity",
		DocumentName: stmt.Entity.Name,
	}

	// The rule's default: `write *` makes it ReadWrite, `read *` ReadOnly,
	// neither leaves it None. A named member matching that default is a no-op
	// and stays legal.
	defaultRights := "None"
	for _, r := range stmt.Rights {
		switch r.Type {
		case ast.EntityAccessWriteAll:
			defaultRights = "ReadWrite"
		case ast.EntityAccessReadAll:
			if defaultRights == "None" {
				defaultRights = "ReadOnly"
			}
		}
	}

	var out []linter.Violation
	flag := func(member, wanted, clause string) {
		if wanted == defaultRights {
			return
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-SEC01",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf(
				"grant on %s.%s gives the audit member %s per-member rights — Mendix stores no member access for audit members, and a rule that carries one fails the build with CE0066",
				stmt.Entity.Module, stmt.Entity.Name, member),
			Suggestion: fmt.Sprintf(
				"Drop %s from the %s (...) list and let `read *` / `write *` cover it, or change the rule's default.",
				member, clause),
		})
	}

	for _, r := range stmt.Rights {
		var wanted, clause string
		switch r.Type {
		case ast.EntityAccessReadMembers:
			wanted, clause = "ReadOnly", "read"
		case ast.EntityAccessWriteMembers:
			wanted, clause = "ReadWrite", "write"
		default:
			continue
		}
		for _, m := range r.Members {
			if auditMemberNames[strings.ToLower(strings.Trim(m, `"`))] {
				flag(m, wanted, clause)
			}
		}
	}
	// The constraint's no-project form rules (ako/mxcli#2099).
	out = append(out, accessRuleXPathFormViolations(stmt, loc)...)
	return out
}
