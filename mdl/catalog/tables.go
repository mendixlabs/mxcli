// SPDX-License-Identifier: Apache-2.0

package catalog

// CatalogSchemaVersion is bumped whenever the schema in createTables changes
// in a way that requires existing on-disk caches to be regenerated.
//
// History:
//
//	32 (xpath widget): xpath_expressions gains WidgetId and WidgetName, and
//	    ComponentId is written as a GUID instead of a Go byte dump
//	    (ako/mxcli#1879) — a cached catalog keeps the dump and lacks the
//	    columns the linter selects.
//	31 (access rule id): permissions gains AccessRuleId and AccessRuleIndex,
//	    which the linter selects — against a cached catalog without them every
//	    permission query fails and the security rules report nothing
//	    (ako/mxcli#1437).
//	    31, not 30: the flow-settings change took 30 in parallel, and a cache
//	    built at 30 by it alone would never rebuild for this one.
//	30 (flow settings, message definition edges): microflows_data gains
//	    ApplyEntityAccess (ako/mxcli#1453), which the linter now selects with
//	    Excluded — against a cached catalog without the column every
//	    Microflows() query fails and every flow rule reports nothing. Also
//	    covers refs gaining the edges from message definitions to the
//	    entities, attributes and associations they expose (ako/mxcli#1781,
//	    which shipped without a bump): a cached catalog kept answering "no
//	    references" for them.
//	29 (service microflow edges): refs gains the edges from an integration
//	    service to the microflows it runs — a published OData resource's
//	    read/change microflows, its actions and authentication microflow, a
//	    published REST service's authentication microflow (`publish`), a
//	    business event handler (`event`), a consumed OData service's
//	    configuration microflows (`call`) (ako/mxcli#1639). No column changes,
//	    but a cached catalog would keep answering "no references" for every one
//	    of them until something else rebuilt it.
//	28 (message definitions): message_definition_collections and
//	    message_definitions, unioned into objects as
//	    MESSAGE_DEFINITION_COLLECTION / MESSAGE_DEFINITION, so documents()
//	    lists them (ako/mxcli#1433). Without the bump a cached catalog has
//	    neither view and an objects view that omits them.
//	    28, not 26: it was written as 26 beside the user-roles change, which
//	    merged first as 27, so a cache built at 27 lacks these tables.
//	27 (user roles): a user_roles table with each project user role's
//	    Description (ako/mxcli#1442). Its own number, not 26: the message
//	    definitions change took 26 in parallel, and a cache built at 26 by
//	    either alone would never rebuild for the other. Without the bump a
//	    cached catalog has no user_roles table at all.
//	25 (documentation in the strings index): strings gains a row for the
//	    documentation of every element the catalog reads a Description for
//	    (entities, attributes, constants, pages, ...), so SEARCH finds words
//	    that appear only there. Without the bump a cached catalog keeps
//	    answering "No matches found." for them.
//	24 (Studio Pro document names): the views rest_clients, rest_operations,
//	    odata_clients, odata_services and queues are now consumed_rest_services,
//	    consumed_rest_operations, consumed_odata_services,
//	    published_odata_services and task_queues, with the old names kept as
//	    alias views; objects.ObjectType says CONSUMED_REST_SERVICE,
//	    CONSUMED_ODATA_SERVICE, PUBLISHED_ODATA_SERVICE and TASK_QUEUE
//	    (mendixlabs/mxcli#1395). Views are CREATE VIEW IF NOT EXISTS, so a
//	    cached catalog has neither the new names nor the new ObjectType values.
//	23 (call and delete targets): activities_data gains QueueRef, the task
//	    queue a call runs in, and its ActionRef / EntityRef now name a call's
//	    target (microflow, nanoflow, Java or JavaScript action) and a delete's
//	    entity (mendixlabs/mxcli#1305). Without the bump a cached catalog
//	    fails every activities_for() with "no such column", and a cached
//	    activities table keeps answering every call and delete with "".
//	22 (11.15 message-definition source): an import/export mapping's schema
//	    source on Mendix 11.15+ comes from MessageDefinition2 (Module.Message),
//	    which replaced the MessageDefinition key (ako/mxcli#987).
//	21 (native bottom bar actions): a native profile's navigation_menu_items
//	    rows take ActionType / Page / Microflow from the bottom bar item's
//	    client action, not only its legacy Page (ako/mxcli#980).
//	20 (widget tree, class/style and actions): widgets_data gains
//	    ParentWidgetId, Depth, Class, Style, DynamicClasses, ActionType and
//	    HasConfirmation (mendixlabs/mxcli#1268). Without the bump a cached
//	    catalog fails every widgets() with "no such column". Its own number:
//	    the commit-refs change below took 19 in parallel, and a cache built at
//	    19 by either alone would never rebuild for the other (as 15/16 did).
//	19 (commit refs): refs gains RefKind "commit" (FLOW -> ENTITY) for a
//	    commit action and a create/change that commits (ako/mxcli#963). No
//	    column changes, but a cached catalog would keep answering "no flow
//	    commits X" for every entity until something else rebuilt it.
//	18 — layouts_data.Platform ("Web" / "Native"), the content wrapper's
//	    type. LayoutType cannot tell the platforms apart ("Popup" is native),
//	    and MPR012 needs it to stay off native pages, where CE0582 does not
//	    apply (ako/mxcli#953). Without the bump a cached catalog fails every
//	    layouts() with "no such column".
//	17 (activities in loops and their properties): activities_data gains the
//	    rows inside loop bodies, with ParentLoopId / LoopDepth, and the
//	    property columns lint rules asked for (real Caption, Description,
//	    AutoGenerateCaption, ConditionExpression, ConditionRule,
//	    ErrorHandlingType, LogLevel, LogNodeExpression, LogMessage, CommitType,
//	    WithEvents, RetrieveSource); microflows_data gains TotalActivityCount
//	    (mendixlabs/mxcli#1266, #1267). Without the bump a cached catalog
//	    fails every activities_for() with "no such column", and a cached
//	    activities table keeps silently omitting every loop body.
//	17 (lint catalog builtins): modules_data.DomainModelDocumentation (the
//	    domain model's own documentation, read nowhere before) and the delete
//	    behaviour of both association ends on associations_data
//	    (FromDeleteBehavior / ToDeleteBehavior and their error messages). A
//	    CREATE TABLE IF NOT EXISTS does not add a column to a cached catalog,
//	    so without the bump associations() and modules() fail with "no such
//	    column" on every project that already has one.
//	17 (workflow and loop-variable refs): refs gains the workflow edges the
//	    walk missed (event handlers, on-created / completion / group-targeting
//	    microflows, boundary-event paths, event sub-processes) and the
//	    change/delete edges of loop iterators and association-retrieve outputs;
//	    workflows_data's activity counts include boundary-event and event
//	    sub-process activities. Same reason as 11 and 13: refs are only written
//	    by a FULL build, and an unforced REFRESH CATALOG FULL on a fresh cache
//	    kept listing workflow.UserTaskEventHandle as dead (measured, TestApp).
//	17 (page title language): pages_data.Title is the project's default
//	    language (else en_US, else the lowest-sorted non-empty language) instead
//	    of whichever translation a map range met first (mendixlabs/mxcli#1262).
//	    A cached catalog keeps the old, build-dependent title. Also carries the
//	    "16" entry below, which never reached the constant.
//	16 — never a real version. This change and the refs change listed as 15
//	    below were each bumped 14 -> 15 on parallel branches (c449e1c2b,
//	    11ff6f204); the merge relabelled this entry "16" but left the constant
//	    at "15", so a cache built at "15" by either branch alone was never
//	    rebuilt for the other's change. 17 carries both. The change itself:
//	    permissions_data.DefaultMemberAccessRights, and the belated bump for
//	    activities_data.UseRequestTimeout / TimeoutExpression. Both columns
//	    were added without a bump. The activity pair happened to be rescued by
//	    14, which landed after it for an unrelated reason; the permissions
//	    column landed after 14 and so is still unreachable on any cache built
//	    at 14. PermissionsFor and Permissions both SELECT it, so on such a
//	    cache every permissions query fails with "no such column" — mxcli lint
//	    says so via QueryErrors, mxcli report builds the same LintContext,
//	    never calls it, and silently scores the project with every entity
//	    access rule finding nothing.
//	15 — refs gains ATTRIBUTE, ENUMERATION and ENUMERATION_VALUE targets (kinds
//	    member / xpath / type / value), IMPORT_MAPPING / EXPORT_MAPPING sources
//	    (kind mapping), and the microflow -> workflow `call` edge; graph_god_nodes
//	    keeps members off the asset side. Same reason as 11 and 13: refs are only
//	    written by REFRESH CATALOG FULL, so a cached catalog would keep answering
//	    `impact Mod.Entity.Attr` with "not referenced" for a used attribute.
//	14 — import_mappings_data / export_mappings_data: Id is the document's ID
//	    (was an AUTOINCREMENT integer) and Excluded is recorded; source gains
//	    ElementId. Two mappings may share a name when one is excluded, and the
//	    catalog could not tell them apart (mendixlabs/mxcli#1185). 13 and 12
//	    were bumped on parallel branches and the merge kept "12", so caches
//	    built at 12 never rebuilt for 13; this bump carries both.
//	13 — entity_event_handlers_data + view, and the `event` edge in refs
//	    (ENTITY -> MICROFLOW). Same reason as 11: refs are only written by
//	    REFRESH CATALOG FULL, so without the bump a cached catalog keeps
//	    reporting every handler-only microflow in GRAPH_DEAD_ASSETS and
//	    answering `show callers` with "(no callers found)" — the wrong answer
//	    this change exists to stop, served from a stale cache.
//	12 — HasCreatedDate / HasChangedDate / HasOwner / HasChangedBy on entities.
//	    A CREATE TABLE IF NOT EXISTS does not add a column to a cached
//	    catalog, so without the bump every query naming one fails with "no
//	    such column" on exactly the projects that have a catalog already --
//	    a hard error rather than a wrong answer, but on the wrong machines.
//	11 — the `widget` edge in refs (page/snippet -> widget definition) and the
//	    graph_god_nodes change that keeps widget targets off the asset side.
//	    Both need the bump for the same reason: refs are only written by
//	    REFRESH CATALOG FULL and a view is CREATE VIEW IF NOT EXISTS, so a
//	    cached catalog would answer `show references to combobox` with
//	    "(no references found)" — a wrong answer, not a missing one.
//	10 — the three lookups expression type checking needs and the catalog could
//	    not answer: attributes_data.EnumerationQualifiedName (DataType says only
//	    "Enumeration", losing which one), enumeration_values_data (the table
//	    stored ValueCount but not the values), and microflow_parameters_data
//	    (likewise ParameterCount but not the parameters). Without the bump a
//	    cached catalog answers "unknown" for every one, which the checker reads
//	    as "cannot tell" and silently skips — a green run that checked nothing.
//	9 — java_action_parameters_data + view: a Java action's parameters, each
//	    with its own Description. Without the bump a cached catalog silently
//	    reports zero parameters, so QUAL002 would under-report rather than
//	    fail (mxcli-formula1: missing documentation on Java actions).
//	2 — split each domain table into <name>_data + <name> view that JOINs
//	    snapshots, removing the denormalized ProjectName / SnapshotDate /
//	    SnapshotSource / SourceId / SourceBranch / SourceRevision columns
//	    from every row (issue #576).
//	1 — initial flat schema with denormalized snapshot columns on every row.
const CatalogSchemaVersion = "32"

