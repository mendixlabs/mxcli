// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ============================================================================
// OData CREATE Statements
// ============================================================================

// ExitCreateODataClientStatement handles CREATE ODATA CLIENT Module.Name (...).
func (b *Builder) ExitCreateODataClientStatement(ctx *parser.CreateODataClientStatementContext) {
	stmt := &ast.CreateODataClientStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	// `folder '…'` after the name (R9); `Folder:` in the list is its alias.
	folderClause := ctx.STRING_LITERAL() != nil
	if folderClause {
		stmt.Folder = unquoteStringLit(ctx.STRING_LITERAL())
	}

	// Parse property assignments
	for i, propCtx := range ctx.AllOdataPropertyAssignment() {
		prop := propCtx.(*parser.OdataPropertyAssignmentContext)
		name := identifierOrKeywordText(prop.IdentifierOrKeyword())
		value := odataAssignmentValueText(prop)

		switch strings.ToLower(name) {
		case "version":
			stmt.Version = value
		case "odataversion":
			stmt.ODataVersion = value
		case "metadataurl":
			stmt.MetadataUrl = value
		case "timeout":
			stmt.TimeoutExpression = value
		case "proxytype":
			stmt.ProxyType = value
		case "description":
			stmt.Description = value
		case "serviceurl":
			stmt.ServiceUrl = value
		case "useauthentication":
			stmt.UseAuthentication = strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
		// Expression-typed: the expression as written, not an unquoted value.
		case "httpusername":
			stmt.HttpUsername, stmt.HttpUsernameIsLiteral = b.odataExpressionValue(prop.OdataPropertyValue(), prop.Expression())
		case "httppassword":
			stmt.HttpPassword, stmt.HttpPasswordIsLiteral = b.odataExpressionValue(prop.OdataPropertyValue(), prop.Expression())
		case "clientcertificate":
			stmt.ClientCertificate, _ = b.odataExpressionValue(prop.OdataPropertyValue(), prop.Expression())
		case "configurationmicroflow":
			// "Configuration microflow" — returns System.ConsumedODataConfiguration.
			stmt.ConfigurationMicroflow = value
		case "headersmicroflow":
			// "Headers microflow" — returns list of System.HttpHeader. A distinct
			// storage slot from the configuration microflow on Mendix >= 11.10
			// (HeaderListMicroflow vs ConfigurationEntityMicroflow); conflating
			// them triggers CE6808.
			stmt.HeadersMicroflow = value
		case "errorhandlingmicroflow":
			stmt.ErrorHandlingMicroflow = value
		case "proxyhost":
			stmt.ProxyHost = value
		case "proxyport":
			stmt.ProxyPort = value
		case "proxyusername":
			stmt.ProxyUsername = value
		case "proxypassword":
			stmt.ProxyPassword = value
		case "folder":
			if !folderClause {
				stmt.Folder = value
			}
			b.recordFolderProperty(ctx.QualifiedName(), ruleContexts(ctx.AllOdataPropertyAssignment()), i, value, folderClause, nil)
		default:
			stmt.UnknownProperties = append(stmt.UnknownProperties, name)
		}
	}

	// Parse HEADERS clause
	if headersCtx := ctx.OdataHeadersClause(); headersCtx != nil {
		stmt.Headers = b.parseODataHeaders(headersCtx)
	}

	// Check for CREATE OR MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	b.statements = append(b.statements, stmt)
}

