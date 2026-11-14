// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/generated/metamodel"
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/settingsoverlay"
	"github.com/mendixlabs/mxcli/model"
)

// listSettings displays an overview table of all settings parts.
func listSettings(ctx *ExecContext) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	tr := &TableResult{
		Columns: []string{"Section", "Key Values"},
	}

	if id := projectID(ctx); id != "" {
		tr.Rows = append(tr.Rows, []any{"Project", "ID: " + id + " (read-only; Business Events app id)"})
	}

	if ps.Model != nil {
		ms := ps.Model
		values := []string{}
		if ms.AfterStartupMicroflow != "" {
			values = append(values, "AfterStartup: "+ms.AfterStartupMicroflow)
		}
		values = append(values, "Hash: "+ms.HashAlgorithm)
		values = append(values, "Java: "+ms.JavaVersion)
		tr.Rows = append(tr.Rows, []any{"Model Settings", strings.Join(values, ", ")})
	}

	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			values := []string{}
			values = append(values, cfg.DatabaseType)
			// An empty DatabaseUrl used to render as a bare ", ," — a gap where
			// a value should be, which reads as a bug in the reader.
			if cfg.DatabaseUrl != "" {
				values = append(values, cfg.DatabaseUrl)
			}
			values = append(values, "db="+cfg.DatabaseName)
			values = append(values, fmt.Sprintf("http=%d", cfg.HttpPortNumber))
			// The root URL decides the host the app answers on, so "did my root
			// URL land?" is the obvious question to ask this command — and the
			// summary used to be unable to answer it (mxcli-formula1 #8).
			if cfg.ApplicationRootUrl != "" {
				values = append(values, "url="+cfg.ApplicationRootUrl)
			}
			if len(cfg.ConstantValues) > 0 {
				values = append(values, fmt.Sprintf("%d constants", len(cfg.ConstantValues)))
			}
			tr.Rows = append(tr.Rows, []any{
				fmt.Sprintf("Configuration '%s'", cfg.Name),
				strings.Join(values, ", "),
			})
		}
	}

	if ps.Language != nil {
		v := "Default: " + ps.Language.DefaultLanguageCode
		if codes := enabledLanguageCodes(ps.Language); len(codes) > 0 {
			v += ", enabled: " + strings.Join(codes, ", ")
		}
		tr.Rows = append(tr.Rows, []any{"Language Settings", v})
	}

	if ps.Workflows != nil {
		ws := ps.Workflows
		values := []string{}
		if ws.UserEntity != "" {
			values = append(values, "UserEntity: "+ws.UserEntity)
		}
		if ws.DefaultTaskParallelism > 0 {
			values = append(values, fmt.Sprintf("TaskParallelism: %d", ws.DefaultTaskParallelism))
		}
		if len(ws.Groups) > 0 {
			values = append(values, fmt.Sprintf("%d group(s)", len(ws.Groups)))
		}
		tr.Rows = append(tr.Rows, []any{"Workflow Settings", strings.Join(values, ", ")})
	}

	if ps.Convention != nil {
		tr.Rows = append(tr.Rows, []any{"Convention Settings", "AssocStorage: " + ps.Convention.DefaultAssociationStorage})
	}

	if ps.WebUI != nil {
		tr.Rows = append(tr.Rows, []any{"Web UI Settings", "OptimizedClient: " + ps.WebUI.UseOptimizedClient})
	}

	return writeResult(ctx, tr)
}

// projectID is the $ID of the Projects$Project root unit: the container every
// module points to, and the app id Business Events registers under — so two
// apps made from one model (a copied .mpr, a template) share it
// (mendixlabs/mxcli#1526). Read-only; empty when the backend cannot say.
func projectID(ctx *ExecContext) string {
	id, err := ctx.Backend.GetProjectRootID()
	if err != nil {
		return ""
	}
	return id
}

// describeSettings outputs the full MDL description of all settings.
// describeSettings prints the project settings as re-executable `alter settings`
// statements. With configName set (DESCRIBE SETTINGS CONFIGURATION 'X') it
// prints only that configuration — the read form of `alter settings
// configuration 'X'`, which used to be a parse error.
func describeSettings(ctx *ExecContext, configName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	if configName != "" {
		return describeSettingsConfiguration(ctx, ps, configName)
	}

	// The project id has no `alter settings` spelling — it is the identity of
	// the model — so it is printed as a comment and the output still executes.
	if id := projectID(ctx); id != "" {
		fmt.Fprintf(ctx.Output, "-- Project ID: %s (read-only; Business Events uses it as the app id, and a copy of the model shares it)\n\n", id)
	}

	writeProjectSettingsMDL(ctx, ps, nil)
	return nil
}

// constantValueFunc renders the value of a configuration's constant override
// in a settings description; nil prints it as a quoted literal.
type constantValueFunc func(configuration, constant, value string) string

