// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// describeMermaid generates a Mermaid diagram for the given object type and name.
// Supported types: entity (renders full domain model), microflow, page.
func describeMermaid(ctx *ExecContext, objectType, name string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	parts := strings.SplitN(name, ".", 2)
	var qn ast.QualifiedName
	if len(parts) == 2 {
		qn = ast.QualifiedName{Module: parts[0], Name: parts[1]}
	} else {
		qn = ast.QualifiedName{Module: name}
	}

	switch strings.ToLower(objectType) {
	case "entity", "domainmodel":
		return domainModelToMermaid(ctx, qn.Module)
	case "microflow":
		return microflowToMermaid(ctx, qn)
	case "page":
		return pageToMermaid(ctx, qn)
	case "nanoflow":
		return nanoflowToMermaid(ctx, qn)
	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("mermaid format not supported for type: %s", objectType))
	}
}

// DescribeMermaid is a method wrapper for external callers.
func (e *Executor) DescribeMermaid(objectType, name string) error {
	return describeMermaid(e.newExecContext(context.Background()), objectType, name)
}

// domainModelToMermaid generates a Mermaid erDiagram for a module's domain model.
func domainModelToMermaid(ctx *ExecContext, moduleName string) error {
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Build entity ID-to-name map for this module
	entityNames := make(map[model.ID]string)
	for _, entity := range dm.Entities {
		entityNames[entity.ID] = entity.Name
	}

	// Also load entities from all modules for cross-module associations
	allEntityNames := make(map[model.ID]string)
	h, err := getHierarchy(ctx)
	if err == nil {
		domainModels, _ := ctx.Backend.ListDomainModels()
		for _, otherDM := range domainModels {
			modName := h.GetModuleName(otherDM.ContainerID)
			for _, entity := range otherDM.Entities {
				allEntityNames[entity.ID] = modName + "." + entity.Name
			}
		}
	}

	// Classify entities by type for coloring
	type entityInfo struct {
		label    string
		category string // "persistent", "nonpersistent", "external", "view"
	}
	entityInfos := make([]entityInfo, 0, len(dm.Entities))
	for _, entity := range dm.Entities {
		label := sanitizeMermaidID(entity.Name)
		cat := "persistent"
		if strings.Contains(entity.Source, "OqlView") {
			cat = "view"
		} else if strings.Contains(entity.Source, "OData") || entity.RemoteSource != "" || entity.RemoteSourceDocument != "" {
			cat = "external"
		} else if !entity.Persistable {
			cat = "nonpersistent"
		}
		entityInfos = append(entityInfos, entityInfo{label: label, category: cat})
	}

	var sb strings.Builder
	sb.WriteString("erDiagram\n")

	// Emit entities with their attributes
	for i, entity := range dm.Entities {
		entityLabel := entityInfos[i].label
		sb.WriteString(fmt.Sprintf("    %s {\n", entityLabel))
		for _, attr := range entity.Attributes {
			typeName := attr.Type.GetTypeName()
			attrName := sanitizeMermaidID(attr.Name)
			sb.WriteString(fmt.Sprintf("        %s %s\n", typeName, attrName))
		}
		sb.WriteString("    }\n")
	}

	// Emit associations as relationships
	for _, assoc := range dm.Associations {
		parentName := entityNames[assoc.ParentID]
		childName := entityNames[assoc.ChildID]

		// For cross-module associations, use full qualified name
		if parentName == "" {
			if qn, ok := allEntityNames[assoc.ParentID]; ok {
				parentName = sanitizeMermaidID(qn)
			} else {
				parentName = "Unknown"
			}
		} else {
			parentName = sanitizeMermaidID(parentName)
		}
		if childName == "" {
			if qn, ok := allEntityNames[assoc.ChildID]; ok {
				childName = sanitizeMermaidID(qn)
			} else {
				childName = "Unknown"
			}
		} else {
			childName = sanitizeMermaidID(childName)
		}

		// Determine relationship cardinality
		// Parent (FROM) is the owner/many side, Child (TO) is the referenced/one side
		rel := "}o--||" // Reference: many-to-one (parent=*, child=1)
		if assoc.Type == domainmodel.AssociationTypeReferenceSet {
			rel = "}o--o{" // ReferenceSet: many-to-many
		}

		label := sanitizeMermaidLabel(assoc.Name)
		sb.WriteString(fmt.Sprintf("    %s %s %s : \"%s\"\n", parentName, rel, childName, label))
	}

	// Emit generalizations
	for _, entity := range dm.Entities {
		if entity.GeneralizationRef != "" {
			childName := sanitizeMermaidID(entity.Name)
			// GeneralizationRef is a qualified name like "System.User"
			parentName := sanitizeMermaidID(entity.GeneralizationRef)
			sb.WriteString(fmt.Sprintf("    %s ||--|{ %s : \"generalizes\"\n", parentName, childName))
		}
	}

	// Emit style classes for entity coloring
	// Mermaid erDiagram uses "style <entity> fill:<color>" syntax isn't supported,
	// but we can use the %%{init}%% block for theming. Instead, use CSS class-based
	// styling via the ":::className" syntax on entity definitions (Mermaid 11+).
	// Since erDiagram doesn't support per-node classes natively, we emit a
	// %%{init}%% block with custom theme variables and add comments for the webview
	// to apply colors via post-render DOM manipulation.
	sb.WriteString("\n")

	// Emit metadata for the webview
	sb.WriteString("%% @type erDiagram\n")

	// Emit a JSON metadata comment that the webview can parse for coloring
	sb.WriteString("%% @colors {")
	first := true
	for _, info := range entityInfos {
		if !first {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`"%s":"%s"`, info.label, info.category))
		first = false
	}
	sb.WriteString("}\n")

	fmt.Fprint(ctx.Output, sb.String())
	return nil
}

