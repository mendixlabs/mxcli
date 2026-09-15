/**
 * MDL (Mendix Definition Language) Parser Grammar
 *
 * ANTLR4 parser for MDL syntax used by the Mendix REPL.
 * Converted from Chevrotain-based parser.
 *
 * This master file contains only the top-level dispatch rules.
 * Domain-specific rules live in domains/ and are merged at compile time
 * via ANTLR4's `import` directive.
 */
parser grammar MDLParser;

options {
    tokenVocab = MDLLexer;
}

import
    MDLDomainModel,
    MDLMicroflow,
    MDLPage,
    MDLSecurity,
    MDLAgent,
    MDLWorkflow,
    MDLService,
    MDLCatalog,
    MDLSettings;

// =============================================================================
// TOP-LEVEL RULES
// =============================================================================

/** Entry point: a program is a sequence of statements */
program
    : statement* EOF
    ;

/** A statement can be DDL, DQL, or utility */
statement
    : docComment? (ddlStatement | dqlStatement | utilityStatement) SEMICOLON? SLASH?
    ;

// =============================================================================
// DDL STATEMENTS (Data Definition Language)
// =============================================================================

ddlStatement
    : createStatement
    | alterStatement
    | dropStatement
    | renameStatement
    | moveStatement
    | updateWidgetsStatement
    | securityStatement
    ;

/**
 * Bulk update widget properties across pages/snippets.
 *
 * @example Preview changes (dry run)
 * ```mdl
 * UPDATE WIDGETS
 *   SET 'showLabel' = false
 *   WHERE WidgetType LIKE '%combobox%'
 *   DRY RUN;
 * ```
 *
 * @example Apply changes to widgets in a module
 * ```mdl
 * UPDATE WIDGETS
 *   SET 'filterMode' = 'contains'
 *   WHERE WidgetType LIKE '%DataGrid%'
 *   IN MyModule;
 * ```
 *
 * @example Multiple property assignments
 * ```mdl
 * UPDATE WIDGETS
 *   SET 'showLabel' = false, 'labelWidth' = 4
 *   WHERE WidgetType LIKE '%textbox%';
 * ```
 */
updateWidgetsStatement
    : UPDATE WIDGETS
      SET widgetPropertyAssignment (COMMA widgetPropertyAssignment)*
      WHERE widgetCondition (AND widgetCondition)*
      (IN (qualifiedName | IDENTIFIER))?
      (DRY RUN)?
    ;

createStatement
    : docComment? annotation*
      CREATE (OR (MODIFY | REPLACE))?
      ( createEntityStatement
      | createAssociationStatement
      | createModuleStatement
      | createMicroflowStatement
      | createJavaActionStatement
      | createJavaScriptActionStatement
      | createPageStatement
      | createLayoutStatement
      | createSnippetStatement
      | createEnumerationStatement
      | createValidationRuleStatement
      | createDatabaseConnectionStatement
      | createConstantStatement
      | createRestClientStatement
      | createIndexStatement
      | createODataClientStatement
      | createODataServiceStatement
      | createExternalEntityStatement
      | createExternalEntitiesStatement
      | createNavigationStatement
      | createBusinessEventServiceStatement
      | createWorkflowStatement
      | createUserRoleStatement
      | createDemoUserStatement
      | createImageCollectionStatement
      | createAnnotationStatement
      | createQueueStatement
      | createScheduledEventStatement
      | createRegularExpressionStatement
      | createJsonStructureStatement
      | createMessageDefinitionCollectionStatement
      | createImportMappingStatement
      | createExportMappingStatement
      | createConfigurationStatement
      | createPublishedRestServiceStatement
      | createDataTransformerStatement
      | createModelStatement
      | createConsumedMCPServiceStatement
      | createKnowledgeBaseStatement
      | createAgentStatement
      | createNanoflowStatement
      | createRuleStatement
      | createMenuStatement
      | createTranslationsStatement
      )
    ;

