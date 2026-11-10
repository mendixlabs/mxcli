// SPDX-License-Identifier: Apache-2.0

// Design note — single-pass parse+check:
// Parsing and semantic hint emission are intentionally combined in one pass
// rather than separated into parse-then-check phases. This avoids a second
// AST traversal and lets the parser emit hints at the exact source position
// of each token while its context is still live on the call stack.
// Trade-off: parsePrimary / parseOr / … carry both responsibilities (SRP
// tension). If a future use case needs parse-without-hints, gate expensive
// catalog lookups behind ctx.IsSemanticEnabled().

package exprcheck

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

type parserImpl struct{}

func NewParser() Parser { return &parserImpl{} }

func (p *parserImpl) Parse(src string, ctx Context) (RobustExpr, []Hint) {
	s := NewStream(Lex(src))
	expr, hs := parseOr(s, ctx)
	// Detect unconsumed trailing tokens (e.g. "emptyor" parsed as a variable,
	// leaving "$X = ''" silently abandoned). This indicates a structural parse
	// error — most commonly a keyword glued to an adjacent token without whitespace.
	// TokError tokens (unrecognised characters) are excluded: the parser's E007
	// recovery already handles them inline. That exclusion is also why ':' had
	// to become an operator rather than stay a TokError: as an error token it
	// ended the parse silently, so `x : 4 < $Max` was checked as just `x`.
	if t := s.Peek(); t.Kind != TokEOF && t.Kind != TokError {
		// This hint used to carry no code, no document and no microflow: the
		// location held only a line and column, which are the offsets WITHIN the
		// expression fragment rather than into the file, so they printed as
		// nothing useful. The reader saw `[]` for the code and had to find the
		// offending expression in a 40-line script by eye
		// (mendixlabs/mxcli#1042). Everything the context knows is attached now,
		// which is what hintsLocation is for.
		//
		// The fix line is also no longer only about glued keywords. That was the
		// case it was written for, and it sent people hunting for an 'emptyor'
		// that was not there: the report's actual input was `empty($List)` —
		// `empty` is a Mendix KEYWORD, not a function, so the parser consumed it
		// and stopped at the '('. Naming the likelier causes first makes the
		// message point at the real one.
		hs = append(hs, hints.Hint{
			Code:     "E014",
			Slug:     "trailing-tokens",
			Severity: hints.SeverityError,
			Where:    hintsLocation(ctx, t.Pos),
			YouWrote: t.Text,
			Problem: "Unexpected token after the expression — everything up to here parsed, " +
				"and this is left over, so the expression is incomplete or malformed.",
			Fix: "Check that a keyword is not being used as a function — `empty` is a keyword, " +
				"so `empty($List)` is not a call; write `$List = empty` (or `length($List) = 0`). " +
				"Also check for glued keywords such as 'emptyor' (should be 'empty or').",
		})
	}
	hs = append(hs, checkValueLiterals(expr, ctx)...)
	hs = append(hs, checkSlotKind(expr, ctx)...)
	hs = append(hs, checkCallArgument(expr, ctx)...)
	hs = append(hs, checkBareIdentifierValue(expr, ctx)...)
	hs = append(hs, checkStringFuncsComparedWithEmpty(expr, nil, ctx)...)
	return expr, hs
}

// checkSlotKind compares the whole expression's inferred kind against what the
// slot it sits in expects.
//
// The expectations table in slot_resolver.go had existed for some time with
// NOTHING READING IT — slotKind() was defined and never called — so a slot
// declared `{Kind: KindString}` constrained nothing. `LOG WARNING 42` passed,
// and so did every non-String log template parameter (mendixlabs/mxcli#1043),
// which is CE0117 "Error(s) in expression" at the activity.
//
// Only a CONCRETE expectation is enforced. An entry carrying ResolveBy
// ("AttributeOf:Parent", "MicroflowReturn", …) names a kind that has to be
// looked up per call site, and the adapter encodes that by appending the
// resolved target to the slot path ("ChangeItem.Value:Sales.Order.Status"),
// which does not match the table at all — so those stay unenforced here rather
// than being enforced against the wrong kind.
//
// Both sides must be known. An inferred KindUnknown is "could not tell", and
// reporting it would flag every expression whose type mxcli cannot resolve.
func checkSlotKind(expr RobustExpr, ctx Context) []Hint {
	sc, ok := slotKind(ctx)
	if !ok || sc.Kind == KindUnknown || sc.ResolveBy != "" {
		return nil
	}
	k := inferKind(expr, ctx)
	if k == KindUnknown || k == sc.Kind || slices.Contains(sc.AlsoAccepts, k) {
		return nil
	}
	// `empty` satisfies any slot: it is Mendix's null, not a kind of its own.
	if k == KindEmpty {
		return nil
	}
	mxbuild := sc.Mxbuild
	if mxbuild == "" {
		mxbuild = "CE0117 \"Error(s) in expression\""
	}
	fix := "Convert it, e.g. with toString(...)."
	if sc.Kind != KindString {
		fix = "Replace it with an expression of kind " + typeKindName(sc.Kind) + "."
	}
	return []Hint{{
		Code:     "E009",
		Slug:     "slot-type-mismatch",
		Severity: hints.SeverityError,
		Where:    hintsLocation(ctx, expr.Pos()),
		YouWrote: "<" + typeKindName(k) + ">",
		Problem: "This position requires " + typeKindName(sc.Kind) + ", but the expression has kind " +
			typeKindName(k) + ". Mendix does not coerce here — it reports " + mxbuild + ".",
		Fix: fix,
	}}
}

