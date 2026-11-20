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
	} else if ctx.ENTITY() != nil && ctx.ACCESS() == nil {
		// SHOW ENTITY Module.Entity. The ACCESS guard is load-bearing: this
		// branch runs long before the ACCESS one below, so without it
		// `SHOW ACCESS ON ENTITY Module.Entity` would answer with the entity's
		// definition instead of its access rules — a wrong answer rather than an
		// error, which is the harder kind to notice.
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.ShowStmt{
				ObjectType: ast.ShowEntity,
				Name:       &name,
			})
		}
	} else if ctx.ANNOTATIONS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowAnnotations}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
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
	} else if ctx.FOLDERS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowFolders}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
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
	} else if ctx.RULES() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowRules}
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
	} else if ctx.PAGE() != nil && ctx.ACCESS() == nil {
		// `SHOW PAGE Module.Page` (singular) is an alias for DESCRIBE PAGE, the
		// same way SHOW ENTITY / SHOW ASSOCIATION read as their describe. The
		// grammar alternative existed with no visitor branch, so the statement
		// parsed to nothing and the command exited 0 printing nothing — which
		// reads as "this page is empty", not "this command does nothing".
		//
		// The ACCESS guard is the sibling of the one on the ENTITY branch above:
		// this branch sits ~200 lines before the ACCESS one, so without it
		// `SHOW ACCESS ON PAGE Module.Page` printed the page's whole definition
		// instead of its allowed roles. Found by the #925 test sweep — the
		// spelling parsed, which is why it was reported as working.
		if qn := ctx.QualifiedName(); qn != nil {
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribePage,
				Name:       buildQualifiedName(qn),
			})
		}
	} else if ctx.CONNECTIONS() != nil && ctx.DATABASE() == nil {
		// SHOW CONNECTIONS lists the external SQL connections open in this
		// session (sql.Manager). The grammar alternative had no visitor branch,
		// so it parsed to nothing and printed nothing — indistinguishable from
		// "no connections are open", which is a plausible and wrong answer.
		//
		// The DATABASE() guard is load-bearing: `SHOW DATABASE CONNECTIONS`
		// also matches CONNECTIONS(), so without it this branch swallows the
		// stored-document listing and answers it with live session state.
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowConnections})
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
	} else if ctx.BUILDING() != nil && ctx.BLOCKS() != nil {
		stmt := &ast.ShowStmt{ObjectType: ast.ShowBuildingBlocks}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.TaskQueuesKw() != nil {
		stmt := &ast.ShowQueuesStmt{}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.Module = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.Module = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.SCHEDULED() != nil && ctx.EVENTS() != nil {
		stmt := &ast.ShowScheduledEventsStmt{}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.Module = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.Module = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.REGULAR() != nil && ctx.EXPRESSIONS() != nil {
		stmt := &ast.ShowRegularExpressionsStmt{}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.Module = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.Module = id.GetText()
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
		// `show features for version x.y` also carries VERSION; it belongs to
		// the FEATURES branch below (ako/mxcli#910).
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowVersion})
	} else if ctx.CATALOG() != nil {
		// SHOW CATALOG STATUS or SHOW CATALOG TABLES
		if ctx.STATUS() != nil {
			b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowCatalogStatus})
		} else {
			b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowCatalogTables})
		}
	} else if ctx.STATUS() != nil {
		// SHOW STATUS — print connection status (path, version, module count)
		b.statements = append(b.statements, &ast.StatusStmt{})
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
	} else if ctx.COMMUNITY() != nil {
		// SHOW COMMUNITY [MEMBERS] OF Module.Asset
		ot := ast.ShowCommunity
		if ctx.MEMBERS() != nil {
			ot = ast.ShowCommunityMembers
		}
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ot, Name: &name})
		}
	} else if ctx.COMMUNITIES() != nil {
		// SHOW COMMUNITIES
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowCommunities})
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
		// SHOW ACCESS ON [ENTITY|MICROFLOW|PAGE|WORKFLOW|NANOFLOW] Module.Name.
		// A bare name and an explicit ENTITY both fall through to ShowAccessOn,
		// which reports entity access rules.
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
			} else if ctx.NANOFLOW() != nil {
				b.statements = append(b.statements, &ast.ShowStmt{
					ObjectType: ast.ShowAccessOnNanoflow,
					Name:       &name,
				})
			} else {
				b.statements = append(b.statements, &ast.ShowStmt{
					ObjectType: ast.ShowAccessOn,
					Name:       &name,
				})
			}
		}
	} else if ctx.ConsumedODataServicesKw() != nil {
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
	} else if ctx.PublishedODataServicesKw() != nil {
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
			stmt.WidgetType = strings.ToLower(wtk.GetText())
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
	} else if ctx.GLYPHS() != nil {
		// SHOW GLYPHS [LIKE 'pattern'] — the Mendix glyph font, not a project
		// document, so there is no IN clause.
		stmt := &ast.ShowStmt{ObjectType: ast.ShowGlyphs}
		if sl := ctx.STRING_LITERAL(); sl != nil {
			stmt.Like = unquoteStringLit(sl)
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.ICON() != nil && ctx.COLLECTION() != nil {
		// SHOW ICON COLLECTION [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowIconCollections}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.AiModelsKw() != nil {
		// SHOW MODELS [IN module] (agent-editor Model documents)
		stmt := &ast.ShowStmt{ObjectType: ast.ShowModels}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.AGENTS() != nil {
		// SHOW AGENTS [IN module] (agent-editor Agent documents)
		stmt := &ast.ShowStmt{ObjectType: ast.ShowAgents}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.KNOWLEDGE() != nil && ctx.BASES() != nil {
		// SHOW KNOWLEDGE BASES [IN module] (agent-editor KB documents)
		stmt := &ast.ShowStmt{ObjectType: ast.ShowKnowledgeBases}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.CONSUMED() != nil && ctx.MCP() != nil && ctx.SERVICES() != nil {
		// SHOW CONSUMED MCP SERVICES [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowConsumedMCPServices}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.PUBLISHED() != nil && ctx.MCP() != nil && ctx.SERVICES() != nil {
		// LIST PUBLISHED MCP SERVICES [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowPublishedMCPServices}
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
	} else if ctx.DATA() != nil && ctx.TRANSFORMERS() != nil {
		// LIST DATA TRANSFORMERS [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowDataTransformers}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.ConsumedRestServicesKw() != nil {
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
	} else if ctx.WORKFLOW() != nil && ctx.GROUPS() != nil {
		// SHOW WORKFLOW GROUPS
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowWorkflowGroups})
	} else if ctx.LANGUAGES() != nil {
		// SHOW LANGUAGES
		b.statements = append(b.statements, &ast.ShowStmt{ObjectType: ast.ShowLanguages})
	} else if ctx.FEATURES() != nil {
		// SHOW FEATURES [IN area] | SHOW FEATURES FOR VERSION x.y | SHOW FEATURES ADDED SINCE x.y
		stmt := &ast.ShowFeaturesStmt{}
		if ctx.ADDED() != nil && ctx.SINCE() != nil {
			if fv := ctx.FeatureVersion(); fv != nil {
				stmt.AddedSince = fv.GetText()
			}
		} else if ctx.FOR() != nil && ctx.VERSION() != nil {
			if fv := ctx.FeatureVersion(); fv != nil {
				stmt.ForVersion = fv.GetText()
			}
		} else if ctx.IN() != nil {
			if id := ctx.IdentifierOrKeyword(); id != nil {
				stmt.InArea = id.GetText()
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
	} else if ctx.MESSAGE() != nil && ctx.DEFINITIONS() != nil {
		// SHOW MESSAGE DEFINITIONS [IN module] — the 11.15 documents (ako/mxcli#987)
		stmt := &ast.ShowStmt{ObjectType: ast.ShowMessageDefinitions}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.MESSAGE() != nil && ctx.DEFINITION() != nil && ctx.COLLECTION() != nil {
		// SHOW MESSAGE DEFINITION COLLECTIONS [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowMessageDefinitionCollections}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.IMPORT() != nil && ctx.MAPPINGS() != nil {
		// SHOW IMPORT MAPPINGS [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowImportMappings}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.EXPORT() != nil && ctx.MAPPINGS() != nil {
		// SHOW EXPORT MAPPINGS [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowExportMappings}
		if ctx.IN() != nil {
			if qn := ctx.QualifiedName(); qn != nil {
				stmt.InModule = getQualifiedNameText(qn)
			} else if id := ctx.IDENTIFIER(); id != nil {
				stmt.InModule = id.GetText()
			}
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.JAR() != nil && ctx.DEPENDENCIES() != nil {
		// LIST JAR DEPENDENCIES [IN module]
		stmt := &ast.ShowStmt{ObjectType: ast.ShowJarDependencies}
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
	// DESCRIBE TRANSLATIONS [IN Module] FOR <lang>. Placed first: TRANSLATIONS is
	// its own token, so nothing else can match, and reading it here keeps the
	// rest of the chain unaware of a statement that names no document.
	if ctx.TRANSLATIONS() != nil {
		ids := ctx.AllIdentifierOrKeyword()
		if len(ids) == 0 {
			return
		}
		stmt := &ast.DescribeTranslationsStmt{Language: identifierOrKeywordText(ids[len(ids)-1])}
		if len(ids) > 1 {
			stmt.Module = identifierOrKeywordText(ids[0])
		}
		stmt.WithoutMarketplace = ctx.MARKETPLACE() != nil
		b.statements = append(b.statements, stmt)
		return
	}

	// R6: the single-thing reports that were `show` forms. Each builds the
	// statement its `show` spelling builds, so the two are one statement and
	// `show` is a pure alias (MDL-DEPR090).
	if stmt := describeReport(ctx); stmt != nil {
		b.statements = append(b.statements, stmt)
		return
	}

	// DESCRIBE GLYPH 57350 | DESCRIBE GLYPH 'star'. Placed with the other
	// no-document statements: a glyph is a character code in a font, so it has no
	// qualified name for the chain below to build.
	if ctx.GLYPH() != nil {
		stmt := &ast.DescribeStmt{ObjectType: ast.DescribeGlyph}
		if n := ctx.NUMBER_LITERAL(); n != nil {
			stmt.Qualifier = n.GetText()
		} else if sl := ctx.STRING_LITERAL(); sl != nil {
			stmt.Qualifier = unquoteStringLit(sl)
		}
		if stmt.Qualifier != "" {
			b.statements = append(b.statements, stmt)
		}
		return
	}

	// DESCRIBE QUEUE Module.Name
	if ctx.TaskQueueKw() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			b.statements = append(b.statements, &ast.DescribeQueueStmt{Name: buildQualifiedName(qn)})
		}
		return
	}

	// DESCRIBE SCHEDULED EVENT Module.Name
	if ctx.SCHEDULED() != nil && ctx.EVENT() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			b.statements = append(b.statements, &ast.DescribeScheduledEventStmt{Name: buildQualifiedName(qn)})
		}
		return
	}

	// DESCRIBE REGULAR EXPRESSION Module.Name
	if ctx.REGULAR() != nil && ctx.EXPRESSION() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			b.statements = append(b.statements, &ast.DescribeRegularExpressionStmt{Name: buildQualifiedName(qn)})
		}
		return
	}

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
			userName := unquoteStringLit(sl)
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeDemoUser,
				Name:       ast.QualifiedName{Name: userName},
			})
		}
		return
	}

	// Handle DESCRIBE USER ROLE 'Name' | Name (quoted or bare — DROP accepts both
	// too, so the two commands are consistent; FINDINGS #5).
	if ctx.USER() != nil && ctx.ROLE() != nil {
		roleName := ""
		if sl := ctx.STRING_LITERAL(); sl != nil {
			roleName = unquoteStringLit(sl)
		} else if iok := ctx.IdentifierOrKeyword(0); iok != nil {
			roleName = identifierOrKeywordText(iok)
		}
		if roleName != "" {
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeUserRole,
				Name:       ast.QualifiedName{Module: roleName, Name: roleName},
			})
		}
		return
	}

	// Handle DESCRIBE MODULE specially (uses identifierOrKeyword — accepts
	// module names that happen to match a keyword, like "Agents").
	if ctx.MODULE() != nil {
		var moduleName string
		if iok := ctx.IdentifierOrKeyword(0); iok != nil {
			moduleName = iok.GetText()
		} else if id := ctx.IDENTIFIER(); id != nil {
			// Fallback for older grammar versions.
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
	if ctx.ConsumedODataServiceKw() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			b.statements = append(b.statements, &ast.DescribeStmt{
				ObjectType: ast.DescribeODataClient,
				Name:       name,
			})
		}
		return
	}
	if ctx.PublishedODataServiceKw() != nil {
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

	// Handle DESCRIBE SETTINGS [CONFIGURATION 'Name']
	if ctx.SETTINGS() != nil {
		stmt := &ast.DescribeStmt{ObjectType: ast.DescribeSettings}
		// `alter settings configuration 'X'` is the write form, so the read
		// form has to accept the same shape — reaching for it and getting a
		// parse error is the wrong lesson (mxcli-formula1 findings #8).
		if ctx.CONFIGURATION() != nil {
			if sl := ctx.STRING_LITERAL(); sl != nil {
				stmt.Qualifier = unquoteStringLit(sl)
			}
		}
		b.statements = append(b.statements, stmt)
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
			if iok := ctx.IdentifierOrKeyword(0); iok != nil {
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
		if iok := ctx.IdentifierOrKeyword(0); iok != nil {
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

	// DESCRIBE WIDGET <keyword|'widget id'> — a widget DEFINITION, not a
	// document, so there is no qualified name. The STYLING branch above
	// returns before this, but WIDGET also appears there (DESCRIBE STYLING …
	// WIDGET name), so the guard stays explicit rather than resting on order.
	if ctx.WIDGET() != nil && ctx.STYLING() == nil {
		name := ""
		if lit := ctx.STRING_LITERAL(); lit != nil {
			name = unquoteStringLit(lit)
		} else if id := ctx.IdentifierOrKeyword(0); id != nil {
			name = id.GetText()
		}
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeWidget,
			Name:       ast.QualifiedName{Name: name},
		})
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

	// Handle DESCRIBE JAR DEPENDENCY ModuleName 'group:artifact'
	if ctx.JAR() != nil && ctx.DEPENDENCY() != nil {
		var moduleName string
		if qn := ctx.QualifiedName(); qn != nil {
			moduleName = getQualifiedNameText(qn)
		} else if id := ctx.IDENTIFIER(); id != nil {
			moduleName = id.GetText()
		}
		coordinate := ""
		if sl := ctx.STRING_LITERAL(); sl != nil {
			coordinate = unquoteStringLit(sl)
		}
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeJarDependency,
			Name:       ast.QualifiedName{Name: moduleName},
			Qualifier:  coordinate,
		})
		return
	}

	// Handle DESCRIBE CONTRACT OPERATION FROM OPENAPI '/path/to/spec.json'
	// This rule uses a STRING_LITERAL (not a qualifiedName) and needs no project connection.
	if ctx.CONTRACT() != nil && ctx.OPERATION() != nil && ctx.FROM() != nil && ctx.OPENAPI() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			b.statements = append(b.statements, &ast.DescribeContractFromOpenAPIStmt{
				SpecPath: unquoteStringLit(sl),
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
			ObjectType:  ast.DescribeMicroflow,
			Name:        name,
			Normalized:  ctx.NORMALIZED() != nil,
			WithHandles: ctx.HANDLES() != nil,
		})
	} else if ctx.NANOFLOW() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeNanoflow,
			Name:       name,
		})
	} else if ctx.RULE() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeRule,
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
	} else if ctx.BUILDING() != nil && ctx.BLOCK() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeBuildingBlock,
			Name:       name,
		})
	} else if ctx.MENU_KW() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeMenu,
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
	} else if ctx.ICON() != nil && ctx.COLLECTION() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeIconCollection,
			Name:       name,
		})
	} else if ctx.AiModelKw() != nil {
		// DESCRIBE AI MODEL Module.Name (agent-editor Model document)
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeModel,
			Name:       name,
		})
	} else if ctx.AGENT() != nil {
		// DESCRIBE AGENT Module.Name (agent-editor Agent document)
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeAgent,
			Name:       name,
		})
	} else if ctx.KNOWLEDGE() != nil && ctx.BASE() != nil {
		// DESCRIBE KNOWLEDGE BASE Module.Name
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeKnowledgeBase,
			Name:       name,
		})
	} else if ctx.PUBLISHED() != nil && ctx.MCP() != nil && ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribePublishedMCPService,
			Name:       name,
		})
	} else if ctx.CONSUMED() != nil && ctx.MCP() != nil && ctx.SERVICE() != nil {
		// DESCRIBE CONSUMED MCP SERVICE Module.Name
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeConsumedMCPService,
			Name:       name,
		})
	} else if ctx.ConsumedRestServiceKw() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeRestClient,
			Name:       name,
		})
	} else if ctx.PUBLISHED() != nil && ctx.REST() != nil && ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribePublishedRestService,
			Name:       name,
		})
	} else if ctx.DATA() != nil && ctx.TRANSFORMER() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeDataTransformer,
			Name:       name,
		})
	} else if ctx.MESSAGE() != nil && ctx.DEFINITION() != nil && ctx.COLLECTION() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeMessageDefinitionCollection,
			Name:       name,
		})
	} else if ctx.MESSAGE() != nil && ctx.DEFINITION() != nil && ctx.CONTRACT() == nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeMessageDefinition,
			Name:       name,
		})
	} else if ctx.JSON() != nil && ctx.STRUCTURE() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeJsonStructure,
			Name:       name,
		})
	} else if ctx.IMPORT() != nil && ctx.MAPPING() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeImportMapping,
			Name:       name,
		})
	} else if ctx.EXPORT() != nil && ctx.MAPPING() != nil {
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeExportMapping,
			Name:       name,
		})
	} else {
		// Bare DESCRIBE qualifiedName — no type keyword matched, so the document
		// type is auto-detected from the project at execution time.
		b.statements = append(b.statements, &ast.DescribeStmt{
			ObjectType: ast.DescribeAuto,
			Name:       name,
		})
	}
}

