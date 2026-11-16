// SPDX-License-Identifier: Apache-2.0

// Package deprecation is the single registry of deprecated MDL spellings
// (ADR-0011, decision 1).
//
// A deprecated spelling is a respelling: it means exactly what its canonical
// form means, so it keeps parsing, `check` and `exec` warn on it, and
// `mxcli fmt --upgrade` can rewrite it mechanically. A spelling that means
// something different from the proposed canonical form is NOT an alias there,
// and is not reported — rewriting it would silently change a script.
//
// # How an alias is marked
//
// Every grammar token or alternative that exists only as an alias carries a
// block comment naming its registry code, next to the alias itself:
//
//	CREATE (OR (MODIFY | REPLACE /* @alias MDL-DEPR001 */))?
//
// ANTLR ignores the comment. TestGrammarAliasesAreRegistered (mdl/grammar)
// reads the .g4 sources and fails when a marker names a code with no entry
// here, or an entry is named by no marker. The marker is what makes a missing
// entry detectable: an alias is marked in the edit that adds it, and the marker
// cannot be satisfied without an entry.
//
// # How a use is detected
//
// Both spellings build the same AST, so the parse tree is the only place the
// source spelling is still visible. The visitor records every use of a
// registered spelling on ast.Program.Deprecations, and the executor's
// ValidateDeprecations turns the records into warnings (errors under
// --deprecations=error). This generalises MDL065, where the AST node itself
// carries the spelling flags.
package deprecation

import (
	"fmt"
	"strings"
)

// Entry is one deprecated spelling.
type Entry struct {
	// Code is the stable warning code, MDL-DEPRnnn. Never reused.
	Code string
	// Old is the deprecated form, as a reader would write it.
	Old string
	// Canonical is the form that replaces it (ADR-0010).
	Canonical string
	// Rewrite is the mechanical rewrite `fmt --upgrade` applies.
	Rewrite Rewrite
	// RemovedIn is the MDL language version (the `mdl <n>;` header) under which
	// the old form is refused. Under earlier versions it warns (ADR-0011).
	RemovedIn int
	// Note is shown with the warning: scope limits, or where the canonical form
	// is expected to move next.
	Note string
	// Example is a complete statement in the old form. A test parses it and
	// requires exactly this code to be recorded.
	Example string
	// CanonicalExample is Example after Rewrite. A test requires it to record no
	// deprecation and to build the same AST as Example — the proof that the
	// rewrite does not change meaning.
	CanonicalExample string
}

// Rewrite is the mechanical rewrite from the deprecated form to the canonical
// one. It is either a keyword swap (Token and Replacement) or, where the two
// forms differ in shape rather than in one word, a structural rewrite that
// Structural names. A structural rewrite is implemented against the parse tree,
// never as text substitution, and its correctness rests on the same test as a
// swap: Example and CanonicalExample must build the same statements.
type Rewrite struct {
	// Token is the keyword to replace, lower-case.
	Token string
	// Replacement is the keyword written in its place, lower-case.
	Replacement string
	// Structural describes a rewrite that is not a keyword swap, e.g.
	// "call form to statement form". Empty for a keyword swap.
	Structural string
}

// IsZero reports whether r is empty: the entry has no mechanical rewrite, and
// `fmt --upgrade` reports its uses instead of guessing at one. A structural
// rewrite is computed per use by the visitor that records it (ast.Fix), and a
// use it cannot rewrite is reported the same way.
func (r Rewrite) IsZero() bool { return r.Token == "" && r.Replacement == "" && r.Structural == "" }

