// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Completion handles textDocument/completion requests.
func (s *mdlServer) Completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	// Get current line text to provide context-aware completions
	docURI := uri.URI(params.TextDocument.URI)
	s.mu.Lock()
	text := s.docs[docURI]
	s.mu.Unlock()

	linePrefix := ""
	if text != "" {
		lines := strings.Split(text, "\n")
		line := int(params.Position.Line)
		if line < len(lines) {
			col := min(int(params.Position.Character), len(lines[line]))
			linePrefix = strings.TrimLeft(lines[line][:col], " \t")
		}
	}
	linePrefixUpper := strings.ToUpper(linePrefix)

	// Check if typing a $ variable reference inside page/snippet context
	if strings.Contains(linePrefix, "$") {
		varItems := s.variableCompletionItems(text, linePrefix, int(params.Position.Line))
		if len(varItems) > 0 {
			return &protocol.CompletionList{
				IsIncomplete: false,
				Items:        varItems,
			}, nil
		}
	}

	// Check if cursor is inside a widget property block — suggest the widget's
	// known property keys from its .def.json (catches typos like
	// `expanBehavior` before mxcli check ever runs).
	if propItems := s.widgetPropertyCompletionItems(text, linePrefix, int(params.Position.Line)); propItems != nil {
		return &protocol.CompletionList{
			IsIncomplete: false,
			Items:        propItems,
		}, nil
	}

	// Check if context calls for catalog-based element completion
	if types := inferCompletionTypes(linePrefixUpper); types != nil {
		items := s.catalogCompletionItems(ctx, linePrefix, types)
		return &protocol.CompletionList{
			IsIncomplete: false,
			Items:        items,
		}, nil
	}

	items := s.mdlCompletionItems(linePrefixUpper)
	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

// CompletionResolve handles completionItem/resolve requests.
func (s *mdlServer) CompletionResolve(ctx context.Context, params *protocol.CompletionItem) (*protocol.CompletionItem, error) {
	return params, nil
}

// mdlCompletionItems returns completion items filtered by context.
func (s *mdlServer) mdlCompletionItems(linePrefixUpper string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// After CREATE, suggest object types and CREATE snippets only
	if strings.HasPrefix(linePrefixUpper, "CREATE ") || linePrefixUpper == "CREATE" {
		items = append(items, mdlCreateSnippets...)
		items = append(items, mdlCreateContextKeywords...)
		return items
	}

	// After SHOW, suggest showable objects
	if strings.HasPrefix(linePrefixUpper, "SHOW ") || linePrefixUpper == "SHOW" {
		items = append(items, mdlShowContextKeywords...)
		return items
	}

	// General context: all generated keywords + snippets + widget types
	items = append(items, mdlGeneratedKeywords...)
	items = append(items, mdlStatementSnippets...)
	items = append(items, mdlCreateSnippets...)
	items = append(items, s.widgetRegistryCompletions()...)

	return items
}

// widgetRegistryCompletions returns completion items for registered widget types,
// including user-defined widgets from global (~/.mxcli/widgets/) and project-level
// (.mxcli/widgets/) directories.
// NOTE: Cached via sync.Once — new .def.json files added while the LSP server is
// running will not appear until the server is restarted.
func (s *mdlServer) widgetRegistryCompletions() []protocol.CompletionItem {
	s.ensureWidgetRegistry()
	return s.widgetCompletionItems
}

// ensureWidgetRegistry loads the widget registry once per server lifetime,
// caching the result on s.widgetRegistry and s.widgetCompletionItems.
// Safe to call from any goroutine.
// NOTE: new .def.json files added while the LSP server is running will not
// appear until the server is restarted.
func (s *mdlServer) ensureWidgetRegistry() {
	s.widgetCompletionsOnce.Do(func() {
		registry, err := executor.NewWidgetRegistry()
		if err != nil {
			return
		}
		if err := registry.LoadUserDefinitions(s.mprPath); err != nil {
			log.Printf("warning: loading user widget definitions for LSP: %v", err)
		}
		s.widgetRegistry = registry
		for _, def := range registry.All() {
			s.widgetCompletionItems = append(s.widgetCompletionItems, protocol.CompletionItem{
				Label:  def.MDLName,
				Kind:   protocol.CompletionItemKindClass,
				Detail: "Pluggable widget: " + def.WidgetID,
			})
		}
	})
}

