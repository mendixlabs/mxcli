// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
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

// inferOQLTypes analyzes an OQL query and returns the expected types for each column.
func inferOQLTypes(ctx *ExecContext, oqlQuery string, declaredAttrs []ast.ViewAttribute) ([]OQLColumnInfo, []string) {
	var warnings []string
	var columns []OQLColumnInfo
	oqlQuery = stripOQLComments(oqlQuery)

	// Extract SELECT clause
	selectClause := extractSelectClause(oqlQuery)
	if selectClause == "" {
		warnings = append(warnings,
			"could not parse select clause from OQL query, so no column was type-checked")
		return columns, warnings
	}

	// Extract FROM clause and build alias map
	aliasMap := extractAliasMap(oqlQuery)

	// An `<alias>.ID` column declares an ASSOCIATION, not an attribute, so it has
	// no declared attribute to line up with. Dropping it here is what makes the
	// remaining columns correspond one-to-one — see attributeSelectColumns.
	columnExprs := attributeSelectColumns(oqlQuery, parseSelectColumns(selectClause))
	if len(columnExprs) != len(declaredAttrs) {
		warnings = append(warnings, fmt.Sprintf(
			"OQL select has %d columns but %d attributes declared",
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
		if aliasMatch := oqlAliasSuffixRe.FindStringSubmatch(expr); aliasMatch != nil {
			col.Alias = unquoteOQLIdent(aliasMatch[1])
			expr = strings.TrimSuffix(expr, aliasMatch[0])
			col.Expression = strings.TrimSpace(expr)
		}

		// Infer type from expression
		col.InferredType = inferTypeFromExpression(ctx, expr, &col, aliasMap)

		columns = append(columns, col)
	}

	return columns, warnings
}

// extractAliasMap parses the FROM clause to build a map of alias -> qualified entity name.
func extractAliasMap(oql string) map[string]string {
	aliasMap := make(map[string]string)

	// Match FROM Entity AS alias or FROM Entity alias patterns
	// Also handles JOIN clauses.
	//
	// Either half of the qualified name may be QUOTED, and that is not exotic:
	// an OQL reserved word has to be quoted to survive MxBuild (CE0174), so
	// `from Mappings."Order" as o` is the only way to write a source entity
	// called Order. Matching bare names only left `o` unresolved — no type
	// inference for its columns, and `o.ID` unrecognisable as an association
	// column, which is exactly the reported example (FINDINGS §1).
	fromPattern := regexp.MustCompile(`(?i)\b(?:from|join)\s+(` + oqlIdent + `\.` + oqlIdent + `)\s+(?:as\s+)?([A-Za-z_][A-Za-z0-9_]*)`)
	matches := fromPattern.FindAllStringSubmatch(oql, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			entityName := unquoteQualifiedOQLName(match[1])
			alias := match[2]
			aliasMap[alias] = entityName
		}
	}

	// An ASSOCIATION-PATH join — `join r/Mod.Reading_Meter/Mod.Meter as m` —
	// binds its alias to the entity at the END of the path. The pattern above
	// cannot see it, because what follows the keyword is a path rather than a
	// qualified name, so `m` resolved to nothing: no type inference for any of
	// its columns, and `m.ID` unrecognisable as an association column. That join
	// form is the ordinary way to reach a related entity in Mendix OQL.
	//
	// The final segment is captured as its own group. It used to be recovered
	// by trimming the alias and a literal "as" off the match, which was
	// case-sensitive while the pattern is not — so `... AS m`, the spelling
	// DESCRIBE prints, left the alias unresolved (#652).
	pathPattern := regexp.MustCompile(
		`(?i)\b(?:from|join)\s+[A-Za-z_]\w*(?:/` + oqlIdent + `\.` + oqlIdent + `)*/(` +
			oqlIdent + `\.` + oqlIdent + `)\s+(?:as\s+)?([A-Za-z_]\w*)`)
	for _, match := range pathPattern.FindAllStringSubmatch(oql, -1) {
		aliasMap[match[2]] = unquoteQualifiedOQLName(match[1])
	}

	return aliasMap
}

// oqlIdent matches one OQL identifier in either spelling: bare, or quoted.
//
// A quoted alias exists because an OQL reserved word has to be quoted to
// survive MxBuild (CE0174), and a view entity's declared attribute name IS its
// select alias — so a view column called `Month` can only be written `as
// "Month"`. Every alias-detection pattern below has to know that form, or a
// quoted alias reads as NO alias and MDL030 reports "select column N has no as
// alias" on a query that has one.
const oqlIdent = `(?:"[^"\r\n]*"|` + "`[^`\r\n]*`" + `|\w+)`

// oqlAliasSuffixRe matches a trailing `AS <alias>` on a select column, with the
// alias captured. Built once from oqlIdent so the five places that strip or read
// an alias cannot disagree about what an alias looks like.
var oqlAliasSuffixRe = regexp.MustCompile(`(?i)\s+as\s+(` + oqlIdent + `)\s*$`)

// unquoteQualifiedOQLName strips quoting from each half of a Module.Entity name,
// so `Mappings."Order"` and `Mappings.Order` resolve to the same entity. The
// STORED query keeps its quotes — MxBuild needs them — but the name the alias
// denotes is the bare one.
func unquoteQualifiedOQLName(qn string) string {
	parts := strings.SplitN(qn, ".", 2)
	if len(parts) != 2 {
		return unquoteOQLIdent(qn)
	}
	return unquoteOQLIdent(parts[0]) + "." + unquoteOQLIdent(parts[1])
}

// unquoteOQLIdent strips the quoting from an OQL identifier, so an alias can be
// compared against a declared attribute name. The stored QUERY keeps its quotes
// — they are what MxBuild needs — but the NAME the alias denotes is the bare
// word.
// isQuotedOQLIdent reports whether an identifier carries MDL quoting.
func isQuotedOQLIdent(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return false
	}
	return (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`')
}

func unquoteOQLIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ValidateOQLTypes performs static type checking of OQL SELECT expressions against
// declared view entity attributes. Does not require a project connection — it only
// checks types that can be inferred from the OQL syntax itself (aggregate functions,
// CASE expressions, literals, datepart, etc.).
func ValidateOQLTypes(oql string, attrs []ast.ViewAttribute) []linter.Violation {
	var violations []linter.Violation
	oql = stripOQLComments(oql)

	selectClause := extractSelectClause(oql)
	if selectClause == "" {
		return violations
	}

	// Same skip as inferOQLTypes: an association column has no declared
	// attribute, and leaving it in shifts every attribute onto its neighbour.
	columnExprs := attributeSelectColumns(oql, parseSelectColumns(selectClause))

	for i, expr := range columnExprs {
		if i >= len(attrs) {
			break
		}

		// Strip AS alias
		if aliasMatch := oqlAliasSuffixRe.FindStringSubmatch(expr); aliasMatch != nil {
			expr = strings.TrimSuffix(expr, aliasMatch[0])
		}
		expr = strings.TrimSpace(expr)

		inferred := inferTypeStatic(expr)
		if inferred.Kind == ast.TypeUnknown {
			continue
		}
		// A DERIVED string column (CAST AS string, a string literal/expression,
		// a CASE whose first branch is one of those) always maps to Mendix's
		// default length (200) in a view entity — a plain attribute reference,
		// and a CASE whose first branch is one, infers TypeUnknown and is
		// skipped above, so any concrete String here is derived. Declaring another
		// length makes mxbuild report CE6770 "out of sync"; normalize so the check
		// flags that mismatch pre-build.
		if inferred.Kind == ast.TypeString {
			inferred.Length = derivedStringLength
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
					if aliasMatch := oqlAliasSuffixRe.FindStringSubmatch(col); aliasMatch != nil {
						col = strings.TrimSuffix(col, aliasMatch[0])
					}
					return inferTypeStatic(strings.TrimSpace(col))
				}
			}
		}
	}

	// String concatenation: a top-level `+` with a string operand is a derived
	// string, String(200) in mxbuild whatever the operands' lengths —
	// `r.Name + ' x'` over a String(100) Name builds only when the view
	// declares String(200); `string` and `string(100)` are CE6770 (measured,
	// 11.13.0, ako/mxcli#981). Checked before the prefix rules below, which
	// would otherwise read `cast(…) + ' x'` as the cast alone.
	if parts := splitTopLevelPlus(expr); len(parts) > 1 {
		for _, p := range parts {
			if p != "" && inferTypeStatic(p).Kind == ast.TypeString {
				return ast.DataType{Kind: ast.TypeString, Length: derivedStringLength}
			}
		}
	}

	// count(...) → Integer (Mendix OQL COUNT returns Integer)
	if strings.HasPrefix(upper, "COUNT(") {
		return ast.DataType{Kind: ast.TypeInteger}
	}

	// sum(...) → preserves input type. When the argument type can't be inferred
	// statically (a bare attribute reference — the common case), we CANNOT know
	// whether the sum is Integer/Long/Decimal, so return Unknown and skip the
	// column rather than guessing Decimal (which false-positives on sum() over an
	// integer attribute declared as integer/long).
	if strings.HasPrefix(upper, "SUM(") {
		innerArg := extractFunctionArg(expr)
		if innerArg != "" {
			innerType := inferTypeStatic(innerArg)
			if innerType.Kind != ast.TypeUnknown {
				return innerType
			}
		}
		return ast.DataType{Kind: ast.TypeUnknown}
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

	// string_agg(...) → depends on the project's Mendix version, which this
	// path does not know: String(200) on one, its argument's type on another
	// (see stringAggType). Unknown, so the column is not judged here.
	if strings.HasPrefix(upper, "STRING_AGG(") {
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

	// CAST(expr AS type) → the target type. Mendix normalizes a derived string
	// column to its default length (200); the view-entity validator applies that.
	if strings.HasPrefix(upper, "CAST(") {
		return castTargetType(expr)
	}

	// CASE expression: infer from its result branches
	if strings.HasPrefix(upper, "CASE") {
		return inferCaseType(expr, func(_ int, branch string) ast.DataType { return inferTypeStatic(branch) })
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

// derivedStringLength is the length Mendix assigns to a DERIVED string column in
// a view entity (CAST(x AS string), a string literal or expression, a CASE
// whose first branch is one of those). It is the platform default (200); declaring any other length for
// such a column makes mxbuild report CE6770 "View Entity is out of sync".
const derivedStringLength = 200

// firstOQLArgument returns the first argument of a function's argument list:
// the text before the first comma outside parentheses and string literals, so
// the separator of string_agg(case when … then 'a,b' end, ', ') is not mistaken
// for part of the expression.
func firstOQLArgument(args string) string {
	depth := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case '\'':
			for i++; i < len(args) && args[i] != '\''; i++ {
			}
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return strings.TrimSpace(args[:i])
			}
		}
	}
	return strings.TrimSpace(args)
}

// stringAggType is the type of a STRING_AGG(expression, 'separator') column.
// Mendix changed it in 11.7 (ako/mxcli#1650). Measured with mx check over
// String(100), String(500) and unlimited attributes:
//
//	11.2.0 – 11.6.12   the type of its argument: String(100) over a String(100)
//	                   attribute, unlimited over an unlimited one — exactly, as
//	                   for a pass-through column; String(200) is CE6770
//	11.7.0 – 11.15.0,  a derived String(200), whatever it aggregates; unlimited,
//	10.24.22 – 10.24.24  String(100) or String(500) are CE6770
//
// Below 11.2 (and 10.24.22) there is no STRING_AGG at all — the version gate.
// argType infers the argument; the caller's column takes its pass-through
// source from it, which holds the 11.2–11.6 era to the exact-length rule.
// Unknown without a project version: neither answer is right for both eras.
func stringAggType(pv *types.ProjectVersion, argType func() ast.DataType) ast.DataType {
	if pv == nil || pv.MajorVersion == 0 {
		return ast.DataType{Kind: ast.TypeUnknown}
	}
	if pv.MajorVersion == 10 || pv.IsAtLeastFull(11, 7, 0) {
		return ast.DataType{Kind: ast.TypeString, Length: derivedStringLength}
	}
	return argType()
}

// castTargetType maps a CAST(expr AS type) to the target Mendix data type. Only
// the target matters for view-entity column typing; the source expression is
// ignored. Unknown targets return TypeUnknown (the column is then not validated).
func castTargetType(expr string) ast.DataType {
	m := regexp.MustCompile(`(?is)\bAS\s+([A-Za-z]+)\s*\)\s*$`).FindStringSubmatch(expr)
	if m == nil {
		return ast.DataType{Kind: ast.TypeUnknown}
	}
	switch strings.ToLower(m[1]) {
	case "string":
		return ast.DataType{Kind: ast.TypeString, Length: derivedStringLength}
	case "integer", "int":
		return ast.DataType{Kind: ast.TypeInteger}
	case "long":
		return ast.DataType{Kind: ast.TypeLong}
	case "decimal", "float":
		return ast.DataType{Kind: ast.TypeDecimal}
	case "boolean", "bool":
		return ast.DataType{Kind: ast.TypeBoolean}
	case "datetime", "date":
		return ast.DataType{Kind: ast.TypeDateTime}
	default:
		return ast.DataType{Kind: ast.TypeUnknown}
	}
}

// caseResultExprs returns a CASE expression's result branches in source order:
// each THEN's expression, then the ELSE's if there is one. Nil for a nested
// CASE, whose inner THENs the patterns would mistake for the outer's.
func caseResultExprs(expr string) []string {
	if strings.Count(strings.ToUpper(expr), "CASE") > 1 {
		return nil
	}
	var out []string
	for _, m := range caseThenRe.FindAllStringSubmatch(expr, -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	if m := caseElseRe.FindStringSubmatch(expr); m != nil {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

var (
	caseThenRe = regexp.MustCompile(`(?is)\bTHEN\s+(.+?)(?:\s+when\b|\s+else\b|\s+end\b)`)
	caseElseRe = regexp.MustCompile(`(?is)\bELSE\s+(.+?)\s+end\b`)
)

// numericRank orders the numeric kinds a CASE widens across; 0 is not numeric.
func numericRank(k ast.DataTypeKind) int {
	switch k {
	case ast.TypeInteger:
		return 1
	case ast.TypeLong:
		return 2
	case ast.TypeDecimal:
		return 3
	}
	return 0
}

// inferCaseType derives a CASE column's type from its result branches, typing
// each with infer (i is the branch's position, 0 the first THEN). Mendix's two
// rules, measured with mx check 11.12.5 on one-column views declared at every
// candidate type (ako/mxcli#1294):
//
//   - A String CASE takes its FIRST branch's type, length included.
//     `then l.Code else l.Name` (8 and 100) is String(8), `then l.Name else
//     l.Code` String(100), and `then l.Code … else '0000'` String(8) — a
//     derived String(200) only when the first branch is itself derived.
//   - A numeric CASE widens across ALL branches, Integer < Long < Decimal:
//     `then l.IntV else l.LngV` is Long either way round, `then 1 else 2.5`
//     Decimal.
//
// Taking the first branch that could be typed instead of the first branch made
// MDL031 demand String(200) for a CASE whose first branch is a String(8)
// attribute — an attribute is untypable without the project — and its fix was
// CE6770. A branch infer cannot type leaves the result unknown wherever it
// could decide it: a first branch that is not a typed String, or a numeric
// branch that could be wider than the rest.
func inferCaseType(expr string, infer func(i int, branch string) ast.DataType) ast.DataType {
	var widest, other ast.DataType
	untyped := false
	for i, branch := range caseResultExprs(expr) {
		t := infer(i, branch)
		switch {
		case i == 0 && t.Kind == ast.TypeString:
			return t
		case t.Kind == ast.TypeUnknown:
			untyped = true
		case numericRank(t.Kind) > 0:
			if numericRank(t.Kind) > numericRank(widest.Kind) {
				widest = t
			}
		case other.Kind == ast.TypeUnknown:
			other = t
		}
	}
	switch {
	case widest.Kind != ast.TypeUnknown && other.Kind != ast.TypeUnknown:
		return ast.DataType{Kind: ast.TypeUnknown} // mixed kinds: not a column mxbuild builds
	case widest.Kind != ast.TypeUnknown:
		if untyped && widest.Kind != ast.TypeDecimal {
			return ast.DataType{Kind: ast.TypeUnknown}
		}
		return widest
	case other.Kind == ast.TypeString:
		// A later branch is a string, but the length is the first branch's.
		return ast.DataType{Kind: ast.TypeUnknown}
	}
	return other
}

// passthroughStringLengthMismatch reports whether a PASS-THROUGH view-entity
// column (a bare source-attribute reference, so sourceAttr is set) declares a
// string length different from the source attribute's inherited length. Mendix's
// view-entity sync requires an exact match — a mismatch (including declaring an
// unbounded `string`, Length 0, against a `string(100)` source) fails the build
// with CE6770. This is stricter than typesCompatible's >= truncation guard, so
// pass-through strings are checked separately. sourceAttr is empty for aggregates
// and derived expressions, which this correctly ignores. (ledger finding #36)
func passthroughStringLengthMismatch(declared, inferred ast.DataType, sourceAttr string) bool {
	return sourceAttr != "" &&
		declared.Kind == ast.TypeString && inferred.Kind == ast.TypeString &&
		// A source length of 0 means mxcli does not KNOW the length, not that the
		// source is unbounded — today that is every System.* string, because the
		// metadata carries none (ako/mxcli#584). Judging against 0 refused every
		// declaration except `String`, which is the one spelling that definitely
		// fails the build. Measured on mxbuild 11.14.0 against System.User.Name,
		// which is String(100):
		//
		//	String(200)  check: refused   mxbuild: CE6770
		//	String(100)  check: refused   mxbuild: 0 errors   ← the correct one
		//	String       check: passed    mxbuild: CE6770
		//
		// So the rule blocked the right answer and waved through the wrong one.
		// It now declines to judge what it cannot measure; #584 restores the
		// judgement by giving it the lengths.
		inferred.Length > 0 &&
		declared.Length != inferred.Length
}

// passthroughLengthError words the pass-through length refusal, for the only
// case the rule now fires in: a source length mxcli actually knows.
//
// The unknown case used to be worded from the same string, reporting "inherits
// length 0" as though 0 were the source's length and then prescribing a
// hardcoded String(200) from formatDataTypeForMDL — which, for System.User.Name
// on 11.14.0, is precisely the value mxbuild rejects (ako/mxcli#585). It is
// no longer reachable: passthroughStringLengthMismatch declines an unknown
// length rather than guessing at it.
//
// The hardcoded String(200) in formatDataTypeForMDL is untouched and still
// right where it is used: a DERIVED string column is String(200) by rule
// whatever its source.
//
// A String CASE whose first branch is an attribute reaches here too, since it
// inherits that branch's length (inferCaseType), and is named as what it is.
func passthroughLengthError(attrName string, declared, inferred ast.DataType, expression, sourceEntity, sourceAttr string) string {
	column, via := "pass-through column", ""
	switch upper := strings.ToUpper(expression); {
	case strings.HasPrefix(upper, "CASE"):
		column, via = "CASE column", " through its first branch"
	case strings.HasPrefix(upper, "STRING_AGG("):
		// Before 11.7 only — see stringAggType.
		column, via = "STRING_AGG column", " through its argument"
	}
	return fmt.Sprintf(
		"attribute '%s': declared as %s but %s '%s' inherits length %d%s from source attribute %s.%s — Mendix requires an exact length match (CE6770 \"View Entity out of sync\"). Fix: change to '%s: %s'",
		attrName, formatDataTypeForError(declared), column, expression, inferred.Length, via,
		sourceEntity, sourceAttr, attrName, formatDataTypeForMDL(inferred))
}

// viewAutoNumberRefused is the language change for ako/mxcli#981 item 4: a view
// attribute declared AutoNumber over an AutoNumber column. Mendix reports
// CE6770 "View Entity is out of sync with the OQL Query" for it (measured, mx
// check 11.13.0); the column is a Long in the view. check accepted it on
// purpose until a header could carry the rejection (ADR-0011).
var viewAutoNumberRefused = langver.Change{
	Code:  "MDL-V1-VIEWAUTONUMBER",
	Since: langver.V1,
	Old: "a view entity attribute declared `autonumber` over an AutoNumber column passes check, " +
		"and mxbuild reports CE6770 \"View Entity is out of sync with the OQL Query\"",
	New: "a check error that names the attribute and suggests `long`, the type the view gives the column",
}

// validateViewEntityTypes validates that declared attribute types match
// inferred OQL types. The warnings are the findings a headerless script keeps
// at their old meaning (viewAutoNumberRefused).
func validateViewEntityTypes(ctx *ExecContext, stmt *ast.CreateViewEntityStmt) (errors, warnings []string) {

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

	columns, warnings := inferOQLTypes(ctx, stmt.Query.RawQuery, stmt.Attributes)
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

		// A PASS-THROUGH string column (a bare source-attribute reference, e.g.
		// `c.Name`) inherits the source attribute's length verbatim, and Mendix's
		// view-entity sync requires the declared length to match it EXACTLY —
		// declaring `string` (unbounded) or a different `string(N)` against a
		// `string(100)` source fails the build with CE6770 "View Entity out of sync".
		// This is stricter than typesCompatible's >= rule (which only guards
		// truncation), so pass-through strings are validated here. col.SourceAttr is
		// set only for direct attribute references, never for aggregates/derived
		// expressions. (ledger finding #36)
		if passthroughStringLengthMismatch(attr.Type, col.InferredType, col.SourceAttr) {
			errors = append(errors, passthroughLengthError(
				attr.Name, attr.Type, col.InferredType, col.Expression, col.SourceEntity, col.SourceAttr))
			continue
		}

		if attr.Type.Kind == ast.TypeAutoNumber && col.InferredType.Kind == ast.TypeAutoNumber {
			msg := fmt.Sprintf(
				"attribute '%s': declared as AutoNumber over the AutoNumber column '%s' — a view reads the "+
					"column as a Long, and mxbuild reports CE6770 \"View Entity is out of sync with the OQL Query\". "+
					"Fix: change to '%s: Long'", attr.Name, col.Expression, attr.Name)
			if viewAutoNumberRefused.Applies(ctx.LanguageVersion) {
				errors = append(errors, msg)
			} else {
				warnings = append(warnings, fmt.Sprintf("[%s] view entity %s: %s. %s",
					viewAutoNumberRefused.Code, stmt.Name.String(), msg,
					viewAutoNumberRefused.Warning(ctx.LanguageVersion)))
			}
			continue
		}

		// Compare types. A derived STRING_AGG column (11.7+, 10.24.22+) is
		// String(200) exactly, like every derived string — the static rule
		// cannot apply it, because only here is the version known
		// (stringAggType). Unlimited and String(500) are CE6770 too, which the
		// "long enough" rule below would let through.
		compatible := typesCompatible(attr.Type, col.InferredType)
		if col.AggregateFunc == "string_agg" && col.SourceAttr == "" {
			compatible = typesStrictlyCompatible(attr.Type, col.InferredType)
		}
		if !compatible {
			errors = append(errors, fmt.Sprintf(
				"attribute '%s': declared as %s but OQL expression '%s' returns %s. Fix: change to '%s: %s'",
				attr.Name,
				formatDataTypeForError(attr.Type),
				col.Expression,
				formatDataTypeForError(col.InferredType),
				attr.Name,
				formatDataTypeForMDL(viewColumnType(col.InferredType))))
		}
	}

	return errors, warnings
}

