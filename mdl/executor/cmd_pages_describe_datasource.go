// SPDX-License-Identifier: Apache-2.0

// cmd_pages_describe_datasource.go — reading a widget's datasource out of page
// BSON, and rendering it back as MDL.
//
// There used to be five copies of the read switch and six of the write switch,
// one per widget family, and they disagreed about which datasource `$Type`s
// exist. The pluggable copy — the one every Gallery, DataGrid2 and chart goes
// through — knew about microflows and nanoflows but not associations, selection
// targets or context sources, so a Gallery bound over an association described
// back with no datasource at all while a ListView bound the same way kept it
// (#941). The write side had the mirror-image defect: the object-list item
// emitter had no type switch, so a chart series bound to a microflow described
// as `database from Module.TheMicroflow` and re-executing it reported
// "entity not found".
//
// A datasource is the same thing whatever contains it, so it is read and
// rendered in one place. Adding a widget family does not mean copying a switch,
// and adding a `$Type` means one case rather than five.
package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Datasource storage names. The set is the metamodel's `DataSource` interface
// (generated/metamodel/types.go): AssociationSource, CustomWidgetXPathSource,
// DataViewSource, GridXPathSource, ImageViewerSource, ListViewXPathSource,
// ListenTargetSource, MicroflowSource, NanoflowSource, ReferenceSetSource —
// plus the CustomWidgets-namespaced nanoflow source seen in stored documents.
const (
	dsTypeMicroflow         = "Forms$MicroflowSource"
	dsTypeNanoflow          = "Forms$NanoflowSource"
	dsTypeCustomNanoflow    = "CustomWidgets$CustomWidgetNanoflowSource"
	dsTypeDatabase          = "Forms$DatabaseSource"
	dsTypeCustomWidgetXPath = "CustomWidgets$CustomWidgetXPathSource"
	// Studio Pro 9's "Database" source on a pluggable widget: simple constraint
	// rows instead of XPath. Mendix 10 merged it into the XPath source and
	// converts every one on upgrade (measured with mx convert 10.12.19), so it
	// exists only in a 9.x model (mendixlabs/mxcli#1399).
	dsTypeCustomWidgetDatabase = "CustomWidgets$CustomWidgetDatabaseSource"
	dsTypeListViewXPath        = "Forms$ListViewXPathSource"
	dsTypeGridXPath            = "Forms$GridXPathSource"
	dsTypeReferenceSet         = "Forms$ReferenceSetSource"
	dsTypeImageViewer          = "Forms$ImageViewerSource"
	dsTypeAssociation          = "Forms$AssociationSource"
	dsTypeDataView             = "Forms$DataViewSource"
	dsTypeEntityPath           = "Forms$EntityPathSource"
	dsTypeListenTarget         = "Forms$ListenTargetSource"
)

// entityBackedSourceTypes are the datasources that name an entity plus an
// optional XPath constraint and sort. They differ only in which container
// declares them, which is why they share one reader.
var entityBackedSourceTypes = map[string]bool{
	dsTypeDatabase:          true,
	dsTypeCustomWidgetXPath: true,
	dsTypeListViewXPath:     true,
	dsTypeGridXPath:         true,
	dsTypeReferenceSet:      true,
	dsTypeImageViewer:       true,
}

