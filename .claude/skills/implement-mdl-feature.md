# Implement New MDL Feature Skill

This skill provides a step-by-step workflow for implementing new MDL syntax for Mendix concepts (microflow actions, page widgets, etc.).

## When to Use This Skill

Use this skill when:
- Adding support for a new Mendix action type (e.g., REST call, web service call)
- Adding support for a new widget type
- Extending existing MDL syntax with new clauses or options
- Implementing round-trip support (CREATE + DESCRIBE) for a Mendix concept

## Overview

Implementing a new MDL feature requires changes across multiple layers:

```
┌─────────────────────────────────────────────────────────────────────┐
│                    1. INVESTIGATION                                  │
│    Dump BSON from MPR → Understand structure → design MDL syntax    │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    2. GRAMMAR (MDLLexer.g4 + MDLParser.g4)          │
│    add tokens → add parser rules → Regenerate (make grammar)        │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    3. AST (mdl/ast/)                                 │
│    define Go structs representing the parsed syntax                  │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    4. VISITOR (mdl/visitor/)                         │
│    Parse ANTLR context → build AST nodes                            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    5. SDK TYPES (sdk/microflows/ or sdk/pages/)     │
│    add/update Go structs for the Mendix concept                     │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                  ┌───────────────┴───────────────┐
                  ▼                               ▼
┌─────────────────────────────┐   ┌─────────────────────────────┐
│  6a. READER (BSON → Go)     │   │  6b. WRITER (Go → BSON)     │
│  mdl/backend/modelsdk/      │   │  mdl/backend/modelsdk/      │
│         *_read.go           │   │         *_write.go          │
│  for describe to work       │   │  for create to work         │
└─────────────────────────────┘   └─────────────────────────────┘
                  │                               │
                  └───────────────┬───────────────┘
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    7. EXECUTOR (mdl/executor/)                       │
│    cmd_*_builder.go (AST → BSON)  +  cmd_*_show.go (Go → MDL)       │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    8. TESTING                                        │
│    Syntax check → describe roundtrip → create in project            │
└─────────────────────────────────────────────────────────────────────┘
```

## Part 1: Investigation

### Step 1: Find an Example in a Mendix Project

First, create or find an example of the feature in Mendix Studio Pro:

1. Open a test project in Studio Pro (e.g., `mx-test-projects/test2-go-app/test2-go.mpr`)
2. Create the element you want to support (e.g., a microflow with a REST call action)
3. Save the project
4. Note the qualified name (e.g., `RestTemplate.GetWebpage`)

### Step 2: Find the Document ID

```bash
./bin/mxcli -p mx-test-projects/test2-go-app/test2-go.mpr \
  -c "select Id, QualifiedName from CATALOG.MICROFLOWS where QualifiedName = 'RestTemplate.GetWebpage'"
```

### Step 3: Dump Raw BSON

Use `mxcli bson dump` to inspect the raw BSON structure (see [BSON Tooling Guide](../../docs/03-development/BSON_TOOLING_GUIDE.md) for the full tool reference):

```bash
mxcli bson dump -p mx-test-projects/test2-go-app/test2-go.mpr --type microflow --object "RestTemplate.GetWebpage"
```

Example output (abbreviated):
```json
{
  "$type": "microflows$microflow",
  "ObjectCollection": {
    "objects": [
      {
        "$type": "microflows$ActionActivity",
        "action": {
          "$type": "microflows$RestCallAction",
          "HttpConfiguration": {
            "$type": "microflows$HttpConfiguration",
            "HttpMethod": "get",
            "CustomLocationTemplate": {
              "text": "{1}",
              "parameters": [
                {"expression": "'http://example.com'"}
              ]
            },
            "HttpHeaderEntries": [
              {"key": "Accept", "value": "'text/html'"}
            ]
          },
          "ResultHandling": {
            "ResultVariableName": "MyPage"
          },
          "ResultHandlingType": "string",
          "TimeOutExpression": "300"
        }
      }
    ]
  }
}
```

### Step 4: Design MDL Syntax

Based on the BSON structure, design MDL syntax that:
- Is readable and intuitive
- Maps clearly to BSON fields
- Follows existing MDL conventions

