/**
 * MDL Domain Model Grammar — entities, associations, enumerations, modules,
 * constants, validation rules, JSON structures, mappings, image collections,
 * index creation, data transformer.
 */
parser grammar MDLDomainModel;

options { tokenVocab = MDLLexer; }

// =============================================================================
// ENTITY / ASSOCIATION CREATION
// =============================================================================

/**
 * Creates a new entity in the domain model.
 */
// IF NOT EXISTS makes the head of a domain script re-runnable without the reach
// of CREATE OR MODIFY, which replaces the whole definition and drops any
// attribute the statement omits. The guarded form never touches an entity that
// is already there. (sudoku findings #10, #24)
createEntityStatement
    : PERSISTENT ENTITY ifNotExists? qualifiedName generalizationClause? entityBody?
    | NON_PERSISTENT ENTITY ifNotExists? qualifiedName generalizationClause? entityBody?
    | VIEW ENTITY ifNotExists? qualifiedName entityBody? AS LPAREN? oqlQuery RPAREN?  // Parentheses optional
    | EXTERNAL ENTITY ifNotExists? qualifiedName entityBody?
    | ENTITY ifNotExists? qualifiedName generalizationClause? entityBody?  // Default to persistent
    ;

generalizationClause
    : EXTENDS qualifiedName
    | GENERALIZATION qualifiedName
    ;

entityBody
    : LPAREN attributeDefinitionList? RPAREN entityOptions?
    | entityOptions
    ;

entityOptions
    : entityOption (COMMA? entityOption)*  // Allow optional commas between options
    ;

// COMMENT is deliberately absent — it was parsed and dropped. Use the `/** … */`
// doc comment before the statement, or ALTER ENTITY … SET COMMENT.
//
// EXPORT LEVEL is the microflow clause, and means what it means there: whether
// the element is usable outside a Protected (add-on) module. An absent clause
// keeps the stored level (mendixlabs/mxcli#1498).
entityOption
    : INDEX indexDefinition
    | eventHandlerDefinition
    | microflowExportLevelClause
    ;

// Entity event handler: ON BEFORE/AFTER CREATE/COMMIT/DELETE/ROLLBACK CALL Mod.Microflow($currentObject) [RAISE ERROR]
eventHandlerDefinition
    : ON eventMoment eventType CALL qualifiedName (LPAREN VARIABLE? RPAREN)? (RAISE ERROR)?
    ;

eventMoment
    : BEFORE | AFTER
    ;

eventType
    : CREATE | COMMIT | DELETE | ROLLBACK
    ;

attributeDefinitionList
    : attributeDefinition (COMMA attributeDefinition)*
    ;

/**
 * Defines an attribute within an entity.
 */
attributeDefinition
    : docComment? annotation* attributeName COLON dataType attributeConstraint*
    ;

// Allow reserved keywords as attribute names
attributeName
    : IDENTIFIER
    | QUOTED_IDENTIFIER                     // Escape any reserved word ("Range", `Order`)
    | keyword
    ;

attributeConstraint
    : NOT_NULL (constraintErrorKeyword STRING_LITERAL)?
    | NOT NULL (constraintErrorKeyword STRING_LITERAL)?
    | NULLABLE                              // explicit: clear NOT NULL (MODIFY ATTRIBUTE, Bug 12a)
    | NOT LOCALIZED                         // DateTime only: LocalizeDate = false (#1373)
    | LOCALIZED                             // DateTime only: LocalizeDate = true, the default stated (flips back on MODIFY)
    | UNIQUE (constraintErrorKeyword STRING_LITERAL)?
    | DEFAULT (literal | expression)
    | REQUIRED (constraintErrorKeyword STRING_LITERAL)?
    | CALCULATED (BY? qualifiedName)?
    | microflowExportLevelClause            // EXPORT LEVEL API | HIDDEN (#1498)
    ;

// `error message '…'` is the one spelling of a rule's user-facing message
// (R8, ako/mxcli#752); a bare `error` is the old one.
constraintErrorKeyword
    : ERROR_MESSAGE
    | ERROR /* @alias MDL-DEPR021 */
    ;

/**
 * Specifies the data type for an attribute.
 */
dataType
    : STRING_TYPE (LPAREN (NUMBER_LITERAL | IDENTIFIER) RPAREN)?
    | INTEGER_TYPE
    | LONG_TYPE
    | DECIMAL_TYPE
    | BOOLEAN_TYPE
    | DATETIME_TYPE
    | DATE_TYPE /* @alias MDL-DEPR160 */  // stored as DateTime: Mendix has no date-only type
    | AUTONUMBER_TYPE
    | AUTOOWNER_TYPE
    | AUTOCHANGEDBY_TYPE
    | AUTOCREATEDDATE_TYPE
    | AUTOCHANGEDDATE_TYPE
    | BINARY_TYPE
    | HASHEDSTRING_TYPE
    | CURRENCY_TYPE
    | FLOAT_TYPE
    | STRINGTEMPLATE_TYPE LPAREN templateContext RPAREN  // StringTemplate(Sql) etc.
    | ENTITY LESS_THAN identifierOrKeyword GREATER_THAN  // ENTITY <pEntity>; a keyword name (String) is legal in Studio Pro, #1183
    | ENUM_TYPE qualifiedName
    | ENUMERATION LPAREN qualifiedName RPAREN  // Enumeration(Module.Enum) syntax
    | LIST_OF qualifiedName
    | qualifiedName  // Entity reference type
    ;

// Template context for StringTemplate types - only SQL or Text are valid
templateContext
    : SQL
    | TEXT
    ;

