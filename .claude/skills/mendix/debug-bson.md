# BSON Serialization Debugging Skill

This skill provides guidance for debugging BSON serialization issues when implementing or fixing Mendix SDK writers.

## When to Use This Skill

Use this skill when:
- Objects created via MDL don't appear correctly in Studio Pro
- Properties are missing, empty, or showing default values
- Widget captions, parameters, or nested structures aren't stored correctly
- You need to compare SDK-generated BSON with Studio Pro-generated BSON
- You encounter "empty" or "(Empty caption)" issues in Studio Pro
- Debugging BSON array version markers or object structures

## Overview

When the ModelSDK Go serializes objects to BSON, the output must exactly match what Mendix Studio Pro expects. Even small differences in:
- Field names (e.g., `Caption` vs `CaptionTemplate`)
- Object structures (e.g., string vs nested object)
- Array version markers (e.g., `[2, items...]` vs `[3, items...]`)

...can cause Studio Pro to ignore the data entirely.

## Part 1: The Debugging Workflow

### Step 1: Identify the Problem

Symptoms that indicate BSON serialization issues:
- MDL `DESCRIBE` command shows correct data, but Studio Pro shows empty/default
- Object is created but properties don't persist
- Nested structures (templates, parameters) are missing
- "(Empty caption)" or similar placeholder text in Studio Pro

### Step 2: Create a Reference Object in Studio Pro

1. Open the MPR project in Mendix Studio Pro
2. Manually create or fix the problematic object
3. Save the project (this writes the correct BSON)
4. Note the exact name/path of the fixed object

### Step 3: Dump Both BSON Structures

Use the `mxcli dump-bson` command to extract and compare:

```bash
# Dump the SDK-generated object (the broken one)
mxcli dump-bson -p app.mpr -o "PgTest.BrokenPage" > broken.json

# Dump the Studio Pro-generated object (the fixed one)
mxcli dump-bson -p app.mpr -o "PgTest.FixedPage" > fixed.json

# Compare the two
diff broken.json fixed.json
```

Or use the `--compare` flag:
```bash
mxcli dump-bson -p app.mpr --compare "PgTest.BrokenPage" "PgTest.FixedPage"
```

### Step 4: Identify Differences

Look for differences in:

| Area | Common Issues |
|------|---------------|
| Field names | `Caption` vs `CaptionTemplate`, `Name` vs `InternalName` |
| Object vs value | String `""` vs nested `{$Type: "..."}` object |
| Array markers | `[2, ...]` vs `[3, ...]` for different contexts |
| Missing fields | Required fields that SDK omits |
| Type mismatches | `int32` vs `int64`, `string` vs `binary` |

### Step 5: Fix the Serialization Code

The serialization code lives in `sdk/mpr/writer_*.go` files:
- `writer_widgets.go` - Page widget serialization
- `writer_microflows.go` - Microflow activity serialization
- `writer_entities.go` - Entity/attribute serialization

## Part 2: Common BSON Patterns

### Array Version Markers

Mendix uses version markers at the start of arrays:

```go
// Empty array (version 3 marker)
bson.A{int32(3)}

// Non-empty array - context dependent!
// Parameters arrays use version 2
bson.A{int32(2), item1, item2, ...}

// Texts$Text.Items arrays use version 3
bson.A{int32(3), item1, item2, ...}
```

**Critical**: The version marker differs by context. Always check a working Studio Pro example.

### Object Structures

Some fields expect nested objects, not simple values:

```go
// WRONG: String value
{Key: "FallbackValue", Value: ""}

// CORRECT: Nested Texts$Text object
{Key: "Fallback", Value: bson.D{
    {Key: "$ID", Value: idToBsonBinary(generateUUID())},
    {Key: "$Type", Value: "Texts$Text"},
    {Key: "Items", Value: bson.A{int32(3)}},
}}
```

### Field Naming

Always use exact field names from Studio Pro:

```go
// WRONG: Simplified name
{Key: "Caption", Value: caption}

// CORRECT: Full field name
{Key: "CaptionTemplate", Value: caption}
```

## Part 3: Key Structures Reference

### Forms$ClientTemplate

Used for templated text like button captions with parameters:

```go
func serializeClientTemplate(ct *pages.ClientTemplate) bson.D {
    captionID := ct.ID
    if captionID == "" {
        captionID = generateUUID()
    }

    // Build template text
    template := bson.D{
        {Key: "$ID", Value: idToBsonBinary(generateUUID())},
        {Key: "$Type", Value: "Texts$Text"},
        {Key: "Items", Value: bson.A{int32(3), /* TextItem objects */}},
    }

    // Build fallback (required, even if empty)
    fallback := bson.D{
        {Key: "$ID", Value: idToBsonBinary(generateUUID())},
        {Key: "$Type", Value: "Texts$Text"},
        {Key: "Items", Value: bson.A{int32(3)}}, // Empty fallback
    }

    // Build parameters array
    params := bson.A{int32(3)} // Empty by default
    if len(ct.Parameters) > 0 {
        params = bson.A{int32(2)} // Non-empty uses version 2!
        for _, param := range ct.Parameters {
            params = append(params, serializeClientTemplateParameter(param))
        }
    }

    return bson.D{
        {Key: "$ID", Value: idToBsonBinary(captionID)},
        {Key: "$Type", Value: "Forms$ClientTemplate"},
        {Key: "Fallback", Value: fallback},    // Must be object, not string
        {Key: "Parameters", Value: params},
        {Key: "Template", Value: template},
    }
}
```

### ActionButton CaptionTemplate

```go
// In serializeActionButton:
{Key: "CaptionTemplate", Value: caption}, // NOT "Caption"!
```

## Part 4: Debugging Tools

### Go Program for Raw BSON Dump

Create a temporary Go program to inspect raw BSON:

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"

    "github.com/mendix/modelsdk-go/sdk/mpr"
)

func main() {
    reader, _ := mpr.NewReader(os.Args[1])
    defer reader.Close()

    docs, _ := reader.GetDocuments()
    for _, doc := range docs {
        if doc.Name == os.Args[2] { // Target object name
            pretty, _ := json.MarshalIndent(doc.RawBSON, "", "  ")
            fmt.Println(string(pretty))
        }
    }
}
```

### Using mxcli dump-bson

```bash
# List all pages in project
mxcli dump-bson -p app.mpr --type page --list

# List all microflows
mxcli dump-bson -p app.mpr --type microflow --list

# Dump specific page as JSON
mxcli dump-bson -p app.mpr --type page --object "PgTest.MyPage"

# Save dump to file for comparison
mxcli dump-bson -p app.mpr --type page --object "PgTest.MyPage" > mypage.json

# Compare two objects (outputs both as JSON)
mxcli dump-bson -p app.mpr --type page --compare "PgTest.Broken,PgTest.Fixed"

# Supported types: page, microflow, nanoflow, enumeration, snippet, layout
```

## Part 5: Checklist for BSON Fixes

Before considering a fix complete:

- [ ] Create reference object manually in Studio Pro
- [ ] Dump both BSON structures (SDK vs Studio Pro)
- [ ] Identify all differences
- [ ] Fix field names to match Studio Pro exactly
- [ ] Fix object structures (nested vs value)
- [ ] Fix array version markers
- [ ] Test: Create object via MDL, verify in Studio Pro
- [ ] Update documentation (`docs/05-mdl-specification/10-bson-mapping.md`)

## Part 6: Common Issues and Solutions

| Symptom | Likely Cause | Solution |
|---------|--------------|----------|
| "(Empty caption)" in Studio Pro | Wrong field name or structure | Check `CaptionTemplate` vs `Caption`, verify nested object |
| Parameters not showing | Wrong array version marker | Use `[2, ...]` for non-empty Parameters |
| Template text missing | Missing Texts$Text structure | Ensure proper TextItem serialization |
| Fallback empty | Using string instead of object | Use `Fallback` with Texts$Text object |
| Widget property ignored | Field name mismatch | Compare with Studio Pro BSON exactly |
| Columns not in Page Explorer | Incomplete nested WidgetObjects | Create ALL properties from template |
| TypeCacheUnknownTypeException | Wrong BSON $Type name | Use `Texts$Translation` not `Texts$TextItem` |
| CE0642 "Property is required" | Wrong value format or missing | Check ValueType.Type in template, use correct BSON field |
| NullReferenceException in GetExpectedExpressionType | Using Expression for non-Expression type | Use `PrimitiveValue` for Boolean/Enum/Integer types |
| CE0495 Duplicate name errors | Same widget in multiple properties | Set content/filter to empty widget arrays |

## Part 7: Nested WidgetObjects (Critical)

**Key Insight**: Pluggable widgets with nested objects (like DataGrid2 columns) require **ALL properties** to be created, not just the ones with explicit values.

### The Problem

When creating nested `WidgetObject` instances (e.g., DataGrid2 columns), creating only the properties with explicit values results in:
- Objects that don't appear in the Page Explorer
- CE0463 "widget definition has changed" errors
- Widgets that render in the editor but are incomplete

### Example: DataGrid2 Columns

**Symptom**: Columns show in the page editor but NOT in the Page Explorer tree.

**Cause**: Column objects have 5 properties instead of the required 21.

```bash
# Compare mxcli-generated vs Studio Pro-generated
mxcli dump-bson -p app.mpr --compare "PgTest.MDLPage,PgTest.StudioProPage"

