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
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck/hints"
)

type parserImpl struct{}

func NewParser() Parser { return &parserImpl{} }

func (p *parserImpl) Parse(src string, ctx Context) (RobustExpr, []Hint) {
	s := NewStream(Lex(src))
	expr, hs := parseOr(s, ctx)
	// Detect unconsumed trailing tokens (e.g. "emptyor" parsed as a variable,
	// leaving "$X = ''" silently abandoned). This indicates a structural parse
	// error — most commonly a keyword glued to an adjacent token without whitespace.
	// TokError tokens (unrecognised characters such as ':') are excluded: the
	// parser's E007 recovery already handles them inline; re-reporting them here
	// would produce false positives for valid expressions that use characters
	// the lexer does not model (e.g. "$Total : $Count" with Mendix ':' division).
	if t := s.Peek(); t.Kind != TokEOF && t.Kind != TokError {
		hs = append(hs, hints.Hint{
			Severity: hints.SeverityError,
			Where: hints.Location{
				Line:   t.Pos.Line,
				Column: t.Pos.Column,
			},
			YouWrote: t.Text,
			Problem:  "Unexpected token after expression — the expression appears incomplete or malformed (possible missing space between keywords).",
			Fix:      "Check for glued keywords such as 'emptyor' (should be 'empty or') or 'andtrue' (should be 'and true').",
		})
	}
	return expr, hs
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
	s.Consume()
	right, h := parseAdd(s, ctx)
	return &BinExpr{Op: op, L: left, R: right}, append(hints, h...)
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
		isDivMod := t.Kind == TokIdent && (t.Text == "div" || t.Text == "mod")
		if t.Kind != TokStar && !isDivMod {
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
		node := &StringLit{baseNode: baseNode{P: t.Pos}, Value: v}
		var hs []Hint
		if v == "true" || v == "false" || v == "True" || v == "False" {
			if sc, ok := slotKind(ctx); ok && sc.Kind == KindBoolean {
				hs = append(hs, Hint{
					Code: "E002", Slug: "bool-string-mismatch", Severity: hints.SeverityError,
					Where: hints.Location{
						Microflow: ctx.Microflow,
						Context:   SlotToContext(ctx.SlotPath),
						Line:      t.Pos.Line,
						Column:    t.Pos.Column,
					},
					YouWrote: "'" + v + "'",
					Problem:  "Mendix Boolean expressions use the unquoted literals true and false; a quoted string is never equal to a Boolean.",
					Fix:      strings.ToLower(v),
				})
			}
		}
		hs = append(hs, checkStringLitVsSlot(node, ctx, t)...)
		return node, hs
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
	return &RecoveredExpr{
			baseNode:       baseNode{P: pos},
			SourceFragment: salvage,
			Reason:         "unrecognised tokens at primary expression position",
		}, []Hint{{
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
		return &QNameExpr{baseNode: baseNode{P: t.Pos}, Module: name, Name: n2}, nil
	}
	return &VariableExpr{baseNode: baseNode{P: t.Pos}, Name: name}, nil
}

func parseDollar(s *Stream, ctx Context) (RobustExpr, []Hint) {
	t := s.Consume()
	name := strings.TrimPrefix(t.Text, "$")
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

func checkStringLitVsSlot(node *StringLit, ctx Context, tok Token) []Hint {
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
		Where:    hintsLocation(ctx, tok.Pos),
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
	case *CallExpr:
		if sig, ok := funcTable[n.Name]; ok {
			return sig.ret
		}
	case *ParenExpr:
		return inferKind(n.Inner, ctx)
	case *BinExpr:
		switch n.Op {
		case "AND", "OR", "=", "!=", "<", "<=", ">", ">=":
			return KindBoolean
		case "div":
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
	case *AttributePathExpr, *QNameExpr, *ConstantRef, *RecoveredExpr:
		return KindUnknown
	}
	return KindUnknown
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
