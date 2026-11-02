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
		Syntax:  "CREATE [OR MODIFY] MICROFLOW Module.Name ($Param: Type) RETURNS Type AS $Result\nBEGIN\n  <statements>\nEND;",
		Example: "mdl 1;\nCREATE MICROFLOW MyModule.ACT_CreateOrder ($Code: String)\nRETURNS MyModule.Order AS $NewOrder\nBEGIN\n  $NewOrder = CREATE MyModule.Order (OrderNumber = $Code);\n  COMMIT $NewOrder;\n  RETURN $NewOrder;\nEND;\n\n-- Re-runnable: replaces the microflow if it already exists\nCREATE OR MODIFY MICROFLOW MyModule.ACT_CreateOrder ($Code: String)\nRETURNS MyModule.Order AS $NewOrder\nBEGIN\n  $NewOrder = CREATE MyModule.Order (OrderNumber = $Code);\n  RETURN $NewOrder;\nEND;",
		SeeAlso: []string{"microflow.create", "microflow.variables", "microflow.control-flow", "create-modifiers", "rename"},
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
		Path:    "microflow.send-email",
		Summary: "SEND EMAIL — the built-in Send Email activity (SMTP, Mendix 11.13+, beta)",
		Keywords: []string{
			"send email", "email", "mail", "smtp", "send mail", "e-mail",
			"cc", "bcc", "subject", "attachment", "email connector",
		},
		Syntax: "SEND EMAIL (\n" +
			"  From: <expr>,                       -- required\n" +
			"  To: <expr>, Cc: <expr>, Bcc: <expr>, -- at least one\n" +
			"  Subject: 'text {1}' [with ({1} = <expr>, ...)],\n" +
			"  Body: template 'text {1}' [with (...)],\n" +
			"  HtmlBody: template '<p>{1}</p>' [with (...)],\n" +
			"  Headers: ('X-Name': 'value', ...),\n" +
			"  Attachment: $FileDocumentOrList,\n" +
			"  Host: <expr>,                       -- required\n" +
			"  Port: <expr>,                       -- required\n" +
			"  SecurityType: none | ssl | tls,     -- default tls\n" +
			"  CheckServerIdentity: true | false,  -- ssl only\n" +
			"  ConnectionTimeout: <milliseconds>,  -- default 20000\n" +
			"  Authentication: basic (Username: <expr>, Password: <expr>),\n" +
			") [ON ERROR ...];\n\n" +
			"Microflow only, Mendix 11.13+ (Microflows$SendEmailAction). Sends mail over\n" +
			"SMTP without the Email Connector module. The dialog's settings are one\n" +
			"property list (ADR-0013); order does not matter, and an unknown, repeated or\n" +
			"misshapen key is an error. A missing From, Host, Port or recipient is\n" +
			"refused too: mxbuild reports each as CE0166.\n\n" +
			"From/To/Cc/Bcc, Host, Port and the credentials are expressions: String,\n" +
			"except Port, which is Integer/Long (mxbuild CE9528 otherwise; `check`\n" +
			"reports E009 first). Prefer constants for host, port and password.\n\n" +
			"Subject is a text template, like LOG's message; Body and HtmlBody take\n" +
			"template '...', like a REST body. Placeholders {1}, {2} ... are filled by\n" +
			"WITH; each parameter must be a String (CE0117) and every placeholder needs\n" +
			"one (CE0720). Any other expression is stored as '{1}' with the expression\n" +
			"as its parameter, so an expression followed by WITH is refused: it has no\n" +
			"{n} to fill (MDL-TEMPLATE01).\n\n" +
			"Headers are plain strings, not expressions: letters, digits and hyphens in\n" +
			"the name, one line in the value (MDL-EMAIL03). Attachment names a\n" +
			"System.FileDocument variable, a list of them, or a specialization.\n" +
			"CheckServerIdentity applies to SSL only (MDL-EMAIL02).\n\n" +
			"Studio Pro's Test Email tab is an editor tool and is not part of MDL: a\n" +
			"rewrite of the microflow stores it empty.",
		Example: "mdl 1;\n" +
			"create or modify microflow Shop.SUB_SendConfirmation ($Order: Shop.Order, $Invoice: Shop.Invoice)\n" +
			"returns Boolean as $Sent\n" +
			"begin\n" +
			"  send email (\n" +
			"    From: @Shop.MailSender,\n" +
			"    To: $Order/CustomerEmail,\n" +
			"    Subject: 'Order {1} confirmed' with ({1} = $Order/OrderNumber),\n" +
			"    Body: template 'Hi {1}, your order ships soon.' with ({1} = $Order/CustomerName),\n" +
			"    Headers: ('X-Correlation-Id': 'order-confirmation'),\n" +
			"    Attachment: $Invoice,\n" +
			"    Host: @Shop.SmtpHost,\n" +
			"    Port: @Shop.SmtpPort,\n" +
			"    Authentication: basic (Username: @Shop.SmtpUser, Password: @Shop.SmtpPassword),\n" +
			"  ) on error without rollback begin\n" +
			"    log error node 'Mail' 'Sending the confirmation failed';\n" +
			"    return false;\n" +
			"  end error;\n" +
			"  return true;\n" +
			"end;",
		SeeAlso: []string{"microflow.error-handling", "rest.call", "microflow.nanoflow"},
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
			"SET $Var = expression;            -- same statement, explicit form\n" +
			"-- $Var must be a primitive or a list (a list is a Change list Replace).\n" +
			"-- An OBJECT variable cannot be reassigned (CE7247, MDL-SET01): return the\n" +
			"-- new object from a sub-microflow, or `change $Obj (…)` its members.\n" +
			"-- A parameter cannot be reassigned either (CE7247, MDL-SET01) unless it is\n" +
			"-- a list: copy it first, `DECLARE $Value Integer = $N;`.\n" +
			"-- An enumeration variable: `DECLARE $S enum Module.Enum = empty;` (what\n" +
			"-- DESCRIBE prints). A bare `Module.Enum` reads as an object type (MDL043).\n" +
			"\n" +
			"-- Conditional create: an object variable cannot be DECLAREd (CE0053, MDL043)\n" +
			"-- and then created in a branch (CE0111, MDL063), and creating the same\n" +
			"-- name in both branches is CE0111 too — variable names are unique\n" +
			"-- flow-wide. Either create once with conditional member values, or let a\n" +
			"-- sub-microflow create and return the object on each branch (see example).",
		Example: "DECLARE $Count Integer = 0;\nDECLARE $Name String;\nset $Count = $Count + 1;\nset $Name = 'Hello';\nSET $Order/Status = 'Pending';\n\n" +
			"-- Conditional create, one activity: the condition goes into the values\n" +
			"$Line = CREATE MyModule.Line (Qty = if $Big then 10 else 1);\n\n" +
			"-- Conditional create, different shapes: a sub-microflow returns per branch\n" +
			"CREATE MICROFLOW MyModule.SUB_MakeLine ($Big: Boolean) RETURNS MyModule.Line AS $Line\n" +
			"BEGIN\n" +
			"  IF $Big THEN\n" +
			"    $BigLine = CREATE MyModule.Line (Qty = 10);\n" +
			"    RETURN $BigLine;\n" +
			"  ELSE\n" +
			"    $SmallLine = CREATE MyModule.Line (Qty = 1);\n" +
			"    RETURN $SmallLine;\n" +
			"  END IF;\n" +
			"END;\n" +
			"-- caller: $Line = CALL MICROFLOW MyModule.SUB_MakeLine(Big = $Big);",
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
			"--     mxcli reports both as MDL-RETRIEVE01 at check time. The reverse,\n" +
			"--     CHANGE over a list retrieve, is CE1613 and MDL-RETRIEVE03.\n" +
			"--   * Over an association, the result follows the association: from the\n" +
			"--     FROM entity of a Reference ($Order above) one object; from its TO\n" +
			"--     entity ($Customer) a LIST, unless the owner is Both; a ReferenceSet a\n" +
			"--     list either way. Using such a list as an object ($Orders/Attr,\n" +
			"--     CHANGE, an object parameter) is CE0117 / CE7247 — check --references\n" +
			"--     reports it as MDL-RETRIEVE03. For one object, retrieve it instead:\n" +
			"--     RETRIEVE $O FROM MyModule.Order WHERE [MyModule.Order_Customer = $Customer] FIRST;\n" +
			"--     A self-reference (MyModule.Order_Parent from Order to Order) is followed\n" +
			"--     from its FROM end: a Reference gives the one object, a ReferenceSet the\n" +
			"--     list. Using the one object as a list (an aggregate, a list operation, a\n" +
			"--     LOOP, a list parameter) is CE0013 / CE0097 / CE0100 / CE0117 —\n" +
			"--     check --references reports it as MDL-RETRIEVE01.\n" +
			"--   * $Obj/System.owner and $Obj/System.changedBy are not associations a\n" +
			"--     retrieve can follow (CE0136, MDL-RETRIEVE02). Use\n" +
			"--     RETRIEVE $U FROM System.User WHERE [id = $Obj/System.owner] FIRST;\n" +
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
			"case", "when", "then", "end case", "enum split", "enumeration split", "rule split",
			"split type", "end split", "type split", "inheritance split",
			"specialization", "cast", "empty", "switch", "decision",
		},
		Syntax: "CASE $EnumVarOrAttr                  -- branches on an ENUMERATION\n" +
			"  WHEN Value1, Value2 THEN\n" +
			"    ...\n" +
			"  WHEN (empty) THEN                  -- required (MDL056 / CE0079)\n" +
			"    ...\n" +
			"END CASE;                            -- no ELSE (MDL008)\n\n" +
			"CASE Module.Rule(Param = $Value)     -- a RULE returning an enumeration\n" +
			"  WHEN Value1 THEN ...               -- same call syntax as IF on a rule;\n" +
			"  WHEN (empty) THEN ...              -- with -p every value needs a branch\n" +
			"END CASE;\n\n" +
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
			"--   List operations, aggregates, CREATE LIST OF, ADD, REMOVE, CAST and\n" +
			"--   SHOW HOME PAGE take a custom handler but not CONTINUE -> MDL076.\n" +
			"--   ROLLBACK takes every form.\n" +
			"--\n" +
			"-- IN A NANOFLOW a statement with no clause aborts the flow on error, and\n" +
			"-- there is no transaction to roll back. DECLARE, SET, RETRIEVE and DELETE\n" +
			"-- take every clause, and so does ROLLBACK. CREATE, COMMIT, CALL NANOFLOW,\n" +
			"-- CALL MICROFLOW, list operations, aggregates, CREATE LIST OF, ADD, REMOVE\n" +
			"-- and CAST take only ON ERROR WITHOUT ROLLBACK BEGIN ... END ERROR. CHANGE, LOG, SHOW\n" +
			"-- PAGE, CLOSE PAGE, SHOW MESSAGE and VALIDATION FEEDBACK take none. Every\n" +
			"-- other form is CE6035 and is refused (MDL091).\n" +
			"--\n" +
			"-- A handler that does NOT end in RETURN/RAISE ERROR merges back into the main\n" +
			"-- flow at the statement after the failing activity — inside an IF, CASE or\n" +
			"-- loop body too, where a branch's last activity continues after END IF —\n" +
			"-- so a variable created after the merge is out of scope on the error\n" +
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
			"reference set", "referenceset", "many-to-many", "add to association",
			"remove from association", "CE0033", "+=", "-=",
		},
		Syntax: "$Obj = CREATE Module.Entity (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];\n" +
			"CHANGE $Obj (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];\n" +
			"ADD $X TO $Obj/Module.Assoc [COMMIT [WITHOUT EVENTS]] [REFRESH];      -- reference set\n" +
			"REMOVE $X FROM $Obj/Module.Assoc [COMMIT [WITHOUT EVENTS]] [REFRESH];\n" +
			"CHANGE $Obj (Attr = …, Module.Assoc += $X, Module.Assoc -= $Y);   -- the same, mixed\n" +
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
			"-- (omitted = No), not the standalone COMMIT $Obj activity.\n\n" +
			"-- On a reference set, CHANGE $Obj (Module.Assoc = $X) ASSIGNS the whole\n" +
			"-- set: what was in it is dropped. ADD/REMOVE with a $Obj/Module.Assoc\n" +
			"-- target is the Change object activity's Add / Remove member instead; $X\n" +
			"-- may be an object or a list. Reference sets only: on an attribute or a\n" +
			"-- plain Reference mxbuild rejects it (CE0033), and mxcli refuses it first.\n" +
			"-- Inside CHANGE, `+=` is that Add member and `-=` that Remove, so one\n" +
			"-- activity can mix them with ordinary `=` members.",
		Example: "$NewOrder = CREATE MyModule.Order (\n  OrderNumber = 'ORD-001',\n  Quantity = $Quantity,\n  CreateDate = [%CurrentDateTime%]\n) COMMIT;\n\nCHANGE $NewOrder (MyModule.Order_Customer = $Customer) COMMIT REFRESH;\nADD $Line TO $NewOrder/MyModule.Order_Lines;     -- append to a reference set\nCHANGE $Draft (Status = 'Imported') COMMIT WITHOUT EVENTS;\n\nCOMMIT $NewOrder;                  -- runs the commit event handlers\nCOMMIT $Staging WITHOUT EVENTS;    -- bulk import: skip them deliberately\nCOMMIT $NewOrder REFRESH;          -- and repaint it on the open page\n\nDELETE $OldOrder REFRESH;\nROLLBACK $DraftOrder;",
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
			"create list",
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
			"-- ($Obj/Module.Assoc as the target is a Change object Add/Remove on a\n" +
			"--  reference set: see microflow.object-operations.)\n" +
			"SET $List = $Other;   -- on a LIST variable: Change list Replace, not\n" +
			"                      -- Change variable (CE7247 on a list)\n\n" +
			"-- One statement per Studio Pro activity. The keyword is the operation's\n" +
			"-- name and the operand is always a variable, as in the activity's dialog.\n" +
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
			"stacktrace", "stack trace", "include latest stack trace",
		},
		Syntax:  "LOG [LEVEL] [NODE 'Name'] 'message';\nLOG [LEVEL] 'template {1}' WITH ({1} = $value);\nLOG [LEVEL] 'message' [WITH ({1} = $value)] WITH STACKTRACE;\n\n-- Levels: INFO, WARNING, ERROR, DEBUG, TRACE, CRITICAL\n-- Defaults: level INFO, node 'Application'. DESCRIBE leaves both out.\n-- WITH STACKTRACE ticks \"Include latest stack trace\"; absent is unticked, the\n-- default, and DESCRIBE prints it only when ticked. It follows the template\n-- parameters, which also open with WITH.\n-- A line break is written into the literal: under `mdl 1;` a template literal\n-- that spans lines is still the template text (without the header: MDL-V1-TEMPLATE).\n-- An expression message (LOG $Msg;) is stored as '{1}' with it as the parameter,\n-- so an expression followed by WITH (...) has no {n} to fill and is refused\n-- (MDL-TEMPLATE01) — as for SHOW MESSAGE, VALIDATION FEEDBACK and SEND EMAIL.\n-- In a template a `{` opens a numbered placeholder ({1}, {2}, ...) or is\n-- escaped as `{{`; a `}` needs no escape. 'Item {number}' fails the build with\n-- CE0712 (MDL-TEMPLATE02): write 'Item {1}' WITH ({1} = $Number).",
		Example: "LOG INFO NODE 'OrderService' 'Order created successfully';\nLOG WARNING 'Customer not found';\nLOG ERROR 'Failed to process {1}' WITH (\n  {1} = $OrderNumber\n);\nLOG ERROR NODE 'OrderService' 'Failed: {1}' WITH ({1} = $latestError/Message) WITH STACKTRACE;",
	})

	Register(SyntaxFeature{
		Path:    "microflow.expressions",
		Summary: "Expressions: string literals and backslashes, regular-expression functions, the *Between functions, template braces",
		Keywords: []string{
			"expression", "expressions", "string literal", "backslash", "escape", "escaping",
			"replaceAll", "replaceFirst", "isMatch", "regex", "regular expression",
			"secondsBetween", "minutesBetween", "hoursBetween", "daysBetween", "between",
			"absolute", "json escape", "brace", "braces", "placeholder", "template",
		},
		Syntax: "-- String literals: '...' with '' for an apostrophe ('it''s'). Under `mdl 1;` a\n" +
			"-- backslash is an ordinary character, as in a Mendix expression: the text is\n" +
			"-- stored exactly as written. Without the header (mdl 0) it escapes: '\\\\' is ONE\n" +
			"-- backslash and '\\n' a newline (MDL-V1-ESCAPE). Examples below assume `mdl 1;`.\n" +
			"\n" +
			"replaceAll(text, regex, replacement)    -- every match\n" +
			"replaceFirst(text, regex, replacement)  -- the first match\n" +
			"isMatch(text, regex)                    -- does the whole text match\n" +
			"\n" +
			"-- The regex is a regular expression (Java's in a microflow, JavaScript's in a\n" +
			"-- nanoflow): '\\\\' matches one backslash, '\\.' one dot. The replacement is\n" +
			"-- LITERAL text — a \\ or $ in it is inserted as written, never a group reference\n" +
			"-- (measured on the 10.24, 11.13 and 11.15 runtimes and the 11.13 client):\n" +
			"--   replaceAll('a\"b', '\"', '\\\"')   ->  a\\\"b\n" +
			"--   replaceAll('a\\b', '\\\\', '\\\\')  ->  a\\\\b\n" +
			"-- Escaping for JSON by hand: backslashes first, then quotes. A regex that ends\n" +
			"-- in a lone backslash ('\\', 'x\\') builds and fails every time it runs\n" +
			"-- (PatternSyntaxException \"Unescaped trailing backslash\"): check warns\n" +
			"-- MDL-REGEX01.\n" +
			"\n" +
			"millisecondsBetween(a, b)  secondsBetween(a, b)  minutesBetween(a, b)\n" +
			"hoursBetween(a, b)  daysBetween(a, b)  weeksBetween(a, b)\n" +
			"\n" +
			"-- Each returns the ABSOLUTE difference: secondsBetween($End, $Start) equals\n" +
			"-- secondsBetween($Start, $End). Compare the dates ($End > $Start) for the\n" +
			"-- direction. The result is a Decimal (an Integer target needs round/floor/ceil,\n" +
			"-- MDL041), and its fraction can run to dozens of digits: toString() of it can\n" +
			"-- be too long for a short String attribute — round it first.\n" +
			"\n" +
			"-- A template (log, show message, validation feedback, send email, a REST call's\n" +
			"-- URL and body, a Java action's text template, workflow texts, a dynamic text,\n" +
			"-- a button caption) is not an expression: a `{` in it opens a numbered\n" +
			"-- placeholder or is escaped as `{{` (CE0712, MDL-TEMPLATE02). In an expression's\n" +
			"-- string literal a brace is just a character.",
		Example: "mdl 1;\n" +
			"create microflow Mod.ToJsonString ($Text: String) returns String\n" +
			"begin\n" +
			"  -- backslashes first, then quotes; a brace in an expression is plain text\n" +
			"  declare $Escaped String = replaceAll($Text, '\\\\', '\\\\');\n" +
			"  set $Escaped = replaceAll($Escaped, '\"', '\\\"');\n" +
			"  return '{\"text\": \"' + $Escaped + '\"}';\n" +
			"end;\n" +
			"\n" +
			"create microflow Mod.ElapsedMinutes ($Start: DateTime, $End: DateTime) returns Integer\n" +
			"begin\n" +
			"  -- absolute, whichever date comes first; rounded for an Integer\n" +
			"  return round(minutesBetween($Start, $End));\n" +
			"end;",
		SeeAlso: []string{"microflow.logging", "microflow.variables", "regular-expression"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.alter",
		Summary: "Patch a stored microflow or nanoflow: insert, replace or drop activities in place, or replace a loop's body",
		Keywords: []string{
			"alter microflow", "alter nanoflow", "insert after", "insert before",
			"replace", "drop activity", "patch microflow", "splice", "handle",
			"replace body", "loop body", "replace body of",
			"existing note", "attach stored note",
		},
		Syntax: "ALTER MICROFLOW|NANOFLOW Module.Name {\n" +
			"  INSERT AFTER|BEFORE <target> BEGIN <statements> END;\n" +
			"  REPLACE <target> WITH BEGIN <statements> END;\n" +
			"  REPLACE BODY OF <loop> WITH BEGIN <statements> END;\n" +
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
			"-- REPLACE by one activity of the same kind (the same action: a filter's expression, a\n" +
			"-- retrieve's where, a member's value, a call's argument changed) edits the stored\n" +
			"-- activity: it keeps its $ID and those of its action and the parts that pair up — a\n" +
			"-- member by its attribute or association, an argument by its parameter (ako/mxcli#1761).\n" +
			"-- CREATE OR MODIFY reports it as `edited in place`; anything else replaced gets new\n" +
			"-- element IDs, reported as `replaced with new element IDs`.\n" +
			"-- Inside a loop body, plain activities are inserted, replaced and dropped the same way,\n" +
			"-- the loop box growing when the body no longer fits (ako/mxcli#1693).\n" +
			"-- REPLACE BODY OF <loop> keeps the loop itself (its $ID, position, iterator and list)\n" +
			"-- and rebuilds only what is inside it: the way to put a decision, a nested loop or a\n" +
			"-- return in a loop body. <loop> is the loop's handle: loop $It in $Items, while $N < 3.\n" +
			"-- DROP/REPLACE of a decision takes it out whole when it has the shape INSERT produces:\n" +
			"-- two paths of plain activities, or none, rejoining at one merge (ako/mxcli#1694).\n" +
			"-- Refused: insert after a decision, insert before an activity several flows enter,\n" +
			"-- drop/replace of any other decision or of an activity with an error handler, a decision, loop,\n" +
			"-- return or error handler put in a loop body (use REPLACE BODY OF), a fragment that\n" +
			"-- returns, and a fragment variable that clashes with one the flow has or reads one not\n" +
			"-- declared on the path. Over --mcp only insert is supported, and not inside a loop body.\n" +
			"-- A fragment is imperative flow, written as the body of `create microflow` is: BEGIN … END.\n" +
			"-- The brace fragment `{ <statements> }` is the deprecated spelling MDL-DEPR074.\n" +
			"-- A fragment statement joins a note ALREADY in the flow with\n" +
			"--   @annotation(existing: 'the note''s text')\n" +
			"-- named by its text exactly as describe prints it. The note keeps its $ID, text and\n" +
			"-- place; a text that no note reads, or that several notes read, is refused (the\n" +
			"-- refusal lists them). `existing:` stands alone and never creates a note — a note of\n" +
			"-- the statement's own is `@annotation 'text'`. The `id:` labels describe prints are\n" +
			"-- not stored, so a fragment cannot name a stored note by one (ako/mxcli#1695).",
		Example: "mdl 1;\nalter microflow FeedbackModule.VAL_Feedback {\n" +
			"  insert after $IsValidEmail begin log info node 'Feedback' 'Email checked'; end;\n" +
			"  replace set $ValidFeedback = false @3 with begin\n" +
			"    set $ValidFeedback = false;\n" +
			"    log warning node 'Feedback' 'Email rejected';\n" +
			"  end;\n" +
			"  drop log debug node 'Feedback' *;\n" +
			"};\n\n" +
			"alter microflow Shop.ACT_ProcessLines {\n" +
			"  replace body of loop $Line in $Lines with begin\n" +
			"    if $Line/Quantity > 0 then\n" +
			"      change $Line (Processed = true);\n" +
			"    end if;\n" +
			"    commit $Line;\n" +
			"  end;\n" +
			"};\n\n" +
			"-- The new statement joins the note the flow already has.\n" +
			"alter microflow Shop.ACT_ProcessOrder {\n" +
			"  insert after $Order begin\n" +
			"    @annotation(existing: 'Validates the order')\n" +
			"    log info node 'Shop' 'order validated';\n" +
			"  end;\n" +
			"};",
		SeeAlso: []string{"microflow"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.show-page",
		Summary: "Open and close pages from microflows",
		Keywords: []string{
			"show page", "open page", "close page", "display page",
			"navigate", "page action", "number of pages to close", "title override",
		},
		Syntax: "SHOW PAGE Module.Page;\nSHOW PAGE Module.Page (Param = $value) [WITH title = 'Title', NumberOfPagesToClose = n];\nCLOSE PAGE [n];",
		Example: "SHOW PAGE MyModule.OrderDetail (Order = $NewOrder);\n\n" +
			"-- Close the current page, then open the overview (Studio Pro: \"Number of\n" +
			"-- pages to close\"; an Integer expression, unstated closes none). A setting,\n" +
			"-- not an argument: (NumberOfPagesToClose: 1) is a page parameter (CE1613).\n" +
			"SHOW PAGE MyModule.OrderOverview WITH NumberOfPagesToClose = 1;\n" +
			"SHOW PAGE MyModule.OrderDetail (Order = $NewOrder) WITH title = 'Edit order';\n\n" +
			"CLOSE PAGE;\n\n-- Close this page and the one that opened it (n >= 1, default 1)\nCLOSE PAGE 2;",
		SeeAlso: []string{"page"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.call",
		Summary: "Call microflows and Java actions with parameters",
		Keywords: []string{
			"call microflow", "call java action", "invoke", "in queue", "queued call", "background execution",
			"sub-microflow", "java action", "parameter passing", "blank argument", "nothing",
			"text template parameter", "stringtemplate", "document parameter", "agent parameter",
		},
		Syntax: "$Result = CALL MICROFLOW Module.Name (Param = value) [IN QUEUE Module.Queue];\n$Result = CALL JAVA ACTION Module.Name (Param = value) [IN QUEUE Module.Queue];\n\n" +
			"-- A Java action parameter of type StringTemplate(Text|Sql) takes a template, filled\n" +
			"-- like a log message: Param = 'text {1}' WITH ({1} = expression, ...). A plain string\n" +
			"-- is the text; any other expression is the template '{1}' with it as the argument,\n" +
			"-- so an expression followed by WITH (...) is refused (MDL-JA02).\n" +
			"-- Each argument is typed from its expression (String, Integer, Decimal, Boolean,\n" +
			"-- DateTime); an enumeration is refused — pass getKey(...) or getCaption(...).\n" +
			"-- A document parameter (DESCRIBE JAVA ACTION shows Document(<type>), e.g. an\n" +
			"-- agent-editor Agent) takes the document's name: Param = Module.Document.",
		Example: "$IsValid = CALL MICROFLOW MyModule.ValidateOrder (\n  Order = $NewOrder\n);\n\n$Token = CALL JAVA ACTION MyModule.GenerateToken (\n  UserId = $User/Id\n);\n\n-- Run the call on a task queue (background execution). The queue must\n-- exist; a queued CALL JAVA ACTION must return Nothing, or the build fails\n-- with CE7038.\nCALL MICROFLOW MyModule.ACT_Refresh () IN QUEUE MyModule.RefreshQueue;\nCALL JAVA ACTION MyModule.RefreshData (Url = $Url) IN QUEUE MyModule.RefreshQueue;\n\n-- A blank argument: the mapping is kept, its expression left empty, as\n-- Studio Pro stores an argument field left blank. Not the same as `empty`\n-- (a Mendix value); only CALL MICROFLOW and CALL NANOFLOW take it. Mendix\n-- reports it as CE0127 \"Missing argument\": it exists so DESCRIBE round-trips.\nCALL MICROFLOW MyModule.ACT_Log (Message = 'x', TraceId = nothing);\n\n-- A text-template parameter, and a document parameter.\nCALL JAVA ACTION BusinessEvents.PublishBusinessEvent_V2 (\n  Subject = 'Order {1} shipped' WITH ({1} = $Order/OrderNumber),\n  BusinessEvent = $Event\n);\n$Response = CALL JAVA ACTION AgentEditorCommons.Agent_CallDocument_WithoutHistory (\n  Agent = MyModule.PaymentReminderAgent, OptionalContextObject = $Invoice\n);",
		SeeAlso: []string{"java-action", "microflow.create", "queue", "microflow.workflow-actions"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.workflow-actions",
		Summary: "Start and manage workflows from a microflow: call workflow, get workflow data, workflow operation, lock/unlock",
		Keywords: []string{
			"call workflow", "start workflow", "trigger workflow", "workflow from microflow",
			"get workflow data", "get workflows for", "get workflow activity records",
			"workflow operation", "abort workflow", "pause workflow", "unpause workflow",
			"retry workflow", "restart workflow", "continue workflow",
			"open workflow", "open user task", "lock workflow", "unlock workflow",
			"pause all", "unpause all", "workflow actions",
		},
		Syntax: "-- Microflow statements (not nanoflow ones). Each takes an optional ON ERROR clause.\n" +
			"$Wf = CALL WORKFLOW Module.Workflow ($ContextObj);      -- start an instance\n" +
			"$Data = GET WORKFLOW DATA $Wf AS Module.Workflow;       -- the instance's context object\n" +
			"$Wfs = GET WORKFLOWS FOR $ContextObj;                   -- instances running on an object\n" +
			"$Records = GET WORKFLOW ACTIVITY RECORDS $Wf;           -- the instance's activity history\n" +
			"WORKFLOW OPERATION ABORT $Wf [REASON <expression>];\n" +
			"WORKFLOW OPERATION PAUSE | UNPAUSE | RETRY | RESTART | CONTINUE $Wf;\n" +
			"OPEN WORKFLOW $Wf;                                      -- show the instance's admin page\n" +
			"OPEN USER TASK $Task;                                   -- show the task's page\n" +
			"LOCK WORKFLOW $WfDef | Module.Workflow [PAUSE ALL];\n" +
			"UNLOCK WORKFLOW $WfDef | Module.Workflow [UNPAUSE ALL];\n\n" +
			"-- CALL WORKFLOW: a workflow has exactly one context parameter, so the argument\n" +
			"-- is positional — the context object, not a `Param = value` list. That is the\n" +
			"-- form DESCRIBE emits; the variable is a System.Workflow.\n" +
			"-- $Wf is a System.Workflow, $Task a System.WorkflowUserTask, $WfDef a\n" +
			"-- System.WorkflowDefinition. A lock always names its workflow: a bare\n" +
			"-- `LOCK WORKFLOW ALL` is refused (MDL-WF17) — it built as CE1825. PAUSE ALL /\n" +
			"-- UNPAUSE ALL are Studio Pro's \"Pause / Unpause instances\".\n" +
			"-- Completing a user task and notifying a running workflow are their own\n" +
			"-- topics: workflow.set-task-outcome and workflow.notify.",
		Example: "mdl 1;\n" +
			"create microflow HR.ACT_StartApproval ($Request: HR.Request)\n" +
			"returns System.Workflow as $Workflow\n" +
			"begin\n" +
			"  $Workflow = call workflow HR.WF_Approval ($Request);\n" +
			"  return $Workflow;\n" +
			"end;\n\n" +
			"create microflow HR.ACT_Inspect ($Request: HR.Request)\n" +
			"begin\n" +
			"  $Workflows = get workflows for $Request;\n" +
			"  $Workflow = head $Workflows;\n" +
			"  $Data = get workflow data $Workflow as HR.WF_Approval;\n" +
			"  $Records = get workflow activity records $Workflow;\n" +
			"  open workflow $Workflow;\n" +
			"end;\n\n" +
			"create microflow HR.ACT_Withdraw ($Workflow: System.Workflow, $Task: System.WorkflowUserTask)\n" +
			"begin\n" +
			"  workflow operation abort $Workflow reason 'Request withdrawn';\n" +
			"  open user task $Task;\n" +
			"end;\n\n" +
			"-- Hold every instance of a definition during a migration, then release them\n" +
			"create microflow HR.ACT_Maintenance ()\n" +
			"begin\n" +
			"  lock workflow HR.WF_Approval pause all;\n" +
			"  unlock workflow HR.WF_Approval unpause all;\n" +
			"end;",
		SeeAlso: []string{"microflow.call", "workflow.call-workflow", "workflow.set-task-outcome", "workflow.notify", "workflow"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.nanoflow",
		Summary: "CREATE NANOFLOW — client-side logic; microflow syntax minus the server-only half",
		Keywords: []string{
			"nanoflow", "create nanoflow", "client-side", "runs in the browser",
			"offline", "client logic", "disallowed in nanoflow", "nanoflow restrictions",
		},
		Syntax: "CREATE [OR MODIFY] NANOFLOW Module.Name ($Param: Type)\n" +
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
			"  IF Module.Rule_Name(Param = $Value) THEN ... END IF;     -- Boolean rule\n" +
			"  CASE Module.Rule_Name(Param = $Value) WHEN V THEN ... END CASE;  -- enumeration rule\n\n" +
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
		Syntax:  "VALIDATION FEEDBACK $Obj/Attr MESSAGE 'error text';\nVALIDATION FEEDBACK $Obj/Attr MESSAGE '{1} is invalid'\n  WITH ({1} = $Value);\n\n-- The message is a template: an expression (MESSAGE $Text) is stored as '{1}'\n-- with it as the parameter, and one followed by WITH is refused (MDL-TEMPLATE01).",
		Example: "VALIDATION FEEDBACK $Order/Quantity MESSAGE 'Quantity must be positive';\nVALIDATION FEEDBACK $Customer/Email MESSAGE '{1} is not valid'\n  WITH ({1} = $Customer/Email);",
		SeeAlso: []string{"microflow.error-handling"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.layout",
		Summary: "Canvas layout annotations — @position, @start, @anchor, @curve, @caption, @color, @excluded",
		Keywords: []string{
			"position", "start", "anchor", "curve", "layout", "canvas",
			"annotation", "caption", "color", "excluded", "bezier",
			"disable activity", "disabled activity",
			"layout style", "layout-config", "mxcli-layout", "else column",
			"wrap", "row width",
		},
		Syntax: "@position(x, y)                       -- the activity's centre point\n" +
			"@position(x, y)                       -- also on a PARAMETER, in the ( … ) list\n" +
			"@start(x, y)                          -- the start event, on the FIRST statement\n" +
			"@anchor(from: bottom, to: top)        -- from = the side the flow LEAVING this statement\n" +
			"                                      -- leaves by; to = the side the flow ARRIVING at\n" +
			"                                      -- this statement enters by (not its far end)\n" +
			"@curve(from: (40, -90), to: (-40, 90))  -- the flow's bezier control vectors\n" +
			"@merge(x, y)                          -- the implicit merge that closes a split\n" +
			"@anchor(from: bottom, to: top, true: (…), false: (…))  -- on an IF: to = its incoming flow,\n" +
			"                                      -- from = the flow leaving its closing merge\n" +
			"@anchor(true: (to: top))              -- one branch's edge; either side may be left out\n" +
			"@anchor(Open: (from: right), (empty): (from: bottom))  -- on a CASE: one outcome's edge,\n" +
			"                                      -- keyed by its WHEN value; from = its merge's exit\n" +
			"@caption 'text'\n@color Green\n@annotation 'a note'\n@excluded\n" +
			"@applyentityaccess | @applyentityaccess(false)  -- DOCUMENT-level, before CREATE MICROFLOW/RULE\n" +
			"@annotation(id: n1, text: 'a note', position: (x, y), size: (w, h))\n" +
			"@annotation(id: n1)                   -- attaches THAT note to another activity\n" +
			"@annotation(existing: 'note text')    -- ALTER fragments only: attaches the stored note\n" +
			"                                      -- with that text (it must be the only one)\n\n" +
			"An @anchor never reaches the far end of its own outgoing flow: that end is\n" +
			"the NEXT statement's `to:`. A straight vertical edge from A down to B is\n" +
			"@anchor(from: bottom) on A and @anchor(to: top) on B; `to: top` on A alone\n" +
			"turns the edge INTO A, and the edge into B keeps entering on the left. A\n" +
			"RETURN has no outgoing flow, so its `from:` does nothing — the edge into the\n" +
			"end event leaves by the previous statement's `from:`. (#990)\n\n" +
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
			"@excluded on a STATEMENT disables that activity (Studio Pro's Disable\n" +
			"activity): it stays in the flow, configured, and is skipped at runtime.\n" +
			"Only an activity can be disabled — on a decision, loop, return or raise\n" +
			"error it is dropped, and check warns (MDL095).\n\n" +
			"@excluded before CREATE is the DOCUMENT annotation (Exclude from project).\n" +
			"@applyentityaccess is document-only — it goes before CREATE, never on a\n" +
			"statement — and runs the flow under the current user's entity access rules\n" +
			"instead of with full access; it is a SECURITY setting and only ever\n" +
			"narrows, so an ABSENT annotation PRESERVES whatever is stored and\n" +
			"@applyentityaccess(false) is how a script turns it off. A nanoflow has no\n" +
			"such property (it runs in the client).\n\n" +
			"Mendix stores no waypoints — a flow's shape is two control vectors, each a\n" +
			"pixel offset from its end of the line. (0, 0) at both ends is straight.\n" +
			"@position on a split belongs to the SPLIT, so its end-if join has its own\n" +
			"annotation. Container Size is still computed, not authorable.\n\n" +
			"WITHOUT @position the builder places everything. The main line runs left to\n" +
			"right and wraps onto a new row past 2880px; the line joining two rows leaves\n" +
			"the bottom of one and arrives on top of the next. A guard (if … return; end\n" +
			"if) drops its branch into the lane below and the main line carries straight\n" +
			"on over it; a custom error handler's row hangs in that lane too, 130px under\n" +
			"its activity. A statement that reaches into the lane — a decision, a loop, an\n" +
			"activity with a handler of its own — waits for it to clear; a plain activity\n" +
			"does not (ako/mxcli#1768). A CASE of four or more branches leaves the split\n" +
			"in three groups — top, right, bottom — so its lines do not cross. A\n" +
			"statement that carries @position is never moved, and starts the row for what\n" +
			"follows it; the statements BEFORE it keep the places the builder derives for\n" +
			"them, and only a @position on the first statement moves the start event.\n" +
			"Prefer no @position at all to a few: hand-placed statements are not measured\n" +
			"against what the builder puts around them. (#1154)\n\n" +
			"@start and @merge position the two nodes that have no statement of their\n" +
			"own, so each is written on the statement it belongs to. Omit @start and the\n" +
			"start is placed one spacing unit left of the first activity, on its centre\n" +
			"line — and a rewrite MOVES it to follow the activities. A start that is not\n" +
			"at that derived spot was put there by hand: it survives a rewrite, and\n" +
			"DESCRIBE emits @start for it so the description round-trips exactly.\n\n" +
			"A PARAMETER is a stored node with its own coordinates, so it takes\n" +
			"@position too — written inside the parameter list, ahead of the parameter\n" +
			"it places. It is the only annotation a parameter takes. Omit it and the\n" +
			"parameters form a row along the top of the canvas from 200;53, spaced 100px\n" +
			"apart or wider when a name needs it (about 7px a character, #1598);\n" +
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
			"An ALTER fragment is built alone, so it cannot see those labels; it names a\n" +
			"note already stored in the flow by its text: @annotation(existing: '…'). The\n" +
			"text must be the text of exactly one note of the flow (CRLF and LF line\n" +
			"breaks match), or the statement is refused, listing the candidates. The note\n" +
			"keeps its $ID, text and place, so `existing:` takes no id:, text:, position:\n" +
			"or size:, and a statement that states the whole flow refuses it. DESCRIBE\n" +
			"never prints it: a whole flow declares its notes. (ako/mxcli#1695)\n\n" +
			"position:/size: are the note's own geometry, omitted whenever they match\n" +
			"what a rewrite re-derives (100px above the activity, stacked 60px per extra\n" +
			"note, 200x50), so an ordinary note keeps the short form. (#1077)\n\n" +
			"LAYOUT STYLE — where mxcli puts what you leave unpositioned — is the\n" +
			"project's layout-config.yaml (.claude/ or next to the .mpr), never something\n" +
			"to imitate with @position. Leave positions out and the style applies:\n" +
			"  flows:\n" +
			"    preset: true-right-false-down   # happy-line (default)\n" +
			"    if-else: true-right-else-column # ELSE as a column under its decision\n" +
			"    case-split: first-right-rest-columns\n" +
			"    guard: true-right               # early return on the line\n" +
			"    wrap: off                       # or max-row-width: <px>\n" +
			"`mxcli layout config` shows the style in force. A flow drawn in a non-default\n" +
			"style carries a `@mxcli-layout: …` line in its /** … */ documentation: keep\n" +
			"it when copying a described flow, never write one by hand. (ako/mxcli#1832)",
		Example: "create microflow MyModule.ACT_Flow (\n  @position(145, 0)\n  $In: String\n)\nreturns String as $Out\nbegin\n" +
			"  @start(145, 100)\n  @position(200, 100)\n  @anchor(from: bottom)\n" +
			"  @curve(from: (40, -90), to: (-40, 90))\n  declare $Tmp String = $In;\n" +
			"  @position(200, 300)\n  @anchor(to: top)\n  declare $Out String = $Tmp;\n  return $Out;\nend;",
		SeeAlso: []string{"microflow", "microflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.mapping",
		Summary: "IMPORT FROM MAPPING / EXPORT TO MAPPING, and the import Range (All/First/Custom)",
		Keywords: []string{
			"import from mapping", "export to mapping", "import mapping activity",
			"range", "all", "first", "limit", "offset", "single object",
			"mapping parameter", "input object", "commit",
		},
		Syntax: "[$Var =] IMPORT FROM MAPPING Module.IMM ($SourceVar) [( <settings> )] [<range>];\n" +
			"$Var = EXPORT TO MAPPING Module.EMM ($EntityVar);\n\n" +
			"<settings> — the activity's dialog, any of:\n" +
			"  Parameter: $Obj              the mapping's input object, for a mapping\n" +
			"                               declared with `parameter Module.Entity`.\n" +
			"                               Required there (mxbuild CE0244 without it),\n" +
			"                               refused on any other mapping (CE0246)\n" +
			"  Commit: Yes | YesWithoutEvents | No\n" +
			"                               omitted = YesWithoutEvents\n\n" +
			"<range> — Studio Pro's Range on the import activity:\n" +
			"  ALL                          bind the whole result\n" +
			"  FIRST                        bind ONE object (not a one-element list)\n" +
			"  LIMIT <expr> [OFFSET <expr>] a bounded list\n\n" +
			"Omit it and the cardinality is inferred from the mapping's root shape.\n" +
			"The range does not change WHAT the mapping returns: an object-rooted\n" +
			"mapping binds an object under ALL too (Studio Pro's own default).\n" +
			"Mendix rejects OFFSET on a non-list mapping with CE6100, and FIRST on\n" +
			"an object-rooted mapping builds clean and throws at runtime — mxcli\n" +
			"check refuses both as MDL-MAP04, and mxcli lint reports an activity\n" +
			"already in the model that stores FIRST on such a mapping under the\n" +
			"same ID. Drop the range on such a mapping.",
		Example: "$Pets  = import from mapping Shop.IMM_Pets($Json) all;\n" +
			"$Pet   = import from mapping Shop.IMM_Pets($Json) first;\n" +
			"$Page  = import from mapping Shop.IMM_Pets($Json) limit 10 offset 5;\n" +
			"import from mapping Shop.IMM_Embed($Json) (Parameter: $Collection, Commit: Yes);\n" +
			"$Json2 = export to mapping Shop.EMM_Pet($Pet);",
		SeeAlso: []string{"import-mapping", "export-mapping", "json-structure"},
	})
}
