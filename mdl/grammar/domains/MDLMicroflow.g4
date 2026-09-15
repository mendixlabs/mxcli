/**
 * MDL Microflow Grammar — microflows, nanoflows, Java actions, all microflow
 * body statements, REST call statements, list operations.
 */
parser grammar MDLMicroflow;

options { tokenVocab = MDLLexer; }

// =============================================================================
// MICROFLOW CREATION
// =============================================================================

/**
 * Creates a new microflow with parameters, return type, and activity body.
 */
createMicroflowStatement
    : MICROFLOW qualifiedName
      LPAREN microflowParameterList? RPAREN
      microflowReturnType?
      microflowOptions?
      BEGIN microflowBody END SEMICOLON? SLASH?
    ;

/**
 * Nanoflow creation — mirrors microflow syntax but targets client-side execution.
 */
createNanoflowStatement
    : NANOFLOW qualifiedName
      LPAREN microflowParameterList? RPAREN
      microflowReturnType?
      microflowOptions?
      BEGIN microflowBody END SEMICOLON? SLASH?
    ;

/**
 * Rule creation — Mendix calls a rule "a special kind of microflow" that returns
 * a Boolean or an enumeration and may only be used from a decision. It shares the
 * microflow body, so this mirrors createNanoflowStatement; what a rule may not do
 * is enforced by the validator, not by the grammar, because restricting the body
 * rule here would duplicate it and turn every violation into a parse error with
 * no explanation.
 */
createRuleStatement
    : RULE qualifiedName
      LPAREN microflowParameterList? RPAREN
      microflowReturnType?
      microflowOptions?
      BEGIN microflowBody END SEMICOLON? SLASH?
    ;

/**
 * Java Action creation with inline Java source code.
 */
createJavaActionStatement
    : JAVA ACTION qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN javaActionParameterList? RPAREN
      javaActionReturnType?
      javaActionExposedClause?
      AS DOLLAR_STRING SEMICOLON?
    ;

javaActionParameterList
    : javaActionParameter (COMMA javaActionParameter)*
    ;

javaActionParameter
    : parameterName COLON dataType NOT_NULL?
    ;

javaActionReturnType
    : RETURNS dataType
    ;

// EXPOSED AS 'Caption' IN 'Category'  — appear in Studio Pro's toolbox
// NOT EXPOSED                         — remove the toolbox entry
//
// An ABSENT clause is neither: it preserves whatever is stored. The entry also
// carries four PNG bitmaps (icon and image, each with a dark variant) that MDL
// cannot name, so a rewrite that rebuilt it from the caption and category alone
// destroyed them — and one that treated silence as removal destroyed the entry.
// Removing is therefore something a script has to say out loud.
javaActionExposedClause
    : EXPOSED AS STRING_LITERAL IN STRING_LITERAL exposeBitmapClause*
    | NOT EXPOSED
    ;

// ICON 'i.png' / ICON DARK 'i-dark.png' / IMAGE 'm.png' / IMAGE DARK 'm-dark.png'
//
// The toolbox entry's four bitmaps. Studio Pro wants a 64x64 PNG for the icon
// and a 256x192 PNG for the image; the dark variants are what its "Customize for
// Dark mode" toggle fills in. Paths are read from disk at exec time and resolve
// against the directory of the .mdl file being executed, so a script and its
// artwork travel together.
//
// An omitted bitmap is preserved, not cleared — the same rule as the clause
// itself. Clearing one is DROP ICON / DROP IMAGE.
exposeBitmapClause
    : ICON DARK? STRING_LITERAL
    | IMAGE DARK? STRING_LITERAL
    | DROP ICON DARK?
    | DROP IMAGE DARK?
    ;

/**
 * JavaScript Action creation with inline JavaScript source code. Mirrors the
 * Java action shape, plus an optional `platform` clause (Web/Native/Hybrid/All;
 * defaults to Web). Reuses the javaAction parameter/return/exposed sub-rules.
 */
createJavaScriptActionStatement
    : JAVASCRIPT ACTION qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN javaActionParameterList? RPAREN
      javaActionReturnType?
      javaActionExposedClause?
      javaScriptPlatformClause?
      AS DOLLAR_STRING SEMICOLON?
    ;

javaScriptPlatformClause
    : PLATFORM identifierOrKeyword
    ;

microflowParameterList
    : microflowParameter (COMMA microflowParameter)*
    ;

// Annotations on a parameter: `@position(x, y)` places the parameter box on the
// canvas. Parameters are stored nodes with real geometry, so this is the same
// annotation the statements below it take, attached to the thing it positions.
microflowParameter
    : annotation* (parameterName | VARIABLE) COLON dataType
    ;

