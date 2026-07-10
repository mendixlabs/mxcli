// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// unquoteIdentifier strips surrounding double-quotes or backticks from a quoted identifier.
// e.g. `"ComboBox"` → `ComboBox`, “ `Order` “ → `Order`
func unquoteIdentifier(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// unquoteQualifiedName strips quotes from each segment of a dotted qualified name.
// e.g. MaisonElegance."FormSubmissionStatus".StatusNew -> MaisonElegance.FormSubmissionStatus.StatusNew
func unquoteQualifiedName(s string) string {
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = unquoteIdentifier(p)
	}
	return strings.Join(parts, ".")
}

// attributeNameText extracts the clean name from an AttributeNameContext,
// stripping quotes from QUOTED_IDENTIFIER tokens.
func attributeNameText(ctx parser.IAttributeNameContext) string {
	if ctx == nil {
		return ""
	}
	return unquoteIdentifier(ctx.GetText())
}

// parameterNameText extracts the clean name from a ParameterNameContext,
// stripping quotes from QUOTED_IDENTIFIER tokens.
func parameterNameText(ctx parser.IParameterNameContext) string {
	if ctx == nil {
		return ""
	}
	return unquoteIdentifier(ctx.GetText())
}

// memberAttributeNameText extracts the clean name from a MemberAttributeNameContext,
// stripping quotes from QUOTED_IDENTIFIER tokens and handling qualified names.
func memberAttributeNameText(ctx parser.IMemberAttributeNameContext) string {
	if ctx == nil {
		return ""
	}
	mac := ctx.(*parser.MemberAttributeNameContext)
	if qn := mac.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	return unquoteIdentifier(ctx.GetText())
}

// identifierOrKeywordText extracts the clean name from an IdentifierOrKeywordContext,
// stripping quotes from QUOTED_IDENTIFIER tokens.
func identifierOrKeywordText(ctx parser.IIdentifierOrKeywordContext) string {
	if ctx == nil {
		return ""
	}
	iok := ctx.(*parser.IdentifierOrKeywordContext)
	if qi := iok.QUOTED_IDENTIFIER(); qi != nil {
		return unquoteIdentifier(qi.GetText())
	}
	return iok.GetText()
}

func getQualifiedNameText(ctx parser.IQualifiedNameContext) string {
	if ctx == nil {
		return ""
	}
	qn := ctx.(*parser.QualifiedNameContext)
	parts := qn.AllIdentifierOrKeyword()
	names := make([]string, len(parts))
	for i, p := range parts {
		names[i] = identifierOrKeywordText(p)
	}
	return strings.Join(names, ".")
}

func buildQualifiedName(ctx parser.IQualifiedNameContext) ast.QualifiedName {
	if ctx == nil {
		return ast.QualifiedName{}
	}
	qn := ctx.(*parser.QualifiedNameContext)
	parts := qn.AllIdentifierOrKeyword()

	if len(parts) == 1 {
		return ast.QualifiedName{Name: identifierOrKeywordText(parts[0])}
	}
	if len(parts) >= 2 {
		// Join parts[1:] to preserve 3+ part names like Module.Enum.Value
		remaining := make([]string, len(parts)-1)
		for i, p := range parts[1:] {
			remaining[i] = identifierOrKeywordText(p)
		}
		return ast.QualifiedName{
			Module: identifierOrKeywordText(parts[0]),
			Name:   strings.Join(remaining, "."),
		}
	}
	return ast.QualifiedName{}
}