// mdlCreateContextKeywords are object types suggested after CREATE.
// These are hand-written because they require semantic knowledge of what can be created.
var mdlCreateContextKeywords = []protocol.CompletionItem{
	kw("ENTITY", "Entity type"),
	kw("PERSISTENT", "Persistent entity modifier"),
	kw("NON-PERSISTENT", "Non-persistent entity modifier"),
	kw("VIEW", "View entity modifier"),
	kw("EXTERNAL", "External entity modifier"),
	kw("MICROFLOW", "Microflow type"),
	kw("NANOFLOW", "Nanoflow type"),
	kw("PAGE", "Page type"),
	kw("SNIPPET", "Snippet type"),
	kw("LAYOUT", "Layout type"),
	kw("ENUMERATION", "Enumeration type"),
	kw("ASSOCIATION", "Association type"),
	kw("CONSTANT", "Constant type"),
	kw("MODULE", "Module type"),
	kw("JAVA ACTION", "Java action type"),
}

// mdlShowContextKeywords are items suggested after SHOW.
var mdlShowContextKeywords = []protocol.CompletionItem{
	kw("ENTITIES", "List entities"),
	kw("MICROFLOWS", "List microflows"),
	kw("NANOFLOWS", "List nanoflows"),
	kw("PAGES", "List pages"),
	kw("SNIPPETS", "List snippets"),
	kw("LAYOUTS", "List layouts"),
	kw("ENUMERATIONS", "List enumerations"),
	kw("MODULES", "List modules"),
	kw("ASSOCIATIONS", "List associations"),
	kw("CONSTANTS", "List constants"),
	kw("WIDGETS", "List widgets"),
	kw("CALLERS", "Show callers of element"),
	kw("CALLEES", "Show callees of element"),
	kw("REFERENCES", "Show references to element"),
	kw("IMPACT", "Show impact analysis"),
	kw("CONTEXT", "Show context of element"),
	kw("CATALOG", "Show catalog info"),
	kw("DATABASE CONNECTIONS", "List database connections"),
}

func kw(label string, detail string) protocol.CompletionItem {
	return protocol.CompletionItem{
		Label:  label,
		Kind:   protocol.CompletionItemKindKeyword,
		Detail: detail,
	}
}

func snippet(label, insertText, detail string) protocol.CompletionItem {
	return protocol.CompletionItem{
		Label:            label,
		Kind:             protocol.CompletionItemKindSnippet,
		Detail:           detail,
		InsertText:       insertText,
		InsertTextFormat: protocol.InsertTextFormatSnippet,
	}
}

