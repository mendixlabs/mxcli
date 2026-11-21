// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
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

	// The old keyword-clause spellings, recorded once per code for the
	// statement with every rewrite (MDL-DEPR075, MDL-DEPR076).
	depr := &bePending{}
	defer depr.flush(b)

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

		b.businessEventMessageOperations(msgDef, msg, depr)

		// Parse attribute definitions
		for _, attrCtx := range msgDef.AllBusinessEventAttrDef() {
			attrDef := attrCtx.(*parser.BusinessEventAttrDefContext)
			if attrDef == nil {
				continue
			}
			attr := &ast.BusinessEventAttributeDef{
				TypeName: dataTypeSimpleName(attrDef.DataType()),
			}
			// A parenthesised String length is stated — a number, or
			// `unlimited` (0) — and is stored; a bare String stores none
			// (ako/mxcli#2490).
			if dt, ok := attrDef.DataType().(*parser.DataTypeContext); ok && dt.STRING_TYPE() != nil && dt.LPAREN() != nil {
				attr.Length, attr.LengthSet = buildDataType(dt).Length, true
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
		if consDef.LPAREN() != nil {
			cons.Entity, cons.Microflow = b.businessEventOpProps("consume "+cons.MessageName, consDef.AllBusinessEventOpProp(), true)
		} else {
			qualNames := consDef.AllQualifiedName()
			idx := 0
			if consDef.ENTITY() != nil && idx < len(qualNames) {
				cons.Entity = getQualifiedNameText(qualNames[idx])
				idx++
			}
			if consDef.MICROFLOW() != nil && idx < len(qualNames) {
				cons.Microflow = getQualifiedNameText(qualNames[idx])
			}
			// The keyword clauses are the old spelling (MDL-DEPR076).
			if first := consumeClauseStart(consDef); first != nil {
				depr.add(deprecation.BusinessEventConsumeClauses, first,
					replaceSpan(first, consDef.SEMICOLON().GetSymbol(), opPropsText("", cons.Entity, cons.Microflow)+";"))
			}
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

// bePending collects one statement's uses of the old keyword-clause spellings,
// so each code is one warning carrying every rewrite.
type bePending struct {
	order []string
	first map[string]antlr.Token
	edits map[string][]ast.TextEdit
}

func (p *bePending) add(code string, at antlr.Token, edit ast.TextEdit) {
	if p.first == nil {
		p.first, p.edits = map[string]antlr.Token{}, map[string][]ast.TextEdit{}
	}
	if _, ok := p.first[code]; !ok {
		p.first[code] = at
		p.order = append(p.order, code)
	}
	p.edits[code] = append(p.edits[code], edit)
}

func (p *bePending) flush(b *Builder) {
	for _, code := range p.order {
		b.recordDeprecation(code, p.first[code], "")
		b.fixLastDeprecation(code, &ast.Fix{Edits: p.edits[code]}, "")
	}
}

// businessEventMessageOperations reads a message's operations into msg.
//
// The canonical form is R2's: the operations are the message's children,
// `{ publish ( Entity: … ); subscribe ( Entity: …, Microflow: … ); }`, and one
// message may have both (ako/mxcli#2490). One operation fills Operation /
// Entity / Microflow; a publish and a subscribe fill PublishAndSubscribe, with
// the publish entity in Entity and the subscription's in SubscribeEntity and
// Microflow. The keyword clauses `publish entity … microflow …` are the old
// spelling (MDL-DEPR075), for one operation.
func (b *Builder) businessEventMessageOperations(def *parser.BusinessEventMessageDefContext, msg *ast.BusinessEventMessageDef, depr *bePending) {
	if def.LBRACE() == nil {
		opTok := def.PUBLISH()
		if opTok == nil {
			opTok = def.SUBSCRIBE()
		}
		if opTok == nil {
			return
		}
		msg.Operation = strings.ToUpper(opTok.GetText())
		qualNames := def.AllQualifiedName()
		idx := 0
		if def.ENTITY() != nil && idx < len(qualNames) {
			msg.Entity = getQualifiedNameText(qualNames[idx])
			idx++
		}
		if def.MICROFLOW() != nil && idx < len(qualNames) {
			msg.Microflow = getQualifiedNameText(qualNames[idx])
		}
		last := opTok.GetSymbol()
		if n := len(qualNames); n > 0 {
			last = qualNames[n-1].GetStop()
		}
		word := keywordLike(opTok.GetText(), opTok.GetText())
		depr.add(deprecation.BusinessEventMessageClauses, opTok.GetSymbol(),
			replaceSpan(opTok.GetSymbol(), last, "{ "+opPropsText(word, msg.Entity, msg.Microflow)+"; }"))
		return
	}
	type op struct{ entity, microflow string }
	var pub, sub *op
	for _, oc := range def.AllBusinessEventOperationDef() {
		o, ok := oc.(*parser.BusinessEventOperationDefContext)
		if !ok {
			continue
		}
		isPub := o.PUBLISH() != nil
		kind := "subscribe"
		if isPub {
			kind = "publish"
		}
		e, m := b.businessEventOpProps(kind+" of message "+msg.MessageName, o.AllBusinessEventOpProp(), true)
		switch {
		case isPub && pub != nil, !isPub && sub != nil:
			b.addError(fmt.Errorf("message %s has two %s operations; a message has at most one publish and one subscribe", msg.MessageName, kind))
		case isPub:
			pub = &op{e, m}
		default:
			sub = &op{e, m}
		}
	}
	switch {
	case pub != nil && sub != nil:
		if pub.microflow != "" {
			b.addError(fmt.Errorf("message %s: a publish operation has no handler; `Microflow` belongs to the subscribe operation", msg.MessageName))
		}
		msg.Operation, msg.PublishAndSubscribe = "PUBLISH", true
		msg.Entity, msg.SubscribeEntity, msg.Microflow = pub.entity, sub.entity, sub.microflow
	case pub != nil:
		msg.Operation, msg.Entity, msg.Microflow = "PUBLISH", pub.entity, pub.microflow
	case sub != nil:
		msg.Operation, msg.Entity, msg.Microflow = "SUBSCRIBE", sub.entity, sub.microflow
	default:
		b.addError(fmt.Errorf("message %s has no operation: write `{ publish ( Entity: … ); }`, `{ subscribe ( Entity: …, Microflow: … ); }` or both", msg.MessageName))
	}
}

// businessEventOpProps reads `( Entity: M.E, Microflow: M.H )`. Keys are
// matched without case; any other key is an error (R11).
func (b *Builder) businessEventOpProps(where string, props []parser.IBusinessEventOpPropContext, microflowAllowed bool) (entity, microflow string) {
	for _, pc := range props {
		p, ok := pc.(*parser.BusinessEventOpPropContext)
		if !ok || p.IdentifierOrKeyword() == nil || p.QualifiedName() == nil {
			continue
		}
		key := identifierOrKeywordText(p.IdentifierOrKeyword())
		val := getQualifiedNameText(p.QualifiedName())
		switch {
		case strings.EqualFold(key, "Entity"):
			entity = val
		case strings.EqualFold(key, "Microflow") && microflowAllowed:
			microflow = val
		default:
			b.addError(fmt.Errorf("%s: unknown property %q; the properties are Entity and Microflow", where, key))
		}
	}
	return entity, microflow
}

// opPropsText is `word ( Entity: e, Microflow: m )` with the empty ones left
// out; word may be "" for a consume's own property list.
func opPropsText(word, entity, microflow string) string {
	var props []string
	if entity != "" {
		props = append(props, "Entity: "+entity)
	}
	if microflow != "" {
		props = append(props, "Microflow: "+microflow)
	}
	list := "(" + strings.Join(props, ", ") + ")"
	if word == "" {
		return list
	}
	return word + " " + list
}

// consumeClauseStart is the first token of a consume's old keyword clauses,
// nil when it has none.
func consumeClauseStart(def *parser.BusinessEventConsumeDefContext) antlr.Token {
	if t := def.ENTITY(); t != nil {
		return t.GetSymbol()
	}
	if t := def.MICROFLOW(); t != nil {
		return t.GetSymbol()
	}
	return nil
}
