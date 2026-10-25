// SPDX-License-Identifier: Apache-2.0

// Package executor - Java Action commands (SHOW/DESCRIBE/CREATE JAVA ACTIONS)
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

// listJavaActions handles SHOW JAVA ACTIONS command.
func listJavaActions(ctx *ExecContext, moduleName string) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Get all Java actions
	javaActions, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		folderPath    string
	}
	var rows []row

	for _, ja := range javaActions {
		modID := h.FindModuleID(ja.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + ja.Name
			folderPath := h.BuildFolderPath(ja.ContainerID)
			rows = append(rows, row{qualifiedName, modName, ja.Name, folderPath})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Folder"},
		Summary: fmt.Sprintf("(%d java actions)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.folderPath})
	}
	return writeResult(ctx, result)
}

// describeJavaAction handles DESCRIBE JAVA ACTION command - outputs MDL-style representation.
func describeJavaAction(ctx *ExecContext, name ast.QualifiedName) error {
	qualifiedName := name.Module + "." + name.Name
	ja, err := ctx.Backend.ReadJavaActionByName(qualifiedName)
	if err != nil {
		return mdlerrors.NewNotFound("java action", qualifiedName)
	}

	// Generate MDL-style output for CREATE JAVA ACTION format
	var sb strings.Builder

	// Documentation comment (JavaDoc style) — normalize \r\n to \n
	doc := strings.ReplaceAll(ja.Documentation, "\r\n", "\n")
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

	// Build CREATE JAVA ACTION statement
	sb.WriteString("create or modify java action ")
	sb.WriteString(qualifiedName)
	header, gap := javaActionTypeParameterHeader(ja)
	sb.WriteString(header)
	sb.WriteString(describeFolderClause(ctx, ja.ContainerID))
	sb.WriteString("(")

	writeCodeActionParams(&sb, ja.Parameters, formatJavaActionType)
	sb.WriteString(")")

	// Return type, and the return value's name
	if ja.ReturnType != nil {
		sb.WriteString(" returns ")
		sb.WriteString(formatJavaActionReturnType(ja.ReturnType))
	}
	sb.WriteString(describeCodeActionReturnName(ja.ActionDefaultReturnName, ja.ReturnType != nil))

	// EXPOSED AS clause
	if ja.MicroflowActionInfo != nil && ja.MicroflowActionInfo.Caption != "" {
		sb.WriteString("\nexposed as '")
		sb.WriteString(ja.MicroflowActionInfo.Caption)
		sb.WriteString("' in '")
		sb.WriteString(ja.MicroflowActionInfo.Category)
		sb.WriteString("'")
		sb.WriteString(describeBitmapComments(ja.MicroflowActionInfo))
	}

	// The grammar requires an `as $$ ... $$` body for CREATE JAVA ACTION, so always
	// emit one. When the .java source can't be read (e.g. add-on modules ship without
	// source on disk), emit a placeholder body so DESCRIBE output still re-parses (#637).
	javaCode := readJavaActionUserCode(ctx.MprPath, name.Module, name.Name)
	sb.WriteString("\nas $$\n")
	if javaCode != "" {
		sb.WriteString(javaCode)
	} else {
		sb.WriteString(javaSourceOmittedBody)
	}
	sb.WriteString("\n$$;")

	// Output the complete statement. A shape MDL has no form for is printed
	// commented out: a statement that parses into something else is worse than
	// none, and re-running the script then leaves the action as it is.
	stmt := sb.String()
	if gap != "" {
		fmt.Fprintf(ctx.Output, "-- NOT re-executable: %s; MDL has no form for this (mendixlabs/mxcli#1404), "+
			"so the statement is commented out and re-running this script leaves the action unchanged\n", gap)
		stmt = "-- " + strings.ReplaceAll(stmt, "\n", "\n-- ")
	}
	fmt.Fprintln(ctx.Output, stmt)

	// Additional info as comments
	if ja.ExportLevel != "" && ja.ExportLevel != "Hidden" {
		fmt.Fprintf(ctx.Output, "-- export level: %s\n", ja.ExportLevel)
	}
	if ja.Excluded {
		fmt.Fprintln(ctx.Output, "-- EXCLUDED: true")
	}

	return nil
}

// javaActionTypeParameterHeader returns the `<T, U>` header that declares the
// action's type parameters, and why they have no MDL form ("" when they have
// one). An `entity <T>` parameter declares T too, so the header is printed only
// when the selectors alone would not reproduce the stored list in its order —
// a type parameter no selector names (mendixlabs/mxcli#1404), or another order.
// What no spelling reaches: an entity selector bound to no type parameter, and
// a reference to a type parameter that is not stored.
func javaActionTypeParameterHeader(ja *javaactions.JavaAction) (header, gap string) {
	stored := make([]string, 0, len(ja.TypeParameters))
	storedSet := map[string]bool{}
	for _, tp := range ja.TypeParameters {
		if tp == nil || tp.Name == "" || storedSet[tp.Name] {
			return "", "a type parameter is unnamed or declared twice"
		}
		stored = append(stored, tp.Name)
		storedSet[tp.Name] = true
	}
	var bySelector []string
	seen := map[string]bool{}
	for _, p := range ja.Parameters {
		e, ok := p.ParameterType.(*javaactions.EntityTypeParameterType)
		if !ok {
			continue
		}
		if e.TypeParameterName == "" {
			return "", fmt.Sprintf("parameter %s is an entity selector bound to no type parameter", p.Name)
		}
		if !seen[e.TypeParameterName] {
			seen[e.TypeParameterName] = true
			bySelector = append(bySelector, e.TypeParameterName)
		}
	}
	refGap := func(what string, t any) string {
		name, id := "", model.ID("")
		switch v := t.(type) {
		case *javaactions.EntityTypeParameterType:
			name = v.TypeParameterName
		case *javaactions.TypeParameter:
			name, id = v.TypeParameter, v.TypeParameterID
		case *javaactions.ListType:
			name, id = v.TypeParameter, v.TypeParameterID
		}
		switch {
		case name == "" && id != "":
			return what + " points at a type parameter that does not exist"
		case name != "" && !storedSet[name]:
			return fmt.Sprintf("%s names type parameter %s, which the action does not declare", what, name)
		}
		return ""
	}
	for _, p := range ja.Parameters {
		if g := refGap("parameter "+p.Name, p.ParameterType); g != "" {
			return "", g
		}
	}
	if g := refGap("the return type", ja.ReturnType); g != "" {
		return "", g
	}
	if strings.Join(stored, "\x00") == strings.Join(bySelector, "\x00") {
		return "", ""
	}
	names := make([]string, len(stored))
	for i, n := range stored {
		names[i] = mdlIdent(n)
	}
	return "<" + strings.Join(names, ", ") + ">", ""
}

// javaSourceOmittedBody is the body DESCRIBE prints for an action whose source it
// cannot read (#637). exec recognises it and writes no source file.
const javaSourceOmittedBody = "// Java source not available from this project; body omitted by DESCRIBE."

// readJavaActionUserCode reads the Java source file and extracts the user code section.
func readJavaActionUserCode(mprPath, moduleName, actionName string) string {
	if mprPath == "" {
		return ""
	}

	// Build path to Java source file
	projectRoot := filepath.Dir(mprPath)
	moduleNameLower := strings.ToLower(moduleName)
	javaPath := filepath.Join(projectRoot, "javasource", moduleNameLower, "actions", actionName+".java")

	// Read the file
	content, err := os.ReadFile(javaPath)
	if err != nil {
		return ""
	}

	// Extract user code between the BEGIN USER CODE and END USER CODE markers.
	// Matched case-insensitively, as for JavaScript actions: a sweep that
	// lowercased MDL keywords in output also lowercased these literals, so no
	// Studio Pro file matched, DESCRIBE always printed the placeholder body, and
	// re-executing that output replaced the action's code with it (#705).
	userCode, ok := sliceBetweenFold(string(content), "// BEGIN USER CODE", "// END USER CODE")
	if !ok {
		return ""
	}
	return normalizeJavaSection(userCode)
}

// normalizeJavaSection makes a marker section's text a fixed point of
// describe -> exec: line endings to LF (Studio Pro writes CRLF), the blank lines
// around it dropped, and the indentation common to every line removed.
// GenerateSource re-indents each line uniformly, so without this every rewrite
// shifted the code further right and a second DESCRIBE never matched the first.
func normalizeJavaSection(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	indent := ""
	first := true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		lead := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if first {
			indent, first = lead, false
			continue
		}
		for !strings.HasPrefix(lead, indent) {
			indent = indent[:len(indent)-1]
		}
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(strings.TrimPrefix(l, indent), " \t")
	}
	return strings.Join(lines, "\n")
}

