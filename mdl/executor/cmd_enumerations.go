// SPDX-License-Identifier: Apache-2.0

// Package executor - Enumeration commands (SHOW/DESCRIBE/CREATE/ALTER/DROP ENUMERATION)
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// refuseSystemEnumerationWrite rejects any write that names the System module.
//
// The System module's enumerations are platform built-ins that the backend
// SYNTHESIZES so they can be read (#1102) — there is no stored unit for the
// module, so there is nothing for a write to live in. Before this guard the four
// write verbs did not fail cleanly: CREATE ENUMERATION System.X reported
// "Created enumeration: System.X" and wrote a unit whose ContainerID was the
// synthetic module ID 00000000-…-0001, which is not a unit in the project — an
// orphan with a dangling parent, on disk, with no error. The others surfaced a
// raw .mxunit path instead.
//
// The signal is the module NAME, not the container: a brand-new enumeration has
// no container yet, and that is precisely the case that used to corrupt (the
// module lookup resolves "System" to the virtual module and hands its synthetic
// ID over as the parent). Unlike the Marketplace guard, which deliberately does
// not trust a name because a user may name a module Atlas_Core, "System" is
// reserved by the platform and cannot be a user module.
func refuseSystemEnumerationWrite(verb string, name ast.QualifiedName) error {
	if name.Module != "System" {
		return nil
	}
	// The hint must not name the statement's own enumeration: on a CREATE that
	// name usually does not exist, and telling the reader to describe it would
	// send them after nothing.
	return mdlerrors.NewValidation(fmt.Sprintf(
		"cannot %s %s: the System module is owned by the Mendix platform and is read-only — "+
			"its enumerations are built in, not stored in the project. "+
			"Define your own enumeration in one of your modules; "+
			"`show enumerations` lists the built-in ones and `describe enumeration System.<Name>` reports their values.",
		verb, name.String()))
}

// execCreateEnumeration handles CREATE ENUMERATION statements.
func execCreateEnumeration(ctx *ExecContext, s *ast.CreateEnumerationStmt) error {

	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	verb := "create enumeration"
	if s.CreateOrModify {
		verb = "create or modify enumeration"
	}
	if err := refuseSystemEnumerationWrite(verb, s.Name); err != nil {
		return err
	}

	// Validate enumeration values for reserved words
	if violations := ValidateEnumeration(s); len(violations) > 0 {
		var msgs []string
		for _, v := range violations {
			msgs = append(msgs, v.Message)
		}
		return mdlerrors.NewValidationf("invalid enumeration '%s':\n  - %s",
			s.Name.String(), strings.Join(msgs, "\n  - "))
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Check if enumeration already exists
	existingEnum := findEnumeration(ctx, s.Name.Module, s.Name.Name)
	if existingEnum != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExistsMsg("enumeration", s.Name.Module+"."+s.Name.Name, fmt.Sprintf("enumeration already exists: %s.%s (use create or modify to update)", s.Name.Module, s.Name.Name))
	}

	// Create enumeration values
	var values []model.EnumerationValue
	for _, v := range s.Values {
		values = append(values, model.EnumerationValue{
			Name: v.Name,
			Caption: &model.Text{
				Translations: map[string]string{authoringLanguage(ctx): v.Caption},
			},
		})
	}

	// Resolve the target container: a module folder if FOLDER was given, else
	// the module root. Mirrors CREATE PAGE (… Folder: '…') so DESCRIBE can
	// round-trip a foldered enumeration (Bug 12b).
	containerID := module.ID
	if s.Folder != "" {
		folderID, ferr := resolveFolder(ctx, module.ID, s.Folder)
		if ferr != nil {
			return mdlerrors.NewBackend("resolve folder "+s.Folder, ferr)
		}
		containerID = folderID
	}

	// Create or update enumeration
	enum := &model.Enumeration{
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		Values:        values,
		// "" — no EXPORT LEVEL clause — keeps the stored level on a rewrite
		// (UpdateEnumeration) and writes Hidden on create (#1498).
		ExportLevel: authoredExportLevel(s.ExportLevel),
	}

	if existingEnum != nil && s.CreateOrModify {
		// In-place update: preserve the existing UUID so BSON git-diff sees a
		// modification rather than a delete+insert pair.
		enum.ID = existingEnum.ID
		// Excluded is model state, not script state: MDL cannot express it for
		// an enumeration, so the stored value is the one that survives (#914).
		enum.Excluded = existingEnum.Excluded
		// So is an OData contract's source: a rebuild without it turns every
		// external entity attribute of this type into CE4597 (ako/mxcli#2477).
		carryRemoteEnumeration(enum, existingEnum)
		// Absent doc comment preserves, explicitly empty clears (#1018).
		if !s.DocumentationSet {
			enum.Documentation = existingEnum.Documentation
		}
		// Placement is model state too when the statement is silent about it.
		if s.Folder == "" {
			enum.ContainerID = existingEnum.ContainerID
		}
		if err := ctx.Backend.UpdateEnumeration(enum); err != nil {
			return mdlerrors.NewBackend("update enumeration", err)
		}
		target, terr := resolveRequestedFolder(ctx, module.ID, s.Folder)
		if terr != nil {
			return terr
		}
		if _, err := applyDocumentFolder(ctx, enum.ID, existingEnum.ContainerID, target); err != nil {
			return err
		}
		invalidateHierarchy(ctx)
		ctx.ReportMutation("Modified", "enumeration: %s", s.Name)
		return nil
	}

	if err := ctx.Backend.CreateEnumeration(enum); err != nil {
		return mdlerrors.NewBackend("create enumeration", err)
	}

	// Invalidate hierarchy cache so the new enumeration's container is visible
	invalidateHierarchy(ctx)

	fmt.Fprintf(ctx.Output, "Created enumeration: %s\n", s.Name)
	return nil
}