// microflowToMermaid generates a Mermaid flowchart for a microflow.
func microflowToMermaid(ctx *ExecContext, name ast.QualifiedName) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Build entity name lookup
	entityNames, err := buildEntityNames(ctx, h)
	if err != nil {
		return err
	}

	// Find the microflow
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	var targetMf *microflows.Microflow
	for _, mf := range allMicroflows {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && mf.Name == name.Name {
			targetMf = mf
			break
		}
	}

	if targetMf == nil {
		return mdlerrors.NewNotFound("microflow", name.String())
	}

	return renderFlowMermaid(ctx, targetMf.ObjectCollection, entityNames)
}

// nanoflowToMermaid renders a nanoflow as a Mermaid flowchart.
func nanoflowToMermaid(ctx *ExecContext, name ast.QualifiedName) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Build entity name lookup
	entityNames, err := buildEntityNames(ctx, h)
	if err != nil {
		return err
	}

	// Find the nanoflow
	allNanoflows, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("list nanoflows", err)
	}

	for _, nf := range allNanoflows {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && nf.Name == name.Name {
			return renderFlowMermaid(ctx, nf.ObjectCollection, entityNames)
		}
	}

	return mdlerrors.NewNotFound("nanoflow", name.String())
}

// buildEntityNames builds a map from entity ID to qualified name (Module.Entity)
// using the hierarchy for module name resolution.
func buildEntityNames(ctx *ExecContext, h *ContainerHierarchy) (map[model.ID]string, error) {
	entityNames := make(map[model.ID]string)
	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return nil, mdlerrors.NewBackend("list domain models", err)
	}
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}
	return entityNames, nil
}

