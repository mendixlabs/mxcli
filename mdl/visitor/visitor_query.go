// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitShowStatement(ctx *parser.ShowStatementContext) {
	if ctx.MODULES() != nil {
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowModules})
	} else if ctx.EXTERNAL() != nil && ctx.ACTIONS() != nil {
		// SHOW EXTERNAL ACTIONS [IN module] - must come before ENTITIES/ACTIONS checks
		stmt := &ast.ShowStmt{ObjectType: ast.ShowExternalActions}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONTRACT() != nil && ctx.ENTITIES() != nil {
		// SHOW CONTRACT ENTITIES FROM Module.Service - must come before ENTITIES check
		stmt := &ast.ShowStmt{ObjectType: ast.ShowContractEntities}
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			stmt.Name = &name
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONTRACT() != nil && ctx.ACTIONS() != nil {
		// SHOW CONTRACT ACTIONS FROM Module.Service
		stmt := &ast.ShowStmt{ObjectType: ast.ShowContractActions}
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			stmt.Name = &name
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONTRACT() != nil && ctx.CHANNELS() != nil {
		// SHOW CONTRACT CHANNELS FROM Module.Service
		stmt := &ast.ShowStmt{ObjectType: ast.ShowContractChannels}
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			stmt.Name = &name
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONTRACT() != nil && ctx.MESSAGES() != nil {
		// SHOW CONTRACT MESSAGES FROM Module.Service
		stmt := &ast.ShowStmt{ObjectType: ast.ShowContractMessages}
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			stmt.Name = &name
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.EXTERNAL() != nil && ctx.ENTITIES() != nil {
		// SHOW EXTERNAL ENTITIES [IN module] - must come before ENTITIES check
		stmt := &ast.ShowStmt{ObjectType: ast.ShowExternalEntities}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.ENTITIES() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowEntities}
		// Handle "IN module" clause
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.ENTITY() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.ShowStmt{
				ObjectType: ast.ShowEntity,
				Name:       &name,
			})
		}
	} else if ctx.ASSOCIATIONS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowAssociations}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.ASSOCIATION() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.ShowStmt{
				ObjectType: ast.ShowAssociation,
				Name:       &name,
			})
		}
	} else if ctx.ENUMERATIONS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowEnumerations}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONSTANT() != nil && ctx.VALUES() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowConstantValues}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONSTANTS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowConstants}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.MICROFLOWS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowMicroflows}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.NANOFLOWS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowNanoflows}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.WORKFLOWS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowWorkflows}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.PAGES() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowPages}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.SNIPPETS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowSnippets}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.LAYOUTS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowLayouts}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.JAVA() != nil && ctx.ACTIONS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowJavaActions}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.JAVASCRIPT() != nil && ctx.ACTIONS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowJavaScriptActions}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.VERSION() != nil && ctx.FEATURES() == nil {
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowVersion})
	} else if ctx.CATALOG() != nil {
		// Check for SHOW CATALOG STATUS
		if ctx.STATUS() != nil {
			b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowCatalogStatus})
		} else {
			// SHOW CATALOG TABLES (or other catalog show commands)
			b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowCatalogTables})
		}
	} else if ctx.CALLERS() != nil {
		// SHOW CALLERS OF Module.Microflow [TRANSITIVE]
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.ShowStmt{
				ObjectType: ast.ShowCallers,
				Name:       &name,
				Transitive: ctx.TRANSITIVE() != nil,
			})
		}
	} else if ctx.CALLEES() != nil {
		// SHOW CALLEES OF Module.Microflow [TRANSITIVE]
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.ShowStmt{
				ObjectType: ast.ShowCallees,
				Name:       &name,
				Transitive: ctx.TRANSITIVE() != nil,
			})
		}
	} else if ctx.REFERENCES() != nil {
		// SHOW REFERENCES TO Module.Entity
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.ShowStmt{
				ObjectType: ast.ShowReferences,
				Name:       &name,
			})
		}
	} else if ctx.IMPACT() != nil {
		// SHOW IMPACT OF Module.Entity
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.ShowStmt{
				ObjectType: ast.ShowImpact,
				Name:       &name,
			})
		}
	} else if ctx.CONTEXT() != nil {
		// SHOW CONTEXT OF Module.Microflow [DEPTH N]
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			depth := 2 // Default depth
			if ctx.DEPTH() != nil {
				if numLit := ctx.NUMBER_LITERAL(); numLit != nil {
					if d, err := strconv.Atoi(numLit.GetText()); err == nil {
						depth = d
					}
				}
			}
			b.statements = append(b.statements, &ast.ShowStmt{
				ObjectType: ast.ShowContext,
				Name:       &name,
				Depth:      depth,
			})
		}
	} else if ctx.SECURITY() != nil && ctx.PROJECT() != nil {
		// SHOW PROJECT SECURITY
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowProjectSecurity})
	} else if ctx.SECURITY() != nil && ctx.MATRIX() != nil {
		// SHOW SECURITY MATRIX [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowSecurityMatrix}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.MODULE() != nil && ctx.ROLES() != nil {
		// SHOW MODULE ROLES [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowModuleRoles}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.USER() != nil && ctx.ROLES() != nil {
		// SHOW USER ROLES
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowUserRoles})
	} else if ctx.DEMO() != nil && ctx.USERS() != nil {
		// SHOW DEMO USERS
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowDemoUsers})
	} else if ctx.ACCESS() != nil {
		// SHOW ACCESS ON [MICROFLOW|PAGE] Module.Entity
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			if ctx.MICROFLOW() != nil {
				b.statements = append(b.statements, &ast.ShowStmt{
					ObjectType: ast.ShowAccessOnMicroflow,
					Name:       &name,
				})
			} else if ctx.PAGE() != nil {
				b.statements = append(b.statements, &ast.ShowStmt{
					ObjectType: ast.ShowAccessOnPage,
					Name:       &name,
				})
			} else if ctx.WORKFLOW() != nil {
				b.statements = append(b.statements, &ast.ShowStmt{
					ObjectType: ast.ShowAccessOnWorkflow,
					Name:       &name,
				})
			} else {
				b.statements = append(b.statements, &ast.ShowStmt{
					ObjectType: ast.ShowAccessOn,
					Name:       &name,
				})
			}
		}
	} else if ctx.ODATA() != nil && ctx.CLIENTS() != nil {
		// SHOW ODATA CLIENTS [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowODataClients}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.ODATA() != nil && ctx.SERVICES() != nil {
		// SHOW ODATA SERVICES [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowODataServices}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.NAVIGATION() != nil {
		// SHOW NAVIGATION / SHOW NAVIGATION MENU / SHOW NAVIGATION HOMES
		if ctx.HOMES() != nil {
			b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowNavigationHomes})
		} else if ctx.MENU_KW() != nil {
			stmt := &ast.ShowStmt{ObjectType: ast.ShowNavigationMenu}
			if qn := ctx.QualifiedName(); qn != nil {
				name := buildQualifiedName(qn)
				stmt.Name = &name
			} else if id := ctx.IDENTIFIER(); id != nil {
				name := ast.QualifiedName{Name: id.GetText()}
				stmt.Name = &name
			}
			b.statements = append(b.statements, stmt)
		} else {
			b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowNavigation})
		}
	} else if ctx.DESIGN() != nil && ctx.PROPERTIES() != nil {
		// SHOW DESIGN PROPERTIES [FOR widgetType]
		stmt := &ast.ShowDesignPropertiesStmt{}
		if wtk := ctx.WidgetTypeKeyword(); wtk != nil {
			stmt.WidgetType = strings.ToUpper(wtk.GetText())
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.STRUCTURE() != nil {
		// SHOW STRUCTURE [DEPTH n] [IN module] [ALL]
		depth := 2
		if ctx.DEPTH() != nil {
			if numLit := ctx.NUMBER_LITERAL(); numLit != nil {
				if d, err := strconv.Atoi(numLit.GetText()); err == nil {
					depth = d
				}
			}
		}
		stmt := &ast.ShowStmt{
			ObjectType: ast.ShowStructure,
			Depth:      depth,
			All:        ctx.ALL() != nil,
		}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.BUSINESS() != nil && ctx.EVENT() != nil && ctx.SERVICES() != nil {
		// SHOW BUSINESS EVENT SERVICES [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowBusinessEventServices}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.BUSINESS() != nil && ctx.EVENT() != nil && ctx.CLIENTS() != nil {
		// SHOW BUSINESS EVENT CLIENTS [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowBusinessEventClients}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.BUSINESS() != nil && ctx.EVENTS() != nil {
		// SHOW BUSINESS EVENTS [IN module] (individual messages)
		stmt := &ast.ShowStmt{ObjectType: ast.ShowBusinessEvents}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.SETTINGS() != nil {
		// SHOW SETTINGS
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowSettings})
	} else if ctx.FRAGMENTS() != nil {
		// SHOW FRAGMENTS
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowFragments})
	} else if ctx.DATABASE() != nil && ctx.CONNECTIONS() != nil {
		// SHOW DATABASE CONNECTIONS [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowDatabaseConnections}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.IMAGE() != nil && ctx.COLLECTION() != nil {
		// SHOW IMAGE COLLECTION [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowImageCollections}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.JSON() != nil && ctx.STRUCTURES() != nil {
		// SHOW JSON STRUCTURES [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowJsonStructures}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.PUBLISHED() != nil && ctx.REST() != nil && ctx.SERVICES() != nil {
		// SHOW PUBLISHED REST SERVICES [IN module] - must come before REST CLIENTS check
		stmt := &ast.ShowStmt{ObjectType: ast.ShowPublishedRestServices}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.LANGUAGES() != nil {
		// SHOW LANGUAGES
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowLanguages})
	} else if ctx.FEATURES() != nil {
		// SHOW FEATURES [IN area] | SHOW FEATURES FOR VERSION x.y | SHOW FEATURES ADDED SINCE x.y
		stmt := &ast.ShowFeaturesStmt{}
		if ctx.ADDED() != nil && ctx.SINCE() != nil {
			if nl := ctx.NUMBER_LITERAL(); nl != nil {
				stmt.AddedSince = nl.GetText()
			}
		} else if ctx.FOR() != nil && ctx.VERSION() != nil {
			if nl := ctx.NUMBER_LITERAL(); nl != nil {
				stmt.ForVersion = nl.GetText()
			}
		} else if ctx.IN() != nil {
			if id := ctx.IDENTIFIER(); id != nil {
				stmt.InArea = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.REST() != nil && ctx.CLIENTS() != nil {
		// SHOW REST CLIENTS [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowRestClients}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.WIDGETS() != nil {
		// SHOW WIDGETS [WHERE ...] [IN module]
		stmt := &ast.ShowWidgetsStmt{
			Filters: make([]ast.WidgetFilter, 0),
		}

		if filterCtx := ctx.ShowWidgetsFilter(); filterCtx != nil {
			// Parse filter conditions
			stmt.Filters = parseWidgetConditions(filterCtx.AllWidgetCondition())

			// Parse IN module clause
			if filterCtx.IN() != nil {
				if qn := filterCtx.QualifiedName(); qn != nil {
					stmt.InModule = getQualifiedNameText(qn)
				} else if id := filterCtx.IDENTIFIER(); id != nil {
					stmt.InModule = id.GetText()
				}
			}
		}

		b.statements = append(b.statements, stmt)
	}
}

// ExitCatalogSelectQuery handles SELECT ... FROM CATALOG.xxx queries.
func (b *Builder) ExitCatalogSelectQuery(ctx *parser.CatalogSelectQueryContext) {
	// Use extractOriginalText to preserve exact spacing, aliases, and dot notation
	query := extractOriginalText(ctx)
	b.statements = append(b.statements, &ast.SelectStmt{
		Query: query,
	})
}

// ExitDescribeStatement handles DESCRIBE ENTITY/ASSOCIATION/ENUMERATION/MODULE
func (b *Builder) ExitDescribeStatement(ctx *parser.DescribeStatementContext) {
	// Handle DESCRIBE MODULE ROLE (uses qualifiedName)
	if ctx.MODULE() != nil && ctx.ROLE() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeModuleRole,
				Name:       name,
			})
		}
		return
	}

	// Handle DESCRIBE DEMO USER 'name' (uses STRING_LITERAL)
	if ctx.DEMO() != nil && ctx.USER() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			userName := unquoteString(sl.GetText())
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeDemoUser,
				Name:       ast.QualifiedName{Name: userName},
			})
		}
		return
	}

	// Handle DESCRIBE USER ROLE 'Name' (uses STRING_LITERAL)
	if ctx.USER() != nil && ctx.ROLE() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			roleName := unquoteString(sl.GetText())
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeUserRole,
				Name:       ast.QualifiedName{Module: roleName, Name: roleName},
			})
		}
		return
	}

	// Handle DESCRIBE MODULE specially (uses IDENTIFIER not qualifiedName)
	if ctx.MODULE() != nil {
		var moduleName string
		if id := ctx.IDENTIFIER(); id != nil {
			moduleName = id.GetText()
		}
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeModule,
			Name:       ast.QualifiedName{Module: moduleName, Name: moduleName},
			WithAll:    ctx.ALL() != nil,
		})
		return
	}

	// Handle DESCRIBE ODATA CLIENT/SERVICE and DESCRIBE EXTERNAL ENTITY
	if ctx.ODATA() != nil && ctx.CLIENT() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeODataClient,
				Name:       name,
			})
		}
		return
	}
	if ctx.ODATA() != nil && ctx.SERVICE() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeODataService,
				Name:       name,
			})
		}
		return
	}
	if ctx.EXTERNAL() != nil && ctx.ENTITY() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeExternalEntity,
				Name:       name,
			})
		}
		return
	}

	// Handle DESCRIBE BUSINESS EVENT SERVICE Module.Name
	if ctx.BUSINESS() != nil && ctx.EVENT() != nil && ctx.SERVICE() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeBusinessEventService,
				Name:       name,
			})
		}
		return
	}

	// Handle DESCRIBE DATABASE CONNECTION Module.Name
	if ctx.DATABASE() != nil && ctx.CONNECTION() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeDatabaseConnection,
				Name:       name,
			})
		}
		return
	}

	// Handle DESCRIBE SETTINGS
	if ctx.SETTINGS() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeSettings,
		})
		return
	}

	// Handle DESCRIBE FRAGMENT FROM PAGE/SNIPPET ... WIDGET ... or DESCRIBE FRAGMENT Name
	if ctx.FRAGMENT() != nil {
		if ctx.FROM() != nil {
			// DESCRIBE FRAGMENT FROM PAGE/SNIPPET qualifiedName WIDGET name
			containerType := "PAGE"
			if ctx.SNIPPET() != nil {
				containerType = "SNIPPET"
			}
			qn := buildQualifiedName(ctx.QualifiedName())
			widgetName := ""
			if iok := ctx.IdentifierOrKeyword(); iok != nil {
				widgetName = identifierOrKeywordText(iok)
			}
			b.statements = append(b.statements, &ast.DescribeFragmentFromStmt{
				ContainerType: containerType,
				ContainerName: qn,
				WidgetName:    widgetName,
			})
			return
		}
		// Simple: DESCRIBE FRAGMENT Name
		if iok := ctx.IdentifierOrKeyword(); iok != nil {
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeFragment,
				Name:       ast.QualifiedName{Name: identifierOrKeywordText(iok)},
			})
		}
		return
	}

	// Handle DESCRIBE STYLING ON PAGE/SNIPPET Module.Name [WIDGET name]
	if ctx.STYLING() != nil && ctx.ON() != nil {
		stmt := &ast.DescribeStylingStmt{}
		if ctx.PAGE() != nil {
			stmt.ContainerType = "PAGE"
		} else if ctx.SNIPPET() != nil {
			stmt.ContainerType = "SNIPPET"
		}
		if qn := ctx.QualifiedName(); qn != nil {
			stmt.ContainerName = buildQualifiedName(qn)
		}
		if ctx.WIDGET() != nil {
			if id := ctx.IDENTIFIER(); id != nil {
				stmt.WidgetName = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
		return
	}

	// Handle DESCRIBE NAVIGATION [profile]
	if ctx.NAVIGATION() != nil {
		stmt := &ast.DescribeStmt{ObjectType: ast.DescribeNavigation}
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			stmt.Name = name
		} else if id := ctx.IDENTIFIER(); id != nil {
			stmt.Name = ast.QualifiedName{Name: id.GetText()}
		}
		b.statements = append(b.statements, stmt)
		return
	}

	// Handle DESCRIBE CATALOG.tablename
	if ctx.CATALOG() != nil {
		var tableName string
		if tbl := ctx.CatalogTableName(); tbl != nil {
			tableName = tbl.GetText()
		} else if id := ctx.IDENTIFIER(); id != nil {
			tableName = id.GetText()
		}
		if tableName != "" {
			b.statements = append(b.statements, &ast.DescribeCatalogTableStmt{
				TableName: strings.ToLower(tableName),
			})
		}
		return
	}

	// All other DESCRIBE statements use qualifiedName
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}
	name := buildQualifiedName(qn)

	if ctx.CONTRACT() != nil && ctx.ENTITY() != nil {
		stmt := &ast.DescribeStmt{
			ObjectType: ast.DescribeContractEntity,
			Name:       name,
		}
		if ctx.FORMAT() != nil && ctx.IDENTIFIER() != nil {
			stmt.Format = ctx.IDENTIFIER().GetText()
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONTRACT() != nil && ctx.ACTION() != nil {
		stmt := &ast.DescribeStmt{
			ObjectType: ast.DescribeContractAction,
			Name:       name,
		}
		if ctx.FORMAT() != nil && ctx.IDENTIFIER() != nil {
			stmt.Format = ctx.IDENTIFIER().GetText()
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONTRACT() != nil && ctx.MESSAGE() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeContractMessage,
			Name:       name,
		})
	} else if ctx.ENTITY() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeEntity,
			Name:       name,
		})
	} else if ctx.ASSOCIATION() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeAssociation,
			Name:       name,
		})
	} else if ctx.ENUMERATION() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeEnumeration,
			Name:       name,
		})
	} else if ctx.CONSTANT() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeConstant,
			Name:       name,
		})
	} else if ctx.MICROFLOW() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeMicroflow,
			Name:       name,
		})
	} else if ctx.WORKFLOW() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeWorkflow,
			Name:       name,
		})
	} else if ctx.PAGE() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribePage,
			Name:       name,
		})
	} else if ctx.SNIPPET() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeSnippet,
			Name:       name,
		})
	} else if ctx.LAYOUT() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeLayout,
			Name:       name,
		})
	} else if ctx.JAVA() != nil && ctx.ACTION() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeJavaAction,
			Name:       name,
		})
	} else if ctx.JAVASCRIPT() != nil && ctx.ACTION() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeJavaScriptAction,
			Name:       name,
		})
	} else if ctx.IMAGE() != nil && ctx.COLLECTION() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeImageCollection,
			Name:       name,
		})
	} else if ctx.JSON() != nil && ctx.STRUCTURE() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeJsonStructure,
			Name:       name,
		})
	} else if ctx.REST() != nil && ctx.CLIENT() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeRestClient,
			Name:       name,
		})
	} else if ctx.PUBLISHED() != nil && ctx.REST() != nil && ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribePublishedRestService,
			Name:       name,
		})
	}
}