// writeProjectSettingsMDL prints the settings document as DESCRIBE SETTINGS
// does, below the project id (which is not part of the document). diff-local
// renders the settings unit with it, passing a constantValue that masks the
// values, so the two cannot drift apart (ako/mxcli#1512).
func writeProjectSettingsMDL(ctx *ExecContext, ps *model.ProjectSettings, constantValue constantValueFunc) {
	// Model settings
	if ps.Model != nil {
		ms := ps.Model
		// Emit only the properties this project's Mendix version actually stores,
		// so the output replays: the executor refuses an ALTER naming one the
		// document does not carry (see refuseIfNotStored), and the versions differ
		// by five properties between 9.24 and 11.13.
		stored := storedModelSettings(ps)
		var parts []string
		// add emits a property only if this project's Mendix version stores it, so
		// the output replays: the executor refuses an ALTER naming a property the
		// document does not carry (refuseIfNotStored), and the set differs by five
		// properties between a blank 9.24 and a blank 11.13 project.
		add := func(key, format string, args ...any) {
			if stored != nil && !settingsoverlay.Has(stored, key) {
				return
			}
			parts = append(parts, "  "+fmt.Sprintf(format, args...))
		}
		// addIfSet additionally skips a property holding no value, so a blank
		// project's describe output is not a wall of empty strings.
		addIfSet := func(key, format string, v string) {
			if v != "" {
				add(key, format, v)
			}
		}

		addIfSet("AfterStartupMicroflow", "AfterStartupMicroflow: '%s'", ms.AfterStartupMicroflow)
		addIfSet("BeforeShutdownMicroflow", "BeforeShutdownMicroflow: '%s'", ms.BeforeShutdownMicroflow)
		addIfSet("HealthCheckMicroflow", "HealthCheckMicroflow: '%s'", ms.HealthCheckMicroflow)
		add("HashAlgorithm", "HashAlgorithm: '%s'", ms.HashAlgorithm)
		add("BcryptCost", "BcryptCost: %d", ms.BcryptCost)
		// JavaVersion is stored under either JavaVersion or JavaMajorVersion; emit it
		// when the document carries whichever spelling, in mxcli's single input name.
		if settingsoverlay.JavaVersionKey(stored) != "" || stored == nil {
			parts = append(parts, fmt.Sprintf("  JavaVersion: '%s'", ms.JavaVersion))
		}
		add("RoundingMode", "RoundingMode: '%s'", ms.RoundingMode)
		add("AllowUserMultipleSessions", "AllowUserMultipleSessions: %t", ms.AllowUserMultipleSessions)
		add("EnableDataStorageOptimisticLocking", "EnableDataStorageOptimisticLocking: %t", ms.EnableDataStorageOptimisticLocking)
		add("UseDatabaseForeignKeyConstraints", "UseDatabaseForeignKeyConstraints: %t", ms.UseDatabaseForeignKeyConstraints)
		add("UseOQLVersion2", "UseOQLVersion2: %t", ms.UseOQLVersion2)
		add("DecimalScale", "DecimalScale: %d", ms.DecimalScale)
		addIfSet("FirstDayOfWeek", "FirstDayOfWeek: '%s'", ms.FirstDayOfWeek)
		addIfSet("SslCertificateAlgorithm", "SslCertificateAlgorithm: '%s'", ms.SslCertificateAlgorithm)
		addIfSet("ScheduledEventTimeZoneCode", "ScheduledEventTimeZoneCode: '%s'", ms.ScheduledEventTimeZoneCode)
		addIfSet("DefaultTimeZoneCode", "DefaultTimeZoneCode: '%s'", ms.DefaultTimeZoneCode)
		// R3: the ( Key: value, … ) list every alter takes (ako/mxcli#751). An
		// empty list does not parse, so nothing is written for none.
		if len(parts) > 0 {
			fmt.Fprintf(ctx.Output, "alter settings runtime (\n%s\n);\n\n", strings.Join(parts, ",\n"))
		}
	}

	// Configuration settings
	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			writeSettingsConfiguration(ctx, cfg, constantValue)
		}
	}

	// Language settings
	if ps.Language != nil {
		// The enabled languages decide what the build emits — a translation for
		// any other language is stored and then discarded — so DESCRIBE must
		// reproduce them, as STATEMENTS rather than the comment this used to
		// print. ADD OR MODIFY is the form that re-executes: plain ADD refuses a
		// language that is already enabled, so a described project could not be
		// replayed onto itself.
		//
		// Order matters. DefaultLanguageCode is validated against the enabled
		// list, so the languages have to exist first — emitting the default line
		// first (as this did) produces output that fails on a project that does
		// not already have that language.
		//
		// Every option is named, including the ones at their default: MODIFY
		// touches only what it names, so an omitted CustomDateFormat would leave a
		// format the target happens to have, and the replay would not reproduce
		// the described project.
		for _, l := range ps.Language.Languages {
			if l.Code == "" {
				continue
			}
			fmt.Fprintf(ctx.Output,
				"alter settings LANGUAGE add or modify %s (\n"+
					"  CheckCompleteness: %t,\n"+
					"  CustomDateFormat: %s,\n"+
					"  CustomTimeFormat: %s,\n"+
					"  CustomDateTimeFormat: %s\n);\n",
				// A date format quotes literal text (`d 'de' MMMM`), so these
				// need the escape as much as any free text does.
				mdlQuoted(l.Code), l.CheckCompleteness, mdlQuoted(l.CustomDateFormat),
				mdlQuoted(l.CustomTimeFormat), mdlQuoted(l.CustomDateTimeFormat))
		}
		fmt.Fprintf(ctx.Output, "alter settings LANGUAGE (\n  DefaultLanguageCode: '%s'\n);\n", ps.Language.DefaultLanguageCode)
		fmt.Fprintln(ctx.Output)
	}

	// Workflow settings
	if ps.Workflows != nil {
		ws := ps.Workflows
		var parts []string
		if ws.UserEntity != "" {
			parts = append(parts, fmt.Sprintf("  UserEntity: '%s'", ws.UserEntity))
		}
		if ws.DefaultTaskParallelism > 0 {
			parts = append(parts, fmt.Sprintf("  DefaultTaskParallelism: %d", ws.DefaultTaskParallelism))
		}
		if ws.WorkflowEngineParallelism > 0 {
			parts = append(parts, fmt.Sprintf("  WorkflowEngineParallelism: %d", ws.WorkflowEngineParallelism))
		}
		if len(parts) > 0 {
			fmt.Fprintf(ctx.Output, "alter settings workflows (\n%s\n);\n\n", strings.Join(parts, ",\n"))
		}
		// The groups, in stored order. `add or modify` so a described project
		// re-executes against a project that already has some of them.
		for _, g := range ws.Groups {
			fmt.Fprintf(ctx.Output,
				"alter settings workflows add or modify group '%s' (\n  Description: '%s'\n);\n",
				escapeMDLString(g.Name), escapeMDLString(g.Description))
		}
		if len(ws.Groups) > 0 {
			fmt.Fprintln(ctx.Output)
		}
	}

	// Web UI settings: which client the app runs on. Emitted only when the
	// project stores the property, so the output replays (ALTER refuses one the
	// part does not carry).
	if ps.WebUI != nil && ps.WebUI.UseOptimizedClient != "" {
		if stored := storedWebUISettings(ps); stored == nil || settingsoverlay.Has(stored, "UseOptimizedClient") {
			fmt.Fprintf(ctx.Output, "alter settings webui (\n  OptimizedClient: %s\n);\n\n", mdlQuoted(ps.WebUI.UseOptimizedClient))
		}
	}
}

