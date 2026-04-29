# MDL Parser Architecture

This document describes the ANTLR4-based parser implementation for MDL (Mendix Definition Language) in the Go library.

## Overview

The MDL parser translates SQL-like MDL syntax into executable operations against Mendix project files. It uses ANTLR4 for grammar definition, enabling cross-language grammar sharing with other implementations (TypeScript, Java, Python).

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         MDL Input string                            │
│              "show entities in MyModule"                            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ANTLR4 Lexer (mdl_lexer.go)                      │
│    Generated from MDLLexer.g4 - Tokenizes input into show, entities,│
│    in, IDENTIFIER tokens                                            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ANTLR4 Parser (mdl_parser.go)                    │
│    Generated from MDLParser.g4 - Builds parse tree according to grammar │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ANTLR Listener (visitor/visitor.go)              │
│    Walks parse tree and builds strongly-typed AST nodes            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         AST (ast/ast.go)                            │
│    *ast.ShowStmt{type: "entities", module: "MyModule"}             │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Executor (executor/executor.go)                  │
│    Executes AST against modelsdk-go api                            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      modelsdk-go Library                            │
│    mpr.Writer, domainmodel.Entity, etc.                            │
└─────────────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
mdl/
├── grammar/
│   ├── MDLLexer.g4         # ANTLR4 lexer grammar (tokens)
│   ├── MDLParser.g4        # ANTLR4 parser grammar (rules)
│   └── parser/             # Generated parser code (DO not EDIT)
│       ├── mdl_lexer.go
│       ├── mdl_parser.go
│       ├── mdlparser_listener.go
│       └── mdlparser_base_listener.go
├── ast/
│   └── ast.go, ast_microflow.go, ast_expression.go, ast_datatype.go, ...
│       # ast_microflow.go covers both CreateMicroflowStmt and CreateNanoflowStmt
├── visitor/
│   └── visitor.go          # ANTLR listener implementation
├── executor/
│   ├── executor.go              # AST execution logic
│   ├── cmd_microflows_builder.go  # microflow/nanoflow builder (variable tracking)
│   └── validate_microflow.go     # AST-level semantic checks (mxcli check) — covers both microflows and nanoflows
├── catalog/
│   └── catalog.go          # SQLite-based project metadata catalog
├── linter/
│   ├── linter.go           # Linting framework
│   └── rules/              # Built-in lint rules (MDL001–MDL004)
└── repl/
    └── repl.go             # Interactive REPL interface

cmd/mxcli/
└── main.go                 # Cobra CLI entry point
```

## Component Details

### 1. ANTLR4 Grammar (`mdl/grammar/MDLLexer.g4` + `MDLParser.g4`)

The grammar defines MDL syntax using ANTLR4's EBNF-like notation.

**Key design patterns:**

#### Case-Insensitive Keywords
Uses fragment rules for case-insensitive matching:

```antlr
// Keywords are case-insensitive
show    : S H O W ;
entity  : E N T I T Y ;

// fragment rules for each letter
fragment S : [sS] ;
fragment H : [hH] ;
fragment O : [oO] ;
fragment W : [wW] ;
// ... etc
```

#### Labeled Alternatives
Parser rules use labeled alternatives for type-safe listener methods:

```antlr
showStatement
    : show modules SEMI?                       # ShowModules
    | show entities (in IDENTIFIER)? SEMI?     # ShowEntities
    | show entity qualifiedName SEMI?          # ShowEntity
    ;
