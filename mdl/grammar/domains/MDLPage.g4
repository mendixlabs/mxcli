/**
 * MDL Page Grammar — pages, snippets, shared page/snippet rules, xpath expressions,
 * page V3 syntax.
 */
parser grammar MDLPage;

options { tokenVocab = MDLLexer; }

// =============================================================================
// PAGE CREATION
// =============================================================================

/**
 * Creates a new page with layout, parameters, and widget content.
 */
// R9: the folder is a clause after the name, as on every document; the
// `Folder:` header property is a registered alias.
createPageStatement
    : PAGE ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      pageHeaderV3
      LBRACE pageBodyV3 RBRACE
    ;

// =============================================================================
// LAYOUT CREATION
// =============================================================================

// A layout is a page's frame: pages bind into its placeholders by name.
// The property block carries the layout type — which is stored on the content
// wrapper, not on the layout element — and which placeholder a page's content
// goes into.
createLayoutStatement
    : LAYOUT ifNotExists? qualifiedName
      widgetPropertiesV3?
      LBRACE pageBodyV3 RBRACE
    ;

// =============================================================================
// SNIPPET CREATION
// =============================================================================

createSnippetStatement
    : SNIPPET ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      snippetHeaderV3?
      snippetOptions?
      LBRACE pageBodyV3 RBRACE
    ;

snippetOptions: snippetOption+ ;
// R9: the folder is a clause after the name; after the header is its old place.
snippetOption: FOLDER STRING_LITERAL /* @alias MDL-DEPR134 */ ;

// =============================================================================
// SHARED PAGE/SNIPPET RULES
// =============================================================================

pageParameterList
    : pageParameter (COMMA pageParameter)*
    ;

// `OPTIONAL` is Forms$PageParameter.IsRequired = false (Mendix 11.5+): the page
// may be opened without an argument for it (mendixlabs/mxcli#1640). A snippet
// parameter shares the rule and has no such flag; the executor refuses it there.
pageParameter
    : (IDENTIFIER | VARIABLE | QUOTED_IDENTIFIER) COLON dataType OPTIONAL?
    ;

// A snippet parameter is a page parameter. There used to be a byte-identical
// `snippetParameterList` rule here with its own visitor, and the two drifted
// twice from the same clause: a quoted entity name reached the resolver with
// its quotes, and a primitive type was taken for an entity
// (mendixlabs/mxcli#1028). One rule, one conversion.

variableDeclarationList
    : variableDeclaration (COMMA variableDeclaration)*
    ;

// A variable's default is an expression, written bare: `$show: Boolean = true`.
// A string whose content is the expression is the old spelling (MDL-DEPR086).
variableDeclaration
    : VARIABLE COLON dataType EQUALS (STRING_LITERAL /* @alias MDL-DEPR086 */ | expression)
    ;

// A sort column. The name may navigate associations, one `/` per hop, with the
// final segment naming the attribute:
//
//   sort by Name asc
//   sort by Sales.Order.Name asc
//   sort by Sales.Order_BillTo/Sales.Address.City asc
//
// Mendix stores the hops as the AttributeRef's EntityRef, and without a spelling
// for them `describe` had to drop them and `exec` had to guess — which silently
// picked the wrong association wherever two reach the same entity
// (mendixlabs/mxcli#1152). `qualifiedName SLASH qualifiedName` is the same shape
// MDLCatalog.g4 uses for `Association/Entity`.
sortColumn
    : (qualifiedName (SLASH qualifiedName)* | IDENTIFIER) (ASC | DESC)?
    ;

// One attribute of a List View's search bar. No direction — unlike a sort
// column, a search attribute is only a name.
searchAttribute
    : (qualifiedName | IDENTIFIER)
    ;

xpathConstraint
    : LBRACKET xpathExpr RBRACKET
    ;

andOrXpath
    : AND
    | OR
    ;

// =============================================================================
// XPATH EXPRESSION RULES
// =============================================================================
//
// Dedicated grammar for XPath expressions inside [...] constraints.
// Separate from the general expression rules because XPath has different semantics:
// - '/' is always path traversal (not division)
// - '[...]' inside paths are nested predicates
// - Bare identifiers/paths are existence checks
// - Functions like not(), contains(), starts-with() are XPath-native
//

xpathExpr
    : xpathAndExpr (OR xpathAndExpr)*
    ;

xpathAndExpr
    : xpathNotExpr (AND xpathNotExpr)*
    ;

xpathNotExpr
    : NOT xpathNotExpr
    | xpathComparisonExpr
    ;

xpathComparisonExpr
    : xpathValueExpr (comparisonOperator xpathValueExpr)?
    ;

xpathValueExpr
    : xpathFunctionCall
    | MINUS xpathValueExpr
    | xpathConstantRef
    | xpathPath
    | LPAREN xpathExpr RPAREN
    ;

/** A constant, `@Module.Constant`, spelled as an expression spells one: a value,
 * never a path step, so it takes no predicate and no `/`.
 *
 * Before this rule `@` was an xpathWord, a one-word name of its own, so
 * `[Total > @Shop.C_Limit]` failed at `Shop` — and the lenient re-parse that
 * describe and the long-constraint layout use recovered to an attribute called
 * "@" and wrote the constraint back without the constant (ako/mxcli#1685).
 * Parsing it is what lets `mxcli check` say what is actually wrong: mxbuild
 * rejects a constant in an XPath constraint (CE0161), see validateXPathConstants. */
xpathConstantRef
    : AT xpathQualifiedName
    ;

xpathPath
    : xpathStep (SLASH xpathStep)*
    ;

// A step takes any number of predicates, `Entity[a][b]`, as XPath does. With
// at most one, the second `[` was a parse error (mendixlabs/mxcli#1281).
xpathStep
    : xpathStepValue (LBRACKET xpathExpr RBRACKET)*
    ;

xpathStepValue
    : xpathQualifiedName
    | VARIABLE
    | STRING_LITERAL
    | NUMBER_LITERAL
    | MENDIX_TOKEN
    ;

/** Qualified name in XPath context: accepts any keyword as identifier part. */
xpathQualifiedName
    : xpathWord (DOT xpathWord)*
    ;

