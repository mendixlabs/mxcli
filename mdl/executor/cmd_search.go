// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// execShowCallers handles SHOW CALLERS OF Module.Microflow [TRANSITIVE].
// callerRefKinds are the reference kinds that mean "this thing invokes the
// target". `show callers` filtered on 'call' alone, which is the kind a MICROFLOW
// activity produces — so a microflow called from a page action button (kind
// 'action') reported "(no callers found)" even though the reference was sitting
// in the refs table, and a page opened by a button or a menu was equally
// invisible (issue #773).
//
// A false negative here reads as "nothing uses this", which is the answer
// somebody acts on before deleting a document — so the set errs toward
// including a kind rather than omitting it.
//
// The recurring shape is an ENTRY POINT: something outside the call graph
// invokes it, so nothing in the model calls it. 'schedule' was the first (a
// microflow run only by a scheduled event reported "(no callers found)" on a job
// that runs nightly); 'publish' is a published REST operation (#1126), or a
// published OData resource, action or authentication microflow
// (ako/mxcli#1639); 'event' is an entity event handler, run by the entity on
// commit or delete, or a business event handler; 'settings'
// is a microflow wired as after-startup, before-shutdown or health check, whose
// edge shipped in v0.22.0 and was added to QUAL004 but not here, so
// `show callers` stayed blind to it. Every new way for something to run a
// microflow belongs in this list and in graphRefKinds — two consumers, neither
// of which shares the other's list. (QUAL004 had a third, MICROFLOW_ENTRY_KINDS;
// it fell behind on the navigation kinds and was removed in favour of counting
// every edge, ako/mxcli#1484.)
//
// Deliberately excluded: 'datasource', 'parameter', 'return', 'retrieve',
// 'create', 'change', 'delete', 'commit', 'associate', 'generalize', 'layout'
// and 'sync'.
// Those are uses of a TYPE or a LAYOUT, not invocations, and folding them in
// would make `show callers of <entity>` a synonym for `show references to`.
var callerRefKinds = []string{
	RefKindCallerCall,     // microflow/nanoflow call activity
	RefKindCallerAction,   // widget action: button, on-change, on-click
	RefKindCallerShowPage, // microflow show-page activity, or a widget action opening a page
	// A page, snippet or layout placing a snippet. Recorded as show_page until
	// ako/mxcli#2017, so leaving it out would turn every snippet uncalled.
	RefKindCallerSnippetCall,
	// A flow applying an import or export mapping (ako/mxcli#1861).
	RefKindCallerMappingCall,
	RefKindCallerCalculate, // calculated attribute
	RefKindCallerHomePage,  // navigation
	RefKindCallerLoginPage,
	RefKindCallerMenuItem,
	RefKindCallerSchedule, // scheduled event: the microflow it runs
	RefKindCallerPublish,  // published REST operation: the microflow behind the endpoint
	RefKindCallerSettings, // project setting: after-startup, before-shutdown, health check
	RefKindCallerEvent,    // entity event handler: the microflow it runs
	// An agent's knowledge bases, MCP services and model are used when it
	// runs, by no flow (ako/mxcli#2074); its tool microflows are calls.
	RefKindCallerTool,
	RefKindCallerModel,
}

// Kind literals, kept next to the set that uses them so the SQL below cannot
// drift from mdl/catalog's constants unnoticed.
const (
	RefKindCallerCall        = "call"
	RefKindCallerAction      = "action"
	RefKindCallerShowPage    = "show_page"
	RefKindCallerSnippetCall = "snippet_call"
	RefKindCallerMappingCall = "mapping_call"
	RefKindCallerCalculate   = "calculate"
	RefKindCallerHomePage    = "home_page"
	RefKindCallerLoginPage   = "login_page"
	RefKindCallerMenuItem    = "menu_item"
	RefKindCallerSchedule    = "schedule"
	RefKindCallerPublish     = "publish"
	RefKindCallerSettings    = "settings"
	RefKindCallerEvent       = "event"
	RefKindCallerTool        = "tool"
	RefKindCallerModel       = "model"
)

