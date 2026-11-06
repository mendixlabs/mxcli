// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// Expect is one @expect assertion.
//
// It holds the annotation as written (for the failure message) and the Mendix
// boolean expression the generated microflow evaluates. The two are not the same
// string: `<>` is accepted in an annotation and rewritten to `!=`, which is the
// spelling Mendix's expression engine understands.
type Expect struct {
	// Raw is the annotation text as the author wrote it.
	Raw string
	// Condition is the Mendix boolean expression that must hold for the test to
	// pass. It is re-rendered from the parsed expression, so it is exactly the
	// text that was validated.
	Condition string
	// Actual is a Mendix expression yielding the observed value as a String, for
	// the failure message. It is empty when no such expression can be derived
	// without guessing the operand's type — see actualExpr.
	Actual string
	// Aggregates are the list aggregates Condition refers to by variable. They
	// have to be computed by an activity before the condition is evaluated —
	// see ExpectAggregate.
	Aggregates []ExpectAggregate
}

// ExpectAggregate is one list aggregate lifted out of an @expect condition.
//
// `count($List)` is not a Mendix expression function — counting a list is an
// Aggregate list activity, so it cannot appear in the decision that evaluates
// the assertion. The generators emit `$Var = COUNT($List);` ahead of that
// decision and the condition refers to $Var, which is the same thing the author
// would write by hand.
type ExpectAggregate struct {
	Var  string // generated variable holding the result, e.g. "$mxtest_count_Brands"
	Op   string // MDL aggregate keyword, e.g. "COUNT"
	List string // the list variable being aggregated, e.g. "$Brands"
}

// ParseExpect parses one @expect annotation body.
//
// Every assertion the runner cannot evaluate is an error here, and an error
// makes the test an ERROR rather than a PASS. That is the whole point: the
// previous implementation matched `$var = <literal>` with a regular expression
// and silently discarded every line that did not fit, so a test whose only
// assertion was `1 = 2` reported PASS. An assertion a test framework cannot
// evaluate has exactly one safe outcome, and passing is not it.
func ParseExpect(raw string) (Expect, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Expect{}, fmt.Errorf("@expect needs an expression")
	}

	toks := exprcheck.Lex(raw)
	p := &expectParser{toks: toks, src: raw}
	node, err := p.parse()
	if err != nil {
		return Expect{}, err
	}

	if !isAssertionShaped(node) {
		return Expect{}, fmt.Errorf(
			"@expect %s is not a condition: it evaluates to a value, not to true or false "+
				"(did you mean to compare it with = or !=?)", raw)
	}

	return Expect{
		Raw:        raw,
		Condition:  node.render(),
		Actual:     actualExpr(node),
		Aggregates: collectAggregates(node),
	}, nil
}

// collectAggregates returns the aggregates in the expression, in source order
// and without repeats. Two assertions counting the same list share one
// variable, so the activity is emitted once.
func collectAggregates(n expectNode) []ExpectAggregate {
	var (
		out  []ExpectAggregate
		seen = map[string]bool{}
		walk func(expectNode)
	)
	walk = func(n expectNode) {
		switch e := n.(type) {
		case *expectAggregate:
			if !seen[e.Var] {
				seen[e.Var] = true
				out = append(out, ExpectAggregate{Var: e.Var, Op: e.Op, List: e.List})
			}
		case *expectBinary:
			walk(e.Left)
			walk(e.Right)
		case *expectUnary:
			walk(e.Operand)
		case *expectParen:
			walk(e.Inner)
		case *expectCall:
			for _, a := range e.Args {
				walk(a)
			}
		case *expectIfThenElse:
			walk(e.Cond)
			walk(e.Then)
			walk(e.Else)
		}
	}
	walk(n)
	return out
}

