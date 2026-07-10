// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitCreateBusinessEventServiceStatement handles CREATE BUSINESS EVENT SERVICE statements.
func (b *Builder) ExitCreateBusinessEventServiceStatement(ctx *parser.CreateBusinessEventServiceStatementContext) {
	stmt := &ast.CreateBusinessEventServiceStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Parse property assignments (ServiceName, EventNamePrefix, Folder)
	for _, propCtx := range ctx.AllOdataPropertyAssignment() {
		prop := propCtx.(*parser.OdataPropertyAssignmentContext)
		name := identifierOrKeywordText(prop.IdentifierOrKeyword())
		value := odataAssignmentValueText(prop)

		switch strings.ToLower(name) {
		case "servicename":
			stmt.ServiceName = value
		case "eventnameprefix":
			stmt.EventNamePrefix = value
		case "folder":
			stmt.Folder = value
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

	// Check for CREATE OR MODIFY (or OR REPLACE, treated identically) and doc comment
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.REPLACE() != nil || createStmt.MODIFY() != nil) {
			stmt.CreateOrModify = true
		}
	}
	stmt.Documentation = findDocCommentText(ctx)

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
		"binary": "Binary",
	}
	if c, ok := canonical[strings.ToLower(text)]; ok {
		return c
	}
	return text
}
