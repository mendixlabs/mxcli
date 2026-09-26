// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	// ── Integration overview ──────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "integration",
		Summary: "Unified discovery of all external services and integration assets",
		Keywords: []string{
			"integration", "services", "external", "contract",
			"odata", "rest", "business events", "database",
		},
		Syntax:  "SHOW ODATA CLIENTS [IN Module];\nSHOW REST CLIENTS [IN Module];\nSHOW PUBLISHED REST SERVICES [IN Module];\nSHOW BUSINESS EVENT SERVICES [IN Module];\nSHOW DATABASE CONNECTIONS [IN Module];\nSHOW EXTERNAL ENTITIES [IN Module];\nSHOW EXTERNAL ACTIONS [IN Module];",
		Example: "SHOW ODATA CLIENTS;\nSHOW REST CLIENTS IN MyModule;\nSHOW EXTERNAL ENTITIES;\nSELECT * FROM CATALOG.REST_CLIENTS;",
		SeeAlso: []string{"odata", "rest", "sql", "business-events"},
	})

	// ── OData ─────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "odata",
		Summary: "OData clients, services, and external entities",
		Keywords: []string{
			"odata", "consumed odata", "published odata",
			"external entity", "external entities", "metadata",
		},
		Syntax:  "SHOW ODATA CLIENTS [IN Module];\nSHOW ODATA SERVICES [IN Module];\nSHOW EXTERNAL ENTITIES [IN Module];\nSHOW EXTERNAL ACTIONS [IN Module];\nDESCRIBE ODATA CLIENT Module.Name;\nDESCRIBE ODATA SERVICE Module.Name;",
		Example: "SHOW ODATA CLIENTS;\nDESCRIBE ODATA CLIENT MyModule.ExternalAPI;\nSHOW EXTERNAL ENTITIES IN MyModule;",
		SeeAlso: []string{"odata.consume", "odata.show", "integration"},
	})

	Register(SyntaxFeature{
		Path:    "odata.consume",
		Summary: "Create consumed OData services and external entities",
		Keywords: []string{
			"create odata client", "consume odata", "external entity",
			"metadata url", "odata4", "headers", "proxy",
			"configurationmicroflow", "headersmicroflow", "service url",
		},
		Syntax: "CREATE ODATA CLIENT Module.Name (\n" +
			"  Version: '1.0',\n" +
			"  ODataVersion: OData4,\n" +
			"  MetadataUrl: 'https://.../$metadata',\n" +
			"  Timeout: 300,\n" +
			"  ServiceUrl: Module.ServiceUrlConstant,    -- must be a constant ref\n" +
			"  -- Configuration source dropdown — pick ONE: constants only (omit\n" +
			"  -- both microflows), Configuration microflow, OR Headers microflow.\n" +
			"  -- Both MDL keywords map to the same BSON field; Studio Pro picks\n" +
			"  -- the label from the microflow's return type:\n" +
			"  --   System.ConsumedODataConfiguration -> Configuration microflow\n" +
			"  --   list of System.HttpHeader         -> Headers microflow\n" +
			"  ConfigurationMicroflow: microflow Module.Configure,\n" +
			"  ErrorHandlingMicroflow: microflow Module.HandleError\n" +
			")\n" +
			"[HEADERS ('Key': 'Value')];\n\n" +
			"CREATE EXTERNAL ENTITY Module.Name\n" +
			"  FROM ODATA CLIENT Module.Client\n" +
			"  (EntitySet: 'Name', RemoteName: 'Name')\n" +
			"  (Attr: Type, ...);\n\n" +
			"CREATE EXTERNAL ENTITIES FROM Module.Client\n" +
			"  [INTO Module] [ENTITIES (Name1, Name2)];",
		Example: "CREATE CONSTANT MyModule.SvcUrl TYPE String DEFAULT 'https://api.example.com/odata/v4/';\n\nCREATE ODATA CLIENT MyModule.SalesforceAPI (\n  Version: '1.0',\n  ODataVersion: OData4,\n  MetadataUrl: 'https://api.example.com/odata/$metadata',\n  Timeout: 300,\n  ServiceUrl: MyModule.SvcUrl\n);\n\nCREATE EXTERNAL ENTITIES FROM MyModule.SalesforceAPI INTO Integration;",
		SeeAlso: []string{"odata", "odata.publish", "odata.show"},
	})

	Register(SyntaxFeature{
		Path:    "odata.publish",
		Summary: "Publish entities as OData services with KEY, options, and navigation properties",
		Keywords: []string{
			"create odata service", "publish entity", "publish odata",
			"expose", "key", "navigation property", "association exposure",
			"authentication", "page size", "servicename", "publishassociations",
			"readmode microflow", "non-persistable", "countable", "skipsupported",
			"topsupported",
			"graphql",
			"supportsgraphql",
		},
		Syntax: "CREATE [OR MODIFY] ODATA SERVICE Module.Name (\n" +
			"  path: 'odata/customers/',           -- no leading slash; trailing slash required\n" +
			"  version: '1.0.0',\n" +
			"  ODataVersion: OData4,\n" +
			"  namespace: 'Module.Customers',\n" +
			"  ServiceName: 'CustomerApi',        -- optional; defaults to the document name\n" +
			"  PublishAssociations: Yes,          -- optional; default Yes (associations as links)\n" +
			"  SupportsGraphQL: Yes               -- optional; also answer GraphQL at the SAME\n" +
			"                                     -- location (POST a query). Mendix 10.14+.\n" +
			"                                     -- Exposed names must then be unique beyond\n" +
			"                                     -- case (CE2881), and query fields are\n" +
			"                                     -- camelCased: Period -> period\n" +
			")\n" +
			"authentication basic, session\n" +
			"-- or, for custom authentication (no per-request password hash):\n" +
			"--   authentication microflow Module.Authenticate\n" +
			"-- The microflow takes a List of System.HttpHeader and returns a\n" +
			"-- System.User; returning empty denies the request. Requires app\n" +
			"-- security to be on (CE6600) and a microflow to be named (CE0333).\n" +
			"{\n" +
			"  publish entity Module.Entity as 'EntitySet' (\n" +
			"    ReadMode: source | microflow Module.Read_X,\n" +
			"    InsertMode: source | not_supported | microflow Module.Insert_X,\n" +
			"    UpdateMode: source | not_supported | microflow Module.Update_X,\n" +
			"    DeleteMode: source | not_supported | microflow Module.Delete_X,\n" +
			"    UsePaging: Yes,\n" +
			"    PageSize: 100,\n" +
			"    Countable: No,                   -- default Yes; No drops the $Response requirement\n" +
			"    SkipSupported: No,               -- default Yes ($skip)\n" +
			"    TopSupported: No                 -- default Yes ($top)\n" +
			"  )\n" +
			"  expose (\n" +
			"    KeyAttr as 'ExposedKey' (KEY, Filterable, Sortable),\n" +
			"    OtherAttr (Filterable),\n" +
			"    AssocName as 'NavProp'   -- bare association name -> PublishedAssociationEnd\n" +
			"  );\n" +
			"\n" +
			"  -- An OData action (an ActionImport in $metadata). Parameter types and\n" +
			"  -- the return type come off the microflow, which already declares them;\n" +
			"  -- omitting the expose clause publishes every parameter by its own name.\n" +
			"  publish microflow Module.DoThing as 'DoThing'\n" +
			"    expose ( Note as 'note', Amount as 'amount' (CanBeEmpty) );\n" +
			"};\n" +
			"\n" +
			"GRANT ACCESS ON ODATA SERVICE Module.Name TO Module.Role;\n" +
			"\n" +
			"-- A NON-PERSISTABLE entity can be published: back it with a read\n" +
			"-- microflow returning a list of that entity. Nothing is stored, so\n" +
			"-- there is no copy of the data in the database.\n" +
			"--\n" +
			"-- While Countable is Yes (the default) the read microflow must take a\n" +
			"-- $Response: System.ODataResponse parameter and set its Count; with\n" +
			"-- Countable: No it takes no parameters at all.\n" +
			"\n" +
			"-- HTTP STATUS CODES. An OData action, and an insert/update/delete\n" +
			"-- microflow, may take a $HttpResponse: System.HttpResponse parameter and\n" +
			"-- set StatusCode/Content. A READ microflow may not — it cannot answer\n" +
			"-- 400, so its contract has to be declared correctly instead:\n" +
			"--\n" +
			"--   * Mendix applies NO query options to a read-microflow resource. It\n" +
			"--     returns exactly what the microflow returns, so TopSupported and\n" +
			"--     SkipSupported describe YOUR microflow. Leaving them unspecified\n" +
			"--     publishes Yes. Declare No for anything you do not parse.\n" +
			"--   * A declared KEY promises a lookup by that key. A client holding a\n" +
			"--     row re-reads it as `?$filter=key eq '…'` on its own; answer it, or\n" +
			"--     the client adopts the first row of your collection default as that\n" +
			"--     object's identity, silently and with a 200.\n" +
			"--\n" +
			"-- Take a $Request: System.HttpRequest parameter to see the query string.\n" +
			"-- MDL-ODATA02 and MDL-ODATA03 flag a read microflow that takes none.",
		Example: "create persistent entity Shop.Customer (\n" +
			"  Email: string(200) unique error 'unique' required error 'required',\n" +
			"  Name:  string(200)\n" +
			");\n" +
			"\n" +
			"create odata service Shop.CustomerAPI (\n" +
			"  path: 'odata/customers/',\n" +
			"  version: '1.0.0',\n" +
			"  ODataVersion: OData4,\n" +
			"  namespace: 'Shop.Customers'\n" +
			")\n" +
			"authentication basic\n" +
			"{\n" +
			"  publish entity Shop.Customer as 'Customers' (\n" +
			"    ReadMode: source\n" +
			"  )\n" +
			"  expose (\n" +
			"    Email as 'customerId' (KEY, Filterable, Sortable),\n" +
			"    Name  (Filterable, Sortable)\n" +
			"  );\n" +
			"};",
		SeeAlso: []string{"odata", "odata.consume", "odata.show"},
	})

	Register(SyntaxFeature{
		Path:    "odata.show",
		Summary: "Browse cached OData contracts — entities, actions, properties",
		Keywords: []string{
			"contract", "show contract", "describe contract",
			"metadata", "entity type", "action", "navigation property",
		},
		Syntax:  "SHOW CONTRACT ENTITIES FROM Module.Client;\nSHOW CONTRACT ACTIONS FROM Module.Client;\nDESCRIBE CONTRACT ENTITY Module.Client.EntityName;\nDESCRIBE CONTRACT ENTITY Module.Client.EntityName FORMAT mdl;\nDESCRIBE CONTRACT ACTION Module.Client.ActionName;",
		Example: "SHOW CONTRACT ENTITIES FROM MyModule.SalesforceAPI;\nDESCRIBE CONTRACT ENTITY MyModule.SalesforceAPI.Product;\nDESCRIBE CONTRACT ENTITY MyModule.SalesforceAPI.Product FORMAT mdl;",
		SeeAlso: []string{"odata", "odata.consume"},
	})

	// ── REST ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "rest",
		Summary: "Consumed and published REST services",
		Keywords: []string{
			"rest", "rest client", "rest service",
			"published rest", "api", "http",
		},
		Syntax:  "SHOW REST CLIENTS [IN Module];\nSHOW PUBLISHED REST SERVICES [IN Module];\nDESCRIBE REST CLIENT Module.Name;\nDESCRIBE PUBLISHED REST SERVICE Module.Name;",
		Example: "SHOW REST CLIENTS;\nDESCRIBE REST CLIENT MyModule.PetStoreAPI;\nSHOW PUBLISHED REST SERVICES IN MyModule;",
		SeeAlso: []string{"rest.call", "rest.consumed", "rest.published", "integration"},
	})

	Register(SyntaxFeature{
		Path:    "rest.call",
		Summary: "REST CALL activity inside a microflow, and its five RETURNS forms",
		Keywords: []string{
			"rest call", "call rest service", "http get", "http post",
			"returns response", "returns string", "returns mapping",
			"file document", "filedocument", "download", "httpresponse",
			"body binary", "binary", "upload", "post binary",
		},
		Syntax: "[$Var =] REST CALL GET|POST|PUT|PATCH|DELETE '<url>' [WITH ({1} = expr, ...)]\n" +
			"  [HEADER 'Name' = expr]\n" +
			"  [AUTH BASIC $user PASSWORD $pass]\n" +
			"  [BODY '<template>' [WITH ({1} = expr)] | BODY <expr> | BODY BINARY <expr>\n" +
			"   | BODY MAPPING Module.EMM FROM $var]\n" +
			"  [TIMEOUT expr]\n" +
			"  RETURNS <one of>;\n\n" +
			"RETURNS String                          -- the response body as a string\n" +
			"RETURNS response                        -- the whole System.HttpResponse object\n" +
			"RETURNS Module.MyFile                   -- store the body in a file document\n" +
			"RETURNS MAPPING Module.IMM AS Module.E  -- apply an import mapping (single object)\n" +
			"RETURNS MAPPING Module.IMM AS LIST OF Module.E\n" +
			"RETURNS NONE | NOTHING                  -- ignore the response\n\n" +
			"-- The file document form takes a SPECIALIZATION of System.FileDocument.\n" +
			"-- Mendix rejects the base type as a return type (CE0362), and MDL064\n" +
			"-- reports that before the write. There is no matching form for an\n" +
			"-- HttpResponse specialization because Mendix does not allow one\n" +
			"-- (CE1540) — `RETURNS response` already names the only type it can be.",
		Example: "create persistent entity MyModule.MyFile extends System.FileDocument ();\n\n" +
			"create microflow MyModule.ACT_Download ($Location: String)\n" +
			"begin\n" +
			"  $file = rest call get '{1}' with ({1} = $Location)\n" +
			"    header 'Accept' = 'application/octet-stream'\n" +
			"    timeout 300\n" +
			"    returns MyModule.MyFile;\n" +
			"end;",
		SeeAlso: []string{"rest", "rest.consumed", "microflow"},
	})

	// ── SOAP (legacy web services) ────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "soap",
		Summary: "Legacy SOAP web service calls: operations, arguments, send/receive mappings",
		Keywords: []string{
			"soap", "web service", "call web service", "wsdl",
			"imported service", "consumed web service", "operation",
			"send mapping", "receive mapping", "request body",
			"parameter path", "ce0178", "ce0369", "ce0386",
		},
		Syntax: "[$Var =] CALL WEB SERVICE Module.ImportedService\n" +
			"  [OPERATION Name [(Param = expr, ...)]]\n" +
			"  [SEND MAPPING Module.ExportMapping FROM $var]\n" +
			"  [RECEIVE MAPPING Module.ImportMapping]\n" +
			"  [TIMEOUT expr]\n" +
			"  [ON ERROR ...];\n\n" +
			"[$Var =] CALL WEB SERVICE RAW '<base64-bson>';\n\n" +
			"-- The REQUEST BODY is one of two things, never both: the operation's\n" +
			"-- arguments, or an export mapping. Mendix stores ONE\n" +
			"-- RequestBodyHandling, so asking for each is refused as MDL-SOAP01 by\n" +
			"-- `mxcli check` and by exec, which run the same function.\n" +
			"--\n" +
			"-- ARGUMENTS use the same (Name = value) form as every other call\n" +
			"-- statement. Mendix keys each one by a ParameterPath\n" +
			"-- (http%3A//www.example.com/:GetOrder|OrderId) built from the\n" +
			"-- operation's request body element; mxcli reads that off the consumed\n" +
			"-- service document, so the script names only the parameter. The\n" +
			"-- service must therefore be present and declare the operation — one it\n" +
			"-- cannot resolve is refused, not written with a guessed path. Omitting\n" +
			"-- arguments an operation requires is CE0178.\n" +
			"--\n" +
			"-- A misspelled PARAMETER NAME cannot be checked: the names live in the\n" +
			"-- WSDL's inline schema, which mxcli does not parse. It arrives as\n" +
			"-- CE0178 from mxbuild.\n" +
			"--\n" +
			"-- SEND MAPPING needs FROM $var — an export mapping maps an object and\n" +
			"-- Mendix stores which one. Without it the call builds as CE0369.\n" +
			"--\n" +
			"-- DESCRIBE renders the structured form only when re-executing it would\n" +
			"-- reproduce the stored document exactly. A call with HTTP\n" +
			"-- authentication, a custom location, SOAP headers, a per-parameter\n" +
			"-- export mapping, or a result typed from the WSDL rather than from an\n" +
			"-- import mapping keeps the RAW form, which round-trips byte for byte.",
		Example: "-- Arguments\n" +
			"$Order = call web service Clients.OrderSoapClient\n" +
			"  operation GetOrder (OrderId = $Customer/OrderId)\n" +
			"  receive mapping Clients.SoapOrdersImportMapping;\n\n" +
			"-- Export mapping as the request body\n" +
			"call web service Clients.OrderSoapClient\n" +
			"  operation SaveOrder\n" +
			"  send mapping Clients.SoapOrderExportMapping from $NewSaveOrder;",
		SeeAlso: []string{"integration", "microflow", "rest"},
	})

	Register(SyntaxFeature{
		Path:    "rest.consumed",
		Summary: "Create consumed REST clients with operations, mappings, and authentication",
		Keywords: []string{
			"create rest client", "consume rest", "rest operation",
			"get", "post", "put", "delete", "patch",
			"body", "response", "mapping", "authentication",
			"json structure", "import mapping", "export mapping",
		},
		Syntax:  "CREATE [OR MODIFY] REST CLIENT Module.Name (\n  BaseUrl: 'https://...',\n  Authentication: NONE | BASIC (...)\n)\n{\n  OPERATION Name {\n    Method: GET|POST|PUT|DELETE|PATCH,\n    Path: '/path/{param}',\n    Parameters: ($param: Type),\n    Query: ($param: Type),\n    Headers: ('Key' = 'Value'),\n    Timeout: 30,\n    Body: JSON FROM $var | MAPPING Entity { jsonField = Attribute, ... },\n    Response: JSON AS $var | MAPPING Entity { Attribute = jsonField, ... }\n  }\n};\n\n-- MAPPING takes a target ENTITY plus a body listing the JSON fields; Mendix\n-- stores it inline on the operation. An existing import/export mapping\n-- document cannot be referenced here (rejected as MDL-REST01).\n-- There is no FILE request body: Mendix's consumed operation stores one of\n-- Rest$JsonBody, Rest$StringBody or Rest$ImplicitMappingBody, so a file\n-- document has nowhere to go. `Body: FILE FROM $Doc` is rejected as\n-- MDL-REST02 rather than sent as the literal text \"$Doc\" (it used to be,\n-- returning 200 with a 4-byte payload). Binary POST lives on the\n-- microflow activity: `rest call post '<url>' body binary $Doc/Contents`.\n-- `Response: FILE AS $Doc` is unaffected — downloads work.",
		Example: "CREATE REST CLIENT Module.PetStore (\n  BaseUrl: 'https://petstore.example.com/api',\n  Authentication: NONE\n)\n{\n  OPERATION GetPet {\n    Method: GET,\n    Path: '/pets/{id}',\n    Parameters: ($id: String),\n    Query: ($verbose: String),\n    Response: MAPPING Module.Pet {\n      Name = name,\n      Status = status\n    }\n  }\n};",
		SeeAlso: []string{"rest", "rest.published"},
	})

	Register(SyntaxFeature{
		Path:    "rest.published",
		Summary: "Create and manage published REST services with resources and operations",
		Keywords: []string{
			"create published rest", "publish rest", "rest resource",
			"rest operation", "microflow", "path parameter",
			"query parameter", "body parameter", "operation parameter",
			"grant access", "revoke access",
		},
		Syntax:  "CREATE [OR MODIFY] PUBLISHED REST SERVICE Module.Name (\n  Path: 'rest/api/v1',\n  Version: '1.0.0',\n  ServiceName: 'My API'\n)\n{\n  RESOURCE 'name' {\n    GET '' MICROFLOW Module.GetAll;\n    GET '{id}' MICROFLOW Module.GetById;\n    POST '' MICROFLOW Module.Create;\n  }\n};\n\n-- Operation parameters come from the microflow's parameters, as in Studio Pro:\n-- a parameter named in the path ({id}) is a path parameter, an object or a list\n-- is the body, System.HttpRequest / HttpResponse are the request and response,\n-- and any other parameter is a query parameter: $orderNumber: String behind\n-- GET 'status' answers /status?orderNumber=... Create the microflow first.\n\nALTER PUBLISHED REST SERVICE Module.Name SET Version = '2.0.0';\nALTER PUBLISHED REST SERVICE Module.Name ADD RESOURCE 'items' { ... };\nALTER PUBLISHED REST SERVICE Module.Name DROP RESOURCE 'legacy';\nDROP PUBLISHED REST SERVICE Module.Name;",
		Example: "CREATE PUBLISHED REST SERVICE Module.OrderAPI (\n  Path: 'rest/orders/v1',\n  Version: '1.0.0',\n  ServiceName: 'Order API'\n)\n{\n  RESOURCE 'orders' {\n    GET '' MICROFLOW Module.GetAllOrders;\n    GET '{id}' MICROFLOW Module.GetOrderById;\n    POST '' MICROFLOW Module.CreateOrder;\n    DELETE '{id}' MICROFLOW Module.DeleteOrder;\n  }\n};\n\nGRANT ACCESS ON PUBLISHED REST SERVICE Module.OrderAPI\n  TO Module.User, Module.Admin;",
		SeeAlso: []string{"rest", "rest.consumed"},
	})

	// ── SQL ───────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "sql",
		Summary: "External SQL queries against PostgreSQL, Oracle, SQL Server",
		Keywords: []string{
			"sql", "external sql", "database", "postgres",
			"oracle", "sqlserver", "mssql", "query",
		},
		Syntax:  "SQL CONNECT <driver> '<dsn>' AS <alias>;\nSQL <alias> SHOW TABLES;\nSQL <alias> SELECT ...;\nSQL CONNECTIONS;\nSQL DISCONNECT <alias>;",
		Example: "SQL CONNECT postgres 'postgres://user:pass@localhost:5432/mydb' AS source;\nSQL source SHOW TABLES;\nSQL source SELECT * FROM users WHERE active = true LIMIT 10;\nSQL DISCONNECT source;",
		SeeAlso: []string{"sql.connect", "sql.query", "sql.import", "sql.generate"},
	})

	Register(SyntaxFeature{
		Path:    "sql.connect",
		Summary: "Connect to external databases with credential isolation",
		Keywords: []string{
			"sql connect", "database connect", "dsn",
			"postgres", "oracle", "sqlserver", "driver",
			"connections.yaml", "credential",
		},
		Syntax:  "SQL CONNECT <driver> '<dsn>' AS <alias>;\nSQL CONNECT postgres '<dsn>' AS <alias>;\nSQL CONNECT oracle '<dsn>' AS <alias>;\nSQL CONNECT sqlserver '<dsn>' AS <alias>;\nSQL CONNECTIONS;\nSQL DISCONNECT <alias>;",
		Example: "SQL CONNECT postgres 'postgres://user:pass@localhost:5432/mydb' AS source;\nSQL CONNECTIONS;\nSQL DISCONNECT source;",
		SeeAlso: []string{"sql", "sql.query"},
	})

	Register(SyntaxFeature{
		Path:    "sql.query",
		Summary: "Execute SQL queries and browse schema of external databases",
		Keywords: []string{
			"sql query", "sql select", "show tables",
			"describe table", "sql insert", "sql execute",
		},
		Syntax:  "SQL <alias> SHOW TABLES;\nSQL <alias> SHOW VIEWS;\nSQL <alias> SHOW FUNCTIONS;\nSQL <alias> DESCRIBE <table>;\nSQL <alias> SELECT ...;\nSQL <alias> INSERT ...;",
		Example: "SQL source SHOW TABLES;\nSQL source DESCRIBE users;\nSQL source SELECT * FROM users WHERE active = true LIMIT 10;",
		SeeAlso: []string{"sql.connect", "sql.import"},
	})

	Register(SyntaxFeature{
		Path:    "sql.import",
		Summary: "Import rows from external DB into Mendix app database",
		Keywords: []string{
			"import", "import from", "import into", "map",
			"batch", "link", "association", "data migration",
		},
		Syntax:  "IMPORT FROM <alias> QUERY '<sql>'\n  INTO Module.Entity\n  MAP (col AS Attr, ...)\n  [LINK (col TO Assoc ON Attr, ...)]\n  [BATCH n]\n  [LIMIT n];",
		Example: "IMPORT FROM source QUERY 'SELECT name, email FROM employees'\n  INTO HR.Employee\n  MAP (name AS Name, email AS Email);\n\nIMPORT FROM source QUERY 'SELECT name, dept_name FROM employees'\n  INTO HR.Employee\n  MAP (name AS Name)\n  LINK (dept_name TO Employee_Department ON Name)\n  BATCH 500\n  LIMIT 10000;",
		SeeAlso: []string{"sql", "sql.connect"},
	})

	Register(SyntaxFeature{
		Path:    "sql.generate",
		Summary: "Auto-generate Database Connector MDL from external database schema",
		Keywords: []string{
			"generate connector", "database connector",
			"constants", "non-persistent entity", "jdbc",
		},
		Syntax:  "SQL <alias> GENERATE CONNECTOR INTO Module\n  [TABLES (t1, t2)]\n  [VIEWS (v1, v2)]\n  [EXEC];",
		Example: "SQL source GENERATE CONNECTOR INTO HRModule;\nSQL source GENERATE CONNECTOR INTO HRModule TABLES (employees, departments) EXEC;",
		SeeAlso: []string{"sql", "sql.connect"},
	})

	// ── OQL ───────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "oql",
		Summary: "OQL query execution against a running Mendix runtime",
		Keywords: []string{
			"oql", "query", "runtime", "m2ee",
			"aggregate", "view entity", "mxcli oql",
		},
		Syntax:  "mxcli oql -p app.mpr \"SELECT ...\";\nmxcli oql -p app.mpr --json \"SELECT ...\";\nmxcli oql --direct --host localhost --port 8090 --token 'pass' \"SELECT ...\";",
		Example: "mxcli oql -p app.mpr \"SELECT Name, Email FROM MyModule.Customer\";\nmxcli oql -p app.mpr --json \"SELECT count(c.ID) FROM MyModule.Order AS c\" | jq '.[0]';",
		SeeAlso: []string{"sql"},
	})

	// ── External database connector ───────────────────────────────────

	Register(SyntaxFeature{
		Path:    "database-connection",
		Summary: "External Database Connector — query another database from a microflow",
		Keywords: []string{
			"database connection", "database connections", "external database",
			"create database connection", "drop database connection",
			"describe database connection", "show database connections",
			"jdbc", "byod", "database connector", "execute database query",
			"postgresql", "mysql", "oracle", "snowflake", "sql server",
		},
		Syntax: `CREATE [OR MODIFY] DATABASE CONNECTION Module.Name [FOLDER 'path']
  TYPE '<type>'
  CONNECTION STRING @Module.UrlConstant
  USERNAME @Module.UserConstant
  PASSWORD @Module.PasswordConstant
[BEGIN
  QUERY <QueryName>
    SQL $$<sql>$$
    [PARAMETER <name>: <Type> [DEFAULT '<value>' | NULL]]
    [RETURNS Module.Entity
      [MAP ( <column> AS <Attribute>, ... )]]
  ;
END];

SHOW DATABASE CONNECTIONS [IN <module>];
DESCRIBE DATABASE CONNECTION Module.Name;
DROP DATABASE CONNECTION Module.Name;

Calling a query from a microflow:
  EXECUTE DATABASE QUERY Module.Connection.QueryName (...);

TYPE is one of Studio Pro's entries — 'MSSQL', 'MySQL', 'Oracle',
'PostgreSQL', 'Snowflake' — or 'BYOD' ("bring your own driver"), which skips
the driver-presence check and takes the connection string verbatim. Use BYOD
for any JDBC driver Mendix has no entry for; put the driver on the classpath
with ALTER MODULE ... ADD JAR DEPENDENCY + mxcli sync-java-deps. An
unrecognised type is reported by MDL-DB01 — mxbuild does not check it, so the
build stays green and the connection simply does not work.

Three traps:

  1. CONNECTION STRING / USERNAME / PASSWORD must reference CONSTANTS
     (@Module.Name), not literals. A literal produces a project Studio Pro
     cannot open at all: StorageLoadException "is not a valid
     ConstantIdentifier". mxcli catches it (MDL058); mxbuild does not, so the
     build is green.
  2. USERNAME and PASSWORD must be given even when the driver needs neither.
     Omitting them writes an empty constant reference, the build stays green,
     and the query fails only at run time with "Could not find value for
     constant ''".
  3. A named type needs its JDBC driver declared on the module, or the build
     fails with CE5278 ("The PostgreSQL JDBC driver (org.postgresql:postgresql)
     is missing from the module settings"). Unlike the first two, this one is
     caught at build time:

       ALTER MODULE Ops ADD JAR DEPENDENCY (
         group = 'org.postgresql', artifact = 'postgresql',
         version = '42.7.4', included = true
       );

     then run: mxcli sync-java-deps -p app.mpr — to put it on the classpath —
     declaring without syncing gives a green build and a
     ClassNotFoundException. BYOD skips this check, which is the point of it.

The connector runtime is part of the platform — no marketplace module is
needed in the project.`,
		Example: `-- The three constants the connection points at.
CREATE CONSTANT Ops.DbUrl  TYPE String DEFAULT 'jdbc:postgresql://localhost:5432/erp';
CREATE CONSTANT Ops.DbUser TYPE String DEFAULT 'reader';
CREATE CONSTANT Ops.DbPass TYPE String DEFAULT '';

CREATE NON-PERSISTENT ENTITY Ops.EmployeeRow (
  EmployeeId: Integer,
  Name: String(100)
);

CREATE DATABASE CONNECTION Ops.Erp
  TYPE 'PostgreSQL'
  CONNECTION STRING @Ops.DbUrl
  USERNAME @Ops.DbUser
  PASSWORD @Ops.DbPass
BEGIN
  QUERY GetEmployees
    SQL $$SELECT id, name FROM employees WHERE dept = {dept}$$
    PARAMETER dept: String DEFAULT 'sales'
    RETURNS Ops.EmployeeRow
    MAP (
      id AS EmployeeId,
      name AS Name
    )
  ;
END;

-- A named type needs its driver declared, or the build fails with CE5278.
ALTER MODULE Ops ADD JAR DEPENDENCY (
  group = 'org.postgresql', artifact = 'postgresql',
  version = '42.7.4', included = true
);

SHOW DATABASE CONNECTIONS IN Ops;
DESCRIBE DATABASE CONNECTION Ops.Erp;`,
		SeeAlso: []string{"integration", "sql", "microflow.call"},
	})

	// ── Business Events ───────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "business-events",
		Summary: "Business event services — publish and subscribe to events via Kafka",
		Keywords: []string{
			"business event", "business events", "kafka",
			"publish", "subscribe", "message", "event channel",
		},
		Syntax:  "SHOW BUSINESS EVENT SERVICES [IN Module];\nSHOW BUSINESS EVENTS [IN Module];\nDESCRIBE BUSINESS EVENT SERVICE Module.Name;\nCREATE [OR MODIFY] BUSINESS EVENT SERVICE Module.Name (...) { ... };\nDROP BUSINESS EVENT SERVICE Module.Name;",
		Example: "SHOW BUSINESS EVENT SERVICES;\nDESCRIBE BUSINESS EVENT SERVICE Module.CustomerEventsApi;",
		SeeAlso: []string{"business-events.create", "integration"},
	})

	Register(SyntaxFeature{
		Path:    "business-events.create",
		Summary: "Create and drop business event service definitions with messages",
		Keywords: []string{
			"create business event", "drop business event",
			"message", "publish", "subscribe", "entity",
			"event name prefix",
		},
		Syntax:  "CREATE [OR MODIFY] BUSINESS EVENT SERVICE Module.Name\n(\n  ServiceName: 'Name',\n  EventNamePrefix: ''\n)\n{\n  MESSAGE EventName (Attr: Type, ...) PUBLISH|SUBSCRIBE\n    ENTITY Module.PBE_Entity;\n};\n\nDROP BUSINESS EVENT SERVICE Module.Name;",
		Example: "CREATE BUSINESS EVENT SERVICE Module.CustomerEventsApi\n(\n  ServiceName: 'CustomerEventsApi',\n  EventNamePrefix: ''\n)\n{\n  MESSAGE CustomerChangedEvent (CustomerId: Long) PUBLISH\n    ENTITY Module.PBE_CustomerChangedEvent;\n};\n\nDROP BUSINESS EVENT SERVICE Module.CustomerEventsApi;",
		SeeAlso: []string{"business-events"},
	})

	// ── XPath ─────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "xpath",
		Summary: "XPath constraint syntax for filtering data in RETRIEVE, pages, and security",
		Keywords: []string{
			"xpath", "constraint", "where", "predicate",
			"filter", "retrieve", "association path", "enumeration",
			"formatting", "readable", "line length", "wrap", "multi-line",
		},
		Syntax:  "WHERE [condition]\nWHERE [cond1][cond2]          -- implicit AND\nWHERE [cond1] AND [cond2]\nWHERE [cond1] OR [cond2]\n\nHow it is STORED is mxcli's choice, not your whitespace: a constraint is\nrebuilt from its parse tree on every write, so the layout is derived from the\nexpression. 80 columns or fewer is stored exactly as written. Longer is broken\nat its top-level and/or joints, one clause per line, so it can be read in\nStudio Pro's XPath editor without scrolling sideways:\n\n  [\n    Archived = false\n    and Status = 'Open'\n    and ReportedOn > '[%BeginOfCurrentDay%]'\n  ]\n\nWhere and/or meet, the and-runs get explicit parentheses (Mendix binds `and`\ntighter). A clause with no boolean joint — one long comparison or association\npath — is left whole and over width. DESCRIBE puts it back on one line, and\nre-executing that re-derives the same stored text.",
		Example: "RETRIEVE $Orders FROM Module.Order\n  WHERE [State = 'Completed'][IsPaid = true]\n  SORT BY OrderDate DESC;\n\n-- Enumeration attribute: qualified name preferred (mxcli converts to 'Open' in BSON)\nRETRIEVE $Open FROM Module.Order\n  WHERE [Status = Module.OrderStatus.Open];\n\n-- OR: string literal form also accepted\nRETRIEVE $Open FROM Module.Order\n  WHERE [Status = 'Open'];\n\n-- Association path traversal\nWHERE [Module.Order_Customer/Module.Customer/Name = $Name]\n\n-- Mendix tokens\nWHERE [System.owner = '[%CurrentUser%]']",
		SeeAlso: []string{"xpath.functions"},
	})

	Register(SyntaxFeature{
		Path:    "xpath.functions",
		Summary: "XPath operators and functions — contains, starts-with, not, boolean literals",
		Keywords: []string{
			"xpath functions", "contains", "starts-with",
			"not", "true", "false", "operators",
			"comparison", "boolean",
		},
		Syntax:  "=, !=, <, >, <=, >=          Comparison\nand, or                      Boolean (lowercase)\nnot(expr)                    Negation\ncontains(attr, 'text')       String contains\nstarts-with(attr, 'text')    String starts-with\ntrue(), false()              Boolean literals",
		Example: "WHERE [not(IsArchived)]\nWHERE [contains(Name, 'Corp')]\nWHERE [starts-with(Code, 'PRD')]\nWHERE [State = 'Ready' and Priority > 5]",
		SeeAlso: []string{"xpath"},
	})

	// ── Java Action ───────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "java-action",
		Summary: "Java actions — custom Java code callable from microflows",
		Keywords: []string{
			"java action", "java", "call java",
			"type parameter", "exposed as", "javaaction",
		},
		Syntax:  "SHOW JAVA ACTIONS [IN Module];\nDESCRIBE JAVA ACTION Module.Name;\nCREATE [OR MODIFY] JAVA ACTION Module.Name [FOLDER 'path'](...) RETURNS Type AS $$ ... $$;\nDROP JAVA ACTION Module.Name;\n\nNOTE: AS $$ ... $$ is mandatory — omitting the body causes a parse error.",
		Example: "SHOW JAVA ACTIONS;\nDESCRIBE JAVA ACTION Utils.FormatCurrency;",
		SeeAlso: []string{"java-action.create"},
	})

	Register(SyntaxFeature{
		Path:    "java-action.create",
		Summary: "Create Java actions with type parameters, EXPOSED AS, and inline code",
		Keywords: []string{
			"create java action", "or modify java action", "type parameter", "entity parameter",
			"exposed as", "returns", "generics", "drop java action",
		},
		Syntax:  "CREATE [OR MODIFY] JAVA ACTION Module.Name [FOLDER 'path'](\n  Param: Type [NOT NULL],\n  EntityType: ENTITY <pEntity> NOT NULL,\n  Obj: pEntity\n) RETURNS ReturnType\n[EXPOSED AS 'Label' IN 'Category'\n   [ICON 'icon.png'] [ICON DARK 'icon-dark.png']\n   [IMAGE 'image.png'] [IMAGE DARK 'image-dark.png']]\n[NOT EXPOSED]\nAS $$\n// Java code — AS $$ ... $$ is mandatory, cannot be omitted\n$$;\n\nOR MODIFY: updates signature/body in-place, preserves UUID.\n\nEXPOSED AS puts the action in Studio Pro's toolbox. The icon is a 64x64 PNG\nand the image a 256x192 PNG, read from disk relative to the .mdl file's own directory.\nAn OMITTED clause preserves what is stored — including bitmaps set in Studio\nPro — so removing an entry is NOT EXPOSED, and clearing one bitmap is\nDROP ICON / DROP IMAGE [DARK].",
		Example: "CREATE JAVA ACTION Utils.FormatCurrency(\n  Amount: Decimal NOT NULL\n) RETURNS String\nEXPOSED AS 'Format Currency' IN 'Formatting'\n  ICON 'assets/currency-64.png'\n  IMAGE 'assets/currency-256.png'\nAS $$\nreturn String.format(\"%.2f\", Amount);\n$$;\n\n-- Generic entity validator with type parameter\nCREATE JAVA ACTION Utils.IsValid(\n  EntityType: ENTITY <pEntity> NOT NULL,\n  Obj: pEntity NOT NULL\n) RETURNS Boolean\nAS $$\nreturn Obj != null;\n$$;\n\n-- Idempotent update (preserves UUID)\nCREATE OR MODIFY JAVA ACTION Utils.FormatCurrency(\n  Amount: Decimal NOT NULL,\n  Decimals: Integer NOT NULL\n) RETURNS String\nAS $$\nreturn String.format(\"%.\" + Decimals + \"f\", Amount);\n$$;",
		SeeAlso: []string{"java-action", "javascript-action"},
	})

	Register(SyntaxFeature{
		Path:    "javascript-action",
		Summary: "JavaScript actions — client-side JS callable from nanoflows",
		Keywords: []string{
			"javascript action", "javascript", "js action", "call javascript",
			"platform", "exposed as", "javascriptaction",
		},
		Syntax:  "SHOW JAVASCRIPT ACTIONS [IN Module];\nDESCRIBE JAVASCRIPT ACTION Module.Name;\nCREATE [OR MODIFY] JAVASCRIPT ACTION Module.Name [FOLDER 'path'](...) RETURNS Type [PLATFORM Web|Native|Hybrid|All] AS $$ ... $$;\nDROP JAVASCRIPT ACTION Module.Name;\n\nWrites the unit plus javascriptsource/<Module>/actions/<Name>.js. AS $$ ... $$ is mandatory. PLATFORM defaults to Web.",
		Example: "CREATE JAVASCRIPT ACTION Utils.IsStrictMode() RETURNS Boolean\nPLATFORM Web\nAS $$\nreturn Promise.resolve((function(){ return !this; })());\n$$;\n\n-- Exposed, native, with parameters\nCREATE JAVASCRIPT ACTION Utils.ShowToast(\n  Message: String NOT NULL,\n  Duration: Integer\n) RETURNS Boolean\nEXPOSED AS 'Show Toast' IN 'UI'\nPLATFORM Native\nAS $$\nreturn Promise.resolve(true);\n$$;",
		SeeAlso: []string{"java-action"},
	})

	// ── JSON Structures ───────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "message-definition",
		Summary: "Message definition collections — a mapping source built from the domain model",
		Keywords: []string{
			"message definition", "message definition collection", "create message definition",
			"exposed entity", "exposed attribute", "exposed association",
		},
		Syntax: "SHOW MESSAGE DEFINITION COLLECTIONS [IN Module];\nDESCRIBE MESSAGE DEFINITION COLLECTION Module.Name;\nCREATE [OR MODIFY] MESSAGE DEFINITION COLLECTION Module.Name [FOLDER 'path']\n(\n  DEFINITION Name FOR Module.Entity [AS 'Exposed'] (\n    AttributeName [AS 'Exposed'] [EXAMPLE 'text'],\n    Module.Assoc/Module.TargetEntity [AS 'Exposed'] ( ... )\n  )\n);\nDROP MESSAGE DEFINITION COLLECTION Module.Name;\n\n" +
			"ALTER MESSAGE DEFINITION COLLECTION Module.Name\n" +
			"  ADD DEFINITION [IF NOT EXISTS] Name FOR Module.Entity [AS 'X'] ( ... )\n" +
			"  | DROP DEFINITION [IF EXISTS] Name\n" +
			"  | RENAME DEFINITION Old TO New;\n\n" +
			"ALTER MESSAGE DEFINITION Module.Collection.Definition\n" +
			"  ADD MEMBER [IF NOT EXISTS] <member> [IN path]\n" +
			"  | DROP MEMBER [IF EXISTS] Name [IN path]\n" +
			"  | SET MEMBER Name [IN path] AS 'Exposed';\n\n" +
			"A message definition is a SELECTION OVER THE DOMAIN MODEL — every element\n" +
			"names an entity, an attribute or an association — which is why it is\n" +
			"authorable where an XML schema or a WSDL is not. It is the source for 74 of\n" +
			"the 327 mappings in the demo corpus.\n\n" +
			"A bare name is an ATTRIBUTE; Assoc/Module.Entity is an ASSOCIATION.\n" +
			"Naming the association's TARGET is required: the stored cardinality follows\n" +
			"the DIRECTION of traversal, so the same association gives a single object\n" +
			"one way and a list the other. One that connects neither way is refused,\n" +
			"because a wrong cardinality builds cleanly.\n\n" +
			"Inherited attributes are named like the entity's own and resolve to the\n" +
			"entity that DECLARES them. Everything else — occurrence bounds, element\n" +
			"types, paths, item names, primitive types — is derived. Studio Pro\n" +
			"pluralises a repeating element's exposed name; mxcli defaults to the\n" +
			"entity's own name and lets AS say otherwise.\n\n" +
			"IN <path> reaches a nested member, in exposed names. SET changes only the\n" +
			"exposed name — it is not a model rename. Authoring is modelsdk-only.",
		Example: "CREATE MESSAGE DEFINITION COLLECTION Sales.MD_Order\n(\n  DEFINITION OrderMessage FOR Sales.Order AS 'Orders' (\n    OrderId,\n    Sales.OrderLine_Order/Sales.OrderLine AS 'Lines' ( Sku, Quantity ),\n    Sales.Order_Customer/Sales.Customer ( FirstName )\n  )\n);\n\nALTER MESSAGE DEFINITION Sales.MD_Order.OrderMessage ADD MEMBER LastName IN Customer;\n\nCREATE IMPORT MAPPING Sales.IMM_Order\n  WITH MESSAGE DEFINITION Sales.MD_Order.OrderMessage\n{ create Sales.Order { OrderId = OrderId } };",
	})

	Register(SyntaxFeature{
		Path:    "json-structure",
		Summary: "JSON structures — schema snapshots used by import/export mappings",
		Keywords: []string{
			"json structure", "create json structure", "drop json structure",
			"snippet", "schema", "json schema",
		},
		Syntax: "SHOW JSON STRUCTURES [IN Module];\nDESCRIBE JSON STRUCTURE Module.Name;\nCREATE JSON STRUCTURE Module.Name [FOLDER 'path'] [COMMENT 'text'] SNIPPET '{ ... }'\n  [CUSTOM NAME MAP (\n    'jsonKey' AS 'CustomName',       -- rename the element that key reaches\n    ITEM OF 'arrayKey' AS 'Name',    -- name the ARRAY's item element\n    ITEM OF 'Root' AS 'Name'         -- ... of a ROOT-level array\n  )];\nCREATE OR MODIFY JSON STRUCTURE Module.Name SNIPPET '{ ... }';\nDROP JSON STRUCTURE Module.Name;\n\n" +
			"An array's item is the anonymous [...] entry, so it has no JSON key and the\n" +
			"plain form cannot reach it — ITEM OF addresses it by the array's key, and\n" +
			"names a primitive array's wrapper too. Left unnamed an item keeps its\n" +
			"generated name. The name matters because a mapping element clones it.\n" +
			"An entry whose key is not in the snippet is an error (MDL-JSON01), as is\n" +
			"ITEM OF on a key that is not an array (MDL-JSON02).",
		Example: "CREATE OR MODIFY JSON STRUCTURE MyModule.JSON_Pet\n  SNIPPET '{\"id\": 1, \"name\": \"Fido\", \"status\": \"available\"}';\n\nCREATE JSON STRUCTURE MyModule.JSON_Invoice\n  SNIPPET '{\"lines\": [{\"sku\": \"A1\"}], \"tags\": [\"urgent\"]}'\n  CUSTOM NAME MAP (\n    'lines' AS 'OrderLines',\n    ITEM OF 'lines' AS 'OrderLine',\n    ITEM OF 'tags' AS 'Tag'\n  );\n\nDESCRIBE JSON STRUCTURE MyModule.JSON_Pet;",
		SeeAlso: []string{"import-mapping", "export-mapping"},
	})

	// ── Image Collections ─────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "image-collection",
		Summary: "Image collections — bundle images (icons, logos) within a module",
		Keywords: []string{
			"image collection", "create image collection", "drop image collection",
			"export level", "image", "icon", "logo",
		},
		Syntax:  "SHOW IMAGE COLLECTION [IN Module];\nDESCRIBE IMAGE COLLECTION Module.Name;\nCREATE IMAGE COLLECTION Module.Name [FOLDER 'path']\n  [EXPORT LEVEL 'Hidden'|'Public']\n  [COMMENT 'text']\n  [(IMAGE name FROM FILE 'path', ...)];\nCREATE OR MODIFY IMAGE COLLECTION Module.Name [...];\nDROP IMAGE COLLECTION Module.Name;",
		Example: "CREATE OR MODIFY IMAGE COLLECTION MyModule.AppIcons\n  EXPORT LEVEL 'Public'\n  COMMENT 'Application icons' (\n  IMAGE logo FROM FILE 'assets/logo.png',\n  IMAGE \"favicon\" FROM FILE 'assets/favicon.ico'\n);\n\nDESCRIBE IMAGE COLLECTION MyModule.AppIcons;",
		SeeAlso: []string{"integration", "icon-collection"},
	})

	Register(SyntaxFeature{
		Path:    "icon-collection",
		Summary: "Icon collections (read-only) — discover icon names for a widget's icon:",
		Keywords: []string{
			"icon collection", "icon set", "iconset", "show icon collections",
			"describe icon collection", "atlas icon", "button icon", "glyph",
		},
		Syntax:  "SHOW ICON COLLECTIONS [IN Module];\nDESCRIBE ICON COLLECTION Module.Name;   -- lists every icon + its reference form",
		Example: "-- Icon collections ship with the theme/Atlas (read-only). Their icons are\n-- referenced from a widget as Module.Collection.IconName (e.g. a button's icon:).\nSHOW ICON COLLECTIONS;\nDESCRIBE ICON COLLECTION Atlas_Core.Atlas_Filled;\n-- → then: ACTIONBUTTON b (Caption: 'Edit', Action: ..., Icon: 'Atlas_Core.Atlas_Filled.pencil')",
		SeeAlso: []string{"image-collection", "glyph", "page.action"},
	})

	Register(SyntaxFeature{
		Path:    "glyph",
		Summary: "Glyph icons — the numeric codes `icon glyph <n>` accepts, and their names",
		Keywords: []string{
			"glyph", "glyphs", "show glyphs", "describe glyph", "icon glyph",
			"glyphicon", "glyph code", "menu icon", "navigation icon",
		},
		Syntax: "SHOW GLYPHS [LIKE 'text'];       -- LIKE matches the icon NAME\n" +
			"DESCRIBE GLYPH 57350;            -- by character code\n" +
			"DESCRIBE GLYPH 'star';           -- or by name",
		Example: "-- A glyph is a character code in the Mendix glyph font, not a document in the\n" +
			"-- project, so there is no module to scope and no connection needed.\n" +
			"SHOW GLYPHS LIKE 'star';\n" +
			"--   57350  star        icon glyph 57350\n" +
			"--   57351  star-empty  icon glyph 57351\n" +
			"DESCRIBE GLYPH 'star';\n" +
			"-- → then: MENU ITEM 'Favourites' PAGE M.Favourites ICON GLYPH 57350;\n" +
			"\n" +
			"-- Prefer an icon COLLECTION reference where the icon exists there: it is a\n" +
			"-- model reference that `check --references` resolves, while a glyph code is\n" +
			"-- an unchecked integer until MDL078 sees it.",
		SeeAlso: []string{"icon-collection", "navigation.create"},
	})

	// ── Import / Export Mappings ──────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "import-mapping",
		Summary: "Import mappings — map JSON/XML to Mendix entities for inbound data",
		Keywords: []string{
			"import mapping", "create import mapping", "drop import mapping",
			"show import mappings", "describe import mapping",
			"with json structure", "with message definition", "find or create",
			"or create", "or error", "or ignore", "overridable", "object handling backup",
			"parameter", "input object", "mapping parameter",
			"object handling", "converter", "value transform", "custom handling", "by microflow",
		},
		Syntax: "SHOW IMPORT MAPPINGS [IN Module];\nDESCRIBE IMPORT MAPPING Module.Name;\n" +
			"CREATE [OR MODIFY] IMPORT MAPPING Module.Name [FOLDER 'path']\n" +
			"  WITH JSON STRUCTURE Module.JsonStruct [ROOT a/b/c]\n" +
			"    | WITH MESSAGE DEFINITION Module.Collection.Definition\n" +
			"    | WITH XML SCHEMA Module.Schema\n" +
			"  [PARAMETER Module.Entity]   -- the mapping's input object, bound by `P: parameter`\n{\n" +
			"  create Module.Entity | find Module.Entity OR CREATE|ERROR|IGNORE [OVERRIDABLE]\n" +
			"    [by Module.MF(P: parent)] {\n    Attr = jsonField [KEY],\n" +
			"    Attr = a/b/c,\n" +
			"    Assoc/Module.Child = nestedKey { ... }\n  }\n};\nDROP IMPORT MAPPING Module.Name;\n\n" +
			"OR MODIFY: updates mapping in-place, preserves UUID.\n\n" +
			"Nested members (Attr = a/b/c):\n" +
			"  Reaches a leaf several levels down with NO entity for the levels in\n" +
			"  between — the shape Studio Pro produces when you tick a nested leaf\n" +
			"  without ticking its parents. One entity, values from several depths.\n" +
			"  Use Assoc/Module.Child = key { ... } instead when you WANT an entity\n" +
			"  per level. The path may not cross a 0..* element: many items cannot\n" +
			"  collapse into one value, and mxbuild rejects it with CE0256.\n\n" +
			"Custom object handling (find X by Module.MF(...)):\n" +
			"  A microflow resolves the object instead of Create/Find. `by` is a\n" +
			"  modifier on `find` because that is what it means — find the object\n" +
			"  by calling this microflow. Parameter sources: parent (the enclosing\n" +
			"  mapped object), parameter (the mapping's own input object),\n" +
			"  parent(2) (an ancestor N levels up), or a member path (a value from\n" +
			"  the payload). modelsdk engine only.\n\n" +
			"Value transform (Attr = Module.MF(jsonField)):\n" +
			"  The value passes through a microflow on its way to the attribute.\n" +
			"  The stored element carries only the microflow — its input IS the\n" +
			"  member the element binds, which is why the member is named inside\n" +
			"  the call. The export form mirrors it: jsonField = Module.MF(Attr).\n\n" +
			"ROOT a/b/c:\n" +
			"  Starts the mapping at a NESTED schema element rather than the\n" +
			"  structure's root — the shape Studio Pro produces when you pick a\n" +
			"  node deeper in the payload. Written in member names; the path may\n" +
			"  pass through an array, and the mapping is then rooted at the item.\n\n" +
			"Arrays of primitives:\n" +
			"  [\"a\",\"b\"] maps to one entity per string. Write it like any other\n" +
			"  array — Assoc/Module.Entity = tags { ... } — and bind the primitive\n" +
			"  to the reserved member Value. The wrapper level Mendix stores is\n" +
			"  generated, the same way an array's item level is.\n\n" +
			"Sources:\n" +
			"  A JSON structure is built from a payload sample; a MESSAGE DEFINITION\n" +
			"  is derived from the domain model and names entities and attributes\n" +
			"  itself, so its members are the definition's exposed names and the\n" +
			"  reference is THREE parts (the definitions live inside a collection\n" +
			"  document). Message definitions are read-only: map over one that\n" +
			"  already exists. An array-rooted structure needs no special syntax —\n" +
			"  the root is taken from the structure.\n\n" +
			"Inherited attributes:\n" +
			"  An entity mapped with EXTENDS can map its inherited attributes too —\n" +
			"  name them exactly like its own. mxcli resolves each to the entity that\n" +
			"  declares it, which is what Studio Pro needs to show the field mapped.",
		Example: "CREATE IMPORT MAPPING Shop.IMM_Order\n  WITH JSON STRUCTURE Shop.JSON_Order\n{\n  create Shop.Order {\n    OrderId = orderId KEY,\n    TotalAmount = total,\n    -- a leaf two levels down, without entities for customer/contact\n    Email = customer/contact/email\n  }\n};\n\n-- Idempotent update\nCREATE OR MODIFY IMPORT MAPPING Shop.IMM_Order\n  WITH JSON STRUCTURE Shop.JSON_Order\n{\n  find or create Shop.Order {\n    OrderId = orderId KEY,\n    TotalAmount = total,\n    Status = status\n  }\n};",
		SeeAlso: []string{"export-mapping", "json-structure"},
	})

	Register(SyntaxFeature{
		Path:    "export-mapping",
		Summary: "Export mappings — map Mendix entities to JSON/XML for outbound data",
		Keywords: []string{
			"export mapping", "create export mapping", "drop export mapping",
			"show export mappings", "describe export mapping",
			"with json structure", "with message definition", "null values",
			"as jsonKey", "converter", "value transform",
		},
		Syntax: "SHOW EXPORT MAPPINGS [IN Module];\nDESCRIBE EXPORT MAPPING Module.Name;\nCREATE [OR MODIFY] EXPORT MAPPING Module.Name [FOLDER 'path']\n  WITH JSON STRUCTURE Module.JsonStruct [ROOT a/b/c]\n    | WITH MESSAGE DEFINITION Module.Collection.Definition\n    | WITH XML SCHEMA Module.Schema\n  [NULL VALUES LeaveOutElement|SendAsNil]\n{\n  Module.Entity {\n    jsonField = Attr,\n    jsonField = Module.MF(Attr),          -- value transform\n    group AS key { ... },                 -- entity-less grouping node\n    Assoc/Module.Child AS nestedKey { ... }\n  }\n};\nDROP EXPORT MAPPING Module.Name;\n\nOR MODIFY: updates mapping in-place, preserves UUID.\n\n" +
			"Grouping nodes (group as key { ... }):\n" +
			"  A JSON object with no Mendix object behind it — Studio Pro's\n" +
			"  entity-less object element. It may hold OBJECT elements only: a\n" +
			"  value there has no entity to bind its attribute to, and Mendix\n" +
			"  reports CE0061. An ARRAY needs no such wrapper — write\n" +
			"  Assoc/Entity AS items { values } and the container is generated.\n\n" +
			"No nested-member form:\n" +
			"  An import mapping can write `Attr = a/b/c` to reach a leaf without an\n" +
			"  entity per level. An export mapping cannot: it has to PRODUCE the\n" +
			"  intermediate node, so Mendix rejects a collapsed member with CE5015\n" +
			"  (\"no child mapping matching schema element\"). Give the level its own\n" +
			"  element: Assoc/Module.Child AS key { ... }.",
		Example: "CREATE EXPORT MAPPING Shop.EMM_Order\n  WITH JSON STRUCTURE Shop.JSON_Order\n  NULL VALUES LeaveOutElement\n{\n  Shop.Order {\n    orderId = OrderId,\n    total = TotalAmount\n  }\n};\n\n-- Idempotent update\nCREATE OR MODIFY EXPORT MAPPING Shop.EMM_Order\n  WITH JSON STRUCTURE Shop.JSON_Order\n{\n  Shop.Order {\n    orderId = OrderId,\n    total = TotalAmount,\n    status = Status\n  }\n};",
		SeeAlso: []string{"import-mapping", "json-structure"},
	})

	// ── Data Transformers ─────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "data-transformer",
		Summary: "Data transformers — JSLT/XSLT document transformations (Mendix 11.9+)",
		Keywords: []string{
			"data transformer", "create data transformer", "drop data transformer",
			"list data transformers", "jslt", "xslt", "transform",
		},
		Syntax:  "LIST DATA TRANSFORMERS [IN Module];\nDESCRIBE DATA TRANSFORMER Module.Name;\nCREATE [OR MODIFY] DATA TRANSFORMER Module.Name [FOLDER 'path']\n  SOURCE JSON '{ ... }'\n{\n  JSLT 'single-line-expression';\n  -- or multi-line:\n  JSLT $$\n{ ... }\n  $$;\n};\nDROP DATA TRANSFORMER Module.Name;\n\nOR MODIFY: updates transformer in-place, preserves UUID.",
		Example: "CREATE DATA TRANSFORMER ETL.FlattenOrder\n  SOURCE JSON '{\"order\": {\"id\": 1, \"total\": 99.0}}'\n{\n  JSLT '{\"id\": .order.id, \"total\": .order.total}';\n};\n\n-- Multi-line JSLT\nCREATE OR MODIFY DATA TRANSFORMER ETL.WeatherSummary\n  SOURCE JSON '{\"current\": {\"temp\": 12.8, \"wind\": 18.3}}'\n{\n  JSLT $$\n{\n  \"temperature\": .current.temp,\n  \"wind_speed\":  .current.wind\n}\n  $$;\n};",
		SeeAlso: []string{"integration"},
	})

	// ── AI Agents ──────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "agents",
		Summary: "AI agent documents — Model, Knowledge Base, Consumed MCP Service, Agent (requires AgentEditorCommons, Mendix 11.9+)",
		Keywords: []string{
			"agent", "agents", "model", "knowledge base", "mcp service",
			"agent editor", "llm", "ai", "genai", "mxcloudgenai",
		},
		Syntax:  "LIST MODELS [IN Module];\nLIST KNOWLEDGE BASES [IN Module];\nLIST CONSUMED MCP SERVICES [IN Module];\nLIST AGENTS [IN Module];\nDESCRIBE MODEL Module.Name;\nCREATE MODEL Module.Name (Provider: MxCloudGenAI, Key: Module.ApiKey);\nCREATE KNOWLEDGE BASE Module.Name (Provider: MxCloudGenAI, Key: Module.KBKey);\nCREATE CONSUMED MCP SERVICE Module.Name (ProtocolVersion: v2025_03_26, ...);\nCREATE AGENT Module.Name (UsageType: Task|Chat, Model: Module.MyModel, SystemPrompt: '...') { ... };\nDROP AGENT Module.Name;",
		Example: "CREATE MODEL MyModule.GPT4 (\n  Provider: MxCloudGenAI,\n  Key: MyModule.ModelApiKey\n);\n\nCREATE AGENT MyModule.Summarizer (\n  UsageType: Task,\n  Model: MyModule.GPT4,\n  SystemPrompt: 'Summarize in 3 sentences.',\n  UserPrompt: 'Enter text.'\n);",
		SeeAlso: []string{"agents.model", "agents.knowledge-base", "agents.mcp-service", "agents.agent"},
	})

	Register(SyntaxFeature{
		Path:     "agents.model",
		Summary:  "CREATE/DROP MODEL documents for AI agents",
		Keywords: []string{"create model", "drop model", "describe model", "list models", "provider", "mxcloudgenai"},
		Syntax:   "CREATE [OR MODIFY] MODEL Module.Name [FOLDER 'path'] (\n  Provider: MxCloudGenAI,\n  Key: Module.ApiKeyConstant\n);\nDESCRIBE MODEL Module.Name;\nLIST MODELS [IN Module];\nDROP MODEL Module.Name;",
		Example:  "create model MyModule.GPT4 (\n  Provider: MxCloudGenAI,\n  Key: MyModule.ModelApiKey\n);",
		SeeAlso:  []string{"agents"},
	})

	Register(SyntaxFeature{
		Path:     "agents.knowledge-base",
		Summary:  "CREATE/DROP KNOWLEDGE BASE documents for AI agents",
		Keywords: []string{"create knowledge base", "drop knowledge base", "knowledge base", "kb", "rag"},
		Syntax:   "CREATE [OR MODIFY] KNOWLEDGE BASE Module.Name [FOLDER 'path'] (\n  Provider: MxCloudGenAI,\n  Key: Module.KBApiKeyConstant\n);\nDESCRIBE KNOWLEDGE BASE Module.Name;\nLIST KNOWLEDGE BASES [IN Module];\nDROP KNOWLEDGE BASE Module.Name;",
		Example:  "create knowledge base MyModule.ProductDocs (\n  Provider: MxCloudGenAI,\n  Key: MyModule.KBApiKey\n);",
		SeeAlso:  []string{"agents"},
	})

	Register(SyntaxFeature{
		Path:     "agents.mcp-service",
		Summary:  "CREATE/DROP CONSUMED MCP SERVICE documents for AI agents",
		Keywords: []string{"consumed mcp service", "mcp", "mcp service", "protocol version"},
		Syntax:   "CREATE [OR MODIFY] CONSUMED MCP SERVICE Module.Name [FOLDER 'path'] (\n  ProtocolVersion: v2025_03_26,\n  Version: '1.0',\n  ConnectionTimeoutSeconds: 30,\n  Documentation: 'description'\n);\nDESCRIBE CONSUMED MCP SERVICE Module.Name;\nLIST CONSUMED MCP SERVICES [IN Module];\nDROP CONSUMED MCP SERVICE Module.Name;",
		Example:  "create consumed mcp service MyModule.WebSearch (\n  ProtocolVersion: v2025_03_26,\n  Version: '1.0',\n  ConnectionTimeoutSeconds: 30\n);",
		SeeAlso:  []string{"agents"},
	})

	Register(SyntaxFeature{
		Path:    "agents.agent",
		Summary: "CREATE/DROP AGENT documents with variables, tools, KB tools, and MCP service tools",
		Keywords: []string{
			"create agent", "drop agent", "usagetype", "systemprompt", "userprompt",
			"variables", "toolchoice", "temperature", "topp", "maxtokens",
		},
		Syntax: `CREATE [OR MODIFY] AGENT Module.Name [FOLDER 'path'] (
  UsageType: Task|Chat,
  Model: Module.MyModel,
  [Description: 'text',]
  [MaxTokens: N,]
  [Temperature: 0.7,]
  [TopP: 0.9,]
  [ToolChoice: Auto|None|Required,]
  [Variables: ("Key": EntityAttribute|String),]
  SystemPrompt: 'prompt or $$multi-line$$',
  [UserPrompt: 'prompt']
)
{
  [MCP SERVICE Module.ServiceName { Enabled: true }]
  [KNOWLEDGE BASE AliaName { Source: Module.KB, Collection: 'col', MaxResults: 5, Enabled: true }]
  [TOOL MicroflowName { Description: 'desc', Enabled: true }]
};`,
		Example: "create agent MyModule.Assistant (\n  UsageType: Chat,\n  Model: MyModule.GPT4,\n  SystemPrompt: $$You are a helpful assistant.$$,\n  UserPrompt: 'Ask me anything.'\n)\n{\n  MCP SERVICE MyModule.WebSearch { Enabled: true }\n};",
		SeeAlso: []string{"agents", "agents.model", "agents.knowledge-base", "agents.mcp-service"},
	})
}