// parseDataSource reads any widget datasource. Returns nil when ds is not a
// datasource at all; a non-nil result with Unsupported set means the datasource
// is real but has no MDL spelling, which the emitter reports as a comment
// rather than dropping (silent loss) or rendering half of it (a parse error).
func parseDataSource(ds map[string]any) *rawDataSource {
	if ds == nil {
		return nil
	}
	dsType := extractString(ds["$Type"])
	if dsType == "" {
		return nil
	}

	// Known-but-empty and unknown are different facts and get different
	// answers. A known type whose payload is empty (a listen target with no
	// target) describes a datasource the model itself has not finished — there
	// is nothing to reproduce and nothing useful to say, so it yields nil, as it
	// always has. A type this build has never heard of is mxcli's gap, not the
	// model's, and is reported rather than guessed at or silently dropped.
	known := true
	switch {
	case dsType == dsTypeMicroflow:
		if mf := microflowSourceRef(ds); mf != "" {
			raw := &rawDataSource{Type: "microflow", Reference: mf, Args: flowSourceArgs(ds, "MicroflowSettings", mf)}
			if settings, ok := ds["MicroflowSettings"].(map[string]any); ok {
				raw.FormValidations = extractString(settings["FormValidations"])
			}
			return raw
		}
	case dsType == dsTypeNanoflow:
		if nf := nanoflowSourceRef(ds); nf != "" {
			return &rawDataSource{Type: "nanoflow", Reference: nf, Args: flowSourceArgs(ds, "NanoflowSettings", nf)}
		}
	case dsType == dsTypeCustomNanoflow:
		if nf := extractString(ds["Nanoflow"]); nf != "" {
			return &rawDataSource{Type: "nanoflow", Reference: nf}
		}
	case dsType == dsTypeListenTarget:
		if target := extractString(ds["ListenTarget"]); target != "" {
			return &rawDataSource{Type: "selection", Reference: target}
		}
	case dsType == dsTypeAssociation:
		if path, ctxVar := associationSourcePath(ds); path != "" {
			return &rawDataSource{Type: "association", Reference: path, ContextVariable: ctxVar, Destination: associationSourceDestination(ds)}
		}
	case dsType == dsTypeCustomWidgetDatabase:
		if got := parseCustomWidgetDatabaseSource(ds); got != nil {
			return got
		}
	case dsType == dsTypeDataView || dsType == dsTypeEntityPath:
		if got := parseContextSource(ds); got != nil {
			return got
		}
	case entityBackedSourceTypes[dsType]:
		if got := parseEntitySource(ds); got != nil {
			return got
		}
		// A database source reached over an association from a context object.
		// It is still a database retrieve — XPath and sort bar included, and a
		// List View's search bar — so it must not fall through to the
		// association reading below, whose spelling executes as a
		// Forms$AssociationSource and drops all three: a List View's
		// (ako/mxcli#721 L5), and a Gallery's or Data grid 2's "Database"
		// source, stored as a CustomWidgetXPathSource with the same
		// IndirectEntityRef (mendixlabs/mxcli#1599).
		if dsType == dsTypeListViewXPath || dsType == dsTypeCustomWidgetXPath {
			if got := parseEntityPathSource(ds); got != nil {
				return got
			}
		}
		// A context source: an XPath source whose navigation lives in
		// SourceVariable / EntityPath, or whose hops lack a stored destination
		// and so cannot be spelled as a database path. Reading only EntityRef
		// produced `database from ` with an empty slot (#941).
		if got := parseContextSource(ds); got != nil {
			return got
		}
	default:
		known = false
	}

	if known {
		return nil
	}
	// A datasource type this build does not know. Reported, not guessed at: an
	// invented `database from` is a statement that cannot be re-executed, and
	// dropping it loses the binding without saying so.
	return &rawDataSource{Unsupported: dsType}
}

// parseEntitySource reads the entity, XPath and sort of an entity-backed
// datasource, or nil when it names no entity.
func parseEntitySource(ds map[string]any) *rawDataSource {
	entityRef, ok := ds["EntityRef"].(map[string]any)
	if !ok || entityRef == nil {
		return nil
	}
	entity := extractString(entityRef["Entity"])
	if entity == "" {
		return nil
	}
	result := &rawDataSource{
		Type:             "database",
		Reference:        entity,
		XPathConstraint:  extractString(ds["XPathConstraint"]),
		SortColumns:      parseSortColumns(ds),
		SearchAttributes: parseSearchAttributes(ds),
	}
	return result
}

// parseEntityPathSource reads a database source whose EntityRef is an
// IndirectEntityRef: the association hops from the context object, each with
// its stored destination, plus the XPath, sort and search a database source
// carries. Returns nil when any hop is incomplete — a half-read path would
// write a different retrieve.
func parseEntityPathSource(ds map[string]any) *rawDataSource {
	entityRef, ok := ds["EntityRef"].(map[string]any)
	if !ok || entityRef == nil {
		return nil
	}
	var steps []rawEntityStep
	for _, raw := range getBsonArrayElements(entityRef["Steps"]) {
		sm, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		step := rawEntityStep{
			Association:       extractString(sm["Association"]),
			DestinationEntity: extractString(sm["DestinationEntity"]),
		}
		if step.Association == "" || step.DestinationEntity == "" {
			return nil
		}
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return nil
	}
	_, ctxVar := associationSourcePath(ds)
	last := steps[len(steps)-1].DestinationEntity
	return &rawDataSource{
		Type:             "database",
		Reference:        last,
		EntitySteps:      steps,
		ContextVariable:  ctxVar,
		XPathConstraint:  extractString(ds["XPathConstraint"]),
		SortColumns:      parseSortColumns(ds),
		SearchAttributes: parseSearchAttributes(ds),
	}
}

