// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#2107: a widget bound to an attribute of an OUTER data view's object.
// Studio Pro stores the binding with the data-view pair as its source,
// SourceVariable {Widget: <outer data view>, PageParameter: <its parameter>}
// (FactoryManagement, Mendix 11.16):
//
//	ConversationalUI.Span_ToolSpan_SlideIn  markdownViewer1.stringAttribute
//	  AttributeRef GenAICommons.Span.Output, SourceVariable {Widget: dataView1, PageParameter: ToolSpan}
//	ConversationalUI.Trace_Details  container112 Visible
//	  Attribute GenAICommons.Trace.HasError, SourceVariable {Widget: dataView6, PageParameter: Trace}
//
// describe printed the bare qualified name, which the rewrite resolved against the
// NEAREST data view: "`GenAICommons.Span.Output` is an attribute of
// GenAICommons.Span, not of ConversationalUI.CopyHelper … CE7247", and
// "ConversationalUI.TraceView has no attribute HasError". The pair already has
// an MDL spelling on input widgets, `$dataView1.Attr` (ako/mxcli#826).
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func outerDataViewPair(widget, param string) map[string]any {
	return map[string]any{"$Type": "Forms$PageVariable", "Widget": widget, "PageParameter": param}
}

func markdownInNestedDataView(sourceVariable any) map[string]any {
	md := map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  "markdownViewer1",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.markdown.Markdown",
			"ObjectType": map[string]any{"PropertyTypes": []any{
				map[string]any{"$ID": "pt-str", "PropertyKey": "stringAttribute",
					"ValueType": map[string]any{"Type": "Attribute"}},
			}},
		},
		"Object": map[string]any{"Properties": []any{
			map[string]any{"TypePointer": "pt-str", "Value": map[string]any{
				"AttributeRef":   map[string]any{"Attribute": "GenAICommons.Span.Output"},
				"SourceVariable": sourceVariable,
			}},
		}},
	}
	inner := map[string]any{
		"$Type": "Forms$DataView", "Name": "dataView3",
		"DataSource": map[string]any{"$Type": "Forms$DataViewSource",
			"EntityRef": map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "ConversationalUI.CopyHelper"}},
		"Widgets": []any{md},
	}
	return map[string]any{
		"$Type": "Forms$DataView", "Name": "dataView1",
		"DataSource": map[string]any{"$Type": "Forms$DataViewSource",
			"EntityRef":      map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "GenAICommons.ToolSpan"},
			"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "ToolSpan"}},
		"Widgets": []any{inner},
	}
}

func describedMarkdownAttribute(t *testing.T, sourceVariable any) string {
	t.Helper()
	ctx, _ := newMockCtx(t)
	outer := parseRawWidget(ctx, markdownInNestedDataView(sourceVariable))
	if len(outer) != 1 || len(outer[0].Children) != 1 || len(outer[0].Children[0].Children) != 1 {
		t.Fatalf("want dataView1 > dataView3 > markdown, got %+v", outer)
	}
	for _, p := range outer[0].Children[0].Children[0].ExplicitProperties {
		if p.Key == "stringAttribute" {
			return p.Value
		}
	}
	t.Fatal("no stringAttribute described")
	return ""
}

func TestDescribePluggableAttributeThroughOuterDataView(t *testing.T) {
	if got := describedMarkdownAttribute(t, outerDataViewPair("dataView1", "ToolSpan")); got != "$dataView1.Output" {
		t.Errorf("stringAttribute = %q, want $dataView1.Output (the stored data-view pair)", got)
	}
	// Control: with no SourceVariable the qualified name stays, as before.
	if got := describedMarkdownAttribute(t, nil); got != "GenAICommons.Span.Output" {
		t.Errorf("stringAttribute without a source = %q, want GenAICommons.Span.Output", got)
	}
}

func describedVisibleAttr(t *testing.T, sourceVariable any) string {
	t.Helper()
	ctx, _ := newMockCtx(t)
	var w rawWidget
	extractConditionalSettings(ctx, &w, map[string]any{"ConditionalVisibilitySettings": map[string]any{
		"Attribute":      "GenAICommons.Trace.HasError",
		"SourceVariable": sourceVariable,
		"Conditions": []any{int32(2),
			map[string]any{"AttributeValue": "true", "EditableVisible": true},
			map[string]any{"AttributeValue": "false", "EditableVisible": false}},
	}})
	return w.VisibleAttr
}

func TestDescribeVisibleAttributeThroughOuterDataView(t *testing.T) {
	if got := describedVisibleAttr(t, outerDataViewPair("dataView6", "Trace")); got != "$dataView6.HasError" {
		t.Errorf("Visible attribute = %q, want $dataView6.HasError", got)
	}
	// Control: a parameter source stays `$Param.Attr`.
	if got := describedVisibleAttr(t, map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "Trace"}); got != "$Trace.HasError" {
		t.Errorf("Visible attribute from a parameter = %q, want $Trace.HasError", got)
	}
}