func buildEnumValues(ctx parser.IEnumerationValueListContext, b *Builder) []ast.EnumValue {
	if ctx == nil {
		return nil
	}
	evl := ctx.(*parser.EnumerationValueListContext)
	var values []ast.EnumValue
	for _, evCtx := range evl.AllEnumerationValue() {
		ev := evCtx.(*parser.EnumerationValueContext)
		// Check if enum value name is nil (parse error)
		if ev.EnumValueName() == nil {
			if b != nil {
				b.addErrorWithExample(
					"Invalid enumeration value: each value must have a name",
					`  CREATE ENUMERATION MyModule.OrderStatus (
    Pending "Pending Order",
    Processing "Being Processed",
    Completed "Order Complete",
    Cancelled "Order Cancelled"
  );`)
			}
			continue
		}
		enumVal := ast.EnumValue{
			Name: unquoteIdentifier(ev.EnumValueName().GetText()),
		}
		// Extract documentation if present
		if docCtx := ev.DocComment(); docCtx != nil {
			enumVal.Documentation = extractDocComment(docCtx.GetText())
		}
		if ev.STRING_LITERAL() != nil {
			enumVal.Caption = unquoteString(ev.STRING_LITERAL().GetText())
		}
		values = append(values, enumVal)
	}
	return values
}

func buildAttributes(ctx parser.IAttributeDefinitionListContext, b *Builder) []ast.Attribute {
	if ctx == nil {
		return nil
	}
	attrList := ctx.(*parser.AttributeDefinitionListContext)
	var attrs []ast.Attribute
	for _, attrCtx := range attrList.AllAttributeDefinition() {
		a := attrCtx.(*parser.AttributeDefinitionContext)
		// Check if attribute name is nil (parse error)
		if a.AttributeName() == nil {
			if b != nil {
				b.addErrorWithExample(
					"Invalid attribute: each attribute must have a name and type",
					`  CREATE PERSISTENT ENTITY MyModule.Customer (
    Name: String(100) NOT NULL,
    Email: String(200),
    Age: Integer,
    IsActive: Boolean DEFAULT true,
    CreatedDate: DateTime
  );`)
			}
			continue
		}
		attr := ast.Attribute{
			Name: attributeNameText(a.AttributeName()),
			Type: buildDataType(a.DataType()),
		}

		// Extract attribute documentation
		if docCtx := a.DocComment(); docCtx != nil {
			attr.Documentation = extractDocComment(docCtx.GetText())
		}

		// Constraints (NOT NULL, UNIQUE, DEFAULT)
		for _, constraintCtx := range a.AllAttributeConstraint() {
			c := constraintCtx.(*parser.AttributeConstraintContext)
			if c.NOT() != nil && c.NULL() != nil || c.NOT_NULL() != nil {
				attr.NotNull = true
				// Extract error message if present
				if c.ERROR() != nil && c.STRING_LITERAL() != nil {
					attr.NotNullError = unquoteString(c.STRING_LITERAL().GetText())
				}
			}
			if c.UNIQUE() != nil {
				attr.Unique = true
				// Extract error message if present
				if c.ERROR() != nil && c.STRING_LITERAL() != nil {
					attr.UniqueError = unquoteString(c.STRING_LITERAL().GetText())
				}
			}
			if c.DEFAULT() != nil {
				attr.HasDefault = true
				// Extract default value from literal or expression
				if lit := c.Literal(); lit != nil {
					attr.DefaultValue = extractLiteralValue(lit)
				} else if expr := c.Expression(); expr != nil {
					attr.DefaultValue = unquoteQualifiedName(expr.GetText())
				}
			}
			if c.REQUIRED() != nil {
				attr.NotNull = true
				// Extract error message if present
				if c.ERROR() != nil && c.STRING_LITERAL() != nil {
					attr.NotNullError = unquoteString(c.STRING_LITERAL().GetText())
				}
			}
			if c.CALCULATED() != nil {
				attr.Calculated = true
				if qn := c.QualifiedName(); qn != nil {
					name := buildQualifiedName(qn)
					attr.CalculatedMicroflow = &name
				}
			}
		}

		attrs = append(attrs, attr)
	}
	return attrs
}

