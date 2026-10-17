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
		Syntax:  "CREATE PAGE Module.Name [FOLDER 'FolderPath']\n  (\n    Title: 'Page Title',\n    Layout: Module.LayoutName\n    [, Params: ( $Param: Module.Entity )]\n    [, Url: 'page-url']\n    [, Variables: ( $var: Boolean = true )]   -- the default is an expression, written bare\n    [, PopupWidth: 800, PopupHeight: 480, PopupResizable: true]\n    [, PopupCloseAction: cancelButton1]\n    [, Class: 'css-class', Style: 'css: rule']\n  )\n  {\n    -- widgets\n  }",
		Example: "CREATE PAGE MyModule.EditCustomer\n  (\n    Params: ( $Customer: MyModule.Customer ),\n    Title: 'Edit Customer',\n    Layout: Atlas_Core.PopupLayout,\n    Class: 'container-fluid'\n  )\n  {\n    DATAVIEW dvCustomer (DataSource: $Customer) {\n      TEXTBOX txtName (Label: 'Name', Attribute: Name)\n      FOOTER {\n        ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)\n        ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)\n      }\n    }\n  };",
		SeeAlso: []string{"page.create", "page.widgets", "page.alter", "snippet", "rename"},
	})

	Register(SyntaxFeature{
		Path:    "page.create",
		Summary: "CREATE PAGE with parameters, variables, layout, and URL",
		Keywords: []string{
			"create page", "new page", "page parameters", "page variables",
			"layout", "url", "folder",
		},
		Syntax:  "CREATE PAGE Module.Name [FOLDER 'FolderPath']\n  (\n    Title: 'Title',\n    Layout: Module.Layout\n    [, Params: ( $P: Module.Entity, $Qty: Integer )]\n    [, Url: 'page-url']\n    [, Variables: ( $showStock: Boolean = true )]\n  )\n  { <widgets> }",
		Example: "CREATE PAGE Module.Products\n  (\n    Title: 'Products',\n    Layout: Atlas_Core.Atlas_Default,\n    Url: 'products',\n    Variables: ( $showStock: Boolean = true )\n  )\n  {\n    DATAGRID gridProducts (DataSource: DATABASE Module.Product) {\n      COLUMN (Attribute: Name, Caption: 'Name')\n    }\n  };",
		SeeAlso: []string{"page", "page.widgets", "page.datasource"},
	})

	Register(SyntaxFeature{
		Path:    "page.widget-describe",
		Summary: "DESCRIBE WIDGET TYPE — a widget's properties, enum values and editor rules",
		Keywords: []string{
			"describe widget type", "describe widget", "widget properties", "widget definition", "what properties",
			"enum values", "widget rules", "hidden properties", "pluggable widget properties",
		},
		Syntax: `DESCRIBE WIDGET TYPE <keyword>;
DESCRIBE WIDGET TYPE '<widget id>';`,
		Example: `DESCRIBE WIDGET TYPE combobox;
DESCRIBE WIDGET TYPE 'com.mendix.widget.web.htmlelement.HTMLElement';

-- Names the widget by its MDL keyword or its full widget id.
--
-- Works with NO project open, answering from mxcli's embedded set. With a
-- project the answer is better: the installed .mpk is version-accurate and is
-- the only place a Marketplace widget appears.
--
-- Reports each property's key, type, caption, category, whether it is required,
-- its default and its enumeration values, plus the dynamic rules the widget's
-- editor uses to HIDE properties under some configurations — the ones that
-- cause CE0463 if written into the pruned half.
--
-- Also emits an MDL example that PARSES AS WRITTEN: the head form and every
-- container in it are chosen by probing the real parser, and anything the
-- grammar cannot yet express is left out and named. So the example widens on
-- its own as MDL gains ground, and cannot promise syntax that fails.
--
-- Same output as ` + "`mxcli widget describe`" + `, because it is the same code.
--
-- The answer is a report on this project's widget package, not MDL: its first
-- line is "-- widget type definition (not executable)", and with --json the
-- document carries "executable": false. The same holds for DESCRIBE GLYPH and
-- DESCRIBE CONTRACT ENTITY|ACTION|MESSAGE (unless FORMAT mdl). Use MXCLI SYNTAX
-- for how to write a widget in MDL; this for what a given project installs.

-- The other direction — which pages already use it — is a reference query,
-- and needs ` + "`refresh catalog full`" + `:
LIST REFERENCES TO combobox;
LIST IMPACT OF htmlelement;

-- Name it as you write it in a page body; the casing does not matter. A
-- built-in Mendix widget (textbox, dynamictext) has no definition and so no
-- reference edge — use LIST WIDGETS for those.`,
		SeeAlso: []string{"page.widgets", "page.create"},
	})

	Register(SyntaxFeature{
		Path:    "page.widget-any",
		Summary: "Any widget with a definition, written by its own MDL name",
		Keywords: []string{
			"htmlelement", "html element", "fileuploader", "file uploader", "markdown",
			"custom widget syntax", "marketplace widget", "pluggable widget name",
			"widget not recognized", "mismatched input", "def-driven", "mdl name",
			"object list", "child slot", "widget container", "attributes list",
		},
		Syntax: `<widget-mdl-name> <name> [( Prop: Value, ... )] [{ <containers and widgets> }]
<container-name> <name> [( Prop: Value, ... )] [{ ... }]`,
		Example: "-- Any widget with a definition is written by its own MDL name. There is no\n" +
			"-- list of blessed keywords: if `describe widget type <name>` knows it, you can write it.\n" +
			"CREATE PAGE Sales.Detail (Title: 'Detail', Layout: Atlas_Core.Atlas_Default) {\n" +
			"  htmlelement frame (tagName: 'div') {\n" +
			"    -- object lists and child slots the widget's own definition declares\n" +
			"    attribute a1 (attributeName: 'title', attributeValueType: 'expression')\n" +
			"    tagcontentcontainer body {\n" +
			"      dynamictext t (Content: 'hello')\n" +
			"    }\n" +
			"  }\n" +
			"  fileuploader up ()\n" +
			"};\n" +
			"\n" +
			"-- The names come from the widget itself, so ask it rather than guessing:\n" +
			"--   mxcli widget describe htmlelement -p app.mpr\n" +
			"-- which lists every property, every container, and an example that parses.\n" +
			"--\n" +
			"-- A name that resolves to no definition is MDL-WIDGET25 (widget) or\n" +
			"-- MDL-WIDGET26 (container), each naming the near misses — but BOTH need a\n" +
			"-- project, because the set of valid widget names IS the project's installed\n" +
			"-- packages. With no -p, `mxcli check` cannot tell a typo from a widget it\n" +
			"-- has simply never seen, and says nothing rather than guessing.",
		SeeAlso: []string{"page.widgets", "page.widget-describe", "page.create"},
	})

	Register(SyntaxFeature{
		Path:    "page.widgets",
		Summary: "Widget types: containers, data widgets, inputs, actions, display",
		Keywords: []string{
			"widget", "layoutgrid", "container", "customcontainer", "dataview", "datagrid",
			"gallery", "listview", "textbox", "textarea", "datepicker",
			"combobox", "checkbox", "radiobuttons", "filemanager", "file upload", "actionbutton", "linkbutton",
			"dynamictext", "title", "snippetcall", "navigationlist", "item",
			"column", "row", "footer", "header", "controlbar",
			"template", "specialization", "list view template",
			"tabcontainer", "tabpage", "tabs", "groupbox", "collapsible",
			"image", "filter", "textfilter", "numberfilter", "datefilter",
			"dropdownfilter", "dropdownsort", "customwidget", "pluggablewidget",
			"statictext", "staticimage", "dynamicimage", "dropdown", "referenceselector",
			"legacydatagrid", "visible", "editable", "conditional visibility",
		},
		Syntax: "-- Containers\nLAYOUTGRID name { ROW { COLUMN (DesktopWidth: 6) { ... } } }\nCONTAINER name (Class: 'cls') { ... }\nCONTAINER name (OnClick: CALL MICROFLOW Module.MF) { ... }   -- clickable container\nCUSTOMCONTAINER name (Class: 'cls') { ... }\nGROUPBOX name (Caption: 'C') { ... }\nTABCONTAINER name { TABPAGE tp (Caption: 'One') { ... } TABPAGE tp2 (Caption: 'Two') { ... } }\n\n" +
			"-- Data widgets\nDATAVIEW name (DataSource: $Param) { ... FOOTER { ... } }\nDATAGRID name (DataSource: DATABASE Module.Entity) { COLUMN (Attribute: A) }\nGALLERY name (DataSource: DATABASE Module.Entity, DesktopColumns: 3) { ... }\nLISTVIEW name (DataSource: DATABASE Module.Entity) { ... }\nLISTVIEW name (...) { ... TEMPLATE FOR Module.Specialization { ... } }\n\n" +
			"-- Data grid 2 column filters go INSIDE the column's own braces\nDATAGRID dg (...) { COLUMN (Attribute: A) { TEXTFILTER tf (Attribute: A) } }\nTEXTFILTER | NUMBERFILTER | DATEFILTER | DROPDOWNFILTER | DROPDOWNSORT\n" +
			"--   Match the filter to the column's type, or MxBuild refuses it: String ->\n" +
			"--   TEXTFILTER, Integer/Long/Decimal -> NUMBERFILTER, Date and time -> DATEFILTER,\n" +
			"--   Enumeration AND Boolean -> DROPDOWNFILTER (the drop-down filter's own\n" +
			"--   attribute types are Enum and Boolean; a Boolean column filters Yes/No).\n" +
			"--   A column may carry BOTH a custom-content widget and a filter — `content`\n" +
			"--   and `filter` are separate slots, so a read-only CHECKBOX cell and a\n" +
			"--   DROPDOWNFILTER live in the same braces:\n" +
			"DATAGRID dg (...) { COLUMN (Attribute: IsActive) {\n" +
			"  CHECKBOX cb (Attribute: IsActive, Editable: Never, ReadOnlyStyle: Control)\n" +
			"  DROPDOWNFILTER ddf } }\n" +
			"--   ReadOnlyStyle (Inherit | Control | Text) is what makes a read-only check\n" +
			"--   box render as the checkbox glyph instead of the text Yes/No.\n" +
			"--   The grid-wide filter bar is CONTROLBAR; a GALLERY spells that same slot\n" +
			"--   FILTER, so `FILTER f { ... }` belongs to a gallery and not to a datagrid:\n" +
			"GALLERY g (...) { FILTER f { TEXTFILTER tf (Attribute: A) } }\n" +
			"--   A FILTER block written on a DATAGRID is not a column filter and not a\n" +
			"--   container the grid declares — it used to be dropped on write with no\n" +
			"--   diagnostic, and is now refused (MDL-WIDGET30).\n\n" +
			"-- Inputs\nTEXTBOX name (Label: 'L', Attribute: Attr)\nTEXTAREA | DATEPICKER | COMBOBOX | CHECKBOX | RADIOBUTTONS\n" +
			"-- A date picker's DateFormat is its mode: Date (default) | Time | DateTime | Custom.\n" +
			"DATEPICKER name (Attribute: Start, DateFormat: DateTime)\n" +
			"-- File upload/download: inside a data view on a System.FileDocument specialization (CE6810 otherwise).\n" +
			"FILEMANAGER name (Label: 'File', Type: Upload, MaxFileSize: 10, AllowedExtensions: '.xlsx', ShowFileInBrowser: false)\n" +
			"DATEPICKER name (Attribute: Start, DateFormat: Custom, CustomDateFormat: 'dd-MM-yyyy HH:mm')\n" +
			"-- A text box's numeric formatting:\n" +
			"TEXTBOX name (Attribute: Amount, DecimalPrecision: 2, GroupDigits: true)\n" +
			"-- Submit while typing (TEXTBOX, TEXTAREA): OnEndEditing (default) | WhileEditing,\n" +
			"-- with the delay in milliseconds (default 300):\n" +
			"TEXTBOX name (Attribute: Query, SubmitBehaviour: WhileEditing, SubmitOnInputDelay: 500)\n" +
			"-- A layout grid's width: FullWidth (default) | FixedWidth:\n" +
			"LAYOUTGRID name (Width: FixedWidth) { ROW { COLUMN (DesktopWidth: 12) { ... } } }\n\n" +
			"-- Conditional visibility / editability: a bare client expression, stored as\n" +
			"-- written (name attributes as $currentObject/Attr). A plain value is static.\n" +
			"TEXTBOX name (Attribute: Attr, Visible: $currentObject/IsActive, Editable: $currentObject/Status != 'Closed')\n" +
			"TEXTBOX name (Attribute: Attr, Visible: false, Editable: Never)\n" +
			"--   A condition may also be a quoted string holding the same expression; Editable:\n" +
			"--   false / true mean Never / Always.\n" +
			"TEXTBOX name (Attribute: Attr, Editable: '$currentObject/Status != ''Closed''')\n" +
			"--   The bracketed Visible: [IsActive] (attributes rooted for you) is the deprecated\n" +
			"--   spelling, MDL-DEPR081.\n\n" +
			"-- Actions\nACTIONBUTTON name (Caption: 'C', Action: SAVE CHANGES, ButtonStyle: Primary)\nLINKBUTTON name (Caption: 'C', Action: ...)\n\n" +
			"-- Display\nDYNAMICTEXT name (Content: 'Hello, {1}!', ContentParams: ({1} = Name))\nTITLE name (Content: 'Heading')\nIMAGE name (Image: 'Module.Collection.ImageName')\nIMAGE name (ImageType: imageUrl, ImageUrl: 'https://…')\n" +
			"--   IMAGE needs a source. Its default, `ImageType: image`, shows an entry from an\n" +
			"--   image collection, named as three parts: Module.Collection.ImageName.\n" +
			"--   `list image collections` lists the collections, `describe image collection\n" +
			"--   Module.Collection` the images inside one. An IMAGE with that source and no\n" +
			"--   entry writes a model mxbuild refuses (\"No image selected.\"); MDL-WIDGET22\n" +
			"--   reports that at check time, and a name that does not resolve is reported by\n" +
			"--   `check --references` rather than failing the build with CE1613.\n" +
			"--   The alternatives are the URL form above, or `ImageType: icon`.\n" +
			"--   A text-template property (ImageUrl, AlternativeText, a pluggable widget's\n" +
			"--   headerCaption/title/…) takes TEXT, so a bare value renders the same string\n" +
			"--   on every row. Bind it with the property's own `<Name>Params` companion:\n" +
			"IMAGE name (ImageType: imageUrl, ImageUrl: '{1}', ImageUrlParams: ({1} = PictureUrl),\n" +
			"            AlternativeText: '{1}', AlternativeTextParams: ({1} = Name))\n" +
			"--   The widget-wide `contentparams:` is one list shared by every template on the\n" +
			"--   widget; `'{AttrName}'` is the shortest form for a single attribute.\n\n" +
			"-- Any pluggable widget by its id (id FIRST, then the name)\nPLUGGABLEWIDGET 'com.mendix.widget.web.badge.Badge' name (value: 'x')\nCUSTOMWIDGET 'com.mendix.widget.custom.x.X' name (prop: 'x')      -- legacy spelling\n\n" +
			"-- DYNAMICIMAGE shows the image held by an OBJECT, so it needs the entity that\n" +
			"-- object belongs to — reachable from the widget's context. Without it mxbuild\n" +
			"-- reports CE0489, so the widget cannot build at all:\n" +
			"DYNAMICIMAGE imgPhoto (DataSource: database from MyModule.Photo,\n" +
			"                       DefaultImage: 'MyModule.Images.placeholder')\n" +
			"--   DefaultImage is the fallback when the object carries none. DisplayAs:\n" +
			"--   thumbnail and OnClickType: enlarge are written too.\n\n" +
			"-- STATICIMAGE takes the same three-part image-collection reference as IMAGE,\n" +
			"-- so a stored one round-trips through DESCRIBE (mendixlabs/mxcli#1057). Without\n" +
			"-- it the widget is written with no image and mxbuild reports CE0436:\n" +
			"STATICIMAGE imgLogo (Image: 'MyModule.Images.logo', Width: 64, Height: 64)\n\n" +
			"-- Not supported by the React client — added in Mendix 10.7, and the only\n" +
			"-- client on 11, so this is not a Mendix 11 rule. These are written correctly,\n" +
			"-- but mxbuild reports CE0582 (\"not supported in React client\") on each\n" +
			"-- wherever that client is enabled, and `mxcli lint` reports them as MPR012.\n" +
			"-- Prefer the alternative:\n" +
			"--   STATICIMAGE    -> IMAGE\n" +
			"--   DYNAMICIMAGE   -> IMAGE\n" +
			"--   DROPDOWN       -> COMBOBOX\n" +
			"-- And three the executor refuses on BOTH engines, each with its own message:\n" +
			"--   STATICTEXT         (writes Forms$Text, a type Mendix no longer has — the\n" +
			"--                       project could not be OPENED afterwards; MDL-WIDGET30.\n" +
			"--                       Use DYNAMICTEXT with a literal Content.)\n" +
			"--   REFERENCESELECTOR  (unsupported widget type)\n" +
			"--   LEGACYDATAGRID     (use DATAGRID for the pluggable equivalent on Mendix 11+)",
		Example: "DATAVIEW dvCustomer (DataSource: $Customer) {\n  TEXTBOX txtName (Label: 'Name', Attribute: Name)\n  COMBOBOX cbStatus (Label: 'Status', Attribute: Status)\n  FOOTER {\n    ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)\n    ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)\n  }\n}",
		SeeAlso: []string{"page.create", "page.datasource"},
	})

	Register(SyntaxFeature{
		Path:    "page.listview-template",
		Summary: "List View specialization templates: one body per specialization",
		Keywords: []string{
			"template", "listview template", "list view template", "specialization",
			"generalization", "inheritance", "template for", "per type",
		},
		Syntax: "LISTVIEW name (DataSource: DATABASE Module.Base) {\n" +
			"  ...widgets...                          -- the default body, used when no template matches\n" +
			"  TEMPLATE FOR Module.Specialization {   -- one body per specialization\n" +
			"    ...widgets...\n" +
			"  }\n" +
			"}\n\n" +
			"A template is identified by the entity it renders, not by a name — that is why it is\n" +
			"TEMPLATE FOR Module.Entity and not TEMPLATE name. (A Gallery's TEMPLATE name is a\n" +
			"different thing: a named content slot.)\n\n" +
			"Rules:\n" +
			"  - the entity must be a SPECIALIZATION of the list view's entity; the list\n" +
			"    view's own entity is CE0543, since its body already renders objects\n" +
			"    no template matches\n" +
			"  - at most one template per entity\n" +
			"  - templates keep their source order, which is the order Mendix stores and matches in\n" +
			"  - inside a template the context object is the specialization, so its own attributes resolve\n\n" +
			"ALTER PAGE — adding one reuses INSERT INTO with the same block, so a template has one\n" +
			"spelling everywhere. Removing one needs its own form, because a template has no name:\n\n" +
			"ALTER PAGE Module.Page {\n" +
			"  INSERT INTO listViewName { TEMPLATE FOR Module.Specialization { ...widgets... } };\n" +
			"  DROP TEMPLATE FOR Module.Specialization IN listViewName;\n" +
			"};\n\n" +
			"Naming the list view in DROP is required, not optional: one page can hold two list views\n" +
			"with a template for the same entity. To change what a template renders, edit the widgets\n" +
			"inside it by name (SET / INSERT AFTER) — they are ordinary widgets. To replace a whole\n" +
			"template, DROP it and INSERT the new one in the same ALTER block; operations apply in\n" +
			"order.",
		Example: "LISTVIEW vehicleListView (DataSource: DATABASE Pages.Vehicle) {\n" +
			"  DYNAMICTEXT defaultVehicle (Content: '{1} {2}', ContentParams: ({1} = Brand, {2} = Model))\n" +
			"  TEMPLATE FOR Pages.Bus {\n" +
			"    DYNAMICTEXT busLabel (Content: 'Bus, capacity {1}', ContentParams: ({1} = PassengerCapacity))\n" +
			"  }\n" +
			"  TEMPLATE FOR Pages.Truck {\n" +
			"    DYNAMICTEXT truckLabel (Content: 'Truck, max load {1} kg', ContentParams: ({1} = MaxLoadKg))\n" +
			"  }\n" +
			"}",
		SeeAlso: []string{"page.widgets", "page.datasource"},
	})

	Register(SyntaxFeature{
		Path:    "page.datasource",
		Summary: "Datasource bindings: variable, database, microflow, selection, association",
		Keywords: []string{
			"datasource", "data source", "database", "microflow",
			"selection", "variable", "binding", "binds", "association", "data from context",
		},
		Syntax:  "DataSource: $Variable                    -- Parameter/variable binding\nDataSource: DATABASE Module.Entity        -- Database query\nDataSource: DATABASE Module.Entity WHERE [Attr != ''] SORT BY Attr ASC\n                                          --   ...optionally constrained and sorted\nDataSource: DATABASE Module.Entity SORT BY Module.Assoc/Attr ASC\n                                          --   ...sorted over an association. Name the\n                                          --   hop when two reach the same entity —\n                                          --   the wrong one builds cleanly and sorts\n                                          --   by the wrong thing.\nDataSource: DATABASE Module.Entity SEARCH BY Attr, Attr2\n                                          --   LIST VIEW only: the attributes its\n                                          --   search bar filters on. Mirrors SORT BY,\n                                          --   but takes no direction.\nDataSource: DATABASE FROM $Ctx/Module.Assoc/Module.Entity [WHERE …] [SORT BY …] [SEARCH BY …]\n                                          --   LIST VIEW only: a DATABASE retrieve of\n                                          --   what the association reaches, keeping\n                                          --   XPath, sort and search. Not the same as\n                                          --   $Ctx/Module.Assoc below (in memory, none\n                                          --   of the three). Name the entity after\n                                          --   each association.\n                                          --   Mendix 9: a pluggable widget (repeater,\n                                          --   gallery, grid) takes it too, as Studio\n                                          --   Pro 9's \"Database\" source; its WHERE is\n                                          --   then simple rows only — Attr op value,\n                                          --   joined by AND (no OR, paths, variables).\nDataSource: MICROFLOW Module.MF          -- Microflow datasource, no parameters\nDataSource: MICROFLOW Module.MF(Param = $P) -- ...one argument per PARAMETER, required,\n                                          --   and NAMED: the positional form\n                                          --   MF($P) is refused. Mendix does\n                                          --   NOT auto-map an object in scope, not\n                                          --   even one of the exact type, so a\n                                          --   missing argument is CE1571\nDataSource: SELECTION widgetName          -- Selection from another widget\nDataSource: $currentObject/Module.Assoc   -- Over an association (\"data from context\")\n                                          --   list widget → to-many collection\n                                          --   nested DATAVIEW → the to-one referenced object\nAttribute: AttributeName                  -- Attribute binding (inputs)",
		Example: "-- Database datasource with grid\nDATAGRID grid (DataSource: DATABASE Module.Customer) {\n  COLUMN (Attribute: Name, Caption: 'Name')\n}\n\n-- Microflow datasource\nDATAVIEW dv (DataSource: MICROFLOW Module.GetData) {\n  TEXTBOX txtName (Label: 'Name', Attribute: Name)\n}\n\n-- Over an association: a nested DataView shows the referenced (to-one) object\nDATAVIEW dvOrder (DataSource: $Order) {\n  DATAVIEW dvCustomer (DataSource: $currentObject/Order_Customer) {\n    TEXTBOX txtCustName (Label: 'Name', Attribute: Name)\n  }\n}\n\n-- Over an association: a list widget shows the (to-many) collection\nLISTVIEW lvLines (DataSource: $currentObject/Order_OrderLine) {\n  DYNAMICTEXT dtLine (Content: 'Line')\n}",
		SeeAlso: []string{"page.widgets", "page.create"},
	})

	Register(SyntaxFeature{
		Path:    "page.action",
		Summary: "Widget actions: save, cancel, close, delete, show page, microflow, nanoflow, nothing",
		Keywords: []string{
			"action", "save", "cancel", "close", "delete",
			"show page", "navigate", "microflow", "create object",
			"button style", "primary", "danger", "success",
			"icon", "linkbutton", "link button",
			"nothing", "no action", "inert", "dead button",
			"confirmation", "confirm", "are you sure", "progress bar", "progress message",
			"disabled during execution", "disabled during action", "asynchronous", "form validations",
		},
		Syntax:  "Action: NOTHING                      -- deliberately no action (Forms$NoAction)\nAction: SAVE CHANGES\nAction: SAVE CHANGES CLOSE PAGE      -- save, then close the pop-up\nAction: CANCEL CHANGES\nAction: CANCEL CHANGES CLOSE PAGE\nAction: CLOSE PAGE\nAction: DELETE\nAction: DELETE CLOSE PAGE\nAction: CALL NANOFLOW Module.NF\nAction: CALL NANOFLOW Module.NF(Param = $val)\nAction: OPEN LINK 'https://example.com'\nAction: OPEN LINK $currentObject/URL  -- address read from an attribute\nAction: SIGN OUT\nAction: COMPLETE TASK 'OutcomeName'\nAction: SHOW PAGE Module.Page\nAction: SHOW PAGE Module.Page(Param = $currentObject)\nAction: CALL MICROFLOW Module.MF\nAction: CALL MICROFLOW Module.MF(Param = $val)\nAction: CREATE OBJECT Module.Entity THEN SHOW PAGE Module.Page\n\nThe list above is exhaustive. Anything else in an action slot is an\nERROR: `Action: OPEN LINK` without a URL or `Action: SHOW PAGE` without a\npage is a syntax error, and a value that is not an action at all\n(or a snake-case keyword short its argument) is MDL-WIDGET28.\nSuch a widget used to be written with NO action at all and rendered as a\ndead control, with check, exec and mxbuild all clean, because a\nno-action widget is legal Mendix (mendixlabs/mxcli#1062). Write NOTHING\nwhen a control really is meant to be inert.\n\nThe actions are the words a microflow uses (R8). The snake-case spellings\n(SAVE_CHANGES, SHOW_PAGE, CLOSE_PAGE, DELETE_OBJECT, OPEN_LINK, SIGN_OUT, ...)\nand a flow call without CALL still parse, but are deprecated (MDL-DEPR020);\n`mxcli fmt --upgrade` rewrites them.\n\nThe same forms serve `OnClick:` (an alias of `Action:`) and `OnChange:`.\n\nSETTINGS. Any action but NOTHING takes a `with ( … )` list of the\nsettings Studio Pro shows under the event (ako/mxcli#721):\n\n  DisabledDuringExecution: true|false  -- every action; default true\n  ProgressBar: None|NonBlocking|Blocking  -- flow calls; default None\n  ProgressMessage: 'text'                -- flow calls\n  Confirmation: 'question'               -- flow calls: ask before running\n  ProceedCaption: 'text', CancelCaption: 'text'  -- its buttons; default Proceed / Cancel\n  Asynchronous: true|false               -- call microflow only; default false\n  FormValidations: All|Widget|None       -- call microflow only; default All\n\nAction: CALL MICROFLOW M.Delete(Order = $currentObject) with (\n  ProgressBar: Blocking, ProgressMessage: 'Deleting…',\n  Confirmation: 'Delete this order?', ProceedCaption: 'Delete', CancelCaption: 'Keep')\n\nA setting left out keeps its default. DESCRIBE prints only the settings\nthat differ from it. An unknown key, or one the action does not have, is an\nerror. For CREATE OBJECT … THEN SHOW PAGE …, the list goes at the end and\nbelongs to the create.\n\nA microflow or nanoflow action is a CALL: it needs an argument for every\nparameter the flow declares, or Mendix rejects the page with CE1571. The\nargument list is the same on every widget that takes an action -- a\nCONTAINER (which is clickable) as much as an ACTIONBUTTON. An enclosing\ndata container of the right type supplies it without an argument; a data\ngrid's CONTROL BAR does not, because it is not row-scoped -- pass the\ngrid's selection there (`$dgOrders`).\n\nA SHOW PAGE argument must be the enclosing widget's context object --\neither $currentObject or the name of the variable the enclosing data\nwidget is bound to. Mendix infers it from that widget, so naming any\nother variable is refused (MDL-PAGEARG01); call a microflow instead.\nThe exception is a parameter of the page or snippet itself: it is bound\nby name (Studio Pro stores an explicit mapping), so `(Item = $Item)` works\nanywhere, outside every data widget too (mendixlabs/mxcli#1403).\nOutside any data widget there is no context object to infer, so such a\nbutton takes no OTHER argument -- not $currentObject, not a literal.\nmxcli used to drop it in silence and mxbuild then reported CE1571 per\nparameter of the target page (mendixlabs/mxcli#1029). Route that\nnavigation through a microflow.\n\nOPEN LINK takes a web address, stored as a Forms$StaticOrDynamicString:\neither a literal, or $currentObject/Attr to read it from an attribute of\nthe enclosing data container's object at runtime (Studio Pro's \"Address:\nattribute\"). An address over an association path is not supported yet.\n\nButton styles: Default, Primary, Success, Info, Warning, Danger\n\nMendix has THREE icon elements and the keyword picks which one:\n\nIcon: 'Atlas_Core.Atlas_Filled.pencil'   -- an icon collection\nIcon: image MyModule.Images.logo         -- an IMAGE collection\nIcon: glyph 57377                        -- a font code point\n\nThe bare form is the icon-collection icon and any collection in the\nproject works, third-party ones included. The image form points into a\ndifferent document, and is spelled the same way apart from the keyword\n-- write it without `image` and mxcli stores a custom-icon reference,\nwhich fails the build with CE1613 (mendixlabs/mxcli#1059).\n`mxcli check -p … --references` resolves each kind against its own\ncollection and names the remedy when the kind is wrong.\n\nA glyph carries a code and no name. Codes are sparse, and an undefined\none fails only at `mxbuild --target=deploy`, naming the PAGE rather\nthan the icon -- so MDL078 checks it against the font's own table.\nList them with `list glyphs`.\n\nA name may be quoted or bare; a hyphenated segment is double-quoted on\nits own: Atlas_Core.Atlas.\"align-center\".\n\nUse `linkbutton` instead of `actionbutton` for link render mode (same properties).",
		Example: "ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)\nACTIONBUTTON btnEdit (Caption: 'Edit',\n  Action: SHOW PAGE Module.EditPage(Item = $currentObject))\nLINKBUTTON btnDelete (Caption: 'Delete', Action: DELETE,\n  Icon: 'Atlas_Core.Atlas_Filled.pencil')\n\n-- A clickable CONTAINER in a data grid's control bar, calling a nanoflow\n-- with the grid's selection as its argument.\nDATAGRID dgOrders (DataSource: DATABASE FROM Sales.Order, Selection: Single) {\n  COLUMN (Attribute: Number, Caption: 'Order #')\n  CONTROLBAR {\n    CONTAINER cShip (Class: 'command',\n      Action: CALL NANOFLOW Sales.ACT_Ship(Order = $dgOrders)) {\n      ACTIONBUTTON btnShip (Caption: 'Ship')\n    }\n  }\n}",
		SeeAlso: []string{"page.widgets"},
	})

	Register(SyntaxFeature{
		Path:    "page.show",
		Summary: "List and describe existing pages",
		Keywords: []string{
			"list pages", "list pages", "describe page",
		},
		Syntax:  "LIST PAGES;\nLIST PAGES IN <module>;\nDESCRIBE PAGE Module.Name;",
		Example: "LIST PAGES IN MyModule;\nDESCRIBE PAGE MyModule.EditCustomer;",
		SeeAlso: []string{"page", "page.create"},
	})

	Register(SyntaxFeature{
		Path:    "page.alter",
		Summary: "ALTER PAGE — modify widget properties, insert, drop, replace widgets in-place",
		Keywords: []string{
			"alter page", "modify page", "update page",
			"set property", "insert widget", "drop widget", "replace widget",
			"popup width", "popup height", "popup resizable",
			"drop template", "insert template", "list view template",
			"add parameter", "drop parameter", "page parameter", "add parameters",
		},
		Syntax:  "ALTER PAGE Module.Name {       -- the generic ALTER: set / insert / replace / drop\n  SET (property: value) ON widgetName;  -- widget property names: any casing\n  SET ('Row size': 'Small') ON lvOrders;  -- an Atlas DESIGN property of that widget's\n                                         --   type; quoted and case-sensitive.\n                                         --   `list design properties for <type>` lists\n                                         --   them. ON/OFF for a toggle, where OFF\n                                         --   REMOVES the entry.\n                                         --   A multi-select ('Hide on') or compound\n                                         --   ('Spacing') one needs the inline\n                                         --   DesignProperties: (...) form, because a\n                                         --   SET assignment carries one value.\n  SET (Action: CALL MICROFLOW Module.MF) ON btnSave;   -- any CREATE PAGE action form\n  SET ('createFileAction': CALL MICROFLOW Module.MF) ON fileUploader1;\n                                         -- a pluggable widget's NAMED action slot,\n                                         --   by the widget's own key; refused on a\n                                         --   key that is not action-typed\n  SET (DataSource: $Param) ON dvOrder;  -- parameter/microflow/nanoflow/selection;\n                                        --   DATABASE and association are REPLACE-only,\n                                        --   and a data view takes no database source\n  SET (RenderMode: H2) ON txtTitle;    -- dynamic text: Text | Paragraph | H1..H6\n  SET (prop1: val1, prop2: val2) ON widgetName;\n  SET (Title: 'New Title');  -- page-level (case-sensitive): no ON\n  SET (Documentation: 'What this page is for.');\n  SET (Class: 'css-class');  -- page-level CSS class / style\n  SET (Style: 'css: rule');\n  SET (PopupWidth: 800, PopupHeight: 480, PopupResizable: true);  -- page-level pop-up\n  INSERT AFTER widgetName { <widgets> };\n  INSERT BEFORE widgetName { <widgets> };\n  INSERT INTO containerName { <widgets> };\n  DROP name1, name2;\n  DROP TEMPLATE FOR Module.Specialization IN listViewName;\n  REPLACE widgetName WITH { <widgets> };\n  ADD PARAMETERS $Order: Module.Order;  -- or a primitive: $Count: Integer.\n                                         --   A page with a Url needs a {Order}\n                                         --   segment: SET (Url: '…/{Order}') in the\n                                         --   same statement (CE5601 otherwise).\n                                         --   A snippet takes entities only (MDL087).\n  DROP PARAMETERS $Order;               -- refused while a data source or an\n                                         --   expression on the page still uses it\n  ADD VARIABLES $show: Boolean = true;\n  DROP VARIABLES $show;\n};\n-- A target is a widget name, or a DataGrid 2 column by what it shows:\n-- grid column(Attr), grid column('Caption'), @n when two match; or a\n-- navigation list item: nav item(widgetInIt), nav item('Text'), nav item@n. The old spellings `SET p = v`,\n-- `SET p: v` (no parentheses) and `DROP WIDGET a` still run and warn\n-- (MDL-DEPR101..103).\n\n-- The BULK form: one design property on every widget of a TYPE.\nALTER PAGES [IN Module]\n  SET 'Compact' = ON, 'Striped' = ON\n  WHERE WIDGETTYPE = datagrid            -- the MDL keyword, which resolves to\n                                         --   exactly one widget id. A full id in\n                                         --   quotes works too. NOT a name: a widget\n                                         --   name is unique only within its page.\n  [DRY RUN];                             -- run this FIRST. It reports the matches\n                                         --   against a discardable copy and writes\n                                         --   nothing.",
		Example: "ALTER PAGE Module.EditPage {\n  SET (Caption: 'Save & Close', ButtonStyle: Success) ON btnSave;\n  INSERT AFTER txtName {\n    TEXTBOX txtMiddleName (Label: 'Middle Name', Attribute: MiddleName)\n  };\n  DROP txtUnused;\n};",
		SeeAlso: []string{"page.create", "page.show", "snippet.alter"},
	})

	Register(SyntaxFeature{
		Path:    "page.styling",
		Summary: "CSS classes, inline styles, dynamic classes and Atlas design properties — written inline, or ALTERed in place",
		Keywords: []string{
			"class", "style", "css", "design properties", "atlas",
			"spacing", "full width", "dynamic classes", "dynamicclasses",
			"conditional class", "runtime class",
			"alter styling", "restyle", "clear design properties", "restyle a widget",
		},
		Syntax: "ON A WIDGET, inside CREATE PAGE / CREATE SNIPPET:\n\n" +
			"  Class: 'css-class-name'                 -- static CSS classes\n" +
			"  Style: 'color: red; padding: 8px;'      -- inline CSS\n" +
			"  DynamicClasses: '<expression>'          -- runtime-computed (stacks on Class)\n" +
			"  DesignProperties: ('Spacing top': 'Large')\n" +
			"  DesignProperties: ('Full width': ON)\n\n" +
			"ON A PAGE THAT ALREADY EXISTS, without rewriting it:\n\n" +
			"  ALTER STYLING ON PAGE|SNIPPET Module.Name WIDGET <widgetName>\n" +
			"    SET (Class: 'css-class', Style: 'css', 'Design property': 'Value'|ON|OFF);\n\n" +
			"  ALTER STYLING ON PAGE|SNIPPET Module.Name WIDGET <widgetName>\n" +
			"    CLEAR DESIGN PROPERTIES;\n\n" +
			"The widget is named by its MDL NAME — the identifier after the widget\n" +
			"keyword (`ACTIONBUTTON btnSave`), not its caption. `DESCRIBE PAGE` prints\n" +
			"the names.\n\n" +
			"A bare `Class:` REPLACES the widget's classes rather than adding to them.\n" +
			"Read the current value first if you meant to append.\n\n" +
			"Reach for ALTER STYLING rather than CREATE OR MODIFY PAGE whenever only\n" +
			"the look changes: replacing the page rewrites every widget in it, so the\n" +
			"diff is the whole document and anything MDL cannot yet spell is lost.\n\n" +
			"`SET Class = 'x', 'P' = ON` (no parentheses, `=`) still runs and warns\n" +
			"MDL-DEPR062.",
		Example: "CONTAINER ctn (\n  Class: 'my-card',\n  DynamicClasses: if $currentObject/Priority = 'High' then 'card-danger' else 'card-normal'\n) {\n  DYNAMICTEXT txt (Content: 'Styled text')\n}\n\n" +
			"-- Restyle one widget on a page that already exists\n" +
			"alter styling on page Sales.OrderOverview widget btnSave\n" +
			"  set (Class: 'btn-primary', 'Spacing top': 'Large');\n\n" +
			"-- Back to Atlas defaults\n" +
			"alter styling on snippet Sales.OrderRow widget ctnMain\n" +
			"  clear design properties;",
		SeeAlso: []string{"page.widgets", "page.alter", "page.update-widgets"},
	})

	Register(SyntaxFeature{
		Path:    "page.update-widgets",
		Summary: "SHOW / UPDATE WIDGETS — find widgets across every page, and set a property on all of them",
		Keywords: []string{
			"list widgets", "update widgets", "bulk", "bulk update", "across pages",
			"widgettype", "dry run", "every page", "all pages", "sweep", "mass edit",
		},
		Syntax: "LIST WIDGETS [WHERE <cond> [AND <cond>...]] [IN Module];\n\n" +
			"UPDATE WIDGETS\n" +
			"  SET 'property' = <value> [, 'property' = <value>...]\n" +
			"  WHERE <cond> [AND <cond>...]\n" +
			"  [IN Module]\n" +
			"  [DRY RUN];\n\n" +
			"A condition is `WidgetType = 'x'` or `WidgetType LIKE '%x%'`, or any other\n" +
			"property name against `=` / `LIKE`. Values are strings, numbers, booleans\n" +
			"or NULL. The property name is QUOTED — it is the widget's own key, as\n" +
			"`DESCRIBE WIDGET` prints it, not an MDL keyword.\n\n" +
			"WHERE IS MANDATORY on UPDATE. There is no \"all widgets\" form, because the\n" +
			"statement rewrites every page a match lands on.\n\n" +
			"RUN IT WITH `DRY RUN` FIRST. It reports the matches and the containers they\n" +
			"sit in and writes nothing — the only way to see what a pattern actually\n" +
			"selects before it has selected it. `LIST WIDGETS` with the same WHERE\n" +
			"answers the same question read-only.\n\n" +
			"Needs a full catalog, which the statement builds itself, and a project open\n" +
			"for writing. The catalog is NOT refreshed by the update — run\n" +
			"`REFRESH CATALOG FULL FORCE` afterwards, or the next query answers from the\n" +
			"model as it was before.\n\n" +
			"EXPERIMENTAL: this reaches into pluggable-widget property bags, where a key\n" +
			"that does not belong to a widget's schema is what CE0463 is made of. Check\n" +
			"the build afterwards.",
		Example: "mdl 1;\n" +
			"-- What would match, read-only\n" +
			"list widgets where WidgetType like '%combobox%' in Sales;\n\n" +
			"-- What would change, still writing nothing\n" +
			"update widgets\n" +
			"  set 'showLabel' = false\n" +
			"  where WidgetType like '%combobox%'\n" +
			"  dry run;\n\n" +
			"-- Apply, scoped to one module\n" +
			"update widgets\n" +
			"  set 'filterMode' = 'contains', 'labelWidth' = 4\n" +
			"  where WidgetType like '%DataGrid%'\n" +
			"  in Sales;",
		SeeAlso: []string{"page.widget-describe", "page.alter", "page.styling"},
	})

	Register(SyntaxFeature{
		Path:    "page.column",
		Summary: "DATAGRID column properties: sortable, resizable, width, alignment, visibility",
		Keywords: []string{
			"column", "sortable", "resizable", "draggable", "hidable",
			"column width", "alignment", "wrap text", "visible",
			"dynamic cell class", "tooltip", "associated attribute", "association column",
		},
		Syntax:  "COLUMN (   -- no name: Mendix stores none for a data grid column (MDL-DEPR005)\n  Attribute: AttrName,          -- own attribute\n  -- or an attribute over an association (bare association name):\n  -- Attribute: Assoc/Attr     e.g. Order_Customer/Name\n  Caption: 'Header'\n  [, Sortable: true|false]\n  [, Resizable: true|false]\n  [, Draggable: true|false]\n  [, Hidable: yes|hidden|no]\n  [, ColumnWidth: autoFill|autoFit|manual]\n  [, Size: integer]\n  [, Alignment: left|center|right]\n  [, WrapText: true|false]\n  [, Visible: <expression>]       -- once for the grid: a page variable or parameter, never $currentObject (MDL-WIDGET43)\n  [, DynamicCellClass: <expression>]  -- per row: $currentObject is the row\n  [, Tooltip: 'text']\n)",
		Example: "COLUMN (\n  Attribute: Price, Caption: 'Price',\n  Alignment: right, Sortable: false,\n  ColumnWidth: manual, Size: 150,\n  Tooltip: 'Price in USD'\n)\n\n-- Associated attribute (attribute over a reference association):\nCOLUMN (Attribute: Order_Customer/Name, Caption: 'Customer')",
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
		Syntax:  "CREATE SNIPPET Module.Name [FOLDER 'path']\n  [( Params: ( $P: Module.Entity ) )]   -- parameters are entities only\n  {\n    -- widgets (same as page)\n  }\n\n-- Embed in a page:\nSNIPPETCALL scName (Snippet: Module.SnippetName)",
		Example: "CREATE SNIPPET MyModule.CustomerInfo (\n  Params: ( $Customer: MyModule.Customer )\n)\n{\n  DATAVIEW dv (DataSource: $Customer) {\n    TEXTBOX txtName (Label: 'Name', Attribute: Name)\n    TEXTBOX txtEmail (Label: 'Email', Attribute: Email)\n  }\n};",
		SeeAlso: []string{"snippet.create", "snippet.alter", "page"},
	})

	Register(SyntaxFeature{
		Path:    "snippet.create",
		Summary: "CREATE SNIPPET with parameters, variables, and folder",
		Keywords: []string{
			"create snippet", "new snippet", "snippet parameters",
			"snippet variables",
		},
		// Every snippet parameter is an entity. This line used to print
		// "$Label: String", which is not executable: mxbuild rejects a
		// primitive snippet parameter with CE0046 "Invalid data type 'String'."
		// (a PAGE parameter may be primitive; a snippet parameter may not), and
		// the reporter of mendixlabs/mxcli#1028 reached the bug by following
		// this line. A primitive is now refused as MDL087.
		Syntax:  "CREATE SNIPPET Module.Name [FOLDER 'Snippets/Common']\n  [( Params: ( $P: Module.Entity ) )]   -- entities only; a primitive is CE0046\n  [( Variables: ( $isEditable: Boolean = true ) )]\n  {\n    -- widgets\n  }\n\n-- To parameterise a snippet on a primitive, keep the primitive on the\n-- calling PAGE and pass an object, or read the value off an entity member.",
		Example: "CREATE SNIPPET MyModule.NavigationMenu\n{\n  NAVIGATIONLIST navMenu {\n    ITEM itemCustomers (Action: SHOW PAGE MyModule.CustomerOverview) {\n      DYNAMICTEXT txtCustomers (Content: 'Customers')\n    }\n  }\n};",
		SeeAlso: []string{"snippet", "snippet.alter", "page.widgets"},
	})

	Register(SyntaxFeature{
		Path:    "snippet.alter",
		Summary: "ALTER SNIPPET — same operations as ALTER PAGE on snippet widgets",
		Keywords: []string{
			"alter snippet", "modify snippet", "update snippet",
		},
		Syntax:  "ALTER SNIPPET Module.Name {\n  SET (property: value) ON widgetName;\n  INSERT AFTER widgetName { <widgets> };\n  INSERT BEFORE widgetName { <widgets> };\n  INSERT INTO containerName { <widgets> };\n  DROP name1, name2;\n  REPLACE widgetName WITH { <widgets> };\n};",
		Example: "ALTER SNIPPET Module.NavSnippet {\n  REPLACE navItem1 WITH {\n    ACTIONBUTTON btnHome (Caption: 'Home', Action: SHOW PAGE Module.HomePage)\n  };\n  DROP txtOldField;\n  INSERT AFTER txtName {\n    TEXTBOX txtNewField (Label: 'New Field', Attribute: NewAttr)\n  };\n};",
		SeeAlso: []string{"snippet", "page.alter"},
	})

	Register(SyntaxFeature{
		Path:    "snippet.show",
		Summary: "List and describe existing snippets",
		Keywords: []string{
			"list snippets", "list snippets", "describe snippet",
		},
		Syntax:  "LIST SNIPPETS;\nLIST SNIPPETS IN <module>;\nDESCRIBE SNIPPET Module.Name;",
		Example: "LIST SNIPPETS IN MyModule;\nDESCRIBE SNIPPET MyModule.NavigationMenu;",
		SeeAlso: []string{"snippet", "snippet.create"},
	})

	// ── Layout ────────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "layout",
		Summary: "The frame a page is built on — regions, navigation, and the placeholders pages bind to",
		Keywords: []string{
			"layout", "layouts", "create layout", "scrollcontainer", "region",
			"placeholder", "navigationtree", "topbar", "sidebar", "frame",
			"menubar", "simplemenubar", "bottom bar", "phone layout", "menu document",
		},
		Syntax: "CREATE [OR MODIFY] LAYOUT Module.Name (\n" +
			"  layouttype: 'Responsive' | 'Phone' | 'Tablet' | 'ModalPopup'   -- web\n" +
			"           -- | 'Default' | 'Popup'                              -- native\n" +
			"  [, class: 'layout-atlas layout-atlas-responsive-topbar']\n" +
			"  [, style: '...']\n" +
			"  [, Layout: Module.Outer]   -- built on another layout: see layout.nested\n" +
			") {\n" +
			"  SCROLLCONTAINER name {\n" +
			"    REGION top | right | bottom | left | center\n" +
			"      [( Size: 60, SizeMode: 'Fixed' | 'Pixels' | 'Auto', Class: '…' )] {\n" +
			"      -- widgets, plus:\n" +
			"      NAVIGATIONTREE name (Profile: 'Responsive')   -- vertical, for a sidebar\n" +
			"      MENUBAR name (Profile: 'Responsive')          -- horizontal, for a topbar\n" +
			"      SIMPLEMENUBAR name (Menu: Module.Menu        -- a phone layout's bottom bar\n" +
			"                    [, Orientation: Horizontal | Vertical])\n" +
			"      -- Every menu widget renders EITHER a navigation profile (Profile: '…')\n" +
			"      -- OR a menu document (Menu: Module.Menu, see CREATE MENU) — not both.\n" +
			"      PLACEHOLDER Main\n" +
			"    }\n" +
			"  }\n" +
			"}",
		Example: "mdl 1;\n" +
			"-- Mendix's own guidance is not to edit the supplied Atlas layouts:\n" +
			"-- a Marketplace update replaces the module and the edit is gone.\n" +
			"-- Create the layout in a module you own instead.\n" +
			"--\n" +
			"-- The class is load-bearing: Atlas scopes ~24 of its layout rules to\n" +
			"-- .layout-atlas, so a layout without one builds clean and renders with\n" +
			"-- no topbar bar and no sidebar rail.\n" +
			"CREATE OR MODIFY LAYOUT MyModule.App_Default (\n" +
			"  layouttype: 'Responsive',\n" +
			"  class: 'layout-atlas layout-atlas-responsive-topbar'\n" +
			") {\n" +
			"  SCROLLCONTAINER layoutContainer {\n" +
			"    REGION top (Size: 60, SizeMode: 'Fixed', Class: 'region-topbar') {\n" +
			"      SNIPPETCALL topbar (Snippet: MyModule.SNIPPET_TopBar)\n" +
			"    }\n" +
			"    REGION left (Size: 232, SizeMode: 'Pixels', Class: 'region-sidebar') {\n" +
			"      NAVIGATIONTREE navMenu (Profile: 'Responsive')\n" +
			"    }\n" +
			"    REGION center (Class: 'region-content') {\n" +
			"      PLACEHOLDER Main\n" +
			"    }\n" +
			"  }\n" +
			"};\n" +
			"\n" +
			"-- A phone layout's bottom bar, as Atlas_Core.Phone_BottomBar has it —\n" +
			"-- a simple menu bar rendering a menu document:\n" +
			"CREATE OR MODIFY MENU MyModule.Phone_Menu {\n" +
			"  menu item 'Home' ( OnClick: show page MyModule.Home_Phone, Icon: Atlas_Core.Atlas.home )\n" +
			"};\n" +
			"CREATE OR MODIFY LAYOUT MyModule.Phone_Bottom (\n" +
			"  layouttype: 'Phone',\n" +
			"  class: 'layout-atlas layout-atlas-phone'\n" +
			") {\n" +
			"  SCROLLCONTAINER scrollContainer1 {\n" +
			"    REGION bottom (Class: 'region-bottombar') {\n" +
			"      SIMPLEMENUBAR bottomBar (Menu: MyModule.Phone_Menu, Class: 'bottom-nav-text-icons')\n" +
			"    }\n" +
			"    REGION center (Class: 'region-content') {\n" +
			"      PLACEHOLDER Main\n" +
			"    }\n" +
			"  }\n" +
			"};\n" +
			"\n" +
			"-- A page binds to a placeholder by name:\n" +
			"CREATE PAGE MyModule.Home (Title: 'Home', Layout: MyModule.App_Default) {\n" +
			"  TITLE t (Content: 'Welcome')\n" +
			"};\n" +
			"\n" +
			"-- A placeholder's name is API: a page references it as Module.Layout.<Name>,\n" +
			"-- so renaming one unbinds every page that used it.\n" +
			"--\n" +
			"-- EXACTLY ONE placeholder must be named Main. This is a rule mxbuild\n" +
			"-- enforces, not a convention: a layout with none fails CE0848 and one\n" +
			"-- with two fails CE0849 — even if no page uses the layout. Placeholder\n" +
			"-- names must also be unique (CE0495). Extra placeholders under other\n" +
			"-- names are fine. mxcli reports all three as MDL081/MDL082.\n" +
			"--\n" +
			"-- A placeholder is DECLARED with no body. `PLACEHOLDER Main { … }` is the\n" +
			"-- page-side spelling that FILLS a slot; in a layout it declares nothing\n" +
			"-- and is reported as MDL083 — unless the layout is built on another\n" +
			"-- one (`Layout:` in the header, see layout.nested).",
		SeeAlso: []string{"layout.nested", "layout.show", "layout.drop", "page.create", "snippet"},
	})

	Register(SyntaxFeature{
		Path:    "layout.nested",
		Summary: "A layout built on another layout — fills the outer layout's placeholders the way a page does",
		Keywords: []string{
			"nested layout", "layout on layout", "master layout", "base layout",
			"layout call", "wrap layout", "inherit layout", "layout: header",
		},
		Syntax: "CREATE [OR MODIFY] LAYOUT Module.Name (\n" +
			"  layouttype: 'Responsive' | 'Phone' | 'Tablet' | 'ModalPopup',   -- web only\n" +
			"  Layout: Module.Outer\n" +
			") {\n" +
			"  PLACEHOLDER <OuterPlaceholder> {   -- fills Module.Outer.<OuterPlaceholder>\n" +
			"    -- widgets, and the placeholders THIS layout declares:\n" +
			"    PLACEHOLDER Main\n" +
			"  }\n" +
			"}",
		Example: "mdl 1;\n" +
			"-- The body is a page body: `placeholder X { … }` fills the outer\n" +
			"-- layout's placeholder X, and bare widgets fill its Main. Inside the\n" +
			"-- blocks, a bodiless `placeholder Main` declares the slot this layout's\n" +
			"-- own pages bind to — it still needs exactly one named Main (CE0848).\n" +
			"CREATE OR MODIFY LAYOUT MyModule.Layout_Framed (\n" +
			"  layouttype: 'Responsive',\n" +
			"  Layout: Atlas_Core.Atlas_Default\n" +
			") {\n" +
			"  PLACEHOLDER Main {\n" +
			"    CONTAINER banner (Class: 'card') {\n" +
			"      DYNAMICTEXT heading (Content: 'Framed area', RenderMode: H3)\n" +
			"    }\n" +
			"    PLACEHOLDER Main\n" +
			"  }\n" +
			"};\n" +
			"\n" +
			"-- Pages use it like any layout:\n" +
			"CREATE PAGE MyModule.Home (Title: 'Home', Layout: MyModule.Layout_Framed) {\n" +
			"  TITLE t (Content: 'Welcome')\n" +
			"};\n" +
			"\n" +
			"-- A block naming a placeholder the outer layout does not declare is\n" +
			"-- refused (mxbuild: CE1613). Native layouts cannot be built on another\n" +
			"-- layout. DESCRIBE LAYOUT prints this form, one block per argument.",
		SeeAlso: []string{"layout", "page.create"},
	})

	Register(SyntaxFeature{
		Path:    "layout.drop",
		Summary: "DROP LAYOUT — remove a layout (warns about pages still bound to it)",
		Keywords: []string{
			"drop layout", "delete layout", "remove layout",
		},
		Syntax: "DROP LAYOUT Module.Name",
		Example: "mdl 1;\n" +
			"DROP LAYOUT MyModule.App_Old;\n\n" +
			"-- Pages still bound to it are named in a warning, and the drop proceeds.\n" +
			"-- Left dropped, each of those pages fails the build with CE1613 — which\n" +
			"-- names the PAGE and never the layout, so repoint them first:\n" +
			"ALTER PAGES SET LAYOUT = MyModule.App_New WHERE LAYOUT = MyModule.App_Old;\n" +
			"DROP LAYOUT MyModule.App_Old;\n\n" +
			"-- Or correct a layout in place by re-creating it under the same name:\n" +
			"-- the pages stay bound by qualified name and rebind to the new document.\n" +
			"-- (CREATE OR MODIFY LAYOUT does this in one statement.)",
		SeeAlso: []string{"layout", "layout.alter", "layout.show"},
	})

	Register(SyntaxFeature{
		Path:    "layout.alter",
		Summary: "ALTER LAYOUT — edit a layout in place; repoint pages onto a different one",
		Keywords: []string{
			"alter layout", "modify layout", "set layout", "repoint pages",
			"alter pages", "change layout", "migrate layout", "region insert",
		},
		Syntax: "-- Edit a layout in place (same operations as ALTER PAGE):\n" +
			"ALTER LAYOUT Module.Name {\n" +
			"  INSERT INTO <scrollContainer>.<top|right|bottom|left|center> { <widgets> };\n" +
			"  INSERT BEFORE|AFTER <widgetName> { <widgets> };\n" +
			"  SET (<property>: <value>) ON <widgetName>;\n" +
			"  DROP <name1>, <name2>;\n" +
			"  REPLACE <widgetName> WITH { <widgets> };\n" +
			"};\n\n" +
			"-- Point one page at a different layout:\n" +
			"ALTER PAGE Module.Page { SET Layout = Module.Layout [MAP (Old AS New, …)]; };\n\n" +
			"-- Point many at once (the migration form):\n" +
			"ALTER PAGES [IN <module>] SET LAYOUT = Module.Layout\n" +
			"  [MAP (Old AS New, …)] [WHERE LAYOUT = Module.OldLayout];",
		Example: "mdl 1;\n" +
			"-- Move every page off the Atlas layout onto your own:\n" +
			"ALTER PAGES SET LAYOUT = MyModule.App_Default\n" +
			"  WHERE LAYOUT = Atlas_Core.Atlas_Default;\n\n" +
			"-- A page bound to a placeholder the new layout lacks is refused; rebind it:\n" +
			"ALTER PAGE MyModule.Split { SET Layout = MyModule.Minimal MAP (HeaderLeft AS Main); };\n\n" +
			"-- Add a topbar snippet to a layout without rewriting the document:\n" +
			"ALTER LAYOUT MyModule.App_Default {\n" +
			"  INSERT INTO layoutContainer.top { SNIPPETCALL bar (Snippet: MyModule.SNIPPET_ThemeBar) };\n" +
			"};\n\n" +
			"-- A region has no name of its own — its slot is its identity — so it is\n" +
			"-- addressed as <scrollContainer>.<slot>. Only INSERT INTO takes a region;\n" +
			"-- BEFORE/AFTER position a widget among siblings, so name the widget.\n" +
			"-- Pages in Marketplace modules are skipped and named; ALTER LAYOUT refuses\n" +
			"-- a Marketplace target outright. Prefer ALTER over CREATE OR MODIFY for a\n" +
			"-- layout you did not author: a rewrite is only as complete as its describe.",
		SeeAlso: []string{"layout", "layout.show", "page.alter"},
	})

	Register(SyntaxFeature{
		Path:    "layout.show",
		Summary: "List and describe layouts (DESCRIBE emits re-executable CREATE LAYOUT)",
		Keywords: []string{
			"list layouts", "list layouts", "describe layout",
		},
		Syntax:  "LIST LAYOUTS;\nLIST LAYOUTS IN <module>;\nDESCRIBE LAYOUT Module.Name;",
		Example: "-- Copy an Atlas layout into your own module: describe it, rename it, run it.\nDESCRIBE LAYOUT Atlas_Core.Atlas_Default;",
		SeeAlso: []string{"layout", "page.show"},
	})

	// ── Building Block ────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "buildingblock.show",
		Summary: "List and describe building blocks (read-only)",
		Keywords: []string{
			"list building blocks", "list building blocks", "describe building block",
			"building block", "building blocks",
		},
		Syntax:  "LIST BUILDING BLOCKS;\nLIST BUILDING BLOCKS IN <module>;\nDESCRIBE BUILDING BLOCK Module.Name;",
		Example: "LIST BUILDING BLOCKS IN MyModule;\nDESCRIBE BUILDING BLOCK MyModule.LoginForm;",
		SeeAlso: []string{"snippet.show", "page.show"},
	})

	Register(SyntaxFeature{
		Path:    "navigation.menu-document",
		Summary: "Create, describe and drop standalone menu documents (Menus$MenuDocument)",
		Keywords: []string{
			"create menu", "describe menu", "drop menu",
			"menu", "menus", "menu document", "menu item",
		},
		Syntax: "CREATE [OR MODIFY] MENU Module.Name [FOLDER 'path'] {\n" +
			"  MENU ITEM '<caption>' [( [OnClick: SHOW PAGE Module.Page | CALL MICROFLOW Module.Flow | SIGN OUT]\n" +
			"                          [, Icon: Module.Collection.name | GLYPH <n> | IMAGE Module.Images.name] )]\n" +
			"  MENU '<caption>' [( Icon: Module.Collection.name )] { <nested items> }\n" +
			"};\n" +
			"DESCRIBE MENU Module.Name;\n" +
			"DROP MENU Module.Name;",
		Example: "CREATE OR MODIFY MENU MyModule.Main_Menu {\n" +
			"  menu item 'Home' ( OnClick: show page MyModule.Home_Web, Icon: Atlas_Core.Atlas.home )\n" +
			"  menu item 'Run' ( OnClick: call microflow MyModule.DoThing )\n" +
			"  menu 'Admin' {\n" +
			"    menu item 'Accounts' ( OnClick: show page Administration.Account_Overview )\n" +
			"  }\n" +
			"  menu item 'Plain'\n" +
			"};\n\n" +
			"-- Notes:\n" +
			"--   * A menu document is the reusable menu a menu widget points at. It is\n" +
			"--     NOT the menu inside a navigation profile — for that use\n" +
			"--     LIST NAVIGATION MENU and ALTER NAVIGATION. Both use these same items.\n" +
			"--   * OR MODIFY replaces the item list wholesale; an omitted item is removed.\n" +
			"--     The document's identity and export level are preserved.\n" +
			"--   * SIGN OUT is the log-out menu item. It needs no target and stores the\n" +
			"--     same Forms$SignOutClientAction a sign-out BUTTON carries. Works both\n" +
			"--     here and in a navigation profile's menu.\n" +
			"--   * The items are children, in { } with no ; between them (R2). The old\n" +
			"--     spelling, ( MENU ITEM 'X' PAGE M.P ICON I; ... ), still parses and warns\n" +
			"--     (MDL-DEPR121, MDL-DEPR122).\n" +
			"--   * Icon: names an icon collection entry; GLYPH <n> and IMAGE M.Images.x\n" +
			"--     write the other two icon kinds.\n" +
			"--   * A page with required parameters cannot be opened from a menu item\n" +
			"--     without an argument — Mendix reports CE1571.",
		SeeAlso: []string{"navigation.create", "navigation.show", "page.show"},
	})

	// ── Fragment ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "fragment",
		Summary: "Transient reusable widget groups defined at script scope",
		Keywords: []string{
			"fragment", "fragments", "reusable widgets", "define fragment",
			"use fragment", "template", "script scope",
		},
		Syntax:  "CREATE FRAGMENT Name AS { <widgets> };\nCREATE FRAGMENT Name AS { <widgets> SLOT [name] <widgets> };\nCREATE FRAGMENT Name ($d: datasource, $a: action) AS { <widgets> };\nUSE FRAGMENT Name [(args)] [AS prefix_];\nUSE FRAGMENT Name [(args)] [AS prefix_] { <payload widgets> };\nLIST FRAGMENTS;\nDESCRIBE FRAGMENT Name;\nDESCRIBE FRAGMENT FROM PAGE Module.Page WIDGET widgetName;",
		Example: "mdl 1;\nCREATE FRAGMENT SaveCancelFooter AS {\n  FOOTER footer1 {\n    ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)\n    ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)\n  }\n};\n\nCREATE PAGE Module.EditPage (Params: ( $Param: Module.Customer ), Title: 'Edit', Layout: 'Atlas_Core.Atlas_Default') {\n  DATAVIEW dv (DataSource: $Param) {\n    TEXTBOX txtName (Label: 'Name', Attribute: Name)\n    USE FRAGMENT SaveCancelFooter\n  }\n};",
		SeeAlso: []string{"fragment.define", "fragment.use", "fragment.slot", "fragment.params", "snippet"},
	})

	Register(SyntaxFeature{
		Path:    "fragment.define",
		Summary: "CREATE FRAGMENT — declare a reusable widget group for the script session",
		Keywords: []string{
			"define fragment", "declare fragment", "create fragment",
		},
		Syntax:  "CREATE FRAGMENT Name AS {\n  <widgets>\n};",
		Example: "CREATE FRAGMENT FormFields AS {\n  TEXTBOX txtName (Label: 'Name', Attribute: Name)\n  TEXTBOX txtEmail (Label: 'Email', Attribute: Email)\n};",
		SeeAlso: []string{"fragment", "fragment.use"},
	})

	Register(SyntaxFeature{
		Path:    "fragment.use",
		Summary: "USE FRAGMENT — insert fragment widgets at current position with optional prefix",
		Keywords: []string{
			"use fragment", "insert fragment", "expand fragment",
			"prefix", "name conflict",
		},
		Syntax:  "USE FRAGMENT Name\nUSE FRAGMENT Name AS prefix_\nUSE FRAGMENT Name [AS prefix_] { <payload widgets> }",
		Example: "-- Basic usage\nCREATE PAGE Module.Page (Params: ( $Param: Module.Customer ), Title: 'Page', Layout: 'Atlas_Core.Atlas_Default') {\n  DATAVIEW dv (DataSource: $Param) {\n    USE FRAGMENT FormFields\n    USE FRAGMENT SaveCancelFooter\n  }\n};\n\n-- With prefix to avoid name conflicts\nUSE FRAGMENT SaveCancelFooter AS order_\n-- Creates: order_footer1, order_btnSave, order_btnCancel\n\n-- Fill a fragment's content slot (see fragment.slot)\nUSE FRAGMENT Card {\n  DYNAMICTEXT cardHeading (Content: 'Welcome', RenderMode: H2)\n  DYNAMICTEXT cardText (Content: 'Wrapped content')\n}",
		SeeAlso: []string{"fragment", "fragment.define", "fragment.slot"},
	})

	Register(SyntaxFeature{
		Path:    "fragment.params",
		Summary: "Fragment datasource/action parameters + building-block rebind overrides (experimental)",
		Keywords: []string{
			"fragment parameter", "fragment param", "datasource param", "action param",
			"binding", "rebind", "building block override", "reusable component",
		},
		Syntax:  "-- Declare typed params, reference with $name in a datasource/action slot:\nCREATE FRAGMENT Name ($data: datasource, $onEdit: action) AS { … };\n-- Supply values at the use site:\nUSE FRAGMENT Name ($data: <datasource>, $onEdit: <action>) [{ payload }]\n-- Building blocks: rebind the outermost datasource / first button:\nUSE BUILDING BLOCK Module.Block (datasource: <ds>, action: <action>) [AS prefix_]",
		Example: "CREATE FRAGMENT DataPanel ($data: datasource, $onEdit: action) AS {\n  CONTAINER panel (Class: 'card') {\n    LISTVIEW lv (DataSource: $data) {\n      SLOT content\n      ACTIONBUTTON edit (Caption: 'Edit', Action: $onEdit, ButtonStyle: Primary)\n    }\n  }\n};\n\nCREATE PAGE Module.Orders (Title: 'Orders', Layout: Atlas_Core.Atlas_Default) {\n  USE FRAGMENT DataPanel ($data: DATABASE Sales.Order, $onEdit: MICROFLOW Sales.Edit) {\n    DYNAMICTEXT heading (Content: 'Orders', RenderMode: H4)\n  }\n};\n\n-- Rebind a building block's datasource and primary button:\nUSE BUILDING BLOCK Atlas_Web_Content.List_Cards (datasource: DATABASE Sales.Order, action: CALL MICROFLOW Sales.Open) AS orders_\n\n-- Notes:\n--   * Param kinds: datasource | action. Every declared param must be supplied.\n--   * A microflow value parses as a datasource and is reinterpreted for an action param.\n--   * BB binding-point rule: datasource → first datasource widget; action → first button.",
		SeeAlso: []string{"fragment", "fragment.slot", "fragment.use"},
	})

	Register(SyntaxFeature{
		Path:    "fragment.slot",
		Summary: "SLOT — a content placeholder that lets a fragment wrap arbitrary caller content",
		Keywords: []string{
			"slot", "content slot", "fragment slot", "wrap content",
			"card wrapper", "reusable shell", "use fragment payload",
		},
		Syntax:  "-- In the definition, mark where caller content lands:\nCREATE FRAGMENT Name AS { <wrapper> SLOT [name] <wrapper> };\n-- At the use site, supply the payload in a brace block:\nUSE FRAGMENT Name [AS prefix_] { <payload widgets> }",
		Example: "mdl 1;\nCREATE FRAGMENT Card AS {\n  CONTAINER cardWrap (Class: 'card', DesignProperties: ('Card style': on)) {\n    CONTAINER cardBody (Class: 'card-body') {\n      SLOT content\n    }\n  }\n};\n\nCREATE PAGE Module.Dashboard (Title: 'Dashboard', Layout: Atlas_Core.Atlas_Default) {\n  USE FRAGMENT Card {\n    DYNAMICTEXT cardHeading (Content: 'Welcome', RenderMode: H2)\n    DYNAMICTEXT cardText (Content: 'Any widgets can go inside the reusable Card shell')\n  }\n};\n\n-- Notes:\n--   * Slot name is optional (defaults to 'content'); one slot per fragment.\n--   * USE FRAGMENT with no payload leaves the slot empty (valid).\n--   * Supplying a payload to a slotless fragment is an error.",
		SeeAlso: []string{"fragment", "fragment.define", "fragment.use"},
	})

	Register(SyntaxFeature{
		Path:    "fragment.show",
		Summary: "SHOW/DESCRIBE FRAGMENTS and extract widget subtrees from existing pages",
		Keywords: []string{
			"list fragments", "describe fragment", "list fragments",
			"extract widget", "widget subtree",
		},
		Syntax:  "LIST FRAGMENTS;\nDESCRIBE FRAGMENT Name;\nDESCRIBE FRAGMENT FROM PAGE Module.Page WIDGET widgetName;\nDESCRIBE FRAGMENT FROM SNIPPET Module.Snippet WIDGET widgetName;",
		Example: "LIST FRAGMENTS;\nDESCRIBE FRAGMENT SaveCancelFooter;\n\n-- Extract a widget subtree from an existing page\nDESCRIBE FRAGMENT FROM PAGE Module.MyPage WIDGET footer1;",
		SeeAlso: []string{"fragment", "fragment.define"},
	})
}
