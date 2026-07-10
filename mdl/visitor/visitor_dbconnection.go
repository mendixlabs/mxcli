// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitCreateDatabaseConnectionStatement handles CREATE DATABASE CONNECTION statements.
func (b *Builder) ExitCreateDatabaseConnectionStatement(ctx *parser.CreateDatabaseConnectionStatementContext) {
	stmt := &ast.CreateDatabaseConnectionStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Parse options
	for _, optCtx := range ctx.AllDatabaseConnectionOption() {
		opt := optCtx.(*parser.DatabaseConnectionOptionContext)

		if opt.TYPE() != nil && opt.STRING_LITERAL() != nil {
			stmt.DatabaseType = unquoteString(opt.STRING_LITERAL().GetText())
		}
		if opt.CONNECTION() != nil && opt.STRING_TYPE() != nil {
			// CONNECTION STRING <value>
			if opt.AT() != nil && opt.QualifiedName() != nil {
				// @Module.Constant reference
				qn := buildQualifiedName(opt.QualifiedName())
				stmt.ConnectionString = qn.String()
				stmt.ConnectionStringIsRef = true
			} else if opt.STRING_LITERAL() != nil {
				stmt.ConnectionString = unquoteString(opt.STRING_LITERAL().GetText())
			}
		}
		if opt.HOST() != nil && opt.STRING_LITERAL() != nil {
			stmt.Host = unquoteString(opt.STRING_LITERAL().GetText())
		}
		if opt.PORT() != nil && opt.NUMBER_LITERAL() != nil {
			stmt.Port, _ = strconv.Atoi(opt.NUMBER_LITERAL().GetText())
		}
		if opt.DATABASE() != nil && opt.STRING_LITERAL() != nil {
			stmt.Database = unquoteString(opt.STRING_LITERAL().GetText())
		}
		if opt.USERNAME() != nil {
			if opt.AT() != nil && opt.QualifiedName() != nil {
				qn := buildQualifiedName(opt.QualifiedName())
				stmt.UserName = qn.String()
				stmt.UserNameIsRef = true
			} else if opt.STRING_LITERAL() != nil {
				stmt.UserName = unquoteString(opt.STRING_LITERAL().GetText())
			}
		}
		if opt.PASSWORD() != nil {
			if opt.AT() != nil && opt.QualifiedName() != nil {
				qn := buildQualifiedName(opt.QualifiedName())
				stmt.Password = qn.String()
				stmt.PasswordIsRef = true
			} else if opt.STRING_LITERAL() != nil {
				stmt.Password = unquoteString(opt.STRING_LITERAL().GetText())
			}
		}
	}

	// Parse queries
	for _, qCtx := range ctx.AllDatabaseQuery() {
		qc := qCtx.(*parser.DatabaseQueryContext)
		q := ast.DatabaseQueryDef{}

		// Query name (first identifierOrKeyword in the rule)
		if iok := qc.IdentifierOrKeyword(0); iok != nil {
			q.Name = identifierOrKeywordText(iok)
		}

		// SQL string (STRING_LITERAL(0) is the SQL query)
		if sl := qc.STRING_LITERAL(0); sl != nil {
			q.SQL = unquoteString(sl.GetText())
		} else if ds := qc.DOLLAR_STRING(); ds != nil {
			q.SQL = unquoteDollarString(ds.GetText())
		}

		// RETURNS entity
		if qn := qc.QualifiedName(); qn != nil {
			q.Returns = buildQualifiedName(qn)
		}

		// PARAMETER clauses
		// Each PARAMETER has: identifierOrKeyword COLON dataType (DEFAULT STRING_LITERAL | NULL)?
		// identifierOrKeyword(0) is the query name, so params start at index 1
		paramTokens := qc.AllPARAMETER()
		defaultIdx := 0 // tracks DEFAULT occurrence index
		nullIdx := 0    // tracks NULL occurrence index
		for pi := range paramTokens {
			paramDef := ast.DatabaseQueryParamDef{}
			// Parameter name is identifierOrKeyword at index pi+1 (0 is query name)
			if iok := qc.IdentifierOrKeyword(pi + 1); iok != nil {
				paramDef.Name = identifierOrKeywordText(iok)
			}
			// DataType at index pi
			if dt := qc.DataType(pi); dt != nil {
				paramDef.DataType = buildDataType(dt)
			}
			// Check for DEFAULT or NULL — scan children to see what follows this PARAMETER's dataType
			// Use positional scanning: find the token that appears after the dataType for this param
			hasDefault := false
			hasNull := false
			paramParseChildren(qc, pi, &hasDefault, &hasNull)
			if hasDefault {
				// STRING_LITERAL index: 0 is SQL (if not dollar), then one per DEFAULT
				slIdx := defaultIdx + 1 // +1 because STRING_LITERAL(0) is the SQL string
				if qc.DOLLAR_STRING() != nil {
					slIdx = defaultIdx // SQL used DOLLAR_STRING
				}
				if sl := qc.STRING_LITERAL(slIdx); sl != nil {
					paramDef.DefaultValue = unquoteString(sl.GetText())
				}
				defaultIdx++
			} else if hasNull {
				paramDef.TestWithNull = true
				nullIdx++
			}
			q.Parameters = append(q.Parameters, paramDef)
		}

		// MAP clause
		for _, mapCtx := range qc.AllDatabaseQueryMapping() {
			mc := mapCtx.(*parser.DatabaseQueryMappingContext)
			ioks := mc.AllIdentifierOrKeyword()
			if len(ioks) >= 2 {
				q.Mappings = append(q.Mappings, ast.DatabaseQueryMappingDef{
					ColumnName:    identifierOrKeywordText(ioks[0]),
					AttributeName: identifierOrKeywordText(ioks[1]),
				})
			}
		}

		stmt.Queries = append(stmt.Queries, q)
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

// paramParseChildren scans the parse tree children of a DatabaseQueryContext to determine
// whether the parameter at index paramIdx has a DEFAULT or NULL modifier.
func paramParseChildren(qc *parser.DatabaseQueryContext, paramIdx int, hasDefault, hasNull *bool) {
	paramCount := 0
	children := qc.GetChildren()
	for i, child := range children {
		tn, ok := child.(antlr.TerminalNode)
		if !ok {
			continue
		}
		if tn.GetSymbol().GetTokenType() == parser.MDLParserPARAMETER {
			if paramCount == paramIdx {
				// Found our PARAMETER, now look ahead for DEFAULT or NULL before next PARAMETER/RETURNS/SEMICOLON
				for j := i + 1; j < len(children); j++ {
					tn2, ok := children[j].(antlr.TerminalNode)
					if !ok {
						continue
					}
					tt := tn2.GetSymbol().GetTokenType()
					if tt == parser.MDLParserPARAMETER || tt == parser.MDLParserRETURNS || tt == parser.MDLParserSEMICOLON {
						return
					}
					if tt == parser.MDLParserDEFAULT {
						*hasDefault = true
						return
					}
					if tt == parser.MDLParserNULL {
						*hasNull = true
						return
					}
				}
				return
			}
			paramCount++
		}
	}
}

// unquoteDollarString removes $$ delimiters from dollar-quoted strings.
func unquoteDollarString(s string) string {
	if strings.HasPrefix(s, "$$") && strings.HasSuffix(s, "$$") {
		return s[2 : len(s)-2]
	}
	return s
}
