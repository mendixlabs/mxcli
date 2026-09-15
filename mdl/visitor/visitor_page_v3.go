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
	"github.com/mendixlabs/mxcli/mdl/types"
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
		stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
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

	// Parse V3 body. Bare widgets bind to Main; `placeholder <Name> { … }`
	// blocks bind to named layout placeholders (issue #532).
	if bodyCtx := ctx.PageBodyV3(); bodyCtx != nil {
		stmt.Widgets = buildPageBodyV3(bodyCtx, b)
		stmt.Placeholders = buildPagePlaceholdersV3(bodyCtx, b)
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
		stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
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

		// Walk the parse tree rather than re-splitting its TEXT. GetText() hands
		// back the source verbatim, so a quoted entity name arrived as
		// `Pd."Thing"` and exec failed with `entity not found: Pd."Thing"` —
		// while the identical quoted form in a PAGE parameter resolved, because
		// that path has always used buildQualifiedName (ako/CapTrackV4 019). The
		// project convention is to quote every identifier, so this was reached by
		// following the house style.
		if dt := spCtx.DataType(); dt != nil {
			if qn := dt.(*parser.DataTypeContext).QualifiedName(); qn != nil {
				param.EntityType = buildQualifiedName(qn)
			}
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
			if ref := buildUseFragmentRef(c, b); ref != nil {
				widgets = append(widgets, ref)
			}
		case *parser.UseBuildingBlockRefContext:
			if ref := buildUseBuildingBlockRef(c); ref != nil {
				widgets = append(widgets, ref)
			}
		case *parser.SlotMarkerV3Context:
			if ref := buildSlotMarkerV3(c); ref != nil {
				widgets = append(widgets, ref)
			}
		case *parser.PlaceholderBlockV3Context:
			// A bodyless `placeholder Main` is a layout *declaring* a slot, not
			// a page filling one: a binding with no widgets fills nothing, and
			// a Forms$Placeholder has no children to carry. The two jobs
			// the one grammar rule serves are told apart by that shape rather
			// than by tracking which document is being built, so the same
			// distinction holds wherever a page body appears — including nested
			// inside a scroll container region.
			//
			// The braced form is left to buildPagePlaceholdersV3, which the
			// page executor calls separately.
			if c.LBRACE() == nil {
				widgets = append(widgets, &ast.WidgetV3{
					Type:       "placeholder",
					Name:       placeholderBlockName(c),
					Properties: map[string]any{},
				})
			} else if b != nil && b.inLayout {
				// Still dropped — the braced form declares nothing. But in a
				// LAYOUT it is a mistake rather than the page-side job, so the
				// name is recorded for the checker to report against. Without
				// this the layout silently ends up with no placeholder and the
				// write fails with a message that contradicts the script
				// (mendixlabs/mxcli#1063).
				b.layoutBracedPlaceholders = append(b.layoutBracedPlaceholders,
					placeholderBlockName(c))
			}
		}
	}

	return widgets
}

// buildPagePlaceholdersV3 extracts `placeholder <Name> { … }` blocks from a page
// body (issue #532). Bare widgets are handled by buildPageBodyV3 (→ Main); this
// collects the named-placeholder groups. Only meaningful for pages (a snippet or
// widget child has no layout), where the caller ignores the result.
func buildPagePlaceholdersV3(ctx parser.IPageBodyV3Context, b *Builder) []*ast.PagePlaceholderV3 {
	if ctx == nil {
		return nil
	}
	bodyCtx := ctx.(*parser.PageBodyV3Context)
	var out []*ast.PagePlaceholderV3
	for _, child := range bodyCtx.GetChildren() {
		pc, ok := child.(*parser.PlaceholderBlockV3Context)
		if !ok {
			continue
		}
		ph := &ast.PagePlaceholderV3{Name: placeholderBlockName(pc)}
		for _, inner := range pc.GetChildren() {
			switch c := inner.(type) {
			case *parser.WidgetV3Context:
				if w := buildWidgetV3(c, b); w != nil {
					ph.Widgets = append(ph.Widgets, w)
				}
			case *parser.UseFragmentRefContext:
				if ref := buildUseFragmentRef(c, b); ref != nil {
					ph.Widgets = append(ph.Widgets, ref)
				}
			case *parser.UseBuildingBlockRefContext:
				if ref := buildUseBuildingBlockRef(c); ref != nil {
					ph.Widgets = append(ph.Widgets, ref)
				}
			case *parser.SlotMarkerV3Context:
				if ref := buildSlotMarkerV3(c); ref != nil {
					ph.Widgets = append(ph.Widgets, ref)
				}
			}
		}
		out = append(out, ph)
	}
	return out
}