```

Each label generates a specific listener method (e.g., `EnterShowModules`, `EnterShowEntities`).

#### Whitespace Handling
Whitespace is sent to a hidden channel (skipped):

```antlr
WS : [ \t\r\n]+ -> skip ;
```

### 2. Generated Parser (`mdl/grammar/parser/`)

ANTLR4 generates four files:

| File | Purpose |
|------|---------|
| `mdl_lexer.go` | Tokenizer - converts input to token stream |
| `mdl_parser.go` | Parser - builds parse tree from tokens |
| `mdl_listener.go` | Listener interface - callbacks for each rule |
| `mdl_base_listener.go` | Empty listener implementation for extension |

**Regenerating the parser:**

```bash
cd mdl/grammar
antlr4 -Dlanguage=Go -package parser -o parser MDLLexer.g4 MDLParser.g4
```

Or from the project root:
```bash
make grammar
```

Requirements:
- ANTLR4 tool (`antlr4` command or Java JAR)
- Go target runtime (`github.com/antlr4-go/antlr/v4`)

### 3. AST Types (`pkg/mdl/ast/ast.go`)

Strongly-typed AST nodes representing MDL statements.

```go
// Statement is the interface for all MDL statements
type Statement interface {
    statementNode()
}

// ShowStmt represents show commands
type ShowStmt struct {
    type   string        // modules, entities, associations, enumerations
    module string        // Optional: filter by module
    Name   QualifiedName // for show entity/association
}

// CreateEntityStmt represents create entity
type CreateEntityStmt struct {
    Name        QualifiedName
    persistent  bool
    attributes  []attribute
    position    *position
    comment     string
    Doc         string
}

// QualifiedName represents Module.Name or just Name
type QualifiedName struct {
    module string
    Name   string
}
```

### 4. ANTLR Listener (`pkg/mdl/visitor/visitor.go`)

The visitor walks the ANTLR parse tree and builds AST nodes.

**Key patterns:**

#### Type Assertions for Context Access
ANTLR generates interface types for rule contexts. To access specific methods, type assertions are required:

```go
func (v *Visitor) EnterShowEntities(ctx *parser.ShowEntitiesContext) {
    stmt := &ast.ShowStmt{type: "entities"}

    // access IDENTIFIER token if present (in clause)
    if id := ctx.IDENTIFIER(); id != nil {
        stmt.Module = id.GetText()
    }

    v.program.Statements = append(v.program.Statements, stmt)
}
```

#### Building Qualified Names
Helper function for Module.Name parsing:

```go
func buildQualifiedName(ctx parser.IQualifiedNameContext) ast.QualifiedName {
    qn := ctx.(*parser.QualifiedNameContext)
    ids := qn.AllIDENTIFIER()

    if len(ids) == 1 {
        return ast.QualifiedName{Name: ids[0].GetText()}
    }
    return ast.QualifiedName{
        module: ids[0].GetText(),
        Name:   ids[1].GetText(),
    }
}
```

#### Error Handling
Syntax errors are collected via a custom error listener:

```go
type ErrorListener struct {
    *antlr.DefaultErrorListener
    Errors []error
}

func (e *ErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{},
    line, column int, msg string, ex antlr.RecognitionException) {
    e.Errors = append(e.Errors, fmt.Errorf("line %d:%d %s", line, column, msg))
}
```

### 5. Executor (`pkg/mdl/executor/executor.go`)

Executes AST statements against the modelsdk-go API.

```go
type Executor struct {
    writer *mpr.Writer
    output io.Writer
}

func (e *Executor) execute(stmt ast.Statement) error {
    switch s := stmt.(type) {
    case *ast.ConnectStmt:
        return e.executeConnect(s)
    case *ast.ShowStmt:
        return e.executeShow(s)
    case *ast.CreateEntityStmt:
        return e.executeCreateEntity(s)
    // ... other statement types
    }
}
```

**Integration with modelsdk-go:**

```go
func (e *Executor) executeCreateEntity(stmt *ast.CreateEntityStmt) error {
    // build domain model entity
    entity := &domainmodel.Entity{
        ID:   mpr.GenerateID(),
        Name: stmt.Name.Name,
        // ... other fields
    }

    // get module and add entity
    module := e.getOrCreateModule(stmt.Name.Module)
    dm := module.DomainModel
    dm.Entities = append(dm.Entities, entity)

    return nil
}
```

### 6. REPL (`pkg/mdl/repl/repl.go`)

Interactive read-eval-print loop for MDL commands.

```go
type REPL struct {
    executor *executor.Executor
    input    io.Reader
    output   io.Writer
}

