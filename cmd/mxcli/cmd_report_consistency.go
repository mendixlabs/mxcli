// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/spf13/cobra"
)

// The scored report states whether the project builds, from Mendix's own
// consistency check (ako/mxcli#2423). The lint rules measure conventions; a
// project with consistency errors cannot be deployed and could still score
// well.

// reportMxCheck runs `mx check --json` on a copy of the project. A seam for
// tests, which have no mx.
var reportMxCheck = docker.CheckFindings

// reportCheckKinds asks mx for warnings and deprecations as well as errors:
// the report lists them, without scoring them. Best-practice findings are left
// to the lint rules.
var reportCheckKinds = docker.CheckKinds{Warnings: true, Deprecations: true}

// reportConsistency runs the check, or reads saved results, or says why it was
// not run. Only an unreadable results file the user named is an error: a
// missing mx is a report that says so, never a clean one.
func reportConsistency(cmd *cobra.Command, projectPath string, progress io.Writer) (*linter.ConsistencyCheck, error) {
	if skip, _ := cmd.Flags().GetBool("no-mx-check"); skip {
		return &linter.ConsistencyCheck{NotRun: "skipped with --no-mx-check."}, nil
	}
	if path, _ := cmd.Flags().GetString("mx-check-results"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("--mx-check-results: %w", err)
		}
		rep, err := docker.ParseCheckJSON(b)
		if err != nil {
			return nil, fmt.Errorf("--mx-check-results %s: %w (it takes the file 'mx check <app.mpr> -j <file>' writes)", path, err)
		}
		return consistencyFrom(rep, "results file "+path), nil
	}
	mxbuild, _ := cmd.Flags().GetString("mxbuild-path")
	fmt.Fprintln(progress, "Running mx check on a temporary copy of the project (--no-mx-check skips it)...")
	rep, runErr := reportMxCheck(docker.CheckOptions{ProjectPath: projectPath, MxBuildPath: mxbuild}, reportCheckKinds)
	if runErr != nil && len(rep.Findings) == 0 && !rep.HasDeclared {
		// mx was not found, or did not get as far as checking.
		return &linter.ConsistencyCheck{NotRun: fmt.Sprintf("%v. 'mxcli setup mxbuild -p %s' sets up the project's Mendix version; "+
			"--mx-check-results reads the JSON report of an earlier 'mx check <app.mpr> -j <file>'.", runErr, projectPath)}, nil
	}
	return consistencyFrom(rep, "mx check"), nil
}

// consistencyFrom turns a parsed `mx check` run into the report's form.
func consistencyFrom(rep docker.CheckReport, source string) *linter.ConsistencyCheck {
	c := &linter.ConsistencyCheck{Ran: true, Source: source}
	for _, f := range rep.Findings {
		switch f.Severity {
		case "error":
			c.Errors++
		case "warning":
			c.Warnings++
		case "deprecation":
			c.Deprecations++
		}
		cf := linter.ConsistencyFinding{Severity: f.Severity, Code: f.Code, Message: f.Message}
		if len(f.Locations) > 0 {
			l := f.Locations[0]
			cf.Module, cf.Document, cf.Element = l.ModuleName, l.DocumentName, l.ElementName
		}
		c.Findings = append(c.Findings, cf)
	}
	if err := rep.Verify(); err != nil {
		c.Incomplete = err.Error()
	}
	return c
}

func init() {
	reportCmd.Flags().Bool("no-mx-check", false, "Do not run Mendix's consistency check (mx check); the report then says whether the project builds is unknown")
	reportCmd.Flags().String("mx-check-results", "", "Read the JSON report of an earlier 'mx check <app.mpr> -j <file>' instead of running the check")
	reportCmd.Flags().String("annotations", "", "YAML or JSON with a verdict per rule for the HTML page: items: {\"rule:MPR006\": {status: by design, note: \"…\"}}")
	reportCmd.Flags().String("mxbuild-path", "", "Path to the mxbuild whose mx runs the check, overriding resolution by the project's Mendix version")
}