// findEnumeration finds an enumeration by module and name.
func findEnumeration(ctx *ExecContext, moduleName, enumName string) *model.Enumeration {

	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return nil
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}

	// Prefer the live enumeration: a module may hold an excluded twin of this
	// name (#914), and the app only has the active one.
	if enum, ok := pickLive(enums,
		func(e *model.Enumeration) bool {
			return e.Name == enumName && h.GetModuleName(h.FindModuleID(e.ContainerID)) == moduleName
		},
		func(e *model.Enumeration) bool { return e.Excluded },
	); ok {
		return enum
	}
	return nil
}

// execAlterEnumeration handles ALTER ENUMERATION ADD/DROP/RENAME VALUE by
// read-modify-writing the enumeration through the backend (engine-agnostic).
func execAlterEnumeration(ctx *ExecContext, s *ast.AlterEnumerationStmt) error {
	if err := refuseSystemEnumerationWrite("alter enumeration", s.Name); err != nil {
		return err
	}
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}
	enum := findEnumeration(ctx, s.Name.Module, s.Name.Name)
	if enum == nil {
		return mdlerrors.NewNotFound("enumeration", s.Name.String())
	}

	switch s.Operation {
	case ast.AlterEnumAdd:
		for _, v := range enum.Values {
			if v.Name == s.ValueName {
				// IF NOT EXISTS makes a script that adds a value re-runnable.
				// Without it the second run errors and `exec` STOPS THERE, so
				// one already-present value silently truncates the rest of the
				// script. (ako/mxcli-rest FINDINGS #60)
				if s.IfNotExists {
					fmt.Fprintf(ctx.Output, "Value '%s' already exists on enumeration %s — skipped\n", s.ValueName, s.Name)
					return nil
				}
				return mdlerrors.NewAlreadyExistsMsg("enumeration value", s.ValueName,
					fmt.Sprintf("value '%s' already exists on enumeration %s — use 'add value if not exists' to make the script re-runnable", s.ValueName, s.Name))
			}
		}
		enum.Values = append(enum.Values, model.EnumerationValue{
			Name:    s.ValueName,
			Caption: &model.Text{Translations: map[string]string{authoringLanguage(ctx): s.Caption}},
		})

	case ast.AlterEnumDrop:
		idx := -1
		for i, v := range enum.Values {
			if v.Name == s.ValueName {
				idx = i
				break
			}
		}
		if idx < 0 {
			// The DROP twin of the guard above: a re-run finds the value gone.
			if s.IfExists {
				fmt.Fprintf(ctx.Output, "Enumeration %s has no value '%s' — skipped\n", s.Name, s.ValueName)
				return nil
			}
			return mdlerrors.NewNotFound("enumeration value", s.ValueName)
		}
		enum.Values = append(enum.Values[:idx], enum.Values[idx+1:]...)

	case ast.AlterEnumRename:
		idx := -1
		for i, v := range enum.Values {
			if v.Name == s.ValueName {
				idx = i
				break
			}
		}
		if idx < 0 {
			return mdlerrors.NewNotFound("enumeration value", s.ValueName)
		}
		enum.Values[idx].Name = s.NewName

	case ast.AlterEnumModifyCaption:
		idx := -1
		for i, v := range enum.Values {
			if v.Name == s.ValueName {
				idx = i
				break
			}
		}
		if idx < 0 {
			return mdlerrors.NewNotFound("enumeration value", s.ValueName)
		}
		// Preserve any existing non-en_US translations; only replace the en_US caption.
		if enum.Values[idx].Caption == nil {
			enum.Values[idx].Caption = &model.Text{Translations: map[string]string{}}
		} else if enum.Values[idx].Caption.Translations == nil {
			enum.Values[idx].Caption.Translations = map[string]string{}
		}
		enum.Values[idx].Caption.Translations[authoringLanguage(ctx)] = s.Caption

	case ast.AlterEnumSetDocumentation:
		enum.Documentation = s.Documentation

	case ast.AlterEnumSetExportLevel:
		enum.ExportLevel = s.ExportLevel

	default:
		return mdlerrors.NewUnsupported("unknown ALTER ENUMERATION operation")
	}

	if err := ctx.Backend.UpdateEnumeration(enum); err != nil {
		return mdlerrors.NewBackend("alter enumeration", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered enumeration: %s\n", s.Name)
	return nil
}

// execDropEnumeration handles DROP ENUMERATION statements.
func execDropEnumeration(ctx *ExecContext, s *ast.DropEnumerationStmt) error {

	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	if err := refuseSystemEnumerationWrite("drop enumeration", s.Name); err != nil {
		return err
	}

	// Find enumeration
	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return mdlerrors.NewBackend("list enumerations", err)
	}

	// The owning module is resolved through the container hierarchy, as SHOW,
	// DESCRIBE and ALTER all do. An enumeration inside a FOLDER has the folder's
	// ID as its ContainerID, so matching that ID against the module list — which
	// is what this did — found nothing and reported the enumeration missing while
	// the other three commands resolved it happily (upstream #976).
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Collect all enumerations matching the name (and optional module).
	type match struct {
		enum       *model.Enumeration
		moduleName string
	}
	var matches []match
	for _, enum := range enums {
		if enum.Name != s.Name.Name {
			continue
		}
		modName := h.GetModuleName(h.FindModuleID(enum.ContainerID))
		if modName == "" {
			continue // container does not walk up to a module; not ours to drop
		}
		if s.Name.Module == "" || modName == s.Name.Module {
			matches = append(matches, match{enum, modName})
		}
	}

	switch len(matches) {
	case 0:
		return mdlerrors.NewNotFound("enumeration", s.Name.String())
	case 1:
		if err := ctx.Backend.DeleteEnumeration(matches[0].enum.ID); err != nil {
			return mdlerrors.NewBackend("delete enumeration", err)
		}
		fmt.Fprintf(ctx.Output, "Dropped enumeration: %s.%s\n", matches[0].moduleName, s.Name.Name)
		return nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, m.moduleName+"."+s.Name.Name)
		}
		return mdlerrors.NewValidationf("ambiguous enumeration name %q — found in: %s; use a fully-qualified name",
			s.Name.Name, strings.Join(names, ", "))
	}
}

