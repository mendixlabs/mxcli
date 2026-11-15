// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/doccomment"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
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
		// A doc comment on a value is not stored — Enumerations$EnumerationValue
		// has no documentation property. It is carried on the AST only so
		// `check` can say so (MDL-ENUMDOC01, ako/mxcli#706).
		if docCtx := ev.DocComment(); docCtx != nil {
			enumVal.Documentation = extractDocComment(docCtx.GetText())
		}
		if ev.STRING_LITERAL() != nil {
			enumVal.Caption = unquoteStringLit(ev.STRING_LITERAL())
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
				if c.ConstraintErrorKeyword() != nil && c.STRING_LITERAL() != nil {
					attr.NotNullError = unquoteStringLit(c.STRING_LITERAL())
				}
			}
			if c.UNIQUE() != nil {
				attr.Unique = true
				// Extract error message if present
				if c.ConstraintErrorKeyword() != nil && c.STRING_LITERAL() != nil {
					attr.UniqueError = unquoteStringLit(c.STRING_LITERAL())
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
				if c.ConstraintErrorKeyword() != nil && c.STRING_LITERAL() != nil {
					attr.NotNullError = unquoteStringLit(c.STRING_LITERAL())
				}
			}
			if c.CALCULATED() != nil {
				attr.Calculated = true
				if qn := c.QualifiedName(); qn != nil {
					name := buildQualifiedName(qn)
					attr.CalculatedMicroflow = &name
				}
			}
			applyLocalizeConstraint(&attr.Type, c)
			if el := c.MicroflowExportLevelClause(); el != nil {
				attr.ExportLevel = exportLevelOf(el.(*parser.MicroflowExportLevelClauseContext))
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
			if c.ConstraintErrorKeyword() != nil && c.STRING_LITERAL() != nil {
				attr.NotNullError = unquoteStringLit(c.STRING_LITERAL())
			}
		}
		if c.UNIQUE() != nil {
			attr.Unique = true
			if c.ConstraintErrorKeyword() != nil && c.STRING_LITERAL() != nil {
				attr.UniqueError = unquoteStringLit(c.STRING_LITERAL())
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
			if c.ConstraintErrorKeyword() != nil && c.STRING_LITERAL() != nil {
				attr.NotNullError = unquoteStringLit(c.STRING_LITERAL())
			}
		}
		if c.CALCULATED() != nil {
			attr.Calculated = true
			if qn := c.QualifiedName(); qn != nil {
				name := buildQualifiedName(qn)
				attr.CalculatedMicroflow = &name
			}
		}
		applyLocalizeConstraint(&attr.Type, c)
		if el := c.MicroflowExportLevelClause(); el != nil {
			attr.ExportLevel = exportLevelOf(el.(*parser.MicroflowExportLevelClauseContext))
		}
	}

	return attr
}

// applyLocalizeConstraint records a `localized` / `not localized` constraint on
// the attribute's type (DateTimeAttributeType.LocalizeDate, #1373). Whether the
// type is a DateTime at all is ExitAttributeConstraint's to refuse; here the
// spec is only carried, so a non-DateTime type keeps it but never reads it.
func applyLocalizeConstraint(dt *ast.DataType, c *parser.AttributeConstraintContext) {
	if c == nil || c.LOCALIZED() == nil {
		return
	}
	if c.NOT() != nil {
		dt.Localize = ast.LocalizeNotLocalized
	} else {
		dt.Localize = ast.LocalizeLocalized
	}
}

func buildIndex(ctx parser.IIndexDefinitionContext) ast.Index {
	if ctx == nil {
		return ast.Index{}
	}
	idxDef := ctx.(*parser.IndexDefinitionContext)
	idx := ast.Index{Columns: buildIndexColumns(idxDef.IndexAttributeList())}
	// The name is not stored (a Mendix index is anonymous); it is carried only
	// for the MDL-IDX01 warning.
	if n := idxDef.IDENTIFIER(); n != nil {
		idx.Name = n.GetText()
	} else if n := idxDef.QUOTED_IDENTIFIER(); n != nil {
		idx.Name = unquoteIdentifier(n.GetText())
	}
	return idx
}

