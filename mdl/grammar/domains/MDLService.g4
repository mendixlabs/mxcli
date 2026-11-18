/**
 * MDL Service Grammar — database connections, REST clients, published REST services,
 * OData clients/services, business event services, navigation, configuration.
 */
parser grammar MDLService;

options { tokenVocab = MDLLexer; }

// =============================================================================
// DATABASE / REST CLIENT
// =============================================================================

// R2 (ako/mxcli#754): a declarative document has its properties in ( ) and
// its children in { }. The database connection was the one declarative
// document written as clauses with a begin … end block of queries:
//
//   create database connection M.Db (
//     Type: 'PostgreSQL', ConnectionString: @M.Url, Username: @M.User, Password: @M.Pass,
//   ) {
//     query GetCustomers (
//       Sql: $$select id, name from customer where id > {minId}$$,
//       Parameters: ( minId: Integer default '0' ),
//       Returns: M.Customer,
//       Map: ( CustomerId = id, Name = name ),
//     )
//   };
//
// A query's column map binds an attribute to a column, `Attr = column`, the way
// a REST mapping binds `Attr = jsonField` (R3: `=` for a mapping side). The
// clause form is the old spelling (MDL-DEPR127), and builds the same statement.
createDatabaseConnectionStatement
    : DATABASE CONNECTION ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN databaseConnectionProp (COMMA databaseConnectionProp)* COMMA? RPAREN
      (LBRACE databaseQueryDef* RBRACE)?
    | DATABASE CONNECTION ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      databaseConnectionOption+ /* @alias MDL-DEPR127 */
      (BEGIN databaseQuery* END)?
    ;

databaseConnectionProp
    : identifierOrKeyword COLON (STRING_LITERAL | NUMBER_LITERAL | AT qualifiedName)
    ;

databaseQueryDef
    : QUERY identifierOrKeyword LPAREN databaseQueryProp (COMMA databaseQueryProp)* COMMA? RPAREN
    ;

databaseQueryProp
    : identifierOrKeyword COLON (STRING_LITERAL | DOLLAR_STRING)                     // Sql: $$…$$
    | identifierOrKeyword COLON LPAREN
      databaseQueryParam (COMMA databaseQueryParam)* COMMA? RPAREN                  // Parameters: ( p: Integer default '0' )
    | identifierOrKeyword COLON LPAREN
      databaseQueryColumn (COMMA databaseQueryColumn)* COMMA? RPAREN                // Map: ( Attr = column )
    | identifierOrKeyword COLON qualifiedName                                       // Returns: M.Entity
    ;

databaseQueryParam
    : identifierOrKeyword COLON dataType (DEFAULT STRING_LITERAL | NULL)?
    ;

databaseQueryColumn
    : identifierOrKeyword EQUALS identifierOrKeyword
    ;

databaseConnectionOption
    : TYPE STRING_LITERAL
    | CONNECTION STRING_TYPE (STRING_LITERAL | AT qualifiedName)
    | HOST STRING_LITERAL
    | PORT NUMBER_LITERAL
    | DATABASE STRING_LITERAL
    | USERNAME (STRING_LITERAL | AT qualifiedName)
    | PASSWORD (STRING_LITERAL | AT qualifiedName)
    ;

databaseQuery
    : QUERY identifierOrKeyword
      SQL (STRING_LITERAL | DOLLAR_STRING)
      (PARAMETER identifierOrKeyword COLON dataType (DEFAULT STRING_LITERAL | NULL)?)*
      (RETURNS qualifiedName
        (MAP LPAREN databaseQueryMapping (COMMA databaseQueryMapping)* RPAREN)?
      )?
      SEMICOLON
    ;

databaseQueryMapping
    : identifierOrKeyword AS identifierOrKeyword
    ;

createConfigurationStatement
    : CONFIGURATION ifNotExists? STRING_LITERAL settingsItemOptions?          // configuration 'X' ( Key: value, … )
    | CONFIGURATION ifNotExists? STRING_LITERAL
      settingsAssignment (COMMA settingsAssignment)*             // old spelling: Key = value, … (MDL-DEPR060)
    ;