// Non-list data type - used for createObjectStatement to avoid matching "CREATE LIST OF"
nonListDataType
    : STRING_TYPE (LPAREN (NUMBER_LITERAL | IDENTIFIER) RPAREN)?
    | INTEGER_TYPE
    | LONG_TYPE
    | DECIMAL_TYPE
    | BOOLEAN_TYPE
    | DATETIME_TYPE
    | DATE_TYPE /* @alias MDL-DEPR160 */  // stored as DateTime: Mendix has no date-only type
    | AUTONUMBER_TYPE
    | AUTOOWNER_TYPE
    | AUTOCHANGEDBY_TYPE
    | AUTOCREATEDDATE_TYPE
    | AUTOCHANGEDDATE_TYPE
    | BINARY_TYPE
    | HASHEDSTRING_TYPE
    | CURRENCY_TYPE
    | FLOAT_TYPE
    | ENUM_TYPE qualifiedName
    | ENUMERATION LPAREN qualifiedName RPAREN
    | qualifiedName  // Entity reference type (NOT list)
    ;

// The optional ON reads SQL-like: `INDEX idx_name ON (Col1, Col2)`. The bare
// form `INDEX idx_name (Col1, Col2)` (and anonymous `INDEX (Col1)`) still parse.
indexDefinition
    : (IDENTIFIER | QUOTED_IDENTIFIER)? ON? LPAREN indexAttributeList RPAREN
    ;

indexAttributeList
    : indexAttribute (COMMA indexAttribute)*
    ;

indexAttribute
    : indexColumnName (ASC | DESC)?  // Column name with optional sort order
    ;

// Allow keywords as index column names (same as attributeName)
indexColumnName
    : IDENTIFIER
    | QUOTED_IDENTIFIER                     // Escape any reserved word
    | keyword
    ;

createAssociationStatement
    : ASSOCIATION ifNotExists? qualifiedName
      FROM qualifiedName
      TO qualifiedName
      associationOptions?
    | ASSOCIATION ifNotExists? qualifiedName LPAREN
      FROM qualifiedName TO qualifiedName
      (COMMA associationOption)*
      RPAREN
    ;

associationOptions
    : associationOption+
    ;

associationOption
    // A clause takes no colon (R3): `type Reference`. The colon is an alias.
    : TYPE (COLON /* @alias MDL-DEPR064 */)? (REFERENCE | REFERENCE_SET)
    | OWNER (COLON /* @alias MDL-DEPR064 */)? (DEFAULT | BOTH)
    | STORAGE (COLON /* @alias MDL-DEPR064 */)? (COLUMN | TABLE)
    | DELETE_BEHAVIOR /* @alias MDL-DEPR022 */ deleteBehavior errorMessageClause?
    | onDeleteClause
    | microflowExportLevelClause            // EXPORT LEVEL API | HIDDEN (#1498)
    // R9: documentation is a `/** … */` doc comment before the statement.
    | COMMENT /* @alias MDL-DEPR100 */ STRING_LITERAL
    ;

// The SQL spelling. Mendix's three delete behaviours ARE SQL's referential
// actions, and MDL's FROM/TO already matches a foreign key's direction, so this
// reads exactly as it does in CREATE TABLE:
//
//   FROM Shop.Order TO Shop.Customer ON DELETE RESTRICT
//     -> deleting a Customer is refused while Orders reference it
//
// The older DELETE_BEHAVIOR clause above keeps working unchanged; it is the one
// that gives no clue which side it governs, which is why this exists.
onDeleteClause
    : ON DELETE referentialAction errorMessageClause?
    ;

referentialAction
    : CASCADE                                   // DeleteMeAndReferences
    | RESTRICT                                  // DeleteMeIfNoReferences
    | SET NULL                                  // DeleteMeButKeepReferences (default)
    ;

// SQL's RESTRICT raises a system error; Mendix lets the modeller write the text
// the user sees. Only meaningful with RESTRICT/PREVENT — the executor reports it
// on the others rather than dropping it silently.
errorMessageClause
    : ERROR_MESSAGE STRING_LITERAL
    ;

deleteBehavior
    : DELETE_AND_REFERENCES
    | DELETE_BUT_KEEP_REFERENCES
    | DELETE_IF_NO_REFERENCES
    | CASCADE
    | PREVENT
    ;

// =============================================================================
// ALTER ENTITY / ASSOCIATION / ENUMERATION ACTIONS
// =============================================================================

// ALTER ENTITIES [IN <module>] ADD ATTRIBUTE ... [WHERE PERSISTENT|NON-PERSISTENT]
//
// The bulk form, and deliberately narrower than alterEntityAction: an audit
// trail is added to every entity in a module at once, which is the case that
// made the single-entity form cost one statement per entity. Only ADD
// ATTRIBUTE is offered, because it is the only action here that is safe to
// aim at a set -- DROP and RENAME in bulk are destructive by a typo, and SET
// POSITION on every entity is meaningless. Same reasoning as ALTER PAGES,
// which is bulk for exactly one operation.
//
// WHERE reuses the persistence words CREATE ENTITY already uses rather than
// inventing a predicate language: audit members belong on stored entities,
// and a non-persistent helper is the thing you want to skip.
alterEntitiesStatement
    : ALTER ENTITIES (IN identifierOrKeyword)?
      alterEntitiesAction (COMMA? alterEntitiesAction)*
      (WHERE entityPersistenceFilter)?
    ;

alterEntitiesAction
    : docComment? ADD ATTRIBUTE ifNotExists? attributeDefinition
    ;

attributeKw
    : ATTRIBUTE
    | COLUMN /* @alias MDL-DEPR093 */
    ;

entityPersistenceFilter
    : PERSISTENT
    | NON_PERSISTENT
    ;