// ExitSearchStatement handles SEARCH 'query'
func (b *Builder) ExitSearchStatement(ctx *parser.SearchStatementContext) {
	if ctx.STRING_LITERAL() != nil {
		b.statements = append(b.statements, &ast.SearchStmt{
			Query: unquoteStringLit(ctx.STRING_LITERAL()),
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
			Path: unquoteStringLit(ctx.STRING_LITERAL()),
		})
	}
}

// ExitSessionSetStatement handles session-level SET key = value
// Grammar: sessionSetStatement: SET identifierOrKeyword EQUALS sessionSetValue
func (b *Builder) ExitSessionSetStatement(ctx *parser.SessionSetStatementContext) {
	keyCtx := ctx.IdentifierOrKeyword()
	valCtx := ctx.SessionSetValue()
	if keyCtx == nil || valCtx == nil {
		return
	}
	key := keyCtx.GetText()
	var value any
	switch {
	case valCtx.TRUE() != nil:
		value = true
	case valCtx.FALSE() != nil:
		value = false
	case valCtx.NUMBER_LITERAL() != nil:
		if i, err := strconv.ParseInt(valCtx.NUMBER_LITERAL().GetText(), 10, 64); err == nil {
			value = i
		} else {
			value = valCtx.NUMBER_LITERAL().GetText()
		}
	case valCtx.STRING_LITERAL() != nil:
		value = unquoteStringLit(valCtx.STRING_LITERAL())
	case valCtx.IdentifierOrKeyword() != nil:
		value = valCtx.IdentifierOrKeyword().GetText()
	}
	b.statements = append(b.statements, &ast.SetStmt{Key: key, Value: value})
}