// formatJavaActionType formats a Java action parameter type for MDL output.
func formatJavaActionType(t javaactions.CodeActionParameterType) string {
	if t == nil {
		return "Object"
	}
	if s, ok := formatCodeActionTypeParameterRef(t); ok {
		return s
	}
	return t.TypeString()
}

// formatJavaActionReturnType formats a Java action return type.
// writeCodeActionParams prints a Java or JavaScript action's parameters, each
// description as the `/** … */` doc comment that create reads before a
// parameter — the form an entity attribute's documentation takes. It was a
// trailing `--` comment, which the lexer discards: a script copying describe's
// output lost every description, and only the first line was printed anyway
// (mendixlabs/mxcli#1588). One parameter per line when any is described.
func writeCodeActionParams(sb *strings.Builder, params []*javaactions.JavaActionParameter,
	formatType func(javaactions.CodeActionParameterType) string) {
	described := false
	for _, p := range params {
		if p.Description != "" {
			described = true
			break
		}
	}
	for i, param := range params {
		if described {
			sb.WriteString("\n")
			if param.Description != "" {
				doc := strings.ReplaceAll(param.Description, "\r\n", "\n")
				doc = strings.ReplaceAll(doc, "\r", "\n")
				// A `*/` would end the doc comment mid-text and the rest would not
				// parse. Shown as a comment, the parameter states none, so a
				// rewrite carries the stored one.
				if strings.Contains(doc, "*/") {
					for line := range strings.SplitSeq(doc, "\n") {
						fmt.Fprintf(sb, "    -- %s\n", line)
					}
				} else if !strings.Contains(doc, "\n") && !strings.HasPrefix(doc, "*") {
					// The inline form holds one line that does not itself start with
					// `*`, which the parser strips as the gutter (mendixlabs/mxcli#1428).
					fmt.Fprintf(sb, "    /** %s */\n", doc)
				} else {
					outputJavadocIndented(sb, doc, "    ")
				}
			}
			sb.WriteString("    ")
		}
		sb.WriteString(param.Name)
		sb.WriteString(": ")
		if param.ParameterType != nil {
			sb.WriteString(formatType(param.ParameterType))
		} else {
			sb.WriteString("Object")
		}
		if param.IsRequired {
			sb.WriteString(" not null")
		}
		if i < len(params)-1 {
			sb.WriteString(",")
			if !described {
				sb.WriteString(" ")
			}
		}
	}
	if described {
		sb.WriteString("\n")
	}
}