var mdlCreateSnippets = []protocol.CompletionItem{
	snippet("CREATE ENTITY", "CREATE ENTITY ${1:Module}.${2:EntityName}\n(\n\t${3:AttributeName} : ${4:String}\n);", "Create a new entity"),
	snippet("CREATE PERSISTENT ENTITY", "CREATE PERSISTENT ENTITY ${1:Module}.${2:EntityName}\n(\n\t${3:AttributeName} : ${4:String}\n);", "Create a persistent entity"),
	snippet("CREATE NON_PERSISTENT ENTITY", "CREATE NON_PERSISTENT ENTITY ${1:Module}.${2:EntityName}\n(\n\t${3:AttributeName} : ${4:String}\n);", "Create a non-persistent entity"),
	snippet("CREATE MICROFLOW", "CREATE MICROFLOW ${1:Module}.${2:MicroflowName}\nBEGIN\n\t$0\nEND;", "Create a new microflow"),
	snippet("CREATE MICROFLOW (with params)", "CREATE MICROFLOW ${1:Module}.${2:MicroflowName}\n(\n\t$$${3:Param}: ${4:Module.Entity}\n)\nRETURNS ${5:Boolean} AS $$${6:Result}\nBEGIN\n\t$0\nEND;", "Create microflow with parameters"),
	snippet("CREATE NANOFLOW", "CREATE NANOFLOW ${1:Module}.${2:NanoflowName}\nBEGIN\n\t$0\nEND;", "Create a new nanoflow"),
	snippet("CREATE NANOFLOW (with params)", "CREATE NANOFLOW ${1:Module}.${2:NanoflowName}\n(\n\t$$${3:Param}: ${4:Module.Entity}\n)\nRETURNS ${5:Boolean} AS $$${6:Result}\nBEGIN\n\t$0\nEND;", "Create nanoflow with parameters"),
	snippet("CREATE ENUMERATION", "CREATE ENUMERATION ${1:Module}.${2:EnumName}\n(\n\t'${3:Value1}' '${4:Caption1}',\n\t'${5:Value2}' '${6:Caption2}'\n);", "Create a new enumeration"),
	snippet("CREATE CONSTANT", "CREATE CONSTANT ${1:Module}.${2:ConstantName}\nTYPE ${3|String,Integer,Long,Decimal,Boolean,DateTime|}\nDEFAULT ${4:'value'};", "Create a new constant"),
	snippet("CREATE PAGE", "CREATE PAGE ${1:Module}.${2:PageName}\n(\n\tTitle: '${3:Page Title}',\n\tLayout: ${4:Atlas_Core.Atlas_Default}\n)\n{\n\t$0\n}", "Create a new page"),
	snippet("CREATE SNIPPET", "CREATE SNIPPET ${1:Module}.${2:SnippetName}\n{\n\t$0\n}", "Create a new snippet"),
	snippet("CREATE ASSOCIATION", "CREATE ASSOCIATION ${1:Module}.${2:AssocName}\nFROM ${1:Module}.${3:ChildEntity}\nTO ${1:Module}.${4:ParentEntity}\nTYPE ${5|Reference,ReferenceSet|};", "Create a new association"),
	snippet("CREATE MODULE", "CREATE MODULE ${1:ModuleName};", "Create a new module"),
}

var mdlStatementSnippets = []protocol.CompletionItem{
	snippet("IF ... END IF", "IF ${1:condition} THEN\n\t$0\nEND IF;", "If-then block"),
	snippet("IF ... ELSE ... END IF", "IF ${1:condition} THEN\n\t${2}\nELSE\n\t$0\nEND IF;", "If-then-else block"),
	snippet("LOOP ... END LOOP", "LOOP $$${1:Item} IN $$${2:List}\nBEGIN\n\t$0\nEND LOOP;", "Loop over a list"),
	snippet("WHILE ... END WHILE", "WHILE ${1:condition}\nBEGIN\n\t$0\nEND WHILE;", "While loop with condition"),
	snippet("DECLARE variable", "DECLARE $$${1:Var} ${2:String} = ${3:''};", "Declare a variable"),
	snippet("RETRIEVE ... FROM", "RETRIEVE $$${1:Var} FROM ${2:Module.Entity} WHERE ${3:condition};", "Retrieve from database"),
	snippet("RETRIEVE ... FROM $Var/Assoc", "RETRIEVE $$${1:List} FROM $$${2:Parent}/${3:Module.AssociationName};", "Retrieve by association"),
	snippet("DATAVIEW", "DATAVIEW ${1:dvName} (DataSource: $$${2:Var}) {\n\t$0\n}", "Data view widget"),
	snippet("INDEX", "INDEX (${1:AttributeName});", "Entity index"),
	snippet("CALL MICROFLOW", "$$${1:Result} = CALL MICROFLOW ${2:Module.MicroflowName}(${3});", "Call a microflow"),
	snippet("CALL NANOFLOW", "$$${1:Result} = CALL NANOFLOW ${2:Module.NanoflowName}(${3});", "Call a nanoflow"),
	snippet("CALL JAVASCRIPT ACTION", "$$${1:Result} = CALL JAVASCRIPT ACTION ${2:Module.ActionName}(${3});", "Call a JavaScript action"),
	snippet("CALL JAVA ACTION", "$$${1:Result} = CALL JAVA ACTION ${2:Module.ActionName}(${3});", "Call a Java action"),
}

