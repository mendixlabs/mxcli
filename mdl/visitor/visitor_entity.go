// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitCreateEntityStatement(ctx *parser.CreateEntityStatementContext) {
	// Handle VIEW entities separately
	if ctx.VIEW() != nil {
		b.buildViewEntity(ctx)
		return
	}

	stmt := &ast.CreateEntityStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
		Kind: ast.EntityPersistent, // Default
	}

	// Entity type
	if ctx.NON_PERSISTENT() != nil {
		stmt.Kind = ast.EntityNonPersistent
	}

	// Navigate to parent CreateStatement to get annotations and doc comment
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		// Check for CREATE OR MODIFY
		if createStmt.OR() != nil && createStmt.MODIFY() != nil {
			stmt.CreateOrModify = true
		}

		// Extract annotations (@Position, etc.)
		for _, annCtx := range createStmt.AllAnnotation() {
			ann := annCtx.(*parser.AnnotationContext)
			annName := ann.AnnotationName().GetText()
			if strings.EqualFold(annName, "Position") {
				// @Position(x, y)
				if params := ann.AnnotationParams(); params != nil {
					paramsCtx := params.(*parser.AnnotationParamsContext)
					allParams := paramsCtx.AllAnnotationParam()
					if len(allParams) >= 2 {
						x := parseAnnotationParamInt(allParams[0])
						y := parseAnnotationParamInt(allParams[1])
						stmt.Position = &ast.Position{X: x, Y: y}
					}
				}
			}
		}

	}
	stmt.Documentation = findDocCommentText(ctx)

	// Generalization clause (EXTENDS/GENERALIZATION before entity body)
	if genClause := ctx.GeneralizationClause(); genClause != nil {
		genCtx := genClause.(*parser.GeneralizationClauseContext)
		genName := buildQualifiedName(genCtx.QualifiedName())
		stmt.Generalization = &genName
	}

	// Entity body (attributes, options)
	if body := ctx.EntityBody(); body != nil {
		bodyCtx := body.(*parser.EntityBodyContext)

		// Attributes
		if attrList := bodyCtx.AttributeDefinitionList(); attrList != nil {
			stmt.Attributes = buildAttributes(attrList, b)
		}

		// Options (comment, extends, indexes, etc.)
		if opts := bodyCtx.EntityOptions(); opts != nil {
			optsCtx := opts.(*parser.EntityOptionsContext)
			for _, opt := range optsCtx.AllEntityOption() {
				optCtx := opt.(*parser.EntityOptionContext)
				if optCtx.COMMENT() != nil && optCtx.STRING_LITERAL() != nil {
					stmt.Comment = unquoteString(optCtx.STRING_LITERAL().GetText())
				}
				// Handle INDEX option
				if optCtx.INDEX() != nil && optCtx.IndexDefinition() != nil {
					stmt.Indexes = append(stmt.Indexes, buildIndex(optCtx.IndexDefinition()))
				}
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// buildViewEntity handles CREATE VIEW ENTITY statements.
func (b *Builder) buildViewEntity(ctx *parser.CreateEntityStatementContext) {
	stmt := &ast.CreateViewEntityStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Navigate to parent CreateStatement to get annotations and doc comment
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		// Check for CREATE OR MODIFY / CREATE OR REPLACE
		if createStmt.OR() != nil {
			if createStmt.MODIFY() != nil {
				stmt.CreateOrModify = true
			}
			if createStmt.REPLACE() != nil {
				stmt.CreateOrReplace = true
			}
		}

		// Extract annotations (@Position, etc.)
		for _, annCtx := range createStmt.AllAnnotation() {
			ann := annCtx.(*parser.AnnotationContext)
			annName := ann.AnnotationName().GetText()
			if strings.EqualFold(annName, "Position") {
				if params := ann.AnnotationParams(); params != nil {
					paramsCtx := params.(*parser.AnnotationParamsContext)
					allParams := paramsCtx.AllAnnotationParam()
					if len(allParams) >= 2 {
						x := parseAnnotationParamInt(allParams[0])
						y := parseAnnotationParamInt(allParams[1])
						stmt.Position = &ast.Position{X: x, Y: y}
					}
				}
			}
		}

	}
	stmt.Documentation = findDocCommentText(ctx)

	// Entity body (view attributes)
	if body := ctx.EntityBody(); body != nil {
		bodyCtx := body.(*parser.EntityBodyContext)
		if attrList := bodyCtx.AttributeDefinitionList(); attrList != nil {
			stmt.Attributes = buildViewAttributes(attrList)
		}
	}

	// OQL Query - use token stream to preserve whitespace, and walk parse tree for structured data
	if oqlCtx := ctx.OqlQuery(); oqlCtx != nil {
		raw := extractOriginalText(oqlCtx)
		stmt.Query = ast.OQLQuery{
			RawQuery: raw,
			Parsed:   buildOQLParsed(oqlCtx.(*parser.OqlQueryContext), raw),
		}
	}

	b.statements = append(b.statements, stmt)
}

// buildOQLParsed walks the ANTLR OQL parse tree and returns a structured OQLParsed.
func buildOQLParsed(ctx *parser.OqlQueryContext, rawQuery string) *ast.OQLParsed {
	if ctx == nil {
		return nil
	}

	parsed := &ast.OQLParsed{
		RawQuery: rawQuery,
	}

	// Take the first query term (UNION queries not supported for plan visualization)
	terms := ctx.AllOqlQueryTerm()
	if len(terms) == 0 {
		return parsed
	}
	term := terms[0].(*parser.OqlQueryTermContext)

	// SELECT clause
	if selCtx := term.SelectClause(); selCtx != nil {
		selClause := selCtx.(*parser.SelectClauseContext)
		if listCtx := selClause.SelectList(); listCtx != nil {
			selList := listCtx.(*parser.SelectListContext)
			if selList.STAR() != nil {
				parsed.Select = append(parsed.Select, ast.OQLSelectItem{Expression: "*"})
			} else {
				for _, itemCtx := range selList.AllSelectItem() {
					item := itemCtx.(*parser.SelectItemContext)
					parsed.Select = append(parsed.Select, buildOQLSelectItem(item))
				}
			}
		}
	}

	// FROM clause + JOINs
	if fromCtx := term.FromClause(); fromCtx != nil {
		fromClause := fromCtx.(*parser.FromClauseContext)

		// Primary table
		if tableRef := fromClause.TableReference(); tableRef != nil {
			parsed.Tables = append(parsed.Tables, buildOQLFromTable(tableRef.(*parser.TableReferenceContext)))
		}

		// JOIN clauses
		for _, joinCtx := range fromClause.AllJoinClause() {
			join := joinCtx.(*parser.JoinClauseContext)
			parsed.Tables = append(parsed.Tables, buildOQLJoinTable(join))
		}
	}

	// WHERE clause
	if whereCtx := term.WhereClause(); whereCtx != nil {
		wc := whereCtx.(*parser.WhereClauseContext)
		if expr := wc.Expression(); expr != nil {
			parsed.Where = extractOriginalText(expr.(antlr.ParserRuleContext))
		}
	}

	// GROUP BY clause
	if groupCtx := term.GroupByClause(); groupCtx != nil {
		gc := groupCtx.(*parser.GroupByClauseContext)
		if exprList := gc.ExpressionList(); exprList != nil {
			parsed.GroupBy = extractOriginalText(exprList.(antlr.ParserRuleContext))
		}
	}

	// HAVING clause
	if havingCtx := term.HavingClause(); havingCtx != nil {
		hc := havingCtx.(*parser.HavingClauseContext)
		if expr := hc.Expression(); expr != nil {
			parsed.Having = extractOriginalText(expr.(antlr.ParserRuleContext))
		}
	}

	// ORDER BY clause
	if orderCtx := term.OrderByClause(); orderCtx != nil {
		parsed.OrderBy = extractOriginalText(orderCtx.(antlr.ParserRuleContext))
	}

	return parsed
}

// buildOQLSelectItem converts a SelectItemContext to an OQLSelectItem.
func buildOQLSelectItem(ctx *parser.SelectItemContext) ast.OQLSelectItem {
	item := ast.OQLSelectItem{}

	if ctx.AggregateFunction() != nil {
		item.Expression = extractOriginalText(ctx.AggregateFunction().(antlr.ParserRuleContext))
		item.IsAggregate = true
	} else if ctx.Expression() != nil {
		item.Expression = extractOriginalText(ctx.Expression().(antlr.ParserRuleContext))
		// Check for scalar subquery inside the expression
		if oqlCtx := findOqlQueryInExpression(ctx.Expression().(antlr.ParserRuleContext)); oqlCtx != nil {
			subRaw := extractOriginalText(oqlCtx)
			item.Subquery = buildOQLParsed(oqlCtx, subRaw)
			item.SubqueryText = subRaw
		}
	}

	if ctx.SelectAlias() != nil {
		item.Alias = ctx.SelectAlias().GetText()
	}

	return item
}

// findOqlQueryInExpression recursively walks an expression parse tree to find
// a scalar subquery (oqlQuery). Returns the first OqlQueryContext found, or nil.
func findOqlQueryInExpression(ctx antlr.ParserRuleContext) *parser.OqlQueryContext {
	for i := 0; i < ctx.GetChildCount(); i++ {
		child := ctx.GetChild(i)
		if oqlCtx, ok := child.(*parser.OqlQueryContext); ok {
			return oqlCtx
		}
		if childRule, ok := child.(antlr.ParserRuleContext); ok {
			if found := findOqlQueryInExpression(childRule); found != nil {
				return found
			}
		}
	}
	return nil
}

// buildOQLFromTable converts a TableReferenceContext into an OQLTableRef for the FROM table.
func buildOQLFromTable(ctx *parser.TableReferenceContext) ast.OQLTableRef {
	ref := ast.OQLTableRef{
		JoinType: "from",
	}

	if qn := ctx.QualifiedName(); qn != nil {
		ref.Entity = getQualifiedNameText(qn)
	} else if oqlCtx := ctx.OqlQuery(); oqlCtx != nil {
		// Subquery: (FROM ... SELECT ...) AS alias
		subRaw := extractOriginalText(oqlCtx.(antlr.ParserRuleContext))
		sub := buildOQLParsed(oqlCtx.(*parser.OqlQueryContext), subRaw)
		ref.Subquery = sub
		ref.SubqueryText = subRaw
		// Use the subquery's first table entity as a fallback display entity
		if sub != nil && len(sub.Tables) > 0 {
			ref.Entity = sub.Tables[0].Entity
		}
	}

	if id := ctx.IDENTIFIER(); id != nil {
		ref.Alias = id.GetText()
	}

	return ref
}

// buildOQLJoinTable converts a JoinClauseContext into an OQLTableRef.
func buildOQLJoinTable(ctx *parser.JoinClauseContext) ast.OQLTableRef {
	ref := ast.OQLTableRef{
		JoinType: buildOQLJoinType(ctx.JoinType()),
	}

	if tableRef := ctx.TableReference(); tableRef != nil {
		// Standard JOIN with table reference (entity or subquery)
		tr := tableRef.(*parser.TableReferenceContext)
		if qn := tr.QualifiedName(); qn != nil {
			ref.Entity = getQualifiedNameText(qn)
		} else if oqlCtx := tr.OqlQuery(); oqlCtx != nil {
			// Subquery in JOIN: JOIN (FROM ... SELECT ...) AS alias
			subRaw := extractOriginalText(oqlCtx.(antlr.ParserRuleContext))
			sub := buildOQLParsed(oqlCtx.(*parser.OqlQueryContext), subRaw)
			ref.Subquery = sub
			ref.SubqueryText = subRaw
			if sub != nil && len(sub.Tables) > 0 {
				ref.Entity = sub.Tables[0].Entity
			}
		}
		if id := tr.IDENTIFIER(); id != nil {
			ref.Alias = id.GetText()
		}
	} else if assocPath := ctx.AssociationPath(); assocPath != nil {
		// Association path JOIN
		ap := assocPath.(*parser.AssociationPathContext)
		ref.AssocPath = extractOriginalText(ap)

		// Extract entity from the last qualified name in the path
		qualNames := ap.AllQualifiedName()
		if len(qualNames) > 0 {
			lastQN := qualNames[len(qualNames)-1]
			ref.Entity = getQualifiedNameText(lastQN)
		}

		// Alias is the IDENTIFIER on the JoinClause (not on AssociationPath)
		if id := ctx.IDENTIFIER(); id != nil {
			ref.Alias = id.GetText()
		}
	}

	// ON condition
	if ctx.ON() != nil && ctx.Expression() != nil {
		ref.OnExpr = extractOriginalText(ctx.Expression().(antlr.ParserRuleContext))
	}

	return ref
}

// buildOQLJoinType converts a JoinTypeContext to a canonical join type string.
func buildOQLJoinType(ctx parser.IJoinTypeContext) string {
	if ctx == nil {
		return "join"
	}
	jt := ctx.(*parser.JoinTypeContext)

	if jt.LEFT() != nil {
		return "left join"
	}
	if jt.RIGHT() != nil {
		return "right join"
	}
	if jt.FULL() != nil {
		return "full join"
	}
	if jt.CROSS() != nil {
		return "cross join"
	}
	if jt.INNER() != nil {
		return "join"
	}
	return "join"
}

// buildViewAttributes converts attribute definitions to ViewAttribute slice.
func buildViewAttributes(attrList parser.IAttributeDefinitionListContext) []ast.ViewAttribute {
	if attrList == nil {
		return nil
	}
	listCtx := attrList.(*parser.AttributeDefinitionListContext)
	var attrs []ast.ViewAttribute
	for _, attrDef := range listCtx.AllAttributeDefinition() {
		defCtx := attrDef.(*parser.AttributeDefinitionContext)
		// Nil check for AttributeName (can be nil on parse errors)
		if defCtx.AttributeName() == nil {
			continue
		}
		attr := ast.ViewAttribute{
			Name: attributeNameText(defCtx.AttributeName()),
		}
		if dt := defCtx.DataType(); dt != nil {
			attr.Type = buildDataType(dt)
		}
		attrs = append(attrs, attr)
	}
	return attrs
}

// findDocCommentText extracts the documentation comment text for a create statement.
// The docComment can appear either on the createStatement itself or on the parent statement rule.
func findDocCommentText(ctx antlr.RuleContext) string {
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if docCtx := createStmt.DocComment(); docCtx != nil {
			return extractDocComment(docCtx.GetText())
		}
	}
	stmtCtx := findParentStatement(ctx)
	if stmtCtx != nil {
		if docCtx := stmtCtx.DocComment(); docCtx != nil {
			return extractDocComment(docCtx.GetText())
		}
	}
	return ""
}

// findParentCreateStatement navigates up the parse tree to find the CreateStatement parent.
func findParentCreateStatement(ctx antlr.RuleContext) *parser.CreateStatementContext {
	parent := ctx.GetParent()
	for parent != nil {
		if createStmt, ok := parent.(*parser.CreateStatementContext); ok {
			return createStmt
		}
		parent = parent.GetParent()
	}
	return nil
}

// findParentStatement navigates up the parse tree to find the Statement parent.
func findParentStatement(ctx antlr.RuleContext) *parser.StatementContext {
	parent := ctx.GetParent()
	for parent != nil {
		if stmtCtx, ok := parent.(*parser.StatementContext); ok {
			return stmtCtx
		}
		parent = parent.GetParent()
	}
	return nil
}

// parseAnnotationParamInt parses an integer from an annotation parameter.
func parseAnnotationParamInt(ctx parser.IAnnotationParamContext) int {
	if ctx == nil {
		return 0
	}
	paramCtx := ctx.(*parser.AnnotationParamContext)
	if valueCtx := paramCtx.AnnotationValue(); valueCtx != nil {
		annValue := valueCtx.(*parser.AnnotationValueContext)
		if lit := annValue.Literal(); lit != nil {
			litCtx := lit.(*parser.LiteralContext)
			if litCtx.NUMBER_LITERAL() != nil {
				text := litCtx.NUMBER_LITERAL().GetText()
				if val, err := strconv.Atoi(text); err == nil {
					return val
				}
			}
		}
	}
	return 0
}

// ExitAlterEntityAction handles ALTER ENTITY ... ADD/DROP/RENAME/MODIFY ATTRIBUTE ...
func (b *Builder) ExitAlterEntityAction(ctx *parser.AlterEntityActionContext) {
	// Walk up to the parent AlterStatement to get the entity's qualified name
	parent := ctx.GetParent()
	for parent != nil {
		if alterStmt, ok := parent.(*parser.AlterStatementContext); ok {
			if alterStmt.ENTITY() == nil {
				return // Not an ALTER ENTITY statement
			}
			qn := alterStmt.QualifiedName()
			if qn == nil {
				return
			}
			name := buildQualifiedName(qn)
			attrNames := ctx.AllAttributeName()

			// ADD ATTRIBUTE / ADD COLUMN
			if ctx.ADD() != nil && (ctx.ATTRIBUTE() != nil || ctx.COLUMN() != nil) {
				if attrDef := ctx.AttributeDefinition(); attrDef != nil {
					attr := buildSingleAttribute(attrDef.(*parser.AttributeDefinitionContext))
					if attr != nil {
						b.statements = append(b.statements, &ast.AlterEntityStmt{
							Name:      name,
							Operation: ast.AlterEntityAddAttribute,
							Attribute: attr,
						})
					}
				}
				return
			}

			// RENAME ATTRIBUTE / RENAME COLUMN
			if ctx.RENAME() != nil && (ctx.ATTRIBUTE() != nil || ctx.COLUMN() != nil) && len(attrNames) >= 2 {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntityRenameAttribute,
					AttributeName: attributeNameText(attrNames[0]),
					NewName:       attributeNameText(attrNames[1]),
				})
				return
			}

			// MODIFY ATTRIBUTE / MODIFY COLUMN
			if ctx.MODIFY() != nil && (ctx.ATTRIBUTE() != nil || ctx.COLUMN() != nil) && len(attrNames) >= 1 {
				dt := buildDataType(ctx.DataType())
				stmt := &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntityModifyAttribute,
					AttributeName: attributeNameText(attrNames[0]),
					DataType:      dt,
				}
				// Capture CALCULATED constraint if present
				for _, constraintCtx := range ctx.AllAttributeConstraint() {
					c := constraintCtx.(*parser.AttributeConstraintContext)
					if c.CALCULATED() != nil {
						stmt.Calculated = true
						if qn := c.QualifiedName(); qn != nil {
							calcName := buildQualifiedName(qn)
							stmt.CalculatedMicroflow = &calcName
						}
					}
				}
				b.statements = append(b.statements, stmt)
				return
			}

			// DROP ATTRIBUTE / DROP COLUMN
			if ctx.DROP() != nil && (ctx.ATTRIBUTE() != nil || ctx.COLUMN() != nil) && len(attrNames) >= 1 {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntityDropAttribute,
					AttributeName: attributeNameText(attrNames[0]),
				})
				return
			}

			// SET DOCUMENTATION
			if ctx.SET() != nil && ctx.DOCUMENTATION() != nil && ctx.STRING_LITERAL() != nil {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntitySetDocumentation,
					Documentation: unquoteString(ctx.STRING_LITERAL().GetText()),
				})
				return
			}

			// SET COMMENT
			if ctx.SET() != nil && ctx.COMMENT() != nil && ctx.STRING_LITERAL() != nil {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:      name,
					Operation: ast.AlterEntitySetComment,
					Comment:   unquoteString(ctx.STRING_LITERAL().GetText()),
				})
				return
			}

			// SET STORE OWNER
			if ctx.SET() != nil && ctx.STORE() != nil && ctx.OWNER() != nil {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:      name,
					Operation: ast.AlterEntitySetStoreOwner,
				})
				return
			}

			// SET POSITION (x, y)
			if ctx.SET() != nil && ctx.POSITION() != nil {
				nums := ctx.AllNUMBER_LITERAL()
				if len(nums) >= 2 {
					x, _ := strconv.Atoi(nums[0].GetText())
					y, _ := strconv.Atoi(nums[1].GetText())
					b.statements = append(b.statements, &ast.AlterEntityStmt{
						Name:      name,
						Operation: ast.AlterEntitySetPosition,
						Position:  &ast.Position{X: x, Y: y},
					})
				}
				return
			}

			// ADD INDEX
			if ctx.ADD() != nil && ctx.INDEX() != nil {
				if idxDef := ctx.IndexDefinition(); idxDef != nil {
					idx := buildIndex(idxDef)
					b.statements = append(b.statements, &ast.AlterEntityStmt{
						Name:      name,
						Operation: ast.AlterEntityAddIndex,
						Index:     &idx,
					})
				}
				return
			}

			// DROP INDEX
			if ctx.DROP() != nil && ctx.INDEX() != nil && ctx.IDENTIFIER() != nil {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:      name,
					Operation: ast.AlterEntityDropIndex,
					IndexName: ctx.IDENTIFIER().GetText(),
				})
				return
			}

			break
		}
		parent = parent.GetParent()
	}
}

