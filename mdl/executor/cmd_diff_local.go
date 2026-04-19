// SPDX-License-Identifier: Apache-2.0

// Package executor - Git-based local diff functions for MPR v2 format
package executor

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ============================================================================
// Local Git Diff Functions
// ============================================================================

// diffLocal compares local changes in mxunit files against a git reference.
// This only works with MPR v2 format (Mendix 10.18+) which stores units in mprcontents/.
//
// The ref parameter can be:
//   - A single ref (e.g., "HEAD", "main") — compares working tree vs ref
//   - A range "base..target" — compares two revisions (no working tree)
func diffLocal(ctx *ExecContext, ref string, opts DiffOptions) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Check MPR version
	if ctx.Backend.Version() != 2 {
		return mdlerrors.NewUnsupported("diff-local only supports MPR v2 format (Mendix 10.18+)")
	}

	contentsDir := ctx.Backend.ContentsDir()
	if contentsDir == "" {
		return mdlerrors.NewValidation("mprcontents directory not found")
	}

	// Set defaults
	if opts.Format == "" {
		opts.Format = DiffFormatUnified
	}
	if opts.Width == 0 {
		opts.Width = 120
	}

	// Find changed mxunit files using git
	changedFiles, err := findChangedMxunitFiles(ctx, contentsDir, ref)
	if err != nil {
		return mdlerrors.NewBackend("find changed files", err)
	}

	if len(changedFiles) == 0 {
		fmt.Fprintln(ctx.Output, "No local changes found in mxunit files.")
		return nil
	}

	var results []DiffResult
	var newCount, modifiedCount, deletedCount int

	for _, change := range changedFiles {
		result, err := diffMxunitFile(ctx, change, contentsDir, ref)
		if err != nil {
			// Log error but continue with other files
			fmt.Fprintf(ctx.Output, "Warning: %v\n", err)
			continue
		}
		if result != nil {
			results = append(results, *result)
			if result.IsNew {
				newCount++
			} else if result.IsDeleted {
				deletedCount++
			} else {
				modifiedCount++
			}
		}
	}

	// Output results
	for _, result := range results {
		switch opts.Format {
		case DiffFormatUnified:
			outputUnifiedDiff(ctx, result, opts.UseColor)
		case DiffFormatSideBySide:
			outputSideBySideDiff(ctx, result, opts.Width, opts.UseColor)
		case DiffFormatStructural:
			outputStructuralDiff(ctx, result, opts.UseColor)
		}
	}

	// Output summary
	fmt.Fprintf(ctx.Output, "\nSummary: %d new, %d modified, %d deleted\n",
		newCount, modifiedCount, deletedCount)

	return nil
}

// DiffLocal is a method wrapper for external callers.
func (e *Executor) DiffLocal(ref string, opts DiffOptions) error {
	return diffLocal(e.newExecContext(context.Background()), ref, opts)
}

// gitChange represents a file change from git
type gitChange struct {
	Status   string // "A" (added), "M" (modified), "D" (deleted)
	FilePath string // relative path from repo root
}

// findChangedMxunitFiles uses git to find changed mxunit files
func findChangedMxunitFiles(_ *ExecContext, contentsDir, ref string) ([]gitChange, error) {
	// Run git diff to find changed files in mprcontents
	cmd := execCommand("git", "diff", "--name-status", ref, "--", contentsDir)
	output, err := cmd.Output()
	if err != nil {
		return nil, mdlerrors.NewBackend("git diff", err)
	}

	var changes []gitChange
	lines := strings.SplitSeq(string(output), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		filePath := parts[1]

		// Only include .mxunit files
		if !strings.HasSuffix(filePath, ".mxunit") {
			continue
		}

		changes = append(changes, gitChange{
			Status:   status,
			FilePath: filePath,
		})
	}

	return changes, nil
}