// buildIndexColumns reads an index's column list. It is shared by the two
// spellings that reach it with the list but not the surrounding definition: the
// entity-body/ALTER form (`index "Idx" on (A, B DESC)`) and the standalone
// `create index Idx on Mod.Entity (A, B DESC)`.
func buildIndexColumns(attrList parser.IIndexAttributeListContext) []ast.IndexColumn {
	if attrList == nil {
		return nil
	}
	ial := attrList.(*parser.IndexAttributeListContext)
	var columns []ast.IndexColumn
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
	return columns
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
	// The name may be a keyword or quoted: Studio Pro accepts any name for a
	// type parameter, including a primitive's (`entity <String>`, #1183).
	if dtCtx.ENTITY() != nil && dtCtx.LESS_THAN() != nil && dtCtx.IdentifierOrKeyword() != nil {
		return ast.DataType{
			Kind:          ast.TypeEntityTypeParam,
			TypeParamName: identifierOrKeywordText(dtCtx.IdentifierOrKeyword()),
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
		// `date` (MDL-DEPR160): Mendix has no date-only type, and a `date`
		// was always stored as a DateTime.
		return ast.DataType{Kind: ast.TypeDateTime}
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
	// HashedString had no branch and fell through to the String below, so a
	// password attribute was stored as an unlimited String. Outside an
	// attribute definition EnterDataType refuses it before this runs.
	if dtCtx.HASHEDSTRING_TYPE() != nil {
		return ast.DataType{Kind: ast.TypeHashedString}
	}

	return ast.DataType{Kind: ast.TypeString}
}

func buildDeleteBehavior(ctx parser.IDeleteBehaviorContext) ast.DeleteBehavior {
	if ctx == nil {
		return ast.DeleteKeepReferences
	}
	db := ctx.(*parser.DeleteBehaviorContext)

	// Every token the deleteBehavior rule admits has to be read here. Only
	// CASCADE was, so PREVENT, DELETE_IF_NO_REFERENCES and DELETE_AND_REFERENCES
	// all fell through to the zero value — a legal behaviour, which is why the
	// substitution was silent all the way to disk and overwrote whatever the
	// association had (upstream #901). Adding a token to the grammar means
	// adding it here; there is no default that can be trusted to be right.
	switch {
	case db.CASCADE() != nil, db.DELETE_AND_REFERENCES() != nil:
		return ast.DeleteCascade
	case db.PREVENT() != nil, db.DELETE_IF_NO_REFERENCES() != nil:
		return ast.DeleteIfNoReferences
	default:
		// DELETE_BUT_KEEP_REFERENCES, and Mendix's default for an association
		// that names no behaviour at all.
		return ast.DeleteKeepReferences
	}
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
		return unquoteStringLit(lit.STRING_LITERAL())
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
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		cleaned = append(cleaned, line)
	}
	// Only the blank lines at the edges are framing (the `/**` and ` */`
	// lines). An interior blank line is a paragraph break and is part of the
	// text: dropping it turned Studio Pro's "\n\n" into "\n" on every
	// describe → exec round trip (mendixlabs/mxcli#1300).
	for len(cleaned) > 0 && cleaned[0] == "" {
		cleaned = cleaned[1:]
	}
	for len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	// Describe writes a `*/` in the text as `*\/`, which does not end the
	// comment (ako/mxcli#2113).
	return doccomment.Unescape(strings.Join(cleaned, "\n"))
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

// extractExpressionText lifts a Mendix expression's source text, without the MDL
// comments the raw input stream would otherwise carry into the model.
//
// Use this rather than extractOriginalText for anything stored as a Mendix
// expression. OQL keeps extractOriginalText: `--` is a legitimate SQL comment
// there, and stripping it would change a different language's meaning.
func extractExpressionText(ctx antlr.ParserRuleContext) string {
	return stripMDLComments(extractOriginalText(ctx), lexedWithStrictEscapes(ctx))
}

// stripMDLComments removes MDL comments from text lifted out of the input stream.
//
// extractOriginalText reads the raw source between two token positions, which is
// what preserves an expression's spacing — and also drags in anything the lexer
// sent to a hidden channel. MDL comments are `--` to end of line and `/* … */`;
// Mendix expressions use neither, so a comment written between two operands
// ended up stored *inside* the expression and the build failed with CE0117
// "Error(s) in expression" (mxcli-formula1 suggested issue 11):
//
//	declare $Msg String = 'a' +
//	  -- explain the second half
//	  'b';
//
// A comment is replaced by a single space rather than deleted, so `1 --c\n+ 2`
// does not become `1+ 2`… and more importantly `'a'--c\n'b'` cannot silently
// weld into one token.
//
// Single-quoted string literals are respected: a Mendix string may legitimately
// contain `--` or `/*`, and removing those would corrupt the value. A literal
// is scanned by the string rule of the text's language (strict: ADR-0010 R11,
// where a backslash is itself; otherwise mdl 0's, where a backslash escapes
// the next character). Scanned the strict way, the mdl 0 `'it\'s'` ended at
// `\'`, so a `--` inside the NEXT string was taken for a comment and the rest
// of the line was dropped from the stored expression or XPath (ako/mxcli#825).
func stripMDLComments(s string, strict bool) string {
	if !strings.Contains(s, "--") && !strings.Contains(s, "/*") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString && c == '\\' && !strict && i+1 < len(s) {
			b.WriteByte(c)
			b.WriteByte(s[i+1])
			i++
			continue
		}
		if c == '\'' {
			if inString && i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte(c)
				b.WriteByte(s[i+1])
				i++
				continue
			}
			inString = !inString
			b.WriteByte(c)
			continue
		}
		if !inString && c == '-' && i+1 < len(s) && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			// Keep the newline itself: it is the statement's own layout, and a
			// multi-line expression stays readable in DESCRIBE.
			if i < len(s) {
				b.WriteByte('\n')
			}
			continue
		}
		if !inString && c == '/' && i+1 < len(s) && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				// Unterminated: the parser would already have complained. Drop
				// the rest rather than emit half a comment into the model.
				break
			}
			i += 2 + end + 1
			b.WriteByte(' ')
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

