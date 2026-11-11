// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"sort"
	"strings"
)

// SyntaxFeature describes a discoverable MDL syntax feature.
type SyntaxFeature struct {
	Path     string   `json:"path"`
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords"`
	Syntax   string   `json:"syntax"`
	Example  string   `json:"example"`

	// Features names the version-registry features (sdk/versions/*.yaml, as
	// "area/name") the topic's statements need. It is the ONLY place a topic
	// says which Mendix version it needs: MinVersion and Requires are filled
	// from the registry, never written by hand, so `mxcli syntax`, `show
	// features` and the exec-time gate cannot disagree (ako/mxcli#1367 — they
	// had drifted apart in both directions).
	Features []string `json:"-"`

	// MinVersion is the version of the FIRST of Features, the topic's primary
	// statement (data-transformer's document is 11.9 although the transform
	// that uses it is 11.4). Computed; Register refuses one set by hand.
	MinVersion string `json:"min_version,omitempty"`
	// Requires lists each feature with its own version, since one topic can
	// span several (notify workflow is 10.0, its event sub-process target 11.7).
	Requires []Requirement `json:"requires,omitempty"`

	SeeAlso []string `json:"see_also,omitempty"`
}

// Requirement is one registry feature a topic needs.
type Requirement struct {
	Feature string `json:"feature"` // "area/name"
	Since   string `json:"since"`
	// Until is the last version that has the feature, for one Mendix removed
	// (a message definition collection: 10.0.0 to 11.14, ako/mxcli#2140).
	Until string `json:"until,omitempty"`
	MDL   string `json:"mdl,omitempty"`
}

var registry []SyntaxFeature
var registeredPaths = map[string]bool{}

// topicAliases maps legacy topic names and common variants to registry paths.
var topicAliases = map[string]string{
	// ALTER MENU lives with the menu document it edits.
	"menu.alter": "navigation.menu-document.alter",
	// Connection aliases
	"connection":  "connect",
	"open":        "connect",
	"close":       "disconnect",
	"show-status": "status",
	"show_status": "status",
	// Domain model aliases
	"keywords":        "domain-model.keywords",
	"reserved":        "domain-model.keywords",
	"types":           "domain-model.types",
	"datatypes":       "domain-model.types",
	"data-types":      "domain-model.types",
	"delete":          "domain-model.association.delete-behavior",
	"delete_behavior": "domain-model.association.delete-behavior",
	"delete-behavior": "domain-model.association.delete-behavior",
	"entity":          "domain-model.entity",
	"entities":        "domain-model.entity",
	"enumeration":     "domain-model.enumeration",
	"enum":            "domain-model.enumeration",
	"enumerations":    "domain-model.enumeration",
	"constant":        "domain-model.constant",
	"constants":       "domain-model.constant",
	"association":     "domain-model.association",
	"associations":    "domain-model.association",
	// "view" alone is deliberately absent: GRANT VIEW ON PAGE owns that word in
	// MDL, so the alias has to carry "entity" to be unambiguous.
	"view-entity":   "domain-model.view-entity",
	"view_entity":   "domain-model.view-entity",
	"viewentity":    "domain-model.view-entity",
	"view-entities": "domain-model.view-entity",
	"viewentities":  "domain-model.view-entity",
	// Plural aliases
	"microflows": "microflow",
	"pages":      "page",
	"snippets":   "snippet",
	"fragments":  "fragment",
	"workflows":  "workflow",
	// Folder aliases — "folder" is the word a user reaches for whether they
	// mean placing a document (document-folder) or listing the layout (folders),
	// and the placement question is the far more common one.
	"folder":           "document-folder",
	"document-folders": "document-folder",
	"list-folders":     "folders",
	// Variant aliases
	"nav":                "navigation",
	"project-settings":   "settings",
	"rest-client":        "rest",
	"rest-clients":       "rest",
	"integrations":       "integration",
	"services":           "integration",
	"contract":           "integration",
	"contracts":          "integration",
	"javaaction":         "java-action",
	"java_action":        "java-action",
	"java-actions":       "java-action",
	"javaactions":        "java-action",
	"javascriptaction":   "javascript-action",
	"javascript_action":  "javascript-action",
	"javascript-actions": "javascript-action",
	"javascriptactions":  "javascript-action",
	"businessevents":     "business-events",
	"business_events":    "business-events",
	"be":                 "business-events",
	"xpath-constraints":  "xpath",
	"external-sql":       "sql",
	"validation":         "errors",
	"testing":            "test",
	"tests":              "test",
	// Agents aliases
	"agent":          "agents",
	"agent-editor":   "agents",
	"agenteditor":    "agents",
	"model":          "agents.model",
	"models":         "agents.model",
	"knowledge-base": "agents.knowledge-base",
	"knowledgebase":  "agents.knowledge-base",
	"mcp":            "agents.mcp-service",
	"mcp-service":    "agents.mcp-service",
}

