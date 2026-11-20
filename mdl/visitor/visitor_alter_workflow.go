// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// `alter workflow` on the generic ALTER (ADR-0012 decision 2, ako/mxcli#712):
//
//	alter workflow M.W {
//	  set ( Key: value, … ) [on <activity>];
//	  insert before|after <activity> { <activities> }
//	  insert into <activity> { outcomes … | path { … } | boundary event … }
//	  replace <activity> with { <activities> }
//	  drop <activity> [outcome 'X' | outcome true | path n | boundary event];
//	}
//
// It builds the same operations the old per-action form builds — each old
// action is a registered alias of one generic operation — so the executor has
// one path for both, and the old form's tests exercise it.

// workflowPropertyKeys maps a `set ( … )` key, lower-cased, to the property a
// SetWorkflowPropertyOp names; activityPropertyKeys does the same for `set
// ( … ) on <activity>`.
var workflowPropertyKeys = map[string]string{
	"display":      "display",
	"description":  "description",
	"exportlevel":  "export_level",
	"duedate":      "due_date",
	"overviewpage": "overview_page",
	"parameter":    "parameter",
	"annotation":   "annotation",
}

var activityPropertyKeys = map[string]string{
	"page":        "page",
	"description": "description",
	"taskname":    "task_name",
	"targeting":   "targeting",
	"duedate":     "due_date",
	"annotation":  "annotation",
}

const (
	workflowKeysText = "Display, Description, ExportLevel, DueDate, OverviewPage, Parameter, Annotation"
	activityKeysText = "Page, Description, TaskName, Targeting, DueDate, Annotation"
)

// buildAlterWorkflowOperation builds one generic operation. A `set` list and a
// `drop` list build one operation per key or target, as the old form wrote
// them one per action; `insert into` builds one per member.
func (b *Builder) buildAlterWorkflowOperation(ctx *parser.AlterWorkflowOperationContext) []ast.AlterWorkflowOp {
	switch {
	case ctx.SET() != nil:
		return b.buildAlterWorkflowSet(ctx)
	case ctx.INSERT() != nil && ctx.INTO() != nil:
		ref, pos, ok := b.alterWorkflowActivityRef(ctx.AlterTarget())
		if !ok {
			return nil
		}
		var ops []ast.AlterWorkflowOp
		for _, m := range ctx.AllAlterWorkflowMember() {
			ops = append(ops, buildAlterWorkflowMember(m.(*parser.AlterWorkflowMemberContext), ref, pos)...)
		}
		return ops
	case ctx.INSERT() != nil:
		ref, pos, ok := b.alterWorkflowActivityRef(ctx.AlterTarget())
		if !ok {
			return nil
		}
		acts := alterWorkflowFragmentActivities(ctx.AlterWorkflowFragment())
		if ctx.BEFORE() != nil {
			return []ast.AlterWorkflowOp{&ast.InsertBeforeOp{ActivityRef: ref, AtPosition: pos, NewActivities: acts}}
		}
		return []ast.AlterWorkflowOp{&ast.InsertAfterOp{ActivityRef: ref, AtPosition: pos, NewActivities: acts}}
	case ctx.REPLACE() != nil:
		ref, pos, ok := b.alterWorkflowActivityRef(ctx.AlterTarget())
		if !ok {
			return nil
		}
		return []ast.AlterWorkflowOp{&ast.ReplaceActivityOp{ActivityRef: ref, AtPosition: pos,
			NewActivities: alterWorkflowFragmentActivities(ctx.AlterWorkflowFragment())}}
	case ctx.DROP() != nil:
		var ops []ast.AlterWorkflowOp
		for _, t := range ctx.AllAlterWorkflowTarget() {
			if op := b.buildAlterWorkflowDrop(t.(*parser.AlterWorkflowTargetContext)); op != nil {
				ops = append(ops, op)
			}
		}
		return ops
	}
	return nil
}