// MetaSchemaVersion is the catalog_meta key that records the schema version
// the cache was built against.
const MetaSchemaVersion = "schema_version"

// createTables creates all catalog tables in the SQLite database.
//
// Each domain table is split into two pieces:
//
//   - `<name>_data` is the storage table — the columns that are genuinely
//     owned by this entity, plus `ProjectId` and `SnapshotId` foreign keys.
//   - `<name>` is a view that JOINs `snapshots` to surface
//     `ProjectName`, `SnapshotDate`, `SnapshotSource`, `SourceId`,
//     `SourceBranch`, and `SourceRevision` (whichever the table historically
//     exposed). This preserves the column shape that existing queries
//     — including the `objects` UNION view and ad-hoc user SQL — expect.
//
// Builders INSERT into `<name>_data`. Readers should use `<name>`.
//
// Tables that were already clean (only `ProjectId` + `SnapshotId`, no
// denormalized columns) keep their original shape:
// navigation_menu_items, navigation_role_homes, user_roles, role_mappings, refs,
// permissions, constant_values.
func (c *Catalog) createTables() error {
	schemas := []string{
		// ----- Metadata + lookup tables (source of truth) -----

		`CREATE TABLE IF NOT EXISTS catalog_meta (
			Key TEXT PRIMARY KEY,
			Value TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS projects (
			ProjectId TEXT PRIMARY KEY,
			ProjectName TEXT,
			MendixVersion TEXT,
			CreatedDate TEXT,
			LastSnapshotDate TEXT,
			SnapshotCount INTEGER DEFAULT 0
		)`,

		`CREATE TABLE IF NOT EXISTS snapshots (
			SnapshotId TEXT PRIMARY KEY,
			SnapshotName TEXT,
			ProjectId TEXT,
			ProjectName TEXT,
			SnapshotDate TEXT,
			SnapshotSource TEXT,
			SourceId TEXT,
			SourceBranch TEXT,
			SourceRevision TEXT,
			ObjectCount INTEGER DEFAULT 0,
			IsActive INTEGER DEFAULT 0
		)`,

		// ----- Domain tables: <name>_data + <name> view -----

		// modules
		`CREATE TABLE IF NOT EXISTS modules_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			Source TEXT DEFAULT '',
			AppStoreVersion TEXT,
			AppStoreGuid TEXT,
			-- The module's domain model's documentation. A Mendix module has
			-- no documentation property (Description above is always empty);
			-- its DomainModels$DomainModel unit does, so this is the only
			-- module-level text a project author can write.
			DomainModelDocumentation TEXT DEFAULT '',
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("modules"),

		// entities
		`CREATE TABLE IF NOT EXISTS entities_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			EntityType TEXT,
			Description TEXT,
			Generalization TEXT,
			AttributeCount INTEGER DEFAULT 0,
			AssociationCount INTEGER DEFAULT 0,
			AccessRuleCount INTEGER DEFAULT 0,
			ValidationRuleCount INTEGER DEFAULT 0,
			HasEventHandlers INTEGER DEFAULT 0,
			-- Mendix stores the four audit members as BOOLEANS on the entity's
			-- generalization node, not as attributes, so they are absent from
			-- CATALOG.ATTRIBUTES by construction: "does this entity have a
			-- CreatedDate" was unanswerable from SQL, and a LEFT JOIN against
			-- attributes reported every entity as missing it forever, even
			-- after one was added. DESCRIBE ENTITY renders them in the
			-- attribute list (CreatedDate: AutoCreatedDate), which is what
			-- makes their absence here surprising.
			HasCreatedDate INTEGER DEFAULT 0,
			HasChangedDate INTEGER DEFAULT 0,
			HasOwner INTEGER DEFAULT 0,
			HasChangedBy INTEGER DEFAULT 0,
			IsExternal INTEGER DEFAULT 0,
			ExternalService TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("entities"),

		// associations
		`CREATE TABLE IF NOT EXISTS associations_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			FromEntity TEXT,
			ToEntity TEXT,
			AssociationType TEXT,
			Owner TEXT,
			StorageFormat TEXT,
			Description TEXT,
			-- Delete behaviour, as the raw Mendix values (DeleteMeAndReferences /
			-- DeleteMeButKeepReferences / DeleteMeIfNoReferences). Mendix stores
			-- both ends on every association, default DeleteMeButKeepReferences,
			-- so "set explicitly" can only be read as "not the default".
			--
			-- Named by the MDL ends, not by Mendix's pointer names, because those
			-- are inverted (CLAUDE.md, "Association Parent/Child Pointer
			-- Semantics"): ParentPointer is the FROM entity, ChildPointer the TO
			-- entity. So:
			--   ToDeleteBehavior   = DeleteBehavior.ChildDeleteBehavior  (the end
			--                        MDL's ON DELETE CASCADE|RESTRICT|SET NULL sets)
			--   FromDeleteBehavior = DeleteBehavior.ParentDeleteBehavior (Studio
			--                        Pro only; MDL has no clause for it)
			-- and each *ErrorMessage is the matching Child/ParentErrorMessage text,
			-- shown when a DeleteMeIfNoReferences delete is refused.
			ToDeleteBehavior TEXT DEFAULT '',
			FromDeleteBehavior TEXT DEFAULT '',
			ToDeleteErrorMessage TEXT DEFAULT '',
			FromDeleteErrorMessage TEXT DEFAULT '',
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("associations"),

		// attributes
		`CREATE TABLE IF NOT EXISTS attributes_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			EntityId TEXT,
			EntityQualifiedName TEXT,
			ModuleName TEXT,
			DataType TEXT,
			-- DataType is the bare kind ("Enumeration"), so the enum's identity
			-- needs its own column. Kept separate rather than folded into
			-- DataType as "Enumeration:QN" because existing queries and lint
			-- rules match DataType by equality.
			EnumerationQualifiedName TEXT,
			Length INTEGER,
			IsUnique INTEGER DEFAULT 0,
			IsRequired INTEGER DEFAULT 0,
			DefaultValue TEXT,
			IsCalculated INTEGER DEFAULT 0,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("attributes"),

		// microflows (includes nanoflows; filtered by view below)
		`CREATE TABLE IF NOT EXISTS microflows_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			MicroflowType TEXT,
			Description TEXT,
			ReturnType TEXT,
			ParameterCount INTEGER DEFAULT 0,
			ActivityCount INTEGER DEFAULT 0,
			TotalActivityCount INTEGER DEFAULT 0,
			Complexity INTEGER DEFAULT 1,
			Excluded BOOLEAN DEFAULT 0,
			-- The flow's "Apply entity access" setting; always 0 for a nanoflow,
			-- which has no such setting (ako/mxcli#1453).
			ApplyEntityAccess BOOLEAN DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("microflows"),

		// nanoflows view (filtered subset of microflows view)
		`CREATE VIEW IF NOT EXISTS nanoflows AS
			SELECT * FROM microflows WHERE MicroflowType = 'NANOFLOW'`,

		// rules view (filtered subset of microflows view). A rule shares the
		// storage because it shares the shape — signature, body, complexity —
		// but it is a distinct doctype, so `microflows` is itself filtered to
		// MicroflowType = 'MICROFLOW' wherever it is presented as microflows.
		`CREATE VIEW IF NOT EXISTS rules AS
			SELECT * FROM microflows WHERE MicroflowType = 'RULE'`,

		// microflow_parameters: one row per parameter, for microflows and
		// nanoflows alike. microflows_data carries only ParameterCount, so a
		// caller could see that a flow takes three arguments but not what they
		// are — which is what typing a CALL's arguments requires. ParameterType
		// uses the same encoding as microflows_data.ReturnType ("String",
		// "Object:Mod.Entity", "Enumeration:Mod.Enum", …).
		`CREATE TABLE IF NOT EXISTS microflow_parameters_data (
			Id TEXT PRIMARY KEY,
			MicroflowId TEXT,
			MicroflowQualifiedName TEXT,
			ModuleName TEXT,
			Name TEXT,
			ParameterType TEXT,
			Description TEXT,
			Ordinal INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("microflow_parameters"),

		// pages
		`CREATE TABLE IF NOT EXISTS pages_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Title TEXT,
			URL TEXT,
			LayoutRef TEXT,
			Description TEXT,
			ParameterCount INTEGER DEFAULT 0,
			WidgetCount INTEGER DEFAULT 0,
			Excluded BOOLEAN DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("pages"),

		// snippets
		`CREATE TABLE IF NOT EXISTS snippets_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ParameterCount INTEGER DEFAULT 0,
			WidgetCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("snippets"),

		// building blocks (read-only reusable widget compositions)
		`CREATE TABLE IF NOT EXISTS building_blocks_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			DisplayName TEXT,
			Platform TEXT,
			Category TEXT,
			WidgetCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("building_blocks"),

		// layouts
		`CREATE TABLE IF NOT EXISTS layouts_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			LayoutType TEXT,
			Platform TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("layouts"),

		// enumerations
		`CREATE TABLE IF NOT EXISTS enumerations_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ValueCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("enumerations"),

		// enumeration_values: one row per value. enumerations_data carries only
		// ValueCount, which cannot answer "is 'Open' a case of this enum" —
		// the check behind the most common expression bug (comparing an enum
		// attribute to a string literal).
		`CREATE TABLE IF NOT EXISTS enumeration_values_data (
			Id TEXT PRIMARY KEY,
			EnumerationId TEXT,
			EnumerationQualifiedName TEXT,
			ModuleName TEXT,
			Name TEXT,
			Caption TEXT,
			Ordinal INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("enumeration_values"),

		// java_actions. ReturnType (and java_action_parameters.ParameterType)
		// encode a type-parameter reference as `TypeParameter:<name>` and the
		// entity-type selector as `EntityTypeParameter:<name>` — a bare name
		// cannot tell a type parameter called String from the primitive
		// (mendixlabs/mxcli#1183). See catalogCodeActionType.
		`CREATE TABLE IF NOT EXISTS java_actions_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Documentation TEXT,
			ExportLevel TEXT,
			ReturnType TEXT,
			ParameterCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("java_actions"),

		// java_action_parameters — one row per parameter of a Java action.
		// Separate from java_actions (which only counts them) because a
		// parameter carries its own Description, and an undocumented parameter
		// is the thing a caller actually sees in Studio Pro's action dialog.
		`CREATE TABLE IF NOT EXISTS java_action_parameters_data (
			Id TEXT PRIMARY KEY,
			JavaActionId TEXT,
			JavaActionName TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Name TEXT,
			Description TEXT,
			ParameterType TEXT,
			IsRequired INTEGER DEFAULT 0,
			Ordinal INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("java_action_parameters"),

		// javascript_actions
		`CREATE TABLE IF NOT EXISTS javascript_actions_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("javascript_actions"),

		// image_collections
		`CREATE TABLE IF NOT EXISTS image_collections_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("image_collections"),

		// icon_collections (custom icon sets; read-only, referenced by widgets)
		`CREATE TABLE IF NOT EXISTS icon_collections_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("icon_collections"),

		// page_templates (Forms$PageTemplate; the starting points Studio Pro's
		// "new page" dialog offers). A separate table from pages: templates are a
		// different document type whose content hangs off LayoutCall, and folding
		// them into pages made every module that ships templates report pages it
		// does not have.
		`CREATE TABLE IF NOT EXISTS page_templates_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("page_templates"),

		// menus (standalone Menus$MenuDocument; read-only reusable menus)
		`CREATE TABLE IF NOT EXISTS menus_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("menus"),

		// scheduled_events — Mendix's cron. Repeat/RepeatDescription come from the
		// Schedule child; IntervalSeconds is derived from it, NOT from the legacy
		// Interval/IntervalType pair (Studio Pro does not keep those in sync).
		`CREATE TABLE IF NOT EXISTS scheduled_events_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			Microflow TEXT,
			Repeat TEXT,
			RepeatDescription TEXT,
			IntervalSeconds INTEGER,
			Enabled INTEGER,
			TimeZone TEXT,
			OnOverlap TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("scheduled_events"),

		// regular_expressions — named patterns referenced by attribute
		// validation rules (DomainModels$RegExRuleInfo.RegExIdentifier).
		`CREATE TABLE IF NOT EXISTS regular_expressions_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			Expression TEXT,
			ExportLevel TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("regular_expressions"),

		// queues — task queues (Queues$Queue). Parallelism is an expression
		// string, not a number.
		`CREATE TABLE IF NOT EXISTS queues_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			Parallelism TEXT,
			ClusterWide INTEGER,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshotOver("task_queues", "queues_data"),
		legacyAliasView("queues", "task_queues"),

		// data_transformers
		`CREATE TABLE IF NOT EXISTS data_transformers_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("data_transformers"),

		// message definitions, in both storage shapes: the collection that
		// holds several definitions (Mendix < 11.15) and the one-definition
		// MessageDefinition2 document that replaced it (ako/mxcli#1433).
		`CREATE TABLE IF NOT EXISTS message_definition_collections_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("message_definition_collections"),
		`CREATE TABLE IF NOT EXISTS message_definitions_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("message_definitions"),

		// agent-editor documents (CustomBlobDocuments, discriminated by
		// CustomDocumentType into agent / model / knowledge base / MCP service).
		`CREATE TABLE IF NOT EXISTS agents_data (
			Id TEXT PRIMARY KEY, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, Description TEXT, ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("agents"),
		`CREATE TABLE IF NOT EXISTS ai_models_data (
			Id TEXT PRIMARY KEY, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, Description TEXT, ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("ai_models"),
		`CREATE TABLE IF NOT EXISTS knowledge_bases_data (
			Id TEXT PRIMARY KEY, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, Description TEXT, ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("knowledge_bases"),
		`CREATE TABLE IF NOT EXISTS consumed_mcp_services_data (
			Id TEXT PRIMARY KEY, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, Description TEXT, ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("consumed_mcp_services"),

		// activities
		`CREATE TABLE IF NOT EXISTS activities_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			Caption TEXT,
			ActivityType TEXT,
			Sequence INTEGER DEFAULT 0,
			MicroflowId TEXT,
			MicroflowQualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			EntityRef TEXT,
			ActionType TEXT,
			ServiceRef TEXT,
			ActionRef TEXT,
			UseRequestTimeout INTEGER DEFAULT 0,
			TimeoutExpression TEXT,
			Description TEXT,
			ParentLoopId TEXT DEFAULT '',
			LoopDepth INTEGER DEFAULT 0,
			AutoGenerateCaption INTEGER DEFAULT 0,
			ConditionExpression TEXT DEFAULT '',
			ConditionRule TEXT DEFAULT '',
			ErrorHandlingType TEXT DEFAULT '',
			LogLevel TEXT DEFAULT '',
			LogNodeExpression TEXT DEFAULT '',
			LogMessage TEXT DEFAULT '',
			CommitType TEXT DEFAULT '',
			WithEvents INTEGER DEFAULT 0,
			RetrieveSource TEXT DEFAULT '',
			QueueRef TEXT DEFAULT '',
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("activities"),

		// widget_definitions — widgets AVAILABLE in the project (.mpk / .def.json)
		`CREATE TABLE IF NOT EXISTS widget_definitions_data (
			WidgetId TEXT PRIMARY KEY,
			MdlName TEXT,
			DisplayName TEXT,
			WidgetKind TEXT,
			Version TEXT,
			MpkPath TEXT,
			DefPath TEXT,
			PropertyCount INTEGER,
			ChildSlotCount INTEGER,
			ObjectListCount INTEGER,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("widget_definitions"),

		// widget_definition_properties — one row per property / child slot / object list
		`CREATE TABLE IF NOT EXISTS widget_definition_properties_data (
			Id TEXT PRIMARY KEY,
			WidgetId TEXT,
			PropertyKey TEXT,
			Kind TEXT,
			Type TEXT,
			MdlKeyword TEXT,
			Required INTEGER,
			DefaultValue TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("widget_definition_properties"),

		// widgets — one row per widget INSTANCE (use site)
		`CREATE TABLE IF NOT EXISTS widgets_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			WidgetType TEXT,
			ContainerId TEXT,
			ContainerQualifiedName TEXT,
			ContainerType TEXT,
			ModuleName TEXT,
			Folder TEXT,
			EntityRef TEXT,
			AttributeRef TEXT,
			MicroflowRef TEXT,
			NanoflowRef TEXT,
			-- The page a widget's action opens (show_page, and the page half of
			-- "create object … then open page"). Without it a page reachable only
			-- from a button had no inbound reference at all and read as dead code
			-- (issue #773).
			PageRef TEXT,
			Description TEXT,
			-- Tree position (mendixlabs/mxcli#1268): the nearest INDEXED
			-- ancestor (skipped wrappers and grid rows/columns are
			-- transparent; empty at the root) and the number of indexed
			-- ancestors (0 at the page or snippet root; a list view template
			-- is a level; a snippet call is not entered).
			ParentWidgetId TEXT,
			Depth INTEGER DEFAULT 0,
			-- Forms$Appearance
			Class TEXT,
			Style TEXT,
			DynamicClasses TEXT,
			-- Stored $Type of the primary action (Action, else OnClickAction,
			-- else ClickAction) and whether it carries a ConfirmationInfo —
			-- only microflow, nanoflow and workflow calls can.
			ActionType TEXT,
			HasConfirmation INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("widgets"),

		// xpath_expressions
		`CREATE TABLE IF NOT EXISTS xpath_expressions_data (
			Id TEXT PRIMARY KEY,
			DocumentType TEXT,
			DocumentId TEXT,
			DocumentQualifiedName TEXT,
			ComponentType TEXT,
			ComponentId TEXT,
			ComponentName TEXT,
			XPathExpression TEXT,
			XPathAST TEXT,
			TargetEntity TEXT,
			ReferencedEntities TEXT,
			IsParameterized INTEGER DEFAULT 0,
			UsageType TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			-- The widget a page or snippet data source belongs to, by the id the
			-- widgets table records for it; empty for flows and access rules.
			WidgetId TEXT,
			WidgetName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("xpath_expressions"),

		// odata_clients
		`CREATE TABLE IF NOT EXISTS odata_clients_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Version TEXT,
			ODataVersion TEXT,
			MetadataUrl TEXT,
			Validated INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshotOver("consumed_odata_services", "odata_clients_data"),
		legacyAliasView("odata_clients", "consumed_odata_services"),

		// odata_services
		`CREATE TABLE IF NOT EXISTS odata_services_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Path TEXT,
			Version TEXT,
			ODataVersion TEXT,
			EntitySetCount INTEGER DEFAULT 0,
			AuthenticationTypes TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshotOver("published_odata_services", "odata_services_data"),
		legacyAliasView("odata_services", "published_odata_services"),

		// workflows
		`CREATE TABLE IF NOT EXISTS workflows_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ExportLevel TEXT,
			ParameterEntity TEXT,
			ActivityCount INTEGER DEFAULT 0,
			UserTaskCount INTEGER DEFAULT 0,
			MicroflowCallCount INTEGER DEFAULT 0,
			DecisionCount INTEGER DEFAULT 0,
			DueDate TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("workflows"),

		// business_event_services
		`CREATE TABLE IF NOT EXISTS business_event_services_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Documentation TEXT,
			ServiceName TEXT,
			EventNamePrefix TEXT,
			MessageCount INTEGER DEFAULT 0,
			PublishCount INTEGER DEFAULT 0,
			SubscribeCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("business_event_services"),

		// navigation_profiles
		`CREATE TABLE IF NOT EXISTS navigation_profiles_data (
			ProfileName TEXT PRIMARY KEY,
			Kind TEXT,
			IsNative INTEGER DEFAULT 0,
			HomePage TEXT,
			HomePageType TEXT,
			LoginPage TEXT,
			NotFoundPage TEXT,
			MenuItemCount INTEGER DEFAULT 0,
			RoleBasedHomeCount INTEGER DEFAULT 0,
			OfflineEntityCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("navigation_profiles"),

		// Offline synchronization, one row per configured entity.
		//
		// navigation_profiles.OfflineEntityCount says HOW MANY and nothing
		// else, so "which entities does this profile sync, and how?" — the
		// question anyone auditing an offline app asks first — needed the
		// document. A count is not a projection of the data; it is a summary
		// that cannot be drilled into.
		//
		// CompatibilityMode is indexed even though MDL cannot author it: the
		// catalog reports what is STORED, and a flag invisible to every query
		// is one nobody discovers until it matters.
		`CREATE TABLE IF NOT EXISTS offline_entity_configs_data (
			ProfileName TEXT,
			ProfileKind TEXT,
			EntityQualifiedName TEXT,
			ModuleName TEXT,
			SyncMode TEXT,
			XPathConstraint TEXT,
			CompatibilityMode INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("offline_entity_configs"),

		// entity_event_handlers — one row per entity event handler.
		// CATALOG.ENTITIES.HasEventHandlers is a flag: it says some exist and
		// nothing else. Which moment, which event and which microflow is the
		// whole question, and refs has no column for the first two — a `before
		// commit` handler that returns false blocks the commit, an `after
		// delete` one cannot. The edge says the microflow is reachable; this
		// table says what it does.
		//
		// Event holds the value Mendix stores: Create/Commit/Delete/RollBack.
		// The capital B in RollBack is Mendix's, confirmed against
		// generated/metamodel, and disagrees with every neighbouring enum
		// there — a query spelling it `Rollback` returns zero rows, not an
		// error.
		`CREATE TABLE IF NOT EXISTS entity_event_handlers_data (
			Id TEXT,
			EntityId TEXT,
			EntityQualifiedName TEXT,
			ModuleName TEXT,
			Moment TEXT,
			Event TEXT,
			Microflow TEXT,
			RaiseErrorOnFalse INTEGER DEFAULT 0,
			PassEventObject INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("entity_event_handlers"),

		// Already-clean tables (no denormalized columns) — kept as plain tables.
		`CREATE TABLE IF NOT EXISTS navigation_menu_items (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			ProfileName TEXT NOT NULL,
			ItemPath TEXT NOT NULL,
			Depth INTEGER DEFAULT 0,
			Caption TEXT,
			ActionType TEXT,
			TargetPage TEXT,
			TargetMicroflow TEXT,
			SubItemCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS navigation_role_homes (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			ProfileName TEXT NOT NULL,
			UserRole TEXT NOT NULL,
			Page TEXT,
			Microflow TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		// rest_clients
		`CREATE TABLE IF NOT EXISTS rest_clients_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			BaseUrl TEXT,
			AuthScheme TEXT,
			OperationCount INTEGER DEFAULT 0,
			Documentation TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshotOver("consumed_rest_services", "rest_clients_data"),
		legacyAliasView("rest_clients", "consumed_rest_services"),

		// rest_operations — partial denormalization (ProjectId, SnapshotId, SnapshotDate, SnapshotSource)
		`CREATE TABLE IF NOT EXISTS rest_operations_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			Name TEXT,
			HttpMethod TEXT,
			Path TEXT,
			ParameterCount INTEGER DEFAULT 0,
			HasBody INTEGER DEFAULT 0,
			ResponseType TEXT,
			Timeout INTEGER DEFAULT 0,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSourceOver("consumed_rest_operations", "rest_operations_data"),
		legacyAliasView("rest_operations", "consumed_rest_operations"),

		// published_rest_services
		`CREATE TABLE IF NOT EXISTS published_rest_services_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Path TEXT,
			Version TEXT,
			ServiceName TEXT,
			ResourceCount INTEGER DEFAULT 0,
			OperationCount INTEGER DEFAULT 0,
			Documentation TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("published_rest_services"),

		// published_rest_operations — partial denormalization
		`CREATE TABLE IF NOT EXISTS published_rest_operations_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			ResourceName TEXT,
			HttpMethod TEXT,
			Path TEXT,
			Summary TEXT,
			Microflow TEXT,
			Deprecated INTEGER DEFAULT 0,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("published_rest_operations"),

		// external_entities — has ProjectName in addition to SnapshotDate/Source
		`CREATE TABLE IF NOT EXISTS external_entities_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			ServiceName TEXT,
			EntitySet TEXT,
			RemoteName TEXT,
			Countable INTEGER DEFAULT 0,
			Creatable INTEGER DEFAULT 0,
			Deletable INTEGER DEFAULT 0,
			Updatable INTEGER DEFAULT 0,
			AttributeCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("external_entities"),

		// external_actions
		`CREATE TABLE IF NOT EXISTS external_actions_data (
			Id TEXT PRIMARY KEY,
			ServiceName TEXT,
			ActionName TEXT,
			ModuleName TEXT,
			UsageCount INTEGER DEFAULT 0,
			CallerNames TEXT,
			ParameterNames TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("external_actions"),

		// business_events — partial denormalization
		`CREATE TABLE IF NOT EXISTS business_events_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			ChannelName TEXT,
			MessageName TEXT,
			CanPublish INTEGER DEFAULT 0,
			CanSubscribe INTEGER DEFAULT 0,
			AttributeCount INTEGER DEFAULT 0,
			Entity TEXT,
			PublishMicroflow TEXT,
			SubscribeMicroflow TEXT,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("business_events"),

		// contract_entities — partial denormalization
		`CREATE TABLE IF NOT EXISTS contract_entities_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			EntityName TEXT,
			EntitySetName TEXT,
			KeyProperties TEXT,
			PropertyCount INTEGER DEFAULT 0,
			NavigationCount INTEGER DEFAULT 0,
			Summary TEXT,
			Description TEXT,
			ModuleName TEXT,
			UsedByExternalEntity TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("contract_entities"),

		// contract_actions
		`CREATE TABLE IF NOT EXISTS contract_actions_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			ActionName TEXT,
			IsBound INTEGER DEFAULT 0,
			ParameterCount INTEGER DEFAULT 0,
			ReturnType TEXT,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("contract_actions"),

		// contract_messages
		`CREATE TABLE IF NOT EXISTS contract_messages_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			ChannelName TEXT,
			OperationType TEXT,
			MessageName TEXT,
			Title TEXT,
			ContentType TEXT,
			PropertyCount INTEGER DEFAULT 0,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("contract_messages"),

		// database_connections
		`CREATE TABLE IF NOT EXISTS database_connections_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			DatabaseType TEXT,
			QueryCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("database_connections"),

		// jar_dependencies
		`CREATE TABLE IF NOT EXISTS jar_dependencies_data (
			Id TEXT PRIMARY KEY,
			ModuleName TEXT,
			GroupId TEXT,
			ArtifactId TEXT,
			Coordinate TEXT,
			Version TEXT,
			IsIncluded INTEGER DEFAULT 1,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("jar_dependencies"),

		// user_roles — one row per project user role, carrying the
		// documentation Studio Pro shows in Project Security (ako/mxcli#1442).
		`CREATE TABLE IF NOT EXISTS user_roles (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			Name TEXT NOT NULL,
			Description TEXT,
			IsAnonymous INTEGER,
			ModuleRoleCount INTEGER,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		// role_mappings, refs, permissions, constant_values — already clean
		`CREATE TABLE IF NOT EXISTS role_mappings (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			UserRoleName TEXT NOT NULL,
			ModuleRoleName TEXT NOT NULL,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS refs (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			SourceType TEXT NOT NULL,
			SourceId TEXT NOT NULL,
			SourceName TEXT NOT NULL,
			TargetType TEXT NOT NULL,
			TargetId TEXT,
			TargetName TEXT NOT NULL,
			RefKind TEXT NOT NULL,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS permissions (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			ModuleRoleName TEXT NOT NULL,
			ElementType TEXT NOT NULL,
			ElementName TEXT NOT NULL,
			MemberName TEXT,
			AccessType TEXT NOT NULL,
			XPathConstraint TEXT,
			DefaultMemberAccessRights TEXT,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT,
			-- The entity access rule a row comes from, and that rule's position
			-- on the entity (from 0). Empty / NULL for a page, flow or service
			-- row, which no access rule produces (ako/mxcli#1437).
			AccessRuleId TEXT,
			AccessRuleIndex INTEGER
		)`,

		// constants — partial denormalization
		`CREATE TABLE IF NOT EXISTS constants_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			DataType TEXT,
			DefaultValue TEXT,
			ExposedToClient INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("constants"),

		`CREATE TABLE IF NOT EXISTS constant_values (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			ConstantName TEXT NOT NULL,
			ConfigurationName TEXT NOT NULL,
			Value TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		// json_structures — partial denormalization
		`CREATE TABLE IF NOT EXISTS json_structures_data (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			Name TEXT NOT NULL,
			QualifiedName TEXT NOT NULL,
			ModuleName TEXT NOT NULL,
			ElementCount INTEGER DEFAULT 0,
			HasSnippet INTEGER DEFAULT 0,
			Documentation TEXT,
			ExportLevel TEXT,
			Folder TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("json_structures"),

		// import_mappings
		`CREATE TABLE IF NOT EXISTS import_mappings_data (
			Id TEXT PRIMARY KEY,
			Name TEXT NOT NULL,
			QualifiedName TEXT NOT NULL,
			ModuleName TEXT NOT NULL,
			SchemaSource TEXT,
			ElementCount INTEGER DEFAULT 0,
			Documentation TEXT,
			Folder TEXT,
			Excluded BOOLEAN DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("import_mappings"),

		// export_mappings
		`CREATE TABLE IF NOT EXISTS export_mappings_data (
			Id TEXT PRIMARY KEY,
			Name TEXT NOT NULL,
			QualifiedName TEXT NOT NULL,
			ModuleName TEXT NOT NULL,
			SchemaSource TEXT,
			NullValueOption TEXT,
			ElementCount INTEGER DEFAULT 0,
			Documentation TEXT,
			Folder TEXT,
			Excluded BOOLEAN DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("export_mappings"),

		// Objects view - union of all object types.
		// Reads through the per-table views so it picks up ProjectName /
		// SnapshotDate / SnapshotSource from snapshots automatically.
		`CREATE VIEW IF NOT EXISTS objects AS
			SELECT Id, 'MODULE' as ObjectType, Name, QualifiedName, '' as ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM modules
			UNION ALL
			SELECT Id, 'ENTITY' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM entities
			UNION ALL
			SELECT Id, 'ASSOCIATION' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM associations
			UNION ALL
			SELECT Id, 'MICROFLOW' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM microflows WHERE MicroflowType = 'MICROFLOW'
			UNION ALL
			SELECT Id, 'NANOFLOW' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM microflows WHERE MicroflowType = 'NANOFLOW'
			UNION ALL
			SELECT Id, 'RULE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM microflows WHERE MicroflowType = 'RULE'
			UNION ALL
			SELECT Id, 'PAGE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM pages
			UNION ALL
			SELECT Id, 'SNIPPET' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM snippets
			UNION ALL
			SELECT Id, 'BUILDING_BLOCK' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM building_blocks
			UNION ALL
			SELECT Id, 'LAYOUT' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM layouts
			UNION ALL
			SELECT Id, 'ENUMERATION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM enumerations
			UNION ALL
			SELECT Id, 'CONSTANT' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM constants
			UNION ALL
			SELECT Id, 'JAVA_ACTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM java_actions
			UNION ALL
			SELECT Id, 'JAVASCRIPT_ACTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM javascript_actions
			UNION ALL
			SELECT Id, 'IMAGE_COLLECTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM image_collections
			UNION ALL
			SELECT Id, 'ICON_COLLECTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM icon_collections
			UNION ALL
			SELECT Id, 'MENU' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM menus
			UNION ALL
			SELECT Id, 'PAGE_TEMPLATE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM page_templates
			UNION ALL
			SELECT Id, 'SCHEDULED_EVENT' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM scheduled_events
			UNION ALL
			SELECT Id, 'TASK_QUEUE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM task_queues
			UNION ALL
			SELECT Id, 'REGULAR_EXPRESSION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM regular_expressions
			UNION ALL
			SELECT Id, 'DATA_TRANSFORMER' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM data_transformers
			UNION ALL
			SELECT Id, 'MESSAGE_DEFINITION_COLLECTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM message_definition_collections
			UNION ALL
			SELECT Id, 'MESSAGE_DEFINITION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM message_definitions
			UNION ALL
			SELECT Id, 'AGENT' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM agents
			UNION ALL
			SELECT Id, 'AI_MODEL' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM ai_models
			UNION ALL
			SELECT Id, 'KNOWLEDGE_BASE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM knowledge_bases
			UNION ALL
			SELECT Id, 'CONSUMED_MCP_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM consumed_mcp_services
			UNION ALL
			SELECT Id, 'CONSUMED_ODATA_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM consumed_odata_services
			UNION ALL
			SELECT Id, 'PUBLISHED_ODATA_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM published_odata_services
			UNION ALL
			SELECT Id, 'WORKFLOW' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM workflows
			UNION ALL
			SELECT Id, 'BUSINESS_EVENT_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM business_event_services
			UNION ALL
			SELECT Id, 'DATABASE_CONNECTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM database_connections
			UNION ALL
			SELECT Id, 'CONSUMED_REST_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM consumed_rest_services
			UNION ALL
			SELECT Id, 'PUBLISHED_REST_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM published_rest_services
			UNION ALL
			SELECT Id, 'EXTERNAL_ENTITY' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM external_entities
			UNION ALL
			SELECT Id, 'EXTERNAL_ACTION' as ObjectType, ActionName as Name, ServiceName || '.' || ActionName as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM external_actions
			UNION ALL
			SELECT Id, 'BUSINESS_EVENT' as ObjectType, MessageName as Name, ServiceQualifiedName || '.' || MessageName as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, SnapshotId || '' as ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM business_events
			UNION ALL
			SELECT Id, 'CONTRACT_ENTITY' as ObjectType, EntityName as Name, ServiceQualifiedName || '.' || EntityName as QualifiedName, ModuleName, '' as Folder, Summary as Description,
				ProjectId, '' as ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM contract_entities
			UNION ALL
			SELECT Id, 'CONTRACT_ACTION' as ObjectType, ActionName as Name, ServiceQualifiedName || '.' || ActionName as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, '' as ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM contract_actions
			UNION ALL
			SELECT Id, 'CONTRACT_MESSAGE' as ObjectType, MessageName as Name, ServiceQualifiedName || '.' || MessageName as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, '' as ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM contract_messages
			UNION ALL
			SELECT CAST(Id AS TEXT), 'JSON_STRUCTURE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM json_structures
			UNION ALL
			SELECT Id, 'IMPORT_MAPPING' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM import_mappings
			UNION ALL
			SELECT Id, 'EXPORT_MAPPING' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM export_mappings
			UNION ALL
			SELECT Id, 'JAR_DEPENDENCY' as ObjectType, Coordinate as Name, ModuleName || '.' || Coordinate as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM jar_dependencies`,

		// FTS5 virtual tables for full-text search
		`CREATE VIRTUAL TABLE IF NOT EXISTS strings USING fts5(
			QualifiedName,
			ObjectType,
			StringValue,
			StringContext,
			Language,
			ElementId,
			ModuleName
		)`,

		// What the source rows were described from: every unit's content
		// hash at the time, whether it held a described document, and which
		// document came from which unit. The next source build compares and
		// re-describes only what changed (builder_source_reuse.go, #1790). A
		// cache without them reuses nothing, so they need no schema bump.
		// Which unit each row came from, as rowid ranges per table; every
		// unit's basis (content hash and container) and whether a per-unit
		// pass reads it; and the qualified names each unit's rows carry. The
		// next build rebuilds only the changed units and the documents that
		// name them, and copies the rest (builder_incremental.go, #1804). A
		// cache without them carries nothing, so they need no schema bump.
		`CREATE TABLE IF NOT EXISTS catalog_row_owners (
			TableName TEXT NOT NULL,
			UnitId TEXT NOT NULL,
			FirstRow INTEGER NOT NULL,
			LastRow INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS catalog_units (
			UnitId TEXT PRIMARY KEY,
			Basis TEXT NOT NULL,
			Owned INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS catalog_unit_names (
			UnitId TEXT NOT NULL,
			QualifiedName TEXT NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS source_units (
			UnitId TEXT PRIMARY KEY,
			Hash TEXT NOT NULL,
			Described INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS source_elements (
			ElementId TEXT NOT NULL,
			UnitId TEXT NOT NULL,
			QualifiedName TEXT NOT NULL,
			ObjectType TEXT NOT NULL
		)`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS source USING fts5(
			QualifiedName,
			ObjectType,
			SourceText,
			ModuleName,
			ElementId UNINDEXED
		)`,

		// Indexes for common queries — target the underlying *_data tables.
		`CREATE INDEX IF NOT EXISTS idx_modules_name ON modules_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_entities_name ON entities_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_entities_module ON entities_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_microflows_name ON microflows_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_microflows_module ON microflows_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_name ON pages_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_module ON pages_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_layouts_name ON layouts_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_layouts_module ON layouts_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_activities_microflow ON activities_data(MicroflowId)`,
		`CREATE INDEX IF NOT EXISTS idx_activities_type ON activities_data(ActivityType)`,
		`CREATE INDEX IF NOT EXISTS idx_widgets_container ON widgets_data(ContainerId)`,
		`CREATE INDEX IF NOT EXISTS idx_widgets_type ON widgets_data(WidgetType)`,
		`CREATE INDEX IF NOT EXISTS idx_widgets_parent ON widgets_data(ParentWidgetId)`,
		`CREATE INDEX IF NOT EXISTS idx_widget_defs_kind ON widget_definitions_data(WidgetKind)`,
		`CREATE INDEX IF NOT EXISTS idx_widget_defs_mdlname ON widget_definitions_data(MdlName)`,
		`CREATE INDEX IF NOT EXISTS idx_widget_def_props_widget ON widget_definition_properties_data(WidgetId)`,
		`CREATE INDEX IF NOT EXISTS idx_widget_def_props_kind ON widget_definition_properties_data(Kind)`,
		`CREATE INDEX IF NOT EXISTS idx_xpath_document ON xpath_expressions_data(DocumentId)`,
		`CREATE INDEX IF NOT EXISTS idx_refs_source ON refs(SourceType, SourceName)`,
		`CREATE INDEX IF NOT EXISTS idx_refs_target ON refs(TargetType, TargetName)`,
		`CREATE INDEX IF NOT EXISTS idx_refs_kind ON refs(RefKind)`,
		`CREATE INDEX IF NOT EXISTS idx_attributes_entity ON attributes_data(EntityId)`,
		`CREATE INDEX IF NOT EXISTS idx_attributes_entity_qname ON attributes_data(EntityQualifiedName)`,
		`CREATE INDEX IF NOT EXISTS idx_enum_values_enum ON enumeration_values_data(EnumerationQualifiedName)`,
		`CREATE INDEX IF NOT EXISTS idx_microflow_params_flow ON microflow_parameters_data(MicroflowQualifiedName)`,
		`CREATE INDEX IF NOT EXISTS idx_java_actions_name ON java_actions_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_java_actions_module ON java_actions_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_odata_clients_name ON odata_clients_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_odata_clients_module ON odata_clients_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_odata_services_name ON odata_services_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_odata_services_module ON odata_services_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_workflows_name ON workflows_data(QualifiedName)`,
		`CREATE INDEX IF NOT EXISTS idx_workflows_module ON workflows_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_be_services_name ON business_event_services_data(QualifiedName)`,
		`CREATE INDEX IF NOT EXISTS idx_be_services_module ON business_event_services_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_role_mappings_user_role ON role_mappings(UserRoleName)`,
		`CREATE INDEX IF NOT EXISTS idx_role_mappings_module_role ON role_mappings(ModuleRoleName)`,
		`CREATE INDEX IF NOT EXISTS idx_role_mappings_module ON role_mappings(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_permissions_role ON permissions(ModuleRoleName)`,
		`CREATE INDEX IF NOT EXISTS idx_permissions_element ON permissions(ElementType, ElementName)`,
		`CREATE INDEX IF NOT EXISTS idx_permissions_access ON permissions(AccessType)`,
		`CREATE INDEX IF NOT EXISTS idx_permissions_module ON permissions(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_menu_items_profile ON navigation_menu_items(ProfileName)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_menu_items_target_page ON navigation_menu_items(TargetPage)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_menu_items_target_mf ON navigation_menu_items(TargetMicroflow)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_role_homes_profile ON navigation_role_homes(ProfileName)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_role_homes_role ON navigation_role_homes(UserRole)`,
		`CREATE INDEX IF NOT EXISTS idx_rest_clients_name ON rest_clients_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_rest_clients_module ON rest_clients_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_rest_operations_service ON rest_operations_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_rest_operations_method ON rest_operations_data(HttpMethod)`,
		`CREATE INDEX IF NOT EXISTS idx_published_rest_name ON published_rest_services_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_published_rest_module ON published_rest_services_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_published_rest_ops_service ON published_rest_operations_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_external_entities_service ON external_entities_data(ServiceName)`,
		`CREATE INDEX IF NOT EXISTS idx_external_entities_module ON external_entities_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_external_actions_service ON external_actions_data(ServiceName)`,
		`CREATE INDEX IF NOT EXISTS idx_external_actions_module ON external_actions_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_business_events_service ON business_events_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_business_events_module ON business_events_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_entities_service ON contract_entities_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_entities_module ON contract_entities_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_actions_service ON contract_actions_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_actions_module ON contract_actions_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_messages_service ON contract_messages_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_messages_module ON contract_messages_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_constants_name ON constants_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_constants_module ON constants_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_constant_values_constant ON constant_values(ConstantName)`,
		`CREATE INDEX IF NOT EXISTS idx_constant_values_config ON constant_values(ConfigurationName)`,
		`CREATE INDEX IF NOT EXISTS idx_jar_deps_module ON jar_dependencies_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_jar_deps_coord ON jar_dependencies_data(Coordinate)`,

		// --- Graph-analysis views (read the refs graph; populated by REFRESH
		// CATALOG FULL). Module is derived from the qualified-name prefix, NOT by
		// joining entities — that would drop non-entity targets. ---

		// graph_god_nodes — degree centrality (most depended-upon / highest fan-out).
		`CREATE VIEW IF NOT EXISTS graph_god_nodes AS
			WITH deg AS (
				-- WIDGET targets are excluded from the ASSET side: a widget
				-- definition belongs to no Mendix module, and every other row here
				-- is a module-qualified document, so it would list as an asset whose
				-- ModuleName is its own name (the ELSE d.Asset fallback below) and
				-- whose ObjectType is NULL. A page's OUT-degree still counts the
				-- widgets it uses, which is a real dependency.
				--
				-- ATTRIBUTE and ENUMERATION_VALUE targets are excluded for the
				-- same reason: they are members of a document, not documents, and
				-- listing them as assets would bury the entity or enumeration
				-- they belong to under its own attributes.
				SELECT TargetName AS Asset, COUNT(*) AS InDeg, 0 AS OutDeg
				FROM refs WHERE TargetName != '' AND TargetType NOT IN ('WIDGET', 'ATTRIBUTE', 'ENUMERATION_VALUE') GROUP BY TargetName
				UNION ALL
				SELECT SourceName AS Asset, 0 AS InDeg, COUNT(*) AS OutDeg
				FROM refs WHERE SourceName != '' GROUP BY SourceName
			)
			SELECT d.Asset,
				(SELECT ObjectType FROM objects WHERE QualifiedName = d.Asset LIMIT 1) AS ObjectType,
				CASE WHEN instr(d.Asset, '.') > 0 THEN substr(d.Asset, 1, instr(d.Asset, '.') - 1) ELSE d.Asset END AS ModuleName,
				SUM(d.InDeg) AS InDegree, SUM(d.OutDeg) AS OutDegree, SUM(d.InDeg) + SUM(d.OutDeg) AS Degree,
				MAX(gc.PageRank) AS PageRank, MAX(gc.Betweenness) AS Betweenness
			FROM deg d
			LEFT JOIN graph_centrality_data gc ON gc.AssetName = d.Asset
			GROUP BY d.Asset`,

		// graph_analysis_scope — which reference kinds reach the asset-level
		// analysis graph, and how many edges each kind contributes.
		//
		// Exists because the two graph families disagree by construction and there
		// was no way to see it from SQL: graph_module_coupling and friends read
		// EVERY kind straight off refs, while the computed pass (communities,
		// graph_cycles, graph_layers, graph_centrality) is restricted to the
		// structural kinds so navigational edges do not blur the clustering. On
		// ako/TestApp that is 110 of 316 edges — a third of the graph — excluded
		// with nothing recording the fact (mendixlabs/mxcli#1060).
		//
		// The IN list is generated from graphRefKinds, so this view cannot drift
		// from the filter it describes.
		`CREATE VIEW IF NOT EXISTS graph_analysis_scope AS
			SELECT RefKind, COUNT(*) AS Edges,
				CASE WHEN RefKind IN (` + graphRefKindsSQL() + `) THEN 1 ELSE 0 END AS InAssetGraph
			FROM refs WHERE SourceName != '' AND TargetName != ''
			GROUP BY RefKind`,

		// graph_module_coupling — cross-module edges ("surprise edges").
		`CREATE VIEW IF NOT EXISTS graph_module_coupling AS
			SELECT substr(SourceName, 1, instr(SourceName, '.') - 1) AS SourceModule,
				substr(TargetName, 1, instr(TargetName, '.') - 1) AS TargetModule,
				COUNT(*) AS Edges,
				group_concat(DISTINCT RefKind) AS RefKinds
			FROM refs
			WHERE instr(SourceName, '.') > 0 AND instr(TargetName, '.') > 0
				AND substr(SourceName, 1, instr(SourceName, '.') - 1) != substr(TargetName, 1, instr(TargetName, '.') - 1)
			GROUP BY SourceModule, TargetModule`,

		// graph_module_cohesion — intra- vs inter-module edge ratio per module.
		`CREATE VIEW IF NOT EXISTS graph_module_cohesion AS
			WITH e AS (
				SELECT substr(SourceName, 1, instr(SourceName, '.') - 1) AS SrcMod,
					CASE WHEN substr(SourceName, 1, instr(SourceName, '.') - 1) = substr(TargetName, 1, instr(TargetName, '.') - 1)
						THEN 1 ELSE 0 END AS Intra
				FROM refs WHERE instr(SourceName, '.') > 0 AND instr(TargetName, '.') > 0
			)
			SELECT SrcMod AS ModuleName,
				SUM(Intra) AS IntraEdges, SUM(1 - Intra) AS InterEdges,
				round(100.0 * SUM(Intra) / COUNT(*), 1) AS CohesionPct
			FROM e GROUP BY SrcMod`,

		// graph_dead_assets — referenceable documents with no inbound edge.
		// Restricted to types that *should* be referenced if used; enums/constants/
		// layouts are excluded because their inbound edges aren't fully captured yet.
		`CREATE VIEW IF NOT EXISTS graph_dead_assets AS
			SELECT o.QualifiedName, o.ObjectType, o.ModuleName
			FROM objects o
			WHERE o.ObjectType IN ('ENTITY', 'MICROFLOW', 'NANOFLOW', 'PAGE', 'SNIPPET')
				AND NOT EXISTS (SELECT 1 FROM refs r WHERE r.TargetName = o.QualifiedName)`,

		// graph_refkind_distribution — the edge vocabulary (calibrates the others).
		`CREATE VIEW IF NOT EXISTS graph_refkind_distribution AS
			SELECT RefKind, SourceType, TargetType, COUNT(*) AS Count,
				round(100.0 * COUNT(*) / (SELECT COUNT(*) FROM refs), 1) AS Pct
			FROM refs GROUP BY RefKind, SourceType, TargetType`,

		// graph_entity_hotspots — entities used by the most flows.
		`CREATE VIEW IF NOT EXISTS graph_entity_hotspots AS
			SELECT TargetName AS Entity,
				COUNT(DISTINCT SourceName) AS UsedByFlows,
				group_concat(DISTINCT ModuleName) AS AcrossModules
			FROM refs
			WHERE TargetType = 'ENTITY' AND SourceType IN ('MICROFLOW', 'NANOFLOW')
			GROUP BY TargetName`,

		// --- Algorithmic graph analysis (populated by REFRESH CATALOG COMMUNITIES,
		// computed by the pure-Go mdl/catalog/graph package). ---

		// communities — Leiden community membership per asset.
		`CREATE TABLE IF NOT EXISTS communities_data (
			AssetName TEXT, ModuleName TEXT, CommunityId INTEGER,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("communities"),

		// graph_cycles — SCC membership for assets in a dependency cycle.
		// ASSET-level, over the structural edge kinds only (graphRefKinds) — the
		// same graph communities and layers are computed on. See
		// graph_module_cycles for the module-level question and
		// graph_analysis_scope for which kinds are in this graph.
		`CREATE TABLE IF NOT EXISTS graph_cycles_data (
			AssetName TEXT, ModuleName TEXT, CycleId INTEGER, CycleSize INTEGER,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("graph_cycles"),

		// graph_module_cycles — SCC membership for MODULES in a dependency cycle.
		//
		// A separate question from graph_cycles, not a rollup of it: modules A and
		// B can depend on each other through four documents that form no cycle
		// between themselves, which is the ordinary shape of a module cycle and why
		// graph_cycles stayed empty while graph_module_coupling listed both
		// directions (mendixlabs/mxcli#1060).
		//
		// Computed over EVERY reference kind, matching graph_module_coupling — the
		// table this one is read next to. A page bound to a layout in another
		// module is a real dependency of that module even though `layout` is not a
		// structural kind for clustering purposes.
		`CREATE TABLE IF NOT EXISTS graph_module_cycles_data (
			ModuleName TEXT, CycleId INTEGER, CycleSize INTEGER, RefKinds TEXT,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("graph_module_cycles"),

		// graph_layers — topological layer (sequence number) per asset.
		`CREATE TABLE IF NOT EXISTS graph_layers_data (
			AssetName TEXT, ModuleName TEXT, Layer INTEGER,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("graph_layers"),

		// graph_centrality — PageRank / betweenness per asset.
		`CREATE TABLE IF NOT EXISTS graph_centrality_data (
			AssetName TEXT, PageRank REAL, Betweenness REAL,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("graph_centrality"),

		// community_summary — per-community size, type breakdown, dominant-module
		// Label, and members.
		`CREATE VIEW IF NOT EXISTS community_summary AS
			SELECT c.CommunityId,
				(SELECT c2.ModuleName FROM communities_data c2 WHERE c2.CommunityId = c.CommunityId
					GROUP BY c2.ModuleName ORDER BY COUNT(*) DESC, c2.ModuleName LIMIT 1) AS Label,
				COUNT(*) AS Size,
				COUNT(DISTINCT c.ModuleName) AS Modules,
				group_concat(c.AssetName) AS Members
			FROM communities_data c GROUP BY c.CommunityId`,

		// graph_module_dependencies — directed module→module edges (the facts a
		// layering/architecture Starlark rule consumes), with kinds and counts.
		`CREATE VIEW IF NOT EXISTS graph_module_dependencies AS
			SELECT substr(SourceName, 1, instr(SourceName, '.') - 1) AS SourceModule,
				substr(TargetName, 1, instr(TargetName, '.') - 1) AS TargetModule,
				RefKind, COUNT(*) AS Edges
			FROM refs
			WHERE instr(SourceName, '.') > 0 AND instr(TargetName, '.') > 0
				AND substr(SourceName, 1, instr(SourceName, '.') - 1) != substr(TargetName, 1, instr(TargetName, '.') - 1)
			GROUP BY SourceModule, TargetModule, RefKind`,

		// graph_integration_surface — cross-community edges classified into the
		// integration mechanism a split would require (UC2 contract list).
		`CREATE VIEW IF NOT EXISTS graph_integration_surface AS
			SELECT cs.CommunityId AS SourceCommunity, ct.CommunityId AS TargetCommunity,
				r.RefKind, COUNT(*) AS Edges,
				CASE r.RefKind
					WHEN 'associate'  THEN 'OData / shared entity'
					WHEN 'retrieve'   THEN 'OData read'
					WHEN 'create'     THEN 'event / REST write'
					WHEN 'change'     THEN 'event / REST write'
					WHEN 'call'       THEN 'REST (published microflow)'
					WHEN 'generalize' THEN 'BLOCKER: inheritance across boundary'
					ELSE 'review'
				END AS Mechanism
			FROM refs r
			JOIN communities_data cs ON r.SourceName = cs.AssetName
			JOIN communities_data ct ON r.TargetName = ct.AssetName
			WHERE cs.CommunityId != ct.CommunityId
			GROUP BY SourceCommunity, TargetCommunity, r.RefKind`,
	}

	for _, schema := range schemas {
		if _, err := c.db.Exec(schema); err != nil {
			return err
		}
	}

	// Record the schema version so future opens of this cache can decide
	// whether to migrate (drop-and-recreate) on mismatch.
	if _, err := c.db.Exec(
		`INSERT OR REPLACE INTO catalog_meta (Key, Value) VALUES (?, ?)`,
		MetaSchemaVersion, CatalogSchemaVersion,
	); err != nil {
		return err
	}

	return nil
}

// viewWithFullSnapshot builds a "CREATE VIEW <name> AS SELECT t.*, JOINed
// columns FROM <name>_data t LEFT JOIN snapshots s …" statement that exposes
// ProjectName + SnapshotDate + SnapshotSource + SourceId + SourceBranch +
// SourceRevision on top of the underlying _data table. Used for tables whose
// pre-refactor schema had all six denormalized columns.
func viewWithFullSnapshot(name string) string {
	return viewWithFullSnapshotOver(name, name+"_data")
}

// viewWithFullSnapshotOver is viewWithFullSnapshot for a view whose name is
// not its storage table's: the document types renamed to Studio Pro's names
// (mendixlabs/mxcli#1395) kept their `_data` tables, which nothing outside the
// builders reads.
func viewWithFullSnapshotOver(name, data string) string {
	return `CREATE VIEW IF NOT EXISTS ` + name + ` AS
		SELECT t.*,
			s.ProjectName,
			s.SnapshotDate,
			s.SnapshotSource,
			s.SourceId,
			s.SourceBranch,
			s.SourceRevision
		FROM ` + data + ` t
		LEFT JOIN snapshots s ON s.SnapshotId = t.SnapshotId`
}

// viewWithProjectNameAndSnapshotDateSource exposes ProjectName + SnapshotDate
// + SnapshotSource. Used for tables whose pre-refactor schema had those three
// denormalized columns but not the Source* group.
func viewWithProjectNameAndSnapshotDateSource(name string) string {
	return `CREATE VIEW IF NOT EXISTS ` + name + ` AS
		SELECT t.*,
			s.ProjectName,
			s.SnapshotDate,
			s.SnapshotSource
		FROM ` + name + `_data t
		LEFT JOIN snapshots s ON s.SnapshotId = t.SnapshotId`
}

// viewWithSnapshotDateSource exposes SnapshotDate + SnapshotSource only.
// Used for tables whose pre-refactor schema had only those two denormalized
// columns (no ProjectName, no Source*).
func viewWithSnapshotDateSource(name string) string {
	return viewWithSnapshotDateSourceOver(name, name+"_data")
}

// viewWithSnapshotDateSourceOver is viewWithSnapshotDateSource over a storage
// table of another name; see viewWithFullSnapshotOver.
func viewWithSnapshotDateSourceOver(name, data string) string {
	return `CREATE VIEW IF NOT EXISTS ` + name + ` AS
		SELECT t.*,
			s.SnapshotDate,
			s.SnapshotSource
		FROM ` + data + ` t
		LEFT JOIN snapshots s ON s.SnapshotId = t.SnapshotId`
}

// LegacyTableAliases maps each catalog view name retired by the Studio Pro
// document names (mendixlabs/mxcli#1395) to its canonical view. The old names
// stay queryable so saved queries and user lint rules keep answering, but
// Tables() lists only the canonical ones.
var LegacyTableAliases = map[string]string{
	"rest_clients":    "consumed_rest_services",
	"rest_operations": "consumed_rest_operations",
	"odata_clients":   "consumed_odata_services",
	"odata_services":  "published_odata_services",
	"queues":          "task_queues",
}

// legacyAliasView keeps a retired view name answering as its canonical view.
func legacyAliasView(old, canonical string) string {
	return `CREATE VIEW IF NOT EXISTS ` + old + ` AS SELECT * FROM ` + canonical
}