alterStatement
    : ALTER ENTITY qualifiedName alterEntityAction (COMMA? alterEntityAction)*
    | ALTER ASSOCIATION qualifiedName alterAssociationAction+
    | ALTER ENUMERATION qualifiedName alterEnumerationAction+
    | ALTER ODATA CLIENT qualifiedName SET odataAlterAssignment (COMMA odataAlterAssignment)*
    | ALTER ODATA SERVICE qualifiedName SET odataAlterAssignment (COMMA odataAlterAssignment)*
    | ALTER STYLING ON (PAGE | SNIPPET) qualifiedName WIDGET IDENTIFIER alterStylingAction+
    | ALTER SETTINGS alterSettingsClause
    | ALTER PAGE qualifiedName LBRACE alterPageOperation+ RBRACE
    | alterPagesLayoutStatement
    // ALTER LAYOUT reuses alterPageOperation wholesale: a layout's widget tree is
    // a page's widget tree with four extra element types, so SET/INSERT/DROP/
    // REPLACE mean exactly the same thing. A scroll-container region is addressed
    // through the dotted widgetRef the grammar already has — `layoutContainer.top`
    // — because a region has no Name of its own.
    | ALTER LAYOUT qualifiedName LBRACE alterPageOperation+ RBRACE
    | ALTER SNIPPET qualifiedName LBRACE alterPageOperation+ RBRACE
    | ALTER WORKFLOW qualifiedName alterWorkflowAction+ SEMICOLON?
    | alterMessageDefinitionCollectionStatement
    | alterMessageDefinitionStatement
    | ALTER PUBLISHED REST SERVICE qualifiedName alterPublishedRestServiceAction (COMMA? alterPublishedRestServiceAction)*
    | ALTER MODEL qualifiedName SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ALTER KNOWLEDGE BASE qualifiedName SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ALTER CONSUMED MCP SERVICE qualifiedName SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ALTER AGENT qualifiedName alterAgentAction+
    | alterModuleJarDepStatement
    ;

alterPublishedRestServiceAction
    : SET publishedRestAlterAssignment (COMMA publishedRestAlterAssignment)*
    | ADD publishedRestResource
    | DROP RESOURCE STRING_LITERAL
    ;

publishedRestAlterAssignment
    : identifierOrKeyword EQUALS STRING_LITERAL
    ;

/**
 * Styling modification actions for ALTER STYLING.
 *
 * @example Set Class and Style
 * ```mdl
 * ALTER STYLING ON PAGE MyModule.Page WIDGET btnSave
 *   SET Class = 'btn-lg', Style = 'margin-top: 8px;';
 * ```
 *
 * @example Set design property
 * ```mdl
 * ALTER STYLING ON PAGE MyModule.Page WIDGET ctn1
 *   SET 'Spacing top' = 'Large', 'Full width' = ON;
 * ```
 *
 * @example Clear all design properties
 * ```mdl
 * ALTER STYLING ON PAGE MyModule.Page WIDGET ctn1
 *   CLEAR DESIGN PROPERTIES;
 * ```
 */
alterStylingAction
    : SET alterStylingAssignment (COMMA alterStylingAssignment)*
    | CLEAR DESIGN PROPERTIES
    ;

alterStylingAssignment
    : CLASS EQUALS STRING_LITERAL                  // Class = 'my-class'
    | STYLE EQUALS STRING_LITERAL                  // Style = 'color: red;'
    | STRING_LITERAL EQUALS STRING_LITERAL         // 'Spacing top' = 'Large'
    | STRING_LITERAL EQUALS ON                     // 'Full width' = ON
    | STRING_LITERAL EQUALS OFF                    // 'Full width' = OFF
    ;

/**
 * ALTER PAGE operations for modifying widget trees in-place.
 *
 * @example Set property on widget
 * ```mdl
 * ALTER PAGE Module.Page {
 *   SET Caption = 'Save' ON btnSave
 * }
 * ```
 *
 * @example Insert widget after another
 * ```mdl
 * ALTER PAGE Module.Page {
 *   INSERT AFTER txtName { TEXTBOX txtNew (Label: 'New', Binds: Attr) }
 * }
 * ```
 *
 * @example Drop widgets
 * ```mdl
 * ALTER PAGE Module.Page {
 *   DROP WIDGET txtOld, txtUnused
 * }
 * ```
 *
 * @example Replace widget subtree
 * ```mdl
 * ALTER PAGE Module.Page {
 *   REPLACE footer1 WITH { FOOTER f1 { ACTIONBUTTON btn1 (Caption: 'OK', Action: SAVE_CHANGES) } }
 * }
 * ```
 */