// inferCompletionTypes examines the line prefix and returns the ObjectType
// values to filter catalog elements on, or nil if no catalog completion applies.
func inferCompletionTypes(linePrefixUpper string) []string {
	// Patterns ordered from most specific to least specific.
	// Each entry: prefix to match → element types to suggest.
	patterns := []struct {
		prefix string
		types  []string
	}{
		// Microflow/nanoflow calls
		{"CALL MICROFLOW ", []string{"MICROFLOW"}},
		{"CALL NANOFLOW ", []string{"NANOFLOW"}},
		{"CALL JAVA ACTION ", []string{"JAVA_ACTION"}},

		// Page actions
		{"SHOW PAGE ", []string{"PAGE"}},

		// Retrieve
		{"RETRIEVE ", []string{"ENTITY"}}, // matches "RETRIEVE $x FROM Module." too

		// Widget datasource patterns
		{"DATASOURCE: DATABASE ", []string{"ENTITY"}},
		{"DATASOURCE: MICROFLOW ", []string{"MICROFLOW"}},
		{"DATASOURCE: NANOFLOW ", []string{"NANOFLOW"}},

		// Widget action patterns
		{"ACTION: SHOW_PAGE ", []string{"PAGE"}},
		{"ACTION: MICROFLOW ", []string{"MICROFLOW"}},
		{"ACTION: NANOFLOW ", []string{"NANOFLOW"}},
		{"ACTION: CREATE_OBJECT ", []string{"ENTITY"}},

		// Page properties
		{"LAYOUT: ", []string{"LAYOUT"}},
		{"SNIPPET: ", []string{"SNIPPET"}},

		// Snippet call widget
		{"SNIPPETCALL ", []string{"SNIPPET"}},
	}

	for _, p := range patterns {
		if strings.HasPrefix(linePrefixUpper, p.prefix) {
			return p.types
		}
		// Also check if the pattern appears after other content on the line
		// e.g. "  DataSource: DATABASE " preceded by whitespace
		if idx := strings.LastIndex(linePrefixUpper, p.prefix); idx >= 0 {
			return p.types
		}
	}

	// RETRIEVE ... FROM pattern: "RETRIEVE $x FROM "
	if strings.Contains(linePrefixUpper, " FROM ") && strings.Contains(linePrefixUpper, "RETRIEVE ") {
		return []string{"ENTITY"}
	}

	// EXTENDS in entity context
	if strings.Contains(linePrefixUpper, "EXTENDS ") {
		return []string{"ENTITY"}
	}

	// Association FROM/TO patterns (but not RETRIEVE FROM which is handled above)
	if (strings.HasPrefix(linePrefixUpper, "FROM ") || strings.HasPrefix(linePrefixUpper, "TO ")) &&
		!strings.Contains(linePrefixUpper, "RETRIEVE") {
		return []string{"ENTITY"}
	}

	return nil
}

// catalogCompletionItems returns completion items from the catalog filtered by types.
func (s *mdlServer) catalogCompletionItems(ctx context.Context, linePrefix string, types []string) []protocol.CompletionItem {
	elems := s.getProjectElements(ctx)
	if len(elems) == 0 {
		return nil
	}

	// Build a set of allowed types
	typeSet := make(map[string]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}

	// Extract the partial text the user has typed after the trigger context.
	// e.g., "CALL MICROFLOW MyMod" → partial = "MyMod"
	partial := ""
	parts := strings.Fields(linePrefix)
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		// If the line ends with a space, no partial filter
		if !strings.HasSuffix(linePrefix, " ") {
			partial = last
		}
	}
	partialUpper := strings.ToUpper(partial)

	// Collect unique module names for module-level completion
	moduleSet := make(map[string]bool)

	var items []protocol.CompletionItem
	for _, elem := range elems {
		if !typeSet[elem.ObjectType] {
			continue
		}

		// Extract module name
		dotIdx := strings.Index(elem.QualifiedName, ".")
		if dotIdx > 0 {
			moduleSet[elem.QualifiedName[:dotIdx]] = true
		}

		// Filter by partial text
		if partialUpper != "" {
			nameUpper := strings.ToUpper(elem.QualifiedName)
			if !strings.Contains(nameUpper, partialUpper) {
				continue
			}
		}

		kind, detail := objectTypeToCompletionKind(elem.ObjectType)
		items = append(items, protocol.CompletionItem{
			Label:  elem.QualifiedName,
			Kind:   kind,
			Detail: detail,
		})
	}

	// If the partial has no dot, also suggest module names
	if !strings.Contains(partial, ".") {
		for mod := range moduleSet {
			modUpper := strings.ToUpper(mod)
			if partialUpper == "" || strings.Contains(modUpper, partialUpper) {
				items = append(items, protocol.CompletionItem{
					Label:  mod,
					Kind:   protocol.CompletionItemKindModule,
					Detail: "module",
				})
			}
		}
	}

	return items
}

