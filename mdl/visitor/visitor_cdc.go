// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// cdcServiceSchema is the property set the statement accepts. AppName is
// absent on purpose: it is the topic's second segment and Studio Pro derives it
// from the project, so letting MDL set it would let a script rename every topic
// the service publishes to.
var cdcServiceSchema = propSchema{on: "a change data capture service", keys: []propKey{
	{"ServiceName", []propShape{shapeString}},
	{"Folder", []propShape{shapeString}},
	// Known so that writing it gets the explanation below rather than a bare
	// "unknown property": an author who tries it has a reason, and the reason
	// is wrong in an interesting way.
	{"AppName", []propShape{shapeString}},
}}

// ExitCreateChangeDataCaptureServiceStatement handles
// CREATE [OR MODIFY] CHANGE DATA CAPTURE SERVICE.
func (b *Builder) ExitCreateChangeDataCaptureServiceStatement(ctx *parser.CreateChangeDataCaptureServiceStatementContext) {
	stmt := &ast.CreateChangeDataCaptureServiceStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	for _, propCtx := range ctx.AllOdataPropertyAssignment() {
		prop := propCtx.(*parser.OdataPropertyAssignmentContext)
		name := identifierOrKeywordText(prop.IdentifierOrKeyword())
		value := odataAssignmentValueText(prop)
		b.checkProperty(prop, &cdcServiceSchema, name, odataPropertyShape(prop))

		switch strings.ToLower(name) {
		case "servicename":
			stmt.ServiceName = value
			stmt.ServiceNameSet = true
		case "folder":
			stmt.Folder = value
		case "appname":
			// Refused rather than ignored: silently dropping it would leave the
			// author believing they had set the topic.
			b.addError(fmt.Errorf("AppName is not settable from MDL: it is the second segment of every topic the service publishes to (cdc.<AppName>.<Entity>.<Major>.<space>) and Studio Pro derives it from the project, so changing it here would re-point every topic. Remove it; the stored value is preserved on modify"))
		}
	}

	for _, srcCtx := range ctx.AllCdcSourceDef() {
		src, ok := srcCtx.(*parser.CdcSourceDefContext)
		if !ok || src == nil {
			continue
		}
		def := &ast.CdcSourceDef{Entity: buildQualifiedName(src.QualifiedName())}
		if lit := src.STRING_LITERAL(); lit != nil {
			def.ExposedName = unquoteStringLit(lit)
		}
		for _, mCtx := range src.AllCdcMemberDef() {
			m, ok := mCtx.(*parser.CdcMemberDefContext)
			if !ok || m == nil {
				continue
			}
			mem := &ast.CdcMemberDef{Name: identifierOrKeywordText(m.IdentifierOrKeyword())}
			if lit := m.STRING_LITERAL(); lit != nil {
				mem.ExposedName = unquoteStringLit(lit)
			}
			// `id` is implicit: Studio Pro's editor lists it as a checked,
			// disabled row and stores nothing for it, and its own editor says
			// "id is a reserved attribute and cannot be excluded". Writing it
			// would break CLAUDE.md's rule 4 (Studio Pro's exact property set).
			if strings.EqualFold(mem.Name, "id") {
				b.addError(fmt.Errorf("`id` is implicit on a change data capture source and must not be listed: Studio Pro shows it as a checked, disabled row but stores no property for it, and the object id reaches a consumer from the runtime's change envelope rather than from the model"))
				continue
			}
			def.Members = append(def.Members, mem)
		}
		if len(def.Members) == 0 {
			b.addError(fmt.Errorf("entity %s tracks no members: a change data capture source needs at least one attribute or association", def.Entity.String()))
		}
		stmt.Sources = append(stmt.Sources, def)
	}

	// The same exposed name twice would publish two sources to one topic.
	seen := map[string]string{}
	for _, src := range stmt.Sources {
		exposed := src.ExposedName
		if exposed == "" {
			exposed = src.Entity.Name
		}
		if prev, dup := seen[strings.ToLower(exposed)]; dup {
			b.addError(fmt.Errorf("two sources share the exposed name %q (%s and %s): the exposed name is the topic's third segment, so both would publish to cdc.<App>.%s.<Major>.<space>",
				exposed, prev, src.Entity.String(), exposed))
		}
		seen[strings.ToLower(exposed)] = src.Entity.String()
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.REPLACE() != nil || createStmt.MODIFY() != nil) {
			stmt.CreateOrModify = true
		}
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	b.statements = append(b.statements, stmt)
}
