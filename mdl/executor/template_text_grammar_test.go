// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1397 (mendixlabs/mxcli#1515): a template's text passed `mxcli check`
// whatever its braces held, and mxbuild then refused it:
//
//	log info node 'Mod' 'Item {number} not found';
//	  -> [CE0712] "Brace should be followed by a number of digits or another brace."
//	     at Log message activity 'Log message (info)'
//
// and the same for a REST call's `Body: template '{}'`. Every slot that stores
// a Mendix text template applies one grammar (mdl/texttemplate, measured on
// 10.24.24, 11.13.0 and 11.15.0); the rows here are the reported statements and
// one per slot, each with its measured code, and the controls mxbuild accepts.

func templateGrammarMicroflow(stmt string) string {
	return "create microflow M.Probe ($Msg: String, $P: System.User)\nbegin\n  " + stmt + ";\nend;\n"
}

func TestTemplateGrammar_MicroflowSlots(t *testing.T) {
	for _, c := range []struct {
		name, stmt string
		want       string // the CE code the refusal names, "" for none
	}{
		// The two reported statements, verbatim.
		{"log, the report", `log info node 'Mod' 'Item {number} not found'`, "CE0712"},
		{"REST body, the report", `$R = call rest service post 'https://example.com/x' (Body: template '{}') returns String`, "CE0712"},
		// One per slot, measured.
		{"log with parameters", `log info 'Item {x} is {1}' with ({1} = $Msg)`, "CE0712"},
		{"show message", `show message 'Done { 1 }' with ({1} = $Msg)`, "CE0712"},
		{"validation feedback", `validation feedback $P/Name message 'Bad {1x}' with ({1} = $Msg)`, "CE0714"},
		{"REST URL", `$R = call rest service get 'https://example.com/{id}' returns String`, "CE0712"},
		{"REST JSON body", `$R = call rest service post 'https://example.com/x' (Body: template '{"name": "{1}"}' with ({1} = $Msg)) returns String`, "CE0712"},
		{"email subject", sendEmailWith(`Subject: 'Order {0}' with ({1} = $Msg)`), "CE0716"},
		{"email body", sendEmailWith(`Subject: 'S', Body: template 'Hi {1' with ({1} = $Msg)`), "CE0713"},
		{"email HTML body with CSS", sendEmailWith(`Subject: 'S', HtmlBody: template '<style>p { color: red }</style>'`), "CE0712"},
		{"log, placeholder too big", `log info 'n {99999999999}' with ({1} = $Msg)`, "CE0717"},
		// A placeholder no parameter fills is the same template's CE0720.
		{"log, unfilled", `log info 'value {1}'`, "CE0720"},
		{"show message, beyond its parameters", `show message 'a {1} b {2}' with ({1} = $Msg)`, "CE0720"},
		// --- controls: each builds clean ---
		{"control: escaped braces", `log info 'a {{ b }} c'`, ""},
		{"control: escaped placeholder", `log info 'literally {{1}}'`, ""},
		{"control: lone closing brace", `log info 'a } b'`, ""},
		{"control: brace at the end", `log info 'abc {'`, ""},
		{"control: filled", `log info 'value {1}' with ({1} = $Msg)`, ""},
		{"control: leading zero", `log info 'value {01}' with ({1} = $Msg)`, ""},
		{"control: escaped JSON body", `$R = call rest service post 'https://example.com/x' (Body: template '{{"name": "{1}"}' with ({1} = $Msg)) returns String`, ""},
		{"control: expression message", `log info $Msg + ' {x}'`, ""},
		{"control: expression body", `$R = call rest service post 'https://example.com/x' (Body: $Msg) returns String`, ""},
		{"control: an unused parameter", `show message 'Done' with ({1} = $Msg)`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			create := firstCreateMicroflow(t, templateGrammarMicroflow(c.stmt))

			var rule string
			for _, v := range ValidateMicroflow(create) {
				if v.RuleID == templateGrammarRule {
					if rule != "" {
						t.Errorf("reported twice: %q and %q", rule, v.Message)
					}
					rule = v.Message
				}
			}
			if (rule != "") != (c.want != "") || !strings.Contains(rule, c.want) {
				t.Errorf("check: %s = %q, want one naming %q", templateGrammarRule, rule, c.want)
			}

			fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
				measurer: &layoutMeasurer{}, varTypes: map[string]string{"P": "System.User"},
				declaredVars: map[string]string{"Msg": "String"}}
			fb.buildFlowGraph(create.Body, create.ReturnType)
			var execErr string
			for _, e := range fb.GetErrors() {
				if strings.Contains(e, "MxBuild rejects") {
					execErr = e
				}
			}
			if (execErr != "") != (c.want != "") {
				t.Errorf("exec: refused = %v, want %v (errors: %q)", execErr != "", c.want != "", fb.GetErrors())
			}
			// exec prints the remedy under the message; check carries it as
			// the violation's suggestion.
			if c.want != "" && !strings.HasPrefix(execErr, rule+"\n  ") {
				t.Errorf("check and exec disagree:\n check: %q\n exec:  %q", rule, execErr)
			}
		})
	}
}

