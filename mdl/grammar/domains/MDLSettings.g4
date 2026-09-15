/**
 * MDL Settings Grammar — ALTER SETTINGS, utility statements, data transformer settings,
 * expressions, common rules.
 */
parser grammar MDLSettings;

options { tokenVocab = MDLLexer; }

// =============================================================================
// ALTER SETTINGS
// =============================================================================

/**
 * ALTER SETTINGS MODEL Key = Value, ...;
 * ALTER SETTINGS CONFIGURATION 'name' Key = Value, ...;
 * ALTER SETTINGS CONSTANT 'name' VALUE 'value' [IN CONFIGURATION 'name'];
 * ALTER SETTINGS LANGUAGE Key = Value, ...;
 * ALTER SETTINGS LANGUAGE ADD [OR MODIFY] 'ar_SD' [(Key: Value, ...)];
 * ALTER SETTINGS LANGUAGE MODIFY 'ar_SD' (Key: Value, ...);
 * ALTER SETTINGS LANGUAGE REMOVE 'ar_SD';
 * ALTER SETTINGS WORKFLOWS Key = Value, ...;
 *
 * ADD/REMOVE name the ENABLED languages — the list Studio Pro shows under
 * App Settings > Languages, and the only languages a build emits anything for.
 * A language is identified by its code alone: Studio Pro's "Arabic, Sudan" is
 * derived from `ar_SD` for display and is not stored (verified against a
 * Studio Pro-authored reference on 11.13.0).
 */
alterSettingsClause
    : settingsSection ADD OR MODIFY STRING_LITERAL languageOptions?
    | settingsSection ADD STRING_LITERAL languageOptions?
    | settingsSection MODIFY STRING_LITERAL languageOptions
    | settingsSection REMOVE STRING_LITERAL
    | settingsSection settingsAssignment (COMMA settingsAssignment)*
    | CONSTANT STRING_LITERAL (VALUE settingsValue | DROP) (IN CONFIGURATION STRING_LITERAL)?
    | DROP CONSTANT STRING_LITERAL (IN CONFIGURATION STRING_LITERAL)?
    | CONFIGURATION STRING_LITERAL settingsAssignment (COMMA settingsAssignment)*
    ;

settingsSection
    : IDENTIFIER   // LANGUAGE, etc.
    | MODEL
    | WORKFLOWS
    ;

settingsAssignment
    : IDENTIFIER EQUALS settingsValue
    ;

// The optional properties of an added language, in the ( key: value ) form every
// other MDL statement uses. All five are what Texts$Language stores; omitting
// them reproduces what Studio Pro's Add Language dialog writes.
//   ( CheckCompleteness: true, CustomDateFormat: 'yyyy-MM-dd' )
languageOptions
    : LPAREN languageOption (COMMA languageOption)* RPAREN
    ;

languageOption
    : IDENTIFIER COLON settingsValue
    ;

settingsValue
    : STRING_LITERAL
    | NUMBER_LITERAL
    | booleanLiteral
    | qualifiedName
    ;

// =============================================================================
// UTILITY STATEMENTS
// =============================================================================

utilityStatement
    : connectStatement
    | disconnectStatement
    | statusStatement
    | updateStatement
    | checkStatement
    | buildStatement
    | executeScriptStatement
    | executeRuntimeStatement
    | lintStatement
    | searchStatement
    | useSessionStatement
    | introspectApiStatement
    | debugStatement
    | defineFragmentStatement
    | sqlStatement
    | importStatement
    | sessionSetStatement
    | helpStatement
    ;

/**
 * Session-level SET: stores a key/value pair in the executor settings map.
 * Distinct from the microflow-level setStatement (SET $var = expr).
 *
 * Examples:
 *   SET format = json
 *   SET format = 'table'
 *   SET debug = true
 */
sessionSetStatement
    : SET identifierOrKeyword EQUALS sessionSetValue
    ;