// Allow reserved keywords as parameter names (similar to attributeName)
parameterName
    : IDENTIFIER
    | QUOTED_IDENTIFIER                            // Escape any reserved word
    | keyword
    ;

microflowReturnType
    : RETURNS dataType (AS VARIABLE)?
    ;

microflowOptions
    : microflowOption+
    ;

// COMMENT is deliberately absent. It was parsed and dropped — the visitor stored
// it on a field the executor never read — so `comment 'text'` reported success
// and set no documentation at all. The `/** … */` doc comment before the
// statement does work, and is richer (@param, @returns), so there is one
// spelling rather than one that works and one that lies.
microflowOption
    : FOLDER STRING_LITERAL
    | microflowExposedClause
    ;

// EXPOSED AS MICROFLOW ACTION 'Caption' IN 'Category'
// EXPOSED AS WORKFLOW  ACTION 'Caption' IN 'Category'
// NOT EXPOSED AS MICROFLOW ACTION
//
// A microflow can be exposed twice, because Studio Pro has two toolboxes: one
// for the microflow editor and one for the workflow editor. Mendix stores the
// two under different keys (MicroflowActionInfo / WorkflowActionInfo) with the
// same element type, so which one is meant has to be named — unlike a Java or
// JavaScript action, which has only one.
//
// As with those, an ABSENT clause preserves what is stored; NOT EXPOSED is how a
// script asks for the entry to go away.
microflowExposedClause
    : EXPOSED AS (MICROFLOW | WORKFLOW) ACTION STRING_LITERAL IN STRING_LITERAL exposeBitmapClause*
    | NOT EXPOSED AS (MICROFLOW | WORKFLOW) ACTION
    ;

microflowBody
    : microflowStatement*
    ;

/**
 * Body shared by both microflow and nanoflow creation.
 * CALL NANOFLOW is valid in both contexts (microflows can call nanoflows).
 * Nanoflow-specific action restrictions are enforced at the executor level,
 * not at the grammar level.
 */
microflowStatement
    : annotation* declareStatement SEMICOLON
    | annotation* caseStatement SEMICOLON
    | annotation* inheritanceSplitStatement SEMICOLON
    | annotation* castObjectStatement SEMICOLON
    | annotation* createListStatement SEMICOLON       // Must be before createObjectStatement to match "CREATE LIST OF"
    | annotation* createObjectStatement SEMICOLON
    | annotation* changeObjectStatement SEMICOLON
    | annotation* commitStatement SEMICOLON
    | annotation* deleteObjectStatement SEMICOLON
    | annotation* rollbackStatement SEMICOLON
    | annotation* retrieveStatement SEMICOLON
    | annotation* ifStatement SEMICOLON
    | annotation* loopStatement SEMICOLON
    | annotation* whileStatement SEMICOLON
    | annotation* continueStatement SEMICOLON
    | annotation* breakStatement SEMICOLON
    | annotation* mergeStatement SEMICOLON
    | annotation* joinStatement SEMICOLON
    | annotation* returnStatement SEMICOLON
    | annotation* raiseErrorStatement SEMICOLON
    | annotation* logStatement SEMICOLON
    | annotation* callMicroflowStatement SEMICOLON
    | annotation* callNanoflowStatement SEMICOLON
    | annotation* callJavaActionStatement SEMICOLON
    | annotation* callJavaScriptActionStatement SEMICOLON
    | annotation* callWebServiceStatement SEMICOLON
    | annotation* executeDatabaseQueryStatement SEMICOLON
    | annotation* callExternalActionStatement SEMICOLON
    | annotation* showPageStatement SEMICOLON
    | annotation* closePageStatement SEMICOLON
    | annotation* showHomePageStatement SEMICOLON
    | annotation* showMessageStatement SEMICOLON
    | annotation* downloadFileStatement SEMICOLON
    | annotation* synchronizeStatement SEMICOLON
    | annotation* throwStatement SEMICOLON
    | annotation* listOperationStatement SEMICOLON
    | annotation* aggregateListStatement SEMICOLON
    | annotation* addToListStatement SEMICOLON
    | annotation* removeFromListStatement SEMICOLON
    | annotation* validationFeedbackStatement SEMICOLON
    | annotation* restCallStatement SEMICOLON
    | annotation* sendRestRequestStatement SEMICOLON
    | annotation* importFromMappingStatement SEMICOLON
    | annotation* exportToMappingStatement SEMICOLON
    | annotation* transformJsonStatement SEMICOLON
    | annotation* callWorkflowStatement SEMICOLON
    | annotation* getWorkflowDataStatement SEMICOLON
    | annotation* getWorkflowsStatement SEMICOLON
    | annotation* getWorkflowActivityRecordsStatement SEMICOLON
    | annotation* workflowOperationStatement SEMICOLON
    | annotation* setTaskOutcomeStatement SEMICOLON
    | annotation* openUserTaskStatement SEMICOLON
    | annotation* notifyWorkflowStatement SEMICOLON
    | annotation* openWorkflowStatement SEMICOLON
    | annotation* lockWorkflowStatement SEMICOLON
    | annotation* unlockWorkflowStatement SEMICOLON
    // LAST on purpose. Since SET became optional, `$X = <expr>` overlaps every
    // `VARIABLE EQUALS <function-call>` statement above — aggregates, list
    // operations, RANGE. Those rules must keep winning: a lower-numbered
    // setStatement swallowed `$Sum = sum($List.Price)` into a Change Variable
    // whose fallback conversion drops the attribute, which mxbuild rejects
    // (CE0015 / CE0109). Last means it only claims what nothing else parses.
    | annotation* setStatement SEMICOLON
    ;

