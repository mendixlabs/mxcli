// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/security"
	"github.com/spf13/cobra"
)

var describeCmd = &cobra.Command{
	Use:   "describe [<type>] <name>",
	Short: "Describe a project element",
	Long: `Describe an element from a Mendix project in MDL syntax.

The description is written in mdl 1 and starts with the header "mdl 1;", so
it can be executed back as it is; descriptions concatenated into one file
repeat the header, which is allowed. --mdl 0 writes the alpha language
instead, with no header (ako/mxcli#840).

The <type> is optional for a qualified document name: 'describe MyModule.Customer'
auto-detects the document type. So it is for a module role ('describe
Administration.Administrator'), a user role and a navigation profile
('describe Responsive'). Pass the type explicitly to disambiguate (a name can
match both an entity and a document, for example) or for forms that have no
single name (module, settings).

Types:
  module           Describe a module and its module roles (--all: everything in it)
  entity           Describe an entity
  externalentity   Describe an external entity (alias for entity)
  association      Describe an association
  enumeration      Describe an enumeration
  constant         Describe a constant
  microflow        Describe a microflow
  nanoflow         Describe a nanoflow
  rule             Describe a rule
  workflow         Describe a workflow
  page             Describe a page
  snippet          Describe a snippet
  buildingblock    Describe a building block (read-only, also: "building block")
  layout           Describe a layout
  javaaction       Describe a java action
  javascriptaction Describe a JavaScript action (also: "javascript action")
  jsonstructure    Describe a JSON structure (also: "json structure")
  importmapping    Describe an import mapping (also: "import mapping")
  exportmapping    Describe an export mapping (also: "export mapping")
  consumedrestservice   Describe a consumed REST service (also: "consumed rest service"; old: restclient)
  consumedodataservice  Describe a consumed OData service (also: "consumed odata service"; old: odataclient)
  publishedodataservice Describe a published OData service (also: "published odata service"; old: odataservice)
  publishedrestservice  Describe a published REST service (also: "published rest service", "rest service")
  imagecollection  Describe an image collection (also: "image collection")
  menu             Describe a standalone menu document
  taskqueue        Describe a task queue (also: "task queue"; old: queue)
  scheduledevent   Describe a scheduled event (also: "scheduled event")
  regularexpression  Describe a regular expression (also: "regular expression")
  businesseventservice  Describe a business event service (also: "business event service")
  databaseconnection    Describe a database connection (also: "database connection")
  agent            Describe an AI agent (also: "agent")
  aimodel          Describe an AI model (also: "ai model"; old: model)
  knowledgebase    Describe a knowledge base (also: "knowledge base")
  consumedmcpservice  Describe a consumed MCP service (also: "consumed mcp service")
  publishedmcpservice Describe a published MCP service (also: "published mcp service")
  datatransformer  Describe a data transformer (also: "data transformer")
  modulerole       Describe a module role
  userrole         Describe a user role
  appsecurity      Describe app security settings (also: "app security"; old: projectsecurity)
  settings         Describe project settings
  demouser         Describe a demo user
  navigation       Describe a navigation profile
  systemoverview   Module dependency graph (requires --format elk)
  domainmodel      A module's domain model as a diagram (--format svg or elk; the name is the module)

Example:
  mxcli describe -p app.mpr MyModule.Customer          # type auto-detected
  mxcli describe -p app.mpr MyModule.ProcessOrder      # type auto-detected
  mxcli describe -p app.mpr module MyModule
  mxcli describe -p app.mpr --all module MyModule    # DESCRIBE MODULE ... WITH ALL
  mxcli describe -p app.mpr entity MyModule.Customer
  mxcli describe -p app.mpr microflow MyModule.ProcessOrder
  mxcli describe -p app.mpr --mdl 0 microflow MyModule.ProcessOrder
  mxcli describe -p app.mpr nanoflow MyModule.ValidateInput
  mxcli describe -p app.mpr page MyModule.Customer_Overview
  mxcli describe -p app.mpr json structure MyModule.CustomerResponse
  mxcli describe -p app.mpr import mapping MyModule.IMM_Customer
  mxcli describe -p app.mpr export mapping MyModule.EMM_Customer
  mxcli describe -p app.mpr consumed rest service MyModule.PetStoreAPI
  mxcli describe -p app.mpr consumed odata service MyModule.CrmApi
  mxcli describe -p app.mpr task queue MyModule.EmailQueue
  mxcli describe -p app.mpr settings Settings
  mxcli describe -p app.mpr navigation Responsive
  mxcli describe -p app.mpr --format elk systemoverview SystemOverview
  mxcli describe -p app.mpr --format svg domainmodel MyModule
  mxcli describe -p app.mpr agent MyModule.MyAgent
  mxcli describe -p app.mpr ai model MyModule.GPT4
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

		mdlCmd, ok := describeMDLCommand(objectType, name)
		if !ok {
			fmt.Fprintf(os.Stderr, "Unknown type: %s\n", strings.Join(args[:len(args)-1], " "))
			fmt.Fprintln(os.Stderr, "Valid types: module, entity, association, enumeration, constant, microflow, nanoflow, rule, workflow, page, snippet, layout, javaaction, javascriptaction, jsonstructure, importmapping, exportmapping, consumedrestservice, consumedodataservice, publishedodataservice, publishedrestservice, imagecollection, menu, taskqueue, scheduledevent, businesseventservice, databaseconnection, agent, aimodel, knowledgebase, consumedmcpservice, publishedmcpservice, datatransformer, modulerole, userrole, appsecurity, settings, demouser, navigation, systemoverview")
			fmt.Fprintln(os.Stderr, "Multi-word types also accepted: json structure, import mapping, export mapping, consumed rest service, consumed odata service, published odata service, published rest service, task queue, app security, image collection, scheduled event, regular expression, business event service, ai model, knowledge base, consumed mcp service, published mcp service, data transformer, etc.")
			os.Exit(1)
		}
		all, _ := cmd.Flags().GetBool("all")
		mdlCmd, err := applyDescribeAll(objectType, mdlCmd, all)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()
		exec.SetQuiet(true) // suppress status messages for programmatic output
		exec.SetDescribeLanguage(mdlFlag(cmd))

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
		} else if format == "svg" {
			upper := objectType
			if upper == "DOMAINMODEL" || upper == "DOMAIN MODEL" || (upper == "ENTITY" && !strings.Contains(name, ".")) {
				if err := exec.DomainModelSVG(name); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				return
			}
			fmt.Fprintf(os.Stderr, "SVG format not supported for type: %s (supported: domainmodel <Module>)\n", typeArg)
			os.Exit(1)
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

		// SYSTEMOVERVIEW and DOMAINMODEL are diagrams with no MDL form
		if mdlCmd == "" {
			fmt.Fprintf(os.Stderr, "Type %s is a diagram: use --format elk (or --format svg for a domain model)\n", args[0])
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

// describeMDLCommand maps a `describe` subcommand type (upper-cased, words joined
// by single spaces) and a name to the MDL DESCRIBE statement it runs. ok is false
// for an unknown type. SYSTEMOVERVIEW returns "" with ok true: it has no MDL form
// and is handled by the --format elk path.
func describeMDLCommand(objectType, name string) (mdlCmd string, ok bool) {
	switch objectType {
	case "MODULE":
		return fmt.Sprintf("DESCRIBE MODULE %s", name), true
	case "ENTITY":
		return fmt.Sprintf("DESCRIBE ENTITY %s", name), true
	case "ASSOCIATION":
		return fmt.Sprintf("DESCRIBE ASSOCIATION %s", name), true
	case "ENUMERATION":
		return fmt.Sprintf("DESCRIBE ENUMERATION %s", name), true
	case "MICROFLOW":
		return fmt.Sprintf("DESCRIBE MICROFLOW %s", name), true
	case "NANOFLOW":
		return fmt.Sprintf("DESCRIBE NANOFLOW %s", name), true
	case "RULE":
		return fmt.Sprintf("DESCRIBE RULE %s", name), true
	case "WORKFLOW":
		return fmt.Sprintf("DESCRIBE WORKFLOW %s", name), true
	case "PAGE":
		return fmt.Sprintf("DESCRIBE PAGE %s", name), true
	case "SNIPPET":
		return fmt.Sprintf("DESCRIBE SNIPPET %s", name), true
	case "BUILDINGBLOCK", "BUILDING BLOCK":
		return fmt.Sprintf("DESCRIBE BUILDING BLOCK %s", name), true
	case "LAYOUT":
		return fmt.Sprintf("DESCRIBE LAYOUT %s", name), true
	case "MODULEROLE", "MODULE ROLE":
		return fmt.Sprintf("DESCRIBE MODULE ROLE %s", name), true
	case "USERROLE", "USER ROLE":
		return fmt.Sprintf("DESCRIBE USER ROLE '%s'", name), true
	case "APPSECURITY", "APP SECURITY", "PROJECTSECURITY", "PROJECT SECURITY":
		return "DESCRIBE APP SECURITY", true
	case "SETTINGS":
		return "DESCRIBE SETTINGS", true
	case "DEMOUSER", "DEMO USER":
		return fmt.Sprintf("DESCRIBE DEMO USER '%s'", name), true
	case "JAVAACTION", "JAVA ACTION":
		return fmt.Sprintf("DESCRIBE JAVA ACTION %s", name), true
	case "JAVASCRIPTACTION", "JAVASCRIPT ACTION":
		return fmt.Sprintf("DESCRIBE JAVASCRIPT ACTION %s", name), true
	case "CONSTANT":
		return fmt.Sprintf("DESCRIBE CONSTANT %s", name), true
	case "JSONSTRUCTURE", "JSON STRUCTURE":
		return fmt.Sprintf("DESCRIBE JSON STRUCTURE %s", name), true
	case "IMPORTMAPPING", "IMPORT MAPPING":
		return fmt.Sprintf("DESCRIBE IMPORT MAPPING %s", name), true
	case "EXPORTMAPPING", "EXPORT MAPPING":
		return fmt.Sprintf("DESCRIBE EXPORT MAPPING %s", name), true
	case "CONSUMEDRESTSERVICE", "CONSUMED REST SERVICE", "RESTCLIENT", "REST CLIENT":
		return fmt.Sprintf("DESCRIBE CONSUMED REST SERVICE %s", name), true
	case "CONSUMEDODATASERVICE", "CONSUMED ODATA SERVICE", "ODATACLIENT", "ODATA CLIENT":
		return fmt.Sprintf("DESCRIBE CONSUMED ODATA SERVICE %s", name), true
	case "PUBLISHEDODATASERVICE", "PUBLISHED ODATA SERVICE", "ODATASERVICE", "ODATA SERVICE":
		return fmt.Sprintf("DESCRIBE PUBLISHED ODATA SERVICE %s", name), true
	case "PUBLISHEDRESTSERVICE", "PUBLISHED REST SERVICE", "RESTSERVICE", "REST SERVICE":
		return fmt.Sprintf("DESCRIBE PUBLISHED REST SERVICE %s", name), true
	case "IMAGECOLLECTION", "IMAGE COLLECTION":
		return fmt.Sprintf("DESCRIBE IMAGE COLLECTION %s", name), true
	case "MENU":
		return fmt.Sprintf("DESCRIBE MENU %s", name), true
	case "TASKQUEUE", "TASK QUEUE", "QUEUE":
		return fmt.Sprintf("DESCRIBE TASK QUEUE %s", name), true
	case "SCHEDULEDEVENT", "SCHEDULED EVENT":
		return fmt.Sprintf("DESCRIBE SCHEDULED EVENT %s", name), true
	case "REGULAREXPRESSION", "REGULAR EXPRESSION", "REGEX":
		return fmt.Sprintf("DESCRIBE REGULAR EXPRESSION %s", name), true
	case "BUSINESSEVENTSERVICE", "BUSINESS EVENT SERVICE":
		return fmt.Sprintf("DESCRIBE BUSINESS EVENT SERVICE %s", name), true
	case "DATABASECONNECTION", "DATABASE CONNECTION":
		return fmt.Sprintf("DESCRIBE DATABASE CONNECTION %s", name), true
	case "EXTERNALENTITY", "EXTERNAL ENTITY":
		return fmt.Sprintf("DESCRIBE ENTITY %s", name), true
	case "NAVIGATION":
		// "Navigation" is project-tree's name for the navigation document
		// itself, not a profile (profiles are named by kind: Responsive,
		// Phone, ...), so it describes every profile.
		if strings.EqualFold(name, "Navigation") {
			return "DESCRIBE NAVIGATION", true
		}
		return fmt.Sprintf("DESCRIBE NAVIGATION %s", name), true
	case "NAVPROFILE":
		return fmt.Sprintf("DESCRIBE NAVIGATION %s", name), true
	case "AGENT":
		return fmt.Sprintf("DESCRIBE AGENT %s", name), true
	case "AIMODEL", "AI MODEL", "MODEL":
		return fmt.Sprintf("DESCRIBE AI MODEL %s", name), true
	case "KNOWLEDGEBASE", "KNOWLEDGE BASE":
		return fmt.Sprintf("DESCRIBE KNOWLEDGE BASE %s", name), true
	case "CONSUMEDMCPSERVICE", "CONSUMED MCP SERVICE", "MCP SERVICE":
		return fmt.Sprintf("DESCRIBE CONSUMED MCP SERVICE %s", name), true
	case "PUBLISHEDMCPSERVICE", "PUBLISHED MCP SERVICE":
		return fmt.Sprintf("DESCRIBE PUBLISHED MCP SERVICE %s", name), true
	case "DATATRANSFORMER", "DATA TRANSFORMER":
		return fmt.Sprintf("DESCRIBE DATA TRANSFORMER %s", name), true
	case "SYSTEMOVERVIEW", "DOMAINMODEL", "DOMAIN MODEL":
		return "", true // diagrams only: handled by the --format elk / svg paths
	}
	return "", false
}

// applyDescribeAll applies the --all flag: for a module it describes everything
// in it (DESCRIBE MODULE ... WITH ALL), where a plain describe module prints the
// module and its roles only (mendixlabs/mxcli#1435). --all on any other type is
// an error rather than a flag silently ignored.
func applyDescribeAll(objectType, mdlCmd string, all bool) (string, error) {
	if !all {
		return mdlCmd, nil
	}
	if objectType != "MODULE" {
		return "", fmt.Errorf("--all applies to describe module only")
	}
	return mdlCmd + " WITH ALL", nil
}

// objectTypeToDescribe maps a catalog `objects` view ObjectType to the `describe`
// keyword. Only directly describable types with a `Module.Name` shape are listed.
var objectTypeToDescribe = map[string]string{
	"MODULE":                  "module",
	"ENTITY":                  "entity",
	"EXTERNAL_ENTITY":         "entity",
	"ASSOCIATION":             "association",
	"MICROFLOW":               "microflow",
	"NANOFLOW":                "nanoflow",
	"RULE":                    "rule",
	"PAGE":                    "page",
	"SNIPPET":                 "snippet",
	"LAYOUT":                  "layout",
	"ENUMERATION":             "enumeration",
	"CONSTANT":                "constant",
	"JAVA_ACTION":             "javaaction",
	"WORKFLOW":                "workflow",
	"JSON_STRUCTURE":          "jsonstructure",
	"IMPORT_MAPPING":          "importmapping",
	"EXPORT_MAPPING":          "exportmapping",
	"CONSUMED_ODATA_SERVICE":  "consumedodataservice",
	"PUBLISHED_ODATA_SERVICE": "publishedodataservice",
	"CONSUMED_REST_SERVICE":   "consumedrestservice",
	"TASK_QUEUE":              "taskqueue",
	// Pre-#1395 ObjectType values, for a catalog cached by an older build.
	"ODATA_CLIENT":           "consumedodataservice",
	"ODATA_SERVICE":          "publishedodataservice",
	"REST_CLIENT":            "consumedrestservice",
	"QUEUE":                  "taskqueue",
	"PUBLISHED_REST_SERVICE": "publishedrestservice",
	"BUSINESS_EVENT_SERVICE": "businesseventservice",
	"DATABASE_CONNECTION":    "databaseconnection",
	"IMAGE_COLLECTION":       "imagecollection",
	"MENU":                   "menu",
	"DATA_TRANSFORMER":       "datatransformer",
	"AGENT":                  "agent",
	"AI_MODEL":               "aimodel",
	"KNOWLEDGE_BASE":         "knowledgebase",
	"CONSUMED_MCP_SERVICE":   "consumedmcpservice",
	"PUBLISHED_MCP_SERVICE":  "publishedmcpservice",
	"JAVASCRIPT_ACTION":      "javascriptaction",
}

// unitTypeToDescribe maps a top-level unit's BSON $Type to the `describe`
// keyword. Used by the live-reader fallback. Only directly describable document
// types are listed (page templates etc. are absent so they don't resolve).
var unitTypeToDescribe = map[string]string{
	"Microflows$Microflow":                 "microflow",
	"Microflows$Nanoflow":                  "nanoflow",
	"Microflows$Rule":                      "rule",
	"Forms$Page":                           "page",
	"Forms$Snippet":                        "snippet",
	"Pages$BuildingBlock":                  "buildingblock",
	"Forms$BuildingBlock":                  "buildingblock",
	"Forms$Layout":                         "layout",
	"Enumerations$Enumeration":             "enumeration",
	"Constants$Constant":                   "constant",
	"JavaActions$JavaAction":               "javaaction",
	"JavaScriptActions$JavaScriptAction":   "javascriptaction",
	"JsonStructures$JsonStructure":         "jsonstructure",
	"ImportMappings$ImportMapping":         "importmapping",
	"ExportMappings$ExportMapping":         "exportmapping",
	"Images$ImageCollection":               "imagecollection",
	"Menus$MenuDocument":                   "menu",
	"Workflows$Workflow":                   "workflow",
	"Queues$Queue":                         "taskqueue",
	"ScheduledEvents$ScheduledEvent":       "scheduledevent",
	"RegularExpressions$RegularExpression": "regularexpression",
	"Rest$PublishedRestService":            "publishedrestservice",
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
	defer reader.Disconnect()

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
	out = append(out, describableMembers(reader, moduleName, name)...)
	return out, nil
}

// describableMembers is the `describe` type of each member named name that
// is not a document of its own: a module role (Module.Role), a user role and
// a navigation profile (both bare names). Each has a describe type, but the
// auto-detect saw only documents, so `describe Administration.Administrator`
// answered "no describable document named …" for a role `describe module
// role` describes (ako/mxcli#1262).
//
// The catalog's objects view does not list them, and has no complete table to
// list module roles from (role_mappings holds only the mapped ones), so these
// are resolved here, by the live scan a catalog miss falls back to.
func describableMembers(reader memberReader, moduleName map[string]string, name string) []string {
	var out []string
	if mss, err := reader.ListModuleSecurity(); err == nil {
		for _, ms := range mss {
			mn := moduleName[string(ms.ContainerID)]
			for _, r := range ms.ModuleRoles {
				if mn+"."+r.Name == name {
					out = append(out, "modulerole")
				}
			}
		}
	}
	if ps, err := reader.GetProjectSecurity(); err == nil && ps != nil {
		for _, r := range ps.UserRoles {
			if r.Name == name {
				out = append(out, "userrole")
			}
		}
	}
	if nav, err := reader.GetNavigation(); err == nil && nav != nil {
		for _, p := range nav.Profiles {
			if p.Name == name {
				out = append(out, "navigation")
			}
		}
	}
	return out
}

// memberReader is the part of the backend describableMembers reads.
type memberReader interface {
	ListModuleSecurity() ([]*security.ModuleSecurity, error)
	GetProjectSecurity() (*security.ProjectSecurity, error)
	GetNavigation() (*types.NavigationDocument, error)
}