/** Any single-word token that can appear as part of a name in XPath.
 *
 * MINUS is excluded: a hyphen inside a name is already lexed as a single
 * HYPHENATED_ID (`starts-with`), so a standalone `-` is never part of a name.
 * While it was admissible here, `[Amount > -7]` parsed the sign as a name word
 * and left the digits stranded — reported as "negative literals truncate to -"
 * (issuetracker finding #18). It is a unary operator; see xpathValueExpr.
 *
 * AT is excluded for the same reason: `@` is never part of a name, it opens a
 * constant (xpathConstantRef). Admitted here, `[Name = @]` compared with an
 * attribute called "@" (ako/mxcli#1685). */
xpathWord
    : ~( DOT | SLASH | LBRACKET | RBRACKET | LPAREN | RPAREN | COMMA
       | EQUALS | NOT_EQUALS | LESS_THAN | LESS_THAN_OR_EQUAL
       | GREATER_THAN | GREATER_THAN_OR_EQUAL
       | AND | OR | NOT
       | SEMICOLON | MINUS | AT
       | STRING_LITERAL | NUMBER_LITERAL | VARIABLE | MENDIX_TOKEN | DOLLAR_STRING
       )
    ;

xpathFunctionCall
    : xpathFunctionName LPAREN (xpathExpr (COMMA xpathExpr)*)? RPAREN
    ;

/** Function name inside a bracketed [ … ] constraint.
 *
 * Any single word may name a function, exactly as any single word may be a name
 * part (see xpathQualifiedName). The enumerated form this replaces listed only
 * IDENTIFIER, HYPHENATED_ID, NOT, TRUE, FALSE and CONTAINS, so a call to a
 * function whose name is also a lexer keyword — `trim(…)`, `length(…)` — never
 * matched xpathFunctionCall. The enclosing `Visible: [...]` / `Editable: [...]`
 * then failed to parse as an xpathConstraint and fell through to the generic
 * property-value alternative, so the widget's whole conditional property was
 * dropped without a diagnostic (a dropped Visible reads as "always visible" at
 * runtime). Issue #852.
 *
 * The grammar deliberately does NOT enumerate a valid function set, because
 * xpathConstraint serves TWO contexts with DIFFERENT ones:
 *
 *   - `Visible:` / `Editable:` — a Mendix *client expression*, where the string
 *     functions apply: trim(), length(), toUpperCase(), find(), contains().
 *   - a datasource `where` clause — real *XPath*, where the function set is
 *     contains/starts-with/ends-with/string-length/not/true/false and the
 *     *-from-dateTime family, `length()` means list length rather than character
 *     count, and the aggregates (count/avg/min/max/sum) are Java-API-only.
 *     `empty` and `NULL` are keywords here (`[Name = empty]`), never calls.
 *     See docs.mendix.com/refguide/xpath-constraint-functions/ and
 *     .../xpath-keywords-and-system-variables/.
 *
 * One rule cannot encode both sets, and guessing wrong rejects valid MDL. So the
 * grammar accepts any name and lets mxbuild adjudicate semantics — it reports an
 * unknown or wrong-context function as CE0117 against the real version's rules,
 * which no table here could track. Verified on 11.6.6: in a widget conditional
 * trim/length/find pass while count/empty give CE0117; in a `where` clause
 * `[Name = empty]`, `[Name = NULL]`, not(), contains(), starts-with() and
 * string-length() all pass.
 *
 * xpathWord is a negated token set, so it self-maintains as the lexer grows new
 * keywords — an enumerated list would silently reacquire this bug with the next
 * function name that gets promoted to a token. NOT is spelled out because
 * xpathWord excludes it (it is an operator elsewhere in the expression grammar)
 * while `not(…)` is a legitimate call.
 *
 * This cannot swallow a path: xpathFunctionCall requires an LPAREN after the
 * name, and no xpathStepValue may be followed by one, so `empty` alone still
 * parses as a word via xpathPath — which is what keeps `[Name = empty]` working. */
xpathFunctionName
    : xpathWord
    | NOT
    ;

// =============================================================================
// PAGE V3 SYNTAX (Agent-Friendly: all properties in parentheses)
// =============================================================================

// V3 Page Header: all metadata in single () block
pageHeaderV3
    : LPAREN pageHeaderPropertyV3 (COMMA pageHeaderPropertyV3)* RPAREN
    ;

pageHeaderPropertyV3
    // A map is a property list, so it is in ( ) (R2, ako/mxcli#754).
    : PARAMS COLON LPAREN pageParameterList COMMA? RPAREN                         // Params: ( $Order: Entity )
    | PARAMS COLON LBRACE /* @alias MDL-DEPR123 */ pageParameterList RBRACE       // Params: { $Order: Entity }
    | VARIABLES_KW COLON LPAREN variableDeclarationList COMMA? RPAREN             // Variables: ( $show: Boolean = 'true' )
    | VARIABLES_KW COLON LBRACE /* @alias MDL-DEPR123 */ variableDeclarationList RBRACE
    | TITLE COLON STRING_LITERAL                                     // Title: 'My Page'
    | LAYOUT COLON (qualifiedName | STRING_LITERAL)                  // Layout: Atlas_Core.Atlas_Default
    | URL COLON STRING_LITERAL                                       // Url: 'my-page'
    | FOLDER COLON /* @alias MDL-DEPR105 */ STRING_LITERAL          // Folder: 'Pages/Admin'
    | CLASS COLON STRING_LITERAL                                     // Class: 'my-page bg-primary'
    | STYLE COLON STRING_LITERAL                                     // Style: 'padding: 10px'
    | DESIGNPROPERTIES COLON designPropertyListV3                    // DesignProperties: ( 'Spacing top': 'Inner large' ) — the page's own (ako/mxcli#1678)
    | IDENTIFIER COLON propertyValueV3                               // Generic page property: PopupWidth: 800, PopupResizable: true
    ;

// V3 Snippet Header
snippetHeaderV3
    : LPAREN snippetHeaderPropertyV3 (COMMA snippetHeaderPropertyV3)* RPAREN
    ;

snippetHeaderPropertyV3
    : PARAMS COLON LPAREN pageParameterList COMMA? RPAREN                         // Params: ( $Customer: Module.Entity ) — entities only (MDL087)
    | PARAMS COLON LBRACE /* @alias MDL-DEPR123 */ pageParameterList RBRACE
    | VARIABLES_KW COLON LPAREN variableDeclarationList COMMA? RPAREN             // Variables: ( $show: Boolean = 'true' )
    | VARIABLES_KW COLON LBRACE /* @alias MDL-DEPR123 */ variableDeclarationList RBRACE
    | FOLDER COLON /* @alias MDL-DEPR105 */ STRING_LITERAL        // Folder: 'Snippets/Common'
    ;

