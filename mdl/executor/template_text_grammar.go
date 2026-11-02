// SPDX-License-Identifier: Apache-2.0

// The text of a template, judged by the grammar mxbuild applies to it.
//
// A log message, a show message, a validation feedback, a REST call's URL and
// custom body, a Java action's text-template argument and a send email subject
// and body each store a text with {n} placeholders beside one parameter per
// placeholder. Mendix reads the text with one grammar (mdl/texttemplate): a
// brace opens a placeholder `{digits}` or is escaped as `{{`, and anything
// else is a build error.
//
// ako/mxcli#1397 (mendixlabs/mxcli#1515): `log info node 'Mod' 'Item {number}
// not found';` and a REST POST's `Body: template '{}'` passed `mxcli check`
// and exec, and mxbuild refused both with CE0712 "Brace should be followed by
// a number of digits or another brace." Each cost a build cycle to find. The
// workflow texts (MDL-WF21), a dynamic text's content and a button's caption
// (MDL-WIDGET04) apply the same grammar from validate_workflow_templates.go
// and validate_widgets.go.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/texttemplate"
)

// templateGrammarRule is the rule ID for a template text mxbuild refuses.
const templateGrammarRule = "MDL-TEMPLATE02"

// templateTextSite is one template text a statement writes.
type templateTextSite struct {
	where  string // "log: the message"
	text   string
	params int // how many parameters the builder stores beside it
}

// templateTextRefusal returns why mxbuild refuses text filled by params
// parameters, and what to write instead; "" when it builds.
//
// This is the single decision: the flow builders call it (refuseTemplateGrammar,
// javaActionTemplateValue) and the check pass calls it (checkTemplateGrammar),
// as do the workflow and page checks, so no slot can pass one and fail the
// other.
func templateTextRefusal(where, text string, params int) (msg, fix string) {
	p := texttemplate.Check(text, params)
	if p == nil {
		return "", ""
	}
	rejects := fmt.Sprintf("MxBuild rejects it (%s %q)", p.Code, p.Message)
	switch p.Code {
	case "CE0720":
		return fmt.Sprintf("%s template uses `%s` but has %d parameter(s) — %s", where, p.At, params, rejects),
			"Fill every placeholder with (…) — `'… {1} …' with ({1} = <expression>)` — or write a literal brace as {{."
	case "CE0716":
		return fmt.Sprintf("%s template has `%s` — %s", where, p.At, rejects),
			"Number the placeholders from {1}."
	case "CE0717":
		return fmt.Sprintf("%s template has `%s` — %s", where, p.At, rejects),
			"Number the placeholders {1}, {2}, … in the order of the parameters in with (…)."
	case "CE0713", "CE0714":
		return fmt.Sprintf("%s template has `%s` — %s", where, p.At, rejects),
			"A placeholder is digits between braces, {1}; write a literal brace as {{."
	default: // CE0712
		return fmt.Sprintf("%s template has `%s` — %s", where, p.At, rejects),
			"A brace opens a numbered placeholder filled by with (…): `'Item {1} not found' with ({1} = $Number)`. " +
				"Write a literal brace as {{ — a JSON body is `'{{\"name\": \"{1}\"}' with ({1} = $Name)`; a closing brace needs no escape."
	}
}

// templateTextOf returns the text a template slot stores when it is written as
// a string literal. A template written as an expression is stored as '{1}' with
// the expression as its parameter, and has no text of its own to judge.
func templateTextOf(e ast.Expression) (string, bool) {
	return restTemplateText(e)
}

// maxTemplateParamIndex is how many parameters a builder that places each
// `{n} = …` at position n stores: the highest n.
func maxTemplateParamIndex(params []ast.TemplateParam) int {
	m := 0
	for _, p := range params {
		if p.Index > m {
			m = p.Index
		}
	}
	return m
}

