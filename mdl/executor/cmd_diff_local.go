// SPDX-License-Identifier: Apache-2.0

// Package executor - Git-based local diff functions for MPR v2 format
package executor

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"go.mongodb.org/mongo-driver/v2/bson"
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

	// Every git command runs in the project's directory, with paths relative to
	// it: running them in the process's working directory failed outright from
	// outside the repository ("failed to git diff: exit status 1"), and from a
	// subdirectory of it read the working-tree files at repo-root-relative
	// paths that did not exist (mendixlabs/mxcli#1602).
	projectDir, err := filepath.Abs(filepath.Dir(contentsDir))
	if err != nil {
		return mdlerrors.NewBackend("resolve project directory", err)
	}

	// Find changed mxunit files using git
	changedFiles, err := findChangedMxunitFiles(ctx, projectDir, filepath.Base(contentsDir), ref)
	if err != nil {
		return mdlerrors.NewBackend("find changed files", err)
	}

	if len(changedFiles) == 0 {
		fmt.Fprintln(ctx.Output, "No local changes found in mxunit files.")
		return nil
	}

	spec, err := resolveRefSpec(projectDir, ref)
	if err != nil {
		return err
	}

	var results []DiffResult
	var newCount, modifiedCount, deletedCount int

	for _, change := range changedFiles {
		result, err := diffMxunitFile(ctx, change, projectDir, spec)
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

	// Output results. A unit whose rendering is the same at both revisions is
	// printed as one line saying what changed (Writes), in every format.
	WriteDiffResults(ctx.Output, results, opts)

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
	FilePath string // path relative to the project directory
}

// gitInProject runs git in the project directory.
func gitInProject(projectDir string, args ...string) *exec.Cmd {
	cmd := execCommand("git", args...)
	cmd.Dir = projectDir
	return cmd
}

// gitShowArg names a project-relative path at a revision. "./" makes git read
// the path relative to the working directory rather than to the repo root, so
// a project below the root resolves the same as one at it.
func gitShowArg(rev, path string) string {
	return rev + ":./" + path
}

// findChangedMxunitFiles uses git to find changed mxunit files under
// contentsName (the mprcontents folder) of projectDir.
func findChangedMxunitFiles(_ *ExecContext, projectDir, contentsName, ref string) ([]gitChange, error) {
	// --relative: paths relative to projectDir, the directory git runs in.
	// --no-renames: a unit's file name is its ID, so a file that moved is a
	// different unit — one deleted, one added. Rename detection reported it as
	// "R100 old new", read here as a change to the old path, which the new
	// revision does not have.
	cmd := gitInProject(projectDir, "diff", "--name-status", "--relative", "--no-renames", ref, "--", contentsName)
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

	// Against the working tree, a unit git has not been told about is still a
	// new document: `git diff <ref>` lists tracked files only, so a module
	// added without `git add` was missing from "new" (mendixlabs/mxcli#1709).
	// A range compares two commits and has no working tree to look at.
	if _, _, isRange, _ := parseRefRange(ref); !isRange {
		untracked, err := gitInProject(projectDir, "ls-files", "--others", "--exclude-standard", "--", contentsName).Output()
		if err != nil {
			return nil, mdlerrors.NewBackend("git ls-files", err)
		}
		for line := range strings.SplitSeq(string(untracked), "\n") {
			if filePath := strings.TrimSpace(line); strings.HasSuffix(filePath, ".mxunit") {
				changes = append(changes, gitChange{Status: "A", FilePath: filePath})
			}
		}
	}

	return changes, nil
}

// refSpec is a --ref resolved to the revisions diff-local reads its two sides
// at, and the labels it names them by.
type refSpec struct {
	twoRevision        bool   // both sides from git; otherwise the working tree against a revision
	base, target       string // revisions to read; target alone in single-ref mode
	fromLabel, toLabel string
}