# Look for property count differences:
# ~ Properties: array length differs (first: 5, second: 22)
```

**Solution**: The embedded templates at `sdk/widgets/templates/mendix-11.6/datagrid.json` contain all 21 column properties:

```
showContentAs, attribute, content, dynamicText, exportValue, header, tooltip,
filter, visible, sortable, resizable, draggable, hidable, allowEventPropagation,
width, minWidth, minWidthLimit, size, alignment, columnClass, wrapText
```

When building columns, iterate through ALL `PropertyTypes` in the template's `ObjectType` and create a `WidgetProperty` for each one, using default values for properties without explicit values.

### Diagnostic Steps

1. Count properties in both versions:
   ```bash
   mxcli dump-bson -p app.mpr --type page --object "PgTest.BrokenPage" | grep "WidgetProperty" | wc -l
   mxcli dump-bson -p app.mpr --type page --object "PgTest.FixedPage" | grep "WidgetProperty" | wc -l
   ```

2. Check the template for required properties:
   ```bash
   grep '"PropertyKey"' sdk/widgets/templates/mendix-11.6/datagrid.json | head -30
   ```

3. Compare specific nested objects using `--compare` flag to find property count mismatches.

## Part 8: Property Value Types (Critical)

**Key Insight**: Pluggable widget properties require different value formats based on their `ValueType.Type` field in the widget template. Using the wrong format causes CE0642 "Property is required" errors or NullReferenceException.

### Determining the Correct Format

Check the `ValueType.Type` field in the widget template JSON:

```json
{
  "PropertyKey": "visible",
  "ValueType": {
    "Type": "Expression",
    "ReturnType": "Boolean"
  }
}
```

### Value Format by Type

| ValueType.Type | BSON Field | Example Value | Notes |
|----------------|------------|---------------|-------|
| `Expression` | `Expression` | `"true"`, `"$currentObject/Name"` | String expression, NOT evaluated |
| `Boolean` | `PrimitiveValue` | `"true"`, `"false"` | String representation |
| `Enumeration` | `PrimitiveValue` | `"left"`, `"autoFill"` | Enum value name |
| `Integer` | `PrimitiveValue` | `"100"`, `"0"` | String representation |
| `Decimal` | `PrimitiveValue` | `"10.5"` | String representation |
| `String` | `PrimitiveValue` | `"text value"` | Direct string |
| `Widgets` | `Widgets` | `bson.A{...}` | Array of widget objects |
| `Object` | `Objects` | `bson.A{...}` | Array of WidgetObject |

### Example: DataGrid2 Column Properties

```go
// WRONG: Using Expression for Boolean-type property
// Causes: NullReferenceException in GetExpectedExpressionType
{Key: "sortable", Value: bson.D{
    {Key: "Expression", Value: "true"},  // WRONG!
}}