// Mendix OQL has TWO clause orders and the MDL grammar accepts both
// (`oqlQueryTerm`, MDLCatalog.g4):
//
//	SELECT … FROM … GROUP BY …        -- select first; FROM ends the list
//	FROM … GROUP BY … SELECT …        -- Mendix's own canonical spelling
//
// The second is not exotic: it is what Studio Pro stores, so it is what
// `DESCRIBE ENTITY` hands back, and describe → check → exec goes through it
// every time.
//
// The two take DIFFERENT terminator sets, and deliberately so. In the
// select-first order FROM ends the list and ORDER/LIMIT cannot — they sit past
// the FROM — so admitting them there would cut `select o.Limit as Limit from …`
// in half and report the remaining columns against the wrong attributes. Only
// the from-first order needs the wider set, because there the list runs to the
// end of the query.
var (
	selectFirstTerminators = []string{"FROM", "UNION"}
	fromFirstTerminators   = []string{"UNION", "ORDER BY", "LIMIT", "OFFSET"}
)

// extractSelectClause extracts the SELECT clause from an OQL query, in either
// clause order. Empty string means the query has no readable select list —
// callers MUST treat that as "could not read", never as "nothing to check":
// every column-level rule hangs off this one string, so a silent "" turns the
// whole view-entity checker off. ValidateOQLSyntax reports it for that reason.
func extractSelectClause(oql string) string {
	clause, _ := extractSelectClauseOK(oql)
	return clause
}