// describeCodeActionReturnName is the ` as $Name` that follows `returns T`,
// naming the return value as create reads it (mendixlabs/mxcli#1588). A name
// the `$Name` token cannot spell, or one with no `returns` to follow, is still
// shown, as a comment on its own line.
func describeCodeActionReturnName(name string, hasReturns bool) string {
	if name == "" {
		return ""
	}
	if hasReturns && isVariableTokenBody(name) {
		return " as $" + name
	}
	return "\n-- return NAME: '" + name + "'"
}

// isVariableTokenBody reports whether `$` + s lexes as one VARIABLE token
// ('$' ID_BODY+).
func isVariableTokenBody(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func formatJavaActionReturnType(t javaactions.CodeActionReturnType) string {
	if t == nil {
		return "Void"
	}
	if s, ok := formatCodeActionTypeParameterRef(t); ok {
		return s
	}
	return t.TypeString()
}

// formatCodeActionTypeParameterRef renders the types that name a type parameter.
// Studio Pro accepts any name for one, including a primitive's, so the name goes
// through mdlIdent: `returns String` re-parses as the primitive, `returns
// "String"` as the type parameter (#1183).
func formatCodeActionTypeParameterRef(t any) (string, bool) {
	switch v := t.(type) {
	case *javaactions.EntityTypeParameterType:
		return "entity <" + mdlIdent(v.TypeParameterName) + ">", true
	case *javaactions.TypeParameter:
		if v.TypeParameter != "" {
			return mdlIdent(v.TypeParameter), true
		}
	case *javaactions.ListType:
		if v.TypeParameter != "" {
			return "List of " + mdlIdent(v.TypeParameter), true
		}
	}
	return "", false
}

// execDropJavaAction handles DROP JAVA ACTION statements.
func execDropJavaAction(ctx *ExecContext, s *ast.DropJavaActionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find and delete the Java action
	jas, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}

	for _, ja := range jas {
		modID := h.FindModuleID(ja.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && ja.Name == s.Name.Name {
			if err := ctx.Backend.DeleteJavaAction(ja.ID); err != nil {
				return mdlerrors.NewBackend("delete java action", err)
			}
			if err := ctx.Backend.DeleteJavaSourceFile(modName, ja.Name); err != nil {
				return mdlerrors.NewBackend("delete java source file", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped java action: %s.%s\n", s.Name.Module, s.Name.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("java action", s.Name.Module+"."+s.Name.Name)
}

// execCreateJavaAction handles CREATE JAVA ACTION statements.
func execCreateJavaAction(ctx *ExecContext, s *ast.CreateJavaActionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the module
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("get modules", err)
	}

	var containerID model.ID
	var moduleName string
	for _, mod := range modules {
		if mod.Name == s.Name.Module {
			containerID = mod.ID
			moduleName = mod.Name
			break
		}
	}
	if containerID == "" {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}

	// Decide what each bare Module.Name in the signature is — an entity or an
	// enumeration — before anything is written (ako/mxcli#935).
	params, returnType, err := settleCodeActionSignature(ctx, "java action "+s.Name.String(), s.Parameters, s.ReturnType)
	if err != nil {
		return err
	}

	// Check if Java action already exists
	jas, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}
	var existingJAID model.ID
	var existingContainer model.ID
	// Target the live action and carry its exclusion forward (#914).
	existingExcluded := false
	var existingJADoc string
	haveExistingJA := false
	var existingActionInfo *javaactions.MicroflowActionInfo
	// A parameter's Category has no MDL spelling, and its Description is set
	// only by a `/** … */` before it, so a rewrite carries both from the stored
	// parameter of the same name unless the statement says otherwise.
	storedParams := map[string]*javaactions.JavaActionParameter{}
	// The export level has no MDL spelling and the return value's name is set
	// only by `returns T as $Name`, so a rewrite carries them. The defaults are
	// what Studio Pro writes on a new action; "Public" — what this used to
	// hardcode — is not a member of JavaActionsExportLevel (API | Hidden), and
	// every rewrite turned a Hidden action into it (ako/mxcli#705).
	exportLevel, defaultReturnName := "Hidden", "ReturnValueName"
	if existing, ok := pickLive(jas,
		func(ja *types.JavaAction) bool {
			return h.GetModuleName(h.FindModuleID(ja.ContainerID)) == s.Name.Module && ja.Name == s.Name.Name
		},
		func(ja *types.JavaAction) bool { return ja.Excluded },
	); ok {
		if !s.CreateOrModify {
			return mdlerrors.NewAlreadyExists("java action", s.Name.Module+"."+s.Name.Name)
		}
		existingJAID = existing.ID
		existingExcluded = existing.Excluded
		existingContainer = existing.ContainerID
		existingJADoc = existing.Documentation
		haveExistingJA = true
		// The toolbox entry holds four PNG bitmaps MDL cannot name, so the
		// stored one has to be read before the rewrite can carry them.
		if full, err := ctx.Backend.ReadJavaActionByName(s.Name.Module + "." + s.Name.Name); err == nil && full != nil {
			existingActionInfo = full.MicroflowActionInfo
			if full.ExportLevel != "" {
				exportLevel = full.ExportLevel
			}
			defaultReturnName = full.ActionDefaultReturnName
			for _, p := range full.Parameters {
				if p != nil {
					storedParams[p.Name] = p
				}
			}
		}
	}

	moduleID := containerID
	containerID, err = containerForDocument(ctx, moduleID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	newID := model.ID(types.GenerateID())
	if existingJAID != "" {
		newID = existingJAID
	}

	// `returns T as $Name` names the return value (mendixlabs/mxcli#1588).
	if s.ReturnName != "" {
		defaultReturnName = s.ReturnName
	}

	// Create the Java action
	ja := &javaactions.JavaAction{
		BaseElement: model.BaseElement{
			ID:       newID,
			TypeName: "JavaActions$JavaAction",
		},
		Excluded:                existingExcluded,
		ContainerID:             containerID,
		Name:                    s.Name.Name,
		Documentation:           s.Documentation,
		ExportLevel:             exportLevel,
		ActionDefaultReturnName: defaultReturnName,
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if haveExistingJA {
		ja.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existingJADoc)
	}

	// Build type parameter definitions (with IDs for BY_ID references)
	typeParamNameToID := make(map[string]model.ID)
	for _, tpName := range s.TypeParameters {
		tpDef := &javaactions.TypeParameterDef{
			BaseElement: model.BaseElement{
				ID: model.ID(types.GenerateID()),
			},
			Name: tpName,
		}
		ja.TypeParameters = append(ja.TypeParameters, tpDef)
		typeParamNameToID[tpName] = tpDef.ID
	}

	if bad := undeclaredJavaActionTypeNames(s); len(bad) > 0 {
		return mdlerrors.NewValidationf("java action %s: %s, which is neither a qualified entity nor a declared type parameter; %s",
			s.Name.String(), strings.Join(bad, ", "), undeclaredJavaActionTypeHint(s))
	}

	// Build a set of type parameter names for quick lookup
	typeParamNames := make(map[string]bool)
	for _, tpName := range s.TypeParameters {
		typeParamNames[tpName] = true
	}

	// Convert parameters:
	// - TypeEntityTypeParam → EntityTypeParameterType (entity type selector)
	// - Bare name matching a type parameter → TypeParameter (ParameterizedEntityType)
	// - Other types → convert normally
	for _, param := range params {
		jaParam := &javaactions.JavaActionParameter{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "JavaActions$JavaActionParameter",
			},
			Name:       param.Name,
			IsRequired: param.IsRequired,
		}
		if param.Type.Kind == ast.TypeEntityTypeParam {
			// Explicit ENTITY <pEntity> → EntityTypeParameterType (entity type selector)
			tpName := param.Type.TypeParamName
			jaParam.ParameterType = &javaactions.EntityTypeParameterType{
				BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
				TypeParameterID:   typeParamNameToID[tpName],
				TypeParameterName: tpName,
			}
		} else if isTypeParamRef(param.Type, typeParamNames) {
			// Bare name matching a type parameter → TypeParameter (ParameterizedEntityType)
			tpName := getTypeParamRefName(param.Type)
			jaParam.ParameterType = &javaactions.TypeParameter{
				BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
				TypeParameterID: typeParamNameToID[tpName],
				TypeParameter:   tpName,
			}
		} else if l := listOfTypeParameter(param.Type, typeParamNameToID); l != nil {
			jaParam.ParameterType = l
		} else {
			jaParam.ParameterType = astDataTypeToJavaActionParamType(param.Type)
		}
		var storedDescription string
		if sp := storedParams[param.Name]; sp != nil {
			storedDescription = sp.Description
			jaParam.Category = sp.Category
		}
		// A doc comment before the parameter sets its description; none keeps
		// the stored one (mendixlabs/mxcli#1588).
		jaParam.Description = carriedDocumentation(param.DescriptionSet, param.Description, storedDescription)
		ja.Parameters = append(ja.Parameters, jaParam)
	}

	// Convert return type — check if it references a type parameter
	if isTypeParamRef(returnType, typeParamNames) {
		tpName := getTypeParamRefName(returnType)
		ja.ReturnType = &javaactions.TypeParameter{
			BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
			TypeParameterID: typeParamNameToID[tpName],
			TypeParameter:   tpName,
		}
	} else if l := listOfTypeParameter(returnType, typeParamNameToID); l != nil {
		ja.ReturnType = l
	} else {
		if ja.ReturnType, err = astDataTypeToJavaActionReturnType(returnType); err != nil {
			return mdlerrors.NewValidationf("java action %s: %v", s.Name.String(), err)
		}
	}

	// Fold the EXPOSED AS clause onto the stored toolbox entry rather than
	// rebuilding it: the four bitmaps are not expressible in MDL.
	if ja.MicroflowActionInfo, err = mergeMicroflowActionInfo(ctx,
		existingActionInfo, s.ExposedCaption, s.ExposedCategory, s.NotExposed,
		s.ExposedBitmaps, exposeWarner(ctx)); err != nil {
		return err
	}

	// Create or update in MPR
	if existingJAID != "" {
		if err := ctx.Backend.UpdateJavaAction(ja); err != nil {
			return mdlerrors.NewBackend("update java action", err)
		}
		if _, err := applyDocumentFolder(ctx, ja.ID, existingContainer, containerID); err != nil {
			return err
		}
	} else {
		if err := ctx.Backend.CreateJavaAction(ja); err != nil {
			return mdlerrors.NewBackend("create java action", err)
		}
	}

	// Write Java source file if code is provided. The placeholder DESCRIBE
	// prints when it could not read the source is not code: executing it would
	// overwrite the real implementation, or put a stub beside an add-on
	// module's compiled one. The statement says nothing about the code, so the
	// file is left alone.
	if s.JavaCode != "" && strings.TrimSpace(s.JavaCode) != javaSourceOmittedBody {
		if err := ctx.Backend.WriteJavaSourceFile(moduleName, s.Name.Name, s.JavaCode, ja.Parameters, ja.ReturnType, s.Imports, s.ExtraCode, ja.Documentation); err != nil {
			return mdlerrors.NewBackend("write java source file", err)
		}
	}

	// Clear cache
	ctx.InvalidateCache()

	if existingJAID != "" {
		ctx.ReportMutation("Modified", "java action: %s.%s", s.Name.Module, s.Name.Name)
	} else {
		fmt.Fprintf(ctx.Output, "Created java action: %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// astDataTypeToJavaActionParamType converts an AST DataType to a Java action parameter type.
func astDataTypeToJavaActionParamType(dt ast.DataType) javaactions.CodeActionParameterType {
	switch dt.Kind {
	case ast.TypeBoolean:
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$BooleanType",
			},
		}
	case ast.TypeInteger:
		return &javaactions.IntegerType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$IntegerType",
			},
		}
	case ast.TypeLong:
		return &javaactions.LongType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$LongType",
			},
		}
	case ast.TypeDecimal:
		return &javaactions.DecimalType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$DecimalType",
			},
		}
	case ast.TypeString:
		return &javaactions.StringType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$StringType",
			},
		}
	case ast.TypeDateTime, ast.TypeDate:
		return &javaactions.DateTimeType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$DateTimeType",
			},
		}
	case ast.TypeStringTemplate:
		// `StringTemplate(Text)` / `StringTemplate(Sql)` — what describe prints
		// for a text-template parameter. It fell to the default below and was
		// written as a String: a describe -> exec round trip turned the
		// parameter into a String, and every call Studio Pro had written for
		// it (a template value) no longer matched — CE0115.
		grammar := "Text"
		if strings.EqualFold(dt.TemplateContext, "sql") {
			grammar = "Sql"
		}
		return &javaactions.StringTemplateParameterType{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Grammar:     grammar,
		}
	case ast.TypeEntity, ast.TypeEnumeration:
		// An unambiguous `ENUM`/`Enumeration(...)` type must serialize as an
		// enumeration parameter, not an entity reference (#680). A bare
		// qualified name (ExplicitEnum=false) can't be told apart from an entity
		// by the parser, so it stays an entity type here: the create handlers
		// settle it against the project first (settleCodeActionSignature,
		// ako/mxcli#935), which retypes an enumeration as an explicit one.
		if dt.Kind == ast.TypeEnumeration && dt.ExplicitEnum && dt.EnumRef != nil {
			return &javaactions.EnumerationType{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "CodeActions$EnumerationType",
				},
				Enumeration: dt.EnumRef.Module + "." + dt.EnumRef.Name,
			}
		}
		// `Microflow` / `Nanoflow` are what DESCRIBE prints for a callback
		// parameter (MCPServer.AddTool's ExecutingMicroflow, and every other
		// "register a handler" action). The parser cannot tell a bare name from
		// an entity, so without this the DESCRIBE output round-tripped into an
		// entity type with an empty module — `.Microflow`. Only the unqualified
		// name is treated this way; a real `Module.Microflow` entity is not.
		if bare := bareDataTypeName(dt); bare == "Microflow" || bare == "Nanoflow" {
			id := model.ID(types.GenerateID())
			if bare == "Nanoflow" {
				return &javaactions.NanoflowType{BaseElement: model.BaseElement{ID: id}}
			}
			return &javaactions.MicroflowType{BaseElement: model.BaseElement{ID: id}}
		}
		entityName := ""
		if dt.EntityRef != nil {
			entityName = dt.EntityRef.Module + "." + dt.EntityRef.Name
		} else if dt.EnumRef != nil {
			entityName = dt.EnumRef.Module + "." + dt.EnumRef.Name
		}
		return &javaactions.EntityType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$EntityType",
			},
			Entity: entityName,
		}
	case ast.TypeListOf:
		entityName := ""
		if dt.EntityRef != nil {
			entityName = dt.EntityRef.Module + "." + dt.EntityRef.Name
		}
		return &javaactions.ListType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$ListType",
			},
			Entity: entityName,
		}
	default:
		// Default to String type for unknown kinds
		return &javaactions.StringType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$StringType",
			},
		}
	}
}

