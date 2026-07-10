// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	genWf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/workflows"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// ListWorkflows reads every Workflows$Workflow unit and converts it to the
// semantic type, mirroring the legacy (*mpr.Reader).ListWorkflows for the
// top-level fields plus the flow/activity tree the catalog's references walker
// consumes: the context Parameter entity, OverviewPage, and the activity types
// that carry references (user tasks → page/entity/user-source/outcome flows,
// call-microflow / call-workflow tasks, exclusive and parallel splits).
//
// Documented gap: boundary events, parameter mappings, jump-to targets and the
// completion/criteria sub-parts of multi-user tasks are not reconstructed — no
// catalog or describe path that reaches this method reads them. Unrecognised
// activity types decode to a GenericWorkflowActivity carrying their $Type, so
// no activity is silently dropped.
func (b *Backend) ListWorkflows() ([]*workflows.Workflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genWf.Workflow](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*workflows.Workflow, 0, len(units))
	for _, u := range units {
		g := u.Element
		w := &workflows.Workflow{
			ContainerID:         model.ID(u.ContainerID),
			Name:                g.Name(),
			Documentation:       g.Documentation(),
			ExportLevel:         g.ExportLevel(),
			Excluded:            g.Excluded(),
			OverviewPage:        g.OverviewPageQualifiedName(),
			DueDate:             g.DueDate(),
			WorkflowName:        workflowTemplateText(g.WorkflowName()),
			WorkflowDescription: workflowTemplateText(g.WorkflowDescription()),
		}
		w.ID = model.ID(g.ID())
		w.TypeName = "Workflows$Workflow"
		w.Annotation = annotationText(g.Annotation())
		if p, ok := g.Parameter().(*genWf.Parameter); ok && p != nil {
			wp := &workflows.WorkflowParameter{EntityRef: p.EntityQualifiedName()}
			wp.ID = model.ID(p.ID())
			w.Parameter = wp
		}
		if f, ok := g.Flow().(*genWf.Flow); ok && f != nil {
			w.Flow = workflowFlowFromGen(f)
		}
		out = append(out, w)
	}
	return out, nil
}

// workflowFlowFromGen converts a gen Flow to the semantic Flow.
func workflowFlowFromGen(g *genWf.Flow) *workflows.Flow {
	f := &workflows.Flow{}
	f.ID = model.ID(g.ID())
	for _, actEl := range g.ActivitiesItems() {
		if a := workflowActivityFromGen(actEl); a != nil {
			f.Activities = append(f.Activities, a)
		}
	}
	return f
}

// workflowActivityFromGen dispatches a gen activity element to its semantic type.
func workflowActivityFromGen(el element.Element) workflows.WorkflowActivity {
	switch a := el.(type) {
	case *genWf.UserTask:
		t := &workflows.UserTask{
			Page:            a.PageQualifiedName(),
			UserTaskEntity:  a.UserTaskEntityQualifiedName(),
			TaskName:        workflowTemplateText(a.TaskName()),
			TaskDescription: workflowTemplateText(a.TaskDescription()),
			DueDate:         a.DueDate(),
			UserSource:      userSourceFromGen(a.UserSource(), nil),
			OnCreated:       microflowEventName(a.OnCreatedEvent()),
		}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$UserTask")
		if t.Page == "" {
			t.Page = taskPageName(a.TaskPage())
		}
		t.Outcomes = userTaskOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.SingleUserTaskActivity:
		t := &workflows.UserTask{
			TaskName:        workflowTemplateText(a.TaskName()),
			TaskDescription: workflowTemplateText(a.TaskDescription()),
			DueDate:         a.DueDate(),
			UserSource:      userSourceFromGen(a.UserSource(), a.UserTargeting()),
			OnCreated:       microflowEventName(a.OnCreatedEvent()),
			Page:            taskPageName(a.TaskPage()),
		}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$SingleUserTaskActivity")
		t.Outcomes = userTaskOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.MultiUserTaskActivity:
		t := &workflows.UserTask{
			IsMulti:         true,
			TaskName:        workflowTemplateText(a.TaskName()),
			TaskDescription: workflowTemplateText(a.TaskDescription()),
			DueDate:         a.DueDate(),
			UserSource:      userSourceFromGen(a.UserSource(), a.UserTargeting()),
			OnCreated:       microflowEventName(a.OnCreatedEvent()),
			Page:            taskPageName(a.TaskPage()),
		}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$MultiUserTaskActivity")
		t.Outcomes = userTaskOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.CallMicroflowTask:
		t := &workflows.CallMicroflowTask{Microflow: a.MicroflowQualifiedName()}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$CallMicroflowTask")
		t.Outcomes = conditionOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.CallMicroflowActivity:
		t := &workflows.CallMicroflowTask{Microflow: a.MicroflowQualifiedName()}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$CallMicroflowActivity")
		t.Outcomes = conditionOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.CallWorkflowActivity:
		t := &workflows.CallWorkflowActivity{
			Workflow:            a.WorkflowQualifiedName(),
			ParameterExpression: a.ParameterExpression(),
		}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$CallWorkflowActivity")
		return t
	case *genWf.ExclusiveSplitActivity:
		t := &workflows.ExclusiveSplitActivity{Expression: a.Expression()}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$ExclusiveSplitActivity")
		t.Outcomes = conditionOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.ParallelSplitActivity:
		t := &workflows.ParallelSplitActivity{}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$ParallelSplitActivity")
		for _, oEl := range a.OutcomesItems() {
			if o, ok := oEl.(*genWf.ParallelSplitOutcome); ok {
				out := &workflows.ParallelSplitOutcome{}
				out.ID = model.ID(o.ID())
				if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
					out.Flow = workflowFlowFromGen(f)
				}
				t.Outcomes = append(t.Outcomes, out)
			}
		}
		return t
	default:
		t := &workflows.GenericWorkflowActivity{TypeString: el.TypeName()}
		t.ID = model.ID(el.ID())
		t.TypeName = el.TypeName()
		return t
	}
}

