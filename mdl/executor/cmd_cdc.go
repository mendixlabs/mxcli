// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// Entity change data capture (parts A2 and C of
// docs/11-proposals/PROPOSAL_kafka_event_integration.md): LIST, DESCRIBE,
// CREATE [OR MODIFY] and DROP.
//
// Only the live side is authored. The baseline identifies what it tracks by the
// GUIDs the runtime keys the database on, so the backend reads every one from
// the domain model and computes the baseline from them — there is no GUID an
// author can write, and therefore none they can get wrong.

// The topic a source publishes to and the version the baseline froze for it
// are model.ChangeDataCaptureService.Topic and .SourceVersion: the catalog
// stores both (ako/mxcli#2384 A2), and a second copy of the formula here would
// be a copy that drifts.

func listChangeDataCaptureServices(ctx *ExecContext, inModule string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	services, err := ctx.Backend.ListChangeDataCaptureServices()
	if err != nil {
		return mdlerrors.NewBackend("list change data capture services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	type row struct {
		module, qualifiedName, service, app, version string
		sources, properties                          int
	}
	var rows []row
	var shown int

	for _, svc := range services {
		moduleName := h.GetModuleName(h.FindModuleID(svc.ContainerID))
		if inModule != "" && !strings.EqualFold(moduleName, inModule) {
			continue
		}
		shown++
		r := row{
			module:        moduleName,
			qualifiedName: moduleName + "." + svc.Name,
			service:       svc.ServiceName,
			app:           svc.AppName,
			sources:       len(svc.Sources),
		}
		for _, src := range svc.Sources {
			r.properties += len(src.Properties)
		}
		if svc.Baseline != nil {
			r.version = svc.Baseline.Version.String()
		}
		rows = append(rows, r)
	}

	if shown == 0 && ctx.Format != FormatJSON {
		if inModule != "" {
			fmt.Fprintf(ctx.Output, "No change data capture services found in module %s\n", inModule)
		} else {
			fmt.Fprintln(ctx.Output, "No change data capture services found")
		}
		return nil
	}

	result := &TableResult{
		// App is a column rather than a detail: it is the topic's second
		// segment, so two services with the same name and different AppNames
		// publish nowhere near each other.
		Columns: []string{"Module", "QualifiedName", "Service", "App", "Version", "Sources", "Properties"},
		Summary: fmt.Sprintf("(%d change data capture services)", shown),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.module, r.qualifiedName, r.service, r.app, r.version, r.sources, r.properties})
	}
	return writeResult(ctx, result)
}

