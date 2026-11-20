// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	"github.com/spf13/cobra"
)

var reportReleaseCmd = newReportKindCmd(reportKindSpec{
	name:  "release",
	short: "What deploying this model does to production data: tables and columns dropped, renamed, added",
	long: `Compare the model with the one that is deployed and say, for every persistent
entity, attribute and association, what the deploy does to the data already
stored:

  destroys   a table or column is dropped with its data: removed, no longer
             persistent, now calculated, or a GUID changed under the same name
  verify     handled in a way that has not been measured: an attribute type, a
             shorter string, an association's type or ends, a generalization
  migrates   renamed or moved under the same GUID: the rows are kept
  adds       a new table, column or association

The runtime keys the database on each element's GUID, not its name, so the two
models are paired by GUID. A model can be valid, pass mx check and build, and
still drop a table on the next deploy; this report is what shows it.

--against is what is deployed: a git revision of the project (a tag, a branch,
a commit) or the path of a .mpr. Annotation keys: release:<kind>:<name>, with
status accepted for a drop that is intended.`,
	example: `  mxcli report release -p app.mpr --against v1.4.0 -o release.html
  mxcli report release -p app.mpr --against origin/production --format json -o release.json
  mxcli report release -p app.mpr --against /backups/deployed/App.mpr`,
	flags: func(cmd *cobra.Command) {
		cmd.Flags().String("against", "", "What is deployed: a git revision of the project, or a .mpr path (required)")
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		against, _ := cmd.Flags().GetString("against")
		if against == "" {
			return nil, fmt.Errorf("--against is required: the git revision (or .mpr) that is deployed, e.g. --against v1.4.0")
		}
		basePath, label, cleanup, err := resolveReleaseBaseline(projectPath, against)
		defer cleanup()
		if err != nil {
			return nil, err
		}
		deployed, err := loadReleaseModel(basePath)
		if err != nil {
			return nil, fmt.Errorf("read the deployed model (%s): %w", label, err)
		}
		next, err := loadReleaseModel(projectPath)
		if err != nil {
			return nil, fmt.Errorf("read the model: %w", err)
		}
		data := projectreport.CompareReleases(deployed, next)
		data.Against, data.Current = label, projectPath
		var warnings []string
		if data.Tables == 0 {
			warnings = append(warnings, "The deployed model has no persistent entities, so nothing can be destroyed. Check that --against is what is deployed.")
		}
		return projectreport.NewEnvelope("release", reportProjectName(projectPath), reportNow(), version, data,
			projectreport.ReleaseMethod, projectreport.ReleaseLimits, warnings)
	},
	render: projectreport.RenderRelease,
})

func init() {
	reportCmd.AddCommand(reportReleaseCmd)
}
