// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE MICROFLOW command
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// isBuiltinModuleEntity returns true for modules whose entities are defined
// internally by the Mendix runtime and are therefore not present in the MPR's
// domain models. These types are serialized using the qualified name reference
// ("System.Workflow", "System.User", etc.) and resolved at runtime.
func isBuiltinModuleEntity(moduleName string) bool {
	return moduleName == "System"
}

// systemEnumerations holds the qualified name of every enumeration the System
// module declares. Like its entities they are not stored in the project, so
// ListEnumerations never returns them and findEnumeration cannot see them.
var systemEnumerations = func() map[string]bool {
	set := make(map[string]bool, len(meta.SystemEnumerations))
	for _, e := range meta.SystemEnumerations {
		set[e.Name] = true
	}
	return set
}()

// isKnownEnumeration reports whether module.name is an enumeration — one of the
// project's, or one of the System module's.
func isKnownEnumeration(ctx *ExecContext, module, name string) bool {
	return systemEnumerations[module+"."+name] || findEnumeration(ctx, module, name) != nil
}

// settleSystemEnumeration retypes a bare System enumeration in a flow
// parameter or return type. The visitor types every bare qualified name there
// as an entity, and the entity-then-enumeration fallback the callers apply is
// skipped for the System module (its entities are not in the project to look
// up), so without this `$T: System.UserType` was written as an ObjectType with
// Entity "System.UserType" — accepted by the local engines, refused by Studio
// Pro ("Reference with qualified name System.UserType of type
// DomainModels$Entity not found").
func settleSystemEnumeration(dt ast.DataType) ast.DataType {
	if dt.Kind == ast.TypeEntity && dt.EntityRef != nil && systemEnumerations[dt.EntityRef.Module+"."+dt.EntityRef.Name] {
		return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: dt.EntityRef}
	}
	return dt
}

// execCreateMicroflow handles CREATE MICROFLOW statements.
// loadRestServices returns all consumed REST services, or nil if no backend.
func loadRestServices(ctx *ExecContext) ([]*model.ConsumedRestService, error) {
	if !ctx.Connected() {
		return nil, nil
	}
	svcs, err := ctx.Backend.ListConsumedRestServices()
	return svcs, err
}

func execCreateMicroflow(ctx *ExecContext, s *ast.CreateMicroflowStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// An existing flow is modified as a patch of what is stored, not rebuilt
	// (ADR-0012 decision 3); see cmd_flow_modify.go.
	if s.CreateOrModify && strings.TrimSpace(s.Name.Name) != "" {
		if err := validateMicroflowRules(s); err != nil {
			return err
		}
		// The splice path does not reach the rebuild's queue guard, and a
		// call restated without its stored retry is an edit it would make.
		if stored := findMicroflowByQualifiedName(ctx, s.Name.String()); stored != nil {
			if err := checkNoDroppedQueueRetries(ctx, stored.ID, s.Name.String(), s, nil); err != nil {
				return err
			}
		}
		if handled, err := modifyFlowInPlace(ctx, microflowDecl(s)); handled || err != nil {
			return err
		}
	}

	built, err := buildMicroflowFromStmt(ctx, s, buildFlowOpts{AllowCreate: true})
	if err != nil {
		return err
	}
	mf := built.Microflow
	containerID := built.ContainerID
	existingID := built.ExistingID
	existingContainerID := built.ExistingContainerID

	// Create or update the microflow
	if existingID != "" {
		if err := ctx.Backend.UpdateMicroflow(mf); err != nil {
			return mdlerrors.NewBackend("update microflow", err)
		}
		if _, err := applyDocumentFolder(ctx, mf.ID, existingContainerID, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Replaced", "microflow: %s.%s", s.Name.Module, s.Name.Name)
	} else {
		if err := ctx.Backend.CreateMicroflow(mf); err != nil {
			return mdlerrors.NewBackend("create microflow", err)
		}
		fmt.Fprintf(ctx.Output, "Created microflow: %s.%s\n", s.Name.Module, s.Name.Name)
	}

	// Track the created microflow so it can be resolved by subsequent page creations
	returnEntityName := extractEntityFromReturnType(mf.ReturnType)
	ctx.trackCreatedMicroflow(s.Name.Module, s.Name.Name, mf.ID, containerID, returnEntityName)

	// Invalidate hierarchy cache so the new microflow's container is visible
	invalidateHierarchy(ctx)
	return nil
}

// storedStartPosition reads the StartEvent position off the microflow being
// replaced, when a person put it there rather than mxcli's own layout — see
// authoredStartPosition for how the two are told apart, and why carrying the
// position over unconditionally pinned the start of every rewritten flow (#951).
//
// Nil for a fresh CREATE, and nil for a start sitting where the layout would
// have put it anyway; both derive from the new first activity. Best-effort: a
// backend that cannot read the flow yields the derived placement rather than
// failing the statement.
func storedStartPosition(ctx *ExecContext, existingID model.ID) *model.Point {
	if existingID == "" || ctx.Backend == nil {
		return nil
	}
	mf, err := ctx.Backend.GetMicroflow(existingID)
	if err != nil || mf == nil {
		return nil
	}
	return authoredStartPosition(mf.ObjectCollection)
}