// alterWorkflowActivityRef reads a generic target as the activity address the
// workflow operations carry: a name or a caption, and @n. A workflow activity
// has no members to dot into and no grid columns, so those forms are refused
// here, where the author wrote them.
func (b *Builder) alterWorkflowActivityRef(ctx parser.IAlterTargetContext) (string, int, bool) {
	if ctx == nil {
		return "", 0, false
	}
	ref := b.buildAlterTarget(ctx)
	if ref.IsColumnAddress() || ref.Column != "" || ref.Item != nil {
		b.addError(fmt.Errorf("alter workflow target %s: a workflow activity is addressed by its name "+
			"(`ReviewOrder`) or its caption (`'Review the order'`), with @n to choose one of several "+
			"matches — `describe workflow` prints every activity's name", ref.Name()))
		return "", 0, false
	}
	if ref.Caption != "" {
		return ref.Caption, ref.Ordinal, true
	}
	return ref.Widget, ref.Ordinal, true
}

// alterWorkflowFragmentActivities builds a `{ … }` fragment's activities,
// exactly as an outcome's body in `create workflow` is built.
func alterWorkflowFragmentActivities(ctx parser.IAlterWorkflowFragmentContext) []ast.WorkflowActivityNode {
	if ctx == nil {
		return nil
	}
	return buildWorkflowBody(ctx.(*parser.AlterWorkflowFragmentContext).WorkflowBody())
}

// buildAlterWorkflowMember builds what `insert into <activity> { … }` adds:
// user-task outcomes (`'X' { … }`), condition outcomes (`true -> { … }`), a
// parallel path, or a boundary event — each the operation the old `insert
// outcome` / `insert condition` / `insert path` / `insert boundary event`
// built.
func buildAlterWorkflowMember(ctx *parser.AlterWorkflowMemberContext, ref string, pos int) []ast.AlterWorkflowOp {
	switch {
	case ctx.OUTCOMES() != nil:
		var ops []ast.AlterWorkflowOp
		for _, c := range ctx.GetChildren() {
			switch o := c.(type) {
			case *parser.WorkflowUserTaskOutcomeContext:
				ops = append(ops, &ast.InsertOutcomeOp{
					OutcomeName: unquoteStringLit(o.STRING_LITERAL()),
					ActivityRef: ref, AtPosition: pos,
					Activities: buildWorkflowBody(o.WorkflowBody()),
				})
			case *parser.WorkflowConditionOutcomeContext:
				ops = append(ops, &ast.InsertBranchOp{
					Condition:   conditionOutcomeText(o),
					ActivityRef: ref, AtPosition: pos,
					Activities: buildWorkflowBody(o.WorkflowBody()),
				})
			}
		}
		return ops
	case ctx.PATH() != nil:
		op := &ast.InsertPathOp{ActivityRef: ref, AtPosition: pos, Activities: buildWorkflowBody(ctx.WorkflowBody())}
		if n := ctx.NUMBER_LITERAL(); n != nil {
			op.PathNumber, _ = strconv.Atoi(n.GetText())
		}
		return []ast.AlterWorkflowOp{op}
	case ctx.BOUNDARY() != nil:
		be := buildBoundaryEventNode(ctx.WorkflowBoundaryEventClause())
		return []ast.AlterWorkflowOp{&ast.InsertBoundaryEventOp{
			ActivityRef: ref, AtPosition: pos,
			EventType: be.EventType, Delay: be.Delay, Caption: be.Caption, Recurrence: be.Recurrence, Activities: be.Activities,
		}}
	}
	return nil
}

// conditionOutcomeText is the condition an InsertBranchOp carries: `true`,
// `false` and `default` as those words, any other outcome by its value — what
// the old `insert condition '<value>'` carried as its string.
func conditionOutcomeText(o *parser.WorkflowConditionOutcomeContext) string {
	switch {
	case o.TRUE() != nil:
		return "true"
	case o.FALSE() != nil:
		return "false"
	case o.DEFAULT() != nil:
		return "default"
	case o.STRING_LITERAL() != nil:
		return unquoteStringLit(o.STRING_LITERAL())
	}
	return ""
}