/**
 * CREATE CONSUMED REST SERVICE — property-based syntax with { } blocks.
 */
// R9: the folder is a clause after the name. `Folder: '…'` in the property
// list is a registered alias /* @alias MDL-DEPR105 */, here and in the
// published REST and OData property lists; the visitor reports it by key.
createRestClientStatement
    : consumedRestServiceKw ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN restClientProperty (COMMA restClientProperty)* RPAREN
      (LBRACE restClientOperation* RBRACE)?
    ;

/**
 * ALTER CONSUMED REST SERVICE — the operation body is CREATE's; MODIFY writes
 * only the keys it names and DROP removes one operation, so every operation the
 * statement does not name stays as stored, including one made in Studio Pro.
 *
 * @example
 * ```mdl
 * alter consumed rest service Shop.BookingApi
 *   add operation GetBooking (Method: get, Path: '/bookings/{id}', Parameters: ($id: String), Response: json),
 *   modify operation PostBooking (Timeout: 60),
 *   drop operation OldBooking,
 *   set (BaseUrl: 'https://api.example.com/v2');
 * ```
 */
alterConsumedRestServiceAction
    : SET LPAREN restClientProperty (COMMA restClientProperty)* COMMA? RPAREN
    | ADD restClientOperation
    | MODIFY restClientOperation
    | DROP OPERATION (identifierOrKeyword | STRING_LITERAL)
    ;

restClientProperty
    : identifierOrKeyword COLON STRING_LITERAL                       // BaseUrl: '...', Username: '...'
    | identifierOrKeyword COLON VARIABLE /* @alias MDL-DEPR083 */    // Username: $Constant (a constant of the client's own module)
    | identifierOrKeyword COLON AT qualifiedName                     // Username: @Module.Constant (the one constant reference, R5)
    | identifierOrKeyword COLON NONE                                 // Authentication: NONE
    | identifierOrKeyword COLON BASIC LPAREN restClientProperty (COMMA restClientProperty)* RPAREN
    ;

// An operation is a child of the service, so its properties are in ( ) like
// every other child's (R2, ako/mxcli#754). The brace form is the old spelling.
restClientOperation
    : docComment?
      OPERATION (identifierOrKeyword | STRING_LITERAL)
      ( LPAREN restClientOpProp (COMMA restClientOpProp)* COMMA? RPAREN
      | LBRACE /* @alias MDL-DEPR070 */ restClientOpProp (COMMA restClientOpProp)* COMMA? RBRACE
      )
    ;

restClientOpProp
    : identifierOrKeyword COLON restHttpMethod                       // Method: GET
    | identifierOrKeyword COLON STRING_LITERAL                       // Path: '/items'
    | identifierOrKeyword COLON NUMBER_LITERAL                       // Timeout: 30
    | identifierOrKeyword COLON NONE                                 // Response: NONE
    | identifierOrKeyword COLON LPAREN restClientParamItem (COMMA restClientParamItem)* RPAREN  // Parameters/Query
    | identifierOrKeyword COLON LPAREN restClientHeaderItem (COMMA restClientHeaderItem)* RPAREN  // Headers
    // Body: JSON FROM $v, Response: JSON AS $v. A response's variable is optional:
    // an operation stores none (the call activity names it), so describe prints
    // `Response: json` and that has to run again (mendixlabs/mxcli#1608).
    | identifierOrKeyword COLON (JSON | FILE_KW | STRING_TYPE | STATUS) ((FROM | AS) VARIABLE)?
    | identifierOrKeyword COLON TEMPLATE STRING_LITERAL              // Body: TEMPLATE '...'
    | identifierOrKeyword COLON MAPPING qualifiedName (LBRACE restClientMappingEntry* RBRACE)?  // Body/Response: MAPPING Entity { ... }
    ;

