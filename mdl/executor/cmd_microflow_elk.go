// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// elkSourceRange maps a diagram node to a line range in the MDL source.
type elkSourceRange struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

// microflowELKData is the JSON output schema for the microflow ELK diagram.
type microflowELKData struct {
	Format     string                    `json:"format"`
	Type       string                    `json:"type"`
	Name       string                    `json:"name"`
	Parameters []microflowELKParam       `json:"parameters"`
	ReturnType string                    `json:"returnType"`
	Nodes      []microflowELKNode        `json:"nodes"`
	Edges      []microflowELKEdge        `json:"edges"`
	MdlSource  string                    `json:"mdlSource,omitempty"`
	SourceMap  map[string]elkSourceRange `json:"sourceMap,omitempty"`
}

type microflowELKParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type microflowELKNode struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Category string   `json:"category"`
	Label    string   `json:"label"`
	Details  []string `json:"details,omitempty"`
	Width    float64  `json:"width"`
	Height   float64  `json:"height"`
	// Compound node fields (for loop bodies)
	Children []microflowELKNode `json:"children,omitempty"`
	Edges    []microflowELKEdge `json:"edges,omitempty"`
}

type microflowELKEdge struct {
	ID             string `json:"id"`
	SourceID       string `json:"sourceId"`
	TargetID       string `json:"targetId"`
	Label          string `json:"label,omitempty"`
	IsErrorHandler bool   `json:"isErrorHandler,omitempty"`
}

// microflowELK generates a JSON graph of a microflow for rendering with ELK.js.
func microflowELK(ctx *ExecContext, name string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return mdlerrors.NewValidationf("expected qualified name Module.Microflow, got: %s", name)
	}

	qn := ast.QualifiedName{Module: parts[0], Name: parts[1]}

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
		if modName == qn.Module && mf.Name == qn.Name {
			targetMf = mf
			break
		}
	}

	if targetMf == nil {
		return mdlerrors.NewNotFound("microflow", name)
	}

	// Generate MDL source with source map (best-effort — diagram works without it)
	mdlSource, sourceMap, _ := describeMicroflowToString(ctx, qn)

	return buildFlowELK(ctx, flowELKInput{
		FlowType:         "microflow",
		QualifiedName:    name,
		ReturnType:       targetMf.ReturnType,
		Parameters:       targetMf.Parameters,
		ObjectCollection: targetMf.ObjectCollection,
		EntityNames:      entityNames,
		MdlSource:        mdlSource,
		SourceMap:        sourceMap,
	})
}

// flowELKInput collects the parameters for buildFlowELK.
type flowELKInput struct {
	FlowType         string
	QualifiedName    string
	ReturnType       microflows.DataType
	Parameters       []*microflows.MicroflowParameter
	ObjectCollection *microflows.MicroflowObjectCollection
	EntityNames      map[model.ID]string
	MdlSource        string
	SourceMap        map[string]elkSourceRange
}

// buildFlowELK generates ELK JSON for any flow type (microflow or nanoflow).
func buildFlowELK(ctx *ExecContext, in flowELKInput) error {
	rt := ""
	if in.ReturnType != nil {
		rt = in.ReturnType.GetTypeName()
	}

	data := microflowELKData{
		Format:     "elk",
		Type:       in.FlowType,
		Name:       in.QualifiedName,
		ReturnType: rt,
		MdlSource:  in.MdlSource,
		SourceMap:  in.SourceMap,
	}

	// Parameters
	for _, p := range in.Parameters {
		paramType := ""
		if p.Type != nil {
			paramType = p.Type.GetTypeName()
		}
		data.Parameters = append(data.Parameters, microflowELKParam{
			Name: p.Name,
			Type: paramType,
		})
	}

	// Handle empty flow
	if in.ObjectCollection == nil || len(in.ObjectCollection.Objects) == 0 {
		data.Nodes = []microflowELKNode{
			{ID: "node-start", Type: "start", Category: "event", Label: "Start", Width: 80, Height: 36},
			{ID: "node-end", Type: "end", Category: "event", Label: "End", Width: 70, Height: 36},
		}
		data.Edges = []microflowELKEdge{
			{ID: "edge-0", SourceID: "node-start", TargetID: "node-end"},
		}
		return emitMicroflowELK(ctx, data)
	}

	// Build nodes — loops become compound nodes with children
	for _, obj := range in.ObjectCollection.Objects {
		node := buildMicroflowELKNodeHierarchical(obj, in.EntityNames, 0)
		data.Nodes = append(data.Nodes, node)
	}

	// Build edges — only top-level flows
	for i, flow := range in.ObjectCollection.Flows {
		edge := buildMicroflowELKEdge(flow, i, "edge")
		data.Edges = append(data.Edges, edge)
	}

	return emitMicroflowELK(ctx, data)
}

