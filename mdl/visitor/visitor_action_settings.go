// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// A client action's `with ( … )` settings (ako/mxcli#721 L2).
//
// Studio Pro stores, on every client action, whether the widget is disabled
// while the action runs, and on a microflow or nanoflow call a progress bar, a
// progress message and a confirmation. None of it had an MDL spelling, so the
// writer hard-coded the defaults and `describe` → `exec` removed a button's
// "Are you sure?" confirmation and its progress bar, and changed whether a
// widget is disabled while its action runs — with `mx check` and the build
// green.

// actionSettingKeys maps each setting key, lower-cased, to its canonical
// spelling and the action kinds that carry it (actionSettingsKind's names);
// nil kinds is every action.
var actionSettingKeys = map[string]struct {
	canonical string
	kinds     []string
	what      string // those actions in words, for the refusal
}{
	"disabledduringexecution": {canonical: "DisabledDuringExecution"},
	"progressbar":             {canonical: "ProgressBar", kinds: flowCalls, what: flowCallsText},
	"progressmessage":         {canonical: "ProgressMessage", kinds: flowCalls, what: flowCallsText},
	"confirmation":            {canonical: "Confirmation", kinds: confirmable, what: confirmableText},
	"proceedcaption":          {canonical: "ProceedCaption", kinds: confirmable, what: confirmableText},
	"cancelcaption":           {canonical: "CancelCaption", kinds: confirmable, what: confirmableText},
	"asynchronous":            {canonical: "Asynchronous", kinds: []string{"microflow"}, what: "`call microflow`"},
	"formvalidations":         {canonical: "FormValidations", kinds: []string{"microflow"}, what: "`call microflow`"},
	// The workflow events' own settings (ako/mxcli#832). Commit is complete
	// task's alone: Mendix 9.12 removed it from the workflow call, which has
	// committed ever since.
	"assignonopen":     {canonical: "AssignOnOpen", kinds: []string{"openUserTask"}, what: "`open user task`"},
	"openwhenassigned": {canonical: "OpenWhenAssigned", kinds: []string{"openUserTask"}, what: "`open user task`"},
	"closepage":        {canonical: "ClosePage", kinds: []string{"completeTask", "callWorkflow"}, what: "`complete task` and `call workflow`"},
	"commit":           {canonical: "Commit", kinds: []string{"completeTask"}, what: "`complete task`"},
	"defaultpage":      {canonical: "DefaultPage", kinds: []string{"openWorkflow"}, what: "`open workflow`"},
	// Show page's "number of pages to close" (ako/mxcli#2106).
	"numberofpagestoclose": {canonical: "NumberOfPagesToClose", kinds: []string{"showPage"}, what: "`show page`"},
}

var (
	flowCalls       = []string{"microflow", "nanoflow"}
	flowCallsText   = "`call microflow` and `call nanoflow`"
	confirmable     = []string{"microflow", "nanoflow", "callWorkflow"}
	confirmableText = "`call microflow`, `call nanoflow` and `call workflow`"
)

// actionSettingsKind names the action a `with ( … )` list belongs to, as
// ActionV3.Type names it; "other" for an action whose only setting is
// DisabledDuringExecution.
func actionSettingsKind(act *parser.ActionExprV3Context) string {
	switch {
	case act.MICROFLOW() != nil:
		return "microflow"
	case act.NANOFLOW() != nil:
		return "nanoflow"
	case act.COMPLETE_TASK() != nil:
		return "completeTask"
	case act.OPEN() != nil && act.USER() != nil:
		return "openUserTask"
	case act.OPEN() != nil && act.WORKFLOW() != nil:
		return "openWorkflow"
	case act.CALL() != nil && act.WORKFLOW() != nil:
		return "callWorkflow"
	case (act.SHOW() != nil && act.PAGE() != nil) || act.SHOW_PAGE() != nil:
		return "showPage"
	}
	return "other"
}

// canonicalProgressBar and canonicalFormValidations spell an enum value the way
// Mendix stores it, whatever case it was written in.
var canonicalProgressBar = map[string]string{
	"none": "None", "nonblocking": "NonBlocking", "blocking": "Blocking",
}

var canonicalFormValidations = map[string]string{
	"all": "All", "widget": "Widget", "none": "None",
}

