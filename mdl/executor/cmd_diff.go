// SPDX-License-Identifier: Apache-2.0

// Package executor - Diff command implementation for comparing MDL scripts against project state
package executor

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// DiffFormat represents the output format for diff results
type DiffFormat string

const (
	DiffFormatUnified    DiffFormat = "unified"
	DiffFormatSideBySide DiffFormat = "side"
	DiffFormatStructural DiffFormat = "struct"
)

// DiffOptions configures diff output
type DiffOptions struct {
	Format   DiffFormat
	UseColor bool
	Width    int
}

// ChangeType represents the type of structural change
type ChangeType string

const (
	ChangeAdded    ChangeType = "+"
	ChangeRemoved  ChangeType = "-"
	ChangeModified ChangeType = "~"
)

// StructuralChange represents a single structural change within an object
type StructuralChange struct {
	ChangeType  ChangeType
	ElementType string // "Attribute", "Parameter", "Value", etc.
	ElementName string
	Details     string
}

// DiffResult represents the diff for a single object
type DiffResult struct {
	ObjectType string
	ObjectName ast.QualifiedName
	Current    string // MDL from MPR (empty if new)
	Proposed   string // MDL from script
	IsNew      bool
	IsDeleted  bool
	Changes    []StructuralChange
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
)

// DiffProgram compares an MDL program against the current project state
func diffProgram(ctx *ExecContext, prog *ast.Program, opts DiffOptions) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Set defaults
	if opts.Format == "" {
		opts.Format = DiffFormatUnified
	}
	if opts.Width == 0 {
		opts.Width = 120
	}

	var results []DiffResult
	var newCount, modifiedCount, unchangedCount int

	// Track processed objects to avoid duplicates (script may have multiple statements for same object)
	processed := make(map[string]bool)

	// Process each statement
	for _, stmt := range prog.Statements {
		result, err := diffStatement(ctx, stmt)
		if err != nil {
			// Skip statements that can't be diffed (e.g., connection statements)
			continue
		}
		if result != nil {
			// Create unique key for deduplication
			key := result.ObjectType + ":" + result.ObjectName.String()
			if processed[key] {
				// Skip duplicate - already processed this object
				continue
			}
			processed[key] = true

			results = append(results, *result)
			if result.IsNew {
				newCount++
			} else if result.Current != result.Proposed {
				modifiedCount++
			} else {
				unchangedCount++
			}
		}
	}

	// Output results based on format
	for _, result := range results {
		if result.Current == result.Proposed && !result.IsNew {
			// Skip unchanged objects unless showing structural
			if opts.Format != DiffFormatStructural {
				continue
			}
		}

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
	fmt.Fprintf(ctx.Output, "\nSummary: %d new, %d modified, %d unchanged\n",
		newCount, modifiedCount, unchangedCount)

	return nil
}

// DiffProgram is a method wrapper for external callers.
func (e *Executor) DiffProgram(prog *ast.Program, opts DiffOptions) error {
	return diffProgram(e.newExecContext(context.Background()), prog, opts)
}

// diffStatement generates a diff result for a single statement
func diffStatement(ctx *ExecContext, stmt ast.Statement) (*DiffResult, error) {
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		return diffEntity(ctx, s)
	case *ast.CreateViewEntityStmt:
		return diffViewEntity(ctx, s)
	case *ast.CreateEnumerationStmt:
		return diffEnumeration(ctx, s)
	case *ast.CreateAssociationStmt:
		return diffAssociation(ctx, s)
	case *ast.CreateMicroflowStmt:
		return diffMicroflow(ctx, s)
	case *ast.CreateNanoflowStmt:
		return diffNanoflow(ctx, s)
	default:
		return nil, nil // Skip unsupported statements
	}
}

// diffEntity compares a CREATE ENTITY statement against the project
func diffEntity(ctx *ExecContext, s *ast.CreateEntityStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "Entity",
		ObjectName: s.Name,
		Proposed:   entityStmtToMDL(ctx, s),
	}

	// Try to find existing entity
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			// Found existing entity - get its MDL representation
			result.Current = entityToMDL(ctx, module.Name, entity, dm)
			result.Changes = compareEntities(ctx, result.Current, result.Proposed)
			return result, nil
		}
	}

	result.IsNew = true
	return result, nil
}

