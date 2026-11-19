// SPDX-License-Identifier: Apache-2.0

// cmd_export_contract.go — write a published service's contract to a file.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/businessevents"
	"github.com/mendixlabs/mxcli/model"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export a published service's contract",
}

var exportContractCmd = &cobra.Command{
	Use:   "contract <Module.Service>",
	Short: "Write a published service's contract to a file",
	Long: `Write the contract of a published service to a file, the way Studio Pro's
"Export AsyncAPI Document" button does.

This is what lets two separate apps exchange business events headlessly. The
producing app mints the channel id, and it reaches a consumer inside the
exported contract — not by both apps spelling the same string. Export it from
app A, then name it in app B:

  mxcli export contract CdcPublish.OrdersEventService -p appA.mpr -o contract.yaml

  -- in app B's script:
  create business event service Orders.OrdersEventClient
  ( ContractFile: 'contract.yaml' )
  {
    consume OrderPlaceEvent
      entity Orders.CBE_OrderPlaceEvent
      microflow Orders.HandleBE_OrderPlaceEvent;
  };

Only business event services are supported so far. A published OData or REST
service's contract is served by the running app, so exporting one means fetching
it rather than rendering it; that is not implemented here yet.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			return fmt.Errorf("a project is required: pass -p <project.mpr>")
		}
		outPath, _ := cmd.Flags().GetString("output")

		name := args[0]
		module, service, ok := strings.Cut(name, ".")
		if !ok || module == "" || service == "" {
			return fmt.Errorf("%q is not a qualified name: write it as Module.Service", name)
		}

		reader, err := openProjectReadOnly(projectPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", projectPath, err)
		}
		defer reader.Disconnect()

		services, err := reader.ListBusinessEventServices()
		if err != nil {
			return fmt.Errorf("list business event services: %w", err)
		}

		var found *model.BusinessEventService
		var known []string
		for _, svc := range services {
			qn := businessEventQualifiedName(reader, svc)
			known = append(known, qn)
			if strings.EqualFold(qn, name) {
				found = svc
			}
		}
		if found == nil {
			detail := ""
			if len(known) > 0 {
				detail = " Known business event services: " + strings.Join(known, ", ")
			}
			return fmt.Errorf("no business event service %s in %s.%s", name, projectPath, detail)
		}

		contract, err := businessevents.RenderAsyncAPI(found)
		if err != nil {
			return err
		}

		if outPath == "" {
			fmt.Print(contract)
			return nil
		}
		if err := os.WriteFile(outPath, []byte(contract), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		fmt.Fprintf(os.Stderr, "Wrote %s (%d bytes): the AsyncAPI contract of %s.\n", outPath, len(contract), name)
		fmt.Fprintf(os.Stderr, "Name it in the consuming app with ContractFile: '%s'.\n", outPath)
		return nil
	},
}

// businessEventQualifiedName resolves a service's Module.Name, falling back to
// the bare name when the hierarchy cannot place it — a service that cannot be
// addressed is still worth listing when the lookup fails.
func businessEventQualifiedName(reader interface {
	ListModules() ([]*model.Module, error)
}, svc *model.BusinessEventService) string {
	modules, err := reader.ListModules()
	if err != nil {
		return svc.Name
	}
	for _, m := range modules {
		if m.ID == svc.ContainerID {
			return m.Name + "." + svc.Name
		}
	}
	return svc.Name
}

func init() {
	exportContractCmd.Flags().StringP("output", "o", "", "write the contract to this file (default: stdout)")
	exportCmd.AddCommand(exportContractCmd)
	rootCmd.AddCommand(exportCmd)
}
