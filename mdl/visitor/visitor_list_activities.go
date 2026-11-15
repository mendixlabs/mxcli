// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// List operations and aggregates are statements that mirror Studio Pro's
// activities: one "List operation" or "Aggregate list" activity per statement,
// named after the operation and taking the inputs its dialog asks for
// (PROPOSAL_mdl_beta_syntax_freeze.md §4 and §5 item 3, #733):
//
//	$Open = filter $Orders by Status = Shop.Status.Open;
//	$N    = count $Open;
//
// The operand is always a variable, so one activity cannot be nested inside
// another. The call form (`filter($Orders, …)`) keeps parsing:
//
//   - for every operation except find and contains it is a respelling, a
//     deprecated alias (MDL-DEPR003 / MDL-DEPR004) that builds the same AST;
//   - `find(…)` and `contains(…)` are also Mendix's string functions, and which
//     one a call means is guessed today from its arguments. That is not a
//     respelling, so it is version-gated (listCallForm): kept and warned under
//     mdl 0, refused under mdl 1;
//   - so is a call after `set`, or nested in another call: under mdl 0 it is
//     turned into an activity (and a nested operand is refused by MDL-LISTOP02),
//     under mdl 1 `set` always assigns an expression and such a call is refused.
//
// `set` is mandatory for reassignment under mdl 1 (setIsMandatory), which is
// what removes the ambiguity: `$x = …` without it is only ever an activity.

// listCallForm is the change of meaning for list operations and aggregates
// written as calls where the call is not a plain respelling of one activity.
var listCallForm = langver.Change{
	Code:  "MDL-V1-LIST",
	Since: langver.V1,
	Old: "a list operation or aggregate written as a call — `find(…)` or `contains(…)`, " +
		"a call after `set`, or one call nested in another — is turned into a List operation " +
		"or Aggregate list activity, guessing from its arguments whether a string function was meant,",
	New: "an error: a List operation or Aggregate list is one statement per activity whose operand is " +
		"a variable (`$x = find $L where …;`, `$n = count $L;`), and after `set` a call is always " +
		"the expression function",
}

// setIsMandatory is the new rejection of a reassignment without `set`.
var setIsMandatory = langver.Change{
	Code:  "MDL-V1-SET",
	Since: langver.V1,
	Old:   "`$x = <expression>` without `set` changes the variable",
	New:   "an error: a reassignment is written `set $x = <expression>;`",
}

// scriptLanguageVersion reads the `mdl <n>;` header of the script that tree is
// part of. The header can only be the first statement, so it is on the root.
//
// It exists for the statement builders, which are free functions without the
// Builder: they must build the AST of the version the script is written in.
// A missing or malformed header is mdl 0; a malformed one is reported by
// ExitLanguageHeader.
func scriptLanguageVersion(tree antlr.Tree) langver.Version {
	for tree != nil {
		if prog, ok := tree.(*parser.ProgramContext); ok {
			h, ok := prog.LanguageHeader().(*parser.LanguageHeaderContext)
			if !ok || h == nil || h.IDENTIFIER() == nil || h.NUMBER_LITERAL() == nil ||
				!strings.EqualFold(h.IDENTIFIER().GetText(), "mdl") {
				return langver.V0
			}
			n, err := strconv.Atoi(h.NUMBER_LITERAL().GetText())
			if v := langver.Version(n); err == nil && v.Known() {
				return v
			}
			return langver.V0
		}
		tree = tree.GetParent()
	}
	return langver.V0
}

