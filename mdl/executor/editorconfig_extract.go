// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// editorConfigExtractStats reports how much of a widget's editorConfig.js the
// extractor could lift into declarative WidgetVisibilityRules — the coverage
// number that tells `check`/serialization how far to trust the rules and when
// to fall back to mxbuild's update-widgets.
type editorConfigExtractStats struct {
	TotalHideCalls int // hidePropertyIn + hidePropertiesIn call sites seen
	Recognized     int // lifted into a top-level WidgetVisibilityRule
	SkippedNested  int // object-list-nested hide whose guard could not be lifted
	SkippedComplex int // ternary/compound guard, or an alias we couldn't resolve
}

// hideCallRE locates a hidePropertyIn / hidePropertiesIn / hideNestedPropertiesIn
// call and captures its (balanced-enough) argument list. All three are matched:
// the nested forms target object-list items, which is where the Accordion hides
// the group properties that make an authored widget fail CE0463 (upstream #931).
var hideCallRE = regexp.MustCompile(`hide(?:Property|Properties|NestedProperties)In\(`)

// hideCallTail returns where a hide call's NAME starts when it ends s, or -1,
// so stripGroupSiblings can tell a hide sibling from any other comma-separated
// expression.
//
// It is a suffix test rather than a `…$` regexp on purpose: s is everything
// before the call, and an end-anchored regexp still scans it from the start.
// Run once per hide call, that made the extractor quadratic in the size of the
// editorConfig.js — 0.5 s for the 400 KB one in Maps, on every refresh (#1848).
func hideCallTail(s string) int {
	for _, name := range [...]string{"hidePropertyIn", "hidePropertiesIn", "hideNestedPropertiesIn"} {
		if strings.HasSuffix(s, name) {
			return len(s) - len(name)
		}
	}
	return -1
}

// aliasAssignRE finds `IDENT=OBJ.PROP` (a `var x=e.selection`-style alias).
// Resolution is scoped to the enclosing function body (see enclosingAliases),
// because minified editorConfig reuses single-letter identifiers across scopes.
var aliasAssignRE = regexp.MustCompile(`([A-Za-z_$][\w$]*)=([A-Za-z_$][\w$]*)\.([A-Za-z_$][\w$]*)`)

// stringLitRE matches a double-quoted JS string literal (no escapes in the
// property/enum keys we care about).
var stringLitRE = regexp.MustCompile(`"([^"\\]*)"`)

// visibilityCache memoizes extracted rules per (projectPath, widgetID) so the
// editorConfig.js is parsed once per build session, not per widget instance.
var (
	visibilityCache   = map[string][]types.WidgetVisibilityRule{}
	visibilityCacheMu sync.Mutex
)

// WidgetVisibilityRules returns the editorConfig-derived property-visibility rules
// for a widget installed in the given project (nil when the .mpk or its editor
// config can't be found). Exported for the `mxcli widget describe` command, which
// surfaces the dynamic property rules mxcli discovered for a project's widget.
func WidgetVisibilityRules(projectPath, widgetID string) []types.WidgetVisibilityRule {
	return resolveWidgetVisibilityRules(projectPath, widgetID)
}

// ExtractWidgetVisibilityStats lifts a widget's editorConfig visibility rules from
// a specific .mpk path and also returns the extractor's coverage stats (how many
// hide-calls were recognized vs skipped). Exported for `mxcli widget describe` to
// report extraction coverage.
func ExtractWidgetVisibilityStats(mpkPath, widgetID string) ([]types.WidgetVisibilityRule, int, int) {
	js, err := mpk.ReadEditorConfig(mpkPath, widgetID)
	if err != nil || js == "" {
		return nil, 0, 0
	}
	rules, stats := extractVisibilityRulesFromJS(js)
	return rules, stats.Recognized, stats.TotalHideCalls
}

// resolveWidgetVisibilityRules returns the property-visibility rules for a
// widget, lifted from its installed .mpk's editorConfig.js. Used to enrich
// built-in widget definitions (DataGrid2, Gallery, …) — which the .def.json
// generator skips — with the version-specific applicability logic of the Data
// Widgets package actually installed in the project. Returns nil when the .mpk
// or its editor config can't be found (degrades to "no rules" → template
// default, exactly today's behaviour). Best-effort: see extractVisibilityRules-
// FromJS for coverage limits.
func resolveWidgetVisibilityRules(projectPath, widgetID string) []types.WidgetVisibilityRule {
	if projectPath == "" || widgetID == "" {
		return nil
	}
	key := projectPath + "\x00" + widgetID
	visibilityCacheMu.Lock()
	if r, ok := visibilityCache[key]; ok {
		visibilityCacheMu.Unlock()
		return r
	}
	visibilityCacheMu.Unlock()

	// getProjectPath() yields the .mpr file path; FindMPK wants the directory
	// that contains widgets/.
	projectDir := projectPath
	if strings.EqualFold(filepath.Ext(projectDir), ".mpr") {
		projectDir = filepath.Dir(projectDir)
	}

	var rules []types.WidgetVisibilityRule
	if mpkPath, err := mpk.FindMPK(projectDir, widgetID); err == nil && mpkPath != "" {
		rules = extractVisibilityRulesFromMPK(mpkPath, widgetID)
	}

	visibilityCacheMu.Lock()
	visibilityCache[key] = rules
	visibilityCacheMu.Unlock()
	return rules
}

// extractVisibilityRulesFromMPK reads a widget's editorConfig.js from its .mpk
// and lifts its property-visibility rules. Returns nil when the widget ships no
// editor config. Used both at build time (via resolveWidgetVisibilityRules) and
// at .def.json generation time, so generated definitions carry the rules and the
// hand-transcribed table can retire.
func extractVisibilityRulesFromMPK(mpkPath, widgetID string) []types.WidgetVisibilityRule {
	js, err := mpk.ReadEditorConfig(mpkPath, widgetID)
	if err != nil || js == "" {
		return nil
	}
	rules, _ := extractVisibilityRulesFromJS(js)
	return rules
}

