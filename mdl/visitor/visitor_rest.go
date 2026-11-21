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
// REST Client CREATE Statements
// ============================================================================

// ExitCreateRestClientStatement handles the property-based CREATE REST CLIENT syntax.
func (b *Builder) ExitCreateRestClientStatement(ctx *parser.CreateRestClientStatementContext) {
	stmt := &ast.CreateRestClientStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	// `folder '…'` after the name (R9); `Folder:` in the list is its alias.
	folderClause := ctx.STRING_LITERAL() != nil
	if folderClause {
		stmt.Folder = unquoteStringLit(ctx.STRING_LITERAL())
	}

	// Parse service-level properties (BaseUrl, Authentication, Folder)
	for i, propCtx := range ctx.AllRestClientProperty() {
		pc, ok := propCtx.(*parser.RestClientPropertyContext)
		if !ok || pc == nil {
			continue
		}
		iok := pc.IdentifierOrKeyword()
		if iok == nil {
			continue
		}
		rawKey := identifierOrKeywordText(iok.(*parser.IdentifierOrKeywordContext))
		b.checkProperty(pc, &restClientSchema, rawKey, restClientPropertyShape(pc))
		key := strings.ToLower(rawKey)
		switch key {
		case "baseurl":
			if sl := pc.STRING_LITERAL(); sl != nil {
				stmt.BaseUrl = unquoteStringLit(sl)
			}
		case "folder":
			if sl := pc.STRING_LITERAL(); sl != nil {
				if !folderClause {
					stmt.Folder = unquoteStringLit(sl)
				}
				b.recordFolderProperty(ctx.QualifiedName(), ruleContexts(ctx.AllRestClientProperty()), i,
					unquoteStringLit(sl), folderClause, nil)
			}
		case "openapi":
			if sl := pc.STRING_LITERAL(); sl != nil {
				stmt.OpenApiPath = unquoteStringLit(sl)
			}
		case "authentication":
			if pc.BASIC() != nil {
				stmt.Authentication = b.restClientBasicAuth(pc, stmt.Name.Module)
			}
			// NONE: leave Authentication nil
		}
	}

	// Parse operations
	for _, opCtx := range ctx.AllRestClientOperation() {
		if opDef := b.restClientOperation(opCtx); opDef != nil {
			stmt.Operations = append(stmt.Operations, opDef)
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

// restClientBasicAuth reads `Authentication: basic (Username: …, Password: …)`
// for the service of module moduleName.
func (b *Builder) restClientBasicAuth(pc *parser.RestClientPropertyContext, moduleName string) *ast.RestAuthDef {
	authDef := &ast.RestAuthDef{Scheme: "BASIC"}
	for _, subProp := range pc.AllRestClientProperty() {
		sp, spOk := subProp.(*parser.RestClientPropertyContext)
		if !spOk || sp == nil || sp.IdentifierOrKeyword() == nil {
			continue
		}
		rawSubKey := identifierOrKeywordText(sp.IdentifierOrKeyword().(*parser.IdentifierOrKeywordContext))
		b.checkProperty(sp, &restClientBasicAuthSchema, rawSubKey, restClientPropertyShape(sp))
		subKey := strings.ToLower(rawSubKey)
		var val string
		if sl := sp.STRING_LITERAL(); sl != nil {
			val = unquoteStringLit(sl)
		} else if v := sp.VARIABLE(); v != nil {
			// $Constant (MDL-DEPR083): a constant of the service's own
			// module, stored qualified as `@Module.Const` stores it.
			val = "$" + dollarConstantName(moduleName, v.GetText())
			b.recordDollarConstant(v, moduleName)
		} else if sp.AT() != nil {
			// @Module.Constant reference (preferred Mendix convention)
			// Store with $ prefix so the writer serializes as Rest$ConstantValue
			if qn := sp.QualifiedName(); qn != nil {
				val = "$" + getQualifiedNameText(qn)
			}
		}
		switch subKey {
		case "username":
			authDef.Username = val
		case "password":
			authDef.Password = val
		}
	}
	return authDef
}

// restClientOperation checks and reads one `operation Name ( … )`.
func (b *Builder) restClientOperation(opCtx parser.IRestClientOperationContext) *ast.RestOperationDef {
	oc, ok := opCtx.(*parser.RestClientOperationContext)
	if !ok || oc == nil {
		return nil
	}
	for _, p := range oc.AllRestClientOpProp() {
		if pc, ok := p.(*parser.RestClientOpPropContext); ok && pc != nil && pc.IdentifierOrKeyword() != nil {
			key := identifierOrKeywordText(pc.IdentifierOrKeyword().(*parser.IdentifierOrKeywordContext))
			b.checkProperty(pc, &restClientOperationSchema, key, restClientOpPropShape(pc))
		}
	}
	return parseRestClientOperation(oc)
}

// parseRestClientOperation parses a single OPERATION { Key: Value, ... } block.
func parseRestClientOperation(ctx *parser.RestClientOperationContext) *ast.RestOperationDef {
	op := &ast.RestOperationDef{}

	// Operation name
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		op.Name = identifierOrKeywordText(iok)
	} else if sl := ctx.STRING_LITERAL(); sl != nil {
		op.Name = unquoteStringLit(sl)
	}

	// Documentation
	if docCtx := ctx.DocComment(); docCtx != nil {
		op.Documentation = extractDocCommentText(docCtx)
	}

	// Parse properties
	for _, propCtx := range ctx.AllRestClientOpProp() {
		pc, ok := propCtx.(*parser.RestClientOpPropContext)
		if !ok || pc == nil {
			continue
		}
		parseRestClientOpProp(pc, op)
		if iok := pc.IdentifierOrKeyword(); iok != nil {
			if k, ok := restClientOperationSchema.lookup(identifierOrKeywordText(iok.(*parser.IdentifierOrKeywordContext))); ok {
				op.Keys = append(op.Keys, k.name)
			}
		}
	}

	return op
}

// parseRestClientOpProp dispatches a single operation property to the right handler.
func parseRestClientOpProp(ctx *parser.RestClientOpPropContext, op *ast.RestOperationDef) {
	if ctx == nil {
		return
	}
	// Determine key name from the identifierOrKeyword
	iokCtx := ctx.IdentifierOrKeyword()
	if iokCtx == nil {
		return
	}
	key := strings.ToLower(identifierOrKeywordText(iokCtx.(*parser.IdentifierOrKeywordContext)))

	// Method: GET
	if mCtx := ctx.RestHttpMethod(); mCtx != nil {
		op.Method = strings.ToLower(mCtx.GetText())
		return
	}

	// MAPPING qualifiedName { ... }
	if ctx.MAPPING() != nil && ctx.QualifiedName() != nil {
		md := &ast.RestMappingDef{
			Entity: buildQualifiedName(ctx.QualifiedName()),
		}
		for _, entryCtx := range ctx.AllRestClientMappingEntry() {
			md.Entries = append(md.Entries, parseRestClientMappingEntry(entryCtx.(*parser.RestClientMappingEntryContext)))
		}
		switch key {
		case "body":
			op.BodyType = "mapping"
			op.BodyMapping = md
		case "response":
			op.ResponseType = "mapping"
			op.ResponseMapping = md
		}
		return
	}

	// TEMPLATE 'string'
	if ctx.TEMPLATE() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			op.BodyType = "template"
			op.BodyVariable = unquoteStringLit(sl)
		}
		return
	}

	// (JSON|FILE|STRING|STATUS) (FROM|AS) $var
	if ctx.VARIABLE() != nil {
		varName := ctx.VARIABLE().GetText()
		if ctx.FROM() != nil {
			// Body: JSON FROM $var, FILE FROM $var
			if ctx.JSON() != nil {
				op.BodyType = "json"
			} else if ctx.FILE_KW() != nil {
				op.BodyType = "file"
			}
			op.BodyVariable = varName
		} else if ctx.AS() != nil {
			// Response: JSON AS $var, STRING AS $var, etc.
			if ctx.JSON() != nil {
				op.ResponseType = "json"
			} else if ctx.STRING_TYPE() != nil {
				op.ResponseType = "string"
			} else if ctx.FILE_KW() != nil {
				op.ResponseType = "file"
			} else if ctx.STATUS() != nil {
				op.ResponseType = "status"
			}
			op.ResponseVariable = varName
		}
		return
	}

	// Response: JSON (no variable — an operation stores none; mendixlabs/mxcli#1608)
	if key == "response" {
		switch {
		case ctx.JSON() != nil:
			op.ResponseType = "json"
			return
		case ctx.STRING_TYPE() != nil:
			op.ResponseType = "string"
			return
		case ctx.FILE_KW() != nil:
			op.ResponseType = "file"
			return
		case ctx.STATUS() != nil:
			op.ResponseType = "status"
			return
		}
	}

	// NONE
	if ctx.NONE() != nil {
		switch key {
		case "response":
			op.ResponseType = "none"
		case "authentication":
			// handled at service level
		}
		return
	}

	// Param list: ($var: Type, ...)
	paramItems := ctx.AllRestClientParamItem()
	if len(paramItems) > 0 {
		for _, pi := range paramItems {
			pic := pi.(*parser.RestClientParamItemContext)
			param := ast.RestParamDef{}
			if v := pic.VARIABLE(); v != nil {
				param.Name = v.GetText()
			}
			if dt := pic.DataType(); dt != nil {
				param.DataType = buildDataType(dt).Kind.String()
			}
			switch key {
			case "parameters":
				op.Parameters = append(op.Parameters, param)
			case "query":
				op.QueryParameters = append(op.QueryParameters, param)
			}
		}
		return
	}

	// Header list: ('Name': 'Value', ...) — or the old ('Name' = 'Value')
	headerItems := ctx.AllRestClientHeaderItem()
	if len(headerItems) > 0 {
		for _, hi := range headerItems {
			hic := hi.(*parser.RestClientHeaderItemContext)
			header := ast.RestHeaderDef{}
			allSL := hic.AllSTRING_LITERAL()
			if len(allSL) >= 1 {
				header.Name = unquoteStringLit(allSL[0])
			}
			if v := hic.VARIABLE(); v != nil {
				// `'prefix' + $P` / `$P` (MDL-DEPR711): the template
				// `'prefix{P}'`. It used to store the prefix alone.
				prefix := ""
				if len(allSL) >= 2 {
					prefix = unquoteStringLit(allSL[1])
				}
				header.Value = prefix + "{" + strings.TrimPrefix(v.GetText(), "$") + "}"
			} else if len(allSL) >= 2 {
				header.Value = unquoteStringLit(allSL[1])
			}
			op.Headers = append(op.Headers, header)
		}
		return
	}

	// String property: Path: '/items'
	if sl := ctx.STRING_LITERAL(); sl != nil {
		switch key {
		case "path":
			op.Path = unquoteStringLit(sl)
		}
		return
	}

	// Number property: Timeout: 30
	if nl := ctx.NUMBER_LITERAL(); nl != nil {
		switch key {
		case "timeout":
			if val, err := strconv.Atoi(nl.GetText()); err == nil {
				op.Timeout = val
			}
		}
		return
	}
}