// diffMxunitFile generates a diff for a single mxunit file.
// For two-revision diffs (ref contains ".."), both sides are read from git.
// For single-ref diffs, the "current" side is read from the working tree.
func diffMxunitFile(ctx *ExecContext, change gitChange, contentsDir, ref string) (*DiffResult, error) {
	var currentContent, gitContent []byte
	var err error

	// Determine if this is a two-revision diff
	baseRef, targetRef, isTwoRevision := parseRefRange(ref)

	if isTwoRevision {
		// Two-revision mode: both sides from git
		if change.Status != "D" {
			cmd := execCommand("git", "show", targetRef+":"+change.FilePath)
			currentContent, err = cmd.Output()
			if err != nil {
				return nil, mdlerrors.NewBackend(fmt.Sprintf("read %s version of %s", targetRef, change.FilePath), err)
			}
		}
		if change.Status != "A" {
			cmd := execCommand("git", "show", baseRef+":"+change.FilePath)
			gitContent, err = cmd.Output()
			if err != nil {
				return nil, mdlerrors.NewBackend(fmt.Sprintf("read %s version of %s", baseRef, change.FilePath), err)
			}
		}
	} else {
		// Single-ref mode: current from working tree, old from git
		if change.Status != "D" {
			currentContent, err = readFile(change.FilePath)
			if err != nil {
				return nil, mdlerrors.NewBackend(fmt.Sprintf("read current file %s", change.FilePath), err)
			}
		}
		if change.Status != "A" {
			cmd := execCommand("git", "show", ref+":"+change.FilePath)
			gitContent, err = cmd.Output()
			if err != nil {
				return nil, mdlerrors.NewBackend(fmt.Sprintf("read git version of %s", change.FilePath), err)
			}
		}
	}

	// Determine unit type and generate MDL
	var result DiffResult
	result.IsNew = change.Status == "A"
	result.IsDeleted = change.Status == "D"

	// Get type from content
	var unitType string
	if len(currentContent) > 0 {
		unitType = getTypeFromBSON(currentContent)
	} else if len(gitContent) > 0 {
		unitType = getTypeFromBSON(gitContent)
	}

	// Set object type for display
	result.ObjectType = simplifyTypeName(unitType)

	// Extract UUID from file path
	unitID := extractUUIDFromPath(change.FilePath)
	result.ObjectName = ast.QualifiedName{Name: unitID}

	// Generate MDL for both versions based on type
	if len(currentContent) > 0 {
		result.Proposed = bsonToMDL(ctx, unitType, unitID, currentContent)
	}
	if len(gitContent) > 0 {
		result.Current = bsonToMDL(ctx, unitType, unitID, gitContent)
	}

	// Generate structural changes
	if !result.IsNew && !result.IsDeleted && result.Current != result.Proposed {
		result.Changes = compareGeneric(ctx, result.Current, result.Proposed)
	}

	return &result, nil
}

// getTypeFromBSON extracts the $Type field from BSON content
func getTypeFromBSON(content []byte) string {
	var raw map[string]any
	if err := bson.Unmarshal(content, &raw); err != nil {
		return "Unknown"
	}
	if typeName, ok := raw["$Type"].(string); ok {
		return typeName
	}
	return "Unknown"
}

// simplifyTypeName converts a full type name like "DomainModels$Entity" to "Entity"
func simplifyTypeName(fullType string) string {
	parts := strings.Split(fullType, "$")
	if len(parts) > 1 {
		return parts[1]
	}
	return fullType
}