// userTaskOutcomesFromGen converts gen user-task outcomes to semantic ones.
func userTaskOutcomesFromGen(items []element.Element) []*workflows.UserTaskOutcome {
	var out []*workflows.UserTaskOutcome
	for _, oEl := range items {
		o, ok := oEl.(*genWf.UserTaskOutcome)
		if !ok {
			continue
		}
		uto := &workflows.UserTaskOutcome{
			Name:    o.Name(),
			Caption: o.Caption(),
			Value:   o.Value(),
		}
		uto.ID = model.ID(o.ID())
		if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
			uto.Flow = workflowFlowFromGen(f)
		}
		out = append(out, uto)
	}
	return out
}

// conditionOutcomesFromGen converts gen condition outcomes to semantic ones,
// mirroring the legacy parseConditionOutcomes dispatch.
func conditionOutcomesFromGen(items []element.Element) []workflows.ConditionOutcome {
	var out []workflows.ConditionOutcome
	for _, el := range items {
		switch o := el.(type) {
		case *genWf.BooleanConditionOutcome:
			c := &workflows.BooleanConditionOutcome{Value: o.Value()}
			c.ID = model.ID(o.ID())
			if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
				c.Flow = workflowFlowFromGen(f)
			}
			out = append(out, c)
		case *genWf.EnumerationValueConditionOutcome:
			c := &workflows.EnumerationValueConditionOutcome{Value: o.ValueQualifiedName()}
			c.ID = model.ID(o.ID())
			if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
				c.Flow = workflowFlowFromGen(f)
			}
			out = append(out, c)
		case *genWf.VoidConditionOutcome:
			c := &workflows.VoidConditionOutcome{}
			c.ID = model.ID(o.ID())
			if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
				c.Flow = workflowFlowFromGen(f)
			}
			out = append(out, c)
		}
	}
	return out
}

// userSourceFromGen resolves the polymorphic user source / user targeting part,
// mirroring the legacy parseUserSource.
func userSourceFromGen(source, targeting element.Element) workflows.UserSource {
	el := source
	if el == nil {
		el = targeting
	}
	switch s := el.(type) {
	case *genWf.MicroflowBasedUserSource:
		return &workflows.MicroflowBasedUserSource{Microflow: s.MicroflowQualifiedName()}
	case *genWf.MicroflowUserTargeting:
		return &workflows.MicroflowBasedUserSource{Microflow: s.MicroflowQualifiedName()}
	case *genWf.MicroflowGroupTargeting:
		return &workflows.MicroflowGroupSource{Microflow: s.MicroflowQualifiedName()}
	case *genWf.XPathBasedUserSource:
		return &workflows.XPathBasedUserSource{XPath: s.XPathConstraint()}
	case *genWf.XPathUserTargeting:
		return &workflows.XPathBasedUserSource{XPath: s.XPathConstraint()}
	default:
		return &workflows.NoUserSource{}
	}
}

// microflowEventName extracts the microflow qualified name from an OnCreated
// event part (Workflows$MicroflowBasedEvent).
func microflowEventName(el element.Element) string {
	if ev, ok := el.(*genWf.MicroflowBasedEvent); ok && ev != nil {
		return ev.MicroflowQualifiedName()
	}
	return ""
}

// taskPageName extracts the page qualified name from a TaskPage part
// (Workflows$PageReference).
func taskPageName(el element.Element) string {
	if pr, ok := el.(*genWf.PageReference); ok && pr != nil {
		return pr.PageQualifiedName()
	}
	return ""
}

// annotationText extracts the description text from a Workflows$Annotation part.
func annotationText(el element.Element) string {
	if an, ok := el.(*genWf.Annotation); ok && an != nil {
		return an.Description()
	}
	return ""
}

// workflowTemplateText extracts the Text of a Microflows$StringTemplate part,
// mirroring the legacy extractStringTemplate.
func workflowTemplateText(el element.Element) string {
	if st, ok := el.(*genMf.StringTemplate); ok && st != nil {
		return st.Text()
	}
	return ""
}

// setWfBase fills a semantic BaseWorkflowActivity from common gen accessors.
func setWfBase(a *workflows.BaseWorkflowActivity, id element.ID, name, caption string, annotation element.Element, typeName string) {
	a.ID = model.ID(id)
	a.Name = name
	a.Caption = caption
	a.TypeName = typeName
	a.Annotation = annotationText(annotation)
}