// isAssertionShaped reports whether the expression can be a pass/fail condition.
//
// A comparison, a logical operator, a Boolean-returning call and a bare variable
// all qualify; a bare literal or an arithmetic expression does not. `1 = 2` is a
// comparison, so it qualifies and — correctly — fails.
func isAssertionShaped(n expectNode) bool {
	switch e := n.(type) {
	case *expectBinary:
		switch e.Op {
		case "=", "!=", "<", "<=", ">", ">=", "and", "or":
			return true
		}
		return false
	case *expectCall:
		if e.Name == "not" {
			return true
		}
		kind, known := exprcheck.FuncReturnKind(e.Name)
		return !known || kind == exprcheck.KindBoolean
	case *expectParen:
		return isAssertionShaped(e.Inner)
	case *expectVar:
		// A Boolean variable or attribute is a valid condition on its own.
		return true
	case *expectLiteral:
		return e.Kind == exprcheck.KindBoolean
	case *expectIfThenElse:
		// Mendix's if-then-else binds loosest, so `if c then a else b = 1` is
		// `if c then a else (b = 1)`; the whole thing is a condition only when
		// both branches are.
		return isAssertionShaped(e.Then) && isAssertionShaped(e.Else)
	}
	return false
}

// actualExpr returns a Mendix expression that renders the observed value as a
// String, or "" when one cannot be derived safely.
//
// "Safely" is doing real work here. Mendix's expression engine is typed: `+`
// only concatenates Strings, and toString() rejects a String operand. So the
// observed value is only reported when its type is pinned down, and it is pinned
// down in two ways: the operand's own inferred kind, and — when that is unknown —
// the kind of the other side of the comparison, which must match for the
// comparison itself to have compiled at all. A String operand is used directly;
// a known non-String scalar is wrapped in toString(). Anything else reports no
// actual value rather than emitting an expression that may not compile.
//
// Measured against mxbuild 11.6.6, Mendix is in fact more permissive than this
// rule assumes — `toString()` accepts a String and `+` accepts an Integer, both
// at 0 errors. The rule stays conservative anyway: `toString()` on an *object*
// is not covered by that measurement, and the cost of guessing wrong is a
// microflow that will not compile, which fails the test for a reason that has
// nothing to do with what it was asserting. The same run confirmed the check is
// not blind to this class — `$result = 3` on a String is CE0117.
func actualExpr(n expectNode) string {
	cmp, ok := n.(*expectBinary)
	if !ok {
		return ""
	}
	switch cmp.Op {
	case "=", "!=", "<", "<=", ">", ">=":
	default:
		return ""
	}

	left, right := cmp.Left, cmp.Right
	_, leftLit := left.(*expectLiteral)
	_, rightLit := right.(*expectLiteral)
	if leftLit && rightLit {
		// `1 = 2` asserts something about nothing the test computed, so there is
		// no observed value to report.
		return ""
	}
	// The interesting side is the one that is not a literal — that is the value
	// the test observed.
	if leftLit {
		left, right = right, left
	}
	lk, rk := left.kind(), right.kind()

	switch {
	case lk == exprcheck.KindString:
		return left.render()
	case isStringifiableScalar(lk):
		return "toString(" + left.render() + ")"
	case lk == exprcheck.KindUnknown && rk == exprcheck.KindString:
		// The comparison only compiles if both sides are Strings.
		return left.render()
	case lk == exprcheck.KindUnknown && isStringifiableScalar(rk):
		return "toString(" + left.render() + ")"
	case lk == exprcheck.KindUnknown && rk == exprcheck.KindEnumeration:
		// An enumeration value on one side pins the other to the same
		// enumeration; toString() yields its key, which is how the expected
		// value is written.
		return "toString(" + left.render() + ")"
	case lk == exprcheck.KindEnumeration && rk == exprcheck.KindUnknown:
		return "toString(" + right.render() + ")"
	}
	return ""
}

// isStringifiableScalar reports whether toString() is defined for the kind and
// the kind is not already a String.
func isStringifiableScalar(k exprcheck.TypeKind) bool {
	switch k {
	case exprcheck.KindBoolean, exprcheck.KindInteger, exprcheck.KindLong,
		exprcheck.KindDecimal, exprcheck.KindDateTime:
		return true
	}
	return false
}