declareStatement
    : DECLARE VARIABLE dataType (EQUALS expression)? onErrorClause?
    ;

caseStatement
    : CASE enumSplitSource
      (WHEN enumSplitCaseValue (COMMA enumSplitCaseValue)* THEN microflowBody)+
      (ELSE microflowBody)?
      END CASE
    ;

enumSplitSource
    : attributePath
    | VARIABLE
    ;

enumSplitCaseValue
    : identifierOrKeyword
    | LPAREN EMPTY RPAREN
    ;

// Branch keyword is `when ... then`, matching caseStatement above and the
// caseExpression in MDLSettings.g4 — `case` used to introduce a BRANCH here
// while introducing the SUBJECT in those two, so the word meant two things
// (mxcli #913). The `case`/`else` spelling still parses: scripts in the wild
// use it, and both spellings build the identical flow. MDL065 warns.
inheritanceSplitStatement
    : SPLIT TYPE VARIABLE
      (inheritanceSplitCase+ inheritanceSplitElse? END SPLIT)?
    ;

inheritanceSplitCase
    : CASE qualifiedName microflowBody       // legacy spelling, warns MDL065
    | WHEN qualifiedName THEN microflowBody
    ;

// The last branch is Mendix's `(empty)` outgoing flow — taken when the object
// is NULL, not when no case matched. `else` is the legacy spelling and reads
// as a default, which it is not: mxbuild still demands a flow for every
// subtype and for the base entity (CE0090) when an `else` is present.
inheritanceSplitElse
    : ELSE microflowBody                          // legacy spelling, warns MDL065
    | WHEN LPAREN EMPTY RPAREN THEN microflowBody
    ;

castObjectStatement
    : CAST VARIABLE
    | VARIABLE EQUALS CAST VARIABLE
    ;

// SET is optional: `$Total = 5;` is what everyone writes, and every other
// assignment form in MDL already works bare (`$X = HEAD($List)`,
// `$X = execute database query …`). Requiring the keyword only here made the
// rule unguessable — and the parse error named the token, not the missing
// keyword (mxcli-formula1 findings #13).
setStatement
    : SET? (VARIABLE | attributePath) EQUALS expression onErrorClause?
    ;

// $NewProduct = CREATE MfTest.Product (Name = $Name, Code = $Code);
// $NewProduct = CREATE MfTest.Product (Name = $Name) COMMIT;
// $NewProduct = CREATE MfTest.Product (Name = $Name) REFRESH;
createObjectStatement
    : (VARIABLE EQUALS)? CREATE nonListDataType (LPAREN memberAssignmentList? RPAREN)? commitClause? REFRESH? onErrorClause?
    ;

// CHANGE $Product (Name = $NewName, ModifiedDate = [%CurrentDateTime%]);
// CHANGE $Product (Name = $NewName) COMMIT WITHOUT EVENTS REFRESH;
changeObjectStatement
    : CHANGE VARIABLE (LPAREN memberAssignmentList? RPAREN)? commitClause? REFRESH? onErrorClause?
    ;

// The Commit flag on a create/change activity: Mendix's Microflows$Commit enum.
// Absent = No (the default, so it is omitted from DESCRIBE output); COMMIT = Yes;
// COMMIT WITHOUT EVENTS = YesWithoutEvents.
//
// This is a modifier, NOT the standalone `COMMIT $Var` activity (commitStatement).
// The two are told apart by what follows: the activity always names a variable.
// Because a body statement must be terminated (see microflowStatement), a stray
// `commit $Other` on the next line cannot be absorbed into this clause.
commitClause
    : COMMIT (WITHOUT EVENTS)?
    ;

// Shared by SET, LOOP, aggregate expressions, and validation feedback targets.
attributePath
    : VARIABLE ((SLASH | DOT) qualifiedName)+
    ;