restClientParamItem
    : VARIABLE COLON dataType
    ;

// A header list is a map, so it is `( 'Name': value, … )` like every other
// property map (R2/R3, ako/mxcli#754); `'Name' = value` is the old spelling.
// A header value is a template: `{Name}` is replaced by the operation
// parameter Name, as in the path (ako/mxcli#707). `'Bearer ' + $Token` and
// `$Token` are the old spellings of `'Bearer {Token}'` and `'{Token}'`; they
// used to store only the text before the `+`.
restClientHeaderItem
    : STRING_LITERAL (COLON | EQUALS /* @alias MDL-DEPR120 */) STRING_LITERAL
    | STRING_LITERAL (COLON | EQUALS /* @alias MDL-DEPR120 */) /* @alias MDL-DEPR711 */ VARIABLE
    | STRING_LITERAL (COLON | EQUALS /* @alias MDL-DEPR120 */) /* @alias MDL-DEPR711 */ STRING_LITERAL PLUS VARIABLE
    ;

restClientMappingEntry
    : identifierOrKeyword EQUALS identifierOrKeyword COMMA?                          // Attr = jsonField,
    | CREATE? qualifiedName SLASH qualifiedName EQUALS identifierOrKeyword
      (LBRACE restClientMappingEntry* RBRACE)? COMMA?                                // CREATE Assoc/Entity = jsonField { ... },
    ;

restHttpMethod
    : GET | POST | PUT | PATCH | DELETE
    ;

// =============================================================================
// PUBLISHED REST SERVICE CREATION
// =============================================================================

createPublishedRestServiceStatement
    : PUBLISHED REST SERVICE ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN publishedRestProperty (COMMA publishedRestProperty)* RPAREN
      LBRACE publishedRestResource* RBRACE
    ;

// `Authentication: none | ( method, … )` is the one non-string value: the
// methods in the order written, which is the order Studio Pro stores them
// (mendixlabs/mxcli#1331). The visitor keys on the shape, not the key, so a
// string `Authentication: 'basic'` is reported as misshapen, not dropped.
publishedRestProperty
    : identifierOrKeyword COLON STRING_LITERAL
    | identifierOrKeyword COLON NONE
    | identifierOrKeyword COLON LPAREN publishedRestAuthMethod (COMMA publishedRestAuthMethod)* COMMA? RPAREN
    ;

publishedRestAuthMethod
    : BASIC
    | SESSION
    | MICROFLOW qualifiedName
      (LPAREN (publishedRestAuthParameter (COMMA publishedRestAuthParameter)* COMMA?)? RPAREN)?
    ;

/** A service-level authentication parameter (mendixlabs/mxcli#1417):
 *  `$Token: header 'X-Api-Token' description '...'`. The kind is any word so
 *  the executor can name a wrong one; only query and header are accepted. */
publishedRestAuthParameter
    : VARIABLE COLON identifierOrKeyword STRING_LITERAL? (DESCRIPTION STRING_LITERAL)?
    ;

publishedRestResource
    : RESOURCE STRING_LITERAL LBRACE publishedRestOperation* RBRACE
    ;

publishedRestOperation
    : restHttpMethod publishedRestOpPath?
      MICROFLOW qualifiedName
      (DEPRECATED)?
      (IMPORT MAPPING qualifiedName)?
      (EXPORT MAPPING qualifiedName)?
      (COMMIT identifierOrKeyword)?
      SEMICOLON?
    ;

publishedRestOpPath
    : STRING_LITERAL
    | SLASH
    ;

// =============================================================================
// ODATA CLIENT / SERVICE
// =============================================================================

// R10 (ADR-0010): a document type is named as Studio Pro names it. Each rule
// below is the one place its name is spelt, the canonical form first; the old
// mxcli name is a registered deprecated alias (mdl/deprecation) that the
// visitor reports and `fmt --upgrade` rewrites.
consumedRestServiceKw
    : CONSUMED REST SERVICE
    | REST CLIENT /* @alias MDL-DEPR550 */
    ;