// listEnumerations handles SHOW ENUMERATIONS command.
func listEnumerations(ctx *ExecContext, moduleName string) error {

	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return mdlerrors.NewBackend("list enumerations", err)
	}

	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		folderPath    string
		values        int
	}
	var rows []row

	for _, enum := range enums {
		modID := h.FindModuleID(enum.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + enum.Name
			folderPath := h.BuildFolderPath(enum.ContainerID)

			rows = append(rows, row{qualifiedName, modName, enum.Name, folderPath, len(enum.Values)})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	// Build TableResult
	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Folder", "Values"},
		Summary: fmt.Sprintf("(%d enumerations)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.folderPath, r.values})
	}
	return writeResult(ctx, result)
}

// describeEnumeration handles DESCRIBE ENUMERATION command.
func describeEnumeration(ctx *ExecContext, name ast.QualifiedName) error {

	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return mdlerrors.NewBackend("list enumerations", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, enum := range enums {
		modID := h.FindModuleID(enum.ContainerID)
		modName := h.GetModuleName(modID)
		if enum.Name == name.Name && (name.Module == "" || modName == name.Module) {
			// Output JavaDoc documentation if present
			if enum.Documentation != "" {
				outputJavadocIndented(ctx.Output, enum.Documentation, "")
			}

			// A System enumeration is a platform built-in: the write paths refuse
			// it, so emitting `create or modify …` would hand the reader a
			// statement mxcli rejects. Report it informationally instead, the way
			// DESCRIBE BUILDING BLOCK does for the other read-only doctype.
			if modName == "System" {
				fmt.Fprintf(ctx.Output, "-- Enumeration: %s.%s\n", modName, enum.Name)
				fmt.Fprintf(ctx.Output, "-- Values (%d):\n", len(enum.Values))
				for _, v := range enum.Values {
					fmt.Fprintf(ctx.Output, "--   %s\n", v.Name)
				}
				fmt.Fprintf(ctx.Output, "-- The System module is owned by the Mendix platform: this enumeration is\n")
				fmt.Fprintf(ctx.Output, "-- built in and read-only, so this output is informational, not re-executable.\n")
				return nil
			}

			// A caption is a Texts$Text, and Mendix has no language-neutral text:
			// on a project whose default language is not en_US the only stored
			// translation carries that language code, so asking for "en_US"
			// reported every caption as empty (mendixlabs/mxcli#1113). Read the
			// language CREATE writes (authoringLanguage), or describe -> exec
			// round-trips a real caption into ''.
			lang := describeDefaultLanguage(ctx)

			fmt.Fprintf(ctx.Output, "create or modify enumeration %s.%s (\n", modName, enum.Name)
			for i, v := range enum.Values {
				comma := ","
				if i == len(enum.Values)-1 {
					comma = ""
				}
				caption := pickTextTranslation(v.Caption, lang)
				// mdlQuote doubles an apostrophe (mendixlabs/mxcli#394): a
				// caption like "Won't fix" must re-parse.
				fmt.Fprintf(ctx.Output, "  %s %s%s\n", v.Name, mdlQuote(ctx, caption), comma)
			}
			// Emit the module folder so a moved enumeration round-trips (Bug 12b).
			// BuildFolderPath returns the module name at the module root and the
			// folder path when the enum lives in a folder.
			folderClause := ""
			if fp := h.BuildFolderPath(enum.ContainerID); fp != "" && fp != modName {
				folderClause = " folder " + mdlQuote(ctx, fp)
			}
			if c := exportLevelClause(enum.ExportLevel); c != "" { // mendixlabs/mxcli#1498
				folderClause += " " + c
			}
			fmt.Fprintf(ctx.Output, ")%s;\n", folderClause)
			return nil
		}
	}

	return mdlerrors.NewNotFound("enumeration", name.String())
}

// mendixReservedWords is the platform's reserved-word list, which lives in
// mdl/types because the parse-error hinting in mdl/visitor needs it too: a
// keyword MDL merely parses can be escaped by quoting, while one of these cannot,
// and telling the two apart is the whole point of suggesting a fix. CE7247.
var mendixReservedWords = types.PlatformReservedWords

// ValidateEnumeration checks enumeration value names for reserved words.
// Returns a list of structured violations with rule IDs (CE7247 equivalent).
// This function does not require a project connection.
func ValidateEnumeration(stmt *ast.CreateEnumerationStmt) []linter.Violation {
	var violations []linter.Violation
	seen := make(map[string]bool)
	for _, v := range stmt.Values {
		lower := strings.ToLower(v.Name)
		if seen[lower] {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL011",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"duplicate enumeration value name '%s'", v.Name),
				Location: linter.Location{
					DocumentType: "enumeration",
					DocumentName: stmt.Name.String(),
				},
				Suggestion: "Each enumeration value must have a unique name",
			})
		}
		seen[lower] = true
		if mendixReservedWords[lower] {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL010",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"enumeration value '%s' is a reserved word (CE7247)",
					v.Name),
				Location: linter.Location{
					DocumentType: "enumeration",
					DocumentName: stmt.Name.String(),
				},
				Suggestion: fmt.Sprintf("Rename to a non-reserved name (e.g., '%s_' or 'Is%s')", v.Name, v.Name),
			})
		}
	}
	return violations
}