// extractVisibilityRulesFromJS lifts top-level property-hide rules from a
// widget's compiled editorConfig.js into declarative WidgetVisibilityRules.
//
// It recognizes the dominant `getProperties` idioms — `"V"===ref && hide(...)`,
// `"V"!==ref && hide(...)`, `ref && hide(...)`, `ref || hide(...)`,
// `ref ? hide(...) : …`, and a hide in a `switch(ref){case "V": …}` arm — where
// `ref` is `obj.prop` or a locally-aliased
// identifier resolved within the enclosing function scope. Everything it cannot
// lift (object-list-nested hides, compound/computed guards, unresolved aliases)
// is counted in the returned stats so callers can gauge coverage; unrecognized
// hides simply produce no rule, which degrades safely to "not hidden".
func extractVisibilityRulesFromJS(js string) ([]types.WidgetVisibilityRule, editorConfigExtractStats) {
	var rules []types.WidgetVisibilityRule
	var stats editorConfigExtractStats
	var pending []ternaryThenCandidate // resolved after the loop
	seen := map[string]bool{}          // dedupe propertyKey+condition
	consts := stringArrayConsts(js)    // module-level arrays, for `[…].concat(N)`

	for _, loc := range hideCallRE.FindAllStringIndex(js, -1) {
		stats.TotalHideCalls++
		callStart, argsOpen := loc[0], loc[1] // argsOpen points just past '('
		args, ok := balancedArgs(js, argsOpen)
		if !ok {
			stats.SkippedComplex++
			continue
		}
		listKey, keys, condKeys, ok := hideTargetKeys(args, consts)
		if !ok || (len(keys) == 0 && len(condKeys) == 0) {
			stats.SkippedComplex++
			continue
		}
		// A nested hide sits inside the list's `forEach(function(item, i){…})`, so
		// a guard reading `item.<prop>` is about the ITEM, not the widget.
		itemIdent := ""
		if listKey != "" {
			itemIdent = enclosingForEachParam(js, callStart)
		}
		// A hide in a `switch` arm runs on the arm's case values, not on a guard
		// beside it, so it is read from the switch and never from parseGuard —
		// whose `:` handling would take the arm's label for a ternary's else.
		var (
			cond        types.WidgetVisibilityCondition
			guardText   string
			conjunctive bool
			chain       []types.WidgetVisibilityCondition
		)
		armCond, inSwitchArm, armOK := switchArmCondition(js, callStart, itemIdent)
		if inSwitchArm {
			cond, ok = armCond, armOK
		} else {
			cond, guardText, ok, conjunctive, chain = parseGuard(js, callStart, itemIdent)
		}
		// A guard inside `outer ? ( … inner && hide(x) … )` states only the INNER
		// term; the branch runs on outer too. Collect the enclosing group guards
		// so the rule carries the whole conjunction — Combo box nests three deep,
		// and a rule keeping only the innermost term claims hidden in
		// configurations the editor shows.
		var extra []types.WidgetVisibilityCondition
		if ok && len(chain) > 0 {
			// An else-if branch: its own condition, the negated earlier
			// branches, and whatever groups enclose the chain. Exact or not
			// lifted — this shape had no rule before, so withholding one loses
			// nothing. (An else-branch hide's chain IS its enclosing groups, so
			// this only dedupes it.)
			enclosing, enclosed := enclosingGroupConditions(js, callStart, itemIdent)
			if !enclosed {
				stats.SkippedComplex++
				continue
			}
			extra = dedupeConditions(cond, append(chain, enclosing...))
		} else if ok && conjunctive {
			enclosing, enclosed := enclosingGroupConditions(js, callStart, itemIdent)
			switch {
			case enclosed:
				// The whole chain read: the rule carries every term and is exact.
				extra = dedupeConditions(cond, enclosing)
			case impliesGroupGuard(js, callStart, cond):
				// The group's condition is about the SAME property and holds
				// wherever this one does, so the conjunction reduces to this
				// condition alone. Maps: `"googleMaps"!==B.mapProvider ?
				// (…, "openStreet"===B.mapProvider && hide([apiKey, apiKeyExp]))`.
			default:
				// The chain could not be read in full, so the terms cannot be
				// stored and the rule states one conjunct of a larger condition.
				//
				// That is a reason to withhold a rule this extractor did not
				// previously produce — emitting it would ADD an over-firing rule,
				// as `!1===e.showNumberOfRows` alone does for Datagrid's
				// pagingPosition. It is NOT a reason to drop a rule that the
				// older, single-condition vocabulary already lifted: that rule's
				// accuracy is unchanged by this work, and removing it would lose
				// detection the previous release had. Combo box is where that
				// distinction shows — six of its rules are exactly this shape.
				//
				// So: newly-supported guard shapes must earn their place (the
				// other branch has to hide the property anyway, which makes the
				// single condition safe); shapes that already worked are kept.
				if !isNewlySupportedGuard(guardText) {
					break
				}
				kept := keys[:0:0]
				for _, k := range keys {
					if hiddenInComplementaryBranch(js, callStart, k) {
						kept = append(kept, k)
					}
				}
				if len(kept) == 0 && len(condKeys) == 0 {
					stats.SkippedComplex++
					continue
				}
				keys = kept
			}
		}
		if !ok {
			// `<outer> ? <inner> && hide(...)` — a ternary THEN branch carrying a
			// second condition. Held back and resolved after the loop, once the
			// else branch's rules are known (see ternaryThenCandidate).
			//
			// ProgressCircle's equivalent parses today only because it wraps the
			// branch in parentheses, which the grouping-paren strip above removes;
			// Slider writes it without them, so the guard arrives as
			// `e.showTooltip?"value"===e.tooltipType` and no rule was produced.
			if outer, inner, split := splitTernaryThenGuard(guardText); split {
				aliases := enclosingAliases(js, callStart)
				ic, iok := guardToCondition(inner, false, aliases, itemIdent)
				oc, ook := guardToCondition(outer, false, aliases, itemIdent)
				if iok && ook {
					for _, key := range keys {
						pending = append(pending, ternaryThenCandidate{
							listKey: listKey, key: key, inner: ic, outer: oc,
						})
					}
				}
			}
			if listKey != "" {
				stats.SkippedNested++
			} else {
				stats.SkippedComplex++
			}
			continue
		}
		// A conditional array element carries its OWN condition, which supersedes
		// the enclosing guard: the branch is what selects the property, so the
		// guard alone would mark both branches hidden together (#956).
		if len(condKeys) > 0 {
			aliases := enclosingAliases(js, callStart)
			// In a switch arm the element's condition holds only where the arm
			// runs, so the arm's condition is a term of its own.
			condExtra := extra
			if inSwitchArm {
				condExtra = append([]types.WidgetVisibilityCondition{cond}, extra...)
			}
			for _, tk := range condKeys {
				c, ok := guardToCondition(tk.guard, tk.falsy, aliases, itemIdent)
				if !ok {
					stats.SkippedComplex++
					continue
				}
				sig := listKey + "\x00" + tk.key + "\x00" + c.PropertyKey + c.Operator + c.Value + c.Scope + condsSig(condExtra)
				if seen[sig] {
					continue
				}
				seen[sig] = true
				cc := c
				rules = append(rules, types.WidgetVisibilityRule{
					PropertyKey:     tk.key,
					ListPropertyKey: listKey,
					HiddenWhen:      &cc,
					And:             condExtra,
				})
			}
		}
		stats.Recognized++
		for _, key := range keys {
			sig := listKey + "\x00" + key + "\x00" + cond.PropertyKey + cond.Operator + cond.Value + cond.Scope + condsSig(extra)
			if seen[sig] {
				continue
			}
			seen[sig] = true
			c := cond // copy per rule
			rules = append(rules, types.WidgetVisibilityRule{
				PropertyKey:     key,
				ListPropertyKey: listKey,
				HiddenWhen:      &c,
				And:             extra,
			})
		}
	}

	// Resolve the held-back ternary-then candidates. The inner condition is
	// emitted only when an existing rule hides the SAME property under the
	// negation of the outer one — the else branch — so the pair reads as a
	// correct disjunction rather than a conjunction it cannot express (#238).
	for _, c := range pending {
		comp, ok := negate(c.outer)
		if !ok {
			continue
		}
		covered := false
		for _, r := range rules {
			if r.PropertyKey != c.key || r.ListPropertyKey != c.listKey || r.HiddenWhen == nil {
				continue
			}
			if *r.HiddenWhen == comp {
				covered = true
				break
			}
		}
		if !covered {
			continue
		}
		sig := c.listKey + "\x00" + c.key + "\x00" + c.inner.PropertyKey + c.inner.Operator + c.inner.Value + c.inner.Scope
		if seen[sig] {
			continue
		}
		seen[sig] = true
		ic := c.inner
		rules = append(rules, types.WidgetVisibilityRule{
			PropertyKey:     c.key,
			ListPropertyKey: c.listKey,
			HiddenWhen:      &ic,
		})
		stats.Recognized++
	}
	return rules, stats
}

// ternaryThenCandidate is a hide whose guard is the THEN branch of a ternary AND
// carries a second condition:
//
//	e.showTooltip ? "value" === e.tooltipType && hidePropertyIn(t, e, "tooltip")
//	              : hidePropertiesIn(t, e, ["tooltip", "tooltipType"])
//
// The property is hidden when `outer AND inner`, which one WidgetVisibilityCondition
// cannot express — so the inner condition is emitted only when another rule already
// hides the same property under the NEGATION of the outer one, i.e. the else branch
// covers the rest. The two rules are then a correct disjunction:
//
//	hidden  <=>  !showTooltip  ||  tooltipType == "value"
//
// Without that cover the conjunction is not representable and the hide is skipped,
// which degrades to "not hidden" and is safe. Emitting the inner condition alone
// would prune the property whenever it holds, including where the widget shows it —
// CE0463's mirror image (#238).
type ternaryThenCandidate struct {
	listKey, key string
	inner, outer types.WidgetVisibilityCondition
}

// splitTernaryThenGuard recognises `<outer> ? <inner>` in a guard parseGuard could
// not read, returning the two halves. The split is on the LAST `?`, so a nested
// ternary yields the innermost then-branch, which is the one guarding this call.
func splitTernaryThenGuard(guard string) (outer, inner string, ok bool) {
	i := strings.LastIndex(guard, "?")
	if i <= 0 || i == len(guard)-1 {
		return "", "", false
	}
	// The outer half still carries whatever preceded it (`exports.getProperties=
	// function(e,t){return …`), so reduce it to the trailing expression the same
	// way the main path does.
	outerExpr, _ := lastGuardExpr(strings.TrimSpace(guard[:i]))
	outer = stripReturnPrefix(outerExpr)
	inner = strings.TrimSpace(guard[i+1:])
	if outer == "" || inner == "" {
		return "", "", false
	}
	return outer, inner, true
}

// negate returns the condition that holds exactly when c does not, and whether it
// is expressible.
func negate(c types.WidgetVisibilityCondition) (types.WidgetVisibilityCondition, bool) {
	out := c
	switch c.Operator {
	case "truthy":
		out.Operator = "falsy"
	case "falsy":
		out.Operator = "truthy"
	case "eq":
		out.Operator = "ne"
	case "ne":
		out.Operator = "eq"
	default:
		return out, false
	}
	return out, true
}

