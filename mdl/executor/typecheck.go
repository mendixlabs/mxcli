// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcatalog"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/exprcheck/adapters"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// TypeCheckProgram type-checks the expressions in a script's microflows and
// nanoflows against the connected project, returning one violation per hint.
//
// This is the catalog-backed tier of PROPOSAL_expression_type_checking: the
// rules that need to know an attribute's type, an enumeration's cases, or a
// microflow's return type. The scope-local tier already runs unconditionally in
// ValidateProgram, so this only adds what a project can answer.
//
// Hints keep exprcheck's own E0xx codes rather than being remapped. They are a
// coherent, documented set with a hints registry behind them, and renaming them
// at the boundary would mean two vocabularies for one diagnostic — the code in
// the message would no longer match the code you can look up.
//
// A project that cannot be read, or a catalog that cannot be built, returns no
// violations rather than an error. Type checking is advisory: a caller that
// could not consult the project should report what it could check, not fail.
func (e *Executor) TypeCheckProgram(prog *ast.Program) []linter.Violation {
	if prog == nil || e == nil {
		return nil
	}
	ctx := e.newExecContext(context.Background())
	if !ctx.Connected() {
		return nil
	}

	// Fast mode is enough: attributes, enumeration values, microflows and their
	// parameters are all built in it. Only permissions, references, strings and
	// XPath need a full build, and none of them feed a type lookup — so a check
	// never pays for a full catalog.
	if err := ensureCatalog(ctx, false); err != nil {
		return nil
	}
	cat := ctx.Catalog
	if cat == nil {
		return nil
	}

	reader, err := exprcatalog.Load(cat.CatalogDB())
	if err != nil {
		return nil
	}

	declareScriptTypes(reader, prog)

	// microflowExprSource falls back to rendering the AST when the visitor did
	// not attach source text, which it does for some slots and not others. The
	// adapter's own default reads SourceExpr only, and would silently skip
	// whichever half of a flow happened not to carry one.
	adapter := adapters.NewCheckAdapter(reader, adapters.WithSourceFunc(microflowExprSource))
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			out = append(out, adapter.CheckMicroflow(s).AsViolations()...)
		case *ast.CreateNanoflowStmt:
			out = append(out, adapter.CheckNanoflow(s).AsViolations()...)
		}
	}
	return out
}

// declareScriptTypes overlays the enumerations and attributes the script itself
// creates onto the catalog-backed reader. The catalog only knows the stored
// project, so a microflow assigning a quoted string to an enumeration attribute
// created a few statements earlier checked clean and failed at build time
// (ako/mxcli#969). Statements are applied in order, so a later redefinition
// wins, as it does when the script runs.
func declareScriptTypes(reader *exprcatalog.Reader, prog *ast.Program) {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEnumerationStmt:
			cases := make([]string, 0, len(s.Values))
			for _, v := range s.Values {
				cases = append(cases, v.Name)
			}
			reader.DeclareEnumeration(s.Name.String(), cases)
		case *ast.CreateEntityStmt:
			for i := range s.Attributes {
				declareScriptAttribute(reader, s.Name.String(), s.Attributes[i].Name, s.Attributes[i].Type)
			}
		case *ast.AlterEntityStmt:
			switch {
			case s.Operation == ast.AlterEntityAddAttribute && s.Attribute != nil:
				declareScriptAttribute(reader, s.Name.String(), s.Attribute.Name, s.Attribute.Type)
			case s.Operation == ast.AlterEntityModifyAttribute:
				declareScriptAttribute(reader, s.Name.String(), s.AttributeName, s.DataType)
			}
		}
	}
	// Flows second, so a parameter typed by an enumeration or entity the
	// script creates resolves whichever order the statements come in.
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			reader.DeclareFlow(s.Name.String(), scriptFlowParams(reader, s.Parameters))
		case *ast.CreateNanoflowStmt:
			reader.DeclareFlow(s.Name.String(), scriptFlowParams(reader, s.Parameters))
		}
	}
}

// scriptFlowParams types the parameters of a flow the script creates, so a
// call to it is checked against them (ako/mxcli#2096). A bare `Module.Name`
// type is ambiguous — the parser cannot tell an entity from an enumeration —
// and is settled only by what the reader knows; otherwise it is left out, and
// its arguments are not checked.
func scriptFlowParams(reader *exprcatalog.Reader, params []ast.MicroflowParam) []exprcatalog.FlowParam {
	out := make([]exprcatalog.FlowParam, 0, len(params))
	for _, p := range params {
		fp := exprcatalog.FlowParam{Name: p.Name}
		dt := p.Type
		switch {
		case dt.Kind == ast.TypeListOf && dt.EntityRef != nil:
			fp.Kind, fp.Ref = exprcheck.KindList, dt.EntityRef.String()
		case dt.Kind == ast.TypeEntity && dt.EntityRef != nil:
			fp.Kind, fp.Ref = exprcheck.KindObject, dt.EntityRef.String()
		case dt.Kind == ast.TypeEnumeration && dt.EnumRef != nil:
			qn := dt.EnumRef.String()
			_, isEnum := reader.EnumCases(qn)
			switch {
			case dt.ExplicitEnum || isEnum:
				fp.Kind, fp.Ref = exprcheck.KindEnumeration, qn
			case reader.KnowsEntity(qn):
				fp.Kind, fp.Ref = exprcheck.KindObject, qn
			}
		case dt.Kind == ast.TypeEnumeration:
			// No reference to compare: the kind alone.
			fp.Kind = exprcheck.KindEnumeration
		default:
			if k, ok := adapters.DataTypeKind(dt.Kind); ok {
				fp.Kind = k
			}
		}
		out = append(out, fp)
	}
	return out
}

func declareScriptAttribute(reader *exprcatalog.Reader, entityQN, attr string, dt ast.DataType) {
	enumQN := ""
	if dt.EnumRef != nil {
		enumQN = dt.EnumRef.String()
	}
	reader.DeclareAttribute(entityQN, attr, scriptAttributeKind(dt.Kind), enumQN)
}

// scriptAttributeKind mirrors exprcatalog's mapping of stored type names.
func scriptAttributeKind(k ast.DataTypeKind) exprcheck.TypeKind {
	switch k {
	case ast.TypeString, ast.TypeHashedString:
		return exprcheck.KindString
	case ast.TypeInteger:
		return exprcheck.KindInteger
	case ast.TypeLong, ast.TypeAutoNumber:
		return exprcheck.KindLong
	case ast.TypeDecimal:
		return exprcheck.KindDecimal
	case ast.TypeBoolean:
		return exprcheck.KindBoolean
	case ast.TypeDateTime, ast.TypeDate, ast.TypeAutoCreatedDate, ast.TypeAutoChangedDate:
		return exprcheck.KindDateTime
	case ast.TypeBinary:
		return exprcheck.KindBinary
	case ast.TypeEnumeration:
		return exprcheck.KindEnumeration
	}
	return exprcheck.KindUnknown
}
