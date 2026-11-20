// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// flowScope is one flow's variable type environment: which entity a variable
// holds, and — for a variable holding a primitive — which kind.
//
// The two halves are separate because exprcheck consumes them through separate
// seams (EntityScope and Scope), and because the questions differ: an object
// variable is resolved against the catalog per attribute, while a primitive is
// its own answer.
type flowScope struct {
	entities map[string]string
	kinds    map[string]exprcheck.TypeKind
}

func newFlowScope() *flowScope {
	return &flowScope{
		entities: map[string]string{},
		kinds:    map[string]exprcheck.TypeKind{},
	}
}

// buildFlowScope walks a microflow body in statement order and records every
// variable it can type.
//
// Order matters: a variable is typed from what introduced it, and what
// introduced it is always an earlier statement — `LOOP $r IN $reqs` can only
// type `$r` once `RETRIEVE $reqs` has been seen. The walk descends into nested
// bodies at the point they appear, so an inner loop over a list built inside an
// outer loop resolves too.
//
// The map is best-effort: an absent entry means "unknown", and every exprcheck
// rule tolerates Unknown by design. It is never a licence to guess — a wrong
// entry produces a false positive on code that builds, which costs more than
// the silence it replaces.
func buildFlowScope(body []ast.MicroflowStatement, params []ast.MicroflowParam, assoc associationResolver) *flowScope {
	s := newFlowScope()
	// Parameters are seeded BEFORE the walk, not after it. They are in scope
	// from the first statement, and a body statement can be typed FROM one —
	// `RETRIEVE $reps FROM $T/Mod.Ticket_Reporter` resolves only if `$T` is
	// already known. Appending them afterwards left every such retrieve, and
	// every loop over its result, untyped.
	addParamTypes(s, params)
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, st := range stmts {
			switch n := st.(type) {
			case *ast.CreateObjectStmt:
				if n.Variable != "" {
					s.entities[n.Variable] = n.EntityType.String()
				}
			case *ast.CreateListStmt:
				if n.Variable != "" {
					s.entities[n.Variable] = n.EntityType.String()
				}
			case *ast.RetrieveStmt:
				s.recordRetrieve(n, assoc)
			case *ast.ListOperationStmt:
				s.recordListOperation(n)
			case *ast.DeclareStmt:
				s.recordDeclare(n)
			case *ast.LoopStmt:
				// The iterator's type is the element type of the list it walks.
				// Without this every rule the checker enforces is silently off
				// for `$r/Attr` inside the body — not because the body is
				// skipped (it is walked), but because the variable resolves to
				// nothing and Unknown is tolerated everywhere
				// (mendixlabs/mxcli#1100).
				if n.LoopVariable != "" && n.ListVariable != "" {
					if qn, ok := s.entities[strings.TrimPrefix(n.ListVariable, "$")]; ok && qn != "" {
						s.entities[n.LoopVariable] = qn
					}
				}
				walk(n.Body)
			case *ast.IfStmt:
				walk(n.ThenBody)
				walk(n.ElseBody)
			case *ast.WhileStmt:
				walk(n.Body)
			}
			// A custom ON ERROR body is a block of ordinary statements, so the
			// variables it introduces are typed the same way. It is walked last
			// because it runs after the statement that carries it.
			if eb := errorHandlerBody(st); eb != nil {
				walk(eb)
			}
		}
	}
	walk(body)
	return s
}

// recordRetrieve types a RETRIEVE's output variable.
//
// A database retrieve names its entity outright. An association retrieve
// (`RETRIEVE $reps FROM $T/Mod.Ticket_Reporter`) names only the association, so
// the entity at the far end has to be resolved through the association index —
// the same hop an expression path makes, and the reason the resolver is passed
// down here rather than being consulted only at expression level.
func (s *flowScope) recordRetrieve(n *ast.RetrieveStmt, assoc associationResolver) {
	if n.Variable == "" || n.Source.Name == "" {
		return
	}
	if n.StartVariable == "" {
		s.entities[n.Variable] = n.Source.String()
		return
	}
	if assoc == nil {
		return
	}
	from, ok := s.entities[strings.TrimPrefix(n.StartVariable, "$")]
	if !ok || from == "" {
		return
	}
	if target, ok := assoc.AssociationTarget(n.Source.String(), from); ok && target != "" {
		s.entities[n.Variable] = target
	}
}