// mendixSystemAttributeNames are attribute names reserved by the Mendix runtime.
// See mdl/types for why the list lives there.
var mendixSystemAttributeNames = types.SystemAttributeNames

// autoMemberNames maps each AutoX pseudo-type to the fixed Mendix system member
// name it materializes as. The declared identifier is discarded on write, so a
// mismatch is worth a warning (MDL022). Compared case-insensitively.
var autoMemberNames = map[ast.DataTypeKind]string{
	ast.TypeAutoOwner:       "owner",
	ast.TypeAutoChangedBy:   "ChangedBy",
	ast.TypeAutoCreatedDate: "CreatedDate",
	ast.TypeAutoChangedDate: "ChangedDate",
}

// ValidateEntity checks entity attribute names for reserved system names.
// Returns a list of structured violations with rule IDs. This function does not require a project connection.
//
// Two distinct checks run here:
//   - MDL020 (mendixSystemAttributeNames): only applies to persistent entities.
//     Owner/ChangedBy/CreatedDate/ChangedDate are auto-managed there and should
//     be declared with the AutoX pseudo-types.
//   - MDL021 (mendixReservedWords / CE7247): applies to ALL entity kinds.
//     Mendix rejects these names at runtime regardless of persistence.
func ValidateEntity(stmt *ast.CreateEntityStmt) []linter.Violation {
	var violations []linter.Violation
	persistent := stmt.Kind == ast.EntityPersistent
	kind := entityPersistenceOf(stmt.Kind)
	entityName := stmt.Name.String()
	violations = append(violations, validateIdempotencyGuard(stmt.CreateOrModify, stmt.IfNotExists, "entity", entityName)...)
	violations = append(violations, validateBareGeneralization(stmt)...)
	// MDL071: the entity's own name reads as an OQL keyword in `from Module.X`.
	violations = append(violations, ValidateOQLReservedEntityName(stmt)...)
	for _, attr := range stmt.Attributes {
		violations = append(violations, validateEntityAttribute(attr, kind, entityName)...)
		if !persistent {
			violations = append(violations, validateNPEValidationRules(attr, entityName)...)
		}
	}
	return violations
}

