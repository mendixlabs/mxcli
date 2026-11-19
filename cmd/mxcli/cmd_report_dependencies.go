// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	mp "github.com/mendixlabs/mxcli/internal/marketplace"
	"github.com/mendixlabs/mxcli/sdk/versions"
	"github.com/spf13/cobra"
)

// newMarketplaceSource is a seam for tests; it fails without a token.
var newMarketplaceSource = func(cmd *cobra.Command) (marketplaceSource, error) {
	client, err := newMarketplaceClient(cmd.Context(), cmd)
	if err != nil {
		return nil, err
	}
	profile, _ := cmd.Flags().GetString("profile")
	return clientSource{client: client, cache: &mp.CatalogCache{Path: marketplaceCachePath(profile)}}, nil
}

var reportDependenciesCmd = newReportKindCmd(reportKindSpec{
	name:  "dependencies",
	short: "What the app stands on: Mendix release line, Marketplace modules, widgets, Java libraries and their known vulnerabilities",
	long: `List what the app depends on and whether it is current and safe:

  Mendix version      the project's version and its release line (LTS, MTS or a
                      monthly release), from mxcli's version registry
  Marketplace modules installed version, newest compatible and newest published,
                      identified by the version UUID each module records
  pluggable widgets   the packages in widgets/ with their version and the newest
                      published one
  Java libraries      the jars in userlib/ and vendorlib/ with their Maven
                      coordinates and the known vulnerabilities (osv.dev) of
                      that version, worst first

The Marketplace needs a token ('mxcli auth login'); without one, or with
--offline, the report says the versions were not checked. --offline also skips
the vulnerability lookup. What was not checked is never shown as up to date.

Annotation keys: vuln:<advisory id> (status accepted, and why it does not
apply), module:<Name>.`,
	example: `  mxcli report dependencies -p app.mpr -o dependencies.html
  mxcli report dependencies -p app.mpr --format json -o deps.json
  mxcli report dependencies -p app.mpr --offline`,
	flags: func(cmd *cobra.Command) {
		cmd.Flags().Bool("offline", false, "Ask no online source: no Marketplace versions, no vulnerability lookup")
		cmd.Flags().String("profile", "", "Credential profile for the Marketplace (see 'mxcli auth login')")
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		offline, _ := cmd.Flags().GetBool("offline")
		mendixVersion, mods, err := marketplaceModules(projectPath)
		if err != nil {
			return nil, err
		}
		dir := filepath.Dir(projectPath)
		d := &projectreport.DependencyData{
			MendixVersion: mendixVersion, Line: versions.LineUnknown,
			Modules: mods, Widgets: widgetPackages(dir), Libraries: javaLibraries(dir),
		}
		if reg, err := versions.Load(); err == nil {
			if sv, err := versions.ParseSemVer(mendixVersion); err == nil {
				d.Line, d.LineName = reg.ReleaseLine(sv)
			}
		}
		var warnings []string
		switch {
		case offline:
			d.VulnNote = "--offline."
			d.MarketplaceNote = "--offline."
		default:
			if err := checkVulnerabilities(cmd.Context(), d.Libraries); err != nil {
				d.VulnNote = fmt.Sprintf("the advisory database could not be reached (%v).", err)
			} else {
				d.VulnChecked, d.VulnSource = true, "OSV (osv.dev)"
			}
			src, err := newMarketplaceSource(cmd)
			switch {
			case err != nil:
				d.MarketplaceNote = "no Marketplace token. Run 'mxcli auth login' to compare with published versions."
			case len(d.Modules) == 0 && len(d.Widgets) == 0:
				d.MarketplaceChecked = true
			default:
				if err := checkMarketplace(cmd.Context(), src, mendixVersion, d.Modules, d.Widgets); err != nil {
					d.MarketplaceNote = fmt.Sprintf("the Marketplace could not be read (%v).", err)
				} else {
					d.MarketplaceChecked = true
				}
			}
		}
		if !d.VulnChecked {
			for i := range d.Libraries {
				if d.Libraries[i].Note == "" {
					d.Libraries[i].Note = "the advisory database was not asked"
				}
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "userlib")); err != nil && len(d.Libraries) == 0 {
			warnings = append(warnings, "The project has no userlib/ directory, so no Java libraries were found. A project whose dependencies come from Gradle has its jars in vendorlib/ after a build or 'mx sync-java-dependencies'.")
		}
		return projectreport.NewEnvelope("dependencies", reportProjectName(projectPath), reportNow(), version, d,
			projectreport.DependenciesMethod, projectreport.DependenciesLimits, warnings)
	},
	render: projectreport.RenderDependencies,
})

func init() {
	reportCmd.AddCommand(reportDependenciesCmd)
}