// V3 Page body. Bare widgets bind to the layout's Main placeholder; a
// `placeholder <Name> { … }` block binds its widgets to that named layout
// placeholder (issue #532 — pages over a layout with >1 placeholder).
pageBodyV3
    // ORDER IS LOAD-BEARING since slices 2-3. widgetV3's last alternative is a
    // generic (IDENTIFIER | keyword) widget type, and SLOT, PLACEHOLDER and USE
    // are all in `keyword` — so with widgetV3 first, `slot content` parsed as a
    // widget of type `slot` named `content`, and `placeholder Main { … }` as a
    // widget named Main. Both still PARSED and still exited 0, which is why a
    // diff of `mxcli check` output across all 515 example scripts did not show
    // it; the damage is to the AST, not to the diagnostics. Two visitor unit
    // tests caught it.
    //
    // The specific alternatives therefore go first, the same ordering fix
    // widgetV3 already applies internally for `template for`.
    : (useFragmentRef | useBuildingBlockRef | placeholderBlockV3 | slotMarkerV3 | widgetV3)*
    ;

// SLOT [name] — a content placeholder inside a `define fragment` body. When the
// fragment is used with a payload (`use fragment X { … }`), the caller's widgets
// are spliced in at this position. The name is optional (defaults to "content")
// and, for v1, cosmetic — a fragment supports a single slot. Outside a fragment
// definition a slot marker is rejected at expansion time.
slotMarkerV3
    : SLOT identifierOrKeyword?
    ;

// PLACEHOLDER <Name> [{ widgets }] — one rule, two jobs, decided by context.
//
// In a page, the body assigns widgets to a named layout placeholder. In a
// layout, the bodiless form DECLARES a placeholder: a slot pages bind to as
// Module.Layout.<Name>. They are the same syntax because they name the same
// thing from the two ends of the binding, and the executor knows which document
// it is building — a bodiless placeholder in a page fills nothing and is
// reported there rather than parsed differently.
//
// The name accepts keywords so placeholders like Right / Left / Content parse.
placeholderBlockV3
    : PLACEHOLDER identifierOrKeyword (LBRACE (widgetV3 | useFragmentRef | useBuildingBlockRef)* RBRACE)?
    ;

// USE FRAGMENT Name [(args)] [AS prefix_] [ { payload widgets } ]
// The optional arg list supplies values for the fragment's declared parameters
// (datasource / action). The optional brace block supplies the widgets that fill
// the fragment's content slot (see slotMarkerV3). Without it, a slotted fragment
// expands with an empty slot.
useFragmentRef
    : USE FRAGMENT identifierOrKeyword fragmentArgs? (AS identifierOrKeyword)? useFragmentPayload?
    ;

useFragmentPayload
    : LBRACE pageBodyV3 RBRACE
    ;

// Fragment argument list: ($p: $Data, $q: microflow Module.Flow)
//
// May be empty, meaning no arguments, as microflowArgsV3 may (ako/mxcli#1516).
// Unlike the flow calls this was not a syntax error: `use fragment Frag()`
// failed this rule and the page body re-read it as two generic widgets, one of
// type `use` named `fragment` and one of type `Frag` — and `mxcli check`
// without a project passed it.
fragmentArgs
    : LPAREN (fragmentArg (COMMA fragmentArg)*)? RPAREN
    ;

fragmentArg
    : VARIABLE COLON fragmentArgValue
    ;

// A fragment arg value is either a datasource or an action. They overlap on
// MICROFLOW/NANOFLOW; the executor disambiguates using the parameter's declared
// kind.
fragmentArgValue
    : dataSourceExprV3
    | actionExprV3
    ;

// USE BUILDING BLOCK Module.Name [(rebind overrides)] [AS prefix_]
// Deep-copies the building block's widget tree into the page/container. The
// optional override list rebinds the block's outermost datasource and/or primary
// action to caller-supplied values after the copy.
useBuildingBlockRef
    : USE BUILDING BLOCK qualifiedName blockOverrides? (AS identifierOrKeyword)?
    ;

blockOverrides
    : LPAREN blockOverride (COMMA blockOverride)* RPAREN
    ;

blockOverride
    : DATASOURCE COLON dataSourceExprV3
    | ACTION COLON actionExprV3
    ;

// V3 Widget: WIDGET name (Props) { children }
// The name accepts QUOTED_IDENTIFIER in addition to IDENTIFIER so widgets named
// after a reserved keyword (e.g. "List", "Column") can be expressed. DESCRIBE
// emits the quoted form for such names so its output re-parses. See issue #619.
widgetV3
    // A List View specialization template: one body per specialization of the
    // list view's entity. It carries an entity, not a name — Studio Pro stores
    // Forms$ListViewTemplate with exactly {Entity, Widgets}.
    //
    // FIRST alternative on purpose. FOR is in the `keyword` rule, so without the
    // ordering `template for Pages.Bus { }` could be read as a TEMPLATE widget
    // named "for". A Gallery content slot named `for` must now be quoted
    // (`template "for" { }`), the same escape hatch reserved names already use
    // (issue #619).
    : TEMPLATE FOR qualifiedName widgetBodyV3
    //
    // The NAME is optional (R12, ako/mxcli#749). Mendix stores no name on a
    // layout-grid row or column, a DataGrid 2 column, or a slot block such as a
    // gallery's `template`, so describe no longer invents one (`row1`, `col3`)
    // and the author need not either: `row { column (DesktopWidth: 6) { … } }`.
    // Where Mendix DOES store a name, a missing one is refused when the widget
    // is built (pageBuilder.buildWidgetV3), not here: whether the model keeps a
    // name depends on the element's parent, which the grammar cannot see.
    // A name written where the parent shows it is not stored is the old
    // spelling (the visitor drops it and reports it):
    //   /* @alias MDL-DEPR005 */
    | widgetTypeV3 (IDENTIFIER | QUOTED_IDENTIFIER | keyword)? widgetPropertiesV3? widgetBodyV3?
    | PLUGGABLEWIDGET STRING_LITERAL (IDENTIFIER | QUOTED_IDENTIFIER | keyword)? widgetPropertiesV3? widgetBodyV3?  // PLUGGABLEWIDGET 'widget.id' [name]
    | CUSTOMWIDGET STRING_LITERAL (IDENTIFIER | QUOTED_IDENTIFIER | keyword)? widgetPropertiesV3? widgetBodyV3?     // CUSTOMWIDGET 'widget.id' [name] (legacy)
    ;

