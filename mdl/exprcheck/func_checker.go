// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

type funcSig struct {
	args    []TypeKind // full parameter list (max arity)
	minArgs int        // minimum required args; 0 means all args required (= len(args))
	ret     TypeKind

	// retByArity overrides ret for a specific argument count. round() is the
	// only built-in whose result type depends on arity: round($d) yields a whole
	// number an Integer target accepts, round($d, 2) yields a genuine Decimal
	// that fails CE0117. Measured on mxbuild 11.13.0 (sudoku finding #49).
	retByArity map[int]TypeKind

	// retFromArgs makes the result kind follow the operand kinds rather than
	// being fixed: abs/max/min return an Integer for Integer operands and a
	// Decimal as soon as one operand is Decimal. Measured the same way —
	// `abs($int)` and `max($int, 3)` into an Integer are accepted, `abs($dec)`
	// and `max($int, 3.5)` are CE0117.
	retFromArgs bool

	// variadic lets the last parameter repeat: max/min take any number of
	// operands from two up (`max($i, 1, 2, 3, 4)` builds at 0 errors).
	variadic bool

	// alts lists further kinds a parameter accepts beside args[i]. An index
	// past the end of args falls on the last parameter, as variadic does.
	alts map[int][]TypeKind

	// unverified marks a row nobody has built against mxbuild. Its argument
	// kinds are not trusted, so E005 never fires on it.
	unverified bool
}

// accepts returns the kinds parameter i accepts.
func (s funcSig) accepts(i int) []TypeKind {
	if len(s.args) == 0 {
		return nil
	}
	if i >= len(s.args) {
		i = len(s.args) - 1
	}
	return append([]TypeKind{s.args[i]}, s.alts[i]...)
}

// retFor returns the result kind for a call with argc arguments.
func (s funcSig) retFor(argc int) TypeKind {
	if k, ok := s.retByArity[argc]; ok {
		return k
	}
	return s.ret
}

// min returns the effective minimum argument count.
func (s funcSig) min() int {
	if s.minArgs > 0 {
		return s.minArgs
	}
	return len(s.args)
}