alterEntityAction
    // R6: `column` is a deprecated synonym for `attribute` (MDL-DEPR093).
    : docComment? ADD attributeKw ifNotExists? attributeDefinition
    | RENAME attributeKw attributeName TO attributeName
    // An attribute definition is always `Name: Type` (R3). The colon is
    // optional only so the old spelling keeps parsing: its ABSENCE is the alias.
    | MODIFY attributeKw attributeName COLON? /* @alias MDL-DEPR065 */ dataType attributeConstraint*
    | DROP attributeKw ifExists? attributeName
    | DROP DEFAULT ON ATTRIBUTE attributeName   // clear an attribute's default value
    | SET DOCUMENTATION STRING_LITERAL
    | SET COMMENT /* @alias MDL-DEPR135 */ STRING_LITERAL   // R9: set documentation
    | SET POSITION LPAREN NUMBER_LITERAL COMMA NUMBER_LITERAL RPAREN
    | SET microflowExportLevelClause        // SET EXPORT LEVEL API | HIDDEN (#1498)
    | SET LPAREN ALLOW_CREATE_CHANGE_LOCALLY COLON (TRUE | FALSE) RPAREN    // set ( AllowCreateChangeLocally: true )
    | SET ALLOW_CREATE_CHANGE_LOCALLY EQUALS /* @alias MDL-DEPR063 */ (TRUE | FALSE)
    | ADD INDEX ifNotExists? indexDefinition
    | DROP INDEX ifExists? indexDefinition
    | DROP INDEX ifExists? IDENTIFIER
    | ADD EVENT HANDLER ifNotExists? eventHandlerDefinition
    // An entity may carry several handlers for one event, so CALL names which
    // one to drop. Without it the drop must be unambiguous (mendixlabs/mxcli#1540).
    | DROP EVENT HANDLER ifExists? ON eventMoment eventType (CALL qualifiedName)?
    ;

// Idempotency guards for a re-runnable domain script: ADD ... IF NOT EXISTS
// skips (with a notice) when the member is already present, and DROP ... IF
// EXISTS skips when it is already gone — instead of erroring and halting the
// run. Accepted on ATTRIBUTE, EVENT HANDLER and INDEX.
//
// On a document-level CREATE it sits after the kind's keywords and before the
// name (`create page if not exists M.P …`) on every kind that names one element
// (ako/mxcli#731, ADR-0010 R1): leave an existing element untouched, create it
// otherwise. The visitor applies it once, in ExitCreateStatement, and the
// executor's dispatch honours it, so a create rule only has to accept it.
//
// EVENT HANDLER and INDEX have no other way to be re-run: a defensive
// drop-then-add fails on the drop when the member is absent, and on the add
// when it is present. INDEX is the sharper case — an unguarded re-run used to
// append a second identical index silently, which mxbuild rejects with CE0072
// "Duplicate indexes". (mxcli-todo findings #18, sudoku findings #10)
ifNotExists
    : IF NOT EXISTS
    ;

ifExists
    : IF EXISTS
    ;

alterAssociationAction
    : SET DELETE_BEHAVIOR /* @alias MDL-DEPR022 */ deleteBehavior errorMessageClause?
    | SET onDeleteClause
    | SET OWNER (DEFAULT | BOTH)
    | SET STORAGE (COLUMN | TABLE)
    | SET microflowExportLevelClause        // SET EXPORT LEVEL API | HIDDEN (#1498)
    | SET DOCUMENTATION STRING_LITERAL
    | SET COMMENT /* @alias MDL-DEPR135 */ STRING_LITERAL   // R9: set documentation
    // Line anchors: where the connector attaches to each entity box, as a
    // PERCENTAGE of the box (0..100). Both ends together — the pair is one
    // visual decision, and `from`/`to` are the association's own words for its
    // two ends. Mirrors `alter entity ... set position (x, y)`. (issue #872)
    | SET ANCHOR FROM anchorPoint TO anchorPoint
    ;

anchorPoint
    : LPAREN NUMBER_LITERAL COMMA NUMBER_LITERAL RPAREN
    ;

// ADD VALUE IF NOT EXISTS / DROP VALUE IF EXISTS are the same idempotency
// guards as on ATTRIBUTE / INDEX above, and for the same reason: without them a
// script that adds an enumeration value is not re-runnable. The unguarded ADD
// errors on the second run and `exec` STOPS THERE, leaving every later statement
// unapplied — so one already-present value silently truncates the script.
// (ako/mxcli-rest FINDINGS #60)
alterEnumerationAction
    // A value is named as `create enumeration` names it (enumValueName), so a
    // value spelled like a keyword (Sample, AI, Model) can be altered too.
    : ADD VALUE ifNotExists? enumValueName (CAPTION STRING_LITERAL)?
    | RENAME VALUE enumValueName TO enumValueName
    | MODIFY VALUE enumValueName CAPTION STRING_LITERAL
    | DROP VALUE ifExists? enumValueName
    | SET microflowExportLevelClause        // SET EXPORT LEVEL API | HIDDEN (#1498)
    | SET DOCUMENTATION STRING_LITERAL
    | SET COMMENT /* @alias MDL-DEPR135 */ STRING_LITERAL   // R9: set documentation
    ;

// =============================================================================
// MODULE CREATION
// =============================================================================

createModuleStatement
    : MODULE ifNotExists? identifierOrKeyword moduleOptions? moduleSettingsBody?
    ;

// The General tab of Studio Pro's Module settings dialog, as a property list:
// `( Version: '2.3.1', ModuleType: addon )` (ako/mxcli#1921). The visitor
// holds the key table; `alter module … set ( … )` takes the same keys.
moduleSettingsBody
    : LPAREN (moduleSettingProperty (COMMA moduleSettingProperty)* COMMA?)? RPAREN
    ;

// NUMBER_LITERAL only so `Version: 2.0` gets the visitor's message, not a
// parse error.
moduleSettingProperty
    : identifierOrKeyword COLON (STRING_LITERAL | NUMBER_LITERAL | identifierOrKeyword)
    ;

// ALTER MODULE name SET ( Version: '…', ModuleType: app | addon )
alterModuleSetStatement
    : ALTER MODULE (qualifiedName | IDENTIFIER) SET moduleSettingsBody
    ;

// =============================================================================
// ALTER MODULE — JAR DEPENDENCY MANAGEMENT
// =============================================================================

alterModuleJarDepStatement
    : ALTER MODULE (qualifiedName | IDENTIFIER) alterModuleJarDepAction+
    ;