// objectTypeToCompletionKind maps catalog ObjectType to LSP CompletionItemKind and detail text.
func objectTypeToCompletionKind(objectType string) (protocol.CompletionItemKind, string) {
	switch objectType {
	case "ENTITY":
		return protocol.CompletionItemKindClass, "entity"
	case "MICROFLOW":
		return protocol.CompletionItemKindMethod, "microflow"
	case "NANOFLOW":
		return protocol.CompletionItemKindMethod, "nanoflow"
	case "PAGE":
		return protocol.CompletionItemKindFile, "page"
	case "SNIPPET":
		return protocol.CompletionItemKindFile, "snippet"
	case "LAYOUT":
		return protocol.CompletionItemKindFile, "layout"
	case "ENUMERATION":
		return protocol.CompletionItemKindEnum, "enumeration"
	case "JAVA_ACTION":
		return protocol.CompletionItemKindMethod, "java action"
	case "WORKFLOW":
		return protocol.CompletionItemKindEvent, "workflow"
	case "CONSTANT":
		return protocol.CompletionItemKindConstant, "constant"
	case "JSON_STRUCTURE":
		return protocol.CompletionItemKindFile, "json structure"
	case "IMPORT_MAPPING":
		return protocol.CompletionItemKindFile, "import mapping"
	case "EXPORT_MAPPING":
		return protocol.CompletionItemKindFile, "export mapping"
	case "REST_CLIENT":
		return protocol.CompletionItemKindInterface, "rest client"
	case "PUBLISHED_REST_SERVICE":
		return protocol.CompletionItemKindInterface, "published rest service"
	case "ODATA_CLIENT":
		return protocol.CompletionItemKindInterface, "odata client"
	case "ODATA_SERVICE":
		return protocol.CompletionItemKindInterface, "odata service"
	case "DATABASE_CONNECTION":
		return protocol.CompletionItemKindInterface, "database connection"
	case "BUSINESS_EVENT_SERVICE":
		return protocol.CompletionItemKindEvent, "business event service"
	case "MODULE":
		return protocol.CompletionItemKindModule, "module"
	default:
		return protocol.CompletionItemKindValue, objectType
	}
}