// buildAlterWorkflowDrop builds one drop target: an activity, or a member of
// one. `outcome 'X'` is an outcome by value (the old `drop outcome`);
// `outcome true|false|default` a decision's (the old `drop condition`).
func (b *Builder) buildAlterWorkflowDrop(ctx *parser.AlterWorkflowTargetContext) ast.AlterWorkflowOp {
	ref, pos, ok := b.alterWorkflowActivityRef(ctx.AlterTarget())
	if !ok {
		return nil
	}
	mc := ctx.AlterWorkflowTargetMember()
	if mc == nil {
		return &ast.DropActivityOp{ActivityRef: ref, AtPosition: pos}
	}
	m := mc.(*parser.AlterWorkflowTargetMemberContext)
	switch {
	case m.OUTCOME() != nil && m.STRING_LITERAL() != nil:
		return &ast.DropOutcomeOp{OutcomeName: unquoteStringLit(m.STRING_LITERAL()), ActivityRef: ref, AtPosition: pos}
	case m.OUTCOME() != nil:
		name := "default"
		if m.TRUE() != nil {
			name = "true"
		} else if m.FALSE() != nil {
			name = "false"
		}
		return &ast.DropBranchOp{BranchName: name, ActivityRef: ref, AtPosition: pos}
	case m.PATH() != nil:
		return &ast.DropPathOp{PathCaption: "Path " + m.NUMBER_LITERAL().GetText(), ActivityRef: ref, AtPosition: pos}
	case m.BOUNDARY() != nil:
		op := &ast.DropBoundaryEventOp{ActivityRef: ref, AtPosition: pos}
		if n := m.NUMBER_LITERAL(); n != nil {
			idx, err := strconv.Atoi(n.GetText())
			if err != nil || idx < 1 {
				tok := n.GetSymbol()
				b.addError(fmt.Errorf("line %d:%d `boundary event %s`: the number is the event's place, from 1, "+
					"in the order describe workflow prints them", tok.GetLine(), tok.GetColumn(), n.GetText()))
				return nil
			}
			op.Index = idx
		}
		return op
	}
	return nil
}

// buildAlterWorkflowSet builds `set ( Key: value, … ) [on <activity>]`: one
// SetWorkflowPropertyOp per key without a target, one SetActivityPropertyOp
// per key with one — the operations the old `set <prop> v` and `set activity
// X <prop> v` built.
func (b *Builder) buildAlterWorkflowSet(ctx *parser.AlterWorkflowOperationContext) []ast.AlterWorkflowOp {
	onActivity := ctx.ON() != nil
	var ref string
	var pos int
	if onActivity {
		var ok bool
		if ref, pos, ok = b.alterWorkflowActivityRef(ctx.AlterTarget()); !ok {
			return nil
		}
	}
	var ops []ast.AlterWorkflowOp
	for _, a := range ctx.AllAlterWorkflowAssignment() {
		ac := a.(*parser.AlterWorkflowAssignmentContext)
		key := identifierOrKeywordText(ac.IdentifierOrKeyword())
		v := ac.AlterWorkflowValue().(*parser.AlterWorkflowValueContext)
		lk := strings.ToLower(key)
		if onActivity {
			prop, known := activityPropertyKeys[lk]
			if !known {
				b.addError(fmt.Errorf("alter workflow: an activity has no property %s — `set ( … ) on <activity>` takes %s; "+
					"a workflow property (%s) is set without `on`", key, activityKeysText, workflowKeysText))
				continue
			}
			if op := b.buildActivitySet(key, prop, v, ref, pos); op != nil {
				ops = append(ops, op)
			}
			continue
		}
		prop, known := workflowPropertyKeys[lk]
		if !known {
			if _, isActivity := activityPropertyKeys[lk]; isActivity {
				b.addError(fmt.Errorf("alter workflow: %s is an activity property — write `set (%s: …) on <activity>`", key, key))
			} else {
				b.addError(fmt.Errorf("alter workflow: a workflow has no property %s — `set ( … )` takes %s; "+
					"an activity's (%s) are set with `on <activity>`", key, workflowKeysText, activityKeysText))
			}
			continue
		}
		if op := b.buildWorkflowSet(key, prop, v); op != nil {
			ops = append(ops, op)
		}
	}
	return ops
}

// alterWorkflowStringValue reads a value that must be a string.
func alterWorkflowStringValue(v *parser.AlterWorkflowValueContext) (string, bool) {
	if we := v.WorkflowExpression(); we != nil && we.STRING_LITERAL() != nil {
		return unquoteStringLit(we.STRING_LITERAL()), true
	}
	if v.TemplateParams() != nil && v.XPATH() == nil && v.STRING_LITERAL() != nil {
		return unquoteStringLit(v.STRING_LITERAL()), true
	}
	return "", false
}