// renderFlowMermaid renders a flow's object collection as a Mermaid flowchart.
func renderFlowMermaid(ctx *ExecContext, oc *microflows.MicroflowObjectCollection, entityNames map[model.ID]string) error {
	var sb strings.Builder
	sb.WriteString("flowchart LR\n")

	if oc == nil || len(oc.Objects) == 0 {
		sb.WriteString("    start([Start]) --> stop([End])\n")
		fmt.Fprint(ctx.Output, sb.String())
		return nil
	}

	// Collect all objects and flows recursively (including nested loop bodies)
	allObjects, allFlows := collectAllObjectsAndFlows(oc)

	// Build activity map and find start event
	activityMap := make(map[model.ID]microflows.MicroflowObject)
	var startID model.ID

	for _, obj := range allObjects {
		activityMap[obj.GetID()] = obj
		if _, ok := obj.(*microflows.StartEvent); ok {
			startID = obj.GetID()
		}
	}
	_ = activityMap // used for reference

	// Build flow graph
	flowsByOrigin := make(map[model.ID][]*microflows.SequenceFlow)
	for _, flow := range allFlows {
		flowsByOrigin[flow.OriginID] = append(flowsByOrigin[flow.OriginID], flow)
	}

	// Sort flows by OriginConnectionIndex
	for originID := range flowsByOrigin {
		flows := flowsByOrigin[originID]
		for i := 0; i < len(flows)-1; i++ {
			for j := i + 1; j < len(flows); j++ {
				if flows[i].OriginConnectionIndex > flows[j].OriginConnectionIndex {
					flows[i], flows[j] = flows[j], flows[i]
				}
			}
		}
	}

	// Collect node info for metadata (detail lines per node)
	nodeInfo := make(map[string][]string) // mermaid node ID -> detail lines

	// Emit node definitions
	for _, obj := range allObjects {
		id := mermaidShortID(obj.GetID())
		label := mermaidActivityLabel(obj, entityNames)

		switch obj.(type) {
		case *microflows.StartEvent:
			sb.WriteString(fmt.Sprintf("    %s([Start])\n", id))
		case *microflows.EndEvent:
			sb.WriteString(fmt.Sprintf("    %s([%s])\n", id, label))
		case *microflows.ExclusiveSplit:
			sb.WriteString(fmt.Sprintf("    %s{%s}\n", id, label))
		case *microflows.InheritanceSplit:
			sb.WriteString(fmt.Sprintf("    %s{%s}\n", id, label))
		case *microflows.ExclusiveMerge:
			// Merge nodes are pass-through; emit as small circle
			sb.WriteString(fmt.Sprintf("    %s(( ))\n", id))
		case *microflows.LoopedActivity:
			sb.WriteString(fmt.Sprintf("    %s[/%s/]\n", id, label))
		default:
			sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", id, label))
		}

		// Collect detail lines for this node
		if details := mermaidActivityDetails(obj, entityNames); len(details) > 0 {
			nodeInfo[id] = details
		}
	}

	// Emit edges
	visited := make(map[string]bool)
	for originID, flows := range flowsByOrigin {
		fromID := mermaidShortID(originID)
		for _, flow := range flows {
			toID := mermaidShortID(flow.DestinationID)
			edgeKey := fromID + "->" + toID
			if visited[edgeKey] {
				continue
			}
			visited[edgeKey] = true

			// Check if this is a split with a case value label
			label := mermaidCaseLabel(flow.CaseValue)
			if label != "" {
				sb.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", fromID, label, toID))
			} else {
				sb.WriteString(fmt.Sprintf("    %s --> %s\n", fromID, toID))
			}
		}
	}

	// Style the start node
	if startID != "" {
		sb.WriteString(fmt.Sprintf("    style %s fill:#4CAF50,color:#fff\n", mermaidShortID(startID)))
	}

	// Emit metadata for the webview
	sb.WriteString("\n%% @type flowchart\n")
	sb.WriteString("%% @direction LR\n")

	// Emit node detail metadata as JSON
	if len(nodeInfo) > 0 {
		sb.WriteString("%% @nodeinfo {")
		first := true
		for id, details := range nodeInfo {
			if !first {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf(`"%s":[`, id))
			for i, d := range details {
				if i > 0 {
					sb.WriteString(",")
				}
				// JSON-escape the detail string
				escaped := strings.ReplaceAll(d, `\`, `\\`)
				escaped = strings.ReplaceAll(escaped, `"`, `\"`)
				escaped = strings.ReplaceAll(escaped, "\n", `\n`)
				escaped = strings.ReplaceAll(escaped, "\r", `\r`)
				escaped = strings.ReplaceAll(escaped, "\t", `\t`)
				sb.WriteString(fmt.Sprintf(`"%s"`, escaped))
			}
			sb.WriteString("]")
			first = false
		}
		sb.WriteString("}\n")
	}

	fmt.Fprint(ctx.Output, sb.String())
	return nil
}

// pageToMermaid generates a Mermaid block diagram for a page's widget structure.
func pageToMermaid(ctx *ExecContext, name ast.QualifiedName) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	allPages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	var foundPage *pages.Page
	for _, p := range allPages {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && p.Name == name.Name {
			foundPage = p
			break
		}
	}

	if foundPage == nil {
		return mdlerrors.NewNotFound("page", name.String())
	}

	// Use raw widget data (same approach as describePage)
	rawWidgets := getPageWidgetsFromRaw(ctx, foundPage.ID)

	var sb strings.Builder
	sb.WriteString("block-beta\n")
	sb.WriteString("    columns 1\n")

	// Title block
	title := name.Module + "." + name.Name
	sb.WriteString(fmt.Sprintf("    page_title[\"%s\"]\n", sanitizeMermaidLabel(title)))

	// Render widget tree
	renderRawWidgetMermaid(ctx, &sb, rawWidgets, 1, 0)

	// Emit metadata for the webview
	sb.WriteString("\n%% @type block\n")

	fmt.Fprint(ctx.Output, sb.String())
	return nil
}