Example design for REST call:
```sql
$response = call rest service get 'http://api.example.com/data' (
  Headers: ('Content-Type': 'application/json', 'Accept': 'application/json'),
  Authentication: basic (Username: $username, Password: $password),
  Body: template '{{"key": "{1}"}' with ({1} = $value),
  Timeout: 30,
) returns string
  on error rollback;
```

### Step 5: Create Examples

Add examples to `mdl-examples/doctype-tests/`:

```bash
# add to 02-microflow-examples.mdl
create microflow RestExamples.SimpleGet()
returns string as $response
begin
  $response = call rest service get 'https://api.example.com/data' (
    Timeout: 30,
  ) returns string;
  return $response;
end;
```

## Part 2: Grammar Implementation

### Step 1: Add Tokens (MDLLexer.g4)

Add new keywords if needed:

```antlr
// in MDLLexer.g4, alphabetically ordered
auth: A U T H;
body: B O D Y;
header: H E A D E R;
mapping: M A P P I N G;
password: P A S S W O R D;
rest: R E S T;
timeout: T I M E O U T;
```

### Step 2: Add Parser Rules (MDLParser.g4)

```antlr
// add to microflowStatement alternatives
microflowStatement
    : // ... existing alternatives
    | restCallStatement
    ;

// define the new statement
restCallStatement
    : (VARIABLE equals)? rest call httpMethod restCallUrl restCallUrlParams?
      restCallHeaderClause*
      restCallAuthClause?
      restCallBodyClause?
      restCallTimeoutClause?
      restCallReturnsClause
      onErrorClause?
    ;

httpMethod
    : get | post | put | patch | delete
    ;

restCallUrl
    : STRING_LITERAL | expression
    ;

restCallHeaderClause
    : header (IDENTIFIER | STRING_LITERAL) equals expression
    ;

// ... other clauses
```

### Step 3: Regenerate Parser

```bash
make grammar
```

This runs ANTLR4 and generates the Go parser code in `mdl/grammar/parser/`.

## Part 3: AST Types

Add AST types in `mdl/ast/ast_microflow.go`:

```go
// HttpMethod represents an HTTP method.
type HttpMethod string

const (
    HttpMethodGet    HttpMethod = "get"
    HttpMethodPost   HttpMethod = "post"
    HttpMethodPut    HttpMethod = "put"
    HttpMethodPatch  HttpMethod = "patch"
    HttpMethodDelete HttpMethod = "delete"
)

// RestHeader represents a custom HTTP header.
type RestHeader struct {
    Name  string
    value expression
}

// RestCallStmt represents a rest call statement.
type RestCallStmt struct {
    OutputVariable string
    method         HttpMethod
    url            expression
    URLParams      []TemplateParam
    headers        []RestHeader
    auth           *RestAuth
    body           *RestBody
    timeout        expression
    Result         RestResult
    ErrorHandling  *ErrorHandlingClause
}

func (s *RestCallStmt) isMicroflowStatement() {}
```

## Part 4: Visitor Implementation

Add visitor handler in `mdl/visitor/visitor_microflow_statements.go`:

```go
func buildRestCallStatement(ctx parser.IRestCallStatementContext) *ast.RestCallStmt {
    restCtx := ctx.(*parser.RestCallStatementContext)
    stmt := &ast.RestCallStmt{}

    // Output variable
    if v := restCtx.VARIABLE(); v != nil {
        stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
    }

    // HTTP method
    if m := restCtx.HttpMethod(); m != nil {
        methodCtx := m.(*parser.HttpMethodContext)
        if methodCtx.GET() != nil {
            stmt.Method = ast.HttpMethodGet
        } else if methodCtx.POST() != nil {
            stmt.Method = ast.HttpMethodPost
        }
        // ... other methods
    }

    // url
    if urlC := restCtx.RestCallUrl(); urlC != nil {
        urlCtx := urlC.(*parser.RestCallUrlContext)
        if strLit := urlCtx.STRING_LITERAL(); strLit != nil {
            stmt.URL = &ast.StringLiteral{value: unquoteString(strLit.GetText())}
        }
    }

    // ... parse other clauses

    return stmt
}
```

Add to the statement switch in `buildMicroflowStatement`:

```go
case ctx.RestCallStatement() != nil:
    return buildRestCallStatement(ctx.RestCallStatement())
```

## Part 5: SDK Types

Update or add types in `sdk/microflows/microflows_actions.go`:

```go
// RestCallAction represents a rest call action.
type RestCallAction struct {
    model.BaseElement
    HttpConfiguration *HttpConfiguration `json:"httpConfiguration,omitempty"`
    RequestHandling   RequestHandling    `json:"requestHandling,omitempty"`
    ResultHandling    ResultHandling     `json:"resultHandling,omitempty"`
    ErrorHandlingType ErrorHandlingType  `json:"errorHandlingType,omitempty"`
    OutputVariable    string             `json:"outputVariable,omitempty"`
    TimeoutExpression string             `json:"timeoutExpression,omitempty"`
}

func (RestCallAction) isMicroflowAction() {}

// HttpConfiguration represents HTTP configuration for a rest call.
type HttpConfiguration struct {
    model.BaseElement
    HttpMethod        HttpMethod    `json:"httpMethod"`
    LocationTemplate  string        `json:"locationTemplate,omitempty"`
    LocationParams    []string      `json:"locationParams,omitempty"`
    CustomHeaders     []*HttpHeader `json:"customHeaders,omitempty"`
    UseAuthentication bool          `json:"useAuthentication,omitempty"`
    username          string        `json:"username,omitempty"`
    password          string        `json:"password,omitempty"`
}
```

## Part 6a: Parser (BSON → Go)

Add read logic in `mdl/backend/modelsdk/microflow_read_actions.go`:

```go
// add case in parseActionActivity switch
case "microflows$RestCallAction":
    return parseRestCallAction(raw)

// add parser function
func parseRestCallAction(raw map[string]interface{}) *microflows.RestCallAction {
    action := &microflows.RestCallAction{}
    action.ID = model.ID(extractBsonID(raw["$ID"]))
    action.TimeoutExpression = extractString(raw["TimeOutExpression"])
    action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))

    // Parse HttpConfiguration
    if httpConfig, ok := raw["HttpConfiguration"].(map[string]interface{}); ok {
        action.HttpConfiguration = parseHttpConfiguration(httpConfig)
    }

    // Parse ResultHandling
    resultHandlingType := extractString(raw["ResultHandlingType"])
    if resultHandling, ok := raw["ResultHandling"].(map[string]interface{}); ok {
        action.ResultHandling = parseResultHandling(resultHandling, resultHandlingType)
    }

    return action
}
```

## Part 6b: Writer (Go → BSON)

Add serialization logic in `mdl/backend/modelsdk/microflow_write.go`:

```go
func serializeRestCallAction(action *microflows.RestCallAction) bson.D {
    doc := bson.D{
        {key: "$ID", value: idToBsonBinary(action.ID)},
        {key: "$type", value: "microflows$RestCallAction"},
        {key: "ErrorHandlingType", value: string(action.ErrorHandlingType)},
        {key: "TimeOutExpression", value: action.TimeoutExpression},
        {key: "UseRequestTimeOut", value: action.TimeoutExpression != ""},
    }

    // Serialize HttpConfiguration
    if action.HttpConfiguration != nil {
        doc = append(doc, bson.E{key: "HttpConfiguration",
            value: serializeHttpConfiguration(action.HttpConfiguration)})
    }

    // ... serialize other fields

    return doc
}
```

## Part 7: Executor

### Builder (AST → BSON)

In `mdl/executor/cmd_microflows_builder.go`:

```go
// add case in addStatement switch
case *ast.RestCallStmt:
    return mb.addRestCallAction(stmt)

// add builder method
func (mb *MicroflowBuilder) addRestCallAction(stmt *ast.RestCallStmt) error {
    action := &microflows.RestCallAction{
        ErrorHandlingType: mb.mapErrorHandling(stmt.ErrorHandling),
    }

    // build HttpConfiguration
    action.HttpConfiguration = &microflows.HttpConfiguration{
        HttpMethod: microflows.HttpMethod(stmt.Method),
    }

    // Evaluate url expression
    if stmt.URL != nil {
        urlVal, err := mb.evaluateExpression(stmt.URL)
        if err != nil {
            return err
        }
        action.HttpConfiguration.LocationTemplate = "{1}"
        action.HttpConfiguration.LocationParams = []string{urlVal}
    }

    // ... build other fields

    return mb.addActivity(action, stmt.OutputVariable, stmt.ErrorHandling)
}
```

### Show (Go → MDL)

In `mdl/executor/cmd_microflows_show.go`:

