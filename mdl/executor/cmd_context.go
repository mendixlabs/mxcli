// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// execShowContext handles SHOW CONTEXT OF <name> [DEPTH n] command.
// It assembles relevant context information for LLM consumption.
func execShowContext(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("show context requires a qualified name")
	}

	// Ensure catalog is built with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return mdlerrors.NewBackend("build catalog", err)
	}

	name := s.Name.String()
	depth := s.Depth
	if depth <= 0 {
		depth = 2
	}

	// Detect the type of the target element
	targetType, err := detectElementType(ctx, name)
	if err != nil {
		return err
	}

	// Assemble context based on type
	var output strings.Builder
	output.WriteString(fmt.Sprintf("## Context: %s\n\n", name))

	switch targetType {
	case "microflow", "nanoflow":
		assembleMicroflowContext(ctx, &output, name, depth)
	case "entity":
		assembleEntityContext(ctx, &output, name, depth)
	case "page":
		assemblePageContext(ctx, &output, name, depth)
	case "enumeration":
		assembleEnumerationContext(ctx, &output, name)
	case "workflow":
		assembleWorkflowContext(ctx, &output, name, depth)
	case "snippet":
		assembleSnippetContext(ctx, &output, name, depth)
	case "javaaction":
		assembleJavaActionContext(ctx, &output, name)
	case "odataclient":
		assembleODataClientContext(ctx, &output, name)
	case "odataservice":
		assembleODataServiceContext(ctx, &output, name)
	default:
		output.WriteString(fmt.Sprintf("Unknown element type for: %s\n", name))
	}

	fmt.Fprint(ctx.Output, output.String())
	return nil
}

// detectElementType determines what kind of element the name refers to.
func detectElementType(ctx *ExecContext, name string) (string, error) {
	// Check catalog tables for known element types
	catalogChecks := []struct {
		table    string
		elemType string
	}{
		{"microflows", "microflow"},
		{"entities", "entity"},
		{"pages", "page"},
		{"enumerations", "enumeration"},
		{"snippets", "snippet"},
		{"workflows", "workflow"},
		{"java_actions", "javaaction"},
		{"odata_clients", "odataclient"},
		{"odata_services", "odataservice"},
	}

	for _, check := range catalogChecks {
		result, err := ctx.Catalog.Query(fmt.Sprintf(
			"select 1 from %s where QualifiedName = '%s' limit 1", check.table, name))
		if err == nil && result.Count > 0 {
			return check.elemType, nil
		}
	}

	return "", mdlerrors.NewNotFound("element", name)
}

// assembleMicroflowContext assembles context for a microflow.
func assembleMicroflowContext(ctx *ExecContext, out *strings.Builder, name string, depth int) {
	// Get microflow basic info
	out.WriteString("### Microflow Definition\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"select Name, ReturnType, ParameterCount, ActivityCount from microflows where QualifiedName = '%s'", name))
	if err == nil && result.Count > 0 {
		row := result.Rows[0]
		out.WriteString(fmt.Sprintf("- **Name**: %v\n", row[0]))
		out.WriteString(fmt.Sprintf("- **Return Type**: %v\n", row[1]))
		out.WriteString(fmt.Sprintf("- **Parameters**: %v\n", row[2]))
		out.WriteString(fmt.Sprintf("- **Activities**: %v\n", row[3]))
	}
	out.WriteString("\n")

	// Entities used by this microflow
	out.WriteString("### Entities Used\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName, RefKind from refs
		 where SourceName = '%s' and TargetType = 'ENTITY'
		 ORDER by RefKind, TargetName`, name))
	if err == nil && result.Count > 0 {
		out.WriteString("| Entity | Usage |\n")
		out.WriteString("|--------|-------|\n")
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("| %v | %v |\n", row[0], row[1]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Pages shown by this microflow
	out.WriteString("### Pages Shown\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName from refs
		 where SourceName = '%s' and RefKind = 'show_page'
		 ORDER by TargetName`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none)\n")
	}
	out.WriteString("\n")

	// Called microflows (with depth)
	out.WriteString(fmt.Sprintf("### Called Microflows (depth %d)\n\n", depth))
	if depth > 0 {
		addCallees(ctx, out, name, depth, 1)
	}
	out.WriteString("\n")

	// Direct callers
	out.WriteString("### Direct Callers\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select SourceName from refs
		 where TargetName = '%s' and RefKind = 'call'
		 ORDER by SourceName limit 10`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
		if result.Count == 10 {
			out.WriteString("- ... (more callers exist)\n")
		}
	} else {
		out.WriteString("(none)\n")
	}
}

// addCallees recursively adds callees up to the specified depth.
func addCallees(ctx *ExecContext, out *strings.Builder, name string, maxDepth, currentDepth int) {
	if currentDepth > maxDepth {
		return
	}

	indent := strings.Repeat("  ", currentDepth-1)
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName from refs
		 where SourceName = '%s' and RefKind = 'call'
		 ORDER by TargetName`, name))
	if err != nil || result.Count == 0 {
		return
	}

	for _, row := range result.Rows {
		callee := fmt.Sprintf("%v", row[0])
		out.WriteString(fmt.Sprintf("%s- %s\n", indent, callee))
		// Recurse for deeper levels
		if currentDepth < maxDepth {
			addCallees(ctx, out, callee, maxDepth, currentDepth+1)
		}
	}
}