// The builder half: `$dataView1.Attr` on a Visible-by-attribute setting and on
// a pluggable widget's attribute key writes the data-view pair, as it already
// did on an input widget.

func TestBuildVisibleWhen_ThroughOuterDataView(t *testing.T) {
	pb := scopedPB(false)
	pb.paramEntityNames["dataView1"] = "M.Job"
	defer pb.registerDataViewVariable("dataView1", &pages.DataViewSource{ParameterName: "Task"})()
	cvs, err := buildVisibleWhen(t, pb, "$dataView1.Status", "Running")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cvs == nil || cvs.Attribute != "M.Job.Status" {
		t.Fatalf("settings = %+v, want Attribute M.Job.Status", cvs)
	}
	if want := (pages.WidgetVariable{Widget: "dataView1", Variable: "Task"}); cvs.SourceVariable == nil || *cvs.SourceVariable != want {
		t.Errorf("SourceVariable = %+v, want %+v", cvs.SourceVariable, want)
	}
	// Control: a name that is neither a parameter nor an enclosing data view.
	if _, err := buildVisibleWhen(t, scopedPB(false), "$dataView9.Status", "Running"); err == nil {
		t.Error("Visible: $dataView9.Status built; want a refusal")
	}
}

func TestBuildPluggableAttribute_ThroughOuterDataView(t *testing.T) {
	e := scopeEngine(t)
	e.pageBuilder.paramEntityNames["dvOuter"] = "Sales.Customer"
	defer e.pageBuilder.registerDataViewVariable("dvOuter", &pages.DataViewSource{ParameterName: "Ctx"})()
	w := twoSourceWidget(map[string]any{"staticAttribute": &ast.WidgetExpressionV3{Text: "$dvOuter.Name"}})
	widget, err := e.Build(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	val := widgetValueFor(widget.RawObject, "Sales.Customer.Name")
	if val == nil {
		t.Fatal("no WidgetValue binds Sales.Customer.Name — the binding was dropped")
	}
	sv, ok := dGet(val, "SourceVariable").(bson.D)
	if !ok {
		t.Fatalf("SourceVariable = %#v, want the data-view pair", dGet(val, "SourceVariable"))
	}
	for k, want := range map[string]any{"Widget": "dvOuter", "PageParameter": "Ctx"} {
		if got := dGet(sv, k); got != want {
			t.Errorf("SourceVariable.%s = %#v, want %#v", k, got, want)
		}
	}
	// Control: a name that is no data view and no parameter is still refused.
	w = twoSourceWidget(map[string]any{"staticAttribute": &ast.WidgetExpressionV3{Text: "$dvNope.Name"}})
	if _, err := scopeEngine(t).Build(multiSourceDef(), w); err == nil {
		t.Error("staticAttribute: $dvNope.Name built; want a refusal")
	}
}

// The check half: MDL-WIDGET34 accepted `Visible: $name.Attr in (…)` for a
// parameter only, so describe's `$dataView6.HasError` would be refused.
func TestCheck_VisibleAttributeThroughOuterDataView(t *testing.T) {
	widget34 := func(src string) int {
		n := 0
		for _, v := range ValidateProgram(parseMDL(t, src), "") {
			if v.RuleID == "MDL-WIDGET34" {
				n++
			}
		}
		return n
	}
	page := func(inner string) string {
		return `mdl 1;
create page M.Trace ( Title: 'T', Layout: Atlas_Core.PopupLayout, Params: ( $Trace: M.Job ) ) {
  dataview dvOuter ( DataSource: $Trace ) {
    dataview dvInner ( DataSource: microflow M.DS_View() ) {
      container c1 ( Visible: $dvOuter.Status in (Running) )
    }
  }
  ` + inner + `
};`
	}
	if n := widget34(page("")); n != 0 {
		t.Errorf("`$dvOuter.Status` inside dvOuter: %d MDL-WIDGET34, want 0", n)
	}
	// Control: the same name outside dvOuter reads nothing.
	if n := widget34(page("container c2 ( Visible: $dvOuter.Status in (Running) )")); n != 1 {
		t.Errorf("`$dvOuter.Status` outside dvOuter: %d MDL-WIDGET34, want 1", n)
	}
}

// The pair with association steps: ConversationalUI.Span_TextGeneration_SlideIn's
// markdownViewer3 reads GenAICommons.Trace.SystemPrompt over Span_Trace from
// dataView1's object, SourceVariable {Widget: dataView1, PageParameter:
// ModelSpan}. The rooted spelling is #833's `$Root/Assoc/Attr`.
func TestDescribePluggableAttributePathThroughOuterDataView(t *testing.T) {
	ctx, _ := newMockCtx(t)
	w := markdownInNestedDataView(outerDataViewPair("dataView1", "ModelSpan"))
	md := w["Widgets"].([]any)[0].(map[string]any)["Widgets"].([]any)[0].(map[string]any)
	prop := md["Object"].(map[string]any)["Properties"].([]any)[0].(map[string]any)
	prop["Value"].(map[string]any)["AttributeRef"] = map[string]any{
		"Attribute": "GenAICommons.Trace.SystemPrompt",
		"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": []any{int32(2),
			map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": "GenAICommons.Span_Trace", "DestinationEntity": "GenAICommons.Trace"}}},
	}
	outer := parseRawWidget(ctx, w)
	var got string
	for _, p := range outer[0].Children[0].Children[0].ExplicitProperties {
		if p.Key == "stringAttribute" {
			got = p.Value
		}
	}
	if !strings.HasPrefix(got, "$dataView1/") || !strings.HasSuffix(got, "Span_Trace/SystemPrompt") {
		t.Errorf("stringAttribute = %q, want $dataView1/…Span_Trace/SystemPrompt", got)
	}
}

func associationEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-sales")}, Name: "Sales"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-sales")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-invoice")}, Name: "Invoice",
						Attributes: []*domainmodel.Attribute{{Name: "Reference"}, {Name: "Label"}}},
					{BaseElement: model.BaseElement{ID: model.ID("e-order")}, Name: "Order",
						Attributes: []*domainmodel.Attribute{{Name: "Number"}, {Name: "Label"}}},
					{BaseElement: model.BaseElement{ID: model.ID("e-customer")}, Name: "Customer",
						Attributes: []*domainmodel.Attribute{{Name: "Name"}, {Name: "Label"}}},
				},
				Associations: []*domainmodel.Association{{Name: "Order_Customer", ParentID: "e-order", ChildID: "e-customer",
					Type: domainmodel.AssociationTypeReference}},
			}}, nil
		},
	}
	pb := &pageBuilder{backend: b, entityContext: "Sales.Invoice", paramEntityNames: map[string]string{}, widgetScope: map[string]model.ID{}}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