// placeholderBlockName returns the placeholder name (unquoted if `"quoted"`;
// keywords like Right/Left/Content are accepted via identifierOrKeyword).
func placeholderBlockName(pc *parser.PlaceholderBlockV3Context) string {
	if idk := pc.IdentifierOrKeyword(); idk != nil {
		return identifierOrKeywordText(idk)
	}
	return ""
}

// buildUseFragmentRef creates a WidgetV3 with sentinel type USE_FRAGMENT. When the
// `use fragment X { … }` form supplies a payload block, its widgets are built and
// stored in the sentinel's Children — the executor splices them into the
// fragment's content slot at expansion time.
func buildUseFragmentRef(ctx *parser.UseFragmentRefContext, b *Builder) *ast.WidgetV3 {
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
	if args := buildFragmentArgs(ctx.FragmentArgs()); len(args) > 0 {
		w.Properties["Args"] = args
	}
	if payload := ctx.UseFragmentPayload(); payload != nil {
		if pc, ok := payload.(*parser.UseFragmentPayloadContext); ok {
			w.Children = buildPageBodyV3(pc.PageBodyV3(), b)
		}
	}
	return w
}

// buildSlotMarkerV3 creates a WidgetV3 with sentinel type SLOT. The optional name
// defaults to "content"; it is cosmetic in v1 (a fragment supports one slot).
func buildSlotMarkerV3(ctx *parser.SlotMarkerV3Context) *ast.WidgetV3 {
	if ctx == nil {
		return nil
	}
	name := "content"
	if idk := ctx.IdentifierOrKeyword(); idk != nil {
		name = identifierOrKeywordText(idk)
	}
	return &ast.WidgetV3{
		Type:       "SLOT",
		Name:       name,
		Properties: make(map[string]interface{}),
	}
}

