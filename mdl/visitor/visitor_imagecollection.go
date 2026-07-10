// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitCreateImageCollectionStatement is called when exiting the createImageCollectionStatement production.
func (b *Builder) ExitCreateImageCollectionStatement(ctx *parser.CreateImageCollectionStatementContext) {
	stmt := &ast.CreateImageCollectionStmt{
		Name:        buildQualifiedName(ctx.QualifiedName()),
		ExportLevel: "Hidden",
	}

	// Extract /** ... */ doc comment (same as other create statements)
	stmt.Comment = findDocCommentText(ctx)

	if opts := ctx.ImageCollectionOptions(); opts != nil {
		optsCtx := opts.(*parser.ImageCollectionOptionsContext)
		for _, opt := range optsCtx.AllImageCollectionOption() {
			optCtx := opt.(*parser.ImageCollectionOptionContext)
			if optCtx.EXPORT() != nil && optCtx.LEVEL() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.ExportLevel = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
			if optCtx.COMMENT() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Comment = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
		}
	}

	if body := ctx.ImageCollectionBody(); body != nil {
		bodyCtx := body.(*parser.ImageCollectionBodyContext)
		for _, item := range bodyCtx.AllImageCollectionItem() {
			itemCtx := item.(*parser.ImageCollectionItemContext)
			name := itemCtx.ImageName().GetText()
			// Strip quotes from quoted identifiers ("Name" or `Name`)
			if len(name) >= 2 && (name[0] == '"' || name[0] == '`') {
				name = name[1 : len(name)-1]
			}
			stmt.Images = append(stmt.Images, ast.ImageItem{
				Name:     name,
				FilePath: unquoteString(itemCtx.GetPath().GetText()),
			})
		}
	}

	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil && createStmt.OR() != nil && (createStmt.REPLACE() != nil || createStmt.MODIFY() != nil) {
		stmt.CreateOrModify = true
	}

	b.statements = append(b.statements, stmt)
}