// COMMIT $Product; or COMMIT $Product WITHOUT EVENTS; or COMMIT $Product REFRESH;
//
// Absent = WITH EVENTS, which is Mendix's default for the Commit activity and
// what Studio Pro stores (measured: CommitActivity.DefaultCommit in ako/TestApp
// stores WithEvents=true). WITH EVENTS therefore parses and means exactly the
// same as writing nothing — it is kept because it is what every script written
// before #895 says, and because spelling the default out is not an error.
// WITHOUT EVENTS is the only form that changes the stored value.
commitStatement
    : COMMIT VARIABLE ((WITH | WITHOUT) EVENTS)? REFRESH? onErrorClause?
    ;

// DELETE $Product; or DELETE $Product REFRESH;
deleteObjectStatement
    : DELETE VARIABLE REFRESH? onErrorClause?
    ;

// ROLLBACK $Product; or ROLLBACK $Product REFRESH;
rollbackStatement
    : ROLLBACK VARIABLE REFRESH?
    ;

// RETRIEVE $ProductList FROM MfTest.Product WHERE Code = $SearchCode SORT BY Name ASC LIMIT 1;
retrieveStatement
    : RETRIEVE VARIABLE FROM retrieveSource
      (WHERE (xpathConstraint (andOrXpath? xpathConstraint)* | expression))?
      (SORT_BY sortColumn (COMMA sortColumn)*)?
      (LIMIT limitExpr=expression)?
      (OFFSET offsetExpr=expression)?
      onErrorClause?
    ;

retrieveSource
    : qualifiedName                          // Database retrieve: Module.Entity
    | VARIABLE SLASH qualifiedName           // Association retrieve: $Parent/Module.Assoc
    | LPAREN oqlQuery RPAREN                 // OQL retrieve
    | DATABASE STRING_LITERAL                // External DB
    ;

// ON ERROR clause for microflow error handling
onErrorClause
    : ON ERROR CONTINUE                                    // Ignore error, continue
    | ON ERROR ROLLBACK                                    // Rollback and abort (default)
    | ON ERROR LBRACE microflowBody RBRACE                 // Custom error handler with rollback
    | ON ERROR WITHOUT ROLLBACK LBRACE microflowBody RBRACE // Custom error handler without rollback
    ;

// IF ... THEN ... END IF;
ifStatement
    : IF expression THEN microflowBody
      (ELSIF expression THEN microflowBody)*
      (ELSE microflowBody)?
      END IF
    ;

// LOOP $Product IN $ProductList BEGIN ... END LOOP;
loopStatement
    : LOOP VARIABLE IN (VARIABLE | attributePath)
      BEGIN microflowBody END LOOP
    ;

whileStatement
    : WHILE expression
      BEGIN? microflowBody END WHILE?
    ;

continueStatement
    : CONTINUE
    ;

breakStatement
    : BREAK
    ;

/**
 * `merge <label>` declares an ExclusiveMerge that paths can `join`.
 *
 * The label exists only in MDL — a Mendix ExclusiveMerge stores no name — so it
 * is resolved at build time and at describe time, never written to the model.
 * Forward and backward references both resolve, which is what makes a retry
 * loop (`merge attempt; … join attempt;`) expressible.
 */
mergeStatement
    : MERGE (IDENTIFIER | QUOTED_IDENTIFIER)
    ;

/** `join <label>` sends this path to the merge declared under that label. */
joinStatement
    : JOIN (IDENTIFIER | QUOTED_IDENTIFIER)
    ;

returnStatement
    : RETURN expression?
    ;

raiseErrorStatement
    : RAISE ERROR
    ;

// LOG INFO NODE 'TEST' 'Message'; or LOG INFO 'Message'; or LOG WARNING 'Message' WITH ({1} = $var);
logStatement
    : LOG logLevel? (NODE expression)? expression logTemplateParams? onErrorClause?
    ;

logLevel
    : INFO
    | WARNING
    | ERROR
    | DEBUG
    | TRACE
    | CRITICAL
    ;

// Template parameters: WITH ({1} = expr, {2} = expr) or PARAMETERS [expr, expr]
templateParams
    : WITH LPAREN templateParam (COMMA templateParam)* RPAREN    // WITH ({1} = $var)
    | PARAMETERS arrayLiteral                                     // PARAMETERS ['val'] (deprecated)
    ;

templateParam
    : LBRACE NUMBER_LITERAL RBRACE EQUALS expression
    ;

// Backward compatibility aliases
logTemplateParams: templateParams;
logTemplateParam: templateParam;