// diffViewEntity compares a CREATE VIEW ENTITY statement against the project
func diffViewEntity(ctx *ExecContext, s *ast.CreateViewEntityStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "View Entity",
		ObjectName: s.Name,
		Proposed:   viewEntityStmtToMDL(ctx, s),
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			result.Current = viewEntityFromProjectToMDL(ctx, module.Name, entity, dm)
			return result, nil
		}
	}

	result.IsNew = true
	return result, nil
}

// diffEnumeration compares a CREATE ENUMERATION statement against the project
func diffEnumeration(ctx *ExecContext, s *ast.CreateEnumerationStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "Enumeration",
		ObjectName: s.Name,
		Proposed:   enumerationStmtToMDL(ctx, s),
	}

	// Try to find existing enumeration
	existingEnum := findEnumeration(ctx, s.Name.Module, s.Name.Name)
	if existingEnum == nil {
		result.IsNew = true
		return result, nil
	}

	h, _ := getHierarchy(ctx)
	modName := h.GetModuleName(existingEnum.ContainerID)
	result.Current = enumerationToMDL(ctx, modName, existingEnum)
	result.Changes = compareEnumerations(ctx, result.Current, result.Proposed)

	return result, nil
}

// diffAssociation compares a CREATE ASSOCIATION statement against the project
func diffAssociation(ctx *ExecContext, s *ast.CreateAssociationStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "Association",
		ObjectName: s.Name,
		Proposed:   associationStmtToMDL(ctx, s),
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			result.Current = associationToMDL(ctx, module.Name, assoc, dm)
			return result, nil
		}
	}

	result.IsNew = true
	return result, nil
}

// diffMicroflow compares a CREATE MICROFLOW statement against the project
func diffMicroflow(ctx *ExecContext, s *ast.CreateMicroflowStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "Microflow",
		ObjectName: s.Name,
		Proposed:   microflowStmtToMDL(ctx, s),
	}

	// Try to find existing microflow
	h, err := getHierarchy(ctx)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && mf.Name == s.Name.Name {
			// Capture current MDL representation
			var buf bytes.Buffer
			oldOutput := ctx.Output
			ctx.Output = &buf
			describeMicroflow(ctx, s.Name)
			ctx.Output = oldOutput
			result.Current = strings.TrimSuffix(buf.String(), "\n")
			result.Changes = compareMicroflows(ctx, result.Current, result.Proposed)
			return result, nil
		}
	}

	result.IsNew = true
	return result, nil
}

// diffNanoflow compares a CREATE NANOFLOW statement against the project
func diffNanoflow(ctx *ExecContext, s *ast.CreateNanoflowStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "Nanoflow",
		ObjectName: s.Name,
		Proposed:   nanoflowStmtToMDL(ctx, s),
	}

	// Try to find existing nanoflow
	// Errors treated as "new" to match diffMicroflow and other diff* functions
	h, err := getHierarchy(ctx)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	nfs, err := ctx.Backend.ListNanoflows()
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	for _, nf := range nfs {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && nf.Name == s.Name.Name {
			// Capture current MDL representation
			var buf bytes.Buffer
			if err := func() error {
				oldOutput := ctx.Output
				ctx.Output = &buf
				defer func() { ctx.Output = oldOutput }()
				return describeNanoflow(ctx, s.Name)
			}(); err != nil {
				return nil, err
			}
			result.Current = strings.TrimSuffix(buf.String(), "\n")
			result.Changes = compareMicroflows(ctx, result.Current, result.Proposed)
			return result, nil
		}
	}

	result.IsNew = true
	return result, nil
}

// ============================================================================
// Structural Comparison Functions
// ============================================================================

// compareEntities extracts structural changes between two entity MDL representations
func compareEntities(ctx *ExecContext, current, proposed string) []StructuralChange {
	var changes []StructuralChange

	// Simple line-based comparison for now
	currentLines := strings.Split(current, "\n")
	proposedLines := strings.Split(proposed, "\n")

	// Extract attributes from both
	currentAttrs := extractAttributes(ctx, currentLines)
	proposedAttrs := extractAttributes(ctx, proposedLines)

	// Find added attributes
	for name, proposed := range proposedAttrs {
		if _, exists := currentAttrs[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeAdded,
				ElementType: "Attribute",
				ElementName: name,
				Details:     proposed,
			})
		}
	}

	// Find removed attributes
	for name := range currentAttrs {
		if _, exists := proposedAttrs[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeRemoved,
				ElementType: "Attribute",
				ElementName: name,
			})
		}
	}

	// Find modified attributes
	for name, proposed := range proposedAttrs {
		if current, exists := currentAttrs[name]; exists && current != proposed {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeModified,
				ElementType: "Attribute",
				ElementName: name,
				Details:     "changed",
			})
		}
	}

	return changes
}

