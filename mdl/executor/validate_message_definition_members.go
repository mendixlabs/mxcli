// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// messageDefinitionMemberErrors reports the attribute members exec would refuse
// with "<entity> has no attribute …" (buildMessageMember). check --references
// used to pass them, so a describe that printed a renamed attribute's old name
// was "Check passed!" and then failed on exec (mendixlabs/mxcli#1733).
//
// It uses the DataGrid column judge, which answers only "provably not an
// attribute": an entity the script alters, imports or declares without its
// members is left alone, so a script that adds the attribute first still passes.
func messageDefinitionMemberErrors(ctx *ExecContext, sc *scriptContext, label string, defs ...*ast.MessageDefinitionDef) error {
	missing := newColumnAttrJudge(ctx, sc)
	var errs []string
	var walk func(holder, where string, members []*ast.MessageMemberDef)
	walk = func(holder, where string, members []*ast.MessageMemberDef) {
		for _, m := range members {
			if m == nil {
				continue
			}
			if m.Association.Name != "" {
				walk(m.Entity.String(), where, m.Members)
				continue
			}
			if missing(holder, m.Attribute) {
				errs = append(errs, fmt.Sprintf("%s: %s has no attribute %q%s",
					where, holder, m.Attribute, availableAttributes(ctx, holder)))
			}
		}
	}
	for _, d := range defs {
		if d == nil {
			continue
		}
		where := label
		if d.Name != "" {
			where = label + "." + d.Name
		}
		walk(d.Entity.String(), "message definition "+where, d.Members)
	}
	if len(errs) == 0 {
		return nil
	}
	return mdlerrors.NewValidation(strings.Join(errs, "\n  - "))
}
