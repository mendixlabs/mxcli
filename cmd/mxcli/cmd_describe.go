// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	modelsdk "github.com/JordtenBulte-OLC/mxcli"
	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var describeCmd = &cobra.Command{
	Use:   "describe [<type>] <name>",
	Short: "Describe a project element",
	Long: `Describe an element from a Mendix project in MDL syntax.

The <type> is optional for a qualified document name: 'describe MyModule.Customer'
auto-detects the document type. Pass the type explicitly to disambiguate (a name
can match both an entity and a document, for example) or for forms that have no
single qualified name (module, settings, navigation, module role).

Types:
  module           Describe a module (all contents)
  entity           Describe an entity
  externalentity   Describe an external entity (alias for entity)
  association      Describe an association
  enumeration      Describe an enumeration
  constant         Describe a constant
  microflow        Describe a microflow
  nanoflow         Describe a nanoflow
  workflow         Describe a workflow
  page             Describe a page
  snippet          Describe a snippet
  layout           Describe a layout
  javaaction       Describe a java action
  jsonstructure    Describe a JSON structure (also: "json structure")
  importmapping    Describe an import mapping (also: "import mapping")
  exportmapping    Describe an export mapping (also: "export mapping")
  restclient       Describe a consumed REST service (also: "rest client")
  odataclient      Describe a consumed OData service
  odataservice     Describe a published OData service
  imagecollection  Describe an image collection (also: "image collection")
  businesseventservice  Describe a business event service (also: "business event service")
  databaseconnection    Describe a database connection (also: "database connection")
  agent            Describe an AI agent (also: "agent")
  aimodel          Describe an AI model (also: "model", "ai model")
  knowledgebase    Describe a knowledge base (also: "knowledge base")
  consumedmcpservice  Describe a consumed MCP service (also: "consumed mcp service")
  datatransformer  Describe a data transformer (also: "data transformer")
  modulerole       Describe a module role
  userrole         Describe a user role
  projectsecurity  Show project security settings
  settings         Describe project settings
  demouser         Describe a demo user
  navigation       Describe a navigation profile
  systemoverview   Module dependency graph (requires --format elk)

Example:
  mxcli describe -p app.mpr MyModule.Customer          # type auto-detected
  mxcli describe -p app.mpr MyModule.ProcessOrder      # type auto-detected
  mxcli describe -p app.mpr module MyModule
  mxcli describe -p app.mpr entity MyModule.Customer
  mxcli describe -p app.mpr microflow MyModule.ProcessOrder
  mxcli describe -p app.mpr nanoflow MyModule.ValidateInput
  mxcli describe -p app.mpr page MyModule.Customer_Overview
  mxcli describe -p app.mpr json structure MyModule.CustomerResponse
  mxcli describe -p app.mpr import mapping MyModule.IMM_Customer
  mxcli describe -p app.mpr export mapping MyModule.EMM_Customer
  mxcli describe -p app.mpr rest client MyModule.PetStoreAPI
  mxcli describe -p app.mpr settings Settings
  mxcli describe -p app.mpr navigation Responsive
  mxcli describe -p app.mpr --format elk systemoverview SystemOverview
  mxcli describe -p app.mpr agent MyModule.MyAgent
  mxcli describe -p app.mpr model MyModule.GPT4
  mxcli describe -p app.mpr knowledge base MyModule.ProductDocs
  mxcli describe -p app.mpr consumed mcp service MyModule.FilesystemMCP
  mxcli describe -p app.mpr data transformer MyModule.TransformCustomer
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Type auto-detection: with a single argument (a name, no type), look up
		// the document's type from the project and prepend it so the rest of the
		// dispatch is unchanged.
		if len(args) == 1 {
			detected, candidates, err := resolveDescribeType(projectPath, args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				fmt.Fprintf(os.Stderr, "hint: pass the type explicitly, e.g. 'describe <type> %s'\n", args[0])
				os.Exit(1)
			}
			if len(candidates) > 0 {
				fmt.Fprintf(os.Stderr, "Error: %q is ambiguous — it matches: %s\n", args[0], strings.Join(candidates, ", "))
				fmt.Fprintf(os.Stderr, "Specify the type, e.g. 'describe %s %s'\n", candidates[0], args[0])
				os.Exit(1)
			}
			args = append([]string{detected}, args...)
		}

		// Support multi-word types: "business event service Module.Name" → type="BUSINESS EVENT SERVICE", name="Module.Name"
		// The last arg is always the name, everything before is the type.
		name := args[len(args)-1]
		objectType := strings.ToUpper(strings.Join(args[:len(args)-1], " "))

		var mdlCmd string
		switch objectType {
		case "MODULE":
			mdlCmd = fmt.Sprintf("DESCRIBE MODULE %s", name)
		case "ENTITY":
			mdlCmd = fmt.Sprintf("DESCRIBE ENTITY %s", name)
		case "ASSOCIATION":
			mdlCmd = fmt.Sprintf("DESCRIBE ASSOCIATION %s", name)
		case "ENUMERATION":
			mdlCmd = fmt.Sprintf("DESCRIBE ENUMERATION %s", name)
		case "MICROFLOW":
			mdlCmd = fmt.Sprintf("DESCRIBE MICROFLOW %s", name)
		case "NANOFLOW":
			mdlCmd = fmt.Sprintf("DESCRIBE NANOFLOW %s", name)
		case "WORKFLOW":
			mdlCmd = fmt.Sprintf("DESCRIBE WORKFLOW %s", name)
		case "PAGE":
			mdlCmd = fmt.Sprintf("DESCRIBE PAGE %s", name)
		case "SNIPPET":
			mdlCmd = fmt.Sprintf("DESCRIBE SNIPPET %s", name)
		case "LAYOUT":
			mdlCmd = fmt.Sprintf("DESCRIBE LAYOUT %s", name)
		case "MODULEROLE", "MODULE ROLE":
			mdlCmd = fmt.Sprintf("DESCRIBE MODULE ROLE %s", name)
		case "USERROLE", "USER ROLE":
			mdlCmd = fmt.Sprintf("DESCRIBE USER ROLE '%s'", name)
		case "PROJECTSECURITY", "PROJECT SECURITY":
			mdlCmd = "SHOW PROJECT SECURITY"
		case "SETTINGS":
			mdlCmd = "DESCRIBE SETTINGS"
		case "DEMOUSER", "DEMO USER":
			mdlCmd = fmt.Sprintf("DESCRIBE DEMO USER '%s'", name)
		case "JAVAACTION", "JAVA ACTION":
			mdlCmd = fmt.Sprintf("DESCRIBE JAVA ACTION %s", name)
		case "CONSTANT":
			mdlCmd = fmt.Sprintf("DESCRIBE CONSTANT %s", name)
		case "JSONSTRUCTURE", "JSON STRUCTURE":
			mdlCmd = fmt.Sprintf("DESCRIBE JSON STRUCTURE %s", name)
		case "IMPORTMAPPING", "IMPORT MAPPING":
			mdlCmd = fmt.Sprintf("DESCRIBE IMPORT MAPPING %s", name)
		case "EXPORTMAPPING", "EXPORT MAPPING":
			mdlCmd = fmt.Sprintf("DESCRIBE EXPORT MAPPING %s", name)
		case "RESTCLIENT", "REST CLIENT":
			mdlCmd = fmt.Sprintf("DESCRIBE REST CLIENT %s", name)
		case "ODATACLIENT", "ODATA CLIENT":
			mdlCmd = fmt.Sprintf("DESCRIBE ODATA CLIENT %s", name)
		case "ODATASERVICE", "ODATA SERVICE":
			mdlCmd = fmt.Sprintf("DESCRIBE ODATA SERVICE %s", name)
		case "IMAGECOLLECTION", "IMAGE COLLECTION":
			mdlCmd = fmt.Sprintf("DESCRIBE IMAGE COLLECTION %s", name)
		case "BUSINESSEVENTSERVICE", "BUSINESS EVENT SERVICE":
			mdlCmd = fmt.Sprintf("DESCRIBE BUSINESS EVENT SERVICE %s", name)
		case "DATABASECONNECTION", "DATABASE CONNECTION":
			mdlCmd = fmt.Sprintf("DESCRIBE DATABASE CONNECTION %s", name)
		case "EXTERNALENTITY", "EXTERNAL ENTITY":
			mdlCmd = fmt.Sprintf("DESCRIBE ENTITY %s", name)
		case "NAVIGATION":
			mdlCmd = fmt.Sprintf("DESCRIBE NAVIGATION %s", name)
		case "NAVPROFILE":
			mdlCmd = fmt.Sprintf("DESCRIBE NAVIGATION %s", name)
		case "AGENT":
			mdlCmd = fmt.Sprintf("DESCRIBE AGENT %s", name)
		case "AIMODEL", "AI MODEL", "MODEL":
			mdlCmd = fmt.Sprintf("DESCRIBE MODEL %s", name)
		case "KNOWLEDGEBASE", "KNOWLEDGE BASE":
			mdlCmd = fmt.Sprintf("DESCRIBE KNOWLEDGE BASE %s", name)
		case "CONSUMEDMCPSERVICE", "CONSUMED MCP SERVICE", "MCP SERVICE":
			mdlCmd = fmt.Sprintf("DESCRIBE CONSUMED MCP SERVICE %s", name)
		case "DATATRANSFORMER", "DATA TRANSFORMER":
			mdlCmd = fmt.Sprintf("DESCRIBE DATA TRANSFORMER %s", name)
		case "SYSTEMOVERVIEW":
			mdlCmd = "" // handled directly by format-specific path
		default:
			fmt.Fprintf(os.Stderr, "Unknown type: %s\n", strings.Join(args[:len(args)-1], " "))
			fmt.Fprintln(os.Stderr, "Valid types: module, entity, association, enumeration, constant, microflow, nanoflow, workflow, page, snippet, layout, javaaction, jsonstructure, importmapping, exportmapping, restclient, odataclient, odataservice, imagecollection, businesseventservice, databaseconnection, agent, aimodel, knowledgebase, consumedmcpservice, datatransformer, modulerole, userrole, projectsecurity, settings, demouser, navigation, systemoverview")
			fmt.Fprintln(os.Stderr, "Multi-word types also accepted: json structure, import mapping, export mapping, rest client, image collection, business event service, agent, model, knowledge base, consumed mcp service, data transformer, etc.")
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()
		exec.SetQuiet(true) // suppress status messages for programmatic output

		// Connect
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		// Check for format overrides - bypass MDL parser for mermaid/elk, set executor for json
		format := resolveFormat(cmd, "mdl")
		if format == "json" {
			exec.SetFormat(executor.FormatJSON)
		}
		typeArg := strings.Join(args[:len(args)-1], " ")
		if format == "mermaid" {
			if err := exec.DescribeMermaid(typeArg, name); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		} else if format == "elk" {
			upper := objectType
			if upper == "SYSTEMOVERVIEW" {
				if err := exec.ModuleOverview(); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			} else if upper == "ENTITY" || upper == "DOMAINMODEL" || upper == "EXTERNALENTITY" || upper == "EXTERNAL ENTITY" {
				if err := exec.DomainModelELK(name); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			} else if upper == "MICROFLOW" {
				if err := exec.MicroflowELK(name); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			} else if upper == "NANOFLOW" {
				if err := exec.NanoflowELK(name); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			} else if upper == "PAGE" {
				if err := exec.PageWireframeJSON(name); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			} else if upper == "SNIPPET" {
				if err := exec.SnippetWireframeJSON(name); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintf(os.Stderr, "ELK format not supported for type: %s\n", typeArg)
				os.Exit(1)
			}
			return
		}

		// SYSTEMOVERVIEW requires elk format
		if mdlCmd == "" {
			fmt.Fprintf(os.Stderr, "Type %s requires --format elk\n", args[0])
			os.Exit(1)
		}

		// Execute describe command
		descProg, errs := visitor.Build(mdlCmd)
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
			}
			os.Exit(1)
		}

		for _, stmt := range descProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

// objectTypeToDescribe maps a catalog `objects` view ObjectType to the `describe`
// keyword. Only directly describable types with a `Module.Name` shape are listed.
var objectTypeToDescribe = map[string]string{
	"MODULE":                 "module",
	"ENTITY":                 "entity",
	"EXTERNAL_ENTITY":        "entity",
	"ASSOCIATION":            "association",
	"MICROFLOW":              "microflow",
	"NANOFLOW":               "nanoflow",
	"PAGE":                   "page",
	"SNIPPET":                "snippet",
	"LAYOUT":                 "layout",
	"ENUMERATION":            "enumeration",
	"CONSTANT":               "constant",
	"JAVA_ACTION":            "javaaction",
	"WORKFLOW":               "workflow",
	"JSON_STRUCTURE":         "jsonstructure",
	"IMPORT_MAPPING":         "importmapping",
	"EXPORT_MAPPING":         "exportmapping",
	"ODATA_CLIENT":           "odataclient",
	"ODATA_SERVICE":          "odataservice",
	"REST_CLIENT":            "restclient",
	"BUSINESS_EVENT_SERVICE": "businesseventservice",
	"DATABASE_CONNECTION":    "databaseconnection",
	"IMAGE_COLLECTION":       "imagecollection",
	"DATA_TRANSFORMER":       "datatransformer",
	"AGENT":                  "agent",
	"AI_MODEL":               "model",
	"KNOWLEDGE_BASE":         "knowledgebase",
	"CONSUMED_MCP_SERVICE":   "consumedmcpservice",
	// JAVASCRIPT_ACTION is cataloged but has no `describe` CLI form, so it is
	// intentionally omitted from auto-detect.
}

// unitTypeToDescribe maps a top-level unit's BSON $Type to the `describe`
// keyword. Used by the live-reader fallback. Only directly describable document
// types are listed (page templates, building blocks, rules, etc. are absent so
// they don't resolve).
var unitTypeToDescribe = map[string]string{
	"Microflows$Microflow":         "microflow",
	"Microflows$Nanoflow":          "nanoflow",
	"Forms$Page":                   "page",
	"Forms$Snippet":                "snippet",
	"Forms$Layout":                 "layout",
	"Enumerations$Enumeration":     "enumeration",
	"Constants$Constant":           "constant",
	"JavaActions$JavaAction":       "javaaction",
	"JsonStructures$JsonStructure": "jsonstructure",
	"ImportMappings$ImportMapping": "importmapping",
	"ExportMappings$ExportMapping": "exportmapping",
	"Images$ImageCollection":       "imagecollection",
	"Workflows$Workflow":           "workflow",
}

// resolveDescribeType auto-detects the `describe` type for a qualified document
// name. It prefers the catalog cache (an O(1) index, when `.mxcli/catalog.db`
// exists) and falls back to a live project scan when the catalog is absent or
// has no entry for the name (e.g. a document added since the last build). It
// returns the single matching type, the candidate list when the name is
// ambiguous (an entity and a document can share a name), or an error when
// nothing matches.
func resolveDescribeType(projectPath, name string) (string, []string, error) {
	matches := resolveViaCatalog(projectPath, name)
	if len(matches) == 0 {
		var err error
		matches, err = resolveViaReader(projectPath, name)
		if err != nil {
			return "", nil, err
		}
	}
	return chooseDescribeType(name, matches)
}

// chooseDescribeType collapses raw matches (which may contain duplicates and
// empty strings) into a single type, a candidate list, or a not-found error.
func chooseDescribeType(name string, matches []string) (string, []string, error) {
	seen := map[string]bool{}
	var uniq []string
	for _, m := range matches {
		if m != "" && !seen[m] {
			seen[m] = true
			uniq = append(uniq, m)
		}
	}
	switch len(uniq) {
	case 0:
		return "", nil, fmt.Errorf("no describable document named %q found in the project", name)
	case 1:
		return uniq[0], nil, nil
	default:
		return "", uniq, nil
	}
}

// resolveViaCatalog looks the name up in the catalog cache. Returns nil (so the
// caller falls back to a live scan) when the catalog is missing, unreadable, or
// has no matching row. Best-effort: any error is treated as a miss.
func resolveViaCatalog(projectPath, name string) []string {
	dbPath := filepath.Join(filepath.Dir(projectPath), ".mxcli", "catalog.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	cat, err := catalog.NewFromFile(dbPath)
	if err != nil {
		return nil
	}
	defer cat.Close()

	// Only trust a cache that matches the current project file. A stale cache
	// (the .mpr changed since it was built) could resolve a renamed/deleted name
	// to the wrong type, so fall back to the authoritative live scan instead.
	if !catalogMatchesProject(cat, projectPath) {
		return nil
	}

	q := "'" + strings.ReplaceAll(name, "'", "''") + "'"
	var out []string
	// The objects view is a complete index (associations included as of catalog
	// schema v3), so a single query covers every auto-detectable type.
	if res, err := cat.Query("SELECT DISTINCT ObjectType FROM objects WHERE QualifiedName = " + q); err == nil {
		for _, row := range res.Rows {
			if len(row) > 0 {
				out = append(out, objectTypeToDescribe[fmt.Sprintf("%v", row[0])])
			}
		}
	}
	return out
}

// catalogMatchesProject reports whether the cached catalog was built from the
// current project file and is still up to date — same MPR path and unchanged
// modification time (Unix seconds, mirroring isCacheValid). Any read failure or
// mismatch is treated as "not fresh" so the caller live-scans (safe but slower).
func catalogMatchesProject(cat *catalog.Catalog, projectPath string) bool {
	info, err := cat.GetCacheInfo()
	if err != nil {
		return false
	}
	fi, err := os.Stat(projectPath)
	if err != nil {
		return false
	}
	if info.MprModTime.Unix() != fi.ModTime().Unix() {
		return false
	}
	// Compare absolute paths when both resolve; a path mismatch means the cache
	// is for a different .mpr in the same directory.
	if info.MprPath != "" {
		a, err1 := filepath.Abs(info.MprPath)
		b, err2 := filepath.Abs(projectPath)
		if err1 == nil && err2 == nil && a != b {
			return false
		}
	}
	return true
}

// resolveViaReader scans the live project for the name. Authoritative but slower
// than the catalog (it enumerates documents). Covers top-level documents plus
// entities and associations (including cross-module associations).
func resolveViaReader(projectPath, name string) ([]string, error) {
	reader, err := modelsdk.Open(projectPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var out []string
	// Top-level documents (microflow, page, snippet, enumeration, ...).
	if units, err := reader.ListRawUnits(""); err == nil {
		for _, u := range units {
			if u.QualifiedName == name {
				out = append(out, unitTypeToDescribe[u.Type])
			}
		}
	}

	// Entities and associations live inside the domain model, keyed by module.
	moduleName := map[string]string{}
	if modules, err := reader.ListModules(); err == nil {
		for _, m := range modules {
			moduleName[string(m.ID)] = m.Name
		}
	}
	if dms, err := reader.ListDomainModels(); err == nil {
		for _, dm := range dms {
			mn := moduleName[string(dm.ContainerID)]
			for _, e := range dm.Entities {
				if mn+"."+e.Name == name {
					out = append(out, "entity")
				}
			}
			for _, a := range dm.Associations {
				if mn+"."+a.Name == name {
					out = append(out, "association")
				}
			}
			for _, a := range dm.CrossAssociations {
				if mn+"."+a.Name == name {
					out = append(out, "association")
				}
			}
		}
	}
	return out, nil
}