// entityPersistence is what the per-attribute checks know about the entity they
// are looking at. It is three-valued because ALTER ENTITY … ADD ATTRIBUTE does
// not carry the kind, and the two rules keyed on it want opposite defaults for
// that case: MDL020 stays silent unless the entity is known persistent, MDL071
// stays loud unless it is known NON-persistent. A boolean cannot express both,
// and the one it used to express dropped MDL071 from the ALTER path.
type entityPersistence int

const (
	persistenceUnknown entityPersistence = iota
	persistencePersistent
	persistenceNonPersistent
)

func entityPersistenceOf(kind ast.EntityKind) entityPersistence {
	switch kind {
	case ast.EntityPersistent:
		return persistencePersistent
	case ast.EntityNonPersistent:
		return persistenceNonPersistent
	default:
		// A VIEW entity is queryable from OQL — indeed its own attribute names
		// ARE select aliases, the one MDL071 case that needs a rename — and an
		// EXTERNAL entity is not known to be out of reach either.
		return persistencePersistent
	}
}

// validateNPEValidationRules (MDL054) rejects a validation rule on a
// non-persistable entity, which Mendix refuses with
//
//	CE0070 "Validations rules are not allowed on entity 'X', because it is
//	        not persistable."
//
// `not null` and `unique` ARE validation rules: Studio Pro models "required"
// and "uniqueness" as rules on the entity rather than as column constraints,
// so both are rejected on an NPE. Verified against mxbuild 11.6.6 — `not null`
// with a message, `not null` bare, and `unique` each produce CE0070; a plain
// attribute does not. The message is optional and does not change the verdict,
// so the bare form is flagged too. Issue #832.
//
// Only the CREATE path can run this: an `ALTER ENTITY … ADD ATTRIBUTE` does not
// carry the entity's persistence kind, so ValidateAlterEntity cannot tell an NPE
// from a persistent entity without a project. That is the same limitation
// MDL020 has, for the same reason — see ValidateAlterEntity.
func validateNPEValidationRules(attr ast.Attribute, entityName string) []linter.Violation {
	var violations []linter.Violation
	flag := func(constraint, mdl string) {
		violations = append(violations, linter.Violation{
			RuleID:   "MDL054",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"attribute '%s' declares `%s` on non-persistent entity %s — Mendix does not allow "+
					"validation rules on a non-persistable entity (CE0070), and `%s` is a validation rule",
				attr.Name, constraint, entityName, constraint),
			Location: linter.Location{DocumentType: "entity", DocumentName: entityName},
			Suggestion: fmt.Sprintf(
				"Drop `%s` from the attribute, or make the entity persistent. To keep the check on a "+
					"non-persistent entity, enforce it in the microflow that populates it "+
					"(e.g. `if %s = empty then` … ) rather than declaring `%s`.",
				mdl, attr.Name, mdl),
		})
	}
	if attr.NotNull {
		flag("not null", "not null")
	}
	if attr.Unique {
		flag("unique", "unique")
	}
	return violations
}

