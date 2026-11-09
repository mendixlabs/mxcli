// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1453 (mendixlabs/mxcli#1556): porting ~90 company guideline rules
// to Starlark, rules could not be written although the catalog or the project
// settings hold the data — rules reported excluded flows, could not resolve an
// enumeration value's caption (#1301), and could not read run-configuration
// constant values or the project settings.

func runDump(t *testing.T, ctx *linter.LintContext, body string) []string {
	t.Helper()
	vs, _ := runSrc(t, ctx, "def check():\n"+body)
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	return got
}

func TestStarlarkMicroflowCarriesExcludedAndApplyEntityAccess(t *testing.T) {
	cat := emptyCatalog(t)
	for _, stmt := range []string{
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source) VALUES ('m1', 'Shop', 'Shop', 'Shop', '')`,
		`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, Excluded, ApplyEntityAccess) VALUES
			('f1', 'ACT_Live', 'Shop.ACT_Live', 'Shop', 'MICROFLOW', 0, 1),
			('f2', 'ACT_Old', 'Shop.ACT_Old', 'Shop', 'MICROFLOW', 1, 0)`,
	} {
		if _, err := cat.CatalogDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	got := runDump(t, linter.NewLintContext(cat, nil),
		`    return [violation(message = "%s|%s|%s" % (m.name, m.excluded, m.apply_entity_access)) for m in microflows()]`)
	want := "ACT_Live|False|True,ACT_Old|True|False"
	if strings.Join(got, ",") != want {
		t.Errorf("microflows() = %v, want %s", got, want)
	}
}

func TestStarlarkEnumerationCarriesItsValuesAndCaptions(t *testing.T) {
	cat := emptyCatalog(t)
	for _, stmt := range []string{
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source) VALUES ('m1', 'Shop', 'Shop', 'Shop', '')`,
		`INSERT INTO enumerations_data (Id, Name, QualifiedName, ModuleName, ValueCount) VALUES
			('e1', 'ApiTimeout', 'Shop.ApiTimeout', 'Shop', 2), ('e2', 'Empty', 'Shop.Empty', 'Shop', 0)`,
		`INSERT INTO enumeration_values_data (Id, EnumerationId, EnumerationQualifiedName, ModuleName, Name, Caption, Ordinal) VALUES
			('v2', 'e1', 'Shop.ApiTimeout', 'Shop', 'Slow', '400', 1),
			('v1', 'e1', 'Shop.ApiTimeout', 'Shop', 'Quick', '30', 0)`,
	} {
		if _, err := cat.CatalogDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	got := runDump(t, linter.NewLintContext(cat, nil), `    out = []
    for e in enumerations():
        out.append(violation(message = "%s:%s" % (e.name, ",".join(["%s=%s@%d" % (v.qualified_name, v.caption, v.ordinal) for v in e.values]))))
    return out`)
	// In ordinal order, not insertion order; an enumeration without values has [].
	want := "ApiTimeout:Shop.ApiTimeout.Quick=30@0,Shop.ApiTimeout.Slow=400@1|Empty:"
	if strings.Join(got, "|") != want {
		t.Errorf("enumerations() values = %v, want %s", got, want)
	}
}