// buildListOperationActivity fills stmt from the statement form.
func buildListOperationActivity(act *parser.ListOperationActivityContext, stmt *ast.ListOperationStmt) {
	vars := act.AllVARIABLE()
	varName := func(i int) string {
		if i < len(vars) {
			return strings.TrimPrefix(vars[i].GetText(), "$")
		}
		return ""
	}
	stmt.InputVariable = varName(0)
	condition := func() {
		c, ok := act.ListOperationCondition().(*parser.ListOperationConditionContext)
		if !ok || c == nil {
			return
		}
		stmt.Condition = buildSourceExpression(c.Expression())
		stmt.ByExpression = c.WHERE() != nil
		if !stmt.ByExpression {
			// `by Number = $N-1`: the value keeps its spelling, the condition
			// stays a member equality (ako/mxcli#1943).
			stmt.Condition = ast.MemberEqualityCondition(stmt.Condition)
		}
	}
	switch {
	case act.HEAD() != nil:
		stmt.Operation = ast.ListOpHead
	case act.TAIL() != nil:
		stmt.Operation = ast.ListOpTail
	case act.FIND() != nil:
		stmt.Operation = ast.ListOpFind
		condition()
	case act.FILTER() != nil:
		stmt.Operation = ast.ListOpFilter
		condition()
	case act.SORT() != nil:
		stmt.Operation = ast.ListOpSort
		for _, item := range act.AllListSortItem() {
			it := item.(*parser.ListSortItemContext)
			stmt.SortSpecs = append(stmt.SortSpecs, ast.SortSpec{
				Attribute: identifierOrKeywordText(it.IdentifierOrKeyword()),
				Ascending: it.DESC() == nil,
			})
		}
	case act.UNION() != nil:
		stmt.Operation, stmt.SecondVariable = ast.ListOpUnion, varName(1)
	case act.INTERSECT() != nil:
		stmt.Operation, stmt.SecondVariable = ast.ListOpIntersect, varName(1)
	case act.SUBTRACT() != nil:
		// subtract $B from $A: the result is A minus B, and A is the first list.
		stmt.Operation, stmt.InputVariable, stmt.SecondVariable = ast.ListOpSubtract, varName(1), varName(0)
	case act.CONTAINS() != nil:
		// contains $Object in $List: the list is the activity's first operand.
		stmt.Operation, stmt.InputVariable, stmt.SecondVariable = ast.ListOpContains, varName(1), varName(0)
	case act.EQUALS_OP() != nil:
		stmt.Operation, stmt.SecondVariable = ast.ListOpEquals, varName(1)
	case act.RANGE() != nil:
		stmt.Operation = ast.ListOpRange
		if act.OFFSET() != nil {
			stmt.OffsetExpr = buildSourceExpression(act.Expression(0))
		}
		if act.LIMIT() != nil {
			stmt.LimitExpr = buildSourceExpression(act.Expression(len(act.AllExpression()) - 1))
		}
	}
}

// buildAggregateListActivity fills stmt from the statement form.
func buildAggregateListActivity(act *parser.AggregateListActivityContext, stmt *ast.AggregateListStmt) {
	if v := act.VARIABLE(); v != nil {
		stmt.InputVariable = strings.TrimPrefix(v.GetText(), "$")
	}
	exprs := act.AllExpression()
	switch {
	case act.COUNT() != nil:
		stmt.Operation = ast.AggregateCount
		return
	case act.SUM() != nil:
		stmt.Operation = ast.AggregateSum
	case act.AVERAGE() != nil:
		stmt.Operation = ast.AggregateAverage
	case act.MINIMUM() != nil:
		stmt.Operation = ast.AggregateMinimum
	case act.MAXIMUM() != nil:
		stmt.Operation = ast.AggregateMaximum
	case act.ALL() != nil, act.ANY() != nil:
		stmt.Operation = ast.AggregateAll
		if act.ANY() != nil {
			stmt.Operation = ast.AggregateAny
		}
		stmt.IsExpression = true
		if len(exprs) > 0 {
			stmt.Expression = buildSourceExpression(exprs[0])
		}
		return
	case act.REDUCE() != nil:
		// reduce $L from <initial> as <type> using <expression>
		stmt.Operation = ast.AggregateReduce
		stmt.IsExpression = true
		if len(exprs) > 0 {
			stmt.InitialValue = buildSourceExpression(exprs[0])
		}
		if len(exprs) > 1 {
			stmt.Expression = buildSourceExpression(exprs[1])
		}
		if dt := act.DataType(); dt != nil {
			t := buildDataType(dt)
			stmt.ReturnType = &t
		}
		return
	}
	// sum / average / minimum / maximum: `by Attribute` or `of <expression>`.
	if act.OF() != nil && len(exprs) > 0 {
		stmt.IsExpression = true
		stmt.Expression = buildSourceExpression(exprs[0])
	} else if id := act.IdentifierOrKeyword(); id != nil {
		stmt.Attribute = identifierOrKeywordText(id)
	}
}

// unwrapSource returns the expression a SourceExpr carries.
func unwrapSource(e ast.Expression) ast.Expression {
	if s, ok := e.(*ast.SourceExpr); ok {
		return s.Expression
	}
	return e
}

// listCallName returns the upper-cased name of a list-operation or aggregate
// call, or "" when value is not one.
func listCallName(value ast.Expression) string {
	call, ok := unwrapSource(value).(*ast.FunctionCallExpr)
	if !ok {
		return ""
	}
	switch name := strings.ToUpper(call.Name); name {
	case "HEAD", "TAIL", "FIND", "FILTER", "SORT", "UNION", "INTERSECT", "SUBTRACT",
		"CONTAINS", "EQUALS", "RANGE", "COUNT", "SUM", "AVERAGE", "MINIMUM", "MAXIMUM":
		return name
	}
	return ""
}

func isStringOverload(name string) bool { return name == "FIND" || name == "CONTAINS" }