// ValidateAlterEntity applies the same per-attribute name/seed checks to the
// ALTER ENTITY … ADD ATTRIBUTE path that ValidateEntity applies on CREATE, so
// an attribute added later is held to the same rules as one declared up front
// (findings #6). The entity's persistence kind is not known from the ALTER
// statement alone, so the persistent-only MDL020 system-attribute check is
// skipped here; the kind-independent checks (MDL021 reserved words, MDL022 AutoX
// rename, MDL023 autonumber seed) all still run.
func ValidateAlterEntity(stmt *ast.AlterEntityStmt) []linter.Violation {
	// MDL071 also applies to RENAME ATTRIBUTE — renaming is how this collision
	// is normally fixed, so it is also how it gets introduced, onto a name that
	// already has references.
	if stmt.Operation == ast.AlterEntityRenameAttribute {
		return ValidateOQLReservedRename(stmt)
	}
	if stmt.Operation != ast.AlterEntityAddAttribute || stmt.Attribute == nil {
		return nil
	}
	return validateEntityAttribute(*stmt.Attribute, persistenceUnknown, stmt.Name.String())
}

// validateEntityAttribute runs the reserved-name / AutoX / autonumber-seed checks
// for a single attribute. kind enables the MDL020 system-attribute check, which
// is only meaningful when the entity is known to be persistent, and suppresses
// MDL071 when it is known NOT to be.
func validateEntityAttribute(attr ast.Attribute, kind entityPersistence, entityName string) []linter.Violation {
	persistent := kind == persistencePersistent
	var violations []linter.Violation
	// MDL071 first, so the early returns further down still carry it: an OQL
	// collision is independent of every other property of the attribute.
	violations = append(violations, validateOQLReservedAttributeName(attr, kind, entityName)...)
	// AutoX pseudo-types ARE the system attributes. The declared identifier is
	// discarded — the field always materializes under its fixed system member
	// name — so warn (MDL022) when the two differ, since the write silently
	// renames it and the resulting member can't be bound as a regular widget
	// attribute (a widget binding it fails the build with CE1613). (findings #7)
	if canon, ok := autoMemberNames[attr.Type.Kind]; ok {
		if !strings.EqualFold(attr.Name, canon) {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL022",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf(
					"attribute '%s: %s' is renamed to the fixed system member '%s' on write — "+
						"the declared name is discarded",
					attr.Name, attr.Type.Kind, canon),
				Location: linter.Location{DocumentType: "entity", DocumentName: entityName},
				Suggestion: fmt.Sprintf(
					"Declare it as '%s: %s' to match. This is a Mendix system member and cannot be "+
						"bound in a widget; to store a value a widget can show, use a plain attribute "+
						"(e.g. '%s: DateTime') and set it yourself.",
					canon, attr.Type.Kind, attr.Name),
			})
		}
		return violations
	}
	// autonumber needs a seed, or the build fails CE7247 "Value cannot be
	// empty". mxcli check accepted it silently before. (findings #6)
	if attr.Type.Kind == ast.TypeAutoNumber && !attr.HasDefault {
		violations = append(violations, linter.Violation{
			RuleID:   "MDL023",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"autonumber attribute '%s' has no seed — the build fails CE7247 \"Value cannot be empty\"",
				attr.Name),
			Location:   linter.Location{DocumentType: "entity", DocumentName: entityName},
			Suggestion: fmt.Sprintf("Give it a start value: '%s: autonumber default 1'", attr.Name),
		})
		return violations
	}
	lower := strings.ToLower(attr.Name)
	// MDL020: system-attribute conflict — only meaningful on persistent entities,
	// where the suggested fix is to use the AutoX pseudo-type.
	if persistent && mendixSystemAttributeNames[lower] {
		violations = append(violations, linter.Violation{
			RuleID:   "MDL020",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"attribute '%s' conflicts with a Mendix system attribute name. "+
					"Mendix automatically manages '%s' on persistent entities",
				attr.Name, attr.Name),
			Location: linter.Location{
				DocumentType: "entity",
				DocumentName: entityName,
			},
			Suggestion: fmt.Sprintf("To use the Mendix built-in audit field, declare it with the pseudo-type: '%s: Auto%s'. To store an unrelated date, choose a different name (e.g., 'EntryDate', 'RecordDate')", attr.Name, attr.Name),
		})
		return violations
	}
	// MDL021: CE7247 runtime reserved words — apply to all entity kinds
	// including non-persistent and view entities.
	if mendixReservedWords[lower] {
		violations = append(violations, linter.Violation{
			RuleID:   "MDL021",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"attribute '%s' is a reserved word (CE7247)",
				attr.Name),
			Location: linter.Location{
				DocumentType: "entity",
				DocumentName: entityName,
			},
			Suggestion: fmt.Sprintf("Rename to a non-reserved name (e.g., '%sValue' or '%sField')", attr.Name, attr.Name),
		})
	}
	return violations
}