// buildSingleAttribute builds an ast.Attribute from a single AttributeDefinitionContext.
func buildSingleAttribute(a *parser.AttributeDefinitionContext) *ast.Attribute {
	if a == nil || a.AttributeName() == nil {
		return nil
	}
	attr := &ast.Attribute{
		Name: attributeNameText(a.AttributeName()),
		Type: buildDataType(a.DataType()),
	}

	// Extract attribute documentation
	if docCtx := a.DocComment(); docCtx != nil {
		attr.Documentation = extractDocComment(docCtx.GetText())
	}

	// Constraints (NOT NULL, UNIQUE, DEFAULT)
	for _, constraintCtx := range a.AllAttributeConstraint() {
		c := constraintCtx.(*parser.AttributeConstraintContext)
		if c.NOT() != nil && c.NULL() != nil || c.NOT_NULL() != nil {
			attr.NotNull = true
			if c.ERROR() != nil && c.STRING_LITERAL() != nil {
				attr.NotNullError = unquoteString(c.STRING_LITERAL().GetText())
			}
		}
		if c.UNIQUE() != nil {
			attr.Unique = true
			if c.ERROR() != nil && c.STRING_LITERAL() != nil {
				attr.UniqueError = unquoteString(c.STRING_LITERAL().GetText())
			}
		}
		if c.DEFAULT() != nil {
			attr.HasDefault = true
			if lit := c.Literal(); lit != nil {
				attr.DefaultValue = extractLiteralValue(lit)
			} else if expr := c.Expression(); expr != nil {
				attr.DefaultValue = unquoteQualifiedName(expr.GetText())
			}
		}
		if c.REQUIRED() != nil {
			attr.NotNull = true
			if c.ERROR() != nil && c.STRING_LITERAL() != nil {
				attr.NotNullError = unquoteString(c.STRING_LITERAL().GetText())
			}
		}
		if c.CALCULATED() != nil {
			attr.Calculated = true
			if qn := c.QualifiedName(); qn != nil {
				name := buildQualifiedName(qn)
				attr.CalculatedMicroflow = &name
			}
		}
	}

	return attr
}

func buildIndex(ctx parser.IIndexDefinitionContext) ast.Index {
	if ctx == nil {
		return ast.Index{}
	}
	idxDef := ctx.(*parser.IndexDefinitionContext)
	var columns []ast.IndexColumn

	if attrList := idxDef.IndexAttributeList(); attrList != nil {
		ial := attrList.(*parser.IndexAttributeListContext)
		for _, ia := range ial.AllIndexAttribute() {
			iaCtx := ia.(*parser.IndexAttributeContext)
			col := ast.IndexColumn{
				Name: unquoteIdentifier(iaCtx.IndexColumnName().GetText()),
			}
			// Check for sort order (ASC/DESC)
			if iaCtx.DESC() != nil {
				col.Descending = true
			}
			// ASC is default, no need to set explicitly
			columns = append(columns, col)
		}
	}

	return ast.Index{Columns: columns}
}