// ExitListOperationStatement reports the call form: a deprecated alias, or for
// find/contains a version-gated construct; and checks that `by` names a member.
func (b *Builder) ExitListOperationStatement(ctx *parser.ListOperationStatementContext) {
	target := ""
	if v := ctx.VARIABLE(); v != nil {
		target = v.GetText()
	}
	if op, ok := ctx.ListOperation().(*parser.ListOperationContext); ok && op != nil {
		byExpression := buildListOperationStatement(ctx).ByExpression
		if op.FIND() != nil || op.CONTAINS() != nil {
			if b.gate(listCallForm, ctx) {
				b.addError(findContainsCallError(ctx.GetStart().GetLine(), target, op.FIND() != nil))
				return
			}
			// Under mdl 0 the flow builder turns a String operand into the
			// string function: `set` says so under mdl 1. Any other operand
			// is the List operation, the statement form.
			fn := strings.ToLower(op.GetStart().GetText())
			stringFix := &ast.Fix{Edits: []ast.TextEdit{insertAt(ctx.GetStart().GetStart(), "set ")}}
			listFix, listWhy := fixOrReason(callFormFix(op, byExpression))
			switch isString, known, pending, why := b.operandReading(ctx, op.VARIABLE(0).GetText(), fn); {
			case !known:
				b.fixLastNote(listCallForm.Code, nil, why)
				if pending != nil {
					pending.StringFix, pending.ListFix, pending.ListNoFix = stringFix, listFix, listWhy
					b.chooseLastNote(listCallForm.Code, pending)
				}
			case isString:
				b.fixLastNote(listCallForm.Code, stringFix, "")
			default:
				b.fixLastNote(listCallForm.Code, listFix, listWhy)
			}
			return
		}
		b.recordDeprecation(deprecation.ListOperationFunctionForm, op.GetStart(), strings.ToLower(op.GetStart().GetText()))
		fix, why := fixOrReason(callFormFix(op, byExpression))
		b.fixLastDeprecation(deprecation.ListOperationFunctionForm, fix, why)
		return
	}
	act, ok := ctx.ListOperationActivity().(*parser.ListOperationActivityContext)
	if !ok || act == nil {
		return
	}
	c, ok := act.ListOperationCondition().(*parser.ListOperationConditionContext)
	if !ok || c == nil || c.BY() == nil {
		return
	}
	if !ast.IsMemberEquality(unwrapSource(buildExpression(c.Expression()))) {
		b.addError(fmt.Errorf("line %d: `by` names a member and the value it must have, `by Member = value`; "+
			"`%s` is not of that shape. For any other condition write `where <expression>`, which is "+
			"Studio Pro's \"by expression\" operation and uses $currentObject",
			c.GetStart().GetLine(), strings.TrimSpace(extractExpressionText(c.Expression()))))
	}
}

func findContainsCallError(line int, target string, find bool) error {
	if find {
		return fmt.Errorf("line %d: `%s = find(…)` is refused under mdl 1: the call is both the string function "+
			"and the List operation, one statement per activity. For the string function write "+
			"`set %s = find(…);` (the variable is declared first); for the List operation write "+
			"`%s = find $List by Member = value;` or `%s = find $List where <expression>;`",
			line, target, target, target, target)
	}
	return fmt.Errorf("line %d: `%s = contains(…)` is refused under mdl 1: the call is both the string function "+
		"and the List operation, one statement per activity. For the string function write "+
		"`set %s = contains(…);` (the variable is declared first); for the List operation write "+
		"`%s = contains $Object in $List;`", line, target, target, target)
}

// ExitAggregateListStatement reports the call form, a deprecated alias.
func (b *Builder) ExitAggregateListStatement(ctx *parser.AggregateListStatementContext) {
	if op, ok := ctx.ListAggregateOperation().(*parser.ListAggregateOperationContext); ok && op != nil {
		b.recordDeprecation(deprecation.AggregateFunctionForm, op.GetStart(), strings.ToLower(op.GetStart().GetText()))
		fix, why := fixOrReason(callFormFix(op, false))
		b.fixLastDeprecation(deprecation.AggregateFunctionForm, fix, why)
	}
}