// ADD … IF NOT EXISTS leaves a coordinate the module already declares untouched,
// as ADD ATTRIBUTE / ADD VALUE IF NOT EXISTS do, so a script that declares a
// module's Java dependency can be executed twice (ako/mxcli#2239).
alterModuleJarDepAction
    : ADD JAR DEPENDENCY ifNotExists? LPAREN jarDepProperty (COMMA jarDepProperty)* COMMA? RPAREN
    | SET JAR DEPENDENCY STRING_LITERAL VERSION STRING_LITERAL
    | SET JAR DEPENDENCY STRING_LITERAL INCLUDED booleanLiteral
    | SET JAR DEPENDENCY STRING_LITERAL ADD EXCLUSION STRING_LITERAL
    | SET JAR DEPENDENCY STRING_LITERAL DROP EXCLUSION STRING_LITERAL
    | DROP JAR DEPENDENCY STRING_LITERAL
    ;

jarDepProperty
    : identifierOrKeyword EQUALS (STRING_LITERAL | booleanLiteral)
    ;

moduleOptions
    : moduleOption+
    ;

// COMMENT is deliberately absent: Projects$Module has no Documentation
// property. A module's documentation is its domain model's, and is written as
// the statement's `/** … */` doc comment (mendixlabs/mxcli#1314).
moduleOption
    : FOLDER STRING_LITERAL
    ;

// =============================================================================
// ENUMERATION CREATION
// =============================================================================

createEnumerationStatement
    : ENUMERATION ifNotExists? qualifiedName
      LPAREN enumerationValueList RPAREN
      enumerationOptions?
    ;

enumerationValueList
    : enumerationValue (COMMA enumerationValue)*
    ;

enumerationValue
    : docComment? enumValueName (CAPTION? STRING_LITERAL)?
    ;

// Allow reserved keywords as enumeration value names.
enumValueName
    : IDENTIFIER
    | QUOTED_IDENTIFIER                                      // Escape any reserved word
    | keyword
    ;

enumerationOptions
    : enumerationOption+
    ;

// COMMENT is deliberately absent — it was parsed and dropped. Use the `/** … */`
// doc comment before the statement.
enumerationOption
    : FOLDER STRING_LITERAL                 // place the enumeration in a module folder (Bug 12b)
    | microflowExportLevelClause            // EXPORT LEVEL API | HIDDEN (#1498)
    ;

// =============================================================================
// TASK QUEUE CREATION
// =============================================================================

/**
 * CREATE [OR MODIFY] TASK QUEUE Module.Name ( Parallelism: 3, ClusterWide: true );
 *
 * Parallelism is stored by Mendix as an EXPRESSION string
 * (Queues$BasicQueueConfig.ParallelismExpression), so it accepts a number or a
 * quoted expression.
 */
createQueueStatement
    : taskQueueKw ifNotExists? qualifiedName (FOLDER STRING_LITERAL)? queueBody?
    ;

queueBody
    : LPAREN (queueProperty (COMMA queueProperty)* COMMA?)? RPAREN
    ;

queueProperty
    : identifierOrKeyword COLON (NUMBER_LITERAL | STRING_LITERAL | booleanLiteral | identifierOrKeyword)
    ;

// =============================================================================
// REGULAR EXPRESSION CREATION
// =============================================================================
//
// A named regex document. Attribute validation rules reference it by qualified
// name (DomainModels$RegExRuleInfo.RegExIdentifier), which is why it is a
// document rather than a string on the rule.

createRegularExpressionStatement
    : REGULAR EXPRESSION ifNotExists? qualifiedName (FOLDER STRING_LITERAL)? regularExpressionBody?
    ;

regularExpressionBody
    : LPAREN (regularExpressionProperty (COMMA regularExpressionProperty)* COMMA?)? RPAREN
    ;

// `Documentation: '…'` is a registered alias of the `/** … */` doc comment
// (R9) /* @alias MDL-DEPR106 */, here and in the task queue and scheduled
// event property lists; the visitor reports it by key. `ExportLevel: Public`
// is a registered alias of `ExportLevel: API` /* @alias MDL-DEPR161 */,
// reported by value (ako/mxcli#827).
regularExpressionProperty
    : identifierOrKeyword COLON (STRING_LITERAL | booleanLiteral | identifierOrKeyword)
    ;

// =============================================================================
// SCHEDULED EVENT CREATION
// =============================================================================
//
// The repeat rule is a property (Repeat: Daily) plus the fields that rule uses,
// rather than an English clause, because the eight ScheduledEvents$*Schedule
// variants differ in WHICH fields they carry — a labelled property list keeps
// the storage's own vocabulary and lets the executor reject a field that does
// not belong to the chosen repeat.

createScheduledEventStatement
    : SCHEDULED EVENT ifNotExists? qualifiedName (FOLDER STRING_LITERAL)? scheduledEventBody?
    ;

scheduledEventBody
    : LPAREN (scheduledEventProperty (COMMA scheduledEventProperty)* COMMA?)? RPAREN
    ;

scheduledEventProperty
    : identifierOrKeyword COLON (qualifiedName | NUMBER_LITERAL | STRING_LITERAL | booleanLiteral | identifierOrKeyword)
    ;

// =============================================================================
// IMAGE COLLECTION CREATION
// =============================================================================

createImageCollectionStatement
    : IMAGE COLLECTION ifNotExists? qualifiedName (FOLDER STRING_LITERAL)? imageCollectionOptions? imageCollectionBody?
    ;

// CREATE [OR MODIFY] ANNOTATION IN Module ( Caption: '…', Position: (x, y), Width: n )
//
// A domain-model annotation is the note box Studio Pro draws on the canvas. It
// has no name — Mendix stores only Caption, ExportLevel, Location and Width — so
// it is addressed by the first line of its caption, the way an unnamed DataGrid2
// column is addressed by its caption.
//
// There is no colour. A "coloured section box" is this element in Studio Pro's
// own styling; nothing about that styling is in the model.
//
// The caption may be a dollar-quoted block, because a note is usually several
// lines and an MDL string literal is single-quoted and single-line.
createAnnotationStatement
    : ANNOTATION IN identifierOrKeyword LPAREN annotationProperty (COMMA annotationProperty)* RPAREN
    ;

