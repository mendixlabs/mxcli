// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateModelStatement bridges the createModelStatement parse tree to
// an ast.CreateModelStmt. Property values are dispatched by alternative:
// identifierOrKeyword (Provider), qualifiedName (Key), or STRING_LITERAL
// (Portal-populated metadata).
func (b *Builder) ExitCreateModelStatement(ctx *parser.CreateModelStatementContext) {
	stmt := &ast.CreateModelStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	b.checkModelProperties(ctx.AllModelProperty(), &modelSchema)
	for _, p := range ctx.AllModelProperty() {
		propCtx := p.(*parser.ModelPropertyContext)
		// The first identifierOrKeyword is the property name; the second
		// (if present) or the literal is the value.
		idents := propCtx.AllIdentifierOrKeyword()
		if len(idents) == 0 {
			continue
		}
		key := strings.ToLower(idents[0].GetText())
		switch key {
		case "provider":
			if len(idents) > 1 {
				stmt.Provider = idents[1].GetText()
			}
		case "key":
			if qn := propCtx.QualifiedName(); qn != nil {
				k := buildQualifiedName(qn)
				stmt.Key = &k
			}
		case "displayname":
			if lit := propCtx.STRING_LITERAL(); lit != nil {
				stmt.DisplayName = unquoteStringLit(lit)
			}
		case "keyname":
			if lit := propCtx.STRING_LITERAL(); lit != nil {
				stmt.KeyName = unquoteStringLit(lit)
			}
		case "keyid":
			if lit := propCtx.STRING_LITERAL(); lit != nil {
				stmt.KeyID = unquoteStringLit(lit)
			}
		case "environment":
			if lit := propCtx.STRING_LITERAL(); lit != nil {
				stmt.Environment = unquoteStringLit(lit)
			}
		case "resourcename":
			if lit := propCtx.STRING_LITERAL(); lit != nil {
				stmt.ResourceName = unquoteStringLit(lit)
			}
		case "deeplinkurl":
			if lit := propCtx.STRING_LITERAL(); lit != nil {
				stmt.DeepLinkURL = unquoteStringLit(lit)
			}
		}
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// parseModelProps extracts key-value pairs from a list of modelProperty
// contexts. Returns a map[lowercaseKey]→value. Values are either the
// second identifierOrKeyword text, a qualifiedName string, a string
// literal (unquoted), or an integer string.
func parseModelProps(props []parser.IModelPropertyContext) map[string]string {
	m := make(map[string]string, len(props))
	for _, p := range props {
		pc := p.(*parser.ModelPropertyContext)
		idents := pc.AllIdentifierOrKeyword()
		if len(idents) == 0 {
			continue
		}
		key := strings.ToLower(idents[0].GetText())
		if qn := pc.QualifiedName(); qn != nil {
			m[key] = getQualifiedNameText(qn)
		} else if lit := pc.STRING_LITERAL(); lit != nil {
			m[key] = unquoteStringLit(lit)
		} else if num := pc.NUMBER_LITERAL(); num != nil {
			m[key] = num.GetText()
		} else if bl := pc.BooleanLiteral(); bl != nil {
			m[key] = strings.ToLower(bl.GetText())
		} else if dq := pc.DOLLAR_STRING(); dq != nil {
			m[key] = unquoteDollarString(dq.GetText())
		} else if len(idents) > 1 {
			m[key] = idents[1].GetText()
		}
	}
	return m
}

// parseVariableDefsFromProps scans modelProperty contexts for a
// Variables: (...) entry and parses the variable list. Returns nil if
// no Variables property is present.
func parseVariableDefsFromProps(props []parser.IModelPropertyContext) []ast.AgentVarDef {
	for _, p := range props {
		pc := p.(*parser.ModelPropertyContext)
		idents := pc.AllIdentifierOrKeyword()
		if len(idents) == 0 {
			continue
		}
		if !strings.EqualFold(idents[0].GetText(), "variables") {
			continue
		}
		vdl := pc.VariableDefList()
		if vdl == nil {
			continue
		}
		vdlCtx := vdl.(*parser.VariableDefListContext)
		var result []ast.AgentVarDef
		for _, vd := range vdlCtx.AllVariableDef() {
			vdc := vd.(*parser.VariableDefContext)
			// Key is STRING_LITERAL or QUOTED_IDENTIFIER
			var key string
			if sl := vdc.STRING_LITERAL(); sl != nil {
				key = unquoteStringLit(sl)
			} else if qi := vdc.QUOTED_IDENTIFIER(); qi != nil {
				key = unquoteIdentifier(qi.GetText())
			}
			// Type is identifierOrKeyword
			typeStr := ""
			if iok := vdc.IdentifierOrKeyword(); iok != nil {
				typeStr = iok.GetText()
			}
			result = append(result, ast.AgentVarDef{
				Key:                 key,
				IsAttributeInEntity: strings.EqualFold(typeStr, "EntityAttribute"),
			})
		}
		return result
	}
	return nil
}

// unquoteIdentifier and unquoteDollarString are defined in visitor_helpers.go
// and visitor_dbconnection.go respectively — reuse those.

// ExitCreateConsumedMCPServiceStatement bridges the grammar to AST.
func (b *Builder) ExitCreateConsumedMCPServiceStatement(ctx *parser.CreateConsumedMCPServiceStatementContext) {
	stmt := &ast.CreateConsumedMCPServiceStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	stmt.OuterDocumentation, stmt.DocumentationSet = findDocComment(ctx)
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	b.checkModelProperties(ctx.AllModelProperty(), &consumedMCPServiceSchema)
	props := parseModelProps(ctx.AllModelProperty())
	stmt.ProtocolVersion = props["protocolversion"]
	stmt.Version = props["version"]
	stmt.InnerDocumentation = props["documentation"]
	if v, ok := props["connectiontimeoutseconds"]; ok {
		fmt.Sscanf(v, "%d", &stmt.ConnectionTimeoutSeconds)
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// ExitCreateKnowledgeBaseStatement bridges the grammar to AST.
func (b *Builder) ExitCreateKnowledgeBaseStatement(ctx *parser.CreateKnowledgeBaseStatementContext) {
	stmt := &ast.CreateKnowledgeBaseStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	b.checkModelProperties(ctx.AllModelProperty(), &knowledgeBaseSchema)
	props := parseModelProps(ctx.AllModelProperty())
	stmt.Provider = props["provider"]
	if k, ok := props["key"]; ok {
		kqn := parseQualifiedNameString(k)
		stmt.Key = &kqn
	}
	stmt.ModelDisplayName = props["modeldisplayname"]
	stmt.ModelName = props["modelname"]
	stmt.KeyName = props["keyname"]
	stmt.KeyID = props["keyid"]
	stmt.Environment = props["environment"]
	stmt.DeepLinkURL = props["deeplinkurl"]

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// ExitCreateAgentStatement bridges the grammar to AST.
func (b *Builder) ExitCreateAgentStatement(ctx *parser.CreateAgentStatementContext) {
	stmt := &ast.CreateAgentStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	b.checkModelProperties(ctx.AllModelProperty(), &agentSchema)
	props := parseModelProps(ctx.AllModelProperty())
	stmt.UsageType = props["usagetype"]
	stmt.Description = props["description"]
	if m, ok := props["model"]; ok {
		mqn := parseQualifiedNameString(m)
		stmt.Model = &mqn
	}
	if e, ok := props["entity"]; ok {
		eqn := parseQualifiedNameString(e)
		stmt.Entity = &eqn
	}
	stmt.SystemPrompt = props["systemprompt"]
	stmt.UserPrompt = props["userprompt"]
	stmt.ToolChoice = props["toolchoice"]
	stmt.EnableBackendStreaming = boolProp(props, "enablebackendstreaming")
	stmt.EnableUIStreaming = boolProp(props, "enableuistreaming")
	if v, ok := props["selectedmodel"]; ok {
		stmt.SelectedModel = &v
	}
	if v, ok := props["selectedmodelname"]; ok {
		stmt.SelectedModelName = &v
	}

	if v, ok := props["maxtokens"]; ok {
		var n int
		fmt.Sscanf(v, "%d", &n)
		stmt.MaxTokens = &n
	}
	if v, ok := props["temperature"]; ok {
		var f float64
		fmt.Sscanf(v, "%g", &f)
		stmt.Temperature = &f
	}
	if v, ok := props["topp"]; ok {
		var f float64
		fmt.Sscanf(v, "%g", &f)
		stmt.TopP = &f
	}

	// Parse Variables: ("Key": EntityAttribute, "Key2": String)
	stmt.Variables = parseVariableDefsFromProps(ctx.AllModelProperty())

	// Parse body blocks (TOOL, MCP SERVICE, KNOWLEDGE BASE)
	if body := ctx.AgentBody(); body != nil {
		bodyCtx := body.(*parser.AgentBodyContext)
		for _, block := range bodyCtx.AllAgentBodyBlock() {
			blk := block.(*parser.AgentBodyBlockContext)
			b.checkAgentBodyBlock(blk)
			blockProps := parseModelProps(blk.AllModelProperty())

			if blk.MCP() != nil && blk.SERVICE() != nil {
				// MCP SERVICE block
				td := ast.AgentToolDef{
					ToolType: "MCP",
					Enabled:  true,
				}
				if qn := blk.QualifiedName(); qn != nil {
					doc := buildQualifiedName(qn)
					td.Document = &doc
					td.Name = doc.String()
				}
				if v, ok := blockProps["enabled"]; ok {
					td.Enabled = strings.EqualFold(v, "true")
				}
				td.Description = blockProps["description"]
				b.toolUserSettings(&td, blockProps)
				stmt.Tools = append(stmt.Tools, td)
			} else if blk.KNOWLEDGE() != nil && blk.BASE() != nil {
				// KNOWLEDGE BASE block
				kbd := ast.AgentKBToolDef{
					Enabled: true,
				}
				if iok := blk.IdentifierOrKeyword(); iok != nil {
					kbd.Name = iok.GetText()
				}
				if src, ok := blockProps["source"]; ok {
					sqn := parseQualifiedNameString(src)
					kbd.Source = &sqn
				}
				kbd.Collection = blockProps["collection"]
				kbd.Description = blockProps["description"]
				if v, ok := blockProps["maxresults"]; ok {
					fmt.Sscanf(v, "%d", &kbd.MaxResults)
				}
				if v, ok := blockProps["enabled"]; ok {
					kbd.Enabled = strings.EqualFold(v, "true")
				}
				kbRetrievalSettings(&kbd, blockProps)
				stmt.KBTools = append(stmt.KBTools, kbd)
			} else if blk.TOOL() != nil {
				stmt.Tools = append(stmt.Tools, b.toolBlockDef(blk, blockProps))
			}
		}
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// toolBlockDef reads a `tool Name ( … )` block of an agent body, on create and
// on alter … add.
func (b *Builder) toolBlockDef(blk *parser.AgentBodyBlockContext, props map[string]string) ast.AgentToolDef {
	td := ast.AgentToolDef{ToolType: "Microflow", Enabled: true}
	if iok := blk.IdentifierOrKeyword(); iok != nil {
		td.Name = iok.GetText()
	}
	if v := props["tooltype"]; v != "" {
		td.ToolType = v
	}
	// describe prints the microflow a tool runs as Document. Ignoring it left
	// the rewritten tool wired to nothing.
	if doc, ok := props["document"]; ok {
		dqn := parseQualifiedNameString(doc)
		td.Document = &dqn
	}
	td.Description = props["description"]
	if v, ok := props["enabled"]; ok {
		td.Enabled = strings.EqualFold(v, "true")
	}
	b.toolUserSettings(&td, props)
	return td
}

// userAccessApprovals are GenAICommons.ENUM_UserAccessApproval's values, the
// ones Agent Editor stores on a tool.
var userAccessApprovals = []string{"HiddenForUser", "VisibleForUser", "UserConfirmationRequired"}

// toolUserSettings reads what a tool or mcp service block says about the
// user's view of the call (#2021).
func (b *Builder) toolUserSettings(td *ast.AgentToolDef, props map[string]string) {
	if v, ok := props["useraccessapproval"]; ok {
		found := false
		for _, a := range userAccessApprovals {
			if strings.EqualFold(v, a) {
				td.UserAccessApproval, found = a, true
			}
		}
		if !found {
			b.addError(fmt.Errorf("tool %s: UserAccessApproval takes %s, not %q", td.Name, strings.Join(userAccessApprovals, ", "), v))
		}
	}
	if v, ok := props["displaytitle"]; ok {
		td.DisplayTitle = &v
	}
	if v, ok := props["displaydescription"]; ok {
		td.DisplayDescription = &v
	}
}

// kbRetrievalSettings reads a knowledge base block's MinSimilarity and
// IsVisible.
func kbRetrievalSettings(kbd *ast.AgentKBToolDef, props map[string]string) {
	if v, ok := props["minsimilarity"]; ok {
		var f float64
		fmt.Sscanf(v, "%g", &f)
		kbd.MinSimilarity = &f
	}
	kbd.IsVisible = boolProp(props, "isvisible")
}

// boolProp is a true/false property, nil when the list leaves it out.
func boolProp(props map[string]string, key string) *bool {
	v, ok := props[key]
	if !ok {
		return nil
	}
	b := strings.EqualFold(v, "true")
	return &b
}

// parseQualifiedNameString splits "Module.Name" into a QualifiedName.
func parseQualifiedNameString(s string) ast.QualifiedName {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) == 2 {
		return ast.QualifiedName{Module: parts[0], Name: parts[1]}
	}
	return ast.QualifiedName{Module: s, Name: s}
}
