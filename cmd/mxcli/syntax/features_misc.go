// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	// ── Language header ─────────────────────────────────────────────────

	// The `mdl <n>;` header (ADR-0011, ako/mxcli#710). Documented as its own
	// topic because it is a property of the whole script, not of a statement.
	Register(SyntaxFeature{
		Path:    "language-header",
		Summary: "mdl <n>; — the MDL language version a script is written in",
		Keywords: []string{
			"mdl 1", "mdl 0", "language version", "header", "edition",
			"beta", "frozen", "version-gated", "meaning", "fmt --upgrade", "upgrade",
			"--mdl", "repl", "describe",
		},
		Syntax: "mdl <n>;\n\n" +
			"-- The FIRST statement of a script. It declares the language version the\n" +
			"-- whole script is read under. It may be repeated later (concatenated\n" +
			"-- describe output) when it names the same version; a header naming\n" +
			"-- another version is an error.\n" +
			"--\n" +
			"--   mdl 1;      the beta language, frozen: describe and fmt --upgrade\n" +
			"--               write this header, and the REPL and -c start in it.\n" +
			"--   no header   a script file is mdl 0, the alpha meaning: a construct\n" +
			"--               whose meaning is different under mdl 1 keeps its old\n" +
			"--               meaning and warns.\n" +
			"--   mdl 2;      refused: this mxcli does not know that version.\n" +
			"--\n" +
			"-- At the REPL and in `mxcli -c`, input without a header is mdl 1;\n" +
			"-- `--mdl 0` starts in mdl 0, and in the REPL `mdl 0;` / `mdl 1;` switches\n" +
			"-- the session. `mxcli describe --mdl 0` writes the mdl 0 spelling.\n" +
			"--\n" +
			"-- Under mdl 1 parsing is strict (each is a warning without the header):\n" +
			"-- every statement ends with ';' and '/' is not a terminator; '' is the\n" +
			"-- only string escape, so a backslash is an ordinary character; and an\n" +
			"-- unknown or mis-shaped property key in a REST, business event or agent\n" +
			"-- property list is an error; a session command (connect, set format,\n" +
			"-- status, show version, help, …) in a script is an error (MDL-V1-SESSION);\n" +
			"-- and `show entity X` / `show association X`, which print a summary no\n" +
			"-- statement prints any more, are an error (MDL-V1-SHOWSUMMARY). A trailing\n" +
			"-- comma is allowed in every bracketed list, with or without the header.\n" +
			"--\n" +
			"-- A script's meaning never depends on which mxcli release runs it: a\n" +
			"-- change of meaning applies only under the version that introduces it.\n" +
			"-- The header is independent of the Mendix version the project targets.\n" +
			"--\n" +
			"-- `mxcli fmt --upgrade` adds it, after rewriting every construct\n" +
			"-- whose meaning it would change: it adds each missing `;`, deletes `/`\n" +
			"-- lines, writes backslash escapes as the characters they stood for,\n" +
			"-- turns `retrieve … limit 1` into `first`, adds `set` to reassignments,\n" +
			"-- and writes list operations one statement per activity. It refuses, and\n" +
			"-- says why, when a construct has no rewrite: an unknown or mis-shaped\n" +
			"-- property (MDL-V1-PROP/PROPVALUE), `create or replace view entity`\n" +
			"-- (MDL-V1-REPLACE01), a session command in a script (MDL-V1-SESSION),\n" +
			"-- `show entity|association X` (MDL-V1-SHOWSUMMARY), a\n" +
			"-- nested list operation, find/contains on a variable\n" +
			"-- whose type the script does not state, and an escaped line break inside\n" +
			"-- an expression.\n" +
			"-- `mxcli fmt --upgrade --header=false` only rewrites deprecated spellings\n" +
			"-- (MDL-DEPRnnn) and adds no header.\n" +
			"--\n" +
			"-- Every warning with an MDL-V1-* or MDL-DEPRnnn code ends with\n" +
			"-- `(mxcli help <code>)`, which prints its entry: old form, new form,\n" +
			"-- whether fmt --upgrade rewrites it, and the version that refuses it.\n" +
			"-- `mxcli syntax <topic> --deprecated` lists a topic's old spellings; the\n" +
			"-- docs page \"Language versions and migration\" tabulates every code.",
		Example: "mdl 1;\n\ncreate persistent entity MyModule.Customer (\n  Name: String(200)\n);",
		SeeAlso: []string{"create-modifiers"},
	})

	// ── CREATE modifiers ────────────────────────────────────────────────

	// OR MODIFY / OR REPLACE sit on the top-level createStatement rule, so they
	// apply to every CREATE uniformly. Individual entries documented them
	// unevenly — most said nothing, none mentioned OR REPLACE — which read as
	// "not supported here". Documented once, and referenced from the entries
	// where the question comes up, rather than repeated across all 27.
	Register(SyntaxFeature{
		Path:    "create-modifiers",
		Summary: "OR REPLACE / OR MODIFY — re-running a CREATE without dropping first",
		Keywords: []string{
			"or replace", "or modify", "create or replace", "create or modify",
			"idempotent", "upsert", "re-run", "already exists", "overwrite",
		},
		Syntax: "CREATE [OR REPLACE | OR MODIFY] <anything>;\n\n" +
			"-- Applies to every CREATE statement — entity, microflow, page, workflow,\n" +
			"-- REST client, security role, and the rest. Without a modifier, creating\n" +
			"-- something that already exists is an error.\n" +
			"--   OR REPLACE  discard the existing document and write a fresh one\n" +
			"--   OR MODIFY   update the existing document in place\n" +
			"-- Both reuse the existing element's ID, so references from other\n" +
			"-- documents survive.\n" +
			"--\n" +
			"-- A rewrite rebuilds the document from the statement, so properties MDL\n" +
			"-- cannot express are CARRIED OVER rather than reset — a microflow's URL\n" +
			"-- and export level, a queued call's binding, translated captions, an\n" +
			"-- entity's identity. Nothing would report the loss if they were not: the\n" +
			"-- result is a valid document either way, so mxcli check, mx check and\n" +
			"-- mxbuild all pass and only Studio Pro shows what went missing.\n" +
			"--\n" +
			"-- DROP followed by CREATE is a NEW document and keeps none of it, and so\n" +
			"-- is a DESCRIBE -> rename -> exec copy. Where a property HAS a spelling,\n" +
			"-- DESCRIBE emits it and the copy is faithful (see microflow.create);\n" +
			"-- where it does not, DESCRIBE flags the gap as a comment rather than\n" +
			"-- producing output that looks complete.",
		Example: "mdl 1;\nCREATE OR MODIFY MICROFLOW MyModule.ACT_Recalculate ()\nBEGIN\n  RETURN;\nEND;\n\nCREATE OR MODIFY PERSISTENT ENTITY MyModule.Customer (\n  Name: String(200)\n);",
		SeeAlso: []string{"microflow", "domain-model.entity", "page", "document-folder", "create-if-not-exists"},
	})

	// IF EXISTS sits on every document-level alternative of dropStatement, so it
	// is documented once here for the same reason OR MODIFY is (#531).
	Register(SyntaxFeature{
		Path:    "drop-if-exists",
		Summary: "DROP … IF EXISTS — a drop that can be re-run",
		Keywords: []string{
			"drop if exists", "if exists", "drop", "re-run", "rerun",
			"idempotent", "not found", "replayable", "cleanup",
		},
		Syntax: "DROP <document type> IF EXISTS Module.Name;\n" +
			"DROP MODULE IF EXISTS ModuleName;\n" +
			"DROP CONFIGURATION IF EXISTS 'Name';\n" +
			"DROP FOLDER IF EXISTS 'path' IN Module;\n\n" +
			"-- Every document-level DROP accepts IF EXISTS: entity, association,\n" +
			"-- enumeration, constant, microflow, nanoflow, rule, page, layout,\n" +
			"-- snippet, menu, module, queue, scheduled event, regular expression,\n" +
			"-- java/javascript action, odata client/service, business event service,\n" +
			"-- workflow, image collection, json structure, message definition\n" +
			"-- collection, import/export mapping, rest client, published rest\n" +
			"-- service, data transformer, model, consumed mcp service, knowledge\n" +
			"-- base, agent, configuration, folder.\n" +
			"--\n" +
			"-- A missing target -- or a missing module -- is reported as skipped\n" +
			"-- instead of stopping the script. Any other failure still errors. The\n" +
			"-- bare DROP keeps SQL semantics and fails on a missing target.\n" +
			"--\n" +
			"-- Sub-document drops have their own guard: ALTER ENTITY … DROP ATTRIBUTE\n" +
			"-- IF EXISTS, DROP INDEX IF EXISTS, ALTER ENUMERATION … DROP VALUE IF\n" +
			"-- EXISTS, DROP USER ROLE IF EXISTS, DROP DEMO USER IF EXISTS.",
		Example: "mdl 1;\n" +
			"-- a stub that broke a page/workflow cycle, dropped once the real page exists\n" +
			"DROP PAGE IF EXISTS FieldService.Stub;\n" +
			"DROP MICROFLOW IF EXISTS FieldService.ACT_Old;\n" +
			"DROP FOLDER IF EXISTS 'Scratch' IN FieldService;",
		SeeAlso: []string{"create-modifiers"},
	})

	// IF NOT EXISTS sits in every create rule that names one element, and is
	// applied once in the visitor and once in the executor's dispatch, so it is
	// documented once here too (ako/mxcli#731, ADR-0010 R1).
	Register(SyntaxFeature{
		Path:    "create-if-not-exists",
		Summary: "CREATE … IF NOT EXISTS — create an element only when it is absent",
		Keywords: []string{
			"if not exists", "create if not exists", "re-run", "rerun",
			"idempotent", "already exists", "leave alone", "skip",
		},
		Syntax: "CREATE <document type> IF NOT EXISTS Module.Name …;\n" +
			"CREATE MODULE IF NOT EXISTS ModuleName;\n" +
			"CREATE USER ROLE IF NOT EXISTS Name (…);\n" +
			"CREATE DEMO USER IF NOT EXISTS 'name' ( Password: '…', UserRoles: (…) );\n" +
			"CREATE CONFIGURATION IF NOT EXISTS 'Name' (…);\n\n" +
			"-- IF NOT EXISTS goes after the kind's keywords, before the name. When the\n" +
			"-- element already exists the statement is SKIPPED (and says so) and the\n" +
			"-- stored element is left exactly as it is; otherwise it creates, like a\n" +
			"-- plain CREATE.\n" +
			"--\n" +
			"-- It is not CREATE OR MODIFY, which makes the stored element match the\n" +
			"-- statement. Writing both is refused as MDL085. DESCRIBE never emits it.\n" +
			"--\n" +
			"-- Every CREATE that names one element accepts it. Not accepted where\n" +
			"-- there is no one named element to test: ANNOTATION, INDEX (use ALTER\n" +
			"-- ENTITY … ADD INDEX IF NOT EXISTS), VALIDATION RULE, NAVIGATION,\n" +
			"-- TRANSLATIONS and EXTERNAL ENTITIES.",
		Example: "mdl 1;\n" +
			"-- seed a module once; later runs leave hand edits alone\n" +
			"CREATE MODULE IF NOT EXISTS Shop;\n" +
			"CREATE ENUMERATION IF NOT EXISTS Shop.Status (Open 'Open', Closed 'Closed');\n" +
			"CREATE CONSTANT IF NOT EXISTS Shop.ApiUrl ( Type: String, DefaultValue: 'https://api.example.com' );\n" +
			"CREATE MICROFLOW IF NOT EXISTS Shop.ACT_Init ()\nBEGIN\n  RETURN;\nEND;",
		SeeAlso: []string{"create-modifiers", "drop-if-exists"},
	})

	// The folder clause is the other cross-cutting CREATE modifier, and gets one
	// topic for the same reason OR MODIFY does: it applies to every document
	// type, so documenting it in all 27 places would guarantee 27 chances to
	// drift.
	Register(SyntaxFeature{
		Path:    "document-folder",
		Summary: "FOLDER — placing a document in a module folder as you create it",
		Keywords: []string{
			"folder", "folder clause", "place document", "module folder",
			"create in folder", "organise", "organize", "subfolder",
			"folder on create", "folder ignored", "document did not move",
		},
		Syntax: "-- Every document type takes a folder clause on CREATE. Where it goes\n" +
			"-- depends on the statement's shape:\n" +
			"--   Microflows, nanoflows FOLDER 'path'    after the signature, before BEGIN\n" +
			"--   Everything else       FOLDER 'path'    after the qualified name\n" +
			"--                                          (pages and snippets included)\n" +
			"--\n" +
			"-- The Folder: 'path' property that pages, snippets and REST/OData services\n" +
			"-- also take is a deprecated alias (MDL-DEPR105); fmt --upgrade moves it.\n" +
			"--\n" +
			"-- Missing folders in the path are created. Nested paths use '/'.\n" +
			"--\n" +
			"-- On CREATE OR MODIFY the clause MOVES an existing document. Omitting it\n" +
			"-- leaves placement alone — it never returns a document to the module\n" +
			"-- root — so adding a folder to an existing script is safe, and removing\n" +
			"-- one is a no-op. DESCRIBE emits the clause, so a description replays\n" +
			"-- into the same folder.\n" +
			"--\n" +
			"-- To move a document without rewriting it, use MOVE.",
		Example: "mdl 1;\n" +
			"CREATE TASK QUEUE MyModule.Q_Orders FOLDER 'Private/Queues' ( Parallelism: 3 );\n\n" +
			"CREATE IMPORT MAPPING MyModule.IMM_Order FOLDER 'Private/Import mappings'\n" +
			"  WITH JSON STRUCTURE MyModule.JSON_Order {\n" +
			"    CREATE MyModule.Order { Id = id }\n" +
			"  };\n\n" +
			"CREATE PAGE MyModule.OrderList FOLDER 'Orders'\n" +
			"  (\n" +
			"    Title: 'Orders',\n" +
			"    Layout: Atlas_Core.Atlas_Default\n" +
			"  )\n" +
			"  {\n" +
			"    DYNAMICTEXT txtHeading (Content: 'Orders')\n" +
			"  };\n\n" +
			"CREATE OR MODIFY MICROFLOW MyModule.ACT_Sync ()\nFOLDER 'Private/Jobs'\nBEGIN\n  RETURN;\nEND;",
		SeeAlso: []string{"move", "folders", "create-modifiers"},
	})

	// ── Session commands (R7, ako/mxcli#755) ───────────────────────────

	Register(SyntaxFeature{
		Path:    "session-commands",
		Summary: "REPL commands that set up the session: connect, set format, status, help, … — not for scripts",
		Keywords: []string{
			"session", "session command", "repl", "repl command", "meta-command",
			"connect", "disconnect", "use", "set format", "status", "check", "build",
			"lint", "debug", "execute script", "execute runtime", "help", "introspect",
			"show version", "show status", "show connections", "show catalog status",
			"MDL-V1-SESSION",
		},
		Syntax: "CONNECT LOCAL '<app.mpr>';   DISCONNECT;   STATUS;\n" +
			"SET format = json|table;       USE <session> | USE ALL;\n" +
			"CHECK;  BUILD;  LINT [target];  DEBUG '<…>';  INTROSPECT API;\n" +
			"EXECUTE SCRIPT '<file.mdl>';   EXECUTE RUNTIME '<command>';\n" +
			"HELP [topic];\n" +
			"SHOW VERSION;  SHOW STATUS;  SHOW CONNECTIONS;  SHOW CATALOG STATUS;\n\n" +
			"-- A session command needs a session or an environment: a connection, an\n" +
			"-- output format, a build, a running app. It is typed at the REPL, or given\n" +
			"-- as a command-line flag. A .mdl script holds model statements only:\n" +
			"--\n" +
			"--   mxcli exec script.mdl -p app.mpr --json\n" +
			"--\n" +
			"-- Under `mdl 1;` a session command in a script is an error; without the\n" +
			"-- header it runs as before and warns MDL-V1-SESSION. The REPL keeps\n" +
			"-- accepting them. EXIT / QUIT end a script and are not session commands.",
		Example: "-- at the REPL\nCONNECT LOCAL '/projects/MyApp/MyApp.mpr';\nSET format = json;\nSTATUS;\n\n" +
			"-- from the shell, for a script\nmxcli exec changes.mdl -p /projects/MyApp/MyApp.mpr --json",
		SeeAlso: []string{"connect", "disconnect", "status", "language-header"},
	})

	// ── Connection ──────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "connect",
		Summary: "Connect to a Mendix project (.mpr file) for the current REPL session",
		Keywords: []string{
			"connect", "connect local", "connect project",
			"open project", "mpr", "connection",
		},
		Syntax: `CONNECT LOCAL '<path/to/app.mpr>';
CONNECT LOCAL '<path>' BRANCH '<branch>';

-- CLI flags (equivalent, and the form for a script)
mxcli -p <path/to/app.mpr> -c "<statement>"
mxcli exec script.mdl -p <path/to/app.mpr>

-- A session command: typed at the REPL. In a script it is an error under
-- mdl 1 and a warning (MDL-V1-SESSION) without the header.`,
		Example: `-- at the REPL
CONNECT LOCAL '/projects/MyApp/MyApp.mpr';
CREATE ENTITY MyModule.Product ( Name: String(200) );
DISCONNECT;`,
		SeeAlso: []string{"disconnect", "status", "session-commands"},
	})

	Register(SyntaxFeature{
		Path:    "disconnect",
		Summary: "Close the current project connection",
		Keywords: []string{
			"disconnect", "close", "close connection", "close project",
		},
		Syntax:  "DISCONNECT;\n\n-- A session command, for the REPL (see session-commands).",
		Example: "DISCONNECT;",
		SeeAlso: []string{"connect", "status", "session-commands"},
	})

	Register(SyntaxFeature{
		Path:    "status",
		Summary: "Show the current connection status: project path, Mendix version, and module count",
		Keywords: []string{
			"status", "show status", "connection status",
			"project info", "version", "connected",
		},
		Syntax:  "STATUS;\nSHOW STATUS;\n\n-- A session command, for the REPL (see session-commands). In a script it\n-- warns MDL-V1-SESSION, and under `mdl 1;` it is an error.",
		Example: "STATUS;\n-- Output: Connected to /projects/MyApp/MyApp.mpr (Mendix 10.24.0, 5 modules)",
		SeeAlso: []string{"connect", "disconnect", "session-commands"},
	})

	// ── Navigation ──────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "navigation",
		Summary: "Navigation profiles — home pages, menus, login pages per device type",
		Keywords: []string{
			"navigation", "nav", "profile", "responsive", "phone", "tablet",
			"home page", "menu", "login page",
		},
		Syntax:  "LIST NAVIGATION;\nDESCRIBE NAVIGATION [profile];\nCREATE OR MODIFY NAVIGATION <profile> ...;",
		Example: "LIST NAVIGATION;\nDESCRIBE NAVIGATION Responsive;",
		SeeAlso: []string{"navigation.show", "navigation.create", "navigation.alter"},
	})

	Register(SyntaxFeature{
		Path:    "navigation.show",
		Summary: "List navigation profiles, menus, and home page assignments",
		Keywords: []string{
			"list navigation", "list navigation", "describe navigation", "navigation menu",
			"navigation homes", "list profiles",
		},
		Syntax: "LIST NAVIGATION;                  -- the profiles, one row each\nLIST NAVIGATION MENU [<profile>];  -- the menu tree\n" +
			"LIST NAVIGATION HOMES;\nDESCRIBE NAVIGATION [<profile>];    -- the profile as MDL\n\n" +
			"-- `list navigation [menu]` is a deprecated alias of the list form (MDL-DEPR002).",
		Example: "LIST NAVIGATION;\nLIST NAVIGATION MENU Responsive;\nDESCRIBE NAVIGATION Responsive;",
	})

	Register(SyntaxFeature{
		Path:    "navigation.create",
		Summary: "Create or replace a navigation profile with home pages, menus, and login page",
		Keywords: []string{
			"create navigation", "replace navigation", "home page",
			"login page", "not found page", "menu item", "menu icon",
			"navigation profile", "phone profile", "tablet profile",
			"offline profile", "offline navigation", "sync", "synchronization",
			"offline sync", "offline entity", "pwa", "progressive web app", "service worker", "precaching", "download mode",
			"throw error", "sync error", "partial sync", "server rejects",
		},
		Syntax: `CREATE OR REPLACE NAVIGATION <profile>
  HOME PAGE Module.Page | HOME MICROFLOW Module.Flow | HOME NANOFLOW Module.Flow
  [HOME PAGE Module.Page FOR UserRole]
  [LOGIN PAGE Module.LoginPage]
  [NOT FOUND PAGE Module.Custom404]
  [ON SYNC ERROR THROW|CONTINUE]
  [PROGRESSIVE WEB APP [( Precaching: true|false, InstallPrompt: true|false )] | PROGRESSIVE WEB APP OFF]
  [SYNC (
    SYNC Module.Entity ONLINE;
    SYNC Module.Entity ALL;
    SYNC Module.Entity WHERE [Amount > 0];
    SYNC Module.Entity NEVER;
    SYNC Module.Entity NONE;
    SYNC Module.Entity NONE PRESERVE DATA;
  )]
  [{
    MENU ITEM 'Label' [( OnClick: SHOW PAGE Module.Page | CALL MICROFLOW Module.Flow
                                | CALL NANOFLOW Module.Flow | OPEN LINK 'url'
                                | CREATE OBJECT Module.Entity [THEN SHOW PAGE Module.Page]
                                | SIGN OUT | NOTHING
                                [WITH (ProgressBar: Blocking, Confirmation: '…', …)]
                        [, Icon: Module.IconCollection.Name] )]
    MENU 'Group' [( Icon: Module.IconCollection.Name )] { ... }
  }];

-- The menu items are the profile's CHILDREN, in { } after its clauses, like a
-- page's widgets: no ; between them, since a child ends in ) or }. An item's
-- action is OnClick: in the words a page action uses. The old spelling,
-- MENU ( MENU ITEM 'Label' PAGE M.P ICON I; ... ), still parses and warns
-- (MDL-DEPR121, MDL-DEPR122); mxcli fmt --upgrade rewrites it.
--
-- OnClick: takes a button's action expression with its WITH ( … ) settings,
-- limited to what a menu item can do (save, delete, close page and complete
-- task are refused). DESCRIBE prints every stored action, so its output
-- re-runs without writing anything; a page title override or a link type
-- other than Web has no spelling, is flagged with a comment, and is kept
-- while the item's caption and action are unchanged.
--
-- A NATIVE profile's flow home is HOME NANOFLOW; mxcli writes its home pages
-- and SYNC block only and refuses a { } block (the bottom bar), LOGIN PAGE,
-- NOT FOUND PAGE, ON SYNC ERROR and PROGRESSIVE WEB APP on it.

-- FOR takes a USER role, written BARE (FOR Administrator). User roles are
-- project-level and have no module part; a module role is a different thing
-- that often shares the name (a blank app has a user role Administrator and
-- module roles called Administrator in three modules). A qualified name here
-- gives a project Mendix cannot LOAD -- StorageLoadException "not a valid
-- UserRoleIdentifier", raised before checking runs, so there is no error code
-- and no line number. List the real ones with LIST USER ROLES.
--
-- Icon: is a qualified name into an ICON COLLECTION (Atlas_Core.Atlas,
-- Atlas_Core.Atlas_Filled, Atlas_Core.Atlas_Styling, or your own) -- a model
-- reference, not a string. Hyphenated Atlas names are double-quoted:
--   Icon: Atlas_Core.Atlas."align-center"
-- Browse the available names with:
--   LIST ICON COLLECTIONS  /  DESCRIBE ICON COLLECTION Module.Name
--
-- <profile> is one of Mendix's fixed web kinds, and the profile is CREATED if
-- the project does not have it yet:
--   Responsive  Phone  Tablet                       online
--   ResponsiveOffline  PhoneOffline  TabletOffline  offline
--
-- SYNC configures offline synchronization, and an offline profile downloads
-- NOTHING until its entities have one -- a profile with no SYNC block builds,
-- routes and installs as a PWA, and shows an empty app.
--
-- The six modes are the members Mendix stores, NOT the captions Studio Pro
-- shows: its "All Objects" is ALL and its "By XPath" is WHERE. WHERE implies
-- the constrained mode rather than naming it, so a constraint without a mode
-- and a mode without a constraint are both unspellable.
--
--   ONLINE               fetched from the server, never held on the device
--   ALL                  every object downloaded
--   WHERE [<xpath>]      only the objects the XPath selects
--   NEVER                not synchronized
--   NONE                 not downloaded; anything already on the device is dropped
--   NONE PRESERVE DATA   not downloaded; what is on the device stays
--
-- WHERE takes the XPath in BRACKETS, verbatim -- nothing inside is escaped.
-- A quoted WHERE '<xpath>' still parses, but every quote inside it doubles,
-- and a stored constraint already carries Mendix's own escaping, so the two
-- compose into runs of six quotes. DESCRIBE emits the bracket form.
--
-- ON SYNC ERROR is Studio Pro's "Throw error when server rejects objects
-- during synchronization", and defaults to THROW. It uses the phrase MDL
-- already has for failure handling (a microflow's ON ERROR CONTINUE) rather
-- than a new keyword. OMITTING it leaves the stored value alone; DESCRIBE emits
-- it only when it is not the default.
--
-- The block REPLACES the stored list, the way { } replaces the menu. An
-- entity's compatibility-mode flag has no syntax and is preserved across the
-- rewrite untouched; DESCRIBE NAVIGATION flags it rather than dropping it.
-- An invented name ("Mobile") is an error: the runtime routes on User-Agent to
-- Mendix's own kinds, so a profile the platform does not define can never route.
--
-- PROGRESSIVE WEB APP is Studio Pro's "Progressive web app" settings
-- (Navigation$ProgressiveWebAppSettings), null until set. An OFFLINE profile
-- needs them: without them Mendix registers no service worker ("registerServiceWorker":
-- false in the built index.js), so no page opens without a network, while
-- check, exec and mx check all pass (mendixlabs/mxcli#1377). Exec warns about
-- an offline profile left without them. The keys are the stored names:
--   Precaching      pre-load the app's pages and resources (default false)
--   InstallPrompt   allow the "Add to home screen" prompt (default true)
-- A key the clause leaves out takes its default (a bare clause writes both
-- defaults), OFF stores null again, and OMITTING the clause
-- leaves the stored settings alone. DESCRIBE emits it when settings are
-- stored, naming only the keys that differ from the defaults.
--
-- An OFFLINE profile restricts every page it can reach -- an attribute may be
-- bound across at most ONE association hop (CE6206). Creating one reports the
-- documents in the project that already exceed that.`,
		Example: `mdl 1;
CREATE OR MODIFY NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web
  HOME PAGE MyModule.AdminDashboard FOR Administrator
  LOGIN PAGE Administration.Login
  {
    MENU ITEM 'Home' ( OnClick: SHOW PAGE MyModule.Home_Web, Icon: Atlas_Core.Atlas.home )
    MENU 'Orders' ( Icon: Atlas_Core.Atlas."shopping-cart" ) {
      MENU ITEM 'All Orders' ( OnClick: SHOW PAGE Orders.Order_Overview, Icon: Atlas_Core.Atlas."list-bullets" )
      MENU ITEM 'New Order' ( OnClick: SHOW PAGE Orders.Order_New, Icon: Atlas_Core.Atlas.add )
    }
    MENU ITEM 'Log out' ( OnClick: SIGN OUT, Icon: Atlas_Core.Atlas."log-out" )
  };

CREATE OR MODIFY NAVIGATION TabletOffline
  HOME PAGE Maintenance.Request_Overview
  {
    MENU ITEM 'Requests' ( OnClick: SHOW PAGE Maintenance.Request_Overview )
  };`,
		SeeAlso: []string{"navigation.show"},
	})

	Register(SyntaxFeature{
		Path:    "navigation.alter",
		Summary: "Modify navigation via round-trip: DESCRIBE, edit, CREATE OR REPLACE",
		Keywords: []string{
			"alter navigation", "modify navigation", "update navigation",
			"round-trip", "edit menu",
		},
		Syntax:  "-- Round-trip workflow:\n-- 1. DESCRIBE NAVIGATION <profile>;\n-- 2. Copy output, modify\n-- 3. Paste as CREATE OR REPLACE NAVIGATION ...",
		Example: "-- Inspect current state\nDESCRIBE NAVIGATION Responsive;\n-- Copy output, modify, paste back as CREATE OR REPLACE",
		SeeAlso: []string{"navigation.create", "navigation.show"},
	})

	// ── Settings ────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "settings",
		Summary: "Project settings — model, configuration, constants, language, workflows",
		Keywords: []string{
			"settings", "project settings", "configuration",
			"startup", "shutdown", "hash algorithm", "java version",
		},
		Syntax:  "DESCRIBE SETTINGS;\nDESCRIBE SETTINGS CONFIGURATION '<name>';   -- just one configuration\nALTER SETTINGS RUNTIME (<key>: <value>, ...);   -- MODEL is a deprecated alias\nALTER SETTINGS CONFIGURATION '<name>' (<key>: <value>, ...);",
		Example: "ALTER SETTINGS RUNTIME (AfterStartupMicroflow: 'Module.MF_Startup');",
		SeeAlso: []string{"settings.show", "settings.alter"},
	})

	Register(SyntaxFeature{
		Path:    "translations",
		Summary: "Bulk translation of every user-visible string, one file per language",
		Keywords: []string{
			"translations", "translate", "language", "languages", "i18n",
			"localisation", "localization", "multilingual", "nl_NL", "de_DE",
		},
		Syntax: `DESCRIBE TRANSLATIONS [IN <Module> | WITHOUT MARKETPLACE] FOR <lang>;
CREATE [OR MODIFY|REPLACE] TRANSLATIONS [IN <Module> | WITHOUT MARKETPLACE] FOR <lang> (
    '<source>' AS '<translation>',
    ...
);

Entries use AS, not a colon: a translation maps a user-provided name to
another name, the same shape CUSTOM NAME map uses.

The thing that exists is the LANGUAGE, so the three verbs read directly:

  CREATE             the language has none yet — errors if it does
  CREATE OR MODIFY   merge; a source the file does not name is left alone
  CREATE OR REPLACE  the file is authoritative; a translation whose source
                     it does not name is REMOVED, and the run says which

IN <Module> scopes both directions. Under OR REPLACE it also BOUNDS the
deletion, so per-module files do not wipe each other on every run.

It does NOT reach PROJECT-level documents, and the navigation is one — so a
scoped run leaves the menu in the source language while the pages switch,
which reads as a half-applied translation rather than a scoping decision.
A scoped run now names the file's own entries it did not reach; re-run the
same file without IN <Module> to land those too.

Without IN, the run reaches the WHOLE project — Marketplace modules
included, Atlas page templates and building blocks among them. A module
update replaces a Marketplace module's contents, so what lands there is
lost at the next update. The run warns with a count per module; add
WITHOUT MARKETPLACE to keep it to your own modules (it reports the file's
entries it left alone), or name one module with IN <Module>.

Keyed on the source string, so one entry translates every occurrence.
DESCRIBE emits the CREATE form, and an untranslated string comes back with
an empty target — which is what makes the output an LLM prompt:

  mxcli -p app.mpr -c "describe translations for de_DE" > de_DE.mdl
  # fill in the right-hand side
  mxcli exec de_DE.mdl -p app.mpr

A key that matches nothing is REPORTED, not skipped: a source edited after
the file was written stops matching, and the run names the string it has
probably become.

An EMPTY entry list is legal: under OR REPLACE it names nothing, so every
translation in scope is removed. That is the way to take a language's
translations out of the model:

  create or replace translations for de_DE ( );

A translation for a language the project has not ENABLED is stored, passes
mx check, and is DISCARDED at build time — no translations_<code>.properties
is produced at all. The run warns; enable the language in project settings
first. Note LIST LANGUAGES lists languages that HAVE translations, not the
enabled ones (8 vs 1 on a stock app); the enabled list is in DESCRIBE
SETTINGS.`,
		Example: `mdl 1;
describe translations for nl_NL;

create or modify translations in Administration for nl_NL (
    'Save'            as 'Opslaan',
    'My Account'      as 'Mijn account',
);`,
		SeeAlso: []string{"settings", "settings.alter"},
	})

	Register(SyntaxFeature{
		Path:    "settings.show",
		Summary: "Show and describe project settings",
		Keywords: []string{
			"list settings", "describe settings", "list settings",
		},
		Syntax: "LIST SETTINGS;                              -- the sections, one row each\n" +
			"DESCRIBE SETTINGS;                          -- the settings as MDL\nDESCRIBE SETTINGS CONFIGURATION '<name>';",
		Example: "LIST SETTINGS;\nDESCRIBE SETTINGS;\nDESCRIBE SETTINGS CONFIGURATION 'Default';",
	})

	Register(SyntaxFeature{
		Path:    "settings.alter",
		Summary: "Modify project settings — model, configuration, constants, language, workflows",
		Keywords: []string{
			"alter settings", "modify settings", "change settings",
			"after startup", "before shutdown", "hash algorithm",
			"database type", "constant override", "language",
			"add language", "remove language", "enable language", "translations",
			"optimistic locking", "concurrency", "lost update",
			"workflow group", "workflow groups", "add group", "task assignment",
		},
		Syntax: `ALTER SETTINGS RUNTIME (<key>: <value>, ...);
ALTER SETTINGS CONFIGURATION '<name>' (<key>: <value>, ...);
ALTER SETTINGS CONSTANT @<qualifiedName> VALUE '<value>' IN CONFIGURATION '<name>';
ALTER SETTINGS DROP CONSTANT @<qualifiedName> IN CONFIGURATION '<name>';
ALTER SETTINGS LANGUAGE (DefaultLanguageCode: '<code>');
ALTER SETTINGS LANGUAGE ADD '<code>' [(CheckCompleteness: true, CustomDateFormat: '<fmt>')];
ALTER SETTINGS LANGUAGE ADD OR MODIFY '<code>' [(...)];
ALTER SETTINGS LANGUAGE MODIFY '<code>' (CheckCompleteness: true, ...);
ALTER SETTINGS LANGUAGE DROP '<code>';
ALTER SETTINGS WORKFLOWS (UserEntity: '<qualifiedName>');
ALTER SETTINGS WORKFLOWS ADD [OR MODIFY] GROUP '<name>' [(Description: '<text>')];
ALTER SETTINGS WORKFLOWS MODIFY GROUP '<name>' (Description: '<text>');
ALTER SETTINGS WORKFLOWS DROP GROUP '<name>';
CREATE [OR MODIFY] CONFIGURATION '<name>' [(<key>: <value>, ...)];
DROP CONFIGURATION '<name>';

-- A property is Key: value in a ( … ) list, as everywhere else in MDL (R3).
-- Key = value, … without the parentheses still runs and warns MDL-DEPR060.`,
		Example: `mdl 1;
ALTER SETTINGS RUNTIME (AfterStartupMicroflow: 'Module.MF_Startup');
ALTER SETTINGS RUNTIME (HashAlgorithm: 'BCrypt', EnableDataStorageOptimisticLocking: true);
ALTER SETTINGS CONFIGURATION 'Default' (
  DatabaseType: 'PostgreSql',
  DatabaseUrl: 'localhost:5432',
  DatabaseName: 'mydb'
);
ALTER SETTINGS CONSTANT @BusinessEvents.ServerUrl VALUE 'kafka:9092'
  IN CONFIGURATION 'Default';
CREATE CONFIGURATION 'Production' (
  DatabaseType: 'PostgreSql',
  HttpPortNumber: 8080
);

-- LANGUAGE ADD/REMOVE change the ENABLED languages — the list under App
-- Settings > Languages, and the only languages a build emits anything for. A
-- translation written for a language that is not enabled is stored, passes every
-- check, and is discarded at build time.
ALTER SETTINGS LANGUAGE ADD 'de_DE';
ALTER SETTINGS LANGUAGE ADD 'ar_SD' (CheckCompleteness: true);
ALTER SETTINGS LANGUAGE MODIFY 'ar_SD' (CustomDateFormat: 'yyyy-MM-dd');
ALTER SETTINGS LANGUAGE DROP 'de_DE';

-- ADD OR MODIFY is the upsert, and what DESCRIBE emits: it enables a language
-- that is not there and changes one that is, so a described project replays onto
-- itself and onto a project that already has some of its languages.

-- CheckCompleteness turns on error reporting for texts with no translation in
-- that language — without it they fall back to the default silently. MODIFY
-- changes only the options it names. The DEFAULT language is always checked by
-- Mendix whatever the flag says.

-- SET THE DEFAULT LANGUAGE BEFORE AUTHORING CONTENT. DefaultLanguageCode is not
-- only the fallback — it is the language a new caption is STORED under, because
-- Mendix has no language-neutral text. Creating a page and THEN switching the
-- default leaves that page's texts in the old language. Most captions then show
-- in Studio Pro as the empty-caption placeholder with a "no translation for this
-- language" warning, and build anyway; a TAB PAGE caption does not — mxbuild
-- refuses it with CE4899 "Empty caption". Changing the default prints how many
-- required captions lack it, check -p reports them for a script that changes it
-- (MDL-I18N01), and mxcli lint lists them (QUAL006). Recovery is to re-run the
-- create statements (the texts are then written under the new default) or
-- ALTER PAGE P { SET (Caption: '…') ON tabPage1; };. CREATE TRANSLATIONS FOR the
-- default is refused — it is the source language, not a translation target.

-- A language is identified by its CODE alone: Studio Pro's "Arabic, Sudan" is
-- derived from ar_SD for display and is not stored in the model.
-- Two refusals: the DEFAULT language cannot be removed (every missing
-- translation falls back on it — change DefaultLanguageCode first), and a
-- language that still carries translations is refused with the count, because
-- removing it would strip work the statement does not name. Say it on purpose
-- with: create or replace translations for <code> ( );

-- WORKFLOW GROUPS are the named buckets under App Settings > Workflows > Groups
-- that a user task's group targeting selects from. Mendix 11.2+ (the metamodel
-- floor for Settings$WorkflowGroup — the release notes' "GA in 11.6" is a
-- different question from whether the document loads).
ALTER SETTINGS WORKFLOWS ADD GROUP 'Approvers' (Description: 'Primary approval group');
ALTER SETTINGS WORKFLOWS ADD GROUP 'Reviewers';
ALTER SETTINGS WORKFLOWS MODIFY GROUP 'Reviewers' (Description: 'Second-line review');
ALTER SETTINGS WORKFLOWS DROP GROUP 'Reviewers';
LIST WORKFLOW GROUPS;

-- Description is the ONLY option: a Settings$WorkflowGroup stores Name and
-- Description and nothing else, so there is no identifier to set and the NAME is
-- the group's identity — which is what MODIFY and REMOVE address, and why adding
-- a second group differing only in case is refused. ADD OR MODIFY is the upsert
-- and what DESCRIBE SETTINGS emits.
--
-- Nothing in the model references a group: a user task targets groups through a
-- microflow or an XPath returning System.WorkflowGroup objects. The coupling is
-- at RUNTIME, where Mendix materialises one System.WorkflowGroup row per entry,
-- keyed on the group's element id — so MODIFY edits the row in place and REMOVE
-- stops it being maintained, while user tasks already assigned to it keep their
-- association.

-- DatabaseType must be a Mendix database type:
--   Db2, Hsqldb, MySql, Oracle, PostgreSql, SapHana, SqlServer
-- (matched case-insensitively and stored in the spelling above).

-- MODEL accepts these keys (an unknown one is refused and lists them):
--   AfterStartupMicroflow, BeforeShutdownMicroflow, HealthCheckMicroflow,
--   HashAlgorithm, BcryptCost, JavaVersion, RoundingMode,
--   AllowUserMultipleSessions, ScheduledEventTimeZoneCode, DefaultTimeZoneCode,
--   FirstDayOfWeek, DecimalScale, EnableDataStorageOptimisticLocking,
--   UseDatabaseForeignKeyConstraints, UseOQLVersion2, SslCertificateAlgorithm
--
-- EnableDataStorageOptimisticLocking is Studio Pro's App Settings → Runtime →
-- "Optimistic locking": it makes a stale commit fail instead of silently
-- overwriting, which is the fix for a read-then-write race in a microflow.
-- FirstDayOfWeek is Default or Monday..Sunday; SslCertificateAlgorithm is
-- PKIX or SunX509. Both are matched case-insensitively.
--
-- Which of these a project stores depends on its Mendix version (a blank 9.24
-- project has 12, a blank 11.13 has 17). An ALTER naming one the project does
-- not store is refused rather than introducing it, and DESCRIBE SETTINGS emits
-- only the stored ones so its output always replays.`,
		SeeAlso: []string{"settings.show"},
	})

	// ── Queues ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "queue",
		Summary: "Task queues — bound concurrency for queued microflow calls",
		Keywords: []string{
			"queue", "queues", "task queue", "create task queue", "create queue", "drop task queue",
			"describe task queue", "list task queues", "parallelism", "cluster wide",
			"background", "async microflow",
		},
		Syntax: `CREATE [OR MODIFY] TASK QUEUE Module.Name [FOLDER 'path'] [( <property>: <value>, ... )];
LIST TASK QUEUES [IN <module>];
LIST TASK QUEUES [IN <module>];
DESCRIBE TASK QUEUE Module.Name;
DROP TASK QUEUE Module.Name;

Properties:
  Parallelism   how many tasks run at once. This is an EXPRESSION, not a
                number — Mendix stores it as a string. A bare integer is the
                common case; quote anything else. Defaults to 1.
  ClusterWide   true = the limit applies across the cluster, false (default)
                = per runtime instance.
  Documentation free text.

Bind a call to a queue with the IN QUEUE clause on CALL MICROFLOW or
CALL JAVA ACTION (see: mxcli syntax microflow.call):

  CALL MICROFLOW Ops.ACT_Process(Order = $Order) IN QUEUE Ops.OrderProcessing;

A rewrite that does NOT restate a stored binding is refused, because it would
drop it silently. A retry policy on a queued call has no MDL spelling and is
also refused rather than reset — change those in Studio Pro.`,
		Example: `mdl 1;
CREATE TASK QUEUE Ops.OrderProcessing (
  Parallelism: 3,
  ClusterWide: true
);

-- Defaults: parallelism 1, per-instance.
CREATE TASK QUEUE Ops.Mail;

-- An expression is legal wherever a number is.
CREATE OR MODIFY TASK QUEUE Ops.OrderProcessing (
  Parallelism: '$MyModule.Workers',
  ClusterWide: true
);

-- Bind a call to it. A queued Java action must return Nothing (CE7038).
CREATE OR MODIFY MICROFLOW Ops.ACT_Enqueue ()
BEGIN
  CALL MICROFLOW Ops.ACT_Process(Order = $Order) IN QUEUE Ops.OrderProcessing;
END;

LIST TASK QUEUES IN Ops;
DESCRIBE TASK QUEUE Ops.OrderProcessing;
DROP TASK QUEUE Ops.Mail;`,
	})

	// ── Regular expressions ─────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "regular-expression",
		Summary: "Regular expressions — named patterns shared by attribute validation rules",
		Keywords: []string{
			"regular expression", "regular expressions", "regex", "pattern", "validation",
			"create regular expression", "drop regular expression", "describe regular expression",
			"list regular expressions", "email regex", "match",
		},
		Syntax: `[/** <documentation> */]
CREATE [OR MODIFY] REGULAR EXPRESSION Module.Name [FOLDER 'path'] (
  Expression: '<pattern>',
  [ExportLevel: Hidden|API,]
);

-- ExportLevel: Public is the deprecated spelling of API (MDL-DEPR161).

-- Documentation is the doc comment; the Documentation: '<text>' property is
-- its deprecated alias (MDL-DEPR106).

LIST REGULAR EXPRESSIONS [IN <module>];
LIST REGULAR EXPRESSIONS [IN <module>];
DESCRIBE REGULAR EXPRESSION Module.Name;
DROP REGULAR EXPRESSION Module.Name;

A regular expression is a DOCUMENT, not a string on a rule: Mendix stores an
attribute validation rule's reference to it by qualified name, so one pattern is
shared by every attribute that validates against it.

Quoting: the pattern is a normal MDL string, so a single quote inside it is
doubled ('^it''s$'). Backslashes are NOT escape characters — write the regex
exactly as Mendix should see it.

Mendix validates with .NET's regex engine, which accepts constructs Go does not
(lookaround, backreferences). mxcli stores such a pattern unchanged and DESCRIBE
notes that it could not verify it — it does not call it invalid.

Bind a pattern to an attribute with CREATE VALIDATION RULE — see
'mxcli syntax validation-rule'.`,
		Example: `mdl 1;
/** A, not too restrictive, email address regular expression */
CREATE REGULAR EXPRESSION Val.EmailAddress (
  Expression: '\w+((-|\+|\.)\w+)*@\w+([\.-]?\w+)*(\.\w{2,})+'
);

CREATE REGULAR EXPRESSION Val.Identifier (
  Expression: '^[a-zA-Z_]+[a-zA-Z0-9_]*$'
);

-- .NET lookbehind: legal in Mendix, not verifiable by mxcli
CREATE REGULAR EXPRESSION Val.NoTrailingSlash ( Expression: '.*(?<!/)$' );

LIST REGULAR EXPRESSIONS IN Val;
DESCRIBE REGULAR EXPRESSION Val.EmailAddress;
DROP REGULAR EXPRESSION Val.Identifier;

-- Which entities validate against a shared pattern
LIST REFERENCES TO Val.EmailAddress;`,
	})

	// ── Validation rules ────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "validation-rule",
		Summary: "Validation rules — constrain an attribute with a pattern or a range",
		Keywords: []string{
			"validation rule", "validation rules", "validate", "constraint",
			"create validation rule", "drop validation rule", "regex rule", "range rule",
			"required", "unique", "not null", "feedback",
		},
		Syntax: `CREATE VALIDATION RULE FOR Module.Entity.Attribute
  REGEX Module.PatternName
  ERROR MESSAGE '<message>';

CREATE VALIDATION RULE FOR Module.Entity.Attribute
  RANGE FROM <literal> TO <literal>
  ERROR MESSAGE '<message>';

DROP VALIDATION RULE [IF EXISTS] FOR Module.Entity.Attribute [REGEX | RANGE];
  -- without a kind, drops both the regex and the range rule on the attribute

The bounds are inclusive and either may be omitted:
  RANGE FROM 1 TO 100   between 1 and 100
  RANGE FROM 1          1 or more
  RANGE TO 100          100 or less
Mendix has no strict < or >, so there is no exclusive form.

A validation rule is anonymous and lives on the ENTITY, keyed by the attribute
it constrains — so the statement names the attribute, not the rule. Re-running
it replaces the rule of the SAME type on that attribute and leaves the others
alone, so an attribute can carry a Required and a RegEx rule at once.

REGEX takes the qualified name of a REGULAR EXPRESSION document, never an inline
pattern — create the pattern first. A name that does not resolve is refused
here, because Mendix stores the reference by name and would otherwise report
CE0135 "No regular expression specified" at build time.

REQUIRED and UNIQUE rules are written as attribute constraints instead, on
CREATE ENTITY or ALTER ENTITY:
  ALTER ENTITY Shop.Product MODIFY ATTRIBUTE Email: string(200)
    NOT NULL ERROR MESSAGE 'Email is required';
  ALTER ENTITY Shop.Product MODIFY ATTRIBUTE Code: string(20)
    UNIQUE ERROR MESSAGE 'Code must be unique';`,
		Example: `mdl 1;
CREATE REGULAR EXPRESSION Shop.EmailPattern (
  Expression: '^[^@\s]+@[^@\s]+\.[^@\s]+$'
);

CREATE VALIDATION RULE FOR Shop.Customer.Email
  REGEX Shop.EmailPattern
  ERROR MESSAGE 'Enter a valid email address';

CREATE VALIDATION RULE FOR Shop.Booking.Guests
  RANGE FROM 1 TO 100
  ERROR MESSAGE 'Between 1 and 100 guests are allowed';

CREATE VALIDATION RULE FOR Shop.Product.Price
  RANGE FROM 0
  ERROR MESSAGE 'Price cannot be negative';`,
	})

	// ── Scheduled events ────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "scheduled-event",
		Summary: "Scheduled events — Mendix's cron: run a microflow on a repeating schedule",
		Keywords: []string{
			"scheduled event", "scheduled events", "schedule", "cron", "recurring",
			"create scheduled event", "drop scheduled event", "describe scheduled event",
			"repeat", "daily", "hourly", "weekly", "monthly", "yearly", "timer", "batch job",
		},
		Syntax: `CREATE [OR MODIFY] SCHEDULED EVENT Module.Name [FOLDER 'path'] ( <property>: <value>, ... );
LIST SCHEDULED EVENTS [IN <module>];
LIST SCHEDULED EVENTS [IN <module>];
DESCRIBE SCHEDULED EVENT Module.Name;
DROP SCHEDULED EVENT Module.Name;

Always required:
  Microflow     the microflow to run, as a qualified name
  Repeat        which schedule to use (below)

Each Repeat takes ONLY its own fields; anything else is refused:
  Minutely          Multiplier
  Hourly            Multiplier, MinuteOffset
  Daily             HourOfDay, MinuteOfHour
  Weekly            Weekdays, HourOfDay, MinuteOfHour
  MonthlyByDate     Multiplier, MonthOffset, DayOfMonth, HourOfDay, MinuteOfHour
  MonthlyByWeekday  Multiplier, MonthOffset, DaySelector, Weekday, HourOfDay, MinuteOfHour
  YearlyByDate      Month, DayOfMonth, HourOfDay, MinuteOfHour
  YearlyByWeekday   Month, DaySelector, Weekday, HourOfDay, MinuteOfHour

  Weekdays is a quoted list: 'Monday, Friday'. DaySelector is First, Second,
  Third, Fourth or Last. Weekday is Sunday..Saturday. Month and DayOfMonth are
  numbers (1-12, 1-31). MonthOffset picks which month of a multi-month cycle
  fires (0-based).

Optional on any repeat:
  Enabled       true or false (default false)
  OnOverlap     DelayNext (default) or SkipNext — what happens when a run is
                still going when the next one is due. This is a scheduled
                event's own concurrency control; it does not use a task queue.
  TimeZone      UTC (default) or Server
  StartDateTime an RFC 3339 timestamp; the event does not run before it
  Documentation free text`,
		Example: `mdl 1;
CREATE SCHEDULED EVENT Ops.NightlyCleanup (
  Microflow: Ops.SE_Cleanup,
  Repeat: Daily,
  HourOfDay: 4,
  MinuteOfHour: 0,
  TimeZone: Server,
  Enabled: true
);

CREATE SCHEDULED EVENT Ops.HourlyPing (
  Microflow: Ops.SE_Ping,
  Repeat: Hourly,
  Multiplier: 2,
  MinuteOffset: 23
);

CREATE SCHEDULED EVENT Ops.WeeklyReport (
  Microflow: Ops.SE_Report,
  Repeat: Weekly,
  Weekdays: 'Monday, Friday',
  HourOfDay: 9,
  MinuteOfHour: 30
);

CREATE SCHEDULED EVENT Ops.QuarterEnd (
  Microflow: Ops.SE_Close,
  Repeat: MonthlyByWeekday,
  Multiplier: 3,
  MonthOffset: 2,
  DaySelector: Last,
  Weekday: Friday,
  HourOfDay: 18
);

LIST SCHEDULED EVENTS IN Ops;
DESCRIBE SCHEDULED EVENT Ops.NightlyCleanup;
DROP SCHEDULED EVENT Ops.HourlyPing;`,
		SeeAlso: []string{"queue"},
	})

	// ── Structure ───────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "structure",
		Summary: "DESCRIBE STRUCTURE — compact project overview at configurable depth",
		Keywords: []string{
			"structure", "describe structure", "project overview",
			"repo map", "module summary", "depth",
		},
		Syntax: "DESCRIBE STRUCTURE [DEPTH 1|2|3] [IN <module>] [ALL];",
		Example: `-- Module counts only
DESCRIBE STRUCTURE DEPTH 1;

-- Elements with signatures (default)
DESCRIBE STRUCTURE;

-- Full types and parameter names
DESCRIBE STRUCTURE DEPTH 3;

-- Focus on one module
DESCRIBE STRUCTURE IN MyModule;

-- Include system modules
DESCRIBE STRUCTURE DEPTH 1 ALL;`,
	})

	// ── Move ────────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "move",
		Summary: "MOVE command — relocate documents between folders and modules",
		Keywords: []string{
			"move", "relocate", "folder", "cross-module move",
			"move page", "move microflow", "move entity",
			"move folder", "drop folder",
			"move import mapping", "move export mapping", "move json structure",
			"move task queue", "move workflow", "move menu", "move layout",
		},
		Syntax: `MOVE <doctype> Module.Name TO FOLDER 'Path';
-- doctype: every top-level document, spelled as DESCRIBE spells it —
--   PAGE | SNIPPET | BUILDING BLOCK | LAYOUT | MENU
--   MICROFLOW | NANOFLOW | WORKFLOW | QUEUE | SCHEDULED EVENT
--   ENUMERATION | CONSTANT | REGULAR EXPRESSION
--   JSON STRUCTURE | IMPORT MAPPING | EXPORT MAPPING
--   JAVA ACTION | JAVASCRIPT ACTION | DATABASE CONNECTION | DATA TRANSFORMER
--   IMAGE COLLECTION | ICON COLLECTION
--   REST CLIENT | PUBLISHED REST SERVICE | ODATA CLIENT | ODATA SERVICE
--   BUSINESS EVENT SERVICE
--   MODEL | AGENT | KNOWLEDGE BASE | CONSUMED MCP SERVICE
-- plus ENTITY (moves between domain models) and FOLDER (moves a folder).
MOVE <doctype> Module.Name TO TargetModule;
MOVE <doctype> OldModule.Name TO FOLDER 'Path' IN NewModule;
MOVE FOLDER Module.FolderName TO FOLDER 'Path';
DROP FOLDER 'Path' IN Module;`,
		Example: `mdl 1;
-- Move page to a folder
MOVE PAGE MyModule.CustomerEdit TO FOLDER 'Customers';

-- Move microflow to nested folder
MOVE MICROFLOW MyModule.ACT_ProcessOrder TO FOLDER 'Orders/Processing';

-- Move entity to different module
MOVE ENTITY OldModule.Customer TO NewModule;

-- Many doctypes have no folder clause on CREATE, so MOVE is the only way to
-- place them
MOVE JAVA ACTION MyModule.ODataQuery TO FOLDER 'Support';
MOVE PUBLISHED ODATA SERVICE MyModule.PublicApi TO FOLDER 'Api/Published';
MOVE IMPORT MAPPING MyModule.IMM_Order TO FOLDER 'Private/Import mappings';
MOVE JSON STRUCTURE MyModule.JSON_Order TO FOLDER 'Private/JSON structures';

-- A FOLDER clause on CREATE OR MODIFY moves an existing document too, so a
-- script can place a document without a separate MOVE. Every doctype accepts
-- one; on most it goes straight after the qualified name
CREATE OR MODIFY JSON STRUCTURE MyModule.JSON_Order
  FOLDER 'Private/JSON structures'
  SAMPLE '{"id": 1}';
CREATE TASK QUEUE MyModule.Q_Orders FOLDER 'Private/Queues' ( Parallelism: 3 );
CREATE IMPORT MAPPING MyModule.IMM_Order FOLDER 'Private/Import mappings'
  WITH JSON STRUCTURE MyModule.JSON_Order { CREATE MyModule.Order { Id = id } };

-- Check impact before cross-module move
LIST IMPACT OF OldModule.CustomerPage;
MOVE PAGE OldModule.CustomerPage TO NewModule;

-- Drop empty folder
DROP FOLDER 'OldFolder' IN Module;

-- Read the placement back
LIST FOLDERS IN MyModule;`,
		SeeAlso: []string{"folders", "rename"},
	})

	// ── Rename ──────────────────────────────────────────────────────────

	// RENAME parsed, ran and was in `mxcli help rename` but had no topic here,
	// so an agent consulting `syntax` concluded a microflow could not be
	// renamed and rebuilt it by hand (mendixlabs/mxcli#1318).
	// rename_topic_test.go holds the target list to the grammar.
	Register(SyntaxFeature{
		Path:    "rename",
		Summary: "RENAME — rename a document, entity, association or module and update every reference",
		Keywords: []string{
			"rename", "rename microflow", "rename nanoflow", "rename page",
			"rename entity", "rename enumeration", "rename association",
			"rename constant", "rename java action", "rename workflow",
			"rename module", "dry run", "refactor", "update references",
		},
		Syntax: `RENAME <target> Module.OldName TO NewName [DRY RUN];
-- target: ENTITY | MICROFLOW | NANOFLOW | PAGE | ENUMERATION | ASSOCIATION
--         | CONSTANT | JAVA ACTION | WORKFLOW
RENAME MODULE OldModule TO NewModule [DRY RUN];

-- The new name is BARE: the element stays in its module (use MOVE to change
-- module). An element of that name already in the module is an error.
--
-- Every reference is updated in the same statement: each stored string in the
-- project that IS the old qualified name, or starts with it plus '.', is
-- rewritten (Module.Old -> Module.New, Module.Old.Attr -> Module.New.Attr).
-- That covers calls, page and microflow parameters, show-page actions,
-- navigation, security, attribute types, association ends.
-- A name inside free text — a microflow expression, an XPath string — is not
-- such a string and is left as it was; build or 'mxcli docker check' reports
-- what remains.
--
-- RENAME JAVA ACTION also renames the .java source file and the class in it
-- (class, constructor, toString); the user and extra code are left as written.
-- RENAME MODULE rewrites every 'OldModule.' prefix project-wide.
--
-- DRY RUN changes nothing and lists each document that would change and how
-- many references it holds.
--
-- Members are renamed with ALTER, not RENAME:
--   ALTER ENTITY Module.E RENAME ATTRIBUTE Old TO New;
--   ALTER ENUMERATION Module.E RENAME VALUE Old TO New;
--
-- From the shell, the same statement for one element:
--   mxcli rename -p app.mpr <type> Module.OldName NewName [--dry-run]
--   type: entity | microflow | nanoflow | page | enumeration | association
--         | constant | java-action | workflow | module
--   (it also updates docs/brain/ anchors; see 'mxcli help rename')`,
		Example: `mdl 1;
-- See what would change first
RENAME MICROFLOW Shop.ACT_Old TO ACT_ProcessOrder DRY RUN;

RENAME MICROFLOW Shop.ACT_Old TO ACT_ProcessOrder;
RENAME NANOFLOW Shop.NF_Old TO NF_Validate;
RENAME PAGE Shop.OldPage TO Order_Edit;
RENAME ENTITY Shop.Customer TO Client;
RENAME ENUMERATION Shop.Status TO OrderStatus;
RENAME ASSOCIATION Shop.Order_Customer TO Order_Client;
RENAME CONSTANT Shop.ApiUrl TO ServiceUrl;
RENAME JAVA ACTION Shop.JA_Old TO JA_Hash;
RENAME WORKFLOW Shop.WF_Old TO WF_Approve;
RENAME MODULE Shop TO Store;`,
		SeeAlso: []string{"move", "domain-model.entity.alter", "domain-model.enumeration"},
	})

	// ── Folders ─────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "folders",
		Summary: "LIST FOLDERS — the folder layout of a module, with what is in each folder",
		Keywords: []string{
			"folders", "list folders", "list folders", "layout",
			"folder tree", "where is this document", "unfiled",
		},
		Syntax: "LIST FOLDERS [IN <module>];",
		Example: `-- Layout of one module
LIST FOLDERS IN MyModule;

-- Every module in the project
LIST FOLDERS;

-- As rows, to diff against an intended layout
mxcli -p app.mpr --json -c "LIST FOLDERS IN MyModule"

-- Complements MOVE: MOVE places a document in a folder, LIST FOLDERS reads
-- the placement back. DESCRIBE STRUCTURE is organised by document type at every
-- depth, so it never shows which folder a document sits in.
--
-- Empty folders are listed too (with [0]), and documents still at the module
-- root appear under "(module root)" — so the output is the whole layout and
-- can be diffed against an intended one.`,
		SeeAlso: []string{"move", "structure"},
	})

	// ── Search ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "search",
		Summary: "Full-text search across project strings and source definitions",
		Keywords: []string{
			"search", "full-text search", "find", "grep",
			"fts", "catalog strings", "catalog source",
		},
		Syntax: `SEARCH '<query>';

-- CLI
mxcli search -p app.mpr "<query>" [--format table|names|json] [-q]

-- Raw FTS queries
SELECT * FROM CATALOG.STRINGS WHERE strings MATCH '<query>';
SELECT * FROM CATALOG.SOURCE WHERE source MATCH '<query>';`,
		Example: `SEARCH 'validation';
SEARCH 'Customer';

-- CLI with piping
mxcli search -p app.mpr "validation" -q --format names

-- FTS5 operators
SEARCH 'word1 OR word2';
SEARCH '"exact phrase"';
SEARCH 'word*';`,
	})

	// ── Testing ────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "test",
		Summary: "Microflow testing — run .test.mdl or .test.md files against a Mendix project (local warm loop, or Docker)",
		Keywords: []string{
			"test", "testing", "microflow test", "nanoflow test",
			"test.mdl", "test.md", "junit", "docker",
			"@test", "@expect", "@throws", "@cleanup",
			"watch", "attach", "test endpoint", "warm",
		},
		Syntax: `mxcli test <file|dir> -p app.mpr [flags]

Flags:
  -l, --list          List tests without executing
  -j, --junit FILE    Write JUnit XML results
  -s, --skip-build    Skip the build (reuse existing deployment)
      --local         Run on mxcli's own runtime — no Docker daemon needed
  -w, --watch         With --local: keep the runtime warm and re-run on
                      every test or model change (Ctrl-C to stop)
      --attach        Run against an app already started with
                      'mxcli run --local --test-endpoint' — no boot at all
      --skip-app-startup
                      With --local, do not run the project's own
                      after-startup microflow (it runs by default)
      --legacy-runner With --local: use the old after-startup runner
      --require-assertions
                      Report a test that asserts nothing as an ERROR
  -v, --verbose       Show runtime log lines
  -t, --timeout DUR   Runtime startup timeout (default: 5m)

Annotations:
  @test <name>              Test name (required)
  @expect <condition>       A Mendix expression that must evaluate to true.
                            Any expression the engine accepts works:
                              $result = 'John Doe'
                              $product/Name != 'Widget'   (<> is accepted too)
                              length($result) = 81
                              find($result, '0') >= 0
                              substring($r, 0, 9) = substring($r, 9, 18)
                              find($r, '0') >= 0 and $count > 3
                            An assertion the runner cannot compile — unknown
                            function, wrong arity, or an expression that
                            yields a value rather than a condition — is an
                            ERROR against that test, never a pass. A failure
                            reports the observed value alongside the
                            expectation whenever the assertion pins its type.
  @throws 'message'         Expect error
  @verify <oql> <op> <lit>  Assert on the DATABASE after the microflow ran:
                              @verify select count(*) as n from Mod.Cell = 81
                              @verify select count(*) as n from Mod.Cell > 0
                            The query must return one row and one column, and
                            the test needs @cleanup none — rollback would undo
                            the writes before the query could see them. An
                            unevaluatable @verify is an ERROR, never a pass.
                            --local / --attach only.
  @cleanup rollback|none    What happens to the test's database writes.
                            rollback (the default) wraps the test in a
                            transaction and rolls it back, so nothing it
                            wrote survives — including when it throws.
                            none lets the writes commit. --local only:
                            the Docker path always commits. An unknown
                            value is a parse error, not a silent commit.

How --local runs tests: one microflow per test, invoked by name over a
token-guarded HTTP endpoint the app registers at boot. A test that throws
fails only itself, and results are returned rather than scraped from the log.
Docker still uses the older after-startup runner.

Boot also runs the project's own after-startup microflow, chained after the
endpoint registration, so tests see the app in the state it really boots into
and a suite behaves the same under --local and --attach.

Cost of a run:
  cold (--local)            ~30s   boots a runtime on its own ports + DB
  warm (--local --watch)    ~2s    runtime stays up between runs
  attached (--attach)       ~2s    no boot; uses the running app's database`,
		Example: `-- .test.mdl file format
/**
 * @test String concatenation
 * @expect $result = 'John Doe'
 * @expect length($result) = 8
 */
$result = CALL MICROFLOW MyModule.ConcatNames(
  FirstName = 'John', LastName = 'Doe'
);
/

-- Run tests
mxcli test tests/ -p app.mpr                      -- Docker
mxcli test tests/ -p app.mpr --local              -- no Docker daemon
mxcli test tests/ -p app.mpr --local --watch      -- warm loop, re-runs on change
mxcli test tests/ -p app.mpr --junit results.xml

-- Or attach to an app you already have running:
mxcli run  --local --test-endpoint -p app.mpr     -- terminal 1
mxcli test tests/ -p app.mpr --attach             -- terminal 2`,
	})

	// ── Errors ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "errors",
		Summary: "Common validation errors and how to fix them",
		Keywords: []string{
			"errors", "validation", "syntax error", "reference error",
			"reserved keyword", "module not found", "entity not found",
			"check", "troubleshooting",
		},
		Syntax: `mxcli check script.mdl                    -- Syntax + anti-pattern check
mxcli check script.mdl -p app.mpr --references  -- With reference validation`,
		Example: `-- Reserved keyword as identifier
-- Error:  mismatched input 'Title' expecting IDENTIFIER
-- Fix:    Use quoted identifiers: "Title"

-- Module not found
-- Error:  module not found: ModuleName
-- Fix:    CREATE MODULE ModuleName;

-- Missing module prefix on enumeration
-- Error:  enumeration reference 'X' is missing module prefix
-- Fix:    Use Enumeration(MyModule.Status)

-- Invalid association path in OQL (dot instead of slash)
-- Wrong:  WHERE l.Library.Loan_Member = m.ID
-- Right:  WHERE l/Library.Loan_Member = m.ID`,
		SeeAlso: []string{"errors.syntax", "errors.reference", "errors.execution"},
	})

	Register(SyntaxFeature{
		Path:    "errors.syntax",
		Summary: "Syntax errors — reserved keywords, invalid types, malformed enumerations",
		Keywords: []string{
			"syntax error", "reserved keyword", "invalid type",
			"malformed enumeration", "parse error", "mismatched input",
		},
		Syntax: "mxcli check script.mdl",
		Example: `-- Reserved keyword used as identifier
-- Error:  mismatched input 'Title' expecting IDENTIFIER
-- Fix:    Use quoted identifiers: "Title", "ComboBox"."Entity"
-- Alt:    Rename to avoid keyword: BookTitle, OrderStatus

-- Invalid data type
-- Error:  Unknown type parsed as enumeration reference
-- Fix:    Use correct type: DateTime (not DateAndTime)

-- Malformed enumeration
-- Error:  Invalid enumeration value: each value must have a name
-- Fix:    Use syntax: ValueName 'Caption'`,
	})

	Register(SyntaxFeature{
		Path:    "errors.reference",
		Summary: "Reference errors — missing modules, entities, enumerations",
		Keywords: []string{
			"reference error", "module not found", "entity not found",
			"enumeration not found", "missing module prefix",
		},
		Syntax: "mxcli check script.mdl -p app.mpr --references",
		Example: `-- Module not found
-- Error:  module not found: ModuleName
-- Fix:    CREATE MODULE ModuleName;

-- Enumeration not found
-- Error:  attribute 'X': enumeration not found: Module.EnumName
-- Fix:    Create the enumeration first, or check spelling

-- Missing module prefix on enumeration
-- Error:  enumeration reference 'X' is missing module prefix
-- Fix:    Use fully qualified name: Enumeration(MyModule.Status)`,
	})

	Register(SyntaxFeature{
		Path:    "module.jar-dependencies",
		Summary: "Manage Maven/JAR dependencies in a module's settings",
		Keywords: []string{
			"jar dependency", "maven", "jar dep", "module settings",
			"group", "artifact", "classpath", "exclusion",
		},
		Syntax: `LIST JAR DEPENDENCIES [IN <module>];
DESCRIBE JAR DEPENDENCY <module> '<group:artifact>';
ALTER MODULE <name>
  ADD JAR DEPENDENCY (
    group    = '<group>',
    artifact = '<artifact>',
    version  = '<version>',
    included = true|false,
  );
ALTER MODULE <name> SET JAR DEPENDENCY '<group:artifact>' VERSION '<version>';
ALTER MODULE <name> SET JAR DEPENDENCY '<group:artifact>' INCLUDED true|false;
ALTER MODULE <name> SET JAR DEPENDENCY '<group:artifact>' ADD EXCLUSION '<group:artifact>';
ALTER MODULE <name> SET JAR DEPENDENCY '<group:artifact>' DROP EXCLUSION '<group:artifact>';
ALTER MODULE <name> DROP JAR DEPENDENCY '<group:artifact>';`,
		Example: `mdl 1;
-- Add a new JAR dependency to a module
ALTER MODULE MyModule
  ADD JAR DEPENDENCY (
    group    = 'org.duckdb',
    artifact = 'duckdb_jdbc',
    version  = '1.1.3',
    included = true,
  );

-- Update the version
ALTER MODULE MyModule SET JAR DEPENDENCY 'org.duckdb:duckdb_jdbc' VERSION '1.2.0';

-- Exclude a transitive dependency
ALTER MODULE MyModule SET JAR DEPENDENCY 'org.duckdb:duckdb_jdbc' ADD EXCLUSION 'com.example:unwanted';

-- List all jar dependencies
LIST JAR DEPENDENCIES;
LIST JAR DEPENDENCIES IN MyModule;

-- Describe (outputs roundtrippable MDL)
DESCRIBE JAR DEPENDENCY MyModule 'org.duckdb:duckdb_jdbc';

-- Remove a dependency
ALTER MODULE MyModule DROP JAR DEPENDENCY 'org.duckdb:duckdb_jdbc';`,
	})

	Register(SyntaxFeature{
		Path:    "errors.execution",
		Summary: "Execution errors — entity exists, type mismatches, validation failures",
		Keywords: []string{
			"execution error", "entity already exists", "type mismatch",
			"boolean default", "view entity", "microflow validation",
			"CE0117", "CE0109", "MDL-LISTOP01",
		},
		Syntax: "mxcli check script.mdl -p app.mpr --references",
		Example: `-- Entity already exists
-- Error:  entity already exists: Module.Entity
-- Fix:    Use CREATE OR MODIFY ENTITY to update existing entities

-- Boolean without default
-- Note:   Boolean attributes auto-default to false

-- OQL invalid association path (dot vs slash)
-- Wrong:  WHERE l.Library.Loan_Member = m.ID
-- Right:  WHERE l/Library.Loan_Member = m.ID

-- FILTER/FIND predicate names something that is not a member
-- Error:  "Nonexistent" is not an attribute or association of Shop.Order
-- Cause:  a bare name in a predicate means a member of the item under test
-- Fix:    check it against DESCRIBE ENTITY, or write $Var for a variable

-- FILTER/FIND predicate uses an iterator Mendix does not define
-- Error:  filter($L, ...): '$item' is not defined ... [MDL-LISTOP01]
-- Fix:    use $currentObject/Attr, or a bare attribute name`,
	})
}