// modelSettingKeys names every property the ALTER SETTINGS MODEL switch below
// assigns, so an unknown key can be rejected with a list of what would have
// worked. A bare "unknown model setting" sent the mxcli-banking app through
// three wrong guesses at the optimistic-locking property without ever naming the
// real one.
//
// Hand-maintained alongside the switch: add a case, add it here.
// TestModelSettingKeys_AllAccepted fails when a listed key is not accepted.
//
// Deliberately absent: UseSystemContextForBackgroundTasks. The property is still
// stored, and mxcli reads and round-trips it, but Mendix has withdrawn it —
// `mx check` on 11.13 rejects a project holding true with
// CE9436 "The project setting 'System context tasks' is not supported anymore."
// Its only legal value on a current version is its default, so an ALTER for it
// could only ever produce a project that does not build.
var modelSettingKeys = []string{
	"AfterStartupMicroflow",
	"BeforeShutdownMicroflow",
	"HealthCheckMicroflow",
	"HashAlgorithm",
	"BcryptCost",
	"JavaVersion",
	"RoundingMode",
	"AllowUserMultipleSessions",
	"ScheduledEventTimeZoneCode",
	"DefaultTimeZoneCode",
	"FirstDayOfWeek",
	"DecimalScale",
	"EnableDataStorageOptimisticLocking",
	"UseDatabaseForeignKeyConstraints",
	"UseOQLVersion2",
	"SslCertificateAlgorithm",
}

// isKnownModelSetting reports whether ALTER SETTINGS MODEL has a case for a key.
// Kept beside modelSettingKeys so the membership test and the "valid keys" list
// in the error can never disagree.
func isKnownModelSetting(key string) bool {
	for _, k := range modelSettingKeys {
		if k == key {
			return true
		}
	}
	return false
}

// firstDayOfWeekValues and sslCertificateAlgorithmValues are the members of the
// corresponding Mendix enumerations as Studio Pro spells them in BSON. Passing an
// unrecognised string through is the mendixlabs/mxcli#759 shape: the metamodel
// cannot resolve it and Studio Pro throws "Sequence contains no matching element",
// while mxbuild loads the project fine.
//
// generated/metamodel is a snapshot of Mendix 11.6, so a member added in a later
// version would be rejected here. That is the safer direction — the error names
// what is accepted, so an over-strict refusal is obvious rather than silent.
var firstDayOfWeekValues = []string{
	string(metamodel.SettingsFirstDayOfWeekDefault),
	string(metamodel.SettingsFirstDayOfWeekMonday),
	string(metamodel.SettingsFirstDayOfWeekTuesday),
	string(metamodel.SettingsFirstDayOfWeekWednesday),
	string(metamodel.SettingsFirstDayOfWeekThursday),
	string(metamodel.SettingsFirstDayOfWeekFriday),
	string(metamodel.SettingsFirstDayOfWeekSaturday),
	string(metamodel.SettingsFirstDayOfWeekSunday),
}

var sslCertificateAlgorithmValues = []string{
	string(metamodel.SettingsSslCertificateAlgorithmPKIX),
	string(metamodel.SettingsSslCertificateAlgorithmSunX509),
}

// useOptimizedClientValues are the members of Settings.UseOptimizedClient, the
// Web UI setting that picks the client the app runs on: Yes is the React client,
// No the classic (Dojo) one, MigrationMode Mendix's mixed mode. In the Model SDK's
// declaration order; no member carries a version gate of its own.
var useOptimizedClientValues = []string{
	string(metamodel.SettingsUseOptimizedClientNo),
	string(metamodel.SettingsUseOptimizedClientYes),
	string(metamodel.SettingsUseOptimizedClientMigrationMode),
}

// configurationSettingKeys names every property ALTER SETTINGS CONFIGURATION and
// CREATE CONFIGURATION assign. DatabaseUseIntegratedSecurity and the Open*Port
// flags are read and written back as stored, but no statement sets them.
var configurationSettingKeys = []string{
	"DatabaseType",
	"DatabaseUrl",
	"DatabaseName",
	"DatabaseUserName",
	"DatabasePassword",
	"HttpPortNumber",
	"ServerPortNumber",
	"ApplicationRootUrl",
	"ExtraJvmParameters",
	"MaxJavaHeapSize",
}

// webUISettingKeys names every property ALTER SETTINGS WEBUI assigns.
// OptimizedClient is the spelling `show settings` prints and the skills point to;
// it is stored as UseOptimizedClient.
var webUISettingKeys = []string{"OptimizedClient"}

// settingsEnumValues maps "<section>/<key>" to the enumeration members accepted for
// it, for the properties validated as settingsKindEnum.
var settingsEnumValues = map[string][]string{
	"model/FirstDayOfWeek":          firstDayOfWeekValues,
	"model/SslCertificateAlgorithm": sslCertificateAlgorithmValues,
	"webui/OptimizedClient":         useOptimizedClientValues,
}

