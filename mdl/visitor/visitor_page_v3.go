// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// parseQualifiedName converts a string like "Module.Name" to ast.QualifiedName.
func parseQualifiedName(text string) ast.QualifiedName {
	parts := strings.Split(text, ".")
	if len(parts) == 1 {
		return ast.QualifiedName{Name: parts[0]}
	}
	return ast.QualifiedName{
		Module: parts[0],
		Name:   parts[len(parts)-1],
	}
}

// ============================================================================
// Page V3 Visitor Functions
// ============================================================================
//
// These functions handle the V3 page syntax with explicit properties.
// Pattern: WIDGET name (Prop: Value) { children }
//

// buildPageV3 builds a V3 page statement from the parse context.
func (b *Builder) buildPageV3(ctx *parser.CreatePageStatementContext) *ast.CreatePageStmtV3 {
	stmt := &ast.CreatePageStmtV3{}

	// Get page name
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Name = buildQualifiedName(qn)
	}

	// Check for CREATE OR REPLACE/MODIFY and parse @excluded
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil {
			if createStmt.REPLACE() != nil {
				stmt.IsReplace = true
			}
			if createStmt.MODIFY() != nil {
				stmt.IsModify = true
			}
		}
		stmt.Documentation = findDocCommentText(ctx)
		for _, ann := range createStmt.AllAnnotation() {
			annCtx := ann.(*parser.AnnotationContext)
			if strings.EqualFold(annCtx.AnnotationName().GetText(), "excluded") {
				stmt.Excluded = true
			}
		}
	}

	// Parse V3 header
	if headerCtx := ctx.PageHeaderV3(); headerCtx != nil {
		b.parsePageHeaderV3(headerCtx, stmt)
	}

	// Parse V3 body
	if bodyCtx := ctx.PageBodyV3(); bodyCtx != nil {
		stmt.Widgets = buildPageBodyV3(bodyCtx, b)
	}

	return stmt
}

// parsePageHeaderV3 extracts properties from the V3 page header.
func (b *Builder) parsePageHeaderV3(ctx parser.IPageHeaderV3Context, stmt *ast.CreatePageStmtV3) {
	if ctx == nil {
		return
	}
	headerCtx := ctx.(*parser.PageHeaderV3Context)

	for _, propCtx := range headerCtx.AllPageHeaderPropertyV3() {
		prop := propCtx.(*parser.PageHeaderPropertyV3Context)

		if prop.PARAMS() != nil {
			// Params: { $Order: Entity, ... }
			if paramList := prop.PageParameterList(); paramList != nil {
				stmt.Parameters = buildPageParameters(paramList)
			}
		} else if prop.VARIABLES_KW() != nil {
			// Variables: { $showStock: Boolean = 'true', ... }
			if varList := prop.VariableDeclarationList(); varList != nil {
				stmt.Variables = buildVariableDeclarations(varList)
			}
		} else if prop.TITLE() != nil {
			// Title: 'My Page'
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.Title = unquoteString(str.GetText())
			}
		} else if prop.LAYOUT() != nil {
			// Layout: Atlas_Core.Atlas_Default or 'Layout Name'
			if qn := prop.QualifiedName(); qn != nil {
				stmt.Layout = getQualifiedNameText(qn)
			} else if str := prop.STRING_LITERAL(); str != nil {
				stmt.Layout = unquoteString(str.GetText())
			}
		} else if prop.URL() != nil {
			// Url: 'my-page'
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.URL = unquoteString(str.GetText())
			}
		} else if prop.FOLDER() != nil {
			// Folder: 'Pages/Admin'
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.Folder = unquoteString(str.GetText())
			}
		} else if prop.CLASS() != nil {
			// Class: 'my-page' — page-level CSS class (issue #714)
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.Class = unquoteString(str.GetText())
			}
		} else if prop.STYLE() != nil {
			// Style: 'padding: 10px' — page-level inline CSS (issue #714)
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.Style = unquoteString(str.GetText())
			}
		} else if id := prop.IDENTIFIER(); id != nil {
			// Generic page header property (e.g. PopupWidth: 800). Only the
			// pop-up dimensions are recognized; anything else is a typo/unsupported.
			b.applyGenericPageHeaderProp(stmt, id.GetText(), buildPropertyValueV3(prop.PropertyValueV3()), id.GetSymbol())
		}
	}
}

// applyGenericPageHeaderProp maps a generic `Name: value` page-header property
// onto the AST. Today only the pop-up dimensions are supported; an unrecognized
// name is reported as an error rather than silently dropped (issue #661).
func (b *Builder) applyGenericPageHeaderProp(stmt *ast.CreatePageStmtV3, name string, val any, tok antlr.Token) {
	switch name {
	case "PopupWidth", "PopupHeight":
		n, ok := popupDimensionValue(val)
		if !ok {
			b.addError(fmt.Errorf("line %d:%d: %s must be a whole number of pixels >= 0 (0 = auto-size), got %v",
				tok.GetLine(), tok.GetColumn(), name, val))
			return
		}
		if name == "PopupWidth" {
			stmt.PopupWidth = &n
		} else {
			stmt.PopupHeight = &n
		}
	case "PopupResizable":
		bval, ok := val.(bool)
		if !ok {
			b.addError(fmt.Errorf("line %d:%d: PopupResizable must be true or false, got %v",
				tok.GetLine(), tok.GetColumn(), val))
			return
		}
		stmt.PopupResizable = &bval
	default:
		b.addError(fmt.Errorf("line %d:%d: unknown page property %q "+
			"(supported: Title, Layout, Url, Folder, Params, Variables, PopupWidth, PopupHeight, PopupResizable, Class, Style)",
			tok.GetLine(), tok.GetColumn(), name))
	}
}

// popupDimensionValue accepts a non-negative whole number within int32 range from
// the generic property value (int literal → int, decimal literal → float64). 0 is
// valid — it is Studio Pro's default and means auto-size (issue #713).
func popupDimensionValue(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		if n >= 0 && n <= math.MaxInt32 {
			return n, true
		}
	case float64:
		if n == math.Trunc(n) && n >= 0 && n <= math.MaxInt32 {
			return int(n), true
		}
	}
	return 0, false
}