annotationProperty
    : CAPTION COLON (STRING_LITERAL | DOLLAR_STRING)
    | POSITION COLON LPAREN NUMBER_LITERAL COMMA NUMBER_LITERAL RPAREN
    | WIDTH COLON NUMBER_LITERAL
    ;

imageCollectionOptions
    : imageCollectionOption+
    ;

imageCollectionOption
    : EXPORT LEVEL STRING_LITERAL   // e.g. EXPORT LEVEL 'Public'
    | COMMENT /* @alias MDL-DEPR100 */ STRING_LITERAL   // R9: a `/** … */` doc comment
    ;

// The images are the collection's children, so they are in { }, each with its
// properties in ( ) (R2, ako/mxcli#754):
//
//   create image collection M.Icons { image Logo ( File: 'assets/logo.png' ) };
//
// The parenthesised list of `image X from file '…'` is the old spelling.
imageCollectionBody
    : LBRACE imageCollectionChild* RBRACE
    | LPAREN /* @alias MDL-DEPR072 */ imageCollectionItem (COMMA imageCollectionItem)* RPAREN
    ;

imageCollectionChild
    : IMAGE imageName LPAREN imageProperty (COMMA imageProperty)* COMMA? RPAREN   // image Logo ( File: 'logo.png' )
    ;

// File: '<path>' reads the image from a file; Data: '<base64>' carries the
// bytes in the script, which is what describe writes (ako/mxcli#707), with
// Format: <png|jpg|gif|svg|bmp|webp> when the bytes do not say it themselves.
imageProperty
    : identifierOrKeyword COLON STRING_LITERAL
    | identifierOrKeyword COLON identifierOrKeyword
    ;

imageCollectionItem
    : IMAGE imageName FROM FILE_KW path=STRING_LITERAL   // IMAGE MyIcon FROM FILE '/path/to/file.png'
    ;

imageName
    : IDENTIFIER
    | QUOTED_IDENTIFIER
    | keyword
    ;

// =============================================================================
// JSON STRUCTURE CREATION
// =============================================================================

createJsonStructureStatement
    : JSON STRUCTURE ifNotExists? qualifiedName (FOLDER STRING_LITERAL)? (COMMENT /* @alias MDL-DEPR100 */ STRING_LITERAL)? jsonSampleKw (STRING_LITERAL | DOLLAR_STRING)
      (CUSTOM_NAME_MAP LPAREN customNameMapping (COMMA customNameMapping)* RPAREN)?
    ;

// R10: the example JSON a structure is derived from is its sample; `snippet`
// is a page document type.
jsonSampleKw
    : SAMPLE
    | SNIPPET /* @alias MDL-DEPR132 */
    ;

/**
 * An entry in CUSTOM NAME MAP. Two shapes, both mapping a name to a name (hence
 * `as`, per the colon-vs-as rule in design-mdl-syntax):
 *
 *   'data' as 'Records'            -- the element reached by JSON key `data`
 *   item of 'data' as 'Record'     -- the ITEM element of the array at `data`
 *
 * An array's item element has no JSON key of its own — it is the anonymous
 * `[…]` entry — so the plain form cannot reach it and its name was previously
 * derived and unspellable (ako/mxcli#272). `item of` addresses it by the key of
 * the array that contains it.
 *
 * The two are independent on purpose: naming the item does not require renaming
 * the array, so adding one is a one-line diff.
 *
 * `item of 'Root'` names the item of a ROOT-level array, which has no key. Root
 * is the fixed exposed name Studio Pro gives the root element (22 of 22
 * measured), and a root array has no keys of its own, so there is nothing for it
 * to collide with.
 */
customNameMapping
    : STRING_LITERAL AS STRING_LITERAL              // 'jsonKey' AS 'CustomName'
    | ITEM OF STRING_LITERAL AS STRING_LITERAL      // ITEM OF 'jsonKey' AS 'CustomName'
    ;

// =============================================================================
// MESSAGE DEFINITION COLLECTION
// =============================================================================

/**
 * CREATE [OR MODIFY] MESSAGE DEFINITION COLLECTION Module.Name
 *   FOLDER 'Private/Messages'
 * {
 *   definition Order for Sales.Order as 'Orders' {
 *     OrderId,
 *     Sales.Order_Line/Sales.Line as 'Lines' { Sku, Quantity }
 *   }
 * };
 *
 * A message definition is a SELECTION OVER THE DOMAIN MODEL — every element
 * names an entity, an attribute or an association — which is what makes it
 * authorable where an XML schema or a WSDL is not (they hold an imported file).
 * It is the source for 74 of the 327 mappings in the demo corpus, and the only
 * one of the four a script could not create.
 *
 * A collection holds one or more definitions; 28 of 36 real collections hold
 * exactly one, but the reference a mapping uses is three parts either way
 * (Module.Collection.Definition), so the collection is never implicit.
 */
createMessageDefinitionCollectionStatement
    : MESSAGE DEFINITION COLLECTION ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      ( LBRACE messageDefinitionDef (COMMA? messageDefinitionDef)* COMMA? RBRACE
      | LPAREN /* @alias MDL-DEPR073 */ messageDefinitionDef (COMMA messageDefinitionDef)* COMMA? RPAREN
      )
    ;

/**
 * CREATE [OR MODIFY] MESSAGE DEFINITION Module.Name
 *   [FOLDER 'path']
 *   FOR Module.Entity [AS 'Exposed'] { members };
 *
 * One message definition as its own document — a
 * MessageDefinitions$MessageDefinition2, which is how Mendix 11.15 stores them
 * (ako/mxcli#987). It is the collection's `definition` entry promoted to a
 * statement, with the same member tree; a mapping references it as
 * Module.Name. Below 11.15 the executor refuses it in favour of the collection.
 */
createMessageDefinitionStatement
    : MESSAGE DEFINITION ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      FOR qualifiedName messageExposedName? messageMemberTree
    ;