// ExitCreateODataServiceStatement handles CREATE ODATA SERVICE Module.Name (...) AUTHENTICATION ... { ... }.
func (b *Builder) ExitCreateODataServiceStatement(ctx *parser.CreateODataServiceStatementContext) {
	stmt := &ast.CreateODataServiceStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	// `folder '…'` after the name (R9); `Folder:` in the list is its alias.
	folderClause := ctx.STRING_LITERAL() != nil
	if folderClause {
		stmt.Folder = unquoteStringLit(ctx.STRING_LITERAL())
	}

	// Parse property assignments
	for i, propCtx := range ctx.AllOdataPropertyAssignment() {
		prop := propCtx.(*parser.OdataPropertyAssignmentContext)
		name := identifierOrKeywordText(prop.IdentifierOrKeyword())
		value := odataAssignmentValueText(prop)

		switch strings.ToLower(name) {
		case "path":
			stmt.Path = value
		case "version":
			stmt.Version = value
		case "odataversion":
			stmt.ODataVersion = value
		case "namespace":
			stmt.Namespace = value
		case "servicename":
			stmt.ServiceName = value
		case "summary":
			stmt.Summary = value
		case "description":
			stmt.Description = value
		case "publishassociations":
			stmt.PublishAssociations = strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
			stmt.PublishAssociationsSet = true
		case "supportsgraphql":
			stmt.SupportsGraphQL = strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
			stmt.SupportsGraphQLSet = true
		case "authentication":
			if types, mf, ok := b.odataAuthenticationProperty(prop); ok {
				stmt.AuthenticationTypes, stmt.AuthMicroflow, stmt.AuthenticationSet = types, mf, true
			}
		case "folder":
			if !folderClause {
				stmt.Folder = value
			}
			b.recordFolderProperty(ctx.QualifiedName(), ruleContexts(ctx.AllOdataPropertyAssignment()), i, value, folderClause, nil)
		default:
			stmt.UnknownProperties = append(stmt.UnknownProperties, name)
		}
	}

	// The trailing `authentication …` clause: the old spelling of the
	// Authentication property (MDL-DEPR139).
	if authCtx, ok := ctx.OdataAuthenticationClause().(*parser.OdataAuthenticationClauseContext); ok && authCtx != nil {
		if stmt.AuthenticationSet {
			b.addError(fmt.Errorf("line %d: authentication is stated both as the Authentication property and as "+
				"the trailing authentication clause; keep the property", authCtx.GetStart().GetLine()))
		} else {
			stmt.AuthenticationTypes, stmt.AuthMicroflow = parseODataAuthTypes(authCtx.AllOdataAuthType())
			stmt.AuthenticationSet = true
			b.recordDeprecation(deprecation.ODataAuthenticationClause, authCtx.AUTHENTICATION().GetSymbol(), "")
			fix, noFix := odataAuthenticationClauseFix(ctx, authCtx)
			b.fixLastDeprecation(deprecation.ODataAuthenticationClause, fix, noFix)
		}
	}

	// Parse PUBLISH MICROFLOW blocks (OData actions)
	for _, blockCtx := range ctx.AllPublishMicroflowBlock() {
		if mf := parsePublishMicroflowBlock(blockCtx); mf != nil {
			stmt.Microflows = append(stmt.Microflows, mf)
		}
	}

	// Parse PUBLISH ENTITY blocks
	for _, blockCtx := range ctx.AllPublishEntityBlock() {
		entity := parsePublishEntityBlock(blockCtx)
		if entity != nil {
			stmt.Entities = append(stmt.Entities, entity)
		}
	}

	// Check for CREATE OR MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	b.statements = append(b.statements, stmt)
}

// ============================================================================
// External Entity Statements
// ============================================================================

// ExitCreateExternalEntityStatement handles CREATE [OR MODIFY] EXTERNAL ENTITY Module.Name FROM ODATA CLIENT ... (...) (...);
func (b *Builder) ExitCreateExternalEntityStatement(ctx *parser.CreateExternalEntityStatementContext) {
	names := ctx.AllQualifiedName()
	if len(names) < 2 {
		return
	}

	stmt := &ast.CreateExternalEntityStmt{
		Name:       buildQualifiedName(names[0]),
		ServiceRef: buildQualifiedName(names[1]),
	}

	// Parse OData property assignments (EntitySet, RemoteName, Countable, etc.)
	for _, propCtx := range ctx.AllOdataPropertyAssignment() {
		prop := propCtx.(*parser.OdataPropertyAssignmentContext)
		name := identifierOrKeywordText(prop.IdentifierOrKeyword())
		value := odataAssignmentValueText(prop)

		boolVal := strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
		strVal := value
		switch strings.ToLower(name) {
		case "entityset":
			stmt.EntitySet = &strVal
		case "remotename":
			stmt.RemoteName = &strVal
		case "countable":
			stmt.Countable = &boolVal
		case "creatable":
			stmt.Creatable = &boolVal
		case "deletable":
			stmt.Deletable = &boolVal
		case "updatable":
			stmt.Updatable = &boolVal
		case "allowcreatechangelocally", "allowcreatingandchanginglocally", "createchangelocally":
			stmt.AllowCreateChangeLocally = &boolVal
		default:
			stmt.UnknownProperties = append(stmt.UnknownProperties, name)
		}
	}

	// Parse attribute definitions (second optional parenthesized block)
	if attrList := ctx.AttributeDefinitionList(); attrList != nil {
		stmt.Attributes = buildAttributes(attrList, b)
	}

	// Check for CREATE OR MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	b.statements = append(b.statements, stmt)
}