// checkBareIdentifierValue reports a bare word standing alone as a member's
// value: `CHANGE $Order (Status = Closed)`.
//
// Mendix expressions have no bare identifiers — a variable is `$Name`, a string
// is 'quoted', an enumeration value is Module.Enum.Value — so the parser reads
// one as a variable reference, it resolves to nothing, and the kind comes out
// Unknown. Unknown is tolerated everywhere by design, which is exactly why this
// slipped through check and exec to arrive as CE0117 (mendixlabs/mxcli#1044).
//
// Scoped to the WHOLE expression of a create/change member, and that scoping is
// the load-bearing part rather than caution for its own sake: a bare name NESTED
// inside a list-operation predicate is legal MDL — `FILTER($L, Status = 'Open')`
// resolves `Status` against the item under test — so a rule that fired on any
// bare identifier would reject working scripts. A member's value is the one
// position where the bare word is the entire expression and can only be a
// mistake.
//
// A call argument is the same position: the whole value handed to one
// parameter. Measured on 11.14.0, a bare word passed to a String, an Integer,
// an enumeration or an object parameter of a called microflow or nanoflow is
// CE0117 at the call activity. (A blank argument, `Param = nothing`, renders
// as no text at all and never reaches this rule.)
func checkBareIdentifierValue(expr RobustExpr, ctx Context) []Hint {
	if !strings.HasPrefix(ctx.SlotPath, "ChangeItem.Value") &&
		!strings.HasPrefix(ctx.SlotPath, "CreateItem.Value") &&
		!strings.HasPrefix(ctx.SlotPath, callArgumentSlot) {
		return nil
	}
	v, ok := expr.(*VariableExpr)
	if !ok || !v.Bare {
		return nil
	}
	// A name that IS in scope is a variable the author spelled without its $,
	// which is worth saying differently.
	fix := "Quote it if it is text ('" + v.Name + "'), write $" + v.Name +
		" if it is a variable, or qualify it (Module.Enum.Value) if it is an enumeration value."
	if ctx.Scope != nil {
		if _, inScope := ctx.Scope.Lookup(v.Name); inScope {
			fix = "Write $" + v.Name + " — a variable reference needs its $."
		}
	}
	return []Hint{{
		Code:     "E013",
		Slug:     "bare-identifier-value",
		Severity: hints.SeverityError,
		Where:    hintsLocation(ctx, expr.Pos()),
		YouWrote: v.Name,
		Problem: "A bare word is not a Mendix expression. Mendix reads a value as a literal, " +
			"a $variable, a qualified name or a function call, so this arrives as " +
			"CE0117 \"Error(s) in expression\".",
		Fix: fix,
	}}
}

func parseOr(s *Stream, ctx Context) (RobustExpr, []Hint) {
	left, hints := parseAnd(s, ctx)
	first := true
	for matchKeyword(s, "or") {
		if first {
			hints = append(hints, checkBoolOperand(left, ctx, "or")...)
			first = false
		}
		right, h := parseAnd(s, ctx)
		hints = append(hints, h...)
		hints = append(hints, checkBoolOperand(right, ctx, "or")...)
		left = &BinExpr{Op: "OR", L: left, R: right}
	}
	return left, hints
}

func parseAnd(s *Stream, ctx Context) (RobustExpr, []Hint) {
	left, hints := parseNot(s, ctx)
	first := true
	for matchKeyword(s, "and") {
		if first {
			hints = append(hints, checkBoolOperand(left, ctx, "and")...)
			first = false
		}
		right, h := parseNot(s, ctx)
		hints = append(hints, h...)
		hints = append(hints, checkBoolOperand(right, ctx, "and")...)
		left = &BinExpr{Op: "AND", L: left, R: right}
	}
	return left, hints
}

func parseNot(s *Stream, ctx Context) (RobustExpr, []Hint) {
	if matchKeyword(s, "not") {
		notPos := s.Peek().Pos
		needsParens := s.Peek().Kind != TokLParen
		inner, h := parseCmp(s, ctx)
		if needsParens {
			h = append(h, Hint{
				Code:     "E011",
				Slug:     "not-missing-parens",
				Severity: hints.SeverityError,
				Where:    hintsLocation(ctx, notPos),
				YouWrote: "not <expr>",
				Problem:  "Mendix requires parentheses: not(expr). 'not expr' without parentheses is rejected by Studio Pro with CE0117.",
				Fix:      "Wrap the operand in parentheses: not(<expr>)",
			})
		}
		h = append(h, checkBoolOperand(inner, ctx, "not")...)
		return &UnaryExpr{Op: "NOT", Operand: inner}, h
	}
	return parseCmp(s, ctx)
}

func parseCmp(s *Stream, ctx Context) (RobustExpr, []Hint) {
	left, hints := parseAdd(s, ctx)
	op := ""
	switch s.Peek().Kind {
	case TokEq:
		op = "="
	case TokNeq:
		op = "!="
	case TokLt:
		op = "<"
	case TokLe:
		op = "<="
	case TokGt:
		op = ">"
	case TokGe:
		op = ">="
	}
	if op == "" {
		return left, hints
	}
	opTok := s.Consume()
	right, h := parseAdd(s, ctx)
	hints = append(hints, h...)
	if op == "=" || op == "!=" {
		hints = append(hints, checkEnumComparedToString(left, right, ctx, opTok)...)
	}
	return &BinExpr{Op: op, L: left, R: right}, hints
}

// neverEmptyStringFuncs are the String built-ins whose result is never
// `empty`: given `empty`, the empty string or blanks, each returns the empty
// string or text. Measured on the
// Mendix 10.18.5 runtime with `mxcli test --local` (mendixlabs/mxcli#580), with
// the bare variable as the control that does compare equal to `empty`. Add a
// function only once it has been measured.
var neverEmptyStringFuncs = map[string]bool{
	"trim":        true,
	"toLowerCase": true,
	"toUpperCase": true,
	"substring":   true,
	"replaceAll":  true,
	"toString":    true,
}