// actionSettingValue returns a setting's value text and whether it was a
// string literal.
func actionSettingValue(s *parser.ActionSettingV3Context) (string, bool) {
	if str := s.STRING_LITERAL(); str != nil {
		return unquoteStringLit(str), true
	}
	if n := s.NUMBER_LITERAL(); n != nil {
		return n.GetText(), false
	}
	if n := s.NONE(); n != nil {
		return n.GetText(), false
	}
	// The key is the identifierOrKeyword, the value a qualifiedName: one
	// segment for every setting but a document reference.
	if qn := s.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn), false
	}
	return "", false
}

// buildActionSettingsV3 builds the settings leniently: a key or value it does
// not understand is skipped here and reported by ExitActionSettingsV3, so a
// caller without a Builder still gets the settings it can use.
func buildActionSettingsV3(ctx parser.IActionSettingsV3Context) *ast.ActionSettingsV3 {
	sc, ok := ctx.(*parser.ActionSettingsV3Context)
	if !ok || sc == nil {
		return nil
	}
	out := &ast.ActionSettingsV3{}
	for _, raw := range sc.AllActionSettingV3() {
		s := raw.(*parser.ActionSettingV3Context)
		key := strings.ToLower(identifierOrKeywordText(s.IdentifierOrKeyword()))
		val, isString := actionSettingValue(s)
		switch key {
		case "disabledduringexecution":
			if b, ok := parseSettingBool(val, isString); ok {
				out.DisabledDuringExecution = &b
			}
		case "asynchronous":
			if b, ok := parseSettingBool(val, isString); ok {
				out.Asynchronous = &b
			}
		case "progressbar":
			if !isString {
				out.ProgressBar = canonicalProgressBar[strings.ToLower(val)]
			}
		case "formvalidations":
			if !isString {
				out.FormValidations = canonicalFormValidations[strings.ToLower(val)]
			}
		case "progressmessage":
			if isString {
				out.ProgressMessage = &val
			}
		case "confirmation":
			if isString {
				out.Confirmation = &val
			}
		case "proceedcaption":
			if isString {
				out.ProceedCaption = &val
			}
		case "cancelcaption":
			if isString {
				out.CancelCaption = &val
			}
		case "assignonopen":
			if b, ok := parseSettingBool(val, isString); ok {
				out.AssignOnOpen = &b
			}
		case "openwhenassigned":
			if b, ok := parseSettingBool(val, isString); ok {
				out.OpenWhenAssigned = &b
			}
		case "closepage":
			if b, ok := parseSettingBool(val, isString); ok {
				out.ClosePage = &b
			}
		case "commit":
			if b, ok := parseSettingBool(val, isString); ok {
				out.Commit = &b
			}
		case "defaultpage":
			if !isString {
				out.DefaultPage = val
			}
		case "numberofpagestoclose":
			if n, err := strconv.Atoi(val); !isString && err == nil && n >= 1 {
				out.NumberOfPagesToClose = val
			}
		}
	}
	return out
}