// V3 Widget types (same as V2)
widgetTypeV3
    : LAYOUTGRID
    | ROW
    | COLUMN
    | DATAGRID
    | DATAVIEW
    | LISTVIEW
    | GALLERY
    | CONTAINER
    | NAVIGATIONLIST
    | ITEM
    | TEXTBOX
    | TEXTAREA
    | DATEPICKER
    | DROPDOWN
    | COMBOBOX
    | CHECKBOX
    | RADIOBUTTONS
    | FILEMANAGER                                     // Forms$FileManager — upload/download a System.FileDocument (#151)
    | REFERENCESELECTOR
    | ACTIONBUTTON
    | LINKBUTTON
    | TITLE
    | LABEL                                           // Forms$Label (Studio Pro's Label widget)
    | DYNAMICTEXT
    | STATICTEXT
    | SNIPPETCALL
    | CUSTOMWIDGET
    | TEXTFILTER
    | NUMBERFILTER
    | DROPDOWNFILTER
    | DATEFILTER
    | DROPDOWNSORT
    | FOOTER
    | HEADER
    | CONTROLBAR
    | FILTER
    | TEMPLATE
    | IMAGE
    | STATICIMAGE
    | DYNAMICIMAGE
    | CUSTOMCONTAINER
    | TABCONTAINER
    | TABPAGE
    | GROUPBOX
    // Layout structure. A ScrollContainer's children are five named region
    // slots — `region top { … }` — not a list; PLACEHOLDER here declares a
    // slot pages bind to as Module.Layout.<Name>, which is a different thing
    // from the page-side placeholderBlock that fills one.
    | SCROLLCONTAINER
    | SCROLLREGION
    | NAVIGATIONTREE
    | MENUBAR
    | SIMPLEMENUBAR   // Atlas phone bottom bar: renders a menu document (#573)
    | SIDEBARTOGGLE   // Atlas's responsive sidebar toggle button (mendixlabs/mxcli#1220)
    | LAYOUTHEADER    // Forms$Header: a phone/tablet layout's header bar, `region left|right { … }`
    // Object-list container keywords for pluggable widgets (Phase 1 — #538).
    // Each is the singular form of a Type:"object"+IsList:true widget property
    // (e.g. Accordion groups → GROUP). Routed at executor time via the parent
    // widget's def.json `objectLists` mapping; unrecognized parents fall back
    // to generic widget handling.
    | GROUP
    | CUSTOMITEM
    | MARKER
    | DYNAMICMARKER
    | SERIES
    | LINE
    | SCALECOLOR
    | CUSTOMBUTTON
    | ALLOWEDFILEFORMAT
    // Dual-stack keyword (Phase 2 — #539). LEGACYDATAGRID always routes to
    // the dojo-based native Forms$DataGrid even on Mendix 11+; useful for
    // migrated projects that still have native datagrids on the page.
    | LEGACYDATAGRID
    // Any widget with a definition, named by its MDL name — `htmlelement frame
    // (...)`, `fileuploader up (...)`. Slice 2 of
    // PROPOSAL_def_driven_widget_bodies.md (mendixlabs/mxcli#1036).
    //
    // The list above was never a capability boundary: cmd_pages_builder_v3.go's
    // default branch already resolves widgetRegistry.Get(ToUpper(w.Type)) FIRST,
    // and every .def.json declares an mdlName. Only ANTLR needed a token, so a
    // widget mxcli could build was one MDL could not spell.
    //
    // ORDERED LAST so every enumerated type keeps winning its own alternative,
    // and the widget's NAME is still a direct IDENTIFIER child of widgetV3 —
    // this one is nested inside widgetTypeV3, so wCtx.IDENTIFIER() is unaffected.
    //
    // An unknown name is no longer a parse error; it is MDL-WIDGET25, which
    // slice 0 added for exactly this reason.
    | IDENTIFIER
    // Slice 3: the same, for a container whose name lexes as a KEYWORD token.
    // This is not defensive — it is the case that motivated the whole issue.
    // `attribute` lexes as ATTRIBUTE and never as IDENTIFIER, so the
    // alternative above cannot match `attribute a1 (...)`, which is the HTML
    // Element object list the reporter could not write.
    | keyword
    ;

// V3 Widget properties: (Prop: Value, Prop: Value)
// The list may be EMPTY. `container c ()` is what an LLM writes when a widget
// needs no properties, and rejecting it gave a parse error at the `)` that read
// as though the widget itself were wrong. Bare `container c` already parsed, so
// this only removes an arbitrary difference between two spellings of the same
// thing (mendixlabs/mxcli#1036).
widgetPropertiesV3
    : LPAREN (widgetPropertyV3 (COMMA widgetPropertyV3)*)? RPAREN
    ;