// variableCompletionItems returns completion items for $ variable references.
// It suggests $currentObject (with entity type from enclosing data container) and
// any page parameters found in the document's CREATE PAGE Params declaration.
// cursorLine is the 0-based line number of the cursor position.
func (s *mdlServer) variableCompletionItems(docText string, linePrefix string, cursorLine int) []protocol.CompletionItem {
	// Extract the partial after the last $ to filter suggestions
	lastDollar := strings.LastIndex(linePrefix, "$")
	partial := ""
	if lastDollar >= 0 && lastDollar < len(linePrefix)-1 {
		partial = strings.ToUpper(linePrefix[lastDollar+1:])
	}

	var items []protocol.CompletionItem

	// Scan upward from cursor to find enclosing data container context
	entityType, widgetName := scanEnclosingDataContainer(docText, cursorLine)

	// Suggest $currentObject with entity type if found
	if partial == "" || strings.HasPrefix("CURRENTOBJECT", partial) {
		detail := "Current object from enclosing data container"
		if entityType != "" {
			detail = entityType
		}
		items = append(items, protocol.CompletionItem{
			Label:  "$currentObject",
			Kind:   protocol.CompletionItemKindVariable,
			Detail: detail,
		})
	}

	// Suggest $widgetName (selection) for list containers
	if widgetName != "" {
		wNameUpper := strings.ToUpper(widgetName)
		if partial == "" || strings.HasPrefix(wNameUpper, partial) {
			detail := "Selection from list container"
			if entityType != "" {
				detail = entityType + " (selection)"
			}
			items = append(items, protocol.CompletionItem{
				Label:  "$" + widgetName,
				Kind:   protocol.CompletionItemKindVariable,
				Detail: detail,
			})
		}
	}

	// Extract page parameter names from CREATE PAGE ... Params: { $Name: Type, ... }
	paramNames := extractPageParamNames(docText)
	for _, name := range paramNames {
		if partial == "" || strings.HasPrefix(strings.ToUpper(name), partial) {
			items = append(items, protocol.CompletionItem{
				Label:  "$" + name,
				Kind:   protocol.CompletionItemKindVariable,
				Detail: "Page parameter",
			})
		}
	}

	return items
}

// scanEnclosingDataContainer scans upward from cursorLine to find the nearest
// enclosing data container widget and its entity type.
// Returns (entityType, widgetName) where widgetName is set for list containers.
// Best-effort: uses brace matching and keyword scanning (Enclosing Scope Scanning pattern).
func scanEnclosingDataContainer(text string, cursorLine int) (string, string) {
	lines := strings.Split(text, "\n")
	if cursorLine >= len(lines) {
		return "", ""
	}

	// Track brace nesting depth as we scan upward
	depth := 0
	for i := cursorLine; i >= 0; i-- {
		line := lines[i]
		// Count braces on this line (right to left for correct nesting)
		for j := len(line) - 1; j >= 0; j-- {
			switch line[j] {
			case '}':
				depth++
			case '{':
				depth--
			}
		}

		// At depth < 0, we've found an opening brace that encloses the cursor
		if depth < 0 {
			trimmed := strings.TrimSpace(line)
			upper := strings.ToUpper(trimmed)

			// Check for data container keywords
			entityType, widgetName, isList := extractContainerInfo(upper, trimmed)
			if entityType != "" {
				if isList {
					return entityType, widgetName
				}
				return entityType, ""
			}
			// Reset depth — we passed through this opening brace but it wasn't a data container
			depth = 0
		}
	}
	return "", ""
}

// extractContainerInfo extracts entity type and widget name from a data container line.
// upperLine is the uppercase version, originalLine preserves case for widget name extraction.
// Returns (entityType, widgetName, isList).
func extractContainerInfo(upperLine string, originalLine string) (string, string, bool) {
	// Patterns: DATAVIEW name (DataSource: ...) {
	//           LISTVIEW name (DataSource: DATABASE FROM Module.Entity ...) {
	//           DATAGRID name (DataSource: DATABASE FROM Module.Entity ...) {
	//           GALLERY name (DataSource: DATABASE FROM Module.Entity ...) {
	type containerPattern struct {
		keyword string
		isList  bool
	}
	patterns := []containerPattern{
		{"DATAVIEW ", false},
		{"LISTVIEW ", true},
		{"DATAGRID ", true},
		{"GALLERY ", true},
	}

	for _, p := range patterns {
		if !strings.HasPrefix(upperLine, p.keyword) {
			continue
		}

		// Extract widget name (first token after keyword)
		rest := strings.TrimSpace(originalLine[len(p.keyword):])
		widgetName := ""
		for j, c := range rest {
			if c == ' ' || c == '(' || c == '{' {
				widgetName = rest[:j]
				break
			}
		}
		if widgetName == "" {
			widgetName = rest
		}

		// Extract entity from DataSource (use original case for entity name)
		entityType := extractEntityFromLine(originalLine)
		if entityType != "" {
			return entityType, widgetName, p.isList
		}
	}
	return "", "", false
}