// buildSnippetV3 builds a V3 snippet statement from the parse context.
func (b *Builder) buildSnippetV3(ctx *parser.CreateSnippetStatementContext) *ast.CreateSnippetStmtV3 {
	stmt := &ast.CreateSnippetStmtV3{}

	// Get snippet name
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Name = buildQualifiedName(qn)
	}

	// Check for CREATE OR REPLACE/MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil {
			if createStmt.REPLACE() != nil {
				stmt.IsReplace = true
			}
			if createStmt.MODIFY() != nil {
				stmt.IsModify = true
			}
		}
		stmt.Documentation = findDocCommentText(ctx)
	}

	// Parse V3 header
	if headerCtx := ctx.SnippetHeaderV3(); headerCtx != nil {
		b.parseSnippetHeaderV3(headerCtx, stmt)
	}

	// Parse options (FOLDER)
	if opts := ctx.SnippetOptions(); opts != nil {
		optsCtx := opts.(*parser.SnippetOptionsContext)
		for _, opt := range optsCtx.AllSnippetOption() {
			optCtx := opt.(*parser.SnippetOptionContext)
			if optCtx.FOLDER() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Folder = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
		}
	}

	// Parse V3 body
	if bodyCtx := ctx.PageBodyV3(); bodyCtx != nil {
		stmt.Widgets = buildPageBodyV3(bodyCtx, b)
	}

	return stmt
}

// parseSnippetHeaderV3 extracts properties from the V3 snippet header.
func (b *Builder) parseSnippetHeaderV3(ctx parser.ISnippetHeaderV3Context, stmt *ast.CreateSnippetStmtV3) {
	if ctx == nil {
		return
	}
	headerCtx := ctx.(*parser.SnippetHeaderV3Context)

	for _, propCtx := range headerCtx.AllSnippetHeaderPropertyV3() {
		prop := propCtx.(*parser.SnippetHeaderPropertyV3Context)

		if prop.PARAMS() != nil {
			// Params: { $Customer: Entity, ... }
			if paramList := prop.SnippetParameterList(); paramList != nil {
				stmt.Parameters = buildSnippetParameterListAsPage(paramList)
			}
		} else if prop.VARIABLES_KW() != nil {
			// Variables: { $showStock: Boolean = 'true', ... }
			if varList := prop.VariableDeclarationList(); varList != nil {
				stmt.Variables = buildVariableDeclarations(varList)
			}
		} else if prop.FOLDER() != nil {
			// Folder: 'Snippets/Common'
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.Folder = unquoteString(str.GetText())
			}
		}
	}
}

// buildSnippetParameterListAsPage converts snippet parameters to page parameters.
func buildSnippetParameterListAsPage(ctx parser.ISnippetParameterListContext) []ast.PageParameter {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.SnippetParameterListContext)
	var params []ast.PageParameter

	for _, sp := range listCtx.AllSnippetParameter() {
		spCtx := sp.(*parser.SnippetParameterContext)
		param := ast.PageParameter{}

		if id := spCtx.IDENTIFIER(); id != nil {
			param.Name = id.GetText()
		} else if v := spCtx.VARIABLE(); v != nil {
			// VARIABLE token is $name, strip the $ prefix
			param.Name = strings.TrimPrefix(v.GetText(), "$")
		} else if qid := spCtx.QUOTED_IDENTIFIER(); qid != nil {
			// Quoted name for reserved-keyword params, e.g. "List". See issue #114.
			param.Name = unquoteIdentifier(qid.GetText())
		}

		if dt := spCtx.DataType(); dt != nil {
			param.EntityType = parseQualifiedName(dt.GetText())
		}

		params = append(params, param)
	}

	return params
}

// buildVariableDeclarations builds variable declarations from the parse context.
func buildVariableDeclarations(ctx parser.IVariableDeclarationListContext) []ast.PageVariable {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.VariableDeclarationListContext)
	var vars []ast.PageVariable

	for _, vd := range listCtx.AllVariableDeclaration() {
		vars = append(vars, buildSingleVariableDeclaration(vd.(*parser.VariableDeclarationContext)))
	}

	return vars
}

// buildSingleVariableDeclaration builds a single PageVariable from a parse context.
func buildSingleVariableDeclaration(vdCtx *parser.VariableDeclarationContext) ast.PageVariable {
	v := ast.PageVariable{}

	if varTok := vdCtx.VARIABLE(); varTok != nil {
		v.Name = strings.TrimPrefix(varTok.GetText(), "$")
	}

	if dt := vdCtx.DataType(); dt != nil {
		v.DataType = dt.GetText()
	}

	if str := vdCtx.STRING_LITERAL(); str != nil {
		v.DefaultValue = unquoteString(str.GetText())
	}

	return v
}

// buildPageBodyV3 extracts widgets from a V3 page body.
// Handles both widgetV3 and useFragmentRef children in parse-tree order.
func buildPageBodyV3(ctx parser.IPageBodyV3Context, b *Builder) []*ast.WidgetV3 {
	if ctx == nil {
		return nil
	}
	bodyCtx := ctx.(*parser.PageBodyV3Context)
	var widgets []*ast.WidgetV3

	// Process children in parse-tree order (widgets and fragment refs interleaved)
	for _, child := range bodyCtx.GetChildren() {
		switch c := child.(type) {
		case *parser.WidgetV3Context:
			if widget := buildWidgetV3(c, b); widget != nil {
				widgets = append(widgets, widget)
			}
		case *parser.UseFragmentRefContext:
			if ref := buildUseFragmentRef(c); ref != nil {
				widgets = append(widgets, ref)
			}
		}
	}

	return widgets
}

// buildUseFragmentRef creates a WidgetV3 with sentinel type USE_FRAGMENT.
func buildUseFragmentRef(ctx *parser.UseFragmentRefContext) *ast.WidgetV3 {
	if ctx == nil {
		return nil
	}
	w := &ast.WidgetV3{
		Type:       "USE_FRAGMENT",
		Properties: make(map[string]interface{}),
	}
	ids := ctx.AllIdentifierOrKeyword()
	if len(ids) > 0 {
		w.Name = identifierOrKeywordText(ids[0]) // Fragment name
	}
	if len(ids) > 1 {
		w.Properties["Prefix"] = identifierOrKeywordText(ids[1]) // Optional prefix
	}
	return w
}