/**
 * `definition <Name> for <Module.Entity> [as '<ExposedName>'] { members }`
 *
 * The definition's Name and its root element's exposed name are independent —
 * measured, 19 of 56 definitions are named something other than their entity.
 */
messageDefinitionDef
    : DEFINITION identifierOrKeyword FOR qualifiedName messageExposedName? messageMemberTree
    ;

// A member tree is children, so it is in { }, as in an import or export
// mapping (R2, ako/mxcli#754). The parenthesised tree is the old spelling. An
// association may select no member of its target.
messageMemberTree
    : LBRACE (messageMember (COMMA messageMember)* COMMA?)? RBRACE
    | LPAREN /* @alias MDL-DEPR073 */ messageMember (COMMA messageMember)* COMMA? RPAREN
    ;

/**
 * A member is an ATTRIBUTE (a bare name) or an ASSOCIATION (Assoc/Module.Entity
 * with its own member list) — the same discriminator import and export mappings
 * use, so there is nothing new to learn.
 *
 * The association's TARGET entity is spelled out rather than inferred, and that
 * is load-bearing: MaxOccurs is not a function of the association's type (all
 * 927 resolvable ones in the corpus are `Reference`, yet 526 store 1 and 401
 * store -1). It tracks the DIRECTION of traversal — holder is the FROM entity
 * gives 1, holder is the TO entity gives -1, with zero counter-examples. Naming
 * the target makes the direction explicit in the source text instead of
 * something a reader has to work out.
 */
messageMember
    : qualifiedName SLASH qualifiedName messageExposedName? messageMemberTree   // association
    | identifierOrKeyword messageExposedName? messageExample?      // attribute
    ;

// `example 'text'` sets the element's Example — author-set sample text. Only a
// value member carries one in any document measured.
messageExample
    : EXAMPLE STRING_LITERAL
    ;

// `as 'Name'` sets ExposedName. A name mapped to a name takes `as`, not `:`.
messageExposedName
    : AS STRING_LITERAL
    ;

/**
 * ALTER MESSAGE DEFINITION COLLECTION Module.Name <op>;
 *
 * Definitions within a collection. Whole-document CREATE OR MODIFY is not
 * enough on its own: definitions nest to depth 7 in the corpus, so restating
 * one to add a leaf is the diff-unfriendliness ADR-0003 argues against.
 */
alterMessageDefinitionCollectionStatement
    : ALTER MESSAGE DEFINITION COLLECTION qualifiedName alterMessageCollectionOperation
    ;

alterMessageCollectionOperation
    : ADD DEFINITION (IF NOT EXISTS)? identifierOrKeyword FOR qualifiedName messageExposedName? messageMemberTree
    | DROP DEFINITION (IF EXISTS)? identifierOrKeyword
    | RENAME DEFINITION identifierOrKeyword TO identifierOrKeyword
    ;

/**
 * ALTER MESSAGE DEFINITION Module.Collection.Definition <op>;
 * ALTER MESSAGE DEFINITION Module.Name <op>;      -- an 11.15 document
 *
 * Members within one definition, addressed by the SAME three-part reference
 * `WITH MESSAGE DEFINITION` takes, so the two cannot drift apart.
 *
 * `SET member X AS 'Name'` changes only the element's ExposedName. It is not
 * RENAME: ALTER ENTITY's RENAME ATTRIBUTE renames the attribute in the model and
 * rewrites every reference to it, and borrowing the verb here would promise
 * something far larger than this does.
 *
 * `IN <path>` reaches a nested member, written in exposed names. `/` is not used
 * as the path separator because it already means "association to entity" inside
 * a member.
 */
alterMessageDefinitionStatement
    : ALTER MESSAGE DEFINITION qualifiedName alterMessageDefinitionOperation
    ;

alterMessageDefinitionOperation
    : ADD MEMBER (IF NOT EXISTS)? messageMember (IN messageMemberPath)?
    | DROP MEMBER (IF EXISTS)? identifierOrKeyword (IN messageMemberPath)?
    | SET MEMBER identifierOrKeyword (IN messageMemberPath)? messageExposedName
    ;

messageMemberPath
    : identifierOrKeyword (SLASH identifierOrKeyword)*
    ;

// =============================================================================
// IMPORT / EXPORT MAPPING CREATION
// =============================================================================

/**
 * CREATE IMPORT MAPPING Module.Name
 *   FOLDER 'Private/Import mappings'
 *   WITH JSON STRUCTURE Module.JsonStructure
 * {
 *   CREATE Module.Entity {
 *     PetId = id KEY,
 *     Name = name,
 *   }
 * };
 */
createImportMappingStatement
    : IMPORT MAPPING ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      importMappingWithClause?
      importMappingParameterClause?
      LBRACE importMappingRootElement RBRACE
    ;

/**
 * The mapping's INPUT object (#265). An import mapping may take an object as a
 * parameter, which its custom handlers then bind via `Param: parameter`:
 *
 *   create import mapping M.IM_Response
 *     with json structure M.JSON_Response
 *     parameter GenAICommons.ChunkCollection
 *   { ... }
 *
 * Stored as ParameterType, a DataTypes$ObjectType naming the entity; an
 * unparameterised mapping stores the DataTypes$UnknownType marker instead.
 *
 * Import only: an export mapping's parameter IS its root object, and Studio Pro
 * writes no ParameterType at all on one (0 of 127 measured).
 */
importMappingParameterClause
    : PARAMETER qualifiedName
    ;

importMappingWithClause
    // ROOT selects the schema element the mapping STARTS at, when that is not
    // the structure's own root — the shape Studio Pro produces when you pick a
    // node deeper in the payload (#267). The path is written in member names and
    // steps through arrays implicitly: `root choices/message` reaches
    // "(Object)|choices|(Object)|message".
    : WITH JSON STRUCTURE qualifiedName (ROOT jsonMemberPath)?
    | WITH XML SCHEMA qualifiedName
    // Module.Collection.Definition — the definitions live inside a collection
    // document, so the reference is three parts. qualifiedName already accepts
    // any number of them.
    | WITH MESSAGE DEFINITION qualifiedName
    ;

