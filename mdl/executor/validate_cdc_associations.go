// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// cdcAssociationRule refuses a change data capture service mxbuild rejects
// with CE1583, "For an association to function correctly in data to capture,
// the associated entity … or its specialization must also be included"
// (ako/mxcli#2613). Measured on 11.17.0, two shapes:
//
//   - a tracked association whose entity at the other end is not a source of
//     the service (reported at the association property);
//   - an association both sides own (owner Both) tracked from one side only:
//     Studio Pro's CDC editor offers it on both entities (an entity is offered
//     every association it owns), and mxbuild reports CE1583 at the other
//     entity's change source until that side tracks it too.
//
// check and exec passed both and mx check failed. An error, as mxbuild's is.
const cdcAssociationRule = "MDL-CDC01"

type cdcAssocEnds struct {
	name     string            // Module.Name as written
	from, to string            // Module.Entity, lower-cased
	display  map[string]string // lower-cased end -> as spelled
	both     bool
}

// CheckCdcAssociations reports MDL-CDC01 for the script.
func (e *Executor) CheckCdcAssociations(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	return CheckCdcAssociations(e.newExecContext(context.Background()), prog)
}

// CheckCdcAssociations checks every tracked association of every change data
// capture service the script writes against the associations the project has
// and the script creates.
func CheckCdcAssociations(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || prog == nil || !ctx.Connected() {
		return nil
	}
	var svcs []*ast.CreateChangeDataCaptureServiceStmt
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateChangeDataCaptureServiceStmt); ok {
			svcs = append(svcs, s)
		}
	}
	if len(svcs) == 0 {
		return nil
	}
	assocs := cdcKnownAssociations(ctx, prog)

	var out []linter.Violation
	for _, s := range svcs {
		// Each source's tracked members, by entity.
		tracked := map[string]map[string]bool{}
		for _, src := range s.Sources {
			m := map[string]bool{}
			for _, mem := range src.Members {
				m[strings.ToLower(mem.Name)] = true
			}
			tracked[strings.ToLower(src.Entity.String())] = m
		}
		for _, src := range s.Sources {
			ent := strings.ToLower(src.Entity.String())
			for _, mem := range src.Members {
				a, ok := assocs[strings.ToLower(src.Entity.Module+"."+mem.Name)]
				if !ok {
					continue
				}
				var other string
				switch {
				case a.from == ent:
					other = a.to
				case a.to == ent && a.both:
					other = a.from
				default:
					continue
				}
				otherMembers, otherTracked := tracked[other]
				otherName := cdcEntityDisplay(s, other, a.display[other])
				switch {
				case !otherTracked:
					out = append(out, cdcAssocViolation(s, fmt.Sprintf(
						"change data capture service %s tracks association %s from %s, but not the entity at its other end, %s: mxbuild refuses it (CE1583)",
						s.Name, a.name, src.Entity, otherName),
						fmt.Sprintf("Add `entity %s ( … )` to the service.", otherName)))
				case a.both && !otherMembers[strings.ToLower(mem.Name)] && !otherMembers[strings.ToLower(a.name[strings.Index(a.name, ".")+1:])]:
					out = append(out, cdcAssocViolation(s, fmt.Sprintf(
						"change data capture service %s tracks association %s (owner Both) from %s only: both sides own it, and mxbuild refuses it until %s tracks it too (CE1583)",
						s.Name, a.name, src.Entity, otherName),
						fmt.Sprintf("Add %s to `entity %s ( … )`, or make the association owner Default.",
							a.name[strings.Index(a.name, ".")+1:], otherName)))
				}
			}
		}
	}
	return out
}

func cdcAssocViolation(s *ast.CreateChangeDataCaptureServiceStmt, msg, fix string) linter.Violation {
	return linter.Violation{
		RuleID:     cdcAssociationRule,
		Severity:   linter.SeverityError,
		Message:    msg,
		Location:   linter.Location{DocumentType: "change data capture service", DocumentName: s.Name.String()},
		Suggestion: fix,
	}
}

// cdcEntityDisplay is the entity as the script spells it, or the lower-cased
// key when the script does not name it.
func cdcEntityDisplay(s *ast.CreateChangeDataCaptureServiceStmt, key, spelled string) string {
	for _, src := range s.Sources {
		if strings.EqualFold(src.Entity.String(), key) {
			return src.Entity.String()
		}
	}
	if spelled != "" {
		return spelled
	}
	return key
}

// cdcKnownAssociations is every association by lower-cased Module.Name: the
// project's, then the script's own, which win.
func cdcKnownAssociations(ctx *ExecContext, prog *ast.Program) map[string]cdcAssocEnds {
	out := map[string]cdcAssocEnds{}
	if dms, err := ctx.Backend.ListDomainModels(); err == nil {
		h, _ := getHierarchy(ctx)
		for _, dm := range dms {
			mod := ""
			if h != nil {
				mod = h.GetModuleName(dm.ContainerID)
			}
			if mod == "" {
				continue
			}
			names := map[string]string{}
			for _, e := range dm.Entities {
				names[string(e.ID)] = mod + "." + e.Name
			}
			for _, a := range dm.Associations {
				out[strings.ToLower(mod+"."+a.Name)] = newCdcAssocEnds(mod+"."+a.Name,
					names[string(a.ParentID)], names[string(a.ChildID)], a.Owner == domainmodel.AssociationOwnerBoth)
			}
		}
	}
	for _, stmt := range prog.Statements {
		if a, ok := stmt.(*ast.CreateAssociationStmt); ok {
			out[strings.ToLower(a.Name.String())] = newCdcAssocEnds(a.Name.String(), a.Parent.String(), a.Child.String(),
				a.Owner == ast.OwnerBoth)
		}
	}
	return out
}

func newCdcAssocEnds(name, from, to string, both bool) cdcAssocEnds {
	return cdcAssocEnds{name: name, from: strings.ToLower(from), to: strings.ToLower(to), both: both,
		display: map[string]string{strings.ToLower(from): from, strings.ToLower(to): to}}
}