// parseRestClientMappingEntry parses a single mapping entry (value or object).
func parseRestClientMappingEntry(ctx *parser.RestClientMappingEntryContext) ast.RestMappingEntry {
	allQN := ctx.AllQualifiedName()
	allIOK := ctx.AllIdentifierOrKeyword()

	// Object mapping: [CREATE] Association/Entity = exposedName { ... }
	if len(allQN) >= 2 {
		entry := ast.RestMappingEntry{
			Create:      ctx.CREATE() != nil,
			Association: buildQualifiedName(allQN[0]),
			Entity:      buildQualifiedName(allQN[1]),
		}
		// ExposedName is the identifierOrKeyword after EQUALS
		if len(allIOK) > 0 {
			entry.ExposedName = identifierOrKeywordText(allIOK[0].(*parser.IdentifierOrKeywordContext))
		}
		for _, childCtx := range ctx.AllRestClientMappingEntry() {
			entry.Children = append(entry.Children, parseRestClientMappingEntry(childCtx.(*parser.RestClientMappingEntryContext)))
		}
		return entry
	}

	// Value mapping: Left = Right
	entry := ast.RestMappingEntry{}
	if len(allIOK) >= 2 {
		entry.Left = identifierOrKeywordText(allIOK[0].(*parser.IdentifierOrKeywordContext))
		entry.Right = identifierOrKeywordText(allIOK[1].(*parser.IdentifierOrKeywordContext))
	}
	return entry
}

