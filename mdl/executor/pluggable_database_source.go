// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// customWidgetDatabaseSourceSupported reports whether the project has Studio
// Pro 9's "Database" source on pluggable widgets
// (CustomWidgets$CustomWidgetDatabaseSource). Mendix 10 merged it into the XPath
// source: mx convert 10.12.19 rewrites every one of the FSM template's ten into
// a CustomWidgetXPathSource, so a 10+ project never holds one. A pluggable
// widget's datasource property itself dates from 8.3.0, so no lower bound
// applies. Without a version (no project) it is not assumed.
func (pb *pageBuilder) customWidgetDatabaseSourceSupported() bool {
	if pb == nil || pb.backend == nil {
		return false
	}
	pv := pb.backend.ProjectVersion()
	return pv != nil && pv.MajorVersion > 0 && !pv.IsAtLeast(10, 0)
}

// buildPluggableDataSourceV3 builds the datasource of a widget the pluggable
// engine writes. `database from $ctx/Assoc/Entity` is the one form it adds to
// buildDataSourceV3: the "Database" source Studio Pro writes for a repeater,
// gallery or grid bound over an association. On 10+ that is a
// CustomWidgetXPathSource holding the hops as an IndirectEntityRef beside its
// XPath and sort bar (mendixlabs/mxcli#1599); on a 9.x project it is the
// CustomWidgetDatabaseSource, its WHERE stored as constraint rows
// (mendixlabs/mxcli#1399). A widget with its own builder keeps
// buildDataSourceV3's refusal.
//
// widgetName is the widget the source belongs to. A direct-entity source
// (`database from E where …`) is written as an XPath source on every version —
// the two are equivalent — EXCEPT where the stored widget of that name already
// holds a 9.x database source: then the rewrite keeps it, when its WHERE fits
// constraint rows (ako/mxcli#1208, ADR-0008: a rewrite keeps what it was not
// asked to change; describe prints both alike, so the statement cannot say).
func (pb *pageBuilder) buildPluggableDataSourceV3(ds *ast.DataSourceV3, widgetName string) (pages.DataSource, string, error) {
	if ds != nil && ds.Type == "database" && ds.AssociationPath == "" &&
		pb.customWidgetDatabaseSourceSupported() && pb.storedPluggables.storedUsesDatabaseSource(widgetName) {
		built, entity, err := pb.buildDataSourceV3(ds)
		if err != nil {
			return nil, "", err
		}
		if src, ok := built.(*pages.DatabaseSource); ok {
			if constraints, cerr := parseDatabaseConstraints(src.XPathConstraint, entity); cerr == nil {
				src.TypeName = "CustomWidgets$CustomWidgetDatabaseSource"
				src.UseConstraints = true
				src.Constraints = constraints
				src.XPathConstraint = ""
			}
			// A WHERE the rows cannot hold stays an XPath source: changing the
			// type is the lesser drift than dropping a term.
		}
		return built, entity, nil
	}
	if ds == nil || ds.Type != "database" || ds.AssociationPath == "" {
		return pb.buildDataSourceV3(ds)
	}
	built, entity, err := pb.buildDatabaseOverAssociationV3(ds)
	if err != nil {
		return nil, "", err
	}
	src, ok := built.(*pages.DatabaseSource)
	if !ok || !pb.customWidgetDatabaseSourceSupported() {
		return built, entity, nil
	}
	constraints, err := parseDatabaseConstraints(src.XPathConstraint, entity)
	if err != nil {
		return nil, "", mdlerrors.NewValidationf(
			"`database from $%s/%s where …`: a Mendix 9 database source stores simple constraints "+
				"(Attribute op value, joined by `and`), not XPath — %v", ds.ContextVariable, ds.AssociationPath, err)
	}
	src.TypeName = "CustomWidgets$CustomWidgetDatabaseSource"
	src.UseConstraints = true
	src.Constraints = constraints
	src.XPathConstraint = ""
	return src, entity, nil
}

// parseDatabaseConstraints reads the XPath a 9.x database source's constraint
// rows were described as (databaseConstraintsXPath) back into those rows:
// one or more bracket groups, each a conjunction of `Attr op literal`,
// `contains(Attr, literal)`, `not(contains(Attr, literal))`, or a bare boolean
// `Attr` / `not(Attr)` — the form Mendix's own 9 → 10 conversion writes for an
// Equals-true row. Anything a row cannot hold — `or`, a path, a variable, an
// arithmetic expression, another function — is refused rather than dropped,
// because dropping a term widens the retrieve.
//
// A bare attribute name is qualified with entity; a three-part name is kept.
func parseDatabaseConstraints(xpath, entity string) ([]pages.DatabaseConstraint, error) {
	xpath = strings.TrimSpace(visitor.FlattenXPathConstraint(xpath))
	if xpath == "" {
		return nil, nil
	}
	groups := visitor.SplitXPathPredicateGroups(xpath)
	if groups == nil {
		return nil, fmt.Errorf("%q is not a bracketed constraint", xpath)
	}
	var out []pages.DatabaseConstraint
	for _, g := range groups {
		toks, err := tokenizeConstraint(g[1 : len(g)-1])
		if err != nil {
			return nil, err
		}
		for _, term := range splitConstraintTerms(toks) {
			row, err := parseConstraintTerm(term, entity)
			if err != nil {
				return nil, err
			}
			out = append(out, row)
		}
	}
	return out, nil
}