// alterWorkflowTemplateValue reads a text with the parameters that fill its
// placeholders: `Display: 'Approval for {1}' with ({1} = $WorkflowContext/Title)`
// (ako/mxcli#1714). A text written without them has none, so setting it clears
// any it had — the statement states the whole value.
func alterWorkflowTemplateValue(v *parser.AlterWorkflowValueContext) (string, []ast.WorkflowTemplateParam, bool) {
	s, ok := alterWorkflowStringValue(v)
	if !ok {
		return "", nil, false
	}
	return s, buildWorkflowTemplateParams(v.TemplateParams()), true
}

// alterWorkflowNoteValue reads a note's text (ako/mxcli#1717): a plain
// string, since a Workflows$Annotation stores a Description and no template.
// The empty string removes the note.
func alterWorkflowNoteValue(v *parser.AlterWorkflowValueContext) (string, bool) {
	if v.TemplateParams() != nil {
		return "", false
	}
	return alterWorkflowStringValue(v)
}

// alterWorkflowExpressionValue reads an expression-valued property (a due
// date): the expression written bare, or — the deprecated form, MDL-DEPR080 —
// its string's content, as workflowExpression reads it everywhere else.
func alterWorkflowExpressionValue(v *parser.AlterWorkflowValueContext) (string, bool) {
	if we := v.WorkflowExpression(); we != nil {
		return workflowExpressionText(we), true
	}
	if qn := v.QualifiedName(); qn != nil && v.VARIABLE() == nil && v.MICROFLOW() == nil {
		// A bare constant or enumeration value reads as a name first.
		return extractExpressionText(qn), true
	}
	return "", false
}

func (b *Builder) alterWorkflowValueError(key, want string) {
	b.addError(fmt.Errorf("alter workflow: %s takes %s", key, want))
}

func (b *Builder) buildWorkflowSet(key, prop string, v *parser.AlterWorkflowValueContext) ast.AlterWorkflowOp {
	op := &ast.SetWorkflowPropertyOp{Property: prop}
	switch prop {
	case "display", "description":
		s, params, ok := alterWorkflowTemplateValue(v)
		if !ok {
			b.alterWorkflowValueError(key, "a string: `"+key+": '…'`, or `"+key+": '… {1}' with ({1} = $WorkflowContext/Attr)`")
			return nil
		}
		op.Value, op.Params = s, params
	case "annotation":
		s, ok := alterWorkflowNoteValue(v)
		if !ok {
			b.alterWorkflowValueError(key, "a string: `"+key+": '…'`, or `"+key+": ''` to remove the note")
			return nil
		}
		op.Value = s
	case "export_level":
		qn := v.QualifiedName()
		if qn == nil || v.VARIABLE() != nil || v.MICROFLOW() != nil || strings.Contains(qn.GetText(), ".") {
			b.alterWorkflowValueError(key, "a level: `ExportLevel: API`, `ExportLevel: Hidden`")
			return nil
		}
		op.Value = qn.GetText()
		if strings.EqualFold(op.Value, "api") {
			op.Value = "API"
		}
	case "due_date":
		s, ok := alterWorkflowExpressionValue(v)
		if !ok {
			b.alterWorkflowValueError(key, "an expression: `DueDate: addDays([%CurrentDateTime%], 3)`")
			return nil
		}
		op.Value = s
	case "overview_page":
		qn := v.QualifiedName()
		if qn == nil || v.VARIABLE() != nil || v.MICROFLOW() != nil {
			b.alterWorkflowValueError(key, "a page: `OverviewPage: Module.Page`")
			return nil
		}
		op.Entity = buildQualifiedName(qn)
	case "parameter":
		if v.VARIABLE() == nil || v.QualifiedName() == nil {
			b.alterWorkflowValueError(key, "a parameter declaration: `Parameter: $WorkflowContext: Module.Entity`")
			return nil
		}
		op.Value = v.VARIABLE().GetText()
		op.Entity = buildQualifiedName(v.QualifiedName())
	}
	return op
}