// $Result = CALL MICROFLOW MfTest.M001_HelloWorld(); or CALL MICROFLOW MfTest.M001_HelloWorld();
callMicroflowStatement
    : (VARIABLE EQUALS)? CALL MICROFLOW qualifiedName LPAREN callArgumentList? RPAREN queueClause? onErrorClause?
    ;

// IN QUEUE Module.QueueName — runs the call on a task queue (Queues$QueueSettings
// on the call activity). Valid on CALL MICROFLOW and CALL JAVA ACTION only;
// Mendix has no queued nanoflow or JavaScript action.
queueClause
    : IN QUEUE qualifiedName
    ;

callNanoflowStatement
    : (VARIABLE EQUALS)? CALL NANOFLOW qualifiedName LPAREN callArgumentList? RPAREN onErrorClause?
    ;

// $Result = CALL JAVA ACTION CustomActivities.ExecuteOQL(OqlStatement = '...');
callJavaActionStatement
    : (VARIABLE EQUALS)? CALL JAVA ACTION qualifiedName LPAREN callArgumentList? RPAREN queueClause? onErrorClause?
    ;

// $Result = CALL JAVASCRIPT ACTION Module.JSAction(Param = 'value');
callJavaScriptActionStatement
    : (VARIABLE EQUALS)? CALL JAVASCRIPT ACTION qualifiedName LPAREN callArgumentList? RPAREN onErrorClause?
    ;

// Legacy SOAP call.
//
// The request body is EITHER the operation's arguments OR an export mapping —
// Mendix stores one RequestBodyHandling, not two — so writing both is refused
// by `mxcli check`. The grammar admits both so the refusal can name them.
//
// Arguments parenthesise on OPERATION, matching CALL EXTERNAL ACTION: an OData
// action and a SOAP operation are the same shape of thing, and `operation X` is
// the callee here (the statement's own target is the service).
//
// SEND MAPPING … FROM $var mirrors REST's `body mapping … from $var`. FROM
// cannot be swallowed by the preceding qualifiedName — that rule only continues
// across a DOT — which is why the same shape already works there.
callWebServiceStatement
    : (VARIABLE EQUALS)? CALL WEB SERVICE
      (RAW STRING_LITERAL
      | webServiceReference
        (OPERATION webServiceReference (LPAREN callArgumentList? RPAREN)?)?
        (SEND MAPPING webServiceReference (FROM VARIABLE)?)?
        (RECEIVE MAPPING webServiceReference)?
        (TIMEOUT expression)?)
      onErrorClause?
    ;

webServiceReference
    : qualifiedName
    | STRING_LITERAL
    ;

// $Result = EXECUTE DATABASE QUERY Module.Connection.QueryName (param = 'value');
executeDatabaseQueryStatement
    : (VARIABLE EQUALS)? EXECUTE DATABASE QUERY qualifiedName
      (DYNAMIC (STRING_LITERAL | DOLLAR_STRING | expression))?
      (LPAREN callArgumentList? RPAREN)?
      (CONNECTION LPAREN callArgumentList? RPAREN)?
      onErrorClause?
    ;

// $Result = CALL EXTERNAL ACTION Module.ODataClient.ActionName(Param = $value);
callExternalActionStatement
    : (VARIABLE EQUALS)? CALL EXTERNAL ACTION qualifiedName LPAREN callArgumentList? RPAREN onErrorClause?
    ;

// ============================================================================
// Workflow microflow actions
// ============================================================================

// $Wf = CALL WORKFLOW Module.WF_Name ($ContextObj);
callWorkflowStatement
    // A workflow has exactly ONE context parameter, so the positional form is
    // unambiguous — and it is what DESCRIBE emits, since the model stores only
    // the context VARIABLE and not the parameter's name. Without this
    // alternative, describing a `call workflow` produced MDL that would not
    // parse on the way back in.
    : (VARIABLE EQUALS)? CALL WORKFLOW qualifiedName LPAREN (callArgumentList | VARIABLE)? RPAREN onErrorClause?
    ;

// $Data = GET WORKFLOW DATA $WorkflowVar AS Module.WorkflowName;
getWorkflowDataStatement
    : (VARIABLE EQUALS)? GET WORKFLOW DATA VARIABLE AS qualifiedName onErrorClause?
    ;

// $Wfs = GET WORKFLOWS FOR $ContextObj;
getWorkflowsStatement
    : (VARIABLE EQUALS)? GET WORKFLOWS FOR VARIABLE onErrorClause?
    ;

// $Records = GET WORKFLOW ACTIVITY RECORDS $WorkflowVar;
getWorkflowActivityRecordsStatement
    : (VARIABLE EQUALS)? GET WORKFLOW ACTIVITY RECORDS VARIABLE onErrorClause?
    ;