// buildWidgetV3 builds a V3 widget from a widgetV3 context.
func buildWidgetV3(ctx parser.IWidgetV3Context, b *Builder) *ast.WidgetV3 {
	if ctx == nil {
		return nil
	}
	wCtx := ctx.(*parser.WidgetV3Context)

	widget := &ast.WidgetV3{
		Properties: make(map[string]any),
		Children:   []*ast.WidgetV3{},
	}

	// Get widget type
	if wCtx.PLUGGABLEWIDGET() != nil {
		widget.Type = "pluggablewidget"
		widget.Properties["WidgetType"] = unquoteString(wCtx.STRING_LITERAL().GetText())
	} else if wCtx.CUSTOMWIDGET() != nil {
		widget.Type = "customwidget"
		widget.Properties["WidgetType"] = unquoteString(wCtx.STRING_LITERAL().GetText())
	} else if typeCtx := wCtx.WidgetTypeV3(); typeCtx != nil {
		widget.Type = strings.ToLower(typeCtx.GetText())
	}

	// Get required identifier. The name may be quoted (QUOTED_IDENTIFIER) when it
	// collides with a reserved keyword, e.g. a widget named "List". See issue #619.
	if id := wCtx.IDENTIFIER(); id != nil {
		widget.Name = id.GetText()
	} else if qid := wCtx.QUOTED_IDENTIFIER(); qid != nil {
		widget.Name = unquoteIdentifier(qid.GetText())
	}

	// Parse properties
	if propsCtx := wCtx.WidgetPropertiesV3(); propsCtx != nil {
		parseWidgetPropertiesV3(propsCtx, widget, b)
	}

	// Parse children
	if bodyCtx := wCtx.WidgetBodyV3(); bodyCtx != nil {
		widget.Children = buildWidgetBodyV3(bodyCtx, b)
	}

	return widget
}

// parseWidgetPropertiesV3 extracts properties from the widget properties context.
func parseWidgetPropertiesV3(ctx parser.IWidgetPropertiesV3Context, widget *ast.WidgetV3, b *Builder) {
	if ctx == nil {
		return
	}
	propsCtx := ctx.(*parser.WidgetPropertiesV3Context)

	for _, propCtx := range propsCtx.AllWidgetPropertyV3() {
		parseWidgetPropertyV3(propCtx, widget, b)
	}
}

// parseWidgetPropertyV3 extracts a single property.
func parseWidgetPropertyV3(ctx parser.IWidgetPropertyV3Context, widget *ast.WidgetV3, b *Builder) {
	if ctx == nil {
		return
	}
	propCtx := ctx.(*parser.WidgetPropertyV3Context)

	// DataSource: ...
	if propCtx.DATASOURCE() != nil {
		if dsCtx := propCtx.DataSourceExprV3(); dsCtx != nil {
			widget.Properties["DataSource"] = buildDataSourceV3(dsCtx)
		}
		return
	}

	// Attribute: ... (unified property for attribute bindings)
	if propCtx.ATTRIBUTE() != nil {
		if pathCtx := propCtx.AttributePathV3(); pathCtx != nil {
			widget.Properties["Attribute"] = buildAttributePathV3(pathCtx)
		}
		return
	}

	// Binds: ... (deprecated — hard error)
	if propCtx.BINDS() != nil {
		tok := propCtx.BINDS().GetSymbol()
		b.addError(fmt.Errorf("line %d:%d: 'Binds:' is no longer supported, use 'Attribute:' instead", tok.GetLine(), tok.GetColumn()))
		return
	}

	// Action: ... (also accepts the OnClick: alias — e.g. clickable CONTAINER, issue #603)
	if propCtx.ACTION() != nil || propCtx.ONCLICK() != nil {
		if actCtx := propCtx.ActionExprV3(); actCtx != nil {
			widget.Properties["Action"] = buildActionV3(actCtx)
		}
		return
	}

	// Caption: ...
	if propCtx.CAPTION() != nil {
		if strCtx := propCtx.StringExprV3(); strCtx != nil {
			widget.Properties["Caption"] = buildStringExprV3(strCtx)
		}
		return
	}

	// Label: ...
	if propCtx.LABEL() != nil {
		if str := propCtx.STRING_LITERAL(); str != nil {
			widget.Properties["Label"] = unquoteString(str.GetText())
		}
		return
	}

	// Attr: ... (deprecated — hard error)
	if propCtx.ATTR() != nil {
		tok := propCtx.ATTR().GetSymbol()
		b.addError(fmt.Errorf("line %d:%d: 'Attr:' is no longer supported, use 'Attribute:' instead", tok.GetLine(), tok.GetColumn()))
		return
	}

	// Content: ...
	if propCtx.CONTENT() != nil {
		if strCtx := propCtx.StringExprV3(); strCtx != nil {
			widget.Properties["Content"] = buildStringExprV3(strCtx)
		}
		return
	}

	// RenderMode: ...
	if propCtx.RENDERMODE() != nil {
		if rmCtx := propCtx.RenderModeV3(); rmCtx != nil {
			widget.Properties["RenderMode"] = rmCtx.GetText()
		}
		return
	}

	// ContentParams: [...]
	if propCtx.CONTENTPARAMS() != nil {
		if plCtx := propCtx.ParamListV3(); plCtx != nil {
			widget.Properties["ContentParams"] = buildParamListV3(plCtx)
		}
		return
	}

	// CaptionParams: [...]
	if propCtx.CAPTIONPARAMS() != nil {
		if plCtx := propCtx.ParamListV3(); plCtx != nil {
			widget.Properties["CaptionParams"] = buildParamListV3(plCtx)
		}
		return
	}

	// ButtonStyle: ...
	if propCtx.BUTTONSTYLE() != nil {
		if styleCtx := propCtx.ButtonStyleV3(); styleCtx != nil {
			widget.Properties["ButtonStyle"] = styleCtx.GetText()
		}
		return
	}

	// Class: ...
	if propCtx.CLASS() != nil {
		if str := propCtx.STRING_LITERAL(); str != nil {
			widget.Properties["Class"] = unquoteString(str.GetText())
		}
		return
	}

	// Style: ...
	if propCtx.STYLE() != nil {
		if str := propCtx.STRING_LITERAL(); str != nil {
			widget.Properties["Style"] = unquoteString(str.GetText())
		}
		return
	}

	// DesktopWidth: ...
	if propCtx.DESKTOPWIDTH() != nil {
		if dwCtx := propCtx.DesktopWidthV3(); dwCtx != nil {
			widget.Properties["DesktopWidth"] = parseWidthValue(dwCtx.GetText())
		}
		return
	}

	// TabletWidth: ...
	if propCtx.TABLETWIDTH() != nil {
		if dwCtx := propCtx.DesktopWidthV3(); dwCtx != nil {
			widget.Properties["TabletWidth"] = parseWidthValue(dwCtx.GetText())
		}
		return
	}

	// PhoneWidth: ...
	if propCtx.PHONEWIDTH() != nil {
		if dwCtx := propCtx.DesktopWidthV3(); dwCtx != nil {
			widget.Properties["PhoneWidth"] = parseWidthValue(dwCtx.GetText())
		}
		return
	}

	// Where: and OrderBy: removed — now handled inline in dataSourceExprV3

	// Selection: ...
	if propCtx.SELECTION() != nil {
		if smCtx := propCtx.SelectionModeV3(); smCtx != nil {
			widget.Properties["Selection"] = smCtx.GetText()
		}
		return
	}

	// Snippet: ...
	if propCtx.SNIPPET() != nil {
		if qn := propCtx.QualifiedName(); qn != nil {
			widget.Properties["Snippet"] = getQualifiedNameText(qn)
		}
		return
	}

	// Params: {$Asset: $var} — snippet call parameter mappings
	if propCtx.PARAMS() != nil {
		if plCtx := propCtx.SnippetCallParamListV3(); plCtx != nil {
			widget.Properties["Params"] = buildSnippetCallParamListV3(plCtx)
		}
		return
	}

	// Attributes: [...] (for filter widgets)
	if propCtx.ATTRIBUTES() != nil {
		if attrListCtx := propCtx.AttributeListV3(); attrListCtx != nil {
			widget.Properties["Attributes"] = buildAttributeListV3(attrListCtx)
		}
		return
	}

	// FilterType: ... (for filter widgets)
	if propCtx.FILTERTYPE() != nil {
		if ftCtx := propCtx.FilterTypeValue(); ftCtx != nil {
			widget.Properties["FilterType"] = ftCtx.GetText()
		}
		return
	}

	// Width: number
	if propCtx.WIDTH() != nil {
		if num := propCtx.NUMBER_LITERAL(); num != nil {
			if n, err := strconv.Atoi(num.GetText()); err == nil {
				widget.Properties["Width"] = n
			}
		}
		return
	}

	// Height: number
	if propCtx.HEIGHT() != nil {
		if num := propCtx.NUMBER_LITERAL(); num != nil {
			if n, err := strconv.Atoi(num.GetText()); err == nil {
				widget.Properties["Height"] = n
			}
		}
		return
	}

	// DesignProperties: [...]
	if propCtx.DESIGNPROPERTIES() != nil {
		if dpCtx := propCtx.DesignPropertyListV3(); dpCtx != nil {
			widget.Properties["DesignProperties"] = buildDesignPropertyListV3(dpCtx)
		}
		return
	}

	// Visible: [expression] (conditional visibility) or Visible: false (static)
	if propCtx.VISIBLE() != nil {
		if xc := propCtx.XpathConstraint(); xc != nil {
			widget.Properties["VisibleIf"] = buildConditionalExpression(xc)
		} else if valCtx := propCtx.PropertyValueV3(); valCtx != nil {
			widget.Properties["Visible"] = buildPropertyValueV3(valCtx)
		}
		return
	}

	// Editable: [expression] (conditional editability) or Editable: Never (static)
	if propCtx.EDITABLE() != nil {
		if xc := propCtx.XpathConstraint(); xc != nil {
			widget.Properties["EditableIf"] = buildConditionalExpression(xc)
		} else if valCtx := propCtx.PropertyValueV3(); valCtx != nil {
			widget.Properties["Editable"] = buildPropertyValueV3(valCtx)
		}
		return
	}

	// Tooltip: 'text' (keyword-based property)
	if propCtx.TOOLTIP() != nil {
		if valCtx := propCtx.PropertyValueV3(); valCtx != nil {
			widget.Properties["Tooltip"] = buildPropertyValueV3(valCtx)
		}
		return
	}

	// Generic property: Identifier: value
	if id := propCtx.IDENTIFIER(); id != nil {
		// Generic datasource-typed property (e.g. chart series `staticDataSource:
		// database Module.View`). The executor's object-list builder resolves the
		// *ast.DataSourceV3 into a widget datasource + entity context. Chart 9a.
		if dsCtx := propCtx.DataSourceExprV3(); dsCtx != nil {
			widget.Properties[id.GetText()] = buildDataSourceV3(dsCtx)
			return
		}
		if valCtx := propCtx.PropertyValueV3(); valCtx != nil {
			widget.Properties[id.GetText()] = buildPropertyValueV3(valCtx)
		}
		return
	}

	// Generic property with keyword name: keyword: value (for pluggable widget property keys
	// that happen to be MDL keywords, e.g., type, datasource, content)
	if kw := propCtx.Keyword(); kw != nil {
		if dsCtx := propCtx.DataSourceExprV3(); dsCtx != nil {
			widget.Properties[kw.GetText()] = buildDataSourceV3(dsCtx)
			return
		}
		if valCtx := propCtx.PropertyValueV3(); valCtx != nil {
			widget.Properties[kw.GetText()] = buildPropertyValueV3(valCtx)
		}
		return
	}
}

