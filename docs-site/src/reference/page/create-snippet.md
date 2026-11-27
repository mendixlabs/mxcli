# CREATE SNIPPET

## Synopsis

```sql
CREATE [ OR REPLACE ] SNIPPET module.Name [ FOLDER 'path' ]
[(
    Params: ( $param : Module.Entity | Type [, ...] )
)]
{
    widget_tree
}
```

## Description

Creates a reusable widget fragment (snippet) in the specified module. Snippets contain a widget tree that can be embedded in pages or other snippets using the `SNIPPETCALL` widget.

If `OR REPLACE` is specified and a snippet with the same qualified name already exists, it is replaced.

Snippets support the same widget types and syntax as pages. The key difference is that snippets do not have a `Title` or `Layout` -- they are fragments meant to be included within a page's layout.

### Snippet Parameters

Snippets can optionally declare parameters. When a snippet has parameters, the `SNIPPETCALL` widget that embeds it must provide the corresponding values.

### Folder Placement

The optional `FOLDER 'path'` clause, after the name, places the snippet in a subfolder within the module.

## Parameters

`module.Name`
:   The qualified name of the snippet (`Module.SnippetName`). The module must already exist.

`Params: ( ... )`
:   Optional snippet parameters. Each parameter has a `$`-prefixed name and an
    **entity** type (`Module.Entity`) or, from **Mendix 11.17**, a primitive
    type (`String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`).

    An input binds a primitive parameter directly — `textbox tb (Attribute:
    $Label)` — and a text template reads it as `{1} = $Label`. The calling
    page passes a page parameter, a page variable or its own snippet
    parameter of the same type: `snippetcall sc (Snippet: M.S, Params: (Label
    = $Caption))`. No data context supplies a primitive, so it is always
    passed.

    Below 11.17 Mendix rejects a primitive snippet parameter with **CE0046**
    *"Invalid data type 'String'."*, so `mxcli check -p` and `exec` refuse it
    there (**MDL-VERSION01**) rather than letting it reach a build. On an older
    project, keep the primitive on the calling page's parameters, or pass an
    object and read the member inside the snippet.

`FOLDER 'path'`
:   Optional folder path within the module, as a clause after the name. The `Folder: 'path'` property is its deprecated alias (`MDL-DEPR105`).

## Examples

Simple snippet with a header:

```sql
CREATE SNIPPET MyModule.CustomerHeader
(
    Params: ( $Customer: MyModule.Customer )
)
{
    CONTAINER cntHeader (Class: 'card-header') {
        DATAVIEW dvCustomer (DataSource: $Customer) {
            DYNAMICTEXT txtName (Attribute: Name)
            DYNAMICTEXT txtEmail (Attribute: Email)
        }
    }
};
```

Snippet with form fields:

```sql
CREATE SNIPPET MyModule.AddressFields
(
    Params: ( $Address: MyModule.Address )
)
{
    DATAVIEW dvAddress (DataSource: $Address) {
        TEXTBOX txtStreet (Label: 'Street', Attribute: Street)
        TEXTBOX txtCity (Label: 'City', Attribute: City)
        TEXTBOX txtZip (Label: 'Zip Code', Attribute: ZipCode)
        TEXTBOX txtCountry (Label: 'Country', Attribute: Country)
    }
};
```

Snippet without parameters:

```sql
CREATE SNIPPET MyModule.AppFooter
{
    CONTAINER cntFooter (Class: 'app-footer') {
        DYNAMICTEXT txtVersion (Content: 'Version 1.0')
    }
};
```

Embedding a snippet in a page:

```sql
CREATE PAGE MyModule.Customer_Edit
(
    Params: ( $Customer: MyModule.Customer ),
    Title: 'Edit Customer',
    Layout: Atlas_Core.PopupLayout
)
{
    DATAVIEW dvCustomer (DataSource: $Customer) {
        SNIPPETCALL snpAddress (Snippet: MyModule.AddressFields)
        FOOTER {
            ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
        }
    }
};
```

## See Also

[CREATE PAGE](create-page.md), [ALTER PAGE](alter-page.md), [DROP PAGE](drop-page.md)