// extractSelectClauseOK is extractSelectClause plus whether a top-level SELECT
// was found at all. The two outcomes need separating because they call for
// different things: no SELECT is a query the grammar would already have
// rejected, while a SELECT whose list could not be read is a checker that has
// quietly stopped checking.
func extractSelectClauseOK(oql string) (string, bool) {
	// Normalize whitespace
	oql = strings.TrimSpace(oql)
	upperOql := strings.ToUpper(oql)

	// Find the top-level SELECT. Depth- and quote-aware, so a subquery's SELECT
	// in a from-first query (`from (select …) as t select …`) is not mistaken
	// for the outer one.
	selectIdx := topLevelKeywordIndex(oql, upperOql, 0, "SELECT")
	if selectIdx < 0 {
		return "", false
	}
	startIdx := selectIdx + len("SELECT")

	// Which clause order this is, is decided by what comes FIRST, not by what
	// is present: a select-first query has a FROM too.
	terminators := selectFirstTerminators
	if fromIdx := topLevelKeywordIndex(oql, upperOql, 0, "FROM"); fromIdx >= 0 && fromIdx < selectIdx {
		terminators = fromFirstTerminators
	}

	endIdx := topLevelKeywordIndex(oql, upperOql, startIdx, terminators...)
	if endIdx < 0 {
		// No terminator: the list runs to the end of the query. That is the
		// ordinary shape of a from-first query (`from … select a as A`), and
		// for a select-first one it means a FROM-less query, whose column list
		// is likewise the rest — either way there is something to check, and
		// reporting "unreadable" here would be a checker refusing its own input.
		endIdx = len(oql)
	}
	return strings.TrimSpace(oql[startIdx:endIdx]), true
}