// buildDataSourceV3 builds a DataSource from the parse context.
func buildDataSourceV3(ctx parser.IDataSourceExprV3Context) *ast.DataSourceV3 {
	if ctx == nil {
		return nil
	}
	dsCtx := ctx.(*parser.DataSourceExprV3Context)
	ds := &ast.DataSourceV3{}

	if v := dsCtx.VARIABLE(); v != nil && dsCtx.SLASH() != nil {
		// $currentObject/Module.Assoc — ByAssociation data source (sugar for ASSOCIATION Path)
		ds.Type = "association"
		ds.ContextVariable = strings.TrimPrefix(v.GetText(), "$")
		if pathCtx := dsCtx.AssociationPathV3(); pathCtx != nil {
			ds.Reference = buildAssociationPathV3(pathCtx)
		}
	} else if v := dsCtx.VARIABLE(); v != nil {
		// $ParamName
		ds.Type = "parameter"
		ds.Reference = v.GetText()
	} else if dsCtx.DATABASE() != nil {
		// DATABASE [FROM] Entity [WHERE ...] [SORT BY ...]
		ds.Type = "database"
		if qn := dsCtx.QualifiedName(); qn != nil {
			ds.Reference = getQualifiedNameText(qn)
		}

		// Inline WHERE clause
		if dsCtx.WHERE() != nil {
			xpathConstraints := dsCtx.AllXpathConstraint()
			if len(xpathConstraints) > 0 {
				ds.Where = normalizeXPathTokens(buildXPathString(xpathConstraints, dsCtx.AllAndOrXpath()))
			} else if expr := dsCtx.Expression(); expr != nil {
				ds.Where = bracketedXPathFromExpr(buildExpression(expr))
			}
		}

		// Inline SORT BY clause
		if dsCtx.SORT_BY() != nil {
			for _, sc := range dsCtx.AllSortColumn() {
				ds.OrderBy = append(ds.OrderBy, buildSortColumnAsOrderBy(sc))
			}
		}
	} else if dsCtx.MICROFLOW() != nil {
		// MICROFLOW Module.Flow
		ds.Type = "microflow"
		if qn := dsCtx.QualifiedName(); qn != nil {
			ds.Reference = getQualifiedNameText(qn)
		}
		if argsCtx := dsCtx.MicroflowArgsV3(); argsCtx != nil {
			ds.Args = buildMicroflowArgsV3(argsCtx)
		}
	} else if dsCtx.NANOFLOW() != nil {
		// NANOFLOW Module.Flow
		ds.Type = "nanoflow"
		if qn := dsCtx.QualifiedName(); qn != nil {
			ds.Reference = getQualifiedNameText(qn)
		}
		if argsCtx := dsCtx.MicroflowArgsV3(); argsCtx != nil {
			ds.Args = buildMicroflowArgsV3(argsCtx)
		}
	} else if dsCtx.ASSOCIATION() != nil {
		// ASSOCIATION Path
		ds.Type = "association"
		if pathCtx := dsCtx.AssociationPathV3(); pathCtx != nil {
			ds.Reference = buildAssociationPathV3(pathCtx)
		}
	} else if dsCtx.SELECTION() != nil {
		// SELECTION widgetName
		ds.Type = "selection"
		if id := dsCtx.IDENTIFIER(); id != nil {
			ds.Reference = id.GetText()
		} else if qid := dsCtx.QUOTED_IDENTIFIER(); qid != nil {
			// SELECTION "widgetName" — reserved-word widget name
			ds.Reference = unquoteIdentifier(qid.GetText())
		}
	}

	return ds
}

