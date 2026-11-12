// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// An association named in an XPath constraint must be written QUALIFIED
// (`Module.Association`); an attribute is written bare. Get it wrong and mxcli
// stores the constraint faithfully, `mxcli check --references` passes, `exec`
// reports success, and mxbuild fails the build with
//
//	ERROR at <Module>, Microflow '<MF>', Retrieve object(s) activity
//	'Retrieve list of <E> from database': Error(s) in XPath constraint.
//
// Measured on a blank Mendix 11.14.0 app, with the qualification as the only
// variable between two runs of the same script:
//
//	[Ticket_Reporter = $currentUser]                 check passed  →  build exit 3
//	[MyFirstModule.Ticket_Reporter = $currentUser]   check passed  →  BUILD SUCCEEDED
//
// This is the expensive shape rather than a cosmetic one, because `exec` applies
// statements one at a time and cannot roll back: a script carrying this in the
// middle writes everything before it and stops at the build, leaving the model
// half-updated with nothing having reported a problem.
//
// The rule is deliberately narrow. It fires only on a bare name that is NOT an
// attribute of the constrained entity AND IS a known association, so it can say
// which spelling to use instead of merely suspecting one. A name that is neither
// is left alone — that is a different error (an unknown member), and guessing at
// it here would produce false positives on every XPath function and keyword.
// That same narrowness is why no keyword list is needed: `and`, `or`, `not` and
// `contains` are not association names, so they cannot match.

// xpathIdentRe matches an identifier and the character before it, so the caller
// can reject a qualified tail (`.Name`) or a variable (`$var`). RE2 has no
// lookahead, so the character AFTER the identifier — which distinguishes a
// module prefix from a member — is inspected by index instead.
var xpathIdentRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// xpathAssocHit is one bare association name found in a constraint.
type xpathAssocHit struct {
	Name      string // as written, e.g. "Ticket_Reporter"
	Qualified string // what it must be, e.g. "MyFirstModule.Ticket_Reporter"
}

// blankXPathLiterals replaces the contents of single-quoted string literals with
// spaces, preserving length so match indices still line up with the original.
//
// Without this an association name mentioned inside a literal is flagged, and
// the fix offered is one that would corrupt the literal. Mendix escapes a quote
// inside a literal by doubling it, so a closing quote immediately followed by
// another is an escape and not the end of the string.
func blankXPathLiterals(s string) string {
	out := []byte(s)
	inLiteral := false
	for i := 0; i < len(out); i++ {
		if out[i] == '\'' {
			if inLiteral && i+1 < len(out) && out[i+1] == '\'' {
				out[i+1] = ' ' // the escaped quote's second half
				i++
				continue
			}
			inLiteral = !inLiteral
			continue
		}
		if inLiteral {
			out[i] = ' '
		}
	}
	return string(out)
}

// unqualifiedAssociationsInConstraint returns the bare association names used in
// an XPath constraint. attrs holds the constrained entity's attribute names;
// assocs maps an unqualified association name to its qualified spelling.
//
// Attributes are checked first, so an association whose name collides with an
// attribute of THIS entity is read as the attribute — which is what Mendix does.
func unqualifiedAssociationsInConstraint(constraint string, attrs map[string]bool, assocs map[string]string) []xpathAssocHit {
	if constraint == "" || len(assocs) == 0 {
		return nil
	}
	scanned := blankXPathLiterals(constraint)

	var hits []xpathAssocHit
	seen := map[string]bool{}
	for _, loc := range xpathIdentRe.FindAllStringIndex(scanned, -1) {
		start, end := loc[0], loc[1]
		// A qualified tail (`Module.Name`) or a variable (`$var`) is already
		// unambiguous; neither is a bare member reference.
		if start > 0 && (scanned[start-1] == '.' || scanned[start-1] == '$') {
			continue
		}
		// Followed by a dot, this is the MODULE half of a qualified name, not a
		// member. It is the qualified spelling this rule asks for.
		if end < len(scanned) && scanned[end] == '.' {
			continue
		}
		name := scanned[start:end]
		if attrs[name] || seen[name] {
			continue
		}
		qualified, ok := assocs[name]
		if !ok {
			continue
		}
		seen[name] = true
		hits = append(hits, xpathAssocHit{Name: name, Qualified: qualified})
	}
	return hits
}