func buildMicroflowELKNode(obj microflows.MicroflowObject, entityNames map[model.ID]string) microflowELKNode {
	id := "node-" + string(obj.GetID())
	label := mermaidActivityLabel(obj, entityNames)
	// Un-escape Mermaid-specific escaping
	label = strings.ReplaceAll(label, "#quot;", "\"")

	details := mermaidActivityDetails(obj, entityNames)
	// Un-escape details too
	for i, d := range details {
		details[i] = strings.ReplaceAll(d, "#quot;", "\"")
	}

	nodeType, category := classifyMicroflowNode(obj)
	width, height := calcMicroflowNodeSize(nodeType, label, details)

	return microflowELKNode{
		ID:       id,
		Type:     nodeType,
		Category: category,
		Label:    label,
		Details:  details,
		Width:    width,
		Height:   height,
	}
}

// buildMicroflowELKNodeHierarchical builds an ELK node, handling LoopedActivity
// as a compound node with children (loop body objects) and inner edges.
func buildMicroflowELKNodeHierarchical(obj microflows.MicroflowObject, entityNames map[model.ID]string, depth int) microflowELKNode {
	loop, isLoop := obj.(*microflows.LoopedActivity)
	if !isLoop || loop.ObjectCollection == nil || len(loop.ObjectCollection.Objects) == 0 {
		return buildMicroflowELKNode(obj, entityNames)
	}

	// Build compound loop node
	id := "node-" + string(loop.GetID())
	label := mermaidActivityLabel(obj, entityNames)
	label = strings.ReplaceAll(label, "#quot;", "\"")
	details := mermaidActivityDetails(obj, entityNames)
	for i, d := range details {
		details[i] = strings.ReplaceAll(d, "#quot;", "\"")
	}

	node := microflowELKNode{
		ID:       id,
		Type:     "loop",
		Category: "loop",
		Label:    label,
		Details:  details,
		// Width/Height: 0 — ELK computes from children + padding
	}

	// Add children (recursively handle nested loops)
	for _, childObj := range loop.ObjectCollection.Objects {
		child := buildMicroflowELKNodeHierarchical(childObj, entityNames, depth+1)
		node.Children = append(node.Children, child)
	}

	// Add inner edges
	for i, flow := range loop.ObjectCollection.Flows {
		edge := buildMicroflowELKEdge(flow, i, id+"-edge")
		node.Edges = append(node.Edges, edge)
	}

	return node
}

// buildMicroflowELKEdge builds an ELK edge from a sequence flow.
func buildMicroflowELKEdge(flow *microflows.SequenceFlow, index int, prefix string) microflowELKEdge {
	edge := microflowELKEdge{
		ID:       fmt.Sprintf("%s-%d", prefix, index),
		SourceID: "node-" + string(flow.OriginID),
		TargetID: "node-" + string(flow.DestinationID),
	}

	label := mermaidCaseLabel(flow.CaseValue)
	if label != "" {
		label = strings.ReplaceAll(label, "#quot;", "\"")
	}
	edge.Label = label
	edge.IsErrorHandler = flow.IsErrorHandler

	return edge
}

// classifyMicroflowNode returns the node type and category for visual rendering.
func classifyMicroflowNode(obj microflows.MicroflowObject) (nodeType, category string) {
	switch a := obj.(type) {
	case *microflows.StartEvent:
		return "start", "event"
	case *microflows.EndEvent:
		return "end", "event"
	case *microflows.ContinueEvent:
		return "continue", "event"
	case *microflows.BreakEvent:
		return "break", "event"
	case *microflows.ErrorEvent:
		return "error", "event"
	case *microflows.ExclusiveSplit:
		return "split", "controlflow"
	case *microflows.InheritanceSplit:
		return "split", "controlflow"
	case *microflows.ExclusiveMerge:
		return "merge", "controlflow"
	case *microflows.LoopedActivity:
		return "loop", "loop"
	case *microflows.ActionActivity:
		return "action", classifyAction(a)
	default:
		return "action", "variable"
	}
}

