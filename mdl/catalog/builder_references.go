// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// Reference kinds for the refs table
const (
	RefKindCall       = "call"       // Microflow calls microflow
	RefKindCreate     = "create"     // Microflow creates entity
	RefKindRetrieve   = "retrieve"   // Microflow retrieves entity
	RefKindShowPage   = "show_page"  // Microflow shows page
	RefKindGeneralize = "generalize" // Entity extends entity
	RefKindAssociate  = "associate"  // Association targets entity
	RefKindLayout     = "layout"     // Page uses layout
	RefKindDatasource = "datasource" // Page/widget uses entity via datasource
	RefKindParameter  = "parameter"  // Page parameter entity type
	RefKindAction     = "action"     // Widget calls microflow/nanoflow
	RefKindHomePage   = "home_page"  // Navigation home page reference
	RefKindLoginPage  = "login_page" // Navigation login page reference
	RefKindMenuItem   = "menu_item"  // Navigation menu item page reference
	RefKindChange     = "change"     // Microflow changes an entity object
	RefKindDelete     = "delete"     // Microflow deletes an entity object
	RefKindCalculate  = "calculate"  // Calculated attribute uses a microflow
	RefKindReturn     = "return"     // Microflow/nanoflow returns an entity type
)

// collectActionActivities returns all ActionActivity objects from an ObjectCollection,
// recursing into LoopedActivity bodies to find nested actions.
func collectActionActivities(oc *microflows.MicroflowObjectCollection) []*microflows.ActionActivity {
	if oc == nil {
		return nil
	}
	var result []*microflows.ActionActivity
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if o.Action != nil {
				result = append(result, o)
			}
		case *microflows.LoopedActivity:
			result = append(result, collectActionActivities(o.ObjectCollection)...)
		}
	}
	return result
}

// microflowActionRef returns the cross-reference a microflow action makes to
// another document: the target's catalog object type, its qualified name, and
// the RefKind label. ok is false when the action references no resolvable
// document target — either it operates only on local variables (change/delete/
// commit/cast resolve their entity through a variable's type, which needs
// dataflow analysis) or the reference is by ID and the name wasn't captured.
//
// Like getMicroflowActionType, this is a single switch rather than ref-emission
// scattered through buildReferences, so the set of reference-bearing actions is
// auditable in one place and unit-testable without a full catalog build.
func microflowActionRef(action microflows.MicroflowAction) (targetType, targetName, refKind string, ok bool) {
	switch a := action.(type) {
	case *microflows.MicroflowCallAction:
		if a.MicroflowCall != nil && a.MicroflowCall.Microflow != "" {
			return "MICROFLOW", a.MicroflowCall.Microflow, RefKindCall, true
		}
	case *microflows.NanoflowCallAction:
		if a.NanoflowCall != nil && a.NanoflowCall.Nanoflow != "" {
			return "NANOFLOW", a.NanoflowCall.Nanoflow, RefKindCall, true
		}
	case *microflows.JavaActionCallAction:
		if a.JavaAction != "" {
			return "JAVA_ACTION", a.JavaAction, RefKindCall, true
		}
	case *microflows.RestOperationCallAction:
		// Operation is a "Module.Service.Operation" name referencing a consumed
		// REST service operation.
		if a.Operation != "" {
			return "REST_OPERATION", a.Operation, RefKindCall, true
		}
	case *microflows.CreateObjectAction:
		if a.EntityQualifiedName != "" {
			return "ENTITY", a.EntityQualifiedName, RefKindCreate, true
		}
	case *microflows.ShowPageAction:
		if a.PageName != "" {
			return "PAGE", a.PageName, RefKindShowPage, true
		}
	case *microflows.RetrieveAction:
		if a.Source == nil {
			break
		}
		switch src := a.Source.(type) {
		case *microflows.DatabaseRetrieveSource:
			if src.EntityQualifiedName != "" {
				return "ENTITY", src.EntityQualifiedName, RefKindRetrieve, true
			}
		case *microflows.AssociationRetrieveSource:
			if src.AssociationQualifiedName != "" {
				return "ASSOCIATION", src.AssociationQualifiedName, RefKindRetrieve, true
			}
		}
	}
	return "", "", "", false
}