// ============================================================================
// CREATE EXTERNAL ENTITIES (bulk)
// ============================================================================

// ExitCreateExternalEntitiesStatement handles CREATE [OR MODIFY] EXTERNAL ENTITIES FROM Module.Service [INTO Module] [ENTITIES (...)].
func (b *Builder) ExitCreateExternalEntitiesStatement(ctx *parser.CreateExternalEntitiesStatementContext) {
	if ctx.QualifiedName(0) == nil {
		return
	}

	stmt := &ast.CreateExternalEntitiesStmt{
		ServiceRef: buildQualifiedName(ctx.QualifiedName(0)),
	}

	// INTO Module — extract target module name
	if ctx.INTO() != nil {
		if ctx.QualifiedName(1) != nil {
			qn := buildQualifiedName(ctx.QualifiedName(1))
			if qn.Module != "" {
				stmt.TargetModule = qn.Module
			} else {
				stmt.TargetModule = qn.Name // single-part name like "Integration"
			}
		} else if id := ctx.IDENTIFIER(); id != nil {
			stmt.TargetModule = id.GetText()
		}
	}

	// ENTITIES (Name1, Name2, ...) — the second ENTITIES token (index 1) indicates the filter clause
	if len(ctx.AllENTITIES()) > 1 {
		for _, iok := range ctx.AllIdentifierOrKeyword() {
			stmt.EntityNames = append(stmt.EntityNames, identifierOrKeywordText(iok))
		}
	}

	// Check for CREATE OR MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	b.statements = append(b.statements, stmt)
}

// ============================================================================
// Helpers
// ============================================================================

// odataValueText extracts the string value from an OData property value context.
func odataValueText(val *parser.OdataPropertyValueContext) string {
	if sl := val.STRING_LITERAL(); sl != nil {
		return unquoteStringLit(sl)
	}
	// A $$…$$ block: a consumed business event service's AsyncAPI Contract
	// (#2384 §5.1). Taken verbatim — it is YAML, and its own quoting is content.
	if ds := val.DOLLAR_STRING(); ds != nil {
		return unquoteDollarString(ds.GetText())
	}
	if nl := val.NUMBER_LITERAL(); nl != nil {
		return nl.GetText()
	}
	if val.TRUE() != nil {
		return "true"
	}
	if val.FALSE() != nil {
		return "false"
	}
	if val.MICROFLOW() != nil {
		if qn := val.QualifiedName(); qn != nil {
			return "MICROFLOW " + getQualifiedNameText(qn)
		}
		return "MICROFLOW"
	}
	if val.AT() != nil {
		if qn := val.QualifiedName(); qn != nil {
			return "@" + getQualifiedNameText(qn)
		}
	}
	if qn := val.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	return ""
}

// odataAssignmentValueText extracts the string value from an OData property assignment.
func odataAssignmentValueText(prop *parser.OdataPropertyAssignmentContext) string {
	valCtx := prop.OdataPropertyValue()
	if valCtx == nil {
		return ""
	}
	return odataValueText(valCtx.(*parser.OdataPropertyValueContext))
}