// ExitHelpStatement handles help/exit/quit commands
// Grammar: helpStatement: IDENTIFIER (DOT? helpTopicWord)*
func (b *Builder) ExitHelpStatement(ctx *parser.HelpStatementContext) {
	if id := ctx.IDENTIFIER(); id != nil {
		cmd := strings.ToLower(id.GetText())
		switch cmd {
		case "help", "?":
			stmt := &ast.HelpStmt{}
			// One word per segment, however it was spelled. The DOT is a
			// separator, not part of a word, and a word may itself be dotted
			// when a whole path arrives as one token — syntax.Lookup splits on
			// both, so the topic is passed on as the reader typed it.
			for _, tok := range ctx.AllHelpTopicWord() {
				stmt.Topic = append(stmt.Topic, strings.ToLower(tok.GetText()))
			}
			b.statements = append(b.statements, stmt)
		case "exit", "quit":
			b.statements = append(b.statements, &ast.ExitStmt{})
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
				Full:        ctx.FULL() != nil,
				Source:      ctx.SOURCE_KW() != nil,
				Force:       ctx.FORCE() != nil,
				Background:  ctx.BACKGROUND() != nil,
				Communities: ctx.COMMUNITIES() != nil,
			}
			if ctx.RESOLUTION() != nil && ctx.NUMBER_LITERAL() != nil {
				if f, err := strconv.ParseFloat(ctx.NUMBER_LITERAL().GetText(), 64); err == nil {
					stmt.Resolution = f
				}
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

// describeReport builds `describe app security`, `describe security matrix`,
// `describe structure` and `describe context of`: the same statements as their
// `show` spellings (see ExitShowStatement). Nil for every other describe.
func describeReport(ctx *parser.DescribeStatementContext) ast.Statement {
	inModule := func() string {
		if ctx.IN() == nil {
			return ""
		}
		if qn := ctx.QualifiedName(); qn != nil {
			return getQualifiedNameText(qn)
		}
		if id := ctx.IDENTIFIER(); id != nil {
			return id.GetText()
		}
		return ""
	}
	depth := func() int {
		if ctx.DEPTH() != nil {
			if n := ctx.NUMBER_LITERAL(); n != nil {
				if d, err := strconv.Atoi(n.GetText()); err == nil {
					return d
				}
			}
		}
		return 2
	}
	switch {
	case ctx.APP() != nil && ctx.SECURITY() != nil:
		return &ast.ShowStmt{ObjectType: ast.ShowProjectSecurity}
	case ctx.SECURITY() != nil && ctx.MATRIX() != nil:
		return &ast.ShowStmt{ObjectType: ast.ShowSecurityMatrix, InModule: inModule()}
	case ctx.STRUCTURE() != nil && ctx.JSON() == nil:
		return &ast.ShowStmt{ObjectType: ast.ShowStructure, Depth: depth(), All: ctx.ALL() != nil, InModule: inModule()}
	case ctx.CONTEXT() != nil && ctx.OF() != nil:
		qn := ctx.QualifiedName()
		if qn == nil {
			return nil
		}
		name := buildQualifiedName(qn)
		return &ast.ShowStmt{ObjectType: ast.ShowContext, Name: &name, Depth: depth()}
	}
	return nil
}