alterPageOperation
    : alterPageSet SEMICOLON?
    | alterPageInsert SEMICOLON?
    | alterPageDrop SEMICOLON?
    | alterPageDropTemplate SEMICOLON?
    | alterPageReplace SEMICOLON?
    | alterPageAddVariable SEMICOLON?
    | alterPageDropVariable SEMICOLON?
    ;

alterPageSet
    : SET LAYOUT EQUALS qualifiedName (MAP LPAREN alterLayoutMapping (COMMA alterLayoutMapping)* RPAREN)?  // SET Layout = Atlas_Core.TopBar MAP (Main AS Content)
    | SET alterPageAssignment ON widgetRef                             // SET Caption = 'Save' ON btnSave  |  ON dgProducts.Name
    | SET LPAREN alterPageAssignment (COMMA alterPageAssignment)* RPAREN ON widgetRef  // SET (Caption = 'Save', ButtonStyle = Success) ON btnSave
    | SET alterPageAssignment                                                    // SET Title = 'Edit' (page-level)
    ;

alterLayoutMapping
    : identifierOrKeyword AS identifierOrKeyword                                // OldPlaceholder AS NewPlaceholder
    ;

// ALTER PAGES [IN <module>] SET LAYOUT = Module.Layout [MAP (...)] [WHERE LAYOUT = Module.Old]
//
// The bulk form is the real one: an app has one layout and many pages, so
// moving off Atlas_Default is a single statement rather than forty. WHERE is
// what makes it safe to run project-wide — "every page currently on X" is the
// migration anyone actually wants — so it is a filter on the current layout and
// nothing else.
alterPagesLayoutStatement
    : ALTER PAGES (IN identifierOrKeyword)? SET LAYOUT EQUALS qualifiedName
      (MAP LPAREN alterLayoutMapping (COMMA alterLayoutMapping)* RPAREN)?
      (WHERE LAYOUT EQUALS qualifiedName)?
    ;

alterPageAssignment
    : DATASOURCE EQUALS dataSourceExprV3               // DataSource = SELECTION widgetName
    | ACTION EQUALS actionExprV3                       // Action = MICROFLOW Module.MF | SHOW_PAGE Module.Page | SAVE_CHANGES CLOSE_PAGE
    | VISIBLE EQUALS xpathConstraint                   // Visible = [Name != ''] (conditional visibility)
    | EDITABLE EQUALS xpathConstraint                  // Editable = [Status = 'Open'] (conditional editability)
    | identifierOrKeyword EQUALS propertyValueV3       // Caption = 'Save'
    | STRING_LITERAL EQUALS propertyValueV3             // 'showLabel' = false
    ;

alterPageInsert
    : INSERT AFTER widgetRef LBRACE pageBodyV3 RBRACE
    | INSERT BEFORE widgetRef LBRACE pageBodyV3 RBRACE
    | INSERT INTO widgetRef LBRACE pageBodyV3 RBRACE   // append as children of a container
    ;

alterPageDrop
    : DROP WIDGET widgetRef (COMMA widgetRef)*
    ;

// DROP TEMPLATE FOR Module.Specialization IN listViewName
//
// A List View specialization template has no name — the entity it renders is
// what identifies it — so it cannot be reached through widgetRef like every
// other DROP target. Naming the list view is required, not optional: one page
// can hold two list views with a template for the same entity.
//
// There is no matching INSERT TEMPLATE. Adding one is
// `INSERT INTO <listview> { template for Module.Entity { ... } }`, which reuses
// the same block as CREATE PAGE, so a template has one spelling everywhere.
alterPageDropTemplate
    : DROP TEMPLATE FOR qualifiedName IN widgetRef
    ;

alterPageReplace
    : REPLACE widgetRef WITH LBRACE pageBodyV3 RBRACE
    ;

// Widget reference: plain name (btnSave) or dotted path (dgProducts.Name)
widgetRef
    : identifierOrKeyword DOT identifierOrKeyword    // dgProducts.Name (column ref)
    | identifierOrKeyword                            // btnSave (widget ref)
    ;