func buildDataType(ctx parser.IDataTypeContext) ast.DataType {
	if ctx == nil {
		return ast.DataType{Kind: ast.TypeString}
	}
	dtCtx := ctx.(*parser.DataTypeContext)
	text := strings.ToUpper(dtCtx.GetText())

	// Check for List type (List OF Entity)
	if dtCtx.LIST_OF() != nil {
		dt := ast.DataType{Kind: ast.TypeListOf}
		if qn := dtCtx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			dt.EntityRef = &name
		}
		return dt
	}

	// Handle StringTemplate(templateContext) for Java actions
	if dtCtx.STRINGTEMPLATE_TYPE() != nil {
		templateContext := ""
		if tc := dtCtx.TemplateContext(); tc != nil {
			templateContext = tc.GetText()
		}
		return ast.DataType{Kind: ast.TypeStringTemplate, TemplateContext: templateContext}
	}

	// Handle ENTITY <pEntity> — type parameter declaration for Java actions
	if dtCtx.ENTITY() != nil && dtCtx.LESS_THAN() != nil && dtCtx.IDENTIFIER() != nil {
		return ast.DataType{
			Kind:          ast.TypeEntityTypeParam,
			TypeParamName: dtCtx.IDENTIFIER().GetText(),
		}
	}

	// Handle ENUMERATION(QualifiedName) or ENUM QualifiedName — unambiguous enum.
	if dtCtx.ENUMERATION() != nil || dtCtx.ENUM_TYPE() != nil {
		if qn := dtCtx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &name, ExplicitEnum: true}
		}
	}

	// Handle bare qualified name (entity reference or enumeration)
	if qn := dtCtx.QualifiedName(); qn != nil {
		name := buildQualifiedName(qn)
		// Check for common type mistakes before treating as enumeration
		upperName := strings.ToUpper(name.String())
		if upperName == "DATEANDTIME" || upperName == ".DATEANDTIME" {
			return ast.DataType{Kind: ast.TypeDateTime}
		}
		return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &name}
	}

	// Parse primitive types
	if strings.HasPrefix(text, "STRING") {
		dt := ast.DataType{Kind: ast.TypeString, Length: 0} // 0 = unlimited by default
		if dtCtx.NUMBER_LITERAL() != nil {
			dt.Length, _ = strconv.Atoi(dtCtx.NUMBER_LITERAL().GetText())
		}
		return dt
	}
	if strings.HasPrefix(text, "INTEGER") {
		return ast.DataType{Kind: ast.TypeInteger}
	}
	if strings.HasPrefix(text, "LONG") {
		return ast.DataType{Kind: ast.TypeLong}
	}
	if strings.HasPrefix(text, "DECIMAL") {
		return ast.DataType{Kind: ast.TypeDecimal}
	}
	if strings.HasPrefix(text, "BOOLEAN") {
		return ast.DataType{Kind: ast.TypeBoolean}
	}
	if strings.HasPrefix(text, "DATETIME") || text == "DATEANDTIME" {
		return ast.DataType{Kind: ast.TypeDateTime}
	}
	if strings.HasPrefix(text, "DATE") {
		return ast.DataType{Kind: ast.TypeDate}
	}
	if strings.HasPrefix(text, "AUTONUMBER") {
		return ast.DataType{Kind: ast.TypeAutoNumber}
	}
	if strings.HasPrefix(text, "AUTOOWNER") {
		return ast.DataType{Kind: ast.TypeAutoOwner}
	}
	if strings.HasPrefix(text, "AUTOCHANGEDBY") {
		return ast.DataType{Kind: ast.TypeAutoChangedBy}
	}
	if strings.HasPrefix(text, "AUTOCREATEDDATE") {
		return ast.DataType{Kind: ast.TypeAutoCreatedDate}
	}
	if strings.HasPrefix(text, "AUTOCHANGEDDATE") {
		return ast.DataType{Kind: ast.TypeAutoChangedDate}
	}
	if strings.HasPrefix(text, "BINARY") {
		return ast.DataType{Kind: ast.TypeBinary}
	}

	return ast.DataType{Kind: ast.TypeString}
}

func buildDeleteBehavior(ctx parser.IDeleteBehaviorContext) ast.DeleteBehavior {
	if ctx == nil {
		return ast.DeleteKeepReferences
	}
	db := ctx.(*parser.DeleteBehaviorContext)

	if db.CASCADE() != nil {
		return ast.DeleteCascade
	}
	// The new grammar may use different tokens for delete behaviors
	// Add more cases as needed based on the actual grammar

	return ast.DeleteKeepReferences
}

// QuoteString escapes s for safe embedding inside an MDL single-quoted string
// literal. It is the inverse of unquoteString: a backslash becomes `\\` and an
// apostrophe becomes `”`. Use it whenever a runtime value (e.g. a filesystem
// path) is interpolated into MDL source such as `CONNECT LOCAL '<path>'`, so a
// Windows path like `C:\temp\App.mpr` round-trips unchanged instead of having
// `\t`/`\n`/`\r` interpreted as escapes. See issue #644.
func QuoteString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return s
}

