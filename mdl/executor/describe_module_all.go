// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// moduleDoc is one top-level document a module-wide describe emits.
type moduleDoc struct {
	id, containerID model.ID
	name            string
}

// moduleDocKind is one document type `describe module M with all` emits: the
// DESCRIBE kind that renders a document of it, and how to list them.
type moduleDocKind struct {
	kind ast.DescribeObjectType
	list func(ctx *ExecContext) ([]moduleDoc, error)
}

// docs adapts a backend list result to moduleDocs.
func docs[T any](items []T, err error, f func(T) moduleDoc) ([]moduleDoc, error) {
	if err != nil {
		return nil, err
	}
	out := make([]moduleDoc, 0, len(items))
	for _, it := range items {
		out = append(out, f(it))
	}
	return out, nil
}

// moduleDocKindsBeforeDomainModel and moduleDocKindsAfterDomainModel are the
// one registry of document types `describe module M with all` emits, in the
// order it emits them. The domain model's entities and associations are emitted
// between the two lists (they are members of one unit, not documents, and are
// ordered by generalization).
//
// It replaced a hand-written section per type that covered 13 of them, so a
// module's nanoflows, JavaScript actions, JSON structures, mappings, image
// collections, workflows, REST services, task queues and more were silently
// absent from the dump (mendixlabs/mxcli#1421). Two guards keep it whole:
// TestModuleDocKindsCoverDescribableTypes holds it against the describe
// auto-detection table, and moduleDescribeRun.reportUnemitted names every document unit
// in the module that no entry emitted, so a type missing here is visible in the
// output rather than gone.
//
// The order is dependency-first, so the dump re-executes top to bottom with as
// few forward references as the model allows: what is referenced (enumerations,
// constants, entities, structures, connections, queues, actions) before what
// references it (mappings, flows, pages, services that publish them). Flows and
// pages reference each other, so no order is cycle-free; microflows stay before
// pages as they always were.
var moduleDocKindsBeforeDomainModel = []moduleDocKind{
	{ast.DescribeEnumeration, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListEnumerations()
		return docs(l, err, func(x *model.Enumeration) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeConstant, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListConstants()
		return docs(l, err, func(x *model.Constant) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	// A consumed OData service precedes the domain model: its external
	// entities name it, and it names only constants (its URL).
	{ast.DescribeODataClient, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListConsumedODataServices()
		return docs(l, err, func(x *model.ConsumedODataService) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
}

var moduleDocKindsAfterDomainModel = []moduleDocKind{
	// Leaves: referenced by attributes, widgets and flows, reference nothing.
	{ast.DescribeRegularExpression, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListRegularExpressions()
		return docs(l, err, func(x *model.RegularExpression) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeImageCollection, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListImageCollections()
		return docs(l, err, func(x *types.ImageCollection) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeIconCollection, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListIconCollections()
		return docs(l, err, func(x *types.IconCollection) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},

	// Structures, then the mappings built on them and on the entities.
	{ast.DescribeJsonStructure, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListJsonStructures()
		return docs(l, err, func(x *types.JsonStructure) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeMessageDefinitionCollection, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListMessageDefinitionCollections()
		return docs(l, err, func(x *model.MessageDefinitionCollection) moduleDoc {
			return moduleDoc{x.ID, x.ContainerID, x.Name}
		})
	}},
	{ast.DescribeMessageDefinition, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListMessageDefinitionDocuments()
		return docs(l, err, func(x *model.MessageDefinitionDocument) moduleDoc {
			return moduleDoc{x.ID, x.ContainerID, x.Name}
		})
	}},
	{ast.DescribeImportMapping, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListImportMappings()
		return docs(l, err, func(x *model.ImportMapping) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeExportMapping, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListExportMappings()
		return docs(l, err, func(x *model.ExportMapping) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},

	// Connections and consumed services: what flows call.
	{ast.DescribeDatabaseConnection, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListDatabaseConnections()
		return docs(l, err, func(x *model.DatabaseConnection) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeRestClient, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListConsumedRestServices()
		return docs(l, err, func(x *model.ConsumedRestService) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeBusinessEventService, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListBusinessEventServices()
		return docs(l, err, func(x *model.BusinessEventService) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeDataTransformer, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListDataTransformers()
		return docs(l, err, func(x *model.DataTransformer) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	// A task queue is resolved eagerly by a microflow's run-in-queue setting
	// (MDL-ORDER01), so it precedes the flows.
	{ast.DescribeQueue, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListQueues()
		return docs(l, err, func(x *types.Queue) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},

	// Actions, then the flows that call them. Rules precede microflows, whose
	// decisions call them.
	{ast.DescribeJavaAction, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListJavaActions()
		return docs(l, err, func(x *types.JavaAction) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeJavaScriptAction, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListJavaScriptActions()
		return docs(l, err, func(x *types.JavaScriptAction) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeRule, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListRules()
		return docs(l, err, func(x *microflows.Rule) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeMicroflow, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListMicroflows()
		return docs(l, err, func(x *microflows.Microflow) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeNanoflow, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListNanoflows()
		return docs(l, err, func(x *microflows.Nanoflow) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},

	// Page structure: layouts, then snippets and building blocks, then pages
	// that place them, then the menus that link to pages.
	{ast.DescribeLayout, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListLayouts()
		return docs(l, err, func(x *pages.Layout) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeSnippet, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListSnippets()
		return docs(l, err, func(x *pages.Snippet) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeBuildingBlock, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListBuildingBlocks()
		return docs(l, err, func(x *pages.BuildingBlock) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribePage, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListPages()
		return docs(l, err, func(x *pages.Page) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeMenu, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListMenuDocuments()
		return docs(l, err, func(x *types.MenuDocument) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},

	// What references flows and pages: workflows, published services, schedules.
	{ast.DescribeWorkflow, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListWorkflows()
		return docs(l, err, func(x *workflows.Workflow) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeODataService, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListPublishedODataServices()
		return docs(l, err, func(x *model.PublishedODataService) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribePublishedRestService, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListPublishedRestServices()
		return docs(l, err, func(x *model.PublishedRestService) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeScheduledEvent, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListScheduledEvents()
		return docs(l, err, func(x *model.ScheduledEvent) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},

	// Agent-editor documents: Model / KnowledgeBase / ConsumedMCPService are
	// leaves, an Agent references them through its TOOL / KNOWLEDGE BASE / MCP
	// SERVICE blocks.
	{ast.DescribeModel, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListAgentEditorModels()
		return docs(l, err, func(x *agenteditor.Model) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeKnowledgeBase, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListAgentEditorKnowledgeBases()
		return docs(l, err, func(x *agenteditor.KnowledgeBase) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeConsumedMCPService, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListAgentEditorConsumedMCPServices()
		return docs(l, err, func(x *agenteditor.ConsumedMCPService) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribePublishedMCPService, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListAgentEditorPublishedMCPServices()
		return docs(l, err, func(x *agenteditor.PublishedMCPService) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
	{ast.DescribeAgent, func(ctx *ExecContext) ([]moduleDoc, error) {
		l, err := ctx.Backend.ListAgentEditorAgents()
		return docs(l, err, func(x *agenteditor.Agent) moduleDoc { return moduleDoc{x.ID, x.ContainerID, x.Name} })
	}},
}

// moduleDescribeRun accumulates what a module-wide describe emitted and what
// failed, so a failure is reported where it happened and fails the statement
// once everything else has been written.
type moduleDescribeRun struct {
	ctx        *ExecContext
	module     string
	containers map[model.ID]bool
	emitted    map[model.ID]bool
	failures   int
}

// fail reports one describe failure in the output, as a comment so the dump
// still parses, in place of the document it could not describe.
func (r *moduleDescribeRun) fail(what string, err error) {
	r.failures++
	msg := strings.ReplaceAll(strings.TrimSpace(err.Error()), "\n", " ")
	fmt.Fprintf(r.ctx.Output, "-- describe failed: %s: %s\n\n", what, msg)
}

// emitKinds describes every document of the given kinds that lives in the
// module, in registry order.
func (r *moduleDescribeRun) emitKinds(kinds []moduleDocKind) {
	for _, k := range kinds {
		label := describeObjectTypeLabel(k.kind)
		list, err := k.list(r.ctx)
		if err != nil {
			r.fail("listing "+label+" documents", err)
			continue
		}
		for _, d := range list {
			if !r.containers[d.containerID] {
				continue
			}
			r.emitted[d.id] = true
			qn := ast.QualifiedName{Module: r.module, Name: d.name}
			if err := r.describe(k.kind, qn); err != nil {
				r.fail(label+" "+qn.String(), err)
				continue
			}
			fmt.Fprintln(r.ctx.Output)
		}
	}
}

// readOnlyDescribeKinds are the document types whose describe is a report
// (a widget tree, an icon table) rather than a statement MDL can execute —
// building blocks and icon collections cannot be created from MDL. In a module
// dump their report is emitted commented out, so the document is accounted for
// and the dump still parses as a script.
var readOnlyDescribeKinds = map[ast.DescribeObjectType]bool{
	ast.DescribeBuildingBlock:  true,
	ast.DescribeIconCollection: true,
}

// describe writes one document's describe output, commented out when it is a
// read-only report.
func (r *moduleDescribeRun) describe(kind ast.DescribeObjectType, qn ast.QualifiedName) error {
	stmt := &ast.DescribeStmt{ObjectType: kind, Name: qn}
	if !readOnlyDescribeKinds[kind] {
		return describeByKind(r.ctx, stmt)
	}
	out := r.ctx.Output
	var buf bytes.Buffer
	r.ctx.Output = &buf
	err := describeByKind(r.ctx, stmt)
	r.ctx.Output = out
	if err != nil {
		return err
	}
	fmt.Fprint(out, commentOutReport(buf.String()))
	return nil
}

// commentOutReport prefixes every line of a report that is not already a
// comment with "-- ".
func commentOutReport(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, ln := range lines {
		if !strings.HasPrefix(ln, "--") {
			lines[i] = "-- " + ln
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// describedWithOwner are document unit types that are not described on their
// own because another document's describe already carries them.
var describedWithOwner = map[string]bool{
	// A view entity's OQL source; `create view entity` prints and writes it.
	"DomainModels$ViewEntitySourceDocument": true,
}

// reportUnemitted names every document unit in the module that no registry
// entry emitted. The enumeration is type-blind, so a document type the registry
// does not know about (a page template, which has no describe handler, or a
// type added to Mendix later) is listed instead of silently absent.
func (r *moduleDescribeRun) reportUnemitted() {
	units, err := r.ctx.Backend.ListDocumentUnits()
	if err != nil {
		r.fail("listing the module's documents", err)
		return
	}
	for _, u := range units {
		if !r.containers[u.ContainerID] || r.emitted[u.ID] || describedWithOwner[u.Type] {
			continue
		}
		fmt.Fprintf(r.ctx.Output, "-- not describable: %s %s.%s\n", u.Kind, r.module, u.Name)
	}
}

// err is the statement's result: a describe failure fails it, after the rest of
// the module has been written (as the output line limit does, #1425).
func (r *moduleDescribeRun) err() error {
	if r.failures == 0 {
		return nil
	}
	return mdlerrors.NewValidationf("describe module %s with all: %d document(s) or document types could not be described; "+
		"see the '-- describe failed:' lines in the output", r.module, r.failures)
}