type constraintToken struct {
	kind byte // 'i' identifier, 's' string, 'n' number, 'o' operator, or the punctuation itself
	text string
}

func tokenizeConstraint(s string) ([]constraintToken, error) {
	var toks []constraintToken
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(' || c == ')' || c == ',':
			toks = append(toks, constraintToken{kind: c, text: string(c)})
			i++
		case c == '\'':
			var b strings.Builder
			j := i + 1
			for ; j < len(s); j++ {
				if s[j] == '\'' {
					if j+1 < len(s) && s[j+1] == '\'' {
						b.WriteByte('\'')
						j++
						continue
					}
					break
				}
				b.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unterminated string in %q", s)
			}
			toks = append(toks, constraintToken{kind: 's', text: b.String()})
			i = j + 1
		case c == '=' || c == '<' || c == '>' || c == '!':
			op := string(c)
			if i+1 < len(s) && s[i+1] == '=' {
				op += "="
			}
			if op == "!" {
				return nil, fmt.Errorf("unexpected `!` in %q", s)
			}
			toks = append(toks, constraintToken{kind: 'o', text: op})
			i += len(op)
		case c == '-' || (c >= '0' && c <= '9'):
			j := i + 1
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			if !databaseConstraintNumber.MatchString(s[i:j]) {
				return nil, fmt.Errorf("%q is not a number", s[i:j])
			}
			toks = append(toks, constraintToken{kind: 'n', text: s[i:j]})
			i = j
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i + 1
			for j < len(s) && (s[j] == '_' || s[j] == '.' || s[j] == '-' || s[j] >= 'a' && s[j] <= 'z' ||
				s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			toks = append(toks, constraintToken{kind: 'i', text: s[i:j]})
			i = j
		default:
			return nil, fmt.Errorf("`%c` has no place in a constraint row (%q)", c, s)
		}
	}
	return toks, nil
}

// splitConstraintTerms splits a group's tokens at its top-level `and`s.
func splitConstraintTerms(toks []constraintToken) [][]constraintToken {
	var terms [][]constraintToken
	depth, start := 0, 0
	for i, t := range toks {
		switch {
		case t.kind == '(':
			depth++
		case t.kind == ')':
			depth--
		case depth == 0 && t.kind == 'i' && t.text == "and":
			terms = append(terms, toks[start:i])
			start = i + 1
		}
	}
	return append(terms, toks[start:])
}

var constraintOperators = map[string]string{
	"=": "Equals", "!=": "NotEquals", "<": "LessThan", "<=": "LessThanOrEquals",
	">": "GreaterThan", ">=": "GreaterThanOrEquals",
}

func parseConstraintTerm(t []constraintToken, entity string) (pages.DatabaseConstraint, error) {
	text := func() string {
		parts := make([]string, len(t))
		for i, tok := range t {
			parts[i] = tok.text
		}
		return strings.Join(parts, " ")
	}
	is := func(i int, kind byte, word string) bool {
		return i < len(t) && t[i].kind == kind && (word == "" || t[i].text == word)
	}
	bad := func() (pages.DatabaseConstraint, error) {
		return pages.DatabaseConstraint{}, fmt.Errorf("`%s` is not a constraint row", text())
	}
	row := func(attr, op, value string) (pages.DatabaseConstraint, error) {
		qn, ok := constraintAttribute(attr, entity)
		if !ok {
			return bad()
		}
		return pages.DatabaseConstraint{Attribute: qn, Operator: op, Value: value}, nil
	}
	// literal reads a literal at i and returns its stored text and length.
	literal := func(i int) (string, int, bool) {
		switch {
		case is(i, 's', ""), is(i, 'n', ""):
			return t[i].text, 1, true
		case (is(i, 'i', "true") || is(i, 'i', "false")) && is(i+1, '(', "") && is(i+2, ')', ""):
			return t[i].text, 3, true
		}
		return "", 0, false
	}
	switch {
	case len(t) == 1 && is(0, 'i', ""):
		return row(t[0].text, "Equals", "true")
	case len(t) == 4 && is(0, 'i', "not") && is(1, '(', "") && is(2, 'i', "") && is(3, ')', ""):
		return row(t[2].text, "Equals", "false")
	case is(0, 'i', "contains") || is(0, 'i', "not"):
		op, at := "Contains", 0
		if is(0, 'i', "not") {
			op, at = "NotContains", 2
			if !is(1, '(', "") || len(t) < 2 || !is(len(t)-1, ')', "") {
				return bad()
			}
		}
		if !is(at, 'i', "contains") || !is(at+1, '(', "") || !is(at+2, 'i', "") || !is(at+3, ',', "") {
			return bad()
		}
		value, n, ok := literal(at + 4)
		end := at + 4 + n
		if !ok || !is(end, ')', "") || len(t) != end+1+at/2 {
			return bad()
		}
		return row(t[at+2].text, op, value)
	case len(t) >= 3 && is(0, 'i', "") && is(1, 'o', ""):
		value, n, ok := literal(2)
		if !ok || len(t) != 2+n {
			return bad()
		}
		return row(t[0].text, constraintOperators[t[1].text], value)
	}
	return bad()
}

// constraintAttribute qualifies a constraint's attribute with the source
// entity, or keeps a name already qualified (Module.Entity.Attribute).
func constraintAttribute(name, entity string) (string, bool) {
	switch strings.Count(name, ".") {
	case 0:
		if entity == "" {
			return "", false
		}
		return entity + "." + name, true
	case 2:
		return name, true
	}
	return "", false
}