// renderRawWidgetMermaid recursively renders raw widgets in Mermaid block-beta syntax.
func renderRawWidgetMermaid(ctx *ExecContext, sb *strings.Builder, widgets []rawWidget, depth int, counter int) int {
	for _, w := range widgets {
		counter++
		id := fmt.Sprintf("w%d", counter)
		label := w.Type
		if w.Name != "" {
			label = fmt.Sprintf("%s: %s", w.Type, w.Name)
		}
		indent := strings.Repeat("    ", depth)

		if len(w.Children) > 0 {
			fmt.Fprintf(sb, "%s%s[\"%s\"]:\n", indent, id, sanitizeMermaidLabel(label))
			counter = renderRawWidgetMermaid(ctx, sb, w.Children, depth+1, counter)
		} else {
			fmt.Fprintf(sb, "%s%s[\"%s\"]\n", indent, id, sanitizeMermaidLabel(label))
		}
	}
	return counter
}

// mermaidActivityLabel returns a short label for a microflow activity node.
func mermaidActivityLabel(obj microflows.MicroflowObject, entityNames map[model.ID]string) string {
	switch a := obj.(type) {
	case *microflows.ActionActivity:
		return mermaidActionLabel(a, entityNames)
	case *microflows.ExclusiveSplit:
		if a.SplitCondition != nil {
			if ec, ok := a.SplitCondition.(*microflows.ExpressionSplitCondition); ok && ec.Expression != "" {
				expr := ec.Expression
				if len(expr) > 40 {
					expr = expr[:37] + "..."
				}
				return sanitizeMermaidLabel(expr)
			}
		}
		return "Split"
	case *microflows.InheritanceSplit:
		return "Inheritance Split"
	case *microflows.LoopedActivity:
		return "Loop"
	case *microflows.StartEvent:
		return "Start"
	case *microflows.EndEvent:
		if a.ReturnValue != "" {
			return "Return: " + sanitizeMermaidLabel(mermaidTruncate(a.ReturnValue, 30))
		}
		return "End"
	default:
		return "Activity"
	}
}

