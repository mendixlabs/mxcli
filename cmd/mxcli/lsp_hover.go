// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Hover handles textDocument/hover requests.
func (s *mdlServer) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	docURI := uri.URI(params.TextDocument.URI)
	s.mu.Lock()
	text := s.docs[docURI]
	s.mu.Unlock()

	if text == "" {
		return nil, nil
	}

	// First try: cursor is on a property key inside a pluggable widget's
	// (...) block. Surface the widget property's description, type, and
	// default from the .def.json.
	if h := s.widgetPropertyHover(text, params.Position); h != nil {
		return h, nil
	}

	name, startCol, endCol, ok := qualifiedNameAtPosition(text, params.Position.Line, params.Position.Character)
	if !ok {
		return nil, nil
	}

	// Check cache
	cacheKey := "describe:" + name
	description, cached := s.cache.Get(cacheKey)

	if !cached {
		description = s.describeElement(ctx, text, params.Position.Line, name)
		if description == "" {
			return nil, nil
		}
		s.cache.Set(cacheKey, description, 60*time.Second)
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: "```mdl\n" + description + "\n```",
		},
		Range: &protocol.Range{
			Start: protocol.Position{Line: params.Position.Line, Character: startCol},
			End:   protocol.Position{Line: params.Position.Line, Character: endCol},
		},
	}, nil
}

// Definition handles textDocument/definition requests.
func (s *mdlServer) Definition(ctx context.Context, params *protocol.DefinitionParams) ([]protocol.Location, error) {
	docURI := uri.URI(params.TextDocument.URI)
	s.mu.Lock()
	text := s.docs[docURI]
	s.mu.Unlock()

	if text == "" {
		return nil, nil
	}

	name, _, _, ok := qualifiedNameAtPosition(text, params.Position.Line, params.Position.Character)
	if !ok {
		return nil, nil
	}

	// Resolve the element type
	elemType := s.resolveElementType(ctx, text, params.Position.Line, name)
	if elemType == "" {
		return nil, nil
	}

	// Return a mendix-mdl:// URI that the VS Code extension's MdlContentProvider handles
	targetURI := fmt.Sprintf("mendix-mdl://describe/%s/%s", elemType, name)
	return []protocol.Location{{
		URI: protocol.DocumentURI(targetURI),
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 0},
		},
	}}, nil
}

