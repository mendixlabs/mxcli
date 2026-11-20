// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#833: an association path read from a snippet parameter, outside
// every data container, is what Studio Pro stores on TestApp's
// WorkflowCommons.Snip_Workflow_Header (`{1}` over System.Workflow_ParentWorkflow
// to Name) and Snip_WorkflowUserTaskView_Details (textBox9, Caption over
// WorkflowUserTaskView_WorkflowSubProcess): an AttributeRef with an
// IndirectEntityRef of steps, beside SourceVariable { SnippetParameter }.
// describe printed the path without its root and exec wrote it with no source:
// mx check CE0402 "No value specified." at Action button 'actionButton1', and
// check refused textBox9 (MDL-WIDGET34). `$Param/Assoc/Attr` — Mendix's own
// expression form, which the snippet's Visible condition already uses — is the
// spelling for the pair.
func rootedPathPB(t *testing.T) *pageBuilder {
	t.Helper()
	pb := ruleActionPB(t)
	pb.entityContext = "" // outside every data container
	pb.isSnippet = true
	pb.paramScope = map[string]model.ID{"Action": "e-action"}
	pb.paramEntityNames = map[string]string{"Action": "Rules.RuleAction"}
	return pb
}

func TestTemplateParamAssociationPathRootedAtParameter(t *testing.T) {
	pb := rootedPathPB(t)
	param := &pages.ClientTemplateParameter{}
	pb.resolveTemplateAttributePathFull("$Action/RuleAction_BusinessRule/Name", param)
	if param.AttributeRef != "Rules.BusinessRule.Name" || len(param.AttributeRefSteps) != 1 ||
		param.AttributeRefSteps[0].Association != "Rules.RuleAction_BusinessRule" {
		t.Errorf("AttributeRef %q steps %+v, want Rules.BusinessRule.Name over Rules.RuleAction_BusinessRule",
			param.AttributeRef, param.AttributeRefSteps)
	}
	if param.SourceVariable != "Action" || param.SourceVariableKind != "snippet" || param.SourceWidget != "" {
		t.Errorf("SourceVariable %q kind %q widget %q, want the snippet parameter Action",
			param.SourceVariable, param.SourceVariableKind, param.SourceWidget)
	}
}

func TestInputAssociationPathRootedAtParameter(t *testing.T) {
	pb := rootedPathPB(t)
	w := &ast.WidgetV3{Type: "TEXTBOX", Name: "textBox9", Properties: map[string]any{
		"Attribute": &ast.DataSourceV3{Type: "association", ContextVariable: "Action", Reference: "RuleAction_BusinessRule/Name"},
	}}
	attr := inputAttributeText(w)
	if attr != "$Action/RuleAction_BusinessRule/Name" {
		t.Fatalf("inputAttributeText = %q", attr)
	}
	path, steps, sv, err := pb.resolveInputBinding(w, attr)
	if err != nil {
		t.Fatal(err)
	}
	if path != "Rules.BusinessRule.Name" || len(steps) != 1 || sv == nil || sv.Variable != "Action" || sv.Kind != "snippet" {
		t.Errorf("binding = %q %+v %+v, want Rules.BusinessRule.Name, one step, the snippet parameter Action", path, steps, sv)
	}
	// The check no longer refuses it outside a data container.
	if msg := inputBindingProblem(w, pageArgContext{known: true}, true, nil); msg != "" {
		t.Errorf("inputBindingProblem: %s", msg)
	}
}

func TestDescribeAssociationPathRootedAtParameter(t *testing.T) {
	ref := map[string]any{
		"Attribute": "System.Workflow.Name",
		"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": []any{int32(2),
			map[string]any{"Association": "System.Workflow_ParentWorkflow", "DestinationEntity": "System.Workflow"}}},
	}
	sv := map[string]any{"$Type": "Forms$PageVariable", "SnippetParameter": "Workflow", "PageParameter": "", "Widget": "", "LocalVariable": ""}
	tpl := map[string]any{"Parameters": []any{int32(2), map[string]any{"AttributeRef": ref, "Expression": "", "SourceVariable": sv}}}
	if got := extractTextTemplateParameters(nil, tpl); len(got) != 1 || got[0] != "$Workflow/System.Workflow_ParentWorkflow/Name" {
		t.Errorf("template described %v, want [$Workflow/System.Workflow_ParentWorkflow/Name]", got)
	}
	// Control: the same path with no source variable stays context-relative.
	tpl = map[string]any{"Parameters": []any{int32(2), map[string]any{"AttributeRef": ref, "Expression": ""}}}
	if got := extractTextTemplateParameters(nil, tpl); len(got) != 1 || got[0] != "System.Workflow_ParentWorkflow/Name" {
		t.Errorf("control described %v", got)
	}
}

// textBox9 of Snip_WorkflowUserTaskView_Details, as stored.
func TestDescribeInputAssociationPathRootedAtParameter(t *testing.T) {
	w := map[string]any{
		"AttributeRef": map[string]any{
			"Attribute": "System.WorkflowSubProcess.Caption",
			"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": []any{int32(2),
				map[string]any{"Association": "WorkflowCommons.WorkflowUserTaskView_WorkflowSubProcess", "DestinationEntity": "System.WorkflowSubProcess"}}},
		},
		"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "SnippetParameter": "WorkflowUserTaskView", "PageParameter": "", "Widget": "", "LocalVariable": ""},
	}
	got := extractInputAttribute(nil, w)
	if !strings.HasPrefix(got, "$WorkflowUserTaskView/") || !strings.HasSuffix(got, "WorkflowUserTaskView_WorkflowSubProcess/Caption") {
		t.Errorf("described %q, want $WorkflowUserTaskView/…WorkflowUserTaskView_WorkflowSubProcess/Caption", got)
	}
}

// Through the template-parameter builder, not just the resolver: `$Var/…` is
// otherwise stored as an Expression (mendixlabs/mxcli#1406), and the
// expression form of this path is mx check CE0117 "Error(s) in expression." at
// Action button 'actionButton1'. A path that resolves from a parameter binds;
// one that does not stays the expression it was.
func TestTemplateParamsRootedPathBindsThroughTheBuilder(t *testing.T) {
	pb := rootedPathPB(t)
	got := pb.buildClientTemplateParams([]ast.ParamAssignmentV3{
		{Index: 1, Value: "$Action/RuleAction_BusinessRule/Name"},
		{Index: 2, Value: "$Customer/Name"}, // control: not a parameter here, an expression (#1406)
	})
	if len(got) != 2 {
		t.Fatalf("%d params", len(got))
	}
	if got[0].Expression != "" || got[0].AttributeRef != "Rules.BusinessRule.Name" || got[0].SourceVariable != "Action" {
		t.Errorf("rooted path: Expression %q AttributeRef %q SourceVariable %q, want the binding",
			got[0].Expression, got[0].AttributeRef, got[0].SourceVariable)
	}
	if got[1].Expression != "$Customer/Name" {
		t.Errorf("control: Expression %q, want $Customer/Name", got[1].Expression)
	}
}