// assembleEntityContext assembles context for an entity.
func assembleEntityContext(ctx *ExecContext, out *strings.Builder, name string, depth int) {
	// Get entity basic info
	out.WriteString("### Entity Definition\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"select Name, EntityType, Generalization, AttributeCount from entities where QualifiedName = '%s'", name))
	if err == nil && result.Count > 0 {
		row := result.Rows[0]
		out.WriteString(fmt.Sprintf("- **Name**: %v\n", row[0]))
		out.WriteString(fmt.Sprintf("- **Type**: %v\n", row[1]))
		if row[2] != nil && row[2] != "" {
			out.WriteString(fmt.Sprintf("- **Extends**: %v\n", row[2]))
		}
		out.WriteString(fmt.Sprintf("- **Attributes**: %v\n", row[3]))
	}
	out.WriteString("\n")

	// Microflows that use this entity
	out.WriteString("### Microflows Using This Entity\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct SourceName, RefKind from refs
		 where TargetName = '%s' and SourceType = 'MICROFLOW'
		 ORDER by RefKind, SourceName limit 20`, name))
	if err == nil && result.Count > 0 {
		out.WriteString("| Microflow | Usage |\n")
		out.WriteString("|-----------|-------|\n")
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("| %v | %v |\n", row[0], row[1]))
		}
		if result.Count == 20 {
			out.WriteString("\n(limited to 20 results)\n")
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Pages displaying this entity
	out.WriteString("### Pages Displaying This Entity\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct SourceName from refs
		 where TargetName = '%s' and SourceType = 'PAGE'
		 ORDER by SourceName limit 10`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Related entities (via associations or generalization)
	out.WriteString("### Related Entities\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName, RefKind from refs
		 where SourceName = '%s' and TargetType = 'ENTITY'
		 union
		 select distinct SourceName, RefKind from refs
		 where TargetName = '%s' and SourceType = 'ENTITY'
		 ORDER by RefKind, TargetName limit 10`, name, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v (%v)\n", row[0], row[1]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
}

// assemblePageContext assembles context for a page.
func assemblePageContext(ctx *ExecContext, out *strings.Builder, name string, depth int) {
	// Get page basic info
	out.WriteString("### Page Definition\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"select Name, Title, url, LayoutRef, WidgetCount from pages where QualifiedName = '%s'", name))
	if err == nil && result.Count > 0 {
		row := result.Rows[0]
		out.WriteString(fmt.Sprintf("- **Name**: %v\n", row[0]))
		if row[1] != nil && row[1] != "" {
			out.WriteString(fmt.Sprintf("- **Title**: %v\n", row[1]))
		}
		if row[2] != nil && row[2] != "" {
			out.WriteString(fmt.Sprintf("- **url**: %v\n", row[2]))
		}
		if row[3] != nil && row[3] != "" {
			out.WriteString(fmt.Sprintf("- **Layout**: %v\n", row[3]))
		}
		out.WriteString(fmt.Sprintf("- **Widgets**: %v\n", row[4]))
	}
	out.WriteString("\n")

	// Entities used on this page
	out.WriteString("### Entities Used\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName from refs
		 where SourceName = '%s' and TargetType = 'ENTITY'
		 ORDER by TargetName`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Microflows called from this page
	out.WriteString("### Microflows Called\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName from refs
		 where SourceName = '%s' and TargetType = 'MICROFLOW'
		 ORDER by TargetName limit 15`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Microflows that show this page
	out.WriteString("### Shown By\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select SourceName from refs
		 where TargetName = '%s' and RefKind = 'show_page'
		 ORDER by SourceName limit 10`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
}

// assembleEnumerationContext assembles context for an enumeration.
func assembleEnumerationContext(ctx *ExecContext, out *strings.Builder, name string) {
	// Get enumeration basic info
	out.WriteString("### Enumeration Definition\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"select Name, ValueCount from enumerations where QualifiedName = '%s'", name))
	if err == nil && result.Count > 0 {
		row := result.Rows[0]
		out.WriteString(fmt.Sprintf("- **Name**: %v\n", row[0]))
		out.WriteString(fmt.Sprintf("- **Values**: %v\n", row[1]))
	}
	out.WriteString("\n")

	// Entities with attributes of this enumeration type
	out.WriteString("### Used By Entities\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct SourceName from refs
		 where TargetName = '%s' and SourceType = 'ENTITY'
		 ORDER by SourceName limit 15`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Microflows that use this enumeration
	out.WriteString("### Used By Microflows\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct SourceName from refs
		 where TargetName = '%s' and SourceType = 'MICROFLOW'
		 ORDER by SourceName limit 15`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
}

// assembleSnippetContext assembles context for a snippet.
func assembleSnippetContext(ctx *ExecContext, out *strings.Builder, name string, depth int) {
	out.WriteString("### Snippet Definition\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"select Name, ParameterCount, WidgetCount from snippets where QualifiedName = '%s'", name))
	if err == nil && result.Count > 0 {
		row := result.Rows[0]
		out.WriteString(fmt.Sprintf("- **Name**: %v\n", row[0]))
		out.WriteString(fmt.Sprintf("- **Parameters**: %v\n", row[1]))
		out.WriteString(fmt.Sprintf("- **Widgets**: %v\n", row[2]))
	}
	out.WriteString("\n")

	// MDL source via DESCRIBE
	out.WriteString("### MDL Source\n\n```sql\n")
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		descStmt := &ast.DescribeStmt{
			ObjectType: ast.DescribeSnippet,
			Name:       ast.QualifiedName{Module: parts[0], Name: parts[1]},
		}
		savedOutput := ctx.Output
		ctx.Output = out
		execDescribe(ctx, descStmt)
		ctx.Output = savedOutput
	}
	out.WriteString("```\n\n")

	// Pages that use this snippet
	out.WriteString("### Used By Pages\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct SourceName from refs
		 where TargetName = '%s' and RefKind = 'snippet_call'
		 ORDER by SourceName limit 15`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
}

// assembleJavaActionContext assembles context for a java action.
func assembleJavaActionContext(ctx *ExecContext, out *strings.Builder, name string) {
	out.WriteString("### Java Action Definition\n\n```sql\n")
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		descStmt := &ast.DescribeStmt{
			ObjectType: ast.DescribeJavaAction,
			Name:       ast.QualifiedName{Module: parts[0], Name: parts[1]},
		}
		savedOutput := ctx.Output
		ctx.Output = out
		execDescribe(ctx, descStmt)
		ctx.Output = savedOutput
	}
	out.WriteString("```\n\n")

	// Microflows that call this java action
	out.WriteString("### Called By Microflows\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		`select distinct SourceName from refs
		 where TargetName = '%s' and RefKind = 'call'
		 ORDER by SourceName limit 15`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
}

// assembleODataClientContext assembles context for a consumed OData service.
func assembleODataClientContext(ctx *ExecContext, out *strings.Builder, name string) {
	out.WriteString("### Consumed OData Service\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"select Name, Version, ODataVersion, MetadataUrl from odata_clients where QualifiedName = '%s'", name))
	if err == nil && result.Count > 0 {
		row := result.Rows[0]
		out.WriteString(fmt.Sprintf("- **Name**: %v\n", row[0]))
		out.WriteString(fmt.Sprintf("- **Version**: %v\n", row[1]))
		out.WriteString(fmt.Sprintf("- **OData Version**: %v\n", row[2]))
		out.WriteString(fmt.Sprintf("- **Metadata url**: %v\n", row[3]))
	}
	out.WriteString("\n")

	// External entities from this service
	out.WriteString("### External Entities\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName from refs
		 where SourceName = '%s' and RefKind = 'odata_entity'
		 ORDER by TargetName limit 15`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
}

// assembleWorkflowContext assembles context for a workflow.
func assembleWorkflowContext(ctx *ExecContext, out *strings.Builder, name string, depth int) {
	// Get workflow basic info
	out.WriteString("### Workflow Definition\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"select Name, ParameterEntity, ActivityCount, UserTaskCount, MicroflowCallCount, DecisionCount, Description from workflows where QualifiedName = '%s'", name))
	if err == nil && result.Count > 0 {
		row := result.Rows[0]
		out.WriteString(fmt.Sprintf("- **Name**: %v\n", row[0]))
		if row[1] != nil && row[1] != "" {
			out.WriteString(fmt.Sprintf("- **Parameter Entity**: %v\n", row[1]))
		}
		out.WriteString(fmt.Sprintf("- **Activities**: %v\n", row[2]))
		out.WriteString(fmt.Sprintf("- **User Tasks**: %v\n", row[3]))
		out.WriteString(fmt.Sprintf("- **Microflow Calls**: %v\n", row[4]))
		out.WriteString(fmt.Sprintf("- **Decisions**: %v\n", row[5]))
		if row[6] != nil && row[6] != "" {
			out.WriteString(fmt.Sprintf("- **Description**: %v\n", row[6]))
		}
	}
	out.WriteString("\n")

	// MDL source via DESCRIBE
	out.WriteString("### MDL Source\n\n```sql\n")
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		descStmt := &ast.DescribeStmt{
			ObjectType: ast.DescribeWorkflow,
			Name:       ast.QualifiedName{Module: parts[0], Name: parts[1]},
		}
		savedOutput := ctx.Output
		ctx.Output = out
		execDescribe(ctx, descStmt)
		ctx.Output = savedOutput
	}
	out.WriteString("```\n\n")

	// Microflows called by this workflow
	out.WriteString("### Microflows Called\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName, RefKind from refs
		 where SourceName = '%s' and TargetType = 'MICROFLOW'
		 ORDER by RefKind, TargetName`, name))
	if err == nil && result.Count > 0 {
		out.WriteString("| Microflow | Usage |\n")
		out.WriteString("|-----------|-------|\n")
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("| %v | %v |\n", row[0], row[1]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Pages used by this workflow (user task pages, overview page)
	out.WriteString("### Pages Used\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName, RefKind from refs
		 where SourceName = '%s' and TargetType = 'PAGE'
		 ORDER by TargetName`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v (%v)\n", row[0], row[1]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Entities referenced by this workflow
	out.WriteString("### Entities Used\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName, RefKind from refs
		 where SourceName = '%s' and TargetType = 'ENTITY'
		 ORDER by TargetName`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v (%v)\n", row[0], row[1]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
	out.WriteString("\n")

	// Direct callers (what calls this workflow)
	out.WriteString("### Direct Callers\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select SourceName, SourceType from refs
		 where TargetName = '%s'
		 ORDER by SourceName limit 15`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v (%v)\n", row[0], row[1]))
		}
		if result.Count == 15 {
			out.WriteString("- ... (more callers exist)\n")
		}
	} else {
		out.WriteString("(none found)\n")
	}
}

// assembleODataServiceContext assembles context for a published OData service.
func assembleODataServiceContext(ctx *ExecContext, out *strings.Builder, name string) {
	out.WriteString("### Published OData Service\n\n")
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"select Name, Path, Version, ODataVersion, EntitySetCount from odata_services where QualifiedName = '%s'", name))
	if err == nil && result.Count > 0 {
		row := result.Rows[0]
		out.WriteString(fmt.Sprintf("- **Name**: %v\n", row[0]))
		out.WriteString(fmt.Sprintf("- **Path**: %v\n", row[1]))
		out.WriteString(fmt.Sprintf("- **Version**: %v\n", row[2]))
		out.WriteString(fmt.Sprintf("- **OData Version**: %v\n", row[3]))
		out.WriteString(fmt.Sprintf("- **Entity Sets**: %v\n", row[4]))
	}
	out.WriteString("\n")

	// Published entities
	out.WriteString("### Published Entities\n\n")
	result, err = ctx.Catalog.Query(fmt.Sprintf(
		`select distinct TargetName from refs
		 where SourceName = '%s' and RefKind = 'odata_publish'
		 ORDER by TargetName limit 15`, name))
	if err == nil && result.Count > 0 {
		for _, row := range result.Rows {
			out.WriteString(fmt.Sprintf("- %v\n", row[0]))
		}
	} else {
		out.WriteString("(none found)\n")
	}
}

// --- Executor method wrappers for backward compatibility ---