sessionSetValue
    : STRING_LITERAL
    | NUMBER_LITERAL
    | TRUE
    | FALSE
    | identifierOrKeyword
    ;

searchStatement
    : SEARCH STRING_LITERAL
    ;

connectStatement
    : CONNECT TO PROJECT STRING_LITERAL (BRANCH STRING_LITERAL)? TOKEN STRING_LITERAL
    | CONNECT LOCAL STRING_LITERAL
    | CONNECT RUNTIME HOST STRING_LITERAL PORT NUMBER_LITERAL (TOKEN STRING_LITERAL)?
    ;

disconnectStatement
    : DISCONNECT
    ;

statusStatement
    : STATUS
    ;

updateStatement
    : UPDATE
    | REFRESH CATALOG COMMUNITIES (RESOLUTION NUMBER_LITERAL)? FORCE? BACKGROUND?
    | REFRESH CATALOG FULL? SOURCE_KW? FORCE? BACKGROUND?
    | REFRESH
    ;

checkStatement
    : CHECK
    ;

buildStatement
    : BUILD
    ;

executeScriptStatement
    : EXECUTE SCRIPT STRING_LITERAL
    ;

executeRuntimeStatement
    : EXECUTE RUNTIME STRING_LITERAL
    ;

lintStatement
    : LINT lintTarget? (FORMAT lintFormat)?
    | SHOW LINT RULES
    ;

lintTarget
    : qualifiedName DOT STAR  // Module.* - lint all in module
    | qualifiedName           // Specific element
    | STAR                    // All
    ;

lintFormat
    : TEXT
    | JSON
    | SARIF
    ;

useSessionStatement
    : USE sessionIdList
    | USE ALL
    ;

sessionIdList
    : sessionId (COMMA sessionId)*
    ;

sessionId
    : IDENTIFIER
    | HYPHENATED_ID
    ;

introspectApiStatement
    : INTROSPECT API
    ;

debugStatement
    : DEBUG STRING_LITERAL
    ;

/**
 * SQL statements for external database connectivity.
 */
/*
 * Connection aliases, driver names and table names are user-chosen words, so
 * they take identifierOrKeyword rather than bare IDENTIFIER — `source` lexes as
 * SOURCE_KW, and it is both the most natural alias and the one used in mxcli's
 * own documentation. IMPORT FROM (below) already accepts an alias this way; the
 * SQL statements were the odd ones out, and a bare IDENTIFIER there meant
 * `SQL DISCONNECT source` did not parse at all.
 */
sqlStatement
    : SQL CONNECT identifierOrKeyword STRING_LITERAL AS identifierOrKeyword  # sqlConnect
    | SQL DISCONNECT identifierOrKeyword                            # sqlDisconnect
    | SQL CONNECTIONS                                               # sqlConnections
    | SQL identifierOrKeyword SHOW identifierOrKeyword              # sqlShowTables
    | SQL identifierOrKeyword DESCRIBE identifierOrKeyword          # sqlDescribeTable
    | SQL identifierOrKeyword GENERATE CONNECTOR INTO identifierOrKeyword
      (TABLES LPAREN identifierOrKeyword (COMMA identifierOrKeyword)* RPAREN)?
      (VIEWS LPAREN identifierOrKeyword (COMMA identifierOrKeyword)* RPAREN)?
      EXEC?                                                          # sqlGenerateConnector
    | SQL identifierOrKeyword sqlPassthrough                         # sqlQuery
    ;

sqlPassthrough
    : ~(SEMICOLON | SLASH | EOF)+
    ;

importStatement
    : IMPORT FROM identifierOrKeyword QUERY (STRING_LITERAL | DOLLAR_STRING)
      INTO qualifiedName
      MAP LPAREN importMapping (COMMA importMapping)* RPAREN
      (LINK LPAREN linkMapping (COMMA linkMapping)* RPAREN)?
      (BATCH NUMBER_LITERAL)?
      (LIMIT NUMBER_LITERAL)?                                    # importFromQuery
    ;