func parseSettingBool(val string, isString bool) (bool, bool) {
	if isString {
		return false, false
	}
	switch strings.ToLower(val) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// ExitActionSettingsV3 reports what buildActionSettingsV3 skipped: an unknown
// key, a repeated key, a value of the wrong kind, and a setting the action does
// not have (R11: a meaningless form is an error, not a no-op). The syntax is
// new, so the rejection applies under every language version.
func (b *Builder) ExitActionSettingsV3(ctx *parser.ActionSettingsV3Context) {
	kind := "other"
	if act, ok := ctx.GetParent().(*parser.ActionExprV3Context); ok {
		kind = actionSettingsKind(act)
	}
	// A microflow data source stores the same Forms$MicroflowSettings, but of
	// its settings only FormValidations ever differs from the default in a
	// Studio Pro-authored page (ako/mxcli#1729): the rest are an action's.
	_, dataSource := ctx.GetParent().(*parser.DataSourceExprV3Context)
	seen := map[string]bool{}
	for _, raw := range ctx.AllActionSettingV3() {
		s := raw.(*parser.ActionSettingV3Context)
		tok := s.GetStart()
		pos := fmt.Sprintf("line %d:%d", tok.GetLine(), tok.GetColumn())
		written := identifierOrKeywordText(s.IdentifierOrKeyword())
		key := strings.ToLower(written)
		spec, known := actionSettingKeys[key]
		if !known {
			b.addError(fmt.Errorf("%s: unknown action setting %q; every action takes "+
				"DisabledDuringExecution, a flow call also ProgressBar, ProgressMessage, "+
				"Confirmation, ProceedCaption and CancelCaption, a microflow call also "+
				"Asynchronous and FormValidations, `call workflow` Confirmation and ClosePage, "+
				"`complete task` ClosePage and Commit, `open user task` AssignOnOpen and "+
				"OpenWhenAssigned, `open workflow` DefaultPage, and `show page` "+
				"NumberOfPagesToClose", pos, written))
			continue
		}
		if seen[key] {
			b.addError(fmt.Errorf("%s: action setting %s is written twice", pos, spec.canonical))
			continue
		}
		seen[key] = true
		if dataSource && key != "formvalidations" {
			b.addError(fmt.Errorf("%s: %s is a setting of a flow call, not of a data source; "+
				"a microflow data source takes FormValidations only", pos, spec.canonical))
			continue
		}
		if spec.kinds != nil && !dataSource && !slices.Contains(spec.kinds, kind) {
			b.addError(fmt.Errorf("%s: %s is a setting of %s only", pos, spec.canonical, spec.what))
			continue
		}
		val, isString := actionSettingValue(s)
		valText := s.GetStop().GetText()
		if qn := s.QualifiedName(); qn != nil {
			valText = qn.GetText()
		}
		if key == "defaultpage" {
			if isString || !strings.Contains(val, ".") {
				b.addError(fmt.Errorf("%s: DefaultPage takes a page, Module.Page, not %s", pos, valText))
			}
			continue
		}
		if !isString && strings.Contains(val, ".") {
			b.addError(fmt.Errorf("%s: %s takes no qualified name, not %s", pos, spec.canonical, valText))
			continue
		}
		switch key {
		case "disabledduringexecution", "asynchronous", "assignonopen", "openwhenassigned", "closepage", "commit":
			if _, ok := parseSettingBool(val, isString); !ok {
				b.addError(fmt.Errorf("%s: %s takes true or false, not %s", pos, spec.canonical, valText))
			}
		case "progressbar":
			if _, ok := canonicalProgressBar[strings.ToLower(val)]; isString || !ok {
				b.addError(fmt.Errorf("%s: ProgressBar takes None, NonBlocking or Blocking, not %s", pos, valText))
			}
		case "formvalidations":
			if _, ok := canonicalFormValidations[strings.ToLower(val)]; isString || !ok {
				b.addError(fmt.Errorf("%s: FormValidations takes All, Widget or None, not %s", pos, valText))
			}
		case "numberofpagestoclose":
			if n, err := strconv.Atoi(val); isString || err != nil || n < 1 {
				b.addError(fmt.Errorf("%s: NumberOfPagesToClose takes a whole number of at least 1, not %s", pos, valText))
			}
		default: // the text settings
			if !isString {
				b.addError(fmt.Errorf("%s: %s takes a string, not %s", pos, spec.canonical, valText))
			}
		}
	}
	if !seen["confirmation"] && (seen["proceedcaption"] || seen["cancelcaption"]) {
		tok := ctx.GetStart()
		b.addError(fmt.Errorf("line %d:%d: ProceedCaption and CancelCaption are the buttons of a "+
			"confirmation; write Confirmation: '<question>' as well", tok.GetLine(), tok.GetColumn()))
	}
}

// requireOpenWorkflowPage refuses `open workflow` without a DefaultPage.
// "Show workflow admin page" opens the workflow's own admin page when it
// overrides one and this page otherwise, and Mendix requires it: written
// without, mx check reports CE1834 "The 'Page' property is required"
// (measured on 11.13.0, ako/mxcli#832).
func (b *Builder) requireOpenWorkflowPage(ctx *parser.ActionExprV3Context) {
	if actionSettingsKind(ctx) != "openWorkflow" {
		return
	}
	if sc, ok := ctx.ActionSettingsV3().(*parser.ActionSettingsV3Context); ok && sc != nil {
		for _, raw := range sc.AllActionSettingV3() {
			if strings.EqualFold(identifierOrKeywordText(raw.(*parser.ActionSettingV3Context).IdentifierOrKeyword()), "DefaultPage") {
				return
			}
		}
	}
	tok := ctx.GetStart()
	b.addError(fmt.Errorf("line %d:%d: open workflow needs a page for a workflow that overrides no "+
		"admin page — write `open workflow with (DefaultPage: Module.Page)`; Mendix refuses the action "+
		"without one (CE1834 \"The 'Page' property is required\")", tok.GetLine(), tok.GetColumn()))
}
