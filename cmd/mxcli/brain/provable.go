// SPDX-License-Identifier: Apache-2.0

// provable.go - which anchors a test can ever be linked to.
//
// A requirement is built when its anchors resolve. Whether a test can PROVE it
// is a second question: a test is linked to a requirement through the
// microflows that implement it (its footprint, computed by `report
// traceability`), and some anchor kinds have no microflow behind them at all.
// A requirement anchored only on module roles, an enumeration or a constant is
// built and permanently unproven — the moment to say so is when the author
// still knows where the thing is really implemented (ako/mxcli#2303).
//
// The classification lives here so `brain check`, `brain capture` and the
// traceability report read one list and cannot disagree about it.
package brain

import (
	"sort"
	"strings"
)

// provableKinds are the anchor kinds (as a Resolution reports them: the
// catalog's object type, lower-cased) that a microflow can implement, and so
// a test can reach: the microflow itself, what a page, snippet, workflow,
// scheduled event or service runs, the microflows that write an entity or use
// an attribute or association, and the callers of a Java or JavaScript action.
//
// It is exactly the set report traceability computes a footprint for
// (TestProvableKindsHaveAFootprint holds them together). A nanoflow is in
// neither list: microflow tests and runtime spans see microflows only.
var provableKinds = map[string]bool{
	"microflow": true, "page": true, "snippet": true, "workflow": true,
	"entity": true, "attribute": true, "association": true, "scheduled_event": true,
	"published_rest_service": true, "published_rest_operation": true, "published_odata_service": true,
	"business_event_service": true, "rest_operation": true,
	"import_mapping": true, "export_mapping": true,
	"java_action": true, "javascript_action": true,
}

// unprovableKinds are kinds known to have no microflow footprint. A kind in
// neither list (a document type the catalog does not index, reported by its
// storage name) is not judged: a warning on a guess would be noise.
var unprovableKinds = map[string]bool{
	"module": true, "module_role": true, "user_role": true, "enumeration": true, "enumeration_value": true,
	"constant": true, "layout": true, "page_template": true, "building_block": true, "image_collection": true,
	"icon_collection": true, "json_structure": true, "regular_expression": true, "menu": true,
	"navigation": true, "task_queue": true, "message_definition": true,
}

func normalizeKind(kind string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(kind)), " ", "_")
}

// ProvableKind reports whether a test can be linked to an anchor of this kind
// through the microflows that implement it.
func ProvableKind(kind string) bool { return provableKinds[normalizeKind(kind)] }

// UnprovableKind reports whether an anchor of this kind is known to have no
// microflow behind it. Unknown kinds are neither provable nor unprovable.
func UnprovableKind(kind string) bool { return unprovableKinds[normalizeKind(kind)] }

// UnprovableFinding is a built requirement no test can ever be linked to,
// because every anchor is of an unprovable kind.
type UnprovableFinding struct {
	Shard   string   `json:"shard"`
	EntryID string   `json:"entry_id"`
	Title   string   `json:"title"`
	Kinds   []string `json:"kinds"`
}

// Unprovable reports whether a requirement's anchors are all of kinds no
// microflow implements. A requirement with no anchors, or one with an anchor
// that did not resolve (not built, kind unknown), is not judged.
func Unprovable(anchors []AnchorResult) ([]string, bool) {
	if len(anchors) == 0 {
		return nil, false
	}
	var kinds []string
	for _, a := range anchors {
		if a.State != Resolved || !UnprovableKind(a.Kind) {
			return nil, false
		}
		if !containsString(kinds, a.Kind) {
			kinds = append(kinds, a.Kind)
		}
	}
	return kinds, true
}

// UnprovableHint is the sentence `check` and `capture` print.
func UnprovableHint(id string, kinds []string) string {
	return "requirement " + id + " is anchored only on " + strings.Join(kinds, ", ") +
		": no microflow implements that, so no test can ever be linked to it. " +
		"Add the microflow, page or entity that implements it as an anchor."
}

func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ProvableKinds lists the provable kinds, sorted, for tests that hold the
// report's footprint to the same list.
func ProvableKinds() []string {
	out := make([]string, 0, len(provableKinds))
	for k := range provableKinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
