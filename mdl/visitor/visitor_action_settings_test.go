// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#721 L2: a client action's settings (disabled during execution,
// progress bar, progress message, confirmation) had no MDL spelling, so a
// describe → exec removed them. `with ( … )` after the action spells them.
package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func buttonAction(t *testing.T, action string) (*ast.ActionV3, []error) {
	t.Helper()
	prog, errs := Build("create page M.P (Title: 'T', Layout: A.L) {\n" +
		"  actionbutton b1 (Caption: 'Go', Action: " + action + ")\n}")
	if len(errs) > 0 {
		return nil, errs
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	a, _ := page.Widgets[0].Properties["Action"].(*ast.ActionV3)
	if a == nil {
		t.Fatalf("no action parsed from %q", action)
	}
	return a, nil
}

func TestActionSettings_MicroflowCall(t *testing.T) {
	a, errs := buttonAction(t, "call microflow M.Delete(Order = $currentObject) with ("+
		"DisabledDuringExecution: false, ProgressBar: blocking, ProgressMessage: 'Deleting…', "+
		"Confirmation: 'Delete it?', ProceedCaption: 'Delete', CancelCaption: 'Keep', "+
		"Asynchronous: true, FormValidations: none,)")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s := a.Settings
	if a.Type != "microflow" || len(a.Args) != 1 || s == nil {
		t.Fatalf("action = %+v, want a microflow call with one argument and settings", a)
	}
	if s.DisabledDuringExecution == nil || *s.DisabledDuringExecution {
		t.Errorf("DisabledDuringExecution = %v, want false", s.DisabledDuringExecution)
	}
	if s.ProgressBar != "Blocking" {
		t.Errorf("ProgressBar = %q, want the stored spelling Blocking", s.ProgressBar)
	}
	for name, got := range map[string]*string{
		"ProgressMessage": s.ProgressMessage, "Confirmation": s.Confirmation,
		"ProceedCaption": s.ProceedCaption, "CancelCaption": s.CancelCaption,
	} {
		if got == nil {
			t.Errorf("%s not set", name)
		}
	}
	if *s.ProgressMessage != "Deleting…" || *s.Confirmation != "Delete it?" ||
		*s.ProceedCaption != "Delete" || *s.CancelCaption != "Keep" {
		t.Errorf("texts = %q %q %q %q", *s.ProgressMessage, *s.Confirmation, *s.ProceedCaption, *s.CancelCaption)
	}
	if s.Asynchronous == nil || !*s.Asynchronous || s.FormValidations != "None" {
		t.Errorf("Asynchronous = %v, FormValidations = %q", s.Asynchronous, s.FormValidations)
	}
}

// Control: an action without the list has no settings, so it keeps the
// writer's defaults exactly as before.
func TestActionSettings_AbsentIsNil(t *testing.T) {
	a, errs := buttonAction(t, "call microflow M.F")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if a.Settings != nil {
		t.Errorf("Settings = %+v, want nil", a.Settings)
	}
}

func TestActionSettings_OtherActions(t *testing.T) {
	for _, action := range []string{
		"save changes close page with (DisabledDuringExecution: false)",
		"show page M.Q with (DisabledDuringExecution: false)",
		"call nanoflow M.N with (ProgressBar: NonBlocking, Confirmation: 'Sure?')",
		"sign out with (DisabledDuringExecution: false)",
	} {
		a, errs := buttonAction(t, action)
		if len(errs) > 0 {
			t.Errorf("%s: %v", action, errs)
			continue
		}
		if a.Settings == nil {
			t.Errorf("%s: settings not built", action)
		}
	}
}

// `create object E then show page P with (…)` is one stored action, the create.
func TestActionSettings_CreateObjectThenHoists(t *testing.T) {
	a, errs := buttonAction(t, "create object M.E then show page M.Q with (DisabledDuringExecution: false)")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if a.Settings == nil || a.Settings.DisabledDuringExecution == nil || *a.Settings.DisabledDuringExecution {
		t.Errorf("create's settings = %+v, want DisabledDuringExecution false", a.Settings)
	}
	if a.ThenAction == nil || a.ThenAction.Settings != nil {
		t.Errorf("then-action kept the settings: %+v", a.ThenAction)
	}
}

// R11: a meaningless form is an error, not a no-op.
func TestActionSettings_Refusals(t *testing.T) {
	for action, want := range map[string]string{
		"call microflow M.F with (Colour: 'red')":                        "unknown action setting",
		"save changes with (ProgressBar: Blocking)":                      "call microflow` and `call nanoflow` only",
		"call nanoflow M.N with (Asynchronous: true)":                    "call microflow` only",
		"call microflow M.F with (ProgressBar: fast)":                    "ProgressBar takes",
		"call microflow M.F with (DisabledDuringExecution: 'no')":        "true or false",
		"call microflow M.F with (Confirmation: Yes)":                    "takes a string",
		"call microflow M.F with (ProceedCaption: 'Go')":                 "write Confirmation",
		"call microflow M.F with (ProgressBar: None, ProgressBar: None)": "written twice",
	} {
		_, errs := buttonAction(t, action)
		if len(errs) == 0 {
			t.Errorf("%s: accepted, want an error mentioning %q", action, want)
			continue
		}
		joined := ""
		for _, e := range errs {
			joined += e.Error() + "\n"
		}
		if !strings.Contains(joined, want) {
			t.Errorf("%s: errors %q, want one mentioning %q", action, joined, want)
		}
	}
}

// ako/mxcli#832: the workflow events and their own settings.
func TestActionSettings_WorkflowActions(t *testing.T) {
	f, tr := false, true
	for _, tc := range []struct {
		action, typ, target string
		want                ast.ActionSettingsV3
	}{
		{"open user task", "openUserTask", "", ast.ActionSettingsV3{}},
		{"open user task with (AssignOnOpen: false, OpenWhenAssigned: true)", "openUserTask", "",
			ast.ActionSettingsV3{AssignOnOpen: &f, OpenWhenAssigned: &tr}},
		{"open workflow with (DefaultPage: M.Admin)", "openWorkflow", "", ast.ActionSettingsV3{DefaultPage: "M.Admin"}},
		{"call workflow M.WF with (ClosePage: false, Confirmation: 'Start?')", "callWorkflow", "M.WF",
			ast.ActionSettingsV3{ClosePage: &f}},
		{"complete task 'Done' with (ClosePage: false, Commit: false)", "completeTask", "",
			ast.ActionSettingsV3{ClosePage: &f, Commit: &f}},
	} {
		a, errs := buttonAction(t, tc.action)
		if len(errs) > 0 {
			t.Errorf("%s: %v", tc.action, errs)
			continue
		}
		if a.Type != tc.typ || a.Target != tc.target {
			t.Errorf("%s: type %q target %q, want %q %q", tc.action, a.Type, a.Target, tc.typ, tc.target)
		}
		s := a.Settings
		if s == nil {
			s = &ast.ActionSettingsV3{}
		}
		eq := func(a, b *bool) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
		if !eq(s.AssignOnOpen, tc.want.AssignOnOpen) || !eq(s.OpenWhenAssigned, tc.want.OpenWhenAssigned) ||
			!eq(s.ClosePage, tc.want.ClosePage) || !eq(s.Commit, tc.want.Commit) || s.DefaultPage != tc.want.DefaultPage {
			t.Errorf("%s: settings %+v, want %+v", tc.action, *s, tc.want)
		}
	}
}

func TestActionSettings_WorkflowRefusals(t *testing.T) {
	for action, want := range map[string]string{
		"open user task with (ClosePage: false)":          "`call workflow`, `save changes` and `cancel changes` only",
		"complete task 'X' with (AssignOnOpen: false)":    "`open user task` only",
		"call workflow M.WF with (ProgressBar: Blocking)": "`call microflow` and `call nanoflow` only",
		"call workflow M.WF with (Commit: false)":         "`complete task` only",
		"save changes with (DefaultPage: M.P)":            "`open workflow` only",
		"open workflow with (DefaultPage: 'M.P')":         "DefaultPage takes a page",
		"open user task with (AssignOnOpen: M.P)":         "takes no qualified name",
		"open user task with (OpenWhenAssigned: maybe)":   "true or false",
		"open workflow": "CE1834",
		"open workflow with (DisabledDuringExecution: false)": "DefaultPage: Module.Page",
	} {
		_, errs := buttonAction(t, action)
		if len(errs) == 0 {
			t.Errorf("%s: accepted, want an error mentioning %q", action, want)
			continue
		}
		joined := ""
		for _, e := range errs {
			joined += e.Error() + "\n"
		}
		if !strings.Contains(joined, want) {
			t.Errorf("%s: errors %q, want one mentioning %q", action, joined, want)
		}
	}
}
