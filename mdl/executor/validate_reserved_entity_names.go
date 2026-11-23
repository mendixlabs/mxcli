// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateReservedEntityName refuses an entity whose name the platform
// reserves: mxbuild reports CE7247 "The name 'Owner' is a reserved word." at
// the entity (ako/mxcli#2531). Measured on 11.12.6, one entity each: every
// word of types.PlatformReservedWords and SystemAttributeNames tried —
// CreatedDate, ChangedDate and ChangedBy included, on a persistent entity —
// is CE7247, on a non-persistent entity too; String, User, System and List
// build. MDL021 covered attribute names only. Quoting is no way out: the
// platform checks the bare name.
func validateReservedEntityName(stmt ast.Statement) []linter.Violation {
	var qn ast.QualifiedName
	name := ""
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		qn, name = s.Name, s.Name.Name
	case *ast.CreateViewEntityStmt:
		qn, name = s.Name, s.Name.Name
	case *ast.RenameStmt:
		if s.ObjectType != "entity" {
			return nil
		}
		qn, name = s.Name, s.NewName
	default:
		return nil
	}
	if !types.IsPlatformReserved(name) {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL021",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf("entity name '%s' is a reserved word — mxbuild reports CE7247 "+
			"\"The name '%s' is a reserved word.\" at the entity, quoted or not", name, name),
		Location:   linter.Location{Module: qn.Module, DocumentType: "entity", DocumentName: qn.Name},
		Suggestion: fmt.Sprintf("Rename the entity, e.g. '%sRecord' or '%sItem'", name, name),
	}}
}

// refuseReservedEntityName is the same verdict in the exec handlers, so a
// script run with --no-check cannot write what check refuses.
func refuseReservedEntityName(stmt ast.Statement) error {
	if v := firstBlockingViolation(validateReservedEntityName(stmt)); v != nil {
		return mdlerrors.NewValidationf("%s — %s", v.Message, v.Suggestion)
	}
	return nil
}
