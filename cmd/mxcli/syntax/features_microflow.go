// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	Register(SyntaxFeature{
		Path:    "microflow",
		Summary: "Programmatic logic — variables, object operations, control flow, and integrations",
		Keywords: []string{
			"microflow", "nanoflow", "logic", "automation",
			"action", "activity", "flow",
		},
		Syntax:  "CREATE [OR REPLACE | OR MODIFY] MICROFLOW Module.Name ($Param: Type) RETURNS Type AS $Result\nBEGIN\n  <statements>\nEND;",
		Example: "mdl 1;\nCREATE MICROFLOW MyModule.ACT_CreateOrder ($Code: String)\nRETURNS MyModule.Order AS $NewOrder\nBEGIN\n  $NewOrder = CREATE MyModule.Order (OrderNumber = $Code);\n  COMMIT $NewOrder;\n  RETURN $NewOrder;\nEND;\n\n-- Re-runnable: replaces the microflow if it already exists\nCREATE OR MODIFY MICROFLOW MyModule.ACT_CreateOrder ($Code: String)\nRETURNS MyModule.Order AS $NewOrder\nBEGIN\n  $NewOrder = CREATE MyModule.Order (OrderNumber = $Code);\n  RETURN $NewOrder;\nEND;",
		SeeAlso: []string{"microflow.create", "microflow.variables", "microflow.control-flow", "create-modifiers"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.synchronize",
		Summary: "SYNCHRONIZE — offline data synchronization (nanoflow only)",
		Keywords: []string{
			"synchronize", "sync", "offline", "unsynchronized", "specific",
			"offline first", "nanoflow",
		},
		Syntax:  "SYNCHRONIZE ALL [ON ERROR ...];\nSYNCHRONIZE UNSYNCHRONIZED [ON ERROR ...];   -- Mendix 9.4+\nSYNCHRONIZE $Var[, $Var...] [ON ERROR ...];\n\nNanoflow only: in a microflow this is MDL057 / CE0009.",
		Example: "CREATE NANOFLOW MyModule.NF_Sync ($Order: MyModule.Order)\nBEGIN\n  SYNCHRONIZE ALL;\n  SYNCHRONIZE UNSYNCHRONIZED;\n  SYNCHRONIZE $Order;\n  SYNCHRONIZE ALL ON ERROR WITHOUT ROLLBACK BEGIN\n    LOG ERROR 'sync failed';\n  END ERROR;\nEND;",
		SeeAlso: []string{"microflow.nanoflow", "microflow.error-handling"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.create",
		Summary: "Create a microflow with parameters, return type, and body",
		Keywords: []string{
			"create microflow", "new microflow", "define microflow",
			"parameters", "returns", "folder",
			"exposed as", "expose as microflow action", "expose as workflow action",
			"toolbox", "icon", "image",
		},
		Syntax: "CREATE MICROFLOW Module.Name ($P1: String, $P2: Integer)\n  RETURNS Type AS $Result\n  [FOLDER 'FolderPath']\n  [EXPOSED AS MICROFLOW ACTION 'Caption' IN 'Category'\n     [ICON 'icon.png'] [ICON DARK 'icon-dark.png']\n     [IMAGE 'image.png'] [IMAGE DARK 'image-dark.png']]\n  [EXPOSED AS WORKFLOW ACTION 'Caption' IN 'Category']\n  [NOT EXPOSED AS MICROFLOW|WORKFLOW ACTION]\nBEGIN\n  <statements>\nEND;\n\nEXPOSED AS puts the microflow in Studio Pro's toolbox, so whoever drags it in\ndoes not need to know it is a microflow. There are two toolboxes — the\nmicroflow editor's and the workflow editor's — so the clause names which.\nThe icon is a 64x64 PNG and the image a 256x192 PNG, read from disk relative\nto the .mdl file's own directory. An OMITTED clause preserves what is stored, so\nremoving an entry is NOT EXPOSED and clearing one bitmap is DROP ICON/IMAGE.\n\n" +
			"Three document properties have their own header clauses:\n\n" +
			"  URL 'item/{Key}'                 the deep link (Mendix 10.6+)\n" +
			"  URL SEARCH PARAMETERS ($Filter)  parameters passed as query arguments\n" +
			"  DROP URL                         remove the deep link and its search params\n" +
			"  EXPORT LEVEL API | HIDDEN        the module's public surface on export\n" +
			"  DISALLOW CONCURRENT EXECUTION ERROR MESSAGE 'text'\n" +
			"  DISALLOW CONCURRENT EXECUTION ERROR MICROFLOW Module.Name\n" +
			"  ALLOW CONCURRENT EXECUTION\n\n" +
			"An OMITTED clause PRESERVES what is stored — the same rule as EXPOSED AS and\n" +
			"@applyentityaccess — so a rewrite that only changes the body leaves all of\n" +
			"them alone. DROP URL / EXPORT LEVEL HIDDEN / ALLOW are the explicit forms.\n\n" +
			"Three platform rules, each checked before the write rather than at build:\n" +
			"  MDL-MF01  every {Name} must name a parameter of this microflow\n" +
			"  MDL-MF02  a PATH parameter may not also be a SEARCH parameter  (CE5612)\n" +
			"  MDL-MF03  DISALLOW needs an error message or microflow         (CE4899)\n" +
			"and with a project, a URL another microflow already owns         (CE0570).\n\n" +
			"`Mark as used` still has no clause and is carried, as all of these were\n" +
			"before they were authorable (mendixlabs/mxcli#1120).",
		Example: "CREATE MICROFLOW MyModule.ACT_CreateOrder (\n  $CustomerCode: String,\n  $Quantity: Integer\n)\nRETURNS MyModule.Order AS $NewOrder\nFOLDER 'Orders'\nEXPOSED AS MICROFLOW ACTION 'Create order' IN 'Orders'\n  ICON 'assets/order-64.png'\nBEGIN\n  $NewOrder = CREATE MyModule.Order (\n    OrderNumber = 'ORD-001',\n    Quantity = $Quantity\n  );\n  COMMIT $NewOrder;\n  RETURN $NewOrder;\nEND;",
		SeeAlso: []string{"microflow.nanoflow", "microflow.variables"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.variables",
		Summary: "Declare variables, assign values, and set object attributes",
		Keywords: []string{
			"declare", "variable", "set", "assign", "change",
			"attribute", "expression",
		},
		Syntax: "DECLARE $Var Type;                 -- a variable must be declared before it is assigned\n" +
			"DECLARE $Var Type = expression;\n" +
			"$Var = expression;                -- assign; SET is optional\n" +
			"$Var/Attribute = expression;\n" +
			"SET $Var = expression;            -- same statement, explicit form",
		Example: "DECLARE $Count Integer = 0;\nDECLARE $Name String;\nset $Count = $Count + 1;\nset $Name = 'Hello';\nSET $Order/Status = 'Pending';",
		SeeAlso: []string{"microflow.object-operations"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.retrieve",
		Summary: "Query the database with WHERE, SORT BY, LIMIT, and OFFSET",
		Keywords: []string{
			"retrieve", "query", "database", "where", "sort",
			"limit", "offset", "find", "fetch",
		},
		// Retrieve-by-association was missing here, so it read as unsupported
		// even though it works and the write-microflows skill documents it.
		Syntax: "-- From the database\nRETRIEVE $Var FROM Module.Entity\n  [WHERE condition]\n  [SORT BY attr ASC|DESC]\n  [FIRST | [LIMIT n] [OFFSET n]];\n\n-- Sort over an association: one `/` per hop, the last segment is the attribute\nRETRIEVE $Var FROM Module.Entity\n  SORT BY Module.Assoc/Module.Other.Attr ASC;\n\n-- Over an association, from an object you already have\nRETRIEVE $Var FROM $Object/Module.Association;",
		Example: "-- FIRST binds a single OBJECT (Mendix's \"First object\" range), not a\n" +
			"-- one-element list — hence the singular variable name here.\n" +
			"RETRIEVE $Customer FROM MyModule.Customer\n  WHERE Code = $CustomerCode\n  FIRST;\n\n" +
			"-- LIMIT/OFFSET is a bounded range, which is a list.\n" +
			"RETRIEVE $Orders FROM MyModule.Order\n  WHERE Status = 'Pending'\n  SORT BY CreateDate DESC\n  LIMIT 10 OFFSET 0;\n\n" +
			"-- Follow an association rather than querying the database\nRETRIEVE $Orders FROM $Customer/MyModule.Order_Customer;\nRETRIEVE $Customer FROM $Order/MyModule.Order_Customer;\n\n" +
			"-- Sort on an attribute of an associated entity. Name the association\n" +
			"-- when two of them reach the same entity.\n" +
			"RETRIEVE $Orders FROM MyModule.Order\n  SORT BY MyModule.Order_BillTo/MyModule.Address.City ASC;\n\n" +
			"-- Notes:\n" +
			"--   * FIRST is the one form that binds an object, in every language version.\n" +
			"--     HEAD() or COUNT() over it is CE0097 at build time and a LOOP CE0100;\n" +
			"--     mxcli reports both as MDL-RETRIEVE01 at check time.\n" +
			"--   * LIMIT 1 without OFFSET depends on the language version: under `mdl 1;`\n" +
			"--     it is a list of one, as in `import from mapping … limit 1`. Without the\n" +
			"--     header it keeps its old meaning, the object, and warns MDL-V1-LIMIT1 —\n" +
			"--     write FIRST for the object. LIMIT 1 OFFSET n is always a list.\n" +
			"--   * SORT BY may navigate associations. Name the hop when more than one\n" +
			"--     reaches the same entity — mxcli infers a single hop, but it cannot\n" +
			"--     tell Order_ShipTo from Order_BillTo, and the wrong one builds\n" +
			"--     cleanly and sorts by the wrong thing.",
		SeeAlso: []string{"microflow.object-operations", "xpath"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.control-flow",
		Summary: "IF/ELSIF/ELSE, LOOP, WHILE, BREAK, CONTINUE, RETURN",
		Keywords: []string{
			"if", "elsif", "else", "then", "end if",
			"loop", "while", "break", "continue", "return",
			"conditional", "branch", "iterate",
		},
		Syntax: "IF condition THEN\n  ...\nELSIF condition THEN\n  ...\nELSE\n  ...\nEND IF;\n\nLOOP $Item IN $List BEGIN ... END LOOP;\nWHILE condition BEGIN ... END WHILE;\nRETURN $Value;\nRETURN empty;\n\n" +
			"-- WHILE takes BEGIN and END WHILE like LOOP. Without the `mdl 1;` header both\n" +
			"-- may still be left out, with a warning (MDL-V1-WHILE); under `mdl 1;` that\n" +
			"-- is an error. `mxcli fmt --upgrade --header` inserts them.",
		Example: "IF $Customer = empty THEN\n  LOG ERROR NODE 'Svc' 'Not found';\n  RETURN empty;\nELSIF $Customer/Active = false THEN\n  LOG WARNING 'Inactive customer';\nELSE\n  CHANGE $Customer (LastAccess = [%CurrentDateTime%]);\nEND IF;\n\nLOOP $Item IN $OrderLines BEGIN\n  COMMIT $Item;\nEND LOOP;",
		SeeAlso: []string{"microflow.variables", "microflow.error-handling", "microflow.splits"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.splits",
		Summary: "CASE (enum split) and SPLIT TYPE (object type split)",
		Keywords: []string{
			"case", "when", "then", "end case", "enum split", "enumeration split",
			"split type", "end split", "type split", "inheritance split",
			"specialization", "cast", "empty", "switch", "decision",
		},
		Syntax: "CASE $EnumVarOrAttr                  -- branches on an ENUMERATION\n" +
			"  WHEN Value1, Value2 THEN\n" +
			"    ...\n" +
			"  WHEN (empty) THEN                  -- required (MDL056 / CE0079)\n" +
			"    ...\n" +
			"END CASE;                            -- no ELSE (MDL008)\n\n" +
			"SPLIT TYPE $ObjectVar                -- branches on the RUNTIME TYPE\n" +
			"  WHEN Module.Specialization THEN\n" +
			"    CAST $Specific;\n" +
			"    ...\n" +
			"  WHEN Module.BaseEntity THEN        -- required too (CE0090)\n" +
			"    ...\n" +
			"  WHEN (empty) THEN                  -- the NULL-object branch, not a default\n" +
			"    ...\n" +
			"END SPLIT;\n\n" +
			"Both take `WHEN ... THEN` branches. `SPLIT TYPE` needs one per subtype AND\n" +
			"the base entity; its `(empty)` branch is the null object and covers no type.\n" +
			"Legacy `CASE Module.Entity` / `ELSE` inside SPLIT TYPE still parse (MDL065).",
		Example: "CASE $Order/Status\n  WHEN Draft, Submitted THEN\n    LOG INFO 'Not shipped yet';\n  WHEN Approved THEN\n    LOG INFO 'Ready to ship';\n  WHEN (empty) THEN\n    LOG INFO 'No status';\nEND CASE;\n\nSPLIT TYPE $Animal\n  WHEN Zoo.Dog THEN\n    LOG INFO 'woof';\n  WHEN Zoo.Cat THEN\n    LOG INFO 'meow';\n  WHEN Zoo.Animal THEN\n    LOG INFO 'some other animal';\n  WHEN (empty) THEN\n    LOG INFO 'no animal at all';\nEND SPLIT;",
		SeeAlso: []string{"microflow.control-flow", "microflow.variables"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.error-handling",
		Summary: "Error handling with ON ERROR, RAISE ERROR, CONTINUE, ROLLBACK",
		Keywords: []string{
			"error", "error handling", "on error", "continue",
			"rollback", "throw", "exception", "try", "catch",
		},
		Syntax: "COMMIT $Obj ON ERROR CONTINUE;\nCOMMIT $Obj ON ERROR ROLLBACK;\n" +
			"COMMIT $Obj ON ERROR BEGIN <statements> END ERROR;\nCOMMIT $Obj ON ERROR WITHOUT ROLLBACK BEGIN <statements> END ERROR;\n\n" +
			"-- The handler is flow, so it is BEGIN ... END ERROR like IF, LOOP and WHILE.\n" +
			"-- The brace form `ON ERROR { ... }` still parses and warns MDL-DEPR540;\n" +
			"-- `mxcli fmt --upgrade` rewrites it.\n" +
			"--\n" +
			"-- The clause goes on the ACTIVITY that may fail. Most statements take it:\n" +
			"-- DECLARE, SET, CREATE, CHANGE, COMMIT, DELETE, RETRIEVE, every CALL,\n" +
			"-- LOG, SHOW PAGE, CLOSE PAGE, SHOW MESSAGE, VALIDATION FEEDBACK,\n" +
			"-- SYNCHRONIZE, DOWNLOAD FILE and the mapping/REST statements.\n" +
			"--\n" +
			"-- RAISE ERROR ends a handler with an error end event, re-raising the error\n" +
			"-- being handled. There is no THROW <expr>: Mendix has no action that raises\n" +
			"-- a new error carrying a value, and THROW is refused (it used to be dropped).\n" +
			"--\n" +
			"-- Two limits, both enforced rather than silently ignored:\n" +
			"--\n" +
			"--   ON ERROR CONTINUE is rejected by Mendix (CE6035) on CREATE, CHANGE,\n" +
			"--   COMMIT, LOG, SHOW PAGE, CLOSE PAGE, SHOW MESSAGE, VALIDATION\n" +
			"--   FEEDBACK and CALL WORKFLOW -> MDL076. A custom handler IS accepted on\n" +
			"--   all of them, and CONTINUE is fine on DECLARE, SET, RETRIEVE, DELETE\n" +
			"--   and CALL MICROFLOW.\n" +
			"--\n" +
			"--   List operations and aggregates ($x = head $l, $n = count $l) have\n" +
			"--   no error handling in Mendix at all -> MDL077.\n" +
			"--\n" +
			"-- IN A NANOFLOW a statement with no clause aborts the flow on error, and\n" +
			"-- there is no transaction to roll back. DECLARE, SET, RETRIEVE and DELETE\n" +
			"-- take every clause. CREATE, COMMIT, CALL NANOFLOW and CALL MICROFLOW take\n" +
			"-- only ON ERROR WITHOUT ROLLBACK BEGIN ... END ERROR. CHANGE, LOG, SHOW\n" +
			"-- PAGE, CLOSE PAGE, SHOW MESSAGE and VALIDATION FEEDBACK take none. Every\n" +
			"-- other form is CE6035 and is refused (MDL091).\n" +
			"--\n" +
			"-- A handler that does NOT end in RETURN/RAISE ERROR merges back into the main\n" +
			"-- flow, so a variable created after the merge is out of scope on the error\n" +
			"-- path (CE0108). End the handler, or expect that.\n" +
			"--\n" +
			"-- An EMPTY handler `BEGIN END ERROR` is not a no-op: it means \"on error, do whatever\n" +
			"-- the enclosing branch does next\". Say where the path goes with JOIN.\n" +
			"--\n" +
			"-- RAISE ERROR re-raises the error being handled, so it belongs INSIDE an\n" +
			"-- ON ERROR handler and nowhere else. On the main flow it is MDL084:\n" +
			"-- Mendix needs an error in scope to re-raise, Studio Pro will not draw\n" +
			"-- the shape, and mxbuild rejects it with CE0710 \"The main flow cannot\n" +
			"-- join an error flow or end in an error event.\". To fail deliberately\n" +
			"-- from the main flow, call a Java action that throws.",
		Example: "COMMIT $Order ON ERROR BEGIN\n  LOG ERROR 'Failed to save order';\n  RAISE ERROR;\nEND ERROR;\n\n" +
			"COMMIT $Batch ON ERROR WITHOUT ROLLBACK BEGIN\n  LOG WARNING 'Batch save failed, continuing';\nEND ERROR;\n\n" +
			"DECLARE $Name String = 'default' ON ERROR BEGIN\n  RETURN 'could not initialise';\nEND ERROR;",
		SeeAlso: []string{"microflow.control-flow"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.normalized-describe",
		Summary: "DESCRIBE MICROFLOW ... NORMALIZED: fold crossed branches into one condition",
		Keywords: []string{
			"normalized", "normalize", "describe", "fold", "recombinable",
			"irreducible", "crossed branches", "guard", "mode 3",
		},
		Syntax: "DESCRIBE MICROFLOW Module.Name NORMALIZED;\n\n" +
			"-- Opt-in rendering for a graph whose branches cross. Nested IF cannot\n" +
			"-- describe one faithfully, so the DEFAULT rendering flattens it and warns\n" +
			"-- (MDL-FLOW01). NORMALIZED instead folds the branch guards into a single\n" +
			"-- condition, which is equivalent and duplicates nothing.\n" +
			"--\n" +
			"-- The graph from mxcli #923:\n" +
			"--   split1: true -> split2      false -> merge1\n" +
			"--   split2: true -> merge1      false -> merge2\n" +
			"-- The activity on merge1 runs on `not(c1) or c2`. Default DESCRIBE renders\n" +
			"-- it as `c1 and c2` -- with the reporter's expressions, a program that\n" +
			"-- always logged described as one that never did.\n" +
			"--\n" +
			"-- WHY OPT-IN. The output re-executes to a DIFFERENT graph: same behaviour,\n" +
			"-- fewer nodes, different layout. Describing a microflow to change one\n" +
			"-- activity must not rebuild the canvas, so this is never the default and\n" +
			"-- the output carries a NOTE saying so.\n" +
			"--\n" +
			"-- WHAT IT REFUSES, rather than guessing:\n" +
			"--   * an activity between the decision and the shared part -- folding\n" +
			"--     would move a side effect;\n" +
			"--   * a rule-based decision -- a rule call cannot go inside an expression;\n" +
			"--   * genuinely interleaved branches -- nesting those needs a duplicated\n" +
			"--     activity or an invented boolean (Boehm-Jacopini), neither of which\n" +
			"--     is a description.\n" +
			"-- A refusal leaves that decision rendered as-is and says why.\n" +
			"--\n" +
			"-- Folding is safe because Mendix `and`/`or` SHORT-CIRCUIT -- measured, see\n" +
			"-- mdl-examples/bug-tests/923-short-circuit-semantics.test.mdl. Were they\n" +
			"-- eager, the folded form could evaluate a guard the original skipped.",
		Example: "DESCRIBE MICROFLOW MyModule.MF_Reporter NORMALIZED;\n\n" +
			"-- emits, for the crossed graph above:\n" +
			"--   if $B or not($A) then\n" +
			"--     log info node 'NODE' 'Do something';\n" +
			"--   end if;",
		SeeAlso: []string{"microflow.merge-join", "microflow.control-flow"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.describe-handles",
		Summary: "DESCRIBE MICROFLOW ... WITH HANDLES: print each activity's content address",
		Keywords: []string{
			"handles", "handle", "describe", "target", "address", "alter microflow",
			"ordinal", "content addressing", "wildcard",
		},
		Syntax: "DESCRIBE MICROFLOW Module.Name WITH HANDLES;\n\n" +
			"-- Prints '-- handle: <target>' above each activity: the address that\n" +
			"-- selects it in an ALTER MICROFLOW target (ADR-0012). Activities have no\n" +
			"-- names, so a target names one by content, in this order of preference:\n" +
			"--   $Var                       the activity whose output variable is $Var\n" +
			"--   'Caption'                  a split, or an activity with a custom caption\n" +
			"--   commit $Order              a statement pattern; * matches any run of\n" +
			"--   log * node 'Debug' *       tokens, and the pattern spans the WHOLE\n" +
			"--                              statement (end with * to match a prefix)\n" +
			"-- A target matching several activities is an error that lists each with\n" +
			"-- its ordinal (@1, @2, ... in describe order); it is never a guess.\n" +
			"-- The handles are comments, so the output still executes unchanged.\n" +
			"-- Cannot be combined with NORMALIZED, whose graph is not the stored one.",
		Example: "DESCRIBE MICROFLOW FeedbackModule.VAL_Feedback WITH HANDLES;\n\n" +
			"-- emits, among others:\n" +
			"--   -- handle: $IsValidEmail\n" +
			"--   $IsValidEmail = call java action FeedbackModule.ValidateEmail(...);\n" +
			"--   -- handle: 'Email is Valid?'\n" +
			"--   if not($IsValidEmail) then\n" +
			"--   -- handle: set $ValidFeedback = false @3\n" +
			"--   set $ValidFeedback = false;",
		SeeAlso: []string{"microflow.normalized-describe"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.merge-join",
		Summary: "Named join points: MERGE <label> and JOIN <label>",
		Keywords: []string{
			"merge", "join", "rejoin", "label", "goto", "converge",
			"exclusive merge", "irreducible", "crossed branches", "retry loop",
		},
		Syntax: "MERGE <label>;                 -- declare a join point\n" +
			"JOIN <label>;                  -- send this path to it\n\n" +
			"-- A Mendix ExclusiveMerge has no name, so the label is MDL-only: it is\n" +
			"-- resolved when the microflow is built and never stored in the model.\n" +
			"--\n" +
			"-- Forward and backward references both resolve, so declaration order is\n" +
			"-- free. A backward one is how a retry loop is written:\n" +
			"--   MERGE attempt;\n" +
			"--   $r = CALL MICROFLOW M.Post() ON ERROR WITHOUT ROLLBACK BEGIN JOIN attempt; END ERROR;\n" +
			"--\n" +
			"-- What this is FOR. Nested IF can only describe a graph whose branches\n" +
			"-- pair up. Two cases do not:\n" +
			"--   1. An ERROR path that rejoins the normal one somewhere other than the\n" +
			"--      enclosing branch's own continuation. Without JOIN the only\n" +
			"--      spellings are \"terminate\" and \"fall through\", and DESCRIBE used to\n" +
			"--      emit an empty handler for anything else — MDL that re-executes to a\n" +
			"--      DIFFERENT graph, with no warning.\n" +
			"--   2. Crossed branches: an inner split's branch landing where an outer\n" +
			"--      split's branch lands. No nesting of IF reproduces that.\n" +
			"--\n" +
			"-- DESCRIBE emits both forms itself, so these are words you will READ as\n" +
			"-- often as write. A crossed graph now describes faithfully by default --\n" +
			"-- every branch ends in JOIN and the shared part follows as MERGE sections\n" +
			"-- -- and the old MDL-FLOW01 'must not be re-executed' warning is gone with\n" +
			"-- it. See microflow.normalized-describe for the opt-in alternative that\n" +
			"-- folds the guards instead of naming the merges.\n" +
			"--\n" +
			"-- Rules, all reported by `mxcli check`:\n" +
			"--   MDL-FLOW02  JOIN with no MERGE of that label, or a MERGE nothing joins\n" +
			"--   MDL-FLOW03  the same label declared twice\n" +
			"--   MDL-FLOW04  MERGE / JOIN inside a LOOP or WHILE body. A Mendix loop\n" +
			"--               owns its own object collection and a sequence flow cannot\n" +
			"--               leave it, so there is no graph this could build.\n" +
			"--\n" +
			"-- A path that has already ended (RETURN, RAISE ERROR, JOIN) does NOT fall\n" +
			"-- through into a following MERGE — the merge starts a new path.",
		Example: "CREATE MICROFLOW M.Post (Payload: String) RETURNS String\n" +
			"BEGIN\n" +
			"  DECLARE $Status String = 'sent';\n" +
			"  $r = CALL MICROFLOW M.Send(Payload = $Payload) ON ERROR WITHOUT ROLLBACK BEGIN\n" +
			"    LOG WARNING NODE 'M' 'send failed, degrading';\n" +
			"    SET $Status = 'degraded';\n" +
			"    JOIN recovered;\n" +
			"  END ERROR;\n" +
			"  JOIN recovered;\n" +
			"  MERGE recovered;\n" +
			"  RETURN $Status;\n" +
			"END;",
		SeeAlso: []string{"microflow.error-handling", "microflow.control-flow"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.object-operations",
		Summary: "CREATE, CHANGE, COMMIT, ROLLBACK, and DELETE objects",
		Keywords: []string{
			"create object", "change object", "commit", "rollback",
			"delete", "save", "persist", "modify object",
			"with events", "refresh", "commit flag", "without events",
		},
		Syntax: "$Obj = CREATE Module.Entity (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];\n" +
			"CHANGE $Obj (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];\n" +
			"COMMIT $Obj [WITHOUT EVENTS] [REFRESH];\n" +
			"DELETE $Obj [REFRESH];\n" +
			"ROLLBACK $Obj [REFRESH];\n\n" +
			"-- An omitted modifier always means Mendix's own default, so a bare\n" +
			"-- statement is exactly what Studio Pro gives you for a fresh activity:\n" +
			"--\n" +
			"--   Activity   With events        Refresh in client\n" +
			"--   CREATE     (Commit: No)       No\n" +
			"--   CHANGE     (Commit: No)       No\n" +
			"--   COMMIT     Yes                No\n" +
			"--   DELETE     n/a                No\n" +
			"--   ROLLBACK   n/a                No\n" +
			"--\n" +
			"-- COMMIT is the one whose default is ON, so WITHOUT EVENTS is the form\n" +
			"-- that changes anything; WITH EVENTS parses and means the default. The\n" +
			"-- COMMIT modifier on CREATE/CHANGE is the activity's Commit setting\n" +
			"-- (omitted = No), not the standalone COMMIT $Obj activity.",
		Example: "$NewOrder = CREATE MyModule.Order (\n  OrderNumber = 'ORD-001',\n  Quantity = $Quantity,\n  CreateDate = [%CurrentDateTime%]\n) COMMIT;\n\nCHANGE $NewOrder (MyModule.Order_Customer = $Customer) COMMIT REFRESH;\nCHANGE $Draft (Status = 'Imported') COMMIT WITHOUT EVENTS;\n\nCOMMIT $NewOrder;                  -- runs the commit event handlers\nCOMMIT $Staging WITHOUT EVENTS;    -- bulk import: skip them deliberately\nCOMMIT $NewOrder REFRESH;          -- and repaint it on the open page\n\nDELETE $OldOrder REFRESH;\nROLLBACK $DraftOrder;",
		SeeAlso: []string{"microflow.retrieve", "microflow.variables"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.list-operations",
		Summary: "List manipulation — HEAD, TAIL, FIND, FILTER, SORT, UNION, RANGE, aggregates",
		Keywords: []string{
			"list", "head", "tail", "find", "filter", "sort",
			"union", "intersect", "subtract", "count", "sum",
			"average", "aggregate", "add to list", "remove from list",
			"clear list", "clear", "change list", "replace list", "CE7247",
			"create list", "append to association", "add to association",
			// RANGE was authorable but absent from this topic, so the paging
			// form could not be discovered from the CLI at all (issue #966).
			"range", "paging", "pagination", "offset", "limit", "amount", "page",
			"reduce", "all", "any", "fold",
			// A CE number in the cached `syntax --json` index leads an agent
			// debugging the build error back to the right topic (issue #1002).
			"$currentObject", "predicate", "CE0117", "CE0109", "MDL-LISTOP01",
			"contains", "equals", "by", "where", "MDL-DEPR003", "MDL-DEPR004", "MDL-V1-LIST",
		},
		Syntax: "$List = CREATE LIST OF Module.Entity;\n" +
			"-- Change list: Add, Remove, Clear, Replace (stored Set).\n" +
			"ADD $Item TO $List;\nREMOVE $Item FROM $List;\nCLEAR $List;\n" +
			"SET $List = $Other;   -- on a LIST variable: Change list Replace, not\n" +
			"                      -- Change variable (CE7247 on a list)\n\n" +
			"-- One statement per Studio Pro activity. The keyword is the operation's\n" +
			"-- name and the operand is a variable, as in the activity's dialog.\n" +
			"-- The target may also be a many-to-many association:\n" +
			"ADD $Item TO $Parent/Module.Parent_Child;    -- append to a n2n association\n" +
			"REMOVE $Item FROM $Parent/Module.Parent_Child; -- detach from it\n" +
			"-- Use this rather than CHANGE $Parent (Assoc = $Item), which ASSIGNS the\n" +
			"-- whole set: three CHANGEs in a row leave one member attached and two\n" +
			"-- orphans in the table.\n" +
			"-- List operation:\n" +
			"$Result = HEAD $List;\n$Result = TAIL $List;\n" +
			"$Result = FIND $List BY Member = value;          -- Find (attribute or association)\n" +
			"$Result = FIND $List WHERE expression;           -- Find by expression\n" +
			"$Result = FILTER $List BY Member = value;        -- Filter\n" +
			"$Result = FILTER $List WHERE expression;         -- Filter by expression\n" +
			"$Result = SORT $List BY Attr DESC, Attr2 ASC;\n" +
			"$Result = UNION $L1 WITH $L2;\n$Result = INTERSECT $L1 WITH $L2;\n" +
			"$Result = SUBTRACT $L2 FROM $L1;                 -- $L1 minus $L2\n" +
			"$Bool   = CONTAINS $Object IN $List;\n$Bool   = EQUALS $L1 AND $L2;\n" +
			"$Result = RANGE $List OFFSET offset LIMIT amount;\n\n" +
			"-- Aggregate list: BY an attribute, or OF an expression over $currentObject.\n" +
			"$Count = COUNT $List;\n$Sum = SUM $List BY Attr;\n$Sum = SUM $List OF expression;\n" +
			"$Avg = AVERAGE $List BY Attr;\n$Min = MINIMUM $List BY Attr;\n$Max = MAXIMUM $List BY Attr;\n" +
			"$AllMatch = ALL $List WHERE boolean-expression;\n$AnyMatch = ANY $List WHERE boolean-expression;\n\n" +
			"-- REDUCE folds the list into one value. $currentResult is the running\n" +
			"-- total; the initial value and the type are required and cannot be inferred.\n" +
			"$Folded = REDUCE $List FROM initial AS Type USING expression;\n\n" +
			"-- RANGE needs at least ONE bound:\n" +
			"--   RANGE $L OFFSET $Offset LIMIT $Amount   page: skip $Offset, take $Amount\n" +
			"--   RANGE $L LIMIT $Amount                   first $Amount\n" +
			"--   RANGE $L OFFSET $Offset                  skip $Offset, take the rest\n" +
			"-- RANGE with no bound is CE6520 at build time (mxcli check: MDL068).\n\n" +
			"BY names a member and the value it must have; anything else goes after\n" +
			"WHERE, which is evaluated once per item with the item bound to\n" +
			"$currentObject -- the same variable the aggregate expressions use:\n\n" +
			"  FILTER $Orders WHERE $currentObject/Amount > 0\n\n" +
			"A bare attribute name after WHERE means the same thing; mxcli resolves it\n" +
			"against the list's entity and writes $currentObject/Attr. Naming any\n" +
			"other variable there is MDL-LISTOP01.\n\n" +
			"Because the operand is a variable, activities cannot nest. Give each\n" +
			"its own statement:\n\n" +
			"  $Approved = FILTER $Orders WHERE $currentObject/Status = 'Approved';\n" +
			"  $Count    = COUNT $Approved;\n\n" +
			"The call forms ($x = FILTER($L, ...), $n = COUNT($L)) are deprecated\n" +
			"aliases (MDL-DEPR003, MDL-DEPR004). $x = FIND(...) and CONTAINS(...) are\n" +
			"also Mendix's string functions: under `mdl 1;` the call form is refused\n" +
			"and `SET $x = find($Text, 'a')` is always the string function\n" +
			"(MDL-V1-LIST). Without the header a nested call is refused as\n" +
			"MDL-LISTOP02.",
		Example: "$AllOrders = CREATE LIST OF MyModule.Order;\nADD $NewOrder TO $AllOrders;\n$First = HEAD $AllOrders;\n" +
			"CLEAR $AllOrders;\nSET $AllOrders = $Orders;   -- Change list: Replace\n\n" +
			"-- Filter by member, and by expression over $currentObject\n" +
			"$Pending = FILTER $AllOrders BY Status = MyModule.OrderStatus.Pending;\n" +
			"$Large = FILTER $AllOrders WHERE $currentObject/Amount > 1000;\n" +
			"$Match = FIND $AllOrders BY OrderNumber = $Number;\n\n" +
			"-- SORT takes attribute names, not an expression\n" +
			"$Sorted = SORT $Pending BY CreateDate DESC;\n" +
			"$Page = RANGE $Sorted OFFSET $Offset LIMIT $PageSize;\n" +
			"$Total = SUM $AllOrders BY Amount;\n" +
			"$AllPaid = ALL $AllOrders WHERE $currentObject/Paid;\n" +
			"$AnyLate = ANY $AllOrders WHERE $currentObject/DueDate < [%CurrentDateTime%];\n\n" +
			"-- One statement per activity\n" +
			"$Paid  = FILTER $AllOrders WHERE $currentObject/Paid;\n" +
			"$Count = COUNT $Paid;\n" +
			"$Discounted = REDUCE $AllOrders FROM 0 AS Decimal\n" +
			"  USING $currentResult + $currentObject/Amount * 0.9;",
		SeeAlso: []string{"microflow.retrieve"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.logging",
		Summary: "LOG statements with level, node, message templates",
		Keywords: []string{
			"log", "logging", "info", "warning", "error", "debug",
			"trace", "critical", "node", "message template",
		},
		Syntax:  "LOG [LEVEL] [NODE 'Name'] 'message';\nLOG [LEVEL] 'template {1}' WITH ({1} = $value);\n\n-- Levels: INFO, WARNING, ERROR, DEBUG, TRACE, CRITICAL\n-- Defaults: level INFO, node 'Application'. DESCRIBE leaves both out.\n-- A line break is written into the literal: under `mdl 1;` a template literal\n-- that spans lines is still the template text (without the header: MDL-V1-TEMPLATE).",
		Example: "LOG INFO NODE 'OrderService' 'Order created successfully';\nLOG WARNING 'Customer not found';\nLOG ERROR 'Failed to process {1}' WITH (\n  {1} = $OrderNumber\n);",
	})

	Register(SyntaxFeature{
		Path:    "microflow.alter",
		Summary: "Patch a stored microflow or nanoflow: insert, replace or drop activities in place",
		Keywords: []string{
			"alter microflow", "alter nanoflow", "insert after", "insert before",
			"replace", "drop activity", "patch microflow", "splice", "handle",
		},
		Syntax: "ALTER MICROFLOW|NANOFLOW Module.Name {\n" +
			"  INSERT AFTER|BEFORE <target> BEGIN <statements> END;\n" +
			"  REPLACE <target> WITH BEGIN <statements> END;\n" +
			"  DROP <target>;\n" +
			"};\n\n" +
			"-- <target> addresses one activity by content, as `describe microflow ... with handles` prints it:\n" +
			"--   $Var            the activity that outputs $Var\n" +
			"--   'Caption'       a decision or an activity with a custom caption\n" +
			"--   <statement>     a statement pattern; * matches any run of tokens\n" +
			"-- followed by @n when it matches more than one. Targets are resolved against the\n" +
			"-- stored flow before any operation runs; an ambiguous or unknown target is an error.\n" +
			"-- Only the new activities, the rewired flows and the objects moved to make room change;\n" +
			"-- every other element keeps its $ID, position and curve.\n" +
			"-- Refused: insert after a decision, insert before an activity several flows enter,\n" +
			"-- drop/replace of a decision or of an activity with an error handler, anything inside\n" +
			"-- a loop body, a fragment that returns, and a fragment variable that clashes with one\n" +
			"-- the flow has or reads one not declared on the path. Over --mcp only insert is supported.\n" +
			"-- A fragment is imperative flow, written as the body of `create microflow` is: BEGIN … END.\n" +
			"-- The brace fragment `{ <statements> }` is the deprecated spelling MDL-DEPR074.",
		Example: "alter microflow FeedbackModule.VAL_Feedback {\n" +
			"  insert after $IsValidEmail begin log info node 'Feedback' 'Email checked'; end;\n" +
			"  replace set $ValidFeedback = false @3 with begin\n" +
			"    set $ValidFeedback = false;\n" +
			"    log warning node 'Feedback' 'Email rejected';\n" +
			"  end;\n" +
			"  drop log debug node 'Feedback' *;\n" +
			"};",
		SeeAlso: []string{"microflow"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.show-page",
		Summary: "Open and close pages from microflows",
		Keywords: []string{
			"show page", "open page", "close page", "display page",
			"navigate", "page action",
		},
		Syntax:  "SHOW PAGE Module.Page;\nSHOW PAGE Module.Page (Param = $value);\nCLOSE PAGE;",
		Example: "SHOW PAGE MyModule.OrderDetail (Order = $NewOrder);\nCLOSE PAGE;",
		SeeAlso: []string{"page"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.call",
		Summary: "Call microflows and Java actions with parameters",
		Keywords: []string{
			"call microflow", "call java action", "invoke", "in queue", "queued call", "background execution",
			"sub-microflow", "java action", "parameter passing",
		},
		Syntax:  "$Result = CALL MICROFLOW Module.Name (Param = value) [IN QUEUE Module.Queue];\n$Result = CALL JAVA ACTION Module.Name (Param = value) [IN QUEUE Module.Queue];",
		Example: "$IsValid = CALL MICROFLOW MyModule.ValidateOrder (\n  Order = $NewOrder\n);\n\n$Token = CALL JAVA ACTION MyModule.GenerateToken (\n  UserId = $User/Id\n);\n\n-- Run the call on a task queue (background execution). The queue must\n-- exist; a queued CALL JAVA ACTION must return Nothing, or the build fails\n-- with CE7038.\nCALL MICROFLOW MyModule.ACT_Refresh () IN QUEUE MyModule.RefreshQueue;\nCALL JAVA ACTION MyModule.RefreshData (Url = $Url) IN QUEUE MyModule.RefreshQueue;",
		SeeAlso: []string{"java-action", "microflow.create", "queue"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.nanoflow",
		Summary: "CREATE NANOFLOW — client-side logic; microflow syntax minus the server-only half",
		Keywords: []string{
			"nanoflow", "create nanoflow", "client-side", "runs in the browser",
			"offline", "client logic", "disallowed in nanoflow", "nanoflow restrictions",
		},
		Syntax: "CREATE [OR REPLACE] NANOFLOW Module.Name ($Param: Type)\n" +
			"RETURNS Type AS $Result\nBEGIN\n  <statements>\nEND;\n\n" +
			"The body is microflow syntax — every topic under `microflow` applies —\n" +
			"MINUS what cannot run in the browser. `mxcli check` refuses each of these\n" +
			"before a build, nested inside IF/LOOP/WHILE and error-handler bodies too:\n\n" +
			"  RAISE ERROR                    ErrorEvent has no nanoflow equivalent\n" +
			"  CALL JAVA ACTION               server-side\n" +
			"  EXECUTE DATABASE QUERY         server-side\n" +
			"  CALL EXTERNAL ACTION           server-side\n" +
			"  CALL REST SERVICE / SEND REST REQUEST\n" +
			"  IMPORT FROM MAPPING / EXPORT TO MAPPING\n" +
			"  TRANSFORM JSON\n" +
			"  DOWNLOAD FILE\n" +
			"  SHOW HOME PAGE\n" +
			"  every WORKFLOW action (call, open, set task outcome, notify, lock, …)\n\n" +
			"A Binary RETURN type is not allowed either.\n\n" +
			"ON ERROR is not universal here. Six activities reject it — change, log,\n" +
			"show page, close page, show message, validation feedback — because Mendix\n" +
			"answers CE6035 \"Error handling type is not supported\"; a nanoflow activity\n" +
			"aborts the flow on error by default, so drop the clause. The other\n" +
			"activities (create, commit, retrieve, the calls, declare, set) take it.\n\n" +
			"SYNCHRONIZE is the mirror image: allowed ONLY in a nanoflow (MDL057 flags\n" +
			"it in a microflow) — see microflow.synchronize.\n\n" +
			"Security is the same shape as a microflow's:\n" +
			"  GRANT EXECUTE ON NANOFLOW Module.Name TO Module.Role;",
		Example: "mdl 1;\n" +
			"CREATE NANOFLOW MyModule.NF_ValidateInput ($Input: String)\nRETURNS Boolean AS $IsValid\nBEGIN\n  IF $Input = empty THEN\n    VALIDATION FEEDBACK $Input MESSAGE 'Required';\n    RETURN false;\n  END IF;\n  RETURN true;\nEND;\n\n" +
			"-- Server-side work belongs behind a microflow call, which IS allowed\n" +
			"CREATE NANOFLOW MyModule.NF_Submit ($Order: Sales.Order)\nBEGIN\n" +
			"  CALL MICROFLOW MyModule.ACT_SubmitOrder (Order = $Order);\n" +
			"  CLOSE PAGE;\nEND;",
		SeeAlso: []string{"microflow.create", "microflow.synchronize", "microflow.error-handling"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.rule",
		Summary: "CREATE RULE — reusable decision logic, callable only from a decision",
		Keywords: []string{
			"rule", "create rule", "list rules", "describe rule", "drop rule",
			"business rule", "decision logic", "reusable condition",
		},
		Syntax: "CREATE [OR MODIFY] RULE Module.Name ($Param: Type)\n" +
			"RETURNS Boolean | enum Module.Enum\n[FOLDER 'path']\nBEGIN\n  <statements>\nEND;\n\n" +
			"LIST RULES [IN Module];\nDESCRIBE RULE Module.Name;\n" +
			"DROP RULE Module.Name;\nMOVE RULE Module.Name TO FOLDER 'path';\n\n" +
			"A rule is called from a decision and nowhere else:\n" +
			"  IF Module.Rule_Name(Param = $Value) THEN ... END IF;\n\n" +
			"The return type is mandatory and must be Boolean or an enumeration\n" +
			"(mxbuild: CE0103/CE0139). A rule may not create, change, delete, commit\n" +
			"or roll back objects, talk to the client, or call a web service\n" +
			"(mxbuild: CE0009) — `mxcli check` refuses these before the build.\n\n" +
			"There is no GRANT EXECUTE ON RULE: a rule is not independently callable,\n" +
			"so its document carries no module-role security.",
		Example: "mdl 1;\n" +
			"create or modify rule Sales.Rule_IsSolvent ($pCustomer: Sales.Customer)\n" +
			"returns Boolean\n" +
			"folder 'Rules'\n" +
			"begin\n" +
			"  return $pCustomer/Balance >= 0;\n" +
			"end;\n" +
			"\n" +
			"create or modify microflow Sales.MF_Screen ($pCustomer: Sales.Customer)\n" +
			"begin\n" +
			"  if Sales.Rule_IsSolvent(pCustomer = $pCustomer) then\n" +
			"    return;\n" +
			"  else\n" +
			"    return;\n" +
			"  end if;\n" +
			"end;\n",
		SeeAlso: []string{"microflow.create", "microflow.control-flow"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.validation",
		Summary: "Show validation feedback on object attributes",
		Keywords: []string{
			"validation", "feedback", "validation feedback",
			"error message", "field error", "form validation",
		},
		Syntax:  "VALIDATION FEEDBACK $Obj/Attr MESSAGE 'error text';\nVALIDATION FEEDBACK $Obj/Attr MESSAGE '{1} is invalid'\n  WITH ({1} = $Value);",
		Example: "VALIDATION FEEDBACK $Order/Quantity MESSAGE 'Quantity must be positive';\nVALIDATION FEEDBACK $Customer/Email MESSAGE '{1} is not valid'\n  WITH ({1} = $Customer/Email);",
		SeeAlso: []string{"microflow.error-handling"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.layout",
		Summary: "Canvas layout annotations — @position, @start, @anchor, @curve, @caption, @color",
		Keywords: []string{
			"position", "start", "anchor", "curve", "layout", "canvas",
			"annotation", "caption", "color", "excluded", "bezier",
		},
		Syntax: "@position(x, y)                       -- the activity's centre point\n" +
			"@position(x, y)                       -- also on a PARAMETER, in the ( … ) list\n" +
			"@start(x, y)                          -- the start event, on the FIRST statement\n" +
			"@anchor(from: right, to: left)        -- which SIDE each end of the outgoing flow attaches to\n" +
			"@curve(from: (40, -90), to: (-40, 90))  -- the flow's bezier control vectors\n" +
			"@merge(x, y)                          -- the implicit merge that closes a split\n" +
			"@anchor(from: bottom, to: top, true: (…), false: (…))  -- on an IF: to = its incoming flow,\n" +
			"                                      -- from = the flow leaving its closing merge\n" +
			"@anchor(true: (to: top))              -- one branch's edge; either side may be left out\n" +
			"@caption 'text'\n@color Green\n@annotation 'a note'\n@excluded\n" +
			"@applyentityaccess | @applyentityaccess(false)  -- DOCUMENT-level, before CREATE MICROFLOW/RULE\n" +
			"@annotation(id: n1, text: 'a note', position: (x, y), size: (w, h))\n" +
			"@annotation(id: n1)                   -- attaches THAT note to another activity\n\n" +
			"Inside ON ERROR ... BEGIN ... END ERROR the annotations mean what they mean\n" +
			"outside it: @anchor(to:) on the handler's first statement is the side the\n" +
			"error edge enters, @anchor(from:) and @curve shape the edge leaving a\n" +
			"statement — for the last one, the edge that rejoins the main flow.\n\n" +
			"@curve has no per-branch form: on a split it curves every outgoing edge, and\n" +
			"@curve(true: …) is refused (MDL060). An @anchor parameter that is not a\n" +
			"side mxcli knows is refused too (MDL092) rather than left on the default.\n\n" +
			"An unrecognised @name is an error (MDL059): it would parse and do nothing,\n" +
			"so a typo of @position would silently discard the layout. That covers\n" +
			"DOCUMENT annotations too — a typo, or one on a document kind that does not\n" +
			"read it (@applyentityaccess on a nanoflow, @excluded on a queue), is\n" +
			"refused with the list of what that document does accept.\n\n" +
			"@excluded and @applyentityaccess are DOCUMENT annotations — they go before\n" +
			"CREATE, not on a statement. @applyentityaccess runs the flow under the\n" +
			"current user's entity access rules instead of with full access; it is a\n" +
			"SECURITY setting and only ever narrows, so an ABSENT annotation PRESERVES\n" +
			"whatever is stored and @applyentityaccess(false) is how a script turns it\n" +
			"off. A nanoflow has no such property (it runs in the client).\n\n" +
			"Mendix stores no waypoints — a flow's shape is two control vectors, each a\n" +
			"pixel offset from its end of the line. (0, 0) at both ends is straight.\n" +
			"@position on a split belongs to the SPLIT, so its end-if join has its own\n" +
			"annotation. Container Size is still computed, not authorable.\n\n" +
			"WITHOUT @position the builder places everything. The main line runs left to\n" +
			"right and wraps onto a new row past 2880px; the line joining two rows leaves\n" +
			"the bottom of one and arrives on top of the next. A guard (if … return; end\n" +
			"if) drops its branch into the lane below and the main line carries straight\n" +
			"on over it. A CASE of four or more branches leaves the split in three groups\n" +
			"— top, right, bottom — so its lines do not cross. A statement that carries\n" +
			"@position is never moved, and starts the row for what follows it. Prefer no\n" +
			"@position at all to a few: hand-placed statements are not measured against\n" +
			"what the builder puts around them. (#1154)\n\n" +
			"@start and @merge position the two nodes that have no statement of their\n" +
			"own, so each is written on the statement it belongs to. Omit @start and the\n" +
			"start is placed one spacing unit left of the first activity, on its centre\n" +
			"line — and a rewrite MOVES it to follow the activities. A start that is not\n" +
			"at that derived spot was put there by hand: it survives a rewrite, and\n" +
			"DESCRIBE emits @start for it so the description round-trips exactly.\n\n" +
			"A PARAMETER is a stored node with its own coordinates, so it takes\n" +
			"@position too — written inside the parameter list, ahead of the parameter\n" +
			"it places. It is the only annotation a parameter takes. Omit it and the\n" +
			"parameters form a row along the top of the canvas at 200;53, 300;53, … ;\n" +
			"the same derived/authored rule as @start then applies, so a parameter on\n" +
			"that row is re-derived and one anywhere else survives a rewrite and is\n" +
			"emitted by DESCRIBE.\n\n" +
			"A NOTE is a node with edges, not a property of the activity it documents:\n" +
			"one note can be wired to several activities and several notes to one. So\n" +
			"@annotation is repeatable, and `id:` names a note so a later\n" +
			"@annotation(id: …) attaches the same one instead of creating a copy. The id\n" +
			"is scoped to the flow being authored and is not stored in the model —\n" +
			"DESCRIBE re-derives labels, and emits one only for a note that really is\n" +
			"shared. Two notes with identical text and no id stay two notes.\n\n" +
			"position:/size: are the note's own geometry, omitted whenever they match\n" +
			"what a rewrite re-derives (100px above the activity, stacked 60px per extra\n" +
			"note, 200x50), so an ordinary note keeps the short form. (#1077)",
		Example: "create microflow MyModule.ACT_Flow (\n  @position(145, 0)\n  $In: String\n)\nreturns String as $Out\nbegin\n" +
			"  @start(145, 100)\n  @position(200, 100)\n  @anchor(from: bottom, to: top)\n" +
			"  @curve(from: (40, -90), to: (-40, 90))\n  declare $Tmp String = $In;\n" +
			"  @position(200, 300)\n  declare $Out String = $Tmp;\n  return $Out;\nend;",
		SeeAlso: []string{"microflow", "microflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.mapping",
		Summary: "IMPORT FROM MAPPING / EXPORT TO MAPPING, and the import Range (All/First/Custom)",
		Keywords: []string{
			"import from mapping", "export to mapping", "import mapping activity",
			"range", "all", "first", "limit", "offset", "single object",
		},
		Syntax: "[$Var =] IMPORT FROM MAPPING Module.IMM ($SourceVar) [<range>];\n" +
			"$Var = EXPORT TO MAPPING Module.EMM ($EntityVar);\n\n" +
			"<range> — Studio Pro's Range on the import activity:\n" +
			"  ALL                          bind the whole result\n" +
			"  FIRST                        bind ONE object (not a one-element list)\n" +
			"  LIMIT <expr> [OFFSET <expr>] a bounded list\n\n" +
			"Omit it and the cardinality is inferred from the mapping's root shape.\n" +
			"The range does not change WHAT the mapping returns: an object-rooted\n" +
			"mapping binds an object under ALL too (Studio Pro's own default).\n" +
			"Mendix rejects OFFSET on a non-list mapping with CE6100, and FIRST on\n" +
			"an object-rooted mapping builds clean and throws at runtime — mxcli\n" +
			"check refuses both as MDL-MAP04. Drop the range on such a mapping.",
		Example: "$Pets  = import from mapping Shop.IMM_Pets($Json) all;\n" +
			"$Pet   = import from mapping Shop.IMM_Pets($Json) first;\n" +
			"$Page  = import from mapping Shop.IMM_Pets($Json) limit 10 offset 5;\n" +
			"$Json2 = export to mapping Shop.EMM_Pet($Pet);",
		SeeAlso: []string{"import-mapping", "export-mapping", "json-structure"},
	})
}