// extractUUIDFromPath extracts the UUID from a path like "mprcontents/ab/cd/abcd1234-...mxunit"
func extractUUIDFromPath(path string) string {
	base := strings.TrimSuffix(path, ".mxunit")
	parts := strings.Split(base, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// bsonToMDL converts BSON content to MDL representation based on type
func bsonToMDL(ctx *ExecContext, unitType, unitID string, content []byte) string {
	var raw map[string]any
	if err := bson.Unmarshal(content, &raw); err != nil {
		return fmt.Sprintf("-- Error parsing BSON: %v", err)
	}

	// Get name from the unit
	name := extractString(raw["Name"])
	if name == "" {
		name = unitID
	}

	// Try to get qualified name by finding module
	qualifiedName := name
	if containerID := extractBsonID(raw["$Container"]); containerID != "" {
		// Try to resolve module name from container
		if h, err := getHierarchy(ctx); err == nil {
			if modName := h.GetModuleName(model.ID(containerID)); modName != "" {
				qualifiedName = modName + "." + name
			} else if modName := h.GetModuleName(h.FindModuleID(model.ID(containerID))); modName != "" {
				qualifiedName = modName + "." + name
			}
		}
	}

	switch {
	case strings.Contains(unitType, "DomainModel"):
		return domainModelBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Entity"):
		return entityBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Microflow"):
		return microflowBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Nanoflow"):
		return nanoflowBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Enumeration"):
		return enumerationBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Page"):
		return pageBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Snippet"):
		return snippetBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Layout"):
		return layoutBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Module"):
		return moduleBsonToMDL(ctx, raw)
	default:
		// Generic representation
		return fmt.Sprintf("-- %s: %s\n-- Type: %s", simplifyTypeName(unitType), qualifiedName, unitType)
	}
}

// domainModelBsonToMDL converts a domain model BSON to MDL.
// Includes full entity definitions (attributes) so diffs show schema changes.
func domainModelBsonToMDL(ctx *ExecContext, raw map[string]any, name string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("-- Domain Model: %s", name))
	lines = append(lines, "")

	// Render full entity definitions so attribute changes show up in diffs
	entities := extractBsonArray(raw["Entities"])
	for _, ent := range entities {
		if entMap, ok := ent.(map[string]any); ok {
			entName := extractString(entMap["Name"])
			if entName == "" {
				continue
			}
			qn := name + "." + entName
			lines = append(lines, entityBsonToMDL(ctx, entMap, qn))
			lines = append(lines, "")
		}
	}

	// Render associations with their from/to references
	associations := extractBsonArray(raw["Associations"])
	for _, assoc := range associations {
		if assocMap, ok := assoc.(map[string]any); ok {
			assocName := extractString(assocMap["Name"])
			if assocName == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("-- Association: %s.%s", name, assocName))
			if assocType := extractString(assocMap["Type"]); assocType != "" {
				lines = append(lines, "--   Type: "+assocType)
			}
			if owner := extractString(assocMap["Owner"]); owner != "" {
				lines = append(lines, "--   Owner: "+owner)
			}
			if delBeh := extractString(assocMap["DeleteBehavior"]); delBeh != "" {
				lines = append(lines, "--   DeleteBehavior: "+delBeh)
			}
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}

// entityBsonToMDL converts an entity BSON to MDL
func entityBsonToMDL(ctx *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string

	// Documentation
	if doc := extractString(raw["Documentation"]); doc != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+doc)
		lines = append(lines, " */")
	}

	// Determine entity type
	entityType := "PERSISTENT"
	if persistable, ok := raw["Persistable"].(bool); ok && !persistable {
		entityType = "NON-PERSISTENT"
	}

	lines = append(lines, fmt.Sprintf("CREATE %s ENTITY %s (", entityType, qualifiedName))

	// Attributes
	attributes := extractBsonArray(raw["Attributes"])
	for i, attr := range attributes {
		if attrMap, ok := attr.(map[string]any); ok {
			attrLine := attributeBsonToMDL(ctx, attrMap)
			comma := ","
			if i == len(attributes)-1 {
				comma = ""
			}
			lines = append(lines, "  "+attrLine+comma)
		}
	}

	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// attributeBsonToMDL converts an attribute BSON to MDL line
func attributeBsonToMDL(_ *ExecContext, raw map[string]any) string {
	name := extractString(raw["Name"])
	typeStr := "Unknown"

	// Get type from $Type field - most common patterns
	if attrType := extractString(raw["$Type"]); attrType != "" {
		switch {
		case strings.Contains(attrType, "StringAttributeType"):
			length := extractInt(raw["Length"])
			if length > 0 {
				typeStr = fmt.Sprintf("String(%d)", length)
			} else {
				typeStr = "String"
			}
		case strings.Contains(attrType, "IntegerAttributeType"):
			typeStr = "Integer"
		case strings.Contains(attrType, "LongAttributeType"):
			typeStr = "Long"
		case strings.Contains(attrType, "DecimalAttributeType"):
			typeStr = "Decimal"
		case strings.Contains(attrType, "BooleanAttributeType"):
			typeStr = "Boolean"
		case strings.Contains(attrType, "DateTimeAttributeType"):
			localize, ok := raw["LocalizeDate"].(bool)
			if !ok || localize {
				typeStr = "DateTime"
			} else {
				typeStr = "Date"
			}
		case strings.Contains(attrType, "AutoNumberAttributeType"):
			typeStr = "AutoNumber"
		case strings.Contains(attrType, "BinaryAttributeType"):
			typeStr = "Binary"
		case strings.Contains(attrType, "EnumerationAttributeType"):
			typeStr = "Enumeration"
		}
	}

	// Check for type object which contains the actual type
	if typeObj, ok := raw["Type"].(map[string]any); ok {
		if typeType := extractString(typeObj["$Type"]); typeType != "" {
			switch {
			case strings.Contains(typeType, "StringAttributeType"):
				length := extractInt(typeObj["Length"])
				if length > 0 {
					typeStr = fmt.Sprintf("String(%d)", length)
				} else {
					typeStr = "String"
				}
			case strings.Contains(typeType, "IntegerAttributeType"):
				typeStr = "Integer"
			case strings.Contains(typeType, "LongAttributeType"):
				typeStr = "Long"
			case strings.Contains(typeType, "DecimalAttributeType"):
				typeStr = "Decimal"
			case strings.Contains(typeType, "BooleanAttributeType"):
				typeStr = "Boolean"
			case strings.Contains(typeType, "DateTimeAttributeType"):
				localize, ok := typeObj["LocalizeDate"].(bool)
				if !ok || localize {
					typeStr = "DateTime"
				} else {
					typeStr = "Date"
				}
			case strings.Contains(typeType, "AutoNumberAttributeType"):
				typeStr = "AutoNumber"
			case strings.Contains(typeType, "BinaryAttributeType"):
				typeStr = "Binary"
			case strings.Contains(typeType, "EnumerationAttributeType"):
				typeStr = "Enumeration"
			}
		}
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("%s: %s", name, typeStr))

	// Add NOT NULL constraint if required
	if rules := extractBsonArray(raw["ValidationRules"]); len(rules) > 0 {
		for _, rule := range rules {
			if ruleMap, ok := rule.(map[string]any); ok {
				if ruleType := extractString(ruleMap["$Type"]); strings.Contains(ruleType, "RequiredRule") {
					result.WriteString(" NOT NULL")
					break
				}
			}
		}
	}

	return result.String()
}

// microflowBsonToMDL converts a microflow BSON to MDL using the same
// renderer as DESCRIBE MICROFLOW, so diffs include activity bodies.
// Falls back to a header-only stub if parsing fails.
func microflowBsonToMDL(ctx *ExecContext, raw map[string]any, qualifiedName string) string {
	qn := splitQualifiedName(qualifiedName)
	mf := ctx.Backend.ParseMicroflowFromRaw(raw, model.ID(qn.Name), "")

	entityNames, microflowNames := buildNameLookups(ctx)
	return renderMicroflowMDL(ctx, mf, qn, entityNames, microflowNames, nil)
}

// splitQualifiedName parses "Module.Name" into an ast.QualifiedName.
// If no module prefix is present, Module is empty.
func splitQualifiedName(qualifiedName string) ast.QualifiedName {
	if idx := strings.LastIndex(qualifiedName, "."); idx > 0 {
		return ast.QualifiedName{Module: qualifiedName[:idx], Name: qualifiedName[idx+1:]}
	}
	return ast.QualifiedName{Name: qualifiedName}
}

// buildNameLookups builds ID → qualified-name maps for entities and
// microflows from the current project. Used by BSON-driven renderers that
// receive IDs (e.g. entity references) and want to resolve them against
// the working-tree model. Returns empty maps if the reader is unavailable.
func buildNameLookups(ctx *ExecContext) (map[model.ID]string, map[model.ID]string) {
	entityNames := make(map[model.ID]string)
	microflowNames := make(map[model.ID]string)
	if !ctx.Connected() {
		return entityNames, microflowNames
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return entityNames, microflowNames
	}
	if domainModels, err := ctx.Backend.ListDomainModels(); err == nil {
		for _, dm := range domainModels {
			modName := h.GetModuleName(dm.ContainerID)
			for _, entity := range dm.Entities {
				entityNames[entity.ID] = modName + "." + entity.Name
			}
		}
	}
	if microflows, err := ctx.Backend.ListMicroflows(); err == nil {
		for _, mf := range microflows {
			microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
		}
	}
	return entityNames, microflowNames
}

// nanoflowBsonToMDL converts a nanoflow BSON to MDL
func nanoflowBsonToMDL(_ *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string

	if doc := extractString(raw["Documentation"]); doc != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+doc)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("CREATE NANOFLOW %s ()", qualifiedName))

	returnType := "Void"
	if rt := extractString(raw["ReturnType"]); rt != "" {
		returnType = rt
	}
	lines = append(lines, fmt.Sprintf("RETURNS %s", returnType))

	lines = append(lines, "BEGIN")
	lines = append(lines, "  -- (nanoflow body)")
	lines = append(lines, "END;")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// enumerationBsonToMDL converts an enumeration BSON to MDL
