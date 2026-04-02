// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// OQLColumnInfo represents inferred type information for an OQL SELECT column.
type OQLColumnInfo struct {
	Alias         string       // The column alias (attribute name)
	Expression    string       // The original expression
	InferredType  ast.DataType // The inferred data type
	SourceEntity  string       // Source entity if it's an attribute reference
	SourceAttr    string       // Source attribute name if it's an attribute reference
	IsAggregate   bool         // Whether this is an aggregate function
	AggregateFunc string       // The aggregate function name (COUNT, SUM, AVG, etc.)
}

// InferOQLTypes analyzes an OQL query and returns the expected types for each column.
func (e *Executor) InferOQLTypes(oqlQuery string, declaredAttrs []ast.ViewAttribute) ([]OQLColumnInfo, []string) {
	var warnings []string
	var columns []OQLColumnInfo

	// Extract SELECT clause
	selectClause := extractSelectClause(oqlQuery)
	if selectClause == "" {
		warnings = append(warnings, "could not parse SELECT clause from OQL query")
		return columns, warnings
	}

	// Extract FROM clause and build alias map
	aliasMap := extractAliasMap(oqlQuery)

	// Parse column expressions
	columnExprs := parseSelectColumns(selectClause)
	if len(columnExprs) != len(declaredAttrs) {
		warnings = append(warnings, fmt.Sprintf(
			"OQL SELECT has %d columns but %d attributes declared",
			len(columnExprs), len(declaredAttrs)))
	}

	// Infer type for each column
	for i, expr := range columnExprs {
		col := OQLColumnInfo{Expression: expr}

		// Determine alias - either explicit AS alias, or use declared attribute name
		if i < len(declaredAttrs) {
			col.Alias = declaredAttrs[i].Name
		}

		// Check for explicit alias
		if aliasMatch := regexp.MustCompile(`(?i)\s+AS\s+(\w+)\s*$`).FindStringSubmatch(expr); aliasMatch != nil {
			col.Alias = aliasMatch[1]
			expr = strings.TrimSuffix(expr, aliasMatch[0])
			col.Expression = strings.TrimSpace(expr)
		}

		// Infer type from expression
		col.InferredType = e.inferTypeFromExpression(expr, &col, aliasMap)

		columns = append(columns, col)
	}

	return columns, warnings
}