// funcTable lists every Mendix built-in function and its signature.
// Extracted from Mendix 10/11 expression documentation.
// When Mendix adds new built-ins, add them here and update the corresponding
// test in func_checker_test.go.
var funcTable = map[string]funcSig{
	// Boolean
	"not": {args: []TypeKind{KindBoolean}, ret: KindBoolean},

	// String — predicates
	"contains":   {args: []TypeKind{KindString, KindString}, ret: KindBoolean},
	"startsWith": {args: []TypeKind{KindString, KindString}, ret: KindBoolean},
	"endsWith":   {args: []TypeKind{KindString, KindString}, ret: KindBoolean},
	"isMatch":    {args: []TypeKind{KindString, KindString}, ret: KindBoolean},

	// String — transforms
	"length": {args: []TypeKind{KindString}, ret: KindInteger},
	// find/findLast(text, fragment [, startIndex]) — the start index builds at
	// 0 errors on 11.6.6 and 11.15.0 (mendixlabs/mxcli#1515).
	"find":         {args: []TypeKind{KindString, KindString, KindInteger}, minArgs: 2, ret: KindInteger},
	"findLast":     {args: []TypeKind{KindString, KindString, KindInteger}, minArgs: 2, ret: KindInteger},
	"substring":    {args: []TypeKind{KindString, KindInteger, KindInteger}, minArgs: 2, ret: KindString},
	"trim":         {args: []TypeKind{KindString}, ret: KindString},
	"toUpperCase":  {args: []TypeKind{KindString}, ret: KindString},
	"toLowerCase":  {args: []TypeKind{KindString}, ret: KindString},
	"replaceAll":   {args: []TypeKind{KindString, KindString, KindString}, ret: KindString},
	"replaceFirst": {args: []TypeKind{KindString, KindString, KindString}, ret: KindString},
	"urlEncode":    {args: []TypeKind{KindString}, ret: KindString},
	"urlDecode":    {args: []TypeKind{KindString}, ret: KindString},

	// Type conversion
	"toString": {args: []TypeKind{KindAny}, ret: KindString},
	// parseInteger(value [, default]) and parseDecimal(value [, format [, default]])
	// return the default when the text does not parse; a two-argument
	// parseDecimal takes either a format or a default. parseBoolean has no
	// default overload: `parseBoolean($s, false)` is CE0117. All measured with
	// mx check on 11.14.0 (mendixlabs/mxcli#1216).
	"parseInteger": {args: []TypeKind{KindString, KindInteger}, minArgs: 1, ret: KindInteger},
	"parseDecimal": {args: []TypeKind{KindString, KindString, KindDecimal}, minArgs: 1, ret: KindDecimal, alts: map[int][]TypeKind{1: {KindDecimal}}},
	"parseBoolean": {args: []TypeKind{KindString}, ret: KindBoolean},
	// formatDecimal(value, format [, languageTag]). The format is NOT
	// optional: `formatDecimal($d)` is CE0117 on 11.6.6 and 11.15.0, as is a
	// number in either String slot (mendixlabs/mxcli#1515).
	"formatDecimal": {args: []TypeKind{KindDecimal, KindString, KindString}, minArgs: 2, ret: KindString},

	// Math. The return kinds here are what Mendix actually accepts in an
	// Integer/Long target, not what the expression documentation prints: floor,
	// ceil and one-argument round produce whole numbers, and abs/max/min preserve
	// their operands' kind. Every row was measured against mxbuild 11.13.0 with a
	// bare `$int div 3` as the control (see round_returns_integer_test.go).
	//
	// There is no `trunc`. It used to be treated as a rounding function here;
	// Mendix has no such built-in and `trunc($d)` fails the build with CE0117, so
	// MDL044 flagging it is correct and adding it to this table would be wrong.
	"abs":   {args: []TypeKind{KindDecimal}, ret: KindDecimal, retFromArgs: true},
	"round": {args: []TypeKind{KindDecimal, KindInteger}, minArgs: 1, ret: KindDecimal, retByArity: map[int]TypeKind{1: KindInteger}},
	"floor": {args: []TypeKind{KindDecimal}, ret: KindInteger},
	"ceil":  {args: []TypeKind{KindDecimal}, ret: KindInteger},
	"pow":   {args: []TypeKind{KindDecimal, KindDecimal}, ret: KindDecimal},
	"sqrt":  {args: []TypeKind{KindDecimal}, ret: KindDecimal},
	// max/min take two or more operands, numbers or DateTimes (`max($dt, $dt)`
	// and `min(1, 2.5, $l)` build at 0 errors on 11.6.6 and 11.15.0; a String
	// or Boolean operand is CE0117). A single-argument max/min is not this
	// function at all: the visitor reads it as the MAXIMUM/MINIMUM aggregate.
	"max":    {args: []TypeKind{KindDecimal, KindDecimal}, ret: KindDecimal, retFromArgs: true, variadic: true, alts: map[int][]TypeKind{0: {KindDateTime}, 1: {KindDateTime}}},
	"min":    {args: []TypeKind{KindDecimal, KindDecimal}, ret: KindDecimal, retFromArgs: true, variadic: true, alts: map[int][]TypeKind{0: {KindDateTime}, 1: {KindDateTime}}},
	"random": {args: []TypeKind{}, ret: KindDecimal},

	// Enumerations
	"getCaption": {args: []TypeKind{KindAny}, ret: KindString},
	"getKey":     {args: []TypeKind{KindAny}, ret: KindString},

	// Special checks — the object-state predicates. Each takes an object and
	// returns Boolean; `isSynced`/`isSyncing` are offline-sync predicates and are
	// nanoflow-only, but that is a context restriction, not an unknown name.
	// All three were missing from this table, so MDL044 flagged them as
	// hallucinated; each was built against mxbuild 11.13.0 at 0 errors before
	// being added here (the table is the sole allow-list MDL044 consults, and
	// MDL044 is now enforced on the exec path).
	"isNew":     {args: []TypeKind{KindAny}, ret: KindBoolean},
	"isSynced":  {args: []TypeKind{KindAny}, ret: KindBoolean},
	"isSyncing": {args: []TypeKind{KindAny}, ret: KindBoolean},

	// DateTime — construction. There is no currentDateTime(): the current time
	// is the [%CurrentDateTime%] token (see tokenFuncs in unknown_funcs.go).
	// dateTime/dateTimeUTC(year [, month, day, hour, minute, second]): every
	// count from 1 to 6 builds at 0 errors on 11.6.6 and 11.15.0
	// (mendixlabs/mxcli#1515).
	"dateTime":    {args: []TypeKind{KindInteger, KindInteger, KindInteger, KindInteger, KindInteger, KindInteger}, minArgs: 1, ret: KindDateTime},
	"dateTimeUTC": {args: []TypeKind{KindInteger, KindInteger, KindInteger, KindInteger, KindInteger, KindInteger}, minArgs: 1, ret: KindDateTime},

	// DateTime — add (local calendar)
	"addMilliseconds": {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addSeconds":      {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addMinutes":      {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addHours":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addDays":         {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addWeeks":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addMonths":       {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addQuarters":     {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addYears":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},

	// DateTime — add (UTC calendar)
	"addDaysUTC":     {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addWeeksUTC":    {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addMonthsUTC":   {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addQuartersUTC": {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addYearsUTC":    {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},

	// DateTime — subtract (local calendar)
	"subtractMilliseconds": {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractSeconds":      {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractMinutes":      {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractHours":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractDays":         {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractWeeks":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractMonths":       {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractQuarters":     {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractYears":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},

	// DateTime — subtract (UTC calendar)
	"subtractDaysUTC":     {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractWeeksUTC":    {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractMonthsUTC":   {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractQuartersUTC": {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractYearsUTC":    {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},

	// DateTime — between
	// The "between dates" duration functions return Decimal in Mendix (e.g.
	// daysBetween can be 1.5), so assigning one to an Integer/Long variable fails
	// the build with CE0117 (findings #2). The "calendar" variants return whole
	// units (Integer); millisecondsBetween returns a whole count (Long).
	"millisecondsBetween":   {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindLong},
	"secondsBetween":        {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"minutesBetween":        {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"hoursBetween":          {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"daysBetween":           {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"weeksBetween":          {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"calendarMonthsBetween": {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindInteger},
	"calendarYearsBetween":  {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindInteger},

	// DateTime — begin-of
	"beginOfDay":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"beginOfWeek":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"beginOfMonth": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"beginOfYear":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},

	// DateTime — end-of
	"endOfDay":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"endOfWeek":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"endOfMonth": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"endOfYear":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},

	// DateTime — trim-to (local calendar)
	"trimToSeconds": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToMinutes": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToHours":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToDays":    {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToMonths":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToYears":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},

	// DateTime — trim-to (UTC calendar)
	"trimToHoursUTC":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToDaysUTC":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToMonthsUTC": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToYearsUTC":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},

	// DateTime — formatting / parsing (local calendar)
	// The format is optional for all three (1 argument builds at 0 errors on
	// 11.6.6 and 11.15.0; a third is CE0117).
	"formatDateTime": {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"formatTime":     {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"formatDate":     {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	// parseDateTime/parseDateTimeUTC(value, format [, default]) — the default is
	// returned when the text does not parse (mendixlabs/mxcli#1216; 3 args build
	// at 0 errors on 11.14.0, 4 args are CE0117).
	"parseDateTime": {args: []TypeKind{KindString, KindString, KindDateTime}, minArgs: 2, ret: KindDateTime},

	// DateTime — formatting / parsing (UTC calendar)
	"formatDateTimeUTC": {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"formatTimeUTC":     {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"formatDateUTC":     {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"parseDateTimeUTC":  {args: []TypeKind{KindString, KindString, KindDateTime}, minArgs: 2, ret: KindDateTime},

	// DateTime — epoch conversion
	"dateTimeToEpoch": {args: []TypeKind{KindDateTime}, ret: KindLong},
	"epochToDateTime": {args: []TypeKind{KindLong}, ret: KindDateTime},

	// DateTime — extraction → Integer.
	// NOTE: `year(...)` and `month(...)` were previously listed here but Mendix
	// has no such built-ins — `mxcli check` passed and the build then failed with
	// CE0117 (ledger finding #16). The correct approach is date formatting/parsing
	// (`parseInteger(formatDateTime($d, 'yyyy'))`). The sibling extraction names
	// below are UNVERIFIED against a real Mendix build and are likely invalid too;
	// remove any that a build rejects (do not add more without confirming against
	// Studio Pro — this whole family looks speculative). Measured since: every
	// row below, dateTimeToDate included, is CE0117 on 11.6.6 and 11.15.0 with
	// a DateTime argument (mendixlabs/mxcli#1515), so these are not built-ins.
	// Removing them turns them into MDL044 errors, a change of its own; until
	// then they are marked unverified and E005 stays off them.
	"dayOfYear":   {args: []TypeKind{KindDateTime}, ret: KindInteger, unverified: true},
	"dayOfMonth":  {args: []TypeKind{KindDateTime}, ret: KindInteger, unverified: true},
	"weekOfYear":  {args: []TypeKind{KindDateTime}, ret: KindInteger, unverified: true},
	"dayOfWeek":   {args: []TypeKind{KindDateTime}, ret: KindInteger, unverified: true},
	"hour":        {args: []TypeKind{KindDateTime}, ret: KindInteger, unverified: true},
	"minute":      {args: []TypeKind{KindDateTime}, ret: KindInteger, unverified: true},
	"second":      {args: []TypeKind{KindDateTime}, ret: KindInteger, unverified: true},
	"millisecond": {args: []TypeKind{KindDateTime}, ret: KindInteger, unverified: true},

	// DateTime — extraction (legacy alias)
	"dateTimeToDate": {args: []TypeKind{KindDateTime}, ret: KindDateTime, unverified: true},
}

// checkCallExpr returns hints for arity (E006) and argument-kind (E005)
// mismatches against the built-in function signature table. User-defined
// microflow calls (CallExpr.Name containing a dot) are not checked here — they
// require catalog lookup.
func checkCallExpr(c *CallExpr, ctx Context) []Hint {
	sig, ok := funcTable[c.Name]
	if !ok {
		return nil
	}
	got := len(c.Args)
	minA, maxA := sig.min(), len(sig.args)
	if got < minA || (!sig.variadic && got > maxA) {
		var expectStr, fixStr string
		switch {
		case sig.variadic:
			expectStr = fmt.Sprintf("at least %d", minA)
			fixStr = fmt.Sprintf("Provide at least %d argument(s) for %s().", minA, c.Name)
		case minA == maxA:
			expectStr = fmt.Sprintf("%d", maxA)
			fixStr = fmt.Sprintf("Provide %d argument(s) for %s().", maxA, c.Name)
		default:
			expectStr = fmt.Sprintf("%d to %d", minA, maxA)
			fixStr = fmt.Sprintf("Provide %d to %d argument(s) for %s().", minA, maxA, c.Name)
		}
		return []Hint{{
			Code: "E006", Slug: "func-arg-arity",
			Severity: hints.SeverityError,
			Where: hints.Location{
				File: ctx.File, Line: c.Pos().Line, Column: c.Pos().Column,
				Microflow: ctx.Microflow,
				Context:   fmt.Sprintf("call to %s()", c.Name),
			},
			YouWrote: fmt.Sprintf("%s(...) with %d argument(s)", c.Name, got),
			Problem:  fmt.Sprintf("%s() expects %s argument(s), got %d.", c.Name, expectStr, got),
			Fix:      fixStr,
			Reference: &hints.Reference{
				FunctionName:    c.Name,
				FunctionReturns: typeKindName(sig.ret),
				FunctionArgs:    typeKindNames(sig.args),
			},
		}}
	}
	if sig.unverified {
		return nil
	}
	var out []Hint
	for i, a := range c.Args {
		k := argKind(a, ctx)
		want := sig.accepts(i)
		if argAccepted(want, k) {
			continue
		}
		wantStr := kindList(want)
		out = append(out, Hint{
			Code: "E005", Slug: "func-arg-type",
			Severity: hints.SeverityError,
			Where: hints.Location{
				File: ctx.File, Line: a.Pos().Line, Column: a.Pos().Column,
				Microflow: ctx.Microflow,
				Context:   fmt.Sprintf("argument %d of %s()", i+1, c.Name),
			},
			YouWrote: fmt.Sprintf("%s(...) with a %s as argument %d", c.Name, typeKindName(k), i+1),
			Problem: fmt.Sprintf("%s() argument %d must be %s, but this expression is %s — "+
				"the build fails CE0117 \"Error(s) in expression\".", c.Name, i+1, wantStr, withArticle(typeKindName(k))),
			Fix: argFix(want, k),
			Reference: &hints.Reference{
				FunctionName:    c.Name,
				FunctionReturns: typeKindName(sig.ret),
				FunctionArgs:    typeKindNames(sig.args),
			},
		})
	}
	return out
}

// argKind is the kind of a call argument as far as E005 is concerned.
//
// A [%Token%] is the one place inferKind is too coarse: it calls every token a
// String, while [%CurrentDateTime%] and the [%BeginOf…%]/[%EndOf…%] family are
// DateTimes and [%CurrentUser%] is an object. Rather than change what every
// other rule sees, the argument check types the date tokens and tolerates the
// rest.
func argKind(e RobustExpr, ctx Context) TypeKind {
	if t, ok := unwrapParens(e).(*TokenExpr); ok {
		name := strings.TrimSuffix(t.Token, "UTC")
		if name == "CurrentDateTime" || strings.HasPrefix(name, "BeginOf") || strings.HasPrefix(name, "EndOf") {
			return KindDateTime
		}
		return KindUnknown
	}
	return inferKind(e, ctx)
}

// argAccepted reports whether an argument of kind got may stand where one of
// want is expected. It only refuses what it can prove: an argument of unknown
// kind, `empty`, an enumeration, an object, a list or a binary is never
// refused, nor is a parameter typed Any.
//
// Numbers follow what mxbuild does, measured on 11.6.6 and 11.15.0
// (mendixlabs/mxcli#1515): Integer and Long are interchangeable and both widen
// to Decimal, but a Decimal where an Integer or Long is expected is CE0117 —
// `addDays($dt, 1.5)`, `substring($s, $d)`, `round($d, $d)`.
func argAccepted(want []TypeKind, got TypeKind) bool {
	switch got {
	case KindBoolean, KindString, KindInteger, KindLong, KindDecimal, KindDateTime:
	default:
		return true
	}
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		switch {
		case w == KindAny || w == KindUnknown || w == got:
			return true
		case w == KindDecimal && (got == KindInteger || got == KindLong):
			return true
		case (w == KindInteger || w == KindLong) && (got == KindInteger || got == KindLong):
			return true
		case w != KindBoolean && w != KindString && w != KindInteger &&
			w != KindLong && w != KindDecimal && w != KindDateTime:
			// A parameter of a kind this check does not model (Enumeration,
			// Object, …): nothing to compare against.
			return true
		}
	}
	return false
}

func kindList(ks []TypeKind) string {
	names := typeKindNames(ks)
	for i, n := range names {
		names[i] = withArticle(n)
	}
	return strings.Join(names, " or ")
}

func withArticle(name string) string {
	if name != "" && strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}

func argFix(want []TypeKind, got TypeKind) string {
	for _, w := range want {
		switch {
		case w == KindString:
			return "Pass a String: quote a literal, or convert the value with toString()."
		case (w == KindInteger || w == KindLong) && got == KindDecimal:
			return "Pass a whole number: round the value with round(), floor() or ceil()."
		case w == KindInteger || w == KindLong || w == KindDecimal:
			return "Pass a number: write a numeric literal, or convert text with parseInteger()/parseDecimal()."
		case w == KindDateTime:
			return "Pass a DateTime: a DateTime variable, [%CurrentDateTime%], or parseDateTime() for text."
		}
	}
	return "Pass an argument of the kind the function's signature lists."
}

func typeKindName(k TypeKind) string {
	switch k {
	case KindBoolean:
		return "Boolean"
	case KindString:
		return "String"
	case KindInteger:
		return "Integer"
	case KindLong:
		return "Long"
	case KindDecimal:
		return "Decimal"
	case KindDateTime:
		return "DateTime"
	case KindBinary:
		return "Binary"
	case KindObject:
		return "Object"
	case KindList:
		return "List"
	case KindEnumeration:
		return "Enumeration"
	case KindAny:
		return "Any"
	case KindEmpty:
		return "Empty"
	}
	return "Unknown"
}

func typeKindNames(ks []TypeKind) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = typeKindName(k)
	}
	return out
}

// KindName is the public accessor for the human-readable name of a TypeKind
// (e.g. KindBoolean → "Boolean"). Used by cmd/mxcli for help/explain output.
func KindName(k TypeKind) string { return typeKindName(k) }

// PublicFuncSig is a JSON/CLI-friendly view of a built-in function signature.
type PublicFuncSig struct {
	Args    []string
	MinArgs int // 0 means all Args are required
	// Variadic means the last of Args may repeat: there is no maximum.
	Variadic bool
	Returns  string
}

// PublicFuncTable returns a JSON-friendly view of the built-in function
// signatures used by the checker.
func PublicFuncTable() map[string]PublicFuncSig {
	out := make(map[string]PublicFuncSig, len(funcTable))
	for k, v := range funcTable {
		out[k] = PublicFuncSig{
			Args:     typeKindNames(v.args),
			MinArgs:  v.minArgs,
			Variadic: v.variadic,
			Returns:  typeKindName(v.ret),
		}
	}
	return out
}

// FuncReturnKind returns the TypeKind of the return value of a named built-in
// function. Returns (KindUnknown, false) for unknown functions.
func FuncReturnKind(name string) (TypeKind, bool) {
	sig, ok := funcTable[name]
	if !ok {
		return KindUnknown, false
	}
	return sig.ret, true
}
