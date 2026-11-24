// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
)

// resolveReferenceTarget returns the spelling of `name` that CATALOG.REFS
// actually stores, and whether it differs from what the user typed.
//
// SHOW REFERENCES TO and SHOW IMPACT OF match TargetName exactly. Every target
// used to be a module-qualified name, which a user copies verbatim from
// `show entities`, so exact matching was right and nothing needed this.
//
// The `widget` edge (slice 5 of PROPOSAL_def_driven_widget_bodies.md) breaks
// that assumption: its TargetName is the widget's MDL name, which is stored
// SHOUTED (COMBOBOX) because that is how widget_definitions_data holds it,
// while MDL keywords are case-insensitive and every example writes them in
// lower case. So the natural
//
//	show references to combobox
//
// found nothing — and reported "(no references found)", which is a WRONG
// answer rather than a missing one. That is the failure mode worth spending a
// lookup to avoid: a user cannot tell it from a widget genuinely being unused.
//
// The fallback is deliberately second, never first. An exact match is returned
// untouched, so no existing answer can change; only a query that would have
// returned nothing gets a second chance. Callers report the resolved spelling
// so the user can see which name was actually matched.
func resolveReferenceTarget(ctx *ExecContext, name string) (resolved string, matchedLoosely bool) {
	if ctx == nil || ctx.Catalog == nil || name == "" {
		return name, false
	}

	exact, err := ctx.Catalog.Query(fmt.Sprintf(
		`SELECT 1 FROM refs WHERE TargetName = '%s' LIMIT 1`, escapeSQLString(name)))
	if err == nil && exact.Count > 0 {
		return name, false
	}

	// Nothing under that spelling. Try case-insensitively, and only accept the
	// answer when it is unambiguous — two targets differing only in case are a
	// question this cannot answer for the user, so leave the exact (empty)
	// result rather than guessing at one of them.
	loose, err := ctx.Catalog.Query(fmt.Sprintf(
		`SELECT DISTINCT TargetName FROM refs WHERE lower(TargetName) = lower('%s')`,
		escapeSQLString(name)))
	if err != nil || loose.Count != 1 || len(loose.Rows) != 1 || len(loose.Rows[0]) == 0 {
		return name, false
	}
	match, ok := loose.Rows[0][0].(string)
	if !ok || match == "" || match == name {
		return name, false
	}
	return match, true
}

// reportResolvedTarget tells the user which stored spelling was matched, when
// it is not the one they typed. Silent on an exact match.
func reportResolvedTarget(ctx *ExecContext, typed, resolved string, matchedLoosely bool) {
	if !matchedLoosely || ctx == nil || ctx.Output == nil {
		return
	}
	fmt.Fprintf(ctx.progress(), "(matched %s)\n", strings.TrimSpace(resolved))
}

// refTargetWhere returns the refs WHERE clause for a target. An enumeration is
// used through its values as well as its type — a page comparing against
// Mod.Enum.Value breaks when the value is removed — so for an enumeration the
// clause also takes the edges to each of its values, and viaValues reports that
// the caller should show which target each row reached.
func refTargetWhere(ctx *ExecContext, target string) (where string, viaValues bool) {
	esc := escapeSQLString(target)
	where = fmt.Sprintf("TargetName = '%s'", esc)
	if catalogHas(ctx, fmt.Sprintf(`select 1 from enumerations where QualifiedName = '%s' limit 1`, esc)) {
		// The name is a LIKE prefix, so its underscores (ENUM_Status) must not
		// act as wildcards.
		where = fmt.Sprintf(`(TargetName = '%s' or (TargetType = 'ENUMERATION_VALUE' and TargetName like '%s.%%' escape '\'))`,
			esc, escapeSQLString(escapeSQLLike(target)))
		return where, true
	}
	return where, false
}

// escapeSQLLike escapes LIKE wildcards; paired with `escape '\'`.
func escapeSQLLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func catalogHas(ctx *ExecContext, query string) bool {
	if ctx == nil || ctx.Catalog == nil {
		return false
	}
	res, err := ctx.Catalog.Query(query)
	return err == nil && res.Count > 0
}

// noReferencesMessage is what refs/impact print when no edge reaches target.
//
// It says what was searched rather than "element is not referenced": the
// reference graph is built from the sites the catalog resolves, and for an
// attribute or an enumeration value some sites are free text it does not
// resolve. "Not referenced" was read — by an agent, reasonably — as "safe to
// delete", on attributes that were in use.
func noReferencesMessage(ctx *ExecContext, target string) string {
	esc := escapeSQLString(target)
	member := target
	if i := strings.LastIndex(target, "."); i >= 0 {
		member = target[i+1:]
	}
	switch {
	case catalogHas(ctx, fmt.Sprintf(`select 1 from attributes where EntityQualifiedName || '.' || Name = '%s' limit 1`, esc)):
		return fmt.Sprintf("(no references found to attribute %s)\n"+
			"Checked: attribute bindings and member changes in microflows, nanoflows, rules, pages,\n"+
			"snippets, workflows and import/export mappings, and XPath constraints.\n"+
			"Not checked: the attribute named through a variable in an expression ($Object/%s).\n"+
			"Run `search '%s'` before treating it as unused.", target, member, member)
	case catalogHas(ctx, fmt.Sprintf(`select 1 from enumeration_values where EnumerationQualifiedName || '.' || Name = '%s' limit 1`, esc)):
		return fmt.Sprintf("(no references found to enumeration value %s)\n"+
			"Checked: qualified uses in expressions (%s) and XPath comparisons of an enumeration\n"+
			"attribute with '%s'.\n"+
			"Not checked: decision branches on an enumeration, which store the bare value name.\n"+
			"Run `search '%s'` before treating it as unused.", target, target, member, member)
	}
	if why := entryPointReason(ctx, target); why != "" {
		// Used without an inbound edge (ako/mxcli#2241): a page or microflow
		// opened by its URL, or one Studio Pro is told to treat as used.
		return fmt.Sprintf("(no references found: nothing in the catalog's reference graph points at %s,\n"+
			"but it is an entry point: %s — do not treat it as unused)", target, why)
	}
	return fmt.Sprintf("(no references found: nothing in the catalog's reference graph points at %s)", target)
}

// entryPointReason says why a page or microflow is used without anything
// referencing it — its URL, or Studio Pro's "Mark as used" — or "".
func entryPointReason(ctx *ExecContext, target string) string {
	if ctx == nil || ctx.Catalog == nil {
		return ""
	}
	esc := escapeSQLString(target)
	res, err := ctx.Catalog.Query(fmt.Sprintf(
		`select 'page', COALESCE(URL, ''), MarkAsUsed from pages where QualifiedName = '%s'
		 union all
		 select lower(MicroflowType), COALESCE(URL, ''), MarkAsUsed from microflows where QualifiedName = '%s'`, esc, esc))
	if err != nil || res == nil || len(res.Rows) == 0 {
		return ""
	}
	row := res.Rows[0]
	kind, _ := row[0].(string)
	url, _ := row[1].(string)
	var reasons []string
	if url != "" {
		reasons = append(reasons, fmt.Sprintf("the %s has URL '%s'", kind, url))
	}
	if truthy(row[2]) {
		reasons = append(reasons, "it is marked as used in Studio Pro")
	}
	return strings.Join(reasons, ", and ")
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	case string:
		return x == "1" || strings.EqualFold(x, "true")
	}
	return false
}