// extractEntityFromLine extracts the entity type from a DataSource declaration in a line.
// Preserves original casing of the entity name (e.g., "Sales.Order" not "SALES.ORDER").
func extractEntityFromLine(line string) string {
	// Case-insensitive search for "DATABASE FROM" pattern
	upperLine := strings.ToUpper(line)
	if idx := strings.Index(upperLine, "DATABASE FROM "); idx >= 0 {
		rest := strings.TrimSpace(line[idx+len("DATABASE FROM "):])
		// Entity is the next qualified name (Module.Entity)
		end := strings.IndexAny(rest, " \t,)}")
		if end < 0 {
			end = len(rest)
		}
		entity := rest[:end]
		if strings.Contains(entity, ".") {
			return entity
		}
	}
	// MICROFLOW/NANOFLOW datasource — entity type not directly available
	// $ParamName datasource — would need to resolve param type
	return ""
}

// extractPageParamNames extracts parameter names from CREATE PAGE/SNIPPET parameter declarations.
// Best-effort: scans for "$Name: Type" patterns (colon after identifier distinguishes
// declarations from usage like "DataSource: $Param" or context comments like "$currentObject").
func extractPageParamNames(text string) []string {
	var names []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip DECLARE lines (variable declarations, not page params)
		if strings.HasPrefix(strings.ToUpper(trimmed), "DECLARE") {
			continue
		}
		// Skip comment lines
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		idx := 0
		for idx < len(trimmed) {
			dollar := strings.Index(trimmed[idx:], "$")
			if dollar < 0 {
				break
			}
			dollar += idx
			// Extract the identifier after $
			end := dollar + 1
			for end < len(trimmed) {
				c := trimmed[end]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
					end++
				} else {
					break
				}
			}
			if end > dollar+1 {
				name := trimmed[dollar+1 : end]
				// Only match parameter declarations: "$Name:" followed by a type.
				// Skip references like "DataSource: $Param" where $ is a value, not a declaration.
				rest := strings.TrimSpace(trimmed[end:])
				if strings.HasPrefix(rest, ":") {
					names = append(names, name)
				}
			}
			idx = end
		}
	}
	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			unique = append(unique, n)
		}
	}
	return unique
}

// widgetPropertyCompletionItems returns the widget's property keys when the
// cursor sits inside a widget's (...) property block. Returns nil when the
// cursor isn't in a widget context (so callers fall through to the generic
// keyword completions).
func (s *mdlServer) widgetPropertyCompletionItems(text, linePrefix string, cursorLine int) []protocol.CompletionItem {
	def := scanEnclosingWidget(text, cursorLine, len(linePrefix), s)
	if def == nil {
		return nil
	}

	// Extract a partial-word prefix the user is currently typing — last
	// run of letters before the cursor. Filter property suggestions to
	// those that start with it (case-insensitive).
	partial := trailingIdent(linePrefix)
	partialUpper := strings.ToUpper(partial)

	var items []protocol.CompletionItem
	seen := make(map[string]bool)

	addProp := func(key, detail string, kind protocol.CompletionItemKind) {
		if key == "" || seen[strings.ToLower(key)] {
			return
		}
		seen[strings.ToLower(key)] = true
		if partialUpper != "" && !strings.HasPrefix(strings.ToUpper(key), partialUpper) {
			return
		}
		items = append(items, protocol.CompletionItem{
			Label:      key,
			Kind:       kind,
			Detail:     detail,
			InsertText: key + ": ",
		})
	}

	for _, m := range def.PropertyMappings {
		addProp(m.PropertyKey, "Property ("+m.Operation+")", protocol.CompletionItemKindProperty)
	}
	for _, mode := range def.Modes {
		for _, m := range mode.PropertyMappings {
			detail := "Property (" + m.Operation + ")"
			if mode.Name != "" {
				detail += " [mode: " + mode.Name + "]"
			}
			addProp(m.PropertyKey, detail, protocol.CompletionItemKindProperty)
		}
	}
	for _, m := range def.ChildSlots {
		addProp(m.PropertyKey, "Child slot ("+m.MDLContainer+" block)", protocol.CompletionItemKindClass)
	}
	for _, m := range def.ObjectLists {
		addProp(m.PropertyKey, "Object list ("+m.MDLContainer+" blocks)", protocol.CompletionItemKindClass)
	}

	// Universal infrastructure properties
	for _, infra := range []struct{ name, detail string }{
		{"Class", "CSS class (Forms$Appearance.Class)"},
		{"Style", "Inline CSS (Forms$Appearance.Style)"},
		{"DesignProperties", "Atlas design properties"},
		{"Visible", "Conditional visibility"},
		{"Editable", "Conditional editability"},
	} {
		addProp(infra.name, infra.detail, protocol.CompletionItemKindProperty)
	}

	if len(items) == 0 {
		// Property block detected but partial matched nothing — return empty
		// list so we don't fall through to the generic keyword completions
		// (which would be noisy inside a widget property block).
		return []protocol.CompletionItem{}
	}
	return items
}