// buildActionV3 builds an Action from the parse context.
func buildActionV3(ctx parser.IActionExprV3Context) *ast.ActionV3 {
	if ctx == nil {
		return nil
	}
	actCtx := ctx.(*parser.ActionExprV3Context)
	action := &ast.ActionV3{}

	if actCtx.SAVE_CHANGES() != nil {
		action.Type = "save"
		action.ClosePage = actCtx.CLOSE_PAGE() != nil
	} else if actCtx.CANCEL_CHANGES() != nil {
		action.Type = "cancel"
		action.ClosePage = actCtx.CLOSE_PAGE() != nil
	} else if actCtx.CLOSE_PAGE() != nil && actCtx.SAVE_CHANGES() == nil && actCtx.CANCEL_CHANGES() == nil {
		action.Type = "close"
	} else if actCtx.DELETE_OBJECT() != nil {
		action.Type = "delete"
	} else if actCtx.DELETE() != nil {
		action.Type = "delete"
		action.ClosePage = actCtx.CLOSE_PAGE() != nil
	} else if actCtx.CREATE_OBJECT() != nil {
		action.Type = "create"
		if qn := actCtx.QualifiedName(); qn != nil {
			action.Target = getQualifiedNameText(qn)
		}
		// Check for THEN action
		if thenCtx := actCtx.ActionExprV3(); thenCtx != nil {
			action.ThenAction = buildActionV3(thenCtx)
		}
	} else if actCtx.SHOW_PAGE() != nil {
		action.Type = "showPage"
		if qn := actCtx.QualifiedName(); qn != nil {
			action.Target = getQualifiedNameText(qn)
		}
		if argsCtx := actCtx.MicroflowArgsV3(); argsCtx != nil {
			action.Args = buildMicroflowArgsV3(argsCtx)
		}
	} else if actCtx.MICROFLOW() != nil {
		action.Type = "microflow"
		if qn := actCtx.QualifiedName(); qn != nil {
			action.Target = getQualifiedNameText(qn)
		}
		if argsCtx := actCtx.MicroflowArgsV3(); argsCtx != nil {
			action.Args = buildMicroflowArgsV3(argsCtx)
		}
	} else if actCtx.NANOFLOW() != nil {
		action.Type = "nanoflow"
		if qn := actCtx.QualifiedName(); qn != nil {
			action.Target = getQualifiedNameText(qn)
		}
		if argsCtx := actCtx.MicroflowArgsV3(); argsCtx != nil {
			action.Args = buildMicroflowArgsV3(argsCtx)
		}
	} else if actCtx.OPEN_LINK() != nil {
		action.Type = "openLink"
		if str := actCtx.STRING_LITERAL(); str != nil {
			action.LinkURL = unquoteString(str.GetText())
		}
	} else if actCtx.SIGN_OUT() != nil {
		action.Type = "signOut"
	} else if actCtx.COMPLETE_TASK() != nil {
		action.Type = "completeTask"
		if str := actCtx.STRING_LITERAL(); str != nil {
			action.OutcomeValue = unquoteString(str.GetText())
		}
	}

	return action
}

// buildMicroflowArgsV3 builds flow arguments from the parse context.
func buildMicroflowArgsV3(ctx parser.IMicroflowArgsV3Context) []ast.FlowArgV3 {
	if ctx == nil {
		return nil
	}
	argsCtx := ctx.(*parser.MicroflowArgsV3Context)
	var args []ast.FlowArgV3

	for _, argCtx := range argsCtx.AllMicroflowArgV3() {
		arg := buildMicroflowArgV3(argCtx)
		args = append(args, arg)
	}

	return args
}

// buildMicroflowArgV3 builds a single flow argument.
func buildMicroflowArgV3(ctx parser.IMicroflowArgV3Context) ast.FlowArgV3 {
	argCtx := ctx.(*parser.MicroflowArgV3Context)
	arg := ast.FlowArgV3{}

	if v := argCtx.VARIABLE(); v != nil {
		// Microflow-style: $Param = $value
		arg.Name = strings.TrimPrefix(v.GetText(), "$")
	} else if id := argCtx.IDENTIFIER(); id != nil {
		// Widget-style: Param: $value
		arg.Name = id.GetText()
	} else if qid := argCtx.QUOTED_IDENTIFIER(); qid != nil {
		// Widget-style with reserved-word param name: "Param": $value
		arg.Name = unquoteIdentifier(qid.GetText())
	}
	if expr := argCtx.Expression(); expr != nil {
		arg.Value = expr.GetText()
	}

	return arg
}

// buildAttributeListV3 builds a list of attribute paths from the parse context.
func buildAttributeListV3(ctx parser.IAttributeListV3Context) []string {
	if ctx == nil {
		return nil
	}
	attrListCtx := ctx.(*parser.AttributeListV3Context)
	var attrs []string

	for _, qnCtx := range attrListCtx.AllQualifiedName() {
		attrs = append(attrs, qnCtx.GetText())
	}

	return attrs
}

// buildAssociationPathV3 builds an association path string from a parser context.
// Format: "Module.Assoc" or "Module.Assoc/Module.Entity" — qualified names separated by /.
func buildAssociationPathV3(ctx parser.IAssociationPathV3Context) string {
	if ctx == nil {
		return ""
	}
	apc := ctx.(*parser.AssociationPathV3Context)
	var parts []string
	for _, qn := range apc.AllQualifiedName() {
		parts = append(parts, getQualifiedNameText(qn))
	}
	return strings.Join(parts, "/")
}

