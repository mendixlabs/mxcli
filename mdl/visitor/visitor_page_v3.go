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

	// `folder '…'` after the name (R9); the header's `Folder:` is its alias.
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
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
	page, _ := headerCtx.GetParent().(*parser.CreatePageStatementContext)

	for i, propCtx := range headerCtx.AllPageHeaderPropertyV3() {
		prop := propCtx.(*parser.PageHeaderPropertyV3Context)

		if prop.PARAMS() != nil {
			// Params: ( $Order: Entity, ... )
			if paramList := prop.PageParameterList(); paramList != nil {
				stmt.Parameters = buildPageParameters(paramList)
			}
		} else if prop.VARIABLES_KW() != nil {
			// Variables: ( $showStock: Boolean = true, ... )
			if varList := prop.VariableDeclarationList(); varList != nil {
				stmt.Variables = buildVariableDeclarations(varList)
			}
		} else if prop.TITLE() != nil {
			// Title: 'My Page'
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.Title = unquoteStringLit(str)
			}
		} else if prop.LAYOUT() != nil {
			// Layout: Atlas_Core.Atlas_Default or 'Layout Name'
			if qn := prop.QualifiedName(); qn != nil {
				stmt.Layout = getQualifiedNameText(qn)
			} else if str := prop.STRING_LITERAL(); str != nil {
				stmt.Layout = unquoteStringLit(str)
			}
		} else if prop.URL() != nil {
			// Url: 'my-page'
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.URL = unquoteStringLit(str)
			}
		} else if prop.FOLDER() != nil {
			// Folder: 'Pages/Admin' — the alias of `folder '…'` after the name
			// (R9, MDL-DEPR105). The clause wins when both are written.
			if str := prop.STRING_LITERAL(); str != nil && page != nil {
				clause := page.STRING_LITERAL() != nil
				if !clause {
					stmt.Folder = unquoteStringLit(str)
				}
				b.recordFolderProperty(page.QualifiedName(), ruleContexts(headerCtx.AllPageHeaderPropertyV3()), i,
					unquoteStringLit(str), clause, nil)
			}
		} else if prop.CLASS() != nil {
			// Class: 'my-page' — page-level CSS class (issue #714)
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.Class = unquoteStringLit(str)
			}
		} else if prop.STYLE() != nil {
			// Style: 'padding: 10px' — page-level inline CSS (issue #714)
			if str := prop.STRING_LITERAL(); str != nil {
				stmt.Style = unquoteStringLit(str)
			}
		} else if dp := prop.DesignPropertyListV3(); dp != nil {
			// DesignProperties: ( … ) — the page's own (ako/mxcli#1678)
			stmt.DesignProperties = buildDesignPropertyListV3(dp)
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
	case "PopupCloseAction":
		// The value is a widget NAME on this page, so it arrives as a bare
		// identifier rather than a quoted string; accept either spelling.
		stmt.PopupCloseAction = strings.Trim(fmt.Sprintf("%v", val), `"'`)
	default:
		b.addError(fmt.Errorf("line %d:%d: unknown page property %q "+
			"(supported: Title, Layout, Url, Folder, Params, Variables, PopupWidth, PopupHeight, PopupResizable, PopupCloseAction, Class, Style, DesignProperties)",
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

	// `folder '…'` after the name (R9); the header's `Folder:` is its alias.
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	// Parse V3 header
	if headerCtx := ctx.SnippetHeaderV3(); headerCtx != nil {
		b.parseSnippetHeaderV3(headerCtx, stmt)
	}

	// Parse options (FOLDER)
	if opts := ctx.SnippetOptions(); opts != nil {
		optsCtx := opts.(*parser.SnippetOptionsContext)
		all := optsCtx.AllSnippetOption()
		for _, opt := range all {
			optCtx := opt.(*parser.SnippetOptionContext)
			if optCtx.FOLDER() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Folder = unquoteStringLit(optCtx.STRING_LITERAL())
				b.recordFolderClausePosition(ctx.QualifiedName(), optCtx.FOLDER().GetSymbol(),
					optCtx.STRING_LITERAL().GetSymbol(), ctx.STRING_LITERAL() != nil || len(all) > 1 || snippetHeaderHasFolder(ctx))
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
	snippet, _ := headerCtx.GetParent().(*parser.CreateSnippetStatementContext)

	for i, propCtx := range headerCtx.AllSnippetHeaderPropertyV3() {
		prop := propCtx.(*parser.SnippetHeaderPropertyV3Context)

		if prop.PARAMS() != nil {
			// Params: ( $Customer: Entity, ... )
			if paramList := prop.PageParameterList(); paramList != nil {
				stmt.Parameters = buildPageParameters(paramList)
			}
		} else if prop.VARIABLES_KW() != nil {
			// Variables: ( $showStock: Boolean = true, ... )
			if varList := prop.VariableDeclarationList(); varList != nil {
				stmt.Variables = buildVariableDeclarations(varList)
			}
		} else if prop.FOLDER() != nil {
			// Folder: 'Snippets/Common' — the alias of `folder '…'` after the
			// name (R9, MDL-DEPR105). The clause wins when both are written.
			if str := prop.STRING_LITERAL(); str != nil && snippet != nil {
				clause := snippet.STRING_LITERAL() != nil
				if !clause {
					stmt.Folder = unquoteStringLit(str)
				}
				// The snippet header is optional, so an only `Folder:` goes with it.
				b.recordFolderProperty(snippet.QualifiedName(), ruleContexts(headerCtx.AllSnippetHeaderPropertyV3()), i,
					unquoteStringLit(str), clause, headerCtx)
			}
		}
	}
}

// A snippet's Params clause is the page's pageParameterList rule (they were
// byte-identical), so one conversion serves both. buildSnippetParameterListAsPage
// used to live here and drifted from buildPageParameters twice out of the same
// clause: it re-split the parse node's TEXT, so a quoted entity name reached
// the resolver as `Pd."Thing"`, and it never called buildDataType, so a
// primitive type was left for the executor to take for an entity name
// (mendixlabs/mxcli#1028 — "entity not found: string").

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

	// The default is an expression: bare, or — the old spelling, MDL-DEPR086
	// — a string whose content is the expression (visitor_variable_default.go).
	if str := vdCtx.STRING_LITERAL(); str != nil {
		v.DefaultValue = unquoteStringLit(str)
	} else if e := vdCtx.Expression(); e != nil {
		v.DefaultValue = bareArgumentText(e)
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
		widget.Properties["WidgetType"] = unquoteStringLit(wCtx.STRING_LITERAL())
	} else if wCtx.CUSTOMWIDGET() != nil {
		widget.Type = "customwidget"
		widget.Properties["WidgetType"] = unquoteStringLit(wCtx.STRING_LITERAL())
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
		b.rememberWidgetName(widget, wCtx, id.GetSymbol())
	} else if qid := wCtx.QUOTED_IDENTIFIER(); qid != nil {
		widget.Name = unquoteIdentifier(qid.GetText())
		b.rememberWidgetName(widget, wCtx, qid.GetSymbol())
	} else if kw := wCtx.Keyword(); kw != nil {
		widget.Name = kw.GetText()
		b.rememberWidgetName(widget, wCtx, kw.GetStop())
	}

	// Parse properties
	if propsCtx := wCtx.WidgetPropertiesV3(); propsCtx != nil {
		parseWidgetPropertiesV3(propsCtx, widget, b)
	}

	// Parse children
	if bodyCtx := wCtx.WidgetBodyV3(); bodyCtx != nil {
		widget.Children = buildWidgetBodyV3(bodyCtx, b)
	}
	b.dropUnstoredChildNames(widget)

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
		} else if refCtx := propCtx.WidgetAttributeRefV3(); refCtx != nil {
			// `$dataView1.Name` / `$Param.Name`: kept as written; the builder
			// resolves the name to the data view it reads through (ako/mxcli#826)
			// or the page/snippet parameter it reads. Only the input builders
			// and the combo box read it — any other widget resolved the `$…`
			// string to no attribute and was written without one, so it stays
			// refused there. `$Param.Module.Assoc` is a combo box's association.
			ref := buildWidgetAttributeRefV3(refCtx)
			kind := strings.ToLower(widget.Type)
			_, attr, _ := strings.Cut(ref, ".")
			if !widgetAttributeRefKinds[kind] {
				b.addError(fmt.Errorf("%s `%s`: `Attribute: %s` — reading an attribute from a named object "+
					"is supported on textbox, textarea, checkbox, datepicker, radiobuttons, dropdown and combobox; "+
					"bind the attribute by name inside a data view instead (`Attribute: %s`)",
					widget.Type, widget.Name, refCtx.GetText(), attr))
				return
			}
			if strings.Contains(attr, ".") && kind != "combobox" {
				b.addError(fmt.Errorf("%s `%s`: `Attribute: %s` — name one attribute after the object (`$name.Attr`); "+
					"a qualified `$name.Module.Association` is a combo box's association",
					widget.Type, widget.Name, refCtx.GetText()))
				return
			}
			widget.Properties["Attribute"] = ref
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
			widget.Properties["Placeholder"] = unquoteStringLit(str)
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
			widget.Properties["Label"] = unquoteStringLit(str)
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

	// ContentParams: (...)
	if propCtx.CONTENTPARAMS() != nil {
		if plCtx := propCtx.ParamListV3(); plCtx != nil {
			widget.Properties["ContentParams"] = buildParamListV3(plCtx)
		}
		return
	}

	// CaptionParams: (...)
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
			widget.Properties["Class"] = unquoteStringLit(str)
		}
		return
	}

	// Style: ...
	if propCtx.STYLE() != nil {
		if str := propCtx.STRING_LITERAL(); str != nil {
			widget.Properties["Style"] = unquoteStringLit(str)
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

	// Params: (Asset = $var) — snippet call arguments
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

	// DesignProperties: (...)
	if propCtx.DESIGNPROPERTIES() != nil {
		if dpCtx := propCtx.DesignPropertyListV3(); dpCtx != nil {
			widget.Properties["DesignProperties"] = buildDesignPropertyListV3(dpCtx)
		}
		return
	}

	// Visible: [expression] (conditional visibility) or Visible: false (static)
	if propCtx.VISIBLE() != nil {
		// `Visible: roles (Mod.A, …)` — Studio Pro's "Visible for module roles",
		// kept under its own key so it combines with the condition forms below
		// (mendixlabs/mxcli#1460).
		if rl := propCtx.VisibleRoleListV3(); rl != nil {
			var roles []string
			for _, qn := range rl.AllQualifiedName() {
				roles = append(roles, getQualifiedNameText(qn))
			}
			widget.Properties["VisibleRoles"] = roles
			return
		}
		// `Visible: Attr in (v1, …)` — Studio Pro's "based on attribute value".
		if propCtx.IN() != nil {
			vw := &ast.VisibleWhenV3{}
			if refCtx := propCtx.WidgetAttributeRefV3(); refCtx != nil {
				// `$Param.Attr in (…)`: read from a page or snippet parameter.
				vw.Attribute = buildWidgetAttributeRefV3(refCtx)
			} else {
				vw.Attribute = getQualifiedNameText(propCtx.QualifiedName())
			}
			for _, v := range propCtx.AllVisibleValueV3() {
				vw.Values = append(vw.Values, unquoteIdentifier(v.GetText()))
			}
			widget.Properties["VisibleWhen"] = vw
			return
		}
		if xc := propCtx.XpathConstraint(); xc != nil {
			widget.Properties["VisibleIf"] = buildConditionalExpression(xc) // MDL-DEPR081
		} else if e := propCtx.Expression(); e != nil {
			widget.Properties["VisibleIf"] = bareArgumentText(e)
		} else if valCtx := propCtx.PropertyValueV3(); valCtx != nil {
			widget.Properties["Visible"] = buildPropertyValueV3(valCtx)
		}
		return
	}

	// Editable: [expression] (conditional editability) or Editable: Never (static)
	if propCtx.EDITABLE() != nil {
		// `Editable: Attr in (v1, …)` — Studio Pro's "based on attribute
		// value", read as `Visible: Attr in (…)` is (ako/mxcli#2111).
		if propCtx.IN() != nil {
			ew := &ast.VisibleWhenV3{}
			if refCtx := propCtx.WidgetAttributeRefV3(); refCtx != nil {
				ew.Attribute = buildWidgetAttributeRefV3(refCtx)
			} else {
				ew.Attribute = getQualifiedNameText(propCtx.QualifiedName())
			}
			for _, v := range propCtx.AllVisibleValueV3() {
				ew.Values = append(ew.Values, unquoteIdentifier(v.GetText()))
			}
			widget.Properties["EditableWhen"] = ew
			return
		}
		if xc := propCtx.XpathConstraint(); xc != nil {
			widget.Properties["EditableIf"] = buildConditionalExpression(xc) // MDL-DEPR081
		} else if e := propCtx.Expression(); e != nil {
			widget.Properties["EditableIf"] = bareArgumentText(e)
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
		// An expression-typed property takes the expression as written, from
		// whichever value alternative matched (visitor_widget_expression.go).
		if isWidgetExpressionProp(id.GetText()) {
			if v := lastRuleChild(propCtx); v != nil {
				widget.Properties[id.GetText()] = b.widgetExpressionPropValue(id.GetText(), v)
			}
			return
		}
		// Any other generic key: kept as an expression value. Whether the
		// property takes one is the widget schema's question, answered by the
		// checker (MDL-WIDGET42) and the builder, never by writing it empty.
		if expr := propCtx.Expression(); expr != nil {
			widget.Properties[id.GetText()] = genericWidgetExpression(expr)
			return
		}
		// `<Name>Params: ({1} = Attr)` — the parameters of a text-template
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
			keepExpressionSource(widget, id.GetText(), dsCtx)
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
		if isWidgetExpressionProp(kw.GetText()) {
			if v := lastRuleChild(propCtx); v != nil {
				widget.Properties[kw.GetText()] = b.widgetExpressionPropValue(kw.GetText(), v)
			}
			return
		}
		// Any other generic key: kept as an expression value. Whether the
		// property takes one is the widget schema's question, answered by the
		// checker (MDL-WIDGET42) and the builder, never by writing it empty.
		if expr := propCtx.Expression(); expr != nil {
			widget.Properties[kw.GetText()] = genericWidgetExpression(expr)
			return
		}
		if plCtx := propCtx.ParamListV3(); plCtx != nil {
			widget.Properties[kw.GetText()] = buildParamListV3(plCtx)
			return
		}
		if dsCtx := propCtx.DataSourceExprV3(); dsCtx != nil {
			widget.Properties[kw.GetText()] = buildDataSourceV3(dsCtx)
			keepExpressionSource(widget, kw.GetText(), dsCtx)
			return
		}
		// A named action slot whose key is an MDL keyword — Switch stores its
		// slot as `action`, PopupMenu's menu items as `action` too (#956).
		if actCtx := propCtx.ActionExprV3(); actCtx != nil {
			widget.Properties[kw.GetText()] = buildActionV3(actCtx)
			return
		}
		if valCtx := propCtx.PropertyValueV3(); valCtx != nil {
			if ds := bareEntityDataSource(kw.GetText(), widget.Type, valCtx); ds != nil {
				widget.Properties["DataSource"] = ds
				return
			}
			widget.Properties[kw.GetText()] = buildPropertyValueV3(valCtx)
		}
		return
	}
}

// bareEntityWidgets are the widget keywords on which `DataSource:` can only mean
// a data source. A pluggable widget is deliberately absent: its generic keys are
// its own .mpk property keys, and two of them reuse the name — the Barcode
// Scanner's `datasource: Module.Entity.Attr` binds an ATTRIBUTE, and Image's
// `datasource` is its image-type enumeration.
var bareEntityWidgets = map[string]bool{
	"datagrid": true, "listview": true, "gallery": true, "dataview": true,
}

// bareEntityDataSource resolves the `DataSource: Module.Entity` shorthand to
// `DataSource: DATABASE Module.Entity`, or returns nil when the property is not
// that shorthand.
//
// No dataSourceExprV3 alternative starts with a bare name, so the shorthand fell
// through to the generic `keyword: propertyValueV3` branch and was stored as a
// plain string that nothing reads — GetDataSource() only sees *DataSourceV3. The
// grid was written unbound: check passed, exec reported "Created page", and
// mxbuild answered CE0488 (ako/mxcli#576). It is resolved here rather than as a
// grammar alternative because only the widget type says what the key means.
// A data view gets the DATABASE source too, and MDL-WIDGET09 refuses it by name.
func bareEntityDataSource(key, widgetType string, valCtx parser.IPropertyValueV3Context) *ast.DataSourceV3 {
	if !strings.EqualFold(key, "DataSource") || !bareEntityWidgets[widgetType] {
		return nil
	}
	pv, ok := valCtx.(*parser.PropertyValueV3Context)
	if !ok {
		return nil
	}
	qn := pv.QualifiedName()
	if qn == nil {
		return nil
	}
	return &ast.DataSourceV3{Type: "database", Reference: getQualifiedNameText(qn)}
}

// buildDataSourceV3 builds a DataSource from the parse context.
func buildDataSourceV3(ctx parser.IDataSourceExprV3Context) *ast.DataSourceV3 {
	if ctx == nil {
		return nil
	}
	dsCtx := ctx.(*parser.DataSourceExprV3Context)
	ds := &ast.DataSourceV3{}

	if dsCtx.DATABASE() != nil {
		buildDatabaseSourceV3(dsCtx, ds)
	} else if v := dsCtx.VARIABLE(); v != nil && dsCtx.SLASH() != nil {
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
	} else if dsCtx.MICROFLOW() != nil {
		// MICROFLOW Module.Flow
		ds.Type = "microflow"
		if qn := dsCtx.QualifiedName(); qn != nil {
			ds.Reference = getQualifiedNameText(qn)
		}
		if argsCtx := dsCtx.MicroflowArgsV3(); argsCtx != nil {
			ds.Args = buildMicroflowArgsV3(argsCtx)
		}
		if sc := dsCtx.ActionSettingsV3(); sc != nil {
			if s := buildActionSettingsV3(sc); s != nil {
				ds.FormValidations = s.FormValidations
			}
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

// buildDatabaseSourceV3 fills a DATABASE source: its entity, or — for
// `database from $ctx/Assoc/Entity` — the context variable and association path
// it is reached over (ako/mxcli#721 L5), then the WHERE, SORT BY and SEARCH BY
// clauses both forms share.
func buildDatabaseSourceV3(dsCtx *parser.DataSourceExprV3Context, ds *ast.DataSourceV3) {
	ds.Type = "database"
	if v := dsCtx.VARIABLE(); v != nil {
		ds.ContextVariable = strings.TrimPrefix(v.GetText(), "$")
		if pathCtx := dsCtx.AssociationPathV3(); pathCtx != nil {
			ds.AssociationPath = buildAssociationPathV3(pathCtx)
		}
	} else if qn := dsCtx.QualifiedName(); qn != nil {
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

	// Inline SEARCH BY clause — a List View's search bar
	// (Forms$ListViewSearch.SearchRefs). Names only, no direction.
	if dsCtx.SEARCH_BY() != nil {
		for _, sa := range dsCtx.AllSearchAttribute() {
			if name := strings.TrimSpace(sa.GetText()); name != "" {
				ds.SearchAttributes = append(ds.SearchAttributes, name)
			}
		}
	}
}

// buildActionV3 builds an Action from the parse context.
func buildActionV3(ctx parser.IActionExprV3Context) *ast.ActionV3 {
	if ctx == nil {
		return nil
	}
	actCtx := ctx.(*parser.ActionExprV3Context)
	action := &ast.ActionV3{}

	// Each page action has a canonical spelling in words and a deprecated
	// snake-case one (R8, ako/mxcli#752); both build the same action, and
	// ExitActionExprV3 reports the old one.
	closePage := actCtx.ClosePageV3() != nil
	isDelete := actCtx.DELETE_OBJECT() != nil || actCtx.DELETE() != nil
	if v := actCtx.VARIABLE(); v != nil && actCtx.OpenLinkV3() == nil && !isDelete {
		// $handler — a fragment action parameter; resolved at expansion.
		// (open link $currentObject/Attr also carries a VARIABLE.)
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
		action.ClosePage = closePage
	} else if actCtx.CANCEL_CHANGES() != nil {
		action.Type = "cancel"
		action.ClosePage = closePage
	} else if isDelete {
		action.Type = "delete"
		action.ClosePage = closePage
		if v := actCtx.VARIABLE(); v != nil {
			action.SourceVariable = strings.TrimPrefix(v.GetText(), "$")
		}
	} else if closePage {
		action.Type = "close"
	} else if actCtx.CREATE_OBJECT() != nil || actCtx.CREATE() != nil {
		action.Type = "create"
		if qn := actCtx.QualifiedName(); qn != nil {
			action.Target = getQualifiedNameText(qn)
		}
		// Check for THEN action
		if thenCtx := actCtx.ActionExprV3(); thenCtx != nil {
			action.ThenAction = buildActionV3(thenCtx)
		}
	} else if actCtx.SHOW_PAGE() != nil || actCtx.SHOW() != nil {
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
	} else if actCtx.OpenLinkV3() != nil {
		action.Type = "openLink"
		if str := actCtx.STRING_LITERAL(); str != nil {
			action.LinkURL = unquoteStringLit(str)
		}
		// A dynamic address: `open link $currentObject/URL`.
		if v := actCtx.VARIABLE(); v != nil {
			action.LinkVariable = v.GetText()
			if pathCtx := actCtx.AttributePathV3(); pathCtx != nil {
				action.LinkAttribute = buildAttributePathV3(pathCtx)
			}
		}
	} else if actCtx.SIGN_OUT() != nil {
		action.Type = "signOut"
	} else if actCtx.COMPLETE_TASK() != nil {
		action.Type = "completeTask"
		if str := actCtx.STRING_LITERAL(); str != nil {
			action.OutcomeValue = unquoteStringLit(str)
		}
	} else if actCtx.OPEN() != nil && actCtx.USER() != nil {
		action.Type = "openUserTask"
	} else if actCtx.OPEN() != nil && actCtx.WORKFLOW() != nil {
		action.Type = "openWorkflow"
	} else if actCtx.CALL() != nil && actCtx.WORKFLOW() != nil {
		action.Type = "callWorkflow"
		if qn := actCtx.QualifiedName(); qn != nil {
			action.Target = getQualifiedNameText(qn)
		}
	}
	if rc := actCtx.ActionReturnsV3(); rc != nil {
		action.Returns = buildActionReturnsV3(rc)
	}
	if sc := actCtx.ActionSettingsV3(); sc != nil {
		action.Settings = buildActionSettingsV3(sc)
	}
	// `create object E then show page P with (…)`: the settings belong to the
	// one stored action, the create — the page it opens is part of it.
	if action.Type == "create" && action.ThenAction != nil && action.ThenAction.Settings != nil {
		action.Settings = action.ThenAction.Settings
		action.ThenAction.Settings = nil
	}

	return action
}

// buildActionReturnsV3 builds a flow call's `returns ( … )` list.
func buildActionReturnsV3(ctx parser.IActionReturnsV3Context) []ast.FlowReturnV3 {
	var out []ast.FlowReturnV3
	for _, rc := range ctx.(*parser.ActionReturnsV3Context).AllActionReturnV3() {
		r := rc.(*parser.ActionReturnV3Context)
		var ret ast.FlowReturnV3
		if v := r.VARIABLE(); v != nil {
			ret.Target = strings.TrimPrefix(v.GetText(), "$")
			ret.Variable = true
		} else if pn := r.ParameterName(); pn != nil {
			ret.Target = parameterNameText(pn)
		}
		if expr := r.Expression(); expr != nil {
			ret.Source = expressionSourceText(expr)
		}
		out = append(out, ret)
	}
	return out
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

	if pn := argCtx.ParameterName(); pn != nil {
		// Canonical (R4): Param = $value
		arg.Name = parameterNameText(pn)
	} else if v := argCtx.VARIABLE(); v != nil {
		// Deprecated (MDL-DEPR006): $Param = $value
		arg.Name = strings.TrimPrefix(v.GetText(), "$")
	} else if iok := argCtx.IdentifierOrKeyword(); iok != nil {
		// Deprecated (MDL-DEPR007): Param: $value. identifierOrKeyword accepts a bare
		// keyword (View/Source/Item/Page/Entity) or a "quoted" name;
		// identifierOrKeywordText unquotes as needed.
		arg.Name = identifierOrKeywordText(iok)
	}
	if expr := argCtx.Expression(); expr != nil {
		arg.Value = expressionSourceText(expr)
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

// widgetAttributeRefKinds are the widgets whose builder resolves `Attribute:
// $name.Attr`: the inputs (resolveInputBinding — an enclosing data view,
// ako/mxcli#826, or a parameter) and the combo box (the pluggable engine — a
// parameter only).
var widgetAttributeRefKinds = map[string]bool{
	"textbox": true, "textarea": true, "checkbox": true,
	"datepicker": true, "radiobuttons": true, "dropdown": true,
	"combobox": true,
}

// buildWidgetAttributeRefV3 renders `$name.Attr` (or `$name.Module.Assoc`)
// with quoted segments unquoted, the form the builders read.
func buildWidgetAttributeRefV3(ctx parser.IWidgetAttributeRefV3Context) string {
	c, ok := ctx.(*parser.WidgetAttributeRefV3Context)
	if !ok || c.VARIABLE() == nil {
		return ""
	}
	text := c.GetText()
	_, rest, _ := strings.Cut(text, ".")
	segs := strings.Split(rest, ".")
	for i, seg := range segs {
		segs[i] = unquoteIdentifier(seg)
	}
	return c.VARIABLE().GetText() + "." + strings.Join(segs, ".")
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
		// A hop may be module-qualified (`"Mod"."Assoc"/Attr`), so each
		// segment is unquoted per dotted part, not as one identifier.
		parts := strings.Split(text, "/")
		for i, p := range parts {
			parts[i] = unquoteQualifiedName(p)
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
		return unquoteStringLit(str)
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
		param.Value = stripExpressionIdentifierQuotes(expressionSourceText(expr))
	}
	// `{1} = <unbound>`: no value; the builder writes the parameter with every
	// binding slot empty, as Studio Pro stores it (ako/mxcli#2181).
	if paCtx.UNBOUND_PARAM() != nil {
		param.Unbound = true
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
		// A string value is unescaped like any other literal: a date pattern
		// quotes literal text (`h ''at'' mm` is `h 'at' mm`), and stripping the
		// outer quotes alone stored the doubled ones (ako/mxcli#977).
		if pv, ok := vc.(*parser.PropertyValueV3Context); ok && pv.STRING_LITERAL() != nil {
			val = unquoteStringLit(pv.STRING_LITERAL())
		}
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

	// Several qualifiedNames mean an association path: every segment but the last
	// is a hop, the last is the attribute (mendixlabs/mxcli#1152).
	if qns := scCtx.AllQualifiedName(); len(qns) > 0 {
		item.Associations = sortColumnHops(qns)
		item.Attribute = getQualifiedNameText(qns[len(qns)-1])
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

// parseWidthValue parses a column width value: numeric (1-12), "AutoFill" or
// "AutoFit" (auto-fit content).
func parseWidthValue(text string) any {
	if strings.EqualFold(text, "AutoFill") {
		return "AutoFill"
	}
	if strings.EqualFold(text, "AutoFit") {
		return "AutoFit"
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
		return unquoteStringLit(str)
	}
	// "AttrName" — a double-quoted value, used for pluggable-widget attribute
	// sub-properties like chart series `staticXAttribute: "StatusValue"`. Strip
	// the quotes; the executor resolves it against the item's datasource entity.
	if q := pvCtx.QUOTED_IDENTIFIER(); q != nil {
		return unquoteIdentifier(q.GetText())
	}
	if num := pvCtx.NUMBER_LITERAL(); num != nil {
		text := num.GetText()
		// A signed literal (`DecimalPrecision: -1`) is a plain value, not an
		// expression: without this alternative it fell through to the generic
		// expression form and was refused as one (MDL-WIDGET42, #1407).
		if pvCtx.MINUS() != nil {
			text = "-" + text
		}
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

	key := unquoteStringLit(allStrings[0])

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
		return &ast.DesignPropertyEntryV3{Key: key, Value: unquoteStringLit(allStrings[1])}
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
	case *ast.ConstantRefExpr:
		// Without a case the constant rendered as "": a data source written
		// `where Total > @Shop.C_Limit` stored `[Total > ]` (ako/mxcli#1685).
		return "@" + e.QualifiedName.String()
	case *ast.ParenExpr:
		return "(" + xpathExprToString(e.Inner) + ")"
	case *ast.IdentifierExpr:
		return e.Name
	case *ast.QualifiedNameExpr:
		// Written as it is. A three-part name is either an attribute of the
		// constrained entity (stored bare) or an enumeration value (stored as a
		// string literal), and only the writer knows which entity the
		// constraint is on — so the executor decides (storedXPathConstraint).
		// Deciding here made `M.Emp.Name` the literal 'Name', in page
		// datasources and in every constraint this formatter re-laid out
		// (ako/mxcli#874).
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
		for _, pred := range step.Predicates {
			s += "[" + xpathExprToString(pred) + "]"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "/")
}

// buildSnippetCallParamListV3 converts a parsed snippetCallParamListV3 context
// into a slice of SnippetCallParam AST nodes.
//
// The canonical form binds `Param = $var` (R4) in ( ); the old brace map
// `{$Param: $var}` / `{Param: $var}` builds the same params. The parameter name
// is stored without its `$`, which the builder strips either way.
func buildSnippetCallParamListV3(ctx parser.ISnippetCallParamListV3Context) []ast.SnippetCallParam {
	var params []ast.SnippetCallParam
	for _, argCtx := range ctx.AllSnippetCallArgV3() {
		param := ast.SnippetCallParam{ParamName: parameterNameText(argCtx.ParameterName())}
		if v := argCtx.VARIABLE(); v != nil {
			param.Variable = v.GetText()
		}
		if param.ParamName != "" && param.Variable != "" {
			params = append(params, param)
		}
	}
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
				param.ParamName = strings.TrimPrefix(vars[0].GetText(), "$")
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
		stmt.Placeholders = buildPagePlaceholdersV3(bodyCtx, b)
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
		return unquoteStringLit(str)
	}
	return ""
}