// checkStringFuncsComparedWithEmpty emits E015 for `trim($v) != empty`: the
// result can never be `empty`, so the comparison is constant — `!=` always
// true, `=` always false. The model is valid and builds, which is why it is a
// warning; the logic is what is wrong, and the blank-check the author meant is
// trim($v) compared with the empty string literal, which also covers `empty`
// since trim(empty) is the empty string.
//
// It runs over the finished tree rather than in parseCmp because it needs the
// comparison's siblings: trim($v) != empty, and-ed with the same call compared
// with the empty string — the empty-and-blank idiom — has a dead first half, but the second decides it and the
// condition does what it says. Reporting that is noise on Studio Pro-authored
// projects, so a comparison is skipped when its own and/or chain also
// compares the same call with the empty string. chain holds those siblings.
func checkStringFuncsComparedWithEmpty(expr RobustExpr, chain []RobustExpr, ctx Context) []Hint {
	switch n := expr.(type) {
	case *BinExpr:
		if n.Op == "AND" || n.Op == "OR" {
			operands := flattenChain(n, n.Op, nil)
			var hs []Hint
			for _, o := range operands {
				hs = append(hs, checkStringFuncsComparedWithEmpty(o, operands, ctx)...)
			}
			return hs
		}
		if n.Op == "=" || n.Op == "!=" {
			if h := stringFuncVsEmptyHint(n, chain, ctx); h != nil {
				return []Hint{*h}
			}
		}
		return append(checkStringFuncsComparedWithEmpty(n.L, nil, ctx),
			checkStringFuncsComparedWithEmpty(n.R, nil, ctx)...)
	case *ParenExpr:
		return checkStringFuncsComparedWithEmpty(n.Inner, chain, ctx)
	case *UnaryExpr:
		return checkStringFuncsComparedWithEmpty(n.Operand, nil, ctx)
	case *IfThenElseExpr:
		hs := checkStringFuncsComparedWithEmpty(n.Cond, nil, ctx)
		hs = append(hs, checkStringFuncsComparedWithEmpty(n.Then, nil, ctx)...)
		return append(hs, checkStringFuncsComparedWithEmpty(n.Else, nil, ctx)...)
	case *CallExpr:
		var hs []Hint
		for _, a := range n.Args {
			hs = append(hs, checkStringFuncsComparedWithEmpty(a, nil, ctx)...)
		}
		return hs
	}
	return nil
}

// flattenChain returns the operands of a run of the same and/or operator,
// looking through parentheses: `a and (b and c)` is one chain of three.
func flattenChain(e RobustExpr, op string, out []RobustExpr) []RobustExpr {
	if b, ok := unwrapParens(e).(*BinExpr); ok && b.Op == op {
		return flattenChain(b.R, op, flattenChain(b.L, op, out))
	}
	return append(out, e)
}

func stringFuncVsEmptyHint(cmp *BinExpr, chain []RobustExpr, ctx Context) *Hint {
	call := neverEmptyCallAgainstEmpty(cmp.L, cmp.R)
	if call == nil {
		call = neverEmptyCallAgainstEmpty(cmp.R, cmp.L)
	}
	if call == nil || chainComparesWithBlank(chain, call) {
		return nil
	}
	always := "true"
	if cmp.Op == "=" {
		always = "false"
	}
	return &Hint{
		Code: "E015", Slug: "string-func-vs-empty", Severity: hints.SeverityWarning,
		Where:    hintsLocation(ctx, call.Pos()),
		YouWrote: call.Name + "(...) " + cmp.Op + " empty",
		Problem: call.Name + "() never returns empty — for an empty input it returns '' — " +
			"so this comparison is always " + always + ".",
		Fix: "Compare with '' instead: " + call.Name + "(...) " + cmp.Op + " ''",
	}
}

// chainComparesWithBlank reports whether some operand of chain compares a call
// structurally equal to call with the empty string literal.
func chainComparesWithBlank(chain []RobustExpr, call *CallExpr) bool {
	key := exprKey(call)
	for _, o := range chain {
		b, ok := unwrapParens(o).(*BinExpr)
		if !ok || (b.Op != "=" && b.Op != "!=") {
			continue
		}
		for _, pair := range [][2]RobustExpr{{b.L, b.R}, {b.R, b.L}} {
			lit, ok := unwrapParens(pair[1]).(*StringLit)
			if ok && lit.Value == "" && exprKey(unwrapParens(pair[0])) == key {
				return true
			}
		}
	}
	return false
}

// exprKey renders the operand shapes a guard repeats — calls over variables,
// attribute paths and literals — position-free, so two spellings of the same
// call compare equal. Anything else keys as unique, which only ever means
// "not the same call" and so errs toward reporting.
func exprKey(e RobustExpr) string {
	switch n := e.(type) {
	case *CallExpr:
		args := make([]string, len(n.Args))
		for i, a := range n.Args {
			args[i] = exprKey(unwrapParens(a))
		}
		return n.Name + "(" + strings.Join(args, ",") + ")"
	case *VariableExpr:
		return "$" + n.Name
	case *AttributePathExpr:
		if n.Predicates != nil {
			return fmt.Sprintf("%p", n)
		}
		return "$" + n.Variable + "/" + strings.Join(n.Path, "/")
	case *StringLit:
		return "'" + n.Value + "'"
	case *NumberLit:
		return n.Value
	}
	return fmt.Sprintf("%p", e)
}

