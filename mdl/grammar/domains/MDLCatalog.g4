/**
 * MDL Catalog Grammar — DQL statements (SHOW/LIST/DESCRIBE/SELECT catalog), OQL query.
 */
parser grammar MDLCatalog;

options { tokenVocab = MDLLexer; }

// =============================================================================
// DQL STATEMENTS (Data Query Language)
// =============================================================================

showOrList: SHOW /* @alias MDL-DEPR002 */ | LIST_KW ;

showStatement
    : showOrList MODULES
    | showOrList CONTRACT ENTITIES FROM qualifiedName    // SHOW CONTRACT ENTITIES FROM Module.Service
    | showOrList CONTRACT ACTIONS FROM qualifiedName     // SHOW CONTRACT ACTIONS FROM Module.Service
    | showOrList CONTRACT CHANNELS FROM qualifiedName   // SHOW CONTRACT CHANNELS FROM Module.Service (AsyncAPI)
    | showOrList CONTRACT MESSAGES FROM qualifiedName   // SHOW CONTRACT MESSAGES FROM Module.Service (AsyncAPI)
    | showOrList ENTITIES (IN (qualifiedName | IDENTIFIER))?
    | showOrList ASSOCIATIONS (IN (qualifiedName | IDENTIFIER))?
    | showOrList ANNOTATIONS (IN (qualifiedName | IDENTIFIER))?   // domain-model canvas notes
    | showOrList MICROFLOWS (IN (qualifiedName | IDENTIFIER))?
    | showOrList NANOFLOWS (IN (qualifiedName | IDENTIFIER))?
    | showOrList RULES (IN (qualifiedName | IDENTIFIER))?
    | showOrList WORKFLOWS (IN (qualifiedName | IDENTIFIER))?
    // The project-level workflow groups (App Settings > Workflows > Groups),
    // not a per-module listing: they are settings, so there is no IN <module>.
    | showOrList WORKFLOW GROUPS
    | showOrList PAGES (IN (qualifiedName | IDENTIFIER))?
    | showOrList SNIPPETS (IN (qualifiedName | IDENTIFIER))?
    | showOrList BUILDING BLOCKS (IN (qualifiedName | IDENTIFIER))?
    | showOrList ENUMERATIONS (IN (qualifiedName | IDENTIFIER))?
    // LIST FOLDERS is the layout view: which folders exist and what is in them.
    // MOVE could place a document but nothing could read the placement back, so
    // a layout could not be confirmed or diffed without opening the .mpr as
    // SQLite (mxcli-formula1 #32).
    | showOrList FOLDERS (IN (qualifiedName | IDENTIFIER))?
    | showOrList CONSTANTS (IN (qualifiedName | IDENTIFIER))?
    | showOrList CONSTANT VALUES (IN (qualifiedName | IDENTIFIER))?
    | showOrList LAYOUTS (IN (qualifiedName | IDENTIFIER))?
    | showOrList taskQueuesKw (IN (qualifiedName | IDENTIFIER))?
    | showOrList SCHEDULED EVENTS (IN (qualifiedName | IDENTIFIER))?
    | showOrList REGULAR EXPRESSIONS (IN (qualifiedName | IDENTIFIER))?
    | showOrList JAVA ACTIONS (IN (qualifiedName | IDENTIFIER))?
    | showOrList JAVASCRIPT ACTIONS (IN (qualifiedName | IDENTIFIER))?
    // COLLECTION also matches `collections` (lexer); the plural is canonical,
    // the singular a deprecated alias (/* @alias MDL-DEPR130 */, R6).
    | showOrList IMAGE COLLECTION (IN (qualifiedName | IDENTIFIER))?
    | showOrList ICON COLLECTION (IN (qualifiedName | IDENTIFIER))?
    // A glyph is a character code in a FONT, not an element in the project, so
    // there is nothing to scope with IN and nothing a connection would add.
    // LIKE filters on the name, which is the direction an author needs: they
    // know they want a star and not that a star is 57350.
    | showOrList GLYPHS (LIKE STRING_LITERAL)?
    | showOrList aiModelsKw (IN (qualifiedName | IDENTIFIER))?
    | showOrList AGENTS (IN (qualifiedName | IDENTIFIER))?
    | showOrList KNOWLEDGE BASES (IN (qualifiedName | IDENTIFIER))?
    | showOrList CONSUMED MCP SERVICES (IN (qualifiedName | IDENTIFIER))?
    | showOrList PUBLISHED MCP SERVICES (IN (qualifiedName | IDENTIFIER))?
    | showOrList JSON STRUCTURES (IN (qualifiedName | IDENTIFIER))?
    | showOrList MESSAGE DEFINITION COLLECTION (IN (qualifiedName | IDENTIFIER))?
    | showOrList MESSAGE DEFINITIONS (IN (qualifiedName | IDENTIFIER))?
    | showOrList IMPORT MAPPINGS (IN (qualifiedName | IDENTIFIER))?
    | showOrList EXPORT MAPPINGS (IN (qualifiedName | IDENTIFIER))?
    // R6: a single thing is described, not shown. `show page X` is a
    // deprecated alias of `describe page X` (MDL-DEPR090). The entity and
    // association summaries have no mdl 1 statement: they keep their summary
    // under mdl 0 and are refused under mdl 1 (MDL-V1-SHOWSUMMARY).
    | showOrList ENTITY qualifiedName
    | showOrList ASSOCIATION qualifiedName
    | showOrList PAGE qualifiedName /* @alias MDL-DEPR090 */
    | showOrList CONNECTIONS
    | showOrList STATUS
    | showOrList VERSION
    | showOrList CATALOG STATUS
    | showOrList CATALOG TABLES
    | showOrList CALLERS OF qualifiedName TRANSITIVE?
    | showOrList CALLEES OF qualifiedName TRANSITIVE?
    | showOrList REFERENCES TO qualifiedName
    | showOrList IMPACT OF qualifiedName
    | showOrList CONTEXT OF qualifiedName (DEPTH NUMBER_LITERAL)? /* @alias MDL-DEPR090 */
    | showOrList COMMUNITY MEMBERS OF qualifiedName
    | showOrList COMMUNITY OF qualifiedName
    | showOrList COMMUNITIES
    | showOrList WIDGETS showWidgetsFilter?
    | showOrList PROJECT SECURITY /* @alias MDL-DEPR090 */
    | showOrList MODULE ROLES (IN (qualifiedName | IDENTIFIER))?
    | showOrList USER ROLES
    | showOrList DEMO USERS
    | showOrList ACCESS ON qualifiedName
    // ENTITY is spelled out as well as implied by the bare form above: every
    // other document kind names itself here, and the generated project CLAUDE.md
    // documents `SHOW ACCESS ON MICROFLOW|PAGE|ENTITY`. Without this alternative
    // `ENTITY` is swallowed by qualifiedName (it is in the `keyword` rule), so
    // the statement ends at `ENTITY` and the real name is extraneous input.
    | showOrList ACCESS ON ENTITY qualifiedName
    | showOrList ACCESS ON MICROFLOW qualifiedName
    | showOrList ACCESS ON PAGE qualifiedName
    | showOrList ACCESS ON WORKFLOW qualifiedName
    | showOrList ACCESS ON NANOFLOW qualifiedName
    | showOrList SECURITY MATRIX (IN (qualifiedName | IDENTIFIER))? /* @alias MDL-DEPR090 */
    | showOrList consumedODataServicesKw (IN (qualifiedName | IDENTIFIER))?
    | showOrList publishedODataServicesKw (IN (qualifiedName | IDENTIFIER))?
    | showOrList EXTERNAL ENTITIES (IN (qualifiedName | IDENTIFIER))?
    | showOrList EXTERNAL ACTIONS (IN (qualifiedName | IDENTIFIER))?
    // The profiles table and the menu tree are listings: `list navigation`.
    | showOrList NAVIGATION
    | showOrList NAVIGATION MENU_KW (qualifiedName | IDENTIFIER)?
    | showOrList NAVIGATION HOMES
    | showOrList DESIGN PROPERTIES (FOR widgetTypeKeyword)?
    | showOrList STRUCTURE (DEPTH NUMBER_LITERAL)? (IN (qualifiedName | IDENTIFIER))? ALL? /* @alias MDL-DEPR090 */
    | showOrList BUSINESS EVENT SERVICES (IN (qualifiedName | IDENTIFIER))?
    | showOrList BUSINESS EVENT CLIENTS (IN (qualifiedName | IDENTIFIER))?
    | showOrList BUSINESS EVENTS (IN (qualifiedName | IDENTIFIER))?
    | showOrList SETTINGS   // the settings sections, one row each: `list settings`
    | showOrList FRAGMENTS
    | showOrList DATABASE CONNECTIONS (IN (qualifiedName | IDENTIFIER))?
    | showOrList consumedRestServicesKw (IN (qualifiedName | IDENTIFIER))?
    | showOrList PUBLISHED REST SERVICES (IN (qualifiedName | IDENTIFIER))?
    | showOrList DATA TRANSFORMERS (IN (qualifiedName | IDENTIFIER))?
    | showOrList LANGUAGES
    | showOrList FEATURES (IN identifierOrKeyword)?   // area names are keywords: workflows, pages, … (#1367)
    | showOrList FEATURES FOR VERSION featureVersion
    | showOrList FEATURES ADDED SINCE featureVersion
    | showOrList JAR DEPENDENCIES (IN (qualifiedName | IDENTIFIER))?
    ;

