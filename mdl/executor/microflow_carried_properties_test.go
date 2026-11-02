// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Executor-level coverage for the microflow properties MDL cannot author and a
// rewrite therefore has to carry: the deep link, the export level, and the four
// concurrency settings. All were hardcoded in buildMicroflowFromStmt's rebuild
// struct or in microflowToGen, and all were lost with every checker green.
//
// Each has a paired control here — a CREATE must not acquire what only a stored
// microflow can supply — because "preserve the stored value" and "invent one"
// are the same edit seen from opposite sides.

// TestCreateOrModifyMicroflow_PreservesDeepLinkURL is the executor half of
// #1120: a statement that never mentions the deep link must not clear one.
//
// A microflow's URL (Mendix 10.6+) has no MDL spelling, so `CREATE OR MODIFY
// MICROFLOW` rebuilt it from the AST default — an empty string — and the deep
// link was gone. The class is the one ADR-0005 calls guard-don't-drop, but with
// nothing to guard against: unlike a queued call or an unwritable REST body,
// a microflow with no URL is a perfectly valid document, so `mxcli check`,
// `mx check` and mxbuild all report success before and after. Only Studio Pro
// shows the loss. Preserving is therefore the whole remedy; there is no
// refusal to fall back on.
func TestCreateOrModifyMicroflow_PreservesDeepLinkURL(t *testing.T) {
	const moduleID = model.ID("module-1")
	stored := []*microflows.Microflow{{
		BaseElement:         model.BaseElement{ID: "mf-item"},
		ContainerID:         moduleID,
		Name:                "ACT_Item",
		URL:                 "item/{Key}",
		URLSearchParameters: []string{"MyModule.ACT_Item.Filter"},
	}}
	ctx, written := microflowWriteProbe(t, stored, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "ACT_Item"},
		CreateOrModify: true,
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if got := (*written).URL; got != "item/{Key}" {
		t.Errorf("rewrite dropped the deep-link URL: got %q, want %q", got, "item/{Key}")
	}
	if got := (*written).URLSearchParameters; len(got) != 1 || got[0] != "MyModule.ACT_Item.Filter" {
		t.Errorf("rewrite dropped UrlSearchParameters: got %v, want [MyModule.ACT_Item.Filter]", got)
	}
}

// TestCreateMicroflow_InventsNoDeepLinkURL is the control for the test above: a
// microflow that never had a URL must not acquire one, or "preserve the stored
// value" would just be a different silent change in the same place.
func TestCreateMicroflow_InventsNoDeepLinkURL(t *testing.T) {
	const moduleID = model.ID("module-1")
	ctx, written := microflowWriteProbe(t, nil, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "ACT_Fresh"},
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if got := (*written).URL; got != "" {
		t.Errorf("a fresh microflow acquired a deep-link URL: %q", got)
	}
	if got := (*written).URLSearchParameters; len(got) != 0 {
		t.Errorf("a fresh microflow acquired UrlSearchParameters: %v", got)
	}
}

// TestCreateOrModifyMicroflow_PreservesExportLevel is the executor half of the
// export-level carry. Same shape as the deep link above and found the same way:
// a constant in the writer that nothing downstream would miss.
func TestCreateOrModifyMicroflow_PreservesExportLevel(t *testing.T) {
	const moduleID = model.ID("module-1")
	stored := []*microflows.Microflow{{
		BaseElement: model.BaseElement{ID: "mf-api"},
		ContainerID: moduleID,
		Name:        "ACT_PublicApi",
		ExportLevel: "API",
	}}
	ctx, written := microflowWriteProbe(t, stored, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "ACT_PublicApi"},
		CreateOrModify: true,
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if got := (*written).ExportLevel; got != "API" {
		t.Errorf("rewrite demoted the export level: got %q, want %q", got, "API")
	}
}