// parseCustomWidgetDatabaseSource reads a CustomWidgets$CustomWidgetDatabaseSource
// (mendixlabs/mxcli#1399): a direct entity, or association hops from a context
// object, plus its constraint rows and sort. It is a database retrieve either
// way, so it reads as the `database from …` spelling the List View's database
// source over an association already has — the constraint rows as the XPath
// they mean, the form Mendix itself rewrites them to on the 9 → 10 upgrade.
//
// A constraint whose operator this build does not know makes the whole source
// unsupported: rendering the others would describe a wider retrieve than the
// stored one.
func parseCustomWidgetDatabaseSource(ds map[string]any) *rawDataSource {
	xpath, ok := databaseConstraintsXPath(ds)
	if !ok {
		return &rawDataSource{Unsupported: dsTypeCustomWidgetDatabase}
	}
	var got *rawDataSource
	if entityRef, _ := ds["EntityRef"].(map[string]any); entityRef != nil && extractString(entityRef["Entity"]) != "" {
		got = parseEntitySource(ds)
	} else {
		got = parseEntityPathSource(ds)
	}
	if got == nil {
		return nil
	}
	got.XPathConstraint = xpath
	return got
}

// databaseConstraintsXPath renders a database source's Forms$DatabaseConstraint
// rows as one bracketed XPath predicate, the rows joined by `and`. ok is false
// when a row cannot be rendered.
func databaseConstraintsXPath(ds map[string]any) (string, bool) {
	var terms []string
	for _, raw := range getBsonArrayElements(ds["DatabaseConstraints"]) {
		row, isMap := raw.(map[string]any)
		if !isMap {
			return "", false
		}
		term, ok := databaseConstraintXPath(extractString(row["Attribute"]), extractString(row["Operator"]), extractString(row["Value"]))
		if !ok {
			return "", false
		}
		terms = append(terms, term)
	}
	if len(terms) == 0 {
		return "", true
	}
	return "[" + strings.Join(terms, " and ") + "]", true
}

// databaseConstraintOps maps a comparison DatabaseConstraintOperator to its
// XPath operator. Contains and NotContains are functions, handled apart.
var databaseConstraintOps = map[string]string{
	"Equals":              "=",
	"NotEquals":           "!=",
	"LessThan":            "<",
	"LessThanOrEquals":    "<=",
	"GreaterThan":         ">",
	"GreaterThanOrEquals": ">=",
}

// databaseConstraintXPath renders one constraint row. The stored Value is
// untyped text, so the literal is chosen from the text alone: true/false as the
// XPath booleans, a number bare, anything else as a quoted string. The writer
// reads each literal back to the same text (parseDatabaseConstraints), so the
// choice never changes what is stored.
func databaseConstraintXPath(attribute, operator, value string) (string, bool) {
	attr := shortAttributeName(attribute)
	if attr == "" {
		return "", false
	}
	lit := databaseConstraintLiteral(value)
	switch operator {
	case "Contains":
		return "contains(" + attr + ", " + lit + ")", true
	case "NotContains":
		return "not(contains(" + attr + ", " + lit + "))", true
	}
	op, ok := databaseConstraintOps[operator]
	if !ok {
		return "", false
	}
	return attr + " " + op + " " + lit, true
}