func unquoteString(s string) string {
	// Remove surrounding quotes and unescape
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		s = s[1 : len(s)-1]
	}
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i++
			} else {
				b.WriteByte('\'')
			}
		case '\\':
			if i+1 >= len(s) {
				b.WriteByte('\\')
				continue
			}
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\':
				b.WriteByte('\\')
			case '\'':
				b.WriteByte('\'')
			default:
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
		default:
			b.WriteByte(s[i])
		}
	}

	return b.String()
}

func extractLiteralValue(ctx parser.ILiteralContext) any {
	if ctx == nil {
		return nil
	}
	lit := ctx.(*parser.LiteralContext)

	// Check for different literal types
	if lit.STRING_LITERAL() != nil {
		return unquoteString(lit.STRING_LITERAL().GetText())
	}
	if lit.NUMBER_LITERAL() != nil {
		text := lit.NUMBER_LITERAL().GetText()
		// Try to parse as integer first
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return i
		}
		// Try as float
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return f
		}
		return text
	}
	// Boolean literals are in a separate rule
	if boolLit := lit.BooleanLiteral(); boolLit != nil {
		boolCtx := boolLit.(*parser.BooleanLiteralContext)
		if boolCtx.TRUE() != nil {
			return true
		}
		if boolCtx.FALSE() != nil {
			return false
		}
	}
	if lit.NULL() != nil {
		return nil
	}

	return lit.GetText()
}

func extractDocComment(s string) string {
	// Remove /** and */ and clean up
	s = strings.TrimPrefix(s, "/**")
	s = strings.TrimSuffix(s, "*/")

	// Remove leading * from each line
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// extractOriginalText extracts the original text from a parse tree context,
// preserving whitespace between tokens.
func extractOriginalText(ctx antlr.ParserRuleContext) string {
	if ctx == nil {
		return ""
	}
	start := ctx.GetStart()
	stop := ctx.GetStop()
	if start == nil || stop == nil {
		return ctx.GetText()
	}
	is := start.GetInputStream()
	if is == nil {
		return ctx.GetText()
	}
	// Guard against invalid positions from parse errors
	startPos := start.GetStart()
	stopPos := stop.GetStop()
	if startPos < 0 || stopPos < 0 || stopPos < startPos {
		return ctx.GetText()
	}
	return is.GetText(startPos, stopPos)
}

// stripExpressionIdentifierQuotes removes double-quote / backtick identifier
// quotes from a Mendix expression or XPath source string, leaving single-quoted
// string literals untouched.
//
// The skills tell users to quote any identifier to escape MDL parser keywords
// ("always safe to quote"). In binding/declaration positions this holds because
// each identifier token is routed through unquoteIdentifier. In expression
// positions the source text was previously stored verbatim, so a quoted
// attribute like $Order/"Status" leaked its quotes into the compiled expression
// and only mxbuild rejected it. This helper extends the same quote-stripping to
// expression contexts.
//
// It is safe because in Mendix expression and XPath syntax string literals use
// single quotes exclusively; double quotes and backticks only ever wrap
// identifiers (attribute / association names). Any that appear outside a
// single-quoted literal are therefore identifier quotes and can be dropped.
// Doubled single quotes (”) inside a literal are the Mendix escape for a
// literal quote and do not terminate the literal.
func stripExpressionIdentifierQuotes(s string) string {
	if !strings.ContainsAny(s, "\"`") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			if inString && i+1 < len(s) && s[i+1] == '\'' {
				// Escaped quote inside a string literal: keep both, stay inside.
				b.WriteByte(c)
				b.WriteByte(s[i+1])
				i++
				continue
			}
			inString = !inString
			b.WriteByte(c)
			continue
		}
		if !inString && (c == '"' || c == '`') {
			// Identifier quote outside a string literal: drop it.
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// ----------------------------------------------------------------------------
// Microflow Statements
// ----------------------------------------------------------------------------

// ExitCreateMicroflowStatement is called when exiting the createMicroflowStatement production.