// CORRECT: Using PrimitiveValue for Boolean-type property
{Key: "sortable", Value: bson.D{
    {Key: "PrimitiveValue", Value: "true"},  // Correct!
}}
```

### DataGrid2 Column Property Types Reference

| Property | ValueType.Type | BSON Field | Default Value |
|----------|---------------|------------|---------------|
| `visible` | Expression | `Expression` | `"true"` |
| `sortable` | Boolean | `PrimitiveValue` | `"true"` |
| `resizable` | Boolean | `PrimitiveValue` | `"true"` |
| `draggable` | Boolean | `PrimitiveValue` | `"true"` |
| `wrapText` | Boolean | `PrimitiveValue` | `"false"` |
| `hidable` | Enumeration | `PrimitiveValue` | `"yes"` |
| `alignment` | Enumeration | `PrimitiveValue` | `"left"` |
| `width` | Enumeration | `PrimitiveValue` | `"autoFill"` |
| `minWidth` | Enumeration | `PrimitiveValue` | `"auto"` |
| `size` | Integer | `PrimitiveValue` | `"100"` |
| `header` | Object | `Objects` | Empty translation |
| `content` | Widgets | `Widgets` | Empty widget array |
| `filter` | Widgets | `Widgets` | Empty widget array |

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| CE0642 "Property 'X' is required" | Missing property or wrong value format | Check ValueType.Type, use correct BSON field |
| NullReferenceException in GetExpectedExpressionType | Using `Expression` for non-Expression type | Use `PrimitiveValue` for Boolean/Enum/Integer |
| CE0463 "widget definition has changed" | Missing properties | Create ALL properties from template |

### How to Find ValueType.Type

```bash
# Check the embedded widget template
grep -A5 '"PropertyKey": "visible"' sdk/widgets/templates/mendix-11.6/datagrid.json

# Or use jq to extract all property types
jq '.PropertyTypes[] | {key: .PropertyKey, type: .ValueType.Type}' sdk/widgets/templates/mendix-11.6/datagrid.json
```

## Part 9: Texts$Translation vs Texts$TextItem

**Critical**: The correct type for translatable text items is `Texts$Translation`, NOT `Texts$TextItem`.

### The Error

```
TypeCacheUnknownTypeException: The type cache does not contain a type with qualified name Texts$TextItem
```

### Correct Structure

```go
// WRONG: Texts$TextItem does not exist
{Key: "$Type", Value: "Texts$TextItem"}

// CORRECT: Use Texts$Translation with LanguageCode
{Key: "$Type", Value: "Texts$Translation"},
{Key: "LanguageCode", Value: "en_US"},
{Key: "Text", Value: "Your text here"},
```

### Full Example: Building a Header Translation

```go
headerTranslation := bson.D{
    {Key: "$ID", Value: idToBsonBinary(generateUUID())},
    {Key: "$Type", Value: "Texts$Translation"},
    {Key: "LanguageCode", Value: "en_US"},
    {Key: "Text", Value: columnHeader},
}

headerText := bson.D{
    {Key: "$ID", Value: idToBsonBinary(generateUUID())},
    {Key: "$Type", Value: "Texts$Text"},
    {Key: "Items", Value: bson.A{int32(3), headerTranslation}},
}
```

## Part 10: CE0463 TextTemplate Troubleshooting

**CE0463 "widget definition has changed"** is one of the most common errors when creating pluggable widgets programmatically. For filter widgets, this error is often caused by TextTemplate properties being `null` instead of proper `Forms$ClientTemplate` structures.

### Identifying TextTemplate Properties

1. **Check the Type section** for properties with `"Type": "TextTemplate"`:
   ```json
   {
     "PropertyKey": "placeholder",
     "ValueType": {
       "$ID": "abc123...",
       "$Type": "CustomWidgets$WidgetValueType",
       "Type": "TextTemplate"  // <-- This is a TextTemplate property
     }
   }
   ```

2. **Find the matching Object property** using the TypePointer:
   ```json
   {
     "TypePointer": "abc123...",  // Matches ValueType.$ID above
     "Value": {
       "TextTemplate": null  // <-- WRONG! Causes CE0463
     }
   }
   ```

### Required TextTemplate Structure

Every TextTemplate property must have this structure (never null):

```json
"TextTemplate": {
  "$ID": "<32-char-guid>",
  "$Type": "Forms$ClientTemplate",
  "Fallback": {
    "$ID": "<32-char-guid>",
    "$Type": "Texts$Text",
    "Items": []
  },
  "Parameters": [],
  "Template": {
    "$ID": "<32-char-guid>",
    "$Type": "Texts$Text",
    "Items": []
  }
}
```

### Critical: Empty Array Serialization

**WRONG** - `[2]` in JSON serializes as an array containing the integer 2:
```json
"Items": [2]      // Creates array with one element: the number 2
"Parameters": [2] // Creates array with one element: the number 2
```

**CORRECT** - Use truly empty arrays:
```json
"Items": []       // Truly empty array
"Parameters": []  // Truly empty array
```

The version markers (like `[2]` or `[3]`) only exist in BSON wire format, not in JSON template files.

### Affected Filter Widgets

| Widget | TextTemplate Properties |
|--------|------------------------|
| TextFilter | `placeholder`, `screenReaderButtonCaption`, `screenReaderInputCaption` |
| DateFilter | `placeholder`, `screenReaderButtonCaption`, `screenReaderCalendarCaption`, `screenReaderInputCaption` |
| DropdownFilter | `emptyOptionCaption`, `ariaLabel`, `emptySelectionCaption`, `filterInputPlaceholderCaption` |
| NumberFilter | `placeholder`, `screenReaderButtonCaption`, `screenReaderInputCaption` |

### Python Script to Find TextTemplate Properties

Use this script to identify which Object properties need TextTemplate structures:

```python
import json