func databaseConstraintLiteral(value string) string {
	switch {
	case value == "true" || value == "false":
		return value + "()"
	case databaseConstraintNumber.MatchString(value):
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// databaseConstraintNumber is a literal XPath reads as a number, and which the
// writer keeps verbatim.
var databaseConstraintNumber = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// entityPathExpr renders a database source's association hops as
// `$ctx/Assoc/Entity[/Assoc/Entity…]`, every destination spelled out.
func entityPathExpr(ds *rawDataSource) string {
	ctx := ds.ContextVariable
	if ctx == "" {
		ctx = "currentObject"
	}
	parts := []string{"$" + ctx}
	for _, st := range ds.EntitySteps {
		parts = append(parts, st.Association, st.DestinationEntity)
	}
	return strings.Join(parts, "/")
}

// parameterSourceEntity is the entity a parameter source stores beside the
// parameter's name (a DirectEntityRef), "" when it stores none.
func parameterSourceEntity(ds map[string]any) string {
	entityRef, ok := ds["EntityRef"].(map[string]any)
	if !ok || extractString(entityRef["$Type"]) != "DomainModels$DirectEntityRef" {
		return ""
	}
	return extractString(entityRef["Entity"])
}

// sourceParameterNames returns the page and snippet parameter a data source is
// bound to. From Mendix 10.0 they are in its SourceVariable (a
// Forms$PageVariable); before, a data view source stored the names on itself,
// as Studio Pro 9.24.30 writes them (ako/mxcli#1209).
func sourceParameterNames(ds map[string]any) (page, snippet string) {
	if sv, ok := ds["SourceVariable"].(map[string]any); ok && sv != nil {
		return extractString(sv["PageParameter"]), extractString(sv["SnippetParameter"])
	}
	return extractString(ds["PageParameter"]), extractString(ds["SnippetParameter"])
}

// parseContextSource reads the "data from context" forms: over an association,
// or straight from a page or snippet parameter.
func parseContextSource(ds map[string]any) *rawDataSource {
	if path, ctxVar := associationSourcePath(ds); path != "" {
		return &rawDataSource{Type: "association", Reference: path, ContextVariable: ctxVar, Destination: associationSourceDestination(ds)}
	}
	// Inside a snippet the parameter has its own slot, PageParameter left
	// empty; reading PageParameter alone described the data view with no
	// DataSource, and re-executing that dropped the binding.
	if page, snippet := sourceParameterNames(ds); page != "" {
		return &rawDataSource{Type: "parameter", Reference: page, Destination: parameterSourceEntity(ds)}
	} else if snippet != "" {
		return &rawDataSource{Type: "parameter", Reference: snippet, Destination: parameterSourceEntity(ds)}
	}
	if entityPath := extractString(ds["EntityPath"]); entityPath != "" {
		return &rawDataSource{Type: "parameter", Reference: entityPath}
	}
	return nil
}

// parseSearchAttributes reads a List View search bar's attributes
// (Forms$ListViewSearch.SearchRefs). Each entry is a DomainModels$AttributeRef,
// the same element a sort item carries — so this reads the same field a sort
// column does, one level less deep.
//
// Its absence is not an error: every ListViewXPathSource carries a Search
// element and all 18 in a blank 11.12.2 project are empty, so "no search
// attributes" is the overwhelmingly common case (ako/mxcli#512).
func parseSearchAttributes(ds map[string]any) []string {
	search, ok := ds["Search"].(map[string]any)
	if !ok || search == nil {
		return nil
	}
	var out []string
	for _, item := range getBsonArrayElements(search["SearchRefs"]) {
		ref, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name := shortAttributeName(extractString(ref["Attribute"])); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// parseSortColumns reads a datasource's sort, accepting both stored shapes:
// a GridSortBar (`SortBar.SortItems`, used by grid-like widgets) and a
// ListViewSort (`Sort.Paths`). Which one a datasource carries depends on its
// type, so reading whichever is present keeps this reader container-agnostic.
func parseSortColumns(ds map[string]any) []rawSortColumn {
	items := []any(nil)
	if sortBar, ok := ds["SortBar"].(map[string]any); ok && sortBar != nil {
		items = getBsonArrayElements(sortBar["SortItems"])
	}
	if len(items) == 0 {
		if sortObj, ok := ds["Sort"].(map[string]any); ok && sortObj != nil {
			items = getBsonArrayElements(sortObj["Paths"])
		}
	}
	var cols []rawSortColumn
	for _, item := range items {
		sortItem, ok := item.(map[string]any)
		if !ok {
			continue
		}
		col := rawSortColumn{Order: "asc"}
		if attrRef, ok := sortItem["AttributeRef"].(map[string]any); ok {
			col.Attribute = shortAttributeName(extractString(attrRef["Attribute"]))
			col.Associations = sortAttributeHops(attrRef)
		}
		if gridSortDirection(sortItem) == "Descending" {
			col.Order = "desc"
		}
		if col.Attribute != "" {
			cols = append(cols, col)
		}
	}
	return cols
}

// sortAttributeHops reads the association hops of a stored AttributeRef — its
// EntityRef.Steps, one DomainModels$EntityRefStep per hop. An own-entity
// attribute carries a DirectEntityRef or no EntityRef and yields none.
//
// Without this a sort over an association described as a bare attribute name,
// and the replay had to guess the hop back (mendixlabs/mxcli#1152).
func sortAttributeHops(attrRef map[string]any) []string {
	entityRef, ok := attrRef["EntityRef"].(map[string]any)
	if !ok || entityRef == nil {
		return nil
	}
	var hops []string
	for _, raw := range getBsonArrayElements(entityRef["Steps"]) {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if assoc := extractString(step["Association"]); assoc != "" {
			hops = append(hops, assoc)
		}
	}
	return hops
}

// sortColumnPath renders a sort column as `Assoc/.../Attribute`.
func sortColumnPath(col rawSortColumn) string {
	if len(col.Associations) == 0 {
		return col.Attribute
	}
	return strings.Join(append(append([]string{}, col.Associations...), col.Attribute), "/")
}

// dataSourceExpr renders a datasource as the MDL that reproduces it — the part
// after `DataSource: `. Returns "" when there is nothing to emit.
//
// One renderer for every widget, because a datasource means the same thing
// wherever it appears. The six hand-written switches this replaces disagreed:
// the object-list one had no switch at all and labelled everything `database`,
// and two others omitted the WHERE and SORT they had read (#941).
func dataSourceExpr(ctx *ExecContext, ds *rawDataSource) string {
	if ds == nil || ds.Unsupported != "" {
		return ""
	}
	switch ds.Type {
	case "database":
		// Never render a database source with no entity: `database from ` parses
		// `from` as the entity name and reports "entity not found: from".
		if ds.Reference == "" {
			return ""
		}
		from := ds.Reference
		if len(ds.EntitySteps) > 0 {
			from = entityPathExpr(ds)
		}
		expr := "database from " + from
		if clause := xpathConstraintClause(ctx, ds.XPathConstraint); clause != "" {
			expr += " where " + clause
		}
		if len(ds.SortColumns) > 0 {
			parts := make([]string, 0, len(ds.SortColumns))
			for _, col := range ds.SortColumns {
				parts = append(parts, sortColumnPath(col)+" "+col.Order)
			}
			expr += " sort by " + strings.Join(parts, ", ")
		}
		// After `sort by`, matching the grammar's clause order so the emitted
		// text re-parses (ako/mxcli#512).
		if len(ds.SearchAttributes) > 0 {
			expr += " search by " + strings.Join(ds.SearchAttributes, ", ")
		}
		return expr
	case "microflow", "nanoflow":
		if ds.Reference == "" {
			return ""
		}
		expr := ds.Type + " " + ds.Reference
		if len(ds.Args) > 0 {
			parts := make([]string, 0, len(ds.Args))
			for _, arg := range ds.Args {
				parts = append(parts, visitor.ParameterNameSpelling(arg.Name)+" = "+arg.Value)
			}
			expr += "(" + strings.Join(parts, ", ") + ")"
		}
		if ds.FormValidations != "" && ds.FormValidations != "All" {
			expr += " with (FormValidations: " + ds.FormValidations + ")"
		}
		return expr
	case "parameter":
		if ds.Reference == "" {
			return ""
		}
		// A page parameter is stored bare ("Account") while an entity path
		// arrives already written as an expression ("$Account/Mod.Assoc"), and
		// both reach here as Type "parameter". Prefixing only the bare form
		// keeps each spelling as MDL writes it.
		if strings.HasPrefix(ds.Reference, "$") {
			return ds.Reference
		}
		return "$" + ds.Reference
	case "selection":
		if ds.Reference == "" {
			return ""
		}
		return "selection " + mdlIdent(ds.Reference)
	case "association":
		if ds.Reference == "" {
			return ""
		}
		return associationDataSourceExpr(withSpecialisedDestination(ctx, ds))
	}
	return ""
}

// withSpecialisedDestination returns ds with its single hop spelled
// `Assoc/Destination` when the stored destination is neither end of the
// association — a specialisation of the end, which re-deriving from the
// association alone loses. Measured on TestApp's
// WorkflowCommons.ManageTaskAssignments (11.14): `TaskAssignmentHelper_User`
// stored with DestinationEntity Administration.Account was described as the
// bare association, rebuilt to System.User, and the labels bound to Account's
// FullName became CE1613 (ako/mxcli#2055). A hop that arrives at an end keeps
// its short spelling.
func withSpecialisedDestination(ctx *ExecContext, ds *rawDataSource) *rawDataSource {
	if ds.Destination == "" || strings.Contains(ds.Reference, "/") || ctx == nil || ctx.Backend == nil {
		return ds
	}
	ends, ok := projectAssociationEnds(ctx)[ds.Reference]
	if !ok || ends[0] == "" || ends[1] == "" || ds.Destination == ends[0] || ds.Destination == ends[1] {
		return ds
	}
	cp := *ds
	cp.Reference = ds.Reference + "/" + ds.Destination
	return &cp
}

// dataSourceProp renders the whole `DataSource: …` property, or "" when the
// datasource cannot be expressed.
func dataSourceProp(ctx *ExecContext, ds *rawDataSource) string {
	expr := dataSourceExpr(ctx, ds)
	if expr == "" {
		return ""
	}
	return "DataSource: " + expr
}

// dataSourceComment describes a datasource MDL cannot express, so a reader of
// the output learns the binding exists rather than silently losing it. Returns
// "" for a datasource that renders normally.
func dataSourceComment(ctx *ExecContext, ds *rawDataSource) string {
	if ds == nil {
		return ""
	}
	if ds.Unsupported != "" {
		return fmt.Sprintf("-- DataSource (%s) has no MDL spelling and is not reproduced here", ds.Unsupported)
	}
	if dataSourceExpr(ctx, ds) == "" && ds.Type != "" {
		return fmt.Sprintf("-- DataSource (%s) is incomplete in the model and is not reproduced here", ds.Type)
	}
	return ""
}

// appendDataSourceProp adds a widget's `DataSource: …` property, or — when the
// datasource cannot be spelled in MDL — a comment saying so.
//
// Every widget goes through this, so a datasource cannot be rendered one way in
// a DataView and another in a Gallery, which is the drift #941 was.
func appendDataSourceProp(ctx *ExecContext, props []string, ds *rawDataSource) []string {
	if prop := dataSourceProp(ctx, ds); prop != "" {
		return append(props, prop)
	}
	if comment := dataSourceComment(ctx, ds); comment != "" {
		return append(props, comment)
	}
	return props
}

// appendWidgetDataSources emits a widget's datasource properties: the named
// spelling when it has several, the generic `DataSource:` clause otherwise.
//
// Every branch of the emitter goes through this for the same reason they all go
// through appendDataSourceProp — a datasource must not be rendered one way in a
// DataView and another in a Gallery, which is the drift #941 was. A branch that
// read w.DataSource directly would silently keep describing a multi-source
// widget as single-source.
func appendWidgetDataSources(ctx *ExecContext, props []string, w rawWidget) []string {
	if len(w.NamedDataSources) > 0 {
		return appendNamedDataSourceProps(ctx, props, w.NamedDataSources)
	}
	return appendDataSourceProp(ctx, props, w.DataSource)
}

// appendNamedDataSourceProps adds one `<schemaKey>: <datasource>` property per
// datasource on a widget that has several, so a describe → exec round trip keeps
// each binding on the mapping it came from.
//
// A source whose schema key did not resolve falls back to the unnamed
// `DataSource:` spelling — the output it would have had before there was a key
// to print. Losing it instead would be #956 with extra steps.
func appendNamedDataSourceProps(ctx *ExecContext, props []string, sources []rawNamedDataSource) []string {
	for _, src := range sources {
		if src.Key == "" {
			props = appendDataSourceProp(ctx, props, src.DataSource)
			continue
		}
		if expr := dataSourceExpr(ctx, src.DataSource); expr != "" {
			props = append(props, fmt.Sprintf("%s: %s", src.Key, expr))
			continue
		}
		// Not spellable in MDL — say so under this key, so a reader learns which
		// of the widget's bindings is the one that did not come through.
		if comment := dataSourceComment(ctx, src.DataSource); comment != "" {
			props = append(props, strings.Replace(comment, "-- DataSource ", "-- "+src.Key+" ", 1))
		}
	}
	return props
}

// xpathConstraintClause renders a stored XPath constraint as the MDL the page
// grammar accepts after WHERE, or "" when there is no constraint.
//
// Always the bracketed `xpathConstraint` production, never a bare expression.
// The grammar takes either, and the bare form reads more nicely — but it only
// parses for simple comparisons, and whether a given XPath happens to also be a
// valid MDL expression is not a question this emitter can answer. Emitting it
// raw is what turned stock Administration.Account_New from describing cleanly
// into describing to MDL that does not parse: its constraint carries an inner
// predicate (`System.grantableRoles[reversed()]/…`), and the `[` ends the
// expression parse (mxcli-formula1 §57.1). A bracketed group is accepted for
// every constraint, so the emitter does not have to be right about which is
// which.
//
// Splitting goes through the same quote- and nesting-aware helper the microflow
// emitter uses (#772) rather than a second copy: the previous code here took
// the outer brackets off by testing the first and last byte, which turns
// `[a][b]` into the mangled `a][b`.
func xpathConstraintClause(ctx *ExecContext, constraint string) string {
	xpath := strings.TrimSpace(constraint)
	if xpath == "" {
		return ""
	}
	// A stored constraint may be broken across lines — mxcli formats a long one
	// that way so it can be read in Studio Pro's editor, and a person may have
	// done the same by hand (upstream #979). MDL keeps it on one line: the
	// datasource is one property among several on a widget, and the executor
	// re-derives the stored layout from the expression anyway.
	//
	// A string in it is spelled for the describe language (describeXPath,
	// ako/mxcli#825).
	xpath = visitor.FlattenXPathConstraint(xpath)
	if groups := visitor.SplitXPathPredicateGroups(xpath); len(groups) > 0 {
		return describeXPath(ctx, strings.Join(groups, " "))
	}
	return describeXPath(ctx, "["+xpath+"]")
}

// flowSourceArgs reads the argument bindings of a microflow or nanoflow
// datasource from its settings sub-document.
//
// Storage qualifies each parameter with the flow it belongs to
// ("Mod.DS_Filtered.Term"), while MDL names the parameter alone — the flow is
// already on the left of the parentheses. The prefix is stripped by matching
// the flow's own qualified name rather than by cutting at the last dot, so a
// document that stores the name bare is left alone instead of losing its first
// segment.
//
// The bound value lives in Expression for a literal or an expression, and in
// Variable — a Forms$PageVariable naming a page parameter, snippet parameter or
// page variable — for a $-reference. Reading only Expression, or reading Variable
// as a flat string, drops every argument Studio Pro wrote (#1140).
// A parameterless flow yields nil, which the renderer emits without parentheses
// — the grammar makes the list optional, and adding empty parens would churn
// every existing description.
//
// A nanoflow source is FLAT in Studio Pro's shape — ParameterMappings directly
// on the source — so the source itself is read when it has no settings child;
// the nested NanoflowSettings form is what mxcli wrote before CE2633 was fixed.
func flowSourceArgs(ds map[string]any, settingsKey, flowName string) []rawDataSourceArg {
	settings, ok := ds[settingsKey].(map[string]any)
	if !ok || settings == nil {
		if _, flat := ds["ParameterMappings"]; !flat {
			return nil
		}
		settings = ds
	}
	var out []rawDataSourceArg
	for _, item := range getBsonArrayElements(settings["ParameterMappings"]) {
		mapping, ok := item.(map[string]any)
		if !ok || mapping == nil {
			continue
		}
		name := strings.TrimPrefix(extractString(mapping["Parameter"]), flowName+".")
		if name == "" {
			continue
		}
		value := strings.TrimSpace(extractString(mapping["Expression"]))
		if value == "" {
			value = flowArgVariableValue(mapping["Variable"])
		}
		if value == "" {
			// A pre-#1140 document, or another writer, may have put the bare
			// reference text in Variable.
			value = extractString(mapping["Variable"])
		}
		if value == "" {
			// A parameter with no bound value is what CE1571 is about. There is
			// nothing to reproduce, and inventing one would be worse than
			// leaving the gap the model already has.
			continue
		}
		out = append(out, rawDataSourceArg{Name: name, Value: value})
	}
	return out
}