// callerRefKindsSQL renders callerRefKinds as a SQL IN list.
func callerRefKindsSQL() string {
	quoted := make([]string, len(callerRefKinds))
	for i, k := range callerRefKinds {
		quoted[i] = "'" + k + "'"
	}
	return "(" + strings.Join(quoted, ", ") + ")"
}

func execShowCallers(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show callers")
	}

	// Ensure catalog is available with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	targetName := s.Name.String()
	fmt.Fprintf(ctx.progress(), "\nCallers of %s", targetName)
	if s.Transitive {
		fmt.Fprintln(ctx.progress(), " (transitive)")
	} else {
		fmt.Fprintln(ctx.progress(), "")
	}

	var query string
	if s.Transitive {
		// Recursive CTE for transitive callers
		query = `
			with RECURSIVE callers_cte as (
				select SourceName as Caller, 1 as Depth
				from refs
				where TargetName = ? and RefKind in ` + callerRefKindsSQL() + `
				union all
				select r.SourceName, c.Depth + 1
				from refs r
				join callers_cte c on r.TargetName = c.Caller
				where r.RefKind in ` + callerRefKindsSQL() + ` and c.Depth < 10
			)
			select distinct Caller, min(Depth) as Depth
			from callers_cte
			GROUP by Caller
			ORDER by Depth, Caller
		`
	} else {
		// Direct callers only
		query = `
			select distinct SourceName as Caller, 1 as Depth
			from refs
			where TargetName = ? and RefKind in ` + callerRefKindsSQL() + `
			ORDER by Caller
		`
	}

	result, err := ctx.Catalog.Query(strings.Replace(query, "?", "'"+targetName+"'", 1))
	if err != nil {
		return mdlerrors.NewBackend("query callers", err)
	}

	if result.Count == 0 {
		return writeEmptyResult(ctx, result.Columns, "(no callers found)")
	}

	fmt.Fprintf(ctx.progress(), "Found %d caller(s)\n", result.Count)
	outputCatalogResults(ctx, result)
	return nil
}

// execShowCallees handles SHOW CALLEES OF Module.Microflow [TRANSITIVE].
func execShowCallees(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show callees")
	}

	// Ensure catalog is available with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	sourceName := s.Name.String()
	fmt.Fprintf(ctx.progress(), "\nCallees of %s", sourceName)
	if s.Transitive {
		fmt.Fprintln(ctx.progress(), " (transitive)")
	} else {
		fmt.Fprintln(ctx.progress(), "")
	}

	var query string
	if s.Transitive {
		// Recursive CTE for transitive callees
		query = `
			with RECURSIVE callees_cte as (
				select TargetName as Callee, 1 as Depth
				from refs
				where SourceName = ? and RefKind = 'call'
				union all
				select r.TargetName, c.Depth + 1
				from refs r
				join callees_cte c on r.SourceName = c.Callee
				where r.RefKind = 'call' and c.Depth < 10
			)
			select distinct Callee, min(Depth) as Depth
			from callees_cte
			GROUP by Callee
			ORDER by Depth, Callee
		`
	} else {
		// Direct callees only
		query = `
			select distinct TargetName as Callee, 1 as Depth
			from refs
			where SourceName = ? and RefKind = 'call'
			ORDER by Callee
		`
	}

	result, err := ctx.Catalog.Query(strings.Replace(query, "?", "'"+sourceName+"'", 1))
	if err != nil {
		return mdlerrors.NewBackend("query callees", err)
	}

	if result.Count == 0 {
		return writeEmptyResult(ctx, result.Columns, "(no callees found)")
	}

	fmt.Fprintf(ctx.progress(), "Found %d callee(s)\n", result.Count)
	outputCatalogResults(ctx, result)
	return nil
}

// execShowReferences handles SHOW REFERENCES TO Module.Entity.
func execShowReferences(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show references")
	}

	// Ensure catalog is available with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	return showReferences(ctx, s.Name.String())
}