consumedRestServicesKw
    : CONSUMED REST SERVICES
    | REST CLIENTS /* @alias MDL-DEPR550 */
    ;

consumedODataServiceKw
    : CONSUMED ODATA SERVICE
    | ODATA CLIENT /* @alias MDL-DEPR551 */
    ;

consumedODataServicesKw
    : CONSUMED ODATA SERVICES
    | ODATA CLIENTS /* @alias MDL-DEPR551 */
    ;

publishedODataServiceKw
    : PUBLISHED ODATA SERVICE
    | ODATA SERVICE /* @alias MDL-DEPR552 */
    ;

publishedODataServicesKw
    : PUBLISHED ODATA SERVICES
    | ODATA SERVICES /* @alias MDL-DEPR552 */
    ;

taskQueueKw
    : TASK QUEUE
    | QUEUE /* @alias MDL-DEPR553 */
    ;

taskQueuesKw
    : TASK QUEUES
    | QUEUES /* @alias MDL-DEPR553 */
    ;

createODataClientStatement
    : consumedODataServiceKw ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN odataPropertyAssignment (COMMA odataPropertyAssignment)* RPAREN
      odataHeadersClause?
    ;

createODataServiceStatement
    : publishedODataServiceKw ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN odataPropertyAssignment (COMMA odataPropertyAssignment)* RPAREN
      odataAuthenticationClause? /* @alias MDL-DEPR139 */
      (LBRACE (publishEntityBlock | publishMicroflowBlock)* RBRACE)?
    ;

odataPropertyValue
    : STRING_LITERAL
    // A $$…$$ block, for a value too long or too quote-laden for one literal:
    // a consumed business event service's AsyncAPI `Contract:` (ako/mxcli#2384
    // §5.1). The grammar accepts it on any key here; which keys may actually
    // take it is the strict-property schema's business (textShapes), so an
    // OData key still reports "expected a string".
    | DOLLAR_STRING
    | NUMBER_LITERAL
    | TRUE
    | FALSE
    | MICROFLOW qualifiedName?
    | AT qualifiedName              // @Module.ConstantName (Mendix constant reference — required for ServiceUrl)
    | qualifiedName
    ;

// A Mendix expression is accepted after the plain value forms, so `'admin'`,
// `@Mod.Const`, `microflow Mod.F` and `OData4` keep their parse and only what
// those reject (`'Bearer ' + @Mod.Token`) reaches it. The visitor stores the
// source text for the expression-typed properties (HttpUsername, HttpPassword,
// ClientCertificate, header values) and refuses an expression anywhere else,
// so a plain-value property cannot silently read it as empty
// (PROPOSAL_first_class_expressions.md §6.4).
// `Authentication: none | ( method, … )` is a published service's
// authentication (R9); the two forms come before `expression`, which would
// otherwise read `(session)` as a parenthesised name. The trailing
// `authentication …` clause is the old spelling (MDL-DEPR139).
odataPropertyAssignment
    : identifierOrKeyword COLON odataPropertyValue
    | identifierOrKeyword COLON NONE
    | identifierOrKeyword COLON LPAREN odataAuthType (COMMA odataAuthType)* COMMA? RPAREN
    | identifierOrKeyword COLON expression
    ;

// ALTER … SET ( Key: value, … ): exactly create's property list (R3).
odataAlterPropertyList
    : LPAREN odataPropertyAssignment (COMMA odataPropertyAssignment)* RPAREN
    ;

// The old spelling of the alter list: `set Key = value, …` (R3).
odataAlterAssignment
    : identifierOrKeyword EQUALS /* @alias MDL-DEPR061 */ odataPropertyValue
    | identifierOrKeyword EQUALS /* @alias MDL-DEPR061 */ expression
    ;

odataAuthenticationClause
    : AUTHENTICATION odataAuthType (COMMA odataAuthType)*
    ;