func enumerationBsonToMDL(_ *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string

	if doc := extractString(raw["Documentation"]); doc != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+doc)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("CREATE ENUMERATION %s (", qualifiedName))

	values := extractBsonArray(raw["Values"])
	for i, val := range values {
		if valMap, ok := val.(map[string]any); ok {
			name := extractString(valMap["Name"])
			caption := extractString(valMap["Caption"])
			if caption == "" {
				caption = name
			}
			comma := ","
			if i == len(values)-1 {
				comma = ""
			}
			lines = append(lines, fmt.Sprintf("  %s '%s'%s", name, caption, comma))
		}
	}

	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// pageBsonToMDL converts a page BSON to MDL
func pageBsonToMDL(_ *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string

	if doc := extractString(raw["Documentation"]); doc != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+doc)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("CREATE PAGE %s ()", qualifiedName))

	if title := extractString(raw["Title"]); title != "" {
		lines = append(lines, fmt.Sprintf("TITLE '%s'", title))
	}

	// Layout reference
	if layoutCall, ok := raw["LayoutCall"].(map[string]any); ok {
		if layout := extractString(layoutCall["Layout"]); layout != "" {
			lines = append(lines, fmt.Sprintf("LAYOUT %s", layout))
		}
	}

	lines = append(lines, "BEGIN")
	lines = append(lines, "  -- (page widgets)")
	lines = append(lines, "END;")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// snippetBsonToMDL converts a snippet BSON to MDL