// scanEnclosingWidget scans backwards from the cursor to find the widget
// statement whose (...) property block the cursor is inside. Returns the
// widget's WidgetDefinition from the server's lazily-loaded registry, or
// nil when the cursor isn't inside a widget property block (or the widget
// isn't registered).
func scanEnclosingWidget(text string, cursorLine, cursorCol int, s *mdlServer) *executor.WidgetDefinition {
	lines := strings.Split(text, "\n")
	if cursorLine >= len(lines) {
		return nil
	}

	// Walk backwards, counting parens. The cursor must sit inside an
	// unmatched (.
	skip := 0
	openLine := -1
	for i := cursorLine; i >= 0; i-- {
		line := lines[i]
		end := len(line)
		if i == cursorLine {
			end = cursorCol
			if end > len(line) {
				end = len(line)
			}
		}
		for j := end - 1; j >= 0; j-- {
			c := line[j]
			switch c {
			case ')':
				skip++
			case '(':
				if skip > 0 {
					skip--
				} else {
					openLine = i
				}
			}
			if openLine != -1 {
				break
			}
		}
		if openLine != -1 {
			break
		}
		// Don't scan back too far — widget statements are typically a few
		// lines above. 200-line cap avoids pathological scans.
		if cursorLine-i > 200 {
			return nil
		}
	}
	if openLine == -1 {
		return nil
	}

	// Read the widget-statement header — joining the opening line and any
	// continuation up to the `(`. The header looks like one of:
	//   pluggablewidget 'com.example.widget.X' name
	//   accordion name
	//   gallery name (... unrelated property
	// We only inspect from the start of openLine; multi-line widget headers
	// are uncommon.
	header := lines[openLine]
	parenIdx := strings.LastIndex(header[:min(len(header), 4000)], "(")
	if parenIdx < 0 {
		return nil
	}
	header = strings.TrimSpace(header[:parenIdx])
	if header == "" {
		return nil
	}

	// Lazy-load the registry (re-use the widget-completions cache).
	s.ensureWidgetRegistry()
	if s.widgetRegistry == nil {
		return nil
	}

	// Two header shapes: (1) pluggablewidget '<id>' name, (2) <keyword> name
	tokens := strings.Fields(header)
	if len(tokens) == 0 {
		return nil
	}
	first := strings.ToLower(tokens[0])
	if first == "pluggablewidget" || first == "customwidget" {
		// Find the quoted widget ID
		quoted := ""
		for _, t := range tokens[1:] {
			if strings.HasPrefix(t, "'") || strings.HasPrefix(t, "\"") {
				quoted = strings.Trim(t, "'\"")
				break
			}
		}
		if quoted == "" {
			return nil
		}
		if def, ok := s.widgetRegistry.GetByWidgetID(quoted); ok {
			return def
		}
		return nil
	}
	// Keyword form: first token is the MDL name (e.g. ACCORDION)
	if def, ok := s.widgetRegistry.Get(strings.ToUpper(first)); ok {
		return def
	}
	return nil
}

// trailingIdent returns the trailing run of identifier characters on a line.
// Used to extract the in-progress word for prefix-filtering completions.
func trailingIdent(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			continue
		}
		return s[i+1:]
	}
	return s
}