alterPageAddVariable
    : ADD VARIABLES_KW variableDeclaration    // ADD Variables $show: Boolean = 'true'
    ;

alterPageDropVariable
    : DROP VARIABLES_KW VARIABLE              // DROP Variables $show
    ;

navigationClause
    : HOME (PAGE | MICROFLOW) qualifiedName (FOR qualifiedName)?
    | LOGIN PAGE qualifiedName
    | NOT FOUND PAGE qualifiedName
    | MENU_KW LPAREN navMenuItemDef* RPAREN
    | SYNC LPAREN navSyncDef* RPAREN
    // Studio Pro's "Throw error when server rejects objects during
    // synchronization", stored as the profile-level ThrowPartialSyncError.
    //
    // Spelled with the phrase MDL already uses for failure handling — a
    // microflow's ON ERROR CONTINUE / ON ERROR ROLLBACK — so it needs no new
    // token and reads as something already learned. "Reject" is the platform's
    // own word, but REJECT appears ~500 times across the examples and skills
    // (approve/reject is one of the commonest things a workflow models), and
    // claiming a heavily-used identifier as a keyword is not worth the closer
    // paraphrase.
    | ON SYNC ERROR (THROW | CONTINUE)
    ;

// Offline synchronization, one statement per entity, mirroring the MENU block:
// a list of rules rather than a property bag, so it diffs a line at a time.
//
// WHERE implies the Constrained mode rather than naming it. A constrained
// entity with no constraint and a constraint with no mode are both nonsense,
// so deriving one from the other makes the invalid pair unspellable instead of
// merely diagnosable — and leaves Constrained with no bare word, which is
// correct because there is nothing to say without the XPath.
navSyncDef
    : SYNC qualifiedName navSyncMode SEMICOLON?
    ;

// Every alternative maps to exactly one Navigation$SyncMode member. The words
// are not the captions Studio Pro shows -- "All Objects" and "By XPath" are not
// members of the enumeration at all -- so the mapping lives in the visitor with
// a test asserting each target is a declared member.
navSyncMode
    : ONLINE
    | ALL
    | NEVER
    | NONE PRESERVE DATA
    | NONE
    // The bracket form is the first-class one and is what DESCRIBE emits: an
    // XPath constraint routinely contains quoted literals, and inside a quoted
    // MDL string every one of them doubles — the stored value already carries
    // Mendix's own escaping, so the two compose into runs of six quotes
    // (mendixlabs/mxcli#750). Brackets take the XPath verbatim.
    //
    // The quoted form still parses, because scripts already use it.
    | WHERE (xpathConstraint | STRING_LITERAL)
    ;

// The icon is a qualifiedName, like every other reference into the model, and
// not a string. Atlas icon names carry hyphens, which IDENTIFIER cannot lex, so
// those segments are double-quoted the same way a keyword-colliding name is:
//   ICON Atlas_Core.Atlas.home
//   ICON Atlas_Core.Atlas."align-center"
// SIGN_OUT is the third action a menu item can carry. Studio Pro writes it as
// the same Forms$SignOutClientAction a button uses (measured on ako/TestApp),
// which is why it sits beside PAGE and MICROFLOW rather than in a syntax of its
// own.
navMenuItemDef
    : MENU_KW ITEM STRING_LITERAL ((PAGE qualifiedName) | (MICROFLOW qualifiedName) | SIGN_OUT)? navMenuIcon? SEMICOLON?
    | MENU_KW STRING_LITERAL navMenuIcon? LPAREN navMenuItemDef* RPAREN SEMICOLON?
    ;

// Mendix stores three DIFFERENT icon elements, and they are not variants of one
// value: an icon-collection icon and an image icon each hold a qualified name —
// into an icon collection and an image collection, which are different documents
// — while a glyph icon holds a numeric character code and no name at all.
//
//   ICON Atlas_Core.Atlas.home            Forms$IconCollectionIcon
//   ICON GLYPH 57345                      Forms$GlyphIcon
//   ICON IMAGE MyModule.Images.logo       Forms$ImageIcon
//
// Only the first was expressible, so DESCRIBE emitted a comment for the other
// two and re-running its own output DESTROYED them.
//
// The two keyword-led alternatives come FIRST. qualifiedName accepts a keyword
// as a name segment (identifierOrKeyword), so `ICON IMAGE …` also matches the
// bare form with `image` read as the name; listing the specific alternatives
// ahead of the general one is what settles it.
navMenuIcon
    : ICON GLYPH NUMBER_LITERAL
    | ICON IMAGE qualifiedName
    | ICON qualifiedName
    ;