importMappingRootElement
    : importMappingObjectHandling qualifiedName mappingCustomHandler? mappingHandlingBackup?
      LBRACE importMappingChild (COMMA importMappingChild)* RBRACE
    ;

/**
 * What happens when the object is NOT found (or, for `create`, when one already
 * exists). Mendix stores this as ObjectHandlingBackup, whose values are
 * {Create, Error, Ignore} — `find` on its own does not name one, and mxcli
 * refuses it rather than choosing (#261).
 *
 *   find Module.Entity or create { ... }     -- the old `find or create`
 *   find Module.Entity or ignore { ... }
 *   find Module.Entity or error  { ... }
 *   create Module.Entity or error { ... }
 *
 * OVERRIDABLE sets ObjectHandlingBackupAllowOverride, which lets the caller
 * choose at runtime.
 */
mappingHandlingBackup
    : OR (CREATE | ERROR | IGNORE) OVERRIDABLE?
    ;

/**
 * `by Module.Microflow ( Param: source, ... )` — a microflow resolves the object
 * instead of Create/Find. Stored as ObjectHandling "Custom" with the call on
 * CustomHandlerCall; "find X by MF(...)" is read as "find the object by calling
 * this microflow".
 *
 * A parameter's source is one of:
 *   parent          the enclosing mapped object
 *   parameter       the mapping's own input object (see PARAMETER on the mapping)
 *   parent(2)       an ancestor N levels up (export mappings)
 *   a/b/c           a value from the payload, addressed like any other member
 */
mappingCustomHandler
    : BY qualifiedName LPAREN (mappingCallParameter (COMMA mappingCallParameter)*)? RPAREN
    ;

mappingCallParameter
    : identifierOrKeyword COLON PARAMETER
    | identifierOrKeyword COLON identifierOrKeyword LPAREN NUMBER_LITERAL RPAREN
    | identifierOrKeyword COLON jsonMemberPath
    ;

/**
 * An OBJECT element's member takes a PATH, not a single name (#260 item 1).
 * Studio Pro routinely maps an object several levels down with nothing mapped in
 * between — `= meta/pagination` — and #927 gave that to value elements only, so
 * describing such a mapping produced output its own grammar rejected.
 *
 * The path may step through an array; the item level is generated, so the
 * markers Mendix stores are never written here (`items/price`, not
 * `items/(Object)/price`).
 */
importMappingChild
    : importMappingObjectHandling qualifiedName SLASH qualifiedName mappingCustomHandler? mappingHandlingBackup? EQUALS jsonMemberPath
      LBRACE importMappingChild (COMMA importMappingChild)* RBRACE       // nested object with children
    | importMappingObjectHandling qualifiedName SLASH qualifiedName mappingCustomHandler? mappingHandlingBackup? EQUALS jsonMemberPath  // leaf object
    // An object element with an ENTITY and NO association — the same object
    // reached again rather than an associated one, which is what a custom
    // handler resolving the mapping's own entity produces. Studio Pro stores
    // Entity set and Association empty; DESCRIBE used to emit `./Module.Entity`,
    // which the grammar never accepted (#260 item 3).
    | importMappingObjectHandling qualifiedName mappingCustomHandler? mappingHandlingBackup? EQUALS jsonMemberPath
      LBRACE importMappingChild (COMMA importMappingChild)* RBRACE
    | importMappingObjectHandling qualifiedName mappingCustomHandler? mappingHandlingBackup? EQUALS jsonMemberPath
    | identifierOrKeyword EQUALS qualifiedName LPAREN jsonMemberPath RPAREN  // value transform: Attr = Module.MF(jsonField)
    | identifierOrKeyword EQUALS jsonMemberPath KEY?                         // value: Attr = a/b/c [KEY]
    ;

/**
 * A JSON member, addressed from the enclosing object element. A single name is
 * a direct child; `a/b/c` reaches a leaf several levels down WITHOUT an entity
 * for the levels in between — the shape Studio Pro produces when you tick a
 * nested leaf without ticking its parents.
 *
 * Stored as Mendix's own pipe-separated JsonPath ("(Object)|a|b|c"); `/` is the
 * MDL spelling because `|` reads badly here and `/` on this side of `=` cannot
 * collide with the association form on the other side.
 */
jsonMemberPath
    : identifierOrKeyword (SLASH identifierOrKeyword)*
    ;

importMappingObjectHandling
    : CREATE
    | FIND
    | FIND OR CREATE
    ;

/**
 * CREATE EXPORT MAPPING Module.Name
 *   FOLDER 'Private/Export mappings'
 *   WITH JSON STRUCTURE Module.JsonStructure
 * {
 *   Module.Entity {
 *     jsonField = Attr,
 *   }
 * };
 */
createExportMappingStatement
    : EXPORT MAPPING ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      exportMappingWithClause?
      exportMappingNullValuesClause?
      LBRACE exportMappingRootElement RBRACE
    ;

exportMappingWithClause
    : WITH JSON STRUCTURE qualifiedName (ROOT jsonMemberPath)?
    | WITH XML SCHEMA qualifiedName
    | WITH MESSAGE DEFINITION qualifiedName
    ;

exportMappingNullValuesClause
    : NULL VALUES identifierOrKeyword
    ;

exportMappingRootElement
    // An entity-less ROOT: a JSON object with no Mendix object behind it, the
    // root-level twin of `group as` (#262). It carries no member name, because a
    // root has none. DESCRIBE emitted `.` here and it never parsed — the last
    // describe-only spelling left after ako/mxcli#260.
    : GROUP LBRACE exportMappingChild (COMMA exportMappingChild)* RBRACE
    | qualifiedName mappingCustomHandler?
      LBRACE exportMappingChild (COMMA exportMappingChild)* RBRACE
    ;