with open('widget-template.json') as f:
    data = json.load(f)

# Extract ValueType IDs for TextTemplate properties
text_template_ids = {}
for prop_type in data['type']['ObjectType']['PropertyTypes']:
    vt = prop_type.get('ValueType', {})
    if vt.get('Type') == 'TextTemplate':
        text_template_ids[vt['$ID']] = prop_type['PropertyKey']

print("TextTemplate properties:", text_template_ids)

# Find matching Object properties with null TextTemplate
for prop in data['object']['Properties']:
    type_ptr = prop['Value'].get('TypePointer')
    if type_ptr in text_template_ids:
        text_template = prop['Value'].get('TextTemplate')
        if text_template is None:
            print(f"NEEDS FIX: {text_template_ids[type_ptr]} (TypePointer: {type_ptr})")
```

### Verification Steps

After fixing templates:

1. Create a test page with the widget
2. Run `mx check app.mpr` - should return 0 errors
3. Open in Studio Pro - widget should load without "Update widget" prompt

### Reading TextTemplate in executor code

Part 10 above covers writing `Forms$ClientTemplate`. The same structure applies when **reading** it in executor code (e.g. `cmd_alter_page.go`, `cmd_pages_describe_output.go`).

The correct traversal is:
```
TextTemplate (Forms$ClientTemplate) → Template (Texts$Text) → Items[] → Translation { Text }
```

**Do not** read `Items` directly off the `TextTemplate` document — that skips the intermediate `Template` node and silently returns an empty slice. Always traverse one level deeper:

```go
// WRONG — Items is always empty
items := dGetArrayElements(dGet(tmplDoc, "Items"))

// CORRECT — traverse Template first
template := dGetDoc(tmplDoc, "Template")
items := dGetArrayElements(dGet(template, "Items"))
```

This applies to any executor function that reads column headers, button captions, or any other translatable text stored as `Forms$ClientTemplate`.

## Related Documentation

- [BSON Mapping Specification](../../docs/05-mdl-specification/10-bson-mapping.md)
- [Page Widget Serialization](../../sdk/mpr/writer_widgets.go)
- [Create Page Skill](./create-page.md)
- [Widget Templates README](../../sdk/widgets/templates/README.md)

## Quick Reference

### Debugging Command Sequence

```bash
# 1. Find the broken object
mxcli -p app.mpr -c "DESCRIBE PAGE PgTest.BrokenPage"

# 2. Create fixed version in Studio Pro, save project

# 3. Dump both objects to JSON files
mxcli dump-bson -p app.mpr --type page --object "PgTest.BrokenPage" > broken.json
mxcli dump-bson -p app.mpr --type page --object "PgTest.FixedPage" > fixed.json

# 4. Compare the JSON files
diff broken.json fixed.json
# Or use a visual diff tool like VS Code:
code --diff broken.json fixed.json

# 5. After fixing code, verify
go build ./... && mxcli exec test.mdl -p app.mpr

# 6. Verify in Studio Pro (open project, check object)
```

### Key File Locations

| File | Purpose |
|------|---------|
| `sdk/mpr/writer_widgets.go` | Page widget BSON serialization |
| `sdk/mpr/writer_microflows.go` | Microflow BSON serialization |
| `sdk/mpr/writer_entities.go` | Entity BSON serialization |
| `docs/05-mdl-specification/10-bson-mapping.md` | BSON format documentation |