// classifyAction returns a category string for coloring action activities.
func classifyAction(a *microflows.ActionActivity) string {
	if a.Action == nil {
		return "variable"
	}

	switch a.Action.(type) {
	case *microflows.CreateObjectAction, *microflows.ChangeObjectAction,
		*microflows.CommitObjectsAction, *microflows.DeleteObjectAction,
		*microflows.RollbackObjectAction:
		return "object"
	case *microflows.RetrieveAction:
		return "retrieve"
	case *microflows.MicroflowCallAction, *microflows.JavaActionCallAction,
		*microflows.CallExternalAction, *microflows.RestCallAction:
		return "call"
	case *microflows.ShowPageAction, *microflows.ClosePageAction,
		*microflows.ShowMessageAction:
		return "navigation"
	case *microflows.CreateVariableAction, *microflows.ChangeVariableAction,
		*microflows.AggregateListAction, *microflows.ListOperationAction,
		*microflows.CastAction:
		return "variable"
	case *microflows.ValidationFeedbackAction:
		return "validation"
	case *microflows.LogMessageAction:
		return "log"
	default:
		return "variable"
	}
}

// calcMicroflowNodeSize returns width and height for a microflow node.
func calcMicroflowNodeSize(nodeType, label string, details []string) (float64, float64) {
	switch nodeType {
	case "start", "end", "continue", "break", "error":
		// Pill shape
		w := float64(len(label))*elkCharWidth + elkHPadding*2
		if w < 70 {
			w = 70
		}
		return w, 36

	case "split":
		// Diamond shape - needs extra space for diagonal
		w := float64(len(label))*elkCharWidth + elkHPadding*2
		if w < 100 {
			w = 100
		}
		return w, 60

	case "merge":
		// Small circle
		return 24, 24

	case "loop":
		// Double-bordered box with header + details
		maxLen := float64(len(label))
		for _, d := range details {
			if l := float64(len(d)); l > maxLen {
				maxLen = l
			}
		}
		w := maxLen*elkCharWidth + elkHPadding
		if w < elkMinWidth {
			w = elkMinWidth
		}
		h := elkHeaderHeight + float64(len(details))*16
		if len(details) == 0 {
			h = elkHeaderHeight + 16
		}
		return w, h

	default:
		// Action: rounded rect with header + detail body
		maxLen := float64(len(label))
		for _, d := range details {
			if l := float64(len(d)); l > maxLen {
				maxLen = l
			}
		}
		w := maxLen*elkCharWidth + elkHPadding
		if w < elkMinWidth {
			w = elkMinWidth
		}
		h := elkHeaderHeight
		if len(details) > 0 {
			h += float64(len(details)) * 16
		}
		// Minimum height
		if h < elkHeaderHeight+8 {
			h = elkHeaderHeight + 8
		}
		return math.Ceil(w), math.Ceil(h)
	}
}

// collectAllObjectsAndFlows recursively collects all objects and flows from a
// MicroflowObjectCollection, including nested LoopedActivity bodies.
func collectAllObjectsAndFlows(oc *microflows.MicroflowObjectCollection) ([]microflows.MicroflowObject, []*microflows.SequenceFlow) {
	if oc == nil {
		return nil, nil
	}

	var objects []microflows.MicroflowObject
	var flows []*microflows.SequenceFlow

	objects = append(objects, oc.Objects...)
	flows = append(flows, oc.Flows...)

	// Recurse into nested LoopedActivity bodies
	for _, obj := range oc.Objects {
		if loop, ok := obj.(*microflows.LoopedActivity); ok && loop.ObjectCollection != nil {
			nestedObjs, nestedFlows := collectAllObjectsAndFlows(loop.ObjectCollection)
			objects = append(objects, nestedObjs...)
			flows = append(flows, nestedFlows...)
		}
	}

	return objects, flows
}

// emitMicroflowELK marshals and writes the microflow ELK data to output.
func emitMicroflowELK(ctx *ExecContext, data microflowELKData) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mdlerrors.NewBackend("marshal json", err)
	}
	fmt.Fprint(ctx.Output, string(out))
	return nil
}

// MicroflowELK is an Executor method wrapper for callers in unmigrated code.
func (e *Executor) MicroflowELK(name string) error {
	return microflowELK(e.newExecContext(context.Background()), name)
}