// neverEmptyCallAgainstEmpty returns side as a never-empty String call when
// other is the `empty` keyword, and nil otherwise.
func neverEmptyCallAgainstEmpty(side, other RobustExpr) *CallExpr {
	if _, ok := unwrapParens(other).(*EmptyExpr); !ok {
		return nil
	}
	call, ok := unwrapParens(side).(*CallExpr)
	if !ok || call.Qualified || !neverEmptyStringFuncs[call.Name] {
		return nil
	}
	return call
}

// checkEnumComparedToString emits E001 for `$obj/EnumAttr = 'Value'`.
//
// This is the same defect checkStringLitVsSlot reports, found a different way.
// That one keys off the *slot* — a create or change member names its attribute,
// so the enum is known before the value is read — which covers assignment and
// nothing else. A comparison has no slot: the only thing that says "this is an
// enumeration" is the other operand, which means resolving an attribute path.
// It is the shape the proposal opens with (`if $Order/Status = 'Open'`) and the
// one a person actually writes.
//
// Same code and message as the slot form on purpose: one defect should not have
// two names depending on where it was spotted.
func checkEnumComparedToString(left, right RobustExpr, ctx Context, opTok Token) []Hint {
	lit, path := pairEnumPathWithStringLit(left, right)
	if lit == nil || path == nil {
		return nil
	}
	enumQN, ok := attributePathEnumQN(path, ctx)
	if !ok {
		return nil
	}
	vals, _ := ctx.Catalog.EnumCases(enumQN)
	return []Hint{{
		Code:     "E001",
		Slug:     "enum-string-mismatch",
		Severity: hints.SeverityError,
		Where:    hintsLocation(ctx, opTok.Pos),
		YouWrote: "'" + lit.Value + "'",
		Problem: "Comparing or assigning an Enumeration attribute against " +
			"a string literal. In Mendix expressions, enumeration values " +
			"must be written as Module.Enum.Value, never as a quoted string.",
		Fix: enumQN + "." + lit.Value,
		Reference: &hints.Reference{
			Enum:          enumQN,
			EnumValues:    vals,
			AttributeName: path.Path[len(path.Path)-1],
		},
	}}
}

// pairEnumPathWithStringLit returns the operands when one side is a string
// literal and the other an attribute path, in either order.
func pairEnumPathWithStringLit(left, right RobustExpr) (*StringLit, *AttributePathExpr) {
	if lit, ok := left.(*StringLit); ok {
		if path, ok := right.(*AttributePathExpr); ok {
			return lit, path
		}
	}
	if lit, ok := right.(*StringLit); ok {
		if path, ok := left.(*AttributePathExpr); ok {
			return lit, path
		}
	}
	return nil, nil
}

func parseAdd(s *Stream, ctx Context) (RobustExpr, []Hint) {
	left, hs := parseMul(s, ctx)
	for s.Peek().Kind == TokPlus || s.Peek().Kind == TokMinus {
		opTok := s.Consume()
		right, h := parseMul(s, ctx)
		hs = append(hs, h...)
		if opTok.Kind == TokPlus {
			lk := inferKind(left, ctx)
			rk := inferKind(right, ctx)
			other := otherKind(lk, rk)
			// E004 fires only when one operand is String and the other is a type
			// that Mendix cannot auto-convert in a + context.
			// Decimal and Integer are auto-converted by the Mendix runtime (verified:
			// mx check accepts "'label' + round(x)" and "'T14' + integer" without CE0117).
			// Only flag truly incompatible types: Boolean, Object, List, Enumeration.
			numericKind := other == KindDecimal || other == KindInteger || other == KindLong
			if (lk == KindString || rk == KindString) && lk != rk &&
				lk != KindUnknown && rk != KindUnknown && !numericKind {
				hs = append(hs, Hint{
					Code: "E004", Slug: "concat-type", Severity: hints.SeverityError,
					Where:    hintsLocation(ctx, opTok.Pos),
					YouWrote: "<left> + <right>",
					Problem: "The '+' operator concatenates Strings. The other operand is " +
						typeKindName(other) +
						", which cannot be concatenated with a String directly.",
					Fix: "Wrap the non-String operand in toString().",
				})
			}
		}
		left = &BinExpr{Op: opTok.Text, L: left, R: right}
	}
	return left, hs
}

func parseMul(s *Stream, ctx Context) (RobustExpr, []Hint) {
	left, hints := parseUnary(s, ctx)
	for {
		t := s.Peek()
		// One left-associative level for every multiplicative operator: `*`,
		// `div`, `:` (division, spelled after ÷) and `mod`.
		isDivMod := t.Kind == TokIdent && (t.Text == "div" || t.Text == "mod")
		if t.Kind != TokStar && t.Kind != TokColon && !isDivMod {
			break
		}
		op := s.Consume().Text
		right, h := parseUnary(s, ctx)
		left = &BinExpr{Op: op, L: left, R: right}
		hints = append(hints, h...)
	}
	return left, hints
}

func parseUnary(s *Stream, ctx Context) (RobustExpr, []Hint) {
	if s.Peek().Kind == TokMinus {
		s.Consume()
		inner, h := parsePrimary(s, ctx)
		return &UnaryExpr{Op: "-", Operand: inner}, h
	}
	return parsePrimary(s, ctx)
}