func (r *REPL) run() error {
    scanner := bufio.NewScanner(r.input)
    for {
        fmt.Fprint(r.output, "mdl> ")
        if !scanner.Scan() {
            break
        }

        input := scanner.Text()
        prog, errs := visitor.Build(input)
        if len(errs) > 0 {
            // Handle parse errors
            continue
        }

        for _, stmt := range prog.Statements {
            if err := r.executor.Execute(stmt); err != nil {
                fmt.Fprintf(r.output, "error: %v\n", err)
            }
        }
    }
    return nil
}
```

### 7. CLI (`cmd/mxcli/main.go`)

Cobra-based command-line interface.

```go
var rootCmd = &cobra.Command{
    use:   "mxcli",
    Short: "Mendix CLI - Work with Mendix projects using MDL syntax",
    run: func(cmd *cobra.Command, args []string) {
        commands, _ := cmd.Flags().GetString("command")
        if commands != "" {
            // execute commands from -c flag
            exec := executor.New(os.Stdout)
            prog, _ := visitor.Build(commands)
            for _, stmt := range prog.Statements {
                exec.Execute(stmt)
            }
        } else {
            // Start interactive REPL
            repl.New(os.Stdin, os.Stdout).Run()
        }
    },
}
```

## Design Decisions

### Why ANTLR4?

| Consideration | ANTLR4 | Parser Combinators |
|---------------|--------|-------------------|
| Cross-language | ✅ Same grammar for Go, TS, Java | ❌ Rewrite per language |
| Grammar docs | ✅ EBNF-like, readable | ⚠️ Code is the doc |
| Error messages | ✅ Built-in recovery | ⚠️ Custom implementation |
| Performance | ✅ Optimized lexer/parser | ✅ Comparable |
| Tooling | ✅ ANTLR Lab, IDE plugins | ⚠️ Limited |

### Why Listener Pattern (not Visitor)?

- **Listener**: Callbacks fired during tree walk, simpler for AST building
- **Visitor**: Returns values from each node, better for expression evaluation

For MDL, statements are independent and don't need return value propagation, making the listener pattern more appropriate.

### Case-Insensitive Keywords

MDL follows SQL conventions with case-insensitive keywords. ANTLR handles this via fragment rules:

```antlr
show : S H O W ;
fragment S : [sS] ;
```

This allows `show`, `show`, `show`, etc. to all match the same token.

## Extending the Parser

### Adding a New Statement Type

1. **Update grammar** (`MDLLexer.g4` for tokens, `MDLParser.g4` for rules):
```antlr
ddlStatement
    : createStatement
    | newStatement      // add new statement
    ;

newStatement
    : NEW KEYWORD qualifiedName SEMI?    # NewKeyword
    ;

NEW : N E W ;
KEYWORD : K E Y W O R D ;
```

2. **Regenerate parser**:
```bash
make grammar
```

3. **Add AST type** (`ast/ast.go`):
```go
type NewKeywordStmt struct {
    Name QualifiedName
}