// The refusal names what to write instead, and for the brace case that is the
// escape — the remedy the reporter needed for both the log and the JSON body.
func TestTemplateGrammar_MessageNamesTheRemedy(t *testing.T) {
	create := firstCreateMicroflow(t, templateGrammarMicroflow(`log info node 'Mod' 'Item {number} not found'`))
	for _, v := range ValidateMicroflow(create) {
		if v.RuleID != templateGrammarRule {
			continue
		}
		for _, want := range []string{"log", "{number}", "CE0712", "Brace should be followed by a number of digits or another brace."} {
			if !strings.Contains(v.Message, want) {
				t.Errorf("message lacks %q: %s", want, v.Message)
			}
		}
		if !strings.Contains(v.Suggestion, "{{") || !strings.Contains(v.Suggestion, "with ({1} =") {
			t.Errorf("suggestion should name the {{ escape and a numbered placeholder: %s", v.Suggestion)
		}
		return
	}
	t.Fatal("not reported")
}

// A Java action's text-template argument is a template too. Without a project
// check knows an argument is one when it states `with (…)`; exec knows the
// parameter's type and checks the rest (javaActionTemplateValue).
func TestTemplateGrammar_JavaActionTemplateArgument(t *testing.T) {
	for _, c := range []struct {
		stmt string
		want bool
	}{
		{`$Ok = call java action M.JA(Subject = 'Order {nr}: {1}' with ({1} = $Msg))`, true},
		{`$Ok = call java action M.JA(Subject = 'Order {1}' with ({1} = $Msg))`, false},
		// A plain String argument is no template: its braces are text.
		{`$Ok = call java action M.JA(Subject = 'Order {nr}')`, false},
	} {
		create := firstCreateMicroflow(t, templateGrammarMicroflow(c.stmt))
		var got bool
		for _, v := range ValidateMicroflow(create) {
			if v.RuleID == templateGrammarRule {
				got = true
			}
		}
		if got != c.want {
			t.Errorf("%s: reported = %v, want %v", c.stmt, got, c.want)
		}
	}
}

// Every flow kind check validates that can hold a template: a nanoflow logs and
// shows messages, a rule logs, and `alter … insert` adds a statement to a
// stored flow.
func TestTemplateGrammar_InEveryFlowKind(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"microflow", templateGrammarMicroflow(`log info 'Item {number}'`)},
		{"nanoflow", "create nanoflow M.Probe ($Msg: String)\nbegin\n  show message 'Item {number}';\nend;\n"},
		{"rule", "create rule M.Probe ($Msg: String) returns Boolean\nbegin\n  log info 'Item {number}';\n  return true;\nend;\n"},
		{"alter fragment", "alter microflow M.Probe {\n  insert after $Result begin\n    log info 'Item {number}';\n  end;\n};\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var found bool
			for _, v := range ValidateProgram(parseMDL(t, c.src), "") {
				if v.RuleID == templateGrammarRule {
					found = true
				}
			}
			if !found {
				t.Errorf("%s not reported for a %s", templateGrammarRule, c.name)
			}
		})
	}
}

func TestTemplateGrammarSites_OtherStatementsHaveNone(t *testing.T) {
	if got := templateGrammarRefusals(&ast.ReturnStmt{}); len(got) != 0 {
		t.Errorf("return: %q, want none", got)
	}
	if got := templateGrammarRefusals(nil); len(got) != 0 {
		t.Errorf("nil: %q, want none", got)
	}
}