func parsePrimary(s *Stream, ctx Context) (RobustExpr, []Hint) {
	t := s.Peek()
	switch t.Kind {
	case TokString:
		s.Consume()
		v := t.Text
		if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
			v = v[1 : len(v)-1]
		}
		// Slot checks (E001/E002) are not run here: a literal is only the
		// slot's value in value position, which the parse cannot know yet.
		// checkValueLiterals runs them over the finished tree.
		return &StringLit{baseNode: baseNode{P: t.Pos}, Value: v}, nil
	case TokNumber:
		s.Consume()
		kind := KindInteger
		if strings.Contains(t.Text, ".") {
			kind = KindDecimal
		}
		return &NumberLit{baseNode: baseNode{P: t.Pos}, Value: t.Text, Kind: kind}, nil
	case TokIdent:
		return parseIdentLed(s, ctx)
	case TokDollarIdent:
		return parseDollar(s, ctx)
	case TokAt:
		s.Consume()
		return parseConstantRef(s, ctx, t.Pos)
	case TokToken:
		s.Consume()
		return parseTokenLit(t), nil
	case TokLParen:
		s.Consume()
		inner, hints := parseOr(s, ctx)
		if s.Peek().Kind == TokRParen {
			s.Consume()
		}
		return &ParenExpr{baseNode: baseNode{P: t.Pos}, Inner: inner}, hints
	}
	pos := s.Peek().Pos
	salvage := consumeUntilSafe(s)
	if salvage == "" {
		salvage = s.Consume().Text
	}
	recovered := &RecoveredExpr{
		baseNode:       baseNode{P: pos},
		SourceFragment: salvage,
		Reason:         "unrecognised tokens at primary expression position",
	}
	return recovered, []Hint{{
		Code:     "E007",
		Slug:     "unknown-token",
		Severity: hints.SeverityWarning,
		Where: hints.Location{
			Microflow: ctx.Microflow,
			Context:   SlotToContext(ctx.SlotPath),
			Line:      pos.Line,
			Column:    pos.Column,
		},
		YouWrote: salvage,
		Problem:  "Unrecognised tokens at this position. The parser skipped to the next safe boundary so the rest of the expression could be parsed; additional hints below assume that recovery point.",
		Fix:      "Replace the highlighted fragment with a valid Mendix expression — a literal, variable, qualified name, or function call.",
	}}
}

func parseIdentLed(s *Stream, ctx Context) (RobustExpr, []Hint) {
	t := s.Consume()
	name := t.Text
	switch strings.ToLower(name) {
	case "true":
		return &BoolLit{baseNode: baseNode{P: t.Pos}, Value: true}, nil
	case "false":
		return &BoolLit{baseNode: baseNode{P: t.Pos}, Value: false}, nil
	case "empty":
		return &EmptyExpr{baseNode: baseNode{P: t.Pos}}, nil
	case "null":
		return &EmptyExpr{baseNode: baseNode{P: t.Pos}}, []Hint{{
			Code: "E003", Slug: "null-to-empty", Severity: hints.SeverityWarning,
			Where: hints.Location{
				Microflow: ctx.Microflow,
				Context:   SlotToContext(ctx.SlotPath),
				Line:      t.Pos.Line,
				Column:    t.Pos.Column,
			},
			YouWrote: "null",
			Problem:  "Mendix expressions use 'empty', not 'null'. Tools auto-correct on BSON write but the source becomes inconsistent on the next round-trip.",
			Fix:      "Replace null with empty.",
		}}
	case "if":
		return parseIfThenElse(s, ctx, t.Pos)
	}
	if s.Peek().Kind == TokLParen {
		s.Consume()
		var args []RobustExpr
		var hs []Hint
		if s.Peek().Kind != TokRParen {
			for {
				a, h := parseOr(s, ctx)
				args = append(args, a)
				hs = append(hs, h...)
				if s.Peek().Kind == TokComma {
					s.Consume()
					continue
				}
				break
			}
		}
		if s.Peek().Kind == TokRParen {
			s.Consume()
		}
		node := &CallExpr{baseNode: baseNode{P: t.Pos}, Name: name, Args: args}
		return node, append(hs, checkCallExpr(node, ctx)...)
	}
	if s.Peek().Kind == TokDot {
		s.Consume()
		if s.Peek().Kind != TokIdent {
			return &QNameExpr{baseNode: baseNode{P: t.Pos}, Module: name}, nil
		}
		n2 := s.Consume().Text
		if s.Peek().Kind == TokDot {
			s.Consume()
			if s.Peek().Kind == TokIdent {
				n3 := s.Consume().Text
				return &QNameExpr{baseNode: baseNode{P: t.Pos}, Module: name, Name: n2, Sub: n3}, nil
			}
		}
		if s.Peek().Kind == TokLParen {
			return parseQualifiedCall(s, ctx, t.Pos, name+"."+n2)
		}
		if ctx.XPath && startsXPathPathStep(s) {
			return parseXPathPath(s, ctx, t.Pos, "", []string{name + "." + n2})
		}
		return &QNameExpr{baseNode: baseNode{P: t.Pos}, Module: name, Name: n2}, nil
	}
	if ctx.XPath && startsXPathPathStep(s) {
		return parseXPathPath(s, ctx, t.Pos, "", []string{name})
	}
	// Bare records that this came from a plain identifier rather than
	// `$Name`. The two collapse to the same node, and telling them apart is
	// what makes the bare-word rule possible.
	return &VariableExpr{baseNode: baseNode{P: t.Pos}, Name: name, Bare: true}, nil
}