// bareDataTypeName returns the name of an unqualified entity/enumeration data
// type (no module part), or "" when the type is qualified or absent.
func bareDataTypeName(dt ast.DataType) string {
	switch {
	case dt.EntityRef != nil && dt.EntityRef.Module == "":
		return dt.EntityRef.Name
	case dt.EnumRef != nil && dt.EnumRef.Module == "":
		return dt.EnumRef.Name
	}
	return ""
}

// astDataTypeToJavaActionReturnType converts an AST DataType to a Java (or
// JavaScript) action return type. A type it does not handle is an error: the
// default used to be Boolean, so `returns StringTemplate(Text)` — which a code
// action cannot return at all — was written as Boolean without a word. Only an
// omitted `returns` (TypeUnknown) is Boolean, Studio Pro's default for a new
// action. check reports the same error as MDL-JA03 (validateJavaActionReturnType).
func astDataTypeToJavaActionReturnType(dt ast.DataType) (javaactions.CodeActionReturnType, error) {
	switch dt.Kind {
	case ast.TypeVoid:
		return &javaactions.VoidType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$VoidType",
			},
		}, nil
	case ast.TypeBoolean:
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$BooleanType",
			},
		}, nil
	case ast.TypeInteger:
		return &javaactions.IntegerType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$IntegerType",
			},
		}, nil
	case ast.TypeLong:
		return &javaactions.LongType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$LongType",
			},
		}, nil
	case ast.TypeDecimal:
		return &javaactions.DecimalType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$DecimalType",
			},
		}, nil
	case ast.TypeString:
		return &javaactions.StringType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$StringType",
			},
		}, nil
	case ast.TypeDateTime, ast.TypeDate:
		return &javaactions.DateTimeType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$DateTimeType",
			},
		}, nil
	case ast.TypeEntity, ast.TypeEnumeration:
		// An unambiguous `ENUM`/`Enumeration(...)` return type serializes as an
		// enumeration, not an entity reference (#680). A bare qualified name
		// can't be told apart from an entity, so it stays an entity type; the
		// create handlers settle it first (settleCodeActionSignature).
		// "void" parses as a bare qualified name; treat it as VoidType.
		if dt.Kind == ast.TypeEnumeration && dt.ExplicitEnum && dt.EnumRef != nil {
			return &javaactions.EnumerationType{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "CodeActions$EnumerationType",
				},
				Enumeration: dt.EnumRef.Module + "." + dt.EnumRef.Name,
			}, nil
		}
		entityName := ""
		if dt.EntityRef != nil {
			entityName = dt.EntityRef.Module + "." + dt.EntityRef.Name
		} else if dt.EnumRef != nil {
			entityName = dt.EnumRef.Module + "." + dt.EnumRef.Name
		}
		if strings.EqualFold(strings.TrimPrefix(entityName, "."), "void") {
			return &javaactions.VoidType{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "CodeActions$VoidType",
				},
			}, nil
		}
		return &javaactions.EntityType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$EntityType",
			},
			Entity: entityName,
		}, nil
	case ast.TypeListOf:
		entityName := ""
		if dt.EntityRef != nil {
			entityName = dt.EntityRef.Module + "." + dt.EntityRef.Name
		}
		return &javaactions.ListType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$ListType",
			},
			Entity: entityName,
		}, nil
	case ast.TypeUnknown:
		// No `returns` clause: Boolean, what Studio Pro gives a new action.
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$BooleanType",
			},
		}, nil
	default:
		return nil, codeActionReturnTypeRefusal(dt)
	}
}

