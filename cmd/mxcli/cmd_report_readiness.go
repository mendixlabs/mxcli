// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/spf13/cobra"
)

var reportReadinessCmd = newReportKindCmd(reportKindSpec{
	name:  "readiness",
	short: "One go/no-go page for production, rolled up from the other reports",
	long: `Roll the other reports up into one verdict with its reasons:

  blocking        consistency errors, a release that destroys production data,
                  a high or critical vulnerability, a location fixed to
                  localhost or a test host, a literal credential, an open high
                  security finding, a failing test or journey, a requirement of
                  the milestone being released (--milestone) not built in full
  risks           lint findings by category, release changes to verify, the
                  Mendix release line, modules behind, medium security
                  findings, untested callable microflows, unproven
                  requirements, steps over --budget-ms, domain cycles
  accepted        brain decisions and the verdicts recorded on the reports,
                  each with what accepts it
  administrator   the configuration report's checklist
  not covered     what no model report can tell, listed every time

The verdict rule is on the page and in the JSON: any open blocking item is
"not ready"; otherwise a report that decides a blocking question but was not
given (or could not check) makes it "not assessed in full"; otherwise "ready,
with risks" or "ready".

--dir reads the reports 'mxcli report all' wrote (<kind>.json, quality.json,
and each <kind>-annotations.yaml); 'report all' also writes this page. With
-p alone it runs the reports that need no input; release and performance then
show as not assessed.

Sign off a blocking item with --annotations: readiness:<key> with status
"signed off" and a note; it then counts as accepted.`,
	example: `  mxcli report all -p app.mpr -o docs/reports --against v1.4.0   # writes readiness.html too
  mxcli report readiness --dir docs/reports --milestone M2 -o docs/reports/readiness.html
  mxcli report readiness -p app.mpr -o readiness.html`,
	flags: func(cmd *cobra.Command) {
		cmd.Flags().String("dir", "", "Read the other reports from this folder, as 'report all' writes it")
		cmd.Flags().String("milestone", "", "The milestone being released: its requirements not built in full are blocking")
		cmd.Flags().Float64("budget-ms", 1000, "Server time a journey step may take before it is a risk (0: no budget)")
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		dir, _ := cmd.Flags().GetString("dir")
		var envs map[string]*projectreport.Envelope
		var quality *linter.JSONReport
		ann := map[string]*projectreport.Annotations{}
		var warnings []string
		switch {
		case dir != "":
			envs, quality = readReportDir(dir)
			for _, k := range projectreport.ReadinessKinds {
				a, err := projectreport.LoadAnnotations(existing(filepath.Join(dir, k+"-annotations.yaml")))
				if err != nil {
					return nil, err
				}
				ann[k] = a
			}
		case projectPath != "":
			envs = map[string]*projectreport.Envelope{}
			for _, k := range []string{"architecture", "security", "configuration", "dependencies", "tests", "traceability"} {
				rk, ok := reportKinds[k]
				if !ok {
					continue
				}
				fmt.Fprintf(os.Stderr, "report %s ...\n", k)
				env, err := rk.spec.collect(rk.cmd, projectPath)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("%s could not run: %v", k, err))
					continue
				}
				envs[k] = env
			}
			fmt.Fprintln(os.Stderr, "report quality ...")
			if r, _, err := buildScoredReport(reportCmd, projectPath, io.Discard); err != nil {
				warnings = append(warnings, fmt.Sprintf("the scored report could not run: %v", err))
			} else if quality, err = qualityJSON(r); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("give --dir <reports folder> or -p <app.mpr> (or --from readiness.json to re-render)")
		}
		in, err := projectreport.ReadinessInputFrom(envs, quality, ann)
		if err != nil {
			return nil, err
		}
		annPath, _ := cmd.Flags().GetString("annotations")
		if in.SignOff, err = projectreport.LoadAnnotations(annPath); err != nil {
			return nil, err
		}
		in.Milestone, _ = cmd.Flags().GetString("milestone")
		in.BudgetMs, _ = cmd.Flags().GetFloat64("budget-ms")
		project := reportProjectName(projectPath)
		if project == "." || projectPath == "" {
			project = reportProjectNameFrom(envs, quality, dir)
		}
		return projectreport.NewEnvelope("readiness", project, reportNow(), version, projectreport.ComputeReadiness(in),
			projectreport.ReadinessMethod, projectreport.ReadinessLimits, warnings)
	},
	render: projectreport.RenderReadiness,
})

func init() {
	reportCmd.AddCommand(reportReadinessCmd)
}

// readReportDir reads the reports a folder holds; a report that is absent or
// unreadable is not given.
func readReportDir(dir string) (map[string]*projectreport.Envelope, *linter.JSONReport) {
	envs := map[string]*projectreport.Envelope{}
	for _, k := range projectreport.ReadinessKinds {
		if k == "quality" {
			continue
		}
		if env, err := projectreport.ReadEnvelope(filepath.Join(dir, k+".json"), k); err == nil {
			envs[k] = env
		}
	}
	var quality *linter.JSONReport
	if b, err := os.ReadFile(filepath.Join(dir, "quality.json")); err == nil {
		q := &linter.JSONReport{}
		if json.Unmarshal(b, q) == nil {
			quality = q
		}
	}
	return envs, quality
}

// qualityJSON is the scored report as its JSON file holds it.
func qualityJSON(r *linter.Report) (*linter.JSONReport, error) {
	var buf bytes.Buffer
	if err := linter.GetReportFormatter("json").FormatReport(r, &buf); err != nil {
		return nil, err
	}
	q := &linter.JSONReport{}
	return q, json.Unmarshal(buf.Bytes(), q)
}

// reportProjectNameFrom names the project from the reports read.
func reportProjectNameFrom(envs map[string]*projectreport.Envelope, quality *linter.JSONReport, dir string) string {
	for _, env := range envs {
		if env.Project != "" {
			return env.Project
		}
	}
	if quality != nil && quality.ProjectName != "" {
		return quality.ProjectName
	}
	return filepath.Base(dir)
}