// -----------------------------------------------------------------------------
// Expression tree
//
// This is a validating parser, not the recovering one in mdl/exprcheck: that one
// is built to keep going and emit hints, which is the right behaviour for a
// linter and the wrong behaviour here. Every construct it does not recognise
// must stop the parse.
// -----------------------------------------------------------------------------

type expectNode interface {
	render() string
	kind() exprcheck.TypeKind
}

type expectLiteral struct {
	Text string
	Kind exprcheck.TypeKind
}

func (e *expectLiteral) render() string           { return e.Text }
func (e *expectLiteral) kind() exprcheck.TypeKind { return e.Kind }

// expectVar is a variable, an attribute path (`$obj/Attr`), a qualified name
// (`Module.Enum.Value`) or a `[%Token%]`.
type expectVar struct{ Text string }

func (e *expectVar) render() string { return e.Text }
func (e *expectVar) kind() exprcheck.TypeKind {
	if isEnumLiteralName(e.Text) {
		return exprcheck.KindEnumeration
	}
	return exprcheck.KindUnknown
}

// isEnumLiteralName reports whether a variable's text is an enumeration value,
// `Module.Enum.Value`. A variable, an attribute path and a token start with
// `$`, `@` or `[%`; an entity or attribute name has one dot, not two.
func isEnumLiteralName(s string) bool {
	if s == "" || strings.ContainsAny(s[:1], "$@[") {
		return false
	}
	return strings.Count(s, ".") == 2 && !strings.ContainsAny(s, "/$ ")
}

type expectCall struct {
	Name string
	Args []expectNode
}

func (e *expectCall) render() string {
	parts := make([]string, len(e.Args))
	for i, a := range e.Args {
		parts[i] = a.render()
	}
	return e.Name + "(" + strings.Join(parts, ", ") + ")"
}

func (e *expectCall) kind() exprcheck.TypeKind {
	if k, ok := exprcheck.FuncReturnKind(e.Name); ok {
		return k
	}
	return exprcheck.KindUnknown
}

// expectAggregate is a list aggregate the generators hoist into an activity. It
// renders as the variable that activity assigns, so from the condition's point
// of view it is an ordinary variable.
type expectAggregate struct {
	Var  string
	Op   string
	List string
}

func (e *expectAggregate) render() string { return e.Var }

// Mendix's Aggregate list activity returns a Long for count.
func (e *expectAggregate) kind() exprcheck.TypeKind { return exprcheck.KindLong }

type expectParen struct{ Inner expectNode }

func (e *expectParen) render() string           { return "(" + e.Inner.render() + ")" }
func (e *expectParen) kind() exprcheck.TypeKind { return e.Inner.kind() }

type expectUnary struct {
	Op      string
	Operand expectNode
}

func (e *expectUnary) render() string           { return e.Op + e.Operand.render() }
func (e *expectUnary) kind() exprcheck.TypeKind { return e.Operand.kind() }

type expectBinary struct {
	Op          string
	Left, Right expectNode
}

func (e *expectBinary) render() string {
	return e.Left.render() + " " + e.Op + " " + e.Right.render()
}

func (e *expectBinary) kind() exprcheck.TypeKind {
	switch e.Op {
	case "=", "!=", "<", "<=", ">", ">=", "and", "or":
		return exprcheck.KindBoolean
	case "+":
		// Mendix overloads + for concatenation; either String operand makes the
		// result a String.
		if e.Left.kind() == exprcheck.KindString || e.Right.kind() == exprcheck.KindString {
			return exprcheck.KindString
		}
	}
	if e.Left.kind() == e.Right.kind() {
		return e.Left.kind()
	}
	return exprcheck.KindUnknown
}

type expectIfThenElse struct{ Cond, Then, Else expectNode }

func (e *expectIfThenElse) render() string {
	return "if " + e.Cond.render() + " then " + e.Then.render() + " else " + e.Else.render()
}

func (e *expectIfThenElse) kind() exprcheck.TypeKind {
	if e.Then.kind() == e.Else.kind() {
		return e.Then.kind()
	}
	return exprcheck.KindUnknown
}