exportMappingChild
    : qualifiedName SLASH qualifiedName mappingCustomHandler? AS jsonMemberPath
      LBRACE exportMappingChild (COMMA exportMappingChild)* RBRACE       // nested object with children
    | qualifiedName SLASH qualifiedName mappingCustomHandler? AS jsonMemberPath  // leaf object
    // A JSON grouping node with no Mendix object behind it — Studio Pro's
    // entity-less object element (#262). It may contain OBJECT elements only:
    // a value needs an entity to bind its attribute to, and Mendix rejects one
    // here with CE0061 "No entity selected."
    | GROUP AS identifierOrKeyword
      LBRACE exportMappingChild (COMMA exportMappingChild)* RBRACE
    // The export twin of the association-less object element (#260 item 3).
    // MUST come after GROUP: `group` is a keyword that qualifiedName accepts, so
    // this alternative placed earlier swallows `group as x` and stores an entity
    // called "Module.group" (caught by TestExportGroupElement).
    | qualifiedName mappingCustomHandler? AS jsonMemberPath
      LBRACE exportMappingChild (COMMA exportMappingChild)* RBRACE
    | qualifiedName mappingCustomHandler? AS jsonMemberPath
    | jsonMemberPath EQUALS qualifiedName LPAREN identifierOrKeyword RPAREN // value transform: a/b/c = Module.MF(Attr)
    | jsonMemberPath EQUALS identifierOrKeyword                           // value: a/b/c = Attr
    ;

// =============================================================================
// VALIDATION RULE CREATION
// =============================================================================

// A validation rule constrains ONE attribute, and Mendix stores it anonymously
// on the entity — there is no rule name to give, so the statement names its
// target instead:
//
//   create validation rule for Shop.Product.Email
//       regex Shop.EmailPattern
//       feedback 'Enter a valid email address';
//
//   create validation rule for Shop.Booking.Guests
//       range from 1 to 100
//       feedback 'Between 1 and 100 guests are allowed';
//
// Only the two constraints nothing else can express are here. Required and
// Unique are already authorable as attribute constraints (`not null error '…'`
// / `unique error '…'` on CREATE ENTITY and ALTER ENTITY), and a second path to
// the same rule would drift from the first; the executor points at that syntax
// rather than accepting a duplicate spelling.
//
// This rule previously existed with an unimplementable shape (an EXPRESSION
// form Mendix has no rule type for, an inline regex literal where Mendix stores
// a reference to a RegularExpression document, and strict < / > bounds Mendix
// cannot represent). It had no visitor and no handler, so every form parsed and
// silently did nothing — nothing can depend on the old spelling.
createValidationRuleStatement
    : VALIDATION RULE FOR qualifiedName
      validationRuleConstraint
      (ERROR_MESSAGE | FEEDBACK /* @alias MDL-DEPR021 */) STRING_LITERAL
    ;

validationRuleConstraint
    : REGEX qualifiedName
    | RANGE validationRuleRange
    ;

// Mendix has exactly three range kinds and no strict inequality, so the bounds
// are spelled inclusively and map one-to-one:
//   from X to Y -> Between, from X -> GreaterThanOrEqualTo, to Y -> SmallerThanOrEqualTo
validationRuleRange
    : FROM literal TO literal
    | FROM literal
    | TO literal
    ;

// =============================================================================
// CONSTANT CREATION
// =============================================================================

// A constant's properties are a ( Key: value ) list, named as Studio Pro's
// Constants$Constant names them: Type, DefaultValue, ExposedToClient (phase 3.6
// of PROPOSAL_mdl_beta_syntax_freeze.md, ako/mxcli#755). The clause form
// `type T default v [exposed to client]` is the deprecated alias. The trailing
// options after the list are the older aliases of the doc comment and the
// folder clause, still read so that each rewrite can land on its own.
createConstantStatement
    : CONSTANT ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?   // R9: the folder is a clause after the name
      constantPropertyList
      constantOptions?
    | CONSTANT ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      TYPE /* @alias MDL-DEPR136 */ dataType
      DEFAULT literal
      constantOptions?
    ;

constantPropertyList
    : LPAREN (constantProperty (COMMA constantProperty)* COMMA?)? RPAREN
    ;

// Type takes a data type, DefaultValue a literal, ExposedToClient a boolean.
// The visitor refuses any other key, and a value of the wrong shape.
constantProperty
    : identifierOrKeyword COLON (literal | dataType)
    ;

constantOptions
    : constantOption+
    ;

constantOption
    : COMMENT /* @alias MDL-DEPR100 */ STRING_LITERAL   // R9: a `/** … */` doc comment
    | FOLDER STRING_LITERAL /* @alias MDL-DEPR134 */   // R9: after the name
    | EXPOSED TO CLIENT
    // `private`, any case: a no-op Mendix never stored (ako/mxcli#865). Up to
    // v0.24.0 it parsed only because the trailing word became a help statement
    // of its own, which built nothing; R7 closed that catch-all. It is matched
    // here by predicate, not as a keyword, so `private` stays a usable name.
    | {IsPrivateWord(p.GetTokenStream().LT(1).GetText())}? IDENTIFIER /* @alias MDL-DEPR138 */
    ;

// =============================================================================
// INDEX CREATION (standalone)
// =============================================================================

createIndexStatement
    : INDEX IDENTIFIER ON qualifiedName LPAREN indexAttributeList RPAREN
    ;

// =============================================================================
// DATA TRANSFORMER
// =============================================================================

/**
 * CREATE DATA TRANSFORMER Module.Name
 * SOURCE JSON '{"latitude": 51.916, ...}'
 * {
 *   JSLT '{ "lat": .latitude }';
 * };
 */
createDataTransformerStatement
    : DATA TRANSFORMER ifNotExists? qualifiedName
      (FOLDER folder=STRING_LITERAL)?
      SOURCE_KW (JSON | XML) source=STRING_LITERAL
      LBRACE dataTransformerStep* RBRACE
    ;

dataTransformerStep
    : (JSLT | XSLT) (STRING_LITERAL | DOLLAR_STRING) SEMICOLON?
    ;
