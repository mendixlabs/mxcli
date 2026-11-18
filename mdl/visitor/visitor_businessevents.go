// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateBusinessEventServiceStatement handles CREATE BUSINESS EVENT SERVICE statements.
func (b *Builder) ExitCreateBusinessEventServiceStatement(ctx *parser.CreateBusinessEventServiceStatementContext) {
	stmt := &ast.CreateBusinessEventServiceStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// R11: a statement is either the published shape (ServiceName /
	// EventNamePrefix / `message`) or the consumed one (Contract* / `consume`),
	// never both. Tracked while reading so the error can name what collided.
	var contractKeys []string
	sawPublisherKey := false

	// Parse property assignments (ServiceName, EventNamePrefix, Folder, Contract*)
	for _, propCtx := range ctx.AllOdataPropertyAssignment() {
		prop := propCtx.(*parser.OdataPropertyAssignmentContext)
		name := identifierOrKeywordText(prop.IdentifierOrKeyword())
		value := odataAssignmentValueText(prop)
		b.checkProperty(prop, &businessEventServiceSchema, name, odataPropertyShape(prop))

		switch strings.ToLower(name) {
		case "servicename":
			stmt.ServiceName = value
			sawPublisherKey = true
		case "eventnameprefix":
			stmt.EventNamePrefix = value
			sawPublisherKey = true
		case "folder":
			stmt.Folder = value
		case "contract":
			stmt.Contract = value
			stmt.ContractSet = true
			contractKeys = append(contractKeys, name)
		case "contractfile":
			stmt.ContractFile = value
			stmt.ContractSet = true
			contractKeys = append(contractKeys, name)
		}
	}

	// Parse message definitions
	for _, msgCtx := range ctx.AllBusinessEventMessageDef() {
		msgDef := msgCtx.(*parser.BusinessEventMessageDefContext)
		if msgDef == nil {
			continue
		}

		msg := &ast.BusinessEventMessageDef{}
		if iok := msgDef.IdentifierOrKeyword(); iok != nil {
			msg.MessageName = identifierOrKeywordText(iok)
		}

		// Parse operation (PUBLISH or SUBSCRIBE)
		if msgDef.PUBLISH() != nil {
			msg.Operation = "PUBLISH"
		} else if msgDef.SUBSCRIBE() != nil {
			msg.Operation = "SUBSCRIBE"
		}

		// Parse ENTITY clause
		qualNames := msgDef.AllQualifiedName()
		idx := 0
		if msgDef.ENTITY() != nil && idx < len(qualNames) {
			msg.Entity = getQualifiedNameText(qualNames[idx])
			idx++
		}
		// Parse MICROFLOW clause
		if msgDef.MICROFLOW() != nil && idx < len(qualNames) {
			msg.Microflow = getQualifiedNameText(qualNames[idx])
		}

		// Parse attribute definitions
		for _, attrCtx := range msgDef.AllBusinessEventAttrDef() {
			attrDef := attrCtx.(*parser.BusinessEventAttrDefContext)
			if attrDef == nil {
				continue
			}
			attr := &ast.BusinessEventAttributeDef{
				TypeName: dataTypeSimpleName(attrDef.DataType()),
			}
			if iok := attrDef.IdentifierOrKeyword(); iok != nil {
				attr.Name = identifierOrKeywordText(iok)
			}
			msg.Attributes = append(msg.Attributes, attr)
		}

		stmt.Messages = append(stmt.Messages, msg)
	}

	// Parse `consume <Message> entity … microflow …` body items.
	for _, consCtx := range ctx.AllBusinessEventConsumeDef() {
		consDef, ok := consCtx.(*parser.BusinessEventConsumeDefContext)
		if !ok || consDef == nil {
			continue
		}
		cons := &ast.BusinessEventConsumeDef{}
		if iok := consDef.IdentifierOrKeyword(); iok != nil {
			cons.MessageName = identifierOrKeywordText(iok)
		}
		qualNames := consDef.AllQualifiedName()
		idx := 0
		if consDef.ENTITY() != nil && idx < len(qualNames) {
			cons.Entity = getQualifiedNameText(qualNames[idx])
			idx++
		}
		if consDef.MICROFLOW() != nil && idx < len(qualNames) {
			cons.Microflow = getQualifiedNameText(qualNames[idx])
		}
		stmt.Consumes = append(stmt.Consumes, cons)
	}

	// R11, reported once with the specific collision rather than a generic
	// "invalid statement": the two shapes write different documents, and
	// guessing which half the author meant is how a contract gets dropped.
	switch {
	case len(contractKeys) > 1:
		b.addError(fmt.Errorf("a business event service takes one of Contract or ContractFile, not both (%s)",
			strings.Join(contractKeys, ", ")))
	case len(contractKeys) > 0 && sawPublisherKey:
		b.addError(fmt.Errorf("%s describes a consumed business event service, so it cannot also set ServiceName or EventNamePrefix — those belong to a published one", contractKeys[0]))
	case len(contractKeys) > 0 && len(stmt.Messages) > 0:
		b.addError(fmt.Errorf("%s describes a consumed business event service, whose events come from the contract: use `consume <Message> entity … microflow …`, not `message …`", contractKeys[0]))
	case len(contractKeys) == 0 && len(stmt.Consumes) > 0:
		b.addError(fmt.Errorf("`consume` needs a contract to take the event from: add Contract: $$…$$ or ContractFile: '…' to the property list"))
	}

	// Check for CREATE OR MODIFY (or OR REPLACE, treated identically) and doc comment
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.REPLACE() != nil || createStmt.MODIFY() != nil) {
			stmt.CreateOrModify = true
		}
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	b.statements = append(b.statements, stmt)
}

// dataTypeSimpleName extracts a simple type name from a DataType context.
// For business event attributes, types are simple names like "Long", "String", "Integer".
func dataTypeSimpleName(ctx parser.IDataTypeContext) string {
	if ctx == nil {
		return "String"
	}
	text := ctx.GetText()
	// Strip trailing length specifications like "String(200)" → "String"
	if idx := strings.Index(text, "("); idx >= 0 {
		text = text[:idx]
	}
	// Normalize to Mendix canonical casing (grammar is case-insensitive, Mendix BSON is not).
	canonical := map[string]string{
		"string": "String", "integer": "Integer", "long": "Long",
		"decimal": "Decimal", "boolean": "Boolean", "datetime": "DateTime",
		"date":   "DateTime", // MDL-DEPR160: Mendix has no date-only type
		"binary": "Binary",
	}
	if c, ok := canonical[strings.ToLower(text)]; ok {
		return c
	}
	return text
}
