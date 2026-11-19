// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	"github.com/spf13/cobra"
)

var reportConfigurationCmd = newReportKindCmd(reportKindSpec{
	name:  "configuration",
	short: "What each environment must supply, read from the model, and what the model fixes",
	long: `List what a deployment has to supply, as a checklist an administrator can
sign off, and what the model fixes so that no environment can change it:

  constants         every constant in the app's modules with its default, its
                    run-configuration values and where the model uses it:
                    must be set (no default, a placeholder), wrong unless
                    overridden (a localhost, private or test-host default), or
                    a secret in the model (the security report's finding)
  fixed locations   REST clients and REST call actions whose location is a
                    literal, worst when it is localhost, a private address or
                    a test host; credentials written as literals
  runs by itself    scheduled events (switched on or off per environment) and
                    the after-startup, before-shutdown and health-check
                    microflows with what they write and call
  published         published REST and OData services and how their callers
                    authenticate

The actual production values live in the deployment portal or pipeline; this
report does not read them and says so.

Annotation keys: constant:<Name>, scheduled:<Name>, published:<Name> (status
done, with a note naming the environment).`,
	example: `  mxcli report configuration -p app.mpr -o configuration.html
  mxcli report configuration -p app.mpr --format json -o configuration.json
  mxcli report configuration --from configuration.json --annotations signoff.yaml -o configuration.html`,
	flags: func(cmd *cobra.Command) {
		scopeFlags(cmd)
		cmd.Flags().Bool("no-source", false, "Do not index microflow bodies (faster; constant uses and REST call locations are then not read)")
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		modes := []string{"full"}
		if noSrc, _ := cmd.Flags().GetBool("no-source"); !noSrc {
			modes = append(modes, "source")
		}
		exec, cleanup, err := openReportCatalog(projectPath, modes...)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		in := projectreport.ConfigurationInput{Scope: scopeOptions(cmd)}
		if ps, err := exec.Backend().GetProjectSettings(); err == nil && ps != nil && ps.Model != nil {
			in.AfterStartup, in.BeforeShutdown, in.HealthCheck = ps.Model.AfterStartupMicroflow, ps.Model.BeforeShutdownMicroflow, ps.Model.HealthCheckMicroflow
		}
		data, err := projectreport.CollectConfiguration(exec.Catalog(), in)
		if err != nil {
			return nil, err
		}
		return projectreport.NewEnvelope("configuration", reportProjectName(projectPath), reportNow(), version, data,
			projectreport.ConfigurationMethod, projectreport.ConfigurationLimits, nil)
	},
	render: projectreport.RenderConfiguration,
})

func init() {
	reportCmd.AddCommand(reportConfigurationCmd)
}