// ako/mxcli#1301, the half #1453 left: values() carries the caption in the
// default language only. A caption in another language is a strings() row whose
// element_id is the value's $ID, and a rule had no id on the value to match it
// with. Measured on CitrusGrove, all 144 enumeration caption rows match a value
// by id. The control row has the same text under another value's id, so a match
// on anything but the id picks the wrong one.
func TestStarlarkEnumerationValueIdMatchesItsTranslations(t *testing.T) {
	cat := emptyCatalog(t)
	for _, stmt := range []string{
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source) VALUES ('m1', 'Shop', 'Shop', 'Shop', '')`,
		`INSERT INTO enumerations_data (Id, Name, QualifiedName, ModuleName, ValueCount) VALUES
			('e1', 'ApiTimeout', 'Shop.ApiTimeout', 'Shop', 2)`,
		`INSERT INTO enumeration_values_data (Id, EnumerationId, EnumerationQualifiedName, ModuleName, Name, Caption, Ordinal) VALUES
			('v1', 'e1', 'Shop.ApiTimeout', 'Shop', 'Quick', '30', 0),
			('v2', 'e1', 'Shop.ApiTimeout', 'Shop', 'Slow', '400', 1)`,
		`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName) VALUES
			('Shop.ApiTimeout', 'ENUMERATION', '30', 'Enumerations$EnumerationValue.Caption', 'en_US', 'v1', 'Shop'),
			('Shop.ApiTimeout', 'ENUMERATION', '400', 'Enumerations$EnumerationValue.Caption', 'en_US', 'v2', 'Shop'),
			('Shop.ApiTimeout', 'ENUMERATION', 'traag', 'Enumerations$EnumerationValue.Caption', 'nl_NL', 'v2', 'Shop'),
			('Shop.ApiTimeout', 'ENUMERATION', 'snel', 'Enumerations$EnumerationValue.Caption', 'nl_NL', 'v1', 'Shop')`,
	} {
		if _, err := cat.CatalogDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	got := runDump(t, linter.NewLintContext(cat, nil), `    nl = {}
    for s in strings(language = "nl_NL"):
        if s.context == "Enumerations$EnumerationValue.Caption":
            nl[s.element_id] = s.value
    out = []
    for e in enumerations():
        for v in e.values:
            out.append(violation(message = "%s id=%s nl=%s" % (v.qualified_name, v.id, nl.get(v.id, "?"))))
    return out`)
	want := "Shop.ApiTimeout.Quick id=v1 nl=snel|Shop.ApiTimeout.Slow id=v2 nl=traag"
	if strings.Join(got, "|") != want {
		t.Errorf("value ids = %v, want %s", got, want)
	}
}

func TestStarlarkConstantValuesAndProjectSettings(t *testing.T) {
	cat := emptyCatalog(t)
	if _, err := cat.CatalogDB().Exec(`INSERT INTO constant_values (ConstantName, ConfigurationName, Value) VALUES
		('Shop.ApiUrl', 'Default', 'https://test'), ('Shop.ApiUrl', 'Prod', 'https://prod')`); err != nil {
		t.Fatal(err)
	}
	reader := &settingsReader{ps: &model.ProjectSettings{
		Model: &model.ModelSettings{UseOQLVersion2: true, JavaVersion: "Java21", HashAlgorithm: "BCrypt",
			DefaultTimeZoneCode: "Europe/Amsterdam", RoundingMode: "HalfUp", DecimalScale: 8,
			EnableDataStorageOptimisticLocking: true, UseDatabaseForeignKeyConstraints: true},
		Language: &model.LanguageSettings{DefaultLanguageCode: "nl_NL"},
		WebUI:    &model.WebUISettings{UseOptimizedClient: "Yes"},
	}}
	ctx := linter.NewLintContext(cat, reader)

	got := runDump(t, ctx,
		`    return [violation(message = "%s|%s|%s" % (c.constant, c.configuration, c.value)) for c in constant_values()]`)
	if want := "Shop.ApiUrl|Default|https://test,Shop.ApiUrl|Prod|https://prod"; strings.Join(got, ",") != want {
		t.Errorf("constant_values() = %v, want %s", got, want)
	}

	got = runDump(t, ctx, `    s = project_settings()
    return [violation(message = "%s|%s|%s|%s|%s|%s|%s|%s|%s|%s" % (s.default_language, s.use_oql_version_2,
        s.java_version, s.hash_algorithm, s.default_time_zone, s.rounding_mode, s.decimal_scale,
        s.optimistic_locking, s.foreign_key_constraints, s.web_client))]`)
	if want := "nl_NL|True|Java21|BCrypt|Europe/Amsterdam|HalfUp|8|True|True|Yes"; strings.Join(got, ",") != want {
		t.Errorf("project_settings() = %v, want %s", got, want)
	}

	// No reader (a rule run without a project): None, so a rule can test for it.
	got = runDump(t, linter.NewLintContext(cat, nil),
		`    return [violation(message = "none")] if project_settings() == None else []`)
	if strings.Join(got, ",") != "none" {
		t.Errorf("project_settings() without a reader = %v, want None", got)
	}
}