widgetPropertyV3
    : DATASOURCE COLON dataSourceExprV3               // DataSource: $var | DATABASE Entity | MICROFLOW ...
    | ATTRIBUTE COLON attributePathV3                 // Attribute: Name | Product/Category
    | ATTRIBUTE COLON widgetAttributeRefV3            // Attribute: $dataView1.Name — read through a data view (#826)
    | BINDS COLON attributePathV3                     // Binds: (deprecated, use Attribute:)
    | ACTION COLON actionExprV3                       // Action: SAVE_CHANGES | SHOW_PAGE ...
    | ONCLICK COLON actionExprV3                      // OnClick: MICROFLOW ... (alias of Action: — e.g. clickable CONTAINER, issue #603)
    | ONCHANGE COLON actionExprV3                     // OnChange: MICROFLOW ... (input widgets — TextBox/TextArea/DatePicker/CheckBox)
    | PLACEHOLDER COLON STRING_LITERAL               // Placeholder: 'Search all articles' (input widgets)
    | CAPTION COLON stringExprV3                      // Caption: 'Save'
    | LABEL COLON STRING_LITERAL                      // Label: 'Name'
    | ATTR COLON attributePathV3                      // Attr: (deprecated, use Attribute:)
    | CONTENT COLON stringExprV3                      // Content: 'Hello {1}'
    | RENDERMODE COLON renderModeV3                   // RenderMode: H3
    | CONTENTPARAMS COLON paramListV3                 // ContentParams: ({1} = $var.Name)
    | CAPTIONPARAMS COLON paramListV3                 // CaptionParams: ({1} = 'hello')
    // A text-template sub-property of an object-list ITEM carries its
    // parameters under `<Name>Params`, and those names are the widget's own
    // (a File Uploader custom button's `ButtonCaptionParams`), so they cannot
    // each have a token. Placed before the generic propertyValueV3
    // alternatives, which also admit a `[...]` array — `{N} = expr` inside is
    // what separates them (#956).
    | (IDENTIFIER | keyword) COLON paramListV3        // <Name>Params: ({1} = Attr)
    | BUTTONSTYLE COLON buttonStyleV3                  // ButtonStyle: Primary
    | ICON COLON widgetIconV3                          // Icon: 'Atlas_Core.Atlas_Filled.pencil' | image Mod.Images.logo | glyph 57377
    | CLASS COLON STRING_LITERAL                       // Class: 'my-class'
    | STYLE COLON STRING_LITERAL                       // Style: 'color: red'
    | DESKTOPWIDTH COLON desktopWidthV3               // DesktopWidth: 6 | AutoFill | AutoFit
    | TABLETWIDTH COLON desktopWidthV3                // TabletWidth: 6 | AutoFill | AutoFit
    | PHONEWIDTH COLON desktopWidthV3                 // PhoneWidth: 12 | AutoFill | AutoFit
    | SELECTION COLON selectionModeV3                 // Selection: Single | Multiple
    | SNIPPET COLON qualifiedName                     // Snippet: Module.SnippetName
    | PARAMS COLON snippetCallParamListV3             // Params: (Asset = $var) — snippet call arguments
    | ATTRIBUTES COLON attributeListV3                // Attributes: [Entity.Attr1, Entity.Attr2]
    | FILTERTYPE COLON filterTypeValue                // FilterType: startsWith | contains | equal
    | DESIGNPROPERTIES COLON designPropertyListV3       // DesignProperties: ( 'Key': 'Value', … )
    | WIDTH COLON NUMBER_LITERAL                        // Width: 200
    | HEIGHT COLON NUMBER_LITERAL                      // Height: 100
    // R5 (ako/mxcli#753): a conditional Visible / Editable is a client
    // expression, written bare like every other expression and stored as
    // written. The bracketed form is the deprecated alias; it roots a bare
    // attribute in $currentObject on the way in. The plain values keep their
    // alternative, ahead of the expression, so `Visible: false` and `Editable:
    // Never` mean what they did.
    | VISIBLE COLON xpathConstraint /* @alias MDL-DEPR081 */  // Visible: [IsActive = true]
    | VISIBLE COLON qualifiedName IN LPAREN visibleValueV3 (COMMA visibleValueV3)* RPAREN  // Visible: Status in (Running, empty) | Mod.Entity.Attr in (…)
    | VISIBLE COLON widgetAttributeRefV3 IN LPAREN visibleValueV3 (COMMA visibleValueV3)* RPAREN  // Visible: $Param.Status in (Running) — read from a page/snippet parameter
    // Studio Pro's "Visible for module roles" (ConditionalVisibilitySettings.
    // ModuleRoles, by-name references). Independent of — and combinable with —
    // the expression or attribute condition: a widget may carry both `Visible:`
    // lines (mendixlabs/mxcli#1460). Ahead of the expression alternative, which
    // would otherwise read `roles (…)` as a function call.
    | VISIBLE COLON ROLES LPAREN visibleRoleListV3 RPAREN  // Visible: roles (Mod.Admin, Mod.User)
    | VISIBLE COLON propertyValueV3                   // Visible: false
    | VISIBLE COLON expression                        // Visible: $currentObject/Status = 'Open'
    | EDITABLE COLON xpathConstraint /* @alias MDL-DEPR081 */ // Editable: [Status != 'Closed']
    | EDITABLE COLON propertyValueV3                  // Editable: Never | Always
    | EDITABLE COLON expression                       // Editable: $currentObject/Status != 'Closed'
    | TOOLTIP COLON propertyValueV3                   // Tooltip: 'text'
    // Generic datasource-typed property (e.g. chart series `staticDataSource:
    // database Module.View`, `dynamicDataSource: $var`). Placed before the
    // scalar generic branches; the token after COLON (DATABASE/MICROFLOW/
    // NANOFLOW/ASSOCIATION/VARIABLE/SELECTION) disambiguates it from
    // propertyValueV3, which can never start with those. Issue: chart series (9a).
    | (IDENTIFIER | keyword) COLON dataSourceExprV3
    // Generic action-typed property — a NAMED action slot addressed by the
    // widget's own key: `createFileAction: show_page Module.P`. Placed AFTER the
    // datasource branch on purpose: actionExprV3 and dataSourceExprV3 overlap on
    // MICROFLOW / NANOFLOW / VARIABLE, and putting this first would read a chart
    // series' `staticDataSource: microflow M.X` as an action. Those overlapping
    // forms therefore still parse as a data source, and the executor converts
    // them when the widget definition says the slot is action-typed — the same
    // split fragmentArgValue already resolves ("the executor disambiguates using
    // the parameter's declared kind"). This branch carries the forms that are
    // unambiguous: show_page, save_changes, close_page, create_object, delete,
    // open_link, sign_out, complete_task. Issue #956.
    | (IDENTIFIER | keyword) COLON actionExprV3
    | IDENTIFIER COLON propertyValueV3                // Generic: any other property
    | keyword COLON propertyValueV3                  // Generic: keyword as property name (for pluggable widgets)
    // A Mendix expression, written as-is: `dynamicclasses: if $currentObject/F
    // then 'a' else ''`. LAST, so every value form above keeps its parse and only
    // what they all reject reaches it. The visitor accepts it only for the
    // expression-typed properties (DynamicClasses, a column's DynamicCellClass)
    // and refuses it elsewhere, so no plain property can read it as empty
    // (PROPOSAL_first_class_expressions.md, slice 2).
    | (IDENTIFIER | keyword) COLON expression
    ;