// validateIdempotencyGuard (MDL085) rejects CREATE OR MODIFY … IF NOT EXISTS.
//
// Both spellings exist to make a script re-runnable and they mean opposite
// things about an existing element: OR MODIFY rebuilds it from the statement,
// IF NOT EXISTS leaves it exactly as it is. Written together, one of them is
// silently ignored, and which one is not something the author can tell by
// reading the statement — so the statement is refused rather than resolved.
// (sudoku findings #10)
func validateIdempotencyGuard(createOrModify, ifNotExists bool, kind, name string) []linter.Violation {
	if !createOrModify || !ifNotExists {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL085",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf("'create or modify %s ... if not exists' combines two contradictory guards — "+
			"'or modify' replaces the stored definition, 'if not exists' leaves it untouched", kind),
		Suggestion: fmt.Sprintf("Keep one: 'create or modify %s %s' to converge on this definition, "+
			"or 'create %s if not exists %s' to leave an existing one alone.", kind, name, kind, name),
		Location: linter.Location{DocumentType: kind, DocumentName: name},
	}}
}

// carryRemoteEnumeration keeps a stored enumeration's OData source, and each
// value's remote member by value name, on a rebuild from a statement that
// cannot say them (ako/mxcli#2477). A value the statement adds has no member.
func carryRemoteEnumeration(enum, stored *model.Enumeration) {
	enum.RemoteService = stored.RemoteService
	enum.RemoteName = stored.RemoteName
	members := map[string]string{}
	for _, v := range stored.Values {
		members[v.Name] = v.RemoteName
	}
	for i := range enum.Values {
		enum.Values[i].RemoteName = members[enum.Values[i].Name]
	}
}