// recordListOperation types a list operation's output.
//
// The operations split three ways: most carry the input's element type through
// (a filtered list of Orders is still Orders), CONTAINS and EQUALS answer a
// Boolean, and the rest are left alone. Nothing is inferred for an operation
// whose input was never typed.
func (s *flowScope) recordListOperation(n *ast.ListOperationStmt) {
	if n.OutputVariable == "" {
		return
	}
	switch n.Operation {
	case ast.ListOpContains, ast.ListOpEquals:
		s.kinds[n.OutputVariable] = exprcheck.KindBoolean
		return
	case ast.ListOpHead, ast.ListOpTail, ast.ListOpFind, ast.ListOpFilter,
		ast.ListOpSort, ast.ListOpUnion, ast.ListOpIntersect, ast.ListOpSubtract,
		ast.ListOpRange:
		if n.InputVariable == "" {
			return
		}
		if qn, ok := s.entities[strings.TrimPrefix(n.InputVariable, "$")]; ok && qn != "" {
			s.entities[n.OutputVariable] = qn
		}
	}
}

// recordDeclare types a DECLARE'd variable from the type it was written with.
//
// This is what makes `$out + $Order/Status` reportable: E004 and the slot rules
// need BOTH operands typed, and a locally declared String was Unknown, so the
// most ordinary shape in the language — accumulate into a String — was exempt
// from every rule (mendixlabs/mxcli#1100).
func (s *flowScope) recordDeclare(n *ast.DeclareStmt) {
	if n.Variable == "" {
		return
	}
	switch {
	case n.Type.EntityRef != nil:
		s.entities[n.Variable] = n.Type.EntityRef.String()
	case n.Type.Kind == ast.TypeEnumeration && n.Type.EnumRef != nil:
		// A bare qualified name parses as TypeEnumeration with EnumRef set and
		// cannot be told from an entity (see CLAUDE.md). The ENTITY guess is
		// free — a name that is really an enumeration resolves no attributes —
		// but the KIND is not, so it is recorded only for the unambiguous
		// spelling ExplicitEnum marks. Calling an entity variable an
		// Enumeration would put the wrong type name in a rule's message.
		s.entities[n.Variable] = n.Type.EnumRef.String()
		if n.Type.ExplicitEnum {
			s.kinds[n.Variable] = exprcheck.KindEnumeration
		}
	default:
		if k, ok := DataTypeKind(n.Type.Kind); ok {
			s.kinds[n.Variable] = k
		}
	}
}

// StatementErrorHandling returns the ON ERROR clause a statement carries, or
// nil when it carries none.
//
// The clause is declared per statement type rather than on an interface, so
// this is a type switch — and a statement missing from it is invisible to every
// caller at once, which is why there is one table rather than one per walk.
// mdl/executor's validators call it too.
func StatementErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	switch s := stmt.(type) {
	case *ast.CreateObjectStmt:
		return s.ErrorHandling
	case *ast.DeleteObjectStmt:
		return s.ErrorHandling
	case *ast.MfCommitStmt:
		return s.ErrorHandling
	case *ast.ExternalObjectStmt:
		return s.ErrorHandling
	case *ast.RetrieveStmt:
		return s.ErrorHandling
	case *ast.CallMicroflowStmt:
		return s.ErrorHandling
	case *ast.CallNanoflowStmt:
		return s.ErrorHandling
	case *ast.CallJavaActionStmt:
		return s.ErrorHandling
	case *ast.DownloadFileStmt:
		return s.ErrorHandling
	case *ast.SynchronizeStmt:
		return s.ErrorHandling
	case *ast.SendEmailStmt:
		return s.ErrorHandling
	case *ast.CallJavaScriptActionStmt:
		return s.ErrorHandling
	case *ast.CallWebServiceStmt:
		return s.ErrorHandling
	case *ast.ExecuteDatabaseQueryStmt:
		return s.ErrorHandling
	// The eight statements #1078 gave an onErrorClause. Without them here, MDL076
	// cannot see a clause these statements now accept.
	case *ast.DeclareStmt:
		return s.ErrorHandling
	case *ast.MfSetStmt:
		return s.ErrorHandling
	case *ast.ChangeObjectStmt:
		return s.ErrorHandling
	case *ast.LogStmt:
		return s.ErrorHandling
	case *ast.ShowPageStmt:
		return s.ErrorHandling
	case *ast.ClosePageStmt:
		return s.ErrorHandling
	case *ast.ShowMessageStmt:
		return s.ErrorHandling
	case *ast.ValidationFeedbackStmt:
		return s.ErrorHandling
	case *ast.ListOperationStmt:
		return s.ErrorHandling
	case *ast.AggregateListStmt:
		return s.ErrorHandling
	}
	return reflectedErrorHandling(stmt)
}

var errorHandlingClauseType = reflect.TypeOf(&ast.ErrorHandlingClause{})

// reflectedErrorHandling reads the clause off any other statement that carries
// one in an `ErrorHandling` field. The switch above is a list, and a list
// drifts: `call workflow`, the REST statements and the other workflow
// statements all take an onErrorClause and were missing from it, so the rules
// that ask for the clause — MDL076 for one (mendixlabs/mxcli#175) — could not
// see it, and their handler bodies were never walked.
func reflectedErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	v := reflect.ValueOf(stmt)
	if v.Kind() != reflect.Ptr || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	f := v.Elem().FieldByName("ErrorHandling")
	if !f.IsValid() || f.Type() != errorHandlingClauseType || f.IsNil() {
		return nil
	}
	return f.Interface().(*ast.ErrorHandlingClause)
}