// extractAliasMap parses the FROM clause to build a map of alias -> qualified entity name.
func extractAliasMap(oql string) map[string]string {
	aliasMap := make(map[string]string)

	// Match FROM Entity AS alias or FROM Entity alias patterns
	// Also handles JOIN clauses
	fromPattern := regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*)\s+(?:AS\s+)?([A-Za-z_][A-Za-z0-9_]*)`)
	matches := fromPattern.FindAllStringSubmatch(oql, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			entityName := match[1]
			alias := match[2]
			aliasMap[alias] = entityName
		}
	}

	return aliasMap
}

// ValidateOQLTypes performs static type checking of OQL SELECT expressions against
// declared view entity attributes. Does not require a project connection — it only
// checks types that can be inferred from the OQL syntax itself (aggregate functions,
// CASE expressions, literals, datepart, etc.).
func ValidateOQLTypes(oql string, attrs []ast.ViewAttribute) []linter.Violation {
	var violations []linter.Violation

	selectClause := extractSelectClause(oql)
	if selectClause == "" {
		return violations
	}

	columnExprs := parseSelectColumns(selectClause)

	for i, expr := range columnExprs {
		if i >= len(attrs) {
			break
		}

		// Strip AS alias
		if aliasMatch := regexp.MustCompile(`(?i)\s+AS\s+\w+\s*$`).FindStringSubmatch(expr); aliasMatch != nil {
			expr = strings.TrimSuffix(expr, aliasMatch[0])
		}
		expr = strings.TrimSpace(expr)

		inferred := inferTypeStatic(expr)
		if inferred.Kind == ast.TypeUnknown {
			continue
		}

		declared := attrs[i].Type
		// Use strict type matching: no implicit widening (e.g., count() returns Integer,
		// declaring as Decimal is wrong even though Decimal can hold integers).
		if !typesStrictlyCompatible(declared, inferred) {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL031",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"attribute '%s': declared as %s but OQL expression returns %s",
					attrs[i].Name,
					formatDataTypeForError(declared),
					formatDataTypeForError(inferred)),
				Location: linter.Location{
					DocumentType: "viewentity",
				},
				Suggestion: fmt.Sprintf("Fix: change to '%s: %s'", attrs[i].Name, formatDataTypeForMDL(inferred)),
			})
		}
	}

	return violations
}

// inferTypeStatic infers an OQL expression's type purely from syntax (no project needed).
func inferTypeStatic(expr string) ast.DataType {
	upper := strings.ToUpper(strings.TrimSpace(expr))

	// Subquery: peel off outer parentheses and recurse
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		inner := strings.TrimSpace(expr[1 : len(expr)-1])
		innerUpper := strings.ToUpper(inner)
		if strings.HasPrefix(innerUpper, "SELECT") {
			// Extract the inner SELECT clause and infer the single column
			innerSelect := extractSelectClause(inner)
			if innerSelect != "" {
				cols := parseSelectColumns(innerSelect)
				if len(cols) == 1 {
					col := cols[0]
					if aliasMatch := regexp.MustCompile(`(?i)\s+AS\s+\w+\s*$`).FindStringSubmatch(col); aliasMatch != nil {
						col = strings.TrimSuffix(col, aliasMatch[0])
					}
					return inferTypeStatic(strings.TrimSpace(col))
				}
			}
		}
	}

	// count(...) → Integer (Mendix OQL COUNT returns Integer)
	if strings.HasPrefix(upper, "COUNT(") {
		return ast.DataType{Kind: ast.TypeInteger}
	}

	// sum(...) → preserves input type (Integer→Integer, else Decimal)
	if strings.HasPrefix(upper, "SUM(") {
		innerArg := extractFunctionArg(expr)
		if innerArg != "" {
			innerType := inferTypeStatic(innerArg)
			if innerType.Kind == ast.TypeInteger || innerType.Kind == ast.TypeLong {
				return innerType
			}
		}
		return ast.DataType{Kind: ast.TypeDecimal}
	}

	// avg(...) → always Decimal
	if strings.HasPrefix(upper, "AVG(") {
		return ast.DataType{Kind: ast.TypeDecimal}
	}

	// min(...) / max(...) → propagates input type
	if strings.HasPrefix(upper, "MIN(") || strings.HasPrefix(upper, "MAX(") {
		innerArg := extractFunctionArg(expr)
		if innerArg != "" {
			innerType := inferTypeStatic(innerArg)
			if innerType.Kind != ast.TypeUnknown {
				return innerType
			}
		}
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	// datepart(...) → Integer
	if strings.HasPrefix(upper, "DATEPART(") {
		return ast.DataType{Kind: ast.TypeInteger}
	}

	// length(...) → Integer
	if strings.HasPrefix(upper, "LENGTH(") {
		return ast.DataType{Kind: ast.TypeInteger}
	}

	// CASE expression: infer from THEN clauses
	if strings.HasPrefix(upper, "CASE") {
		return inferCaseType(expr)
	}

	// Numeric literals
	if regexp.MustCompile(`^-?\d+$`).MatchString(expr) {
		return ast.DataType{Kind: ast.TypeInteger}
	}
	if regexp.MustCompile(`^-?\d+\.\d+$`).MatchString(expr) {
		return ast.DataType{Kind: ast.TypeDecimal}
	}

	// String literal
	if strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'") {
		return ast.DataType{Kind: ast.TypeString, Length: len(expr) - 2}
	}

	// Boolean literals
	if upper == "TRUE" || upper == "FALSE" {
		return ast.DataType{Kind: ast.TypeBoolean}
	}

	return ast.DataType{Kind: ast.TypeUnknown}
}

// inferCaseType infers the type of a CASE expression by looking at THEN clause values.
// Only reports a type when it can be confidently inferred from a non-literal THEN branch.
// Falls back to ELSE only if all THEN branches are unknown AND the ELSE is non-trivial.
// A bare "0" in ELSE is ambiguous (could be Integer or Decimal depending on context).
func inferCaseType(expr string) ast.DataType {
	// Nested CASE expressions are too complex for static regex-based inference.
	// The regex would match inner THEN clauses, producing wrong types.
	upperExpr := strings.ToUpper(expr)
	if strings.Count(upperExpr, "CASE") > 1 {
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	// Find THEN ... WHEN/ELSE/END patterns to extract result expressions
	thenPattern := regexp.MustCompile(`(?i)\bTHEN\s+(.+?)(?:\s+WHEN\b|\s+ELSE\b|\s+END\b)`)
	matches := thenPattern.FindAllStringSubmatch(expr, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			t := inferTypeStatic(strings.TrimSpace(match[1]))
			if t.Kind != ast.TypeUnknown {
				return t
			}
		}
	}
	// Try ELSE clause — but only if the value is not a bare integer literal.
	// Bare "0" or "1" in ELSE are type-ambiguous fallback values that should
	// not override the actual branch type (which may involve division or
	// expressions the static inferrer can't parse).
	elsePattern := regexp.MustCompile(`(?i)\bELSE\s+(.+?)\s+END\b`)
	if match := elsePattern.FindStringSubmatch(expr); len(match) >= 2 {
		elseExpr := strings.TrimSpace(match[1])
		// Skip bare integer literals — they're ambiguous in CASE context
		if !regexp.MustCompile(`^-?\d+$`).MatchString(elseExpr) {
			t := inferTypeStatic(elseExpr)
			if t.Kind != ast.TypeUnknown {
				return t
			}
		}
	}
	return ast.DataType{Kind: ast.TypeUnknown}
}

// ValidateViewEntityTypes validates that declared attribute types match inferred OQL types.
func (e *Executor) ValidateViewEntityTypes(stmt *ast.CreateViewEntityStmt) []string {
	var errors []string

	// First validate OQL syntax for common mistakes
	syntaxViolations := ValidateOQLSyntax(stmt.Query.RawQuery)
	for _, v := range syntaxViolations {
		errors = append(errors, v.Message)
	}

	// Static type checks (no project needed)
	typeViolations := ValidateOQLTypes(stmt.Query.RawQuery, stmt.Attributes)
	for _, v := range typeViolations {
		errors = append(errors, v.Message)
	}

	columns, warnings := e.InferOQLTypes(stmt.Query.RawQuery, stmt.Attributes)
	errors = append(errors, warnings...)

	// Match columns to declared attributes by position (OQL columns map 1:1 to attributes)
	for i, attr := range stmt.Attributes {
		if i >= len(columns) {
			break
		}
		col := columns[i]

		// Skip if we couldn't infer the type
		if col.InferredType.Kind == ast.TypeUnknown {
			continue
		}

		// Compare types
		if !typesCompatible(attr.Type, col.InferredType) {
			errors = append(errors, fmt.Sprintf(
				"attribute '%s': declared as %s but OQL expression '%s' returns %s. Fix: change to '%s: %s'",
				attr.Name,
				formatDataTypeForError(attr.Type),
				col.Expression,
				formatDataTypeForError(col.InferredType),
				attr.Name,
				formatDataTypeForMDL(col.InferredType)))
		}
	}

	return errors
}

// extractSelectClause extracts the SELECT clause from an OQL query.
// Handles subqueries by tracking parenthesis depth to find the main FROM clause.
func extractSelectClause(oql string) string {
	// Normalize whitespace
	oql = strings.TrimSpace(oql)
	upperOql := strings.ToUpper(oql)

	// Find SELECT keyword
	selectIdx := strings.Index(upperOql, "SELECT")
	if selectIdx == -1 {
		return ""
	}

	// Start after SELECT keyword
	startIdx := selectIdx + 6 // len("SELECT")

	// Find the main FROM clause or UNION (not inside subqueries)
	depth := 0
	for i := startIdx; i < len(oql); i++ {
		ch := oql[i]
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 {
				// Check for FROM keyword at depth 0
				if i+4 <= len(oql) {
					word := strings.ToUpper(oql[i : i+4])
					if word == "FROM" {
						// Make sure it's a word boundary (not part of another identifier)
						prevOk := i == startIdx || !isIdentChar(oql[i-1])
						nextOk := i+4 >= len(oql) || !isIdentChar(oql[i+4])
						if prevOk && nextOk {
							return strings.TrimSpace(oql[startIdx:i])
						}
					}
				}
				// Check for UNION keyword at depth 0 (ends current query term)
				if i+5 <= len(oql) {
					word := strings.ToUpper(oql[i : i+5])
					if word == "UNION" {
						prevOk := i == startIdx || !isIdentChar(oql[i-1])
						nextOk := i+5 >= len(oql) || !isIdentChar(oql[i+5])
						if prevOk && nextOk {
							return strings.TrimSpace(oql[startIdx:i])
						}
					}
				}
			}
		}
	}

	return ""
}

// isIdentChar returns true if ch is a valid identifier character.
func isIdentChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_'
}

// hasTopLevelKeyword checks if a SQL keyword appears at parenthesis depth 0 in the OQL query.
// This ensures keywords inside subqueries are not matched.
func hasTopLevelKeyword(oql string, keyword string) bool {
	upper := strings.ToUpper(keyword)
	kLen := len(upper)
	depth := 0
	for i := 0; i < len(oql); i++ {
		switch oql[i] {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && i+kLen <= len(oql) {
				word := strings.ToUpper(oql[i : i+kLen])
				if word == upper {
					prevOk := i == 0 || !isIdentChar(oql[i-1])
					nextOk := i+kLen >= len(oql) || !isIdentChar(oql[i+kLen])
					if prevOk && nextOk {
						return true
					}
				}
			}
		}
	}
	return false
}

// hasTopLevelPhrase checks if a two-word SQL phrase (like "ORDER BY") appears at parenthesis depth 0.
// This avoids false positives from entity names containing keywords (e.g., "Orders.Order").
func hasTopLevelPhrase(oql string, phrase string) bool {
	// Use regex to match the phrase as whole words at depth 0
	// First, extract top-level content (outside subqueries)
	var topLevel strings.Builder
	depth := 0
	for _, ch := range oql {
		switch ch {
		case '(':
			depth++
			if depth == 1 {
				topLevel.WriteRune(' ') // Replace subquery with space
			}
		case ')':
			depth--
		default:
			if depth == 0 {
				topLevel.WriteRune(ch)
			}
		}
	}

	// Check for phrase as whole words (case-insensitive)
	pattern := `(?i)\b` + regexp.QuoteMeta(phrase) + `\b`
	matched, _ := regexp.MatchString(pattern, topLevel.String())
	return matched
}

// parseSelectColumns splits the SELECT clause into individual column expressions.
func parseSelectColumns(selectClause string) []string {
	var columns []string
	var current strings.Builder
	depth := 0

	for _, ch := range selectClause {
		switch ch {
		case '(':
			depth++
			current.WriteRune(ch)
		case ')':
			depth--
			current.WriteRune(ch)
		case ',':
			if depth == 0 {
				columns = append(columns, strings.TrimSpace(current.String()))
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	// Add last column
	if current.Len() > 0 {
		columns = append(columns, strings.TrimSpace(current.String()))
	}

	return columns
}

// inferTypeFromExpression infers the data type from an OQL expression.
func (e *Executor) inferTypeFromExpression(expr string, col *OQLColumnInfo, aliasMap map[string]string) ast.DataType {
	expr = strings.TrimSpace(expr)

	// Check for aggregate functions
	if aggType := e.inferAggregateType(expr, col, aliasMap); aggType.Kind != ast.TypeUnknown {
		return aggType
	}

	// Check for attribute reference: [Entity/Attribute] or [Module.Entity/Attribute]
	attrPattern := regexp.MustCompile(`^\[([^\]]+)\]$`)
	if match := attrPattern.FindStringSubmatch(expr); match != nil {
		return e.inferAttributeType(match[1], col)
	}

	// Check for MDL-style alias.attribute reference (e.g., p.Name, o.OrderDate)
	aliasAttrPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)$`)
	if match := aliasAttrPattern.FindStringSubmatch(expr); match != nil {
		alias := match[1]
		attrName := match[2]
		if entityName, ok := aliasMap[alias]; ok {
			// Resolve alias to entity and look up attribute
			col.SourceEntity = entityName
			col.SourceAttr = attrName
			return e.inferAttributeTypeFromEntity(entityName, attrName)
		}
	}

	// Check for string literal
	if strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'") {
		return ast.DataType{Kind: ast.TypeString, Length: len(expr) - 2}
	}

	// Check for numeric literal
	if regexp.MustCompile(`^-?\d+$`).MatchString(expr) {
		return ast.DataType{Kind: ast.TypeInteger}
	}
	if regexp.MustCompile(`^-?\d+\.\d+$`).MatchString(expr) {
		return ast.DataType{Kind: ast.TypeDecimal}
	}

	// Fallback to static inference for expressions not handled above (CASE, boolean literals, etc.)
	if staticType := inferTypeStatic(expr); staticType.Kind != ast.TypeUnknown {
		return staticType
	}

	// Unknown type
	return ast.DataType{Kind: ast.TypeUnknown}
}

