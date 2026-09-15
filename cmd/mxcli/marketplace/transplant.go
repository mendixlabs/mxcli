// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"fmt"

	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
)

// projectContainment is the containment name a module sits under on the project
// unit. Measured on a real project: every Projects$ModuleImpl hangs off the
// single Projects$Project unit under "Modules".
const projectContainment = "Modules"

// TransplantModule copies a module and everything under it from one project into
// another, using mxcli's own writer.
//
// This is the import step of a module update, and it deliberately does not use
// `mx module-import`. That command rewrites an MPR v2 project as v1 — measured,
// and refused outright by `marketplace install` — and it also refuses theme
// modules. Copying the units directly avoids both, and keeps the destination in
// whatever format it already uses, because the writer handles v1 and v2 alike.
//
// Units are copied verbatim, including their unit IDs. That is sound because the
// destination's copy of the module has been removed first, so the IDs are free,
// and because no element `$ID` pointer crosses a unit boundary (§4). Only the
// module unit is re-parented, onto the destination's project unit.
//
// It does NOT preserve identities: the copied module carries the *package's*
// GUIDs. Pair it with CaptureIdentities before and ApplyIdentities after, or the
// update destroys the module's data on the next deploy (§8).
func TransplantModule(srcMpr, dstMpr, moduleName string) (copied int, err error) {
	src, err := modelsdk.Open(srcMpr)
	if err != nil {
		return 0, fmt.Errorf("open source %s: %w", srcMpr, err)
	}
	defer src.Disconnect()

	srcUnits, err := src.ListUnits()
	if err != nil {
		return 0, fmt.Errorf("list source units: %w", err)
	}
	moduleUnits, err := unitsOfModule(src, srcUnits, moduleName)
	if err != nil {
		return 0, err
	}

	byID := make(map[string]*unitCopy, len(moduleUnits))
	order := make([]string, 0, len(moduleUnits))
	for _, u := range srcUnits {
		id := string(u.ID)
		if !containsStr(moduleUnits, id) {
			continue
		}
		raw, rerr := src.GetRawUnitBytes(model.ID(id))
		if rerr != nil || len(raw) == 0 {
			return 0, fmt.Errorf("read source unit %s: %w", id, rerr)
		}
		byID[id] = &unitCopy{
			id:          id,
			containerID: string(u.ContainerID),
			containment: u.ContainmentName,
			unitType:    u.Type,
			contents:    raw,
		}
		order = append(order, id)
	}

	moduleUnitID, err := moduleUnitIDOf(src, moduleName)
	if err != nil {
		return 0, err
	}

	dstProjectID, err := projectUnitID(dstMpr)
	if err != nil {
		return 0, err
	}
	if err := refuseIfPresent(dstMpr, moduleName); err != nil {
		return 0, err
	}

	writer, err := modelsdk.OpenForWriting(dstMpr)
	if err != nil {
		return 0, fmt.Errorf("open destination %s for writing: %w", dstMpr, err)
	}
	defer writer.Disconnect()

	for _, id := range order {
		u := byID[id]
		container := u.containerID
		containment := u.containment
		if id == moduleUnitID {
			// The only re-parenting: the module attaches to the destination's
			// project unit. Everything below it keeps its existing container,
			// which is a unit being copied in the same pass.
			container = dstProjectID
			containment = projectContainment
		}
		if err := writer.AddRawUnit(id, container, containment, u.unitType, u.contents); err != nil {
			return copied, fmt.Errorf("copy unit %s: %w", id, err)
		}
		copied++
	}
	return copied, nil
}

type unitCopy struct {
	id, containerID, containment, unitType string
	contents                               []byte
}

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func moduleUnitIDOf(reader backend.FullBackend, moduleName string) (string, error) {
	mods, err := reader.ListModules()
	if err != nil {
		return "", err
	}
	for _, m := range mods {
		if equalFold(m.Name, moduleName) {
			return string(m.ID), nil
		}
	}
	return "", fmt.Errorf("module %q not found in the source project", moduleName)
}

// projectUnitID finds the destination's Projects$Project unit — the container
// every module hangs off.
func projectUnitID(mprPath string) (string, error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Disconnect()

	units, err := reader.ListUnits()
	if err != nil {
		return "", err
	}
	for _, u := range units {
		if u.Type == "Projects$Project" {
			return string(u.ID), nil
		}
	}
	return "", fmt.Errorf("%s has no project unit to attach a module to", mprPath)
}

// refuseIfPresent stops a transplant onto a module that is still there. Copying
// on top would leave two modules of the same name, which is a corrupt model
// rather than an update — the destination's copy must be removed first.
func refuseIfPresent(mprPath, moduleName string) error {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Disconnect()

	mods, err := reader.ListModules()
	if err != nil {
		return err
	}
	for _, m := range mods {
		if equalFold(m.Name, moduleName) {
			return fmt.Errorf("module %q is still present in %s; remove it before transplanting, "+
				"or the project ends up with two modules of the same name", moduleName, mprPath)
		}
	}
	return nil
}

func equalFold(a, b string) bool { return strings.EqualFold(a, b) }