// parseQualifiedCall consumes the argument list of a `Module.Name(...)` call.
// Nothing in a Mendix expression can call one, so the node exists to keep the
// parse whole: without it the `(` was left on the stream and Parse reported
// "Unexpected token after expression … glued keywords such as 'emptyor'" with an
// empty location — on the valid decision form as much as the invalid ones, so it
// carried no signal and pointed at a typo that was not there (#939).
func parseQualifiedCall(s *Stream, ctx Context, pos Position, name string) (RobustExpr, []Hint) {
	s.Consume() // '('
	var args []RobustExpr
	var hs []Hint
	if s.Peek().Kind != TokRParen {
		for {
			a, h := parseOr(s, ctx)
			args = append(args, a)
			hs = append(hs, h...)
			if s.Peek().Kind == TokComma {
				s.Consume()
				continue
			}
			break
		}
	}
	if s.Peek().Kind == TokRParen {
		s.Consume()
	}
	return &CallExpr{baseNode: baseNode{P: pos}, Name: name, Args: args, Qualified: true}, hs
}

func parseDollar(s *Stream, ctx Context) (RobustExpr, []Hint) {
	t := s.Consume()
	name := strings.TrimPrefix(t.Text, "$")
	if ctx.XPath && s.Peek().Kind == TokSlash {
		return parseXPathPath(s, ctx, t.Pos, name, nil)
	}
	if s.Peek().Kind != TokSlash {
		return &VariableExpr{baseNode: baseNode{P: t.Pos}, Name: name}, nil
	}
	var path []string
	for s.Peek().Kind == TokSlash {
		s.Consume()
		if s.Peek().Kind == TokIdent {
			seg := s.Consume().Text
			for s.Peek().Kind == TokDot {
				s.Consume()
				if s.Peek().Kind != TokIdent {
					break
				}
				seg += "." + s.Consume().Text
			}
			path = append(path, seg)
		} else {
			break
		}
	}
	return &AttributePathExpr{baseNode: baseNode{P: t.Pos}, Variable: name, Path: path}, nil
}

// startsXPathPathStep reports whether the next token continues an XPath path:
// a `/` to the next segment, or a `[` opening a predicate on this one. The
// lexer has no bracket token (`[` is only meaningful as `[%token%]` in an
// expression), so a bare bracket arrives as a one-character TokError.
func startsXPathPathStep(s *Stream) bool {
	t := s.Peek()
	return t.Kind == TokSlash || isBracket(t, "[")
}

func isBracket(t Token, b string) bool { return t.Kind == TokError && t.Text == b }

// parseXPathPath reads the rest of an XPath path whose first segments (if
// any) are already in path: `/`-separated segments, each a possibly
// qualified name, each optionally followed by a `[...]` predicate.
func parseXPathPath(s *Stream, ctx Context, pos Position, variable string, path []string) (RobustExpr, []Hint) {
	var preds []RobustExpr
	var hs []Hint
	hasPred := false
	predicate := func() {
		var pred RobustExpr
		if isBracket(s.Peek(), "[") {
			s.Consume()
			var h []Hint
			pred, h = parseOr(s, ctx)
			hs = append(hs, h...)
			if isBracket(s.Peek(), "]") {
				s.Consume()
			}
			hasPred = true
		}
		preds = append(preds, pred)
	}
	if len(path) > 0 {
		// Only the last segment already read can still take a predicate.
		preds = make([]RobustExpr, len(path)-1)
		predicate()
	}
	for s.Peek().Kind == TokSlash {
		s.Consume()
		if s.Peek().Kind != TokIdent {
			break
		}
		seg := s.Consume().Text
		for s.Peek().Kind == TokDot {
			s.Consume()
			if s.Peek().Kind != TokIdent {
				break
			}
			seg += "." + s.Consume().Text
		}
		path = append(path, seg)
		predicate()
	}
	node := &AttributePathExpr{baseNode: baseNode{P: pos}, Variable: variable, Path: path}
	if hasPred {
		node.Predicates = preds
	}
	return node, hs
}

func parseConstantRef(s *Stream, ctx Context, p Position) (RobustExpr, []Hint) {
	if s.Peek().Kind != TokIdent {
		return &RecoveredExpr{baseNode: baseNode{P: p}, SourceFragment: "@", Reason: "expected qualified name after '@'"}, nil
	}
	parts := []string{s.Consume().Text}
	for s.Peek().Kind == TokDot {
		s.Consume()
		if s.Peek().Kind != TokIdent {
			break
		}
		parts = append(parts, s.Consume().Text)
	}
	return &ConstantRef{baseNode: baseNode{P: p}, QName: strings.Join(parts, ".")}, nil
}

func parseTokenLit(t Token) *TokenExpr {
	inner := strings.TrimPrefix(t.Text, "[%")
	inner = strings.TrimSuffix(inner, "%]")
	arg := ""
	if i := strings.Index(inner, "'"); i >= 0 {
		arg = inner[i:]
		inner = inner[:i]
	}
	return &TokenExpr{baseNode: baseNode{P: t.Pos}, Token: inner, Arg: arg}
}

func parseIfThenElse(s *Stream, ctx Context, p Position) (RobustExpr, []Hint) {
	cond, h1 := parseOr(s, ctx)
	if !matchKeyword(s, "then") {
		return &IfThenElseExpr{baseNode: baseNode{P: p}, Cond: cond}, h1
	}
	thn, h2 := parseOr(s, ctx)
	var els RobustExpr
	var h3 []Hint
	if matchKeyword(s, "else") {
		els, h3 = parseOr(s, ctx)
	}
	return &IfThenElseExpr{baseNode: baseNode{P: p}, Cond: cond, Then: thn, Else: els}, append(append(h1, h2...), h3...)
}

func matchKeyword(s *Stream, kw string) bool {
	t := s.Peek()
	if t.Kind == TokIdent && strings.EqualFold(t.Text, kw) {
		s.Consume()
		return true
	}
	return false
}