/**
 * Widget filtering for SHOW WIDGETS and UPDATE WIDGETS.
 */
showWidgetsFilter
    : WHERE widgetCondition (AND widgetCondition)* (IN (qualifiedName | IDENTIFIER))?
    | IN (qualifiedName | IDENTIFIER)
    ;

/**
 * Widget type keyword for SHOW DESIGN PROPERTIES FOR <type>.
 */
widgetTypeKeyword
    : CONTAINER | TEXTBOX | TEXTAREA | CHECKBOX | RADIOBUTTONS | DATEPICKER
    | COMBOBOX | DYNAMICTEXT | ACTIONBUTTON | LINKBUTTON | DATAVIEW
    | LISTVIEW | DATAGRID | GALLERY | LAYOUTGRID | IMAGE | STATICIMAGE
    | DYNAMICIMAGE | HEADER | FOOTER | SNIPPETCALL | NAVIGATIONLIST
    | CUSTOMCONTAINER | TABCONTAINER | TABPAGE | DROPDOWN | REFERENCESELECTOR | GROUPBOX
    | FILEMANAGER
    | IDENTIFIER
    ;

widgetCondition
    : WIDGETTYPE (EQUALS | LIKE) STRING_LITERAL
    | IDENTIFIER (EQUALS | LIKE) STRING_LITERAL
    ;