// settingsEnum canonicalises an enumeration-typed settings value to the member
// Mendix stores, matching case-insensitively. Anything unrecognised is rejected
// rather than written through — same contract as settingsDatabaseType.
func settingsEnum(key, valStr string, members []string) (string, error) {
	want := strings.TrimSpace(valStr)
	for _, m := range members {
		if strings.EqualFold(m, want) {
			return m, nil
		}
	}
	return "", mdlerrors.NewValidationf("%s must be one of %s, got %q",
		key, strings.Join(members, ", "), valStr)
}

// storedModelSettings returns the raw Settings$ModelSettings part, so the handler
// can tell "this project does not store that property" from "that property is
// false". Returns nil when the part is not in RawParts.
func storedModelSettings(ps *model.ProjectSettings) map[string]any {
	for _, part := range ps.RawParts {
		if t, _ := part["$Type"].(string); t == "Settings$ModelSettings" {
			return part
		}
	}
	return nil
}

// storedWebUISettings returns the raw Web UI settings part, or nil when it is not
// in RawParts — the same contract as storedModelSettings.
func storedWebUISettings(ps *model.ProjectSettings) map[string]any {
	for _, part := range ps.RawParts {
		if t, _ := part["$Type"].(string); settingsoverlay.IsWebUIPart(t) {
			return part
		}
	}
	return nil
}

// refuseIfNotStored rejects an ALTER naming a property this project's Mendix
// version does not store. The overlay is presence-gated (it will not introduce a
// property the type may not define), so without this check the statement would
// report success and change nothing — the silent no-op shape of #805.
//
// JavaVersion is stored under either JavaVersion or JavaMajorVersion, and
// settingsoverlay.JavaVersionKey resolves which. A document carrying neither
// (Mendix 9.x) has no Java version setting at all, and the overlay would write
// nothing — mendixlabs/mxcli#1438 reported "Unchanged model settings" there.
func refuseIfNotStored(raw map[string]any, key string) error {
	if raw == nil {
		return nil
	}
	if key == "JavaVersion" {
		if settingsoverlay.JavaVersionKey(raw) != "" {
			return nil
		}
		return mdlerrors.NewUnsupported(
			"this project does not store the model setting JavaVersion\n" +
				"  The JavaVersion setting does not exist on this project's Mendix version " +
				"(Mendix 9.x has none); the JDK is chosen by the platform.\n" +
				"  hint: upgrade the project to choose a Java version in the model")
	}
	if settingsoverlay.Has(raw, key) {
		return nil
	}
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"this project does not store the model setting %s\n"+
			"  Mendix adds model settings over time — a blank 9.24 project stores 12 of them, "+
			"a blank 11.13 project stores 17.\n"+
			"  mxcli will not introduce a property the project's Mendix version may not define: "+
			"Studio Pro refuses to open a model carrying one, and mxbuild does not catch it.\n"+
			"  hint: set it in Studio Pro once, or upgrade the project", key))
}