// checkValueLiterals runs the slot-literal checks (E002 quoted Boolean, E001
// quoted enumeration value) on the string literals that ARE the slot's value:
// the whole expression, a parenthesised one, or a then/else result —
// recursively, since a result can itself be an if.
//
// They used to run on every string literal the parser met, so in
// `change $Log (Windrichting = if $Dir = 'NW' then E.NW else E.N)` the 'NW'
// compared with a String variable was reported as assigning a string to the
// enumeration, and every literal argument of find(...) as well — an error, so
// exec refused a valid microflow (ako/mxcli#969). A literal in a comparison, a
// function argument or a condition is an operand, not the value; the
// comparison form of the enum mistake has its own check
// (checkEnumComparedToString), which resolves the other operand instead.
func checkValueLiterals(expr RobustExpr, ctx Context) []Hint {
	switch n := expr.(type) {
	case *StringLit:
		return checkValueStringLit(n, ctx)
	case *ParenExpr:
		return checkValueLiterals(n.Inner, ctx)
	case *IfThenElseExpr:
		var hs []Hint
		if n.Then != nil {
			hs = append(hs, checkValueLiterals(n.Then, ctx)...)
		}
		if n.Else != nil {
			hs = append(hs, checkValueLiterals(n.Else, ctx)...)
		}
		return hs
	}
	return nil
}

func checkValueStringLit(node *StringLit, ctx Context) []Hint {
	var hs []Hint
	v := node.Value
	if v == "true" || v == "false" || v == "True" || v == "False" {
		if sc, ok := slotKind(ctx); ok && sc.Kind == KindBoolean {
			hs = append(hs, Hint{
				Code: "E002", Slug: "bool-string-mismatch", Severity: hints.SeverityError,
				Where: hints.Location{
					Microflow: ctx.Microflow,
					Context:   SlotToContext(ctx.SlotPath),
					Line:      node.P.Line,
					Column:    node.P.Column,
				},
				YouWrote: "'" + v + "'",
				Problem:  "Mendix Boolean expressions use the unquoted literals true and false; a quoted string is never equal to a Boolean.",
				Fix:      strings.ToLower(v),
			})
		}
	}
	return append(hs, checkStringLitVsSlot(node, ctx)...)
}

func checkStringLitVsSlot(node *StringLit, ctx Context) []Hint {
	if ctx.Catalog == nil || ctx.SlotPath == "" {
		return nil
	}
	_, qual := splitSlotQual(ctx.SlotPath)
	entity, attr := splitEntityAttr(qual)
	if entity == "" || attr == "" {
		return nil
	}
	kind, ok := ctx.Catalog.AttributeKind(entity, attr)
	if !ok || kind != KindEnumeration {
		return nil
	}
	enumQN, _ := ctx.Catalog.AttributeEnumQN(entity, attr)
	vals, _ := ctx.Catalog.EnumCases(enumQN)
	return []Hint{{
		Code:     "E001",
		Slug:     "enum-string-mismatch",
		Severity: hints.SeverityError,
		Where:    hintsLocation(ctx, node.P),
		YouWrote: "'" + node.Value + "'",
		Problem: "Comparing or assigning an Enumeration attribute against " +
			"a string literal. In Mendix expressions, enumeration values " +
			"must be written as Module.Enum.Value, never as a quoted string.",
		Fix: enumQN + "." + node.Value,
		Reference: &hints.Reference{
			Enum:          enumQN,
			EnumValues:    vals,
			AttributeName: attr,
			EntityType:    entity,
		},
	}}
}