widgetPropertyAssignment
    : STRING_LITERAL EQUALS widgetPropertyValue
    ;

widgetPropertyValue
    : STRING_LITERAL
    | NUMBER_LITERAL
    | booleanLiteral
    | NULL
    ;

describeStatement
    : DESCRIBE CONTRACT ENTITY qualifiedName (FORMAT IDENTIFIER)?   // DESCRIBE CONTRACT ENTITY Service.Entity [FORMAT mdl] (must precede DESCRIBE ENTITY)
    | DESCRIBE CONTRACT ACTION qualifiedName (FORMAT IDENTIFIER)?   // DESCRIBE CONTRACT ACTION Service.Action [FORMAT mdl]
    | DESCRIBE CONTRACT MESSAGE qualifiedName    // DESCRIBE CONTRACT MESSAGE Module.Service.MessageName
    | DESCRIBE ENTITY qualifiedName
    | DESCRIBE ASSOCIATION qualifiedName
    | DESCRIBE MICROFLOW qualifiedName NORMALIZED? (WITH HANDLES)?   // NORMALIZED folds a recombinable irreducible graph into nested ifs (Mode 3); WITH HANDLES prints each activity's alter target
    | DESCRIBE NANOFLOW qualifiedName
    | DESCRIBE RULE qualifiedName
    | DESCRIBE WORKFLOW qualifiedName
    | DESCRIBE PAGE qualifiedName
    | DESCRIBE SNIPPET qualifiedName
    | DESCRIBE BUILDING BLOCK qualifiedName
    | DESCRIBE MENU_KW qualifiedName
    | DESCRIBE LAYOUT qualifiedName
    | DESCRIBE ENUMERATION qualifiedName
    | DESCRIBE CONSTANT qualifiedName
    | DESCRIBE JAVA ACTION qualifiedName
    | DESCRIBE JAVASCRIPT ACTION qualifiedName
    | DESCRIBE MODULE identifierOrKeyword (WITH ALL)?  // DESCRIBE MODULE Name [WITH ALL] - optionally include all objects
    | DESCRIBE MODULE ROLE qualifiedName        // DESCRIBE MODULE ROLE Module.RoleName
    | DESCRIBE USER ROLE (STRING_LITERAL | identifierOrKeyword)  // DESCRIBE USER ROLE 'Administrator' | Administrator
    | DESCRIBE DEMO USER STRING_LITERAL          // DESCRIBE DEMO USER 'demo_admin'
    | DESCRIBE consumedODataServiceKw qualifiedName   // DESCRIBE CONSUMED ODATA SERVICE Module.ServiceName
    | DESCRIBE publishedODataServiceKw qualifiedName  // DESCRIBE PUBLISHED ODATA SERVICE Module.ServiceName
    | DESCRIBE EXTERNAL ENTITY qualifiedName    // DESCRIBE EXTERNAL ENTITY Module.EntityName
    | DESCRIBE NAVIGATION (qualifiedName | IDENTIFIER)?  // DESCRIBE NAVIGATION [profile]
    // R6: `widget type` names the kind of widget, so it cannot be read as a
    // widget on a page. `describe widget <name>` is a deprecated alias.
    | DESCRIBE WIDGET TYPE (identifierOrKeyword | STRING_LITERAL)              // DESCRIBE WIDGET TYPE combobox | DESCRIBE WIDGET TYPE 'com.mendix…'
    | DESCRIBE WIDGET identifierOrKeyword /* @alias MDL-DEPR095 */             // DESCRIBE WIDGET combobox
    | DESCRIBE WIDGET STRING_LITERAL /* @alias MDL-DEPR095 */                  // …by full widget id, which contains dots
    | DESCRIBE STYLING ON (PAGE | SNIPPET) qualifiedName (WIDGET IDENTIFIER)?  // DESCRIBE STYLING ON PAGE Module.Page [WIDGET name]
    | DESCRIBE CATALOG DOT (catalogTableName)  // DESCRIBE CATALOG.ENTITIES
    | DESCRIBE BUSINESS EVENT SERVICE qualifiedName  // DESCRIBE BUSINESS EVENT SERVICE Module.Name
    | DESCRIBE taskQueueKw qualifiedName               // DESCRIBE TASK QUEUE Module.Name
    | DESCRIBE SCHEDULED EVENT qualifiedName           // DESCRIBE SCHEDULED EVENT Module.Name
    | DESCRIBE REGULAR EXPRESSION qualifiedName        // DESCRIBE REGULAR EXPRESSION Module.Name
    | DESCRIBE DATABASE CONNECTION qualifiedName       // DESCRIBE DATABASE CONNECTION Module.Name
    | DESCRIBE SETTINGS (CONFIGURATION STRING_LITERAL)?  // DESCRIBE SETTINGS [CONFIGURATION 'Default']
    | DESCRIBE FRAGMENT FROM PAGE qualifiedName WIDGET identifierOrKeyword     // DESCRIBE FRAGMENT FROM PAGE Module.Page WIDGET name
    | DESCRIBE FRAGMENT FROM SNIPPET qualifiedName WIDGET identifierOrKeyword  // DESCRIBE FRAGMENT FROM SNIPPET Module.Snippet WIDGET name
    | DESCRIBE IMAGE COLLECTION qualifiedName           // DESCRIBE IMAGE COLLECTION Module.Name
    | DESCRIBE ICON COLLECTION qualifiedName            // DESCRIBE ICON COLLECTION Module.Name
    | DESCRIBE GLYPH (NUMBER_LITERAL | STRING_LITERAL)  // DESCRIBE GLYPH 57350 | DESCRIBE GLYPH 'star' 
    | DESCRIBE aiModelKw qualifiedName                  // DESCRIBE AI MODEL Module.Name (agent-editor)
    | DESCRIBE AGENT qualifiedName                      // DESCRIBE AGENT Module.Name (agent-editor)
    | DESCRIBE KNOWLEDGE BASE qualifiedName             // DESCRIBE KNOWLEDGE BASE Module.Name
    | DESCRIBE CONSUMED MCP SERVICE qualifiedName       // DESCRIBE CONSUMED MCP SERVICE Module.Name
    | DESCRIBE PUBLISHED MCP SERVICE qualifiedName      // DESCRIBE PUBLISHED MCP SERVICE Module.Name
    | DESCRIBE JSON STRUCTURE qualifiedName              // DESCRIBE JSON STRUCTURE Module.Name
    | DESCRIBE MESSAGE DEFINITION COLLECTION qualifiedName
    | DESCRIBE MESSAGE DEFINITION qualifiedName
    | DESCRIBE IMPORT MAPPING qualifiedName             // DESCRIBE IMPORT MAPPING Module.Name
    | DESCRIBE EXPORT MAPPING qualifiedName             // DESCRIBE EXPORT MAPPING Module.Name
    | DESCRIBE consumedRestServiceKw qualifiedName      // DESCRIBE CONSUMED REST SERVICE Module.Name
    | DESCRIBE CONTRACT OPERATION FROM OPENAPI STRING_LITERAL   // DESCRIBE CONTRACT OPERATION FROM OPENAPI '/path/to/spec.json'
    | DESCRIBE PUBLISHED REST SERVICE qualifiedName    // DESCRIBE PUBLISHED REST SERVICE Module.Name
    | DESCRIBE DATA TRANSFORMER qualifiedName          // DESCRIBE DATA TRANSFORMER Module.Name
    | DESCRIBE FRAGMENT identifierOrKeyword            // DESCRIBE FRAGMENT Name
    | DESCRIBE JAR DEPENDENCY (qualifiedName | IDENTIFIER) STRING_LITERAL   // DESCRIBE JAR DEPENDENCY ModuleName 'group:artifact'
    | DESCRIBE TRANSLATIONS (IN identifierOrKeyword | WITHOUT MARKETPLACE)? FOR identifierOrKeyword   // DESCRIBE TRANSLATIONS [IN Module | WITHOUT MARKETPLACE] FOR nl_NL
    // R6: the single-thing reports that were `show` forms. Each builds the
    // same statement as its `show` spelling (MDL-DEPR090).
    | DESCRIBE APP SECURITY                                                     // DESCRIBE APP SECURITY
    | DESCRIBE SECURITY MATRIX (IN (qualifiedName | IDENTIFIER))?              // DESCRIBE SECURITY MATRIX [IN Module]
    | DESCRIBE STRUCTURE (DEPTH NUMBER_LITERAL)? (IN (qualifiedName | IDENTIFIER))? ALL?  // DESCRIBE STRUCTURE [DEPTH n] [IN Module] [ALL]
    | DESCRIBE CONTEXT OF qualifiedName (DEPTH NUMBER_LITERAL)?                // DESCRIBE CONTEXT OF Module.Name [DEPTH n]
    | DESCRIBE qualifiedName    // DESCRIBE Module.Name — type auto-detected at execution time (must be LAST so all typed forms above win)
    ;