// errorHandlerBody returns a statement's custom ON ERROR body, or nil when it
// has no clause or the clause is one of the bodyless forms (CONTINUE/ROLLBACK).
func errorHandlerBody(stmt ast.MicroflowStatement) []ast.MicroflowStatement {
	eh := StatementErrorHandling(stmt)
	if eh == nil || len(eh.Body) == 0 {
		return nil
	}
	return eh.Body
}

// DataTypeKind maps an MDL primitive data-type kind to an exprcheck kind,
// reporting false for kinds that have no primitive answer (entities, lists,
// void, type parameters).
//
// It lives here rather than in mdl/executor because both the scope-local
// validator and this adapter need the same answer, and two copies of a type
// mapping is how a resolver drifts.
func DataTypeKind(k ast.DataTypeKind) (exprcheck.TypeKind, bool) {
	switch k {
	case ast.TypeString, ast.TypeStringTemplate, ast.TypeHashedString:
		return exprcheck.KindString, true
	case ast.TypeInteger, ast.TypeAutoNumber:
		return exprcheck.KindInteger, true
	case ast.TypeLong:
		return exprcheck.KindLong, true
	case ast.TypeDecimal:
		return exprcheck.KindDecimal, true
	case ast.TypeBoolean:
		return exprcheck.KindBoolean, true
	case ast.TypeDateTime, ast.TypeDate:
		return exprcheck.KindDateTime, true
	case ast.TypeBinary:
		return exprcheck.KindBinary, true
	case ast.TypeEnumeration:
		return exprcheck.KindEnumeration, true
	default:
		return exprcheck.KindUnknown, false
	}
}

// entityScope adapts a variable→entity map plus an association resolver to
// exprcheck.EntityScope, which is what lets `$Order/Customer/Name` be typed.
type entityScope struct {
	vars  map[string]string
	assoc associationResolver
}

// associationResolver is the association half of the model. exprcatalog.Reader
// satisfies it, and so does mdl/xpathrefs' Model — the two resolvers answer the
// same question and the signature is deliberately shared.
type associationResolver interface {
	AssociationTarget(assocQN, fromEntityQN string) (string, bool)
}

var _ exprcheck.EntityScope = entityScope{}

func (e entityScope) VariableEntity(name string) (string, bool) {
	qn, ok := e.vars[strings.TrimPrefix(name, "$")]
	return qn, ok && qn != ""
}

func (e entityScope) AssociationTarget(assocQN, fromEntityQN string) (string, bool) {
	if e.assoc == nil {
		return "", false
	}
	return e.assoc.AssociationTarget(assocQN, fromEntityQN)
}

// kindScope adapts a variable→kind map to exprcheck.Scope.
type kindScope map[string]exprcheck.TypeKind

var _ exprcheck.Scope = kindScope{}

func (k kindScope) Lookup(name string) (exprcheck.TypeKind, bool) {
	v, ok := k[strings.TrimPrefix(name, "$")]
	return v, ok && v != exprcheck.KindUnknown
}

// addParamTypes records what a parameter holds.
//
// buildFlowScope walks only the body, so it sees a variable a CREATE or
// RETRIEVE introduced and misses every parameter — and a microflow that takes
// its object as a parameter is the ordinary case, not an edge one.
//
// The visitor cannot tell `$P: Mod.Person` from an enumeration-typed parameter:
// a bare qualified name parses as TypeEnumeration with EnumRef set (see
// CLAUDE.md). Both spellings are recorded rather than guessed between — a name
// that turns out to be an enumeration simply resolves no attributes, so the
// wrong guess costs nothing.
//
// It is called from buildFlowScope before the body walk; see the note there on
// why the order is load-bearing.
//
// A `list of Mod.Entity` parameter records the ELEMENT entity, which is what a
// LOOP over it needs; the list itself is not an object and nothing resolves an
// attribute against it.
func addParamTypes(s *flowScope, params []ast.MicroflowParam) {
	for _, p := range params {
		if p.Name == "" {
			continue
		}
		switch {
		case p.Type.EntityRef != nil:
			s.entities[p.Name] = p.Type.EntityRef.String()
		case p.Type.Kind == ast.TypeEnumeration && p.Type.EnumRef != nil:
			s.entities[p.Name] = p.Type.EnumRef.String()
			if p.Type.ExplicitEnum {
				s.kinds[p.Name] = exprcheck.KindEnumeration
			}
		default:
			if k, ok := DataTypeKind(p.Type.Kind); ok {
				s.kinds[p.Name] = k
			}
		}
	}
}