// parseODataAuthTypes extracts authentication types from the clause, and the
// microflow named by `authentication microflow X`.
//
// The grammar has always accepted the qualified name; only the name was
// discarded, so `authentication microflow M.Auth` parsed, checked and executed
// while the model got a Microflow auth type with no microflow — a service that
// then failed to build with CE0333 (mxcli-formula1 §40).
func parseODataAuthTypes(methods []parser.IOdataAuthTypeContext) (types []string, microflow string) {
	for _, atCtx := range methods {
		at := atCtx.(*parser.OdataAuthTypeContext)
		if at.BASIC() != nil {
			types = append(types, "Basic")
		} else if at.SESSION() != nil {
			types = append(types, "Session")
		} else if at.GUEST() != nil {
			types = append(types, "Guest")
		} else if at.MICROFLOW() != nil {
			types = append(types, "Microflow")
			if qn := at.QualifiedName(); qn != nil {
				microflow = buildQualifiedName(qn).String()
			}
		} else if at.IDENTIFIER() != nil {
			types = append(types, at.IDENTIFIER().GetText())
		}
	}

	return types, microflow
}

// parsePublishEntityBlock converts a PUBLISH ENTITY parse context into an AST node.
func parsePublishEntityBlock(ctx parser.IPublishEntityBlockContext) *ast.PublishedEntityDef {
	block := ctx.(*parser.PublishEntityBlockContext)

	entity := &ast.PublishedEntityDef{
		Entity: buildQualifiedName(block.QualifiedName()),
	}

	// Optional AS 'ExposedName'
	if sl := block.STRING_LITERAL(); sl != nil {
		entity.ExposedName = unquoteStringLit(sl)
	}

	// Parse entity-level properties (ReadMode, InsertMode, etc.)
	for _, propCtx := range block.AllOdataPropertyAssignment() {
		prop := propCtx.(*parser.OdataPropertyAssignmentContext)
		name := identifierOrKeywordText(prop.IdentifierOrKeyword())
		value := odataAssignmentValueText(prop)

		switch strings.ToLower(name) {
		case "readmode":
			entity.ReadMode = value
		case "insertmode":
			entity.InsertMode = value
		case "updatemode":
			entity.UpdateMode = value
		case "deletemode":
			entity.DeleteMode = value
		case "usepaging":
			entity.UsePaging = strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
		case "pagesize":
			if n, err := strconv.Atoi(value); err == nil {
				entity.PageSize = n
			}
		case "countable":
			entity.Countable = odataBoolPtr(value)
		case "skipsupported":
			entity.SkipSupported = odataBoolPtr(value)
		case "topsupported":
			entity.TopSupported = odataBoolPtr(value)
		default:
			entity.UnknownProperties = append(entity.UnknownProperties, name)
		}
	}

	// Parse EXPOSE clause
	if exposeCtx := block.ExposeClause(); exposeCtx != nil {
		entity.Members = parseExposeMembers(exposeCtx)
	}

	return entity
}

// parseODataHeaders converts a HEADERS clause into header definitions.
func (b *Builder) parseODataHeaders(ctx parser.IOdataHeadersClauseContext) []ast.HeaderDef {
	clause := ctx.(*parser.OdataHeadersClauseContext)
	var headers []ast.HeaderDef

	for _, entryCtx := range clause.AllOdataHeaderEntry() {
		entry := entryCtx.(*parser.OdataHeaderEntryContext)
		key := unquoteStringLit(entry.STRING_LITERAL())
		// A header value is a Mendix expression, kept as written.
		value, isLiteral := b.odataExpressionValue(entry.OdataPropertyValue(), entry.Expression())
		headers = append(headers, ast.HeaderDef{Key: key, Value: value, ValueIsLiteral: isLiteral})
	}

	return headers
}