// splitSlotQual splits "<base>:<entity.attr>" into base and qual parts.
// If no ':' is present, qual is empty.
func splitSlotQual(s string) (base, qual string) {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// splitEntityAttr splits "<Module.Entity>.<Attribute>" using the LAST dot:
// "Sales.Customer.Status" → ("Sales.Customer", "Status").
func splitEntityAttr(qual string) (entity, attr string) {
	if i := strings.LastIndexByte(qual, '.'); i > 0 {
		return qual[:i], qual[i+1:]
	}
	return "", ""
}

func slotKind(ctx Context) (SlotConstraint, bool) {
	if ctx.Slots == nil || ctx.SlotPath == "" {
		return SlotConstraint{}, false
	}
	return ctx.Slots.Expect(ctx.SlotPath)
}

func inferKind(e RobustExpr, ctx Context) TypeKind {
	switch n := e.(type) {
	case *StringLit:
		return KindString
	case *NumberLit:
		return n.Kind
	case *BoolLit:
		return KindBoolean
	case *EmptyExpr:
		return KindEmpty
	case *VariableExpr:
		if ctx.Scope != nil {
			if k, ok := ctx.Scope.Lookup(n.Name); ok {
				return k
			}
		}
		// A variable the entity scope knows holds an OBJECT. Without this a
		// `$Customer` infers Unknown, and Unknown is tolerated everywhere — so
		// an object handed to a slot that wants a String went unreported, which
		// is the case mendixlabs/mxcli#1043 was actually filed about.
		if ctx.Entities != nil {
			if _, ok := ctx.Entities.VariableEntity(n.Name); ok {
				return KindObject
			}
		}
	case *CallExpr:
		if sig, ok := funcTable[n.Name]; ok {
			if sig.retFromArgs {
				return callArgsResult(n.Args, ctx)
			}
			return sig.retFor(len(n.Args))
		}
	case *ParenExpr:
		return inferKind(n.Inner, ctx)
	case *BinExpr:
		switch n.Op {
		case "AND", "OR", "=", "!=", "<", "<=", ">", ">=":
			return KindBoolean
		case "div", ":":
			// Mendix division always yields a Decimal, even Integer div Integer.
			// Assigning the result to an Integer/Long fails mx check with CE0117.
			return KindDecimal
		case "+":
			l := inferKind(n.L, ctx)
			r := inferKind(n.R, ctx)
			// String '+' concatenates (Mendix auto-converts a numeric operand).
			if l == KindString || r == KindString {
				return KindString
			}
			return arithResult(l, r)
		case "-", "*", "mod":
			return arithResult(inferKind(n.L, ctx), inferKind(n.R, ctx))
		}
	case *UnaryExpr:
		if n.Op == "NOT" {
			return KindBoolean
		}
		return inferKind(n.Operand, ctx)
	case *IfThenElseExpr:
		if n.Then != nil {
			if k := inferKind(n.Then, ctx); k != KindUnknown {
				return k
			}
		}
		if n.Else != nil {
			return inferKind(n.Else, ctx)
		}
		return KindUnknown
	case *TokenExpr:
		return KindString
	case *AttributePathExpr:
		return attributePathKind(n, ctx)
	case *QNameExpr, *ConstantRef, *RecoveredExpr:
		return KindUnknown
	}
	return KindUnknown
}

// attributePathKind resolves `$Var/Attr` and `$Var/Mod.Assoc/Attr` to the kind
// of the attribute they land on.
//
// Both seams must be present: Entities to type the variable and follow the
// association hops, Catalog to answer what the terminal attribute is. Anything
// unresolvable returns KindUnknown, which suppresses the rule that asked —
// catching less rather than guessing.
func attributePathKind(n *AttributePathExpr, ctx Context) TypeKind {
	entity, ok := pathTargetEntity(n, ctx)
	if !ok {
		return KindUnknown
	}
	kind, ok := ctx.Catalog.AttributeKind(entity, n.Path[len(n.Path)-1])
	if !ok {
		return KindUnknown
	}
	return kind
}

// pathTargetEntity walks everything before the final segment and returns the
// entity that segment is a member of.
func pathTargetEntity(n *AttributePathExpr, ctx Context) (string, bool) {
	if ctx.Entities == nil || ctx.Catalog == nil || n == nil || len(n.Path) == 0 {
		return "", false
	}
	cur, ok := ctx.Entities.VariableEntity(n.Variable)
	if !ok || cur == "" {
		return "", false
	}
	// Every segment but the last is an association hop. Unlike XPath, a Mendix
	// expression does not name the intermediate entity, so each one has to be
	// resolved rather than read off the path.
	for _, seg := range n.Path[:len(n.Path)-1] {
		next, ok := ctx.Entities.AssociationTarget(seg, cur)
		if !ok {
			return "", false
		}
		cur = next
	}
	return cur, true
}

// attributePathEnumQN returns the enumeration a path lands on, when it lands on
// an enumeration attribute.
func attributePathEnumQN(n *AttributePathExpr, ctx Context) (string, bool) {
	entity, ok := pathTargetEntity(n, ctx)
	if !ok {
		return "", false
	}
	attr := n.Path[len(n.Path)-1]
	if kind, ok := ctx.Catalog.AttributeKind(entity, attr); !ok || kind != KindEnumeration {
		return "", false
	}
	return ctx.Catalog.AttributeEnumQN(entity, attr)
}

// checkBoolOperand emits E009 when expr's inferred kind is known and non-Boolean.
// op is the operator keyword ("not", "and", "or") used in the hint message.
func checkBoolOperand(expr RobustExpr, ctx Context, op string) []Hint {
	k := inferKind(expr, ctx)
	if k == KindUnknown || k == KindBoolean {
		return nil
	}
	return []Hint{{
		Code:     "E009",
		Slug:     "slot-type-mismatch",
		Severity: hints.SeverityError,
		Where:    hintsLocation(ctx, expr.Pos()),
		YouWrote: op + " <" + typeKindName(k) + ">",
		Problem: "'" + op + "' requires a Boolean operand, but this expression has kind " +
			typeKindName(k) + ".",
		Fix: "Replace the operand with a Boolean expression " +
			"(e.g. a comparison, a Boolean attribute path, or true/false).",
	}}
}

func hintsLocation(ctx Context, pos Position) hints.Location {
	return hints.Location{
		File:      ctx.File,
		Line:      pos.Line,
		Column:    pos.Column,
		Microflow: ctx.Microflow,
		Context:   SlotToContext(ctx.SlotPath),
	}
}

func otherKind(l, r TypeKind) TypeKind {
	if l == KindString {
		return r
	}
	return l
}

// callArgsResult folds arithResult over a call's arguments, for the built-ins
// that preserve their operands' numeric kind (abs, max, min). It inherits
// arithResult's conservatism: one argument of unknown kind makes the whole
// result unknown, so a caller can never prove an assignment wrong from it.
func callArgsResult(args []RobustExpr, ctx Context) TypeKind {
	if len(args) == 0 {
		return KindUnknown
	}
	k := inferKind(args[0], ctx)
	for _, a := range args[1:] {
		k = arithResult(k, inferKind(a, ctx))
	}
	return k
}

// arithResult returns the Mendix result kind of a non-division arithmetic
// operation (+, -, *, mod) from its operand kinds. Decimal is contagious (a
// Decimal operand makes the whole expression Decimal), then Long, then Integer
// when both sides are Integer. Any operand of unknown kind yields KindUnknown so
// callers stay conservative and never flag an assignment they can't prove wrong.
func arithResult(l, r TypeKind) TypeKind {
	if l == KindDecimal || r == KindDecimal {
		return KindDecimal
	}
	if l == KindLong || r == KindLong {
		if l == KindUnknown || r == KindUnknown {
			return KindUnknown
		}
		return KindLong
	}
	if l == KindInteger && r == KindInteger {
		return KindInteger
	}
	return KindUnknown
}