func TestBuildPluggableAttributePath_ThroughOuterDataView(t *testing.T) {
	e := associationEngine(t)
	e.pageBuilder.paramEntityNames["dvOuter"] = "Sales.Order"
	defer e.pageBuilder.registerDataViewVariable("dvOuter", &pages.DataViewSource{ParameterName: "Ctx"})()
	w := twoSourceWidget(map[string]any{"staticAttribute": &ast.WidgetExpressionV3{Text: "$dvOuter/Order_Customer/Name"}})
	widget, err := e.Build(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	val := widgetValueFor(widget.RawObject, "Sales.Customer.Name")
	if val == nil {
		t.Fatal("no WidgetValue binds Sales.Customer.Name — the binding was dropped")
	}
	ref, _ := dGet(val, "AttributeRef").(bson.D)
	entityRef, _ := dGet(ref, "EntityRef").(bson.D)
	steps, _ := dGet(entityRef, "Steps").(bson.A)
	var step bson.D
	if len(steps) == 2 {
		step, _ = steps[1].(bson.D)
	}
	if dGet(step, "Association") != "Sales.Order_Customer" {
		t.Errorf("EntityRef steps = %#v, want one hop over Sales.Order_Customer", dGet(ref, "EntityRef"))
	}
	sv, _ := dGet(val, "SourceVariable").(bson.D)
	if dGet(sv, "Widget") != "dvOuter" || dGet(sv, "PageParameter") != "Ctx" {
		t.Errorf("SourceVariable = %#v, want the data-view pair", dGet(val, "SourceVariable"))
	}
}

// As the parser hands it over: `stringAttribute: $dataView1/Span_Trace/SystemPrompt`
// parses as an association data source rooted at the data view, which the
// engine refused as "an attribute property … cannot hold the expression"
// (MDL-WIDGET42) on Span_TextGeneration_SlideIn.
func TestBuildPluggableAttributePath_ParsedForm(t *testing.T) {
	e := associationEngine(t)
	e.pageBuilder.paramEntityNames["dvOuter"] = "Sales.Order"
	defer e.pageBuilder.registerDataViewVariable("dvOuter", &pages.DataViewSource{ParameterName: "Ctx"})()
	w := twoSourceWidget(map[string]any{"staticAttribute": &ast.DataSourceV3{
		Type: "association", Reference: "Order_Customer/Name", ContextVariable: "dvOuter"}})
	widget, err := e.Build(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if widgetValueFor(widget.RawObject, "Sales.Customer.Name") == nil {
		t.Fatal("no WidgetValue binds Sales.Customer.Name")
	}
}
