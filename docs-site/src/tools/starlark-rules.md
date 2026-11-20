# Starlark Rules

In addition to the built-in Go rules, mxcli bundles 29 Starlark-based lint rules. Starlark is a Python-like language that allows rules to be extended and customized without recompiling mxcli.

## Bundled Starlark Rules

### Security Rules (SEC004-SEC009)

| Rule | Description |
|------|-------------|
| **SEC004** | Guest access -- Warns about overly permissive guest/anonymous access |
| **SEC005** | Strict mode -- Checks for strict security mode settings |
| **SEC006** | PII exposure -- Detects potentially sensitive data without restricted access |
| **SEC007** | Anonymous access -- Flags entities accessible to anonymous users |
| **SEC008** | Member restrictions -- Checks for overly broad member-level access |
| **SEC009** | Additional security rules |

### Architecture Rules (ARCH001-ARCH003)

| Rule | Description |
|------|-------------|
| **ARCH001** | Cross-module data -- Detects tight coupling between modules via direct data access. The System workflow entities (`System.Workflow*`) are exempt: a task page and a task inbox must use them |
| **ARCH002** | Microflow-based writes -- Ensures data modifications go through microflows |
| **ARCH003** | Entity business keys -- Checks that entities have meaningful business key attributes |

### Quality Rules (QUAL001-QUAL004)