// -----------------------------------------------------------------------------
// Parser
// -----------------------------------------------------------------------------

type expectParser struct {
	toks []exprcheck.Token
	pos  int
	src  string
}

func (p *expectParser) peek() exprcheck.Token { return p.toks[p.pos] }

func (p *expectParser) next() exprcheck.Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

// atKeyword reports whether the current token is the given case-insensitive
// keyword. Mendix expression keywords are lower-case; the annotation is allowed
// to use any case and is normalised on render.
func (p *expectParser) atKeyword(kw string) bool {
	t := p.peek()
	return t.Kind == exprcheck.TokIdent && strings.EqualFold(t.Text, kw)
}

func (p *expectParser) errorAt(t exprcheck.Token, format string, args ...any) error {
	what := t.Text
	if t.Kind == exprcheck.TokEOF {
		what = "end of expression"
	} else {
		what = fmt.Sprintf("%q", what)
	}
	return fmt.Errorf("@expect %s: %s at column %d (%s)",
		p.src, fmt.Sprintf(format, args...), t.Pos.Column, what)
}

func (p *expectParser) parse() (expectNode, error) {
	// A lexer error token anywhere means a character the Mendix expression
	// grammar has no place for, so reject before parsing.
	for _, t := range p.toks {
		if t.Kind == exprcheck.TokError {
			return nil, p.errorAt(t, "unexpected character")
		}
	}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.Kind != exprcheck.TokEOF {
		return nil, p.errorAt(t, "unexpected trailing input")
	}
	return n, nil
}

func (p *expectParser) parseOr() (expectNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.atKeyword("or") {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &expectBinary{Op: "or", Left: left, Right: right}
	}
	return left, nil
}

func (p *expectParser) parseAnd() (expectNode, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.atKeyword("and") {
		p.next()
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = &expectBinary{Op: "and", Left: left, Right: right}
	}
	return left, nil
}

// comparisonOps maps a lexed comparison token to the Mendix spelling. `<>` and
// `!=` both lex to TokNeq; Mendix only accepts `!=`, which is why the operator is
// taken from this table rather than from the token's own text.
var comparisonOps = map[exprcheck.TokKind]string{
	exprcheck.TokEq:  "=",
	exprcheck.TokNeq: "!=",
	exprcheck.TokLt:  "<",
	exprcheck.TokLe:  "<=",
	exprcheck.TokGt:  ">",
	exprcheck.TokGe:  ">=",
}

func (p *expectParser) parseComparison() (expectNode, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	op, ok := comparisonOps[p.peek().Kind]
	if !ok {
		return left, nil
	}
	p.next()
	right, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	return &expectBinary{Op: op, Left: left, Right: right}, nil
}

func (p *expectParser) parseAdditive() (expectNode, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for {
		var op string
		switch p.peek().Kind {
		case exprcheck.TokPlus:
			op = "+"
		case exprcheck.TokMinus:
			op = "-"
		default:
			return left, nil
		}
		p.next()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &expectBinary{Op: op, Left: left, Right: right}
	}
}

func (p *expectParser) parseMultiplicative() (expectNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		var op string
		switch {
		case p.peek().Kind == exprcheck.TokStar:
			op = "*"
		case p.atKeyword("div"):
			op = "div"
		case p.atKeyword("mod"):
			op = "mod"
		default:
			return left, nil
		}
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &expectBinary{Op: op, Left: left, Right: right}
	}
}

func (p *expectParser) parseUnary() (expectNode, error) {
	if p.peek().Kind == exprcheck.TokMinus {
		p.next()
		operand, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &expectUnary{Op: "-", Operand: operand}, nil
	}
	return p.parsePrimary()
}

