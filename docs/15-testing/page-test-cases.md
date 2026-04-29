# Page, Snippet & Layout Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Pages | Snippets | Layouts |
|-----|-----------|-------|----------|---------|
| Lato Enquiry Management | 11.4.0 | <N> | <N> | <N> |
| Evora - Factory Management | 10.24.15 | <N> | <N> | <N> |
| Lato Product Inventory | 11.2.0 | <N> | <N> | <N> |

---

## Setup

### 1. Download test apps

1. Go to [Mendix App Gallery](https://appgallery.mendixcloud.com/)
2. Download each demo app listed above
3. Open each `.mpk` in Studio Pro to extract the `.mpr` file

### 2. Build mxcli

```bash
make build && make test && make lint-go
```

### 3. Smoke test

```bash
APPS_DIR=<path-to-extracted-apps>
for mpr in "$APPS_DIR"/*/*.mpr; do
  echo "=== $(basename $(dirname $mpr)) ==="
  echo "show pages;" > /tmp/show-pages.mdl
  mxcli exec /tmp/show-pages.mdl -p "$mpr" 2>&1 | tail -1
done
```

Expected: count line `(N pages)` for each project.

### 4. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

### 5. Script-based testing

```bash
mxcli exec test-sequence.mdl -p <mpr>
```

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder.
> Dropped pages/snippets cannot be recovered.
>
> ```bash
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW PAGES

### 1.1 List all pages

```
show pages;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Excluded | Folder | Title | URL | Params`. Summary `(N pages)`.

### 1.2 List pages in a module

```
show pages in Administration;
```

**Expected:** Only pages from `Administration` module.

### 1.3 Empty module

```
show pages in NonExistentModule;
```

**Expected:** Empty result or `(0 pages)`.

---

## 2. SHOW SNIPPETS

### 2.1 List all snippets

```
show snippets;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Folder | Params`. Summary `(N snippets)`.

### 2.2 List snippets in a module

```
show snippets in MyModule;
```

**Expected:** Only snippets from specified module.

---

## 3. SHOW LAYOUTS

### 3.1 List all layouts

```
show layouts;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Folder | Type`. Summary `(N layouts)`.

### 3.2 List layouts in a module

```
show layouts in Atlas_Core;
```

**Expected:** Only layouts from specified module.

---

## 4. SHOW WIDGETS

### 4.1 List all widgets

```
show widgets;
```

**Expected:** Table with columns: NAME, widget type, container, module.

### 4.2 Filter by widget type

```
show widgets where WidgetType = 'Forms$DataView';
```

**Expected:** Only DataView widgets listed.

### 4.3 Filter with LIKE

```
show widgets where Name LIKE '%button%';
```

**Expected:** Widgets with "button" in name.

### 4.4 Filter by module

```
show widgets in MyModule;
```

**Expected:** Only widgets from specified module.

---

## 5. DESCRIBE PAGE

### 5.1 Basic page

```
describe page Administration.AccountOverview;
```

**Expected:** Full MDL V3 output:
```
create or modify page Administration.AccountOverview (
  Title: 'Account Overview',
  Layout: Atlas_Core.Atlas_Default,
  Url: 'account-overview'
) {
  -- widget tree
}
/
```

### 5.2 Page with parameters

```
describe page <Module.PageWithParams>;
```

**Expected:** `Params: { $ParamName: Module.Entity }` in header.

### 5.3 Page with variables

```
describe page <Module.PageWithVars>;
```

**Expected:** `Variables: { $varName: Boolean = 'expression' }` in header.

### 5.4 Page with folder

```
describe page <Module.PageInFolder>;
```

**Expected:** `Folder: 'path/to/folder'` in header.

### 5.5 Page with security roles

```
describe page <Module.SecuredPage>;
```

**Expected:** Output ends with `grant view on page Module.Name to Role1, Role2;`.

### 5.6 Non-existent page

```
describe page Fake.Missing;
```

**Expected:** Error — not found.

---

## 6. DESCRIBE SNIPPET

### 6.1 Basic snippet

```
describe snippet <Module.SnippetName>;
```

**Expected:** `create or modify snippet Module.Name (Params: {...}) { widget tree }`.

### 6.2 Snippet with parameters

```
describe snippet <Module.ParamSnippet>;
```

**Expected:** Parameters listed in header.

---

## 7. DESCRIBE LAYOUT

### 7.1 Layout description

```
describe layout Atlas_Core.Atlas_Default;
```

**Expected:** Comment-only representation. Includes note that layouts cannot be created via MDL.

---

## 8. CREATE PAGE

### 8.1 Minimal page

```
create page MyModule.SimplePage (
  Title: 'Simple Page',
  Layout: Atlas_Core.Atlas_Default
) {
  text 'Hello World';
}
```

**Expected:** Page created. `show pages in MyModule` includes it.

### 8.2 Page with URL

```
create page MyModule.ProductDetail (
  Title: 'Product Detail',
  Layout: Atlas_Core.Atlas_Default,
  Url: 'product/{Product/Name}',
  Params: { $Product: MyModule.Product }
) {
  dataview dv1 (DataSource: $Product) {
    textbox (Attribute: Name);
    textbox (Attribute: Price);
  }
}
```

**Expected:** Page created with URL and parameter.

### 8.3 Page with DataView

```
create page MyModule.EntityPage (
  Title: 'Entity Page',
  Layout: Atlas_Core.Atlas_Default,
  Params: { $Item: MyModule.Item }
) {
  dataview main (DataSource: $Item) {
    textbox tbName (Attribute: Name);
    textarea tbDesc (Attribute: Description);
    datepicker dpDate (Attribute: CreatedDate);
    dropdown ddStatus (Attribute: Status);
    checkbox cbActive (Attribute: IsActive);
  }
}
```

**Expected:** Page with DataView containing input widgets bound to attributes.

### 8.4 Page with DataGrid

```
create page MyModule.ListPage (
  Title: 'Items',
  Layout: Atlas_Core.Atlas_Default
) {
  datagrid grid1 (DataSource: DATABASE MyModule.Item) {
    column colName (Attribute: Name, Caption: 'Name');
    column colPrice (Attribute: Price, Caption: 'Price', Alignment: right);
  }
}
```

**Expected:** Page with DataGrid2 and columns.

### 8.5 Page with LayoutGrid (rows/columns)

```
create page MyModule.GridPage (
  Title: 'Grid Layout',
  Layout: Atlas_Core.Atlas_Default
) {
  layoutgrid {
    row {
      column (Weight: 6) {
        text 'Left side';
      }
      column (Weight: 6) {
        text 'Right side';
      }
    }
  }
}
```

**Expected:** Page with responsive grid layout.

### 8.6 Page with buttons and actions

```
create page MyModule.ActionPage (
  Title: 'Actions',
  Layout: Atlas_Core.Atlas_Default,
  Params: { $Item: MyModule.Item }
) {
  dataview dv (DataSource: $Item) {
    button btnSave (Caption: 'Save', Action: SAVE_CHANGES CLOSE_PAGE);
    button btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES CLOSE_PAGE);
    button btnDelete (Caption: 'Delete', Action: DELETE_OBJECT);
    button btnOpen (Caption: 'Open', Action: SHOW_PAGE MyModule.DetailPage);
    button btnRun (Caption: 'Run', Action: MICROFLOW MyModule.MF_Process);
  }
}
```

**Expected:** Buttons with different action types.

### 8.7 Page with TabContainer

```
create page MyModule.TabbedPage (
  Title: 'Tabs',
  Layout: Atlas_Core.Atlas_Default
) {
  tabcontainer {
    tabpage (Caption: 'General') {
      text 'General content';
    }
    tabpage (Caption: 'Details') {
      text 'Detail content';
    }
  }
}
```

**Expected:** Tab container with two tab pages.

### 8.8 Page with SnippetCall

```
create page MyModule.WithSnippet (
  Title: 'With Snippet',
  Layout: Atlas_Core.Atlas_Default
) {
  snippetcall MyModule.HeaderSnippet;
}
```

**Expected:** Snippet call widget referencing existing snippet.

### 8.9 Page with conditional visibility

```
create page MyModule.ConditionalPage (
  Title: 'Conditional',
  Layout: Atlas_Core.Atlas_Default,
  Variables: { $ShowExtra: Boolean = 'false' }
) {
  text 'Always visible';
  container (VisibleIf: '$ShowExtra') {
    text 'Conditionally visible';
  }
}
```

**Expected:** Widget with conditional visibility expression.

### 8.10 Page in folder

```
create page MyModule.FolderPage (
  Title: 'In Folder',
  Layout: Atlas_Core.Atlas_Default,
  Folder: 'Admin/Reports'
) {
  text 'Content';
}
```

**Expected:** Page placed in specified folder.

### 8.11 `create or modify` — existing page

```
create or modify page MyModule.SimplePage (
  Title: 'Updated Title',
  Layout: Atlas_Core.Atlas_Default
) {
  text 'Updated content';
}
```

**Expected:** Updates existing page (preserves ID).

### 8.12 `create or replace` — drop and recreate

```
create or replace page MyModule.SimplePage (
  Title: 'Replaced',
  Layout: Atlas_Core.Atlas_Default
) {
  text 'New content';
}
```

**Expected:** Drops and recreates (new ID).

### 8.13 Duplicate page error

```
create page MyModule.SimplePage (
  Title: 'Dup',
  Layout: Atlas_Core.Atlas_Default
) {
  text 'X';
}
```

**Expected:** Error — page already exists.

---

## 9. CREATE SNIPPET

### 9.1 Minimal snippet

```
create snippet MyModule.SimpleSnippet () {
  text 'Snippet content';
}
```

**Expected:** Snippet created.

### 9.2 Snippet with parameters

```
create snippet MyModule.ItemCard (
  Params: { $Item: MyModule.Item }
) {
  dataview (DataSource: $Item) {
    textbox (Attribute: Name);
    textbox (Attribute: Price);
  }
}
```

**Expected:** Snippet with entity parameter and DataView.

### 9.3 Snippet in folder

```
create snippet MyModule.FolderSnippet (
  Folder: 'Shared/Components'
) {
  text 'In folder';
}
```

**Expected:** Snippet placed in folder.

---

## 10. DROP PAGE / DROP SNIPPET

### 10.1 Drop existing page

```
drop page MyModule.SimplePage;
```

**Expected:** Page removed.

### 10.2 Drop non-existent page

```
drop page MyModule.Fake;
```

**Expected:** Error — not found.

### 10.3 Drop existing snippet

```
drop snippet MyModule.SimpleSnippet;
```

**Expected:** Snippet removed.

### 10.4 Drop non-existent snippet

```
drop snippet MyModule.Fake;
```

**Expected:** Error — not found.

---

## 11. ALTER PAGE

### 11.1 SET widget property

```
alter page MyModule.EntityPage {
  SET tbName.Placeholder = 'Enter name here';
};
```

**Expected:** Widget property updated. `describe` shows new value.

### 11.2 SET DataSource

```
alter page MyModule.ListPage {
  SET grid1.DataSource = MICROFLOW MyModule.MF_GetItems;
};
```

**Expected:** DataGrid source changed to microflow.

### 11.3 INSERT widget

```
alter page MyModule.EntityPage {
  INSERT INTO dv1 POSITION last {
    button btnNew (Caption: 'Submit', Action: SAVE_CHANGES);
  }
};
```

**Expected:** Button added as last child of DataView.

### 11.4 INSERT widget at first position

```
alter page MyModule.EntityPage {
  INSERT INTO dv1 POSITION first {
    title (Caption: 'Edit Item');
  }
};
```

**Expected:** Title widget inserted as first child.

### 11.5 DROP widget

```
alter page MyModule.EntityPage {
  DROP WIDGET tbDesc;
};
```

**Expected:** TextArea widget removed from page.

### 11.6 DROP multiple widgets

```
alter page MyModule.EntityPage {
  DROP WIDGET dpDate, cbActive;
};
```

**Expected:** Both widgets removed.

### 11.7 REPLACE widget

```
alter page MyModule.EntityPage {
  REPLACE ddStatus WITH {
    radiobuttons (Attribute: Status);
  }
};
```

**Expected:** DropDown replaced with RadioButtons.

### 11.8 ADD variable

```
alter page MyModule.ConditionalPage {
  ADD VARIABLES $EditMode: Boolean = 'false';
};
```

**Expected:** Variable added to page.

### 11.9 DROP variable

```
alter page MyModule.ConditionalPage {
  DROP VARIABLES $ShowExtra;
};
```

**Expected:** Variable removed.

### 11.10 SET layout

```
alter page MyModule.SimplePage {
  SET LAYOUT = Atlas_Core.Atlas_TopBar;
};
```

**Expected:** Page layout changed.

### 11.11 SET column property

```
alter page MyModule.ListPage {
  SET grid1.colName.Caption = 'Full Name';
};
```

**Expected:** DataGrid column caption updated.

---

## 12. ALTER SNIPPET

### 12.1 INSERT widget into snippet

```
alter snippet MyModule.ItemCard {
  INSERT INTO dv POSITION last {
    button (Caption: 'View', Action: SHOW_PAGE MyModule.Detail);
  }
};
```

**Expected:** Widget added to snippet.

### 12.2 DROP widget from snippet

```
alter snippet MyModule.ItemCard {
  DROP WIDGET btnView;
};
```

**Expected:** Widget removed from snippet.

---

## 13. WIREFRAME OUTPUT

### 13.1 Page wireframe (JSON)

```
describe page MyModule.EntityPage --format wireframe;
```

**Expected:** JSON output with structure:
```json
{
  "format": "wireframe",
  "type": "page",
  "name": "MyModule.EntityPage",
  "title": "...",
  "layout": "...",
  "parameters": [...],
  "root": [...],
  "mdlSource": "...",
  "sourceMap": {...}
}
```

### 13.2 Snippet wireframe

```
describe snippet MyModule.ItemCard --format wireframe;
```

**Expected:** JSON wireframe for snippet.

### 13.3 Wireframe node structure

Verify wireframe nodes contain: `id`, `widget`, `name`, `label`, `binding`, `action`, `datasource`, `children`.

---

## 14. UPDATE WIDGETS

### 14.1 Update widget property

```
update widgets where Name = 'btnSave' set Caption = 'Save Changes';
```

**Expected:** All matching widgets updated.

### 14.2 Dry run

```
update widgets where WidgetType = 'Forms$TextBox' set Placeholder = 'Enter...' dry run;
```

**Expected:** Shows what would change without modifying anything.

---

## 15. ROUNDTRIP (BSON)

### 15.1 Page roundtrip

1. Create a page with DataView, TextBox, Button
2. `describe page MyModule.TestPage`
3. Drop: `drop page MyModule.TestPage`
4. Execute described MDL
5. `describe` again

**Expected:** Output identical between step 2 and step 5.

### 15.2 Snippet roundtrip

Same process for a snippet with parameters and widgets.

---

## 16. MULTI-STEP WORKFLOWS

### 16.1 Entity → page → grant access

```
create persistent entity MyModule.Task (
  Title: string(200) not null,
  Done: boolean default false
);