// topLevelKeywordIndex returns the byte offset of the first of words appearing
// at parenthesis depth 0, on a word boundary, at or after start — or -1.
//
// upperOql must be strings.ToUpper(oql) and words must already be upper-case:
// the comparison is uppercase-to-uppercase, because a lowercase needle against
// an upper-cased haystack never matches and made this whole family return ""
// for every query once already (bug 9b).
//
// Quoted runs are skipped. OQL takes double-quoted identifiers exactly as SQL
// does, and mxcli passes them through so a reserved word survives MxBuild —
// so `select s."Order" as OrderValue` is a query a user really writes, and
// matching the ORDER inside those quotes would cut the select list in half.
//
// A word containing a space is a PHRASE, and the space matches any run of
// whitespace: "ORDER BY" must be spelled that way rather than as "ORDER",
// because a bare ORDER also occurs as an ordinary name.
func topLevelKeywordIndex(oql, upperOql string, start int, words ...string) int {
	depth := 0
	for i := start; i < len(oql); i++ {
		switch c := oql[i]; c {
		case '(':
			depth++
			continue
		case ')':
			depth--
			continue
		case '\'', '"', '`':
			// Skip to the closing quote. A doubled quote (OQL's own escape,
			// 'it''s') reads as two adjacent runs, which lands in the same place.
			for i++; i < len(oql) && oql[i] != c; i++ {
			}
			continue
		}
		if depth != 0 {
			continue
		}
		for _, w := range words {
			end, ok := matchPhraseAt(oql, upperOql, i, w)
			if !ok {
				continue
			}
			prevOk := i == 0 || !isIdentChar(oql[i-1])
			nextOk := end >= len(oql) || !isIdentChar(oql[end])
			if prevOk && nextOk {
				return i
			}
		}
	}
	return -1
}

