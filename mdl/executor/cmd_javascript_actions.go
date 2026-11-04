// SPDX-License-Identifier: Apache-2.0

// Package executor - JavaScript Action commands (SHOW/DESCRIBE JAVASCRIPT ACTIONS)
package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// listJavaScriptActions handles SHOW JAVASCRIPT ACTIONS command.
func listJavaScriptActions(ctx *ExecContext, moduleName string) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	jsActions, err := ctx.Backend.ListJavaScriptActions()
	if err != nil {
		return mdlerrors.NewBackend("list javascript actions", err)
	}

	type row struct {
		qualifiedName string
		module        string
		name          string
		platform      string
		folderPath    string
	}
	var rows []row

	for _, jsa := range jsActions {
		modID := h.FindModuleID(jsa.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + jsa.Name
			folderPath := h.BuildFolderPath(jsa.ContainerID)
			platform := jsa.Platform
			if platform == "" {
				platform = "All"
			}
			rows = append(rows, row{qualifiedName, modName, jsa.Name, platform, folderPath})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Platform", "Folder"},
		Summary: fmt.Sprintf("(%d javascript actions)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.platform, r.folderPath})
	}
	return writeResult(ctx, result)
}

// jsSourceOmittedBody is the body DESCRIBE prints for an action whose .js source
// it cannot read. exec recognises it and writes no source file (#731), as it
// does for the Java twin, javaSourceOmittedBody.
const jsSourceOmittedBody = "// JavaScript source not available from this project; body omitted by DESCRIBE."

// describeJavaScriptAction handles DESCRIBE JAVASCRIPT ACTION command.
func describeJavaScriptAction(ctx *ExecContext, name ast.QualifiedName) error {
	qualifiedName := name.Module + "." + name.Name
	jsa, err := ctx.Backend.ReadJavaScriptActionByName(qualifiedName)
	if err != nil {
		return mdlerrors.NewNotFound("javascript action", qualifiedName)
	}

	var sb strings.Builder

	// Documentation comment
	doc := strings.ReplaceAll(jsa.Documentation, "\r\n", "\n")
	doc = strings.ReplaceAll(doc, "\r", "\n")
	if doc != "" {
		sb.WriteString("/**\n")
		for line := range strings.SplitSeq(doc, "\n") {
			sb.WriteString(" * ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString(" */\n")
	}

	// Type parameters have no clause of their own: as for Java actions, each is
	// declared by an `entity <T>` parameter. The `Name<T>` this used to print
	// after the action name does not parse.
	sb.WriteString("create or modify javascript action ")
	sb.WriteString(qualifiedName)
	sb.WriteString(describeFolderClause(ctx, jsa.ContainerID))
	sb.WriteString("(")

	// Parameters, each description as the doc comment create reads (see the Java twin).
	writeCodeActionParams(&sb, jsa.Parameters, formatJavaScriptActionType)
	sb.WriteString(")")

	// Clauses are emitted in grammar order (returns, exposed, platform, body) so
	// the DESCRIBE output re-parses as a CREATE JAVASCRIPT ACTION.

	// Return type
	if jsa.ReturnType != nil {
		sb.WriteString("\n  returns ")
		sb.WriteString(formatJavaActionReturnType(jsa.ReturnType))
	}
	sb.WriteString(describeCodeActionReturnName(jsa.ActionDefaultReturnName, jsa.ReturnType != nil))

	// EXPOSED AS clause
	if jsa.MicroflowActionInfo != nil && jsa.MicroflowActionInfo.Caption != "" {
		sb.WriteString("\n  exposed as '")
		sb.WriteString(jsa.MicroflowActionInfo.Caption)
		sb.WriteString("' in '")
		sb.WriteString(jsa.MicroflowActionInfo.Category)
		sb.WriteString("'")
	}

	// PLATFORM clause (always emitted so the platform setting round-trips).
	platform := jsa.Platform
	if platform == "" {
		platform = "Web"
	}
	sb.WriteString("\n  platform ")
	sb.WriteString(platform)

	// JavaScript source. The grammar requires an `as $$ ... $$` body, so always
	// emit one; when the .js source can't be read (add-on modules without source
	// on disk) emit a placeholder so the output still re-parses (cf. #637).
	userCode, extraCode := readJavaScriptActionSource(ctx.MprPath, name.Module, name.Name)
	if userCode == "" {
		userCode = jsSourceOmittedBody
	}
	sb.WriteString("\nas ")
	sb.WriteString(mdlSourceBlock(ctx, userCode))
	sb.WriteString(";")

	stmt := sb.String()
	if gap := jsTypeParameterGap(jsa); gap != "" {
		// A statement that parses into something else is worse than none:
		// `EntityObject: T` with no `entity <T>` re-executes as the entity `.T`
		// (mendixlabs/mxcli#1404). Commented out, the script still parses and
		// re-running it leaves the stored action as it is.
		fmt.Fprintf(ctx.Output, "-- NOT re-executable: %s; MDL has no form for this (mendixlabs/mxcli#1404), "+
			"so the statement is commented out and re-running this script leaves the action unchanged\n", gap)
		stmt = "-- " + strings.ReplaceAll(stmt, "\n", "\n-- ")
	}
	fmt.Fprintln(ctx.Output, stmt)

	// Additional info as comments.
	if jsa.MicroflowActionInfo != nil && len(jsa.MicroflowActionInfo.IconData) > 0 {
		fmt.Fprintf(ctx.Output, "-- icon: %d bytes\n", len(jsa.MicroflowActionInfo.IconData))
	}
	if jsa.ExportLevel != "" && jsa.ExportLevel != "Hidden" {
		fmt.Fprintf(ctx.Output, "-- export level: %s\n", jsa.ExportLevel)
	}
	if jsa.Excluded {
		fmt.Fprintln(ctx.Output, "-- EXCLUDED: true")
	}
	if extraCode != "" {
		fmt.Fprintln(ctx.Output, "-- EXTRA CODE:")
		for line := range strings.SplitSeq(extraCode, "\n") {
			fmt.Fprintf(ctx.Output, "-- %s\n", line)
		}
	}

	return nil
}

// jsTypeParameterGap reports why the action's type parameters have no MDL form,
// or "" when they have one. MDL declares a type parameter only through an
// `entity <T>` parameter (the visitor collects them in parameter order), so the
// stored list must be exactly the declared names in that order, and every
// reference to a type parameter must name one of them.
func jsTypeParameterGap(jsa *types.JavaScriptAction) string {
	var declared []string
	seen := map[string]bool{}
	for _, p := range jsa.Parameters {
		e, ok := p.ParameterType.(*types.EntityTypeParameterType)
		if !ok {
			continue
		}
		if e.TypeParameterName == "" {
			return fmt.Sprintf("parameter %s is an entity selector bound to no type parameter", p.Name)
		}
		if !seen[e.TypeParameterName] {
			seen[e.TypeParameterName] = true
			declared = append(declared, e.TypeParameterName)
		}
	}
	refGap := func(what string, t any) string {
		name, id := "", model.ID("")
		switch v := t.(type) {
		case *types.TypeParameter:
			name, id = v.TypeParameter, v.TypeParameterID
		case *types.ListType:
			name, id = v.TypeParameter, v.TypeParameterID
		}
		switch {
		case name == "" && id != "":
			return what + " points at a type parameter that does not exist"
		case name != "" && !seen[name]:
			return fmt.Sprintf("%s is typed by type parameter %s, which no `entity <%s>` parameter declares", what, name, name)
		}
		return ""
	}
	for _, p := range jsa.Parameters {
		if gap := refGap("parameter "+p.Name, p.ParameterType); gap != "" {
			return gap
		}
	}
	if gap := refGap("the return type", jsa.ReturnType); gap != "" {
		return gap
	}
	stored := make([]string, 0, len(jsa.TypeParameters))
	for _, tp := range jsa.TypeParameters {
		stored = append(stored, tp.Name)
	}
	if strings.Join(stored, ",") != strings.Join(declared, ",") {
		return fmt.Sprintf("type parameters [%s] are not the ones its `entity <T>` parameters declare, in that order ([%s])",
			strings.Join(stored, ", "), strings.Join(declared, ", "))
	}
	return ""
}

// readJavaScriptActionSource reads the JavaScript source file and extracts user code and extra code.
func readJavaScriptActionSource(mprPath, moduleName, actionName string) (userCode, extraCode string) {
	if mprPath == "" {
		return "", ""
	}

	projectRoot := filepath.Dir(mprPath)
	// JavaScript source uses original module name casing (not lowercased like javasource)
	jsPath := filepath.Join(projectRoot, "javascriptsource", moduleName, "actions", actionName+".js")

	content, err := os.ReadFile(jsPath)
	if err != nil {
		// Try lowercase module name as fallback
		jsPath = filepath.Join(projectRoot, "javascriptsource", strings.ToLower(moduleName), "actions", actionName+".js")
		content, err = os.ReadFile(jsPath)
		if err != nil {
			return "", ""
		}
	}

	source := string(content)

	// Marker casing varies (Studio Pro / the warning banner use uppercase
	// "BEGIN USER CODE"; some older content used lowercase), so match
	// case-insensitively.
	if uc, ok := sliceBetweenFold(source, "// BEGIN USER CODE", "// END USER CODE"); ok {
		uc = strings.TrimPrefix(uc, "\n")
		uc = strings.TrimSuffix(uc, "\n")
		userCode = strings.TrimRight(uc, " \t")
	}
	if ec, ok := sliceBetweenFold(source, "// BEGIN EXTRA CODE", "// END EXTRA CODE"); ok {
		extraCode = strings.TrimSpace(ec)
	}

	return userCode, extraCode
}

// sliceBetweenFold returns the substring of s between the first case-insensitive
// occurrence of begin and the following case-insensitive occurrence of end.
//
// Only ASCII letters are folded. strings.ToLower changes the byte length of some
// characters ("İ" 2 -> 3 bytes, the Kelvin sign 3 -> 1), so an index into its
// result is not an index into s, and one such character above the markers cut
// the section a byte off — the markers are ASCII, so nothing else needs folding.
func sliceBetweenFold(s, begin, end string) (string, bool) {
	lower := asciiLower(s)
	bi := strings.Index(lower, asciiLower(begin))
	if bi == -1 {
		return "", false
	}
	rest := bi + len(begin)
	ei := strings.Index(lower[rest:], asciiLower(end))
	if ei == -1 {
		return "", false
	}
	return s[rest : rest+ei], true
}

// formatJavaScriptActionType formats a JavaScript action parameter type for MDL output.
func formatJavaScriptActionType(t javaactions.CodeActionParameterType) string {
	if t == nil {
		return "Object"
	}
	if s, ok := formatCodeActionTypeParameterRef(t); ok {
		return s
	}
	return t.TypeString()
}

// asciiLower lower-cases ASCII letters only, so the result has s's byte offsets.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