// mermaidActionLabel returns a label for an action activity.
func mermaidActionLabel(a *microflows.ActionActivity, entityNames map[model.ID]string) string {
	if a.Action == nil {
		return "Action"
	}

	switch act := a.Action.(type) {
	case *microflows.CreateObjectAction:
		entityName := mermaidResolveEntityName(act.EntityID, act.EntityQualifiedName, entityNames)
		return "Create " + sanitizeMermaidLabel(entityName)
	case *microflows.ChangeObjectAction:
		if act.ChangeVariable != "" {
			return "change $" + sanitizeMermaidLabel(act.ChangeVariable)
		}
		return "Change Object"
	case *microflows.CommitObjectsAction:
		if act.CommitVariable != "" {
			return "commit $" + sanitizeMermaidLabel(act.CommitVariable)
		}
		return "Commit"
	case *microflows.DeleteObjectAction:
		if act.DeleteVariable != "" {
			return "delete $" + sanitizeMermaidLabel(act.DeleteVariable)
		}
		return "Delete"
	case *microflows.RollbackObjectAction:
		if act.RollbackVariable != "" {
			return "rollback $" + sanitizeMermaidLabel(act.RollbackVariable)
		}
		return "Rollback"
	case *microflows.CreateVariableAction:
		if act.VariableName != "" {
			return "declare $" + sanitizeMermaidLabel(act.VariableName)
		}
		return "Declare Variable"
	case *microflows.ChangeVariableAction:
		if act.VariableName != "" {
			return "set $" + sanitizeMermaidLabel(act.VariableName)
		}
		return "Set Variable"
	case *microflows.RetrieveAction:
		if act.Source != nil {
			switch src := act.Source.(type) {
			case *microflows.DatabaseRetrieveSource:
				entityName := mermaidResolveEntityName(src.EntityID, src.EntityQualifiedName, entityNames)
				return "Retrieve " + sanitizeMermaidLabel(entityName)
			case *microflows.AssociationRetrieveSource:
				return "Retrieve by Association"
			}
		}
		return "Retrieve"
	case *microflows.MicroflowCallAction:
		if act.MicroflowCall != nil && act.MicroflowCall.Microflow != "" {
			return "Call " + sanitizeMermaidLabel(mermaidTruncate(act.MicroflowCall.Microflow, 30))
		}
		return "Call Microflow"
	case *microflows.NanoflowCallAction:
		if act.NanoflowCall != nil && act.NanoflowCall.Nanoflow != "" {
			return "Call " + sanitizeMermaidLabel(mermaidTruncate(act.NanoflowCall.Nanoflow, 30))
		}
		return "Call Nanoflow"
	case *microflows.JavaActionCallAction:
		if act.JavaAction != "" {
			return "Call " + sanitizeMermaidLabel(mermaidTruncate(act.JavaAction, 30))
		}
		return "Call Java Action"
	case *microflows.RestCallAction:
		return "rest Call"
	case *microflows.ExecuteDatabaseQueryAction:
		if act.Query != "" {
			return "DB Query " + sanitizeMermaidLabel(mermaidTruncate(act.Query, 30))
		}
		return "Execute DB Query"
	case *microflows.CallExternalAction:
		if act.Name != "" {
			return "Call External " + sanitizeMermaidLabel(mermaidTruncate(act.Name, 25))
		}
		return "Call External"
	case *microflows.ShowPageAction:
		if act.PageName != "" {
			return "Show " + sanitizeMermaidLabel(mermaidTruncate(act.PageName, 30))
		}
		return "Show Page"
	case *microflows.ClosePageAction:
		return "Close Page"
	case *microflows.ShowMessageAction:
		return "Show Message"
	case *microflows.ValidationFeedbackAction:
		return "Validation Feedback"
	case *microflows.LogMessageAction:
		return "Log Message"
	case *microflows.AggregateListAction:
		if act.OutputVariable != "" {
			return "Aggregate $" + sanitizeMermaidLabel(act.OutputVariable)
		}
		return "Aggregate List"
	case *microflows.ListOperationAction:
		return "List Operation"
	case *microflows.CastAction:
		return "Cast Object"
	default:
		return "Action"
	}
}

// mermaidActivityDetails returns detailed property lines for a microflow activity node.
// These lines are emitted as metadata for the webview to show on expand/click.
func mermaidActivityDetails(obj microflows.MicroflowObject, entityNames map[model.ID]string) []string {
	switch a := obj.(type) {
	case *microflows.ActionActivity:
		return mermaidActionDetails(a, entityNames)
	case *microflows.ExclusiveSplit:
		var lines []string
		if a.Caption != "" {
			lines = append(lines, "Caption: "+a.Caption)
		}
		if a.SplitCondition != nil {
			if ec, ok := a.SplitCondition.(*microflows.ExpressionSplitCondition); ok && ec.Expression != "" {
				lines = append(lines, "Condition: "+ec.Expression)
			}
		}
		return lines
	case *microflows.LoopedActivity:
		var lines []string
		switch ls := a.LoopSource.(type) {
		case *microflows.IterableList:
			if ls.ListVariableName != "" {
				lines = append(lines, "List: $"+ls.ListVariableName)
			}
			if ls.VariableName != "" {
				lines = append(lines, "Iterator: $"+ls.VariableName)
			}
		case *microflows.WhileLoopCondition:
			lines = append(lines, "While: "+ls.WhileExpression)
		}
		return lines
	case *microflows.EndEvent:
		if a.ReturnValue != "" {
			return []string{"Return: " + a.ReturnValue}
		}
	}
	return nil
}