// matchPhraseAt reports whether phrase matches oql at offset i, and the offset
// just past the match. A space in phrase matches one or more whitespace
// characters; every other character is compared upper-case against upperOql.
func matchPhraseAt(oql, upperOql string, i int, phrase string) (int, bool) {
	for p := 0; p < len(phrase); p++ {
		if phrase[p] == ' ' {
			start := i
			for i < len(oql) && isOQLSpace(oql[i]) {
				i++
			}
			if i == start {
				return 0, false
			}
			continue
		}
		if i >= len(oql) || upperOql[i] != phrase[p] {
			return 0, false
		}
		i++
	}
	return i, true
}

func isOQLSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
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
func inferTypeFromExpression(ctx *ExecContext, expr string, col *OQLColumnInfo, aliasMap map[string]string) ast.DataType {
	expr = strings.TrimSpace(expr)

	// String concatenation is a derived String(200), whatever its operands —
	// see inferTypeStatic. Here an operand can also be a string attribute.
	if parts := splitTopLevelPlus(expr); len(parts) > 1 {
		for _, p := range parts {
			var scratch OQLColumnInfo
			if p != "" && inferTypeFromExpression(ctx, p, &scratch, aliasMap).Kind == ast.TypeString {
				return ast.DataType{Kind: ast.TypeString, Length: derivedStringLength}
			}
		}
	}

	// Check for aggregate functions
	if aggType := inferAggregateType(ctx, expr, col, aliasMap); aggType.Kind != ast.TypeUnknown {
		return aggType
	}

	// Check for attribute reference: [Entity/Attribute] or [Module.Entity/Attribute]
	attrPattern := regexp.MustCompile(`^\[([^\]]+)\]$`)
	if match := attrPattern.FindStringSubmatch(expr); match != nil {
		return inferAttributeType(ctx, match[1], col)
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
			return inferAttributeTypeFromEntity(ctx, entityName, attrName)
		}
	}

	// A string literal is a derived String(200) whatever its own length: `'abc'`
	// builds only declared String(200) (measured, mx check 11.12.5,
	// ako/mxcli#1294).
	if strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'") {
		return ast.DataType{Kind: ast.TypeString, Length: derivedStringLength}
	}

	// CASE: type each branch here as well, so an attribute branch resolves
	// against the project rather than leaving the column unknown. A String CASE
	// is its first branch, source attribute included — which puts it under the
	// pass-through rule's exact length match — so col goes to that branch.
	if strings.HasPrefix(strings.ToUpper(expr), "CASE") {
		return inferCaseType(expr, func(i int, branch string) ast.DataType {
			c := &OQLColumnInfo{}
			if i == 0 {
				c = col
			}
			return inferTypeFromExpression(ctx, branch, c, aliasMap)
		})
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
func inferAttributeTypeFromEntity(ctx *ExecContext, entityQualifiedName, attrName string) ast.DataType {
	parts := strings.Split(entityQualifiedName, ".")
	if len(parts) != 2 {
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	moduleName := parts[0]
	entityName := parts[1]

	entity, err := findEntity(ctx, moduleName, entityName)
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
func inferAggregateType(ctx *ExecContext, expr string, col *OQLColumnInfo, aliasMap map[string]string) ast.DataType {
	upperExpr := strings.ToUpper(strings.TrimSpace(expr))

	// COUNT(*) or COUNT(expression) → Integer (Mendix OQL COUNT returns Integer)
	if strings.HasPrefix(upperExpr, "COUNT(") {
		col.IsAggregate = true
		col.AggregateFunc = "count"
		return ast.DataType{Kind: ast.TypeInteger}
	}

	// SUM(expression) → preserves the input type, and stays UNKNOWN when that
	// type could not be resolved.
	//
	// Falling back to Decimal looks harmless and is not: the argument is
	// unresolvable exactly when the source entity is created by the same script
	// (check skips references to script-created objects), which is the common
	// shape for a view entity. Measured against mxbuild 11.6.6 on `sum(s.Units)`
	// where Units is an Integer attribute:
	//
	//	declared Integer -> 0 errors    <- what mxcli flagged
	//	declared Decimal -> CE6770      <- what mxcli's hint told the user to write
	//
	// So the guess inverted the truth and the "Fix:" would break a working
	// project. inferTypeStatic's SUM branch already says this in its own comment;
	// this path disagreed with it.
	if strings.HasPrefix(upperExpr, "SUM(") {
		col.IsAggregate = true
		col.AggregateFunc = "sum"
		innerArg := extractFunctionArg(expr)
		if innerArg != "" {
			innerType := inferTypeFromExpression(ctx, innerArg, &OQLColumnInfo{}, aliasMap)
			if innerType.Kind != ast.TypeUnknown {
				return innerType
			}
		}
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	// AVG(expression) → always Decimal
	if strings.HasPrefix(upperExpr, "AVG(") {
		col.IsAggregate = true
		col.AggregateFunc = "avg"
		return ast.DataType{Kind: ast.TypeDecimal}
	}

	// MIN/MAX preserve the input type — try to resolve inner expression
	if strings.HasPrefix(upperExpr, "MIN(") || strings.HasPrefix(upperExpr, "MAX(") {
		col.IsAggregate = true
		col.AggregateFunc = strings.Split(upperExpr, "(")[0]
		innerArg := extractFunctionArg(expr)
		if innerArg != "" {
			innerType := inferTypeFromExpression(ctx, innerArg, &OQLColumnInfo{}, aliasMap)
			if innerType.Kind != ast.TypeUnknown {
				return innerType
			}
		}
		return ast.DataType{Kind: ast.TypeUnknown}
	}

	if strings.HasPrefix(upperExpr, "STRING_AGG(") {
		col.IsAggregate = true
		col.AggregateFunc = "string_agg"
		return stringAggType(ctx.Backend.ProjectVersion(), func() ast.DataType {
			return inferTypeFromExpression(ctx, firstOQLArgument(extractFunctionArg(expr)), col, aliasMap)
		})
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
func inferAttributeType(ctx *ExecContext, attrPath string, col *OQLColumnInfo) ast.DataType {
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
	entity, err := findEntity(ctx, moduleName, entityName)
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
//
// It reads the named module's domain model alone. It used to list and convert
// every domain model of the project to find the one, and CREATE ASSOCIATION
// calls it four times (ako/mxcli#2160). The listing remains for what the
// direct read does not answer: a module the module list does not carry (the
// virtual System module, #610) and a backend that serves no single domain
// model.
func findEntity(ctx *ExecContext, moduleName, entityName string) (*domainmodel.Entity, error) {
	if dm := moduleDomainModel(ctx, moduleName); dm != nil {
		for _, entity := range dm.Entities {
			if entity != nil && entity.Name == entityName {
				return entity, nil
			}
		}
		return nil, mdlerrors.NewNotFound("entity", moduleName+"."+entityName)
	}

	// Get all entities
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return nil, err
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, err
	}

	for _, dm := range dms {
		// Resolve the owning module from the domain model's container (the
		// module ID), not by walking up from the DM's own unit ID. The virtual
		// System domain model is not a real unit, so it is absent from the
		// hierarchy's container-parent map and FindModuleID(dm.ID) would yield
		// an empty module name — making System entities unresolvable (#610).
		modName := h.GetModuleName(dm.ContainerID)
		if modName != moduleName {
			continue
		}

		for _, entity := range dm.Entities {
			if entity.Name == entityName {
				return entity, nil
			}
		}
	}

	return nil, mdlerrors.NewNotFound("entity", moduleName+"."+entityName)
}

// moduleDomainModel reads the domain model of the module with exactly this
// name, or returns nil when it cannot be read that way — the caller then lists
// the project, which is what every caller did before.
func moduleDomainModel(ctx *ExecContext, moduleName string) *domainmodel.DomainModel {
	if ctx == nil || ctx.Backend == nil || moduleName == "" {
		return nil
	}
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return nil
	}
	for _, m := range modules {
		if m == nil || m.Name != moduleName {
			continue
		}
		dm, err := ctx.Backend.GetDomainModel(m.ID)
		if err != nil || dm == nil {
			return nil
		}
		return dm
	}
	return nil
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
	case *domainmodel.HashedStringAttributeType:
		return ast.DataType{Kind: ast.TypeHashedString}
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
	if declared.Kind != inferred.Kind {
		return false
	}
	// For a derived string column the LENGTH must also match: Mendix's view-entity
	// sync (CE6770) requires the declared length to equal the OQL column's length
	// (200 for a derived column — see ValidateOQLTypes). Length 0 =
	// unlimited, which also mismatches the fixed 200.
	if inferred.Kind == ast.TypeString {
		return declared.Length == inferred.Length
	}
	return true
}

// typesCompatible checks if declared and inferred types are compatible.
// viewColumnType is the type a view entity attribute over a column of type t
// is declared with: an AutoNumber column is a Long there (see typesCompatible).
func viewColumnType(t ast.DataType) ast.DataType {
	if t.Kind == ast.TypeAutoNumber {
		t.Kind = ast.TypeLong
	}
	return t
}

func typesCompatible(declared, inferred ast.DataType) bool {
	// If inferred is unknown, we can't validate
	if inferred.Kind == ast.TypeUnknown {
		return true
	}

	// An AutoNumber source column is a Long in the view: the database fills
	// it, and the view entity only reads it. Mendix accepts the view attribute
	// declared Long, and reports CE6770 "View Entity is out of sync with the
	// OQL Query" for one declared AutoNumber (measured, mx check 11.13). Long
	// used to be refused here once the source entity existed — accepted on the
	// run that created it, refused on every run after (ako/mxcli#859,
	// rehearsal V1). A declared AutoNumber is accepted here: validateViewEntityTypes
	// refuses it under `mdl 1;` and warns without the header, because refusing
	// it is a new rejection, which ADR-0011 reserves for the header
	// (viewAutoNumberRefused, ako/mxcli#981).
	if inferred.Kind == ast.TypeAutoNumber {
		if declared.Kind == ast.TypeAutoNumber {
			return true
		}
		inferred = viewColumnType(inferred)
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

	// Numeric kinds are NOT interchangeable, in either direction. Integer and
	// Long used to be accepted for each other, and Decimal over either as a
	// "widening" — none of which mxbuild allows. Measured with mx check on
	// 11.14.0, one view per pairing over Integer, Long and AutoNumber columns,
	// both pass-through (`r.Col`) and aggregated (`sum`, `max`): every declared
	// kind other than the column's own is CE6770 "View Entity is out of sync
	// with the OQL Query" — Long over an Integer sum, Integer over a Long sum,
	// Decimal over an Integer sum or column, Integer or Decimal over an
	// AutoNumber. Only the same kind (and Long over AutoNumber, above) builds.
	// The widening made `sum(Integer) declared Long` pass check and exec and
	// fail the build (ako/mxcli#565), the very distinction typesStrictlyCompatible
	// already drew for the static path.
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
	oql = stripOQLComments(oql)

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

	// Check that all top-level SELECT columns have explicit AS aliases.
	//
	// An unreadable select list is reported rather than skipped. Every rule
	// below hangs off this one string, so returning "" used to switch the
	// column checks off without saying so — which is how the FROM-first clause
	// order went unnoticed: the only visible symptom was ONE bogus error from
	// inferOQLTypes, while MDL030 and MDL072 quietly stopped running. A checker
	// that cannot read its input has to say so.
	selectClause, hasSelect := extractSelectClauseOK(oql)
	if hasSelect && selectClause == "" {
		violations = append(violations, linter.Violation{
			RuleID:   "MDL030",
			Severity: linter.SeverityError,
			Message: "the OQL has a select clause but its column list could not be read, " +
				"so no column was checked (alias, type and length rules all skipped)",
			Location: linter.Location{DocumentType: "viewentity"},
			Suggestion: "Both clause orders are supported — `select … from …` and Mendix's own " +
				"`from … group by … select …`. If the query is one of those and still " +
				"lands here, it is an mxcli gap: please report it with the query.",
		})
	}
	if selectClause != "" {
		columns := parseSelectColumns(selectClause)
		aliasPattern := oqlAliasSuffixRe
		for i, col := range columns {
			col = strings.TrimSpace(col)
			if col == "" {
				continue
			}
			// MDL072: the alias is there but quoted, which OQL does not accept.
			// Reported before MDL030 for this column, because "has no as alias"
			// is actively misleading about a column that plainly has one.
			if m := aliasPattern.FindStringSubmatch(col); m != nil && isQuotedOQLIdent(m[1]) {
				bare := unquoteOQLIdent(m[1])
				violations = append(violations, linter.Violation{
					RuleID:   "MDL072",
					Severity: linter.SeverityError,
					Message: fmt.Sprintf(
						"select column %d has a quoted alias %s — OQL requires a bare identifier "+
							"here, and MxBuild rejects the view with CE0174 (the alias position "+
							"lists only IDENTIFIER as valid, unlike a source position which also "+
							"allows OPEN_QUOTE)",
						i+1, m[1]),
					Location: linter.Location{DocumentType: "viewentity"},
					Suggestion: fmt.Sprintf(
						"Write it unquoted: `as %s`. Note quoting IS allowed in a source position "+
							"(`s.%s`, `from Module.%s as s`) — the alias is the one place it is not, "+
							"so if %q is an OQL reserved word the view column has to be renamed "+
							"(e.g. %sValue) rather than quoted.",
						bare, m[1], m[1], bare, bare),
				})
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
						"select column %d has no as alias: '%s'",
						i+1, display),
					Location:   linter.Location{DocumentType: "viewentity"},
					Suggestion: "All select columns in a view entity must have an explicit alias (e.g., '... as MyAlias')",
				})
			}
		}
	}

	// Check that top-level ORDER BY is accompanied by LIMIT
	if hasTopLevelPhrase(oql, "ORDER by") && !hasTopLevelKeyword(oql, "limit") {
		violations = append(violations, linter.Violation{
			RuleID:     "MDL030",
			Severity:   linter.SeverityError,
			Message:    "ORDER by without limit: view entity OQL queries that use ORDER by must also specify a limit clause",
			Location:   linter.Location{DocumentType: "viewentity"},
			Suggestion: "Add a limit clause after ORDER by",
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

	// `empty` is the microflow/XPath literal; OQL has none. It reads the word
	// as a path, and mxbuild rejects the view with CE0174 "Cannot resolve path
	// part 'empty'" (measured on 11.16.0, ako/mxcli#2208). String literals are
	// blanked first so `= 'empty'` is not mistaken for it.
	if m := oqlEmptyComparisonRe.FindStringSubmatch(blankOQLStringLiterals(oql)); m != nil {
		op := strings.TrimSpace(m[1] + m[2])
		fix := "is null"
		if op == "!=" || op == "<>" {
			fix = "is not null"
		}
		violations = append(violations, linter.Violation{
			RuleID:   "MDL030",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"'%s empty' is not OQL: OQL has no empty literal and reads 'empty' as a path, "+
					"so MxBuild rejects the view with CE0174 (\"Cannot resolve path part 'empty'\")", op),
			Location:   linter.Location{DocumentType: "viewentity"},
			Suggestion: fmt.Sprintf("Write '%s' instead", fix),
		})
	}

	// Check for correlated subquery pattern
	correlatedPattern := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)/([A-Z][a-zA-Z0-9_]*\.[A-Z][a-zA-Z0-9_]*_[A-Z][a-zA-Z0-9_]*)\s*=\s*([a-zA-Z_][a-zA-Z0-9_]*)(?:\s|$|and|or|\))`)
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

	// Reserved-word collision. An attribute/alias named after a Mendix OQL
	// keyword (notably date-part words like Quarter/Month/Year) parses fine in
	// mxcli but makes MxBuild reject the view with CE0174 "The '<word>' part is
	// incomplete or incorrect", because OQL reads the bare word as the keyword.
	// OQL DOES have a quoted-identifier form — double quotes, as in SQL — and
	// mxcli passes it through unchanged: `s."Month"` and `from Module."Year"`
	// both build at 0 errors (measured, 11.13.0). So this is surfaced as the
	// prompt to quote, not as an unfixable name.
	//
	// The check matches only the UNQUOTED form, and by accident rather than by
	// design: the regex anchors on `.` or `as` followed immediately by the word,
	// and a `"` sits in between. TestMDL032_DoesNotFireOnAQuotedIdentifier pins
	// that, because the rule's advice is wrong the day it stops holding.
	//
	// A reserved word counts only in identifier position: after a `.` (attribute
	// access, `s.Quarter`) or after `as` (alias). The alias is the one position
	// quoting cannot rescue, and the limit is OQL's own: it takes a bare
	// identifier there for ANY name (`as "Total"` is CE0174 too). So a view
	// column carrying the name has to be renamed. Warning, not error: the set is
	// the common date-part list, not an exhaustive mirror of Mendix's grammar.
	reservedIdentRe := regexp.MustCompile(`(?i)(?:\.\s*|\bas\s+)(` + oqlReservedIdentAlternation + `)\b`)
	seenReserved := map[string]bool{}
	for _, m := range reservedIdentRe.FindAllStringSubmatch(oql, -1) {
		word := m[1]
		if seenReserved[strings.ToLower(word)] {
			continue
		}
		seenReserved[strings.ToLower(word)] = true
		violations = append(violations, linter.Violation{
			RuleID:   "MDL032",
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf(
				"OQL reserved word %q used unquoted — MxBuild rejects the view with CE0174; quote it as %q",
				word, word),
			Location: linter.Location{DocumentType: "viewentity"},
			// The regex sees "after a dot or after AS" and cannot tell which
			// kind of name it matched, so the suggestion must not assert one:
			// `from Module.Year as y` matches the ENTITY name, and an entity
			// called Year trips the same CE0174 (measured on 11.13.0).
			//
			// Quoting is the fix in a SOURCE position — `s."Month"`,
			// `from Module."Year"` both build at 0 errors, and mxcli writes the
			// quotes through unchanged. It is NOT available for an alias: the
			// MDL grammar rejects `as "X"` for any X, reserved or not. So a view
			// column cannot carry the name, and only that case needs a rename.
			Suggestion: reservedWordSuggestion(word),
		})
	}

	// A comparison as a column, and the GROUP BY rules (MDL033–MDL036,
	// ako/mxcli#981): select lists that are well typed and still fail.
	violations = append(violations, validateOQLSelectExpressions(oql)...)

	return violations
}

// oqlReservedWords are Mendix OQL keywords that realistically collide with
// attribute names — the date/time-part words. Referenced unquoted (`s.Quarter`,
// `as Quarter`) they trip CE0174 in MxBuild. Not exhaustive (SQL words like
// SELECT/FROM are never plausible attribute names, so they're omitted).
var oqlReservedWords = []string{
	"YEAR", "QUARTER", "MONTH", "WEEK", "WEEKDAY", "DAY",
	"DAYOFYEAR", "DAYOFMONTH", "DAYOFWEEK", "HOUR", "MINUTE", "SECOND",
}

// oqlReservedSQLWords are the OQL keywords that are plausible ENTITY or
// ATTRIBUTE names (an `Order` entity, a `Key` or `Count` attribute). Each was
// measured on mxbuild 11.16.0, unquoted both as an entity (`from M.Order as o`,
// `…/M.Order as o`) and as an attribute (`x.Order`): every one is CE0174 "The
// 'Order' part is incomplete or incorrect", and the quoted form builds. In the
// same run Status, Value, Date, Position, Index, Coalesce and Top built
// unquoted, so they are deliberately absent (ako/mxcli#2208, where `Order` hit
// a real project twice).
//
// Only words that make sense as a name are listed: the regex below matches a
// word after `.` or `as`, and type names such as DECIMAL or STRING follow `as`
// inside cast(), so they must never be added here.
var oqlReservedSQLWords = []string{
	"ORDER", "GROUP", "LIMIT", "OFFSET", "END", "KEY", "COUNT", "SUM",
	"DISTINCT", "SELECT", "LIKE", "ASC", "DESC", "HAVING", "WHERE", "JOIN",
	"EXISTS", "CAST",
}

// oqlReservedIdentAlternation is the regex alternation MDL032 matches: the
// date-part words plus the SQL keywords above, e.g. "YEAR|...|ORDER|...".
//
// MDL071 (validate_oql_reserved_names.go), which warns when an entity or
// attribute is CREATED with such a name, keeps to oqlReservedWords. The SQL
// list holds names nearly every app has (Order, Key, Count), and warning on
// each of them at create time, for a view most of them never get, would be
// noise; MDL032 still reports the unquoted use where it actually breaks.
var oqlReservedIdentAlternation = strings.Join(append(append([]string{}, oqlReservedWords...), oqlReservedSQLWords...), "|")

// oqlEmptyComparisonRe matches a comparison with the bare word `empty` on
// either side: `x != empty`, `x = empty`, `empty = x`. Group 1 is the operator
// before the word, group 2 the one after it; one of them is empty.
var oqlEmptyComparisonRe = regexp.MustCompile(`(?i)(?:(!=|<>|=)\s*\bempty\b|\bempty\b\s*(!=|<>|=))`)

// blankOQLStringLiterals replaces the contents of every '…' literal with
// spaces, keeping offsets, so a pattern cannot match inside a string. OQL
// escapes a quote by doubling it, which this treats as closing and reopening
// the literal — the doubled quote itself stays, so the result is the same.
func blankOQLStringLiterals(oql string) string {
	b := []byte(oql)
	in := false
	for i, c := range b {
		switch {
		case c == '\'':
			in = !in
		case in:
			b[i] = ' '
		}
	}
	return string(b)
}

// reservedWordSuggestion is MDL032's advice for one word. Only the date-part
// words are also reported at CREATE time (MDL071), so only they mention it.
func reservedWordSuggestion(word string) string {
	msg := fmt.Sprintf("In a source position, quote it: `s.%q`, `from Module.%q as s` — OQL takes "+
		"double-quoted identifiers like SQL. For an ALIAS / view column name this is not available "+
		"(MDL cannot express `as %q`), so rename that one, e.g. %sValue.", word, word, word, word)
	if isOQLReservedName(word) {
		msg += " MDL071 reports the collision at CREATE time, before the name spreads."
	}
	return msg
}