// entityOfDataType returns the entity qualified name a microflow data type refers
// to (object or list of objects), or "" for primitive/enum/void types.
func entityOfDataType(dt microflows.DataType) string {
	switch t := dt.(type) {
	case *microflows.ObjectType:
		return t.EntityQualifiedName
	case *microflows.ListType:
		return t.EntityQualifiedName
	}
	return ""
}

// microflowVarActionRef resolves the entity reference of an action that operates
// on a *variable* (change/delete) rather than naming an entity directly. varEntity
// maps a (normalised) variable name to the entity qualified name it holds, built
// by the caller from parameters + create/retrieve outputs in the same flow. This
// is intra-flow resolution only — no cross-flow dataflow — so it returns ok=false
// when the variable's entity isn't known locally.
func microflowVarActionRef(action microflows.MicroflowAction, varEntity map[string]string) (targetType, targetName, refKind string, ok bool) {
	resolve := func(v string) (string, bool) {
		qn, found := varEntity[strings.TrimPrefix(v, "$")]
		return qn, found && qn != ""
	}
	switch a := action.(type) {
	case *microflows.ChangeObjectAction:
		if qn, found := resolve(a.ChangeVariable); found {
			return "ENTITY", qn, RefKindChange, true
		}
	case *microflows.DeleteObjectAction:
		if qn, found := resolve(a.DeleteVariable); found {
			return "ENTITY", qn, RefKindDelete, true
		}
	}
	return "", "", "", false
}

// buildVarEntityMap builds the variable→entity map for a single flow: seed with
// object-typed parameters, then walk create/retrieve actions whose output
// variable holds a known entity. Linear in the number of activities.
func buildVarEntityMap(params []*microflows.MicroflowParameter, acts []*microflows.ActionActivity) map[string]string {
	norm := func(v string) string { return strings.TrimPrefix(v, "$") }
	varEntity := map[string]string{}
	for _, p := range params {
		if qn := entityOfDataType(p.Type); qn != "" {
			varEntity[norm(p.Name)] = qn
		}
	}
	for _, act := range acts {
		switch a := act.Action.(type) {
		case *microflows.CreateObjectAction:
			if a.OutputVariable != "" && a.EntityQualifiedName != "" {
				varEntity[norm(a.OutputVariable)] = a.EntityQualifiedName
			}
		case *microflows.RetrieveAction:
			if a.OutputVariable == "" || a.Source == nil {
				continue
			}
			if db, ok := a.Source.(*microflows.DatabaseRetrieveSource); ok && db.EntityQualifiedName != "" {
				varEntity[norm(a.OutputVariable)] = db.EntityQualifiedName
			}
		}
	}
	return varEntity
}

