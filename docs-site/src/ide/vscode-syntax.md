# Syntax Highlighting and Diagnostics

The VS Code extension provides three layers of language feedback: syntax highlighting, parse diagnostics, and semantic diagnostics.

## Syntax Highlighting

MDL files (`.mdl`) receive full syntax coloring based on a TextMate grammar:

| Token Type | Examples | Color Theme Category |
|-----------|---------|---------------------|
| Language header | `mdl 1;` | `keyword.control.directive` |
| Keywords | `CREATE`, `LIST`, `DESCRIBE`, `ALTER`, `GRANT` | `keyword` |
| Types | `String`, `Integer`, `Boolean`, `DateTime` | `support.type` |
| Strings | `'Hello World'` | `string` |
| Numbers | `100`, `3.14` | `constant.numeric` |
| Comments | `-- line comment`, `/** doc comment */` | `comment` |
| Identifiers | `MyModule.Customer` | `variable` |
| Operators | `=`, `!=`, `AND`, `OR` | `keyword.operator` |

Highlighting works immediately on file open -- no project or language server connection is required.

## Parse Diagnostics (Real-Time)

As you type, the extension sends the file content to the MDL language server, which runs the ANTLR4 parser and reports syntax errors in real time. These appear as:

- **Red underlines** on the offending tokens
- **Error entries** in the Problems panel (Ctrl+Shift+M)
- **Inline hover messages** when you point at the underlined text

Parse diagnostics catch issues like:

- Missing semicolons
- Unmatched parentheses or braces
- Invalid keyword combinations
- Malformed qualified names

Example error:

```
Line 3, Col 42: mismatched input ')' expecting ','
```

Parse diagnostics run **on every keystroke** (debounced) and do not require a Mendix project file.

## Language Version Diagnostics

The `mdl 1;` header a script starts with sets the language version it is checked
against. Deprecated spellings (`MDL-DEPR*`) and, in a script without the header,
constructs whose meaning `mdl 1` changes (`MDL-V1-*`) are underlined as warnings.
Hover over one for `mxcli help <code>`, and use the quick fix (Ctrl+.) to apply the
`fmt --upgrade` rewrite. See [LSP Capabilities](lsp-capabilities.md).

## Semantic Diagnostics (On Save)

When you save a `.mdl` file, the language server performs deeper validation against the actual Mendix project (`.mpr` file). Semantic diagnostics check:

- **Entity references** -- does `MyModule.Customer` actually exist?
- **Attribute references** -- does the entity have the named attribute?
- **Microflow references** -- do called microflows exist with correct signatures?
- **Association references** -- are FROM/TO entities valid?
- **Module existence** -- does the referenced module exist in the project?

Semantic diagnostics require:

1. A valid `mdl.mprPath` setting (or auto-discovered `.mpr` file)
2. The `mxcli lsp` server to be running

These diagnostics appear as warnings or errors in the Problems panel, identical in appearance to parse errors but typically with more descriptive messages referencing the project state.

## Diagnostic Severity Levels

| Severity | Meaning |
|----------|---------|
| **Error** | Syntax error or invalid reference that would prevent execution |
| **Warning** | Valid syntax but potentially problematic (e.g., unused variable) |
| **Information** | Suggestion or style recommendation |

## Equivalent CLI Commands

The same checks available in the extension can be run from the command line:

```bash
# Syntax check only (--no-project: do not auto-detect an .mpr in this directory)
mxcli check script.mdl --no-project

# Syntax + reference validation
mxcli check script.mdl -p app.mpr --references
```