// A standalone menu document (Menus$MenuDocument) — the reusable menu a menu
// widget points at, as opposed to the menu inside a navigation profile. Both are
// built from the same items, so this reuses navMenuItemDef rather than defining a
// second item syntax.
createMenuStatement
    : MENU_KW qualifiedName (FOLDER STRING_LITERAL)? LPAREN navMenuItemDef* RPAREN
    ;

dropStatement
    : DROP ENTITY qualifiedName
    | DROP ASSOCIATION qualifiedName
    | DROP ENUMERATION qualifiedName
    | DROP CONSTANT qualifiedName
    | DROP MICROFLOW qualifiedName
    | DROP NANOFLOW qualifiedName
    | DROP RULE qualifiedName
    | DROP PAGE qualifiedName
    | DROP LAYOUT qualifiedName
    | DROP SNIPPET qualifiedName
    | DROP MENU_KW qualifiedName
    | DROP MODULE qualifiedName
    | DROP QUEUE qualifiedName
    | DROP SCHEDULED EVENT qualifiedName
    | DROP REGULAR EXPRESSION qualifiedName
    | DROP JAVA ACTION qualifiedName
    | DROP JAVASCRIPT ACTION qualifiedName
    | DROP INDEX qualifiedName ON qualifiedName
    | DROP ODATA CLIENT qualifiedName
    | DROP ODATA SERVICE qualifiedName
    | DROP BUSINESS EVENT SERVICE qualifiedName
    | DROP WORKFLOW qualifiedName
    | DROP IMAGE COLLECTION qualifiedName
    | DROP ANNOTATION STRING_LITERAL IN identifierOrKeyword
    | DROP ANNOTATION AT_KW LPAREN NUMBER_LITERAL COMMA NUMBER_LITERAL RPAREN IN identifierOrKeyword
    | DROP JSON STRUCTURE qualifiedName
    | DROP MESSAGE DEFINITION COLLECTION qualifiedName
    | DROP IMPORT MAPPING qualifiedName
    | DROP EXPORT MAPPING qualifiedName
    | DROP REST CLIENT qualifiedName
    | DROP PUBLISHED REST SERVICE qualifiedName
    | DROP DATA TRANSFORMER qualifiedName
    | DROP MODEL qualifiedName                               // DROP MODEL Module.Name (agent-editor)
    | DROP CONSUMED MCP SERVICE qualifiedName                // DROP CONSUMED MCP SERVICE Module.Name
    | DROP KNOWLEDGE BASE qualifiedName                      // DROP KNOWLEDGE BASE Module.Name
    | DROP AGENT qualifiedName                               // DROP AGENT Module.Name
    | DROP CONFIGURATION STRING_LITERAL
    | DROP FOLDER STRING_LITERAL IN (qualifiedName | IDENTIFIER)
    ;

renameStatement
    : RENAME renameTarget qualifiedName TO identifierOrKeyword (DRY RUN)?
    | RENAME MODULE identifierOrKeyword TO identifierOrKeyword (DRY RUN)?
    ;

renameTarget
    : ENTITY | MICROFLOW | NANOFLOW | PAGE | ENUMERATION | ASSOCIATION | CONSTANT | JAVA ACTION | WORKFLOW
    ;