// buildAttributePathV3 builds an attribute path string.
// Handles quoted identifiers (e.g., "Order") by stripping quotes.
func buildAttributePathV3(ctx parser.IAttributePathV3Context) string {
	if ctx == nil {
		return ""
	}
	text := ctx.GetText()
	// Strip double quotes or backticks from each path segment
	if strings.ContainsAny(text, "\"`") {
		parts := strings.Split(text, "/")
		for i, p := range parts {
			parts[i] = unquoteIdentifier(p)
		}
		return strings.Join(parts, "/")
	}
	return text
}

// buildStringExprV3 extracts string from stringExprV3.
// Can return either a quoted literal string or an unquoted attribute reference.
func buildStringExprV3(ctx parser.IStringExprV3Context) string {
	if ctx == nil {
		return ""
	}
	strCtx := ctx.(*parser.StringExprV3Context)

	// String literal: 'Hello {1}'
	if str := strCtx.STRING_LITERAL(); str != nil {
		return unquoteString(str.GetText())
	}

	// Attribute path: Name or Entity/Attr
	if attrPath := strCtx.AttributePathV3(); attrPath != nil {
		return attrPath.GetText()
	}

	// Variable reference: $var or $var.Attr
	if variable := strCtx.VARIABLE(); variable != nil {
		result := variable.GetText()
		// Check for .Attr suffix
		if dot := strCtx.DOT(); dot != nil {
			if id := strCtx.IDENTIFIER(); id != nil {
				result += "." + id.GetText()
			} else if kw := strCtx.Keyword(); kw != nil {
				result += "." + kw.GetText()
			}
		}
		return result
	}

	return ""
}

// buildParamListV3 builds parameter assignments from paramListV3.
func buildParamListV3(ctx parser.IParamListV3Context) []ast.ParamAssignmentV3 {
	if ctx == nil {
		return nil
	}
	plCtx := ctx.(*parser.ParamListV3Context)
	var params []ast.ParamAssignmentV3

	for _, paCtx := range plCtx.AllParamAssignmentV3() {
		params = append(params, buildParamAssignmentV3(paCtx))
	}

	return params
}

// buildParamAssignmentV3 builds a single parameter assignment.
func buildParamAssignmentV3(ctx parser.IParamAssignmentV3Context) ast.ParamAssignmentV3 {
	paCtx := ctx.(*parser.ParamAssignmentV3Context)
	param := ast.ParamAssignmentV3{}

	if num := paCtx.NUMBER_LITERAL(); num != nil {
		if n, err := strconv.Atoi(num.GetText()); err == nil {
			param.Index = n
		}
	}
	if expr := paCtx.Expression(); expr != nil {
		param.Value = stripExpressionIdentifierQuotes(expr.GetText())
	}

	return param
}

// buildXPathString builds a WHERE string from xpath constraints and and/or operators.
// xpathTokenRe matches a Mendix XPath token like [%CurrentUser%] or
// [%UserRole_Admin%]. The body is anything but % or ].
var xpathTokenRe = regexp.MustCompile(`\[%[^%\]]+%\]`)

// normalizeXPathTokens quotes any bare [%Token%] in an XPath constraint string.
// A token used as a value must be quoted ('[%CurrentDateTime%]') or Studio Pro
// rejects the constraint with CE0161. The inline bracket form preserved the raw
// (unquoted) token via the constraint's original source text; this requotes it.
// Tokens already wrapped in single quotes are left untouched (no double-quoting).
// Issue #641.
func normalizeXPathTokens(xpath string) string {
	locs := xpathTokenRe.FindAllStringIndex(xpath, -1)
	if locs == nil {
		return xpath
	}
	var b strings.Builder
	prev := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		b.WriteString(xpath[prev:start])
		quotedBefore := start > 0 && xpath[start-1] == '\''
		quotedAfter := end < len(xpath) && xpath[end] == '\''
		if quotedBefore || quotedAfter {
			b.WriteString(xpath[start:end])
		} else {
			b.WriteByte('\'')
			b.WriteString(xpath[start:end])
			b.WriteByte('\'')
		}
		prev = end
	}
	b.WriteString(xpath[prev:])
	return b.String()
}

// bracketedXPathFromExpr converts a datasource WHERE expression (the
// `where '<xpath>'` / `where <expr>` form, as opposed to inline `where [<xpath>]`)
// into a bracketed XPath constraint. A bare quoted string is the constraint as a
// literal — use its UNQUOTED value; re-serializing it via xpathExprToString would
// re-double the ” escapes and produce `['[Title=”abc”]']`, which fails CE0161
// (issue #642). Other expressions serialize normally.
func bracketedXPathFromExpr(built ast.Expression) string {
	if lit, ok := built.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		if s, ok := lit.Value.(string); ok {
			s = strings.TrimSpace(s)
			if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
				s = "[" + s + "]"
			}
			return normalizeXPathTokens(s)
		}
	}
	return normalizeXPathTokens("[" + xpathExprToString(built) + "]")
}

func buildXPathString(xpathConstraints []parser.IXpathConstraintContext, andOrOps []parser.IAndOrXpathContext) string {
	if len(xpathConstraints) == 0 {
		return ""
	}

	// Build AST expressions from each xpath constraint
	var exprs []ast.Expression
	for _, xc := range xpathConstraints {
		xcCtx := xc.(*parser.XpathConstraintContext)
		if xpathExpr := xcCtx.XpathExpr(); xpathExpr != nil {
			exprs = append(exprs, buildXPathExpr(xpathExpr))
		}
	}

	if len(exprs) == 0 {
		return ""
	}

	if len(exprs) == 1 {
		return "[" + xpathExprToString(exprs[0]) + "]"
	}

	// Check if any operator is OR
	hasOr := false
	for _, op := range andOrOps {
		opCtx := op.(*parser.AndOrXpathContext)
		if opCtx.OR() != nil {
			hasOr = true
			break
		}
	}

	if hasOr {
		// If any OR operator, combine into single bracket: [(expr1) op (expr2) ...]
		var parts []string
		for i, expr := range exprs {
			parts = append(parts, "("+xpathExprToString(expr)+")")
			if i < len(andOrOps) {
				opCtx := andOrOps[i].(*parser.AndOrXpathContext)
				if opCtx.OR() != nil {
					parts = append(parts, "or")
				} else {
					parts = append(parts, "and")
				}
			}
		}
		return "[" + strings.Join(parts, " ") + "]"
	}

	// All AND: keep as separate brackets [expr1][expr2]
	var sb strings.Builder
	for _, expr := range exprs {
		sb.WriteString("[" + xpathExprToString(expr) + "]")
	}
	return sb.String()
}