// Register adds a syntax feature to the global registry.
// Panics if a feature with the same path is already registered.
func Register(f SyntaxFeature) {
	if registeredPaths[f.Path] {
		panic("syntax: duplicate feature path: " + f.Path)
	}
	if f.MinVersion != "" || len(f.Requires) > 0 {
		panic("syntax: " + f.Path + " sets a version by hand; name its sdk/versions feature in Features instead (#1367)")
	}
	registeredPaths[f.Path] = true
	registry = append(registry, f)
}

// ResolveAlias returns the canonical registry path for a topic alias.
// If the input is not an alias, it is returned unchanged.
func ResolveAlias(path string) string {
	if alias, ok := topicAliases[path]; ok {
		return alias
	}
	return path
}

// All returns every registered feature, sorted by path.
func All() []SyntaxFeature {
	resolveVersions()
	out := make([]SyntaxFeature, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ByPrefix returns features whose path equals or starts with prefix+".".
func ByPrefix(prefix string) []SyntaxFeature {
	resolveVersions()
	var out []SyntaxFeature
	for _, f := range registry {
		if f.Path == prefix || strings.HasPrefix(f.Path, prefix+".") {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ByPath returns the feature with the exact path, or nil.
func ByPath(path string) *SyntaxFeature {
	resolveVersions()
	for i := range registry {
		if registry[i].Path == path {
			return &registry[i]
		}
	}
	return nil
}

// HasPrefix reports whether any registered feature matches the prefix.
func HasPrefix(prefix string) bool {
	for _, f := range registry {
		if f.Path == prefix || strings.HasPrefix(f.Path, prefix+".") {
			return true
		}
	}
	return false
}

// BySegmentMatch returns features that have a path SEGMENT containing q,
// case-insensitively, sorted by path.
//
// Registry lookup is otherwise anchored at the left — `HasPrefix` matches whole
// segments from the start of a path — so a topic is reachable only by its first
// segment or via a hand-written alias. That made `mxcli syntax rule` an
// "Unknown topic" while `microflow.rule` and `validation-rule` both existed
// (#955), and it was not one gap: of the 73 distinct leaf names in the registry,
// 60 could not be reached as a bare topic. An alias each is 60 entries to keep
// in step with the registry; matching on any segment needs none, and answers
// with everything relevant rather than the one spelling an alias happened to
// pick — `rule` returns both the microflow rule and the validation rule.
//
// Substring rather than equality because a segment is often a compound of the
// word the reader has in mind: `task` should find `workflow.user-task`.
func BySegmentMatch(q string) []SyntaxFeature {
	resolveVersions()
	if q == "" {
		return nil
	}
	q = strings.ToLower(q)
	var out []SyntaxFeature
	for _, f := range registry {
		for _, seg := range strings.Split(strings.ToLower(f.Path), ".") {
			if strings.Contains(seg, q) {
				out = append(out, f)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ByKeyword returns features listing q as a whole word of one of their
// keywords, case-insensitively. A feature listing q as a keyword on its own
// ranks ahead of one using it inside a phrase ("grant" before "grant microflow");
// ties sort by path.
//
// Whole words, not substrings: keywords are prose, and a substring match on
// "ant" would answer with every topic that mentions "grant".
func ByKeyword(q string) []SyntaxFeature {
	resolveVersions()
	q = strings.ToLower(q)
	if q == "" {
		return nil
	}
	type ranked struct {
		f     SyntaxFeature
		exact bool
	}
	var hits []ranked
	for _, f := range registry {
		matched, exact := false, false
		for _, kw := range f.Keywords {
			kw = strings.ToLower(kw)
			if kw == q {
				matched, exact = true, true
				break
			}
			for _, w := range strings.FieldsFunc(kw, func(r rune) bool {
				return r == ' ' || r == '-' || r == '/' || r == ','
			}) {
				if w == q {
					matched = true
				}
			}
		}
		if matched {
			hits = append(hits, ranked{f, exact})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].exact != hits[j].exact {
			return hits[i].exact
		}
		return hits[i].f.Path < hits[j].f.Path
	})
	out := make([]SyntaxFeature, len(hits))
	for i, h := range hits {
		out[i] = h.f
	}
	return out
}