```go
// add case in formatActionStatement switch
case *microflows.RestCallAction:
    return e.formatRestCallAction(a)

// add formatter
func (e *Executor) formatRestCallAction(a *microflows.RestCallAction) string {
    var sb strings.Builder

    // Output variable
    if outputVar := getOutputVariable(a); outputVar != "" {
        sb.WriteString("$")
        sb.WriteString(outputVar)
        sb.WriteString(" = ")
    }

    sb.WriteString("rest call ")

    // HTTP method
    if a.HttpConfiguration != nil {
        sb.WriteString(strings.ToUpper(string(a.HttpConfiguration.HttpMethod)))
    }
    sb.WriteString(" ")

    // url with parameters
    if a.HttpConfiguration != nil {
        sb.WriteString("'")
        sb.WriteString(a.HttpConfiguration.LocationTemplate)
        sb.WriteString("'")
        if len(a.HttpConfiguration.LocationParams) > 0 {
            sb.WriteString(" with (")
            for i, param := range a.HttpConfiguration.LocationParams {
                if i > 0 {
                    sb.WriteString(", ")
                }
                sb.WriteString(fmt.Sprintf("{%d} = %s", i+1, param))
            }
            sb.WriteString(")")
        }
    }

    // ... format other clauses

    sb.WriteString(";")
    return sb.String()
}
```

## Part 7b: CREATE OR MODIFY Support

Every new top-level document type that uses `CREATE` should also support
`CREATE OR MODIFY` (for documents without a replace-by-recreate semantic) or
`CREATE OR REPLACE` (when full replacement is acceptable).

**Why UUID matters**: Mendix identifies documents by UUID. Dropping and
re-creating a document generates a new UUID, which can silently break runtime
references (associations, microflow call targets, etc.). `CREATE OR MODIFY`
preserves the existing UUID by calling `Update*` instead of `Create*`.

### Pattern: implement OR MODIFY in the executor

```go
func execCreateFoo(ctx *ExecContext, s *ast.CreateFooStmt) error {
    if !ctx.ConnectedForWrite() {
        return mdlerrors.NewNotConnectedWrite()
    }

    // 1. Check for existing document
    existing, _ := ctx.Backend.GetFooByQualifiedName(s.Name.Module, s.Name.Name)
    if existing != nil && !s.CreateOrModify {
        return mdlerrors.NewAlreadyExists("foo", s.Name.String())
    }

    // 2. Build the new struct (always fresh)
    foo := &model.Foo{
        ContainerID: containerID,
        Name:        s.Name.Name,
        // ... other fields
    }

    // 3. If existing, preserve UUID and update
    if existing != nil {
        foo.ID = existing.ID
        if err := ctx.Backend.UpdateFoo(foo); err != nil {
            return mdlerrors.NewBackend("update foo", err)
        }
        if !ctx.Quiet {
            fmt.Fprintf(ctx.Output, "Modified foo %s\n", s.Name)
        }
        return nil
    }

    // 4. Otherwise create
    if err := ctx.Backend.CreateFoo(foo); err != nil {
        return mdlerrors.NewBackend("create foo", err)
    }
    if !ctx.Quiet {
        fmt.Fprintf(ctx.Output, "Created foo %s\n", s.Name)
    }
    return nil
}
```

### Pattern: set CreateOrModify in the visitor

```go
func (v *Visitor) ExitCreateFooStatement(ctx *parser.CreateFooStatementContext) {
    stmt := &ast.CreateFooStmt{ /* ... */ }
    if createStmt := findParentCreateStatement(ctx); createStmt != nil {
        if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
            stmt.CreateOrModify = true
        }
    }
    v.program.Statements = append(v.program.Statements, stmt)
}
```

### Required backend additions for OR MODIFY

1. **`UpdateFoo(foo *model.Foo) error`** on the backend interface (`mdl/backend/`)
2. **MPR implementation** in `mdl/backend/mpr/backend.go` delegating to `b.writer.UpdateFoo(foo)`
3. **Mock stub** in `mdl/backend/mock/` with `UpdateFooFunc` field and descriptive default error
4. **`CreateOrModify bool`** field on the AST struct in `mdl/ast/`

### Entity / association special case

- **Associations** have no own `Update*` in the SDK writer; mutate the struct
  fields in the loaded `DomainModel` then call `UpdateDomainModel`.
