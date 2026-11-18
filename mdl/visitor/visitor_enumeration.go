// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitCreateEnumerationStatement(ctx *parser.CreateEnumerationStatementContext) {
	stmt := &ast.CreateEnumerationStmt{
		Name:   buildQualifiedName(ctx.QualifiedName()),
		Values: buildEnumValues(ctx.EnumerationValueList(), b),
	}

	// Handle options (COMMENT, etc.)
	if opts := ctx.EnumerationOptions(); opts != nil {
		optsCtx := opts.(*parser.EnumerationOptionsContext)
		for _, opt := range optsCtx.AllEnumerationOption() {
			optCtx := opt.(*parser.EnumerationOptionContext)
			if optCtx.FOLDER() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Folder = unquoteStringLit(optCtx.STRING_LITERAL())
			}
			if el := optCtx.MicroflowExportLevelClause(); el != nil {
				stmt.ExportLevel = exportLevelOf(el.(*parser.MicroflowExportLevelClauseContext))
			}
		}
	}

	// Check for CREATE OR MODIFY / CREATE OR REPLACE
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	b.statements = append(b.statements, stmt)
}

// ExitAlterEnumerationAction handles ALTER ENUMERATION ... ADD/DROP/RENAME VALUE
func (b *Builder) ExitAlterEnumerationAction(ctx *parser.AlterEnumerationActionContext) {
	// Get the parent ALTER statement to find the enumeration name
	parent := ctx.GetParent()
	for parent != nil {
		if alterStmt, ok := parent.(*parser.AlterStatementContext); ok {
			if alterStmt.ENUMERATION() != nil {
				qn := alterStmt.QualifiedName()
				if qn == nil {
					return
				}

				name := buildQualifiedName(qn)
				var ids []string
				for _, v := range ctx.AllEnumValueName() {
					ids = append(ids, unquoteIdentifier(v.GetText()))
				}

				if ctx.ADD() != nil && len(ids) >= 1 {
					// No caption written: the value's name, as for `create
					// enumeration` (ako/mxcli#2397).
					caption := ids[0]
					if ctx.STRING_LITERAL() != nil {
						caption = unquoteStringLit(ctx.STRING_LITERAL())
					}
					b.statements = append(b.statements, &ast.AlterEnumerationStmt{
						Name:        name,
						Operation:   ast.AlterEnumAdd,
						ValueName:   ids[0],
						Caption:     caption,
						IfNotExists: ctx.IfNotExists() != nil,
					})
				} else if ctx.DROP() != nil && ctx.VALUE() != nil && len(ids) >= 1 {
					b.statements = append(b.statements, &ast.AlterEnumerationStmt{
						Name:      name,
						Operation: ast.AlterEnumDrop,
						ValueName: ids[0],
						IfExists:  ctx.IfExists() != nil,
					})
				} else if ctx.RENAME() != nil && ctx.VALUE() != nil && len(ids) >= 2 {
					b.statements = append(b.statements, &ast.AlterEnumerationStmt{
						Name:      name,
						Operation: ast.AlterEnumRename,
						ValueName: ids[0],
						NewName:   ids[1],
					})
				} else if el := ctx.MicroflowExportLevelClause(); ctx.SET() != nil && el != nil {
					b.statements = append(b.statements, &ast.AlterEnumerationStmt{
						Name:        name,
						Operation:   ast.AlterEnumSetExportLevel,
						ExportLevel: *exportLevelOf(el.(*parser.MicroflowExportLevelClauseContext)),
					})
				} else if ctx.SET() != nil && ctx.STRING_LITERAL() != nil && (ctx.DOCUMENTATION() != nil || ctx.COMMENT() != nil) {
					// SET DOCUMENTATION; SET COMMENT is its deprecated spelling
					// (R9, MDL-DEPR135). Neither built a statement before.
					if ctx.COMMENT() != nil {
						b.recordDeprecation(deprecation.SetComment, ctx.COMMENT().GetSymbol(), "")
					}
					b.statements = append(b.statements, &ast.AlterEnumerationStmt{
						Name:          name,
						Operation:     ast.AlterEnumSetDocumentation,
						Documentation: unquoteStringLit(ctx.STRING_LITERAL()),
					})
				} else if ctx.MODIFY() != nil && ctx.VALUE() != nil && ctx.CAPTION() != nil && len(ids) >= 1 && ctx.STRING_LITERAL() != nil {
					b.statements = append(b.statements, &ast.AlterEnumerationStmt{
						Name:      name,
						Operation: ast.AlterEnumModifyCaption,
						ValueName: ids[0],
						Caption:   unquoteStringLit(ctx.STRING_LITERAL()),
					})
				}
			}
			break
		}
		parent = parent.GetParent()
	}
}

