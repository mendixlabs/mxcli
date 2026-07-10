// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	// ── Page ──────────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "page",
		Summary: "Create and manage pages with widgets, datasources, and actions",
		Keywords: []string{
			"page", "pages", "form", "UI", "user interface",
			"widget", "layout", "screen",
		},
		Syntax:  "CREATE PAGE Module.Name\n  (\n    Title: 'Page Title',\n    Layout: Module.LayoutName\n    [, Params: { $Param: Module.Entity }]\n    [, Url: 'page-url']\n    [, Folder: 'FolderPath']\n    [, Variables: { $var: Boolean = 'true' }]\n    [, PopupWidth: 800, PopupHeight: 480, PopupResizable: true]\n    [, Class: 'css-class', Style: 'css: rule']\n  )\n  {\n    -- widgets\n  }",
		Example: "CREATE PAGE MyModule.EditCustomer\n  (\n    Params: { $Customer: MyModule.Customer },\n    Title: 'Edit Customer',\n    Layout: Atlas_Core.PopupLayout,\n    Class: 'container-fluid'\n  )\n  {\n    DATAVIEW dvCustomer (DataSource: $Customer) {\n      TEXTBOX txtName (Label: 'Name', Binds: Name)\n      FOOTER footer1 {\n        ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)\n        ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)\n      }\n    }\n  }",
		SeeAlso: []string{"page.create", "page.widgets", "page.alter", "snippet"},
	})

	Register(SyntaxFeature{
		Path:    "page.create",
		Summary: "CREATE PAGE with parameters, variables, layout, and URL",
		Keywords: []string{
			"create page", "new page", "page parameters", "page variables",
			"layout", "url", "folder",
		},
		Syntax:  "CREATE PAGE Module.Name\n  (\n    Title: 'Title',\n    Layout: Module.Layout\n    [, Params: { $P: Module.Entity, $Qty: Integer }]\n    [, Url: 'page-url']\n    [, Folder: 'FolderPath']\n    [, Variables: { $showStock: Boolean = 'true' }]\n  )\n  { <widgets> }",
		Example: "CREATE PAGE Module.Products\n  (\n    Title: 'Products',\n    Layout: Atlas_Core.Atlas_Default,\n    Url: 'products',\n    Variables: { $showStock: Boolean = 'true' }\n  )\n  {\n    DATAGRID gridProducts (DataSource: DATABASE Module.Product) {\n      COLUMN colName (Attribute: Name, Caption: 'Name')\n    }\n  }",
		SeeAlso: []string{"page", "page.widgets", "page.datasource"},
	})

	Register(SyntaxFeature{
		Path:    "page.widgets",
		Summary: "Widget types: containers, data widgets, inputs, actions, display",
		Keywords: []string{
			"widget", "layoutgrid", "container", "dataview", "datagrid",
			"gallery", "listview", "textbox", "textarea", "datepicker",
			"combobox", "checkbox", "radiobuttons", "actionbutton",
			"dynamictext", "snippetcall", "navigationlist",
			"column", "row", "footer", "header", "controlbar",
		},
		Syntax:  "-- Containers\nLAYOUTGRID name { ROW r { COLUMN c (DesktopWidth: 6) { ... } } }\nCONTAINER name (Class: 'cls') { ... }\nCONTAINER name (OnClick: MICROFLOW Module.MF) { ... }   -- clickable container\n\n-- Data widgets\nDATAVIEW name (DataSource: $Param) { ... FOOTER f { ... } }\nDATAGRID name (DataSource: DATABASE Module.Entity) { COLUMN c (Attribute: A) }\nGALLERY name (DataSource: DATABASE Module.Entity, DesktopColumns: 3) { ... }\nLISTVIEW name (DataSource: DATABASE Module.Entity) { ... }\n\n-- Inputs\nTEXTBOX name (Label: 'L', Binds: Attr)\nTEXTAREA | DATEPICKER | COMBOBOX | CHECKBOX | RADIOBUTTONS\n\n-- Actions\nACTIONBUTTON name (Caption: 'C', Action: SAVE_CHANGES, ButtonStyle: Primary)\n\n-- Display\nDYNAMICTEXT name (Content: 'Hello, {1}!', ContentParams: [{1} = Name])",
		Example: "DATAVIEW dvCustomer (DataSource: $Customer) {\n  TEXTBOX txtName (Label: 'Name', Binds: Name)\n  COMBOBOX cbStatus (Label: 'Status', Binds: Status)\n  FOOTER footer1 {\n    ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)\n    ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)\n  }\n}",
		SeeAlso: []string{"page.create", "page.datasource"},
	})

	Register(SyntaxFeature{
		Path:    "page.datasource",
		Summary: "Datasource bindings: variable, database, microflow, selection, association",
		Keywords: []string{
			"datasource", "data source", "database", "microflow",
			"selection", "variable", "binding", "binds", "association", "data from context",
		},
		Syntax:  "DataSource: $Variable                    -- Parameter/variable binding\nDataSource: DATABASE Module.Entity        -- Database query\nDataSource: MICROFLOW Module.MF()         -- Microflow datasource\nDataSource: SELECTION widgetName          -- Selection from another widget\nDataSource: $currentObject/Module.Assoc   -- Over an association (\"data from context\")\n                                          --   list widget → to-many collection\n                                          --   nested DATAVIEW → the to-one referenced object\nBinds: AttributeName                      -- Attribute binding (inputs)",
		Example: "-- Database datasource with grid\nDATAGRID grid (DataSource: DATABASE Module.Customer) {\n  COLUMN colName (Attribute: Name, Caption: 'Name')\n}\n\n-- Microflow datasource\nDATAVIEW dv (DataSource: MICROFLOW Module.GetData()) { ... }\n\n-- Over an association: a nested DataView shows the referenced (to-one) object\nDATAVIEW dvOrder (DataSource: $Order) {\n  DATAVIEW dvCustomer (DataSource: $currentObject/Order_Customer) {\n    TEXTBOX (Label: 'Name', Attribute: Name)\n  }\n}\n\n-- Over an association: a list widget shows the (to-many) collection\nLISTVIEW lvLines (DataSource: $currentObject/Order_OrderLine) { ... }",
		SeeAlso: []string{"page.widgets", "page.create"},
	})

	Register(SyntaxFeature{
		Path:    "page.action",
		Summary: "Button actions: save, cancel, close, delete, show page, microflow",
		Keywords: []string{
			"action", "save", "cancel", "close", "delete",
			"show page", "navigate", "microflow", "create object",
			"button style", "primary", "danger", "success",
		},
		Syntax:  "Action: SAVE_CHANGES\nAction: CANCEL_CHANGES\nAction: CLOSE_PAGE\nAction: DELETE\nAction: SHOW_PAGE Module.Page\nAction: SHOW_PAGE Module.Page(Param: $val)\nAction: MICROFLOW Module.MF\nAction: MICROFLOW Module.MF(Param: $val)\nAction: CREATE_OBJECT Module.Entity THEN SHOW_PAGE Module.Page\n\nButton styles: Default, Primary, Success, Info, Warning, Danger",
		Example: "ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)\nACTIONBUTTON btnEdit (Caption: 'Edit',\n  Action: SHOW_PAGE Module.EditPage(Item: $currentObject))\nACTIONBUTTON btnNew (Caption: 'New',\n  Action: CREATE_OBJECT Module.Entity THEN SHOW_PAGE Module.EditPage,\n  ButtonStyle: Primary)",
		SeeAlso: []string{"page.widgets"},
	})

	Register(SyntaxFeature{
		Path:    "page.show",
		Summary: "List and describe existing pages",
		Keywords: []string{
			"show pages", "list pages", "describe page",
		},
		Syntax:  "SHOW PAGES;\nSHOW PAGES IN <module>;\nDESCRIBE PAGE Module.Name;",
		Example: "SHOW PAGES IN MyModule;\nDESCRIBE PAGE MyModule.EditCustomer;",
		SeeAlso: []string{"page", "page.create"},
	})

	Register(SyntaxFeature{
		Path:    "page.alter",
		Summary: "ALTER PAGE — modify widget properties, insert, drop, replace widgets in-place",
		Keywords: []string{
			"alter page", "modify page", "update page",
			"set property", "insert widget", "drop widget", "replace widget",
			"popup width", "popup height", "popup resizable",
		},
		Syntax:  "ALTER PAGE Module.Name {\n  SET property = value ON widgetName;\n  SET (prop1 = val1, prop2 = val2) ON widgetName;\n  SET Title = 'New Title';  -- page-level (case-sensitive)\n  SET Class = 'css-class';  -- page-level CSS class / style\n  SET Style = 'css: rule';\n  SET PopupWidth = 800;     -- page-level pop-up dimensions\n  SET PopupHeight = 480;\n  SET PopupResizable = true;\n  INSERT AFTER widgetName { <widgets> };\n  INSERT BEFORE widgetName { <widgets> };\n  DROP WIDGET name1, name2;\n  REPLACE widgetName WITH { <widgets> };\n};",
		Example: "ALTER PAGE Module.EditPage {\n  SET (Caption = 'Save & Close', ButtonStyle = Success) ON btnSave;\n  INSERT AFTER txtName {\n    TEXTBOX txtMiddleName (Label: 'Middle Name', Binds: MiddleName)\n  };\n  DROP WIDGET txtUnused;\n};",
		SeeAlso: []string{"page.create", "page.show", "snippet.alter"},
	})

	Register(SyntaxFeature{
		Path:    "page.styling",
		Summary: "CSS classes, inline styles, and Atlas design properties on widgets",
		Keywords: []string{
			"class", "style", "css", "design properties", "atlas",
			"spacing", "full width",
		},
		Syntax:  "Class: 'css-class-name'\nStyle: 'color: red; padding: 8px;'\nDesignProperties: ['Spacing top': 'Large']\nDesignProperties: ['Full width': ON]",
		Example: "CONTAINER ctn (Class: 'my-card', Style: 'padding: 16px;') {\n  DYNAMICTEXT txt (Content: 'Styled text')\n}",
		SeeAlso: []string{"page.widgets"},
	})

	Register(SyntaxFeature{
		Path:    "page.column",
		Summary: "DATAGRID column properties: sortable, resizable, width, alignment, visibility",
		Keywords: []string{
			"column", "sortable", "resizable", "draggable", "hidable",
			"column width", "alignment", "wrap text", "visible",
			"dynamic cell class", "tooltip", "associated attribute", "association column",
		},
		Syntax:  "COLUMN name (\n  Attribute: AttrName,          -- own attribute\n  -- or an attribute over an association (bare association name):\n  -- Attribute: Assoc/Attr     e.g. Order_Customer/Name\n  Caption: 'Header'\n  [, Sortable: true|false]\n  [, Resizable: true|false]\n  [, Draggable: true|false]\n  [, Hidable: yes|hidden|no]\n  [, ColumnWidth: autoFill|autoFit|manual]\n  [, Size: integer]\n  [, Alignment: left|center|right]\n  [, WrapText: true|false]\n  [, Visible: 'expression']\n  [, DynamicCellClass: 'expression']\n  [, Tooltip: 'text']\n)",
		Example: "COLUMN colPrice (\n  Attribute: Price, Caption: 'Price',\n  Alignment: right, Sortable: false,\n  ColumnWidth: manual, Size: 150,\n  Tooltip: 'Price in USD'\n)\n\n-- Associated attribute (attribute over a reference association):\nCOLUMN colCustomer (Attribute: Order_Customer/Name, Caption: 'Customer')",
		SeeAlso: []string{"page.widgets"},
	})

	// ── Snippet ───────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "snippet",
		Summary: "Reusable page fragments embedded via SNIPPETCALL",
		Keywords: []string{
			"snippet", "snippets", "reusable", "snippetcall",
			"page fragment", "component",
		},
		Syntax:  "CREATE SNIPPET Module.Name\n  [( Params: { $P: Module.Entity }, Folder: 'path' )]\n  {\n    -- widgets (same as page)\n  }\n\n-- Embed in a page:\nSNIPPETCALL scName (Snippet: Module.SnippetName)",
		Example: "CREATE SNIPPET MyModule.CustomerInfo (\n  Params: { $Customer: MyModule.Customer }\n)\n{\n  DATAVIEW dv (DataSource: $Customer) {\n    TEXTBOX txtName (Label: 'Name', Attribute: Name)\n    TEXTBOX txtEmail (Label: 'Email', Attribute: Email)\n  }\n}",
		SeeAlso: []string{"snippet.create", "snippet.alter", "page"},
	})

	Register(SyntaxFeature{
		Path:    "snippet.create",
		Summary: "CREATE SNIPPET with parameters, variables, and folder",
		Keywords: []string{
			"create snippet", "new snippet", "snippet parameters",
			"snippet variables",
		},
		Syntax:  "CREATE SNIPPET Module.Name\n  [( Params: { $P: Module.Entity, $Label: String } )]\n  [( Variables: { $isEditable: Boolean = 'true' } )]\n  [( Folder: 'Snippets/Common' )]\n  {\n    -- widgets\n  }",
		Example: "CREATE SNIPPET MyModule.NavigationMenu\n{\n  NAVIGATIONLIST navMenu {\n    ITEM itemCustomers (Action: SHOW_PAGE MyModule.CustomerOverview) {\n      DYNAMICTEXT txtCustomers (Content: 'Customers')\n    }\n  }\n}",
		SeeAlso: []string{"snippet", "snippet.alter", "page.widgets"},
	})

	Register(SyntaxFeature{
		Path:    "snippet.alter",
		Summary: "ALTER SNIPPET — same operations as ALTER PAGE on snippet widgets",
		Keywords: []string{
			"alter snippet", "modify snippet", "update snippet",
		},
		Syntax:  "ALTER SNIPPET Module.Name {\n  SET property = value ON widgetName;\n  INSERT AFTER widgetName { <widgets> };\n  INSERT BEFORE widgetName { <widgets> };\n  DROP WIDGET name1, name2;\n  REPLACE widgetName WITH { <widgets> };\n};",
		Example: "ALTER SNIPPET Module.NavSnippet {\n  REPLACE navItem1 WITH {\n    ACTIONBUTTON btnHome (Caption: 'Home', Action: SHOW_PAGE Module.HomePage)\n  };\n  DROP WIDGET txtOldField;\n  INSERT AFTER txtName {\n    TEXTBOX txtNewField (Label: 'New Field', Binds: NewAttr)\n  };\n};",
		SeeAlso: []string{"snippet", "page.alter"},
	})

	Register(SyntaxFeature{
		Path:    "snippet.show",
		Summary: "List and describe existing snippets",
		Keywords: []string{
			"show snippets", "list snippets", "describe snippet",
		},
		Syntax:  "SHOW SNIPPETS;\nSHOW SNIPPETS IN <module>;\nDESCRIBE SNIPPET Module.Name;",
		Example: "SHOW SNIPPETS IN MyModule;\nDESCRIBE SNIPPET MyModule.NavigationMenu;",
		SeeAlso: []string{"snippet", "snippet.create"},
	})

	// ── Fragment ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "fragment",
		Summary: "Transient reusable widget groups defined at script scope",
		Keywords: []string{
			"fragment", "fragments", "reusable widgets", "define fragment",
			"use fragment", "template", "script scope",
		},
		Syntax:  "DEFINE FRAGMENT Name AS { <widgets> };\nUSE FRAGMENT Name [AS prefix_];\nSHOW FRAGMENTS;\nDESCRIBE FRAGMENT Name;\nDESCRIBE FRAGMENT FROM PAGE Module.Page WIDGET widgetName;",
		Example: "DEFINE FRAGMENT SaveCancelFooter AS {\n  FOOTER footer1 {\n    ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)\n    ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)\n  }\n};\n\nCREATE PAGE Module.EditPage (...) {\n  DATAVIEW dv (DataSource: $Param) {\n    TEXTBOX txtName (Label: 'Name', Binds: Name)\n    USE FRAGMENT SaveCancelFooter\n  }\n};",
		SeeAlso: []string{"fragment.define", "fragment.use", "snippet"},
	})

	Register(SyntaxFeature{
		Path:    "fragment.define",
		Summary: "DEFINE FRAGMENT — declare a reusable widget group for the script session",
		Keywords: []string{
			"define fragment", "declare fragment", "create fragment",
		},
		Syntax:  "DEFINE FRAGMENT Name AS {\n  <widgets>\n};",
		Example: "DEFINE FRAGMENT FormFields AS {\n  TEXTBOX txtName (Label: 'Name', Binds: Name)\n  TEXTBOX txtEmail (Label: 'Email', Binds: Email)\n};",
		SeeAlso: []string{"fragment", "fragment.use"},
	})

	Register(SyntaxFeature{
		Path:    "fragment.use",
		Summary: "USE FRAGMENT — insert fragment widgets at current position with optional prefix",
		Keywords: []string{
			"use fragment", "insert fragment", "expand fragment",
			"prefix", "name conflict",
		},
		Syntax:  "USE FRAGMENT Name\nUSE FRAGMENT Name AS prefix_",
		Example: "-- Basic usage\nCREATE PAGE Module.Page (...) {\n  DATAVIEW dv (DataSource: $Param) {\n    USE FRAGMENT FormFields\n    USE FRAGMENT SaveCancelFooter\n  }\n};\n\n-- With prefix to avoid name conflicts\nUSE FRAGMENT SaveCancelFooter AS order_\n-- Creates: order_footer1, order_btnSave, order_btnCancel",
		SeeAlso: []string{"fragment", "fragment.define"},
	})

	Register(SyntaxFeature{
		Path:    "fragment.show",
		Summary: "SHOW/DESCRIBE FRAGMENTS and extract widget subtrees from existing pages",
		Keywords: []string{
			"show fragments", "describe fragment", "list fragments",
			"extract widget", "widget subtree",
		},
		Syntax:  "SHOW FRAGMENTS;\nDESCRIBE FRAGMENT Name;\nDESCRIBE FRAGMENT FROM PAGE Module.Page WIDGET widgetName;\nDESCRIBE FRAGMENT FROM SNIPPET Module.Snippet WIDGET widgetName;",
		Example: "SHOW FRAGMENTS;\nDESCRIBE FRAGMENT SaveCancelFooter;\n\n-- Extract a widget subtree from an existing page\nDESCRIBE FRAGMENT FROM PAGE Module.MyPage WIDGET footer1;",
		SeeAlso: []string{"fragment", "fragment.define"},
	})
}