// qualifiedNameRegexp matches Module.Name patterns.
var qualifiedNameRegexp = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*`)

// qualifiedNameAtPosition extracts a qualified name (Module.Name) at the given
// cursor position. Returns the name, start column, end column, and whether a
// match was found.
func qualifiedNameAtPosition(text string, line, col uint32) (name string, startCol, endCol uint32, ok bool) {
	lines := strings.Split(text, "\n")
	if int(line) >= len(lines) {
		return "", 0, 0, false
	}
	lineText := lines[line]

	matches := qualifiedNameRegexp.FindAllStringIndex(lineText, -1)
	for _, m := range matches {
		start := uint32(m[0])
		end := uint32(m[1])
		if col >= start && col <= end {
			return lineText[m[0]:m[1]], start, end, true
		}
	}
	return "", 0, 0, false
}

// inferElementType guesses the element type from context keywords on the given line.
func inferElementType(text string, line uint32) string {
	lines := strings.Split(text, "\n")
	if int(line) >= len(lines) {
		return ""
	}
	upper := strings.ToUpper(lines[line])

	switch {
	case strings.Contains(upper, "CALL MICROFLOW"):
		return "microflow"
	case strings.Contains(upper, "CALL NANOFLOW"):
		return "nanoflow"
	case strings.Contains(upper, "SHOW PAGE"):
		return "page"
	case strings.Contains(upper, "SNIPPETCALL"):
		return "snippet"
	case strings.Contains(upper, "ENTITY") || strings.Contains(upper, "RETRIEVE") || strings.Contains(upper, "CREATE "):
		// CREATE could be CREATE ENTITY or CREATE object — be cautious
		if strings.Contains(upper, "CREATE ENTITY") || strings.Contains(upper, "ALTER ENTITY") || strings.Contains(upper, "DROP ENTITY") {
			return "entity"
		}
		if strings.Contains(upper, "RETRIEVE") {
			return "entity"
		}
		return ""
	case strings.Contains(upper, "ASSOCIATION"):
		return "association"
	case strings.Contains(upper, "ENUMERATION"):
		return "enumeration"
	case strings.Contains(upper, "PAGE"):
		return "page"
	case strings.Contains(upper, "LAYOUT"):
		return "layout"
	}
	return ""
}

// describeElement calls mxcli describe to get the MDL description of an element.
// It tries the inferred type first, then falls back to trying multiple types.
func (s *mdlServer) describeElement(ctx context.Context, text string, line uint32, name string) string {
	// Try inferred type first
	elemType := inferElementType(text, line)
	if elemType != "" {
		result, err := s.runMxcli(ctx, "describe", elemType, name)
		if err == nil && result != "" && !strings.HasPrefix(result, "Error") && !strings.Contains(result, "not found") {
			// Cache the element type for go-to-definition
			s.cache.Set("elemtype:"+name, elemType, 120*time.Second)
			return result
		}
	}

	// Try common types in order (frequently referenced types first)
	for _, t := range []string{"entity", "microflow", "page", "enumeration", "nanoflow", "association", "snippet", "workflow", "constant", "layout", "importmapping", "exportmapping", "restclient", "jsonstructure", "agent", "aimodel", "knowledgebase", "consumedmcpservice", "datatransformer"} {
		if t == elemType {
			continue // Already tried
		}
		result, err := s.runMxcli(ctx, "describe", t, name)
		if err == nil && result != "" && !strings.HasPrefix(result, "Error") && !strings.Contains(result, "not found") {
			s.cache.Set("elemtype:"+name, t, 120*time.Second)
			return result
		}
	}
	return ""
}

// resolveElementType determines the element type for a qualified name.
func (s *mdlServer) resolveElementType(ctx context.Context, text string, line uint32, name string) string {
	// Check cache first
	if cached, ok := s.cache.Get("elemtype:" + name); ok {
		return cached
	}

	// Try inferred type
	elemType := inferElementType(text, line)
	if elemType != "" {
		result, err := s.runMxcli(ctx, "describe", elemType, name)
		if err == nil && result != "" && !strings.HasPrefix(result, "Error") && !strings.Contains(result, "not found") {
			s.cache.Set("elemtype:"+name, elemType, 120*time.Second)
			return elemType
		}
	}

	// Try common types (frequently referenced types first)
	for _, t := range []string{"entity", "microflow", "page", "enumeration", "nanoflow", "association", "snippet", "workflow", "constant", "layout", "importmapping", "exportmapping", "restclient", "jsonstructure", "agent", "aimodel", "knowledgebase", "consumedmcpservice", "datatransformer"} {
		if t == elemType {
			continue
		}
		result, err := s.runMxcli(ctx, "describe", t, name)
		if err == nil && result != "" && !strings.HasPrefix(result, "Error") && !strings.Contains(result, "not found") {
			s.cache.Set("elemtype:"+name, t, 120*time.Second)
			return t
		}
	}
	return ""
}

// widgetPropertyHover returns a hover description when the cursor is on a
// property key inside a pluggable widget's (...) block. Returns nil when
// the cursor is elsewhere or the property isn't recognized by the widget.
func (s *mdlServer) widgetPropertyHover(text string, pos protocol.Position) *protocol.Hover {
	lines := strings.Split(text, "\n")
	if int(pos.Line) >= len(lines) {
		return nil
	}
	line := lines[pos.Line]
	wordStart, wordEnd, ok := identifierRangeAt(line, int(pos.Character))
	if !ok {
		return nil
	}
	key := line[wordStart:wordEnd]
	if key == "" {
		return nil
	}

	def := scanEnclosingWidget(text, int(pos.Line), wordEnd, s)
	if def == nil {
		return nil
	}
	desc, found := findPropertyHoverContent(def, key)
	if !found {
		return nil
	}
	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: desc,
		},
		Range: &protocol.Range{
			Start: protocol.Position{Line: pos.Line, Character: uint32(wordStart)},
			End:   protocol.Position{Line: pos.Line, Character: uint32(wordEnd)},
		},
	}
}

// findPropertyHoverContent looks up a property/childSlot/objectList by key
// (case-insensitive) on a widget definition and renders a Markdown hover
// blurb. Returns the rendered text and whether the key was recognized.
func findPropertyHoverContent(def *executor.WidgetDefinition, key string) (string, bool) {
	lk := strings.ToLower(key)

	for _, m := range def.PropertyMappings {
		if strings.EqualFold(m.PropertyKey, lk) || strings.EqualFold(m.Source, lk) {
			return renderPropertyHover(def, m.PropertyKey, m.Operation, m.Default, m.Description, m.Value, ""), true
		}
	}
	for _, mode := range def.Modes {
		for _, m := range mode.PropertyMappings {
			if strings.EqualFold(m.PropertyKey, lk) || strings.EqualFold(m.Source, lk) {
				return renderPropertyHover(def, m.PropertyKey, m.Operation, m.Default, m.Description, m.Value, mode.Name), true
			}
		}
	}
	for _, slot := range def.ChildSlots {
		if strings.EqualFold(slot.PropertyKey, lk) {
			return fmt.Sprintf("**%s** _(child slot — `%s` block)_\n\nWidget: `%s`",
				slot.PropertyKey, slot.MDLContainer, def.MDLName), true
		}
	}
	for _, ol := range def.ObjectLists {
		if strings.EqualFold(ol.PropertyKey, lk) {
			return fmt.Sprintf("**%s** _(object list — `%s` blocks)_\n\nWidget: `%s` · items: %d properties, %d slots",
				ol.PropertyKey, ol.MDLContainer, def.MDLName, len(ol.ItemProperties), len(ol.ItemSlots)), true
		}
	}
	return "", false
}

func renderPropertyHover(def *executor.WidgetDefinition, propKey, operation, defaultVal, description, value, mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** _(property", propKey)
	if operation != "" {
		fmt.Fprintf(&b, " · %s", operation)
	}
	if mode != "" {
		fmt.Fprintf(&b, " · mode: %s", mode)
	}
	b.WriteString(")_\n\n")
	if description != "" {
		b.WriteString(description)
		b.WriteString("\n\n")
	}
	if defaultVal != "" {
		fmt.Fprintf(&b, "_Default:_ `%s`\n\n", defaultVal)
	} else if value != "" {
		fmt.Fprintf(&b, "_Value:_ `%s`\n\n", value)
	}
	fmt.Fprintf(&b, "Widget: `%s` (`%s`)", def.MDLName, def.WidgetID)
	return b.String()
}

// identifierRangeAt returns the start (inclusive) and end (exclusive) column
// indices of the identifier containing or immediately to the right of the
// given column. Falls back to false when no identifier is present.
func identifierRangeAt(line string, col int) (int, int, bool) {
	if col > len(line) {
		col = len(line)
	}
	start := col
	for start > 0 && isIdentChar(line[start-1]) {
		start--
	}
	end := col
	for end < len(line) && isIdentChar(line[end]) {
		end++
	}
	if start == end {
		return 0, 0, false
	}
	return start, end, true
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}