catalogSelectQuery
    : SELECT (DISTINCT | ALL)? selectList
      FROM CATALOG DOT catalogTableName (AS? IDENTIFIER)?
      (catalogJoinClause)*
      (WHERE whereExpr=expression)?
      (GROUP_BY groupByList (HAVING havingExpr=expression)?)?
      (ORDER_BY orderByList)?
      (LIMIT NUMBER_LITERAL)?
      (OFFSET NUMBER_LITERAL)?
    ;

catalogJoinClause
    : joinType? JOIN CATALOG DOT catalogTableName (AS? IDENTIFIER)? (ON expression)?
    ;

// Table names for catalog can be keywords or identifiers
catalogTableName
    : MODULES
    | ENTITIES
    | ASSOCIATIONS  // keyword token — must be listed explicitly
    | MICROFLOWS
    | NANOFLOWS
    | RULES
    | PAGES
    | SNIPPETS
    | LAYOUTS
    | ENUMERATIONS
    | ATTRIBUTES
    | WIDGETS
    | WORKFLOWS
    | CONSTANTS     // keyword token — must be listed explicitly
    | OBJECTS       // keyword token — must be listed explicitly
    | COMMUNITIES   // keyword token (SHOW COMMUNITIES) — must be listed explicitly for CATALOG.COMMUNITIES
    | QUEUES        // keyword token (SHOW QUEUES) — must be listed explicitly for CATALOG.QUEUES
    | FOLDERS       // keyword token (LIST FOLDERS) — must be listed explicitly for CATALOG.FOLDERS (ako/mxcli#1436)
    | AGENTS        // keyword token (SHOW AGENTS) — CATALOG.AGENTS parsed to nothing until TestEveryCatalogTableParses caught it
    | ANNOTATIONS   // keyword token (LIST ANNOTATIONS) — for CATALOG.ANNOTATIONS (ako/mxcli#1300)
    | SOURCE_KW     // For CATALOG.SOURCE FTS table
    | ODATA         // For CATALOG.ODATA_CLIENTS and CATALOG.ODATA_SERVICES (via IDENTIFIER)
    | IDENTIFIER    // For tables like activities, xpath_expressions, projects, snapshots, refs, strings, odata_clients, odata_services, java_actions
    ;