func (b *Builder) buildActivitySet(key, prop string, v *parser.AlterWorkflowValueContext, ref string, pos int) ast.AlterWorkflowOp {
	op := &ast.SetActivityPropertyOp{ActivityRef: ref, AtPosition: pos, Property: prop}
	switch prop {
	case "page":
		qn := v.QualifiedName()
		if qn == nil || v.VARIABLE() != nil || v.MICROFLOW() != nil {
			b.alterWorkflowValueError(key, "a page: `Page: Module.Page`")
			return nil
		}
		op.PageName = buildQualifiedName(qn)
	case "description", "task_name":
		s, params, ok := alterWorkflowTemplateValue(v)
		if !ok {
			b.alterWorkflowValueError(key, "a string: `"+key+": '…'`, or `"+key+": '… {1}' with ({1} = $WorkflowContext/Attr)`")
			return nil
		}
		op.Value, op.Params = s, params
	case "annotation":
		s, ok := alterWorkflowNoteValue(v)
		if !ok {
			b.alterWorkflowValueError(key, "a string: `"+key+": '…'`, or `"+key+": ''` to remove the note")
			return nil
		}
		op.Value = s
	case "targeting":
		switch {
		case v.MICROFLOW() != nil:
			op.Property = "targeting_microflow"
			op.Microflow = buildQualifiedName(v.QualifiedName())
		case v.XPATH() != nil:
			op.Property = "targeting_xpath"
			if groups := v.AllXpathConstraint(); len(groups) > 0 {
				op.Value = bracketedXPathText(groups)
			} else if lit := v.STRING_LITERAL(); lit != nil {
				op.Value = stripExpressionIdentifierQuotes(unquoteStringLit(lit)) // MDL-DEPR031, recorded by ExitAlterWorkflowValue; quoted names as in brackets (mendixlabs/mxcli#1243)
			}
		default:
			b.alterWorkflowValueError(key, "`Targeting: microflow Module.Microflow` or `Targeting: xpath [ … ]`")
			return nil
		}
	case "due_date":
		s, ok := alterWorkflowExpressionValue(v)
		if !ok {
			b.alterWorkflowValueError(key, "an expression: `DueDate: addDays([%CurrentDateTime%], 1)`")
			return nil
		}
		op.Value = s
	}
	return op
}

// ExitAlterWorkflowValue records `Targeting: xpath '…'`, the XPath in a
// string (MDL-DEPR031), as the old `set activity … targeting xpath '…'` does.
func (b *Builder) ExitAlterWorkflowValue(ctx *parser.AlterWorkflowValueContext) {
	if ctx.XPATH() != nil {
		b.recordQuotedTargetingXPath(ctx.STRING_LITERAL())
	}
}

// isAlterWorkflowStringProperty reports whether a workflowExpression is the
// value of a `set ( … )` key other than DueDate: there a string is the value
// (Display: 'x'), not the deprecated spelling of an expression.
func isAlterWorkflowStringProperty(ctx antlr.Tree) bool {
	v, ok := ctx.GetParent().(*parser.AlterWorkflowValueContext)
	if !ok {
		return false
	}
	a, ok := v.GetParent().(*parser.AlterWorkflowAssignmentContext)
	if !ok {
		return false
	}
	return !strings.EqualFold(identifierOrKeywordText(a.IdentifierOrKeyword()), "DueDate")
}

// ---------------------------------------------------------------------------
// The old per-action form: MDL-DEPR140-149 and their rewrites
// ---------------------------------------------------------------------------

// oldAlterWorkflowUse is one old action's record: its code, and the rewrite
// of the action itself (the block's braces are added around them), or why
// there is none.
type oldAlterWorkflowUse struct {
	code  string
	tok   antlr.Token
	edits []ast.TextEdit
	noFix string
	// separate says the canonical operation is followed by `;` (set, drop);
	// an insert or replace ends in its fragment's `}`.
	separate bool
}

var pathCaption = regexp.MustCompile(`^Path ([1-9][0-9]*)$`)