// extractDocCommentText extracts documentation text from a DocComment context.
func extractDocCommentText(ctx parser.IDocCommentContext) string {
	if ctx == nil {
		return ""
	}
	return extractDocComment(ctx.GetText())
}

// ExitCreatePublishedRestServiceStatement handles CREATE PUBLISHED REST SERVICE.
func (b *Builder) ExitCreatePublishedRestServiceStatement(ctx *parser.CreatePublishedRestServiceStatementContext) {
	stmt := &ast.CreatePublishedRestServiceStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx) // mendixlabs/mxcli#1510
	// `folder '…'` after the name (R9); `Folder:` in the list is its alias.
	folderClause := ctx.STRING_LITERAL() != nil
	if folderClause {
		stmt.Folder = unquoteStringLit(ctx.STRING_LITERAL())
	}

	// Check for CREATE OR MODIFY (or OR REPLACE, treated identically)
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.REPLACE() != nil || createStmt.MODIFY() != nil) {
			stmt.CreateOrModify = true
		}
	}

	// Parse properties (Path, Version, ServiceName, Authentication)
	for i, propCtx := range ctx.AllPublishedRestProperty() {
		pc := propCtx.(*parser.PublishedRestPropertyContext)
		key, val, auth, ok := b.publishedRestProperty(pc)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "path":
			stmt.Path = val
		case "version":
			stmt.Version = val
		case "servicename":
			stmt.ServiceName = val
		case "authentication":
			stmt.Authentication = auth
		case "folder":
			if !folderClause {
				stmt.Folder = val
			}
			b.recordFolderProperty(ctx.QualifiedName(), ruleContexts(ctx.AllPublishedRestProperty()), i, val, folderClause, nil)
		}
	}

	// Parse resources
	for _, resCtx := range ctx.AllPublishedRestResource() {
		rc := resCtx.(*parser.PublishedRestResourceContext)
		if res := buildPublishedRestResourceDef(rc); res != nil {
			stmt.Resources = append(stmt.Resources, res)
		}
	}

	b.statements = append(b.statements, stmt)
}