// =============================================================================
// OQL QUERY (Object Query Language)
// =============================================================================

/**
 * OQL (Object Query Language) query for retrieving data.
 */
oqlQuery
    : oqlQueryTerm (UNION ALL? oqlQueryTerm)*
    ;

oqlQueryTerm
    : selectClause fromClause? whereClause? groupByClause? havingClause?
      orderByClause? limitOffsetClause?
    | fromClause whereClause? groupByClause? havingClause?
      selectClause orderByClause? limitOffsetClause?
    ;

selectClause
    : SELECT (DISTINCT | ALL)? selectList
    ;

selectList
    : STAR
    | selectItem (COMMA selectItem)*
    ;

selectItem
    : expression (AS selectAlias)?
    | aggregateFunction (AS selectAlias)?
    ;

// Allow keywords as aliases in SELECT, and quoted identifiers.
//
// QUOTED_IDENTIFIER is not cosmetic here: an OQL reserved word (the date-part
// keywords Year/Month/Quarter/Day/...) MUST be quoted to survive MxBuild, which
// otherwise rejects the view with CE0174 "The 'Month' part is incomplete or
// incorrect". OQL takes double-quoted identifiers exactly as SQL does, and the
// stored query keeps the raw source text, so the quotes reach MxBuild intact.
//
// A view entity's declared attribute name IS its select alias — they have to
// match — so without this a view simply could not have a column called `Month`:
// unquoted the alias is CE0174, and quoted it did not parse. The `keyword`
// alternative above covers MDL's own keywords, which is a different grammar and
// does not help with OQL's. (ako/mxcli-captrack)
selectAlias
    : IDENTIFIER
    | QUOTED_IDENTIFIER
    | keyword
    ;