func (p *expectParser) parsePrimary() (expectNode, error) {
	t := p.peek()
	switch t.Kind {
	case exprcheck.TokString:
		p.next()
		return &expectLiteral{Text: t.Text, Kind: exprcheck.KindString}, nil

	case exprcheck.TokNumber:
		p.next()
		kind := exprcheck.KindInteger
		if strings.Contains(t.Text, ".") {
			kind = exprcheck.KindDecimal
		}
		return &expectLiteral{Text: t.Text, Kind: kind}, nil

	case exprcheck.TokToken:
		p.next()
		return &expectVar{Text: t.Text}, nil

	case exprcheck.TokDollarIdent:
		return p.parseVariablePath()

	case exprcheck.TokAt:
		// @Module.Constant
		p.next()
		name, err := p.parseDottedName()
		if err != nil {
			return nil, err
		}
		return &expectVar{Text: "@" + name}, nil

	case exprcheck.TokLParen:
		p.next()
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().Kind != exprcheck.TokRParen {
			return nil, p.errorAt(p.peek(), "expected a closing parenthesis")
		}
		p.next()
		return &expectParen{Inner: inner}, nil

	case exprcheck.TokIdent:
		return p.parseIdentPrimary()
	}
	return nil, p.errorAt(t, "expected a value")
}

// parseVariablePath parses `$var`, `$var/Attr/Sub` and `$var.Attr`.
func (p *expectParser) parseVariablePath() (expectNode, error) {
	t := p.next()
	if t.Text == "$" {
		return nil, p.errorAt(t, "expected a variable name after $")
	}
	var b strings.Builder
	b.WriteString(t.Text)
	for {
		sep := ""
		switch p.peek().Kind {
		case exprcheck.TokSlash:
			sep = "/"
		case exprcheck.TokDot:
			sep = "."
		default:
			return &expectVar{Text: b.String()}, nil
		}
		p.next()
		name := p.peek()
		if name.Kind != exprcheck.TokIdent {
			return nil, p.errorAt(name, "expected a member name after %q", sep)
		}
		p.next()
		b.WriteString(sep)
		b.WriteString(name.Text)
	}
}

// parseDottedName parses `Module.Name` and `Module.Enum.Value`.
func (p *expectParser) parseDottedName() (string, error) {
	first := p.peek()
	if first.Kind != exprcheck.TokIdent {
		return "", p.errorAt(first, "expected a name")
	}
	p.next()
	var b strings.Builder
	b.WriteString(first.Text)
	for p.peek().Kind == exprcheck.TokDot {
		p.next()
		part := p.peek()
		if part.Kind != exprcheck.TokIdent {
			return "", p.errorAt(part, "expected a name after '.'")
		}
		p.next()
		b.WriteString(".")
		b.WriteString(part.Text)
	}
	return b.String(), nil
}

// parseIdentPrimary handles the four things an identifier can start: a keyword
// literal, `if ... then ... else ...`, a function call, and a qualified name.
func (p *expectParser) parseIdentPrimary() (expectNode, error) {
	t := p.peek()
	switch strings.ToLower(t.Text) {
	case "true", "false":
		p.next()
		return &expectLiteral{Text: strings.ToLower(t.Text), Kind: exprcheck.KindBoolean}, nil
	case "empty":
		// `empty` is Mendix's null literal, but `empty(...)` is not a function —
		// so only the bare form is accepted.
		p.next()
		if p.peek().Kind == exprcheck.TokLParen {
			return nil, p.errorAt(p.peek(), "empty is a value, not a function")
		}
		return &expectLiteral{Text: "empty", Kind: exprcheck.KindEmpty}, nil
	case "if":
		return p.parseIfThenElse()
	}

	// A call: the name must be a Mendix built-in. A bare name(...) in a Mendix
	// expression is always a built-in — entity and enumeration references are
	// qualified names, not calls — so an unknown name is an error, not a
	// user-defined function.
	if p.toks[p.pos+1].Kind == exprcheck.TokLParen {
		if isListAggregate(t.Text) {
			return p.parseAggregate()
		}
		return p.parseCall()
	}

	name, err := p.parseDottedName()
	if err != nil {
		return nil, err
	}
	if !strings.Contains(name, ".") {
		return nil, p.errorAt(t,
			"%q is not a variable, a function or a qualified name", name)
	}
	return &expectVar{Text: name}, nil
}