| Rule | Description |
|------|-------------|
| **QUAL001** | McCabe complexity -- Flags microflows with high cyclomatic complexity |
| **QUAL002** | Documentation -- Missing documentation across every document type: modules, entities, pages, microflows, workflows, Java/JavaScript actions and their parameters, REST services, mappings, constants ([options](#qual002-options)) |
| **QUAL003** | Long microflows -- Warns about microflows with too many activities |
| **QUAL004** | Orphaned elements -- Detects unused entities, microflows, or pages ([options](#qual004-and-custom002-options)) |

### Design Rules (DESIGN001)

| Rule | Description |
|------|-------------|
| **DESIGN001** | Entity attribute count -- Warns when entities have too many attributes |

### Convention Rules (CONV001-CONV010, CONV015-CONV019)

| Rule | Description |
|------|-------------|
| **CONV001-CONV010** | Best practice conventions including boolean naming, page suffixes, enumeration prefixes, snippet prefixes |
| **CONV015** | Validation rules -- Checks for consistent validation patterns |
| **CONV016** | Event handlers -- Validates event handler configuration |
| **CONV017** | Calculated attributes -- Checks calculated attribute patterns |
| **CONV018** | Module folder organization -- A module past a readable size with every document loose in its root and no folders at all ([options](#conv018-options)) |
| **CONV019** | Navigation page URL -- A page a navigation profile routes to that has no URL, so it cannot be linked to or bookmarked ([details](#conv019-details)) |

Additional convention rules cover access rule constraints, role mapping, microflow size and content.

### QUAL002 options {#qual002-options}

Undocumented model elements are invisible to `mxcli check` and to the build —
nothing fails, so nothing reminds you. QUAL002 is that reminder, and it sweeps
**every document type a user authors**, not just the domain model.

**On by default** — one option per document type:

| Option | Element |
|---|---|
| `check_modules` | Module |
| `check_entities` | Entity |
| `check_pages` | Page |
| `check_snippets` | Snippet |
| `check_building_blocks` | Building block |
| `check_layouts` | Layout |
| `check_enumerations` | Enumeration |
| `check_microflows` | Microflow (nanoflows always exempt) |
| `check_java_actions` | Java action |
| `check_java_action_params` | Java action **parameter** |
| `check_javascript_actions` | JavaScript action |
| `check_workflows` | Workflow |
| `check_constants` | Constant |
| `check_image_collections` | Image collection |
| `check_data_transformers` | Data transformer |
| `check_business_event_services` | Business event service |
| `check_rest_clients` | Consumed REST client |
| `check_published_rest_services` | Published REST service |
| `check_json_structures` | JSON structure |
| `check_import_mappings` | Import mapping |
| `check_export_mappings` | Export mapping |

**Off by default** — members, suppressed for volume rather than for being
unimportant:

| Option | Element |
|---|---|
| `check_attributes` | Entity attribute |
| `check_associations` | Association |

Plus `min_activities` (default `3`): microflows with fewer activities are exempt,
on the grounds that a three-step flow's name says what a description would.

Why the split. A Java action has a handful of parameters, and Studio Pro renders
each description in the dialog where someone wires up the call — an undocumented
parameter is a blank field next to a name like `pInput` at exactly the moment a
caller has to decide what to pass. A domain model, by contrast, has hundreds of
attributes and associations, so the same check there is a wall of text rather
than a signal. Turn those two on when you are actively working through
documentation debt:

```yaml
rules:
  QUAL002:
    enabled: true
    options:
      check_attributes: true
      check_associations: true
      check_pages: false        # e.g. if page names are the convention here
      min_activities: 5
```

Elements in System and Marketplace modules are never reported — that is code you
did not write, and flagging Community Commons would bury the findings you can
act on. This applies to **every** rule, not just QUAL002: the `System` module is
not distinguishable by its `Source` (which is empty, exactly like your own
modules), so it used to slip past the Marketplace filter into every rule that
walks entities, pages, microflows or widgets. On a blank Mendix 9.24 project that
was the difference between 60 findings and 8.

Adding a document type to the sweep is two rows: one in `documentableSources`
(`mdl/linter/context.go`) naming the catalog table and its documentation column,
and one in `_DOC_KINDS` (`missing_documentation.star`) giving the option name and
the suggestion text. `TestQUAL002_SweepsEveryAdvertisedDocumentType` fails if the
Go side advertises a kind the tests do not cover.

### QUAL004 and CUSTOM002 options {#qual004-and-custom002-options}

Both rules decide by name prefix. QUAL004 (orphaned elements) does not report a
microflow whose name starts with an **entry-point prefix**, because something
outside the model calls it; CUSTOM002 (microflow prefix convention) reports a
microflow whose name starts with none of the **valid prefixes**. Projects name
things differently, so both lists are options — set them in `lint-config.yaml`
rather than in the rule files, which `mxcli init` rewrites:

| Rule | Option | Default |
|------|--------|---------|
| QUAL004 | `entry_point_prefixes` | `ACT_, SCH_, WS_, REST_, OData_` |
| CUSTOM002 | `valid_prefixes` | `ACT_, SUB_, DS_, VAL_, SCH_, IVK_, OCH_` |

```yaml
rules:
  QUAL004:
    options:
      entry_point_prefixes: [ACT_, SCH_, WS_, REST_, OData_, TEST_]
  CUSTOM002:
    options:
      valid_prefixes: "ACT_, SUB_, DS_, VAL_, SCH_, TEST_, GET_"
```

Write the list as a YAML list or as one comma-separated string; spaces around
the commas are ignored. A configured list **replaces** the default, so repeat the
default prefixes you still want. An empty list is a real setting: for QUAL004 it
means no microflow is exempt by name. A value of any other shape (a number, a
list holding a number) is not silently ignored — the rule runs with its default
and the run reports a warning naming the option, listed with the rules that could
not run and kept out of the score.

QUAL004 counts a microflow or page as used when anything in the catalog's
reference graph points at it — exactly what `mxcli refs` lists: a call, a page
or widget, the navigation (a home page or home microflow, a menu item), a
schedule, a published service, an event handler, a project setting. Every edge
kind counts, including ones the catalog adds later, so the rule cannot fall
behind it. A call from an MDL test script (`tests/*.test.mdl`) is not in the
model, so a helper used only by tests is reported; give such helpers a prefix
and list it in `entry_point_prefixes`. If `refs` finds nothing for a document
you know is live, the gap is in the catalog, not the rule.

### CONV018 options {#conv018-options}

Studio Pro lets a module hold folders, and every Mendix style guide expects them
once a module grows past a handful of documents. Nothing reported their absence,
so a project with 200 documents in one flat module scored clean.

CONV018 reports a module only when **both** halves hold:

1. more than `max_root_documents` documents sit directly in the module root, and
2. **not one** document in the module is in a folder.

The second half is what keeps the rule from nagging. A module that has started to
organise itself — even one folder — is never reported, however much is still at
its root: the team has evidently made a choice about where things go, and a
linter guessing at the rest is noise. The first half exempts modules that are
small rather than disorganised. Together they catch one thing: a module nobody
has ever organised.

| Option | Default | Meaning |
|--------|---------|---------|
| `max_root_documents` | `20` | Documents allowed at a module root before the module is expected to have folders |

```yaml
rules:
  CONV018:
    enabled: true
    options:
      max_root_documents: 40
```

Only the document kinds Studio Pro actually lets you file in a folder are
counted. Associations, entities and external entities are not documents — they
belong to the domain model or to a consumed service — so counting them would
report a module as unorganised on the strength of elements nobody can move. That
list is an allow-list in the rule (`FOLDERABLE_KINDS`), where it is visible and
editable: a document type missing from it is undercounted, so the rule stays
quiet rather than inventing a violation.

The projection behind it is the `documents()` builtin — every element of the App
Explorer tree as `(kind, name, qualified_name, module_name, folder)`, read from
the catalog's `objects` view so a new document type is covered without a second
list to keep in step. It is the companion to `documentable_elements()`, which
projects only what can carry documentation and therefore leaves out microflows
and Java actions — the two kinds that fill up an unorganised module.

### CONV019 details {#conv019-details}

A Mendix page is reachable at `/p/<url>` only if it has been given a URL.
Without one the page exists solely at the end of a click path: it cannot be
bookmarked, linked to from an email, reopened after a browser refresh, or
captured directly with `mxcli run --local --screenshot-url` — verifying one
screen means driving a browser through login and the menu.

**Why only navigation targets.** Measured on a blank Mendix 11.12.1 app, **0 of
16 pages carry a URL** — Mendix's own Home page and every Administration and
FeedbackModule page included. Reporting every page without one would warn about
every page of every project from the day it is created, which is noise rather
than a finding. The pages a profile routes to are different: they are the app's
top-level screens, the ones a user lands on, bookmarks and shares. A page
reached only from a button inside another screen is never reported.

Also never reported:

- the **login page** and the **not-found page** — the platform routes to those
  itself, so a URL on them buys nothing;
- **microflow-valued** menu items and home pages — the microflow decides what
  opens, so there is no page here to be addressable;
- targets in **System and Marketplace modules**.

A page routed to several ways — a home page that is also a menu item — is one
finding naming every route, not one finding per route.

The remedy is `Url: 'orders'` on `CREATE PAGE`, or the URL property in Studio
Pro. An app that deliberately has no deep links can turn the rule off:

```yaml
rules:
  CONV019:
    enabled: false
```

Behind it is the `navigation_targets()` builtin — `(profile, kind, role,
caption, page)` for every page a navigation profile routes to, where `kind` is
`home`, `role_home` or `menu`. Navigation was previously reachable only from the
Go rules, so no Starlark rule could ask which pages a user can actually reach.

## Where Starlark Rules Live

When you run `mxcli init`, Starlark rules are installed to:

```
your-project/
└── .claude/
    └── lint-rules/
        ├── sec004_guest_access.star
        ├── arch001_cross_module.star
        ├── qual001_complexity.star
        └── ...
```

## Running Starlark Rules

Starlark rules run automatically alongside built-in rules:

```bash
mxcli lint -p app.mpr
```

Use `--list-rules` to see all available rules:

```bash
mxcli lint -p app.mpr --list-rules
```