/**
 * Moves a document to a different folder or module.
 *
 * @example Move page to folder in same module
 * ```mdl
 * MOVE PAGE MyModule.MyPage TO FOLDER 'Resources/Pages';
 * ```
 *
 * @example Move microflow to folder in different module
 * ```mdl
 * MOVE MICROFLOW MyModule.MyMicroflow TO FOLDER 'Utils' IN OtherModule;
 * ```
 *
 * @example Move snippet to module root (no folder)
 * ```mdl
 * MOVE SNIPPET MyModule.MySnippet TO OtherModule;
 * ```
 *
 * @example Move entity to different module (no folder support)
 * ```mdl
 * MOVE ENTITY MyModule.Customer TO OtherModule;
 * ```
 *
 * @example Move enumeration to different module
 * ```mdl
 * MOVE ENUMERATION MyModule.OrderStatus TO OtherModule;
 * ```
 *
 * @example Move an import mapping or JSON structure
 * ```mdl
 * MOVE IMPORT MAPPING MyModule.IMM_Order TO FOLDER 'Private/Import mappings';
 * MOVE JSON STRUCTURE MyModule.JSON_Order TO FOLDER 'Private/JSON structures';
 * ```
 */
moveStatement
    : MOVE moveDocumentType qualifiedName TO FOLDER STRING_LITERAL (IN (qualifiedName | IDENTIFIER))?
    | MOVE moveDocumentType qualifiedName TO (qualifiedName | IDENTIFIER)
    | MOVE ENTITY qualifiedName TO (qualifiedName | IDENTIFIER)
    | MOVE FOLDER qualifiedName TO FOLDER STRING_LITERAL (IN (qualifiedName | IDENTIFIER))?
    | MOVE FOLDER qualifiedName TO (qualifiedName | IDENTIFIER)
    ;

/**
 * The document types MOVE accepts — every top-level document, spelled as
 * DESCRIBE spells it.
 *
 * This is a rule rather than an inline alternation so that MOVE FOLDER can be
 * told from a document move by ONE check (`moveDocumentType` present or not)
 * instead of by a hand-maintained negation of every doctype keyword. That
 * negation is the trap mxcli-formula1 #32 flagged: each keyword added to the
 * move rule had to be added to the folder discriminator too, and forgetting one
 * silently turns `MOVE FOLDER …` into a document move.
 *
 * ENTITY is deliberately absent: an entity is not a unit, it lives inside a
 * domain model, and its move converts associations rather than reparenting a
 * row — so it keeps its own alternative and its own handler.
 */
moveDocumentType
    : PAGE
    | MICROFLOW
    | NANOFLOW
    | RULE
    | SNIPPET
    | BUILDING BLOCK
    | LAYOUT
    | MENU_KW
    | ENUMERATION
    | CONSTANT
    | WORKFLOW
    | QUEUE
    | SCHEDULED EVENT
    | REGULAR EXPRESSION
    | JSON STRUCTURE
    | IMPORT MAPPING
    | EXPORT MAPPING
    | JAVA ACTION
    | JAVASCRIPT ACTION
    | DATABASE CONNECTION
    | DATA TRANSFORMER
    | IMAGE COLLECTION
    | ICON COLLECTION
    | REST CLIENT
    | PUBLISHED REST SERVICE
    | ODATA CLIENT
    | ODATA SERVICE
    | BUSINESS EVENT SERVICE
    | MODEL
    | AGENT
    | KNOWLEDGE BASE
    | CONSUMED MCP SERVICE
    ;

// =============================================================================
// SECURITY STATEMENTS (dispatch list — rules in MDLSecurity.g4)
// =============================================================================

securityStatement
    : createModuleRoleStatement
    | dropModuleRoleStatement
    | alterUserRoleStatement
    | dropUserRoleStatement
    | grantEntityAccessStatement
    | revokeEntityAccessStatement
    | grantMicroflowAccessStatement
    | revokeMicroflowAccessStatement
    | grantNanoflowAccessStatement
    | revokeNanoflowAccessStatement
    | grantPageAccessStatement
    | revokePageAccessStatement
    | grantWorkflowAccessStatement
    | revokeWorkflowAccessStatement
    | grantODataServiceAccessStatement
    | revokeODataServiceAccessStatement
    | grantPublishedRestServiceAccessStatement
    | revokePublishedRestServiceAccessStatement
    | alterProjectSecurityStatement
    | dropDemoUserStatement
    | updateSecurityStatement
    ;

// =============================================================================
// DQL STATEMENTS (Data Query Language) — dispatch
// =============================================================================

dqlStatement
    : showStatement
    | describeStatement
    | catalogSelectQuery
    | oqlQuery
    ;
