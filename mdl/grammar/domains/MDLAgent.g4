/**
 * MDL Agent Grammar — agent editor: models, knowledge bases,
 * consumed and published MCP services, agents.
 */
parser grammar MDLAgent;

options { tokenVocab = MDLLexer; }

// =============================================================================
// AGENT-EDITOR MODEL CREATION
// =============================================================================
// CREATE MODEL Module.Name (
//   Provider: MxCloudGenAI,
//   Key: @Module.SomeConstant
//   [, DisplayName: '...', KeyName: '...', etc. — Portal-populated metadata]
// );
createModelStatement
    : aiModelKw ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
    ;

// R10: Studio Pro calls the agent editor's model document an AI model; `model`
// alone is too generic (it is also the old `alter settings model`).
aiModelKw
    : AI MODEL
    | MODEL /* @alias MDL-DEPR131 */
    ;

aiModelsKw
    : AI MODELS
    | MODELS /* @alias MDL-DEPR131 */
    ;

modelProperty
    : identifierOrKeyword COLON identifierOrKeyword       // Provider: MxCloudGenAI
    | identifierOrKeyword COLON AT qualifiedName          // Key: @Module.Constant (the one constant reference, R5)
    // A document name (Model: Module.Model). For Key, a constant, the bare name
    // is the deprecated spelling of Key: @Module.Constant.
    | identifierOrKeyword COLON qualifiedName /* @alias MDL-DEPR084 */
    | identifierOrKeyword COLON STRING_LITERAL            // DisplayName: 'GPT-4 Turbo' etc.
    | identifierOrKeyword COLON NUMBER_LITERAL            // ConnectionTimeoutSeconds: 30
    | identifierOrKeyword COLON booleanLiteral            // Enabled: true
    | identifierOrKeyword COLON DOLLAR_STRING              // SystemPrompt: $$multi-line...$$
    | identifierOrKeyword COLON LPAREN variableDefList RPAREN  // Variables: ("Key": EntityAttribute, ...)
    ;

variableDefList
    : variableDef (COMMA variableDef)*
    ;

variableDef
    : (STRING_LITERAL | QUOTED_IDENTIFIER) COLON identifierOrKeyword  // "Key": EntityAttribute
    ;

// =============================================================================
// AGENT-EDITOR CONSUMED MCP SERVICE CREATION
// =============================================================================
// CREATE CONSUMED MCP SERVICE Module.Name (
//   ProtocolVersion: v2025_03_26,
//   Version: '0.0.1',
//   ConnectionTimeoutSeconds: 30,
//   Documentation: '...'
// );
createConsumedMCPServiceStatement
    : CONSUMED MCP SERVICE ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
    ;

// =============================================================================
// AGENT-EDITOR PUBLISHED MCP SERVICE CREATION (Agent Editor 2.6+, MCP Server 5.x)
// =============================================================================
// CREATE PUBLISHED MCP SERVICE Module.Name (
//   ServerName: 'Orders', EndpointPath: 'orders', Version: '1.0.0',
//   ProtocolVersion: v2025_03_26, Documentation: '...'
// ) {
//   tool get_order ( Microflow: Module.GetOrder, Description: '...', Title: '...' )
// };
createPublishedMCPServiceStatement
    : PUBLISHED MCP SERVICE ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
      publishedMCPServiceBody?
    ;

publishedMCPServiceBody
    : LBRACE publishedMCPToolBlock* RBRACE
    ;

// A microflow exposed as an MCP tool, named as the client sees it.
publishedMCPToolBlock
    : TOOL identifierOrKeyword LPAREN modelProperty (COMMA modelProperty)* COMMA? RPAREN
    ;

// ALTER PUBLISHED MCP SERVICE actions: SET for the service's properties,
// ADD / DROP for its tools.
alterPublishedMCPServiceAction
    : SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ADD publishedMCPToolBlock
    | DROP TOOL identifierOrKeyword
    ;

// =============================================================================
// AGENT-EDITOR KNOWLEDGE BASE CREATION
// =============================================================================
// CREATE KNOWLEDGE BASE Module.Name (
//   Provider: MxCloudGenAI,
//   Key: @Module.SomeConstant
// );
createKnowledgeBaseStatement
    : KNOWLEDGE BASE ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
    ;

// =============================================================================
// AGENT-EDITOR AGENT CREATION
// =============================================================================
// CREATE AGENT Module.Name (
//   UsageType: Task,
//   Model: Module.MyModel,
//   SystemPrompt: '...',
//   ...
// )
// [ { tool X ( ... ) | mcp service M.X ( ... ) | knowledge base KB ( ... ) } ]
// ;
createAgentStatement
    : AGENT ifNotExists? qualifiedName
      (FOLDER STRING_LITERAL)?
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
      agentBody?
    ;

agentBody
    : LBRACE agentBodyBlock* RBRACE
    ;

// An attachment is a child of the agent, so its properties are in ( ) like
// every other child's (R2, ako/mxcli#754). The brace form is the old spelling.
agentBodyBlock
    : MCP SERVICE qualifiedName
      ( LPAREN modelProperty (COMMA modelProperty)* COMMA? RPAREN                       // mcp service Mod.Name ( ... )
      | LBRACE /* @alias MDL-DEPR071 */ modelProperty (COMMA modelProperty)* COMMA? RBRACE
      )
    | KNOWLEDGE BASE identifierOrKeyword
      ( LPAREN modelProperty (COMMA modelProperty)* COMMA? RPAREN                       // knowledge base MyKB ( ... )
      | LBRACE /* @alias MDL-DEPR071 */ modelProperty (COMMA modelProperty)* COMMA? RBRACE
      )
    | TOOL identifierOrKeyword
      ( LPAREN modelProperty (COMMA modelProperty)* COMMA? RPAREN                       // tool ToolName ( ... )
      | LBRACE /* @alias MDL-DEPR071 */ modelProperty (COMMA modelProperty)* COMMA? RBRACE
      )
    ;

// =============================================================================
// ALTER actions for agent-editor documents
// =============================================================================
// Used by ALTER MODEL / ALTER KNOWLEDGE BASE / ALTER CONSUMED MCP SERVICE
// (SET-only — these document types have no collections), and by the SET
// clause of ALTER AGENT.
agentEditorAlterAssignment
    : identifierOrKeyword EQUALS agentEditorAlterValue
    ;

agentEditorAlterValue
    : STRING_LITERAL
    | NUMBER_LITERAL
    | DOLLAR_STRING
    | booleanLiteral
    | AT qualifiedName   // Key = @Module.Constant
    | qualifiedName
    | identifierOrKeyword
    ;

// ALTER AGENT actions: a mix of SET (scalar properties) and ADD/DROP for
// the agent's three collections (tools, MCP services, knowledge bases).
//
// @example Combine SET with collection mutations:
// ```mdl
// ALTER AGENT MyModule.Helper
//   SET SystemPrompt = 'New prompt', Temperature = 0.5
//   ADD TOOL DoSomething ( Description: '...', Enabled: true )
//   ADD MCP SERVICE MyModule.Weather ( Description: '...', Enabled: true )
//   DROP KNOWLEDGE BASE OldKB
// ;
// ```
alterAgentAction
    : SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ADD agentBodyBlock
    | DROP TOOL identifierOrKeyword
    | DROP MCP SERVICE qualifiedName
    | DROP KNOWLEDGE BASE identifierOrKeyword
    ;
