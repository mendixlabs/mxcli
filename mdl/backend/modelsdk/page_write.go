// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDT "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatypes"
	genPg "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/pages"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func init() {
	// A page always emits its Parameters and Variables arrays (empty = marker 3).
	codec.RegisterTypeDefaults("Forms$Page", codec.TypeDefaults{
		MandatoryLists: []string{"Parameters", "Variables"},
	})
	// FormCall arguments use the typed-array marker 2 when populated (the codec
	// emits 3 for empty lists automatically).
	codec.RegisterListMarker("Forms$FormCallArgument", 2)
}

// CreatePage inserts a new Forms$Page document unit (header, layout call, the
// widget tree, parameters, and variables) via pageToGen.
func (b *Backend) CreatePage(page *pages.Page) error {
	if page == nil {
		return fmt.Errorf("CreatePage: nil page")
	}
	if b.writer == nil {
		return fmt.Errorf("CreatePage: not connected for writing")
	}
	if page.ID == "" {
		page.ID = model.ID(mmpr.GenerateID())
	}
	g, err := pageToGen(page)
	if err != nil {
		return err
	}
	g.SetID(element.ID(page.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreatePage: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(page.ID), string(page.ContainerID), "Documents", "Forms$Page", contents); err != nil {
		return fmt.Errorf("CreatePage: insert: %w", err)
	}
	return nil
}

// UpdatePage rewrites an existing Forms$Page unit in place, preserving its UUID.
// Used by CREATE OR REPLACE PAGE and the styling alterations, which rebuild the
// full page (header + widget tree) and replace the existing unit. Serialization
// is identical to CreatePage.
func (b *Backend) UpdatePage(page *pages.Page) error {
	if page == nil {
		return fmt.Errorf("UpdatePage: nil page")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdatePage: not connected for writing")
	}
	g, err := pageToGen(page)
	if err != nil {
		return err
	}
	g.SetID(element.ID(page.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdatePage: encode: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(page.ID), contents); err != nil {
		return fmt.Errorf("UpdatePage: update: %w", err)
	}
	return nil
}

// DeletePage removes a page unit by ID.
func (b *Backend) DeletePage(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeletePage: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// popupDimension returns the pop-up width/height for the gen Page (int32).
// Studio Pro's own default is 0 (auto-size), so 0 is a valid value and is
// written through verbatim (issue #713); only a stray negative is clamped to 0.
func popupDimension(n int) int32 {
	if n < 0 {
		return 0
	}
	return int32(n)
}

// pageToGen builds the full gen Page: header, layout call, the widget tree (under
// the layout call's form-call arguments), parameters, and variables.
func pageToGen(page *pages.Page) (*genPg.Page, error) {
	out := genPg.NewPage()
	out.SetName(page.Name)
	out.SetDocumentation(page.Documentation)
	out.SetExcluded(page.Excluded)
	out.SetExportLevel("Hidden")
	out.SetAutofocus("DesktopOnly")
	out.SetCanvasWidth(1200)
	out.SetCanvasHeight(600)
	out.SetMarkAsUsed(page.MarkAsUsed)
	out.SetUrl(page.URL)
	out.SetPopupCloseAction("")
	out.SetPopupWidth(popupDimension(page.PopupWidth))
	out.SetPopupHeight(popupDimension(page.PopupHeight))
	out.SetPopupResizable(page.PopupResizable)
	out.SetAllowedRolesQualifiedNames(moduleRoleNames(page.AllowedRoles))
	out.SetTitle(captionToGen(page.Title))
	out.SetAppearance(newAppearance(page.Class, page.Style, "", nil))

	if page.LayoutCall != nil {
		lc, err := layoutCallToGen(page.LayoutCall)
		if err != nil {
			return nil, err
		}
		out.SetLayoutCall(lc)
	}

	for _, p := range page.Parameters {
		out.AddParameters(pageParameterToGen(p))
	}
	for _, v := range page.Variables {
		out.AddVariables(localVariableToGen(v))
	}
	return out, nil
}

// localVariableToGen builds a Forms$LocalVariable (a page-level variable: name,
// default-value expression, and data type). Without this, a column header or widget
// bound to $VarName dangles → CE1151 "Missing variable".
func localVariableToGen(v *pages.LocalVariable) element.Element {
	lv := genPg.NewLocalVariable()
	if v.ID != "" {
		lv.SetID(element.ID(v.ID))
	}
	assignID(lv)
	lv.SetName(v.Name)
	lv.SetDefaultValue(v.DefaultValue)
	lv.SetVariableType(localVarTypeToGen(v.VariableType))
	return lv
}

// localVarTypeToGen maps a LocalVariable's BSON $Type ("DataTypes$StringType", …)
// to the gen data-type element. Defaults to String.
func localVarTypeToGen(typeName string) element.Element {
	var t element.Element
	switch typeName {
	case "DataTypes$IntegerType":
		t = genDT.NewIntegerType()
	case "DataTypes$BooleanType":
		t = genDT.NewBooleanType()
	case "DataTypes$DecimalType":
		t = genDT.NewDecimalType()
	case "DataTypes$DateTimeType":
		t = genDT.NewDateTimeType()
	default:
		t = genDT.NewStringType()
	}
	assignID(t)
	return t
}

// layoutCallToGen builds the Forms$LayoutCall (page → layout binding) with one
// Forms$FormCallArgument per placeholder, including each placeholder's widget.
func layoutCallToGen(lc *pages.LayoutCall) (*genPg.LayoutCall, error) {
	out := genPg.NewLayoutCall()
	assignID(out)
	out.SetLayoutQualifiedName(lc.LayoutName)
	for _, arg := range lc.Arguments {
		ga := genPg.NewLayoutCallArgument()
		// The gen mislabels this type; real BSON is Forms$FormCallArgument.
		ga.SetTypeName("Forms$FormCallArgument")
		assignID(ga)
		ga.SetParameterQualifiedName(string(arg.ParameterID))
		if arg.Widget != nil {
			wg, err := widgetToGen(arg.Widget)
			if err != nil {
				return nil, err
			}
			ga.AddWidgets(wg)
		}
		out.AddArguments(ga)
	}
	return out, nil
}

// pageParameterToGen converts a page parameter, including its ParameterType — an
// entity (DataTypes$ObjectType) or a primitive (DataTypes$StringType, …). Without
// the type the parameter can't resolve (CE5601/CE5606).
func pageParameterToGen(p *pages.PageParameter) *genPg.PageParameter {
	gp := genPg.NewPageParameter()
	if p.ID != "" {
		gp.SetID(element.ID(p.ID))
	}
	assignID(gp)
	gp.SetName(p.Name)
	gp.SetIsRequired(p.IsRequired)
	gp.SetDefaultValue(p.DefaultValue)
	gp.SetParameterType(pageParamTypeToGen(p))
	return gp
}

// pageParamTypeToGen builds a page parameter's type: a DataTypes$ObjectType for an
// entity parameter, or the named primitive DataTypes type. p.TypeName carries the
// primitive's BSON $Type (e.g. "DataTypes$StringType") when set.
func pageParamTypeToGen(p *pages.PageParameter) element.Element {
	if p.TypeName == "" {
		t := genDT.NewObjectType()
		assignID(t)
		t.SetEntityQualifiedName(p.EntityName)
		return t
	}
	var t element.Element
	switch p.TypeName {
	case "DataTypes$IntegerType":
		t = genDT.NewIntegerType()
	case "DataTypes$BooleanType":
		t = genDT.NewBooleanType()
	case "DataTypes$DecimalType":
		t = genDT.NewDecimalType()
	case "DataTypes$DateTimeType":
		t = genDT.NewDateTimeType()
	default:
		t = genDT.NewStringType()
	}
	assignID(t)
	return t
}