func (*NewKeywordStmt) statementNode() {}
```

4. **Update visitor** (`visitor/visitor.go`):
```go
func (v *Visitor) EnterNewKeyword(ctx *parser.NewKeywordContext) {
    stmt := &ast.NewKeywordStmt{
        Name: buildQualifiedName(ctx.QualifiedName()),
    }
    v.program.Statements = append(v.program.Statements, stmt)
}
```

5. **Update executor** (`executor/executor.go`):
```go
func (e *Executor) execute(stmt ast.Statement) error {
    switch s := stmt.(type) {
    // ... existing cases
    case *ast.NewKeywordStmt:
        return e.executeNewKeyword(s)
    }
}
```

## Testing

### Parser Tests

```go
func TestParseShowEntities(t *testing.T) {
    prog, errs := visitor.Build("show entities in MyModule")

    if len(errs) > 0 {
        t.Fatalf("unexpected errors: %v", errs)
    }

    if len(prog.Statements) != 1 {
        t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
    }

    show, ok := prog.Statements[0].(*ast.ShowStmt)
    if !ok {
        t.Fatalf("expected ShowStmt, got %T", prog.Statements[0])
    }

    if show.Type != "entities" || show.Module != "MyModule" {
        t.Errorf("unexpected statement: %+v", show)
    }
}
```

### Integration Tests

```go
func TestExecuteShowEntities(t *testing.T) {
    // create test MPR file
    // connect executor
    // execute show entities
    // Verify output
}
```

## Common Issues

### "no viable alternative at input" Error

Usually caused by:
1. Typo in grammar keyword definition (e.g., `entities : E N T I E S` missing letters)
2. Lexer rule ordering issues (longer matches should come first)
3. Missing whitespace handling

### Type Assertion Panics

When accessing ANTLR context methods, always use type assertions:

```go
// Wrong - will panic if ctx is nil or wrong type
ids := ctx.AllIDENTIFIER()