// compareEnumerations extracts structural changes between two enumeration MDL representations
func compareEnumerations(ctx *ExecContext, current, proposed string) []StructuralChange {
	var changes []StructuralChange

	currentValues := extractEnumValues(ctx, strings.Split(current, "\n"))
	proposedValues := extractEnumValues(ctx, strings.Split(proposed, "\n"))

	for name := range proposedValues {
		if _, exists := currentValues[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeAdded,
				ElementType: "Value",
				ElementName: name,
			})
		}
	}

	for name := range currentValues {
		if _, exists := proposedValues[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeRemoved,
				ElementType: "Value",
				ElementName: name,
			})
		}
	}

	return changes
}

// compareMicroflows extracts structural changes between two microflow MDL representations
func compareMicroflows(ctx *ExecContext, current, proposed string) []StructuralChange {
	var changes []StructuralChange

	currentParams := extractParameters(ctx, strings.Split(current, "\n"))
	proposedParams := extractParameters(ctx, strings.Split(proposed, "\n"))

	for name := range proposedParams {
		if _, exists := currentParams[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeAdded,
				ElementType: "Parameter",
				ElementName: name,
			})
		}
	}

	for name := range currentParams {
		if _, exists := proposedParams[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeRemoved,
				ElementType: "Parameter",
				ElementName: name,
			})
		}
	}

	// Count body statements
	currentStmts := countBodyStatements(ctx, current)
	proposedStmts := countBodyStatements(ctx, proposed)
	if currentStmts != proposedStmts {
		diff := proposedStmts - currentStmts
		if diff > 0 {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeAdded,
				ElementType: "Body",
				ElementName: "statements",
				Details:     fmt.Sprintf("%d statements added", diff),
			})
		} else {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeRemoved,
				ElementType: "Body",
				ElementName: "statements",
				Details:     fmt.Sprintf("%d statements removed", -diff),
			})
		}
	}

	return changes
}

// extractAttributes extracts attribute definitions from MDL lines
func extractAttributes(_ *ExecContext, lines []string) map[string]string {
	attrs := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "create") && !strings.HasPrefix(line, "/**") && !strings.HasPrefix(line, "*") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				if !strings.HasPrefix(name, "$") && !strings.HasPrefix(name, "@") {
					attrs[name] = strings.TrimSuffix(strings.TrimSpace(parts[1]), ",")
				}
			}
		}
	}
	return attrs
}

// extractEnumValues extracts enumeration values from MDL lines
func extractEnumValues(_ *ExecContext, lines []string) map[string]bool {
	values := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "'") && !strings.HasPrefix(line, "create") {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				name := strings.TrimSuffix(parts[0], ",")
				if name != "" && !strings.HasPrefix(name, "/") && !strings.HasPrefix(name, "*") {
					values[name] = true
				}
			}
		}
	}
	return values
}

// extractParameters extracts parameter names from MDL lines
func extractParameters(_ *ExecContext, lines []string) map[string]bool {
	params := make(map[string]bool)
	inParams := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "create microflow") || strings.HasPrefix(line, "create nanoflow") ||
			strings.HasPrefix(line, "create or modify microflow") || strings.HasPrefix(line, "create or modify nanoflow") {
			inParams = true
			continue
		}
		if inParams {
			if strings.HasPrefix(line, ")") {
				inParams = false
				continue
			}
			if strings.HasPrefix(line, "$") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) >= 1 {
					name := strings.TrimPrefix(parts[0], "$")
					name = strings.TrimSuffix(name, ",")
					params[strings.TrimSpace(name)] = true
				}
			}
		}
	}
	return params
}

// countBodyStatements counts statements in a microflow body
func countBodyStatements(_ *ExecContext, mdl string) int {
	count := 0
	inBody := false
	for line := range strings.SplitSeq(mdl, "\n") {
		line = strings.TrimSpace(line)
		if line == "begin" {
			inBody = true
			continue
		}
		if line == "end;" {
			break
		}
		if inBody && line != "" && !strings.HasPrefix(line, "--") {
			count++
		}
	}
	return count
}