// showReferences prints the references to typed from the loaded catalog.
//
// One row per distinct (source, kind): a microflow with two retrieve
// activities over the same entity is one retrieve reference, not two rows
// that read as two callers.
func showReferences(ctx *ExecContext, typed string) error {
	fmt.Fprintf(ctx.progress(), "\nReferences to %s\n", typed)

	// A widget's TargetName is stored SHOUTED (COMBOBOX) while MDL keywords are
	// written in lower case, so an exact-only match answers the natural spelling
	// with "(no references found)" — wrong, not missing. See resolveReferenceTarget.
	targetName, loose := resolveReferenceTarget(ctx, typed)
	reportResolvedTarget(ctx, typed, targetName, loose)

	where, viaValues := refTargetWhere(ctx, targetName)
	cols, order := "SourceType, SourceName, RefKind", "RefKind, SourceType, SourceName"
	if viaValues {
		cols, order = cols+", TargetName as Target", order+", Target"
	}
	query := `select distinct ` + cols + ` from refs where ` + where + ` order by ` + order

	result, err := ctx.Catalog.Query(query)
	if err != nil {
		return mdlerrors.NewBackend("query references", err)
	}

	if result.Count == 0 {
		return writeEmptyResult(ctx, result.Columns, noReferencesMessage(ctx, targetName))
	}

	fmt.Fprintf(ctx.progress(), "Found %d reference(s)\n", result.Count)
	outputCatalogResults(ctx, result)
	return nil
}

// execShowImpact handles SHOW IMPACT OF Module.Entity.
// This shows all elements that would be affected by changing the target.
func execShowImpact(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show impact")
	}

	// Ensure catalog is available with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	return showImpact(ctx, s.Name.String())
}

// showImpact prints the elements that reference typed, from the loaded catalog.
//
// The summary counts ELEMENTS: it used to count rows, so a microflow that both
// retrieves and deletes an entity was two "affected" microflows, and the types
// came out in map order, different from run to run.
func showImpact(ctx *ExecContext, typed string) error {
	fmt.Fprintf(ctx.progress(), "\nImpact analysis for %s\n", typed)

	targetName, loose := resolveReferenceTarget(ctx, typed)
	reportResolvedTarget(ctx, typed, targetName, loose)

	where, viaValues := refTargetWhere(ctx, targetName)
	cols, order := "SourceType, SourceName, RefKind", "SourceType, SourceName, RefKind"
	if viaValues {
		cols, order = cols+", TargetName as Target", order+", Target"
	}
	directQuery := `select distinct ` + cols + ` from refs where ` + where + ` order by ` + order

	result, err := ctx.Catalog.Query(directQuery)
	if err != nil {
		return mdlerrors.NewBackend("query impact", err)
	}

	if result.Count == 0 {
		return writeEmptyResult(ctx, result.Columns, noReferencesMessage(ctx, targetName))
	}

	// Distinct elements per type. Rows are ordered by SourceType, so the types
	// come out sorted.
	var types []string
	perType := map[string]map[string]bool{}
	elements := 0
	for _, row := range result.Rows {
		if len(row) < 2 {
			continue
		}
		t, name := fmt.Sprint(row[0]), fmt.Sprint(row[1])
		if perType[t] == nil {
			perType[t] = map[string]bool{}
			types = append(types, t)
		}
		if !perType[t][name] {
			perType[t][name] = true
			elements++
		}
	}

	fmt.Fprintf(ctx.progress(), "\nSummary:\n")
	for _, t := range types {
		fmt.Fprintf(ctx.progress(), "  %s: %d\n", t, len(perType[t]))
	}
	fmt.Fprintln(ctx.progress())

	fmt.Fprintf(ctx.progress(), "Found %d affected element(s) (%d reference(s))\n", elements, result.Count)
	outputCatalogResults(ctx, result)

	return nil
}

// --- Executor method wrappers for backward compatibility ---
