// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// emptyConditionValue is how Studio Pro stores an enumeration attribute's
// "(empty)" choice; MDL spells it `empty`.
const emptyConditionValue = "(empty)"

// applyVisibleWhen writes `Visible: Attr in (v1, …)` — Studio Pro's "Visible:
// based on attribute value".
//
// Studio Pro stores ONE condition PER VALUE of the attribute, flagged visible or
// not: every enumeration value in declaration order plus "(empty)", or "true"
// then "false" for a boolean. Measured on the 12 attribute-based settings in a
// stock Administration v4.3.2 + Feedback v4.0.2 project at 11.13.0 (e.g.
// Administration.ScheduledEvents: Running true; Completed, Error, Stopped,
// (empty) false). MDL names only the values that show the widget, and the full
// list is filled from the domain model.
//
// Without this the setting had no MDL spelling, so describe → exec dropped it
// and the widget became ALWAYS visible — with mx check clean.
func (pb *pageBuilder) applyVisibleWhen(widget pages.Widget, w *ast.WidgetV3) error {
	vw, ok := w.Properties["VisibleWhen"].(*ast.VisibleWhenV3)
	if !ok || vw == nil {
		return nil
	}
	type baseWidgetGetter interface {
		GetBaseWidget() *pages.BaseWidget
	}
	bwg, ok := widget.(baseWidgetGetter)
	if !ok {
		return mdlerrors.NewValidationf("%s %s: `Visible: %s in (…)` is not supported on this widget", w.Type, w.Name, vw.Attribute)
	}
	cvs, err := pb.visibleWhenSettings(w, vw)
	if err != nil {
		return err
	}
	bwg.GetBaseWidget().ConditionalVisibility = cvs
	return nil
}

// visibleWhenSettings resolves `Visible: Attr in (…)` to the settings element,
// for applyVisibleWhen and for a tab page, which is no widget but carries the
// same setting (mendixlabs/mxcli#1465).
func (pb *pageBuilder) visibleWhenSettings(w *ast.WidgetV3, vw *ast.VisibleWhenV3) (*pages.ConditionalVisibilitySettings, error) {
	where := fmt.Sprintf("%s %s: Visible: %s in (…)", w.Type, w.Name, vw.Attribute)
	if strings.Contains(vw.Attribute, "/") {
		return nil, mdlerrors.NewValidationf("%s: association paths are not supported — name an attribute of the data container's own entity", where)
	}

	// The entity is the data container's, or — qualified, Module.Entity.Attr —
	// named outright. DESCRIBE writes the qualified form under a container whose
	// flow cannot be resolved, where there is no entity in scope at all.
	//
	// `$Param.Attr` reads a page or snippet parameter: Studio Pro stores the
	// parameter in the setting's SourceVariable, as on TestApp's
	// WorkflowCommons.Snip_UserTask_NameColumnWithIcon, where the image sits in
	// the snippet with no data container around it.
	entity, attrName := pb.entityContext, vw.Attribute
	var source *pages.WidgetVariable
	//
	// `$dataView1.Attr` reads an enclosing data view's object: the data-view pair
	// {Widget, PageParameter}, as on FactoryManagement's
	// ConversationalUI.Trace_Details (ako/mxcli#2107).
	if name, rest, ok := namedObjectBinding(vw.Attribute); ok {
		wv, paramEntity, isParam := pb.namedObjectVariable(name)
		if !isParam || strings.Contains(rest, ".") {
			return nil, mdlerrors.NewValidationf("%s: `$%s` is not a parameter of this %s or an enclosing data view — "+
				"`Visible: $Param.Attr in (…)` reads an attribute of a page or snippet parameter, `$dataView1.Attr` one of "+
				"a data view the widget is inside; for the nearest data container's object name the attribute bare",
				where, name, pb.documentKind())
		}
		entity, attrName, source = paramEntity, rest, &wv
	} else if parts := strings.Split(vw.Attribute, "."); len(parts) == 3 {
		entity, attrName = parts[0]+"."+parts[1], parts[2]
	} else if len(parts) != 1 {
		return nil, mdlerrors.NewValidationf("%s: name the attribute bare, as $Param.Attribute, or as Module.Entity.Attribute", where)
	}
	if entity == "" {
		return nil, mdlerrors.NewValidationf("%s: the attribute is read from the enclosing data container's object — place the widget inside a data container, or read it from a parameter ($Param.%s)", where, attrName)
	}

	declaring, ok := pb.declaringEntityFor(entity, attrName)
	if !ok {
		return nil, mdlerrors.NewValidationf("%s: %s has no attribute %s", where, entity, attrName)
	}
	attrQN := declaring + "." + attrName

	var all []string
	switch t := pb.findAttributeType(attrQN).(type) {
	case *domainmodel.BooleanAttributeType:
		all = []string{"true", "false"}
	case *domainmodel.EnumerationAttributeType:
		values, err := pb.enumerationValueNames(t.EnumerationRef)
		if err != nil {
			return nil, mdlerrors.NewValidationf("%s: %v", where, err)
		}
		all = append(values, emptyConditionValue)
	default:
		return nil, mdlerrors.NewValidationf("%s: %s is not a Boolean or enumeration attribute — use an expression instead: `Visible: $currentObject/…`", where, attrQN)
	}

	visible := map[string]bool{}
	for _, v := range vw.Values {
		stored := v
		if strings.EqualFold(v, "empty") {
			stored = emptyConditionValue
		}
		idx := indexFold(all, stored)
		if idx < 0 {
			return nil, mdlerrors.NewValidationf("%s: %q is not a value of %s (values: %s)", where, v, attrQN,
				strings.Join(mdlValueNames(all), ", "))
		}
		visible[all[idx]] = true
	}

	conds := make([]pages.ValueCondition, 0, len(all))
	for _, v := range all {
		conds = append(conds, pages.ValueCondition{Value: v, Visible: visible[v]})
	}
	return &pages.ConditionalVisibilitySettings{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$ConditionalVisibilitySettings",
		},
		Attribute:      attrQN,
		Conditions:     conds,
		SourceVariable: source,
	}, nil
}