// Codes of the registered entries, for the visitor to record.
const (
	CreateOrReplace = "MDL-DEPR001"
	Show            = "MDL-DEPR002"
	// ListOperationFunctionForm is `$x = head($L)` and the other list
	// operations written as calls; find and contains are excluded, because the
	// call form clashes with the string functions (see mdl/visitor, MDL-V1-LIST).
	ListOperationFunctionForm = "MDL-DEPR003"
	// AggregateFunctionForm is `$n = count($L)` and the other aggregates
	// written as calls.
	AggregateFunctionForm = "MDL-DEPR004"
	// UnstoredWidgetName is a name written on a page element Mendix stores no
	// name for: a layout grid's rows and columns, a data grid's columns and
	// control bar, a gallery's template and filter (R12, ako/mxcli#749), a
	// data view's footer (ako/mxcli#528).
	UnstoredWidgetName = "MDL-DEPR005"

	// R10: document types named as Studio Pro names them (ako/mxcli#755). The
	// codes are a block of their own so parallel work does not collide.
	ConsumedRestService   = "MDL-DEPR550" // rest client -> consumed rest service
	ConsumedODataService  = "MDL-DEPR551" // odata client -> consumed odata service
	PublishedODataService = "MDL-DEPR552" // odata service -> published odata service
	TaskQueue             = "MDL-DEPR553" // queue -> task queue
	AppSecurity           = "MDL-DEPR554" // project security -> app security
	SettingsRuntime       = "MDL-DEPR555" // alter settings model -> alter settings runtime
	// ReversedEntityGrant is `grant M.Role on M.E (rights) where '…'`, the only
	// grant with the role first and the XPath in a string (R5, ako/mxcli#753).
	// Codes 006-029 are taken by the other wave-2 changes in flight.
	ReversedEntityGrant = "MDL-DEPR030"
	// QuotedTargetingXPath is a workflow user task's `targeting xpath '…'`,
	// the XPath in a string instead of in [ ] (R5, ako/mxcli#753).
	QuotedTargetingXPath = "MDL-DEPR031"

	// Codes 020–029 are R8's (ako/mxcli#752, PROPOSAL_mdl_beta_syntax_freeze.md
	// §3 R8): words, not SCREAMING_SNAKE, and one spelling per keyword. They
	// start at 020 rather than 006 because the other phase-3 issues add entries
	// in parallel; a gap in the numbering means nothing.

	// PageActionWord is a page action written as one snake-case token —
	// `show_page`, `save_changes`, `close_page`, `create_object`,
	// `delete_object`, `open_link`, `sign_out`, `complete_task`,
	// `cancel_changes` — or a flow call without `call` (`microflow M.F`).
	PageActionWord = "MDL-DEPR020"
	// ErrorMessageKeyword is the user-facing message of a validation, spelled
	// anything but `error message`: `not null error '…'` (also after unique
	// and required), a validation rule's `feedback '…'`, and `error_message` /
	// `errormessage`.
	ErrorMessageKeyword = "MDL-DEPR021"
	// DeleteBehaviorClause is an association's `delete_behavior <behaviour>`
	// clause, in any of its spellings; `on delete …` says the same thing.
	DeleteBehaviorClause = "MDL-DEPR022"
	// ReferenceSetUnderscore is `reference_set` for the `ReferenceSet` type.
	ReferenceSetUnderscore = "MDL-DEPR023"
	// ReturnsNone is a REST call's `returns none`, the second spelling of
	// `returns nothing`.
	ReturnsNone = "MDL-DEPR024"
	// OnErrorBraces is a custom error handler written `on error { … }`: the
	// only brace block inside a microflow, where flow is `begin … end <keyword>`
	// (R2, ako/mxcli#754).
	OnErrorBraces = "MDL-DEPR540"
	// DollarArgumentName is `$Param = expr` at a call site: the parameter
	// named with the `$` of a variable (R4, ako/mxcli#751).
	DollarArgumentName = "MDL-DEPR006"
	// ColonArgument is `Param: expr` at a call site (`show page`, `call
	// microflow`/nanoflow/java action/…, a page action or data source): `:` sets
	// a model property, `=` binds a value (R3/R4, ako/mxcli#751, #533).
	ColonArgument = "MDL-DEPR007"
	// WorkflowStringArgument is a workflow call's `with (Param = '<expr>')`:
	// the argument expression written inside a string (R4, ako/mxcli#751).
	WorkflowStringArgument = "MDL-DEPR008"
	// PositionalTemplateArguments is `objects [a, b]` / `parameters [a, b]`
	// on a text template: the placeholders bound by position (R4,
	// ako/mxcli#751).
	PositionalTemplateArguments = "MDL-DEPR009"

	// R9 (ako/mxcli#755): where document metadata lives. Codes 100–119 are
	// this change's block; 101–103 are left to the parallel work that took them.

	// DocumentationClause is the `comment '…'` clause on a constant,
	// association, JSON structure or image collection: the document's
	// documentation, which every other document takes as a `/** … */` doc
	// comment.
	DocumentationClause = "MDL-DEPR100"
	// WorkflowCommentCaption is a workflow activity's `comment '…'`, which sets
	// the activity's caption, not a comment.
	WorkflowCommentCaption = "MDL-DEPR104"
	// FolderProperty is `Folder: '…'` in a page, snippet, consumed REST
	// service, consumed or published OData service or published REST service
	// header: the folder is a clause after the name on every document.
	FolderProperty = "MDL-DEPR105"
	// DocumentationProperty is `Documentation: '…'` in a regular expression,
	// task queue or scheduled event property list.
	DocumentationProperty = "MDL-DEPR106"

	// R6: one verb per job (ako/mxcli#755). A block of their own, 090-099, so
	// the parallel phase-3 changes do not collide.

	// ShowSingleThing is `show` (or `list`) on a form that names one thing
	// whose describe is the same statement: page, app security, security
	// matrix, structure, context. Its canonical verb is `describe`.
	ShowSingleThing = "MDL-DEPR090"
	// UserRoleRemove is `alter user role … remove module roles`.
	UserRoleRemove = "MDL-DEPR091"
	// SettingsRemove is `alter settings language|workflows remove …`.
	SettingsRemove = "MDL-DEPR092"
	// ColumnForAttribute is `column` for `attribute` in alter entity.
	ColumnForAttribute = "MDL-DEPR093"
	// RestCall is the `rest call` microflow statement.
	RestCall = "MDL-DEPR094"
	// DescribeWidgetType is `describe widget <name>` for a widget definition.
	DescribeWidgetType = "MDL-DEPR095"
	// DefineFragment is `define fragment`.
	DefineFragment = "MDL-DEPR096"

	// Codes 130-139 finish R6 and R10 (ako/mxcli#755).

	// SingularCollectionList is `list image|icon|message definition
	// collection`: `list` names a plural.
	SingularCollectionList = "MDL-DEPR130"
	// AIModel is `model` for the agent editor's model document: Studio Pro
	// calls it an AI model (R10).
	AIModel = "MDL-DEPR131"
	// JSONStructureSample is a JSON structure's `snippet '…'`: the example
	// JSON is its sample, and `snippet` is a page document type (R10).
	JSONStructureSample = "MDL-DEPR132"
	// AppSecurityClause is `alter app security level|demo users|guest
	// access|strict mode …`: the clause forms of what is a property list.
	AppSecurityClause = "MDL-DEPR133"
	// FolderClausePosition is a `folder '…'` clause written anywhere but
	// right after the name: among a constant's trailing options, or after a
	// snippet's header (R9).
	FolderClausePosition = "MDL-DEPR134"
	// SetComment is `alter entity|association|enumeration … set comment '…'`:
	// it sets the element's documentation (R9).
	SetComment = "MDL-DEPR135"
	// ConstantClauses is a constant's `type T default v [exposed to client]`:
	// the clause form of its property list (phase 3.6).
	ConstantClauses = "MDL-DEPR136"
	// DemoUserClauses is a demo user's `password 'p' [entity E] (Role, …)`:
	// the clause form of its property list (phase 3.6).
	DemoUserClauses = "MDL-DEPR137"
	// ConstantPrivate is `create constant … private`: a modifier Mendix has no
	// property for, which was never stored (ako/mxcli#865). The one entry
	// refused from mdl 1 rather than 2, because mdl 1 never had it.
	ConstantPrivate = "MDL-DEPR138"
	// ODataAuthenticationClause is a published OData service's trailing
	// `authentication basic, session, …` clause: the `Authentication: ( … )`
	// property in the service's list (R9), which `alter … set ( … )` can set.
	ODataAuthenticationClause = "MDL-DEPR139"

	// Codes 160-169 are the migration aliases the beta dress rehearsal found
	// (ako/mxcli#714), numbered apart so the parallel fixes do not collide.

	// DateType is `date` as a type: Mendix has no date-only type, and mxcli
	// always stored it as a DateTime (ako/mxcli#706, rehearsal U1). Refused
	// from mdl 1 rather than 2, because mdl 1 never had it.
	DateType = "MDL-DEPR160"
	// RegexExportLevelPublic is a regular expression's `ExportLevel: Public`:
	// the metamodel's values are Hidden and API, and Public was written to the
	// model verbatim (ako/mxcli#827). It builds API.
	RegexExportLevelPublic = "MDL-DEPR161"

	// Codes 080–089 are the rest of R5 (ako/mxcli#753): expressions bare, one
	// constant reference, and the revoke that mirrors the grant.

	// WorkflowStringExpression is a workflow decision's condition, a timer's
	// delay or first execution time, or a due date written inside a string:
	// `decision '$WorkflowContext/Total > 1000'`.
	WorkflowStringExpression = "MDL-DEPR080"
	// BracketedWidgetCondition is a page widget's conditional `Visible: [expr]`
	// / `Editable: [expr]`: a client expression written in the brackets of an
	// XPath constraint.
	BracketedWidgetCondition = "MDL-DEPR081"
	// ReversedEntityRevoke is `revoke M.Role on M.E [(rights)]`, the revoke
	// with the role first — the mirror of MDL-DEPR030's grant.
	ReversedEntityRevoke = "MDL-DEPR082"
	// DollarConstant is a consumed REST service credential written `$Const`:
	// a constant named like a variable, and without its module.
	DollarConstant = "MDL-DEPR083"
	// BareConstantKey is an agent-editor model's or knowledge base's `Key:
	// Module.Const` (and `set Key = Module.Const`): a constant written as a
	// plain document name.
	BareConstantKey = "MDL-DEPR084"
	// QuotedSettingsConstant is `alter settings [drop] constant 'Module.Const'`:
	// a constant named in a string.
	QuotedSettingsConstant = "MDL-DEPR085"
	// QuotedVariableDefault is a page or snippet variable's default written in
	// a string: `$show: boolean = 'true'`.
	QuotedVariableDefault = "MDL-DEPR086"

	// Codes 070-079 are R2's integration documents (ako/mxcli#754): properties
	// in ( ), declarative children in { }.

	// RestOperationBraces is a consumed REST service's `operation X { … }`:
	// the operation's properties in braces.
	RestOperationBraces = "MDL-DEPR070"
	// AgentAttachmentBraces is an agent's `tool X { … }`, `mcp service M.S
	// { … }` or `knowledge base K { … }`: the attachment's properties in braces.
	AgentAttachmentBraces = "MDL-DEPR071"
	// ImageCollectionParens is an image collection's images in parentheses,
	// each written `image X from file '…'`.
	ImageCollectionParens = "MDL-DEPR072"
	// MessageTreeParens is a message definition collection's definitions and
	// member trees in parentheses.
	MessageTreeParens = "MDL-DEPR073"
	// AlterFlowFragmentBraces is an `alter microflow` / `alter nanoflow`
	// fragment in braces: `insert after $X { … }`, `replace … with { … }`.
	AlterFlowFragmentBraces = "MDL-DEPR074"

	// Codes 120-129 are the rest of R2 (ako/mxcli#754): navigation and menus,
	// property maps, and the database connection.

	// RestHeaderEquals is a consumed REST service operation's header written
	// `'Name' = value`: a header list is a map, `( 'Name': value )`.
	RestHeaderEquals = "MDL-DEPR120"
	// MenuChildrenParens is a navigation profile's `menu ( … )`, a menu
	// document's or a sub-menu's items in ( ), and the `;` after an item:
	// menu items are children, in { } with no separator.
	MenuChildrenParens = "MDL-DEPR121"
	// MenuItemClauses is a menu item's action or icon written as a clause after
	// its caption, `menu item 'X' page M.P icon I`, rather than in its property
	// list `( OnClick: show page M.P, Icon: I )`.
	MenuItemClauses = "MDL-DEPR122"
	// HeaderMapBraces is a page or snippet header's `Params: { … }` /
	// `Variables: { … }`: a map in braces.
	HeaderMapBraces = "MDL-DEPR123"
	// TemplateParamsBrackets is a text template's parameters in brackets,
	// `ContentParams: [{1} = …]` (also CaptionParams and `<Name>Params`).
	TemplateParamsBrackets = "MDL-DEPR124"
	// DesignPropertiesBrackets is `DesignProperties: ['Key': 'Value']`.
	DesignPropertiesBrackets = "MDL-DEPR125"
	// SnippetCallParamsBraces is a snippet call's `Params: {$P: $v}`.
	SnippetCallParamsBraces = "MDL-DEPR126"
	// DatabaseConnectionClauses is a database connection written as clauses
	// with a begin … end block of queries.
	DatabaseConnectionClauses = "MDL-DEPR127"

	// Codes 060-069 and 101-103 are R3's (ako/mxcli#751,
	// PROPOSAL_mdl_beta_syntax_freeze.md §3 R3): `:` sets a model property, so
	// an `alter` sets properties in create's `( Key: value, … )` list, and a
	// colon is written where a property list or an attribute definition has
	// one and nowhere else.

	// AlterPageSetEquals is the generic alter's `set Key = value` /
	// `set (Key = value, …)`. Numbered from 101 because it shipped with the
	// generic alter (ako/mxcli#712) before the registry existed; the code is
	// published, so it is kept.
	AlterPageSetEquals = "MDL-DEPR101"
	// AlterPageSetUnparenthesised is `set Key: value` without the list's
	// parentheses.
	AlterPageSetUnparenthesised = "MDL-DEPR102"
	// AlterPageDropWidget is `drop widget a, b`.
	AlterPageDropWidget = "MDL-DEPR103"
	// SettingsAssignment is a settings property written `Key = value`, outside
	// a list: `alter settings <section>`, `alter settings configuration` and
	// `create configuration`.
	SettingsAssignment = "MDL-DEPR060"
	// ODataAlterAssignment is `alter … odata service X set Key = value, …`.
	ODataAlterAssignment = "MDL-DEPR061"
	// StylingAssignment is `alter styling … set Class = 'x', 'Prop' = on`.
	StylingAssignment = "MDL-DEPR062"
	// AllowCreateChangeLocally is `alter entity … set allow_create_change_locally
	// = true`, the only snake-case `=` alter action.
	AllowCreateChangeLocally = "MDL-DEPR063"
	// AssociationClauseColon is `type: Reference` (also `owner:`, `storage:`)
	// on an association: a clause, which takes no colon.
	AssociationClauseColon = "MDL-DEPR064"
	// ModifyAttributeColon is `modify attribute A T`: an attribute definition
	// is always `Name: Type`.
	ModifyAttributeColon = "MDL-DEPR065"

	// Codes 710-719 are ako/mxcli#707's: describe output that did not re-parse
	// or lost data, where the fix needed a canonical form the old one lacked.

	// UserRolePositional is `create user role R (M.A, M.B) manage all roles`:
	// the module roles by position, with no slot for the role's description,
	// its check-security flag or its manageable roles.
	UserRolePositional = "MDL-DEPR710"
	// RestHeaderConcat is a consumed REST service header written
	// `'Bearer ' + $Token` or `$Token`: the value template `'Bearer {Token}'`
	// written as an expression.
	RestHeaderConcat = "MDL-DEPR711"

	// Codes 720-729 are ADR-0013's: a microflow activity's dialog settings in
	// one ( Key: value, … ) list after its main operand, instead of clauses.

	// RestCallClauses is `call rest service`'s `header N = v`, `auth basic $u
	// password $p`, `body …` and `timeout n` clauses: the settings list
	// `( Headers: (…), Authentication: basic (…), Body: …, Timeout: n )`.
	RestCallClauses = "MDL-DEPR720"
)