// WORKFLOW OPERATION ABORT $Wf REASON 'text';
workflowOperationStatement
    : WORKFLOW OPERATION workflowOperationType onErrorClause?
    ;

workflowOperationType
    : ABORT VARIABLE (REASON expression)?
    | CONTINUE VARIABLE
    | PAUSE VARIABLE
    | RESTART VARIABLE
    | RETRY VARIABLE
    | UNPAUSE VARIABLE
    ;

// SET TASK OUTCOME $UserTask 'OutcomeName';
setTaskOutcomeStatement
    : SET TASK OUTCOME VARIABLE STRING_LITERAL onErrorClause?
    ;

// OPEN USER TASK $UserTask;
openUserTaskStatement
    : OPEN USER TASK VARIABLE onErrorClause?
    ;

// [$Notified =] NOTIFY WORKFLOW $Wf TARGET Module.Workflow.Name;
// The target names the notification start, notification activity, notification
// boundary event or wait-for-notification the action reaches. It is optional here
// and required by check (MDL-WF16): Mendix refuses a notify with none (CE0166).
notifyWorkflowStatement
    : (VARIABLE EQUALS)? NOTIFY WORKFLOW VARIABLE (TARGET qualifiedName)? onErrorClause?
    ;

// OPEN WORKFLOW $Wf;
openWorkflowStatement
    : OPEN WORKFLOW VARIABLE onErrorClause?
    ;

// LOCK WORKFLOW $Wf; or LOCK WORKFLOW ALL;
lockWorkflowStatement
    : LOCK WORKFLOW (VARIABLE | ALL) onErrorClause?
    ;

// UNLOCK WORKFLOW $Wf; or UNLOCK WORKFLOW ALL;
unlockWorkflowStatement
    : UNLOCK WORKFLOW (VARIABLE | ALL) onErrorClause?
    ;

callArgumentList
    : callArgument (COMMA callArgument)*
    ;

// Named arguments: $FirstName = 'Hello' or Level = 'INFO' or OqlStatement = '...'
callArgument
    : (VARIABLE | parameterName) EQUALS expression
    ;

showPageStatement
    : SHOW PAGE qualifiedName (LPAREN showPageArgList? RPAREN)? (FOR VARIABLE)? (WITH memberAssignmentList)? onErrorClause?
    ;

showPageArgList
    : showPageArg (COMMA showPageArg)*
    ;

showPageArg
    : VARIABLE EQUALS (VARIABLE | expression)       // $Param = $value (canonical)
    | identifierOrKeyword COLON expression           // Param: $value (widget-style, also accepted)
    ;

closePageStatement
    : CLOSE PAGE onErrorClause?
    ;

showHomePageStatement
    : SHOW HOME PAGE
    ;

// SHOW MESSAGE 'Hello {1}' TYPE Information OBJECTS [$Name];
showMessageStatement
    : SHOW MESSAGE expression (TYPE identifierOrKeyword)? (OBJECTS LBRACKET expressionList RBRACKET)? BLOCKING? onErrorClause?
    ;

// SYNCHRONIZE ALL;
// SYNCHRONIZE UNSYNCHRONIZED;
// SYNCHRONIZE $Order, $Lines;              -- Specific mode
// SYNCHRONIZE ALL ON ERROR WITHOUT ROLLBACK { ... };
//
// Nanoflow-only: Mendix rejects a synchronize in a microflow, which is
// server-side. The bare `SYNCHRONIZE;` form is deliberately absent — the mode is
// always written out, so the statement says what it does without the reader
// having to know that the platform default is All.
synchronizeStatement
    : SYNCHRONIZE (ALL | UNSYNCHRONIZED | VARIABLE (COMMA VARIABLE)*) onErrorClause?
    ;

downloadFileStatement
    : DOWNLOAD FILE_KW VARIABLE (SHOW IN BROWSER)? onErrorClause?
    ;

throwStatement
    : THROW expression
    ;

// VALIDATION FEEDBACK $Product/Code MESSAGE 'Product code cannot be empty';
validationFeedbackStatement
    : VALIDATION FEEDBACK (attributePath | VARIABLE) MESSAGE expression (OBJECTS LBRACKET expressionList RBRACKET)? onErrorClause?
    ;

// =============================================================================
// REST CALL STATEMENTS
// =============================================================================

/**
 * REST call statement for making HTTP requests to external APIs.
 */
restCallStatement
    : (VARIABLE EQUALS)? REST CALL httpMethod restCallUrl restCallUrlParams?
      restCallHeaderClause*
      restCallAuthClause?
      restCallBodyClause?
      restCallTimeoutClause?
      restCallReturnsClause
      onErrorClause?
    ;

httpMethod
    : GET
    | POST
    | PUT
    | PATCH
    | DELETE
    ;