importMapping
    : identifierOrKeyword AS identifierOrKeyword
    ;

linkMapping
    : identifierOrKeyword TO identifierOrKeyword ON identifierOrKeyword   # linkLookup
    | identifierOrKeyword TO identifierOrKeyword                          # linkDirect
    ;

helpStatement
    : IDENTIFIER (identifierOrKeyword)*  // HELP [topic words...]
    ;

/**
 * DEFINE FRAGMENT Name [($p: datasource, $q: action)] AS { widgets }
 *
 * Optional typed parameters let a fragment bind a datasource or an action
 * handler supplied by the caller (`use fragment Name ($p: $Data, $q: microflow M)`).
 * References in the body use the bare `$p` form in a datasource/action position.
 */
defineFragmentStatement
    : DEFINE FRAGMENT identifierOrKeyword fragmentParams? AS LBRACE pageBodyV3 RBRACE
    ;

fragmentParams
    : LPAREN fragmentParam (COMMA fragmentParam)* RPAREN
    ;

fragmentParam
    : VARIABLE COLON fragmentParamType
    ;

fragmentParamType
    : DATASOURCE
    | ACTION
    ;

// =============================================================================
// EXPRESSIONS (operator precedence from lowest to highest)
// =============================================================================

expression
    : orExpression
    ;

orExpression
    : andExpression (OR andExpression)*
    ;

andExpression
    : notExpression (AND notExpression)*
    ;

notExpression
    : NOT LPAREN expression RPAREN   // not(expr) — Mendix requires parens (CE0117)
    | comparisonExpression
    ;

comparisonExpression
    : additiveExpression
      ( comparisonOperator additiveExpression
      | IS_NULL
      | IS_NOT_NULL
      | IN LPAREN (oqlQuery | expressionList) RPAREN
      | NOT? BETWEEN additiveExpression AND additiveExpression
      | NOT? LIKE additiveExpression
      | MATCH additiveExpression
      )?
    ;

comparisonOperator
    : EQUALS
    | NOT_EQUALS
    | LESS_THAN
    | LESS_THAN_OR_EQUAL
    | GREATER_THAN
    | GREATER_THAN_OR_EQUAL
    ;

additiveExpression
    : multiplicativeExpression ((PLUS | MINUS) multiplicativeExpression)*
    ;

multiplicativeExpression
    : unaryExpression ((STAR | SLASH | COLON | PERCENT | MOD | DIV) unaryExpression)*  // COLON is OQL division
    ;

unaryExpression
    : (PLUS | MINUS)? primaryExpression
    ;

primaryExpression
    : LPAREN expression RPAREN
    | LPAREN oqlQuery RPAREN          // Scalar subquery
    | EXISTS LPAREN oqlQuery RPAREN   // EXISTS / NOT EXISTS subquery
    | ifThenElseExpression            // Inline if...then...else (Mendix expression)
    | caseExpression
    | castExpression                  // CAST(expr AS type) for OQL type conversion
    | listAggregateOperation          // COUNT, SUM, etc. on lists as expressions (must be before aggregateFunction)
    | listOperation                   // HEAD, TAIL, FIND, etc. as expressions
    | aggregateFunction               // SQL aggregate functions (COUNT, SUM, AVG, etc.) for OQL
    | functionCall
    | atomicExpression
    ;

caseExpression
    : CASE
      (WHEN expression THEN expression)+
      (ELSE expression)?
      END
    ;

/** Inline if-then-else expression (Mendix expression syntax):
 *  if condition then trueExpr else falseExpr
 */
ifThenElseExpression
    : IF condition=expression THEN thenExpr=expression ELSE elseExpr=expression
    ;