func snippetBsonToMDL(_ *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string

	if doc := extractString(raw["Documentation"]); doc != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+doc)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("CREATE SNIPPET %s ()", qualifiedName))
	lines = append(lines, "BEGIN")
	lines = append(lines, "  -- (snippet widgets)")
	lines = append(lines, "END;")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// layoutBsonToMDL converts a layout BSON to MDL
func layoutBsonToMDL(_ *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string

	if doc := extractString(raw["Documentation"]); doc != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+doc)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("CREATE LAYOUT %s ()", qualifiedName))
	lines = append(lines, "BEGIN")
	lines = append(lines, "  -- (layout structure)")
	lines = append(lines, "END;")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// moduleBsonToMDL converts a module BSON to MDL
func moduleBsonToMDL(_ *ExecContext, raw map[string]any) string {
	name := extractString(raw["Name"])
	var lines []string

	if doc := extractString(raw["Documentation"]); doc != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+doc)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("CREATE MODULE %s;", name))
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// compareGeneric provides a generic comparison for MDL content
func compareGeneric(_ *ExecContext, current, proposed string) []StructuralChange {
	var changes []StructuralChange

	currentLines := strings.Split(current, "\n")
	proposedLines := strings.Split(proposed, "\n")

	added := 0
	removed := 0

	// Simple line count comparison
	for i, line := range proposedLines {
		if i >= len(currentLines) {
			added++
		} else if currentLines[i] != line {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeModified,
				ElementType: "Line",
				ElementName: fmt.Sprintf("%d", i+1),
				Details:     "changed",
			})
		}
	}

	if len(proposedLines) > len(currentLines) {
		added = len(proposedLines) - len(currentLines)
	} else if len(currentLines) > len(proposedLines) {
		removed = len(currentLines) - len(proposedLines)
	}

	if added > 0 {
		changes = append(changes, StructuralChange{
			ChangeType:  ChangeAdded,
			ElementType: "Lines",
			ElementName: "",
			Details:     fmt.Sprintf("%d lines added", added),
		})
	}
	if removed > 0 {
		changes = append(changes, StructuralChange{
			ChangeType:  ChangeRemoved,
			ElementType: "Lines",
			ElementName: "",
			Details:     fmt.Sprintf("%d lines removed", removed),
		})
	}

	return changes
}

