// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/mendixexpr"
)

// declaredMatches reports whether a declared microflow AST value (from the
// script) states the same thing as the stored flow's AST value (from parsing
// what `describe` prints for it). It is the equality diff-then-patch matches
// statements by (ADR-0012 decision 3; plan item 4.2g).
//
// It is structural equality with one asymmetry: canvas GEOMETRY the script
// leaves out is not a difference. A statement written without `@position`,
// `@curve`, `@anchor`, `@merge` or `@start` has no opinion about where its node
// sits or how its flow bends, so it matches the stored statement wherever that
// is drawn — `create or modify` changes only what differs (R1), and a layout
// nobody stated is not a difference. A statement that DOES state geometry must
// state the stored one: a moved node is a real change, and one the splice does
// not make (it only places new nodes), so it must not pass as unchanged.
//
// Everything else is compared exactly, including captions, colours and notes,
// which are content Studio Pro shows rather than layout.
func declaredMatches(declared, stored any) bool {
	return matchValue(reflect.ValueOf(declared), reflect.ValueOf(stored), matchDeclared)
}

// sameIgnoringLayout reports whether two statements differ in canvas geometry
// at most — a node moved, a connector redrawn.
func sameIgnoringLayout(declared, stored any) bool {
	return matchValue(reflect.ValueOf(declared), reflect.ValueOf(stored), matchAnyLayout)
}

// sameExceptPositions reports whether two statements differ at most in where
// their nodes are drawn (@position, @start) — which the patch makes by moving
// the stored nodes (ako/mxcli#818) — and not in how their flows are drawn.
func sameExceptPositions(declared, stored any) bool {
	return matchValue(reflect.ValueOf(declared), reflect.ValueOf(stored), matchAnyPosition)
}

// matchMode is how matchValue treats geometry.
type matchMode int

const (
	// matchDeclared: geometry the declared side leaves out is not a
	// difference; geometry it states must be the stored one.
	matchDeclared matchMode = iota
	// matchAnyLayout: no geometry is a difference.
	matchAnyLayout
	// matchAnyPosition: a node's position is not a difference; the rest of
	// the geometry is compared as for matchDeclared.
	matchAnyPosition
)

// positionFields are the geometry fields of a statement's annotations that
// say where its node is drawn, as opposed to how its flows are.
var positionFields = map[string]bool{"Position": true, "Start": true}

// The geometry is what stripFlowLayout clears for `mxcli layout flows`; a
// note's size is included here because the writer defaults it when absent.
var (
	annotationsStructType = reflect.TypeOf(ast.ActivityAnnotations{})
	noteStructType        = reflect.TypeOf(ast.MicroflowAnnotation{})
	paramStructType       = reflect.TypeOf(ast.MicroflowParam{})
	retrieveStructType    = reflect.TypeOf(ast.RetrieveStmt{})
	sourceExprStructType  = reflect.TypeOf(ast.SourceExpr{})
)

// geometryFields names, per AST type, the fields that hold canvas geometry and
// that a script may therefore leave nil without that being a difference.
var geometryFields = map[reflect.Type]map[string]bool{
	annotationsStructType: {
		"Position": true, "Anchor": true, "TrueBranchAnchor": true, "FalseBranchAnchor": true,
		"IteratorAnchor": true, "BodyTailAnchor": true, "CaseAnchors": true, "Curve": true, "Merge": true, "Start": true,
		"MergeSize": true, "Size": true, // ako/mxcli#2369
	},
	noteStructType:  {"Position": true, "Size": true},
	paramStructType: {"Position": true},
}

// spellingFields names, per AST type, the fields that record how a statement
// was SPELLED rather than what it stores. Describe prints one spelling, so the
// stored side never carries the other, and comparing these would make a
// declared statement fail to match its own stored activity (ako/mxcli#942):
// re-spliced on every run, and next to a loop the run of "changes" swallowed
// the loop and rebuilt it with new $IDs under mdl 1. They exist only for
// diagnostics (MDL067, MDL065) and nothing downstream branches on them.
// unstatedFields names, per AST type, the content fields a script may leave
// out without that being a difference: nil states nothing, and the stored
// value is kept. Unlike geometry they are compared in every mode — a changed
// documentation is never "only layout" (ako/mxcli#2328).
var unstatedFields = map[reflect.Type]map[string]bool{
	annotationsStructType: {"Documentation": true},
}