// buildAssociationIndex maps every association's unqualified name to its
// qualified spelling. An association is stored in the FROM entity's module
// (see MDL070), so the domain model holding it names it.
//
// A name defined in more than one module is left OUT rather than guessed at:
// the rule's value is naming the right spelling, and offering one of two would
// be wrong half the time. Such a constraint still fails the build, but it fails
// with mxbuild's message rather than with our wrong advice.
func buildAssociationIndex(ctx *ExecContext) map[string]string {
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return nil
	}
	moduleNames := make(map[model.ID]string, len(modules))
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}
	dms, err := projectDomainModels(ctx)
	if err != nil {
		return nil
	}
	index := map[string]string{}
	ambiguous := map[string]bool{}
	add := func(modName, name string) {
		if modName == "" || name == "" {
			return
		}
		qualified := modName + "." + name
		if prev, ok := index[name]; ok && prev != qualified {
			ambiguous[name] = true
			return
		}
		index[name] = qualified
	}
	for _, dm := range dms {
		modName := moduleNames[dm.ContainerID]
		for _, a := range dm.Associations {
			add(modName, a.Name)
		}
		for _, a := range dm.CrossAssociations {
			add(modName, a.Name)
		}
	}
	for name := range ambiguous {
		delete(index, name)
	}
	return index
}

// validateXPathAssociations flags a bare association name in a retrieve's XPath
// constraint — accepted by mxcli, rejected by the build.
//
// It reads the script's own declarations as well as the project's, because the
// common shape is ONE script that creates the entity, the association and the
// microflow that constrains on it. Seeing only stored associations would leave
// the rule silent on exactly the scripts that reach a build half-written, and
// silent with no project at all — which is how `mxcli check script.mdl` is most
// often run.
func validateXPathAssociations(ctx *ExecContext, retrieves []retrieveConstraintRef, sc *scriptContext) []string {
	assocs := buildAssociationIndex(ctx)
	if assocs == nil {
		assocs = map[string]string{}
	}
	entities := buildEntityIndex(ctx)

	// Script declarations win over stored ones: within this script, that is the
	// definition about to be in force.
	if sc != nil {
		for name, qualified := range sc.associations {
			if !sc.ambiguousAssc[name] {
				assocs[name] = qualified
			}
		}
		for name := range sc.ambiguousAssc {
			delete(assocs, name)
		}
	}
	if len(assocs) == 0 {
		return nil
	}

	var errors []string
	for _, r := range retrieves {
		attrs, ok := entityAttrNames(entities, sc, r.entity)
		if !ok {
			continue // entity-not-found is reported separately
		}
		for _, hit := range unqualifiedAssociationsInConstraint(r.constraint, attrs, assocs) {
			errors = append(errors, fmt.Sprintf(
				"constraint on %s names the association %s unqualified — Mendix requires the qualified form and the build fails with "+
					"\"Error(s) in XPath constraint\" (CE0161). Write %s instead.",
				r.entity, hit.Name, hit.Qualified))
		}
	}
	return errors
}

// XPathAssociationViolations is the linter-facing form of the same rule, for
// callers that report violations rather than error strings.
func XPathAssociationViolations(entity, constraint string, attrs map[string]bool, assocs map[string]string) []linter.Violation {
	var out []linter.Violation
	for _, hit := range unqualifiedAssociationsInConstraint(constraint, attrs, assocs) {
		out = append(out, linter.Violation{
			RuleID:   xpathAssociationRule,
			Severity: linter.SeverityError,
			Location: linter.Location{
				Module:       qualifiedNameModule(entity),
				DocumentType: "entity",
				DocumentName: strings.TrimPrefix(entity, qualifiedNameModule(entity)+"."),
			},
			Message: fmt.Sprintf(
				"XPath constraint on %s names the association %s unqualified; Mendix rejects this with CE0161",
				entity, hit.Name),
			Suggestion: fmt.Sprintf("Write %s instead of %s.", hit.Qualified, hit.Name),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Message < out[j].Message })
	return out
}

const xpathAssociationRule = "MDL-XPATH01"

// entityAttrNames returns the constrained entity's attribute names, from the
// script when it declares the entity and from the project otherwise. ok is
// false when neither knows it — then the entity reference itself is the error,
// and this rule stays out of the way.
//
// The script is consulted FIRST because a CREATE OR MODIFY in the script is the
// shape that will be in force, attributes included.
func entityAttrNames(entities map[string]*domainmodel.Entity, sc *scriptContext, qualified string) (map[string]bool, bool) {
	if sc != nil {
		if attrs, ok := sc.entityAttrs[qualified]; ok {
			return attrs, true
		}
	}
	if entities == nil {
		return nil, false
	}
	ent := entities[qualified]
	if ent == nil {
		return nil, false
	}
	attrs := make(map[string]bool, len(ent.Attributes))
	for _, a := range ent.Attributes {
		attrs[a.Name] = true
	}
	return attrs, true
}