// ----------------------------------------------------------------------------
// Constant Statements
// ----------------------------------------------------------------------------

// ExitCreateConstantStatement is called when exiting the createConstantStatement production.
func (b *Builder) ExitCreateConstantStatement(ctx *parser.CreateConstantStatementContext) {
	stmt := &ast.CreateConstantStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	if pl, ok := ctx.ConstantPropertyList().(*parser.ConstantPropertyListContext); ok && pl != nil {
		b.constantProperties(stmt, pl)
	} else if ctx.DataType() != nil {
		// The clause form (MDL-DEPR136).
		stmt.DataType = buildDataType(ctx.DataType())
		if lit := ctx.Literal(); lit != nil {
			stmt.DefaultValue = extractLiteralValue(lit)
		}
		b.recordDeprecation(deprecation.ConstantClauses, ctx.TYPE().GetSymbol(), "")
		b.fixLastDeprecation(deprecation.ConstantClauses, constantClausesFix(ctx), "")
	}

	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	// `folder '…'` after the name (R9); a trailing `folder` option is its old
	// position and, being later, wins when both are written.
	if lit := ctx.STRING_LITERAL(); lit != nil && ctx.FOLDER() != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	// Handle options (COMMENT, FOLDER, EXPOSED TO CLIENT)
	if opts := ctx.ConstantOptions(); opts != nil {
		optsCtx := opts.(*parser.ConstantOptionsContext)
		for _, opt := range optsCtx.AllConstantOption() {
			optCtx := opt.(*parser.ConstantOptionContext)
			if c := optCtx.COMMENT(); c != nil && optCtx.STRING_LITERAL() != nil {
				// R9: `comment '…'` is the documentation, which a doc
				// comment also states; the clause wins, as it always has.
				text := unquoteStringLit(optCtx.STRING_LITERAL())
				b.recordDocumentationClause(ctx, c.GetSymbol(), optCtx.STRING_LITERAL().GetSymbol(), text, true)
				stmt.Documentation, stmt.DocumentationSet = text, true
			}
			if optCtx.FOLDER() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Folder = unquoteStringLit(optCtx.STRING_LITERAL())
				b.recordFolderClausePosition(ctx.QualifiedName(), optCtx.FOLDER().GetSymbol(),
					optCtx.STRING_LITERAL().GetSymbol(), ctx.FOLDER() != nil || countFolderOptions(optsCtx) > 1)
			} else if id := optCtx.IDENTIFIER(); id != nil {
				// `private` (MDL-DEPR138, ako/mxcli#865): never stored, so it
				// builds nothing; fmt --upgrade deletes it with the blanks before it.
				// One divergence from v0.24.0, where the word began a help
				// statement that swallowed the words after it: `private exposed
				// to client` now exposes the constant, as it plainly says. No
				// script in the repo's corpus or mxcli-formula1 writes that order.
				tok := id.GetSymbol()
				b.recordDeprecation(deprecation.ConstantPrivate, tok, stmt.Name.String())
				b.fixLastDeprecation(deprecation.ConstantPrivate, &ast.Fix{Edits: []ast.TextEdit{
					{Start: startAfterSpace(tok), Stop: tok.GetStop() + 1},
				}}, "")
			} else if optCtx.EXPOSED() != nil {
				if ctx.ConstantPropertyList() != nil {
					// Not a legacy spelling: the property list is new, and
					// says this as ExposedToClient: true.
					b.addError(fmt.Errorf("line %d: constant %s: `exposed to client` is the property ExposedToClient: true in the list",
						optCtx.GetStart().GetLine(), stmt.Name))
				}
				stmt.ExposedToClient = true
			}
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

// ----------------------------------------------------------------------------
// Entity Statements
// ----------------------------------------------------------------------------

// ExitCreateEntityStatement is called when exiting the createEntityStatement production.