// buildSortColumnAsOrderBy converts a sortColumn context to an OrderByItemV3.
func buildSortColumnAsOrderBy(ctx parser.ISortColumnContext) ast.OrderByItemV3 {
	scCtx := ctx.(*parser.SortColumnContext)
	item := ast.OrderByItemV3{Direction: "ASC"}

	if qn := scCtx.QualifiedName(); qn != nil {
		item.Attribute = getQualifiedNameText(qn)
	} else if id := scCtx.IDENTIFIER(); id != nil {
		item.Attribute = id.GetText()
	}

	if scCtx.DESC() != nil {
		item.Direction = "DESC"
	}

	return item
}

// buildConditionalExpression turns a `Visible: [...]` / `Editable: [...]`
// constraint into a Mendix client-side visibility expression. Unlike a data
// source XPath, conditional visibility is evaluated against the widget's data
// context, so a bare attribute reference (`Name`) must be rooted in the context
// object as `$currentObject/Name`; otherwise Studio Pro rejects it with CE0117.
// Paths already rooted in a variable (`$currentObject/...`, `$Param/...`) and
// literals/functions are left untouched. See issue #627.
func buildConditionalExpression(xc parser.IXpathConstraintContext) string {
	if xc == nil {
		return ""
	}
	xcCtx, ok := xc.(*parser.XpathConstraintContext)
	if !ok {
		return stripExpressionIdentifierQuotes(extractXpathText(xc))
	}
	xe := xcCtx.XpathExpr()
	if xe == nil {
		return stripExpressionIdentifierQuotes(extractXpathText(xc))
	}
	expr := buildXPathExpr(xe)
	if expr == nil {
		return stripExpressionIdentifierQuotes(extractXpathText(xc))
	}
	return stripExpressionIdentifierQuotes(conditionalExprToString(expr))
}

// conditionalExprToString serializes an expression as a Mendix conditional-
// visibility expression, prefixing bare attribute references with the widget
// data context ($currentObject). It mirrors xpathExprToString for every other
// node, recursing through the logical/comparison structure.
func conditionalExprToString(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.IdentifierExpr:
		// A bare attribute (`Active`, `Name`) — root it in the context object.
		return "$currentObject/" + e.Name
	case *ast.XPathPathExpr:
		// Multi-step / predicated path. If it already starts with a variable
		// ($currentObject/…, $Param/…) it is fully qualified; otherwise it is a
		// bare attribute path and needs the context root.
		if len(e.Steps) > 0 {
			if _, isVar := e.Steps[0].Expr.(*ast.VariableExpr); !isVar {
				return "$currentObject/" + xpathPathToString(e)
			}
		}
		return xpathPathToString(e)
	case *ast.BinaryExpr:
		return conditionalExprToString(e.Left) + " " + strings.ToLower(e.Operator) + " " + conditionalExprToString(e.Right)
	case *ast.UnaryExpr:
		op := strings.ToLower(e.Operator)
		if op == "not" {
			if p, ok := e.Operand.(*ast.ParenExpr); ok {
				return "not(" + conditionalExprToString(p.Inner) + ")"
			}
			return "not(" + conditionalExprToString(e.Operand) + ")"
		}
		return op + " " + conditionalExprToString(e.Operand)
	case *ast.ParenExpr:
		return "(" + conditionalExprToString(e.Inner) + ")"
	case *ast.FunctionCallExpr:
		args := make([]string, 0, len(e.Arguments))
		for _, arg := range e.Arguments {
			args = append(args, conditionalExprToString(arg))
		}
		return e.Name + "(" + strings.Join(args, ", ") + ")"
	case *ast.QualifiedNameExpr:
		// Enum value (Module.Enum.Value) in a client visibility/editability
		// expression: keep the QUALIFIED literal. Unlike an XPath datasource
		// constraint (evaluated at the database level, where enums are strings),
		// a client expression compares to the qualified enum value, not 'Value' —
		// stringifying it produces CE0117 "Error(s) in expression" (#627 regression).
		return e.QualifiedName.String()
	default:
		// Literals, $variables — same as XPath rendering.
		return xpathExprToString(expr)
	}
}

// buildPropertyValueV3 builds a generic property value.
// extractXpathText extracts the expression text from inside [brackets].
func extractXpathText(xc parser.IXpathConstraintContext) string {
	if xc == nil {
		return ""
	}
	// Get the full text including brackets, then strip them
	text := xc.GetText()
	if len(text) >= 2 && text[0] == '[' && text[len(text)-1] == ']' {
		return text[1 : len(text)-1]
	}
	return text
}

// parseWidthValue parses a column width value: numeric (1-12) or "AutoFill".
func parseWidthValue(text string) any {
	if strings.EqualFold(text, "AutoFill") {
		return "AutoFill"
	}
	if n, err := strconv.Atoi(text); err == nil {
		return n
	}
	return text
}

func buildPropertyValueV3(ctx parser.IPropertyValueV3Context) any {
	if ctx == nil {
		return nil
	}
	pvCtx := ctx.(*parser.PropertyValueV3Context)

	if str := pvCtx.STRING_LITERAL(); str != nil {
		return unquoteString(str.GetText())
	}
	// "AttrName" — a double-quoted value, used for pluggable-widget attribute
	// sub-properties like chart series `staticXAttribute: "StatusValue"`. Strip
	// the quotes; the executor resolves it against the item's datasource entity.
	if q := pvCtx.QUOTED_IDENTIFIER(); q != nil {
		return unquoteIdentifier(q.GetText())
	}
	if num := pvCtx.NUMBER_LITERAL(); num != nil {
		text := num.GetText()
		if strings.Contains(text, ".") {
			if f, err := strconv.ParseFloat(text, 64); err == nil {
				return f
			}
		}
		if n, err := strconv.Atoi(text); err == nil {
			return n
		}
		return text
	}
	if bl := pvCtx.BooleanLiteral(); bl != nil {
		return strings.EqualFold(bl.GetText(), "true")
	}
	if qn := pvCtx.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	if id := pvCtx.IDENTIFIER(); id != nil {
		return id.GetText()
	}
	// Handle H1-H6 tokens (used for HeaderMode)
	for _, hFn := range []func() antlr.TerminalNode{pvCtx.H1, pvCtx.H2, pvCtx.H3, pvCtx.H4, pvCtx.H5, pvCtx.H6} {
		if h := hFn(); h != nil {
			return h.GetText()
		}
	}

	// Handle array values: [expr1, expr2, ...]
	if pvCtx.LBRACKET() != nil {
		var items []string
		for _, expr := range pvCtx.AllExpression() {
			items = append(items, expr.GetText())
		}
		return items
	}

	return pvCtx.GetText()
}

