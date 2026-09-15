// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

// StatementAnnotations and StatementBodies both read a statement by REFLECTION
// rather than through a type switch, on the argument that a switch silently
// skips the type added after it was written. That argument is only as good as
// the field-shape assumptions the reflection makes, and those are what these
// tests pin — by reading this package's own source, so a new statement type is
// covered the moment it is declared rather than when someone remembers to
// extend a list.

// astStructFields parses the package source and yields every struct type with
// its fields. Source-driven because Go cannot enumerate the types implementing
// an interface at runtime, and a hand-maintained list here would have exactly
// the staleness problem the reflective readers exist to avoid.
func astStructFields(t *testing.T) map[string][]*ast.Field {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parsing the ast package source: %v", err)
	}
	out := map[string][]*ast.Field{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				out[ts.Name.Name] = st.Fields.List
				return true
			})
		}
	}
	if len(out) == 0 {
		t.Fatal("no structs found — the source scan is not looking where it thinks it is")
	}
	return out
}

func fieldTypeString(f *ast.Field) string {
	switch tp := f.Type.(type) {
	case *ast.Ident:
		return tp.Name
	case *ast.StarExpr:
		if id, ok := tp.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	case *ast.ArrayType:
		if id, ok := tp.Elt.(*ast.Ident); ok {
			return "[]" + id.Name
		}
	}
	return ""
}

// TestEveryAnnotatedStatementIsReachable pins the assumption StatementAnnotations
// rests on: the annotations of a statement are held in a field literally named
// `Annotations` of type *ActivityAnnotations. A statement type that spelled it
// any other way would have its annotations silently dropped — which is the very
// failure the reflective read was introduced to prevent (upstream #884).
func TestEveryAnnotatedStatementIsReachable(t *testing.T) {
	for name, fields := range astStructFields(t) {
		for _, f := range fields {
			if fieldTypeString(f) != "*ActivityAnnotations" {
				continue
			}
			if len(f.Names) != 1 || f.Names[0].Name != "Annotations" {
				got := "embedded"
				if len(f.Names) == 1 {
					got = f.Names[0].Name
				}
				t.Errorf("%s holds its *ActivityAnnotations in a field named %q; "+
					"StatementAnnotations looks up \"Annotations\" by name, so this "+
					"statement's annotations are silently dropped", name, got)
			}
		}
	}
}

// TestStatementBodiesReachesEveryNestedBody pins the two assumptions
// StatementBodies rests on. The first — that a nested statement list is a field
// of type []MicroflowStatement — is checked structurally by the walker itself.
// The second is the fragile one: for a CASE ARM (a struct in a slice, like
// EnumSplitCase), the walker finds the arm's statements by looking up a field
// named `Body`. An arm that called it `Statements` would take its whole branch
// out of every whole-flow check with nothing to notice.
func TestStatementBodiesReachesEveryNestedBody(t *testing.T) {
	structs := astStructFields(t)
	for name, fields := range structs {
		for _, f := range fields {
			// A slice of some other struct declared in this package — the case-arm
			// shape. If that struct holds statements, they must be in `Body`.
			elem := fieldTypeString(f)
			if len(elem) < 3 || elem[:2] != "[]" {
				continue
			}
			armFields, ok := structs[elem[2:]]
			if !ok {
				continue
			}
			var stmtField string
			for _, af := range armFields {
				if fieldTypeString(af) == "[]MicroflowStatement" && len(af.Names) == 1 {
					stmtField = af.Names[0].Name
				}
			}
			if stmtField != "" && stmtField != "Body" {
				t.Errorf("%s.%s is a slice of %s, whose statements live in %q; "+
					"StatementBodies looks up \"Body\", so that branch is invisible "+
					"to every check that walks the whole flow",
					name, fieldName(f), elem[2:], stmtField)
			}
		}
	}
}

func fieldName(f *ast.Field) string {
	if len(f.Names) == 1 {
		return f.Names[0].Name
	}
	return "<embedded>"
}