- **Entities**: `CREATE OR REPLACE entity` must map to `CreateOrModify=true`,
  NOT to delete+recreate. Dropping and recreating an entity drops the DB table.
  Always preserve the entity UUID.

## Part 8: Testing

### Step 1: Syntax Check

```bash
./bin/mxcli check mdl-examples/doctype-tests/02-microflow-examples.mdl
```

Expected: `Syntax OK (N statements)`

### Step 2: Describe Roundtrip

```bash
./bin/mxcli -p mx-test-projects/test2-go-app/test2-go.mpr \
  -c "describe microflow RestTemplate.GetWebpage"
```

Expected: Valid MDL output that could be re-parsed.

### Step 3: Create and Verify

```bash
# create via MDL
./bin/mxcli -p test-project.mpr -c "
create microflow Test.RestExample()
returns string as \$R
begin
  \$R = call rest service get 'http://example.com' (Timeout: 30) returns string;
  return \$R;
end;"

# describe to verify
./bin/mxcli -p test-project.mpr -c "describe microflow Test.RestExample"

# open in Studio Pro to verify it works
```

### Step 4: Run Existing Tests

```bash
make test
```

## Checklist

Before considering the implementation complete:

- [ ] Investigation complete - BSON structure understood
- [ ] MDL syntax designed and documented in examples
- [ ] Grammar tokens added (`MDLLexer.g4`)
- [ ] Grammar rules added (`MDLParser.g4`)
- [ ] Parser regenerated (`make grammar`)
- [ ] AST types added (`mdl/ast/`)
- [ ] Visitor implemented (`mdl/visitor/`)
- [ ] SDK types added/updated (`sdk/microflows/` or `sdk/pages/`)
- [ ] BSON reader added (`mdl/backend/modelsdk/*_read.go`)
- [ ] BSON writer added (`mdl/backend/modelsdk/*_write.go`)
- [ ] Executor builder added (`mdl/executor/cmd_*_builder.go`)
- [ ] Executor show/describe added (`mdl/executor/cmd_*_show.go`)
- [ ] Syntax check passes
- [ ] Describe roundtrip works
- [ ] CREATE works and opens in Studio Pro
- [ ] Build succeeds (`make build`)
- [ ] Tests pass (`make test`)

## File Reference

| Layer | Files | Purpose |
|-------|-------|---------|
| Grammar | `mdl/grammar/MDLLexer.g4` | Token definitions |
| Grammar | `mdl/grammar/MDLParser.g4` | Parser rules |
| AST | `mdl/ast/ast_microflow.go` | Microflow statement types |
| AST | `mdl/ast/ast_page.go` | Page/widget statement types |
| Visitor | `mdl/visitor/visitor_microflow_statements.go` | Microflow parsing |
| Visitor | `mdl/visitor/visitor_page.go` | Page parsing |
| SDK | `sdk/microflows/microflows_actions.go` | Action Go types |
| SDK | `sdk/pages/pages.go` | Widget Go types |
| Reader | `mdl/backend/modelsdk/microflow_read_actions.go` | BSON → Go (microflows) |
| Reader | `mdl/backend/modelsdk/page.go` | BSON → Go (pages) |
| Writer | `mdl/backend/modelsdk/microflow_write.go` | Go → BSON (microflows) |
| Writer | `mdl/backend/modelsdk/widget_write.go` | Go → BSON (widgets) |
| Codec | `modelsdk/codec/encoder.go`, `decoder.go` | the layer underneath both |
| Executor | `mdl/executor/cmd_microflows_builder.go` | AST → microflow BSON |
| Executor | `mdl/executor/cmd_microflows_show.go` | Microflow → MDL |
| Executor | `mdl/executor/cmd_pages_builder.go` | AST → page BSON |
| Executor | `mdl/executor/cmd_pages_show.go` | Page → MDL |
| Debug | `cmd/debug/main.go` | Raw BSON dump tool |
| Examples | `mdl-examples/doctype-tests/` | MDL examples |

## Related Documentation

- [BSON Tooling Guide](../../docs/03-development/BSON_TOOLING_GUIDE.md) - Which BSON tool to use when (dump, compare, discover, TUI, Python)
- [Debug BSON](./debug-bson.md) - Fixing BSON serialization issues (CE errors, widget templates)
- [Write Microflows](./write-microflows.md) - MDL microflow syntax reference