// buildDesignPropertyListV3 builds design properties from the parse context.
func buildDesignPropertyListV3(ctx parser.IDesignPropertyListV3Context) []ast.DesignPropertyEntryV3 {
	if ctx == nil {
		return nil
	}
	dpCtx := ctx.(*parser.DesignPropertyListV3Context)
	var props []ast.DesignPropertyEntryV3

	for _, entryCtx := range dpCtx.AllDesignPropertyEntryV3() {
		if entry := buildDesignPropertyEntryV3(entryCtx); entry != nil {
			props = append(props, *entry)
		}
	}

	return props
}

// buildDesignPropertyEntryV3 builds a single design property entry.
func buildDesignPropertyEntryV3(ctx parser.IDesignPropertyEntryV3Context) *ast.DesignPropertyEntryV3 {
	if ctx == nil {
		return nil
	}
	entryCtx := ctx.(*parser.DesignPropertyEntryV3Context)

	// Key is always the first STRING_LITERAL
	allStrings := entryCtx.AllSTRING_LITERAL()
	if len(allStrings) == 0 {
		return nil
	}

	key := unquoteString(allStrings[0].GetText())

	// Compound (nested): 'Spacing': ['margin-top': 'Large', 'margin-bottom': 'Medium']
	if listCtx := entryCtx.DesignPropertyListV3(); listCtx != nil {
		return &ast.DesignPropertyEntryV3{Key: key, Nested: buildDesignPropertyListV3(listCtx)}
	}

	// Value: second STRING_LITERAL, ON, or OFF
	if entryCtx.ON() != nil {
		return &ast.DesignPropertyEntryV3{Key: key, Value: "on"}
	}
	if entryCtx.OFF() != nil {
		return &ast.DesignPropertyEntryV3{Key: key, Value: "off"}
	}
	if len(allStrings) >= 2 {
		return &ast.DesignPropertyEntryV3{Key: key, Value: unquoteString(allStrings[1].GetText())}
	}

	return nil
}

// buildWidgetBodyV3 extracts children from a widget body.
func buildWidgetBodyV3(ctx parser.IWidgetBodyV3Context, b *Builder) []*ast.WidgetV3 {
	if ctx == nil {
		return nil
	}
	bodyCtx := ctx.(*parser.WidgetBodyV3Context)

	if pbCtx := bodyCtx.PageBodyV3(); pbCtx != nil {
		return buildPageBodyV3(pbCtx, b)
	}

	return nil
}

// ExitDefineFragmentStatement handles DEFINE FRAGMENT Name AS { widgets }.
func (b *Builder) ExitDefineFragmentStatement(ctx *parser.DefineFragmentStatementContext) {
	stmt := &ast.DefineFragmentStmt{}
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		stmt.Name = identifierOrKeywordText(iok)
	}
	if bodyCtx := ctx.PageBodyV3(); bodyCtx != nil {
		stmt.Widgets = buildPageBodyV3(bodyCtx, b)
	}
	b.statements = append(b.statements, stmt)
}

// xpathExprToString converts an AST Expression to a properly formatted XPath expression string.
// XPath uses lowercase boolean operators (and, or, not) and requires proper whitespace.
func xpathExprToString(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.LiteralExpr:
		switch e.Kind {
		case ast.LiteralString:
			strVal := fmt.Sprintf("%v", e.Value)
			strVal = strings.ReplaceAll(strVal, `'`, `''`)
			return "'" + strVal + "'"
		case ast.LiteralBoolean:
			if e.Value.(bool) {
				return "true"
			}
			return "false"
		case ast.LiteralNull, ast.LiteralEmpty:
			return "empty"
		default:
			return fmt.Sprintf("%v", e.Value)
		}
	case *ast.VariableExpr:
		return "$" + e.Name
	case *ast.AttributePathExpr:
		return "$" + e.Variable + "/" + strings.Join(e.Path, "/")
	case *ast.BinaryExpr:
		left := xpathExprToString(e.Left)
		right := xpathExprToString(e.Right)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		operand := xpathExprToString(e.Operand)
		op := strings.ToLower(e.Operator)
		// For 'not' with parenthesized operand, output as not(expr) instead of not (expr)
		if op == "not" {
			if p, ok := e.Operand.(*ast.ParenExpr); ok {
				return "not(" + xpathExprToString(p.Inner) + ")"
			}
			return "not(" + operand + ")"
		}
		return op + " " + operand
	case *ast.XPathPathExpr:
		return xpathPathToString(e)
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, xpathExprToString(arg))
		}
		return e.Name + "(" + strings.Join(args, ", ") + ")"
	case *ast.TokenExpr:
		return "[%" + e.Token + "%]"
	case *ast.ParenExpr:
		return "(" + xpathExprToString(e.Inner) + ")"
	case *ast.IdentifierExpr:
		return e.Name
	case *ast.QualifiedNameExpr:
		// XPath constraints run at the database level; enum values must be string literals.
		// 3-part names (Module.EnumName.Value) → 'Value'; 2-part names pass through.
		if dotIdx := strings.LastIndex(e.QualifiedName.Name, "."); dotIdx >= 0 {
			return "'" + e.QualifiedName.Name[dotIdx+1:] + "'"
		}
		return e.QualifiedName.String()
	default:
		return ""
	}
}

// xpathPathToString serializes an XPathPathExpr to a string like "Module.Assoc/Entity/Attr"
// or "System.roles[reversed()]/System.UserRole".
func xpathPathToString(path *ast.XPathPathExpr) string {
	var parts []string
	for _, step := range path.Steps {
		s := xpathExprToString(step.Expr)
		if step.Predicate != nil {
			s += "[" + xpathExprToString(step.Predicate) + "]"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "/")
}

// buildSnippetCallParamListV3 converts a parsed snippetCallParamListV3 context
// into a slice of SnippetCallParam AST nodes.
func buildSnippetCallParamListV3(ctx parser.ISnippetCallParamListV3Context) []ast.SnippetCallParam {
	var params []ast.SnippetCallParam
	for _, mappingCtx := range ctx.AllSnippetCallParamMappingV3() {
		param := ast.SnippetCallParam{}
		if iok := mappingCtx.IdentifierOrKeyword(); iok != nil {
			// Param name written without $: Agent: $someVar or Asset: $someVar
			param.ParamName = iok.GetText()
			if vars := mappingCtx.AllVARIABLE(); len(vars) > 0 {
				param.Variable = vars[0].GetText()
			}
		} else {
			// Param name written with $: $Asset: $someVar
			vars := mappingCtx.AllVARIABLE()
			if len(vars) >= 2 {
				param.ParamName = vars[0].GetText()
				param.Variable = vars[1].GetText()
			}
		}
		if param.ParamName != "" && param.Variable != "" {
			params = append(params, param)
		}
	}
	return params
}