// TestDescribeMicroflow_EmitsAuthoredProperties is the round-trip half.
//
// This test used to assert the OPPOSITE: while the deep link and export level
// had no MDL spelling, DESCRIBE emitted them as `-- URL:` / `-- Export level:`
// comments so its output did not look complete when it was not. They are real
// clauses now, which is what makes describe -> rename -> exec a faithful copy
// rather than an approximate one — the hole preservation alone could not close,
// because a copy is a new document with nothing to preserve from.
//
// Still conditional: every document in every marketplace module measured stores
// Hidden and allows concurrent execution, so emitting the defaults would add
// three lines to every describe in order to say nothing.
func TestDescribeMicroflow_EmitsAuthoredProperties(t *testing.T) {
	ctx, _ := newMockCtx(t)
	name := ast.QualifiedName{Module: "MyModule", Name: "ACT_Item"}

	render := func(mf *microflows.Microflow) string {
		return renderMicroflowMDL(ctx, "microflow", mf, name, nil, nil, nil)
	}

	got := render(&microflows.Microflow{
		Name:                     "ACT_Item",
		URL:                      "item/{Key}",
		URLSearchParameters:      []string{"MyModule.ACT_Item.Filter"},
		ExportLevel:              "API",
		AllowConcurrentExecution: false,
		ConcurrencyErrorMessage:  &model.Text{Translations: map[string]string{"en_US": "Busy"}},
	})
	for _, want := range []string{
		"url 'item/{Key}'",
		"url search parameters ($Filter)",
		"export level api",
		"disallow concurrent execution error message 'Busy'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe omitted %q:\n%s", want, got)
		}
	}
	// The clause names the PARAMETER, not the stored qualified name: that is
	// what the reader has in front of them, and it is what re-executing needs.
	if strings.Contains(got, "MyModule.ACT_Item.Filter") {
		t.Errorf("search parameter emitted as a qualified name:\n%s", got)
	}

	// The control: an ordinary microflow gets none of these lines. Without it
	// the test would pass against a describer that emits them unconditionally.
	plain := render(&microflows.Microflow{
		Name: "ACT_Item", ExportLevel: "Hidden", AllowConcurrentExecution: true,
	})
	for _, unwanted := range []string{"url ", "export level", "concurrent execution"} {
		if strings.Contains(plain, unwanted) {
			t.Errorf("describe emitted %q for a default microflow:\n%s", unwanted, plain)
		}
	}
}

// TestDescribeMicroflow_FlagsUntranslatableMessage: a concurrency error message
// is a Texts$Text and MDL states ONE language, so a translated message cannot
// round-trip through the clause. DESCRIBE emits it anyway — omitting it would
// describe a microflow that fails CE4899 — and names the languages a replay
// would not carry, the same honesty rule it applies to a range bounded by
// another attribute.
//
// Note this is about a COPY. Rewriting the same microflow keeps every language,
// because canon.CarryTranslations pairs the texts and carries them (measured:
// restating the English text left the Dutch one untouched).
func TestDescribeMicroflow_FlagsUntranslatableMessage(t *testing.T) {
	ctx, _ := newMockCtx(t)
	got := renderMicroflowMDL(ctx, "microflow", &microflows.Microflow{
		Name:                     "ACT_Item",
		AllowConcurrentExecution: false,
		ConcurrencyErrorMessage: &model.Text{Translations: map[string]string{
			"en_US": "Busy", "nl_NL": "Bezet",
		}},
	}, ast.QualifiedName{Module: "MyModule", Name: "ACT_Item"}, nil, nil, nil)

	if !strings.Contains(got, "also translated into nl_NL") {
		t.Errorf("describe did not flag the language a replay would drop:\n%s", got)
	}
}

// TestDescribeMicroflow_MessageInDefaultLanguage: the concurrency message is
// described in the PROJECT's default language, as every other text is (#702).
// Picking the alphabetically first one printed de_DE on an en_US project, and a
// drop + create from that output wrote the German text under en_US, so the
// English message was gone with nothing reporting it (#1869).
//
// Both projects store the same two languages; only the default differs. The
// second case is the control: a describer that always prints en_US passes the
// first and fails it.
func TestDescribeMicroflow_MessageInDefaultLanguage(t *testing.T) {
	for _, tc := range []struct {
		defaultLang, want, other string
	}{
		{"en_US", "Another save is running.", "de_DE"},
		{"de_DE", "Ein anderes Speichern läuft.", "en_US"},
	} {
		t.Run(tc.defaultLang, func(t *testing.T) {
			got := renderMicroflowMDL(ctxWithDefaultLanguage(tc.defaultLang), "microflow", &microflows.Microflow{
				Name:                     "SaveOrder",
				AllowConcurrentExecution: false,
				ConcurrencyErrorMessage: &model.Text{Translations: map[string]string{
					"en_US": "Another save is running.",
					"de_DE": "Ein anderes Speichern läuft.",
				}},
			}, ast.QualifiedName{Module: "MyModule", Name: "SaveOrder"}, nil, nil, nil)

			if !strings.Contains(got, "error message '"+tc.want+"'") {
				t.Errorf("describe did not print the %s message %q:\n%s", tc.defaultLang, tc.want, got)
			}
			if !strings.Contains(got, "also translated into "+tc.other) {
				t.Errorf("describe did not flag %s as the other language:\n%s", tc.other, got)
			}
		})
	}
}