// hideTargetKeys returns the object-list property a hide call targets (empty for
// a top-level hide) and the property key(s) it hides.
//
//	hidePropertyIn(t, e, "key")                        → "",       ["key"]
//	hidePropertiesIn(t, e, ["a","b"])                  → "",       ["a","b"]
//	hidePropertyIn(t, e, "groups", i, "key")           → "groups", ["key"]
//	hideNestedPropertiesIn(t, e, "groups", i, ["a"])   → "groups", ["a"]
//
// ok is false for a shape it does not recognize; the caller counts it as skipped
// and emits no rule, which degrades to "not hidden".
func hideTargetKeys(args string, consts map[string][]string) (listKey string, keys []string, condKeys []ternaryKey, ok bool) {
	parts := splitTopLevelCommas(args)
	// Collect string-literal positional args and any array literal.
	var stringArgs []string
	var arrayKeys []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "[") {
			// The argument may be `[…].concat(N, W)`, so read the literal with a
			// balanced scan and resolve the concatenated arrays separately —
			// trimming a trailing "]" would stop at the first element and drop
			// everything after it.
			body, bodyOK := balancedArgs(p, 1)
			if !bodyOK {
				body = strings.TrimSuffix(strings.TrimPrefix(p, "["), "]")
			}
			if bodyOK {
				if extra, extraOK := concatMembers(p[len(body)+2:], consts); extraOK {
					arrayKeys = append(arrayKeys, extra...)
				}
			}
			for _, el := range splitTopLevelCommas(body) {
				if tk, ok := ternaryElement(el); ok {
					// `cond ? "a" : "b"` as an array ELEMENT. Harvesting its string
					// literals would invent a key from the comparison value and mark
					// BOTH branches hidden under the outer guard — see #956.
					condKeys = append(condKeys, tk...)
					continue
				}
				for _, m := range stringLitRE.FindAllStringSubmatch(el, -1) {
					arrayKeys = append(arrayKeys, m[1])
				}
			}
			continue
		}
		if m := stringLitRE.FindStringSubmatch(p); m != nil && strings.HasPrefix(p, `"`) {
			stringArgs = append(stringArgs, m[1])
		}
	}
	if len(arrayKeys) > 0 {
		switch len(stringArgs) {
		case 0:
			return "", arrayKeys, condKeys, true // hidePropertiesIn(obj, obj, [keys])
		case 1:
			return stringArgs[0], arrayKeys, condKeys, true // hideNestedPropertiesIn(…, "groups", i, [keys])
		default:
			return "", nil, condKeys, false
		}
	}
	switch len(stringArgs) {
	case 1:
		return "", stringArgs, condKeys, true // hidePropertyIn(obj, obj, "key")
	case 2:
		return stringArgs[0], stringArgs[1:], condKeys, true // hidePropertyIn(…, "groups", i, "key")
	default:
		return "", nil, condKeys, false
	}
}

// arrayDeclRE finds a module-level array binding — `z=["lazyLoading",…]` in the
// minified bundle. Combo box's biggest hide lists are built by concatenating
// three such arrays onto a literal one, so without resolving them the call is
// counted as recognized while most of its properties are silently dropped.
var arrayDeclRE = regexp.MustCompile(`(?:^|[,;({=\s])([A-Za-z_$][\w$]*)\s*=\s*\[`)

// concatCallRE matches the `.concat(` that follows a hide call's array argument.
var concatCallRE = regexp.MustCompile(`^\s*\.concat\(`)

// stringArrayConsts maps each identifier bound to an array of STRING LITERALS to
// its members. An identifier bound more than once is dropped rather than
// guessed at: the minifier reuses short names across scopes, and resolving one
// to the wrong array would invent hide rules for properties the editor shows.
// Arrays holding anything but string literals (an element list, a data URI
// built by a call) are skipped for the same reason.
func stringArrayConsts(js string) map[string][]string {
	out := map[string][]string{}
	ambiguous := map[string]bool{}
	for _, loc := range arrayDeclRE.FindAllStringSubmatchIndex(js, -1) {
		name := js[loc[2]:loc[3]]
		if ambiguous[name] {
			continue
		}
		body, ok := balancedArgs(js, loc[1]) // loc[1] is just past the '['
		if !ok {
			continue
		}
		members, ok := stringArrayMembers(body)
		if !ok {
			continue
		}
		if prev, seen := out[name]; seen && !sameStrings(prev, members) {
			delete(out, name)
			ambiguous[name] = true
			continue
		}
		out[name] = members
	}
	return out
}