// The behavioural half: a statement nesting every shape the walker handles must
// yield every one of its bodies. The control is the count — dropping any single
// arm of the switch in StatementBodies takes this below 5.
func TestStatementBodies_YieldsEveryShape(t *testing.T) {
	mark := func(tag string) []MicroflowStatement {
		return []MicroflowStatement{&LogStmt{Message: &LiteralExpr{Value: tag, Kind: LiteralString}}}
	}

	ifStmt := &IfStmt{ThenBody: mark("then"), ElseBody: mark("else")}
	if got := len(StatementBodies(ifStmt)); got != 2 {
		t.Errorf("IfStmt: got %d bodies, want 2 (then, else)", got)
	}

	enum := &EnumSplitStmt{
		Cases:    []EnumSplitCase{{Value: "A", Body: mark("a")}, {Value: "B", Body: mark("b")}},
		ElseBody: mark("else"),
	}
	if got := len(StatementBodies(enum)); got != 3 {
		t.Errorf("EnumSplitStmt: got %d bodies, want 3 (two arms + else)", got)
	}

	declare := &DeclareStmt{
		Variable:      "X",
		ErrorHandling: &ErrorHandlingClause{Type: ErrorHandlingCustom, Body: mark("handler")},
	}
	if got := len(StatementBodies(declare)); got != 1 {
		t.Errorf("DeclareStmt with ON ERROR: got %d bodies, want 1 (the handler)", got)
	}

	loop := &LoopStmt{LoopVariable: "Item", ListVariable: "Items", Body: mark("loop")}
	if got := len(StatementBodies(loop)); got != 1 {
		t.Errorf("LoopStmt: got %d bodies, want 1", got)
	}

	// A statement with nothing nested must yield nothing, or a caller recursing
	// on the result would never terminate.
	if got := StatementBodies(&ReturnStmt{}); got != nil {
		t.Errorf("ReturnStmt: got %v, want no bodies", got)
	}
	if got := StatementBodies(nil); got != nil {
		t.Errorf("nil: got %v, want no bodies", got)
	}
	var typed *LogStmt
	if got := StatementBodies(typed); got != nil {
		t.Errorf("typed nil: got %v, want no bodies", got)
	}
}

// The reflective readers must not be fooled by a non-struct or a nil interior.
func TestStatementAnnotations_NilSafety(t *testing.T) {
	if got := StatementAnnotations(nil); got != nil {
		t.Errorf("nil: got %v", got)
	}
	var typed *LogStmt
	if got := StatementAnnotations(typed); got != nil {
		t.Errorf("typed nil: got %v", got)
	}
	stmt := &LogStmt{Annotations: &ActivityAnnotations{Caption: "c"}}
	if got := StatementAnnotations(stmt); got == nil || got.Caption != "c" {
		t.Errorf("got %v, want the annotations", got)
	}
	// Guards the field-name lookup against a same-named field of another type.
	if reflect.TypeOf(ActivityAnnotations{}).Kind() != reflect.Struct {
		t.Fatal("ActivityAnnotations is no longer a struct")
	}
}

// SetStatementAnnotations is the write side. The field shape it depends on is
// pinned for every statement type by TestEveryAnnotatedStatementIsReachable;
// this pins the reflection itself — including the workflow statements, which
// the type switch it replaced skipped.
func TestSetStatementAnnotations(t *testing.T) {
	ann := &ActivityAnnotations{Position: &Position{X: 740, Y: 320}}
	for _, s := range []MicroflowStatement{
		&LogStmt{},
		&EnumSplitStmt{},
		&OpenWorkflowStmt{},
		&NotifyWorkflowStmt{},
		&CallWorkflowStmt{},
		&ImportFromMappingStmt{},
	} {
		if !SetStatementAnnotations(s, ann) {
			t.Errorf("%T: SetStatementAnnotations reported no Annotations field", s)
			continue
		}
		if got := StatementAnnotations(s); got != ann {
			t.Errorf("%T: read back %v, want the annotations just set", s, got)
		}
	}

	// A nil annotation clears, and nil / non-pointer / field-less statements are
	// refused rather than panicking.
	log := &LogStmt{Annotations: ann}
	if !SetStatementAnnotations(log, nil) || log.Annotations != nil {
		t.Errorf("setting nil did not clear: %v", log.Annotations)
	}
	if SetStatementAnnotations(nil, ann) {
		t.Error("nil statement: reported success")
	}
	var typed *LogStmt
	if SetStatementAnnotations(typed, ann) {
		t.Error("typed nil: reported success")
	}
}