// buildReferences extracts cross-references from all documents.
// This is only run in full mode as it requires parsing all documents.
func (b *Builder) buildReferences() error {
	if !b.fullMode {
		return nil
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID := b.catalog.projectID
	snapshotID := b.snapshot.ID
	refCount := 0

	// emitActionRefs walks the action activities of a microflow or nanoflow and
	// records the document reference each action makes. Nanoflows share the same
	// action types as microflows, so both flow kinds use the same extraction.
	emitActionRefs := func(sourceType, sourceID string, containerID model.ID, name string, params []*microflows.MicroflowParameter, returnType microflows.DataType, oc *microflows.MicroflowObjectCollection) {
		moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(containerID))
		sourceQN := moduleName + "." + name
		emit := func(targetType, targetName, refKind string) {
			if _, err := stmt.Exec(sourceType, sourceID, sourceQN,
				targetType, "", targetName,
				refKind, moduleName, projectID, snapshotID); err == nil {
				refCount++
			}
		}

		// Boundary entity refs: the flow operates on its object-typed parameters
		// and return type even when it never create/retrieves them.
		for _, p := range params {
			if qn := entityOfDataType(p.Type); qn != "" {
				emit("ENTITY", qn, RefKindParameter)
			}
		}
		if qn := entityOfDataType(returnType); qn != "" {
			emit("ENTITY", qn, RefKindReturn)
		}

		if oc == nil {
			return
		}
		acts := collectActionActivities(oc)
		// Intra-flow variable→entity map so change/delete (which operate on a
		// variable, not a named entity) can resolve their target.
		varEntity := buildVarEntityMap(params, acts)
		for _, act := range acts {
			if tt, tn, rk, ok := microflowActionRef(act.Action); ok {
				emit(tt, tn, rk)
			}
			if tt, tn, rk, ok := microflowVarActionRef(act.Action, varEntity); ok {
				emit(tt, tn, rk)
			}
		}
	}

	// Extract microflow references (using cached list — no re-parsing)
	mfs, err := b.cachedMicroflows()
	if err != nil {
		return err
	}
	for _, mf := range mfs {
		emitActionRefs("MICROFLOW", string(mf.ID), mf.ContainerID, mf.Name, mf.Parameters, mf.ReturnType, mf.ObjectCollection)
	}

	// Extract nanoflow references — nanoflows also call microflows/nanoflows,
	// show pages, retrieve, etc.
	nfs, err := b.cachedNanoflows()
	if err == nil {
		for _, nf := range nfs {
			emitActionRefs("NANOFLOW", string(nf.ID), nf.ContainerID, nf.Name, nf.Parameters, nf.ReturnType, nf.ObjectCollection)
		}
	}

	// Extract entity references (generalization) — using cached list
	dms, err := b.cachedDomainModels()
	if err == nil {
		for _, dm := range dms {
			moduleID := b.hierarchy.findModuleID(dm.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)

			for _, ent := range dm.Entities {
				sourceQN := moduleName + "." + ent.Name
				// Check generalization
				if ent.GeneralizationRef != "" {
					_, err = stmt.Exec("ENTITY", string(ent.ID), sourceQN,
						"ENTITY", "", ent.GeneralizationRef,
						RefKindGeneralize, moduleName, projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
				// Calculated-by: an attribute whose value is computed by a microflow.
				for _, attr := range ent.Attributes {
					if attr.Value != nil && attr.Value.MicroflowName != "" {
						_, err = stmt.Exec("ENTITY", string(ent.ID), sourceQN,
							"MICROFLOW", "", attr.Value.MicroflowName,
							RefKindCalculate, moduleName, projectID, snapshotID)
						if err == nil {
							refCount++
						}
					}
				}
			}

			// Association references: an association connects a FROM entity
			// (ParentID) and a TO entity (ChildID). Emit an `associate` ref to
			// each endpoint so the association is discoverable from either entity
			// via `show references of Entity` and impact analysis sees both sides.
			for _, assoc := range dm.Associations {
				assocQN := moduleName + "." + assoc.Name
				for _, target := range []string{b.resolveEntityID(assoc.ParentID), b.resolveEntityID(assoc.ChildID)} {
					if target == "" {
						continue
					}
					if _, err := stmt.Exec("ASSOCIATION", string(assoc.ID), assocQN,
						"ENTITY", "", target,
						RefKindAssociate, moduleName, projectID, snapshotID); err == nil {
						refCount++
					}
				}
			}
			// Cross-module associations: ChildRef is already a qualified entity
			// name (the TO entity lives in another module); ParentID is local.
			for _, ca := range dm.CrossAssociations {
				assocQN := moduleName + "." + ca.Name
				for _, target := range []string{b.resolveEntityID(ca.ParentID), ca.ChildRef} {
					if target == "" {
						continue
					}
					if _, err := stmt.Exec("ASSOCIATION", string(ca.ID), assocQN,
						"ENTITY", "", target,
						RefKindAssociate, moduleName, projectID, snapshotID); err == nil {
						refCount++
					}
				}
			}
		}
	}

	// Extract page references (layout, datasources, parameters) — using cached list
	pageList, err := b.cachedPages()
	if err == nil {
		for _, pg := range pageList {
			moduleID := b.hierarchy.findModuleID(pg.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			sourceQN := moduleName + "." + pg.Name

			// Layout reference. ListPages() does not populate pg.LayoutCall (only
			// pg.LayoutID), so read the layout name the pages table already
			// resolved from raw BSON (extractLayoutRef) rather than the always-nil
			// LayoutCall. NOTE: widget-level datasource/action refs still require a
			// parsed widget tree, which no reader currently exposes — tracked as the
			// remaining part of #663 gap 3.
			if layoutRef := b.resolvePageLayoutRef(pg.ID); layoutRef != "" {
				_, err = stmt.Exec("PAGE", string(pg.ID), sourceQN,
					"LAYOUT", "", layoutRef,
					RefKindLayout, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}

			// Page parameter entity types
			for _, param := range pg.Parameters {
				if param.EntityName != "" {
					_, err = stmt.Exec("PAGE", string(pg.ID), sourceQN,
						"ENTITY", "", param.EntityName,
						RefKindParameter, moduleName, projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
			}
		}

		// Widget references: page → entity (datasource) and page → microflow /
		// nanoflow (action). Projected from the widgets table, which buildPages
		// already populated from each page's raw BSON (so no extra parse here).
		// DISTINCT collapses the many widgets on a page that target the same
		// document into a single edge.
		widgetProjections := []struct{ col, targetType, refKind string }{
			{"EntityRef", "ENTITY", RefKindDatasource},
			{"MicroflowRef", "MICROFLOW", RefKindAction},
			{"NanoflowRef", "NANOFLOW", RefKindAction},
		}
		for _, p := range widgetProjections {
			res, perr := b.tx.Exec(
				`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
				 SELECT DISTINCT ContainerType, '', ContainerQualifiedName, ?, '', `+p.col+`, ?, ModuleName, ?, ?
				 FROM widgets_data WHERE `+p.col+` != ''`,
				p.targetType, p.refKind, projectID, snapshotID)
			if perr == nil {
				if n, e := res.RowsAffected(); e == nil {
					refCount += int(n)
				}
			}
		}
	}

	// Extract navigation references (home pages, menu items, login pages)
	nav, err := b.reader.GetNavigation()
	if err == nil {
		for _, profile := range nav.Profiles {
			sourceName := "Navigation." + profile.Name

			// Default home page
			if profile.HomePage != nil {
				if profile.HomePage.Page != "" {
					_, err = stmt.Exec("NAVIGATION", "", sourceName,
						"PAGE", "", profile.HomePage.Page,
						RefKindHomePage, "", projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
				if profile.HomePage.Microflow != "" {
					_, err = stmt.Exec("NAVIGATION", "", sourceName,
						"MICROFLOW", "", profile.HomePage.Microflow,
						RefKindHomePage, "", projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
			}

			// Role-based home pages
			for _, rh := range profile.RoleBasedHomePages {
				if rh.Page != "" {
					_, err = stmt.Exec("NAVIGATION", "", sourceName,
						"PAGE", "", rh.Page,
						RefKindHomePage, "", projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
				if rh.Microflow != "" {
					_, err = stmt.Exec("NAVIGATION", "", sourceName,
						"MICROFLOW", "", rh.Microflow,
						RefKindHomePage, "", projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
			}

			// Login page
			if profile.LoginPage != "" {
				_, err = stmt.Exec("NAVIGATION", "", sourceName,
					"PAGE", "", profile.LoginPage,
					RefKindLoginPage, "", projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}

			// Menu items (recursive)
			refCount += b.extractMenuItemRefs(stmt, profile.MenuItems, sourceName, projectID, snapshotID)
		}
	}

	// Extract workflow references — using cached list
	wfs, wfErr := b.cachedWorkflows()
	if wfErr == nil {
		for _, wf := range wfs {
			moduleID := b.hierarchy.findModuleID(wf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			sourceQN := moduleName + "." + wf.Name

			// Parameter entity reference
			if wf.Parameter != nil && wf.Parameter.EntityRef != "" {
				_, err = stmt.Exec("WORKFLOW", string(wf.ID), sourceQN,
					"ENTITY", "", wf.Parameter.EntityRef,
					RefKindParameter, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}

			// Overview page reference
			if wf.OverviewPage != "" {
				_, err = stmt.Exec("WORKFLOW", string(wf.ID), sourceQN,
					"PAGE", "", wf.OverviewPage,
					RefKindShowPage, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}

			// Extract references from workflow activities
			if wf.Flow != nil {
				refCount += b.extractWorkflowFlowRefs(stmt, wf.Flow, string(wf.ID), sourceQN, moduleName, projectID, snapshotID)
			}
		}
	}

	b.report("References", refCount)
	return nil
}

// extractMenuItemRefs extracts page and microflow references from menu items recursively.
func (b *Builder) extractMenuItemRefs(stmt *sql.Stmt, items []*types.NavMenuItem, sourceName, projectID, snapshotID string) int {
	refCount := 0
	for _, item := range items {
		if item.Page != "" {
			_, err := stmt.Exec("NAVIGATION", "", sourceName,
				"PAGE", "", item.Page,
				RefKindMenuItem, "", projectID, snapshotID)
			if err == nil {
				refCount++
			}
		}
		if item.Microflow != "" {
			_, err := stmt.Exec("NAVIGATION", "", sourceName,
				"MICROFLOW", "", item.Microflow,
				RefKindMenuItem, "", projectID, snapshotID)
			if err == nil {
				refCount++
			}
		}
		if len(item.Items) > 0 {
			refCount += b.extractMenuItemRefs(stmt, item.Items, sourceName, projectID, snapshotID)
		}
	}
	return refCount
}

// extractWidgetRefs extracts entity and microflow references from a widget and its children.
func (b *Builder) extractWidgetRefs(stmt *sql.Stmt, w pages.Widget, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID string) int {
	if w == nil {
		return 0
	}

	refCount := 0

	// Extract datasource refs based on widget type
	switch widget := w.(type) {
	case *pages.DataView:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		// Recurse into children
		for _, child := range widget.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
		for _, child := range widget.FooterWidgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.ListView:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		for _, child := range widget.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.DataGrid:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		for _, child := range widget.ControlBarWidgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.TemplateGrid:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		for _, child := range widget.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
		for _, child := range widget.ControlBarWidgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.Gallery:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		if widget.ContentWidget != nil {
			refCount += b.extractWidgetRefs(stmt, widget.ContentWidget, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
		for _, child := range widget.FilterWidgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.Container:
		for _, child := range widget.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.LayoutGrid:
		for _, row := range widget.Rows {
			for _, col := range row.Columns {
				for _, child := range col.Widgets {
					refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
				}
			}
		}

	case *pages.CustomWidget:
		// Pluggable widget - extract refs from WidgetObject properties
		if widget.WidgetObject != nil {
			refCount += b.extractWidgetObjectRefs(stmt, widget.WidgetObject, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
	}

	return refCount
}

// extractWidgetObjectRefs extracts refs from a pluggable widget's WidgetObject.
func (b *Builder) extractWidgetObjectRefs(stmt *sql.Stmt, obj *pages.WidgetObject, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID string) int {
	if obj == nil {
		return 0
	}

	refCount := 0

	for _, prop := range obj.Properties {
		if prop.Value == nil {
			continue
		}
		val := prop.Value

		// Extract datasource refs
		if val.DataSource != nil {
			refCount += b.extractDataSourceRefs(stmt, val.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

		// Extract entity ref
		if val.EntityRef != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				"ENTITY", "", val.EntityRef,
				RefKindDatasource, moduleName, projectID, snapshotID)
			refCount++
		}

		// Extract microflow ref
		if val.Microflow != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				"MICROFLOW", "", val.Microflow,
				RefKindAction, moduleName, projectID, snapshotID)
			refCount++
		}

		// Extract nanoflow ref
		if val.Nanoflow != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				"NANOFLOW", "", val.Nanoflow,
				RefKindAction, moduleName, projectID, snapshotID)
			refCount++
		}

		// Extract form (page) ref
		if val.Form != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				"PAGE", "", val.Form,
				RefKindShowPage, moduleName, projectID, snapshotID)
			refCount++
		}

		// Recurse into nested widgets
		for _, child := range val.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

		// Recurse into nested objects
		for _, childObj := range val.Objects {
			refCount += b.extractWidgetObjectRefs(stmt, childObj, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
	}

	return refCount
}

// extractDataSourceRefs extracts entity and microflow references from a datasource.
func (b *Builder) extractDataSourceRefs(stmt *sql.Stmt, ds pages.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID string) int {
	if ds == nil {
		return 0
	}

	refCount := 0

	switch src := ds.(type) {
	case *pages.DatabaseSource:
		// Resolve entity ID to qualified name
		if src.EntityID != "" {
			entityQN := b.resolveEntityID(src.EntityID)
			if entityQN != "" {
				stmt.Exec(sourceType, sourceID, sourceQN,
					"ENTITY", string(src.EntityID), entityQN,
					RefKindDatasource, moduleName, projectID, snapshotID)
				refCount++
			}
		}

	case *pages.DataViewSource:
		// Has either EntityID or EntityName
		entityQN := ""
		if src.EntityName != "" {
			entityQN = src.EntityName
		} else if src.EntityID != "" {
			entityQN = b.resolveEntityID(src.EntityID)
		}
		if entityQN != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				"ENTITY", "", entityQN,
				RefKindDatasource, moduleName, projectID, snapshotID)
			refCount++
		}

	case *pages.EntityPathSource:
		// Parse entity path to get root entity (e.g., "Customer/Orders" -> get entity for Customer)
		if src.EntityPath != "" {
			// Entity path starts with entity name or is an association path
			// For now, we'll extract what we can - the first segment might be the entity
			parts := strings.Split(src.EntityPath, "/")
			if len(parts) > 0 && parts[0] != "" {
				// This might be a qualified name or just entity name
				// We store it as-is for now
				stmt.Exec(sourceType, sourceID, sourceQN,
					"ENTITY", "", src.EntityPath,
					RefKindDatasource, moduleName, projectID, snapshotID)
				refCount++
			}
		}

	case *pages.AssociationSource:
		// Similar to EntityPathSource
		if src.EntityPath != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				"ENTITY", "", src.EntityPath,
				RefKindDatasource, moduleName, projectID, snapshotID)
			refCount++
		}

	case *pages.MicroflowSource:
		// Resolve microflow ID to qualified name
		if src.MicroflowID != "" {
			mfQN := b.resolveMicroflowID(src.MicroflowID)
			if mfQN != "" {
				stmt.Exec(sourceType, sourceID, sourceQN,
					"MICROFLOW", string(src.MicroflowID), mfQN,
					RefKindDatasource, moduleName, projectID, snapshotID)
				refCount++
			}
		}

	case *pages.NanoflowSource:
		// Resolve nanoflow ID to qualified name
		if src.NanoflowID != "" {
			nfQN := b.resolveMicroflowID(src.NanoflowID) // Uses same table
			if nfQN != "" {
				stmt.Exec(sourceType, sourceID, sourceQN,
					"NANOFLOW", string(src.NanoflowID), nfQN,
					RefKindDatasource, moduleName, projectID, snapshotID)
				refCount++
			}
		}
	}

	return refCount
}

// resolveEntityID looks up the qualified name for an entity ID.
func (b *Builder) resolveEntityID(entityID model.ID) string {
	if entityID == "" {
		return ""
	}
	var qualifiedName string
	err := b.tx.QueryRow("SELECT QualifiedName FROM entities WHERE Id = ?", string(entityID)).Scan(&qualifiedName)
	if err != nil {
		return ""
	}
	return qualifiedName
}

// resolvePageLayoutRef returns the layout qualified name the pages table already
// resolved for a page (from raw-BSON FormCall via extractLayoutRef), or "" if the
// page has none. buildReferences runs after buildPages in full mode, so the value
// is available; this avoids re-parsing the page BSON and reuses the pages table's
// resolution instead of the always-nil pg.LayoutCall.
func (b *Builder) resolvePageLayoutRef(pageID model.ID) string {
	var layoutRef string
	err := b.tx.QueryRow("SELECT LayoutRef FROM pages WHERE Id = ?", string(pageID)).Scan(&layoutRef)
	if err != nil {
		return ""
	}
	return layoutRef
}

// resolveMicroflowID looks up the qualified name for a microflow/nanoflow ID.
func (b *Builder) resolveMicroflowID(mfID model.ID) string {
	if mfID == "" {
		return ""
	}
	var qualifiedName string
	err := b.tx.QueryRow("SELECT QualifiedName FROM microflows WHERE Id = ?", string(mfID)).Scan(&qualifiedName)
	if err != nil {
		return ""
	}
	return qualifiedName
}

// extractWorkflowFlowRefs extracts references from a workflow flow and its nested sub-flows.
func (b *Builder) extractWorkflowFlowRefs(stmt *sql.Stmt, flow *workflows.Flow, sourceID, sourceQN, moduleName, projectID, snapshotID string) int {
	if flow == nil {
		return 0
	}

	refCount := 0
	for _, act := range flow.Activities {
		switch a := act.(type) {
		case *workflows.UserTask:
			if a.Page != "" {
				_, err := stmt.Exec("WORKFLOW", sourceID, sourceQN,
					"PAGE", "", a.Page,
					RefKindShowPage, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}
			if a.UserTaskEntity != "" {
				_, err := stmt.Exec("WORKFLOW", sourceID, sourceQN,
					"ENTITY", "", a.UserTaskEntity,
					RefKindDatasource, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}
			if a.UserSource != nil {
				if us, ok := a.UserSource.(*workflows.MicroflowBasedUserSource); ok && us.Microflow != "" {
					_, err := stmt.Exec("WORKFLOW", sourceID, sourceQN,
						"MICROFLOW", "", us.Microflow,
						RefKindCall, moduleName, projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
			}
			for _, outcome := range a.Outcomes {
				refCount += b.extractWorkflowFlowRefs(stmt, outcome.Flow, sourceID, sourceQN, moduleName, projectID, snapshotID)
			}

		case *workflows.CallMicroflowTask:
			if a.Microflow != "" {
				_, err := stmt.Exec("WORKFLOW", sourceID, sourceQN,
					"MICROFLOW", "", a.Microflow,
					RefKindCall, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}
			for _, outcome := range a.Outcomes {
				refCount += b.extractWorkflowConditionOutcomeRefs(stmt, outcome, sourceID, sourceQN, moduleName, projectID, snapshotID)
			}

		case *workflows.SystemTask:
			if a.Microflow != "" {
				_, err := stmt.Exec("WORKFLOW", sourceID, sourceQN,
					"MICROFLOW", "", a.Microflow,
					RefKindCall, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}
			for _, outcome := range a.Outcomes {
				refCount += b.extractWorkflowConditionOutcomeRefs(stmt, outcome, sourceID, sourceQN, moduleName, projectID, snapshotID)
			}

		case *workflows.CallWorkflowActivity:
			if a.Workflow != "" {
				_, err := stmt.Exec("WORKFLOW", sourceID, sourceQN,
					"WORKFLOW", "", a.Workflow,
					RefKindCall, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}

		case *workflows.ExclusiveSplitActivity:
			for _, outcome := range a.Outcomes {
				refCount += b.extractWorkflowConditionOutcomeRefs(stmt, outcome, sourceID, sourceQN, moduleName, projectID, snapshotID)
			}

		case *workflows.ParallelSplitActivity:
			for _, outcome := range a.Outcomes {
				refCount += b.extractWorkflowFlowRefs(stmt, outcome.Flow, sourceID, sourceQN, moduleName, projectID, snapshotID)
			}
		}
	}

	return refCount
}

// extractWorkflowConditionOutcomeRefs extracts references from a condition outcome's flow.
func (b *Builder) extractWorkflowConditionOutcomeRefs(stmt *sql.Stmt, outcome workflows.ConditionOutcome, sourceID, sourceQN, moduleName, projectID, snapshotID string) int {
	if outcome == nil {
		return 0
	}
	return b.extractWorkflowFlowRefs(stmt, outcome.GetFlow(), sourceID, sourceQN, moduleName, projectID, snapshotID)
}