// Mendix stores three DIFFERENT icon elements on a widget, exactly as it does on
// a navigation menu item (navMenuIcon in MDLParser.g4), and they are not
// variants of one value: an icon-collection icon and an image icon each hold a
// qualified name — into an icon collection and an image collection, which are
// different documents — while a glyph icon holds a numeric character code and no
// name at all.
//
//   Icon: 'Atlas_Core.Atlas_Filled.pencil'   Forms$IconCollectionIcon
//   Icon: image MyModule.Images.logo         Forms$ImageIcon
//   Icon: glyph 57377                        Forms$GlyphIcon
//
// Only the first was expressible, so a stored image icon came back out of
// DESCRIBE spelled like a collection reference and re-executed as one —
// CE1613 "The selected custom icon … no longer exists" — and a glyph icon was
// emitted as nothing at all and deleted on replay (mendixlabs/mxcli#1059).
//
// The bare form stays the collection icon, so every existing script means what
// it did. Both the quoted and unquoted spellings are accepted: `Icon:` has
// carried a quoted string since #602, while a reference into the model is
// spelled as a qualifiedName everywhere else (ADR-0003). An Atlas icon name
// carries hyphens, which IDENTIFIER cannot lex, so those segments are
// double-quoted per segment: image Mod.Images."my-logo".
//
// The keyword-led alternatives come FIRST. qualifiedName accepts a keyword as a
// name segment (identifierOrKeyword), so `image Mod.Images.logo` also matches
// the bare form with `image` read as the first segment of the name; listing the
// specific alternatives ahead of the general one is what settles it. Same trap,
// same remedy, as navMenuIcon.
widgetIconV3
    : GLYPH NUMBER_LITERAL
    | IMAGE (qualifiedName | STRING_LITERAL)
    | qualifiedName
    | STRING_LITERAL
    ;

// Filter type values - handle keywords like CONTAINS that are also filter types
filterTypeValue
    : CONTAINS      // contains
    | EMPTY         // empty
    | IDENTIFIER    // startsWith, endsWith, greater, greaterEqual, equal, notEqual, smaller, smallerEqual, notEmpty
    ;

// Snippet call arguments: (Asset = $var, Other = $other). A snippet call is a
// call site, so it binds its arguments the way every call does, `Param = value`
// (R4), in the ( ) of a property map (R2, ako/mxcli#754). The old spelling was
// a brace map `{$Asset: $var}`.
snippetCallParamListV3
    : LPAREN snippetCallArgV3 (COMMA snippetCallArgV3)* COMMA? RPAREN
    | LBRACE /* @alias MDL-DEPR126 */ snippetCallParamMappingV3 (COMMA snippetCallParamMappingV3)* RBRACE
    ;

snippetCallArgV3
    : parameterName EQUALS VARIABLE
    ;

snippetCallParamMappingV3
    : (identifierOrKeyword | VARIABLE) COLON VARIABLE
    ;

// V3 Attribute list for filter widgets
attributeListV3
    : LBRACKET qualifiedName (COMMA qualifiedName)* RBRACKET
    ;

// V3 DataSource expressions
//
// `database from $ctx/Assoc/Entity` is a DATABASE retrieve reached over an
// association from a context object (a Forms$ListViewXPathSource whose EntityRef
// is an IndirectEntityRef): it keeps its XPath, sort and search. The bare
// `$ctx/Assoc` is an ASSOCIATION source, an in-memory retrieve with none of them.
// Studio Pro distinguishes the two, so MDL does (ako/mxcli#721 L5).
dataSourceExprV3
    : VARIABLE SLASH associationPathV3                // $currentObject/Module.Assoc (ByAssociation — sugar for ASSOCIATION)
    | VARIABLE                                        // $ParamName
    | DATABASE FROM? (qualifiedName | VARIABLE SLASH associationPathV3) // DATABASE [FROM] Entity|$ctx/Assoc/Entity [WHERE ...] [SORT BY ...]
      (WHERE (xpathConstraint (andOrXpath? xpathConstraint)* | expression))?
      (SORT_BY sortColumn (COMMA sortColumn)*)?
      (SEARCH_BY searchAttribute (COMMA searchAttribute)*)?
    | MICROFLOW qualifiedName microflowArgsV3? actionSettingsV3? // MICROFLOW Module.Flow [with (FormValidations: None)]
    | NANOFLOW qualifiedName microflowArgsV3?         // NANOFLOW Module.Flow
    | ASSOCIATION associationPathV3                   // ASSOCIATION Module.Assoc (explicit form)
    | ASSOCIATION VARIABLE SLASH associationPathV3    // ASSOCIATION $currentObject/Module.Assoc (keyword + sugar)
    | SELECTION (IDENTIFIER | QUOTED_IDENTIFIER)      // SELECTION widgetName ("name" if reserved)
    ;

// Association path: Module.Assoc or Module.Assoc/Module.Entity or multi-step
associationPathV3
    : qualifiedName (SLASH qualifiedName)*
    ;

// V3 Action expressions
//
// NOTHING is a real alternative, not a courtesy. It is the documented spelling
// for a deliberately inert button (docs-site/src/language/alter-page.md, the
// quick reference, the synced alter-page skill, nine mdl-examples scripts) and
// it was never in this rule: it reached Forms$NoAction by FAILING to match here
// and falling through to `keyword COLON propertyValueV3` at the end of
// widgetPropertyV3, which stores the slot as a plain string.
//
// That fall-through is what mendixlabs/mxcli#1062 reports: `Action: OPEN_LINK`
// (a real keyword short its argument) and `Action: TOTALLY_MADE_UP` take the
// same route to the same NoAction, silently. The scalar cannot be rejected
// while the documented form still depends on it, so the promotion below is the
// half of the fix that makes MDL-WIDGET28 possible.
actionExprV3
    : VARIABLE                                        // $handler — a fragment action parameter (see fragmentParam)
    | NOTHING                                         // NOTHING — an explicitly inert widget (Forms$NoAction)
    | SAVE_CHANGES closePageV3? actionSettingsV3?     // save changes [close page]
    | CANCEL_CHANGES closePageV3? actionSettingsV3?   // cancel changes [close page]
    | closePageV3 actionSettingsV3?                   // close page
    // `delete $grid1` deletes grid1's selection, `delete $Param` a page or
    // snippet parameter; bare `delete` the context object (ako/mxcli#1932).
    | DELETE VARIABLE? closePageV3? actionSettingsV3? // delete [$source] [close page]
    | DELETE_OBJECT /* @alias MDL-DEPR020 */ VARIABLE? closePageV3? actionSettingsV3?
    | CREATE OBJECT qualifiedName (THEN actionExprV3 | actionSettingsV3)? // create object Entity then show page ...
    | CREATE_OBJECT /* @alias MDL-DEPR020 */ qualifiedName (THEN actionExprV3 | actionSettingsV3)?
    | SHOW PAGE qualifiedName microflowArgsV3? actionSettingsV3?        // show page Module.Page (Param: val)
    | SHOW_PAGE /* @alias MDL-DEPR020 */ qualifiedName microflowArgsV3? actionSettingsV3?
    | CALL MICROFLOW qualifiedName microflowArgsV3? actionReturnsV3? actionSettingsV3?   // call microflow Module.Flow
    | MICROFLOW /* @alias MDL-DEPR020 */ qualifiedName microflowArgsV3? actionReturnsV3? actionSettingsV3?
    | CALL NANOFLOW qualifiedName microflowArgsV3? actionReturnsV3? actionSettingsV3?    // call nanoflow Module.Flow
    | NANOFLOW /* @alias MDL-DEPR020 */ qualifiedName microflowArgsV3? actionReturnsV3? actionSettingsV3?
    | openLinkV3 STRING_LITERAL actionSettingsV3?                       // open link 'https://...'
    | openLinkV3 VARIABLE SLASH attributePathV3 actionSettingsV3?       // open link $currentObject/URL (address read from an attribute)
    | SIGN_OUT actionSettingsV3?                                        // sign out
    | COMPLETE_TASK STRING_LITERAL actionSettingsV3?                    // complete task 'OutcomeName'
    // The workflow events Studio Pro offers beside "Complete user task", in the
    // words the microflow actions use (ako/mxcli#832). Without them describe
    // printed no action for a stored one and a rewrite wrote Forms$NoAction.
    | OPEN USER TASK actionSettingsV3?                                  // open user task (Show user task page)
    | OPEN WORKFLOW actionSettingsV3?                                   // open workflow (Show workflow admin page)
    | CALL WORKFLOW qualifiedName actionSettingsV3?                     // call workflow Module.Workflow
    ;