// publishedRestProperty reads one `Key: value` of a published REST service's
// property list: the key, the string value, or the authentication value when
// the property is `Authentication`. The value's shape is checked against the
// key, so `Authentication: 'basic'` or `Path: none` is reported rather than
// read as empty. ok is false when error recovery left the property without a
// key.
func (b *Builder) publishedRestProperty(pc *parser.PublishedRestPropertyContext) (key, val string, auth *ast.PublishedRestAuthentication, ok bool) {
	ik, isIK := pc.IdentifierOrKeyword().(*parser.IdentifierOrKeywordContext)
	if !isIK || ik == nil {
		return "", "", nil, false
	}
	key = identifierOrKeywordText(ik)
	shape := shapeOther
	switch {
	case pc.STRING_LITERAL() != nil:
		shape = shapeString
		val = unquoteStringLit(pc.STRING_LITERAL())
	case pc.NONE() != nil:
		shape = shapeNone
		auth = &ast.PublishedRestAuthentication{}
	case pc.LPAREN() != nil:
		shape = shapeAuthMethods
		auth = b.publishedRestAuthMethods(pc)
	}
	b.checkProperty(pc, &publishedRestSchema, key, shape)
	if shape == shapeOther {
		// Error recovery: the syntax error is already recorded.
		return "", "", nil, false
	}
	return key, val, auth, true
}

// publishedRestAuthMethods reads `( basic, session, microflow M.F )` in the
// order written, which is the order Studio Pro stores the methods in.
func (b *Builder) publishedRestAuthMethods(pc *parser.PublishedRestPropertyContext) *ast.PublishedRestAuthentication {
	auth := &ast.PublishedRestAuthentication{}
	seen := map[string]bool{}
	for _, mCtx := range pc.AllPublishedRestAuthMethod() {
		mc := mCtx.(*parser.PublishedRestAuthMethodContext)
		var method string
		switch {
		case mc.BASIC() != nil:
			method = "Basic"
		case mc.SESSION() != nil:
			method = "Session"
		case mc.MICROFLOW() != nil && mc.QualifiedName() != nil:
			method = "Microflow"
			auth.Microflow = buildQualifiedName(mc.QualifiedName()).String()
			for _, pCtx := range mc.AllPublishedRestAuthParameter() {
				if p := publishedRestAuthParameter(pCtx.(*parser.PublishedRestAuthParameterContext)); p != nil {
					auth.Parameters = append(auth.Parameters, p)
				}
			}
		default:
			continue
		}
		if seen[method] {
			b.addError(fmt.Errorf("line %d: authentication method '%s' listed twice",
				mc.GetStart().GetLine(), strings.ToLower(method)))
			continue
		}
		seen[method] = true
		auth.Methods = append(auth.Methods, method)
	}
	return auth
}