// Correct - check interface first
qn, ok := ctx.(*parser.QualifiedNameContext)
if !ok {
    return ast.QualifiedName{}
}
ids := qn.AllIDENTIFIER()
```

### Missing Tokens in Parse Tree

Check that:
1. Lexer rules are defined before `IDENTIFIER` rule
2. Keywords aren't being matched as identifiers
3. Whitespace is properly skipped

## Microflow Body Validation (`validate_microflow.go`)

Before execution, `mxcli check` runs AST-level semantic checks on microflow bodies via `ValidateMicroflow()`. These checks require no project connection — they operate purely on the parsed AST.

The `microflowValidator` struct walks the body and checks:

1. **Return value consistency** — RETURN must provide a value when the microflow declares a return type; RETURN must not provide a value on void microflows (except `return empty`).
2. **Return type plausibility** — Scalar literals (string, integer, boolean, decimal) cannot be returned from entity-typed microflows.
3. **Return path coverage** — All code paths must end with RETURN for non-void microflows. The `bodyReturns()` helper recursively checks whether the last statement in a body is a RETURN, or an IF/ELSE where both branches return.
4. **Variable scope** — Variables declared inside IF/ELSE branches or ON ERROR bodies cannot be referenced after the branch ends. The `checkBranchScoping()` method collects variables declared inside branches and checks if subsequent statements reference them.
5. **Validation feedback** — VALIDATION FEEDBACK must have a non-empty message template (CE0091).

This is separate from `ValidateMicroflowBody()` and `ValidateNanoflowBody()` (in `cmd_microflows_builder_validate.go`), which check undeclared variable usage and run during `--references` validation. Both delegate to the shared `validateFlowBody()` helper that accepts `[]ast.MicroflowParam` and `[]ast.MicroflowStatement`.

## Microflow Builder Architecture

The microflow builder (`cmd_microflows_builder.go`) converts MDL microflow AST nodes into Mendix microflow objects. A key aspect is **variable type tracking**. The `flowBuilder` also maintains `microflowsCache`/`nanoflowsCache` (with `Loaded` bool flags) for lazy-cached return type lookups, and `allNames()` includes nanoflows for forward-reference detection in multi-statement scripts.

### Variable Type Tracking (`varTypes`)

The `flowBuilder` struct maintains a `map[string]string` called `varTypes` that tracks the type of each variable during microflow construction. This is essential for building qualified names in CHANGE statements.

**Type Format:**
- Single entity: `"Module.Entity"` (e.g., `"MfTest.Product"`)
- List of entities: `"list of Module.Entity"` (e.g., `"list of MfTest.Product"`)

**Sources of Variable Types:**

| Source | Registration | Type Format |
|--------|-------------|-------------|
| Parameters (entity/list) | `cmd_microflows_create.go` | `"Module.Entity"` or `"list of Module.Entity"` |
| CREATE statement | `addCreateObjectAction` | `"Module.Entity"` (single) |
| RETRIEVE with LIMIT 1 | `addRetrieveAction` | `"Module.Entity"` (single) |
| RETRIEVE without LIMIT 1 | `addRetrieveAction` | `"list of Module.Entity"` (list) |
| FOREACH loop variable | `addLoopStatement` | Derived from list type |

**FOREACH Loop Variable Derivation:**

```go
// if $ProductList is "list of MfTest.Product", then $Product is "MfTest.Product"
listType := fb.varTypes[s.ListVariable]
if strings.HasPrefix(listType, "list of ") {
    elementType := strings.TrimPrefix(listType, "list of ")
    fb.varTypes[s.LoopVariable] = elementType
}
```

**Usage in CHANGE Statements:**

The `AttributeQualifiedName` field in `MemberChange` is built by looking up the variable's entity type:

```go
entityQN := fb.varTypes[s.Variable]  // e.g., "MfTest.Product"
memberChange.AttributeQualifiedName = entityQN + "." + change.Attribute
// Result: "MfTest.Product.LastProcessedDate"
```

### Common Pitfalls

1. **RETRIEVE Type Depends on LIMIT**: RETRIEVE with `limit 1` returns a single entity, otherwise it returns a list. The output variable must be registered accordingly. FOREACH loops require a list type to derive the element type.

2. **Variable Scope Sharing**: The `loopBuilder` shares the same `varTypes` map with its parent, so loop variable registrations are visible to nested statements.

3. **Order of Operations**: In `addLoopStatement`, the loop variable must be registered in `varTypes` **before** processing the loop body statements.

## Handling Nil Values from ANTLR Parser

**CRITICAL**: ANTLR parsers can return partial parse trees with `nil` nodes when there are syntax errors. Always check if grammar element getters return `nil` before calling methods on them.

### The Problem

When parsing malformed MDL like:
```mdl
create persistent entity Test.Broken (
  : string(100),    -- missing attribute name
  ValidAttr: integer
);
```

The ANTLR parser creates an `AttributeDefinitionContext` for the malformed line, but `attributename()` returns `nil` because there's no valid identifier. Code like this will panic:

```go
// DANGEROUS - will panic if attributename() returns nil
attr.Name = a.AttributeName().GetText()
```

### The Solution

Always add nil checks before accessing potentially-nil grammar elements:

```go
// SAFE - check for nil first
if a.AttributeName() == nil {
    b.addErrorWithExample(
        "Invalid attribute: each attribute must have a name and type",
        `  create persistent entity MyModule.Customer (
    Name: string(100) not null,
    Email: string(200),
    Age: integer
  );`)
    continue
}
attr.Name = a.AttributeName().GetText()
```

### Error Messages with Examples

Use `Builder.addErrorWithExample()` to provide helpful error messages that include example MDL syntax. This helps LLMs (and humans) understand the expected format:

```go
func (b *Builder) addErrorWithExample(message, example string) {
    b.errors = append(b.errors, fmt.Errorf("%s\n\nExpected syntax:\n%s", message, example))
}
```

The error output will look like:
```
Invalid attribute: each attribute must have a name and type

Expected syntax:
  create persistent entity MyModule.Customer (
    Name: string(100) not null,
    Email: string(200),
    Age: integer
  );
```

### Grammar Elements That May Return Nil

Common ANTLR context methods that can return `nil` on parse errors:
- `attributename()` - missing attribute identifier
- `EnumValueName()` - missing enumeration value identifier
- `QualifiedName()` - missing or malformed qualified name
- `DataType()` - missing type specification
- `expression()` - missing or malformed expression

**Rule of thumb**: Any grammar element that could be missing due to a syntax error should be checked for `nil` before use.

## References

- [ANTLR4 Documentation](https://github.com/antlr/antlr4/blob/master/doc/index.md)
- [ANTLR4 Go Target](https://github.com/antlr/antlr4/blob/master/doc/go-target.md)
- [MDL Syntax Reference](../07-references/mdl/MDL_SYNTAX_REFERENCE.md)