// isTypeParamRef checks if an AST DataType is a bare name that matches a type parameter.
// Type parameter names like "pEntity" parse as either TypeEnumeration (with EnumRef)
// or TypeEntity (with EntityRef) with a single-part name (no module prefix).
func isTypeParamRef(dt ast.DataType, typeParamNames map[string]bool) bool {
	name := getTypeParamRefName(dt)
	return name != "" && typeParamNames[name]
}

// getTypeParamRefName extracts the name from a DataType that could be a type parameter reference.
// Returns empty string if the DataType doesn't look like a type parameter reference.
func getTypeParamRefName(dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeEnumeration:
		if dt.EnumRef != nil && dt.EnumRef.Module == "" {
			return dt.EnumRef.Name
		}
		if dt.EnumRef != nil {
			return dt.EnumRef.Module + "." + dt.EnumRef.Name
		}
	case ast.TypeEntity:
		if dt.EntityRef != nil && dt.EntityRef.Module == "" {
			return dt.EntityRef.Name
		}
		if dt.EntityRef != nil {
			return dt.EntityRef.Module + "." + dt.EntityRef.Name
		}
	}
	return ""
}

// undeclaredJavaActionTypeNames lists the parameters (and "the return type")
// typed by an unqualified name that is not one of the action's type
// parameters. Only a type parameter can be unqualified: an entity or an
// enumeration is always Module.Name, so such a name was written as the entity
// `.Name` — CE1613 at build time, after check and exec had both passed
// (mendixlabs/mxcli#1404).
func undeclaredJavaActionTypeNames(s *ast.CreateJavaActionStmt) []string {
	declared := map[string]bool{}
	for _, tp := range s.TypeParameters {
		declared[tp] = true
	}
	unqualified := func(dt ast.DataType) string {
		// The callback types DESCRIBE prints bare, which exec maps itself.
		if bare := bareDataTypeName(dt); bare == "Microflow" || bare == "Nanoflow" {
			return ""
		}
		switch dt.Kind {
		case ast.TypeEntity, ast.TypeListOf:
			if dt.EntityRef != nil && dt.EntityRef.Module == "" {
				return dt.EntityRef.Name
			}
		case ast.TypeEnumeration:
			if dt.EnumRef != nil && dt.EnumRef.Module == "" {
				return dt.EnumRef.Name
			}
		}
		return ""
	}
	var out []string
	check := func(what string, dt ast.DataType) {
		if n := unqualified(dt); n != "" && !declared[n] {
			out = append(out, fmt.Sprintf("%s is typed %s", what, mdlIdent(n)))
		}
	}
	for _, p := range s.Parameters {
		check("parameter "+p.Name, p.Type)
	}
	check("the return type", s.ReturnType)
	return out
}

// undeclaredJavaActionTypeHint is the remedy both check and exec print.
func undeclaredJavaActionTypeHint(s *ast.CreateJavaActionStmt) string {
	return fmt.Sprintf("qualify the entity as Module.Entity, or declare the type parameter: "+
		"`create java action %s<T>(…)` (or an `entity <T>` parameter)", s.Name.String())
}

// listOfTypeParameter returns the list type for `list of T` when T is a type
// parameter declared on the action, and nil otherwise. Without it the element
// went through the entity path and became the entity `.T` — an empty module
// (#1183). Only an unqualified name can be a type parameter.
func listOfTypeParameter(dt ast.DataType, typeParamNameToID map[string]model.ID) *javaactions.ListType {
	if dt.Kind != ast.TypeListOf || dt.EntityRef == nil || dt.EntityRef.Module != "" {
		return nil
	}
	id, ok := typeParamNameToID[dt.EntityRef.Name]
	if !ok {
		return nil
	}
	return &javaactions.ListType{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "CodeActions$ListType",
		},
		TypeParameter:   dt.EntityRef.Name,
		TypeParameterID: id,
	}
}
