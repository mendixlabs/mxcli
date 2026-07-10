// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitCreateAssociationStatement(ctx *parser.CreateAssociationStatementContext) {
	names := ctx.AllQualifiedName()
	if len(names) < 3 {
		return
	}

	stmt := &ast.CreateAssociationStmt{
		Name:           buildQualifiedName(names[0]),
		Parent:         buildQualifiedName(names[1]),
		Child:          buildQualifiedName(names[2]),
		Type:           ast.AssocReference, // Default
		Owner:          ast.OwnerDefault,
		DeleteBehavior: ast.DeleteKeepReferences,
	}

	// Association options
	if opts := ctx.AssociationOptions(); opts != nil {
		optsCtx := opts.(*parser.AssociationOptionsContext)
		for _, opt := range optsCtx.AllAssociationOption() {
			optCtx := opt.(*parser.AssociationOptionContext)

			// TYPE
			if optCtx.TYPE() != nil {
				if optCtx.REFERENCE_SET() != nil {
					stmt.Type = ast.AssocReferenceSet
				}
			}

			// OWNER (grammar supports DEFAULT and BOTH)
			if optCtx.OWNER() != nil {
				if optCtx.BOTH() != nil {
					stmt.Owner = ast.OwnerBoth
				} else if optCtx.DEFAULT() != nil {
					stmt.Owner = ast.OwnerDefault
				}
			}

			// STORAGE
			if optCtx.STORAGE() != nil {
				if optCtx.COLUMN() != nil {
					stmt.Storage = ast.StorageColumn
				} else if optCtx.TABLE() != nil {
					stmt.Storage = ast.StorageTable
				}
			}

			// DELETE_BEHAVIOR
			if delBehavior := optCtx.DeleteBehavior(); delBehavior != nil {
				stmt.DeleteBehavior = buildDeleteBehavior(delBehavior)
			}

			// COMMENT
			if optCtx.COMMENT() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Comment = unquoteString(optCtx.STRING_LITERAL().GetText())
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

// ExitAlterAssociationAction handles ALTER ASSOCIATION ... SET ... actions.
func (b *Builder) ExitAlterAssociationAction(ctx *parser.AlterAssociationActionContext) {
	// Walk up to the parent AlterStatement to get the association's qualified name
	parent := ctx.GetParent()
	for parent != nil {
		if alterStmt, ok := parent.(*parser.AlterStatementContext); ok {
			if alterStmt.ASSOCIATION() == nil {
				return
			}
			qn := alterStmt.QualifiedName()
			if qn == nil {
				return
			}
			name := buildQualifiedName(qn)

			// SET DELETE_BEHAVIOR
			if ctx.DELETE_BEHAVIOR() != nil {
				if delBehavior := ctx.DeleteBehavior(); delBehavior != nil {
					b.statements = append(b.statements, &ast.AlterAssociationStmt{
						Name:           name,
						Operation:      ast.AlterAssociationSetDeleteBehavior,
						DeleteBehavior: buildDeleteBehavior(delBehavior),
					})
				}
				return
			}

			// SET OWNER
			if ctx.OWNER() != nil {
				owner := ast.OwnerDefault
				if ctx.BOTH() != nil {
					owner = ast.OwnerBoth
				}
				b.statements = append(b.statements, &ast.AlterAssociationStmt{
					Name:      name,
					Operation: ast.AlterAssociationSetOwner,
					Owner:     owner,
				})
				return
			}

			// SET STORAGE
			if ctx.STORAGE() != nil {
				storage := ast.StorageTable
				if ctx.COLUMN() != nil {
					storage = ast.StorageColumn
				}
				b.statements = append(b.statements, &ast.AlterAssociationStmt{
					Name:      name,
					Operation: ast.AlterAssociationSetStorage,
					Storage:   storage,
				})
				return
			}

			// SET COMMENT
			if ctx.COMMENT() != nil && ctx.STRING_LITERAL() != nil {
				b.statements = append(b.statements, &ast.AlterAssociationStmt{
					Name:      name,
					Operation: ast.AlterAssociationSetComment,
					Comment:   unquoteString(ctx.STRING_LITERAL().GetText()),
				})
				return
			}

			return
		}
		parent = parent.GetParent()
	}
}

// ----------------------------------------------------------------------------
// Query Statements (SHOW/DESCRIBE)
// ----------------------------------------------------------------------------

// ExitShowStatement handles SHOW MODULES/ENTITIES/ASSOCIATIONS/etc.
