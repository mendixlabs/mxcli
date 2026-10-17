// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// declaredTypeResolver answers "does this type name exist" for the types a
// document DECLARES — a flow's parameters and return type, a page's or
// snippet's parameters — as opposed to the ones its body uses.
//
// The body was resolved (retrieve, create, a data source), and so were an
// association's endpoints (#555) and an EXTENDS target (#972); the signature
// never was. `create microflow M.F ($p: System.Nope)` passed
// `check --references`, as did `M.Nope` and a page parameter of either. exec
// refuses the user-module shapes and every page parameter after the statements
// before them are written; a System entity in a flow signature it writes by
// name (isBuiltinModuleEntity), leaving a reference nothing resolves.
//
// System is resolved like any module: the backend lists its virtual domain
// model and its enumerations, so System.User resolves and System.Nope does not.
// The sets are built once per statement, lazily — most signatures are primitive.
type declaredTypeResolver struct {
	ctx      *ExecContext
	sc       *scriptContext
	entities map[string]bool
	// listsSystem is whether the backend lists the System module at all.
	listsSystem bool
}

func newDeclaredTypeResolver(ctx *ExecContext, sc *scriptContext) *declaredTypeResolver {
	return &declaredTypeResolver{ctx: ctx, sc: sc}
}

func (r *declaredTypeResolver) entityExists(qn ast.QualifiedName) bool {
	if r.sc.producesEntity(qn) {
		return true
	}
	if r.entities == nil {
		r.entities = buildEntityQualifiedNames(r.ctx)
		for name := range r.entities {
			if strings.HasPrefix(name, "System.") {
				r.listsSystem = true
				break
			}
		}
	}
	if qn.Module == "System" && !r.listsSystem {
		// A backend that does not expose the System module cannot say a
		// System entity is missing — absence of the whole module is not
		// evidence about one name in it.
		return true
	}
	return r.entities[qn.String()]
}

func (r *declaredTypeResolver) enumExists(qn ast.QualifiedName) bool {
	return (r.sc != nil && r.sc.enumerations[qn.String()]) || enumerationExists(r.ctx, qn.String())
}

// check returns the problem with one declared type, or "" when it resolves or
// names nothing. where says whose type it is ("parameter $p of microflow M.F").
//
// A bare `Module.Name` is ambiguous — the parser cannot tell an entity from an
// enumeration and records either kind — so it resolves if either exists, as
// exec's own fallback does. Only the explicit `Enumeration(…)` / `enum …`
// spelling is held to enumerations alone.
func (r *declaredTypeResolver) check(dt ast.DataType, where string) string {
	var qn *ast.QualifiedName
	enumOnly := false
	switch {
	case dt.EntityRef != nil:
		qn = dt.EntityRef
	case dt.Kind == ast.TypeEnumeration && dt.EnumRef != nil:
		qn = dt.EnumRef
		enumOnly = dt.ExplicitEnum
	}
	if qn == nil || qn.Module == "" || qn.Name == "" {
		return ""
	}
	if enumOnly {
		if r.enumExists(*qn) {
			return ""
		}
		return fmt.Sprintf("%s: enumeration %s does not exist", where, qn.String())
	}
	if r.entityExists(*qn) || r.enumExists(*qn) {
		return ""
	}
	// A bare scalar name may have meant an enumeration, so the message names both.
	kind := "entity"
	if dt.Kind == ast.TypeEntity {
		kind = "entity or enumeration"
	}
	msg := fmt.Sprintf("%s: %s %s does not exist", where, kind, qn.String())
	if hint := entityNameHint(*qn); hint != "" {
		msg += ". " + hint
	}
	return msg
}

// bareEnumeration is the check-side twin of bareEnumerationType: a bare
// `Module.Name` that names no entity but names an enumeration, as exec's
// create handlers read it. It returns dt retyped as that enumeration, or false
// when dt is anything else — including a name both an entity and an
// enumeration have, which exec keeps as the entity.
func (r *declaredTypeResolver) bareEnumeration(dt ast.DataType) (ast.DataType, bool) {
	if dt.Kind != ast.TypeEntity || dt.EntityRef == nil || isBuiltinModuleEntity(dt.EntityRef.Module) {
		return dt, false
	}
	if r.entityExists(*dt.EntityRef) || !r.enumExists(*dt.EntityRef) {
		return dt, false
	}
	return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: dt.EntityRef}, true
}

// flowSignatureErrors resolves a microflow's, nanoflow's or rule's parameter
// and return types.
func flowSignatureErrors(ctx *ExecContext, sc *scriptContext, kind string, name ast.QualifiedName,
	params []ast.MicroflowParam, ret *ast.MicroflowReturnType) []string {
	if !ctx.Connected() {
		return nil
	}
	r := newDeclaredTypeResolver(ctx, sc)
	var errs []string
	for _, p := range params {
		if msg := r.check(p.Type, fmt.Sprintf("parameter $%s of %s %s", p.Name, kind, name.String())); msg != "" {
			errs = append(errs, msg)
		}
	}
	if ret != nil {
		if msg := r.check(ret.Type, fmt.Sprintf("return type of %s %s", kind, name.String())); msg != "" {
			errs = append(errs, msg)
		}
	}
	return errs
}

// documentParameterErrors resolves a page's or snippet's entity parameters,
// the ones exec resolves through pageBuilder.resolveEntity (and refuses).
func documentParameterErrors(ctx *ExecContext, sc *scriptContext, kind string, name ast.QualifiedName,
	params []ast.PageParameter) []string {
	if !ctx.Connected() {
		return nil
	}
	r := newDeclaredTypeResolver(ctx, sc)
	var errs []string
	for _, p := range params {
		if pageParamBSONType(p.Type) != "" || p.EntityType.Name == "" || p.EntityType.Module == "" {
			continue // primitive, or a bare name MDL's own checks report
		}
		if r.entityExists(p.EntityType) {
			continue
		}
		msg := fmt.Sprintf("parameter $%s of %s %s: entity %s does not exist", p.Name, kind, name.String(), p.EntityType.String())
		if hint := entityNameHint(p.EntityType); hint != "" {
			msg += ". " + hint
		}
		errs = append(errs, msg)
	}
	return errs
}