// templateTextSites returns the template texts stmt writes that are known from
// the statement alone. A Java action argument is a template when it states
// `with (…)`; whether one without is a template depends on the action's
// parameter type, which exec checks where it reads it (javaActionTemplateValue).
func templateTextSites(stmt ast.MicroflowStatement) []templateTextSite {
	var out []templateTextSite
	add := func(where string, text ast.Expression, params int) {
		if s, ok := templateTextOf(text); ok {
			out = append(out, templateTextSite{where: where, text: s, params: params})
		}
	}
	switch s := stmt.(type) {
	case *ast.LogStmt:
		add("log: the message", s.Message, maxTemplateParamIndex(s.Template))
	case *ast.ShowMessageStmt:
		add("show message: the message", s.Message, len(s.TemplateArgs))
	case *ast.ValidationFeedbackStmt:
		add("validation feedback: the message", s.Message, len(s.TemplateArgs))
	case *ast.SendEmailStmt:
		add("send email: the subject", s.Subject.Text, maxTemplateParamIndex(s.Subject.Params))
		if b := s.BodyText; b != nil {
			add("send email: the body", b.Text, maxTemplateParamIndex(b.Params))
		}
		if b := s.BodyHTML; b != nil {
			add("send email: the HTML body", b.Text, maxTemplateParamIndex(b.Params))
		}
	case *ast.RestCallStmt:
		add("call rest service: the URL", s.URL, maxTemplateParamIndex(s.URLParams))
		if b := s.Body; b != nil && b.Type == ast.RestBodyCustom {
			add("call rest service: the body", b.Template, maxTemplateParamIndex(b.TemplateParams))
		}
	case *ast.CallJavaActionStmt:
		for _, arg := range s.Arguments {
			if len(arg.TemplateParams) > 0 {
				add(javaActionTemplateWhere(s.ActionName.String(), arg.Name), arg.Value, maxTemplateParamIndex(arg.TemplateParams))
			}
		}
	}
	return out
}

// javaActionTemplateWhere names a Java action's text-template argument the same
// way for check and exec.
func javaActionTemplateWhere(actionQN, param string) string {
	return fmt.Sprintf("call java action %s: the %s", actionQN, param)
}

// templateGrammarRefusal is one refused template text.
type templateGrammarRefusal struct{ msg, fix string }

func templateGrammarRefusalsOf(stmt ast.MicroflowStatement) []templateGrammarRefusal {
	var out []templateGrammarRefusal
	for _, site := range templateTextSites(stmt) {
		if msg, fix := templateTextRefusal(site.where, site.text, site.params); msg != "" {
			out = append(out, templateGrammarRefusal{msg, fix})
		}
	}
	return out
}

// templateGrammarRefusals returns the messages of every template text of stmt
// mxbuild refuses, or nil.
func templateGrammarRefusals(stmt ast.MicroflowStatement) []string {
	var out []string
	for _, r := range templateGrammarRefusalsOf(stmt) {
		out = append(out, r.msg)
	}
	return out
}

// checkTemplateGrammar surfaces the refusals as MDL-TEMPLATE02 for every
// statement of body, nested and error-handler bodies included.
func (v *microflowValidator) checkTemplateGrammar(body []ast.MicroflowStatement) {
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		for _, r := range templateGrammarRefusalsOf(s) {
			v.addViolation(templateGrammarRule, linter.SeverityError, r.msg, r.fix)
		}
	})
}

// templateGrammarViolations runs MDL-TEMPLATE02 alone over a flow that does not
// go through ValidateMicroflow — a rule.
func templateGrammarViolations(docType, name string, body []ast.MicroflowStatement) []linter.Violation {
	v := &microflowValidator{mfName: name, docType: docType}
	v.checkTemplateGrammar(body)
	return v.violations
}

// refuseTemplateGrammar records the refusals as build errors, so exec refuses
// what check reports as MDL-TEMPLATE02, --no-check included.
func (fb *flowBuilder) refuseTemplateGrammar(stmt ast.MicroflowStatement) {
	for _, r := range templateGrammarRefusalsOf(stmt) {
		fb.addError("%s\n  %s", r.msg, r.fix)
	}
}