// publishedRestAuthParameter reads `$Name: kind ['httpName'] [description '…']`;
// nil when error recovery left it without a name or a kind.
func publishedRestAuthParameter(pc *parser.PublishedRestAuthParameterContext) *ast.PublishedRestAuthParameter {
	ik, ok := pc.IdentifierOrKeyword().(*parser.IdentifierOrKeywordContext)
	if pc.VARIABLE() == nil || !ok || ik == nil {
		return nil
	}
	p := &ast.PublishedRestAuthParameter{
		Name: strings.TrimPrefix(pc.VARIABLE().GetText(), "$"),
		Kind: strings.ToLower(identifierOrKeywordText(ik)),
	}
	strs := pc.AllSTRING_LITERAL()
	if pc.DESCRIPTION() != nil && len(strs) > 0 {
		p.Description = unquoteStringLit(strs[len(strs)-1])
		strs = strs[:len(strs)-1]
	}
	if len(strs) > 0 {
		p.HTTPName = unquoteStringLit(strs[0])
	}
	return p
}

// buildPublishedRestResourceDef converts a PublishedRestResourceContext to a
// PublishedRestResourceDef AST node. Shared by CREATE and ALTER.
// Returns nil when the resource name token is absent (ANTLR error-recovery path).
func buildPublishedRestResourceDef(rc *parser.PublishedRestResourceContext) *ast.PublishedRestResourceDef {
	if rc.STRING_LITERAL() == nil {
		return nil
	}
	resDef := &ast.PublishedRestResourceDef{
		Name: unquoteStringLit(rc.STRING_LITERAL()),
	}

	for _, opCtx := range rc.AllPublishedRestOperation() {
		resDef.Operations = append(resDef.Operations, buildPublishedRestOperationDef(opCtx.(*parser.PublishedRestOperationContext)))
	}

	return resDef
}

// buildPublishedRestOperationDef converts one operation line, shared by a
// resource block and ALTER … ADD OPERATION (ako/mxcli#2449).
func buildPublishedRestOperationDef(oc *parser.PublishedRestOperationContext) *ast.PublishedRestOperationDef {
	opDef := &ast.PublishedRestOperationDef{}

	// HTTP method
	if mCtx := oc.RestHttpMethod(); mCtx != nil {
		opDef.HTTPMethod = strings.ToUpper(mCtx.GetText())
	}

	// Operation path — strip leading/trailing slashes (CE6550/CE6551)
	if pCtx := oc.PublishedRestOpPath(); pCtx != nil {
		pc := pCtx.(*parser.PublishedRestOpPathContext)
		if pc.STRING_LITERAL() != nil {
			opDef.Path = strings.Trim(unquoteStringLit(pc.STRING_LITERAL()), "/")
		}
	}

	// Microflow reference
	allQN := oc.AllQualifiedName()
	if len(allQN) >= 1 {
		opDef.Microflow = buildQualifiedName(allQN[0])
	}

	if oc.DEPRECATED() != nil {
		opDef.Deprecated = true
	}

	// Import/Export mapping (qualifiedName after IMPORT/EXPORT MAPPING)
	if oc.IMPORT() != nil && len(allQN) >= 2 {
		opDef.ImportMapping = buildQualifiedName(allQN[1]).String()
	}
	if oc.EXPORT() != nil {
		idx := 1
		if oc.IMPORT() != nil {
			idx = 2
		}
		if len(allQN) > idx {
			opDef.ExportMapping = buildQualifiedName(allQN[idx]).String()
		}
	}

	if oc.COMMIT() != nil {
		if idCtx := oc.IdentifierOrKeyword(); idCtx != nil {
			opDef.Commit = identifierOrKeywordText(idCtx.(*parser.IdentifierOrKeywordContext))
		}
	}

	return opDef
}

