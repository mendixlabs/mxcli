// SPDX-License-Identifier: Apache-2.0

package hints

type Entry struct {
	Code     string
	Slug     string
	Severity Severity
	Trigger  string
	WhyWrong string
	HowToFix string
	Examples []ExampleFix
}

type ExampleFix struct {
	Wrong string
	Right string
	Note  string
}

type registry struct {
	byCode map[string]Entry
}

func (r *registry) Lookup(code string) (Entry, bool) {
	e, ok := r.byCode[code]
	return e, ok
}

func (r *registry) All() []Entry {
	out := make([]Entry, 0, len(r.byCode))
	for _, e := range r.byCode {
		out = append(out, e)
	}
	return out
}

var Registry = &registry{byCode: map[string]Entry{
	"E001": {
		Code:     "E001",
		Slug:     "enum-string-mismatch",
		Severity: SeverityError,
		Trigger: "Your MDL has a comparison or assignment where one side is " +
			"an Enumeration attribute (or Enumeration parameter) and the " +
			"other side is a quoted string literal.",
		WhyWrong: "Mendix expressions cannot compare an Enumeration value " +
			"to a String. The comparison would always be false at runtime, " +
			"or trigger CE0109 in Studio Pro.",
		HowToFix: "Replace the string literal with the fully-qualified " +
			"enumeration value: 'NewAlert' → FraudDetection.AlertStatus.NewAlert",
		Examples: []ExampleFix{
			{
				Wrong: "CHANGE $Alert (Status = 'NewAlert')",
				Right: "CHANGE $Alert (Status = FraudDetection.AlertStatus.NewAlert)",
				Note:  "CREATE/CHANGE assignment",
			},
			{
				Wrong: "IF $Alert/Status = 'NewAlert' THEN ...",
				Right: "IF $Alert/Status = FraudDetection.AlertStatus.NewAlert THEN ...",
				Note:  "IF condition",
			},
			{
				Wrong: "CALL Mf(Status = 'Validated')",
				Right: "CALL Mf(Status = FraudDetection.AlertStatus.Validated)",
				Note:  "CALL parameter",
			},
		},
	},
	"E002": {
		Code: "E002", Slug: "bool-string-mismatch", Severity: SeverityError,
		Trigger:  "A Boolean attribute is compared against a string literal like 'true' or 'false'.",
		WhyWrong: "Mendix Boolean expressions use the unquoted literals true and false. Comparing a Boolean against a string is always false.",
		HowToFix: "Replace 'true'/'false' with the unquoted literals true/false.",
		Examples: []ExampleFix{
			{Wrong: "IF $Config/IsActive = 'true' THEN ...", Right: "IF $Config/IsActive = true THEN ...", Note: "IF condition"},
		},
	},
	"E003": {
		Code: "E003", Slug: "null-to-empty", Severity: SeverityWarning,
		Trigger:  "The keyword null is used in a Mendix expression.",
		WhyWrong: "Mendix expressions use empty, not null. Tools auto-correct on write but the source becomes inconsistent on the next round-trip.",
		HowToFix: "Replace null with empty.",
		Examples: []ExampleFix{
			{Wrong: "IF $Alert = null THEN ...", Right: "IF $Alert = empty THEN ..."},
		},
	},
	"E004": {
		Code: "E004", Slug: "concat-type", Severity: SeverityError,
		Trigger:  "The '+' operator is used between values of incompatible kinds (e.g. String and Integer).",
		WhyWrong: "'+' concatenates Strings only. Mixing kinds raises CE0109 in Studio Pro.",
		HowToFix: "Wrap the non-String operand in toString().",
		Examples: []ExampleFix{
			{Wrong: "'count=' + $n", Right: "'count=' + toString($n)", Note: "$n is Integer"},
		},
	},
	"E005": {
		Code: "E005", Slug: "func-arg-type", Severity: SeverityError,
		Trigger:  "A built-in function received an argument of the wrong kind.",
		WhyWrong: "Built-in functions have fixed argument signatures.",
		HowToFix: "Cast the argument to the expected kind, e.g. wrap with toString() or toInteger().",
		Examples: []ExampleFix{
			{Wrong: "length($Alert/RiskScore)", Right: "length(toString($Alert/RiskScore))", Note: "RiskScore is Decimal; length expects String"},
		},
	},
	"E006": {
		Code: "E006", Slug: "func-arg-arity", Severity: SeverityError,
		Trigger:  "A built-in function was called with the wrong number of arguments.",
		WhyWrong: "Each built-in expects a fixed number of arguments.",
		HowToFix: "Provide the exact number of arguments listed in the function signature.",
		Examples: []ExampleFix{
			{Wrong: "substring('hello')", Right: "substring('hello', 0, 3)"},
		},
	},
	"E007": {
		Code:     "E007",
		Slug:     "unknown-token",
		Severity: SeverityWarning,
		Trigger:  "The parser encountered tokens it does not recognise as a valid Mendix expression and skipped to the next safe boundary.",
		WhyWrong: "The unrecognised text is not part of the Mendix expression grammar — typos, foreign characters, or stray punctuation usually cause this.",
		HowToFix: "Replace the unrecognised fragment with a valid expression: a literal, a variable, a function call, or a qualified name.",
		Examples: []ExampleFix{
			{
				Wrong: "SET $msg = 'count=' + length(@@@broken@@@) + ' items';",
				Right: "SET $msg = 'count=' + toString(length($list)) + ' items';",
				Note:  "argument of length()",
			},
		},
	},
	"E008": {
		Code: "E008", Slug: "enum-missing-module", Severity: SeverityError,
		Trigger:  "An enum value was written without its module prefix.",
		WhyWrong: "Mendix requires fully-qualified Module.Enum.Value references.",
		HowToFix: "Add the module prefix.",
		Examples: []ExampleFix{
			{Wrong: "$Status = AlertStatus.NewAlert", Right: "$Status = FraudDetection.AlertStatus.NewAlert"},
		},
	},
	"E009": {
		Code: "E009", Slug: "slot-type-mismatch", Severity: SeverityError,
		Trigger:  "An expression's inferred kind does not match the slot's expected kind (catch-all).",
		WhyWrong: "The surrounding statement requires a specific kind (Boolean for IF condition, Integer for LIMIT, etc.).",
		HowToFix: "Adjust the expression so its result matches the slot's expected kind.",
		Examples: []ExampleFix{
			{Wrong: "IF 'active' THEN ...", Right: "IF $obj/IsActive THEN ..."},
		},
	},
	"E010": {
		Code: "E010", Slug: "attribute-not-found", Severity: SeverityError,
		Trigger:  "An attribute path references an attribute that does not exist on the entity.",
		WhyWrong: "Catalog lookup confirmed the entity does not have the requested attribute.",
		HowToFix: "Use the correct attribute name from the entity definition.",
		Examples: []ExampleFix{
			{Wrong: "$Customer/EmialAddress", Right: "$Customer/EmailAddress"},
		},
	},
	"E011": {
		Code:     "E011",
		Slug:     "not-missing-parens",
		Severity: SeverityError,
		Trigger:  "The 'not' keyword is used without parentheses around its operand.",
		WhyWrong: "Mendix expression syntax requires not(expr) — 'not expr' without parentheses is rejected by Studio Pro with CE0117.",
		HowToFix: "Wrap the operand in parentheses: not(expr).",
		Examples: []ExampleFix{
			{
				Wrong: "not $Validation/IsValid",
				Right: "not($Validation/IsValid)",
			},
			{
				Wrong: "not isMatch($Value, '^[0-9]+$')",
				Right: "not(isMatch($Value, '^[0-9]+$'))",
			},
			{
				Wrong: "$x != empty and not contains($s, '@')",
				Right: "$x != empty and not(contains($s, '@'))",
			},
		},
	},
	"E014": {
		Code:     "E014",
		Slug:     "trailing-tokens",
		Severity: SeverityError,
		Trigger:  "The expression parsed, but tokens were left over after it.",
		WhyWrong: "The expression is incomplete or malformed — most often a Mendix keyword used as if it were a function, or two keywords glued together.",
		HowToFix: "`empty` is a keyword, not a function: write `$List = empty` rather than `empty($List)`. For a count, use `length($List) = 0`.",
		Examples: []ExampleFix{
			{
				Wrong: "IF empty($Orders) THEN",
				Right: "IF $Orders = empty THEN",
				Note:  "empty is a keyword, so the '(' is left over",
			},
			{
				Wrong: "IF $X = '' emptyor $Y THEN",
				Right: "IF $X = '' empty or $Y THEN",
				Note:  "glued keywords",
			},
		},
	},
	"E015": {
		Code:     "E015",
		Slug:     "string-func-vs-empty",
		Severity: SeverityWarning,
		Trigger:  "The result of a String function (trim, toLowerCase, toUpperCase, substring, replaceAll, toString) is compared with empty.",
		WhyWrong: "These functions never return empty: for an empty input they return ''. Measured on the Mendix 10.18.5 runtime. So `= empty` is always false and `!= empty` always true, and the decision never takes the branch the author meant.",
		HowToFix: "Compare with '' instead. trim(empty) is '', so `trim($v) != ''` also covers an empty $v — no separate `$v != empty` is needed.",
		Examples: []ExampleFix{
			{
				Wrong: "IF trim($Value) != empty THEN",
				Right: "IF trim($Value) != '' THEN",
				Note:  "a blank or empty $Value took the 'has text' branch",
			},
		},
	},
	"E013": {
		Code:     "E013",
		Slug:     "bare-identifier-value",
		Severity: SeverityError,
		Trigger:  "A bare word stands alone as the value of a create/change member or of a call microflow/nanoflow argument.",
		WhyWrong: "Mendix expressions have no bare identifiers: a value is a literal, a $variable, a qualified name or a function call. The bare word reaches the build as CE0117 \"Error(s) in expression\".",
		HowToFix: "Quote it if it is text, add the $ if it is a variable, or qualify it if it is an enumeration value.",
		Examples: []ExampleFix{
			{
				Wrong: "CHANGE $Order (Status = Closed);",
				Right: "CHANGE $Order (Status = 'Closed');",
				Note:  "a String attribute takes a quoted literal",
			},
			{
				Wrong: "CHANGE $Order (Status = Closed);",
				Right: "CHANGE $Order (Status = Sales.OrderStatus.Closed);",
				Note:  "an enumeration attribute takes a qualified value",
			},
		},
	},
	"E012": {
		Code:     "E012",
		Slug:     "id-attribute-illegal",
		Severity: SeverityError,
		Trigger:  "The path '$Object/id' is used in a microflow expression or MDL SET statement.",
		WhyWrong: "Mendix reserves 'id' as a system attribute name — it cannot be accessed via '$Object/id' in microflow expressions. " +
			"It is only valid in XPath constraints (e.g. '[id = $Variable]'), not in expressions.",
		HowToFix: "Option A (preferred): change the microflow return type to the entity object itself instead of Long, " +
			"and let callers use the object directly.\n" +
			"Option B: add an AutoNumber attribute to the entity (e.g. 'WorkHistoryNo') and return '$Object/WorkHistoryNo' instead.",
		Examples: []ExampleFix{
			{
				Wrong: "SET $Id = $WorkHistory/id;",
				Right: "RETURN $WorkHistory;  -- change RETURNS type to the entity",
				Note:  "Option A — return the object",
			},
			{
				Wrong: "SET $Id = $WorkHistory/id;",
				Right: "SET $Id = $WorkHistory/WorkHistoryNo;  -- WorkHistoryNo is AutoNumber",
				Note:  "Option B — use a dedicated AutoNumber attribute",
			},
		},
	},
}}
