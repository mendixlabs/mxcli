// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// objectTypeToDescribeKind maps a catalog `objects` view ObjectType to the
// concrete DescribeObjectType used by bare `DESCRIBE Module.Name` auto-detection.
// It covers every type the executor has a describe handler for. (The `mxcli
// describe` CLI uses a parallel string map in cmd/mxcli; this one targets the AST
// kind directly and additionally includes JAVASCRIPT_ACTION / PUBLISHED_REST_SERVICE,
// which the executor can describe but the CLI dispatch does not wire.)
var objectTypeToDescribeKind = map[string]ast.DescribeObjectType{
	"MODULE":                  ast.DescribeModule,
	"ENTITY":                  ast.DescribeEntity,
	"EXTERNAL_ENTITY":         ast.DescribeExternalEntity,
	"ASSOCIATION":             ast.DescribeAssociation,
	"MICROFLOW":               ast.DescribeMicroflow,
	"NANOFLOW":                ast.DescribeNanoflow,
	"RULE":                    ast.DescribeRule,
	"PAGE":                    ast.DescribePage,
	"SNIPPET":                 ast.DescribeSnippet,
	"BUILDING_BLOCK":          ast.DescribeBuildingBlock,
	"MENU":                    ast.DescribeMenu,
	"TASK_QUEUE":              ast.DescribeQueue,
	"QUEUE":                   ast.DescribeQueue, // pre-#1395 catalog
	"SCHEDULED_EVENT":         ast.DescribeScheduledEvent,
	"REGULAR_EXPRESSION":      ast.DescribeRegularExpression,
	"LAYOUT":                  ast.DescribeLayout,
	"ENUMERATION":             ast.DescribeEnumeration,
	"CONSTANT":                ast.DescribeConstant,
	"JAVA_ACTION":             ast.DescribeJavaAction,
	"JAVASCRIPT_ACTION":       ast.DescribeJavaScriptAction,
	"WORKFLOW":                ast.DescribeWorkflow,
	"JSON_STRUCTURE":          ast.DescribeJsonStructure,
	"IMPORT_MAPPING":          ast.DescribeImportMapping,
	"EXPORT_MAPPING":          ast.DescribeExportMapping,
	"CONSUMED_ODATA_SERVICE":  ast.DescribeODataClient,
	"PUBLISHED_ODATA_SERVICE": ast.DescribeODataService,
	"CONSUMED_REST_SERVICE":   ast.DescribeRestClient,
	// The ObjectType values before mendixlabs/mxcli#1395, which a catalog
	// cached by an older build still holds.
	"ODATA_CLIENT":                  ast.DescribeODataClient,
	"ODATA_SERVICE":                 ast.DescribeODataService,
	"REST_CLIENT":                   ast.DescribeRestClient,
	"PUBLISHED_REST_SERVICE":        ast.DescribePublishedRestService,
	"BUSINESS_EVENT_SERVICE":        ast.DescribeBusinessEventService,
	"DATABASE_CONNECTION":           ast.DescribeDatabaseConnection,
	"IMAGE_COLLECTION":              ast.DescribeImageCollection,
	"ICON_COLLECTION":               ast.DescribeIconCollection,
	"DATA_TRANSFORMER":              ast.DescribeDataTransformer,
	"MESSAGE_DEFINITION_COLLECTION": ast.DescribeMessageDefinitionCollection,
	"MESSAGE_DEFINITION":            ast.DescribeMessageDefinition,
	"AGENT":                         ast.DescribeAgent,
	"AI_MODEL":                      ast.DescribeModel,
	"KNOWLEDGE_BASE":                ast.DescribeKnowledgeBase,
	"CONSUMED_MCP_SERVICE":          ast.DescribeConsumedMCPService,
	"PUBLISHED_MCP_SERVICE":         ast.DescribePublishedMCPService,
}

// DescribeKindFor maps a catalog `objects` view ObjectType to the DESCRIBE kind
// that renders it, reporting false for a type with no describe handler.
//
// Exported so callers outside the executor (the marketplace differ, which walks
// a whole module) resolve types through the same table bare DESCRIBE uses,
// rather than keeping a fourth copy of this mapping. Three copies already exist
// — this map, the catalog's objects view, and cmd/mxcli's own dispatch — and
// they have drifted apart once each.
func DescribeKindFor(objectType string) (ast.DescribeObjectType, bool) {
	kind, ok := objectTypeToDescribeKind[objectType]
	return kind, ok
}

// resolveDescribeAuto auto-detects the type of a bare `DESCRIBE Module.Name`
// against the connected project. It builds (fast mode) the catalog if needed and
// looks the qualified name up in the `objects` index — a complete index for every
// describable top-level document type (#658). Because the catalog is built fresh
// from the connected project here, there is no staleness concern (unlike the CLI's
// on-disk cache path). Returns the single matching kind, or an actionable error
// when nothing matches or the name is ambiguous.
func resolveDescribeAuto(ctx *ExecContext, name string) (ast.DescribeObjectType, error) {
	if err := ensureCatalog(ctx, false); err != nil {
		return 0, fmt.Errorf("auto-detecting the type of %q: %w", name, err)
	}

	q := "'" + strings.ReplaceAll(name, "'", "''") + "'"
	res, err := ctx.Catalog.Query("SELECT DISTINCT ObjectType FROM objects WHERE QualifiedName = " + q)
	if err != nil {
		return 0, fmt.Errorf("auto-detecting the type of %q: %w", name, err)
	}

	seen := map[ast.DescribeObjectType]bool{}
	var kinds []ast.DescribeObjectType
	var candidates []string
	for _, row := range res.Rows {
		if len(row) == 0 {
			continue
		}
		kind, ok := objectTypeToDescribeKind[fmt.Sprintf("%v", row[0])]
		if !ok || seen[kind] {
			continue
		}
		seen[kind] = true
		kinds = append(kinds, kind)
		candidates = append(candidates, kind.String())
	}

	switch len(kinds) {
	case 0:
		return 0, fmt.Errorf("no describable document named %q found in the project; "+
			"specify the type explicitly, e.g. 'describe <type> %s'", name, name)
	case 1:
		return kinds[0], nil
	default:
		sort.Strings(candidates)
		return 0, fmt.Errorf("%q is ambiguous — it matches: %s; "+
			"specify the type, e.g. 'describe %s %s'",
			name, strings.Join(candidates, ", "), strings.ToLower(candidates[0]), name)
	}
}