// mermaidActionDetails returns detailed property lines for an action activity.
func mermaidActionDetails(a *microflows.ActionActivity, entityNames map[model.ID]string) []string {
	if a.Action == nil {
		return nil
	}

	var lines []string

	switch act := a.Action.(type) {
	case *microflows.CreateObjectAction:
		entityName := mermaidResolveEntityName(act.EntityID, act.EntityQualifiedName, entityNames)
		lines = append(lines, "Entity: "+entityName)
		if act.OutputVariable != "" {
			lines = append(lines, "Output: $"+act.OutputVariable)
		}
		if string(act.Commit) != "" && act.Commit != "No" {
			lines = append(lines, "Commit: "+string(act.Commit))
		}
		for _, mc := range act.InitialMembers {
			name := mermaidMemberName(mc)
			if name != "" && mc.Value != "" {
				lines = append(lines, name+" = "+mermaidTruncate(mc.Value, 50))
			}
		}

	case *microflows.ChangeObjectAction:
		if act.ChangeVariable != "" {
			lines = append(lines, "Variable: $"+act.ChangeVariable)
		}
		if string(act.Commit) != "" && act.Commit != "No" {
			lines = append(lines, "Commit: "+string(act.Commit))
		}
		for _, mc := range act.Changes {
			name := mermaidMemberName(mc)
			if name != "" && mc.Value != "" {
				lines = append(lines, name+" = "+mermaidTruncate(mc.Value, 50))
			}
		}

	case *microflows.CommitObjectsAction:
		if act.CommitVariable != "" {
			lines = append(lines, "Variable: $"+act.CommitVariable)
		}
		if act.WithEvents {
			lines = append(lines, "With events: true")
		}

	case *microflows.DeleteObjectAction:
		if act.DeleteVariable != "" {
			lines = append(lines, "Variable: $"+act.DeleteVariable)
		}

	case *microflows.RollbackObjectAction:
		if act.RollbackVariable != "" {
			lines = append(lines, "Variable: $"+act.RollbackVariable)
		}

	case *microflows.RetrieveAction:
		if act.OutputVariable != "" {
			lines = append(lines, "Output: $"+act.OutputVariable)
		}
		if act.Source != nil {
			switch src := act.Source.(type) {
			case *microflows.DatabaseRetrieveSource:
				entityName := mermaidResolveEntityName(src.EntityID, src.EntityQualifiedName, entityNames)
				lines = append(lines, "From: "+entityName)
				if src.XPathConstraint != "" {
					lines = append(lines, "Where: "+mermaidTruncate(src.XPathConstraint, 60))
				}
				if src.Range != nil && src.Range.RangeType != "" {
					rangeStr := string(src.Range.RangeType)
					if src.Range.Limit != "" {
						rangeStr += " limit=" + src.Range.Limit
					}
					if src.Range.Offset != "" {
						rangeStr += " offset=" + src.Range.Offset
					}
					lines = append(lines, "Range: "+rangeStr)
				}
				for _, s := range src.Sorting {
					if s.AttributeQualifiedName != "" {
						lines = append(lines, "Sort: "+s.AttributeQualifiedName+" "+string(s.Direction))
					}
				}
			case *microflows.AssociationRetrieveSource:
				if src.StartVariable != "" {
					lines = append(lines, "From: $"+src.StartVariable)
				}
				if src.AssociationQualifiedName != "" {
					lines = append(lines, "Via: "+src.AssociationQualifiedName)
				}
			}
		}

	case *microflows.MicroflowCallAction:
		if act.MicroflowCall != nil {
			if act.MicroflowCall.Microflow != "" {
				lines = append(lines, "Microflow: "+act.MicroflowCall.Microflow)
			}
			for _, pm := range act.MicroflowCall.ParameterMappings {
				paramName := pm.Parameter
				// Extract just the parameter name (last part of qualified name)
				if idx := strings.LastIndex(paramName, "."); idx >= 0 {
					paramName = paramName[idx+1:]
				}
				lines = append(lines, paramName+" = "+mermaidTruncate(pm.Argument, 50))
			}
		}
		if act.ResultVariableName != "" {
			lines = append(lines, "Result: $"+act.ResultVariableName)
		}

	case *microflows.NanoflowCallAction:
		if act.NanoflowCall != nil {
			if act.NanoflowCall.Nanoflow != "" {
				lines = append(lines, "Nanoflow: "+act.NanoflowCall.Nanoflow)
			}
			for _, pm := range act.NanoflowCall.ParameterMappings {
				paramName := pm.Parameter
				if idx := strings.LastIndex(paramName, "."); idx >= 0 {
					paramName = paramName[idx+1:]
				}
				lines = append(lines, paramName+" = "+mermaidTruncate(pm.Argument, 50))
			}
		}
		if act.OutputVariableName != "" {
			lines = append(lines, "Result: $"+act.OutputVariableName)
		}

	case *microflows.ShowPageAction:
		if act.PageName != "" {
			lines = append(lines, "Page: "+act.PageName)
		}
		if act.PageSettings != nil && act.PageSettings.Location != "" {
			lines = append(lines, "Location: "+string(act.PageSettings.Location))
		}
		for _, pm := range act.PageParameterMappings {
			paramName := pm.Parameter
			if idx := strings.LastIndex(paramName, "."); idx >= 0 {
				paramName = paramName[idx+1:]
			}
			lines = append(lines, paramName+" = "+mermaidTruncate(pm.Argument, 50))
		}

	case *microflows.ShowMessageAction:
		if act.Type != "" {
			lines = append(lines, "Type: "+string(act.Type))
		}
		if act.Template != nil {
			if msg := mermaidTextPreview(act.Template); msg != "" {
				lines = append(lines, "Message: "+mermaidTruncate(msg, 60))
			}
		}

	case *microflows.ValidationFeedbackAction:
		if act.ObjectVariable != "" {
			target := "$" + act.ObjectVariable
			if act.AttributeName != "" {
				// Extract attribute name (last part)
				attr := act.AttributeName
				if idx := strings.LastIndex(attr, "."); idx >= 0 {
					attr = attr[idx+1:]
				}
				target += "." + attr
			}
			lines = append(lines, "Target: "+target)
		}
		if act.Template != nil {
			if msg := mermaidTextPreview(act.Template); msg != "" {
				lines = append(lines, "Message: "+mermaidTruncate(msg, 60))
			}
		}

	case *microflows.LogMessageAction:
		if act.LogLevel != "" {
			lines = append(lines, "Level: "+string(act.LogLevel))
		}
		if act.LogNodeName != "" {
			lines = append(lines, "Node: "+act.LogNodeName)
		}
		if act.MessageTemplate != nil {
			if msg := mermaidTextPreview(act.MessageTemplate); msg != "" {
				lines = append(lines, "Message: "+mermaidTruncate(msg, 60))
			}
		}

	case *microflows.AggregateListAction:
		if act.InputVariable != "" {
			lines = append(lines, "List: $"+act.InputVariable)
		}
		if act.Function != "" {
			fn := string(act.Function)
			if act.AttributeQualifiedName != "" {
				fn += " on " + act.AttributeQualifiedName
			}
			lines = append(lines, "Function: "+fn)
		}
		if act.OutputVariable != "" {
			lines = append(lines, "Output: $"+act.OutputVariable)
		}

	case *microflows.CreateVariableAction:
		if act.VariableName != "" {
			lines = append(lines, "Variable: $"+act.VariableName)
		}
		if act.InitialValue != "" {
			lines = append(lines, "Value: "+mermaidTruncate(act.InitialValue, 60))
		}

	case *microflows.ChangeVariableAction:
		if act.VariableName != "" {
			lines = append(lines, "Variable: $"+act.VariableName)
		}
		if act.Value != "" {
			lines = append(lines, "Value: "+mermaidTruncate(act.Value, 60))
		}

	case *microflows.JavaActionCallAction:
		if act.JavaAction != "" {
			lines = append(lines, "Java Action: "+act.JavaAction)
		}
		if act.ResultVariableName != "" {
			lines = append(lines, "Result: $"+act.ResultVariableName)
		}

	case *microflows.RestCallAction:
		if act.HttpConfiguration != nil {
			if act.HttpConfiguration.HttpMethod != "" {
				method := string(act.HttpConfiguration.HttpMethod)
				url := act.HttpConfiguration.LocationTemplate
				if url == "" {
					url = act.HttpConfiguration.CustomLocation
				}
				if url != "" {
					lines = append(lines, method+" "+mermaidTruncate(url, 50))
				} else {
					lines = append(lines, "Method: "+method)
				}
			}
		}
		if act.OutputVariable != "" {
			lines = append(lines, "Output: $"+act.OutputVariable)
		}

	case *microflows.ExecuteDatabaseQueryAction:
		if act.Query != "" {
			lines = append(lines, "Query: "+act.Query)
		}
		if act.DynamicQuery != "" {
			lines = append(lines, "Dynamic: "+mermaidTruncate(act.DynamicQuery, 50))
		}
		if act.OutputVariableName != "" {
			lines = append(lines, "Output: $"+act.OutputVariableName)
		}

	case *microflows.CallExternalAction:
		if act.ConsumedODataService != "" {
			lines = append(lines, "Service: "+act.ConsumedODataService)
		}
		if act.Name != "" {
			lines = append(lines, "Action: "+act.Name)
		}
		if act.ResultVariableName != "" {
			lines = append(lines, "Result: $"+act.ResultVariableName)
		}

	case *microflows.ClosePageAction:
		if act.NumberOfPages > 0 {
			lines = append(lines, fmt.Sprintf("Pages: %d", act.NumberOfPages))
		}

	case *microflows.ListOperationAction:
		if act.OutputVariable != "" {
			lines = append(lines, "Output: $"+act.OutputVariable)
		}
	}

	return lines
}