// The client-action settings Studio Pro shows under an event: "Disabled during
// action", and for a microflow or nanoflow call its progress bar, progress
// message and confirmation (ako/mxcli#721 L2). They are the action's own model
// properties, so they are a `( Key: value )` list (R2, R3), introduced by `with`
// because a bare `( … )` after `call microflow M.F` would read as its argument
// list. The keys are checked by the visitor, not here: an unknown key or one
// the action does not have is an error (R11).
//
//   call microflow M.Delete(Order = $currentObject) with (
//     ProgressBar: blocking, ProgressMessage: 'Deleting…',
//     Confirmation: 'Delete this order?', ProceedCaption: 'Delete', CancelCaption: 'Keep')
actionSettingsV3
    : WITH LPAREN actionSettingV3 (COMMA actionSettingV3)* COMMA? RPAREN
    ;

// A flow call's return value mappings (Mendix 11.5, Studio Pro's "Return value
// mappings"): `returns (Brand = $ActionReturnValue/Brand, $result = $ActionReturnValue,
// Brand = 'fixed')`. Target = source, like the argument list: the target is an
// attribute of the data container's object or a page variable, the source the
// return value, one of its attributes, or an expression (ako/mxcli#977).
actionReturnsV3
    : RETURNS LPAREN actionReturnV3 (COMMA actionReturnV3)* COMMA? RPAREN
    ;

actionReturnV3
    : (VARIABLE | parameterName) EQUALS expression
    ;

// The value is a qualifiedName so a setting can name a document (`open
// workflow with (DefaultPage: Module.Page)`); a single segment is the
// identifierOrKeyword every other setting takes.
actionSettingV3
    : identifierOrKeyword COLON (STRING_LITERAL | NUMBER_LITERAL | NONE | qualifiedName)
    ;

// The page actions are the words a microflow uses (R8, ako/mxcli#752):
// `show page`, `close page`, `create object`, `call microflow`, `open link`.
// The snake-case tokens are the deprecated second spellings; `save changes`,
// `cancel changes`, `sign out` and `complete task` are single lexer tokens that
// admit both (MDLLexer.g4).
closePageV3
    : CLOSE PAGE
    | CLOSE_PAGE /* @alias MDL-DEPR020 */
    ;

openLinkV3
    : OPEN LINK
    | OPEN_LINK /* @alias MDL-DEPR020 */
    ;

// V3 Microflow arguments: (Param = value, ...) — R4, the argument form of every
// call site. `Param: value` and `$Param = value` are deprecated spellings.
//
// The list may be EMPTY, and `M.F()` means exactly what `M.F` means — no
// mappings (ako/mxcli#1516). A flow body writes `call microflow M.F()`, so the
// same call on a button was written that way by analogy and was a syntax error
// at the `)`, reported as "expecting the start of a statement". This one rule
// serves every page action (call microflow/nanoflow, show page and their
// deprecated spellings), both flow data sources, and so every site that takes
// an action: Action/OnClick/OnChange, a pluggable widget's action property,
// alter page set, a navigation or menu document item, a fragment argument and
// a building-block override. describe keeps writing a parameterless call
// without the brackets.
microflowArgsV3
    : LPAREN (microflowArgV3 (COMMA microflowArgV3)*)? RPAREN
    ;

microflowArgV3
    : parameterName EQUALS expression                 // Param = $value (parameterName so a param named
                                                      // after a keyword — View/Source/Item/Page/Entity —
                                                      // works unquoted, matching callArgument)
    | identifierOrKeyword COLON /* @alias MDL-DEPR007 */ expression // Param: $value
    | VARIABLE /* @alias MDL-DEPR006 */ EQUALS expression           // $Param = $value
    | expression                                                    // positional: refused by the visitor (#569)
    ;

// A value in `Visible: Attr in (…)`: an enumeration value name, true/false,
// or `empty` for Studio Pro's "(empty)".
visibleValueV3
    : IDENTIFIER | QUOTED_IDENTIFIER | keyword
    ;

// V3 Attribute path: Name, Product/Category, "Order" (quoted to escape reserved words).
// An association hop — every segment before the last — may be module-qualified,
// `Module.Assoc/Attr`, the way XPath and data view sources name it. Without that
// the qualified form missed `Attribute: attributePathV3`, parsed as the division
// `Module.Assoc / Attr` and was refused as an expression (ChipCoV8 #10). The final
// segment stays a bare name: a dotted name with no slash is not a path.
attributePathV3
    : (attributePathHopV3 SLASH)* attributePathSegmentV3
    ;

attributePathHopV3
    : attributePathSegmentV3 (DOT attributePathSegmentV3)?
    ;

attributePathSegmentV3
    : IDENTIFIER | QUOTED_IDENTIFIER | keyword
    ;

