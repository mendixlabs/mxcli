// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// settingsValueKind classifies the settings properties whose value the executor
// parses rather than storing verbatim. Anything unlisted is a string as written.
type settingsValueKind int

const (
	settingsKindInt settingsValueKind = iota
	settingsKindBool
	settingsKindDatabaseType
	// settingsKindEnum covers any other enumeration-typed property; its accepted
	// members are looked up per section/key in settingsEnumValues.
	settingsKindEnum
)

// typedSettingsKeys maps a lower-cased ALTER SETTINGS section to the properties
// whose value must parse as something other than a string. It mirrors the
// assignment switches in cmd_settings.go; TestTypedSettingsKeys_MatchExecutor
// fails if the two drift apart.
var typedSettingsKeys = map[string]map[string]settingsValueKind{
	"model": {
		"BcryptCost":                         settingsKindInt,
		"DecimalScale":                       settingsKindInt,
		"AllowUserMultipleSessions":          settingsKindBool,
		"EnableDataStorageOptimisticLocking": settingsKindBool,
		"UseDatabaseForeignKeyConstraints":   settingsKindBool,
		"UseOQLVersion2":                     settingsKindBool,
		"FirstDayOfWeek":                     settingsKindEnum,
		"SslCertificateAlgorithm":            settingsKindEnum,
	},
	"workflows": {
		"DefaultTaskParallelism":    settingsKindInt,
		"WorkflowEngineParallelism": settingsKindInt,
	},
	"configuration": {
		"HttpPortNumber":   settingsKindInt,
		"ServerPortNumber": settingsKindInt,
		"MaxJavaHeapSize":  settingsKindInt,
		"DatabaseType":     settingsKindDatabaseType,
	},
	"webui": {
		"OptimizedClient": settingsKindEnum,
	},
}

// settingsSectionKeys names, per key/value section of ALTER SETTINGS, every
// property the executor assigns. It is the gate for which keys exist: alterSettings
// refuses any other key before it reads the project, and `mxcli check` reports
// the same refusal from the same function (settingsStatementErrors), so the two
// cannot disagree. A key given a case in an executor switch but not listed here is
// refused by exec too — so the feature's own test fails, not a user's script.
var settingsSectionKeys = map[string][]string{
	"model":         modelSettingKeys,
	"configuration": configurationSettingKeys,
	"language":      {"DefaultLanguageCode"},
	"workflows":     {"UserEntity", "DefaultTaskParallelism", "WorkflowEngineParallelism"},
	"webui":         webUISettingKeys,
}

// settingsKeyNoun is how an unknown-key refusal names its section.
var settingsKeyNoun = map[string]string{
	"model":         "model",
	"configuration": "configuration",
	"language":      "language",
	"workflows":     "workflow",
	"webui":         "webui",
}

// unknownSettingsSection is the refusal for a section ALTER SETTINGS does not
// have. The grammar takes any identifier as a section, so this is the only place
// a misspelled one is caught. The list names `runtime`, the spelling MDL writes;
// `model` is its deprecated alias.
func unknownSettingsSection(section string) error {
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"unknown settings section: %s (expected runtime, configuration, constant, language, workflows, or webui)",
		section))
}

// unknownSettingsKey is the refusal for a key the section does not have, naming
// the keys it does — a bare "unknown setting" sent the mxcli-banking app through
// three wrong guesses at the optimistic-locking property.
func unknownSettingsKey(section, key string) error {
	return mdlerrors.NewUnsupported(fmt.Sprintf("unknown %s setting: %s\n  valid keys: %s",
		settingsKeyNoun[section], key, strings.Join(settingsSectionKeys[section], ", ")))
}

// unknownSettingsKeys refuses every property of props the section does not have,
// in key order so the first refusal — the one exec returns — is deterministic.
func unknownSettingsKeys(section string, props map[string]any) []error {
	known := settingsSectionKeys[section]
	var errs []error
	for _, key := range sortedSettingsOptionKeys(props) {
		if !slices.Contains(known, key) {
			errs = append(errs, unknownSettingsKey(section, key))
		}
	}
	return errs
}

// settingsStatementErrors returns the refusals alterSettings gives a statement
// without looking at the project: a verb the section does not define, a section
// that does not exist, a key the section does not have. exec returns the first;
// check reports them all (MDL-SET04). Before this, check passed `alter settings
// <anything>` and only exec refused it, so a script could pass the gate and stop
// halfway through (mendixlabs/mxcli#1549).
//
// The options of LANGUAGE ADD/MODIFY and of a workflow GROUP are left to their
// handlers, which refuse an unknown one themselves.
func settingsStatementErrors(stmt *ast.AlterSettingsStmt) []error {
	section := strings.ToLower(stmt.Section)
	if stmt.AddGroup || stmt.ModifyGroup || stmt.UpsertGroup || stmt.RemoveGroup {
		if section != "workflows" {
			return []error{mdlerrors.NewUnsupported(fmt.Sprintf(
				"GROUP is only defined for the WORKFLOWS section, not %s", stmt.Section))}
		}
		return nil
	}
	if stmt.AddLanguage || stmt.ModifyLanguage || stmt.UpsertLanguage || stmt.RemoveLanguage {
		if section != "language" {
			return []error{mdlerrors.NewUnsupported(fmt.Sprintf(
				"add/remove is only defined for the LANGUAGE section, not %s", stmt.Section))}
		}
		return nil
	}
	if section == "constant" {
		return nil // a clause the grammar fixes, not a key list
	}
	if _, ok := settingsSectionKeys[section]; !ok {
		return []error{unknownSettingsSection(section)}
	}
	if errs := unknownSettingsKeys(section, stmt.Properties); len(errs) > 0 {
		return errs
	}
	return settingsListValueErrors(section, stmt.Properties)
}