// applyVisibleRoles writes `Visible: roles (Mod.A, …)` — Studio Pro's "Visible
// for module roles" — onto the widget's settings element, creating one when no
// expression or attribute condition did. Studio Pro keeps the roles beside the
// condition on the same element (TestApp WorkflowCommons), so they never
// replace it. Runs after applyConditionalSettings and applyVisibleWhen.
func applyVisibleRoles(widget pages.Widget, w *ast.WidgetV3) error {
	if len(widgetVisibleRoles(w)) == 0 {
		return nil
	}
	type baseWidgetGetter interface {
		GetBaseWidget() *pages.BaseWidget
	}
	bwg, ok := widget.(baseWidgetGetter)
	if !ok {
		return mdlerrors.NewValidationf("%s %s: `Visible: roles (…)` is not supported on this widget", w.Type, w.Name)
	}
	bw := bwg.GetBaseWidget()
	bw.ConditionalVisibility = withVisibleRoles(bw.ConditionalVisibility, w)
	return nil
}

// withVisibleRoles returns cvs carrying the statement's `Visible: roles (…)`,
// a fresh settings element when cvs is nil, and cvs unchanged when the
// statement names no roles.
func withVisibleRoles(cvs *pages.ConditionalVisibilitySettings, w *ast.WidgetV3) *pages.ConditionalVisibilitySettings {
	roles := widgetVisibleRoles(w)
	if len(roles) == 0 {
		return cvs
	}
	if cvs == nil {
		cvs = &pages.ConditionalVisibilitySettings{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ConditionalVisibilitySettings",
			},
		}
	}
	cvs.ModuleRoles = roles
	return cvs
}

// widgetVisibleRoles is the statement's `Visible: roles (…)` list, if any.
func widgetVisibleRoles(w *ast.WidgetV3) []string {
	if w == nil {
		return nil
	}
	roles, _ := w.Properties["VisibleRoles"].([]string)
	return roles
}

func indexFold(values []string, v string) int {
	for i, x := range values {
		if strings.EqualFold(x, v) {
			return i
		}
	}
	return -1
}

// mdlValueNames lists condition values as MDL spells them.
func mdlValueNames(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		if v == emptyConditionValue {
			v = "empty"
		}
		out[i] = v
	}
	return out
}

// enumerationValueNames returns an enumeration's value names in declaration
// order.
func (pb *pageBuilder) enumerationValueNames(enumQN string) ([]string, error) {
	enums, err := pb.getEnumerations()
	if err != nil {
		return nil, err
	}
	h, err := pb.getHierarchy()
	if err != nil {
		return nil, err
	}
	for _, e := range enums {
		if h.GetModuleName(h.FindModuleID(e.ContainerID))+"."+e.Name != enumQN {
			continue
		}
		names := make([]string, 0, len(e.Values))
		for _, v := range e.Values {
			names = append(names, v.Name)
		}
		return names, nil
	}
	return nil, fmt.Errorf("enumeration %s not found", enumQN)
}

// getEnumerations returns cached enumerations or loads them.
func (pb *pageBuilder) getEnumerations() ([]*model.Enumeration, error) {
	if pb.execCache != nil && pb.execCache.enumerations != nil {
		return pb.execCache.enumerations, nil
	}
	if pb.backend == nil {
		return nil, fmt.Errorf("no project loaded")
	}
	enums, err := pb.backend.ListEnumerations()
	if err != nil {
		return nil, err
	}
	if pb.execCache != nil {
		pb.execCache.enumerations = enums
	}
	return enums, nil
}
