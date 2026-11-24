// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// Entity change data capture, read side (part A2 of
// docs/11-proposals/PROPOSAL_kafka_event_integration.md). There is no CREATE
// yet: part C writes the two-sided document, and its baseline identifies what it
// tracks by GUID — the identity the runtime keys the database on — so the write
// path is deliberately not a by-product of the read one.

// cdcTopic is the Kafka topic a source publishes to. Studio Pro's editor shows
// it beside the revision, and every segment but the last is a model value: the
// AppName, the source's ExposedName and the *major* version. So a major bump, an
// AppName change or a rename starts a NEW topic — downstream, a new table with
// no history — which is why describe prints it rather than leaving the reader to
// assemble it.
func cdcTopic(appName, exposedName string, major int) string {
	if appName == "" || exposedName == "" {
		return ""
	}
	return fmt.Sprintf("cdc.%s.%s.%d.{space}", appName, exposedName, major)
}

// sourceVersion is the version the baseline froze for a live source, found
// through the source's pointer into the baseline, or by exposed name when the
// pointer is absent. mxbuild does not validate those pointers, so a document
// with none still has to describe.
func sourceVersion(svc *model.ChangeDataCaptureService, src *model.ChangeSource) *model.ChangeVersion {
	if svc.Baseline == nil {
		return nil
	}
	for _, b := range svc.Baseline.Sources {
		if src.VersionedSourceID != "" && b.ID == src.VersionedSourceID {
			return b.Version
		}
	}
	for _, b := range svc.Baseline.Sources {
		if b.ExposedName == src.ExposedName {
			return b.Version
		}
	}
	return nil
}

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
	fmt.Fprintln(out, "-- There is no CREATE statement for this document yet (part C of")
	fmt.Fprintln(out, "-- PROPOSAL_kafka_event_integration.md), so this is a report, not a script.")

	for _, src := range svc.Sources {
		v := sourceVersion(svc, src)
		fmt.Fprintln(out)
		fmt.Fprintf(out, "-- entity %s", src.Entity)
		if src.ExposedName != "" && src.ExposedName != cdcLastSegment(src.Entity) {
			fmt.Fprintf(out, " as '%s'", src.ExposedName)
		}
		fmt.Fprintln(out)
		if v != nil {
			fmt.Fprintf(out, "--   revision %s   topic %s\n", v.String(), cdcTopic(svc.AppName, src.ExposedName, v.Major))
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
