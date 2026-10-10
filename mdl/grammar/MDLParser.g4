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

// Hand-written parser helpers. In the Go target `@members` is package-level
// code; the predicates in the imported grammars call it by name.
@parser::members {
// IsHelpWord reports whether word begins a help statement: `help`, `exit` or
// `quit`, in any letter case. They are IDENTIFIERs rather than keywords, so
// that reserving them does not take the words away as names; the predicate
// on helpStatement is what keeps that rule from being the grammar's
// catch-all, where a misspelt statement keyword (`craete entity …`) parsed
// as a help topic and was silently dropped (ako/mxcli#755, R7).
//
// The generated file imports no strings package, so the letter case is folded
// here by hand; the words are ASCII.
func IsHelpWord(word string) bool {
	if len(word) != 4 {
		return false
	}
	folded := []byte(word)
	for i, c := range folded {
		if c >= 'A' && c <= 'Z' {
			folded[i] = c + ('a' - 'A')
		}
	}
	switch string(folded) {
	case "help", "exit", "quit":
		return true
	}
	return false
}

// IsPrivateWord reports whether word is `private`, in any letter case: the
// no-op modifier older scripts wrote on a constant (ako/mxcli#865). Like the
// help words it is an IDENTIFIER, so reserving it takes no name away.
func IsPrivateWord(word string) bool {
	if len(word) != 7 {
		return false
	}
	for i, c := range []byte(word) {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != "private"[i] {
			return false
		}
	}
	return true
}
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

/**
 * Entry point: a program is an optional language header and a sequence of statements.
 * A later header may only repeat the script's own (concatenated describe output);
 * the visitor refuses one that names another version.
 */
program
    : languageHeader? (statement | repeatedLanguageHeader)* EOF
    ;

/**
 * The MDL language version a script is written in (ADR-0011): `mdl <n>;`.
 * Only the first statement may be a header; a script without one is mdl 0, the alpha meaning.
 * It is independent of the Mendix version the project targets.
 *
 * @example
 * ```mdl
 * mdl 1;
 * create entity Shop.Customer ( Name: String(200) );
 * ```
 */
languageHeader
    // The word is an IDENTIFIER, not a keyword, and the visitor requires it to
    // be `mdl`: reserving it would take the word away from every place that
    // accepts only an IDENTIFIER, such as `describe contract entity X format mdl`.
    : IDENTIFIER NUMBER_LITERAL SEMICOLON
    ;

/**
 * A language header after the first statement. Concatenating describe outputs,
 * each of which starts with `mdl 1;`, repeats the header; that is accepted when it
 * names the script's own version and refused when it names another (ADR-0011:
 * a script is written in one language version).
 */
repeatedLanguageHeader
    : IDENTIFIER NUMBER_LITERAL SEMICOLON
    ;

/** A statement can be DDL, DQL, or utility */
statement
    : docComment? (reservedDocumentStatement | ddlStatement | dqlStatement | utilityStatement) SEMICOLON? SLASH?
    ;

/**
 * R10 (ako/mxcli#755): Studio Pro document types MDL does not support yet.
 * Their names are reserved, so a statement naming one is refused by name
 * (the visitor reports it) rather than failing somewhere in its body, and no
 * later syntax can give the words another meaning.
 */
reservedDocumentStatement
    : (CREATE (OR MODIFY)? | ALTER | DROP | DESCRIBE | LIST_KW | SHOW) reservedDocumentName ~SEMICOLON*
    ;

reservedDocumentName
    : CONSUMED WEB SERVICES?
    | PUBLISHED WEB SERVICES?
    | XML SCHEMA
    | XML IDENTIFIER   // `xml schemas`: SCHEMA has no plural token
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
      CREATE (OR (MODIFY | REPLACE /* @alias MDL-DEPR001 */))?
      ( createEntityStatement
      | createAssociationStatement
      // Before createModuleStatement: `create module role M.R` is a role, and
      // `create module Role;` (no qualified name follows) is still a module.
      | createModuleRoleStatement
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
      | createMessageDefinitionStatement
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
    | alterEntitiesStatement
    | ALTER ASSOCIATION qualifiedName alterAssociationAction+
    | ALTER ENUMERATION qualifiedName alterEnumerationAction+
    // R3 (ako/mxcli#751): `set ( Key: value, … )`, create's property list.
    // The unparenthesised `set Key = value, …` is the old spelling.
    | ALTER consumedODataServiceKw qualifiedName SET odataAlterPropertyList
    | ALTER consumedODataServiceKw qualifiedName SET odataAlterAssignment (COMMA odataAlterAssignment)*
    | ALTER publishedODataServiceKw qualifiedName SET odataAlterPropertyList
    | ALTER publishedODataServiceKw qualifiedName SET odataAlterAssignment (COMMA odataAlterAssignment)*
    | ALTER STYLING ON (PAGE | SNIPPET) qualifiedName WIDGET IDENTIFIER alterStylingAction+
    | ALTER SETTINGS alterSettingsClause
    // The generic ALTER (ADR-0012 decision 2): one patch grammar for every
    // document type — `alter <type> Module.Name { set / insert / replace / drop }`.
    // The document type chooses how a target is resolved (a per-type
    // backend.AlterTargetResolver) and what a fragment is written in (exactly the
    // `create` syntax of that type). A layout's widget tree is a page's with four
    // extra element types, so SET/INSERT/DROP/REPLACE mean exactly the same thing
    // there; a scroll-container region is addressed as `layoutContainer.top`,
    // because a region has no Name of its own.
    | ALTER alterDocumentType qualifiedName LBRACE alterOperation+ RBRACE
    // The generic ALTER on a microflow or nanoflow (ADR-0012 decision 3): a
    // graph splice into the stored flow. Its targets are content addresses
    // (`$Var`, `'Caption'`, a statement pattern) and its fragments are
    // microflow statements, so it has its own operation rule.
    | ALTER (MICROFLOW | NANOFLOW) qualifiedName LBRACE alterFlowOperation* RBRACE
    | alterPagesLayoutStatement
    | alterPagesStylingStatement
    // The generic ALTER on a workflow (ADR-0012 decision 2, ako/mxcli#712): its
    // targets are activities (name, 'caption', @n) and its fragments are
    // workflow activities, so it has its own operation rule, like microflows.
    | ALTER WORKFLOW qualifiedName LBRACE alterWorkflowOperation+ RBRACE
    // The old per-action form: each alternative of alterWorkflowAction is a
    // registered alias (MDL-DEPR140-149).
    | ALTER WORKFLOW qualifiedName alterWorkflowAction+ SEMICOLON?
    | alterMessageDefinitionCollectionStatement
    | alterMessageDefinitionStatement
    | ALTER PUBLISHED REST SERVICE qualifiedName alterPublishedRestServiceAction (COMMA? alterPublishedRestServiceAction)*
    | ALTER aiModelKw qualifiedName SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ALTER KNOWLEDGE BASE qualifiedName SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ALTER CONSUMED MCP SERVICE qualifiedName SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ALTER AGENT qualifiedName alterAgentAction+
    | alterModuleJarDepStatement
    ;

// `set ( Key: value, … )` takes create's property list (R3); the
// `set Key = 'value'` form is the older spelling for the string properties.
alterPublishedRestServiceAction
    : SET LPAREN publishedRestProperty (COMMA publishedRestProperty)* COMMA? RPAREN
    | SET publishedRestAlterAssignment (COMMA publishedRestAlterAssignment)*
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
    : SET LPAREN alterStylingAssignment (COMMA alterStylingAssignment)* RPAREN  // set ( Class: 'x', 'Full width': on )
    | SET alterStylingAssignment (COMMA alterStylingAssignment)*  /* @alias MDL-DEPR062 */  // set Class = 'x'
    | CLEAR DESIGN PROPERTIES
    ;

// `Key: value` is canonical (R3: `:` sets a model property); `=` is the old
// spelling, still accepted.
alterStylingAssignOp
    : COLON
    | EQUALS   /* @alias MDL-DEPR062 */
    ;

alterStylingAssignment
    : CLASS alterStylingAssignOp STRING_LITERAL                  // Class: 'my-class'
    | STYLE alterStylingAssignOp STRING_LITERAL                  // Style: 'color: red;'
    | STRING_LITERAL alterStylingAssignOp STRING_LITERAL         // 'Spacing top': 'Large'
    | STRING_LITERAL alterStylingAssignOp ON                     // 'Full width': ON
    | STRING_LITERAL alterStylingAssignOp OFF                    // 'Full width': OFF
    ;

/**
 * The generic ALTER's operations (ADR-0012 decision 2, ako/mxcli#712).
 *
 * Canonical form, the same for every document type:
 *
 * ```mdl
 * alter page Module.Page {
 *   set (Caption: 'Save', ButtonStyle: Success) on btnSave;
 *   set (Title: 'Edit order');                       -- the document itself
 *   insert after txtName { textbox txtNew (Label: 'New', Attribute: Attr) }
 *   insert into ctnMain { … }
 *   replace dvMain.footer with { footer { … } }
 *   drop txtOld, dgOrders.Total;
 * }
 * ```
 *
 * Alternatives marked `alias:` are the old page spellings. They still parse to
 * the identical operation and warn with the named deprecation code; the table
 * that maps each code to its rewrite is mdl/executor/alter_aliases.go, and a
 * test fails when a marker here has no entry there (or the reverse).
 *
 * Page-family operations that have no generic spelling yet (`set layout = … map`,
 * `drop template for … in …`, `add variables`, `drop variables`,
 * `add parameters`, `drop parameters`) keep their own
 * form inside the generic block; they are not aliases.
 */
alterDocumentType
    : PAGE
    | SNIPPET
    | LAYOUT
    ;

alterOperation
    : alterSet SEMICOLON?
    | alterInsert SEMICOLON?
    | alterReplace SEMICOLON?
    | alterDrop SEMICOLON?
    | alterPageDropTemplate SEMICOLON?
    | alterPageAddVariable SEMICOLON?
    | alterPageDropVariable SEMICOLON?
    | alterPageAddParameter SEMICOLON?
    | alterPageDropParameter SEMICOLON?
    ;

alterSet
    : SET LAYOUT EQUALS qualifiedName (MAP LPAREN alterLayoutMapping (COMMA alterLayoutMapping)* RPAREN)?  // SET Layout = Atlas_Core.TopBar MAP (Main AS Content)
    | SET LPAREN alterPageAssignment (COMMA alterPageAssignment)* RPAREN (ON alterTarget)?  // set (Caption: 'Save', ButtonStyle: Success) on btnSave
    | SET alterPageAssignment (ON alterTarget)?     /* @alias MDL-DEPR102 */  // set Caption: 'Save' on btnSave
    ;

alterLayoutMapping
    : identifierOrKeyword AS identifierOrKeyword                                // OldPlaceholder AS NewPlaceholder
    ;

alterInsert
    : INSERT (AFTER | BEFORE | INTO) alterTarget alterFragment   // INTO appends as children of a container
    ;

alterReplace
    : REPLACE alterTarget WITH alterFragment
    ;

alterDrop
    : DROP alterTarget (COMMA alterTarget)*
    | DROP WIDGET /* @alias MDL-DEPR103 */ alterTarget (COMMA alterTarget)*   // drop widget a, b
    ;

// A fragment is written exactly as `create` writes the same content. A
// workflow's fragment is a workflow body (alterWorkflowFragment).
alterFragment
    : LBRACE pageBodyV3 RBRACE
    ;

// The one address syntax every document type shares. Which forms a type
// accepts is its resolver's call, not the grammar's: a page element is
// addressed by name (`btnSave`, `layoutContainer.top`); elements with no name
// are addressed by content (`'Approve order'`). `@n` picks one of several
// matches — an ambiguous address is an error that lists them, never a guess.
//
// A DataGrid 2 column has no name in the model, so it is addressed by what
// describe prints in it (R12, ako/mxcli#749): `dg column(Name)` for the column
// whose Attribute is Name — written as describe writes it, `Owner/Name` over an
// association — or `dg column('Total')` for the one captioned Total. Two
// columns over one attribute share the address and need `@n`. FIRST, so the
// COLUMN keyword after the grid name is not left to the plain-name form.
// `dg.Name`, the older address by a name mxcli derived, keeps working.
alterTarget
    : identifierOrKeyword COLUMN LPAREN (attributePathV3 | STRING_LITERAL) RPAREN (AT NUMBER_LITERAL)?
    | identifierOrKeyword (DOT identifierOrKeyword)? (AT NUMBER_LITERAL)?
    | STRING_LITERAL (AT NUMBER_LITERAL)?
    ;

/**
 * `alter microflow` / `alter nanoflow` operations (ADR-0012 decision 3,
 * ako/mxcli#736):
 *
 * ```mdl
 * alter microflow FeedbackModule.VAL_Feedback {
 *   insert after $IsValidEmail begin log info node 'Feedback' 'Email checked'; end;
 *   insert before 'Email is Valid?' begin … end;
 *   replace commit $Order with begin commit $Order with events; end;
 *   drop log * node 'Debug' *;
 * }
 * ```
 *
 * A fragment is written exactly as the same statements are in `create
 * microflow`. A target is a content address, resolved by mfmutator: `$Var`
 * (the activity that outputs it), `'Caption'`, or a statement pattern with `*`
 * wildcards, each optionally followed by `@n`. A pattern is any run of tokens,
 * so the target is taken as raw text up to the `begin`, `{`, `with` or `;` that ends it;
 * that is why `drop` needs its semicolon.
 */
alterFlowOperation
    : INSERT (AFTER | BEFORE) alterFlowTarget alterFlowFragment SEMICOLON?
    | REPLACE alterFlowTarget WITH alterFlowFragment SEMICOLON?
    | DROP alterFlowTarget SEMICOLON
    ;

// A fragment is imperative flow, so it is `begin … end` like the body of the
// `create microflow` it is copied from (R2, ako/mxcli#754). The operations
// around it are the alter's declarative children and stay in the alter's { }.
// The brace fragment is the old spelling.
alterFlowFragment
    : BEGIN microflowBody END
    | LBRACE /* @alias MDL-DEPR074 */ microflowBody RBRACE
    ;

// BEGIN ends a target as `{` does. describe's handles never contain it: a loop
// prints `begin` on a line of its own, and an error handler's `begin` is
// stripped from its activity's handle.
alterFlowTarget
    : ~(LBRACE | RBRACE | SEMICOLON | WITH | BEGIN)+
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

// ALTER PAGES [IN <module>] SET '<design property>' = <value>, ... WHERE WIDGETTYPE = <kw> [DRY RUN]
//
// The bulk form of ALTER PAGE's design-property SET, and the same argument: a
// house style is "every data grid is compact and striped", which is one
// statement rather than one per page. It mirrors the layout form above --- same
// verb, same optional IN, same WHERE --- and is told apart from it at parse time
// by what follows SET, since LAYOUT is a keyword and a design property is a
// quoted string.
//
// WHERE selects a widget TYPE, never a name: a widget name is unique only within
// its page (measured --- `actionButton1` exists in 30 units of a blank project),
// so a name predicate would sweep unrelated widgets together. The type is named
// by its MDL keyword, which resolves to exactly one widget id, rather than by a
// LIKE over the stored id, which also matches the data grid's FILTER widgets.
//
// DRY RUN is not optional politeness: this statement rewrites every page a match
// lands on, and the preview is the only way to see what a pattern selects before
// it selects it.
alterPagesStylingStatement
    : ALTER PAGES (IN identifierOrKeyword)? SET alterPagesStylingAssignment
      (COMMA alterPagesStylingAssignment)*
      WHERE WIDGETTYPE EQUALS (STRING_LITERAL | identifierOrKeyword)
      (DRY RUN)?
    ;

// The same three value shapes alterStylingAssignment takes, minus CLASS/STYLE:
// those are per-widget CSS, which a project-wide sweep has no business setting.
alterPagesStylingAssignment
    : STRING_LITERAL EQUALS STRING_LITERAL         // 'Row size' = 'Small'
    | STRING_LITERAL EQUALS ON                     // 'Striped' = ON
    | STRING_LITERAL EQUALS OFF                    // 'Striped' = OFF
    ;

// `Key: value` is canonical (R3: `:` binds a property, `=` compares). `=` is
// the old spelling, still accepted.
alterAssignOp
    : COLON
    | EQUALS   /* @alias MDL-DEPR101 */  // set (Caption = 'Save') / set Caption = 'Save'
    ;

alterPageAssignment
    : DATASOURCE alterAssignOp dataSourceExprV3               // DataSource: selection widgetName
    | ACTION alterAssignOp actionExprV3                       // Action: MICROFLOW Module.MF | SHOW_PAGE Module.Page | SAVE_CHANGES CLOSE_PAGE
    // R5 (ako/mxcli#753): the condition is a bare expression; the bracketed
    // form is the deprecated alias. The plain value keeps its reading, ahead of
    // the expression, as in widgetPropertyV3.
    | VISIBLE alterAssignOp xpathConstraint /* @alias MDL-DEPR081 */  // Visible: [Name != ''] (conditional visibility)
    | EDITABLE alterAssignOp xpathConstraint /* @alias MDL-DEPR081 */ // Editable: [Status = 'Open'] (conditional editability)
    | (VISIBLE | EDITABLE) alterAssignOp propertyValueV3      // Visible: false, Editable: Never
    | (VISIBLE | EDITABLE) alterAssignOp expression           // Visible: $currentObject/Name != ''
    // A pluggable widget's NAMED action slot, addressed by the widget's own key:
    // `set 'createFileAction' = microflow M.F on fileUploader1`. The ALTER-level
    // twin of widgetPropertyV3's `key: actionExprV3` (#956); without it the value
    // fell to propertyValueV3, which has no `microflow <name>` form, and the only
    // way to retarget one slot was to REPLACE the whole widget
    // (mendixlabs/mxcli#995). Placed before the scalar alternatives, as on CREATE,
    // so a bare action keyword (`close_page`) is an action; unlike CREATE there is
    // no datasource overlap to yield to, since DataSource is its own alternative.
    // Whether the key IS an action slot is the stored widget's call, not the
    // grammar's — the mutator refuses one that is not.
    | STRING_LITERAL alterAssignOp actionExprV3                // 'createFileAction': microflow Module.MF
    | identifierOrKeyword alterAssignOp actionExprV3           // createFileAction: microflow Module.MF
    | identifierOrKeyword alterAssignOp propertyValueV3       // Caption: 'Save'
    | STRING_LITERAL alterAssignOp propertyValueV3             // 'showLabel': false
    | identifierOrKeyword alterAssignOp expression             // DynamicClasses: if $x/F then 'a' else '' (see widgetPropertyV3)
    ;

// DROP TEMPLATE FOR Module.Specialization IN listViewName
//
// A List View specialization template has no name — the entity it renders is
// what identifies it — so it cannot be reached through alterTarget like every
// other DROP target. Naming the list view is required, not optional: one page
// can hold two list views with a template for the same entity.
//
// There is no matching INSERT TEMPLATE. Adding one is
// `INSERT INTO <listview> { template for Module.Entity { ... } }`, which reuses
// the same block as CREATE PAGE, so a template has one spelling everywhere.
alterPageDropTemplate
    : DROP TEMPLATE FOR qualifiedName IN alterTarget
    ;

alterPageAddVariable
    : ADD VARIABLES_KW variableDeclaration    // ADD Variables $show: Boolean = true
    ;

alterPageDropVariable
    : DROP VARIABLES_KW VARIABLE              // DROP Variables $show
    ;

// A page or snippet parameter, declared with the same `pageParameter` rule as
// CREATE's `Params: (...)`, so the two spellings cannot drift. Before these
// existed the only way to add a parameter was CREATE OR REPLACE, which rebuilds
// the whole page and loses whatever describe does not round-trip
// (mendixlabs/mxcli#1234).
alterPageAddParameter
    : ADD PARAMETERS pageParameter            // ADD Parameters $Customer: Module.Customer
    ;

alterPageDropParameter
    : DROP PARAMETERS VARIABLE                // DROP Parameters $Customer
    ;

// A native profile's home is a page or a NANOFLOW (Navigation$NativeHomePage
// HomePagePage / HomePageNanoflow); `home nanoflow` names the second. Before it
// existed describe printed a native nanoflow home as `home microflow`
// (ako/mxcli#980). Additive: `home microflow` still parses.
navigationClause
    : HOME (PAGE | MICROFLOW | NANOFLOW) qualifiedName (FOR qualifiedName)?
    | LOGIN PAGE qualifiedName
    | NOT FOUND PAGE qualifiedName
    // The profile's menu items are its declarative children, in { } like a
    // page's widgets and with no separators (R2, ako/mxcli#754). describe
    // writes the block after every other clause; the clauses are order-free,
    // so it parses anywhere among them. `menu ( item; … )` is the old spelling.
    | navMenuChildren
    | MENU_KW LPAREN /* @alias MDL-DEPR121 */ navMenuItemDef* RPAREN
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
    // Studio Pro's "Progressive web app" settings, stored as the profile's
    // Navigation$ProgressiveWebAppSettings: null unless set. An offline profile
    // without them gets no service worker, so no page opens without a network
    // (mendixlabs/mxcli#1377). The keys are the stored property names,
    // Precaching and InstallPrompt (R3, R10); a bare clause sets the platform
    // defaults, OFF stores null again, and an omitted clause leaves them alone.
    | PROGRESSIVE WEB APP (settingsItemOptions | OFF)?
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
//
// R2 (ako/mxcli#754): a menu item is a child with the shape every child has,
// `<kind> Caption ( props ) [ { children } ]`. Its action is `OnClick:` with the
// words a page action uses (R8), and its icon is `Icon:` as on a widget:
//   menu item 'Home' ( OnClick: show page Shop.Home, Icon: Atlas_Core.Atlas.home )
//   menu 'Admin' ( Icon: glyph 57345 ) { menu item 'Users' ( OnClick: call microflow M.F ) }
// A child ends in `)` or `}`, so no separator is needed.
//
// The old spelling is still read: the action and icon as clauses after the
// caption (MDL-DEPR122), and a sub-menu's items in ( ) with `;` after each
// item (MDL-DEPR121) — a `;` is read after either shape, so a half-converted
// menu still parses. The canonical alternatives come first, so a bare
// `menu item 'x'` is read as the canonical form.
navMenuItemDef
    : MENU_KW ITEM STRING_LITERAL navMenuItemProps? (SEMICOLON /* @alias MDL-DEPR121 */)?
    | MENU_KW STRING_LITERAL navMenuItemProps? navMenuChildren (SEMICOLON /* @alias MDL-DEPR121 */)?
    | MENU_KW ITEM STRING_LITERAL
      ((PAGE qualifiedName) | (MICROFLOW qualifiedName) | SIGN_OUT)? /* @alias MDL-DEPR122 */ navMenuIcon? SEMICOLON?
    | MENU_KW STRING_LITERAL navMenuIcon? LPAREN /* @alias MDL-DEPR121 */ navMenuItemDef* RPAREN SEMICOLON?
    // Half-converted: the items already in { }, the icon still a clause.
    | MENU_KW STRING_LITERAL navMenuIcon /* @alias MDL-DEPR122 */ navMenuChildren SEMICOLON?
    ;

navMenuChildren
    : LBRACE navMenuItemDef* RBRACE
    ;

navMenuItemProps
    : LPAREN (navMenuItemProp (COMMA navMenuItemProp)* COMMA?)? RPAREN
    ;

// OnClick takes a page action, in the page-action words (R8); Icon the three
// icon elements, as navMenuIcon.
navMenuItemProp
    : ONCLICK COLON navMenuAction
    | ICON COLON navMenuIconValue
    ;

// A menu item stores the same client action a button does (Forms$FormAction,
// Forms$MicroflowAction, Forms$CallNanoflowClientAction, Forms$OpenLinkClientAction,
// Forms$CreateObjectClientAction, Forms$SignOutClientAction, Forms$NoAction —
// measured on ako/TestApp), so OnClick takes the widget action expression,
// settings included:
//   menu item 'Reports' ( OnClick: call nanoflow M.ShowReports with (ProgressBar: Blocking) )
//   menu item 'Docs' ( OnClick: open link 'https://docs.mendix.com' )
//   menu item 'New order' ( OnClick: create object M.Order then show page M.Order_New )
// It used to take only show page, call microflow and sign out, so describe
// printed every other action as nothing and a re-run stored Forms$NoAction
// (ako/mxcli#980). The three old forms are actionExprV3 alternatives, so every
// script that parsed still parses. The kinds a menu item cannot carry (save,
// delete, close page, …) are refused by the visitor.
navMenuAction
    : actionExprV3
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
    : ICON navMenuIconValue
    ;

navMenuIconValue
    : GLYPH NUMBER_LITERAL
    | IMAGE qualifiedName
    | qualifiedName
    ;

// A standalone menu document (Menus$MenuDocument) — the reusable menu a menu
// widget points at, as opposed to the menu inside a navigation profile. Both are
// built from the same items, so this reuses navMenuItemDef rather than defining a
// second item syntax.
createMenuStatement
    : MENU_KW ifNotExists? qualifiedName (FOLDER STRING_LITERAL)? navMenuChildren
    | MENU_KW ifNotExists? qualifiedName (FOLDER STRING_LITERAL)? LPAREN /* @alias MDL-DEPR121 */ navMenuItemDef* RPAREN
    ;

dropStatement
    : DROP ENTITY ifExists? qualifiedName
    // R6 (ako/mxcli#755): every document that can be created can be dropped.
    // An external entity is an entity; the word says which kind is meant, so a
    // local entity named by mistake is refused rather than dropped.
    | DROP EXTERNAL ENTITY ifExists? qualifiedName
    | DROP DATABASE CONNECTION ifExists? qualifiedName
    // A validation rule is anonymous and lives on its attribute, so the drop
    // names the attribute, as `create validation rule for` does. Without a
    // kind it drops both the regex and the range rule.
    | DROP VALIDATION RULE ifExists? FOR qualifiedName (REGEX | RANGE)?
    | DROP ASSOCIATION ifExists? qualifiedName
    | DROP ENUMERATION ifExists? qualifiedName
    | DROP CONSTANT ifExists? qualifiedName
    | DROP MICROFLOW ifExists? qualifiedName
    | DROP NANOFLOW ifExists? qualifiedName
    | DROP RULE ifExists? qualifiedName
    | DROP PAGE ifExists? qualifiedName
    | DROP LAYOUT ifExists? qualifiedName
    | DROP SNIPPET ifExists? qualifiedName
    | DROP MENU_KW ifExists? qualifiedName
    | DROP MODULE ifExists? qualifiedName
    | DROP taskQueueKw ifExists? qualifiedName
    | DROP SCHEDULED EVENT ifExists? qualifiedName
    | DROP REGULAR EXPRESSION ifExists? qualifiedName
    | DROP JAVA ACTION ifExists? qualifiedName
    | DROP JAVASCRIPT ACTION ifExists? qualifiedName
    | DROP INDEX qualifiedName ON qualifiedName
    | DROP consumedODataServiceKw ifExists? qualifiedName
    | DROP publishedODataServiceKw ifExists? qualifiedName
    | DROP BUSINESS EVENT SERVICE ifExists? qualifiedName
    | DROP WORKFLOW ifExists? qualifiedName
    | DROP IMAGE COLLECTION ifExists? qualifiedName
    | DROP ANNOTATION STRING_LITERAL IN identifierOrKeyword
    | DROP ANNOTATION AT_KW LPAREN NUMBER_LITERAL COMMA NUMBER_LITERAL RPAREN IN identifierOrKeyword
    | DROP JSON STRUCTURE ifExists? qualifiedName
    | DROP MESSAGE DEFINITION COLLECTION ifExists? qualifiedName
    | DROP MESSAGE DEFINITION ifExists? qualifiedName
    | DROP IMPORT MAPPING ifExists? qualifiedName
    | DROP EXPORT MAPPING ifExists? qualifiedName
    | DROP consumedRestServiceKw ifExists? qualifiedName
    | DROP PUBLISHED REST SERVICE ifExists? qualifiedName
    | DROP DATA TRANSFORMER ifExists? qualifiedName
    | DROP aiModelKw ifExists? qualifiedName                           // DROP AI MODEL Module.Name (agent-editor)
    | DROP CONSUMED MCP SERVICE ifExists? qualifiedName                // DROP CONSUMED MCP SERVICE Module.Name
    | DROP KNOWLEDGE BASE ifExists? qualifiedName                      // DROP KNOWLEDGE BASE Module.Name
    | DROP AGENT ifExists? qualifiedName                               // DROP AGENT Module.Name
    | DROP CONFIGURATION ifExists? STRING_LITERAL
    | DROP FOLDER ifExists? STRING_LITERAL IN (qualifiedName | IDENTIFIER)
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
    | taskQueueKw
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
    | consumedRestServiceKw
    | PUBLISHED REST SERVICE
    | consumedODataServiceKw
    | publishedODataServiceKw
    | BUSINESS EVENT SERVICE
    | aiModelKw
    | AGENT
    | KNOWLEDGE BASE
    | CONSUMED MCP SERVICE
    ;

// =============================================================================
// SECURITY STATEMENTS (dispatch list — rules in MDLSecurity.g4)
// =============================================================================

securityStatement
    : dropModuleRoleStatement
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