// inferAttributeTypeFromEntity looks up an attribute's type from a qualified entity name.
func (e *Executor) inferAttributeTypeFromEntity(entityQualifiedName, attrName string) ast.DataType {
	parts := strings.Split(entityQualifiedName, ".")
	if len(parts) != 2 {
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	moduleName := parts[0]
	entityName := parts[1]

	entity, err := e.findEntity(moduleName, entityName)
	if err != nil {
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	for _, attr := range entity.Attributes {
		if attr.Name == attrName {
			return convertDomainModelTypeToAST(attr.Type)
		}
	}

	return ast.DataType{Kind: ast.TypeUnknown}
}

// inferAggregateType infers the return type of aggregate functions.
func (e *Executor) inferAggregateType(expr string, col *OQLColumnInfo, aliasMap map[string]string) ast.DataType {
	upperExpr := strings.ToUpper(strings.TrimSpace(expr))

	// COUNT(*) or COUNT(expression) → Integer (Mendix OQL COUNT returns Integer)
	if strings.HasPrefix(upperExpr, "COUNT(") {
		col.IsAggregate = true
		col.AggregateFunc = "COUNT"
		return ast.DataType{Kind: ast.TypeInteger}
	}

	// SUM(expression) → preserves input type (Integer→Integer, else Decimal)
	if strings.HasPrefix(upperExpr, "SUM(") {
		col.IsAggregate = true
		col.AggregateFunc = "SUM"
		innerArg := extractFunctionArg(expr)
		if innerArg != "" {
			innerType := e.inferTypeFromExpression(innerArg, &OQLColumnInfo{}, aliasMap)
			if innerType.Kind == ast.TypeInteger || innerType.Kind == ast.TypeLong {
				return innerType
			}
		}
		return ast.DataType{Kind: ast.TypeDecimal}
	}

	// AVG(expression) → always Decimal
	if strings.HasPrefix(upperExpr, "AVG(") {
		col.IsAggregate = true
		col.AggregateFunc = "AVG"
		return ast.DataType{Kind: ast.TypeDecimal}
	}

	// MIN/MAX preserve the input type — try to resolve inner expression
	if strings.HasPrefix(upperExpr, "MIN(") || strings.HasPrefix(upperExpr, "MAX(") {
		col.IsAggregate = true
		col.AggregateFunc = strings.Split(upperExpr, "(")[0]
		innerArg := extractFunctionArg(expr)
		if innerArg != "" {
			innerType := e.inferTypeFromExpression(innerArg, &OQLColumnInfo{}, aliasMap)
			if innerType.Kind != ast.TypeUnknown {
				return innerType
			}
		}
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	// LENGTH returns Integer
	if strings.HasPrefix(upperExpr, "LENGTH(") {
		return ast.DataType{Kind: ast.TypeInteger}
	}

	// COALESCE - we'd need to analyze the arguments
	if strings.HasPrefix(upperExpr, "COALESCE(") {
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	return ast.DataType{Kind: ast.TypeUnknown}
}

// inferAttributeType looks up an attribute's type from the referenced entity.
func (e *Executor) inferAttributeType(attrPath string, col *OQLColumnInfo) ast.DataType {
	// Parse [Module.Entity/Attribute] or [Entity/Attribute]
	parts := strings.Split(attrPath, "/")
	if len(parts) != 2 {
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	entityPath := parts[0]
	attrName := parts[1]

	col.SourceAttr = attrName

	// Parse entity path (may be Module.Entity or just Entity with alias)
	entityParts := strings.Split(entityPath, ".")
	var moduleName, entityName string

	if len(entityParts) == 2 {
		moduleName = entityParts[0]
		entityName = entityParts[1]
		col.SourceEntity = entityPath
	} else {
		// Could be an alias - we can't resolve aliases without full OQL parsing
		col.SourceEntity = entityPath
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	// Look up the entity in the project
	entity, err := e.findEntity(moduleName, entityName)
	if err != nil {
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	// Find the attribute
	for _, attr := range entity.Attributes {
		if attr.Name == attrName {
			return convertDomainModelTypeToAST(attr.Type)
		}
	}

	return ast.DataType{Kind: ast.TypeUnknown}
}

// findEntity looks up an entity by module and name.
func (e *Executor) findEntity(moduleName, entityName string) (*domainmodel.Entity, error) {
	// Get all entities
	dms, err := e.reader.ListDomainModels()
	if err != nil {
		return nil, err
	}

	h, err := e.getHierarchy()
	if err != nil {
		return nil, err
	}

	for _, dm := range dms {
		modID := h.FindModuleID(dm.ID)
		modName := h.GetModuleName(modID)
		if modName != moduleName {
			continue
		}

		for _, entity := range dm.Entities {
			if entity.Name == entityName {
				return entity, nil
			}
		}
	}

	return nil, fmt.Errorf("entity not found: %s.%s", moduleName, entityName)
}

// convertDomainModelTypeToAST converts a domainmodel.AttributeType to ast.DataType.
func convertDomainModelTypeToAST(attrType domainmodel.AttributeType) ast.DataType {
	if attrType == nil {
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	switch t := attrType.(type) {
	case *domainmodel.StringAttributeType:
		return ast.DataType{Kind: ast.TypeString, Length: t.Length}
	case *domainmodel.IntegerAttributeType:
		return ast.DataType{Kind: ast.TypeInteger}
	case *domainmodel.LongAttributeType:
		return ast.DataType{Kind: ast.TypeLong}
	case *domainmodel.DecimalAttributeType:
		return ast.DataType{Kind: ast.TypeDecimal}
	case *domainmodel.BooleanAttributeType:
		return ast.DataType{Kind: ast.TypeBoolean}
	case *domainmodel.DateTimeAttributeType:
		return ast.DataType{Kind: ast.TypeDateTime}
	case *domainmodel.DateAttributeType:
		return ast.DataType{Kind: ast.TypeDate}
	case *domainmodel.AutoNumberAttributeType:
		return ast.DataType{Kind: ast.TypeAutoNumber}
	case *domainmodel.BinaryAttributeType:
		return ast.DataType{Kind: ast.TypeBinary}
	case *domainmodel.EnumerationAttributeType:
		var enumRef *ast.QualifiedName
		if t.EnumerationRef != "" {
			parts := strings.Split(t.EnumerationRef, ".")
			if len(parts) == 2 {
				enumRef = &ast.QualifiedName{Module: parts[0], Name: parts[1]}
			}
		}
		return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: enumRef}
	default:
		return ast.DataType{Kind: ast.TypeUnknown}
	}
}

// typesStrictlyCompatible checks if declared and inferred types match exactly.
// This is used for static OQL type checking where the inferred type is definitive
// (e.g., count() always returns Integer, sum() always returns Decimal).
// MxBuild treats Integer and Long as distinct types for VIEW entity sync validation,
// so we must not treat them as interchangeable here.
func typesStrictlyCompatible(declared, inferred ast.DataType) bool {
	if inferred.Kind == ast.TypeUnknown {
		return true
	}

	return declared.Kind == inferred.Kind
}

// typesCompatible checks if declared and inferred types are compatible.
func typesCompatible(declared, inferred ast.DataType) bool {
	// If inferred is unknown, we can't validate
	if inferred.Kind == ast.TypeUnknown {
		return true
	}

	// Same kind is always compatible
	if declared.Kind == inferred.Kind {
		// For strings, check length compatibility
		if declared.Kind == ast.TypeString {
			// Declared length should be >= inferred length
			if declared.Length > 0 && inferred.Length > 0 && declared.Length < inferred.Length {
				return false
			}
		}
		return true
	}

	// Integer and Long are compatible
	if (declared.Kind == ast.TypeInteger && inferred.Kind == ast.TypeLong) ||
		(declared.Kind == ast.TypeLong && inferred.Kind == ast.TypeInteger) {
		return true
	}

	// Decimal is compatible with Integer/Long (widening)
	if declared.Kind == ast.TypeDecimal && (inferred.Kind == ast.TypeInteger || inferred.Kind == ast.TypeLong) {
		return true
	}

	return false
}

// formatDataTypeForError formats a data type for error messages.
func formatDataTypeForError(dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeString:
		if dt.Length > 0 {
			return fmt.Sprintf("String(%d)", dt.Length)
		}
		return "String"
	case ast.TypeInteger:
		return "Integer"
	case ast.TypeLong:
		return "Long"
	case ast.TypeDecimal:
		return "Decimal"
	case ast.TypeBoolean:
		return "Boolean"
	case ast.TypeDateTime:
		return "DateTime"
	case ast.TypeDate:
		return "Date"
	case ast.TypeAutoNumber:
		return "AutoNumber"
	case ast.TypeEnumeration:
		if dt.EnumRef != nil {
			return fmt.Sprintf("Enumeration(%s)", dt.EnumRef.String())
		}
		return "Enumeration"
	default:
		return "Unknown"
	}
}

// formatDataTypeForMDL formats a data type as MDL syntax for "Fix:" suggestions.
func formatDataTypeForMDL(dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeString:
		if dt.Length > 0 {
			return fmt.Sprintf("String(%d)", dt.Length)
		}
		return "String(200)"
	case ast.TypeInteger:
		return "Integer"
	case ast.TypeLong:
		return "Long"
	case ast.TypeDecimal:
		return "Decimal"
	case ast.TypeBoolean:
		return "Boolean"
	case ast.TypeDateTime:
		return "DateTime"
	case ast.TypeDate:
		return "Date"
	case ast.TypeAutoNumber:
		return "AutoNumber"
	case ast.TypeEnumeration:
		if dt.EnumRef != nil {
			return dt.EnumRef.String()
		}
		return "Enumeration"
	default:
		return "Unknown"
	}
}

// extractFunctionArg extracts the inner argument from a function call like "func(expr)",
// handling nested parentheses. Returns empty string if not a function call.
func extractFunctionArg(expr string) string {
	idx := strings.Index(expr, "(")
	if idx < 0 || !strings.HasSuffix(expr, ")") {
		return ""
	}
	return strings.TrimSpace(expr[idx+1 : len(expr)-1])
}

// ValidateOQLSyntax checks for common OQL syntax mistakes.
// Returns a list of structured violations with rule IDs.
// This function can be called without an Executor instance.
func ValidateOQLSyntax(oql string) []linter.Violation {
	var violations []linter.Violation

	// Check for association paths using '.' instead of '/'
	assocDotPattern := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\.([A-Z][a-zA-Z0-9_]*)\.([A-Z][a-zA-Z0-9_]*_[A-Z][a-zA-Z0-9_]*)\b`)
	matches := assocDotPattern.FindAllStringSubmatch(oql, -1)
	for _, match := range matches {
		if len(match) >= 4 {
			wrongPath := match[0]
			alias := match[1]
			module := match[2]
			assocName := match[3]
			correctPath := fmt.Sprintf("%s/%s.%s", alias, module, assocName)
			violations = append(violations, linter.Violation{
				RuleID:   "MDL030",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"invalid association path '%s': association references must use '/' not '.'",
					wrongPath),
				Location:   linter.Location{DocumentType: "viewentity"},
				Suggestion: fmt.Sprintf("Use '%s' instead", correctPath),
			})
		}
	}

	// Check that all top-level SELECT columns have explicit AS aliases
	selectClause := extractSelectClause(oql)
	if selectClause != "" {
		columns := parseSelectColumns(selectClause)
		aliasPattern := regexp.MustCompile(`(?i)\s+AS\s+\w+\s*$`)
		for i, col := range columns {
			col = strings.TrimSpace(col)
			if col == "" {
				continue
			}
			if !aliasPattern.MatchString(col) {
				display := col
				if len(display) > 60 {
					display = strings.TrimSpace(display[:57]) + "..."
				}
				violations = append(violations, linter.Violation{
					RuleID:   "MDL030",
					Severity: linter.SeverityError,
					Message: fmt.Sprintf(
						"SELECT column %d has no AS alias: '%s'",
						i+1, display),
					Location:   linter.Location{DocumentType: "viewentity"},
					Suggestion: "All SELECT columns in a view entity must have an explicit alias (e.g., '... AS MyAlias')",
				})
			}
		}
	}

	// Check that top-level ORDER BY is accompanied by LIMIT
	if hasTopLevelPhrase(oql, "ORDER BY") && !hasTopLevelKeyword(oql, "LIMIT") {
		violations = append(violations, linter.Violation{
			RuleID:     "MDL030",
			Severity:   linter.SeverityError,
			Message:    "ORDER BY without LIMIT: view entity OQL queries that use ORDER BY must also specify a LIMIT clause",
			Location:   linter.Location{DocumentType: "viewentity"},
			Suggestion: "Add a LIMIT clause after ORDER BY",
		})
	}

	// Check for '/' used as division instead of ':'
	divisionPattern := regexp.MustCompile(`(\)|[0-9]|[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*)\s*/\s*([a-z_(0-9])`)
	divMatches := divisionPattern.FindAllStringSubmatch(oql, -1)
	for _, match := range divMatches {
		violations = append(violations, linter.Violation{
			RuleID:   "MDL030",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"'/' is the association traversal operator in OQL, not division. Found: '...%s / %s...'",
				match[1], match[2]),
			Location:   linter.Location{DocumentType: "viewentity"},
			Suggestion: "Use ':' for division (e.g., 'expr1 : expr2')",
		})
	}

	// Check for correlated subquery pattern
	correlatedPattern := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)/([A-Z][a-zA-Z0-9_]*\.[A-Z][a-zA-Z0-9_]*_[A-Z][a-zA-Z0-9_]*)\s*=\s*([a-zA-Z_][a-zA-Z0-9_]*)(?:\s|$|AND|OR|\))`)
	correlatedMatches := correlatedPattern.FindAllStringSubmatch(oql, -1)
	for _, match := range correlatedMatches {
		if len(match) >= 4 {
			alias := match[1]
			assocPath := match[2]
			targetAlias := match[3]
			violations = append(violations, linter.Violation{
				RuleID:   "MDL030",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"invalid association comparison '%s/%s = %s': cannot compare association to a bare entity alias",
					alias, assocPath, targetAlias),
				Location:   linter.Location{DocumentType: "viewentity"},
				Suggestion: fmt.Sprintf("Use '%s/%s = %s.ID' to compare against the entity's ID", alias, assocPath, targetAlias),
			})
		}
	}

	return violations
}