// resolveRefSpec resolves ref once, before any unit is read. The old side of a
// three-dot range is the merge base, which is what `git diff a...b` lists the
// changes against; reading it at a would put the other branch's changes into
// every hunk.
func resolveRefSpec(projectDir, ref string) (refSpec, error) {
	base, target, isRange, mergeBase := parseRefRange(ref)
	if !isRange {
		return refSpec{target: target, fromLabel: target, toLabel: "working tree"}, nil
	}
	spec := refSpec{twoRevision: true, base: base, target: target, fromLabel: base, toLabel: target}
	if mergeBase {
		out, err := gitInProject(projectDir, "merge-base", base, target).Output()
		if err != nil {
			return refSpec{}, mdlerrors.NewBackend(fmt.Sprintf("find the merge base of %s and %s", base, target), err)
		}
		spec.base = strings.TrimSpace(string(out))
		spec.fromLabel = fmt.Sprintf("merge base of %s and %s", base, target)
	}
	return spec, nil
}

// diffMxunitFile generates a diff for a single mxunit file.
// For two-revision diffs (a range), both sides are read from git.
// For single-ref diffs, the "current" side is read from the working tree.
func diffMxunitFile(ctx *ExecContext, change gitChange, projectDir string, spec refSpec) (*DiffResult, error) {
	var currentContent, gitContent []byte
	var err error

	baseRef, targetRef := spec.base, spec.target
	if spec.twoRevision {
		// Two-revision mode: both sides from git
		if change.Status != "D" {
			cmd := gitInProject(projectDir, "show", gitShowArg(targetRef, change.FilePath))
			currentContent, err = cmd.Output()
			if err != nil {
				return nil, mdlerrors.NewBackend(fmt.Sprintf("read %s version of %s", targetRef, change.FilePath), err)
			}
		}
		if change.Status != "A" {
			cmd := gitInProject(projectDir, "show", gitShowArg(baseRef, change.FilePath))
			gitContent, err = cmd.Output()
			if err != nil {
				return nil, mdlerrors.NewBackend(fmt.Sprintf("read %s version of %s", baseRef, change.FilePath), err)
			}
		}
	} else {
		// Single-ref mode: current from working tree, old from git
		if change.Status != "D" {
			currentContent, err = readFile(filepath.Join(projectDir, change.FilePath))
			if err != nil {
				return nil, mdlerrors.NewBackend(fmt.Sprintf("read current file %s", change.FilePath), err)
			}
		}
		if change.Status != "A" {
			cmd := gitInProject(projectDir, "show", gitShowArg(targetRef, change.FilePath))
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

	// Name the sides after what they are: both are git revisions in range
	// mode, and the working tree against a ref otherwise. Neither is a script.
	result.FromLabel, result.ToLabel = spec.fromLabel, spec.toLabel

	// Extract UUID from file path; it is the unit ID.
	unitID := extractUUIDFromPath(change.FilePath)
	newRaw := unmarshalUnit(currentContent)
	oldRaw := unmarshalUnit(gitContent)
	nameSource := newRaw
	if nameSource == nil {
		nameSource = oldRaw
	}
	qualifiedName := unitQualifiedName(ctx, nameSource, unitID)
	result.ObjectName = splitQualifiedName(qualifiedName)

	// Values the rendering must not print are masked so that a change still
	// shows: each side needs to know the other's value.
	var oldMask, newMask constantValueFunc
	if unitType == projectSettingsType {
		oldMask, newMask = constantValueMasks(ctx, oldRaw, newRaw)
	}

	// Generate MDL for both versions based on type
	if len(currentContent) > 0 {
		result.Proposed = bsonToMDL(ctx, unitType, unitID, currentContent, unitQualifiedName(ctx, newRaw, unitID), newMask)
	}
	if len(gitContent) > 0 {
		result.Current = bsonToMDL(ctx, unitType, unitID, gitContent, unitQualifiedName(ctx, oldRaw, unitID), oldMask)
	}

	// Generate structural changes
	if !result.IsNew && !result.IsDeleted && result.Current != result.Proposed {
		result.Changes = compareGeneric(ctx, result.Current, result.Proposed)
	}

	// git says the unit changed, so identical renderings mean the change is in
	// what the rendering leaves out. Say so, rather than print nothing under a
	// summary that counts it (ako/mxcli#1512, ako/mxcli#1562).
	if !result.IsNew && !result.IsDeleted && result.Current == result.Proposed {
		result.Writes = unrenderedChangeNote(gitContent, currentContent)
	}

	return &result, nil
}

// getTypeFromBSON extracts the $Type field from BSON content
func getTypeFromBSON(content []byte) string {
	var raw map[string]any
	if err := bsoncompat.UnmarshalDocMap(content, &raw); err != nil {
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

// unmarshalUnit decodes a unit's BSON, nil when there is none or it is unreadable.
func unmarshalUnit(content []byte) map[string]any {
	if len(content) == 0 {
		return nil
	}
	var raw map[string]any
	if err := bsoncompat.UnmarshalDocMap(content, &raw); err != nil {
		return nil
	}
	return raw
}

// unitQualifiedName names a unit as Module.Name.
//
// An MPR v2 .mxunit does not carry its container: that is a column of the
// Unit table in the .mpr, so the module has to come from the project's
// hierarchy, keyed on the unit ID (the file name). Looking for a $Container
// key in the document never hit, which is why diff-local printed
// "Microflow.<uuid>" and rendered the MDL without its module (#1393).
func unitQualifiedName(ctx *ExecContext, raw map[string]any, unitID string) string {
	name := extractString(raw["Name"])
	h, _ := getHierarchy(ctx)
	if h == nil {
		if name == "" {
			return unitID
		}
		return name
	}
	if h.IsModule(model.ID(unitID)) {
		if name == "" {
			name = h.GetModuleName(model.ID(unitID))
		}
		if name == "" {
			return unitID
		}
		return name
	}
	containerID := h.ContainerOf(model.ID(unitID))
	if containerID == "" {
		// A unit absent from the opened project (deleted since): fall back to
		// whatever container reference the document itself holds.
		containerID = model.ID(extractBsonID(raw["$ContainerID"]))
		if containerID == "" {
			containerID = model.ID(extractBsonID(raw["$Container"]))
		}
	}
	modName := ""
	if containerID != "" {
		modName = h.GetModuleName(h.FindModuleID(containerID))
	}
	if name == "" {
		// A domain model is nameless: it is the module's.
		if modName != "" {
			return modName
		}
		return unitID
	}
	if modName == "" {
		return name
	}
	return modName + "." + name
}

// unrenderedChangeNote names what changed in a unit whose MDL rendering is the
// same at both revisions: the property paths that differ, never their values.
// Without it the unit was counted as modified and printed nothing — every
// format, every unit type the rendering is lossy for (settings, access rules,
// navigation, security, …).
func unrenderedChangeNote(before, after []byte) string {
	const shown = 4
	paths := ChangedPaths(before, after, shown+1)
	if len(paths) == 0 {
		return "rewritten with no property change (element $IDs, pointers or property order only)"
	}
	if len(paths) > shown {
		paths = append(paths[:shown], "…")
	}
	return "changed in properties the MDL rendering does not show: " + strings.Join(paths, ", ")
}

// projectSettingsType is the storage type of the project settings unit.
const projectSettingsType = "Settings$ProjectSettings"

// bsonToMDL converts BSON content to MDL representation based on type.
// constantValue renders a configuration constant's value in the settings unit
// (see constantValueMasks); other unit types ignore it.
func bsonToMDL(ctx *ExecContext, unitType, unitID string, content []byte, qualifiedName string, constantValue constantValueFunc) string {
	var raw map[string]any
	if err := bsoncompat.UnmarshalDocMap(content, &raw); err != nil {
		return fmt.Sprintf("-- Error parsing BSON: %v", err)
	}
	if qualifiedName == "" {
		qualifiedName = unitID
	}

	switch {
	// An exact match, first: the dispatch below is by containment, and
	// "Settings" is in "Projects$ModuleSettings" too.
	case unitType == projectSettingsType:
		return projectSettingsBsonToMDL(ctx, raw, constantValue)
	case strings.Contains(unitType, "DomainModel"):
		return domainModelBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Entity"):
		return entityBsonToMDL(ctx, raw, qualifiedName)
	// Nanoflow before Microflow: the storage type is "Microflows$Nanoflow",
	// which contains "Microflow", so the other order sent every nanoflow to
	// the microflow parser, which rejected it.
	case strings.Contains(unitType, "Nanoflow"):
		return nanoflowBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Microflow"):
		return microflowBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Enumeration"):
		return enumerationBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Page"):
		return pageBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Snippet"):
		return snippetBsonToMDL(ctx, raw, qualifiedName)
	case strings.Contains(unitType, "Layout"):
		return layoutBsonToMDL(ctx, raw, qualifiedName)
	// Exact, not Contains: Security$ModuleSecurity and Projects$ModuleSettings
	// contain "Module" too, and rendered as `create module ;` on both sides.
	case unitType == "Projects$ModuleImpl" || unitType == "Projects$Module":
		return moduleBsonToMDL(ctx, raw)
	default:
		// Generic representation
		return fmt.Sprintf("-- %s: %s\n-- Type: %s", simplifyTypeName(unitType), qualifiedName, unitType)
	}
}

// projectSettingsBsonToMDL renders the project settings unit with DESCRIBE
// SETTINGS' own writer. The unit had no case here, so it fell to the generic
// two-comment rendering, which is the same at every revision: a changed
// constant value or runtime setting counted as modified and printed nothing
// (mendixlabs/mxcli#1602, ako/mxcli#1512). Configuration constant values are
// masked — they often hold secrets, and diff output lands in reviews and CI
// logs — so constantValue is never left to print them.
func projectSettingsBsonToMDL(ctx *ExecContext, raw map[string]any, constantValue constantValueFunc) string {
	ps, err := ctx.Backend.ParseProjectSettingsFromRaw(raw)
	if err != nil {
		return "-- project settings could not be read: " + err.Error()
	}
	if constantValue == nil {
		constantValue = func(_, _, _ string) string { return maskedValue }
	}
	return strings.TrimRight(renderToString(ctx, func() {
		writeProjectSettingsMDL(ctx, ps, constantValue)
	}), "\n")
}

// maskedValue stands in for a configuration constant's value. It is not a
// string literal on purpose: pasted into a script, it fails to parse instead
// of setting the constant to the text "masked".
const maskedValue = "<masked>"

// constantValueMasks returns the value renderers for the old and the new
// revision of the settings unit. A value is printed as <masked> — except where
// the same constant in the same configuration holds a different value on the
// other side, where it is "<masked: old value>" / "<masked: new value>", so
// the change shows as a -/+ pair without either value appearing.
func constantValueMasks(ctx *ExecContext, oldRaw, newRaw map[string]any) (oldMask, newMask constantValueFunc) {
	oldVals, newVals := constantValues(ctx, oldRaw), constantValues(ctx, newRaw)
	mask := func(other map[string]string, label string) constantValueFunc {
		return func(configuration, constant, value string) string {
			if o, ok := other[configuration+"\x00"+constant]; ok && o != value {
				return "<masked: " + label + ">"
			}
			return maskedValue
		}
	}
	return mask(newVals, "old value"), mask(oldVals, "new value")
}

// constantValues maps configuration + "\x00" + constant to the shared value
// the settings document holds for it; private values have none.
func constantValues(ctx *ExecContext, raw map[string]any) map[string]string {
	vals := map[string]string{}
	if raw == nil {
		return vals
	}
	ps, err := ctx.Backend.ParseProjectSettingsFromRaw(raw)
	if err != nil || ps.Configuration == nil {
		return vals
	}
	for _, cfg := range ps.Configuration.Configurations {
		for _, cv := range cfg.ConstantValues {
			if !cv.IsPrivate {
				vals[cfg.Name+"\x00"+cv.ConstantId] = cv.Value
			}
		}
	}
	return vals
}

// domainModelBsonToMDL converts a domain model BSON to MDL.
// Includes full entity definitions (attributes) so diffs show schema changes,
// each followed by its access rules as DESCRIBE ENTITY prints them.
func domainModelBsonToMDL(ctx *ExecContext, raw map[string]any, name string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("-- Domain Model: %s", name))
	lines = append(lines, "")

	// The access rules are rendered by DESCRIBE ENTITY's own grant writer, so
	// they need the semantic entity, not the raw map the rest of this renderer
	// reads. Without them a GRANT or REVOKE changed the unit but nothing this
	// printed, so it diffed to nothing (mendixlabs/mxcli#1630, ako/mxcli#1562).
	accessByEntity := map[string]*domainmodel.Entity{}
	if dm, err := ctx.Backend.ParseDomainModelFromRaw(raw, "", ""); err != nil {
		lines = append(lines, "-- access rules not shown: "+err.Error(), "")
	} else {
		for _, e := range dm.Entities {
			accessByEntity[e.Name] = e
		}
	}

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
			if e := accessByEntity[entName]; e != nil {
				grants := renderToString(ctx, func() {
					outputEntityAccessGrants(ctx, e, name, entName)
				})
				if grants = strings.TrimRight(grants, "\n"); grants != "" {
					lines = append(lines, grants)
				}
			}
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
	entityType := "persistent"
	if persistable, ok := raw["Persistable"].(bool); ok && !persistable {
		entityType = "non-persistent"
	}

	lines = append(lines, fmt.Sprintf("create %s entity %s (", entityType, qualifiedName))

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

	return strings.Join(lines, "\n")
}

// attributeBsonToMDL converts an attribute BSON to MDL line
func attributeBsonToMDL(_ *ExecContext, raw map[string]any) string {
	name := extractString(raw["Name"])
	typeStr := "Unknown"

	// Get type from $Type field - most common patterns
	if attrType := extractString(raw["$Type"]); attrType != "" {
		switch {
		// Before String: "HashedStringAttributeType" contains "StringAttributeType".
		case strings.Contains(attrType, "HashedStringAttributeType"):
			typeStr = "HashedString"
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
				typeStr = "DateTime not localized" // #1373; `Date` is deprecated (MDL-DEPR160)
			}
		case strings.Contains(attrType, "AutoNumberAttributeType"):
			typeStr = "AutoNumber"
		case strings.Contains(attrType, "BinaryAttributeType"):
			typeStr = "Binary"
		case strings.Contains(attrType, "EnumerationAttributeType"):
			typeStr = "Enumeration"
		}
	}

	// Check for the type object, which carries the actual type. Mendix stores
	// it under the STORAGE name "NewType"; "Type" is the SDK name and is not
	// a key any real document has. Reading only "Type" left every attribute
	// rendering as "Unknown", so a String(200) -> String(50) change diffed to
	// nothing at all while the summary still counted the unit as modified.
	typeObj, ok := raw["NewType"].(map[string]any)
	if !ok {
		typeObj, ok = raw["Type"].(map[string]any)
	}
	if ok {
		if typeType := extractString(typeObj["$Type"]); typeType != "" {
			switch {
			// Before String: "HashedStringAttributeType" contains "StringAttributeType".
			case strings.Contains(typeType, "HashedStringAttributeType"):
				typeStr = "HashedString"
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
					typeStr = "DateTime not localized" // #1373; `Date` is deprecated (MDL-DEPR160)
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
					result.WriteString(" not null")
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
	if mf == nil {
		return fmt.Sprintf("microflow %s\n  -- parse failed --\nend microflow\n", qualifiedName)
	}

	entityNames, microflowNames := buildNameLookups(ctx)
	return renderMicroflowMDL(ctx, "microflow", mf, qn, entityNames, microflowNames, nil)
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
	if nanoflows, err := ctx.Backend.ListNanoflows(); err == nil {
		for _, nf := range nanoflows {
			microflowNames[nf.ID] = h.GetQualifiedName(nf.ContainerID, nf.Name)
		}
	}
	return entityNames, microflowNames
}

// nanoflowBsonToMDL converts a nanoflow BSON to MDL with the renderer
// DESCRIBE NANOFLOW uses, so diffs include activity bodies. It was a fixed
// "-- (nanoflow body)" stub, so any change inside a nanoflow counted as
// modified and diffed to nothing (#1393). Falls back to a header-only stub
// if parsing fails.
func nanoflowBsonToMDL(ctx *ExecContext, raw map[string]any, qualifiedName string) string {
	qn := splitQualifiedName(qualifiedName)
	nf := ctx.Backend.ParseNanoflowFromRaw(raw, model.ID(qn.Name), "")
	if nf == nil {
		return fmt.Sprintf("nanoflow %s\n  -- parse failed --\nend nanoflow\n", qualifiedName)
	}

	entityNames, microflowNames := buildNameLookups(ctx)
	return renderMicroflowMDL(ctx, "nanoflow", nanoflowAsMicroflow(nf), qn, entityNames, microflowNames, nil)
}

// enumerationBsonToMDL converts an enumeration BSON to MDL
func enumerationBsonToMDL(_ *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string

	if doc := extractString(raw["Documentation"]); doc != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+doc)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("create enumeration %s (", qualifiedName))

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

	return strings.Join(lines, "\n")
}

// pageBsonToMDL converts a page BSON to MDL, widgets included, using the
// renderer DESCRIBE PAGE uses. The body used to be a fixed
// "-- (page widgets)" placeholder, so a change to any widget rendered the
// same on both sides: "1 modified" and no hunk (#1393).
func pageBsonToMDL(ctx *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string
	lines = append(lines, docCommentLines(extractString(raw["Documentation"]))...)

	var props []string
	if title := extractTextsText(ctx, raw, "Title"); title != "" {
		props = append(props, fmt.Sprintf("Title: %s", mdlQuote(ctx, title)))
	}
	if formCall, ok := raw["FormCall"].(map[string]any); ok {
		layoutName := ""
		if layoutID := extractBinaryID(formCall["Layout"]); layoutID != "" {
			layoutName = resolveLayoutName(ctx, model.ID(layoutID))
		} else {
			layoutName = extractString(formCall["Form"])
		}
		if layoutName != "" {
			props = append(props, "Layout: "+layoutName)
		}
	}
	if url := extractString(raw["Url"]); url != "" {
		props = append(props, fmt.Sprintf("Url: %s", mdlQuote(ctx, url)))
	}
	header := "create or modify page " + qualifiedName
	if len(props) > 0 {
		header += " (" + strings.Join(props, ", ") + ")"
	}
	lines = append(lines, header+" {")

	var nonEmpty []pageWidgetGroup
	for _, g := range pageWidgetGroupsFromRawData(ctx, raw) {
		if len(g.Widgets) > 0 {
			nonEmpty = append(nonEmpty, g)
		}
	}
	body := renderToString(ctx, func() {
		switch {
		case len(nonEmpty) == 0:
		case len(nonEmpty) == 1 && nonEmpty[0].Placeholder == "Main":
			for _, w := range nonEmpty[0].Widgets {
				outputWidgetMDLV3(ctx, w, 1)
			}
		default:
			for _, g := range nonEmpty {
				fmt.Fprintf(ctx.Output, "  placeholder %s {\n", mdlIdent(g.Placeholder))
				for _, w := range g.Widgets {
					outputWidgetMDLV3(ctx, w, 2)
				}
				fmt.Fprint(ctx.Output, "  }\n")
			}
		}
	})
	if body = strings.TrimRight(body, "\n"); body != "" {
		lines = append(lines, body)
	}
	lines = append(lines, "};")

	return strings.Join(lines, "\n")
}

// snippetBsonToMDL converts a snippet BSON to MDL, widgets included.
func snippetBsonToMDL(ctx *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string
	lines = append(lines, docCommentLines(extractString(raw["Documentation"]))...)
	lines = append(lines, "create or modify snippet "+qualifiedName+" {")
	widgets := snippetWidgetsFromRawData(ctx, raw)
	body := renderToString(ctx, func() {
		for _, w := range widgets {
			outputWidgetMDLV3(ctx, w, 1)
		}
	})
	if body = strings.TrimRight(body, "\n"); body != "" {
		lines = append(lines, body)
	}
	lines = append(lines, "};")

	return strings.Join(lines, "\n")
}

// renderToString captures what a DESCRIBE renderer writes to ctx.Output.
func renderToString(ctx *ExecContext, render func()) string {
	var buf strings.Builder
	saved := ctx.Output
	ctx.Output = &buf
	defer func() { ctx.Output = saved }()
	render()
	return buf.String()
}

// docCommentLines renders documentation as a /** */ block, nil when empty.
func docCommentLines(doc string) []string {
	if doc == "" {
		return nil
	}
	lines := []string{"/**"}
	for line := range strings.SplitSeq(doc, "\n") {
		lines = append(lines, " * "+line)
	}
	return append(lines, " */")
}

// layoutBsonToMDL converts a layout BSON to MDL, widgets included, the way
// DESCRIBE LAYOUT prints it. The body was a fixed "-- (layout structure)"
// stub, so a change to a layout's widgets diffed to nothing (#1393).
func layoutBsonToMDL(ctx *ExecContext, raw map[string]any, qualifiedName string) string {
	var lines []string
	lines = append(lines, docCommentLines(extractString(raw["Documentation"]))...)

	// LayoutType lives on the Content wrapper, the class on Appearance — see
	// layout_write.go for the measured document shape.
	layoutType := "(not read)"
	if content, ok := raw["Content"].(map[string]any); ok {
		if lt := extractString(content["LayoutType"]); lt != "" {
			layoutType = lt
		}
	}
	class := ""
	if ap, ok := raw["Appearance"].(map[string]any); ok {
		class = extractString(ap["Class"])
	}
	header := layoutHeaderMDL(ctx, layoutType, class, layoutOuterName(raw))
	lines = append(lines, fmt.Sprintf("create or modify layout %s (\n%s\n) {", qualifiedName, header))

	body := renderToString(ctx, func() {
		outputLayoutBodyMDL(ctx, raw)
	})
	if body = strings.TrimRight(body, "\n"); body != "" {
		lines = append(lines, body)
	}
	lines = append(lines, "};")

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

	lines = append(lines, fmt.Sprintf("create module %s;", name))

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

// parseRefRange splits a ref like "base..target" or "base...target" into its
// parts; mergeBase reports the three-dot form, whose old side is the merge base
// of the two. An empty side is HEAD, as it is to git. Returns ("", ref, false,
// false) for a single ref.
//
// "..." is matched before "..": splitting "a...b" on the first ".." read the
// target as ".b", and every unit failed to load.
func parseRefRange(ref string) (base, target string, isRange, mergeBase bool) {
	sep := ".."
	if strings.Contains(ref, "...") {
		sep, mergeBase = "...", true
	}
	idx := strings.Index(ref, sep)
	if idx < 0 {
		return "", ref, false, false
	}
	base, target = ref[:idx], ref[idx+len(sep):]
	if base == "" {
		base = "HEAD"
	}
	if target == "" {
		target = "HEAD"
	}
	return base, target, true, mergeBase
}

// execCommand creates an exec.Cmd for running git commands.
// It is a package-level variable so tests can replace it with a stub that
// simulates git failures without actually invoking git.
var execCommand = func(name string, args ...string) *exec.Cmd {
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
	case bson.Binary:
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

	arr, ok := v.(bson.A)
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