// recordOldAlterWorkflowActions records every action of the old form under its
// code, with its rewrite. The old statement has no braces: the first action's
// rewrite opens the block after the workflow's name, the last one's closes it
// (adding the statement's `;` when it had none). An action with no rewrite
// leaves the whole statement unrewritten — half a block would not parse.
func (b *Builder) recordOldAlterWorkflowActions(ctx *parser.AlterStatementContext) {
	actions := ctx.AllAlterWorkflowAction()
	if len(actions) == 0 {
		return
	}
	uses := make([]oldAlterWorkflowUse, 0, len(actions))
	noFix := ""
	for _, a := range actions {
		u := oldAlterWorkflowActionUse(a.(*parser.AlterWorkflowActionContext))
		if u.noFix != "" && noFix == "" {
			noFix = u.noFix
		}
		uses = append(uses, u)
	}
	if noFix != "" {
		for i := range uses {
			if uses[i].noFix == "" {
				uses[i].noFix = "another action of this `alter workflow` has no rewrite (" + noFix + "); rewrite the statement by hand"
			}
		}
	} else {
		if qn := ctx.QualifiedName(); qn != nil && qn.GetStop() != nil {
			uses[0].edits = append([]ast.TextEdit{insertAt(qn.GetStop().GetStop()+1, " {")}, uses[0].edits...)
		}
		for i := range uses {
			end := actions[i].GetStop().GetStop() + 1
			if uses[i].separate {
				uses[i].edits = append(uses[i].edits, insertAt(end, ";"))
			}
			if i == len(uses)-1 {
				// A statement written over several lines closes on a line of
				// its own, as a block does; a one-liner stays on its line.
				closing := " }"
				if qn := ctx.QualifiedName(); qn != nil && qn.GetStop() != nil &&
					actions[i].GetStop().GetLine() != qn.GetStop().GetLine() {
					closing = "\n}"
				}
				if !alterStatementTerminated(ctx) {
					closing += ";"
				}
				uses[i].edits = append(uses[i].edits, insertAt(end, closing))
			}
		}
	}
	for _, u := range uses {
		b.recordDeprecation(u.code, u.tok, "alter workflow")
		if u.noFix != "" {
			b.fixLastDeprecation(u.code, nil, u.noFix)
		} else {
			b.fixLastDeprecation(u.code, &ast.Fix{Edits: u.edits}, "")
		}
	}
}

// alterStatementTerminated reports whether the statement has its own `;`.
// An old action ending in an activity (`insert after X call microflow M.F;`)
// takes the `;` itself, and the statement then has none.
func alterStatementTerminated(ctx *parser.AlterStatementContext) bool {
	if ctx.SEMICOLON() != nil {
		return true
	}
	if ddl, ok := ctx.GetParent().(*parser.DdlStatementContext); ok {
		if st, ok := ddl.GetParent().(*parser.StatementContext); ok && st.SEMICOLON() != nil {
			return true
		}
	}
	return false
}

// srcText is the source between two tokens, as written.
func srcText(start, stop antlr.Token) string {
	if start == nil || stop == nil || start.GetInputStream() == nil || stop.GetStop() < start.GetStart() {
		return ""
	}
	return start.GetInputStream().GetText(start.GetStart(), stop.GetStop())
}

// spaceBefore is " " when the source has blank space before tok, as the old
// form did between its keywords and the value: `due date'x'` stays glued, so
// the rewrite of the string itself can decide the spacing.
func spaceBefore(tok antlr.Token) string {
	is := tok.GetInputStream()
	if is == nil || tok.GetStart() == 0 {
		return " "
	}
	switch is.GetText(tok.GetStart()-1, tok.GetStart()-1) {
	case " ", "\t", "\n", "\r":
		return " "
	}
	return ""
}

// replaceBefore replaces from the start of first up to (not including) the
// start of next.
func replaceBefore(first, next antlr.Token, text string) ast.TextEdit {
	return ast.TextEdit{Start: first.GetStart(), Stop: next.GetStart(), Text: text}
}