// buildUseBuildingBlockRef creates a WidgetV3 with sentinel type USE_BUILDING_BLOCK.
// The Name holds the block's qualified name (e.g. "Atlas_Web_Content.Card"); the
// executor resolves the block, renders its widget tree to MDL, re-parses it, and
// deep-copies the widgets into the page/container with an optional prefix rename.
func buildUseBuildingBlockRef(ctx *parser.UseBuildingBlockRefContext) *ast.WidgetV3 {
	if ctx == nil {
		return nil
	}
	w := &ast.WidgetV3{
		Type:       "USE_BUILDING_BLOCK",
		Properties: make(map[string]interface{}),
	}
	if qn := ctx.QualifiedName(); qn != nil {
		w.Name = buildQualifiedName(qn).String() // "Module.BlockName"
	}
	w.Properties["Prefix"] = ""
	if idk := ctx.IdentifierOrKeyword(); idk != nil {
		w.Properties["Prefix"] = identifierOrKeywordText(idk) // Optional prefix
	}
	// Optional rebind overrides: (datasource: <ds>, action: <action>).
	if ov := ctx.BlockOverrides(); ov != nil {
		if oc, ok := ov.(*parser.BlockOverridesContext); ok {
			for _, o := range oc.AllBlockOverride() {
				bo, ok := o.(*parser.BlockOverrideContext)
				if !ok {
					continue
				}
				if ds := bo.DataSourceExprV3(); ds != nil {
					w.Properties["DataSourceOverride"] = buildDataSourceV3(ds)
				} else if act := bo.ActionExprV3(); act != nil {
					w.Properties["ActionOverride"] = buildActionV3(act)
				}
			}
		}
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

	// A List View specialization template: `template for Module.Entity { ... }`.
	// It has no name — the entity is what identifies it — so this returns before
	// the name lookup below, which would otherwise find nothing.
	if wCtx.FOR() != nil && wCtx.TEMPLATE() != nil {
		widget.Type = "template"
		if qn := wCtx.QualifiedName(); qn != nil {
			widget.Specialization = qn.GetText()
		}
		if bodyCtx := wCtx.WidgetBodyV3(); bodyCtx != nil {
			widget.Children = buildWidgetBodyV3(bodyCtx, b)
		}
		return widget
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
		// Which alternative matched, taken from the parse tree rather than by
		// comparing the text against a list of known widget names. A generic
		// type must resolve to a widget definition; an enumerated one is a
		// built-in. See ast.WidgetV3.TypeIsGeneric.
		// Both generic alternatives count. IDENTIFIER covers `htmlelement`
		// (slice 2); Keyword covers a container whose name lexes as a keyword
		// token, such as `attribute` (slice 3) — the case that motivated the
		// issue. An enumerated widget type is a direct token alternative of
		// widgetTypeV3 and matches neither accessor.
		if typeCtx.IDENTIFIER() != nil || typeCtx.Keyword() != nil {
			widget.TypeIsGeneric = true
		}
	}

	// Get required identifier. The name may be quoted (QUOTED_IDENTIFIER) when it
	// collides with a reserved keyword, e.g. a widget named "List" (issue #619), or
	// a bare MDL keyword used unquoted as a name (`container body`, `dynamictext
	// content`) — accepted by the grammar so a natural widget name is not blocked
	// (traceops #12).
	if id := wCtx.IDENTIFIER(); id != nil {
		widget.Name = id.GetText()
	} else if qid := wCtx.QUOTED_IDENTIFIER(); qid != nil {
		widget.Name = unquoteIdentifier(qid.GetText())
	} else if kw := wCtx.Keyword(); kw != nil {
		widget.Name = kw.GetText()
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

	// OnChange: ... (input widgets — the "On change" action)
	if propCtx.ONCHANGE() != nil {
		if actCtx := propCtx.ActionExprV3(); actCtx != nil {
			widget.Properties["OnChange"] = buildActionV3(actCtx)
		}
		return
	}

	// Placeholder: 'text' (input widgets)
	if propCtx.PLACEHOLDER() != nil {
		if str := propCtx.STRING_LITERAL(); str != nil {
			widget.Properties["Placeholder"] = unquoteString(str.GetText())
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

	// Icon: 'Mod.Coll.name' | image Mod.Images.logo | glyph 57377
	if propCtx.ICON() != nil {
		if icon := buildWidgetIconV3(propCtx.WidgetIconV3()); icon != nil {
			widget.Properties["Icon"] = icon
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
		// `<Name>Params: [{1} = Attr]` — the parameters of a text-template
		// sub-property whose name belongs to the WIDGET rather than to MDL (a
		// File Uploader custom button's ButtonCaptionParams). ContentParams and
		// CaptionParams have their own tokens and are handled above; every other
		// template's companion arrives here (#956).
		if plCtx := propCtx.ParamListV3(); plCtx != nil {
			widget.Properties[id.GetText()] = buildParamListV3(plCtx)
			return
		}
		// Generic datasource-typed property (e.g. chart series `staticDataSource:
		// database Module.View`). The executor's object-list builder resolves the
		// *ast.DataSourceV3 into a widget datasource + entity context. Chart 9a.
		if dsCtx := propCtx.DataSourceExprV3(); dsCtx != nil {
			widget.Properties[id.GetText()] = buildDataSourceV3(dsCtx)
			return
		}
		// Generic action-typed property — a named action slot (#956). Only the
		// forms that do not also parse as a data source reach here; MICROFLOW /
		// NANOFLOW / VARIABLE are matched by the branch above and converted by
		// the executor, which is the layer that knows the slot is an action.
		if actCtx := propCtx.ActionExprV3(); actCtx != nil {
			widget.Properties[id.GetText()] = buildActionV3(actCtx)
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
		if plCtx := propCtx.ParamListV3(); plCtx != nil {
			widget.Properties[kw.GetText()] = buildParamListV3(plCtx)
			return
		}
		if dsCtx := propCtx.DataSourceExprV3(); dsCtx != nil {
			widget.Properties[kw.GetText()] = buildDataSourceV3(dsCtx)
			return
		}
		// A named action slot whose key is an MDL keyword — Switch stores its
		// slot as `action`, PopupMenu's menu items as `action` too (#956).
		if actCtx := propCtx.ActionExprV3(); actCtx != nil {
			widget.Properties[kw.GetText()] = buildActionV3(actCtx)
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

	if v := actCtx.VARIABLE(); v != nil {
		// $handler — a fragment action parameter; resolved at expansion.
		action.Type = "param"
		action.Target = strings.TrimPrefix(v.GetText(), "$")
	} else if actCtx.NOTHING() != nil {
		// An explicitly inert widget. Byte-identical to what the scalar
		// fall-through already produced (Forms$NoAction) — what changes is that
		// the slot now holds an *ast.ActionV3, so a scalar left in it means the
		// action expression failed to parse rather than "the author wrote
		// NOTHING". See MDL-WIDGET28 (mendixlabs/mxcli#1062).
		action.Type = "none"
	} else if actCtx.SAVE_CHANGES() != nil {
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
	} else if iok := argCtx.IdentifierOrKeyword(); iok != nil {
		// Widget-style: Param: $value. identifierOrKeyword accepts a bare
		// keyword (View/Source/Item/Page/Entity) or a "quoted" name;
		// identifierOrKeywordText unquotes as needed.
		arg.Name = identifierOrKeywordText(iok)
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
	if fmtCtx := paCtx.ParamFormatV3(); fmtCtx != nil {
		param.Format = buildParamFormatV3(fmtCtx)
	}

	return param
}

// buildParamFormatV3 collects the raw key/value pairs of a parameter format
// block, e.g. `(decimalPrecision: 2, groupDigits: true)`. Keys are lowercased;
// string values have surrounding quotes stripped. Validation of keys/values
// happens later at check time (validate_widgets), not here.
func buildParamFormatV3(ctx parser.IParamFormatV3Context) *ast.ParamFormatV3 {
	fc, ok := ctx.(*parser.ParamFormatV3Context)
	if !ok {
		return nil
	}
	f := &ast.ParamFormatV3{}
	for _, p := range fc.AllParamFormatPropV3() {
		pp, ok := p.(*parser.ParamFormatPropV3Context)
		if !ok {
			continue
		}
		id := pp.IDENTIFIER()
		vc := pp.PropertyValueV3()
		if id == nil || vc == nil {
			continue
		}
		val := strings.Trim(vc.GetText(), "'\"")
		f.Props = append(f.Props, ast.ParamFormatProp{
			Key:   strings.ToLower(id.GetText()),
			Value: val,
		})
	}
	return f
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
	// `[(k: v, …), …]` — a repeatable widget property written as a property
	// VALUE. Built into its own type so the property validator can REPORT it
	// (MDL-WIDGET27) rather than mis-handle it: the single-key shape used to
	// flatten into a []string that no writer claimed, so it checked clean,
	// exec'd, and vanished (mendixlabs/mxcli#999). It is never given a write
	// path — the entries belong in a container block in the widget body.
	if oel := pvCtx.ObjectEntryListV3(); oel != nil {
		return buildObjectEntryListV3(oel)
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

// ExitDefineFragmentStatement handles DEFINE FRAGMENT Name [(params)] AS { widgets }.
func (b *Builder) ExitDefineFragmentStatement(ctx *parser.DefineFragmentStatementContext) {
	stmt := &ast.DefineFragmentStmt{}
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		stmt.Name = identifierOrKeywordText(iok)
	}
	stmt.Params = buildFragmentParams(ctx.FragmentParams())
	if bodyCtx := ctx.PageBodyV3(); bodyCtx != nil {
		stmt.Widgets = buildPageBodyV3(bodyCtx, b)
	}
	b.statements = append(b.statements, stmt)
}

// buildFragmentParams extracts a fragment's typed parameter declarations.
func buildFragmentParams(ctx parser.IFragmentParamsContext) []ast.FragmentParam {
	if ctx == nil {
		return nil
	}
	pc, ok := ctx.(*parser.FragmentParamsContext)
	if !ok {
		return nil
	}
	var out []ast.FragmentParam
	for _, p := range pc.AllFragmentParam() {
		param, ok := p.(*parser.FragmentParamContext)
		if !ok {
			continue
		}
		fp := ast.FragmentParam{Kind: "datasource"}
		if v := param.VARIABLE(); v != nil {
			fp.Name = strings.TrimPrefix(v.GetText(), "$")
		}
		if t := param.FragmentParamType(); t != nil {
			if tc, ok := t.(*parser.FragmentParamTypeContext); ok && tc.ACTION() != nil {
				fp.Kind = "action"
			}
		}
		out = append(out, fp)
	}
	return out
}

// buildFragmentArgs extracts the values supplied at a `use fragment` site. Each
// value parses as either a datasource or an action (they overlap on
// microflow/nanoflow); the executor picks by the parameter's declared kind.
func buildFragmentArgs(ctx parser.IFragmentArgsContext) []ast.FragmentArg {
	if ctx == nil {
		return nil
	}
	ac, ok := ctx.(*parser.FragmentArgsContext)
	if !ok {
		return nil
	}
	var out []ast.FragmentArg
	for _, a := range ac.AllFragmentArg() {
		argCtx, ok := a.(*parser.FragmentArgContext)
		if !ok {
			continue
		}
		arg := ast.FragmentArg{}
		if v := argCtx.VARIABLE(); v != nil {
			arg.Name = strings.TrimPrefix(v.GetText(), "$")
		}
		if val := argCtx.FragmentArgValue(); val != nil {
			if vc, ok := val.(*parser.FragmentArgValueContext); ok {
				if ds := vc.DataSourceExprV3(); ds != nil {
					arg.DataSource = buildDataSourceV3(ds)
				} else if act := vc.ActionExprV3(); act != nil {
					arg.Action = buildActionV3(act)
				}
			}
		}
		out = append(out, arg)
	}
	return out
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
		// Unary minus binds to its operand: `-7`, not `- 7`.
		if op == "-" {
			return "-" + operand
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

// buildLayoutV3 builds `create layout Module.Name ( … ) { … }`.
//
// The property block is parsed with the widget-property machinery rather than a
// header rule of its own: a layout's header is a flat ( key: value ) list, which
// is exactly what widgetPropertiesV3 already reads, and reusing it means a
// layout property behaves like every other property in MDL.
func (b *Builder) buildLayoutV3(ctx *parser.CreateLayoutStatementContext) *ast.CreateLayoutStmt {
	stmt := &ast.CreateLayoutStmt{Properties: map[string]any{}}

	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Name = buildQualifiedName(qn)
	}
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil {
			stmt.IsReplace = createStmt.REPLACE() != nil
			stmt.IsModify = createStmt.MODIFY() != nil
		}
		stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
	}
	if props := ctx.WidgetPropertiesV3(); props != nil {
		holder := &ast.WidgetV3{Properties: map[string]any{}}
		parseWidgetPropertiesV3(props, holder, b)
		stmt.Properties = holder.Properties
	}
	if bodyCtx := ctx.PageBodyV3(); bodyCtx != nil {
		// Saved and restored rather than just set: the flag steers the shared
		// page-body builder, and leaving it on would make the next page in the
		// script report its own (correct) placeholder blocks as layout mistakes.
		prevIn, prevBraced := b.inLayout, b.layoutBracedPlaceholders
		b.inLayout, b.layoutBracedPlaceholders = true, nil
		stmt.Widgets = buildPageBodyV3(bodyCtx, b)
		stmt.BracedPlaceholders = b.layoutBracedPlaceholders
		b.inLayout, b.layoutBracedPlaceholders = prevIn, prevBraced
	}
	return stmt
}

// buildObjectEntryListV3 collects `[(k: v, …), …]` verbatim. The values are kept
// so the diagnostic can name the first key the author wrote instead of
// describing the shape abstractly.
func buildObjectEntryListV3(ctx parser.IObjectEntryListV3Context) *ast.ObjectEntryListV3 {
	c, ok := ctx.(*parser.ObjectEntryListV3Context)
	if !ok {
		return nil
	}
	out := &ast.ObjectEntryListV3{}
	for _, entryCtx := range c.AllObjectEntryV3() {
		e, ok := entryCtx.(*parser.ObjectEntryV3Context)
		if !ok {
			continue
		}
		entry := map[string]any{}
		for _, fieldCtx := range e.AllObjectEntryFieldV3() {
			f, ok := fieldCtx.(*parser.ObjectEntryFieldV3Context)
			if !ok {
				continue
			}
			key := ""
			if k := f.IdentifierOrKeyword(); k != nil {
				key = k.GetText()
			}
			if key == "" {
				continue
			}
			entry[key] = buildPropertyValueV3(f.PropertyValueV3())
		}
		out.Entries = append(out.Entries, entry)
	}
	return out
}

// buildWidgetIconV3 reads a widget's `Icon:` value onto a typed WidgetIcon.
//
// Mendix stores three different icon ELEMENTS, not three spellings of one value,
// so the KIND is recorded and the writer emits the matching $Type. Collapsing
// them onto one string is what made describe -> exec rewrite an image icon as a
// custom-icon reference (CE1613) and delete a glyph icon outright
// (mendixlabs/mxcli#1059).
//
// The bare form is the collection icon, which keeps every existing script
// meaning exactly what it did. Both spellings of a name are accepted — quoted,
// as `Icon:` has taken since #602, and bare, as every other reference into the
// model is written.
func buildWidgetIconV3(ctx parser.IWidgetIconV3Context) *ast.WidgetIcon {
	if ctx == nil {
		return nil
	}
	c, ok := ctx.(*parser.WidgetIconV3Context)
	if !ok {
		return nil
	}
	icon := &ast.WidgetIcon{}
	switch {
	case c.GLYPH() != nil:
		icon.Kind = types.MenuIconGlyph
		if n := c.NUMBER_LITERAL(); n != nil {
			// A glyph code is a character code: whole, and small. A fractional or
			// unparseable literal leaves the code at zero rather than guessing,
			// and the builder refuses to write a glyph without one.
			if v, err := strconv.Atoi(n.GetText()); err == nil {
				icon.Code = v
			}
		}
	case c.IMAGE() != nil:
		icon.Kind = types.MenuIconImage
		icon.Name = widgetIconName(c)
	default:
		icon.Kind = types.MenuIconCollection
		icon.Name = widgetIconName(c)
	}
	return icon
}

// widgetIconName reads the qualified name out of whichever spelling was used.
func widgetIconName(c *parser.WidgetIconV3Context) string {
	if qn := c.QualifiedName(); qn != nil {
		return buildQualifiedName(qn).String()
	}
	if str := c.STRING_LITERAL(); str != nil {
		return unquoteString(str.GetText())
	}
	return ""
}