// exitAlterRestClientStatement handles ALTER CONSUMED REST SERVICE
// (mendixlabs/mxcli#1608).
func (b *Builder) exitAlterRestClientStatement(ctx *parser.AlterStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}
	stmt := &ast.AlterRestClientStmt{Name: buildQualifiedName(qn)}
	for _, actCtx := range ctx.AllAlterConsumedRestServiceAction() {
		ac, ok := actCtx.(*parser.AlterConsumedRestServiceActionContext)
		if !ok || ac == nil {
			continue
		}
		switch {
		case ac.SET() != nil:
			set := &ast.RestClientSetAction{}
			for _, propCtx := range ac.AllRestClientProperty() {
				pc, ok := propCtx.(*parser.RestClientPropertyContext)
				if !ok || pc == nil || pc.IdentifierOrKeyword() == nil {
					continue
				}
				rawKey := identifierOrKeywordText(pc.IdentifierOrKeyword().(*parser.IdentifierOrKeywordContext))
				b.checkProperty(pc, &restClientAlterSetSchema, rawKey, restClientPropertyShape(pc))
				switch strings.ToLower(rawKey) {
				case "baseurl":
					if sl := pc.STRING_LITERAL(); sl != nil {
						set.BaseUrl, set.BaseUrlSet = unquoteStringLit(sl), true
					}
				case "authentication":
					switch {
					case pc.BASIC() != nil:
						set.Authentication, set.AuthenticationSet = b.restClientBasicAuth(pc, stmt.Name.Module), true
					case pc.NONE() != nil:
						set.AuthenticationSet = true
					}
				}
			}
			stmt.Actions = append(stmt.Actions, set)
		case ac.ADD() != nil:
			if op := b.restClientOperation(ac.RestClientOperation()); op != nil {
				stmt.Actions = append(stmt.Actions, &ast.RestClientAddOperationAction{Operation: op})
			}
		case ac.MODIFY() != nil:
			if op := b.restClientOperation(ac.RestClientOperation()); op != nil {
				stmt.Actions = append(stmt.Actions, &ast.RestClientModifyOperationAction{Operation: op})
			}
		case ac.DROP() != nil:
			name := ""
			if iok := ac.IdentifierOrKeyword(); iok != nil {
				name = identifierOrKeywordText(iok.(*parser.IdentifierOrKeywordContext))
			} else if sl := ac.STRING_LITERAL(); sl != nil {
				name = unquoteStringLit(sl)
			}
			stmt.Actions = append(stmt.Actions, &ast.RestClientDropOperationAction{Name: name})
		}
	}
	b.statements = append(b.statements, stmt)
}

