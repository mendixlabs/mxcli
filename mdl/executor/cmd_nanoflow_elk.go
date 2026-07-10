// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// nanoflowELK generates a JSON graph of a nanoflow for rendering with ELK.js.
func nanoflowELK(ctx *ExecContext, name string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return mdlerrors.NewValidationf("expected qualified name Module.Nanoflow, got: %s", name)
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

	// Find the nanoflow
	allNanoflows, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("list nanoflows", err)
	}

	var targetNf *microflows.Nanoflow
	for _, nf := range allNanoflows {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == qn.Module && nf.Name == qn.Name {
			targetNf = nf
			break
		}
	}

	if targetNf == nil {
		return mdlerrors.NewNotFound("nanoflow", name)
	}

	// Generate MDL source with source map (best-effort — diagram works without it)
	mdlSource, sourceMap, _ := describeNanoflowToString(ctx, qn)

	return buildFlowELK(ctx, flowELKInput{
		FlowType:         "nanoflow",
		QualifiedName:    name,
		ReturnType:       targetNf.ReturnType,
		Parameters:       targetNf.Parameters,
		ObjectCollection: targetNf.ObjectCollection,
		EntityNames:      entityNames,
		MdlSource:        mdlSource,
		SourceMap:        sourceMap,
	})
}

// NanoflowELK is an Executor method wrapper.
func (e *Executor) NanoflowELK(name string) error {
	return nanoflowELK(e.newExecContext(context.Background()), name)
}