// oldAlterWorkflowActionUse computes one old action's code and rewrite. Each
// rewrite touches only the action's own words and moves the activity address;
// the value, the fragment and the boundary event clause are left as written,
// so a deprecation inside them (a quoted XPath, an expression in a string, a
// `comment` caption) is rewritten by its own record.
func oldAlterWorkflowActionUse(a *parser.AlterWorkflowActionContext) (u oldAlterWorkflowUse) {
	start := a.GetStart()
	// A malformed action (a parse error the parser recovered from) is missing
	// the children a rewrite reads; the parse error is what gets reported.
	defer func() {
		if recover() != nil {
			u = oldAlterWorkflowUse{code: deprecation.AlterWorkflowSet, tok: start, noFix: "the action does not parse"}
		}
	}()
	ref := ""
	if r := a.AlterActivityRef(); r != nil {
		ref = srcText(r.GetStart(), r.GetStop())
	}
	after := func(t antlr.Token, text string) ast.TextEdit { return insertAt(t.GetStop()+1, text) }
	// Keywords the rewrite writes follow the action's own letter case.
	kw := func(w string) string { return keywordLike(start.GetText(), w) }

	switch {
	case a.WorkflowSetProperty() != nil:
		return oldWorkflowSetUse(a.WorkflowSetProperty().(*parser.WorkflowSetPropertyContext), start, kw)

	case a.ActivitySetProperty() != nil:
		return oldActivitySetUse(a.ActivitySetProperty().(*parser.ActivitySetPropertyContext), start, ref, kw)

	case a.INSERT() != nil && a.AFTER() != nil:
		st := a.WorkflowActivityStmt()
		return oldAlterWorkflowUse{code: deprecation.AlterWorkflowInsertAfter, tok: start, edits: []ast.TextEdit{
			insertAt(st.GetStart().GetStart(), "{ "), after(st.GetStop(), " }")}}

	case a.REPLACE() != nil:
		st := a.WorkflowActivityStmt()
		act := a.ACTIVITY().GetSymbol()
		return oldAlterWorkflowUse{code: deprecation.AlterWorkflowReplaceActivity, tok: start, edits: []ast.TextEdit{
			{Start: act.GetStart(), Stop: afterBlank(act), Text: ""},
			insertAt(st.GetStart().GetStart(), "{ "), after(st.GetStop(), " }")}}

	case a.DROP() != nil && a.ACTIVITY() != nil:
		act := a.ACTIVITY().GetSymbol()
		return oldAlterWorkflowUse{code: deprecation.AlterWorkflowDropActivity, tok: start, separate: true,
			edits: []ast.TextEdit{{Start: act.GetStart(), Stop: afterBlank(act), Text: ""}}}

	case a.INSERT() != nil && a.BOUNDARY() != nil:
		clause := a.WorkflowBoundaryEventClause()
		return oldAlterWorkflowUse{code: deprecation.AlterWorkflowInsertBoundaryEvent, tok: start, edits: []ast.TextEdit{
			replaceBefore(start, clause.GetStart(), kw("insert into")+" "+ref+" { "+kw("boundary event")+" "),
			after(clause.GetStop(), " }")}}

	case a.INSERT() != nil:
		code, member := "", ""
		switch {
		case a.OUTCOME() != nil:
			code, member = deprecation.AlterWorkflowInsertOutcome, kw("outcomes")+" "+a.STRING_LITERAL().GetText()+" "
		case a.PATH() != nil:
			code, member = deprecation.AlterWorkflowInsertPath, kw("path")+" "
		case a.CONDITION() != nil:
			code, member = deprecation.AlterWorkflowInsertCondition, kw("outcomes")+" "+a.STRING_LITERAL().GetText()+" -> "
		}
		return oldAlterWorkflowUse{code: code, tok: start, edits: []ast.TextEdit{
			replaceBefore(start, a.LBRACE().GetSymbol(), kw("insert into")+" "+ref+" { "+member),
			after(a.RBRACE().GetSymbol(), " }")}}

	case a.DROP() != nil:
		return oldDropMemberUse(a, start, ref, kw)
	}
	return oldAlterWorkflowUse{code: deprecation.AlterWorkflowSet, tok: start, noFix: "unrecognised alter workflow action"}
}

