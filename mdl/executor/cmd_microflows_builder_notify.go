// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// setNotifyTarget resolves `notify workflow … target Module.Workflow.Name` and
// stores it the way the project's Mendix version does.
//
// The statement names the element, not its kind, and the stored target type
// depends on the kind (ako/TestApp, Studio Pro 11.14):
//
//	interrupting notification start       Workflows$InterruptingNotificationEventSubProcessStartActivityTarget
//	non-interrupting notification start   Workflows$NonInterruptingNotificationEventSubProcessStartActivityTarget
//	notification activity                 Workflows$NotifyNotificationActivityTarget
//	wait for notification                 Workflows$NotifyWaitForNotificationActivityTarget
//	notification boundary event           Workflows$NotifyNotificationBoundaryEventTarget (key BoundaryEvent)
//
// so the element is looked up in the stored workflow. Before Mendix 11.7 the
// action held the name directly (Activity), and only a wait for notification
// could be named.
func (fb *flowBuilder) setNotifyTarget(action *microflows.NotifyWorkflowAction, target string) {
	i := strings.LastIndex(target, ".")
	if strings.Count(target, ".") < 2 || i == len(target)-1 {
		fb.errors = append(fb.errors, fmt.Sprintf(
			"notify workflow target %q: name the element as Module.Workflow.Name", target))
		return
	}
	if fb.backend != nil {
		if pv := fb.backend.ProjectVersion(); pv != nil && !pv.IsAtLeast(11, 7) {
			action.Activity = target
			return
		}
	}
	wf := fb.findWorkflow(target[:i])
	if wf == nil {
		if fb.backend != nil {
			fb.errors = append(fb.errors, fmt.Sprintf(
				"workflow not found: %s (referenced by notify workflow target %s)", target[:i], target))
		}
		return
	}
	name := target[i+1:]
	typeName, problem, found := notifyTargetType(wf, name)
	switch {
	case !found:
		fb.errors = append(fb.errors, fmt.Sprintf(
			"notify workflow target %s: workflow %s has no element named %q. A notify reaches %s",
			target, target[:i], name, notifyTargetCandidates(wf)))
	case problem != "":
		fb.errors = append(fb.errors, fmt.Sprintf(
			"notify workflow target %s is %s — a notify reaches a notification start, a notification activity, "+
				"a notification boundary event or a wait for notification", target, problem))
	default:
		action.Target = &microflows.NotifyTarget{TypeName: typeName, Name: target}
	}
}

// findWorkflow returns the stored workflow with this qualified name, nil when
// there is none or no backend to ask.
func (fb *flowBuilder) findWorkflow(qualifiedName string) *workflows.Workflow {
	if fb.backend == nil || fb.hierarchy == nil {
		return nil
	}
	all, err := fb.backend.ListWorkflows()
	if err != nil {
		return nil
	}
	for _, wf := range all {
		if fb.hierarchy.GetModuleName(fb.hierarchy.FindModuleID(wf.ContainerID))+"."+wf.Name == qualifiedName {
			return wf
		}
	}
	return nil
}

// notifyTargetType returns the stored target type for the element named name.
// found reports whether any element has that name; problem describes an element
// a notification cannot reach.
func notifyTargetType(wf *workflows.Workflow, name string) (typeName, problem string, found bool) {
	visit := func(acts []workflows.WorkflowActivity) bool { return false }
	visit = func(acts []workflows.WorkflowActivity) bool {
		for _, act := range acts {
			for _, be := range activityBoundaryEvents(act) {
				if be != nil && be.Name == name {
					found = true
					if be.IsNotification() {
						typeName = microflows.NotifyBoundaryEventTargetType
					} else {
						problem = "a timer boundary event"
					}
					return true
				}
			}
			if act.GetName() == name {
				found = true
				switch a := act.(type) {
				case *workflows.EventSubProcessStartActivity:
					switch {
					case a.Timer:
						problem = "a timer-started event sub-process's start"
					case a.Interrupting:
						typeName = "Workflows$InterruptingNotificationEventSubProcessStartActivityTarget"
					default:
						typeName = "Workflows$NonInterruptingNotificationEventSubProcessStartActivityTarget"
					}
				case *workflows.NotificationActivity:
					typeName = "Workflows$NotifyNotificationActivityTarget"
				case *workflows.WaitForNotificationActivity:
					typeName = "Workflows$NotifyWaitForNotificationActivityTarget"
				default:
					problem = "a " + act.ActivityType() + " activity"
				}
				return true
			}
			for _, f := range nestedFlows(act) {
				if visit(f.Activities) {
					return true
				}
			}
		}
		return false
	}
	if wf.Flow != nil && visit(wf.Flow.Activities) {
		return typeName, problem, found
	}
	for _, esp := range wf.EventSubProcesses {
		if esp != nil && esp.Flow != nil && visit(esp.Flow.Activities) {
			return typeName, problem, found
		}
	}
	return "", "", false
}

// notifyTargetCandidates lists what the workflow offers a notify, for the error.
func notifyTargetCandidates(wf *workflows.Workflow) string {
	var names []string
	collect := func(acts []workflows.WorkflowActivity) {}
	collect = func(acts []workflows.WorkflowActivity) {
		for _, act := range acts {
			if t, problem, _ := notifyTargetType(wf, act.GetName()); t != "" && problem == "" {
				names = append(names, act.GetName())
			}
			for _, be := range activityBoundaryEvents(act) {
				if be != nil && be.IsNotification() {
					names = append(names, be.Name)
				}
			}
			for _, f := range nestedFlows(act) {
				collect(f.Activities)
			}
		}
	}
	if wf.Flow != nil {
		collect(wf.Flow.Activities)
	}
	for _, esp := range wf.EventSubProcesses {
		if esp != nil && esp.Flow != nil {
			collect(esp.Flow.Activities)
		}
	}
	if len(names) == 0 {
		return "a notification start, notification activity, notification boundary event or wait for notification, and this workflow has none."
	}
	sort.Strings(names)
	return "one of: " + strings.Join(names, ", ") + "."
}