// exitAlterPublishedRestServiceStatement handles ALTER PUBLISHED REST SERVICE.
func (b *Builder) exitAlterPublishedRestServiceStatement(ctx *parser.AlterStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.AlterPublishedRestServiceStmt{
		Name: buildQualifiedName(qn),
	}

	for _, actCtx := range ctx.AllAlterPublishedRestServiceAction() {
		ac, ok := actCtx.(*parser.AlterPublishedRestServiceActionContext)
		if !ok {
			continue
		}

		// SET ( Key: value, ... ) — create's property list (R3)
		if ac.SET() != nil && ac.LPAREN() != nil {
			set := &ast.PublishedRestSetAction{Changes: make(map[string]string)}
			for _, propCtx := range ac.AllPublishedRestProperty() {
				key, val, auth, ok := b.publishedRestProperty(propCtx.(*parser.PublishedRestPropertyContext))
				if !ok {
					continue
				}
				if strings.EqualFold(key, "authentication") {
					set.Authentication = auth
					continue
				}
				set.Changes[key] = val
			}
			stmt.Actions = append(stmt.Actions, set)
			continue
		}

		// SET key = 'value' [, ...]
		if ac.SET() != nil {
			changes := make(map[string]string)
			for _, asnCtx := range ac.AllPublishedRestAlterAssignment() {
				asn := asnCtx.(*parser.PublishedRestAlterAssignmentContext)
				key := identifierOrKeywordText(asn.IdentifierOrKeyword().(*parser.IdentifierOrKeywordContext))
				val := unquoteStringLit(asn.STRING_LITERAL())
				changes[key] = val
			}
			stmt.Actions = append(stmt.Actions, &ast.PublishedRestSetAction{Changes: changes})
			continue
		}

		// ADD OPERATION IN RESOURCE 'name' { ... } (ako/mxcli#2449)
		if ac.ADD() != nil && ac.OPERATION() != nil {
			act := &ast.PublishedRestAddOperationsAction{Resource: unquoteStringLit(ac.STRING_LITERAL())}
			for _, opCtx := range ac.AllPublishedRestOperation() {
				act.Operations = append(act.Operations, buildPublishedRestOperationDef(opCtx.(*parser.PublishedRestOperationContext)))
			}
			stmt.Actions = append(stmt.Actions, act)
			continue
		}

		// DROP OPERATION <method> ['path'] IN RESOURCE 'name'
		if ac.DROP() != nil && ac.OPERATION() != nil {
			act := &ast.PublishedRestDropOperationAction{Resource: unquoteStringLit(ac.STRING_LITERAL())}
			if m := ac.RestHttpMethod(); m != nil {
				act.HTTPMethod = strings.ToUpper(m.GetText())
			}
			if pCtx := ac.PublishedRestOpPath(); pCtx != nil {
				if pc := pCtx.(*parser.PublishedRestOpPathContext); pc.STRING_LITERAL() != nil {
					act.Path = strings.Trim(unquoteStringLit(pc.STRING_LITERAL()), "/")
				}
			}
			stmt.Actions = append(stmt.Actions, act)
			continue
		}

		// ADD RESOURCE 'name' { ... }
		if ac.ADD() != nil {
			if rc := ac.PublishedRestResource(); rc != nil {
				if resDef := buildPublishedRestResourceDef(rc.(*parser.PublishedRestResourceContext)); resDef != nil {
					stmt.Actions = append(stmt.Actions, &ast.PublishedRestAddResourceAction{Resource: resDef})
				}
			}
			continue
		}

		// DROP RESOURCE 'name'
		if ac.DROP() != nil && ac.RESOURCE() != nil {
			name := unquoteStringLit(ac.STRING_LITERAL())
			stmt.Actions = append(stmt.Actions, &ast.PublishedRestDropResourceAction{Name: name})
			continue
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitRestClientHeaderItem records the expression form of a header value,
// `'prefix' + $P` or `$P` (MDL-DEPR711).
func (b *Builder) ExitRestClientHeaderItem(ctx *parser.RestClientHeaderItemContext) {
	v := ctx.VARIABLE()
	if v == nil {
		return
	}
	prefix := ""
	if sl := ctx.AllSTRING_LITERAL(); len(sl) >= 2 {
		prefix = unquoteStringLit(sl[1])
	}
	b.recordDeprecation(deprecation.RestHeaderConcat, v.GetSymbol(), "")
	fix, why := restHeaderConcatFix(ctx, prefix)
	b.fixLastDeprecation(deprecation.RestHeaderConcat, fix, why)
}

// restHeaderConcatFix rewrites a header value written `'prefix' + $P` or `$P`
// as the template `'prefix{P}'` (MDL-DEPR711). It returns the fix, or the
// reason there is none: a prefix holding a brace, which the template would read
// as a placeholder, or a backslash escape whose meaning depends on the version.
func restHeaderConcatFix(hic *parser.RestClientHeaderItemContext, prefix string) (*ast.Fix, string) {
	if strings.ContainsAny(prefix, "{}") {
		return nil, "the text before `+` holds a brace, which a header template reads as a placeholder; write the value by hand"
	}
	allSL := hic.AllSTRING_LITERAL()
	v := hic.VARIABLE().GetSymbol()
	name := strings.TrimPrefix(v.GetText(), "$")
	if len(allSL) < 2 {
		// `'X-Key' = $Key` -> `'X-Key' = '{Key}'`
		return &ast.Fix{Edits: []ast.TextEdit{replaceSpan(v, v, "'{"+name+"}'")}}, ""
	}
	lit := allSL[1]
	if holdsInterpretedEscape(lit) {
		return nil, "the text before `+` holds a backslash escape; write the value by hand"
	}
	// `'Bearer ' + $Token` -> `'Bearer {Token}'`: the literal keeps its text
	// and its escapes, and the placeholder goes before its closing quote.
	raw := lit.GetText()
	return &ast.Fix{Edits: []ast.TextEdit{replaceSpan(lit.GetSymbol(), v, raw[:len(raw)-1]+"{"+name+"}'")}}, ""
}