// buildReferentialAction maps SQL's referential actions onto Mendix's three
// delete behaviours. They correspond exactly:
//
//	ON DELETE CASCADE   -> DeleteMeAndReferences
//	ON DELETE RESTRICT  -> DeleteMeIfNoReferences
//	ON DELETE SET NULL  -> DeleteMeButKeepReferences (Mendix's default)
//
// Every alternative the referentialAction rule admits is read explicitly, for
// the reason buildDeleteBehavior spells out above: a token that falls through to
// a zero value is a LEGAL behaviour, so the substitution is silent all the way
// to disk and overwrites whatever the association had (upstream #901). Adding an
// alternative to the rule means adding it here. The default below is unreachable
// while the rule has three alternatives, and is not a fallback to rely on.
func buildReferentialAction(ctx parser.IReferentialActionContext) ast.DeleteBehavior {
	if ctx == nil {
		return ast.DeleteKeepReferences
	}
	ra := ctx.(*parser.ReferentialActionContext)
	switch {
	case ra.CASCADE() != nil:
		return ast.DeleteCascade
	case ra.RESTRICT() != nil:
		return ast.DeleteIfNoReferences
	case ra.SET() != nil && ra.NULL() != nil:
		return ast.DeleteKeepReferences
	default:
		return ast.DeleteKeepReferences
	}
}

// buildErrorMessage reads the text of an ERROR_MESSAGE clause.
func buildErrorMessage(ctx parser.IErrorMessageClauseContext) string {
	if ctx == nil {
		return ""
	}
	emc, ok := ctx.(*parser.ErrorMessageClauseContext)
	if !ok || emc.STRING_LITERAL() == nil {
		return ""
	}
	return unquoteStringLit(emc.STRING_LITERAL())
}

// expressionSourceText is an expression as the author wrote it — whitespace kept,
// MDL comments removed — for the places that store an expression as TEXT rather
// than building its AST. Never use ctx.GetText() for this: it joins the tokens
// without the whitespace between them, so literals and `+` survive but a keyword
// operator fuses with its neighbours (`$a and $b` -> `$aand$b`, `if $x then 'a'`
// -> `if$xthen'a'`), and that fused text is what reaches the model.
func expressionSourceText(expr parser.IExpressionContext) string {
	if expr == nil {
		return ""
	}
	if prc, ok := expr.(antlr.ParserRuleContext); ok {
		if source := strings.TrimSpace(extractExpressionText(prc)); source != "" {
			return source
		}
	}
	return expr.GetText()
}
