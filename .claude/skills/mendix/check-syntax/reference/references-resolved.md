# What `check -p --references` resolves

Beyond the page, microflow, entity and icon names a statement names directly,
`check -p --references` resolves these against the project (mendixlabs/mxcli#1472):

- an `ALTER PAGE` `INSERT` / `REPLACE` / `DROP` target, and the flows, pages and
  snippets the inserted or replacing widgets name (a `SET` target is dry-run
  against the stored page);
- a DataGrid 2 column's `Attribute:` on a `database` grid;
- an access rule's `where […]` members;
- an entity event handler's microflow;
- `Module.Enum.Key` in an expression;
- a view entity's FROM / JOIN entities;
- the pages and flows a navigation profile or a menu document names: the home
  page (and one per user role), a home flow, the login and not-found pages, and
  every menu item's page, microflow or nanoflow at any depth. The target must
  exist, or be created by the script (further down only in a run that rolls
  back, as for a flow call); `exec` refuses the same statement
  before it rewrites the profile or menu.

## The project's Mendix version

`check -p` applies the version gates `exec` refuses with — the same
statement-to-feature table, the floors in `sdk/versions/*.yaml` (`show features`)
— as **MDL-VERSION01** (mendixlabs/mxcli#1434):

```
statement 5: create view entity requires Mendix 10.18.0+ (project is 9.24.30.51827) [MDL-VERSION01]
```

`exec` reports the same line in its pre-flight and writes nothing. Without a
project there is no version, so nothing is reported. Still exec-only: a gate that
depends on what the project stores (a `drop`/`alter message definition` of a
document that is not there), and the write-time property floor (`this Mendix X
project cannot store …`), which needs the encoded document.

## Not yet resolved

A passing check does not prove these; `mxcli docker check` is still the gate:

- a built-in input widget's `Attribute:` (`textbox (Attribute: Typo)`);
- any attribute inside an `ALTER PAGE` insert;
- an enumeration key inside a page widget's expression.