// ============================================================================
// Helper Functions for Git and BSON Parsing
// ============================================================================

// parseRefRange splits a ref like "base..target" into its parts.
// Returns (base, target, true) for ranges, or ("", ref, false) for single refs.
func parseRefRange(ref string) (base, target string, isRange bool) {
	if idx := strings.Index(ref, ".."); idx >= 0 {
		return ref[:idx], ref[idx+2:], true
	}
	return "", ref, false
}

// execCommand creates an exec.Cmd for running git commands
func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// readFile reads a file from disk
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// extractString extracts a string from various BSON representations
func extractString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// extractInt extracts an integer from various BSON number types
func extractInt(v any) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int32:
		return int(val)
	case int64:
		return int(val)
	case int:
		return val
	case float64:
		return int(val)
	}
	return 0
}

// extractBsonID extracts an ID string from various BSON ID representations
func extractBsonID(v any) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return blobToUUID(val)
	case primitive.Binary:
		return blobToUUID(val.Data)
	case map[string]any:
		// Binary UUID stored as {Subtype: 0, Data: "base64..."}
		if data, ok := val["Data"].(string); ok {
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err == nil {
				return blobToUUID(decoded)
			}
		}
		// Also try $ID field
		if id, ok := val["$ID"]; ok {
			return extractBsonID(id)
		}
	}

	return ""
}

// extractBsonArray extracts items from a Mendix BSON array
func extractBsonArray(v any) []any {
	if v == nil {
		return nil
	}

	arr, ok := v.(primitive.A)
	if !ok {
		// Try regular slice
		if slice, ok := v.([]any); ok {
			// Check if first element is the array type indicator
			if len(slice) > 0 {
				if typeIndicator, ok := slice[0].(int32); ok && typeIndicator == 3 {
					// Skip the type indicator
					return slice[1:]
				}
			}
			return slice
		}
		return nil
	}

	// Check if first element is array type indicator
	if len(arr) > 0 {
		if typeIndicator, ok := arr[0].(int32); ok && typeIndicator == 3 {
			result := make([]any, len(arr)-1)
			for i := 1; i < len(arr); i++ {
				result[i-1] = arr[i]
			}
			return result
		}
	}

	return arr
}

// blobToUUID converts a 16-byte blob to a UUID string using Microsoft GUID format
func blobToUUID(blob []byte) string {
	if len(blob) != 16 {
		return fmt.Sprintf("%x", blob)
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		blob[3], blob[2], blob[1], blob[0],
		blob[5], blob[4],
		blob[7], blob[6],
		blob[8], blob[9],
		blob[10], blob[11], blob[12], blob[13], blob[14], blob[15])
}