// stringArrayMembers reads an array literal's body as a list of string literals.
// ok is false if ANY element is something else — a partial read would drop
// members without saying so, which is the defect this exists to fix.
func stringArrayMembers(body string) ([]string, bool) {
	if strings.TrimSpace(body) == "" {
		return nil, false
	}
	var out []string
	for _, el := range splitTopLevelCommas(body) {
		el = strings.TrimSpace(el)
		m := stringLitRE.FindStringSubmatch(el)
		if m == nil || !strings.HasPrefix(el, `"`) || len(m[0]) != len(el) {
			return nil, false
		}
		out = append(out, m[1])
	}
	return out, true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// concatMembers reads the `.concat(a, b)` chain trailing an array literal and
// returns the members its arguments contribute. Each argument must be either an
// inline string array or an identifier in consts; anything else (a call such as
// `.concat(i(a))`) yields ok=false, and the caller keeps the literal keys alone
// rather than emitting a list it knows to be short.
func concatMembers(tail string, consts map[string][]string) ([]string, bool) {
	var out []string
	for {
		m := concatCallRE.FindStringIndex(tail)
		if m == nil {
			return out, true
		}
		args, ok := balancedArgs(tail, m[1])
		if !ok {
			return out, false
		}
		for _, a := range splitTopLevelCommas(args) {
			a = strings.TrimSpace(a)
			switch {
			case strings.HasPrefix(a, "["):
				body, ok := balancedArgs(a, 1)
				if !ok {
					return out, false
				}
				members, ok := stringArrayMembers(body)
				if !ok {
					return out, false
				}
				out = append(out, members...)
			default:
				members, ok := consts[a]
				if !ok {
					return out, false
				}
				out = append(out, members...)
			}
		}
		tail = tail[m[1]+len(args)+1:] // past the closing ')', for a chained .concat
	}
}

// ternaryKey is an array element of the form `cond ? "a" : "b"` — a hide whose
// TARGET depends on a condition, rather than the whole call being guarded. Each
// branch becomes its own rule carrying that condition (the then-branch when the
// comparison holds, the else-branch when it does not).
//
// Harvesting the element's string literals instead — which is what the extractor
// did — invents a key from the comparison VALUE and marks both branches hidden
// under the enclosing guard, losing the condition that distinguishes them.
// Measured on File Uploader 2.5.0: `associatedImages` was left ungated, so
// authoring the widget's datasource wrote it alongside `associatedFiles` and the
// build failed CE0463 (#956).
type ternaryKey struct {
	key   string
	guard string
	falsy bool // else-branch: the hide fires when the comparison is FALSE
}

// ternaryElementRE splits `<guard>?"then":"else"` (minified: no spaces).
var ternaryElementRE = regexp.MustCompile(`^(.+?)\?\s*"([^"]*)"\s*:\s*"([^"]*)"$`)

// ternaryElement recognizes a conditional array element and returns one entry
// per branch. ok is false for anything else, so a plain element is unaffected.
func ternaryElement(el string) ([]ternaryKey, bool) {
	m := ternaryElementRE.FindStringSubmatch(strings.TrimSpace(el))
	if m == nil {
		return nil, false
	}
	guard := strings.TrimSpace(m[1])
	return []ternaryKey{
		{key: m[2], guard: guard, falsy: false},
		{key: m[3], guard: guard, falsy: true},
	}, true
}

// forEachParamRE matches the callback parameter list of a `.forEach(function(a,b){`
// immediately before an enclosing block brace. The extra `\(*` is load-bearing:
// the Accordion Mendix ships is compiled as `forEach((function(n,o){…}))`, and
// without it the regex reads `function` itself as the parameter name — which
// scoped every nested condition to the widget and dropped the group's own
// `initialCollapsedState` guard on the floor.
var forEachParamRE = regexp.MustCompile(`forEach\(\s*\(*\s*(?:function\b\s*)?\(\s*([A-Za-z_$][\w$]*)`)

// enclosingForEachParam returns the name of the first parameter of the
// `.forEach(function(item, index){…})` callback whose body encloses pos, or ""
// when pos is not inside one.
//
// It matters because a nested hide's guard can read either object: the Accordion
// writes `"dynamic" !== item.initialCollapsedState && hide(…, "initiallyCollapsed")`
// (an ITEM property) in the same callback as
// `(…) && widget.collapsible || hideNested(…)` (a WIDGET property). Without the
// distinction the two conditions are indistinguishable once resolveRef has
// dropped the base identifier.
func enclosingForEachParam(js string, pos int) string {
	for i := 0; i < 8; i++ { // bounded: a hide is never nested this deep
		open := enclosingOpener(js, pos)
		if open < 0 {
			return ""
		}
		if js[open] == '{' {
			// Look just before the brace for the callback's parameter list.
			start := open - 80
			if start < 0 {
				start = 0
			}
			if m := forEachParamRE.FindStringSubmatch(js[start:open]); m != nil && m[1] != "function" {
				return m[1]
			}
		}
		pos = open
	}
	return ""
}

// enclosingOpener returns the index of the nearest unbalanced opening bracket
// before pos, or -1 when there is none.
func enclosingOpener(js string, pos int) int {
	depth := 0
	for i := pos - 1; i >= 0; i-- {
		switch js[i] {
		case '}', ')', ']':
			depth++
		case '{', '(', '[':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

// nsPrefix returns where the widget-editor namespace prefix before a hide call
// starts when it ends s (`<ident>.`), or -1. The minifier names it per widget
// (`_.`, `D.`, `M.`, `j.`, `A.`, …), so it must be matched generically rather
// than hard-coded to `_.`. A suffix scan, not a regexp, for the reason given
// at hideCallTail.
func nsPrefix(s string) int {
	if !strings.HasSuffix(s, ".") {
		return -1
	}
	start := len(s) - 1
	for start > 0 && isIdentByte(s[start-1]) {
		start--
	}
	// An identifier does not start with a digit: skip leading ones.
	for start < len(s)-1 && s[start] >= '0' && s[start] <= '9' {
		start++
	}
	if start == len(s)-1 {
		return -1
	}
	return start
}

// parseGuard reads the guard expression immediately preceding a hide call and
// converts it to a WidgetVisibilityCondition. callStart points at the hide
// function name; the connector just before it is `&&`, `||`, or `?`.
// The fourth result marks a guard that is one conjunct of a larger condition
// (it sits inside a grouping paren carrying its own guard). Such a rule is only
// safe when the ternary's other branch hides the same property anyway — see
// hiddenInComplementaryBranch, which the caller consults.
//
// The fifth result holds the further conditions the rule must carry: for a
// guard that heads an else-if branch (`A ? x : guard && hide(…)`), the negated
// conditions of the chain's earlier branches — see elseChainTerms; for an
// else-branch hide inside a guarded group, that group's conditions.
func parseGuard(js string, callStart int, itemIdent string) (types.WidgetVisibilityCondition, string, bool, bool, []types.WidgetVisibilityCondition) {
	pre := strings.TrimRight(js[:callStart], " ")
	// Strip the widget-editor namespace prefix (any `<ident>.`, not just `_.`).
	if at := nsPrefix(pre); at >= 0 {
		pre = pre[:at]
	}
	pre = strings.TrimRight(pre, " ")
	// Strip an optional grouping paren: `cond && ( hide(...), … )` groups several
	// hides under one condition; the first hide sits right after the `(`.
	//
	// The LATER hides in that group sit after a comma instead, so their guard is
	// their sibling's. Skipping back over the preceding call expressions reaches
	// the same `(` and the same condition. Without this the second hide in
	// `cond ? (hide(a), hide(b))` gets no rule at all: `pre` ends with `,`, which
	// is not a connector, and the property reads as always visible. Combobox's
	// `attributeEnumeration` is that case — it is the FIRST hide of one group and
	// the second of another, so it was extracted once and missed once.
	pre = stripGroupSiblings(pre)
	if strings.HasSuffix(pre, "(") {
		pre = strings.TrimRight(pre[:len(pre)-1], " ")
	}
	// Identify the connector.
	var falsy bool // || connector or !prefix ⇒ hide when guard is falsy
	elseBranch := false
	switch {
	case strings.HasSuffix(pre, "&&"):
		pre = pre[:len(pre)-2]
	case strings.HasSuffix(pre, "?"):
		pre = pre[:len(pre)-1]
	case strings.HasSuffix(pre, "||"):
		pre = pre[:len(pre)-2]
		falsy = true
	case strings.HasSuffix(pre, ":"):
		// The ELSE branch of a ternary: `cond ? (…hides…) : hide(…)`. The hide
		// fires when cond is falsy, and cond is not the text next to the `:` —
		// it sits before the matching `?`, past the whole then-branch.
		//
		// ProgressCircle 3.3.2 is the case that made this matter:
		//
		//	return e.showLabel
		//	  ? ("text" !== e.labelType && hidePropertyIn(t,e,"labelText"))
		//	  : hidePropertiesIn(t,e,["customLabel","labelText","labelType"])
		//
		// Without this branch only the inner `!==` rule was seen, so labelText
		// read as VISIBLE whenever labelType was "text" — its default — even
		// with showLabel false. See the CE0463 that produced (ledger #104).
		cond, ok := ternaryCondition(pre[:len(pre)-1])
		if !ok {
			return types.WidgetVisibilityCondition{}, pre, false, false, nil
		}
		pre = cond
		falsy = true
		elseBranch = true
	default:
		return types.WidgetVisibilityCondition{}, pre, false, false, nil
	}
	guard, boundary, boundaryAt := lastGuardExprAt(pre)
	guard = stripReturnPrefix(guard) // getProperties' first statement is `return <guard> && hide…`
	if guard == "" {
		return types.WidgetVisibilityCondition{}, guard, false, false, nil
	}
	// Skip guards nested inside a larger expression. A clean statement-level guard
	// is bounded by a statement separator (`,`, `;`, `{`, or start-of-input); a
	// boundary of `(`/`?`/`:`/`|` means the guard is one operand of a compound
	// or ternary condition (e.g. `"web"===r ? (e.advanced || hide(...))`), of which
	// we'd capture only a fragment — producing a WRONG rule that over-fires. Better
	// to emit no rule (→ "not hidden" → template default), which is safe.
	//
	// `&` is the one compound boundary that is safe, and only for the `||`
	// connector: in `X && Y || hide`, the hide fires when `X && Y` is falsy, and
	// `Y` falsy is sufficient for that WHATEVER X is. So "hide when Y falsy" is
	// implied by the code rather than guessed — it may miss the case where X
	// alone is falsy, never fire where the widget would not hide. This is the
	// Accordion's shape, and the reason its group properties went unflagged:
	//
	//	(e.advancedMode || "web" !== platform) && e.collapsible
	//	  || hideNestedPropertiesIn(t, e, "groups", i, ["initialCollapsedState", …])
	//
	// The `&&` connector gets no such rule: there, hiding needs BOTH operands
	// truthy, which a single condition cannot express.
	// A `,` boundary reads as statement-level but is not one when the guard sits
	// inside a grouping paren that carries its OWN condition: the hide fires on
	// the CONJUNCTION, and a single condition can only express one conjunct —
	// which over-fires, hiding a property in configurations where the editor
	// shows it. Datagrid is the case:
	//
	//	e.pagination ? hide("showNumberOfRows")
	//	             : (hide("showPagingButtons"), !1===e.showNumberOfRows && hide("pagingPosition"))
	//
	// pagingPosition is hidden only when pagination is off AND showNumberOfRows
	// is false. Reading the `!1===` alone hides it whenever the row count is off,
	// pagination or not — and `pagination` defaults ON, so that is the common
	// configuration. Emitting no rule leaves it visible, which is the safe way to
	// be wrong.
	// Conjunctive only when the enclosing group's own condition is about the SAME
	// object this guard reads — i.e. another of the widget's properties.
	//
	// editorConfig's getProperties also receives the target PLATFORM, and widgets
	// branch on it: TreeNode hides its icon properties under
	// `"web"===platform ? (e.advancedMode || hide([...])) : …`. That outer
	// conjunct is not part of the widget's configuration and is always true for
	// the pages MDL writes, so folding it away loses nothing — whereas dropping
	// Datagrid's `e.pagination` conjunct, a real property with a real default,
	// changes what the rule claims.
	conjunctive := boundary == ',' &&
		insideOpenGroup(pre[:len(pre)-len(guard)]) &&
		sameReceiver(groupGuard(js, callStart), guard)
	var chain []types.WidgetVisibilityCondition
	switch boundary {
	case 0, ',', ';', '{':
		// clean
	case ':':
		// The guard heads an else branch: `A ? (…) : guard && hide(…)`, or the
		// else-if `A ? (…) : guard ? (hide(…), …)`. The hide fires on the
		// guard AND on every earlier branch's condition being false.
		terms, ok := elseChainTerms(pre[:boundaryAt], enclosingAliases(js, callStart), itemIdent)
		if !ok {
			return types.WidgetVisibilityCondition{}, guard, false, false, nil
		}
		chain = terms
	case '?':
		// The guard is the THEN branch of a ternary and carries its own condition
		// (`outer ? inner && hide(...)`). Hand back the WHOLE expression, `?`
		// included, so the caller can split it and decide whether the else branch
		// makes the pair expressible — see ternaryThenCandidate (#238).
		return types.WidgetVisibilityCondition{}, pre, false, false, nil
	case '&':
		if !falsy {
			// `A && B && hide(…)`: the hide fires when every conjunct holds.
			// The rule's And carries the earlier ones. Combo box hides
			// noOptionsText this way — `"none"===filterType && "rowclick"!==
			// selectionMethod && hide(["noOptionsText"])` — and the rule was
			// dropped, so mxcli wrote a text template Studio Pro stores as
			// null there, and mx check reported CE0463 (ako/mxcli#2264).
			// Every conjunct must read and the chain must start a statement;
			// otherwise no rule, as before.
			terms, ok := andChainTerms(pre, boundaryAt, enclosingAliases(js, callStart), itemIdent)
			if !ok {
				return types.WidgetVisibilityCondition{}, guard, false, false, nil
			}
			chain = terms
		}
	case '(':
		// The paren heads a STATEMENT rather than grouping a guarded branch:
		// terser folds the expression statements before a `switch`/`if` into
		// its head, and their contents run unconditionally. Timeline's first
		// hide is the THEN branch of
		//
		//	switch(e.customVisualization ? hide([…]) : hide([…]), e.groupEvents || hide([…]), e.groupByKey){…}
		//
		// and was dropped as "one operand of a compound condition" while its
		// else branch, read through trailingExpr, was lifted (ako/mxcli#1762).
		// A call's paren is not this case — the call can itself be guarded.
		// boundaryAt indexes js only while pre is still a prefix of it, which
		// an else-branch guard (cut out by ternaryCondition) is not.
		if elseBranch || !strings.HasPrefix(js, pre) || !statementHeadParen(js, boundaryAt) {
			return types.WidgetVisibilityCondition{}, guard, false, false, nil
		}
	default:
		return types.WidgetVisibilityCondition{}, guard, false, false, nil
	}
	aliases := enclosingAliases(js, callStart)
	c, ok := guardToCondition(guard, falsy, aliases, itemIdent)
	// An else-branch hide inside a guarded group, `G ? (…, C ? hide(a) :
	// hide(b), …)`, fires on G as well as on !C. Its guard was isolated from the
	// group by the ternary walk above, so it read as statement-level and b was
	// hidden wherever C is false — Combo box's `selectedItemsSorting`, hidden by
	// the database branch whenever its item selection is not Multi, read as
	// hidden in association mode too. Carried only when the whole enclosing
	// chain reads; otherwise the rule stays as it was lifted before.
	if ok && elseBranch && chain == nil {
		if enclosing, enclosed := enclosingGroupConditions(js, callStart, itemIdent); enclosed && len(enclosing) > 0 {
			chain = enclosing
		}
	}
	return c, guard, ok, conjunctive, chain
}

// ternaryCondition returns the text preceding the `?` that matches a trailing
// `:`, i.e. the condition of the ternary whose else-branch is about to start.
//
// It walks backwards balancing brackets, and counts nested ternaries: every
// further `:` seen at depth 0 needs its own `?` before the one we want. A `?`
// that never arrives means this `:` was not a ternary at all (a label, or an
// object literal key), and the caller emits no rule — which is the safe
// direction, since a wrong rule hides a property the user set.
func ternaryCondition(pre string) (string, bool) {
	depth, pending := 0, 0
	for i := len(pre) - 1; i >= 0; i-- {
		switch pre[i] {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			if depth == 0 {
				return "", false // ran out of enclosing expression
			}
			depth--
		case ':':
			if depth == 0 {
				pending++
			}
		case '?':
			if depth == 0 {
				if pending == 0 {
					return trailingExpr(pre[:i]), true
				}
				pending--
			}
		}
	}
	return "", false
}

// trailingExpr returns the last expression in s, bounded by a statement
// separator. The ternary is usually not the first thing in its function —
// ProgressCircle's is preceded by a whole `switch` — and returning everything
// back to the start hands the caller a fragment with an unbalanced `}` that no
// guard parser can read.
func trailingExpr(s string) string {
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')', ']':
			depth++
		case '(', '[':
			if depth == 0 {
				return strings.TrimSpace(s[i+1:])
			}
			depth--
		case '}', '{', ';', ',':
			// A brace at depth 0 closes or opens a preceding BLOCK, so the
			// expression starts after it. Braces are not counted as nesting here
			// for that reason — an object literal inside the condition would be
			// bounded by its own parens.
			if depth == 0 {
				return strings.TrimSpace(s[i+1:])
			}
		}
	}
	return strings.TrimSpace(s)
}

// stripReturnPrefix removes a leading `return` keyword from a guard expression.
// A widget's getProperties body often starts `return <cond> && hide(…), …`, so
// the first guard is prefixed with `return` (minified: `return"none"===…`).
func stripReturnPrefix(g string) string {
	g = strings.TrimSpace(g)
	const kw = "return"
	if strings.HasPrefix(g, kw) {
		rest := g[len(kw):]
		// Only strip when `return` is a standalone keyword, not the start of an
		// identifier like `returnValue`.
		if rest == "" || !isIdentByte(rest[0]) {
			return strings.TrimSpace(rest)
		}
	}
	return g
}

// isIdentByte reports whether b can appear in a JS identifier.
func isIdentByte(b byte) bool {
	return b == '_' || b == '$' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// lastGuardExpr returns the balanced expression ending at the end of `pre`,
// bounded by the previous top-level separator, and the boundary byte that
// terminated it (0 for start-of-input). The boundary lets the caller reject
// guards nested inside a compound/ternary expression.
func lastGuardExpr(pre string) (string, byte) {
	expr, boundary, _ := lastGuardExprAt(pre)
	return expr, boundary
}

// lastGuardExprAt is lastGuardExpr plus the index of the boundary byte in pre
// (-1 for start-of-input), for a caller that has to keep walking past it.
func lastGuardExprAt(pre string) (string, byte, int) {
	depth := 0
	var boundary byte
	i := len(pre) - 1
	for ; i >= 0; i-- {
		c := pre[i]
		switch c {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			if depth == 0 {
				boundary = c
				goto done
			}
			depth--
		case ',', ';', ':', '?', '&', '|':
			if depth == 0 {
				boundary = c
				goto done
			}
		}
	}
done:
	return strings.TrimSpace(pre[i+1:]), boundary, i
}

// andChainTerms reads the conjuncts before a guard that follows `&&` at index
// at (the second `&`) in pre: `A && B && <guard>` gives [B, A]. It fails unless
// each conjunct is a condition the guard vocabulary can represent and the chain
// begins a statement (start of input, `,`, `;` or `{`) — a chain inside a
// larger expression or behind a `||` cannot be lifted without changing what
// the rule claims.
func andChainTerms(pre string, at int, aliases map[string]string, itemIdent string) ([]types.WidgetVisibilityCondition, bool) {
	var out []types.WidgetVisibilityCondition
	for depth := 0; depth < maxGroupNesting; depth++ {
		if at < 1 || pre[at-1] != '&' {
			return nil, false
		}
		before := pre[:at-1]
		text, boundary, next := lastGuardExprAt(before)
		text = stripReturnPrefix(text)
		if text == "" {
			return nil, false
		}
		c, ok := guardToCondition(text, false, aliases, itemIdent)
		if !ok {
			return nil, false
		}
		out = append(out, c)
		switch boundary {
		case '&':
			pre, at = before, next
		case 0, ',', ';', '{':
			return out, true
		default:
			return nil, false
		}
	}
	return nil, false
}

// elseChainTerms returns the conditions under which an ELSE branch runs, for
// the branch opened by the `:` that follows before (exclusive): the negation
// of the ternary's own condition and, when that condition heads an else branch
// in turn, of every earlier one in the chain.
//
//	A ? x : B ? y : <here>     →  [!B, !A]
//
// Combo box selects its option source this way, and the database and static
// branches' hides were never lifted: their guard's boundary is the outer `:`,
// which reads as "one fragment of a larger expression" (mendixlabs/mxcli#1583).
// The chain ends at a statement separator or a group's `(` — the caller's
// enclosing-group walk carries on from there. Any condition the guard
// vocabulary cannot represent fails the whole chain: a branch lifted on its own
// condition fires where an unread earlier branch runs instead.
func elseChainTerms(before string, aliases map[string]string, itemIdent string) ([]types.WidgetVisibilityCondition, bool) {
	var out []types.WidgetVisibilityCondition
	for depth := 0; depth < maxGroupNesting; depth++ {
		q := matchingTernaryQuestion(before)
		if q < 0 {
			return nil, false
		}
		text, boundary, at := lastGuardExprAt(before[:q])
		c, ok := guardToCondition(stripReturnPrefix(text), true, aliases, itemIdent)
		if !ok {
			return nil, false
		}
		out = append(out, c)
		switch boundary {
		case ':':
			before = before[:at]
		case 0, ',', ';', '{', '(':
			return out, true
		default:
			return nil, false
		}
	}
	return nil, false
}

var (
	eqCmpRE  = regexp.MustCompile(`^"([^"]*)"===([A-Za-z_$][\w$.]*)$`)
	neCmpRE  = regexp.MustCompile(`^"([^"]*)"!==([A-Za-z_$][\w$.]*)$`)
	eqCmpRE2 = regexp.MustCompile(`^([A-Za-z_$][\w$.]*)==="([^"]*)"$`)
	neCmpRE2 = regexp.MustCompile(`^([A-Za-z_$][\w$.]*)!=="([^"]*)"$`)
	refRE    = regexp.MustCompile(`^(!?)([A-Za-z_$][\w$.]*)$`)

	// Shapes that mean "the author has not picked anything", which editorConfig
	// writes two ways. Both are narrower than falsy — see the `empty` operator's
	// note in mdl/types/widget_visibility.go.
	//   null === ref   /  ref === null
	nullCmpRE  = regexp.MustCompile(`^null(===|!==)([A-Za-z_$][\w$.]*)$`)
	nullCmpRE2 = regexp.MustCompile(`^([A-Za-z_$][\w$.]*)(===|!==)null$`)
	//   0 === ref.length  /  ref.length === 0
	lenCmpRE  = regexp.MustCompile(`^0(===|!==)([A-Za-z_$][\w$.]*)\.length$`)
	lenCmpRE2 = regexp.MustCompile(`^([A-Za-z_$][\w$.]*)\.length(===|!==)0$`)

	// Minified booleans: terser writes `false` as `!1` and `true` as `!0`, so a
	// guard reading `!1===e.showFooter` is `false===e.showFooter`. Mapped to
	// eq/ne against the literal rather than to falsy/truthy, because `===false`
	// does NOT fire on an unset property and falsy would.
	boolCmpRE  = regexp.MustCompile(`^!([01])(===|!==)([A-Za-z_$][\w$.]*)$`)
	boolCmpRE2 = regexp.MustCompile(`^([A-Za-z_$][\w$.]*)(===|!==)!([01])$`)

	// ["a","b"].includes(ref) — a set-membership test over enum values.
	includesRE = regexp.MustCompile(`^\[([^\]]*)\]\.includes\(([A-Za-z_$][\w$.]*)\)$`)
)

// guardToCondition parses a single guard expression into a visibility
// condition, resolving a bare identifier through the scope's alias map.
func guardToCondition(guard string, falsy bool, aliases map[string]string, itemIdent string) (types.WidgetVisibilityCondition, bool) {
	// A comparison guard obeys the connector's polarity just as a bare reference
	// does: with `||`, or in a ternary's ELSE branch, the hide fires when the
	// comparison is FALSE, so `===` becomes "not equal" and `!==` becomes
	// "equal". The Accordion is where this shows:
	//
	//	"text" === item.headerRenderMode
	//	  ? (hide(…, "headerContent"), …)
	//	  : (hide(…, "headerText"), hide(…, "headerHeading"))
	//
	// headerContent is hidden when the mode IS "text"; headerText when it is NOT.
	// Reading both as `eq` marks headerText hidden in exactly the configuration
	// where it is the property being used.
	eqOp, neOp := "eq", "ne"
	if falsy {
		eqOp, neOp = "ne", "eq"
	}
	// "V" === ref  /  ref === "V"
	if m := eqCmpRE.FindStringSubmatch(guard); m != nil {
		if key, scope, ok := resolveRef(m[2], aliases, itemIdent); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: eqOp, Value: m[1], Scope: scope}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := eqCmpRE2.FindStringSubmatch(guard); m != nil {
		if key, scope, ok := resolveRef(m[1], aliases, itemIdent); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: eqOp, Value: m[2], Scope: scope}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := neCmpRE.FindStringSubmatch(guard); m != nil {
		if key, scope, ok := resolveRef(m[2], aliases, itemIdent); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: neOp, Value: m[1], Scope: scope}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := neCmpRE2.FindStringSubmatch(guard); m != nil {
		if key, scope, ok := resolveRef(m[1], aliases, itemIdent); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: neOp, Value: m[2], Scope: scope}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	// null === ref — "no datasource / action picked". Distinct from falsy.
	if m := nullCmpRE.FindStringSubmatch(guard); m != nil {
		return emptyCond(m[2], m[1], falsy, aliases, itemIdent)
	}
	if m := nullCmpRE2.FindStringSubmatch(guard); m != nil {
		return emptyCond(m[1], m[2], falsy, aliases, itemIdent)
	}
	// 0 === ref.length — the same claim about a string or list property.
	if m := lenCmpRE.FindStringSubmatch(guard); m != nil {
		return emptyCond(m[2], m[1], falsy, aliases, itemIdent)
	}
	if m := lenCmpRE2.FindStringSubmatch(guard); m != nil {
		return emptyCond(m[1], m[2], falsy, aliases, itemIdent)
	}
	// !1 === ref  /  ref === !0 — minified boolean comparison.
	if m := boolCmpRE.FindStringSubmatch(guard); m != nil {
		return boolCond(m[3], m[2], m[1], falsy, aliases, itemIdent)
	}
	if m := boolCmpRE2.FindStringSubmatch(guard); m != nil {
		return boolCond(m[1], m[2], m[3], falsy, aliases, itemIdent)
	}
	// ["a","b"].includes(ref) — set membership.
	if m := includesRE.FindStringSubmatch(guard); m != nil {
		members := stringLitRE.FindAllStringSubmatch(m[1], -1)
		if len(members) == 0 {
			return types.WidgetVisibilityCondition{}, false
		}
		vals := make([]string, 0, len(members))
		for _, mm := range members {
			if strings.ContainsRune(mm[1], ',') {
				// A comma inside a member would make the joined Value ambiguous.
				// Not observed in any marketplace widget; refuse rather than
				// store a set that decodes wrong.
				return types.WidgetVisibilityCondition{}, false
			}
			vals = append(vals, mm[1])
		}
		key, scope, ok := resolveRef(m[2], aliases, itemIdent)
		if !ok {
			return types.WidgetVisibilityCondition{}, false
		}
		op := "in"
		if falsy {
			op = "notin"
		}
		return types.WidgetVisibilityCondition{PropertyKey: key, Operator: op, Value: strings.Join(vals, ","), Scope: scope}, true
	}
	// bare ref (truthy) or !ref (falsy), combined with the connector polarity:
	//   ref && hide   → hide when ref truthy
	//   ref || hide   → hide when ref falsy   (falsy==true here)
	//   !ref && hide  → hide when ref falsy
	//   ref ? hide:…  → hide when ref truthy
	if m := refRE.FindStringSubmatch(guard); m != nil {
		key, scope, ok := resolveRef(m[2], aliases, itemIdent)
		if !ok {
			return types.WidgetVisibilityCondition{}, false
		}
		neg := (m[1] == "!")
		wantFalsy := falsy != neg // XOR: || or ! flips polarity (both flips cancel)
		op := "truthy"
		if wantFalsy {
			op = "falsy"
		}
		return types.WidgetVisibilityCondition{PropertyKey: key, Operator: op, Scope: scope}, true
	}
	return types.WidgetVisibilityCondition{}, false
}

// impliesGroupGuard reports whether cond alone entails the enclosing group's
// condition, which happens when both constrain the same property and cond pins
// it to a value the group's condition accepts. The conjunction is then
// redundant and the single condition is exact rather than an over-fire.
func impliesGroupGuard(js string, callStart int, cond types.WidgetVisibilityCondition) bool {
	if cond.Operator != "eq" || cond.PropertyKey == "" {
		return false
	}
	// A group in an else-if branch carries the chain's negations as well,
	// which cond need not imply.
	if _, elseIf, _ := groupElseChain(js, callStart, ""); elseIf {
		return false
	}
	text := groupGuard(js, callStart)
	if text == "" {
		return false
	}
	outer, ok := guardToCondition(text, groupIsElseBranch(js, callStart), enclosingAliases(js, callStart), "")
	if !ok || outer.PropertyKey != cond.PropertyKey {
		return false
	}
	// The group's condition must HOLD where this one does. Note the sense: the
	// group guard is the condition under which the branch RUNS, and the branch
	// running is what makes the hide fire, so it must be satisfied — Hidden()
	// here is just the evaluator, not a claim about hiding.
	return outer.Hidden(map[string]string{cond.PropertyKey: cond.Value})
}

// groupIsElseBranch reports whether the group enclosing callStart is the ELSE
// branch of a ternary, in which case its condition is negated.
func groupIsElseBranch(js string, callStart int) bool {
	open := enclosingGroupOpen(js, callStart)
	if open < 0 {
		return false
	}
	head := strings.TrimRight(js[:open], " ")
	return strings.HasSuffix(head, ":") || strings.HasSuffix(head, "||")
}

// groupGuard returns the condition text attached to the grouping paren that
// encloses the hide at callStart ("" when there is none).
func groupGuard(js string, callStart int) string {
	open := enclosingGroupOpen(js, callStart)
	if open < 0 {
		return ""
	}
	head := strings.TrimRight(js[:open], " ")
	for _, c := range []string{"&&", "||"} {
		if strings.HasSuffix(head, c) {
			return stripReturnPrefix(operandBefore(head[:len(head)-2]))
		}
	}
	for _, c := range []string{"?", ":"} {
		if strings.HasSuffix(head, c) {
			head = head[:len(head)-1]
			if c == ":" {
				q := matchingTernaryQuestion(head)
				if q < 0 {
					return ""
				}
				head = head[:q]
			}
			// An else-if group, `A ? (…) : B ? ( … )`: its condition is B, not
			// the whole chain trailingExpr would return — which is no
			// comparison, so every hide in the group went unread or lost its
			// conjunction (mendixlabs/mxcli#1583). The negated A is
			// groupElseChain's.
			if g, boundary := lastGuardExpr(head); boundary == ':' && g != "" {
				return g
			}
			// trailingExpr, not lastGuardExpr: the ternary is rarely the first
			// thing in its function — ProgressCircle's is preceded by a whole
			// `switch` — and taking everything back to the enclosing `{` hands
			// the guard parser a fragment with an unbalanced `}`, which reads as
			// "unsupported shape" and drops a sound rule.
			return stripReturnPrefix(trailingExpr(head))
		}
	}
	return ""
}

// operandBefore returns the single expression immediately to the left of a
// connector, which is the group's own condition.
//
// `trailingExpr` alone is too greedy here. It stops at a STATEMENT separator,
// and a chained ternary contains none — so for Combo box's
//
//	["enumeration","boolean"].includes(t.optionsSourceType)
//	  ? ( …hides… )
//	  : "association"===t.optionsSourceType && ( …hides… )
//
// it returns the whole `A ? (…) : B` expression as the "condition" of the `&&`
// group, which is not a comparison, so the chain reads as unreadable and six
// rules go unlifted. `lastGuardExpr` bounds at `:` and `?` as well and yields
// exactly `"association"===t.optionsSourceType`.
//
// It is not a straight swap: `lastGuardExpr` bounds at `{` too, so where the
// expression is preceded by a block — ProgressCircle's ternary follows a whole
// `switch` — it hands back a fragment with an unbalanced `}`. So take
// lastGuardExpr's answer only when it stopped at a boundary INSIDE an
// expression (`:`, `?`, `,`), and fall back to trailingExpr otherwise.
func operandBefore(head string) string {
	guard, boundary := lastGuardExpr(head)
	switch boundary {
	case ':', '?', ',':
		if guard != "" {
			return guard
		}
	}
	return trailingExpr(head)
}

// matchingTernaryQuestion returns the index of the `?` matching the `:` that
// ends head, skipping over parenthesised groups and nested ternaries. A plain
// LastIndexByte finds the innermost `?` instead — in Maps that is a nested
// `B.geodecodeApiKey?…`, whose receiver is the widget, so the outer
// platform test read as a configuration conjunct and dropped a sound rule.
func matchingTernaryQuestion(head string) int {
	depth, pending := 0, 0
	inStr := byte(0)
	for i := len(head) - 1; i >= 0; i-- {
		c := head[i]
		if inStr != 0 {
			if c == inStr && (i == 0 || head[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
		case ')':
			depth++
		case '(':
			if depth == 0 {
				return -1 // ran out of the enclosing group
			}
			depth--
		case ':':
			if depth == 0 {
				pending++
			}
		case '?':
			if depth == 0 {
				if pending == 0 {
					return i
				}
				pending--
			}
		case '{', ';':
			if depth == 0 {
				return -1
			}
		}
	}
	return -1
}

// sameReceiver reports whether two guards read properties off the same object.
// An empty receiver on either side (a bare identifier, a literal-only guard)
// answers false: not demonstrably the same object, so not treated as a
// configuration conjunct.
func sameReceiver(a, b string) bool {
	ra, rb := guardReceiver(a), guardReceiver(b)
	return ra != "" && ra == rb
}

// guardReceiver returns the object part of the first `obj.prop` reference in a
// guard, or "" when it reads no property off an object.
func guardReceiver(guard string) string {
	m := receiverRE.FindStringSubmatch(guard)
	if m == nil {
		return ""
	}
	return m[1]
}

var receiverRE = regexp.MustCompile(`([A-Za-z_$][\w$]*)\.[A-Za-z_$][\w$]*`)

// dedupeConditions drops terms equal to the rule's own condition or to an
// earlier term. The innermost enclosing group is often the very group whose
// guard the hide already carries — `cond ? (hide(x), …)` gives the first hide
// its condition through the paren strip — and a conjunction repeating a term
// says nothing extra while reading as though it did.
func dedupeConditions(own types.WidgetVisibilityCondition, cs []types.WidgetVisibilityCondition) []types.WidgetVisibilityCondition {
	seen := map[types.WidgetVisibilityCondition]bool{own: true}
	out := cs[:0:0]
	for _, c := range cs {
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// isNewlySupportedGuard reports whether a guard is one of the shapes this
// extractor learned alongside conjunctions — `null===x`, a minified boolean,
// `0===x.length`, `["a","b"].includes(x)`. Those had no rule before, so
// withholding one under a conjunction loses nothing; the older shapes did, and
// withholding theirs would be a regression.
func isNewlySupportedGuard(guard string) bool {
	for _, re := range []*regexp.Regexp{
		nullCmpRE, nullCmpRE2, lenCmpRE, lenCmpRE2, boolCmpRE, boolCmpRE2, includesRE,
	} {
		if re.MatchString(guard) {
			return true
		}
	}
	return false
}

// condsSig renders conditions into a dedupe key.
func condsSig(cs []types.WidgetVisibilityCondition) string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString("\x00")
		b.WriteString(c.PropertyKey)
		b.WriteString(c.Operator)
		b.WriteString(c.Value)
		b.WriteString(c.Scope)
	}
	return b.String()
}

// enclosingGroupConditions walks outward from a hide call, collecting the
// condition of every grouping paren that encloses it, innermost first.
//
// ok is false when any link cannot be read as a condition — a platform test, a
// computed expression, a shape the guard vocabulary does not cover. The caller
// then emits nothing rather than a partial conjunction.
//
// A guard about something other than the widget's own properties is SKIPPED
// rather than failing the walk: editorConfig's getProperties also receives the
// target platform, and widgets branch on it (`"web"===platform ? (…)`). That
// term is not part of the configuration an MDL author writes and is always true
// for the pages MDL produces, so folding it away loses nothing.
func enclosingGroupConditions(js string, callStart int, itemIdent string) ([]types.WidgetVisibilityCondition, bool) {
	var out []types.WidgetVisibilityCondition
	at := callStart
	for depth := 0; depth < maxGroupNesting; depth++ {
		open := enclosingGroupOpen(js, at)
		if open < 0 {
			return out, true // reached statement level: the chain is complete
		}
		text := groupGuard(js, at)
		if text == "" {
			return nil, false
		}
		c, ok := guardToCondition(text, groupIsElseBranch(js, at), enclosingAliases(js, at), itemIdent)
		if ok {
			out = append(out, c)
		} else if guardReceiver(text) != "" {
			// Reads a property off an object but is not a shape we understand —
			// a real term we cannot represent, so the conjunction is incomplete.
			return nil, false
		}
		// A group whose condition heads an else branch runs only where the
		// chain's earlier conditions are false.
		chain, elseIf, chainOK := groupElseChain(js, at, itemIdent)
		switch {
		case chainOK:
			out = append(out, chain...)
		case elseIf:
			return nil, false
		}
		at = open
	}
	return nil, false
}

// groupElseChain returns the negated earlier conditions of the ternary chain
// whose else branch the group enclosing at is guarded in:
//
//	A ? (…) : B ? ( …at… )       → [!A]
//	A ? (…) : B && ( …at… )      → [!A]
//
// elseIf reports whether the group's condition heads an else branch at all; ok
// whether the chain was read. A `?`/`:` group whose chain is unreadable is not
// representable (elseIf && !ok). An `&&`/`||` group keeps the reading it had
// before chains were followed — its rule stands on the group's own condition —
// so an unreadable chain there reports elseIf false rather than withdrawing a
// rule an older release lifted.
func groupElseChain(js string, at int, itemIdent string) (chain []types.WidgetVisibilityCondition, elseIf, ok bool) {
	open := enclosingGroupOpen(js, at)
	if open < 0 {
		return nil, false, false
	}
	head := strings.TrimRight(js[:open], " ")
	conditional := false
	switch {
	case strings.HasSuffix(head, "&&"), strings.HasSuffix(head, "||"):
		head = head[:len(head)-2]
	case strings.HasSuffix(head, "?"):
		head = head[:len(head)-1]
		conditional = true
	case strings.HasSuffix(head, ":"):
		q := matchingTernaryQuestion(head[:len(head)-1])
		if q < 0 {
			return nil, false, false
		}
		head = head[:q]
		conditional = true
	default:
		return nil, false, false
	}
	_, boundary, boundaryAt := lastGuardExprAt(head)
	if boundary != ':' {
		return nil, false, false
	}
	terms, read := elseChainTerms(head[:boundaryAt], enclosingAliases(js, at), itemIdent)
	if !read {
		return nil, conditional, false
	}
	return terms, true, true
}

// maxGroupNesting bounds the outward walk. Combo box reaches three; a file that
// nests further answers "cannot read" and its rules are simply not lifted.
const maxGroupNesting = 8

// hiddenInComplementaryBranch reports whether the OTHER branch of the ternary
// enclosing this hide also hides propertyKey.
//
// That is what separates a conjunctive guard that is merely imprecise from one
// that is wrong. Both of these hide on `outer AND inner`, and both are stored as
// `inner` alone:
//
//	ProgressBar   showLabel ? (… "text"!==labelType && hide("labelText"))
//	                        : hide(["customLabel","labelText","labelType"])
//	Datagrid      pagination ? hide("showNumberOfRows")
//	                         : (…, !1===showNumberOfRows && hide("pagingPosition"))
//
// labelText is hidden in the else branch too, so `labelType != "text"` can only
// fail to fire where the widget hides it anyway — it over-lists, never hides a
// binding the author needs. pagingPosition is hidden in neither complementary
// case, so the same reading claims hidden with pagination on, which is its
// default. The first is kept, the second dropped.
//
// The scan is textual and bounded to the sibling branch. It answers "no" when
// the shape is not recognised, which drops the rule — the safe direction.
func hiddenInComplementaryBranch(js string, callStart int, propertyKey string) bool {
	open := enclosingGroupOpen(js, callStart)
	if open < 0 {
		return false
	}
	// The group is one branch of `cond ? A : B`. Find the sibling branch: for a
	// then-group `? ( … )` it follows the matching `)`; for an else-group
	// `: ( … )` it is the text between the `?` and this `:`.
	pre := strings.TrimRight(js[:open], " ")
	var sibling string
	switch {
	case strings.HasSuffix(pre, "?"):
		close := matchingCloseParen(js, open)
		if close < 0 {
			return false
		}
		rest := js[close+1:]
		i := strings.IndexByte(rest, ':')
		if i < 0 {
			return false
		}
		sibling = rest[i+1:]
		if len(sibling) > complementScanLimit {
			sibling = sibling[:complementScanLimit]
		}
	case strings.HasSuffix(pre, ":"):
		q := strings.LastIndexByte(pre[:len(pre)-1], '?')
		if q < 0 {
			return false
		}
		sibling = pre[q+1 : len(pre)-1]
	default:
		return false
	}
	return mentionsHiddenProperty(sibling, propertyKey)
}

// complementScanLimit bounds the then-branch scan, whose end is not delimited by
// a paren the way the else-branch's is. Every observed ternary branch is far
// shorter; a longer one simply answers "no" and drops the rule.
const complementScanLimit = 4000

// mentionsHiddenProperty reports whether s contains a hide call naming
// propertyKey as a quoted argument.
func mentionsHiddenProperty(s, propertyKey string) bool {
	lit := `"` + propertyKey + `"`
	for _, loc := range hideCallRE.FindAllStringIndex(s, -1) {
		args, ok := balancedArgs(s, loc[1])
		if !ok {
			continue
		}
		if strings.Contains(args, lit) {
			return true
		}
	}
	return false
}

// enclosingGroupOpen returns the index of the unmatched '(' that opens the group
// containing callStart, or -1 when the hide is not inside one.
func enclosingGroupOpen(js string, callStart int) int {
	depth := 0
	inStr := byte(0)
	for i := callStart - 1; i >= 0; i-- {
		c := js[i]
		if inStr != 0 {
			if c == inStr && (i == 0 || js[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
		case ')':
			depth++
		case '(':
			if depth == 0 {
				head := strings.TrimRight(js[:i], " ")
				if strings.HasSuffix(head, "?") || strings.HasSuffix(head, ":") ||
					strings.HasSuffix(head, "&&") || strings.HasSuffix(head, "||") {
					return i
				}
				return -1
			}
			depth--
		case '{', ';':
			if depth == 0 {
				return -1
			}
		}
	}
	return -1
}

// matchingCloseParen returns the index of the ')' matching the '(' at open.
func matchingCloseParen(js string, open int) int {
	depth := 0
	inStr := byte(0)
	for i := open; i < len(js); i++ {
		c := js[i]
		if inStr != 0 {
			if c == inStr && js[i-1] != '\\' {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// insideOpenGroup reports whether prefix leaves a grouping paren open — i.e. the
// guard that follows it is one operand inside `cond ? ( … )` rather than a
// statement of its own.
//
// The scan is backwards and stops at the enclosing statement (`{` or `;` at
// depth zero), so the parens of an enclosing `function(a,b){…}` or a
// `.forEach((function(o,r){…}))` are not miscounted as an open group — without
// that bound every nested (object-list) rule would read as conjunctive.
func insideOpenGroup(prefix string) bool {
	depth := 0
	inStr := byte(0)
	for i := len(prefix) - 1; i >= 0; i-- {
		c := prefix[i]
		if inStr != 0 {
			if c == inStr && (i == 0 || prefix[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
		case ')':
			depth++
		case '(':
			if depth == 0 {
				// An unmatched open paren — but only a paren that FOLLOWS a
				// conditional connector groups a guarded branch. `switch(a,b,c)`,
				// `if(...)` and an ordinary call also leave one open, and their
				// contents are not conditioned on anything, so treating them as
				// groups would drop sound rules: Timeline writes its hides inside
				// `switch(A, B, e.groupByKey)`.
				head := strings.TrimRight(prefix[:i], " ")
				return strings.HasSuffix(head, "?") || strings.HasSuffix(head, ":") ||
					strings.HasSuffix(head, "&&") || strings.HasSuffix(head, "||")
			}
			depth--
		case '{', ';':
			if depth == 0 {
				return false // reached the enclosing statement cleanly
			}
		}
	}
	return false
}

// stripGroupSiblings walks back over `hide…(…),` siblings so a hide that is not
// the first in a comma group is attributed to the group's guard.
//
// Only a preceding *hide call* is skipped, never an arbitrary expression: a
// comma in editorConfig also separates object literals and array elements, and
// skipping one of those would attach a guard belonging to something else. The
// walk stops at anything it does not recognise, which leaves pre where it was
// and the hide unattributed — today's behaviour, and the safe direction.
func stripGroupSiblings(pre string) string {
	walked := stripGroupSiblingsRaw(pre)
	// Commit only when the walk lands on the group's own `(`. Landing anywhere
	// else means the "sibling" was not a comma-group member at all: in
	// `A ? B : hide(x), hide(y)` the text before hide(y) is a ternary ELSE
	// branch, and attributing y to `A` is simply wrong — Maps hides `advanced`
	// there, and the mis-read made it "hidden when geodecodeApiKey is not set".
	if strings.HasSuffix(walked, "(") {
		return walked
	}
	return pre
}

func stripGroupSiblingsRaw(pre string) string {
	for {
		trimmed := strings.TrimRight(pre, " ")
		if !strings.HasSuffix(trimmed, ",") {
			return pre
		}
		body := strings.TrimRight(trimmed[:len(trimmed)-1], " ")
		if !strings.HasSuffix(body, ")") {
			return pre
		}
		open := matchingOpenParen(body)
		if open < 0 {
			return pre
		}
		head := strings.TrimRight(body[:open], " ")
		at := hideCallTail(head)
		if at < 0 {
			return pre // not a hide call — do not cross this comma
		}
		pre = strings.TrimRight(head[:at], " ")
		if l := nsPrefix(pre); l >= 0 {
			pre = strings.TrimRight(pre[:l], " ")
		}
	}
}

// matchingOpenParen returns the index of the '(' matching the ')' that ends s,
// or -1 when it is unbalanced. Quoted strings are skipped so a paren inside a
// caption literal does not throw the count off.
func matchingOpenParen(s string) int {
	depth := 0
	inStr := byte(0)
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if inStr != 0 {
			if c == inStr && (i == 0 || s[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// emptyCond builds an empty/notempty condition for `null===ref` and
// `0===ref.length`. cmp is the JS operator as written; falsy is the connector
// polarity, and the two compose — `!==` under `||` cancels back to "empty".
func emptyCond(ref, cmp string, falsy bool, aliases map[string]string, itemIdent string) (types.WidgetVisibilityCondition, bool) {
	key, scope, ok := resolveRef(ref, aliases, itemIdent)
	if !ok {
		return types.WidgetVisibilityCondition{}, false
	}
	wantEmpty := (cmp == "===") != falsy // XOR
	op := "notempty"
	if wantEmpty {
		op = "empty"
	}
	return types.WidgetVisibilityCondition{PropertyKey: key, Operator: op, Scope: scope}, true
}

// boolCond builds an eq/ne condition against a minified boolean literal, where
// digit is terser's "1" for false (`!1`) and "0" for true (`!0`).
func boolCond(ref, cmp, digit string, falsy bool, aliases map[string]string, itemIdent string) (types.WidgetVisibilityCondition, bool) {
	key, scope, ok := resolveRef(ref, aliases, itemIdent)
	if !ok {
		return types.WidgetVisibilityCondition{}, false
	}
	lit := "false"
	if digit == "0" {
		lit = "true"
	}
	wantEq := (cmp == "===") != falsy // XOR
	op := "ne"
	if wantEq {
		op = "eq"
	}
	return types.WidgetVisibilityCondition{PropertyKey: key, Operator: op, Value: lit, Scope: scope}, true
}

// resolveRef turns a guard reference into a property key and the scope that key
// belongs to: `obj.prop` yields `prop`; a bare identifier is looked up in the
// scope alias map. A bare identifier with no alias (e.g. a computed local) is
// unresolvable.
//
// When itemIdent is non-empty (the hide is inside that object list's forEach
// callback) a reference based on it is scoped to the list ITEM; everything else
// is a property of the widget.
func resolveRef(ref string, aliases map[string]string, itemIdent string) (key, scope string, ok bool) {
	if i := strings.LastIndexByte(ref, '.'); i >= 0 {
		if itemIdent != "" && ref[:i] == itemIdent {
			return ref[i+1:], types.ConditionScopeItem, true
		}
		return ref[i+1:], "", true
	}
	if key, ok := aliases[ref]; ok {
		return key, "", true
	}
	return "", "", false
}

// enclosingAliases returns the `ident → property` aliases declared in the
// function body that encloses the hide call at pos, resolved by scanning back
// to the nearest unbalanced `{`. Scoping matters: minified editorConfig reuses
// identifiers like `r`/`n` across functions, so only same-scope `var r=e.prop`
// assignments are trustworthy.
func enclosingAliases(js string, pos int) map[string]string {
	// Walk back to the enclosing block's opening brace.
	depth := 0
	open := 0
	for i := pos - 1; i >= 0; i-- {
		switch js[i] {
		case '}', ')', ']':
			depth++
		case '{', '(', '[':
			if depth == 0 {
				open = i
				goto found
			}
			depth--
		}
	}
found:
	body := js[open:pos]
	aliases := map[string]string{}
	ambiguous := map[string]bool{}
	for _, m := range aliasAssignRE.FindAllStringSubmatch(body, -1) {
		ident, prop := m[1], m[3]
		if ambiguous[ident] {
			continue
		}
		if existing, ok := aliases[ident]; ok && existing != prop {
			delete(aliases, ident)
			ambiguous[ident] = true
			continue
		}
		aliases[ident] = prop
	}
	return aliases
}

// balancedArgs returns the argument-list text between the '(' just before
// `open` and its matching ')'. Handles nested (), [], {} and string literals.
func balancedArgs(js string, open int) (string, bool) {
	depth := 1
	inStr := byte(0)
	for i := open; i < len(js); i++ {
		c := js[i]
		if inStr != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return js[open:i], true
			}
		}
	}
	return "", false
}

// splitTopLevelCommas splits an argument list on commas that are not nested
// inside (), [], {}, or string literals.
func splitTopLevelCommas(args string) []string {
	var parts []string
	depth := 0
	inStr := byte(0)
	start := 0
	for i := 0; i < len(args); i++ {
		c := args[i]
		if inStr != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, args[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, args[start:])
	return parts
}