// An attribute read from a named object: an enclosing data view, Studio Pro's
// widget-scoped SourceVariable {Widget: dataView1, …} (ako/mxcli#826), or a page
// or snippet parameter, {SnippetParameter|PageParameter: Param}. The same
// `$name.Attr` spelling a text template parameter uses. The optional second
// segment is a module-qualified association: `$Param.Module.Assoc`.
widgetAttributeRefV3
    : VARIABLE DOT (IDENTIFIER | QUOTED_IDENTIFIER | keyword) (DOT (IDENTIFIER | QUOTED_IDENTIFIER | keyword))?
    ;

// V3 String expression (may include template placeholders or attribute binding)
stringExprV3
    : STRING_LITERAL
    | attributePathV3
    | VARIABLE (DOT (IDENTIFIER | keyword))?
    ;

// V3 Parameter list: ({1} = value, {2} = value). The parameters of a text
// template are a map, so they are in ( ) (R2, ako/mxcli#754), and each binds a
// runtime value with `=` (R4, `with ({1} = …)`). `[…]` is the old spelling.
paramListV3
    : LPAREN paramAssignmentV3 (COMMA paramAssignmentV3)* COMMA? RPAREN
    | LBRACKET /* @alias MDL-DEPR124 */ paramAssignmentV3 (COMMA paramAssignmentV3)* RBRACKET
    ;

// `{1} = <unbound>` keeps a parameter with no binding — what `describe` prints
// for one Studio Pro stored unbound, typically in a building block, and what
// writes it back that way (ako/mxcli#2181). mxbuild reports it until bound.
paramAssignmentV3
    : LBRACE NUMBER_LITERAL RBRACE EQUALS (expression | UNBOUND_PARAM) (FORMAT paramFormatV3)?
    ;

// Optional per-parameter formatting for a dynamic-text parameter, mapping to the
// Mendix ClientTemplateParameter FormattingInfo (decimalPrecision, groupDigits,
// dateFormat, customDateFormat, enumFormat). The FORMAT keyword introduces the
// block — a bare `(…)` after the expression is ambiguous with a function call
// because COLON is a valid expression (OQL division) operator.
//   {1} = Amount FORMAT (decimalPrecision: 2, groupDigits: true)
paramFormatV3
    : LPAREN paramFormatPropV3 (COMMA paramFormatPropV3)* RPAREN
    ;

paramFormatPropV3
    : IDENTIFIER COLON propertyValueV3
    ;

// V3 Render modes
renderModeV3
    : H1 | H2 | H3 | H4 | H5 | H6 | PARAGRAPH | TEXT | IDENTIFIER
    ;

// V3 Button styles
buttonStyleV3
    : PRIMARY | DEFAULT | SUCCESS | DANGER | WARNING | WARNING_STYLE | INFO | INFO_STYLE | IDENTIFIER
    ;

// V3 Desktop width
desktopWidthV3
    : NUMBER_LITERAL | AUTOFILL | AUTOFIT   // AutoFit = "Auto-fit content" (-2)
    ;

// V3 Selection mode
selectionModeV3
    : SINGLE | MULTIPLE | NONE
    ;

// V3 Generic property value
// The module roles of `Visible: roles (…)`, a rule of its own so the
// widgetPropertyV3 context keeps its single qualifiedName accessor.
visibleRoleListV3
    : qualifiedName (COMMA qualifiedName)*
    ;

propertyValueV3
    : STRING_LITERAL
    | QUOTED_IDENTIFIER              // "AttrName" — quoted attribute value for pluggable sub-props (chart series staticXAttribute)
    | NUMBER_LITERAL
    | MINUS NUMBER_LITERAL           // DecimalPrecision: -1 — Studio Pro's "no fixed precision" (mendixlabs/mxcli#1407)
    | booleanLiteral
    | qualifiedName
    | IDENTIFIER
    | H1 | H2 | H3 | H4 | H5 | H6  // HeaderMode values
    | objectEntryListV3            // [(k: v, k: v)] — see below; parsed only to be REJECTED
    | LBRACKET (expression (COMMA expression)*)? RBRACKET  // Array
    ;

// `attributes: [(attributeName: 'x', attributeValueType: 'expression')]` —
// a repeatable widget property written as a property VALUE.
//
// This is not how MDL writes an object list. The entries are CONTAINER BLOCKS in
// the widget body:
//
//     htmlelement frame (tagName: 'div') {
//       attribute a1 (attributeName: 'x', attributeValueType: 'expression')
//     }
//
// The alternative exists ONLY so the mistake is reported precisely
// (MDL-WIDGET27), never to give the construct a second spelling. Without it the
// two shapes failed differently and both badly (mendixlabs/mxcli#999):
//
//   single key  [(configMode: simple)]                 parsed as a list of
//                                                      EXPRESSIONS, checked
//                                                      clean, exec'd, and was
//                                                      silently discarded
//   multi key   [(configMode: simple, x: y)]           died as
//                                                      `missing ')' at ','`
//
// Ordered BEFORE the expression array so the single-key shape lands here rather
// than being flattened to a string nobody claims. Measured: `[(` appears nowhere
// in mdl-examples/ outside comments, so nothing legitimate is captured.
objectEntryListV3
    : LBRACKET objectEntryV3 (COMMA objectEntryV3)* RBRACKET
    ;

objectEntryV3
    : LPAREN objectEntryFieldV3 (COMMA objectEntryFieldV3)* RPAREN
    ;

objectEntryFieldV3
    : identifierOrKeyword COLON propertyValueV3
    ;

// V3 Design property list: ('Key': 'Value', 'Key': on). A map of properties, so
// it is in ( ) (R2, ako/mxcli#754); `['Key': 'Value']` is the old spelling.
designPropertyListV3
    : LPAREN (designPropertyEntryV3 (COMMA designPropertyEntryV3)* COMMA?)? RPAREN
    | LBRACKET /* @alias MDL-DEPR125 */ designPropertyEntryV3 (COMMA designPropertyEntryV3)* RBRACKET
    | LBRACKET /* @alias MDL-DEPR125 */ RBRACKET
    ;

designPropertyEntryV3
    : STRING_LITERAL COLON STRING_LITERAL
    | STRING_LITERAL COLON ON
    | STRING_LITERAL COLON OFF
    | STRING_LITERAL COLON designPropertyListV3   // compound: 'Spacing': ('margin-top': 'Large', ...)
    ;

// V3 Widget body: { children }
widgetBodyV3
    : LBRACE pageBodyV3 RBRACE
    ;