create page MyModule.TaskList (
  Title: 'Tasks',
  Layout: Atlas_Core.Atlas_Default
) {
  datagrid (DataSource: DATABASE MyModule.Task) {
    column (Attribute: Title, Caption: 'Task');
    column (Attribute: Done, Caption: 'Done');
  }
}

create module role MyModule.User;
grant view on page MyModule.TaskList to User;
```

**Expected:** All succeed. `describe page` shows grant.

### 16.2 Create snippet → use in page

```
create snippet MyModule.TaskRow (
  Params: { $Task: MyModule.Task }
) {
  dataview (DataSource: $Task) {
    textbox (Attribute: Title);
  }
}

create page MyModule.TaskPage (
  Title: 'Tasks',
  Layout: Atlas_Core.Atlas_Default
) {
  snippetcall MyModule.TaskRow;
}
```

**Expected:** Page references snippet.

---

## 17. FAILURE MODES & ERROR RECOVERY

### 17.1 Unknown widget type

```
create page MyModule.Bad (
  Title: 'Bad',
  Layout: Atlas_Core.Atlas_Default
) {
  unknownwidget;
}
```

**Expected:** Error — unknown widget type.

### 17.2 Non-existent layout

```
create page MyModule.Bad (
  Title: 'Bad',
  Layout: Fake.Layout
) {
  text 'X';
}
```

**Expected:** Error — layout not found.

### 17.3 ALTER on non-existent widget

```
alter page MyModule.EntityPage {
  DROP WIDGET fakeWidget;
};
```

**Expected:** Error — widget not found.

### 17.4 Invalid DataSource

```
create page MyModule.Bad (
  Title: 'Bad',
  Layout: Atlas_Core.Atlas_Default
) {
  datagrid (DataSource: DATABASE Fake.Entity) {
    column (Attribute: X);
  }
}
```

**Expected:** Error — entity not found.

---

## 18. BOUNDARY & STRESS

### 18.1 Page with many widgets (50+)

Create a page with 50+ widgets nested in layout grid. Verify `describe` and wireframe handle it.

### 18.2 Deeply nested widgets

```
create page MyModule.DeepNest (
  Title: 'Deep',
  Layout: Atlas_Core.Atlas_Default
) {
  container {
    container {
      container {
        container {
          text 'Deep content';
        }
      }
    }
  }
}
```

**Expected:** Created and described with correct nesting.

### 18.3 Page with all widget types

Create a page using every supported widget type. Verify roundtrip.

---

## 19. SECURITY CASCADES

### 19.1 GRANT VIEW ON PAGE

```
grant view on page MyModule.OrderOverview to MyModule.Admin, MyModule.User;
```

**Expected:** Page allowed roles updated. `describe page MyModule.OrderOverview` includes `grant view on page` in MDL output.

### 19.2 REVOKE VIEW ON PAGE

```
revoke view on page MyModule.OrderOverview from MyModule.User;
```

**Expected:** User role removed. Admin role remains.

### 19.3 Drop module role cascades to page

1. `grant view on page MyModule.OrderOverview to MyModule.TempRole;`
2. `drop module role MyModule.TempRole;`

**Expected:** Step 2 output mentions removal from page allowed roles. `describe page MyModule.OrderOverview` no longer shows TempRole.

### 19.4 Describe page shows grant MDL

```
describe page MyModule.OrderOverview;
```

**Expected:** Output ends with `grant view on page MyModule.OrderOverview to <roles>;` when roles are assigned.

### 19.5 Grant on non-existent page

```
grant view on page MyModule.FakePage to MyModule.Admin;
```

**Expected:** Error — page not found.

### 19.6 Grant non-existent role

```
grant view on page MyModule.OrderOverview to MyModule.FakeRole;
```

**Expected:** Error — module role not found.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW PAGES | x | x | x |
| SHOW SNIPPETS | x | x | x |
| SHOW LAYOUTS | x | x | x |
| SHOW WIDGETS | x | x | |
| DESCRIBE PAGE | x | x | x |
| DESCRIBE SNIPPET | x | x | |
| DESCRIBE LAYOUT | x | | |
| CREATE PAGE | x | | |
| CREATE SNIPPET | x | | |
| ALTER PAGE | x | | |
| ALTER SNIPPET | x | | |
| DROP PAGE/SNIPPET | x | | |
| WIREFRAME | x | x | |
| UPDATE WIDGETS | x | | |
| GRANT/REVOKE VIEW | x | | |

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW PAGES | Mock tests | |
| 2. SHOW SNIPPETS | Mock tests | |
| 3. SHOW LAYOUTS | Mock tests | |
| 4. SHOW WIDGETS | Mock tests | LIKE filter |
| 5. DESCRIBE PAGE | Mock tests | Complex pages |
| 6. DESCRIBE SNIPPET | Mock tests | |
| 7. DESCRIBE LAYOUT | Mock tests | |
| 8. CREATE PAGE | Mock tests | Complex widgets |
| 9. CREATE SNIPPET | Mock tests | |
| 10. DROP | Mock tests | |
| 11. ALTER PAGE | Mock tests | Complex ops |
| 12. ALTER SNIPPET | Mock tests | |
| 13. WIREFRAME | Mock tests | JSON structure |
| 14. UPDATE WIDGETS | Mock tests | Dry run |
| 15. Roundtrip | | All manual |
| 16. Multi-step | | All manual |
| 17. Failure modes | Partial | |
| 18. Boundary | | All manual |
| 19. Security cascades | Mock tests | Role cascade |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW PAGES | List all | | | | |
| 1.2 | SHOW PAGES | Filter module | | | | |
| 1.3 | SHOW PAGES | Empty module | | | | |
| 2.1 | SHOW SNIPPETS | List all | | | | |
| 2.2 | SHOW SNIPPETS | Filter module | | | | |
| 3.1 | SHOW LAYOUTS | List all | | | | |
| 3.2 | SHOW LAYOUTS | Filter module | | | | |
| 4.1 | SHOW WIDGETS | List all | | | | |
| 4.2 | SHOW WIDGETS | Filter type | | | | |
| 4.3 | SHOW WIDGETS | Filter LIKE | | | | |
| 4.4 | SHOW WIDGETS | Filter module | | | | |
| 5.1 | DESCRIBE PAGE | Basic | | | | |
| 5.2 | DESCRIBE PAGE | With params | | | | |
| 5.3 | DESCRIBE PAGE | With variables | | | | |
| 5.4 | DESCRIBE PAGE | In folder | | | | |
| 5.5 | DESCRIBE PAGE | Security roles | | | | |
| 5.6 | DESCRIBE PAGE | Not found | | | | |
| 6.1 | DESCRIBE SNIPPET | Basic | | | | |
| 6.2 | DESCRIBE SNIPPET | With params | | | | |
| 7.1 | DESCRIBE LAYOUT | Layout | | | | |
| 8.1 | CREATE PAGE | Minimal | | | | |
| 8.2 | CREATE PAGE | With URL + params | | | | |
| 8.3 | CREATE PAGE | DataView | | | | |
| 8.4 | CREATE PAGE | DataGrid | | | | |
| 8.5 | CREATE PAGE | LayoutGrid | | | | |
| 8.6 | CREATE PAGE | Buttons + actions | | | | |
| 8.7 | CREATE PAGE | TabContainer | | | | |
| 8.8 | CREATE PAGE | SnippetCall | | | | |
| 8.9 | CREATE PAGE | Conditional visibility | | | | |
| 8.10 | CREATE PAGE | In folder | | | | |
| 8.11 | CREATE PAGE | create or modify | | | | |
| 8.12 | CREATE PAGE | create or replace | | | | |
| 8.13 | CREATE PAGE | Duplicate error | | | | |
| 9.1 | CREATE SNIPPET | Minimal | | | | |
| 9.2 | CREATE SNIPPET | With params | | | | |
| 9.3 | CREATE SNIPPET | In folder | | | | |
| 10.1 | DROP | Page | | | | |
| 10.2 | DROP | Page not found | | | | |
| 10.3 | DROP | Snippet | | | | |
| 10.4 | DROP | Snippet not found | | | | |
| 11.1 | ALTER PAGE | SET property | | | | |
| 11.2 | ALTER PAGE | SET DataSource | | | | |
| 11.3 | ALTER PAGE | INSERT last | | | | |
| 11.4 | ALTER PAGE | INSERT first | | | | |
| 11.5 | ALTER PAGE | DROP widget | | | | |
| 11.6 | ALTER PAGE | DROP multiple | | | | |
| 11.7 | ALTER PAGE | REPLACE widget | | | | |
| 11.8 | ALTER PAGE | ADD variable | | | | |
| 11.9 | ALTER PAGE | DROP variable | | | | |
| 11.10 | ALTER PAGE | SET layout | | | | |
| 11.11 | ALTER PAGE | SET column prop | | | | |
| 12.1 | ALTER SNIPPET | INSERT | | | | |
| 12.2 | ALTER SNIPPET | DROP | | | | |
| 13.1 | WIREFRAME | Page JSON | | | | |
| 13.2 | WIREFRAME | Snippet JSON | | | | |
| 13.3 | WIREFRAME | Node structure | | | | |
| 14.1 | UPDATE | Widget property | | | | |
| 14.2 | UPDATE | Dry run | | | | |
| 15.1 | ROUNDTRIP | Page | | | | |
| 15.2 | ROUNDTRIP | Snippet | | | | |
| 16.1 | MULTI-STEP | Entity + page + grant | | | | |
| 16.2 | MULTI-STEP | Snippet + page | | | | |
| 17.1 | FAILURE | Unknown widget | | | | |
| 17.2 | FAILURE | Non-existent layout | | | | |
| 17.3 | FAILURE | ALTER bad widget | | | | |
| 17.4 | FAILURE | Invalid DataSource | | | | |
| 18.1 | BOUNDARY | Many widgets | | | | |
| 18.2 | BOUNDARY | Deep nesting | | | | |
| 18.3 | BOUNDARY | All widget types | | | | |
| 19.1 | SECURITY | Grant view | | | | |
| 19.2 | SECURITY | Revoke view | | | | |
| 19.3 | SECURITY | Drop role cascade | | | | |
| 19.4 | SECURITY | Describe shows grant | | | | |
| 19.5 | SECURITY | Grant bad page | | | | |
| 19.6 | SECURITY | Grant bad role | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