// ExitSetStatement gates the two constructs whose meaning mdl 1 changes: an
// assignment without `set`, and a list-operation or aggregate call as the value.
func (b *Builder) ExitSetStatement(ctx *parser.SetStatementContext) {
	v := ctx.VARIABLE()
	if v == nil || ctx.Expression() == nil {
		// An attribute target (`$o/A = v`) is a respelling of `change`, not a
		// reassignment; it is left to that alias.
		return
	}
	target := v.GetText()
	line := ctx.GetStart().GetLine()
	value := buildExpression(ctx.Expression())
	name := listCallName(value)
	valueText := strings.TrimSpace(extractExpressionText(ctx.Expression()))

	switch {
	case name != "" && !(isStringOverload(name) && ctx.SET() != nil):
		// A call the mdl 0 builder turns into an activity (when it does).
		if call, ok := unwrapSource(value).(*ast.FunctionCallExpr); ok &&
			buildListOrAggregateStatement(strings.TrimPrefix(target, "$"), call) == nil {
			// The string reading of find/contains: an expression either way.
			break
		}
		if b.gate(listCallForm, ctx) {
			if isStringOverload(name) {
				b.addError(findContainsCallError(line, target, name == "FIND"))
				return
			}
			b.addError(fmt.Errorf("line %d: `%s = %s` is refused under mdl 1: a list operation or aggregate is "+
				"one statement per activity, and its operand is a variable, never another call or an "+
				"expression. Write `%s = %s $List …;`, with each inner call as a statement of its own "+
				"(e.g. `$Open = filter $Orders where …;` then `$N = count $Open;`)",
				line, target, valueText, target, strings.ToLower(name)))
			return
		}
		fix, why, pending := b.setCallFix(ctx, value, isStringOverload(name))
		b.fixLastNote(listCallForm.Code, fix, why)
		b.chooseLastNote(listCallForm.Code, pending)
		return
	case name != "" && ctx.SET() != nil:
		// `set $x = find(…)` / `contains(…)`: under mdl 1 always the string
		// function; under mdl 0 an activity when the arguments look like one.
		if call, ok := unwrapSource(value).(*ast.FunctionCallExpr); ok &&
			buildListOrAggregateStatement(strings.TrimPrefix(target, "$"), call) != nil && !b.gate(listCallForm, ctx) {
			fix, why, pending := b.setCallFix(ctx, value, true)
			b.fixLastNote(listCallForm.Code, fix, why)
			b.chooseLastNote(listCallForm.Code, pending)
		}
		return
	}
	if ctx.SET() == nil {
		if b.gate(setIsMandatory, ctx) {
			b.addError(fmt.Errorf("line %d: a reassignment says `set` under mdl 1: write `set %s = %s;`, "+
				"or `declare %s <Type> = %s;` if %s is a new variable. "+
				"A List operation or Aggregate list activity is written without it, as its own statement "+
				"(`%s = filter $List where …;`)", line, target, valueText, target, valueText, target, target))
			return
		}
		b.fixLastNote(setIsMandatory.Code, &ast.Fix{Edits: []ast.TextEdit{insertAt(ctx.GetStart().GetStart(), "set ")}}, "")
	}
}

// setCallFix is the rewrite of an assignment whose value is a list-operation or
// aggregate call that mdl 0 turns into an activity (MDL-V1-LIST): the statement
// form, without `set`. For find and contains the activity is what mdl 0 builds
// only when the operand is not a String; for a String it is the string
// function, which is what `set $x = find(…)` means under mdl 1, so a script
// already written that way needs no edit. When the operand's type waits on
// the project, the choice between the two is returned instead of a fix.
func (b *Builder) setCallFix(ctx *parser.SetStatementContext, value ast.Expression, overloaded bool) (*ast.Fix, string, *ast.OperandChoice) {
	op := singleListCall(ctx.Expression())
	if op == nil {
		return nil, "the operand is not a variable (a nested call or an expression), and one activity takes a " +
			"variable: write each inner call as a statement of its own", nil
	}
	listFix := func() (*ast.Fix, string) {
		byExpression := false
		if call, ok := unwrapSource(value).(*ast.FunctionCallExpr); ok {
			if lo, ok := buildListOrAggregateStatement(strings.TrimPrefix(ctx.VARIABLE().GetText(), "$"), call).(*ast.ListOperationStmt); ok {
				byExpression = lo.ByExpression
			}
		}
		edits, why := callFormFix(op, byExpression)
		if why != "" {
			return nil, why
		}
		if set := ctx.SET(); set != nil {
			edits = append(edits, ast.TextEdit{Start: set.GetSymbol().GetStart(), Stop: ctx.VARIABLE().GetSymbol().GetStart()})
		}
		return &ast.Fix{Edits: edits}, ""
	}
	if !overloaded {
		fix, why := listFix()
		return fix, why, nil
	}
	lo, ok := op.(*parser.ListOperationContext)
	if !ok || lo.VARIABLE(0) == nil {
		return nil, "the call has no list operand", nil
	}
	stringFix := &ast.Fix{}
	if ctx.SET() == nil {
		stringFix.Edits = []ast.TextEdit{insertAt(ctx.GetStart().GetStart(), "set ")}
	}
	v, fn := lo.VARIABLE(0), strings.ToLower(op.GetStart().GetText())
	isString, known, pending, why := b.operandReading(ctx, v.GetText(), fn)
	switch {
	case !known:
		if pending != nil {
			pending.StringFix = stringFix
			pending.ListFix, pending.ListNoFix = listFix()
		}
		return nil, why, pending
	case isString:
		return stringFix, "", nil
	}
	fix, why := listFix()
	return fix, why, nil
}