fromClause
    : FROM tableReference (joinClause)*
    ;

tableReference
    : qualifiedName oqlSourceAlias?
    | LPAREN oqlQuery RPAREN oqlSourceAlias?
    ;

joinClause
    : joinType? JOIN tableReference (ON expression)?
    | joinType? JOIN associationPath oqlSourceAlias?
    ;

// A source alias (FROM / JOIN / derived table).
//
// MDL's keywords are not OQL's, so a name like ROLE, STATUS or VALUE is a legal
// alias in a Mendix model, and DESCRIBE prints it verbatim — which made the
// round trip fail to parse (mendixlabs/mxcli#1174). After an explicit AS only
// an alias can follow, so any keyword is accepted there. Without AS the alias
// stays IDENTIFIER-only: `from M.Sale s left join …` must keep LEFT as the join.
oqlSourceAlias
    : AS (IDENTIFIER | keyword)
    | IDENTIFIER
    ;

// OQL association path formats:
// - Association/Entity (e.g., Shop.BillingAddress_Customer/Shop.Customer)
// - alias/Association/Entity (e.g., c/Shop.DeliveryAddress_Customer/Shop.Address)
//
// The leading alias takes a keyword for the same reason as oqlSourceAlias: it
// names a source that may itself have been declared `as Role`.
associationPath
    : (IDENTIFIER | keyword) SLASH qualifiedName SLASH qualifiedName  // alias/Association/Entity
    | qualifiedName SLASH qualifiedName                                // Association/Entity
    ;

joinType
    : LEFT OUTER?
    | RIGHT OUTER?
    | INNER
    | FULL OUTER?
    | CROSS
    ;

whereClause
    : WHERE expression
    ;

groupByClause
    : GROUP_BY expressionList
    ;

havingClause
    : HAVING expression
    ;

orderByClause
    : ORDER_BY orderByList
    ;

orderByList
    : orderByItem (COMMA orderByItem)*
    ;

orderByItem
    : expression (ASC | DESC)?
    ;

groupByList
    : expression (COMMA expression)*
    ;

limitOffsetClause
    : LIMIT NUMBER_LITERAL (OFFSET NUMBER_LITERAL)?
    | OFFSET NUMBER_LITERAL (LIMIT NUMBER_LITERAL)?
    ;

// A Mendix version as written everywhere else: 10.24 or 10.24.3. NUMBER_LITERAL
// takes one dot, so a patch arrives as DOT NUMBER_LITERAL (ako/mxcli#1367).
featureVersion
    : NUMBER_LITERAL (DOT NUMBER_LITERAL)?
    ;