/** CAST expression for OQL type conversion: CAST(expr AS type) */
castExpression
    : CAST LPAREN expression AS castDataType RPAREN
    ;

/** Data types supported by CAST in OQL */
castDataType
    : BOOLEAN_TYPE
    | DATETIME_TYPE
    | DECIMAL_TYPE
    | INTEGER_TYPE
    | LONG_TYPE
    | STRING_TYPE
    ;

aggregateFunction
    : (COUNT | SUM | AVG | MIN | MAX) LPAREN (DISTINCT? expression | STAR) RPAREN
    ;

/** Function call: built-in (`length($s)`) or qualified rule / sub-microflow */
functionCall
    : (functionName | qualifiedName) LPAREN argumentList? RPAREN
    ;

/** Function names - includes identifiers and keywords that are valid function names */
functionName
    : IDENTIFIER
    | HYPHENATED_ID
    | TRUE           // true() function
    | FALSE          // false() function
    | CONTAINS       // contains(string, substring)
    | LENGTH         // length(string)
    | TRIM           // trim(string)
    | FIND           // find(list, condition)
    | FILTER         // filter(list, condition)
    | EMPTY          // empty(value)
    | COUNT          // count(list)
    | SUM            // sum(list, attribute)
    | AVG            // avg(list, attribute)
    | MIN            // min(list, attribute)
    | MAX            // max(list, attribute)
    ;

argumentList
    : expression (COMMA expression)*
    ;

atomicExpression
    : literal
    | VARIABLE (DOT attributeName)*    // $Var or $Widget.Attribute (data source ref)
    | AT qualifiedName                 // @Module.ConstantName (constant reference)
    | qualifiedName
    | IDENTIFIER
    | MENDIX_TOKEN
    ;

expressionList
    : expression (COMMA expression)*
    ;

// =============================================================================
// COMMON RULES
// =============================================================================

/** Qualified name: Module.Entity or Module.Entity.Attribute */
qualifiedName
    : identifierOrKeyword (DOT identifierOrKeyword)*
    ;

/** An identifier that may be a keyword or a quoted name like "ComboBox" */
identifierOrKeyword
    : IDENTIFIER
    | QUOTED_IDENTIFIER
    | keyword
    ;

/** Literal values */
literal
    : STRING_LITERAL
    | NUMBER_LITERAL
    | booleanLiteral
    | NULL
    | EMPTY
    ;

arrayLiteral
    : LBRACKET (literal (COMMA literal)*)? RBRACKET
    ;

booleanLiteral
    : TRUE
    | FALSE
    ;

/**
 * CREATE [OR MODIFY|REPLACE] TRANSLATIONS [IN Module] FOR <lang> ( 'src' AS 'target', ... );
 *
 * A translation maps a user-provided name to another name, so entries use AS
 * rather than COLON — the same rule CUSTOM NAME map follows
 * (.claude/skills/design-mdl-syntax.md, "colon vs as").
 *
 * The thing that exists is the LANGUAGE: bare CREATE refuses when it already has
 * translations, OR MODIFY merges, OR REPLACE makes the file authoritative.
 *
 * The entry list may be EMPTY. Under OR REPLACE that is the statement's own
 * semantics taken to the limit — the file names nothing, so every translation in
 * scope is removed — and it is the only way to take a language's translations
 * out of the model, which `alter settings LANGUAGE remove` points at. It parsed
 * as an error before, so the documented way to do it did not exist.
 * See docs/11-proposals/PROPOSAL_translations.md.
 */
createTranslationsStatement
    : TRANSLATIONS (IN identifierOrKeyword)? FOR identifierOrKeyword
      LPAREN (translationEntry (COMMA translationEntry)* COMMA?)? RPAREN
    ;

translationEntry
    : STRING_LITERAL AS STRING_LITERAL
    ;

/** Documentation comment */
docComment
    : DOC_COMMENT
    ;

