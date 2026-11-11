// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// renderClientActionSettingsMDL renders a client action's settings as
// ` with (Key: value, …)`, or "" when every one holds the writer's default
// (ako/mxcli#721 L2).
//
// Only a value that differs from what exec writes when the list is absent is
// printed, so a page whose actions are all defaults describes exactly as
// before. The defaults are the writer's: DisabledDuringExecution true,
// ProgressBar None, no progress message, no confirmation, Asynchronous false,
// FormValidations All. Before these had a spelling, the writer put the
// defaults back on every rewrite, which removed a button's confirmation and
// progress bar and changed whether a widget is disabled while its action
// runs — a behaviour change neither `mx check` nor the build reports.
func renderClientActionSettingsMDL(ctx *ExecContext, action map[string]any) string {
	var props []string
	if dde, ok := action["DisabledDuringExecution"].(bool); ok && !dde {
		props = append(props, "DisabledDuringExecution: false")
	}
	typeName, _ := action["$Type"].(string)
	var flow map[string]any
	switch typeName {
	case "Forms$MicroflowAction", "Pages$MicroflowClientAction":
		flow = actionMapForKey(action, "MicroflowSettings")
	case "Forms$CallNanoflowClientAction", "Pages$CallNanoflowClientAction":
		flow = action
	case "Forms$CallWorkflowClientAction", "Pages$CallWorkflowClientAction":
		// A workflow call has a confirmation and no progress bar, and closes
		// its page unless told not to.
		flow = action
		if closePage, ok := action["ClosePage"].(bool); ok && !closePage {
			props = append(props, "ClosePage: false")
		}
	case "Forms$SetTaskOutcomeClientAction", "Pages$SetTaskOutcomeClientAction":
		for _, key := range []string{"ClosePage", "Commit"} {
			if v, ok := action[key].(bool); ok && !v {
				props = append(props, key+": false")
			}
		}
	case "Forms$OpenUserTaskClientAction", "Pages$OpenUserTaskClientAction":
		// Studio Pro's new action: auto-assign on, only the assignee opens it.
		if v, ok := action["AssignOnOpen"].(bool); ok && !v {
			props = append(props, "AssignOnOpen: false")
		}
		if v, ok := action["OpenWhenAssigned"].(bool); ok && v {
			props = append(props, "OpenWhenAssigned: true")
		}
	case "Forms$OpenWorkflowClientAction", "Pages$OpenWorkflowClientAction":
		if page, _ := action["DefaultPage"].(string); page != "" {
			props = append(props, "DefaultPage: "+page)
		}
	case "Forms$FormAction", "Pages$PageClientAction":
		if n, _ := action["NumberOfPagesToClose2"].(string); strings.TrimSpace(n) != "" {
			props = append(props, "NumberOfPagesToClose: "+strings.TrimSpace(n))
		}
	}
	if flow != nil {
		if pb, _ := flow["ProgressBar"].(string); pb != "" && pb != "None" {
			props = append(props, "ProgressBar: "+pb)
		}
		if msg := actionMapForKey(flow, "ProgressMessage"); msg != nil {
			props = append(props, "ProgressMessage: "+mdlQuote(ctx, settingText(ctx, msg)))
		}
		if ci := actionMapForKey(flow, "ConfirmationInfo"); ci != nil {
			props = append(props, "Confirmation: "+mdlQuote(ctx, settingText(ctx, actionMapForKey(ci, "Question"))))
			// Both captions always print: a confirmation written without them
			// gets Studio Pro's defaults, which need not be what is stored.
			props = append(props, "ProceedCaption: "+mdlQuote(ctx, settingText(ctx, actionMapForKey(ci, "ProceedButtonCaption"))))
			props = append(props, "CancelCaption: "+mdlQuote(ctx, settingText(ctx, actionMapForKey(ci, "CancelButtonCaption"))))
		}
		if flow["$Type"] == "Forms$MicroflowSettings" {
			if async, _ := flow["Asynchronous"].(bool); async {
				props = append(props, "Asynchronous: true")
			}
			if fv, _ := flow["FormValidations"].(string); fv != "" && fv != "All" {
				props = append(props, "FormValidations: "+fv)
			}
		}
	}
	if len(props) == 0 {
		return ""
	}
	return " with (" + strings.Join(props, ", ") + ")"
}

// settingText is a Texts$Text in the describe language.
func settingText(ctx *ExecContext, text map[string]any) string {
	if text == nil {
		return ""
	}
	return selectTranslationText(getBsonArrayElements(text["Items"]), describeDefaultLanguage(ctx))
}

// formatActionSettingsV3 renders parsed settings back as ` with ( … )`, in the
// order describe prints them — for SHOW FRAGMENT, which prints the AST.
func formatActionSettingsV3(s *ast.ActionSettingsV3) string {
	if s == nil {
		return ""
	}
	var props []string
	boolText := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}
	if s.DisabledDuringExecution != nil {
		props = append(props, "DisabledDuringExecution: "+boolText(*s.DisabledDuringExecution))
	}
	if s.ProgressBar != "" {
		props = append(props, "ProgressBar: "+s.ProgressBar)
	}
	for _, kv := range []struct {
		key string
		val *string
	}{
		{"ProgressMessage", s.ProgressMessage}, {"Confirmation", s.Confirmation},
		{"ProceedCaption", s.ProceedCaption}, {"CancelCaption", s.CancelCaption},
	} {
		if kv.val != nil {
			props = append(props, kv.key+": "+mdlQuote(nil, *kv.val))
		}
	}
	if s.Asynchronous != nil {
		props = append(props, "Asynchronous: "+boolText(*s.Asynchronous))
	}
	if s.FormValidations != "" {
		props = append(props, "FormValidations: "+s.FormValidations)
	}
	for _, kv := range []struct {
		key string
		val *bool
	}{
		{"AssignOnOpen", s.AssignOnOpen}, {"OpenWhenAssigned", s.OpenWhenAssigned},
		{"ClosePage", s.ClosePage}, {"Commit", s.Commit},
	} {
		if kv.val != nil {
			props = append(props, kv.key+": "+boolText(*kv.val))
		}
	}
	if s.DefaultPage != "" {
		props = append(props, "DefaultPage: "+s.DefaultPage)
	}
	if s.NumberOfPagesToClose != "" {
		props = append(props, "NumberOfPagesToClose: "+s.NumberOfPagesToClose)
	}
	if len(props) == 0 {
		return ""
	}
	return " with (" + strings.Join(props, ", ") + ")"
}
