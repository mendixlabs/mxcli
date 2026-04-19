// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/versions"
)

// checkFeature verifies that a feature is available in the connected project's
// version. Returns nil if available, or an actionable error with the version
// requirement and a hint. Safe to call when not connected (returns nil).
func checkFeature(ctx *ExecContext, area, name, statement, hint string) error {

	if !ctx.Connected() {
		return nil // No project connected; skip check
	}
	reg, err := versions.Load()
	if err != nil {
		return nil // Registry unavailable; don't block execution
	}
	rpv := ctx.Backend.ProjectVersion()
	pv := versions.SemVer{Major: rpv.MajorVersion, Minor: rpv.MinorVersion, Patch: rpv.PatchVersion}
	if reg.IsAvailable(area, name, pv) {
		return nil
	}

	// Find the min_version for the error message.
	features := reg.FeaturesForVersion(versions.SemVer{Major: 99, Minor: 0, Patch: 0})
	minV := "a newer version"
	for _, f := range features {
		if f.Area == area && f.Name == name {
			minV = "Mendix " + f.MinVersion.String() + "+"
			break
		}
	}

	msg := fmt.Sprintf("%s requires %s (project is %s)", statement, minV, rpv.ProductVersion)
	if hint != "" {
		msg += "\n  hint: " + hint
	}
	return mdlerrors.NewUnsupported(msg)
}

// execShowFeatures handles SHOW FEATURES, SHOW FEATURES FOR VERSION, and
// SHOW FEATURES ADDED SINCE commands.
func execShowFeatures(ctx *ExecContext, s *ast.ShowFeaturesStmt) error {

	reg, err := versions.Load()
	if err != nil {
		return mdlerrors.NewBackend("load version registry", err)
	}

	// Determine the project version to use.
	var pv versions.SemVer

	switch {
	case s.AddedSince != "":
		// SHOW FEATURES ADDED SINCE x.y
		sinceV, err := versions.ParseSemVer(s.AddedSince)
		if err != nil {
			return mdlerrors.NewValidationf("invalid version %q: %v", s.AddedSince, err)
		}
		return showFeaturesAddedSince(ctx, reg, sinceV)

	case s.ForVersion != "":
		// SHOW FEATURES FOR VERSION x.y — no project connection needed
		pv, err = versions.ParseSemVer(s.ForVersion)
		if err != nil {
			return mdlerrors.NewValidationf("invalid version %q: %v", s.ForVersion, err)
		}

	default:
		// SHOW FEATURES [IN area] — requires project connection
		if !ctx.Connected() {
			return mdlerrors.NewNotConnectedMsg("not connected to a project\n  hint: use SHOW FEATURES FOR VERSION x.y without a project connection")
		}
		rpv := ctx.Backend.ProjectVersion()
		pv = versions.SemVer{Major: rpv.MajorVersion, Minor: rpv.MinorVersion, Patch: rpv.PatchVersion}
	}

	if s.InArea != "" {
		return showFeaturesInArea(ctx, reg, pv, s.InArea)
	}
	return showFeaturesAll(ctx, reg, pv)
}

func showFeaturesAll(ctx *ExecContext, reg *versions.Registry, pv versions.SemVer) error {
	features := reg.FeaturesForVersion(pv)
	if len(features) == 0 {
		fmt.Fprintf(ctx.Output, "No features found for version %s\n", pv)
		return nil
	}

	fmt.Fprintf(ctx.Output, "Features for Mendix %s:\n\n", pv)

	available, unavailable := 0, 0
	tr := &TableResult{
		Columns: []string{"Feature", "Available", "Since", "Notes"},
	}
	for _, f := range features {
		avail := "Yes"
		if !f.Available {
			avail = "No"
			unavailable++
		} else {
			available++
		}
		notes := f.Notes
		if !f.Available && f.Workaround != nil {
			notes = f.Workaround.Description
		}
		if len(notes) > 38 {
			notes = notes[:35] + "..."
		}
		tr.Rows = append(tr.Rows, []any{f.DisplayName(), avail, fmt.Sprintf("%s", f.MinVersion), notes})
	}
	tr.Summary = fmt.Sprintf("(%d available, %d not available in %s)", available, unavailable, pv)
	return writeResult(ctx, tr)
}

func showFeaturesInArea(ctx *ExecContext, reg *versions.Registry, pv versions.SemVer, area string) error {
	features := reg.FeaturesInArea(area, pv)
	if len(features) == 0 {
		// Check if the area exists at all.
		areas := reg.Areas()
		fmt.Fprintf(ctx.Output, "No features found in area %q for version %s\n", area, pv)
		fmt.Fprintf(ctx.Output, "Available areas: %s\n", strings.Join(areas, ", "))
		return nil
	}

	fmt.Fprintf(ctx.Output, "Features in %s for Mendix %s:\n\n", area, pv)

	tr := &TableResult{
		Columns: []string{"Feature", "Available", "Since", "Notes"},
	}
	for _, f := range features {
		avail := "Yes"
		if !f.Available {
			avail = "No"
		}
		notes := f.Notes
		if !f.Available && f.Workaround != nil {
			notes = f.Workaround.Description
		}
		if len(notes) > 38 {
			notes = notes[:35] + "..."
		}
		tr.Rows = append(tr.Rows, []any{f.DisplayName(), avail, fmt.Sprintf("%s", f.MinVersion), notes})
	}
	return writeResult(ctx, tr)
}

func showFeaturesAddedSince(ctx *ExecContext, reg *versions.Registry, sinceV versions.SemVer) error {
	added := reg.FeaturesAddedSince(sinceV)
	if len(added) == 0 {
		fmt.Fprintf(ctx.Output, "No new features found since %s\n", sinceV)
		return nil
	}

	fmt.Fprintf(ctx.Output, "Features added since Mendix %s:\n\n", sinceV)

	tr := &TableResult{
		Columns: []string{"Feature", "Area", "Since", "Notes"},
		Summary: fmt.Sprintf("(%d features added since %s)", len(added), sinceV),
	}
	for _, f := range added {
		notes := f.Notes
		if f.MDL != "" && notes == "" {
			notes = f.MDL
		}
		if len(notes) > 38 {
			notes = notes[:35] + "..."
		}
		tr.Rows = append(tr.Rows, []any{f.DisplayName(), f.Area, fmt.Sprintf("%s", f.MinVersion), notes})
	}
	return writeResult(ctx, tr)
}
