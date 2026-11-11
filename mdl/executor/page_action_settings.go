// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// applyActionSettings copies an action's `with ( … )` settings onto the built
// client action (ako/mxcli#721 L2). The visitor has already refused a setting
// the action type does not have; the checks here are for a caller that built
// the AST by hand.
func applyActionSettings(a pages.ClientAction, s *ast.ActionSettingsV3) error {
	if s.DisabledDuringExecution != nil {
		exec := pages.ExecutionOf(a)
		if exec == nil {
			return mdlerrors.NewValidation("DisabledDuringExecution: this action has no execution to disable the widget during")
		}
		v := *s.DisabledDuringExecution
		exec.DisabledDuringExecution = &v
	}
	if err := applyWorkflowActionSettings(a, s); err != nil {
		return err
	}
	hasProgress := s.ProgressBar != "" || s.ProgressMessage != nil
	hasConfirmation := s.Confirmation != nil || s.ProceedCaption != nil || s.CancelCaption != nil
	hasMicroflowSettings := s.Asynchronous != nil || s.FormValidations != ""
	var flow *pages.FlowCallSettings
	switch x := a.(type) {
	case *pages.MicroflowClientAction:
		flow = &x.FlowCallSettings
	case *pages.NanoflowClientAction:
		flow = &x.FlowCallSettings
		if hasMicroflowSettings {
			return mdlerrors.NewValidation("Asynchronous and FormValidations are settings of `call microflow` only")
		}
	case *pages.CallWorkflowClientAction:
		// A workflow call asks a confirmation as a flow call does, and has no
		// progress bar.
		if hasProgress || hasMicroflowSettings {
			return mdlerrors.NewValidation("ProgressBar, ProgressMessage, Asynchronous and FormValidations are settings of `call microflow` and `call nanoflow` only")
		}
		var c confirmationHolder
		if err := c.apply(s); err != nil {
			return err
		}
		x.Confirmation = c.Confirmation
		return nil
	default:
		if hasProgress || hasConfirmation || hasMicroflowSettings {
			return mdlerrors.NewValidation("ProgressBar, ProgressMessage and Confirmation are settings of `call microflow` and `call nanoflow` only")
		}
		return nil
	}
	flow.ProgressBar = s.ProgressBar
	if s.ProgressMessage != nil {
		flow.ProgressMessage = authoringText(*s.ProgressMessage)
	}
	c := confirmationHolder{Confirmation: flow.Confirmation}
	if err := c.apply(s); err != nil {
		return err
	}
	flow.Confirmation = c.Confirmation
	if s.Asynchronous != nil {
		flow.Asynchronous = *s.Asynchronous
	}
	flow.FormValidations = s.FormValidations
	return nil
}

// confirmationHolder applies Confirmation, ProceedCaption and CancelCaption,
// which a flow call and a workflow call share.
type confirmationHolder struct {
	Confirmation *pages.ConfirmationInfo
}

func (h *confirmationHolder) apply(s *ast.ActionSettingsV3) error {
	if s.Confirmation != nil {
		// Studio Pro's captions for a new confirmation: measured on
		// ako/TestApp, where the one confirmation whose captions were never
		// edited stores exactly these.
		proceed, cancel := "Proceed", "Cancel"
		if s.ProceedCaption != nil {
			proceed = *s.ProceedCaption
		}
		if s.CancelCaption != nil {
			cancel = *s.CancelCaption
		}
		h.Confirmation = &pages.ConfirmationInfo{
			Question:       authoringText(*s.Confirmation),
			ProceedCaption: authoringText(proceed),
			CancelCaption:  authoringText(cancel),
		}
	} else if s.ProceedCaption != nil || s.CancelCaption != nil {
		return mdlerrors.NewValidation("ProceedCaption and CancelCaption are the buttons of a confirmation; write Confirmation: '<question>' as well")
	}
	return nil
}

// applyWorkflowActionSettings copies the workflow events' own settings
// (ako/mxcli#832): `open user task`'s AssignOnOpen and OpenWhenAssigned, and
// ClosePage and Commit on `complete task` (ClosePage also on `call workflow`).
// DefaultPage names a page, so the builder resolves it.
func applyWorkflowActionSettings(a pages.ClientAction, s *ast.ActionSettingsV3) error {
	if s.AssignOnOpen != nil || s.OpenWhenAssigned != nil {
		x, ok := a.(*pages.OpenUserTaskClientAction)
		if !ok {
			return mdlerrors.NewValidation("AssignOnOpen and OpenWhenAssigned are settings of `open user task` only")
		}
		if s.AssignOnOpen != nil {
			v := *s.AssignOnOpen
			x.AssignOnOpen = &v
		}
		if s.OpenWhenAssigned != nil {
			v := *s.OpenWhenAssigned
			x.OpenWhenAssigned = &v
		}
	}
	if s.ClosePage != nil {
		switch x := a.(type) {
		case *pages.SetTaskOutcomeClientAction:
			x.ClosePage = *s.ClosePage
		case *pages.CallWorkflowClientAction:
			x.ClosePage = *s.ClosePage
		default:
			return mdlerrors.NewValidation("ClosePage is a setting of `complete task` and `call workflow` only")
		}
	}
	if s.Commit != nil {
		x, ok := a.(*pages.SetTaskOutcomeClientAction)
		if !ok {
			return mdlerrors.NewValidation("Commit is a setting of `complete task` only")
		}
		x.Commit = *s.Commit
	}
	if _, ok := a.(*pages.OpenWorkflowClientAction); s.DefaultPage != "" && !ok {
		return mdlerrors.NewValidation("DefaultPage is a setting of `open workflow` only")
	}
	if s.NumberOfPagesToClose != "" {
		x, ok := a.(*pages.PageClientAction)
		if !ok {
			return mdlerrors.NewValidation("NumberOfPagesToClose is a setting of `show page` only")
		}
		x.NumberOfPagesToClose = s.NumberOfPagesToClose
	}
	return nil
}

// authoringText is s as a text in the project's authoring language; the
// rewrite carries a stored text's other translations (ako/mxcli#705).
//
// The empty string is a text with no translations, not an empty one: that is
// how Studio Pro stores a progress message left blank (all 7 blank ones in PedApp and
// ako/TestApp), and an en_US "" in its place is a rewrite of every such button.
func authoringText(s string) *model.Text {
	if s == "" {
		return &model.Text{}
	}
	return &model.Text{Translations: map[string]string{model.AuthoringLanguage(): s}}
}