func describeChangeDataCaptureService(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	services, err := ctx.Backend.ListChangeDataCaptureServices()
	if err != nil {
		return mdlerrors.NewBackend("list change data capture services", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	var svc *model.ChangeDataCaptureService
	var moduleName string
	var known []string
	for _, s := range services {
		mod := h.GetModuleName(h.FindModuleID(s.ContainerID))
		qn := mod + "." + s.Name
		known = append(known, qn)
		if strings.EqualFold(qn, name.String()) {
			svc, moduleName = s, mod
		}
	}
	if svc == nil {
		// Name the ones that exist: the likeliest slip is the module prefix, and
		// a bare "not found" sends the reader looking for a missing document.
		if len(known) == 0 {
			return mdlerrors.NewNotFoundMsg("change data capture service", name.String(),
				"this project has no change data capture services")
		}
		return mdlerrors.NewNotFoundMsg("change data capture service", name.String(),
			"this project has "+strings.Join(known, ", "))
	}

	out := ctx.Output
	// Not MDL: there is no CREATE statement for this document yet, so emitting
	// one would hand the reader a script that cannot run. Comments say what is
	// stored and stay honest about it — the same choice the consumed business
	// event describe made before its grammar landed.
	fmt.Fprintf(out, "-- change data capture service %s.%s\n", moduleName, svc.Name)
	fmt.Fprintf(out, "-- ServiceName: %s\n", svc.ServiceName)
	fmt.Fprintf(out, "-- AppName: %s (the topic's second segment; mxbuild reports CE1577 when empty)\n", svc.AppName)
	if svc.Baseline != nil && svc.Baseline.Version != nil {
		fmt.Fprintf(out, "-- Baseline version: %s\n", svc.Baseline.Version.String())
	} else {
		fmt.Fprintln(out, "-- Baseline: absent (mxbuild reports CE1585)")
	}
	if svc.Documentation != "" {
		fmt.Fprintf(out, "-- Documentation: %s\n", strings.ReplaceAll(svc.Documentation, "\n", " "))
	}
	fmt.Fprintln(out, "--")
	// A report, not a script: the authored form carries only the live side, so
	// re-emitting it as MDL would hide the versions, the topics and the GUIDs
	// that make this output worth reading. `create or modify` over an unchanged
	// service is elided anyway (ADR-0008), so a round-trippable describe would
	// buy nothing.
	fmt.Fprintln(out, "-- Author the live side with `create or modify change data capture service`;")
	fmt.Fprintln(out, "-- the baseline below is computed from the domain model, never authored.")

	for _, src := range svc.Sources {
		v := svc.SourceVersion(src)
		fmt.Fprintln(out)
		fmt.Fprintf(out, "-- entity %s", src.Entity)
		if src.ExposedName != "" && src.ExposedName != cdcLastSegment(src.Entity) {
			fmt.Fprintf(out, " as '%s'", src.ExposedName)
		}
		fmt.Fprintln(out)
		if v != nil {
			fmt.Fprintf(out, "--   revision %s   topic %s\n", v.String(), svc.Topic(src))
		}
		for _, p := range src.Properties {
			kind := "attribute"
			if p.IsAssociation() {
				kind = "association"
			}
			line := fmt.Sprintf("--   %-11s %s", kind, p.Member())
			if p.ExposedName != "" && p.ExposedName != cdcLastSegment(p.Member()) {
				line += fmt.Sprintf(" as '%s'", p.ExposedName)
			}
			if t := baselinePropertyType(svc, p); t != "" {
				line += fmt.Sprintf("   [%s]", t)
			}
			fmt.Fprintln(out, line)
		}
		if len(src.Properties) == 0 {
			fmt.Fprintln(out, "--   (no properties)")
		}
	}
	if len(svc.Sources) == 0 {
		fmt.Fprintln(out, "-- (no entities tracked)")
	}
	return nil
}

// baselinePropertyType is the CDC type the baseline froze for a live property,
// matched through its pointer and then by exposed name.
func baselinePropertyType(svc *model.ChangeDataCaptureService, p *model.ChangeProperty) string {
	if svc.Baseline == nil {
		return ""
	}
	for _, bs := range svc.Baseline.Sources {
		for _, bp := range bs.Properties {
			if p.VersionedPropertyID != "" && bp.ID == p.VersionedPropertyID {
				return bp.Type
			}
		}
	}
	for _, bs := range svc.Baseline.Sources {
		for _, bp := range bs.Properties {
			if bp.ExposedName == p.ExposedName {
				return bp.Type
			}
		}
	}
	return ""
}

// cdcLastSegment is the member or entity name without its module or entity prefix,
// which is what an exposed name defaults to.
func cdcLastSegment(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// createChangeDataCaptureService applies CREATE [OR MODIFY] CHANGE DATA CAPTURE
// SERVICE.
//
// Only the live side comes from the statement; the backend resolves every
// tracked member against the domain model and computes the baseline from it,
// because the baseline identifies what it tracks by the GUIDs the runtime keys
// the database on.
func createChangeDataCaptureService(ctx *ExecContext, stmt *ast.CreateChangeDataCaptureServiceStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	moduleName := stmt.Name.Module
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return mdlerrors.NewNotFound("module", moduleName)
	}

	existing, err := ctx.Backend.ListChangeDataCaptureServices()
	if err != nil {
		return mdlerrors.NewBackend("list change data capture services", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var stored *model.ChangeDataCaptureService
	if found, ok := pickLive(existing,
		func(s *model.ChangeDataCaptureService) bool {
			return strings.EqualFold(h.GetModuleName(h.FindModuleID(s.ContainerID)), moduleName) &&
				strings.EqualFold(s.Name, stmt.Name.Name)
		},
		func(s *model.ChangeDataCaptureService) bool { return s.Excluded },
	); ok {
		// `if not exists` is honoured centrally, in the dispatch
		// (skipExistingCreate), by embedding ast.CreateGuard.
		if !stmt.CreateOrModify {
			return mdlerrors.NewAlreadyExistsMsg("change data capture service",
				moduleName+"."+stmt.Name.Name,
				fmt.Sprintf("change data capture service already exists: %s.%s (use create or modify to update)",
					moduleName, stmt.Name.Name))
		}
		stored = found
	}

	containerID := module.ID
	if stmt.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, stmt.Folder)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("resolve folder '%s'", stmt.Folder), err)
		}
		containerID = folderID
	} else if stored != nil {
		// Placement is carried when the statement says nothing about folders.
		containerID = stored.ContainerID
	}

	svc := &model.ChangeDataCaptureService{
		ContainerID:   containerID,
		Name:          stmt.Name.Name,
		ServiceName:   stmt.ServiceName,
		Documentation: stmt.Documentation,
		ExportLevel:   "Hidden",
	}
	for _, src := range stmt.Sources {
		cs := &model.ChangeSource{
			Entity:      src.Entity.String(),
			ExposedName: src.ExposedName,
		}
		for _, mem := range src.Members {
			// Which kind it is comes from the domain model, so the member name
			// travels in Attribute and the backend moves it if it turns out to
			// be an association.
			cs.Properties = append(cs.Properties, &model.ChangeProperty{
				Attribute:   mem.Name,
				ExposedName: mem.ExposedName,
			})
		}
		svc.Sources = append(svc.Sources, cs)
	}

	if stored != nil {
		svc.ID = stored.ID
		svc.Excluded = stored.Excluded
		svc.AppName = stored.AppName
		svc.Documentation = carriedDocumentation(stmt.DocumentationSet, stmt.Documentation, stored.Documentation)
		if err := ctx.Backend.UpdateChangeDataCaptureService(svc); err != nil {
			return mdlerrors.NewBackend("update change data capture service", err)
		}
		ctx.ReportMutation("Modified", "change data capture service: %s.%s", moduleName, svc.Name)
		return nil
	}
	if err := ctx.Backend.CreateChangeDataCaptureService(svc); err != nil {
		return mdlerrors.NewBackend("create change data capture service", err)
	}
	fmt.Fprintf(ctx.Output, "Created change data capture service: %s.%s\n", moduleName, svc.Name)
	return nil
}

// dropChangeDataCaptureService applies DROP CHANGE DATA CAPTURE SERVICE.
func dropChangeDataCaptureService(ctx *ExecContext, stmt *ast.DropChangeDataCaptureServiceStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	services, err := ctx.Backend.ListChangeDataCaptureServices()
	if err != nil {
		return mdlerrors.NewBackend("list change data capture services", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}
	for _, s := range services {
		mod := h.GetModuleName(h.FindModuleID(s.ContainerID))
		if strings.EqualFold(mod, stmt.Name.Module) && strings.EqualFold(s.Name, stmt.Name.Name) {
			if err := ctx.Backend.DeleteChangeDataCaptureService(s.ID); err != nil {
				return mdlerrors.NewBackend("delete change data capture service", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped change data capture service: %s\n", stmt.Name.String())
			return nil
		}
	}
	if stmt.IfExists {
		fmt.Fprintf(ctx.Output, "Change data capture service %s does not exist — skipped\n", stmt.Name)
		return nil
	}
	return mdlerrors.NewNotFound("change data capture service", stmt.Name.String())
}