odataAuthType
    : BASIC
    | SESSION
    | GUEST
    | MICROFLOW qualifiedName?
    | IDENTIFIER  // For custom types like 'Custom'
    ;

publishEntityBlock
    : PUBLISH ENTITY qualifiedName (AS STRING_LITERAL)?
      (LPAREN odataPropertyAssignment (COMMA odataPropertyAssignment)* RPAREN)?
      exposeClause?
      SEMICOLON?
    ;

// An OData action/function: a published microflow. Mendix exposes it in
// $metadata as an ActionImport, so a client can POST arguments to it rather
// than reading them back as echoed columns of an entity set.
//
// Parameter DataTypes and the return type are NOT restated here — they are read
// off the microflow, which already declares them. Restating would let the two
// drift, and Studio Pro derives them the same way.
publishMicroflowBlock
    : PUBLISH MICROFLOW qualifiedName (AS STRING_LITERAL)?
      exposeClause?
      SEMICOLON?
    ;

exposeClause
    : EXPOSE LPAREN (STAR | exposeMember (COMMA exposeMember)*) RPAREN
    ;

exposeMember
    : identifierOrKeyword (AS STRING_LITERAL)? exposeMemberOptions?
    ;

exposeMemberOptions
    : LPAREN identifierOrKeyword (COMMA identifierOrKeyword)* RPAREN
    ;

createExternalEntityStatement
    : EXTERNAL ENTITY ifNotExists? qualifiedName
      FROM consumedODataServiceKw qualifiedName
      LPAREN odataPropertyAssignment (COMMA odataPropertyAssignment)* RPAREN
      (LPAREN attributeDefinitionList? RPAREN)?
    ;

createExternalEntitiesStatement
    : EXTERNAL ENTITIES FROM qualifiedName
      (INTO (qualifiedName | IDENTIFIER))?
      (ENTITIES LPAREN identifierOrKeyword (COMMA identifierOrKeyword)* RPAREN)?
    ;

createNavigationStatement
    : NAVIGATION (qualifiedName | IDENTIFIER) navigationClause*
    ;

odataHeadersClause
    : HEADERS LPAREN odataHeaderEntry (COMMA odataHeaderEntry)* RPAREN
    ;

odataHeaderEntry
    : STRING_LITERAL COLON odataPropertyValue
    | STRING_LITERAL COLON expression
    ;

// =============================================================================
// BUSINESS EVENT SERVICE
// =============================================================================

// One statement, two documents. A PUBLISHED service declares its own messages
// (`message … publish|subscribe`); a CONSUMED one names a contract in its
// property list and binds events from it (`consume …`). Both shapes are
// accepted here and told apart in the visitor, which refuses a statement that
// mixes them — a parse error could not say which half to drop (#2384 §5.1).
createBusinessEventServiceStatement
    : BUSINESS EVENT SERVICE ifNotExists? qualifiedName
      LPAREN odataPropertyAssignment (COMMA odataPropertyAssignment)* RPAREN
      LBRACE (businessEventMessageDef | businessEventConsumeDef)+ RBRACE
    ;

// `consume <Message> entity … microflow …` binds one event of the contract to
// the entity it is mapped onto and the microflow that handles it. It is spelled
// CONSUME, not SUBSCRIBE: `message X (…) subscribe` already means "this app
// consumes X of its *own* definition", and two spellings writing the same
// ServiceOperation would be unreadable even though R11 keeps them unambiguous.
businessEventConsumeDef
    : CONSUME identifierOrKeyword
      (ENTITY qualifiedName)?
      (MICROFLOW qualifiedName)?
      SEMICOLON
    ;

businessEventMessageDef
    : MESSAGE identifierOrKeyword
      LPAREN businessEventAttrDef (COMMA businessEventAttrDef)* RPAREN
      (PUBLISH | SUBSCRIBE)
      (ENTITY qualifiedName)?
      (MICROFLOW qualifiedName)?
      SEMICOLON
    ;

businessEventAttrDef
    : identifierOrKeyword COLON dataType
    ;