// parsePublishMicroflowBlock converts a PUBLISH MICROFLOW block (an OData
// action) into an AST node. Parameter types and the return type are not read
// here: they come off the microflow at execution time, so the two cannot drift.
func parsePublishMicroflowBlock(ctx parser.IPublishMicroflowBlockContext) *ast.PublishedMicroflowDef {
	block := ctx.(*parser.PublishMicroflowBlockContext)
	if block.QualifiedName() == nil {
		return nil
	}
	def := &ast.PublishedMicroflowDef{
		Microflow: buildQualifiedName(block.QualifiedName()),
	}
	if sl := block.STRING_LITERAL(); sl != nil {
		def.ExposedName = unquoteStringLit(sl)
	}
	if exposeCtx := block.ExposeClause(); exposeCtx != nil {
		expose := exposeCtx.(*parser.ExposeClauseContext)
		if expose.STAR() != nil {
			def.ExposeAll = true
		}
		for _, memberCtx := range expose.AllExposeMember() {
			member := memberCtx.(*parser.ExposeMemberContext)
			if member.IdentifierOrKeyword() == nil {
				continue
			}
			p := &ast.PublishedParamDef{Name: identifierOrKeywordText(member.IdentifierOrKeyword())}
			if sl := member.STRING_LITERAL(); sl != nil {
				p.ExposedName = unquoteStringLit(sl)
			}
			if opts := member.ExposeMemberOptions(); opts != nil {
				optsCtx := opts.(*parser.ExposeMemberOptionsContext)
				for _, id := range optsCtx.AllIdentifierOrKeyword() {
					if strings.EqualFold(id.GetText(), "canbeempty") ||
						strings.EqualFold(id.GetText(), "optional") {
						p.CanBeEmpty = true
					}
				}
			}
			def.Parameters = append(def.Parameters, p)
		}
	} else {
		// No clause at all means every parameter, under its own name.
		def.ExposeAll = true
	}
	return def
}

// parseExposeMembers converts an EXPOSE clause into AST member definitions.
func parseExposeMembers(ctx parser.IExposeClauseContext) []*ast.PublishedMemberDef {
	expose := ctx.(*parser.ExposeClauseContext)

	// EXPOSE (*) means all members — return nil to signal wildcard
	if expose.STAR() != nil {
		return nil
	}

	var members []*ast.PublishedMemberDef
	for _, memberCtx := range expose.AllExposeMember() {
		member := memberCtx.(*parser.ExposeMemberContext)

		// Guard against incomplete parse (e.g., user typing in LSP)
		if member.IdentifierOrKeyword() == nil {
			continue
		}

		m := &ast.PublishedMemberDef{
			Name: identifierOrKeywordText(member.IdentifierOrKeyword()),
		}

		// Optional AS 'ExposedName'
		if sl := member.STRING_LITERAL(); sl != nil {
			m.ExposedName = unquoteStringLit(sl)
		}

		// Optional options (Filterable, Sortable, IsPartOfKey)
		if opts := member.ExposeMemberOptions(); opts != nil {
			optsCtx := opts.(*parser.ExposeMemberOptionsContext)
			for _, id := range optsCtx.AllIdentifierOrKeyword() {
				switch strings.ToLower(id.GetText()) {
				case "filterable":
					m.Filterable = true
				case "sortable":
					m.Sortable = true
				case "key", "ispartofkey":
					m.IsPartOfKey = true
				}
			}
		}

		members = append(members, m)
	}

	return members
}

// odataBoolPtr parses an OData property value as a bool, keeping "specified"
// distinct from "true": these properties default to true, so only an explicit
// value may turn one off.
func odataBoolPtr(value string) *bool {
	b := strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
	return &b
}

