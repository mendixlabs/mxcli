// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// A hide call whose list is computed by a switch in an immediately-invoked
// function — the native Image's (Native Mobile Resources):
//
//	return x.hidePropertiesIn(t, v, function(s){switch(s){
//	    case "image":    return L.filter(function(p){return "imageObject" !== p});
//	    case "imageUrl": return L.filter(function(p){return "imageUrl"    !== p});
//	    default:         return L }}(v.datasource))
//
// Each case hides the whole list L but one member, and the default hides all
// of it. So a member is hidden unless the switched property holds one of the
// cases that keeps it: `notin` over those cases.
//
// Only that exact shape is read, and only at a function's own statement level
// with nothing guarding it. Anything else — a default other than L, a case
// keeping more than one member, a guard before the call — is left unread,
// which degrades to "not hidden" as every unread hide does.
var (
	iifeSwitchRE = regexp.MustCompile(`^function\(([\w$]+)\)\{switch\(([\w$]+)\)\{(.*)\}\}\(([\w$]+\.[\w$]+)\)$`)
	iifeCaseRE   = regexp.MustCompile(`^case"([^"]*)":return ([\w$]+)\.filter\(function\(([\w$]+)\)\{return(?:"([^"]*)"!==([\w$]+)|([\w$]+)!=="([^"]*)")\}\)$`)
	iifeDefRE    = regexp.MustCompile(`^default:return ([\w$]+)$`)
	unguardedRE  = regexp.MustCompile(`return\s*(?:[\w$]+\.)?$`)
)

func iifeSwitchHideRules(js string, callStart int, args string, consts map[string][]string) ([]types.WidgetVisibilityRule, bool) {
	parts := splitTopLevelCommas(args)
	if len(parts) != 3 {
		return nil, false
	}
	m := iifeSwitchRE.FindStringSubmatch(strings.TrimSpace(parts[2]))
	if m == nil || m[1] != m[2] {
		return nil, false
	}
	// Unguarded: `return [receiver.]hide…(` opening a function's statement.
	loc := unguardedRE.FindStringIndex(js[:callStart])
	if loc == nil || !atFunctionStatementLevel(js, loc[0]) {
		return nil, false
	}
	key, scope, ok := resolveRef(m[4], enclosingAliases(js, callStart), "")
	if !ok || scope != "" {
		return nil, false
	}

	list, keeps := "", map[string][]string{} // member -> case values keeping it
	sawDefault := false
	for _, arm := range strings.Split(m[3], ";") {
		if d := iifeDefRE.FindStringSubmatch(arm); d != nil {
			if sawDefault || (list != "" && d[1] != list) {
				return nil, false
			}
			list, sawDefault = d[1], true
			continue
		}
		c := iifeCaseRE.FindStringSubmatch(arm)
		if c == nil {
			return nil, false
		}
		param, kept, ref := c[3], c[4], c[5]
		if kept == "" {
			kept, ref = c[7], c[6]
		}
		if ref != param || (list != "" && c[2] != list) {
			return nil, false
		}
		list = c[2]
		keeps[kept] = append(keeps[kept], c[1])
	}
	members, known := consts[list]
	if !sawDefault || !known {
		return nil, false
	}
	inList := map[string]bool{}
	for _, k := range members {
		inList[k] = true
	}
	for k := range keeps {
		if !inList[k] {
			return nil, false // a case keeps something the list does not hold
		}
	}
	var rules []types.WidgetVisibilityRule
	for _, k := range members {
		rules = append(rules, types.WidgetVisibilityRule{
			PropertyKey: k,
			HiddenWhen:  &types.WidgetVisibilityCondition{PropertyKey: key, Operator: "notin", Value: strings.Join(keeps[k], ",")},
		})
	}
	return rules, true
}