var spellingFields = map[reflect.Type]map[string]bool{
	// `commit $X with events` is a bare `commit $X`: both store events on.
	// WithoutEvents, which IS stored, is still compared.
	reflect.TypeOf(ast.MfCommitStmt{}): {"ExplicitWithEvents": true},
	// `case X` / `else` are the legacy spellings of `when X then` /
	// `when (empty) then`; both build the identical split.
	reflect.TypeOf(ast.InheritanceSplitStmt{}): {"LegacyCaseKeyword": true, "LegacyElseKeyword": true},
}

func matchValue(d, s reflect.Value, mode matchMode) bool {
	if !d.IsValid() || !s.IsValid() {
		return d.IsValid() == s.IsValid()
	}
	if d.Type() != s.Type() {
		return false
	}
	switch d.Kind() {
	case reflect.Pointer:
		if d.Type() == errorHandlingPtrType {
			// `on error rollback` is what an activity with no clause stores,
			// so describe never prints it (formatErrorHandlingSuffix): the
			// stored side never has it, and a declared one that states it is
			// the same activity (ako/mxcli#859).
			d, s = withoutDefaultErrorHandling(d), withoutDefaultErrorHandling(s)
		}
		if d.IsNil() && s.IsNil() {
			return true
		}
		if d.Type().Elem() == annotationsStructType {
			// A statement written with no annotations at all states no
			// geometry, and no caption, colour or note either.
			if d.IsNil() {
				d = reflect.New(annotationsStructType)
			}
			if s.IsNil() {
				s = reflect.New(annotationsStructType)
			}
		}
		if d.IsNil() || s.IsNil() {
			return false
		}
		return matchValue(d.Elem(), s.Elem(), mode)
	case reflect.Interface:
		if d.IsNil() || s.IsNil() {
			return d.IsNil() == s.IsNil()
		}
		if d.Type() == expressionType {
			if same, ok := sameMendixExpression(d, s); ok {
				return same
			}
		}
		return matchValue(d.Elem(), s.Elem(), mode)
	case reflect.Struct:
		if d.Type() == dataTypeStructType {
			d, s = storedFlowDataType(d), storedFlowDataType(s)
		}
		if messageStructTypes[d.Type()] {
			d, s = messageAsTemplate(d), messageAsTemplate(s)
		}
		geo := geometryFields[d.Type()]
		spelling := spellingFields[d.Type()]
		for i := 0; i < d.NumField(); i++ {
			df := d.Field(i)
			name := d.Type().Field(i).Name
			if spelling[name] {
				continue
			}
			skip := mode == matchAnyLayout ||
				(mode == matchAnyPosition && d.Type() == annotationsStructType && positionFields[name])
			if geo[name] && df.Kind() == reflect.Pointer && (skip || df.IsNil()) {
				continue
			}
			if unstatedFields[d.Type()][name] && df.IsNil() {
				continue
			}
			if d.Type() == sourceExprStructType && name == "Source" {
				// Whitespace around and between an expression's tokens is
				// not part of it, nor is a keyword's case: the writer stores
				// it as written, describe prints its own (ako/mxcli#886).
				if mendixexpr.Canonical(df.String()) != mendixexpr.Canonical(s.Field(i).String()) {
					return false
				}
				continue
			}
			if d.Type() == noteStructType && name == "Text" {
				// A note's line breaks are CRLF as Studio Pro stores them and
				// describe prints them; an editor that saves the script with
				// LF changed nothing the canvas shows (mendixlabs/mxcli#1505).
				// A matched note is left as stored, so its bytes stay CRLF.
				if lfLineBreaks(df.String()) != lfLineBreaks(s.Field(i).String()) {
					return false
				}
				continue
			}
			if d.Type() == retrieveStructType && name == "Where" {
				if !sameStoredConstraint(df, s.Field(i), retrieveEntity(d), retrieveEntity(s)) {
					return false
				}
				continue
			}
			if !matchValue(df, s.Field(i), mode) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		// nil and empty are the same list.
		if d.Len() != s.Len() {
			return false
		}
		for i := 0; i < d.Len(); i++ {
			if !matchValue(d.Index(i), s.Index(i), mode) {
				return false
			}
		}
		return true
	case reflect.Map:
		if d.Len() != s.Len() {
			return false
		}
		for _, k := range d.MapKeys() {
			sv := s.MapIndex(k)
			if !sv.IsValid() || !matchValue(d.MapIndex(k), sv, mode) {
				return false
			}
		}
		return true
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return d.IsNil() == s.IsNil()
	default:
		return d.Equal(s)
	}
}

// lfLineBreaks returns text with every line break spelled LF.
func lfLineBreaks(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

var errorHandlingPtrType = reflect.TypeOf((*ast.ErrorHandlingClause)(nil))

var dataTypeStructType = reflect.TypeOf(ast.DataType{})

// storedFlowDataType returns v, an ast.DataType, as the type a flow stores
// for it: a flow has one integer type, Integer/Long, which the writer stores
// as DataTypes$IntegerType and describe prints `Integer` — so `Long` is the
// same declaration (ako/mxcli#886).
func storedFlowDataType(v reflect.Value) reflect.Value {
	dt, ok := v.Interface().(ast.DataType)
	if !ok || dt.Kind != ast.TypeLong {
		return v
	}
	dt.Kind = ast.TypeInteger
	return reflect.ValueOf(dt)
}

// messageStructTypes are the statements whose message the builder stores as
// a template: a string literal as its text, any other expression as '{1}'
// with the expression as the first parameter.
var messageStructTypes = map[reflect.Type]bool{
	reflect.TypeOf(ast.LogStmt{}):                true,
	reflect.TypeOf(ast.ShowMessageStmt{}):        true,
	reflect.TypeOf(ast.ValidationFeedbackStmt{}): true,
	// A REST call's URL and custom body: restCallAsTemplates (ako/mxcli#1444).
	reflect.TypeOf(ast.RestCallStmt{}): true,
}

// messageAsTemplate returns v, a statement with a message, with its message
// in the form the builder stores it (addLogMessageAction,
// addShowMessageAction, addValidationFeedbackAction): `'failed for ' + $Name`
// is the template '{1}' with ({1} = 'failed for ' + $Name), which describe
// prints — so the two spellings are one activity (ako/mxcli#905). A log
// that states its own parameters keeps its message as written, as the
// builder does.
func messageAsTemplate(v reflect.Value) reflect.Value {
	templated := func(msg ast.Expression) bool {
		if msg == nil {
			return false
		}
		lit, ok := msg.(*ast.LiteralExpr)
		return !ok || lit.Kind != ast.LiteralString
	}
	placeholder := &ast.LiteralExpr{Kind: ast.LiteralString, Value: "{1}"}
	switch st := v.Interface().(type) {
	case ast.LogStmt:
		if len(st.Template) == 0 && templated(st.Message) {
			st.Template = []ast.TemplateParam{{Index: 1, Value: st.Message}}
			st.Message = placeholder
		}
		return reflect.ValueOf(st)
	case ast.ShowMessageStmt:
		if templated(st.Message) {
			st.TemplateArgs = append([]ast.Expression{st.Message}, st.TemplateArgs...)
			st.Message = placeholder
		}
		return reflect.ValueOf(st)
	case ast.ValidationFeedbackStmt:
		if templated(st.Message) {
			st.TemplateArgs = append([]ast.Expression{st.Message}, st.TemplateArgs...)
			st.Message = placeholder
		}
		return reflect.ValueOf(st)
	case ast.RestCallStmt:
		return reflect.ValueOf(restCallAsTemplates(st))
	}
	return v
}

var expressionType = reflect.TypeOf((*ast.Expression)(nil)).Elem()

// sameMendixExpression compares two Mendix expressions of a statement by
// their canonical tokens (mendixexpr.Canonical), which is what they store
// once the writer's own spelling is set aside (ako/mxcli#886). The same
// expression has several trees: `Active = false` written before a line break
// is source text kept with the break, `Active = false` on one line a literal;
// `AND` where an operand forces the source to be kept is text, `and` where it
// does not a binary node. ok is false when either side holds something the
// renderer cannot spell (an XPath path outside its source), which is then
// compared as a tree.
func sameMendixExpression(d, s reflect.Value) (same, ok bool) {
	de, _ := d.Interface().(ast.Expression)
	se, _ := s.Interface().(ast.Expression)
	if !renderable(de) || !renderable(se) {
		return false, false
	}
	return mendixexpr.Canonical(mendixexpr.String(de)) == mendixexpr.Canonical(mendixexpr.String(se)), true
}

// renderable reports whether mendixexpr.String spells all of e: a source
// expression is its source; any other node must be one the renderer knows, and
// so must every node under it.
func renderable(e ast.Expression) bool {
	switch x := e.(type) {
	case nil:
		return false
	case *ast.SourceExpr:
		return x != nil && (x.Source != "" || renderable(x.Expression))
	case *ast.LiteralExpr, *ast.VariableExpr, *ast.AttributePathExpr, *ast.TokenExpr,
		*ast.IdentifierExpr, *ast.QualifiedNameExpr, *ast.ConstantRefExpr:
		return !reflect.ValueOf(e).IsNil()
	case *ast.BinaryExpr:
		return x != nil && renderable(x.Left) && renderable(x.Right)
	case *ast.UnaryExpr:
		return x != nil && renderable(x.Operand)
	case *ast.ParenExpr:
		return x != nil && renderable(x.Inner)
	case *ast.IfThenElseExpr:
		return x != nil && renderable(x.Condition) && renderable(x.ThenExpr) && renderable(x.ElseExpr)
	case *ast.FunctionCallExpr:
		if x == nil {
			return false
		}
		for _, a := range x.Arguments {
			if !renderable(a) {
				return false
			}
		}
		return true
	}
	return false
}

// withoutDefaultErrorHandling returns v, an *ast.ErrorHandlingClause, as nil
// when it is a bare `on error rollback`. In a microflow that is the stored
// default; in a nanoflow, whose default is Abort, describe prints neither
// (both emit nothing), so no comparison with a described flow can tell them
// apart either way.
func withoutDefaultErrorHandling(v reflect.Value) reflect.Value {
	if eh, ok := v.Interface().(*ast.ErrorHandlingClause); ok && eh != nil &&
		eh.Type == ast.ErrorHandlingRollback && len(eh.Body) == 0 {
		return reflect.Zero(v.Type())
	}
	return v
}

// reflectElem returns the struct a statement pointer points at, or the zero
// Value when it is not a pointer to a struct.
func reflectElem(v any) reflect.Value {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return rv.Elem()
}

// sameStoredConstraint compares two retrieve where clauses as the XPath
// constraint each stores (ako/mxcli#839).
//
// The AST is the wrong measure here: the bracketed form an author writes,
// `where [a and b]`, is read as XPath and keeps its source text, while the bare
// form describe prints, `where a and b`, is read as an expression, so one
// constraint parses to two different trees — different node types, `and`
// against `AND`, the author's line breaks in one and not the other. What a
// retrieve means is the constraint it stores, so that is what is compared,
// with whitespace between tokens ignored: the writer lays a long constraint
// out over several lines itself (FormatXPathConstraint), and a line break
// between two tokens states nothing. Whitespace inside a string literal is
// data and is compared as written.
//
// Each side is resolved against its own retrieve's entity, as the writer does,
// so `[M.Emp.Name = 'y']` is the `Name = 'y'` describe prints for what it
// stored (ako/mxcli#874).
func sameStoredConstraint(d, s reflect.Value, dEntity, sEntity string) bool {
	de, _ := d.Interface().(ast.Expression)
	se, _ := s.Interface().(ast.Expression)
	return xpathTokens(retrieveXPathConstraint(de, dEntity)) == xpathTokens(retrieveXPathConstraint(se, sEntity))
}

// retrieveEntity is the entity a retrieve statement's struct value reads from.
func retrieveEntity(retrieve reflect.Value) string {
	if q, ok := retrieve.FieldByName("Source").Interface().(ast.QualifiedName); ok {
		return q.String()
	}
	return ""
}

// xpathTokens is an XPath constraint with the whitespace between its tokens
// normalised: a run of it becomes one space, and none is kept inside a
// bracket or parenthesis. String literals are copied as they are.
func xpathTokens(xpath string) string {
	var b strings.Builder
	var last byte // the last byte written
	space := false
	for i := 0; i < len(xpath); {
		c := xpath[i]
		switch c {
		case ' ', '\t', '\n', '\r':
			space = true
			i++
			continue
		}
		if space && last != 0 && last != '[' && last != '(' && c != ']' && c != ')' {
			b.WriteByte(' ')
		}
		space = false
		end := i + 1
		if c == '\'' {
			end = xpathLiteralEnd(xpath, i)
		}
		b.WriteString(xpath[i:end])
		last = xpath[end-1]
		i = end
	}
	return b.String()
}
