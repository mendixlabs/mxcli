// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateJavaActionStatement handles CREATE JAVA ACTION statements.
func (b *Builder) ExitCreateJavaActionStatement(ctx *parser.CreateJavaActionStatementContext) {
	stmt := &ast.CreateJavaActionStmt{}

	// Get qualified name
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Name = buildQualifiedName(qn)
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	// Get parameters
	stmt.Parameters = buildJavaActionParams(ctx.JavaActionParameterList())

	// Type parameters: those the `Name<T, U>` header declares, in its order,
	// then any ENTITY <pEntity> parameter declares that the header did not
	// (mendixlabs/mxcli#1404).
	declare := func(name string) {
		for _, existing := range stmt.TypeParameters {
			if existing == name {
				return
			}
		}
		stmt.TypeParameters = append(stmt.TypeParameters, name)
	}
	if tps := ctx.JavaActionTypeParameters(); tps != nil {
		for _, id := range tps.AllIdentifierOrKeyword() {
			if name := identifierOrKeywordText(id); name != "" {
				declare(name)
			}
		}
	}
	for _, param := range stmt.Parameters {
		if param.Type.Kind == ast.TypeEntityTypeParam && param.Type.TypeParamName != "" {
			declare(param.Type.TypeParamName)
		}
	}

	// Get return type
	stmt.ReturnType, stmt.ReturnName = buildJavaActionReturn(ctx.JavaActionReturnType())

	// Get exposed clause (EXPOSED AS 'caption' IN 'category')
	if exposed := ctx.JavaActionExposedClause(); exposed != nil {
		allStrings := exposed.AllSTRING_LITERAL()
		switch {
		case exposed.NOT() != nil:
			// NOT EXPOSED removes the toolbox entry. An absent clause preserves
			// it, so removal has to be said out loud.
			stmt.NotExposed = true
		case len(allStrings) >= 2:
			stmt.ExposedCaption = unquoteStringLit(allStrings[0])
			stmt.ExposedCategory = unquoteStringLit(allStrings[1])
			stmt.ExposedBitmaps = buildExposeBitmaps(exposed.AllExposeBitmapClause())
		}
	}

	// Get Java code from the dollar-quoted (or string) body
	if body := ctx.SourceCodeText(); body != nil {
		// Trim leading/trailing whitespace but preserve internal formatting
		code := strings.TrimSpace(sourceCodeTextValue(body))
		// Extract import lines so they go into the file-level import section,
		// not into the executeAction() method body (a common AI agent mistake).
		stmt.JavaCode, stmt.Imports = extractJavaImports(code)
	}

	// `extra code $$ … $$`: the file's EXTRA CODE section (ako/mxcli#1702).
	// Kept as written apart from the blank lines around it — describe prints
	// the section as the file holds it, so trimming indentation would shift
	// every round trip. An import line goes to the import list, as in the body.
	if ec := ctx.JavaActionExtraCode(); ec != nil && ec.SourceCodeText() != nil {
		var imports []string
		stmt.ExtraCode, imports = extractJavaExtraCode(sourceCodeTextValue(ec.SourceCodeText()))
		stmt.Imports = append(stmt.Imports, imports...)
		stmt.ExtraCodeSet = true
	}

	// Check for documentation comment and OR MODIFY/REPLACE from parent createStatement
	if parent, ok := ctx.GetParent().(*parser.CreateStatementContext); ok {
		if docComment := parent.DocComment(); docComment != nil {
			stmt.Documentation, stmt.DocumentationSet = extractDocComment(docComment.GetText()), true
		}
		if parent.OR() != nil && (parent.MODIFY() != nil || parent.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	// Also check for doc comment at statement level (grammar allows it at both levels)
	if stmt.Documentation == "" {
		if stmtCtx := findParentStatement(ctx); stmtCtx != nil {
			if docCtx := stmtCtx.DocComment(); docCtx != nil {
				stmt.Documentation, stmt.DocumentationSet = extractDocComment(docCtx.GetText()), true
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitCreateJavaScriptActionStatement handles CREATE JAVASCRIPT ACTION statements.
// It mirrors the Java action handler, adding the optional `platform` clause
// (default Web) and treating the $$ body as JavaScript source.
func (b *Builder) ExitCreateJavaScriptActionStatement(ctx *parser.CreateJavaScriptActionStatementContext) {
	stmt := &ast.CreateJavaScriptActionStmt{Platform: "Web"}

	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Name = buildQualifiedName(qn)
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	stmt.Parameters = buildJavaActionParams(ctx.JavaActionParameterList())

	// Extract type parameters from ENTITY <pEntity> parameter declarations.
	for _, param := range stmt.Parameters {
		if param.Type.Kind == ast.TypeEntityTypeParam && param.Type.TypeParamName != "" {
			found := false
			for _, existing := range stmt.TypeParameters {
				if existing == param.Type.TypeParamName {
					found = true
					break
				}
			}
			if !found {
				stmt.TypeParameters = append(stmt.TypeParameters, param.Type.TypeParamName)
			}
		}
	}

	stmt.ReturnType, stmt.ReturnName = buildJavaActionReturn(ctx.JavaActionReturnType())

	if exposed := ctx.JavaActionExposedClause(); exposed != nil {
		allStrings := exposed.AllSTRING_LITERAL()
		switch {
		case exposed.NOT() != nil:
			// NOT EXPOSED removes the toolbox entry. An absent clause preserves
			// it, so removal has to be said out loud.
			stmt.NotExposed = true
		case len(allStrings) >= 2:
			stmt.ExposedCaption = unquoteStringLit(allStrings[0])
			stmt.ExposedCategory = unquoteStringLit(allStrings[1])
			stmt.ExposedBitmaps = buildExposeBitmaps(exposed.AllExposeBitmapClause())
		}
	}

	if plat := ctx.JavaScriptPlatformClause(); plat != nil {
		if id := plat.IdentifierOrKeyword(); id != nil {
			stmt.Platform = canonicalJavaScriptPlatform(unquoteIdentifier(id.GetText()))
		}
	}

	if body := ctx.SourceCodeText(); body != nil {
		stmt.JavaScriptCode = strings.TrimSpace(sourceCodeTextValue(body))
	}

	if parent, ok := ctx.GetParent().(*parser.CreateStatementContext); ok {
		if docComment := parent.DocComment(); docComment != nil {
			stmt.Documentation, stmt.DocumentationSet = extractDocComment(docComment.GetText()), true
		}
		if parent.OR() != nil && (parent.MODIFY() != nil || parent.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	if stmt.Documentation == "" {
		if stmtCtx := findParentStatement(ctx); stmtCtx != nil {
			if docCtx := stmtCtx.DocComment(); docCtx != nil {
				stmt.Documentation, stmt.DocumentationSet = extractDocComment(docCtx.GetText()), true
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// canonicalJavaScriptPlatform normalises a platform value to Studio Pro's casing
// (Web/Native/Hybrid/All); an unrecognised value is title-cased as a fallback.
func canonicalJavaScriptPlatform(p string) string {
	switch strings.ToLower(p) {
	case "web":
		return "Web"
	case "native":
		return "Native"
	case "hybrid":
		return "Hybrid"
	case "all":
		return "All"
	default:
		if p == "" {
			return "Web"
		}
		return strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
}

func buildJavaActionReturnType(ctx parser.IDataTypeContext) ast.DataType {
	dt := buildDataType(ctx)
	if isVoidReturnType(dt) {
		return ast.DataType{Kind: ast.TypeVoid}
	}
	return dt
}

func isVoidReturnType(dt ast.DataType) bool {
	var name ast.QualifiedName
	switch dt.Kind {
	case ast.TypeVoid:
		return true
	case ast.TypeEntity:
		if dt.EntityRef == nil {
			return false
		}
		name = *dt.EntityRef
	case ast.TypeEnumeration:
		if dt.EnumRef == nil {
			return false
		}
		name = *dt.EnumRef
	default:
		return false
	}
	return name.Module == "" && strings.EqualFold(name.Name, "void")
}

// extractJavaImports separates `import ...;` lines from Java code.
// Lines matching the Java import statement pattern are returned as imports;
// the remaining lines form the method body. This handles the common case
// where AI agents prepend import statements inside the $$ block, which
// would otherwise end up as illegal Java inside executeAction().
func extractJavaImports(code string) (body string, imports []string) {
	var bodyLines []string
	for _, line := range strings.Split(code, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") && strings.HasSuffix(trimmed, ";") {
			imports = append(imports, trimmed)
		} else {
			bodyLines = append(bodyLines, line)
		}
	}
	return strings.TrimSpace(strings.Join(bodyLines, "\n")), imports
}

// sourceCodeTextValue is the text of a Java or JavaScript action's source
// literal: a `$$ … $$` without its delimiters, or a string literal read under
// the escape rule it was lexed with — the form describe writes for a text
// holding `$$` (ako/mxcli#1003).
func sourceCodeTextValue(c parser.ISourceCodeTextContext) string {
	if sl := c.STRING_LITERAL(); sl != nil {
		return unquoteStringLit(sl)
	}
	if ds := c.DOLLAR_STRING(); ds != nil {
		return unquoteDollarString(ds.GetText())
	}
	return ""
}

// extractJavaExtraCode returns the text of an `extra code $$ … $$` clause
// (its delimiters already removed), its import lines taken out, with the
// blank lines at either end dropped and every other line as written — apart
// from the spaces between a `$$` and text on its own line, which are not
// indentation.
func extractJavaExtraCode(dollar string) (code string, imports []string) {
	dollar = strings.TrimRight(strings.TrimLeft(dollar, " \t"), " \t")
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(dollar, "\r\n", "\n"), "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "import ") && strings.HasSuffix(t, ";") {
			imports = append(imports, t)
			continue
		}
		lines = append(lines, line)
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n"), imports
}

// buildJavaActionParams reads a Java or JavaScript action's parameter list —
// one helper for both, since they share the grammar rule and a field read by
// one handler only is a silent drop in the other.
func buildJavaActionParams(list parser.IJavaActionParameterListContext) []ast.JavaActionParam {
	if list == nil {
		return nil
	}
	var params []ast.JavaActionParam
	for _, paramCtx := range list.AllJavaActionParameter() {
		param := ast.JavaActionParam{}
		if pn := paramCtx.ParameterName(); pn != nil {
			param.Name = parameterNameText(pn)
		}
		if dt := paramCtx.DataType(); dt != nil {
			param.Type = buildDataType(dt)
		}
		// Check for NOT NULL constraint
		if paramCtx.NOT_NULL() != nil {
			param.IsRequired = true
		}
		// The parameter's description (mendixlabs/mxcli#1588).
		if doc := paramCtx.DocComment(); doc != nil {
			param.Description, param.DescriptionSet = extractDocComment(doc.GetText()), true
		}
		params = append(params, param)
	}
	return params
}

// buildJavaActionReturn reads `returns T [as $Name]`: the return type, and the
// return value's name without its `$` ("" when the clause names none).
func buildJavaActionReturn(ret parser.IJavaActionReturnTypeContext) (ast.DataType, string) {
	if ret == nil {
		return ast.DataType{}, ""
	}
	var dt ast.DataType
	if d := ret.DataType(); d != nil {
		dt = buildJavaActionReturnType(d)
	}
	var name string
	if v := ret.VARIABLE(); v != nil {
		name = strings.TrimPrefix(v.GetText(), "$")
	}
	return dt, name
}