/** Annotation: @Name or @Name(params) or @Name value */
annotation
    : AT annotationName (LPAREN annotationParams RPAREN | annotationValue)?
    ;

annotationName
    : IDENTIFIER
    | POSITION
    | COMMENT
    | ICON
    | FOLDER
    | REQUIRED
    | NULLABLE
    | CAPTION
    | ANNOTATION
    | ANCHOR
    | MERGE       // @merge(x, y) — the layout hint predates the `merge` keyword
    ;

annotationParams
    : annotationParam (COMMA annotationParam)*
    ;

annotationParam
    : annotationParamName COLON (annotationValue | annotationParenValue)   // Named parameter
    | annotationValue                                                      // Positional parameter
    ;

// Keywords that are permitted as annotation parameter keys.
annotationParamName
    : IDENTIFIER
    | FROM
    | TO
    | TRUE
    | FALSE
    | TAIL        // @anchor(... tail: (...))
    // @annotation(id: n1, text: '…', position: (x, y), size: (w, h)) — #1077.
    // `id` and `size` are already IDENTIFIER; these two are lexer keywords, and
    // a keyword key does NOT fail to parse — annotationParam falls through to
    // its positional alternative, so the parameter is accepted and silently
    // means nothing. Anything added here must be listed, not assumed.
    | POSITION    // the note's own place on the canvas, distinct from the
                  // @position of the activity it documents
    | TEXT
    ;

annotationValue
    : literal
    | anchorSide       // top | right | bottom | left (must come before expression)
    | expression
    | qualifiedName
    ;

anchorSide
    : TOP
    | RIGHT
    | BOTTOM
    | LEFT
    ;

// A parenthesised value for nested annotation parameters like:
//   @anchor(true: (from: right, to: left), false: (from: bottom, to: left))
annotationParenValue
    : LPAREN annotationParams RPAREN
    ;

/** Keywords that can be used as identifiers in certain contexts.
 *  Every word-type lexer token must appear here so that user-defined names
 *  that happen to match a keyword can still be parsed.
 *  Maintain alphabetical order within each group for easy auditing.
 */