// ExitDropStatement handles DROP ENTITY/ASSOCIATION/ENUMERATION/MODULE/MICROFLOW/PAGE/SNIPPET
func (b *Builder) ExitDropStatement(ctx *parser.DropStatementContext) {
	// DROP CONFIGURATION uses STRING_LITERAL, not qualifiedName — handle first
	if ctx.CONFIGURATION() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			b.statements = append(b.statements, &ast.DropConfigurationStmt{
				Name: unquoteString(sl.GetText()),
			})
		}
		return
	}

	// Get the first qualified name (most DROP statements have at least one)
	names := ctx.AllQualifiedName()
	if len(names) == 0 {
		return
	}

	if ctx.ENTITY() != nil {
		b.statements = append(b.statements, &ast.DropEntityStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.ASSOCIATION() != nil {
		b.statements = append(b.statements, &ast.DropAssociationStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.ENUMERATION() != nil {
		b.statements = append(b.statements, &ast.DropEnumerationStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.CONSTANT() != nil {
		b.statements = append(b.statements, &ast.DropConstantStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.MODULE() != nil {
		name := getQualifiedNameText(names[0])
		b.statements = append(b.statements, &ast.DropModuleStmt{
			Name: name,
		})
	} else if ctx.MICROFLOW() != nil {
		b.statements = append(b.statements, &ast.DropMicroflowStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.PAGE() != nil {
		b.statements = append(b.statements, &ast.DropPageStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.SNIPPET() != nil {
		b.statements = append(b.statements, &ast.DropSnippetStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.JAVA() != nil && ctx.ACTION() != nil {
		b.statements = append(b.statements, &ast.DropJavaActionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.ODATA() != nil && ctx.CLIENT() != nil {
		b.statements = append(b.statements, &ast.DropODataClientStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.ODATA() != nil && ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.DropODataServiceStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.BUSINESS() != nil && ctx.EVENT() != nil && ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.DropBusinessEventServiceStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.WORKFLOW() != nil {
		b.statements = append(b.statements, &ast.DropWorkflowStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.IMAGE() != nil && ctx.COLLECTION() != nil {
		b.statements = append(b.statements, &ast.DropImageCollectionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.JSON() != nil && ctx.STRUCTURE() != nil {
		b.statements = append(b.statements, &ast.DropJsonStructureStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.REST() != nil && ctx.CLIENT() != nil {
		b.statements = append(b.statements, &ast.DropRestClientStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.FOLDER() != nil {
		folderPath := unquoteString(ctx.STRING_LITERAL().GetText())
		// Module can be a qualifiedName or IDENTIFIER
		var moduleName string
		if len(names) > 0 {
			moduleName = getQualifiedNameText(names[0])
		} else if ctx.IDENTIFIER() != nil {
			moduleName = ctx.IDENTIFIER().GetText()
		}
		b.statements = append(b.statements, &ast.DropFolderStmt{
			FolderPath: folderPath,
			Module:     moduleName,
		})
	}
}

// ExitMoveStatement handles MOVE PAGE/MICROFLOW/SNIPPET/NANOFLOW/ENTITY/ENUMERATION to folder/module
func (b *Builder) ExitMoveStatement(ctx *parser.MoveStatementContext) {
	names := ctx.AllQualifiedName()
	if len(names) == 0 {
		return
	}

	// Handle MOVE FOLDER separately — different AST type
	// MOVE FOLDER is identified by having FOLDER as the first token after MOVE (no document type keyword)
	if len(ctx.AllFOLDER()) > 0 && ctx.PAGE() == nil && ctx.MICROFLOW() == nil &&
		ctx.SNIPPET() == nil && ctx.NANOFLOW() == nil && ctx.ENTITY() == nil &&
		ctx.ENUMERATION() == nil && ctx.CONSTANT() == nil && ctx.DATABASE() == nil {
		b.exitMoveFolderStatement(ctx, names)
		return
	}

	stmt := &ast.MoveStmt{
		Name: buildQualifiedName(names[0]),
	}

	// Determine document type
	if ctx.PAGE() != nil {
		stmt.DocumentType = ast.DocumentTypePage
	} else if ctx.MICROFLOW() != nil {
		stmt.DocumentType = ast.DocumentTypeMicroflow
	} else if ctx.SNIPPET() != nil {
		stmt.DocumentType = ast.DocumentTypeSnippet
	} else if ctx.NANOFLOW() != nil {
		stmt.DocumentType = ast.DocumentTypeNanoflow
	} else if ctx.ENTITY() != nil {
		stmt.DocumentType = ast.DocumentTypeEntity
	} else if ctx.ENUMERATION() != nil {
		stmt.DocumentType = ast.DocumentTypeEnumeration
	} else if ctx.CONSTANT() != nil {
		stmt.DocumentType = ast.DocumentTypeConstant
	} else if ctx.DATABASE() != nil {
		stmt.DocumentType = ast.DocumentTypeDatabaseConnection
	}

	// Parse folder path if specified
	if len(ctx.AllFOLDER()) > 0 && ctx.STRING_LITERAL() != nil {
		stmt.Folder = unquoteString(ctx.STRING_LITERAL().GetText())
	}

	// Parse target module if specified (IN Module or just Module)
	if ctx.IN() != nil && len(names) > 1 {
		// MOVE ... TO FOLDER 'path' IN Module
		stmt.TargetModule = getQualifiedNameText(names[1])
	} else if ctx.IDENTIFIER() != nil {
		// MOVE ... TO FOLDER 'path' IN ModuleName (identifier)
		stmt.TargetModule = ctx.IDENTIFIER().GetText()
	} else if len(ctx.AllFOLDER()) == 0 && len(names) > 1 {
		// MOVE ... TO Module (no folder, just target module)
		stmt.TargetModule = getQualifiedNameText(names[1])
	}

	b.statements = append(b.statements, stmt)
}

// exitMoveFolderStatement handles MOVE FOLDER Module.FolderName TO ...
func (b *Builder) exitMoveFolderStatement(ctx *parser.MoveStatementContext, names []parser.IQualifiedNameContext) {
	stmt := &ast.MoveFolderStmt{
		Name: buildQualifiedName(names[0]),
	}

	// Parse target: either FOLDER 'path' [IN Module] or just Module
	if ctx.STRING_LITERAL() != nil {
		// MOVE FOLDER ... TO FOLDER 'path' [IN Module]
		stmt.TargetFolder = unquoteString(ctx.STRING_LITERAL().GetText())
		if len(names) > 1 {
			stmt.TargetModule = getQualifiedNameText(names[1])
		} else if ctx.IDENTIFIER() != nil {
			stmt.TargetModule = ctx.IDENTIFIER().GetText()
		}
	} else if len(names) > 1 {
		// MOVE FOLDER ... TO Module
		stmt.TargetModule = getQualifiedNameText(names[1])
	} else if ctx.IDENTIFIER() != nil {
		stmt.TargetModule = ctx.IDENTIFIER().GetText()
	}

	b.statements = append(b.statements, stmt)
}

// ----------------------------------------------------------------------------
// Association Statements
// ----------------------------------------------------------------------------

// ExitCreateAssociationStatement is called when exiting the createAssociationStatement production.