// settingsListValueErrors refuses a list where a property takes one value, and
// one value where it takes a list. ThemeModuleOrder is the only list-valued
// settings property (ako/mxcli#1286); a module named twice in it is refused
// too, since the order would be ambiguous.
func settingsListValueErrors(section string, props map[string]any) []error {
	var errs []error
	for key, val := range props {
		list, isList := val.([]string)
		wantsList := section == "webui" && key == "ThemeModuleOrder"
		switch {
		case isList && !wantsList:
			errs = append(errs, mdlerrors.NewValidationf("%s takes one value, not a list", key))
		case !isList && wantsList:
			errs = append(errs, mdlerrors.NewValidationf(
				"ThemeModuleOrder takes a list of module names, as in ThemeModuleOrder: ('Atlas_Core', 'BrandTheme')"))
		case isList:
			seen := map[string]bool{}
			for _, n := range list {
				if seen[n] {
					errs = append(errs, mdlerrors.NewValidationf("ThemeModuleOrder names module %s twice", n))
				}
				seen[n] = true
			}
		}
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
	return errs
}

// settingsStatementViolations turns those refusals into check diagnostics. The
// message is exec's first line, so check and exec say the same thing; the valid
// keys, when the refusal lists them, become the suggestion.
func settingsStatementViolations(errs []error, what string) []linter.Violation {
	var out []linter.Violation
	for _, err := range errs {
		msg, rest, _ := strings.Cut(err.Error(), "\n")
		suggestion := strings.TrimSpace(rest)
		if suggestion == "" {
			suggestion = "See `mxcli syntax settings.alter` for the sections and their keys."
		}
		out = append(out, linter.Violation{
			RuleID:     "MDL-SET04",
			Severity:   linter.SeverityError,
			Location:   linter.Location{DocumentType: "settings", DocumentName: what},
			Message:    msg,
			Suggestion: suggestion,
		})
	}
	return out
}

// ValidateSettings reports what the executor would refuse in an ALTER SETTINGS
// statement before opening the project: an unknown section or key (MDL-SET04),
// and values that will not parse as their property's type (MDL-SET01..03, #805).
func ValidateSettings(stmt *ast.AlterSettingsStmt) []linter.Violation {
	section := strings.ToLower(stmt.Section)
	what := "alter settings " + section
	out := settingsStatementViolations(settingsStatementErrors(stmt), what)
	keys, ok := typedSettingsKeys[section]
	if !ok {
		return out
	}
	return append(out, validateTypedSettings(section, keys, stmt.Properties, what)...)
}

// ValidateCreateConfiguration reports the same for CREATE CONFIGURATION, which
// accepts the configuration properties directly.
func ValidateCreateConfiguration(stmt *ast.CreateConfigurationStmt) []linter.Violation {
	what := fmt.Sprintf("create configuration '%s'", stmt.Name)
	out := settingsStatementViolations(unknownSettingsKeys("configuration", stmt.Properties), what)
	return append(out, validateTypedSettings("configuration", typedSettingsKeys["configuration"], stmt.Properties, what)...)
}

func validateTypedSettings(section string, keys map[string]settingsValueKind, props map[string]any, what string) []linter.Violation {
	if len(keys) == 0 || len(props) == 0 {
		return nil
	}
	// Iterate the properties in a stable order: a map would make the diagnostics
	// order (and so the check output) non-deterministic.
	names := make([]string, 0, len(props))
	for key := range props {
		names = append(names, key)
	}
	sort.Strings(names)

	loc := linter.Location{DocumentType: "settings", DocumentName: what}
	var out []linter.Violation
	for _, key := range names {
		kind, ok := keys[key]
		if !ok {
			continue
		}
		valStr := settingsValueToString(props[key])
		switch kind {
		case settingsKindInt:
			if _, err := settingsInt(key, valStr); err != nil {
				out = append(out, linter.Violation{
					RuleID:     "MDL-SET01",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    fmt.Sprintf("%s: %s must be an integer, got %q", what, key, valStr),
					Suggestion: fmt.Sprintf("Use a whole number, quoted or not: `%s = 10` or `%s = '10'`.", key, key),
				})
			}
		case settingsKindBool:
			if _, err := settingsBool(key, valStr); err != nil {
				out = append(out, linter.Violation{
					RuleID:     "MDL-SET02",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    fmt.Sprintf("%s: %s must be true or false, got %q", what, key, valStr),
					Suggestion: fmt.Sprintf("Use `%s = true` or `%s = false`.", key, key),
				})
			}
		case settingsKindDatabaseType:
			if _, err := settingsDatabaseType(key, valStr); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-SET03",
					Severity: linter.SeverityError,
					Location: loc,
					Message: fmt.Sprintf("%s: %s must be a Mendix database type, got %q",
						what, key, valStr),
					Suggestion: fmt.Sprintf("Use one of: %s.", strings.Join(databaseTypes, ", ")),
				})
			}
		case settingsKindEnum:
			members := settingsEnumValues[section+"/"+key]
			if _, err := settingsEnum(key, valStr, members); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-SET03",
					Severity: linter.SeverityError,
					Location: loc,
					Message: fmt.Sprintf("%s: %s must be one of %s, got %q",
						what, key, strings.Join(members, ", "), valStr),
					Suggestion: fmt.Sprintf("Use one of: %s.", strings.Join(members, ", ")),
				})
			}
		}
	}
	return out
}