keyword
    // DDL / DML
    : QUEUE | QUEUES | ADD | ALTER | BATCH | BROWSER | CHANGE | CLOSE | COMMIT | CREATE | DECLARE | DELETE | DESCRIBE
    | DOWNLOAD | DROP | EXECUTE | EXPORT | GENERATE | IMPORT | INSERT | INTO | MODIFY | MOVE | REFRESH
    | SYNCHRONIZE | UNSYNCHRONIZED
    | REMOVE | RENAME | REPLACE | RETRIEVE | RETURN | ROLLBACK | SET | UPDATE

    // Entity / Domain model
    | ASSOCIATION | ASSOCIATIONS | CALCULATED | CONSTANT | CONSTANTS | ENTITY | ENTITIES
    | ENUMERATION | ENUMERATIONS | GENERALIZATION | EXTENDS | INDEX | PERSISTENT
    | NON_PERSISTENT | REFERENCE | REFERENCE_SET | STORAGE | TABLE | UNIQUE
    | CASCADE | PREVENT | RESTRICT | ERROR_MESSAGE | DELETE_BEHAVIOR | DELETE_AND_REFERENCES
    | DELETE_BUT_KEEP_REFERENCES | DELETE_IF_NO_REFERENCES
    | ALLOW_CREATE_CHANGE_LOCALLY
    | CHANGED | CREATED

    // Types
    | AUTONUMBER_TYPE | AUTOOWNER_TYPE | AUTOCHANGEDBY_TYPE | AUTOCREATEDDATE_TYPE | AUTOCHANGEDDATE_TYPE
    | BINARY_TYPE | BOOLEAN_TYPE | CURRENCY_TYPE | DATE_TYPE
    | DATETIME_TYPE | DECIMAL_TYPE | ENUM_TYPE | FLOAT_TYPE | HASHEDSTRING_TYPE
    | INTEGER_TYPE | LONG_TYPE | STRING_TYPE | STRINGTEMPLATE_TYPE

    // Module / project structure
    | ACTIONS | ARTIFACT | COLLECTION | DEPENDENCIES | DEPENDENCY | EXCLUSION | FOLDER | FOLDERS
    | INCLUDED | JAR | LAYOUT | LAYOUTS | LOCAL | MODEL | MODELS | MODULE | MODULES
    | PAGE | PAGES | PROJECT | SNIPPET | SNIPPETS
    | BUILDING | BLOCK | BLOCKS
    | STORE | STRUCTURE | STRUCTURES | VIEW

    // Agent editor
    | AGENT | AGENTS | KNOWLEDGE | BASES | CONSUMED | MCP | TOOL

    // Workflow multi-user task completion — words, so still usable as names
    | DECIDE | CONSENSUS | MAJORITY | MORE_KW | THAN | HALF | MOST | CHOSEN | THRESHOLD
    | PERCENT_KW | VOTES | VETO | FALLBACK | PARTICIPANTS | AWAIT

    // Microflow / Nanoflow
    | MICROFLOW | MICROFLOWS | NANOFLOW | NANOFLOWS
    | BEGIN | END | IF | ELSE | ELSIF | ELSEIF | THEN | WHILE | LOOP
    | BREAK | CONTINUE | THROW | RAISE | CASE | WHEN
    | CALL | LOG | TRACE | WITH | FOR | TO | OF | RETURNING | RETURNS
    | NOTHING | EXPRESSION | JAVASCRIPT
    | MERGE
    | NORMALIZED

    // Query / SQL
    | SELECT | FROM | WHERE | JOIN | LEFT | RIGHT | INNER | OUTER | FULL | CROSS
    | ORDER_BY | GROUP_BY | SORT_BY | HAVING | LIMIT | OFFSET | FIRST | AS | ON
    | AND | OR | NOT | NULL | IN | LIKE | BETWEEN | TRUE | FALSE
    | COUNT | SUM | AVG | MIN | MAX | DISTINCT | ALL
    | ASC | DESC | UNION | INTERSECT | SUBTRACT | EXISTS
    | CAST | COALESCE | TRIM | LENGTH | CONTAINS | MATCH
    | AVERAGE | MINIMUM | MAXIMUM | REDUCE | ANY | INITIAL
    | IS_NULL | IS_NOT_NULL | NOT_NULL
    | HEAD | TAIL | FIND | SORT | EMPTY
    | LIST_OF | LIST_KW | EQUALS_OP

    // Control flow / microflow statements
    | BEGIN | END | IF | ELSE | ELSIF | THEN | WHILE | LOOP
    | DECLARE | SET | CHANGE | RETRIEVE | DELETE | COMMIT | RETURN
    | CALL | LOG | WITH | FOR | TO | OF
    | DESCRIBE | CLOSE | REPLACE | UPDATE | REFRESH | EXECUTE
    | INSERT | IMPORT | EXPORT | INTO | BATCH | NOTHING

    // Database / connection
    | CONNECT | CONNECTION | CONNECTIONS | DATABASE | DISCONNECT | QUERY
    | HOST | PORT | TOKEN | RUNTIME | BRANCH | INTROSPECT
    | SCHEMA | KEY | VALUES | RECORDS

    // Entity / structure
    | INDEX | UNIQUE | REFERENCE | CASCADE | STORAGE | TABLE
    | LOCAL | PROJECT | STRUCTURE | COLLECTION | STRUCTURES
    | LAYOUT | EXPRESSION | CHANGED | CREATED | GENERATE | FOLDER

    // Widget types
    | ACTIONBUTTON | CHECKBOX | COMBOBOX | CONTAINER | CONTROLBAR
    | CUSTOMCONTAINER | CUSTOMWIDGET | DATAGRID | DATEPICKER | DATAVIEW
    | DATEFILTER | DROPDOWN | DROPDOWNFILTER | DROPDOWNSORT | DYNAMICTEXT
    | FILEINPUT | GALLERY | GROUPBOX | IMAGE | IMAGEINPUT
    | INPUTREFERENCESETSELECTOR | LAYOUTGRID | LINKBUTTON | LISTVIEW
    | NAVIGATIONLIST | NUMBERFILTER | PLACEHOLDER | PLUGGABLEWIDGET
    | RADIOBUTTONS | REFERENCESELECTOR | SEARCHBAR | SNIPPETCALL
    | STATICIMAGE | STATICTEXT | DYNAMICIMAGE | TEXTAREA | TEXTBOX | TEXTFILTER
    | TABCONTAINER | TABPAGE | WIDGET | WIDGETS
    // Layout structure. SCROLLREGION is spelled `region`, an ordinary enough
    // word that leaving it out of this rule would make `region` unusable as an
    // attribute or widget name anywhere in MDL.
    | SCROLLCONTAINER | SCROLLREGION | NAVIGATIONTREE | MENUBAR
    // Object-list container keywords for pluggable widgets (#538)
    | GROUP | CUSTOMITEM | MARKER | DYNAMICMARKER | SERIES | LINE | SCALECOLOR
    | CUSTOMBUTTON | ALLOWEDFILEFORMAT
    // Dual-stack keyword (#539)
    | LEGACYDATAGRID

    // Widget properties
    | ATTR | ATTRIBUTES | ATTRIBUTE | AUTOFILL | BINDS | BUTTONSTYLE
    | CAPTION | CAPTIONPARAMS | CLASS | COLUMN | COLUMNS | CONTENT | CONTENTPARAMS
    | DATASOURCE | DEFAULT | DESIGNPROPERTIES | DESKTOPWIDTH | DISPLAY | DOCUMENTATION
    | EDITABLE | FILTER | FILTERTYPE | HEADER | FOOTER
    | ICON | GLYPH | GLYPHS | DARK | LABEL | ONCLICK | ONCHANGE | PARAMS | PASSING
    | PHONEWIDTH | TABLETWIDTH | READONLY | RENDERMODE | REQUIRED | NULLABLE
    | SELECTION | STYLE | STYLING | TABINDEX | TITLE | TOOLTIP
    | URL | POSITION | VISIBLE | WIDTH | HEIGHT | WIDGETTYPE
    | VARIABLES_KW

    // Button actions
    | CALL_MICROFLOW | CALL_NANOFLOW | CANCEL_CHANGES | CLOSE_PAGE
    | CREATE_OBJECT | DELETE_ACTION | DELETE_OBJECT | OPEN_LINK
    | SAVECHANGES | SAVE_CHANGES | SHOW_PAGE | SIGN_OUT

    // Button styles / headings
    | BUTTON | PRIMARY | DANGER | CANCEL | INFO_STYLE | WARNING_STYLE
    | H1 | H2 | H3 | H4 | H5 | H6 | PARAGRAPH | ROW

    // Security
    | ACCESS | APPLY | AUTH | AUTHENTICATION | BASIC | DEMO
    | DESCRIPTION | GRANT | GUEST | LEVEL | MANAGE | MATRIX
    | OFF | OWNER | PASSWORD | PRODUCTION | PROTOTYPE
    | REVOKE | ROLE | ROLES | SECURITY | SESSION | USER | USERNAME | USERS

    // Validation
    | CONSTRAINT | FEEDBACK | PATTERN | RANGE | REGEX | RULE | VALIDATION | WITHOUT

    // Navigation
    | FOUND | HOME | HOMES | LOGIN | MENU_KW | NAVIGATION
    // Offline synchronization. These have to stay usable as identifiers: NEVER
    // is already a value elsewhere in MDL (`editable: never` on a list view),
    // and ONLINE/SYNC/PRESERVE are plausible entity and attribute names. A new
    // keyword that is not listed here silently steals every existing use of
    // that word — which is what TestKeywordRuleCoverage exists to catch.
    | SYNC | ONLINE | NEVER | PRESERVE

    // Log levels
    | CRITICAL | DEBUG | ERROR | INFO | SUCCESS | WARNING

    // OData / REST / API
    | API | BASE | BODY | CHANNELS | CLIENT | CLIENTS | CONTRACT | OPENAPI
    | DEPRECATED | EXPOSE | EXPOSED | EXTERNAL | HEADERS | JSON
    | MAP | MAPPING | MAPPINGS | MESSAGES | METHOD | NAMESPACE_KW
    | NOT_SUPPORTED | ODATA | OAUTH | OPERATION | PAGING
    | PARAMETER | PARAMETERS | PATH | PLATFORM | PUBLISH | PUBLISHED
    | RAW | RECEIVE | REQUEST | RESOURCE | RESPONSE | REST | SEND | SERVICE | SERVICES
    | SOURCE_KW | TIMEOUT | VERSION | WEB | XML
    | FILE_KW | LINK | DYNAMIC

    // HTTP methods
    | GET | POST | PUT | PATCH

    // Workflow
    | ABORT | ACTIVITY | ANNOTATION | ANNOTATIONS | AT_KW | BOUNDARY | BY | COMPLETE_TASK
    | CONDITION | DATE | DECISION | DUE | GROUPS | INTERRUPTING | JUMP
    | LOCK | MULTI | NODE | NON | NOTIFICATION | NOTIFY
    | OPEN | OUTCOME | OUTCOMES | OVERVIEW | PARALLEL | PAUSE
    | REASON | RESTART | RETRY | SPLIT | SUBPROCESS | TARGET | TARGETING | TASK | TIMER
    | UNLOCK | UNPAUSE | WAIT | WORKFLOW | WORKFLOWS

    // Business events / settings
    | BUSINESS | CONFIGURATION | EVENT | EVENTS | HANDLER | REGULAR | EXPRESSIONS | SCHEDULED | SETTINGS | SUBSCRIBE

    // Code search / analysis
    | BACKGROUND | CALLERS | CALLEES | DEPTH | IMPACT | REFERENCES
    | SEARCH | TRANSITIVE | COMMUNITY | COMMUNITIES | RESOLUTION

    // CLI commands
    | BUILD | CATALOG | CHECK | CLEAR | COMMENT | CUSTOM_NAME_MAP
    | DESIGN | DRY | EXEC | FEATURES | ADDED | SINCE | FORCE
    | LANGUAGES | LINT | PROPERTIES | READ | RULES | RUN | SARIF | SCRIPT | TRANSLATIONS
    | SHOW | USE | STATUS | WRITE | VIA | VIEWS | TABLES

    // Sequence flow anchors (for @anchor annotation)
    | ANCHOR | TOP | BOTTOM

    // Fragment / ALTER PAGE
    | AFTER | BEFORE | DEFINE | FRAGMENT | FRAGMENTS | SLOT

    // General-purpose words (only tokens not already listed above)
    | ACTION | BLOCKING | BOTH | CONTEXT | DATA | FORMAT | ITEM | LIST
    | DEFINITION | IGNORE | MESSAGE | MOD | DIV | MULTIPLE | NONE | OBJECT | OBJECTS
    | OVERRIDABLE | ROOT
    | SINGLE | SQL | TEMPLATE | TEXT | TYPE | VALUE

    // Data transformers
    | TRANSFORM | TRANSFORMER | TRANSFORMERS | JSLT | XSLT

    // Import/Export mapping / SQL generate
    | ATTRIBUTE_NAME | CONNECTOR | EXAMPLE | MEMBER | MEMBERS | OVER | JAVA | XPATH
    ;