// URL can be a string literal or expression
restCallUrl
    : STRING_LITERAL
    | expression
    ;

// URL template parameters: WITH ({1} = expr, {2} = expr)
restCallUrlParams
    : templateParams
    ;

// HEADER name = 'value' or HEADER 'Content-Type' = 'value'
restCallHeaderClause
    : HEADER (IDENTIFIER | STRING_LITERAL) EQUALS expression
    ;

// AUTH BASIC $user PASSWORD $pass
restCallAuthClause
    : AUTH BASIC expression PASSWORD expression
    ;

// BODY 'template' [WITH params] or BODY MAPPING Name FROM $var
restCallBodyClause
    : BODY BINARY_TYPE expression                            // Binary body (a FileDocument's Contents)
    | BODY STRING_LITERAL templateParams?                    // Custom body template
    | BODY expression templateParams?                        // Expression body
    | BODY MAPPING qualifiedName FROM VARIABLE               // Export mapping
    ;

// TIMEOUT expression (in seconds)
restCallTimeoutClause
    : TIMEOUT expression
    ;

// RETURNS clause specifies how to handle the response
restCallReturnsClause
    : RETURNS STRING_TYPE                                       // Return as string
    | RETURNS RESPONSE                                          // Return HttpResponse object
    | RETURNS MAPPING qualifiedName AS LIST_OF qualifiedName    // Import mapping → list result
    | RETURNS MAPPING qualifiedName AS qualifiedName            // Import mapping → single object
    | RETURNS NONE                                              // Ignore response
    | RETURNS NOTHING                                           // Ignore response (alias)
    | RETURNS qualifiedName                                     // Store in file document (a System.FileDocument specialization)
    ;

/**
 * SEND REST REQUEST — calls a consumed REST service operation defined via CREATE REST CLIENT.
 */
sendRestRequestStatement
    : (VARIABLE EQUALS)? SEND REST REQUEST qualifiedName
      sendRestRequestWithClause?
      sendRestRequestBodyClause?
      onErrorClause?
    ;

sendRestRequestWithClause
    : WITH LPAREN sendRestRequestParam (COMMA sendRestRequestParam)* RPAREN
    ;

sendRestRequestParam
    : VARIABLE EQUALS expression
    ;

sendRestRequestBodyClause
    : BODY VARIABLE
    ;

/**
 * Import from mapping: [$Var =] IMPORT FROM MAPPING Module.IMM($SourceVar);
 */
importFromMappingStatement
    : (VARIABLE EQUALS)? IMPORT FROM MAPPING qualifiedName LPAREN VARIABLE RPAREN
      importMappingRange?
      onErrorClause?
    ;

/**
 * How much of the mapping's result to bind — Mendix's "Range" on the import
 * activity, stored as Microflows$ConstantRange{SingleObject} or
 * Microflows$CustomRange{LimitExpression, OffsetExpression}.
 *
 *   (omitted)          infer from the mapping's own root shape, as mxcli
 *                      always has — hand-written MDL keeps working unchanged
 *   ALL                All — bind the whole list, explicitly
 *   FIRST              First — bind ONE object, not a list
 *   LIMIT e [OFFSET e] Custom — a bounded list
 *
 * DESCRIBE always emits one of the three, never nothing: an object-rooted
 * mapping set to All is a real state (Studio Pro's own default — the blank
 * app ships one), and the inference would turn it into First on re-exec.
 *
 * LIMIT/OFFSET mirror the RETRIEVE clause rather than inventing a second
 * spelling for the same idea. FIRST is a separate word on purpose: `limit 1`
 * is a LIST of one, while First binds a single OBJECT — different result
 * variable types, so they cannot share syntax. (issue #881)
 */
importMappingRange
    : ALL
    | FIRST
    | LIMIT limitExpr=expression (OFFSET offsetExpr=expression)?
    ;

/**
 * Export to mapping: $Var = EXPORT TO MAPPING Module.EMM($SourceVar);
 */
exportToMappingStatement
    : (VARIABLE EQUALS)? EXPORT TO MAPPING qualifiedName LPAREN VARIABLE RPAREN
      onErrorClause?
    ;

/**
 * Transform JSON: $Result = TRANSFORM $Input WITH Module.Transformer;
 */
transformJsonStatement
    : (VARIABLE EQUALS)? TRANSFORM VARIABLE WITH qualifiedName
      onErrorClause?
    ;

// =============================================================================
// LIST OPERATIONS
// =============================================================================

/**
 * List operations that return a single item or a modified list.
 */
listOperationStatement
    : VARIABLE EQUALS listOperation
    ;