// mermaidMemberName extracts a short member name from a MemberChange.
func mermaidMemberName(mc *microflows.MemberChange) string {
	name := mc.AttributeQualifiedName
	if name == "" {
		name = mc.AssociationQualifiedName
	}
	if name == "" {
		return ""
	}
	// Extract last part (attribute name) from "Module.Entity.Attribute"
	parts := strings.Split(name, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return name
}

// mermaidTextPreview extracts the first non-empty translation from a model.Text.
func mermaidTextPreview(t *model.Text) string {
	if t == nil {
		return ""
	}
	// Try English first, then any language
	if msg, ok := t.Translations["en_US"]; ok && msg != "" {
		return strings.TrimSpace(msg)
	}
	for _, msg := range t.Translations {
		if msg != "" {
			return strings.TrimSpace(msg)
		}
	}
	return ""
}

// mermaidCaseLabel extracts a display label from a CaseValue.
func mermaidCaseLabel(cv microflows.CaseValue) string {
	if cv == nil {
		return ""
	}
	switch c := cv.(type) {
	case *microflows.NoCase:
		return ""
	case *microflows.BooleanCase:
		if c.Value {
			return "true"
		}
		return "false"
	case *microflows.EnumerationCase:
		return sanitizeMermaidLabel(c.Value)
	case *microflows.ExpressionCase:
		return sanitizeMermaidLabel(mermaidTruncate(c.Expression, 20))
	default:
		return ""
	}
}

// mermaidResolveEntityName resolves an entity name from ID or qualified name.
func mermaidResolveEntityName(entityID model.ID, qualifiedName string, entityNames map[model.ID]string) string {
	if qualifiedName != "" {
		return qualifiedName
	}
	if name, ok := entityNames[entityID]; ok {
		return name
	}
	return "Entity"
}

// mermaidShortID generates a short, safe Mermaid node ID from a model.ID.
func mermaidShortID(id model.ID) string {
	s := string(id)
	// Use last 8 chars of UUID to keep it short but unique
	if len(s) > 8 {
		s = s[len(s)-8:]
	}
	// Replace hyphens with underscores for Mermaid compatibility
	s = strings.ReplaceAll(s, "-", "_")
	return "n_" + s
}

// sanitizeMermaidID replaces characters that are not safe in Mermaid identifiers.
func sanitizeMermaidID(s string) string {
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

// sanitizeMermaidLabel escapes characters in a Mermaid label string.
func sanitizeMermaidLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "#quot;")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// mermaidTruncate truncates a string to max length with "..." suffix.
func mermaidTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