// ExitSearchStatement handles SEARCH 'query'
func (b *Builder) ExitSearchStatement(ctx *parser.SearchStatementContext) {
	if ctx.STRING_LITERAL() != nil {
		b.statements = append(b.statements, &ast.SearchStmt{
			Query: unquoteString(ctx.STRING_LITERAL().GetText()),
		})
	}
}

// ----------------------------------------------------------------------------
// Utility Statements
// ----------------------------------------------------------------------------

// ExitExecuteScriptStatement handles EXECUTE SCRIPT 'script.mdl'
func (b *Builder) ExitExecuteScriptStatement(ctx *parser.ExecuteScriptStatementContext) {
	if ctx.STRING_LITERAL() != nil {
		b.statements = append(b.statements, &ast.ExecuteScriptStmt{
			Path: unquoteString(ctx.STRING_LITERAL().GetText()),
		})
	}
}

// ExitHelpStatement handles help/exit/quit commands
// Grammar: helpStatement: IDENTIFIER
func (b *Builder) ExitHelpStatement(ctx *parser.HelpStatementContext) {
	if id := ctx.IDENTIFIER(); id != nil {
		cmd := strings.ToLower(id.GetText())
		switch cmd {
		case "help", "?":
			b.statements = append(b.statements, &ast.HelpStmt{})
		case "exit", "quit":
			b.statements = append(b.statements, &ast.ExitStmt{})
		case "status":
			b.statements = append(b.statements, &ast.StatusStmt{})
		}
	}
}

// ExitUpdateStatement handles UPDATE and REFRESH [CATALOG [FULL] [FORCE] [BACKGROUND]] commands
func (b *Builder) ExitUpdateStatement(ctx *parser.UpdateStatementContext) {
	if ctx.UPDATE() != nil {
		b.statements = append(b.statements, &ast.UpdateStmt{})
	} else if ctx.REFRESH() != nil {
		if ctx.CATALOG() != nil {
			stmt := &ast.RefreshCatalogStmt{
				Full:       ctx.FULL() != nil,
				Source:     ctx.SOURCE_KW() != nil,
				Force:      ctx.FORCE() != nil,
				Background: ctx.BACKGROUND() != nil,
			}
			b.statements = append(b.statements, stmt)
		} else {
			b.statements = append(b.statements, &ast.RefreshStmt{})
		}
	}
}

// ----------------------------------------------------------------------------
// Helper Functions
// ----------------------------------------------------------------------------