// alterSettings modifies project settings based on ALTER SETTINGS statement.
func alterSettings(ctx *ExecContext, stmt *ast.AlterSettingsStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	// The refusals that need no project — an unknown section or key, a verb the
	// section does not define. `mxcli check` reports the same errors from the
	// same function, so a script that cannot run does not pass the check
	// (mendixlabs/mxcli#1549).
	if errs := settingsStatementErrors(stmt); len(errs) > 0 {
		return errs[0]
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	section := strings.ToLower(stmt.Section)
	// Set when this statement changes DefaultLanguageCode; see
	// defaultLanguageChanged.
	newDefaultLanguage := ""

	// Resolve the qualified names this statement would write BEFORE writing them.
	// `mxcli check --references` runs the same functions, so exec refuses exactly
	// what check reports — and a script run without check still cannot leave a
	// dangling AfterStartupMicroflow or an override for a constant that does not
	// exist (#274). No script context here: at exec time the statements before
	// this one have already run, so anything the script created is in the project.
	if section == "constant" {
		if known, trusted := buildConstantQualifiedNames(ctx); trusted {
			if errs := validateSettingsConstantRef(stmt, known, nil); len(errs) > 0 {
				return errs[0]
			}
		}
	} else {
		var mfs, ents map[string]bool
		var rets map[string]string
		if section == "model" {
			mfs = buildMicroflowQualifiedNames(ctx)
			rets = buildMicroflowReturnTypes(ctx)
		}
		if section == "workflows" {
			ents = buildEntityQualifiedNames(ctx)
		}
		if errs := validateSettingsReferences(stmt, mfs, ents, rets, nil); len(errs) > 0 {
			return errs[0]
		}
	}

	// ADD/REMOVE GROUP change the workflows part's Groups list rather than a key
	// on it, so they are dispatched before the key/value sections — as the
	// language forms below are, and for the same reason.
	if stmt.AddGroup || stmt.ModifyGroup || stmt.UpsertGroup || stmt.RemoveGroup {
		return alterSettingsWorkflowGroup(ctx, ps, stmt)
	}

	// ADD/REMOVE change the list of ENABLED languages rather than a key on the
	// settings part, so they are dispatched before the key/value sections.
	if stmt.AddLanguage || stmt.ModifyLanguage || stmt.UpsertLanguage || stmt.RemoveLanguage {
		switch {
		case stmt.UpsertLanguage:
			return alterSettingsLanguageUpsert(ctx, ps, stmt)
		case stmt.AddLanguage:
			return alterSettingsLanguageAdd(ctx, ps, stmt)
		case stmt.ModifyLanguage:
			return alterSettingsLanguageModify(ctx, ps, stmt)
		}
		return alterSettingsLanguageRemove(ctx, ps, stmt)
	}
	switch section {
	case "model":
		if ps.Model == nil {
			return mdlerrors.NewNotFound("settings section", "model")
		}
		storedModel := storedModelSettings(ps)
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			// Order matters: a key mxcli does not know at all is a different
			// problem from a real key this Mendix version does not store, and the
			// presence check cannot tell them apart. Running it first answered a
			// typo with "this project does not store NotARealSetting … upgrade the
			// project", sending the reader after a version problem that does not
			// exist instead of at the misspelling in front of them.
			if !isKnownModelSetting(key) {
				return unknownSettingsKey("model", key)
			}
			// The overlay will not introduce a property the stored document does
			// not carry, so refuse here rather than reporting a success that
			// changes nothing.
			if err := refuseIfNotStored(storedModel, key); err != nil {
				return err
			}
			switch key {
			case "AfterStartupMicroflow":
				ps.Model.AfterStartupMicroflow = valStr
			case "BeforeShutdownMicroflow":
				ps.Model.BeforeShutdownMicroflow = valStr
			case "HealthCheckMicroflow":
				ps.Model.HealthCheckMicroflow = valStr
			case "HashAlgorithm":
				ps.Model.HashAlgorithm = valStr
			case "BcryptCost":
				v, err := settingsInt(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.BcryptCost = v
			case "JavaVersion":
				ps.Model.JavaVersion = valStr
			case "RoundingMode":
				ps.Model.RoundingMode = valStr
			case "AllowUserMultipleSessions":
				v, err := settingsBool(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.AllowUserMultipleSessions = v
			case "ScheduledEventTimeZoneCode":
				ps.Model.ScheduledEventTimeZoneCode = valStr
			case "EnableDataStorageOptimisticLocking":
				// Mendix's App Settings → Runtime → "Optimistic locking". With it
				// on, the runtime tracks an MxObjectVersion per persistable entity
				// and a commit whose version no longer matches the database throws
				// ConcurrentModificationRuntimeException — the mitigation for a
				// read-then-write race (check balance, then write it) inside a
				// microflow, which the transaction alone does not make serialisable.
				// It detects rather than retries: the handler must catch, reload,
				// re-apply and re-commit.
				v, err := settingsBool(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.EnableDataStorageOptimisticLocking = v
			case "DefaultTimeZoneCode":
				ps.Model.DefaultTimeZoneCode = valStr
			case "FirstDayOfWeek":
				v, err := settingsEnum(key, valStr, firstDayOfWeekValues)
				if err != nil {
					return err
				}
				ps.Model.FirstDayOfWeek = v
			case "SslCertificateAlgorithm":
				v, err := settingsEnum(key, valStr, sslCertificateAlgorithmValues)
				if err != nil {
					return err
				}
				ps.Model.SslCertificateAlgorithm = v
			case "DecimalScale":
				v, err := settingsInt(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.DecimalScale = v
			case "UseDatabaseForeignKeyConstraints":
				v, err := settingsBool(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.UseDatabaseForeignKeyConstraints = v
			case "UseOQLVersion2":
				v, err := settingsBool(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.UseOQLVersion2 = v
			default:
				return unknownSettingsKey("model", key)
			}
		}

	case "language":
		if ps.Language == nil {
			return mdlerrors.NewNotFound("settings section", "language")
		}
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			switch key {
			case "DefaultLanguageCode":
				if err := validateLanguageCode(ps.Language, valStr); err != nil {
					return err
				}
				if !strings.EqualFold(ps.Language.DefaultLanguageCode, valStr) {
					newDefaultLanguage = valStr
				}
				ps.Language.DefaultLanguageCode = valStr

			default:
				return unknownSettingsKey("language", key)
			}
		}

	case "workflows":
		if ps.Workflows == nil {
			return mdlerrors.NewNotFound("settings section", "workflows")
		}
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			switch key {
			case "UserEntity":
				ps.Workflows.UserEntity = valStr
			case "DefaultTaskParallelism":
				v, err := settingsInt(key, valStr)
				if err != nil {
					return err
				}
				ps.Workflows.DefaultTaskParallelism = v
			case "WorkflowEngineParallelism":
				v, err := settingsInt(key, valStr)
				if err != nil {
					return err
				}
				ps.Workflows.WorkflowEngineParallelism = v
			default:
				return unknownSettingsKey("workflows", key)
			}
		}

	case "webui":
		if ps.WebUI == nil {
			return mdlerrors.NewNotFound("settings section", "webui")
		}
		storedWebUI := storedWebUISettings(ps)
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			switch key {
			case "OptimizedClient":
				// The overlay is presence-gated (the property arrived in 9.10), so
				// refuse rather than report a success that changes nothing.
				if storedWebUI != nil && !settingsoverlay.Has(storedWebUI, "UseOptimizedClient") {
					return mdlerrors.NewUnsupported(
						"this project does not store the Web UI setting UseOptimizedClient\n" +
							"  Mendix added it in 9.10; mxcli will not introduce a property the project's " +
							"Mendix version does not define.")
				}
				v, err := settingsEnum(key, valStr, useOptimizedClientValues)
				if err != nil {
					return err
				}
				ps.WebUI.UseOptimizedClient = v
			default:
				return unknownSettingsKey("webui", key)
			}
		}

	case "configuration":
		return alterSettingsConfiguration(ctx, ps, stmt)

	case "constant":
		return alterSettingsConstant(ctx, ps, stmt)

	default:
		return unknownSettingsSection(section)
	}

	// Write updated settings
	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	ctx.reportWrite(section+" settings", "Updated %s settings", section)
	if newDefaultLanguage != "" {
		defaultLanguageChanged(ctx, newDefaultLanguage)
	}
	return nil
}

// defaultLanguageChanged runs after a write that changed DefaultLanguageCode
// (ako/mxcli#944).
//
// The cached authoring language is dropped first: it was resolved once per
// session, so a page created later in the SAME script was still written in the
// old default — measured, its tab page caption then failed the build with
// CE4899 in the new one. Then it reports the required captions the project now
// has no text for, which is the moment they break.
func defaultLanguageChanged(ctx *ExecContext, lang string) {
	if ctx.Cache != nil {
		ctx.Cache.defaultLangLoaded = false
		describeDefaultLanguage(ctx) // republish the authoring language
	}
	missing, err := missingDefaultCaptions(ctx, lang)
	if err != nil {
		return
	}
	writeMissingDefaultCaptionsNote(ctx.Output, lang, missing)
}

// validateLanguageCode rejects a DefaultLanguageCode that is not one of the
// project's configured languages. Mendix has no such guard: `alter settings
// LANGUAGE` would accept e.g. 'nl_NL' on an en_US-only project, the write would
// report success, and the *next* `mx check` would die with an unhandled
// NullReferenceException rather than a model error (FINDINGS #6). Skipped when the
// project's language list is empty (unavailable) to avoid false rejections.
func validateLanguageCode(ls *model.LanguageSettings, code string) error {
	if ls == nil || len(ls.Languages) == 0 {
		return nil
	}
	avail := make([]string, 0, len(ls.Languages))
	for _, l := range ls.Languages {
		if l.Code == code {
			return nil
		}
		avail = append(avail, l.Code)
	}
	sort.Strings(avail)
	return mdlerrors.NewValidationf(
		"language %q is not configured in this project (available: %s) — add it in Studio Pro (Project ▸ Settings ▸ Languages) before making it the default",
		code, strings.Join(avail, ", "))
}

// findConfigurationByName resolves a configuration the way MDL addresses it, by
// name (case-insensitive). Studio Pro does not keep names unique: two
// configurations called "Default" arrive through a merge (ako/mxcli#2184). A
// statement naming one of them cannot say which it means, so it is refused
// rather than applied to whichever comes first; writes that address no
// configuration are unaffected.
func findConfigurationByName(ps *model.ProjectSettings, name string) (*model.ServerConfiguration, int, error) {
	found, idx, n := (*model.ServerConfiguration)(nil), -1, 0
	for i, c := range ps.Configuration.Configurations {
		if strings.EqualFold(c.Name, name) {
			if n == 0 {
				found, idx = c, i
			}
			n++
		}
	}
	switch n {
	case 0:
		return nil, -1, mdlerrors.NewNotFound("configuration", name)
	case 1:
		return found, idx, nil
	}
	return nil, -1, mdlerrors.NewValidationf(
		"configuration name '%s' is not unique: the project has %d configurations with that name; rename or remove one in Studio Pro (App Settings > Configurations) first",
		name, n)
}

func alterSettingsConfiguration(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	cfg, _, err := findConfigurationByName(ps, stmt.ConfigName)
	if err != nil {
		return err
	}

	for key, val := range stmt.Properties {
		valStr := settingsValueToString(val)
		switch key {
		case "DatabaseType":
			v, err := settingsDatabaseType(key, valStr)
			if err != nil {
				return err
			}
			cfg.DatabaseType = v
		case "DatabaseUrl":
			cfg.DatabaseUrl = valStr
		case "DatabaseName":
			cfg.DatabaseName = valStr
		case "DatabaseUserName":
			cfg.DatabaseUserName = valStr
		case "DatabasePassword":
			cfg.DatabasePassword = valStr
		case "HttpPortNumber":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			cfg.HttpPortNumber = v
		case "ServerPortNumber":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			cfg.ServerPortNumber = v
		case "ApplicationRootUrl":
			cfg.ApplicationRootUrl = valStr
		case "ExtraJvmParameters":
			// Studio Pro's Configuration ▸ Server ▸ "Extra JVM parameters", e.g. a
			// proxy: '-Dhttps.proxyHost=127.0.0.1 -Dhttps.proxyPort=8888'. Stored
			// verbatim; every supported Mendix version carries the property
			// (mendixlabs/mxcli#131).
			cfg.ExtraJvmParameters = valStr
		case "MaxJavaHeapSize":
			// Server ▸ Java heap ▸ Maximum size (in MB). The model has no
			// Default/Custom flag: 0 is Studio Pro's "Default".
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			cfg.MaxJavaHeapSize = v
		default:
			return unknownSettingsKey("configuration", key)
		}
	}

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	ctx.reportWrite(fmt.Sprintf("configuration '%s'", stmt.ConfigName), "Updated configuration '%s'", stmt.ConfigName)
	return nil
}

// settingsConstantType returns the data type of the constant a configuration
// override names (Module.Name), or the zero type when it cannot be resolved — an
// unresolved constant's value is then stored as written.
func settingsConstantType(ctx *ExecContext, constantID string) model.ConstantDataType {
	modName, name, ok := strings.Cut(constantID, ".")
	if !ok {
		return model.ConstantDataType{}
	}
	constants, err := ctx.Backend.ListConstants()
	if err != nil {
		return model.ConstantDataType{}
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return model.ConstantDataType{}
	}
	for _, c := range constants {
		if strings.EqualFold(c.Name, name) &&
			strings.EqualFold(h.GetModuleName(h.FindModuleID(c.ContainerID)), modName) {
			return c.Type
		}
	}
	return model.ConstantDataType{}
}

func alterSettingsConstant(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	// Find the target configuration
	targetConfig := stmt.ConfigName
	if targetConfig == "" {
		// Default to first configuration
		if len(ps.Configuration.Configurations) > 0 {
			targetConfig = ps.Configuration.Configurations[0].Name
		} else {
			return mdlerrors.NewValidation("no configurations found")
		}
	}

	cfg, _, err := findConfigurationByName(ps, targetConfig)
	if err != nil {
		return err
	}

	if stmt.DropConstant {
		// Remove the constant override
		for i, cv := range cfg.ConstantValues {
			if cv.ConstantId == stmt.ConstantId {
				cfg.ConstantValues = append(cfg.ConstantValues[:i], cfg.ConstantValues[i+1:]...)
				if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
					return mdlerrors.NewBackend("update project settings", err)
				}
				fmt.Fprintf(ctx.Output, "Dropped constant '%s' from configuration '%s'\n",
					stmt.ConstantId, targetConfig)
				return nil
			}
		}
		return mdlerrors.NewNotFoundMsg("constant", stmt.ConstantId, fmt.Sprintf("constant '%s' not found in configuration '%s'", stmt.ConstantId, targetConfig))
	}

	// A Boolean override is stored as "True" / "False", like the default (#1321).
	value := storedConstantDefault(settingsConstantType(ctx, stmt.ConstantId), stmt.Value)

	// Find or create the constant value
	found := false
	for _, cv := range cfg.ConstantValues {
		if cv.ConstantId == stmt.ConstantId {
			// Setting a value on a private override would convert it to a shared one,
			// publishing into the shared model a value the developer chose to keep on
			// their workstation — and breaking their local binding. The shared/private
			// choice belongs to the constant, so refuse rather than flip it silently.
			if cv.IsPrivate {
				return mdlerrors.NewValidationf(
					"constant '%s' has a private value in configuration '%s'; "+
						"its value is stored on the developer's workstation, not in the shared model. "+
						"Change the constant to a shared value in Studio Pro first, "+
						"or use `alter settings drop constant @%s in configuration '%s'` to remove the override",
					stmt.ConstantId, targetConfig, stmt.ConstantId, targetConfig)
			}
			cv.Value = value
			found = true
			break
		}
	}
	if !found {
		cv := &model.ConstantValue{
			ConstantId: stmt.ConstantId,
			Value:      value,
		}
		cv.TypeName = "Settings$ConstantValue"
		cfg.ConstantValues = append(cfg.ConstantValues, cv)
	}

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	ctx.reportWrite(fmt.Sprintf("constant '%s' in configuration '%s'", stmt.ConstantId, targetConfig),
		"Updated constant '%s' = '%s' in configuration '%s'", stmt.ConstantId, value, targetConfig)
	return nil
}

// createConfiguration handles CREATE CONFIGURATION 'name' [properties...].
func createConfiguration(ctx *ExecContext, stmt *ast.CreateConfigurationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if errs := unknownSettingsKeys("configuration", stmt.Properties); len(errs) > 0 {
		return errs[0]
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	// Check if configuration already exists. OR MODIFY updates it instead of
	// refusing — which is what makes a described project replayable, since
	// DESCRIBE emits every configuration and most targets already have Default.
	for _, cfg := range ps.Configuration.Configurations {
		if strings.EqualFold(cfg.Name, stmt.Name) {
			if !stmt.CreateOrModify {
				return mdlerrors.NewAlreadyExists("configuration", stmt.Name)
			}
			return alterSettingsConfiguration(ctx, ps, &ast.AlterSettingsStmt{
				Section: "configuration", ConfigName: cfg.Name, Properties: stmt.Properties,
			})
		}
	}

	// Mirror the configuration Studio Pro creates: the enum member "Hsqldb" (not
	// "HSQLDB", which no metamodel member matches) and both default ports, so a
	// fresh configuration is runnable and loads without repair (#759).
	newCfg := &model.ServerConfiguration{
		Name:             stmt.Name,
		DatabaseType:     string(metamodel.SettingsDatabaseTypeHsqldb),
		HttpPortNumber:   8080,
		ServerPortNumber: 8090,
		ConstantValues:   []*model.ConstantValue{},
	}
	newCfg.TypeName = "Settings$ServerConfiguration"

	// Apply optional properties
	for key, val := range stmt.Properties {
		valStr := settingsValueToString(val)
		switch key {
		case "DatabaseType":
			v, err := settingsDatabaseType(key, valStr)
			if err != nil {
				return err
			}
			newCfg.DatabaseType = v
		case "DatabaseUrl":
			newCfg.DatabaseUrl = valStr
		case "DatabaseName":
			newCfg.DatabaseName = valStr
		case "DatabaseUserName":
			newCfg.DatabaseUserName = valStr
		case "DatabasePassword":
			newCfg.DatabasePassword = valStr
		case "HttpPortNumber":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			newCfg.HttpPortNumber = v
		case "ServerPortNumber":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			newCfg.ServerPortNumber = v
		case "ApplicationRootUrl":
			newCfg.ApplicationRootUrl = valStr
		case "ExtraJvmParameters":
			newCfg.ExtraJvmParameters = valStr
		case "MaxJavaHeapSize":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			newCfg.MaxJavaHeapSize = v
		default:
			return unknownSettingsKey("configuration", key)
		}
	}

	ps.Configuration.Configurations = append(ps.Configuration.Configurations, newCfg)

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Created configuration: %s\n", stmt.Name)
	return nil
}

// dropConfiguration handles DROP CONFIGURATION 'name'.
func dropConfiguration(ctx *ExecContext, stmt *ast.DropConfigurationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	_, i, err := findConfigurationByName(ps, stmt.Name)
	if err != nil {
		return err
	}
	ps.Configuration.Configurations = append(
		ps.Configuration.Configurations[:i],
		ps.Configuration.Configurations[i+1:]...,
	)
	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped configuration: %s\n", stmt.Name)
	return nil
}

// settingsInt parses an Integer-typed settings value. The error used to be
// discarded (`if v, err := strconv.Atoi(...); err == nil`), so a non-numeric value
// skipped the assignment while the handler still printed its success line — the
// field silently kept its old value (#805).
func settingsInt(key, valStr string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(valStr))
	if err != nil {
		return 0, mdlerrors.NewValidationf("%s must be an integer, got %q", key, valStr)
	}
	return v, nil
}

// settingsBool parses a Boolean-typed settings value. Comparing against "true"
// turned every other spelling — a typo, or a plausible value like 'yes' — into
// false while still reporting success, the same silent no-op as settingsInt.
func settingsBool(key, valStr string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(valStr)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, mdlerrors.NewValidationf("%s must be true or false, got %q", key, valStr)
}

// databaseTypes lists the members of the Mendix Settings.DatabaseType enumeration
// exactly as Studio Pro spells them in BSON (generated/metamodel.SettingsDatabaseType).
// A configuration stored with anything else — mxcli hardcoded "HSQLDB" for every
// CREATE CONFIGURATION — is a value the metamodel cannot resolve, and Studio Pro
// throws "Sequence contains no matching element" when it loads the configuration
// (mendixlabs/mxcli#759).
var databaseTypes = []string{
	string(metamodel.SettingsDatabaseTypeDb2),
	string(metamodel.SettingsDatabaseTypeHsqldb),
	string(metamodel.SettingsDatabaseTypeMySql),
	string(metamodel.SettingsDatabaseTypeOracle),
	string(metamodel.SettingsDatabaseTypePostgreSql),
	string(metamodel.SettingsDatabaseTypeSapHana),
	string(metamodel.SettingsDatabaseTypeSqlServer),
}

// settingsDatabaseType canonicalises a DatabaseType value to the enum member
// Mendix stores, matching case-insensitively so 'postgresql' and 'PostgreSql' both
// land on the stored spelling. Anything unrecognised is rejected rather than
// written through.
func settingsDatabaseType(key, valStr string) (string, error) {
	want := strings.TrimSpace(valStr)
	for _, dt := range databaseTypes {
		if strings.EqualFold(dt, want) {
			return dt, nil
		}
	}
	return "", mdlerrors.NewValidationf("%s must be one of %s, got %q",
		key, strings.Join(databaseTypes, ", "), valStr)
}

// settingsValueToString converts an AST settings value to string.
func settingsValueToString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// writeSettingsConfiguration emits one configuration as re-executable MDL.
func writeSettingsConfiguration(ctx *ExecContext, cfg *model.ServerConfiguration, constantValue constantValueFunc) {
	var parts []string
	parts = append(parts, "  DatabaseType: "+mdlQuoted(cfg.DatabaseType))
	parts = append(parts, "  DatabaseUrl: "+mdlQuoted(cfg.DatabaseUrl))
	parts = append(parts, "  DatabaseName: "+mdlQuoted(cfg.DatabaseName))
	parts = append(parts, "  DatabaseUserName: "+mdlQuoted(cfg.DatabaseUserName))
	// DatabasePassword is deliberately not printed (ako/mxcli#707): describe
	// output is what gets committed and reviewed, and printing the stored value
	// put a database credential into PR diffs. Omitting the key is lossless for
	// the replay that matters — `create or modify` on an existing configuration
	// is a patch, so the stored password is left as it is. On a project without
	// this configuration the password starts empty, which the comment says.
	parts = append(parts, fmt.Sprintf("  HttpPortNumber: %d", cfg.HttpPortNumber))
	parts = append(parts, fmt.Sprintf("  ServerPortNumber: %d", cfg.ServerPortNumber))
	if cfg.ApplicationRootUrl != "" {
		parts = append(parts, "  ApplicationRootUrl: "+mdlQuoted(cfg.ApplicationRootUrl))
	}
	if cfg.ExtraJvmParameters != "" {
		parts = append(parts, "  ExtraJvmParameters: "+mdlQuoted(cfg.ExtraJvmParameters))
	}
	if cfg.MaxJavaHeapSize != 0 {
		parts = append(parts, fmt.Sprintf("  MaxJavaHeapSize: %d", cfg.MaxJavaHeapSize))
	}
	if cfg.DatabasePassword != "" {
		fmt.Fprintf(ctx.Output, "-- DatabasePassword is set in configuration %s and is not printed; "+
			"add `DatabasePassword: '…'` to set it on a project that lacks it.\n", mdlQuoted(cfg.Name))
	}
	// CREATE OR MODIFY, not ALTER: a described project has to replay onto a
	// target that does not have this configuration yet. ALTER answered
	// "configuration not found: Acceptance" and stopped the whole file, which is
	// the same shape as the language list emitting a comment — output that reads
	// correctly and cannot be run.
	fmt.Fprintf(ctx.Output, "create or modify configuration %s (\n%s\n);\n\n", mdlQuoted(cfg.Name), strings.Join(parts, ",\n"))

	// Output constant overrides. A private override has no value in the
	// model — emitting `value ''` would round-trip into a *shared* empty
	// override, moving a value that is deliberately kept off the shared model
	// into it. MDL does not author the shared/private choice, so describe
	// reports it as a comment instead of a re-executable statement.
	for _, cv := range cfg.ConstantValues {
		if cv.IsPrivate {
			fmt.Fprintf(ctx.Output, "-- constant '%s' has a private value in configuration '%s'\n"+
				"-- (stored on the developer's workstation; not part of the shared model)\n\n",
				cv.ConstantId, cfg.Name)
			continue
		}
		value := mdlQuoted(cv.Value)
		if constantValue != nil {
			value = constantValue(cfg.Name, cv.ConstantId, cv.Value)
		}
		fmt.Fprintf(ctx.Output, "alter settings constant %s value %s\n  in configuration %s;\n\n",
			mdlQuoted(cv.ConstantId), value, mdlQuoted(cfg.Name))
	}
}

// describeSettingsConfiguration prints a single named configuration, or names
// the ones that exist when the requested name is not among them.
func describeSettingsConfiguration(ctx *ExecContext, ps *model.ProjectSettings, name string) error {
	var available []string
	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			if strings.EqualFold(cfg.Name, name) {
				writeSettingsConfiguration(ctx, cfg, nil)
				return nil
			}
			available = append(available, "'"+cfg.Name+"'")
		}
	}
	return mdlerrors.NewNotFoundMsg("settings configuration", name,
		fmt.Sprintf("settings configuration not found: '%s' (available: %s)", name, strings.Join(available, ", ")))
}