// odataAuthenticationProperty reads a published OData service's
// `Authentication: none | ( method, … )`. The methods keep the order written.
// Unlike the clause it replaces, the property takes only the methods MDL has a
// keyword for: an arbitrary identifier wrote an unknown method into the model.
// ok is false when the value has another shape (reported) or error recovery
// left it empty.
func (b *Builder) odataAuthenticationProperty(prop *parser.OdataPropertyAssignmentContext) (types []string, microflow string, ok bool) {
	line := prop.GetStart().GetLine()
	switch {
	case prop.NONE() != nil, isBareNone(prop):
		return nil, "", true
	case prop.LPAREN() != nil && len(prop.AllOdataAuthType()) > 0:
	default:
		b.addError(fmt.Errorf("line %d: Authentication takes none or a method list "+
			"(basic, session, guest, microflow Module.Name), not %s", line, odataPropertyValueSource(prop)))
		return nil, "", false
	}
	seen := map[string]bool{}
	for _, atCtx := range prop.AllOdataAuthType() {
		at := atCtx.(*parser.OdataAuthTypeContext)
		if at.IDENTIFIER() != nil {
			b.addError(fmt.Errorf("line %d: '%s' is not an authentication method: write basic, session, guest "+
				"or microflow Module.Name", at.GetStart().GetLine(), at.IDENTIFIER().GetText()))
			return nil, "", false
		}
		t, mf := parseODataAuthTypes([]parser.IOdataAuthTypeContext{at})
		if len(t) == 0 {
			continue
		}
		if seen[t[0]] {
			b.addError(fmt.Errorf("line %d: authentication method '%s' listed twice", at.GetStart().GetLine(), strings.ToLower(t[0])))
			continue
		}
		seen[t[0]] = true
		types = append(types, t[0])
		if mf != "" {
			microflow = mf
		}
	}
	return types, microflow, true
}

// isBareNone reports a value of `none` that parsed as a bare name: the first
// alternative of odataPropertyAssignment takes it before the NONE one does.
func isBareNone(prop *parser.OdataPropertyAssignmentContext) bool {
	v, ok := prop.OdataPropertyValue().(*parser.OdataPropertyValueContext)
	return ok && v != nil && v.QualifiedName() != nil && v.MICROFLOW() == nil && v.AT() == nil &&
		strings.EqualFold(v.GetText(), "none")
}

// odataPropertyValueSource is a property's value as written, for a message.
func odataPropertyValueSource(prop *parser.OdataPropertyAssignmentContext) string {
	if prop.COLON() == nil || prop.GetStop() == nil {
		return "that"
	}
	in := prop.GetStart().GetInputStream()
	return strings.TrimSpace(in.GetText(prop.COLON().GetSymbol().GetStop()+1, prop.GetStop().GetStop()))
}

// odataAuthenticationClauseFix moves a trailing `authentication m1, m2` clause
// into the property list as its last property, `Authentication: (m1, m2)`, the
// methods as written. A method MDL has no keyword for has no property spelling,
// so that clause is reported and left alone.
func odataAuthenticationClauseFix(ctx *parser.CreateODataServiceStatementContext, clause *parser.OdataAuthenticationClauseContext) (*ast.Fix, string) {
	props := ctx.AllOdataPropertyAssignment()
	if len(props) == 0 || clause.AUTHENTICATION() == nil || clause.GetStop() == nil {
		return nil, "the statement did not parse completely"
	}
	in := clause.GetStart().GetInputStream()
	var items []string
	for _, atCtx := range clause.AllOdataAuthType() {
		at := atCtx.(*parser.OdataAuthTypeContext)
		if at.IDENTIFIER() != nil {
			return nil, "'" + at.IDENTIFIER().GetText() + "' is not an authentication method MDL has a keyword for"
		}
		items = append(items, in.GetText(at.GetStart().GetStart(), at.GetStop().GetStop()))
	}
	// A list written one property per line gets the new one on its own line,
	// indented like the last; a one-line list gets it inline.
	lastProp := props[len(props)-1]
	last := lastProp.GetStop()
	sep := ", "
	if lp := ctx.LPAREN(); lp != nil && lastProp.GetStart().GetLine() > lp.GetSymbol().GetLine() {
		sep = ",\n" + strings.Repeat(" ", lastProp.GetStart().GetColumn())
	}
	return &ast.Fix{Edits: []ast.TextEdit{
		insertAt(last.GetStop()+1, sep+"Authentication: ("+strings.Join(items, ", ")+")"),
		{Start: startAfterSpace(clause.AUTHENTICATION().GetSymbol()), Stop: clause.GetStop().GetStop() + 1},
	}}, ""
}