// entries is the registry. Append only: a code is never reused or renumbered,
// because scripts, CI allowlists and docs refer to it.
var entries = []Entry{
	{
		Code:      CreateOrReplace,
		Old:       "create or replace …",
		Canonical: "create or modify …",
		Rewrite:   Rewrite{Token: "replace", Replacement: "modify"},
		RemovedIn: 2,
		Note: "Not reported for `create or replace translations`, which replaces the " +
			"whole set. Under mdl 0 (no header) it is not reported for a view entity " +
			"(drops and recreates) or a user role / demo user (a plain create) either: " +
			"those warn MDL-V1-REPLACE01/02 instead, and are aliases from `mdl 1;` on.",
		Example:          "create or replace enumeration M.Color (Red 'Red');",
		CanonicalExample: "create or modify enumeration M.Color (Red 'Red');",
	},
	{
		Code:      Show,
		Old:       "show …",
		Canonical: "list …",
		Rewrite:   Rewrite{Token: "show", Replacement: "list"},
		RemovedIn: 2,
		Note: "Reported for plurals, relationship queries and the summary tables " +
			"(`show navigation [menu]`, `show settings`), whose canonical form is `list`. " +
			"Forms that name a single thing (`show page X`, `show project security`, …) " +
			"become `describe` (MDL-DEPR090); `show entity X` / `show association X` have no " +
			"mdl 1 statement (MDL-V1-SHOWSUMMARY); session state (`show version`, `show status`, " +
			"`show catalog status`) is a session command (MDL-V1-SESSION).",
		Example:          "show entities in M;",
		CanonicalExample: "list entities in M;",
	},
	{
		Code:      ListOperationFunctionForm,
		Old:       "$x = <operation>($List, …)",
		Canonical: "$x = <operation> $List …",
		Rewrite: Rewrite{Structural: "call form to statement form: head/tail $L; filter/find $L by Member = v " +
			"(when the condition has that shape) or where <expr>; sort $L by …; union/intersect $A with $B; " +
			"subtract($A, $B) -> subtract $B from $A; equals $A and $B; range($L, o, n) -> range $L offset o limit n"},
		RemovedIn: 2,
		Note: "A list operation is one Studio Pro activity whose operand is a variable, so the statement form " +
			"cannot nest. find(…) and contains(…) are not reported here: the call form is also the string " +
			"function, so they are version-gated instead (MDL-V1-LIST).",
		Example:          "create microflow M.F ($L: List of M.E) begin $H = head($L); end;",
		CanonicalExample: "create microflow M.F ($L: List of M.E) begin $H = head $L; end;",
	},
	{
		Code:      AggregateFunctionForm,
		Old:       "$n = <function>($List, …)",
		Canonical: "$n = <function> $List …",
		Rewrite: Rewrite{Structural: "call form to statement form: count $L; sum|average|minimum|maximum " +
			"$L by Attr (for $L.Attr) or of <expr>; all|any $L where <expr>; reduce $L from <initial> as <type> using <expr>"},
		RemovedIn:        2,
		Note:             "An aggregate is one Studio Pro Aggregate list activity whose operand is a variable.",
		Example:          "create microflow M.F ($L: List of M.E) begin $N = count($L); end;",
		CanonicalExample: "create microflow M.F ($L: List of M.E) begin $N = count $L; end;",
	},
	{
		Code:      UnstoredWidgetName,
		Old:       "row row1 { … } / column Name (…) — a name on an element Mendix stores none for",
		Canonical: "row { … } / column (…)",
		Rewrite:   Rewrite{Structural: "name out of the element: `row row1 {` becomes `row {`"},
		RemovedIn: 2,
		Note: "Mendix stores no name on a layout grid's row, a row's column, a data grid's column or control bar, " +
			"a gallery's template or filter, or a data view's footer, so the name was never written and describe no longer invents one. " +
			"A data grid column is addressed as `grid column(Attr)` or `grid column('Caption')`, a data view's footer as `dv.footer`.",
		Example:          "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { datagrid dg (DataSource: database from M.E) { column Name (Attribute: Name) } };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { datagrid dg (DataSource: database from M.E) { column (Attribute: Name) } };",
	},
	{
		Code:             ConsumedRestService,
		Old:              "rest client / rest clients",
		Canonical:        "consumed rest service / consumed rest services",
		Rewrite:          Rewrite{Structural: "document type name: `rest client` becomes `consumed rest service`, `rest clients` `consumed rest services`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls the document a consumed REST service (R10). Every statement that names the type takes both spellings: create, alter, drop, describe, list, move.",
		Example:          "drop rest client M.Api;",
		CanonicalExample: "drop consumed rest service M.Api;",
	},
	{
		Code:             ConsumedODataService,
		Old:              "odata client / odata clients",
		Canonical:        "consumed odata service / consumed odata services",
		Rewrite:          Rewrite{Structural: "document type name: `odata client` becomes `consumed odata service`, `odata clients` `consumed odata services`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls the document a consumed OData service (R10), including where an external entity names its source (`from consumed odata service M.Crm`).",
		Example:          "drop odata client M.Crm;",
		CanonicalExample: "drop consumed odata service M.Crm;",
	},
	{
		Code:             PublishedODataService,
		Old:              "odata service / odata services",
		Canonical:        "published odata service / published odata services",
		Rewrite:          Rewrite{Structural: "document type name: `odata service` becomes `published odata service`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls the document a published OData service (R10), including in `grant|revoke access on published odata service`.",
		Example:          "drop odata service M.Api;",
		CanonicalExample: "drop published odata service M.Api;",
	},
	{
		Code:             TaskQueue,
		Old:              "queue / queues",
		Canonical:        "task queue / task queues",
		Rewrite:          Rewrite{Structural: "document type name: `queue` becomes `task queue`, `queues` `task queues`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls the document a task queue (R10). `call microflow … in queue M.Q` is an option of the call, not a document type name, and is unchanged.",
		Example:          "drop queue M.Jobs;",
		CanonicalExample: "drop task queue M.Jobs;",
	},
	{
		Code:             AppSecurity,
		Old:              "alter project security …",
		Canonical:        "alter app security …",
		Rewrite:          Rewrite{Structural: "security name: `project security` becomes `app security`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls it App Security (R10). `show project security` becomes `describe app security` (MDL-DEPR090).",
		Example:          "alter project security ( EnableDemoUsers: false );",
		CanonicalExample: "alter app security ( EnableDemoUsers: false );",
	},
	{
		Code:             SettingsRuntime,
		Old:              "alter settings model …",
		Canonical:        "alter settings runtime …",
		Rewrite:          Rewrite{Structural: "section name: `model` becomes `runtime`"},
		RemovedIn:        2,
		Note:             "`runtime` is the App Settings tab that holds these values in Studio Pro (R10); `model` also collided with the agent-editor document type.",
		Example:          "alter settings model ( AfterStartupMicroflow: 'M.Startup' );",
		CanonicalExample: "alter settings runtime ( AfterStartupMicroflow: 'M.Startup' );",
	},
	{
		Code:      ReversedEntityGrant,
		Old:       "grant M.Role on M.E (rights) where '[xpath]'",
		Canonical: "grant rights on entity M.E to M.Role where [xpath]",
		Rewrite: Rewrite{Structural: "rights before `on entity`, roles after `to`, and the XPath out of its " +
			"string: `grant R on M.E (read *) where '[A = ''x'']'` becomes `grant read * on entity M.E to R where [A = 'x']`"},
		RemovedIn: 2,
		Note: "Every other grant names the right first and the role after `to`. XPath is written in [ ] " +
			"everywhere (R5), so the quotes inside it are no longer doubled. A string whose value is not " +
			"a bracketed XPath is left in place and reported by `fmt --upgrade`.",
		Example:          "grant M.User on M.Order (read *, write *) where '[Status = ''Open'']';",
		CanonicalExample: "grant read *, write * on entity M.Order to M.User where [Status = 'Open'];",
	},
	{
		Code:      QuotedTargetingXPath,
		Old:       "targeting [users|groups] xpath '[xpath]'",
		Canonical: "targeting [users|groups] xpath [xpath]",
		Rewrite:   Rewrite{Structural: "XPath out of its string: `xpath '[Name = ''Admin'']'` becomes `xpath [Name = 'Admin']`"},
		RemovedIn: 2,
		Note: "XPath is written in [ ] everywhere (R5), so the quotes inside it are no longer doubled. " +
			"A string whose value is not a bracketed XPath is left in place and reported by `fmt --upgrade`.",
		Example:          "alter workflow M.WF { set (Targeting: xpath '[Role = ''Manager'']') on 'Review'; };",
		CanonicalExample: "alter workflow M.WF { set (Targeting: xpath [Role = 'Manager']) on 'Review'; };",
	},
	{
		Code:      OnErrorBraces,
		Old:       "on error [without rollback] { … }",
		Canonical: "on error [without rollback] begin … end error",
		Rewrite:   Rewrite{Structural: "`{` becomes `begin` and the closing `}` becomes `end error`"},
		RemovedIn: 2,
		Note: "Braces hold declarative children (widgets, operations, menu items); imperative flow is " +
			"`begin … end <keyword>`, as for `if`, `loop` and `while` (R2).",
		Example:          "create microflow M.F ($O: M.E) begin commit $O on error without rollback { log warning 'x'; }; end;",
		CanonicalExample: "create microflow M.F ($O: M.E) begin commit $O on error without rollback begin log warning 'x'; end error; end;",
	},
	{
		Code:      DollarArgumentName,
		Old:       "call microflow M.F($Param = expr)",
		Canonical: "call microflow M.F(Param = expr)",
		Rewrite:   Rewrite{Structural: "parameter name without its `$` (quoted when it is not an identifier or keyword)"},
		RemovedIn: 2,
		Note: "Every call site binds an argument as `Param = expression` (R4): call microflow, nanoflow, java " +
			"action, javascript action, external action, web service operation, execute database query, " +
			"send rest request, show page, and page/button actions and data sources.",
		Example:          "create microflow M.F ($O: M.E) begin call microflow M.G($Order = $O); end;",
		CanonicalExample: "create microflow M.F ($O: M.E) begin call microflow M.G(Order = $O); end;",
	},
	{
		Code:      ColonArgument,
		Old:       "show page M.P(Param: expr)",
		Canonical: "show page M.P(Param = expr)",
		Rewrite:   Rewrite{Structural: "colon as `=`: `Param: expr` -> `Param = expr`"},
		RemovedIn: 2,
		Note: "`:` sets a model property and `=` binds a runtime value (R3). An argument binds a value, so " +
			"it takes `=` wherever the call appears: show page, call microflow/nanoflow/java action and the " +
			"other call statements, and page/button actions and data sources (`Action: microflow M.F(Param = expr)`).",
		Example:          "create microflow M.F ($O: M.E) begin show page M.P(Order: $O); end;",
		CanonicalExample: "create microflow M.F ($O: M.E) begin show page M.P(Order = $O); end;",
	},
	{
		Code:      WorkflowStringArgument,
		Old:       "call microflow M.F with (Param = '<expression>')",
		Canonical: "call microflow M.F(Param = <expression>)",
		Rewrite:   Rewrite{Structural: "string list as a list after the callee, each string's content written as the bare expression"},
		RemovedIn: 2,
		Note: "In a workflow. The string form keeps its meaning — its content is the expression — so it is an alias, not a " +
			"change of meaning. A string whose content does not parse as an MDL expression is left in place " +
			"and reported by fmt --upgrade.",
		Example: "create workflow M.W parameter $WorkflowContext: M.E begin " +
			"call microflow M.F with (Order = '$WorkflowContext'); end workflow;",
		CanonicalExample: "create workflow M.W parameter $WorkflowContext: M.E begin " +
			"call microflow M.F(Order = $WorkflowContext); end workflow;",
	},
	{
		Code:      PositionalTemplateArguments,
		Old:       "objects [$a, $b] / parameters ['a', 'b']",
		Canonical: "with ({1} = $a, {2} = $b)",
		Rewrite:   Rewrite{Structural: "positional list as numbered placeholders: `objects [a, b]` -> `with ({1} = a, {2} = b)`"},
		RemovedIn: 2,
		Note: "One text-template form everywhere: show message, validation feedback, log, and REST " +
			"url and body templates.",
		Example:          "create microflow M.F ($N: String) begin show message 'Hi {1}' type Information objects [$N]; end;",
		CanonicalExample: "create microflow M.F ($N: String) begin show message 'Hi {1}' type Information with ({1} = $N); end;",
	},
}

func init() {
	entries = append(entries, r8Entries...)
	entries = append(entries, r9Entries...)
	entries = append(entries, r6Entries...)
	entries = append(entries, r5Entries...)
	entries = append(entries, r2Entries...)
	entries = append(entries, r2RestEntries...)
	entries = append(entries, r3Entries...)
	entries = append(entries, issue707Entries...)
	entries = append(entries, headerPropertyEntries...)
}

// headerPropertyEntries are phase 3.6's (ako/mxcli#755): a document header
// written as clauses where every other header is a ( Key: value ) list, keyed
// by Studio Pro's property names. The user role's is MDL-DEPR710 (#707).
var headerPropertyEntries = []Entry{
	{
		Code:      ConstantClauses,
		Old:       "create constant M.C type T default v [exposed to client]",
		Canonical: "create constant M.C ( Type: T, DefaultValue: v, ExposedToClient: true )",
		Rewrite: Rewrite{Structural: "constant properties: `type T default v exposed to client` becomes " +
			"`( Type: T, DefaultValue: v, ExposedToClient: true )`"},
		RemovedIn: 2,
		Note: "The keys are Studio Pro's Constants$Constant property names. Type and DefaultValue are required, " +
			"as the clauses were; ExposedToClient defaults to false. The folder stays a clause after the name " +
			"and the documentation a doc comment (R9).",
		Example:          "create constant M.ApiUrl type String default 'https://x' exposed to client;",
		CanonicalExample: "create constant M.ApiUrl ( Type: String, DefaultValue: 'https://x', ExposedToClient: true );",
	},
	{
		Code:      ODataAuthenticationClause,
		Old:       "create published odata service M.S ( … ) authentication basic, session, microflow M.F",
		Canonical: "create published odata service M.S ( …, Authentication: (basic, session, microflow M.F) )",
		Rewrite: Rewrite{Structural: "published OData authentication: the trailing `authentication m1, m2` clause " +
			"becomes the last property, `Authentication: (m1, m2)`"},
		RemovedIn: 2,
		Note: "The methods keep their order. A clause naming a method MDL has no keyword for is reported but not " +
			"rewritten. The property also takes `none`, which the clause could not say, and `alter … set ( … )` " +
			"takes it.",
		Example:          "create published odata service M.S ( Path: 'odata/s/v1', Namespace: 'M' ) authentication basic, session;",
		CanonicalExample: "create published odata service M.S ( Path: 'odata/s/v1', Namespace: 'M', Authentication: (basic, session) );",
	},
	{
		Code:      ConstantPrivate,
		Old:       "create constant M.C … private",
		Canonical: "create constant M.C …",
		Rewrite:   Rewrite{Structural: "constant's `private` modifier away: it is deleted"},
		RemovedIn: 1,
		Note: "`private` was never stored: a Mendix constant has no such property, so it did nothing and " +
			"protected nothing. Keep a secret out of the model with `mxcli constant set Module.Name <value>`, " +
			"which stores the value on this machine only (gitignored), and leave DefaultValue empty.",
		Example:          "create constant M.ApiKey ( Type: String, DefaultValue: '' ) private;",
		CanonicalExample: "create constant M.ApiKey ( Type: String, DefaultValue: '' );",
	},
	{
		Code:      DemoUserClauses,
		Old:       "create demo user 'u' password 'p' [entity M.E] (Role, …)",
		Canonical: "create demo user 'u' ( Password: 'p', Entity: M.E, UserRoles: (Role, …) )",
		Rewrite: Rewrite{Structural: "demo user properties: `password 'p' entity M.E (R1, R2)` becomes " +
			"`( Password: 'p', Entity: M.E, UserRoles: (R1, R2) )`"},
		RemovedIn: 2,
		Note: "The keys are Studio Pro's Security$DemoUser property names. Password is required; UserRoles may be " +
			"empty or left out, which the clause form could not say. Without Entity the user entity is detected " +
			"from the project, as before.",
		Example:          "create demo user 'demo_admin' password 'Admin1!' entity Administration.Account (Administrator, User);",
		CanonicalExample: "create demo user 'demo_admin' ( Password: 'Admin1!', Entity: Administration.Account, UserRoles: (Administrator, User) );",
	},
}

// issue707Entries are ako/mxcli#707's: forms describe could not write, so its
// output lost what they had no slot for.
var issue707Entries = []Entry{
	{
		Code:      UserRolePositional,
		Old:       "create user role R (M.A, …) [manage all roles]",
		Canonical: "create user role R ( ModuleRoles: (M.A, …), ManageAllRoles: true, … )",
		Rewrite: Rewrite{Structural: "user role properties: the role list becomes `ModuleRoles: (…)` in a " +
			"( Key: value ) list, and `manage all roles` becomes `ManageAllRoles: true`"},
		RemovedIn: 2,
		Note: "The property list also takes Description, CheckSecurity, ManageableRoles and " +
			"ManageUsersWithoutRoles, and may be empty or left out: `create user role Guest;`.",
		Example:          "create user role Clerk (M.User, M.Viewer) manage all roles;",
		CanonicalExample: "create user role Clerk ( ModuleRoles: (M.User, M.Viewer), ManageAllRoles: true );",
	},
	{
		Code:      RestHeaderConcat,
		Old:       "Headers: ('Authorization': 'Bearer ' + $Token) / ('X-Key': $Key)",
		Canonical: "Headers: ('Authorization': 'Bearer {Token}') / ('X-Key': '{Key}')",
		Rewrite:   Rewrite{Structural: "header value as a template: `'text' + $P` becomes `'text{P}'`"},
		RemovedIn: 2,
		Note: "A header value is a template like the path: `{P}` is the operation parameter P. The old " +
			"form stored only the text before the `+`. Not rewritten when that text holds a `{` or `}`.",
		Example: "create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) " +
			"{ operation Get (Method: get, Path: '/a', Parameters: ($Token: String), " +
			"Headers: ('Authorization': 'Bearer ' + $Token), Response: none) };",
		CanonicalExample: "create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) " +
			"{ operation Get (Method: get, Path: '/a', Parameters: ($Token: String), " +
			"Headers: ('Authorization': 'Bearer {Token}'), Response: none) };",
	},
	{
		Code: RestCallClauses,
		Old: "call rest service <method> <url> [header N = v …] [auth basic $u password $p] " +
			"[body …] [timeout n] returns …",
		Canonical: "call rest service <method> <url> (Headers: ('N': v), " +
			"Authentication: basic (Username: $u, Password: $p), Body: …, Timeout: n) returns …",
		Rewrite: Rewrite{Structural: "activity settings as one property list (ADR-0013): `header N = v` becomes " +
			"`Headers: ('N': v)`, `auth basic $u password $p` becomes `Authentication: basic (Username: $u, " +
			"Password: $p)`, `body '…'` becomes `Body: template '…'`, `body binary|mapping …` becomes " +
			"`Body: binary|mapping …`, `timeout n` becomes `Timeout: n`"},
		RemovedIn: 2,
		Note: "The method, URL, `returns …` and `on error …` stay words; the dialog's settings are one list after " +
			"the URL, keyed as the consumed REST service names them. The two forms cannot be mixed in one statement.",
		Example: "create microflow M.F ($T: String) begin $R = call rest service post 'https://example.com' " +
			"header 'Accept' = 'application/json' auth basic 'u' password $T body '{1}' with ({1} = $T) timeout 30 " +
			"returns string; end;",
		CanonicalExample: "create microflow M.F ($T: String) begin $R = call rest service post 'https://example.com' " +
			"(Headers: ('Accept': 'application/json'), Authentication: basic (Username: 'u', Password: $T), " +
			"Body: template '{1}' with ({1} = $T), Timeout: 30) returns string; end;",
	},
}

// r9Entries are R9's (ako/mxcli#755): documentation is a `/** … */` doc
// comment, the folder is a `folder '…'` clause, and a workflow activity's
// caption is `caption '…'`.
var r9Entries = []Entry{
	{
		Code:      DocumentationClause,
		Old:       "create constant|association|json structure|image collection … comment '…'",
		Canonical: "/** … */ before the statement",
		Rewrite: Rewrite{Structural: "documentation as a doc comment: the clause is deleted and its text " +
			"written as `/** … */` before the statement"},
		RemovedIn: 2,
		Note: "Documentation is a doc comment on every document. A statement that has both is reported, " +
			"not rewritten, and so is a text a doc comment cannot hold exactly (a `*/`, blank lines, " +
			"or space at the start or end of a line).",
		Example:          "create constant M.ApiUrl ( Type: string, DefaultValue: 'https://x' ) comment 'Base URL';",
		CanonicalExample: "/** Base URL */\ncreate constant M.ApiUrl ( Type: string, DefaultValue: 'https://x' );",
	},
	{
		Code:             WorkflowCommentCaption,
		Old:              "<workflow activity> comment '…'",
		Canonical:        "<workflow activity> caption '…'",
		Rewrite:          Rewrite{Token: "comment", Replacement: "caption"},
		RemovedIn:        2,
		Note:             "The clause sets the caption Studio Pro shows on the activity; it was never a comment. Also on `end workflow` and an event sub-process's timer start.",
		Example:          "create workflow M.W parameter $WorkflowContext: M.E begin notification Ready comment 'Ready'; end workflow;",
		CanonicalExample: "create workflow M.W parameter $WorkflowContext: M.E begin notification Ready caption 'Ready'; end workflow;",
	},
	{
		Code:      FolderProperty,
		Old:       "create page|snippet|consumed rest service|… M.N (…, Folder: '…', …)",
		Canonical: "create page|snippet|consumed rest service|… M.N folder '…' (…)",
		Rewrite: Rewrite{Structural: "folder as a clause: the property is deleted from the list and written " +
			"as `folder '…'` after the name"},
		RemovedIn: 2,
		Note: "The folder is a clause after the name on every document, as for a microflow or an enumeration. " +
			"On a page, snippet, consumed REST service, consumed or published OData service and published " +
			"REST service.",
		Example:          "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Folder: 'Admin') { };",
		CanonicalExample: "create page M.P folder 'Admin' (Title: 'P', Layout: Atlas_Core.Atlas_Default) { };",
	},
	{
		Code:      DocumentationProperty,
		Old:       "Documentation: '…' in a regular expression, task queue or scheduled event",
		Canonical: "/** … */ before the statement",
		Rewrite: Rewrite{Structural: "documentation as a doc comment: the property is deleted from the list and " +
			"its text written as `/** … */` before the statement"},
		RemovedIn: 2,
		Note: "A statement that has both is reported, not rewritten, and so is a text a doc comment cannot " +
			"hold exactly.",
		Example:          "create regular expression M.Zip (Expression: '[0-9]{4}', Documentation: 'Dutch zip');",
		CanonicalExample: "/** Dutch zip */\ncreate regular expression M.Zip (Expression: '[0-9]{4}');",
	},
}

// r6Entries are R6's verbs (ako/mxcli#755, PROPOSAL_mdl_beta_syntax_freeze.md
// §3 R6).
var r6Entries = []Entry{
	{
		Code:      ShowSingleThing,
		Old:       "show page|project security|security matrix|structure|context of …",
		Canonical: "describe page|app security|security matrix|structure|context of …",
		Rewrite: Rewrite{Structural: "verb as `describe`: `show page X` -> `describe page X`, `show project security` -> " +
			"`describe app security`; the same for `list` on these forms"},
		RemovedIn: 2,
		Note: "`show` is dropped (R6): plurals are listed, one thing is described, and each of these describes " +
			"is the same statement as its `show`. Not covered here: `show navigation [menu]` and `show settings` " +
			"print tables, so they are `list navigation [menu]` / `list settings` (MDL-DEPR002); `show entity X` and " +
			"`show association X` print a summary no mdl 1 statement prints, so they are not aliases at all and are " +
			"refused under mdl 1 (MDL-V1-SHOWSUMMARY).",
		Example:          "show security matrix in M;",
		CanonicalExample: "describe security matrix in M;",
	},
	{
		Code:             UserRoleRemove,
		Old:              "alter user role R remove module roles (…)",
		Canonical:        "alter user role R drop module roles (…)",
		Rewrite:          Rewrite{Token: "remove", Replacement: "drop"},
		RemovedIn:        2,
		Note:             "An alter adds and drops its children (R6), as `alter entity … drop attribute` does.",
		Example:          "alter user role Clerk remove module roles (M.User);",
		CanonicalExample: "alter user role Clerk drop module roles (M.User);",
	},
	{
		Code:             SettingsRemove,
		Old:              "alter settings language remove '…' / alter settings workflows remove group '…'",
		Canonical:        "alter settings language drop '…' / alter settings workflows drop group '…'",
		Rewrite:          Rewrite{Token: "remove", Replacement: "drop"},
		RemovedIn:        2,
		Note:             "An alter adds and drops its children (R6). `modify` and `add or modify` are unchanged for now.",
		Example:          "alter settings language remove 'ar_SD';",
		CanonicalExample: "alter settings language drop 'ar_SD';",
	},
	{
		Code:             ColumnForAttribute,
		Old:              "alter entity E add|rename|modify|drop column …",
		Canonical:        "alter entity E add|rename|modify|drop attribute …",
		Rewrite:          Rewrite{Token: "column", Replacement: "attribute"},
		RemovedIn:        2,
		Note:             "An entity has attributes; `column` was a synonym in four alter entity actions.",
		Example:          "alter entity M.E drop column Note;",
		CanonicalExample: "alter entity M.E drop attribute Note;",
	},
	{
		Code:      RestCall,
		Old:       "rest call get|post|… 'url' …",
		Canonical: "call rest service get|post|… 'url' …",
		Rewrite:   Rewrite{Structural: "statement keyword: `rest call` becomes `call rest service`"},
		RemovedIn: 2,
		Note: "Studio Pro's name for the activity, in the `call <kind>` pattern of every other call (R6). " +
			"The clauses after the method are unchanged.",
		Example:          "create microflow M.F () begin $R = rest call get 'https://example.com' returns string; end;",
		CanonicalExample: "create microflow M.F () begin $R = call rest service get 'https://example.com' returns string; end;",
	},
	{
		Code:      DescribeWidgetType,
		Old:       "describe widget <name>",
		Canonical: "describe widget type <name>",
		Rewrite:   Rewrite{Structural: "`type` after `widget`: `describe widget combobox` -> `describe widget type combobox`"},
		RemovedIn: 2,
		Note: "`widget type` asks which kind of widget, and cannot be read as a widget on a page " +
			"(`describe fragment … widget`, `describe styling … widget`).",
		Example:          "describe widget combobox;",
		CanonicalExample: "describe widget type combobox;",
	},
	{
		Code:             DefineFragment,
		Old:              "define fragment F as { … }",
		Canonical:        "create fragment F as { … }",
		Rewrite:          Rewrite{Token: "define", Replacement: "create"},
		RemovedIn:        2,
		Note:             "`create` is the verb every other definition uses (R6). A fragment is still session-scoped and unqualified.",
		Example:          "define fragment Header as { dynamictext t (Content: 'x') };",
		CanonicalExample: "create fragment Header as { dynamictext t (Content: 'x') };",
	},
	{
		Code:             SingularCollectionList,
		Old:              "list image|icon|message definition collection [in M]",
		Canonical:        "list image|icon|message definition collections [in M]",
		Rewrite:          Rewrite{Structural: "singular as the plural: `collection` -> `collections` after `list`"},
		RemovedIn:        2,
		Note:             "`list` enumerates, and names what it enumerates in the plural, as `list entities` does (R6).",
		Example:          "list image collection in M;",
		CanonicalExample: "list image collections in M;",
	},
	{
		Code:      AIModel,
		Old:       "create|alter|drop|describe|move model M.X / list models",
		Canonical: "create|alter|drop|describe|move ai model M.X / list ai models",
		Rewrite:   Rewrite{Structural: "document name: `model` becomes `ai model`, `models` becomes `ai models`"},
		RemovedIn: 2,
		Note: "Studio Pro's name for the agent editor's model document (R10). `model` alone is too generic, and " +
			"collided with `alter settings model` (now `alter settings runtime`, MDL-DEPR555). An agent's " +
			"`Model: M.X` property is unchanged.",
		Example:          "drop model M.Gpt;",
		CanonicalExample: "drop ai model M.Gpt;",
	},
	{
		Code:             JSONStructureSample,
		Old:              "create json structure M.J snippet '…'",
		Canonical:        "create json structure M.J sample '…'",
		Rewrite:          Rewrite{Token: "snippet", Replacement: "sample"},
		RemovedIn:        2,
		Note:             "The example JSON the structure is derived from is its sample (R10); `snippet` is a page document type.",
		Example:          "create json structure M.J snippet '{\"a\": 1}';",
		CanonicalExample: "create json structure M.J sample '{\"a\": 1}';",
	},
	{
		Code:      AppSecurityClause,
		Old:       "alter app security level …|demo users on|off|guest access on [role R]|off|strict mode on|off",
		Canonical: "alter app security ( SecurityLevel: …, EnableDemoUsers: …, EnableGuestAccess: …, GuestUserRole: R, StrictMode: … )",
		Rewrite:   Rewrite{Structural: "clause as a property list: `level production` -> `( SecurityLevel: production )`"},
		RemovedIn: 2,
		Note: "App security is a document with properties, set like every other one (R10, R3). The keys are " +
			"Studio Pro's property names, and one statement can set several.",
		Example:          "alter app security guest access on role Guest;",
		CanonicalExample: "alter app security ( EnableGuestAccess: true, GuestUserRole: Guest );",
	},
	{
		Code:      FolderClausePosition,
		Old:       "create constant M.C ( … ) folder '…' / create snippet M.S (…) folder '…' { … }",
		Canonical: "create constant M.C folder '…' ( … ) / create snippet M.S folder '…' (…) { … }",
		Rewrite:   Rewrite{Structural: "clause moved: `folder '…'` goes right after the name"},
		RemovedIn: 2,
		Note: "The folder is a clause right after the name on every document (R9). A statement with the clause in " +
			"both places is reported, not rewritten: the later one is what is stored.",
		Example:          "create constant M.Url ( Type: String, DefaultValue: 'x' ) folder 'Config';",
		CanonicalExample: "create constant M.Url folder 'Config' ( Type: String, DefaultValue: 'x' );",
	},
	{
		Code:             SetComment,
		Old:              "alter entity|association|enumeration … set comment '…'",
		Canonical:        "alter entity|association|enumeration … set documentation '…'",
		Rewrite:          Rewrite{Token: "comment", Replacement: "documentation"},
		RemovedIn:        2,
		Note:             "It sets the element's documentation, which is what the alter form is called (R9).",
		Example:          "alter entity M.E set comment 'Orders';",
		CanonicalExample: "alter entity M.E set documentation 'Orders';",
	},
}

// r5Entries are the rest of R5's spellings (ako/mxcli#753), kept apart for the
// same reason as r8Entries.
var r5Entries = []Entry{
	{
		Code:      WorkflowStringExpression,
		Old:       "decision '<expression>' / timer '<expression>' / due date '<expression>'",
		Canonical: "decision <expression> / timer <expression> / due date <expression>",
		Rewrite:   Rewrite{Structural: "expression out of its string: `decision '$Ctx/Total > 1000'` becomes `decision $Ctx/Total > 1000`"},
		RemovedIn: 2,
		Note: "Expressions are bare everywhere (R5): a workflow decision, `wait for timer`, a timer boundary " +
			"event, a timer event sub-process and a due date (workflow, user task, alter workflow). The string " +
			"form keeps its meaning — its content is the expression — so it is an alias, not a change of " +
			"meaning. A string whose content does not read back as the same bare expression is left in place " +
			"and reported by fmt --upgrade.",
		Example: "create workflow M.W parameter $WorkflowContext: M.E begin " +
			"decision '$WorkflowContext/Total > 1000' outcomes true -> { } false -> { }; end workflow;",
		CanonicalExample: "create workflow M.W parameter $WorkflowContext: M.E begin " +
			"decision $WorkflowContext/Total > 1000 outcomes true -> { } false -> { }; end workflow;",
	},
	{
		Code: BracketedWidgetCondition,
		Old:  "Visible: [<expression>] / Editable: [<expression>]",
		// The bare form alone is not the same meaning: inside the brackets a bare
		// attribute is rooted in $currentObject, so the canonical form names it
		// (sudoku FINDINGS #63 migrated from this text by hand and rebound them).
		Canonical: "Visible: <expression> / Editable: <expression>, each attribute as $currentObject/Attr",
		Rewrite: Rewrite{Structural: "brackets into the expression they store: `Visible: [Active]` becomes " +
			"`Visible: $currentObject/Active`"},
		RemovedIn: 2,
		Note: "A conditional visibility or editability is a client expression, not XPath, so it is written bare " +
			"like every other expression (R5) and stored as written — name an attribute as " +
			"`$currentObject/Attr`. Also in `alter page … set (Visible: …)`. The bracketed form rooted a bare " +
			"attribute in $currentObject; the rewrite writes the expression it stored. One whose stored text " +
			"would not read back as the same bare expression is left in place and reported by fmt --upgrade.",
		Example: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $E) { " +
			"textbox t (Attribute: Name, Visible: [Active and $currentObject/Name != empty]) } };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $E) { " +
			"textbox t (Attribute: Name, Visible: $currentObject/Active and $currentObject/Name != empty) } };",
	},
	{
		Code:      ReversedEntityRevoke,
		Old:       "revoke M.Role on M.E [(rights)]",
		Canonical: "revoke rights|all on entity M.E from M.Role",
		Rewrite: Rewrite{Structural: "rights (or `all` when none are listed) before `on entity`, roles after `from`: " +
			"`revoke R on M.E (write *)` becomes `revoke write * on entity M.E from R`, `revoke R on M.E` becomes " +
			"`revoke all on entity M.E from R`"},
		RemovedIn: 2,
		Note: "The revoke mirrors the grant (MDL-DEPR030). `all` removes the roles' access rule; a rights list " +
			"takes those rights away and keeps the rule.",
		Example:          "revoke M.User, M.Admin on M.Order (write *, delete);",
		CanonicalExample: "revoke write *, delete on entity M.Order from M.User, M.Admin;",
	},
	{
		Code:      DollarConstant,
		Old:       "Username: $Const",
		Canonical: "Username: @Module.Const",
		Rewrite:   Rewrite{Structural: "`$Const` becomes `@<the service's module>.Const`"},
		RemovedIn: 2,
		Note: "A constant is referred to one way everywhere: `@Module.Const` (R5). `$` names a variable, and " +
			"`$Const` meant a constant of the consumed REST service's own module.",
		Example: "create consumed rest service M.Api (BaseUrl: 'https://example.com', " +
			"Authentication: basic (Username: $ApiUser, Password: @M.ApiPassword)) { };",
		CanonicalExample: "create consumed rest service M.Api (BaseUrl: 'https://example.com', " +
			"Authentication: basic (Username: @M.ApiUser, Password: @M.ApiPassword)) { };",
	},
	{
		Code:             BareConstantKey,
		Old:              "Key: Module.Const",
		Canonical:        "Key: @Module.Const",
		Rewrite:          Rewrite{Structural: "`@` before the constant's name"},
		RemovedIn:        2,
		Note:             "A constant is referred to one way everywhere: `@Module.Const` (R5). Also in `alter ai model|knowledge base … set Key = …`.",
		Example:          "create ai model M.GPT (Provider: MxCloudGenAI, Key: M.ApiKey);",
		CanonicalExample: "create ai model M.GPT (Provider: MxCloudGenAI, Key: @M.ApiKey);",
	},
	{
		Code:      QuotedSettingsConstant,
		Old:       "alter settings constant 'Module.Const' …",
		Canonical: "alter settings constant @Module.Const …",
		Rewrite:   Rewrite{Structural: "constant's name out of its string, with `@`: `constant 'M.ApiUrl'` becomes `constant @M.ApiUrl`"},
		RemovedIn: 2,
		Note: "A constant is referred to one way everywhere: `@Module.Const` (R5). Also in `alter settings drop " +
			"constant`. A string that is not a qualified name is left in place and reported by fmt --upgrade.",
		Example:          "alter settings constant 'M.ApiUrl' value 'https://test.example.com' in configuration 'Default';",
		CanonicalExample: "alter settings constant @M.ApiUrl value 'https://test.example.com' in configuration 'Default';",
	},
	{
		Code:      QuotedVariableDefault,
		Old:       "Variables: ( $name: Type = '<expression>' ) / add variables $name: Type = '<expression>'",
		Canonical: "Variables: ( $name: Type = <expression> ) / add variables $name: Type = <expression>",
		Rewrite:   Rewrite{Structural: "default out of its string: `$show: boolean = 'true'` becomes `$show: boolean = true`"},
		RemovedIn: 2,
		Note: "A variable's default is an expression, written bare like every other (R5). The string form keeps " +
			"its meaning — its content is the expression — under every language version, so it is an alias. " +
			"A default that is itself a string (`$s: string = '''abc'''`) or empty has no other spelling yet and " +
			"is not reported; one whose content would not read back as the same bare expression is left in " +
			"place and reported by fmt --upgrade.",
		Example: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, " +
			"Variables: ( $show: boolean = 'if (3 < 4) then true else false' )) { };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, " +
			"Variables: ( $show: boolean = if (3 < 4) then true else false )) { };",
	},
}

// r2Entries are R2's integration-document brackets (ako/mxcli#754).
var r2Entries = []Entry{
	{
		Code:      RestOperationBraces,
		Old:       "operation X { Method: get, … }",
		Canonical: "operation X ( Method: get, … )",
		Rewrite:   Rewrite{Structural: "operation's braces: `operation X { … }` becomes `operation X ( … )`"},
		RemovedIn: 2,
		Note: "An operation is a child of the service: its properties are in ( ) like every child's, " +
			"and { } holds children (R2). A body or response mapping keeps its { } tree.",
		Example: "create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) " +
			"{ operation GetUser { Method: get, Path: '/u', Response: none } };",
		CanonicalExample: "create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) " +
			"{ operation GetUser ( Method: get, Path: '/u', Response: none ) };",
	},
	{
		Code:      AgentAttachmentBraces,
		Old:       "tool X { … } / mcp service M.S { … } / knowledge base K { … }",
		Canonical: "tool X ( … ) / mcp service M.S ( … ) / knowledge base K ( … )",
		Rewrite:   Rewrite{Structural: "attachment's braces: `tool X { … }` becomes `tool X ( … )`"},
		RemovedIn: 2,
		Note:      "In create agent and in alter agent … add. An attachment is a child of the agent: its properties are in ( ) (R2).",
		Example: "create agent M.A (UsageType: Task, Model: M.Gpt, SelectedModel: 'gpt-4o', SystemPrompt: 'x') " +
			"{ tool Lookup { Description: 'Find', Enabled: true } };",
		CanonicalExample: "create agent M.A (UsageType: Task, Model: M.Gpt, SelectedModel: 'gpt-4o', SystemPrompt: 'x') " +
			"{ tool Lookup ( Description: 'Find', Enabled: true ) };",
	},
	{
		Code:      ImageCollectionParens,
		Old:       "image collection M.C ( image X from file '…', … )",
		Canonical: "image collection M.C { image X ( File: '…' ) … }",
		Rewrite: Rewrite{Structural: "image list: the images move into { } without commas, and `from file '…'` " +
			"becomes `( File: '…' )`"},
		RemovedIn:        2,
		Note:             "The images are the collection's children, so they are in { }, each with its properties in ( ) (R2).",
		Example:          "create image collection M.Icons (image Logo from file 'logo.png', image Home from file 'home.png');",
		CanonicalExample: "create image collection M.Icons {image Logo ( File: 'logo.png' ) image Home ( File: 'home.png' )};",
	},
	{
		Code:      MessageTreeParens,
		Old:       "message definition collection M.C ( definition D for M.E ( A, M.E_B/M.B ( C ) ) )",
		Canonical: "message definition collection M.C { definition D for M.E { A, M.E_B/M.B { C } } }",
		Rewrite:   Rewrite{Structural: "message trees: each parenthesised definition list and member tree moves into { }"},
		RemovedIn: 2,
		Note: "The definitions and members are children, so they are in { }, as in an import or export " +
			"mapping (R2). Also in `alter message definition collection … add definition` and " +
			"`alter message definition … add member`. One warning per statement.",
		Example:          "create message definition collection M.Msgs (definition Order for M.Order (Number, M.Order_Line/M.Line (Sku)));",
		CanonicalExample: "create message definition collection M.Msgs {definition Order for M.Order {Number, M.Order_Line/M.Line {Sku}}};",
	},
	{
		Code:      AlterFlowFragmentBraces,
		Old:       "alter microflow M.F { insert after $X { … } }",
		Canonical: "alter microflow M.F { insert after $X begin … end; }",
		Rewrite:   Rewrite{Structural: "fragment's braces: `{` becomes `begin` and `}` becomes `end`"},
		RemovedIn: 2,
		Note: "A fragment is imperative flow, written exactly as the body of `create microflow`, so it is " +
			"`begin … end` (R2). The operations around it are the alter's children and stay in its { }.",
		Example:          "alter microflow M.F { insert after $X { log info 'x'; } };",
		CanonicalExample: "alter microflow M.F { insert after $X begin log info 'x'; end };",
	},
}

// r2RestEntries are the rest of R2 (ako/mxcli#754): navigation and menus,
// property maps, and the database connection.
var r2RestEntries = []Entry{
	{
		Code:      RestHeaderEquals,
		Old:       "Headers: ( 'Name' = value )",
		Canonical: "Headers: ( 'Name': value )",
		Rewrite:   Rewrite{Structural: "header list: `'Name' = value` becomes `'Name': value`"},
		RemovedIn: 2,
		Note:      "A header list is a map of the operation's properties, so it is `( key: value )` like every property map (R2/R3).",
		Example: "create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) " +
			"{ operation Ping ( Method: get, Path: '/p', Headers: ('Accept' = 'application/json'), Response: none ) };",
		CanonicalExample: "create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) " +
			"{ operation Ping ( Method: get, Path: '/p', Headers: ('Accept': 'application/json'), Response: none ) };",
	},
	{
		Code:      MenuChildrenParens,
		Old:       "navigation P menu ( menu item …; menu 'X' ( … ); )  /  create menu M.M ( … )",
		Canonical: "navigation P { menu item … menu 'X' { … } }  /  create menu M.M { … }",
		Rewrite: Rewrite{Structural: "menu items: `menu (` becomes `{`, a menu's or sub-menu's ( ) become { }, " +
			"and the `;` after each item is dropped"},
		RemovedIn: 2,
		Note: "Menu items are children, so they are in { } like a page's widgets, and a child ends in `)` or `}`, " +
			"so it needs no separator (R2). In a navigation profile the items are the profile's own children.",
		Example:          "create or modify navigation Responsive home page M.Home menu ( menu item 'Home'; menu 'Admin' ( menu item 'Users'; ); );",
		CanonicalExample: "create or modify navigation Responsive home page M.Home { menu item 'Home' menu 'Admin' { menu item 'Users' } };",
	},
	{
		Code:      MenuItemClauses,
		Old:       "menu item 'X' page M.P icon I  /  microflow M.F  /  sign out",
		Canonical: "menu item 'X' ( OnClick: show page M.P, Icon: I )  /  call microflow M.F  /  sign out",
		Rewrite: Rewrite{Structural: "item clauses: `page M.P` becomes `( OnClick: show page M.P )`, `microflow M.F` " +
			"`OnClick: call microflow M.F`, `sign out` `OnClick: sign out`, and `icon I` `Icon: I`"},
		RemovedIn: 2,
		Note: "A menu item is a child: its properties are in ( ), and its action is written in the words a page " +
			"action uses (R2, R8). A sub-menu's icon moves the same way: `menu 'X' ( Icon: I ) { … }`.",
		Example:          "create menu M.Main { menu item 'Home' page M.Home icon Atlas_Core.Atlas.home menu item 'Bye' sign out };",
		CanonicalExample: "create menu M.Main { menu item 'Home' ( OnClick: show page M.Home, Icon: Atlas_Core.Atlas.home ) menu item 'Bye' ( OnClick: sign out ) };",
	},
	{
		Code:             HeaderMapBraces,
		Old:              "Params: { $P: M.E }  /  Variables: { $v: Boolean = 'true' }",
		Canonical:        "Params: ( $P: M.E )  /  Variables: ( $v: Boolean = 'true' )",
		Rewrite:          Rewrite{Structural: "header map: the braces become ( )"},
		RemovedIn:        2,
		Note:             "In a page's and a snippet's header. A map is a property list, so it is in ( ); { } holds children (R2).",
		Example:          "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: { $Order: M.Order }) { };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $Order: M.Order )) { };",
	},
	{
		Code:      TemplateParamsBrackets,
		Old:       "ContentParams: [{1} = expr]",
		Canonical: "ContentParams: ({1} = expr)",
		Rewrite:   Rewrite{Structural: "template parameters: the brackets become ( )"},
		RemovedIn: 2,
		Note: "Also CaptionParams and a pluggable widget's `<Name>Params`. The parameters are a map, in ( ) (R2), " +
			"and each binds a value with `=`, as `with ({1} = …)` does (R4).",
		Example: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) " +
			"{ dynamictext t (Content: 'Hi {1}', ContentParams: [{1} = 'x']) };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) " +
			"{ dynamictext t (Content: 'Hi {1}', ContentParams: ({1} = 'x')) };",
	},
	{
		Code:      DesignPropertiesBrackets,
		Old:       "DesignProperties: ['Key': 'Value', 'Group': ['k': on]]",
		Canonical: "DesignProperties: ('Key': 'Value', 'Group': ('k': on))",
		Rewrite:   Rewrite{Structural: "design properties: each bracketed list becomes ( )"},
		RemovedIn: 2,
		Note:      "The design properties are a map of properties, so they are in ( ) (R2).",
		Example: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) " +
			"{ container c (DesignProperties: ['Spacing': ['margin-top': 'Large'], 'Full width': on]) };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) " +
			"{ container c (DesignProperties: ('Spacing': ('margin-top': 'Large'), 'Full width': on)) };",
	},
	{
		Code:      SnippetCallParamsBraces,
		Old:       "snippetcall s (Snippet: M.S, Params: {$Asset: $var})",
		Canonical: "snippetcall s (Snippet: M.S, Params: (Asset = $var))",
		Rewrite:   Rewrite{Structural: "snippet call arguments: `{$P: $v}` becomes `(P = $v)`"},
		RemovedIn: 2,
		Note: "A snippet call is a call site, so it binds `Param = value` without a `$` on the parameter name (R4), " +
			"in the ( ) of a map (R2).",
		Example: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($Asset: M.Asset)) " +
			"{ snippetcall s (Snippet: M.S, Params: {$Asset: $Asset}) };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($Asset: M.Asset)) " +
			"{ snippetcall s (Snippet: M.S, Params: (Asset = $Asset)) };",
	},
	{
		Code:      DatabaseConnectionClauses,
		Old:       "database connection M.Db type '…' connection string @M.C … begin query Q sql '…' returns M.E map (c as A); end",
		Canonical: "database connection M.Db ( Type: '…', ConnectionString: @M.C, … ) { query Q ( Sql: '…', Returns: M.E, Map: ( A = c ) ) }",
		Rewrite: Rewrite{Structural: "clauses become the property list: `type` `Type:`, `connection string` `ConnectionString:`, " +
			"`host` `Host:`, `port` `Port:`, `database` `DatabaseName:`, `username` `Username:`, `password` `Password:`; " +
			"`begin … end` becomes { }; a query's `sql`, `parameter`, `returns` and `map` become Sql, Parameters, Returns " +
			"and Map, and `column as Attr` becomes `Attr = column`"},
		RemovedIn: 2,
		Note: "The database connection was the one declarative document written as clauses and begin … end. Its " +
			"properties are in ( ) and its queries are children in { } (R2).",
		Example: "create database connection M.Db type 'PostgreSQL' connection string @M.Url username @M.User password @M.Pass " +
			"begin query Q sql $$select id from t where id > {min}$$ parameter min: Integer default '0' returns M.T map (id as Id); end;",
		CanonicalExample: "create database connection M.Db ( Type: 'PostgreSQL', ConnectionString: @M.Url, Username: @M.User, Password: @M.Pass ) " +
			"{ query Q ( Sql: $$select id from t where id > {min}$$, Parameters: ( min: Integer default '0' ), Returns: M.T, Map: (Id = id) ) };",
	},
}

// r3Entries are R3's spellings (ako/mxcli#751): `:` sets a model property. An
// `alter` takes exactly create's `( Key: value, … )` list, so a fragment of
// describe output pastes into an alter unchanged.
var r3Entries = []Entry{
	{
		Code:      AlterPageSetEquals,
		Old:       "set Key = value [on target]  /  set (Key = value, …) [on target]",
		Canonical: "set (Key: value, …) [on target]",
		Rewrite:   Rewrite{Structural: "each `=` as `:`, and the assignments in parentheses when they are not"},
		RemovedIn: 2,
		Note: "In `alter page`, `alter snippet` and `alter layout`. `set layout = M.L` is a separate form " +
			"and is not reported.",
		Example:          "alter page M.P { set Caption = 'Save' on btnSave; };",
		CanonicalExample: "alter page M.P { set (Caption: 'Save') on btnSave; };",
	},
	{
		Code:             AlterPageSetUnparenthesised,
		Old:              "set Key: value [on target]",
		Canonical:        "set (Key: value) [on target]",
		Rewrite:          Rewrite{Structural: "assignment in parentheses"},
		RemovedIn:        2,
		Note:             "Properties are a parenthesised list, even when there is one.",
		Example:          "alter page M.P { set Caption: 'Save' on btnSave; };",
		CanonicalExample: "alter page M.P { set (Caption: 'Save') on btnSave; };",
	},
	{
		Code:             AlterPageDropWidget,
		Old:              "drop widget a, b",
		Canonical:        "drop a, b",
		Rewrite:          Rewrite{Structural: "`drop widget a` as `drop a`"},
		RemovedIn:        2,
		Note:             "The target names the element; the kind is its own.",
		Example:          "alter page M.P { drop widget txtOld; };",
		CanonicalExample: "alter page M.P { drop txtOld; };",
	},
	{
		Code:      SettingsAssignment,
		Old:       "alter settings runtime Key = value, …  /  create configuration 'X' Key = value, …",
		Canonical: "alter settings runtime ( Key: value, … )  /  create configuration 'X' ( Key: value, … )",
		Rewrite:   Rewrite{Structural: "assignments in parentheses, each `=` as `:`"},
		RemovedIn: 2,
		Note: "Every settings section (runtime, language, workflows, configuration 'X') and `create configuration`. " +
			"`alter settings constant 'C' value 'v'` is a clause, not a property, and is unchanged.",
		Example:          "alter settings runtime AfterStartupMicroflow = 'M.Startup', BcryptCost = 11;",
		CanonicalExample: "alter settings runtime ( AfterStartupMicroflow: 'M.Startup', BcryptCost: 11 );",
	},
	{
		Code:             ODataAlterAssignment,
		Old:              "alter consumed|published odata service X set Key = value, …",
		Canonical:        "alter consumed|published odata service X set ( Key: value, … )",
		Rewrite:          Rewrite{Structural: "assignments in parentheses, each `=` as `:`"},
		RemovedIn:        2,
		Note:             "The list takes exactly the keys and values of the service's `create` statement.",
		Example:          "alter consumed odata service M.Crm set Version = '2.0', Timeout = 30;",
		CanonicalExample: "alter consumed odata service M.Crm set ( Version: '2.0', Timeout: 30 );",
	},
	{
		Code:             StylingAssignment,
		Old:              "alter styling on page P widget w set Class = 'x', 'Full width' = on",
		Canonical:        "alter styling on page P widget w set ( Class: 'x', 'Full width': on )",
		Rewrite:          Rewrite{Structural: "assignments in parentheses, each `=` as `:`"},
		RemovedIn:        2,
		Note:             "The same list `alter page … set ( … ) on w` takes for a widget's class, style and design properties.",
		Example:          "alter styling on page M.P widget ctn1 set Class = 'card', 'Full width' = on;",
		CanonicalExample: "alter styling on page M.P widget ctn1 set ( Class: 'card', 'Full width': on );",
	},
	{
		Code:             AllowCreateChangeLocally,
		Old:              "alter entity M.E set allow_create_change_locally = true",
		Canonical:        "alter entity M.E set ( AllowCreateChangeLocally: true )",
		Rewrite:          Rewrite{Structural: "property as create's list: `set ( AllowCreateChangeLocally: <value> )`"},
		RemovedIn:        2,
		Note:             "The key `create external entity` takes for the same property.",
		Example:          "alter entity M.Remote set allow_create_change_locally = true;",
		CanonicalExample: "alter entity M.Remote set ( AllowCreateChangeLocally: true );",
	},
	{
		Code:             AssociationClauseColon,
		Old:              "type: Reference / owner: Both / storage: Table",
		Canonical:        "type Reference / owner Both / storage Table",
		Rewrite:          Rewrite{Structural: "clause without its colon: `type: Reference` as `type Reference` (also `owner`, `storage`)"},
		RemovedIn:        2,
		Note:             "A clause outside a property list takes no colon, as describe writes it.",
		Example:          "create association M.Order_Customer from M.Order to M.Customer type: Reference;",
		CanonicalExample: "create association M.Order_Customer from M.Order to M.Customer type Reference;",
	},
	{
		Code:             ModifyAttributeColon,
		Old:              "alter entity M.E modify attribute A Type",
		Canonical:        "alter entity M.E modify attribute A: Type",
		Rewrite:          Rewrite{Structural: "attribute definition with its colon: `A Type` as `A: Type`"},
		RemovedIn:        2,
		Note:             "An attribute definition is always `Name: Type`, as in `create entity` and `add attribute`.",
		Example:          "alter entity M.E modify attribute Code String(20);",
		CanonicalExample: "alter entity M.E modify attribute Code: String(20);",
	},
}

// r8Entries are R8's spellings (ako/mxcli#752). Kept apart from the list above
// only so the parallel phase-3 changes do not all edit its last lines.
var r8Entries = []Entry{
	{
		Code:      PageActionWord,
		Old:       "show_page, save_changes, close_page, microflow M.F, …",
		Canonical: "show page, save changes, close page, call microflow M.F, …",
		Rewrite: Rewrite{Structural: "page action as words: the underscore becomes a space (`show_page` -> " +
			"`show page`, also `save_changes`, `cancel_changes`, `close_page`, `create_object`, `open_link`, " +
			"`sign_out`, `complete_task`); `delete_object` -> `delete`; `microflow M.F` / `nanoflow M.F` -> " +
			"`call microflow M.F` / `call nanoflow M.F`"},
		RemovedIn: 2,
		Note: "The words are the ones a microflow uses for the same activity. A navigation menu's " +
			"`sign_out` is the same keyword. Arguments are unchanged.",
		Example:          "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { actionbutton b (Caption: 'Save', Action: sign_out) };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { actionbutton b (Caption: 'Save', Action: sign out) };",
	},
	{
		Code:      ErrorMessageKeyword,
		Old:       "error '…' / feedback '…' / error_message '…'",
		Canonical: "error message '…'",
		Rewrite: Rewrite{Structural: "message keyword as `error message`: `not null error '…'` (and after " +
			"`unique` or `required`), a validation rule's `feedback '…'`, and `error_message` / `errormessage`"},
		RemovedIn:        2,
		Note:             "One keyword for the text a user sees when a rule refuses a change.",
		Example:          "create entity M.E (Name: String(100) not null error 'Name is required');",
		CanonicalExample: "create entity M.E (Name: String(100) not null error message 'Name is required');",
	},
	{
		Code:      DeleteBehaviorClause,
		Old:       "delete_behavior …",
		Canonical: "on delete cascade|restrict|set null",
		Rewrite: Rewrite{Structural: "delete behaviour as the SQL referential action: " +
			"`delete_behavior cascade` / `delete_and_references` -> `on delete cascade`; " +
			"`prevent` / `delete_if_no_references` -> `on delete restrict`; " +
			"`delete_but_keep_references` -> `on delete set null`"},
		RemovedIn: 2,
		Note: "Also in `alter association … set delete_behavior …`. The three compound behaviour keywords " +
			"had three spellings each; the SQL referential actions have one.",
		Example:          "create association M.Order_Customer from M.Order to M.Customer type Reference delete_behavior prevent;",
		CanonicalExample: "create association M.Order_Customer from M.Order to M.Customer type Reference on delete restrict;",
	},
	{
		Code:             ReferenceSetUnderscore,
		Old:              "type reference_set",
		Canonical:        "type ReferenceSet",
		Rewrite:          Rewrite{Structural: "type name as Mendix writes it: `reference_set` -> `ReferenceSet`"},
		RemovedIn:        2,
		Example:          "create association M.Order_Tag from M.Order to M.Tag type reference_set;",
		CanonicalExample: "create association M.Order_Tag from M.Order to M.Tag type ReferenceSet;",
	},
	{
		Code:             ReturnsNone,
		Old:              "call rest service … returns none",
		Canonical:        "call rest service … returns nothing",
		Rewrite:          Rewrite{Token: "none", Replacement: "nothing"},
		RemovedIn:        2,
		Example:          "create microflow M.F () begin call rest service get 'https://example.com' returns none; end;",
		CanonicalExample: "create microflow M.F () begin call rest service get 'https://example.com' returns nothing; end;",
	},
	{
		Code:      DateType,
		Old:       "date",
		Canonical: "DateTime",
		Rewrite:   Rewrite{Structural: "type `date` as the type it was stored as: `DateTime`"},
		RemovedIn: 1,
		Note: "Mendix has no date-only type: `date` was always stored as a DateTime, and still is. " +
			"To show only the date, give the widget a date format. For a calendar date that must not shift " +
			"with the user's time zone (a birth date), write `DateTime not localized` — the rewrite does not, " +
			"because `date` was always stored localized.",
		Example:          "create persistent entity M.Account ( LastImport: date );",
		CanonicalExample: "create persistent entity M.Account ( LastImport: DateTime );",
	},
	{
		Code:      RegexExportLevelPublic,
		Old:       "create regular expression M.R ( …, ExportLevel: Public )",
		Canonical: "create regular expression M.R ( …, ExportLevel: API )",
		Rewrite:   Rewrite{Structural: "regular expression's export level value `Public` as `API`"},
		RemovedIn: 2,
		Note: "A regular expression's export level is Hidden or API, as Studio Pro names it and describe " +
			"prints it. `Public` was stored as written, a value the metamodel does not have; it now stores API.",
		Example:          "create regular expression M.R ( Expression: '^a$', ExportLevel: Public );",
		CanonicalExample: "create regular expression M.R ( Expression: '^a$', ExportLevel: API );",
	},
}

// All returns every registered entry, in code order.
func All() []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}

// Lookup returns the entry for code.
func Lookup(code string) (Entry, bool) {
	for _, e := range entries {
		if e.Code == code {
			return e, true
		}
	}
	return Entry{}, false
}

// IsDeprecationCode reports whether a violation rule ID is a registry code.
func IsDeprecationCode(ruleID string) bool {
	return strings.HasPrefix(ruleID, "MDL-DEPR")
}

// Policy is what `check` and `exec` do with a deprecated spelling.
type Policy int

const (
	// Warn reports it and carries on. The default.
	Warn Policy = iota
	// Error fails the run, for CI over docs, skills and examples.
	Error
)

// ParsePolicy reads the --deprecations flag value.
func ParsePolicy(s string) (Policy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "warn":
		return Warn, nil
	case "error":
		return Error, nil
	}
	return Warn, fmt.Errorf("invalid --deprecations value %q: want warn or error", s)
}
