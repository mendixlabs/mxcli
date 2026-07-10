// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/JordtenBulte-OLC/mxcli/internal/auth"
	"github.com/JordtenBulte-OLC/mxcli/internal/catalog"
	"github.com/spf13/cobra"
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Search and manage Mendix Catalog services (catalog.mendix.com)",
	Long: `Search for data sources and services registered in Mendix Catalog (catalog.mendix.com).

NOTE: This is the external Mendix Catalog service, NOT the MDL CATALOG keyword.
  - CLI catalog commands: Search external service registry (requires auth)
  - MDL CATALOG keyword: Query local project metadata (SELECT ... FROM CATALOG.entities)

Requires authentication via Personal Access Token (PAT). Create a PAT at:
  https://user-settings.mendix.com/

Storage priority:
  1. MENDIX_PAT env var (set MXCLI_PROFILE to target a non-default profile)
  2. ~/.mxcli/auth.json (mode 0600)`,
}

var catalogSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for services in the Catalog",
	Long: `Search for data sources and services in Mendix Catalog.

Examples:
  mxcli catalog search "customer"
  mxcli catalog search "order" --service-type OData
  mxcli catalog search "api" --production-only --json`,
	Args: cobra.ExactArgs(1),
	RunE: runCatalogSearch,
}

var catalogShowCmd = &cobra.Command{
	Use:   "show <uuid>",
	Short: "Show detailed endpoint metadata",
	Long: `Display detailed metadata for a Catalog endpoint including entities, actions, and contract.

Examples:
  mxcli catalog show a7f3c2d1-4b5e-6c7f-8d9e-0a1b2c3d4e5f
  mxcli catalog show a7f3c2d1 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runCatalogShow,
}

func init() {
	catalogSearchCmd.Flags().String("profile", auth.ProfileDefault, "credential profile name")
	catalogSearchCmd.Flags().String("service-type", "", "filter by service type (OData, REST, SOAP)")
	catalogSearchCmd.Flags().Bool("production-only", false, "show only production endpoints")
	catalogSearchCmd.Flags().Bool("owned-only", false, "show only owned services")
	catalogSearchCmd.Flags().Int("limit", 20, "results per page (max 100)")
	catalogSearchCmd.Flags().Int("offset", 0, "pagination offset")
	catalogSearchCmd.Flags().Bool("json", false, "output as JSON array")

	catalogShowCmd.Flags().String("profile", auth.ProfileDefault, "credential profile name")
	catalogShowCmd.Flags().Bool("json", false, "output full JSON response")

	catalogCmd.AddCommand(catalogSearchCmd)
	catalogCmd.AddCommand(catalogShowCmd)
	rootCmd.AddCommand(catalogCmd)
}

func runCatalogSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	profile, _ := cmd.Flags().GetString("profile")
	serviceType, _ := cmd.Flags().GetString("service-type")
	prodOnly, _ := cmd.Flags().GetBool("production-only")
	ownedOnly, _ := cmd.Flags().GetBool("owned-only")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	asJSON, _ := cmd.Flags().GetBool("json")

	// Create client
	client, err := catalog.NewClient(cmd.Context(), profile)
	if err != nil {
		if _, ok := err.(*auth.ErrNoCredential); ok {
			return fmt.Errorf("no credential found. Run: mxcli auth login")
		}
		return err
	}

	// Execute search
	opts := catalog.SearchOptions{
		Query:                   query,
		ServiceType:             serviceType,
		ProductionEndpointsOnly: prodOnly,
		OwnedContentOnly:        ownedOnly,
		Limit:                   limit,
		Offset:                  offset,
	}
	resp, err := client.Search(cmd.Context(), opts)
	if err != nil {
		if _, ok := err.(*auth.ErrUnauthenticated); ok {
			return fmt.Errorf("authentication failed. Run: mxcli auth login")
		}
		return err
	}

	// Output
	if asJSON {
		return outputJSON(cmd, resp.Data)
	}
	return outputTable(cmd, resp)
}

func outputTable(cmd *cobra.Command, resp *catalog.SearchResponse) error {
	if len(resp.Data) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No results found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tVERSION\tAPPLICATION\tENVIRONMENT\tPROD\tUUID")

	for _, item := range resp.Data {
		name := truncate(item.Name, 22)
		typ := truncate(item.ServiceType, 8)
		version := truncate(item.Version, 10)
		app := truncate(item.Application.Name, 20)
		env := truncate(item.Environment.Type, 12)
		prod := ""
		if item.Environment.Type == "Production" {
			prod = "Yes"
		}
		uuid := item.UUID // Full UUID (36 chars) so users can use it with `show`

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			name, typ, version, app, env, prod, uuid)
	}

	fmt.Fprintf(w, "\nTotal: %d results (showing %d-%d)\n",
		resp.TotalResults, resp.Offset+1, resp.Offset+len(resp.Data))

	return w.Flush()
}

func outputJSON(cmd *cobra.Command, data []catalog.SearchResult) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func runCatalogShow(cmd *cobra.Command, args []string) error {
	uuid := args[0]
	profile, _ := cmd.Flags().GetString("profile")
	asJSON, _ := cmd.Flags().GetBool("json")

	// Create client
	client, err := catalog.NewClient(cmd.Context(), profile)
	if err != nil {
		if _, ok := err.(*auth.ErrNoCredential); ok {
			return fmt.Errorf("no credential found. Run: mxcli auth login")
		}
		return err
	}

	// Get endpoint details
	endpoint, err := client.GetEndpoint(cmd.Context(), uuid)
	if err != nil {
		if _, ok := err.(*auth.ErrUnauthenticated); ok {
			return fmt.Errorf("authentication failed. Run: mxcli auth login")
		}
		return err
	}

	// Output
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(endpoint)
	}
	return outputEndpointDetails(cmd, endpoint)
}

func outputEndpointDetails(cmd *cobra.Command, ep *catalog.EndpointDetails) error {
	w := cmd.OutOrStdout()
	sv := ep.ServiceVersion

	// Basic info
	fmt.Fprintf(w, "Name:         %s\n", sv.Description)
	fmt.Fprintf(w, "Type:         %s\n", sv.Type)
	fmt.Fprintf(w, "Version:      %s\n", sv.Version)
	fmt.Fprintf(w, "Application:  %s\n", ep.Environment.Application.Name)
	fmt.Fprintf(w, "Environment:  %s (%s)\n", ep.Environment.Type, ep.Environment.Location)
	if ep.Location != "" {
		fmt.Fprintf(w, "Location:     %s\n", ep.Location)
	}
	fmt.Fprintf(w, "\n")

	// Security
	if sv.SecurityScheme != nil && len(sv.SecurityScheme.SecurityTypes) > 0 {
		var types []string
		for _, st := range sv.SecurityScheme.SecurityTypes {
			types = append(types, st.Name)
		}
		fmt.Fprintf(w, "Security:     %s\n", strings.Join(types, ", "))
	}
	fmt.Fprintf(w, "Validated:    %v\n", ep.Validated)
	fmt.Fprintf(w, "Last Updated: %s\n", ep.LastUpdated)
	fmt.Fprintf(w, "\n")

	// Entities (OData only)
	if sv.TotalEntities > 0 {
		fmt.Fprintf(w, "Entities (%d):\n", sv.TotalEntities)
		for _, ent := range sv.Entities {
			fmt.Fprintf(w, "  - %s (%d attributes", ent.Name, ent.TotalAttributes)
			if ent.TotalAssociations > 0 {
				fmt.Fprintf(w, ", %d associations", ent.TotalAssociations)
			}
			fmt.Fprintf(w, ")\n")

			// Show first 3 attributes
			if len(ent.Attributes) > 0 {
				var attrNames []string
				for i, attr := range ent.Attributes {
					if i >= 3 {
						break
					}
					attrNames = append(attrNames, attr.Name)
				}
				fmt.Fprintf(w, "    Attributes: %s", strings.Join(attrNames, ", "))
				if len(ent.Attributes) > 3 {
					fmt.Fprintf(w, ", ...")
				}
				fmt.Fprintf(w, "\n")
			}
		}
		fmt.Fprintf(w, "\n")
	}

	// Actions (OData only)
	if sv.TotalActions > 0 {
		fmt.Fprintf(w, "Actions (%d):\n", sv.TotalActions)
		for _, action := range sv.Actions {
			fmt.Fprintf(w, "  - %s", action.Name)
			if action.TotalParameters > 0 {
				fmt.Fprintf(w, " (%d parameters)", action.TotalParameters)
			}
			fmt.Fprintf(w, "\n")
		}
	}

	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