listOperation
    : HEAD LPAREN VARIABLE RPAREN                                      // $var = HEAD($list)
    | TAIL LPAREN VARIABLE RPAREN                                      // $var = TAIL($list)
    | FIND LPAREN VARIABLE COMMA expression RPAREN                     // $var = FIND($list, condition)
    | FILTER LPAREN VARIABLE COMMA expression RPAREN                   // $var = FILTER($list, condition)
    | SORT LPAREN VARIABLE COMMA sortSpecList RPAREN                   // $var = SORT($list, attr ASC)
    | UNION LPAREN VARIABLE COMMA VARIABLE RPAREN                      // $var = UNION($list1, $list2)
    | INTERSECT LPAREN VARIABLE COMMA VARIABLE RPAREN                  // $var = INTERSECT($list1, $list2)
    | SUBTRACT LPAREN VARIABLE COMMA VARIABLE RPAREN                   // $var = SUBTRACT($list1, $list2)
    | CONTAINS LPAREN VARIABLE COMMA VARIABLE RPAREN                   // $bool = CONTAINS($list, $item)
    | EQUALS_OP LPAREN VARIABLE COMMA VARIABLE RPAREN                  // $bool = EQUALS($list1, $list2)
    | RANGE LPAREN VARIABLE (COMMA expression (COMMA expression)?)? RPAREN // $var = RANGE($list, offset, limit)
    ;

sortSpecList
    : sortSpec (COMMA sortSpec)*
    ;

sortSpec
    : (IDENTIFIER | QUOTED_IDENTIFIER) (ASC | DESC)?  // "attr" if reserved
    ;

/**
 * Aggregate operations on lists.
 */
aggregateListStatement
    : VARIABLE EQUALS listAggregateOperation
    ;

listAggregateOperation
    : COUNT LPAREN VARIABLE RPAREN                                                    // $count = COUNT($list)
    | SUM LPAREN VARIABLE COMMA expression RPAREN                                     // $sum = SUM($list, expr)
    | SUM LPAREN attributePath RPAREN                                                 // $sum = SUM($list.attr)
    | AVERAGE LPAREN VARIABLE COMMA expression RPAREN                                 // $avg = AVERAGE($list, expr)
    | AVERAGE LPAREN attributePath RPAREN                                             // $avg = AVERAGE($list.attr)
    | MINIMUM LPAREN VARIABLE COMMA expression RPAREN                                 // $min = MINIMUM($list, expr)
    | MINIMUM LPAREN attributePath RPAREN                                             // $min = MINIMUM($list.attr)
    | MAXIMUM LPAREN VARIABLE COMMA expression RPAREN                                 // $max = MAXIMUM($list, expr)
    | MAXIMUM LPAREN attributePath RPAREN                                             // $max = MAXIMUM($list.attr)
    // REDUCE folds a list into one value. Both extra inputs are required and
    // neither is derivable: `initial` seeds $currentResult, `returns` is the
    // type Mendix stores alongside it (#1004).
    | REDUCE LPAREN VARIABLE COMMA expression COMMA reduceFoldOptions RPAREN          // $total = REDUCE($list, expr, initial: 0, returns: Decimal)
    // ALL / ANY test a Boolean expression over every item. Their return type is
    // always Boolean, so it is derived rather than written.
    | ALL LPAREN VARIABLE COMMA expression RPAREN                                     // $allMatch = ALL($list, expr)
    | ANY LPAREN VARIABLE COMMA expression RPAREN                                     // $anyMatch = ANY($list, expr)
    ;

// REDUCE's seed and result type, in the ( key: value ) property style used
// across MDL. Order is fixed so the statement reads the way it is written.
reduceFoldOptions
    : INITIAL COLON expression COMMA RETURNS COLON dataType
    ;

/**
 * Create an empty list of a specific entity type.
 */
createListStatement
    : VARIABLE EQUALS CREATE LIST_OF qualifiedName
    ;

/**
 * Add an item to a list.
 */
addToListStatement
    : ADD expression TO VARIABLE
    ;

/**
 * Remove an item from a list.
 */
removeFromListStatement
    : REMOVE VARIABLE FROM VARIABLE
    ;

// Member assignments for CREATE and CHANGE: Name = $Name, Code = $Code
memberAssignmentList
    : memberAssignment (COMMA memberAssignment)*
    ;

memberAssignment
    : memberAttributeName EQUALS expression
    ;

// Allow keywords and qualified names as member attribute names
memberAttributeName
    : qualifiedName
    | IDENTIFIER
    | QUOTED_IDENTIFIER                     // Escape any reserved word
    | keyword
    ;

// Legacy changeList for backwards compatibility
changeList
    : changeItem (COMMA changeItem)*
    ;

changeItem
    : IDENTIFIER EQUALS expression
    ;