// oldWorkflowSetUse rewrites `set <property> v` as `set (Key: v)`.
func oldWorkflowSetUse(wp *parser.WorkflowSetPropertyContext, start antlr.Token, kw func(string) string) oldAlterWorkflowUse {
	key, value := "", antlr.Token(nil)
	switch {
	case wp.DISPLAY() != nil:
		key, value = "Display", wp.STRING_LITERAL().GetSymbol()
	case wp.DESCRIPTION() != nil:
		key, value = "Description", wp.STRING_LITERAL().GetSymbol()
	case wp.EXPORT() != nil:
		key = "ExportLevel"
		switch {
		case wp.API() != nil:
			value = wp.API().GetSymbol()
		case wp.HIDDEN_KW() != nil:
			value = wp.HIDDEN_KW().GetSymbol()
		case wp.IDENTIFIER() != nil:
			value = wp.IDENTIFIER().GetSymbol()
		}
	case wp.DUE() != nil:
		key = "DueDate"
		if e := wp.WorkflowExpression(); e != nil {
			value = e.GetStart()
		}
	case wp.OVERVIEW() != nil:
		key = "OverviewPage"
		if q := wp.QualifiedName(); q != nil {
			value = q.GetStart()
		}
	case wp.PARAMETER() != nil:
		key, value = "Parameter", wp.VARIABLE().GetSymbol()
	}
	u := oldAlterWorkflowUse{code: deprecation.AlterWorkflowSet, tok: start, separate: true}
	if value == nil {
		u.noFix = "the property has no value to move into `set ( … )`"
		return u
	}
	u.edits = []ast.TextEdit{replaceBefore(start, value, kw("set")+" ("+key+":"+spaceBefore(value)), insertAt(wp.GetStop().GetStop()+1, ")")}
	return u
}

// oldActivitySetUse rewrites `set activity X <property> v` as `set (Key: v) on X`.
func oldActivitySetUse(ap *parser.ActivitySetPropertyContext, start antlr.Token, ref string, kw func(string) string) oldAlterWorkflowUse {
	key, extra, value := "", "", antlr.Token(nil)
	switch {
	case ap.PAGE() != nil:
		key, value = "Page", ap.QualifiedName().GetStart()
	case ap.DESCRIPTION() != nil:
		key, value = "Description", ap.STRING_LITERAL().GetSymbol()
	case ap.TARGETING() != nil && ap.MICROFLOW() != nil:
		key, extra, value = "Targeting", " "+kw("microflow"), ap.QualifiedName().GetStart()
	case ap.TARGETING() != nil:
		key, extra = "Targeting", " "+kw("xpath")
		if g := ap.AllXpathConstraint(); len(g) > 0 {
			value = g[0].GetStart()
		} else if lit := ap.STRING_LITERAL(); lit != nil {
			value = lit.GetSymbol()
		}
	case ap.DUE() != nil:
		key = "DueDate"
		if e := ap.WorkflowExpression(); e != nil {
			value = e.GetStart()
		}
	}
	u := oldAlterWorkflowUse{code: deprecation.AlterWorkflowSetActivity, tok: start, separate: true}
	if value == nil {
		u.noFix = "the property has no value to move into `set ( … ) on`"
		return u
	}
	u.edits = []ast.TextEdit{replaceBefore(start, value, kw("set")+" ("+key+":"+extra+spaceBefore(value)),
		insertAt(ap.GetStop().GetStop()+1, ") "+kw("on")+" "+ref)}
	return u
}

// oldDropMemberUse rewrites `drop outcome|condition|path|boundary event … on X`
// as `drop X outcome …|path n|boundary event`.
func oldDropMemberUse(a *parser.AlterWorkflowActionContext, start antlr.Token, ref string, kw func(string) string) oldAlterWorkflowUse {
	u := oldAlterWorkflowUse{code: deprecation.AlterWorkflowDropMember, tok: start, separate: true}
	member := ""
	switch {
	case a.OUTCOME() != nil:
		member = " " + kw("outcome") + " " + a.STRING_LITERAL().GetText()
	case a.CONDITION() != nil:
		v := unquoteStringLit(a.STRING_LITERAL())
		switch strings.ToLower(v) {
		case "true", "false", "default":
			member = " " + kw("outcome") + " " + kw(v)
		default:
			// An enumeration outcome by value: `drop outcome` and `drop
			// condition` match a non-Boolean outcome the same way.
			member = " " + kw("outcome") + " " + a.STRING_LITERAL().GetText()
		}
	case a.PATH() != nil:
		m := pathCaption.FindStringSubmatch(unquoteStringLit(a.STRING_LITERAL()))
		if m == nil {
			u.noFix = "the path caption " + a.STRING_LITERAL().GetText() +
				" is not `Path n`, and a path is dropped by its number (`drop <split> path n`)"
			return u
		}
		member = " " + kw("path") + " " + m[1]
	case a.BOUNDARY() != nil:
		member = " " + kw("boundary event")
	}
	u.edits = []ast.TextEdit{{Start: start.GetStart(), Stop: a.GetStop().GetStop() + 1, Text: kw("drop") + " " + ref + member}}
	return u
}