// listAggregates are Mendix's list aggregates. None of them is an expression
// function — they are Aggregate list *activities* — so a bare one in an @expect
// used to be rejected as "not a Mendix expression function", which is true and
// tells the author nothing about what to do instead.
var listAggregates = map[string]bool{
	"count": true, "sum": true, "average": true, "minimum": true, "maximum": true,
}

func isListAggregate(name string) bool { return listAggregates[strings.ToLower(name)] }

// parseAggregate parses `count($List)`.
//
// Only count is accepted. The other four aggregate an attribute over the list
// (`SUM($list.Amount)`), which needs the attribute's type to render a
// comparison, so they are refused with the helper-microflow workaround rather
// than guessed at.
func (p *expectParser) parseAggregate() (expectNode, error) {
	nameTok := p.next()
	name := strings.ToLower(nameTok.Text)
	p.next() // consume '('

	if name != "count" {
		return nil, p.errorAt(nameTok,
			"%s() aggregates an attribute over a list, which a test assertion cannot do "+
				"on its own — call a microflow that returns the %s and assert on its "+
				"result (count($list) is supported here)", name, name)
	}

	arg := p.peek()
	if arg.Kind != exprcheck.TokDollarIdent {
		return nil, p.errorAt(arg, "count() counts a list variable, as in count($MyList)")
	}
	node, err := p.parseVariablePath()
	if err != nil {
		return nil, err
	}
	list := node.render()
	if strings.ContainsAny(list, "/.") {
		return nil, p.errorAt(arg,
			"count() counts a list variable, not the path %s — retrieve the list into a "+
				"variable first", list)
	}
	if p.peek().Kind != exprcheck.TokRParen {
		return nil, p.errorAt(p.peek(), "expected a closing parenthesis for count()")
	}
	p.next()

	return &expectAggregate{
		// Named after the list so two assertions over the same list share one
		// activity, and prefixed so it cannot collide with a test's own
		// variables.
		Var:  "$mxtest_count_" + strings.TrimPrefix(list, "$"),
		Op:   "COUNT",
		List: list,
	}, nil
}

func (p *expectParser) parseCall() (expectNode, error) {
	nameTok := p.next()
	name := nameTok.Text
	sig, known := exprcheck.PublicFuncTable()[name]
	if !known {
		return nil, p.errorAt(nameTok, "%s() is not a Mendix expression function", name)
	}
	p.next() // consume '('

	var args []expectNode
	if p.peek().Kind != exprcheck.TokRParen {
		for {
			arg, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.peek().Kind != exprcheck.TokComma {
				break
			}
			p.next()
		}
	}
	if p.peek().Kind != exprcheck.TokRParen {
		return nil, p.errorAt(p.peek(), "expected a closing parenthesis for %s()", name)
	}
	p.next()

	minArgs := sig.MinArgs
	if minArgs == 0 {
		minArgs = len(sig.Args)
	}
	if len(args) < minArgs || (!sig.Variadic && len(args) > len(sig.Args)) {
		return nil, p.errorAt(nameTok, "%s() takes %s, got %d",
			name, arityText(minArgs, len(sig.Args)), len(args))
	}
	return &expectCall{Name: name, Args: args}, nil
}

func arityText(min, max int) string {
	if min == max {
		return fmt.Sprintf("%d argument(s)", min)
	}
	return fmt.Sprintf("%d to %d arguments", min, max)
}

func (p *expectParser) parseIfThenElse() (expectNode, error) {
	p.next() // if
	cond, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if !p.atKeyword("then") {
		return nil, p.errorAt(p.peek(), "expected 'then'")
	}
	p.next()
	thenExpr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if !p.atKeyword("else") {
		return nil, p.errorAt(p.peek(), "expected 'else'")
	}
	p.next()
	elseExpr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	return &expectIfThenElse{Cond: cond, Then: thenExpr, Else: elseExpr}, nil
}
