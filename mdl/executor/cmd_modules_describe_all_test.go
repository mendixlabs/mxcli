// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// languageHeader matches the `mdl N;` line describe prints ahead of its MDL.
var languageHeader = regexp.MustCompile(`(?m)^mdl \d+;\n`)

// catalogProgress matches the lines bare `describe M.N` prints while it loads
// the catalog it resolves the type through, ahead of the MDL.
var catalogProgress = regexp.MustCompile(`\A(?:(?:Loading cached catalog|Building catalog|✓ )[^\n]*\n)+`)

// `describe module M with all` must account for every document in the module:
// each one is either in the output — exactly as its own `describe` prints it —
// (commented out, for a read-only report such as a building block) or named
// on a `-- not describable:` line. A hand-kept list of document types
// is what this replaced, and it emitted 0 of FeedbackModule's 11 nanoflows (and
// none of its JavaScript actions, JSON structures, mappings, image collections
// or its building block) with no error (mendixlabs/mxcli#1421).
//
// The documents are enumerated type-blind (ListDocumentUnits, the way
// project-tree counts a module), so a document type nobody wired into the
// registry fails here rather than going missing.
func TestDescribeModuleWithAll_AccountsForEveryDocument(t *testing.T) {
	for _, tc := range []struct {
		fixture, mpr, module string
		// parses is false where a document's own describe does not parse;
		// the module is then still checked for completeness. FeedbackModule
		// needed it until JavaScript actions with type parameters described
		// as parseable MDL (#1170).
		parses bool
	}{
		{"pedapp", "PedApp.mpr", "FeedbackModule", true},
		{"pedapp", "PedApp.mpr", "Atlas_Core", true},        // icon collections: read-only reports
		{"pedapp", "PedApp.mpr", "Atlas_Web_Content", true}, // building blocks, page templates
		{"pedapp", "PedApp.mpr", "Administration", true},
		{"testapp-views", "TestApp.mpr", "MyFirstModule", true}, // view entity, published OData
	} {
		module := tc.module
		t.Run(tc.fixture+"/"+module, func(t *testing.T) {
			exec, out := openFixtureCopy(t, tc.fixture, tc.mpr)
			out.Reset()
			if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeModule, Name: ast.QualifiedName{Module: module}, WithAll: true}); err != nil {
				t.Fatalf("describe module %s with all: %v\n%s", module, err, out.String())
			}
			all := languageHeader.ReplaceAllString(out.String(), "")

			// The whole output must be MDL a script can hold: read-only reports
			// and failures are comments.
			if _, errs := visitor.Build(all); tc.parses && len(errs) > 0 {
				t.Fatalf("describe module %s with all does not parse: %v", module, errs[0])
			}

			ctx := exec.newExecContext(context.Background())
			units, err := ctx.Backend.ListDocumentUnits()
			if err != nil {
				t.Fatal(err)
			}
			mods, _ := ctx.Backend.ListModules()
			var modID model.ID
			for _, m := range mods {
				if m.Name == module {
					modID = m.ID
				}
			}
			containers := getModuleContainers(ctx, modID)

			total, described := 0, 0
			var missing []string
			for _, u := range units {
				if !containers[u.ContainerID] || describedWithOwner[u.Type] {
					continue
				}
				total++
				qn := module + "." + u.Name
				out.Reset()
				ownErr := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeAuto, Name: ast.QualifiedName{Module: module, Name: u.Name}})
				if strings.Contains(all, "-- not describable: "+u.Kind+" "+qn+"\n") {
					// Only honest when describe really cannot answer for it.
					if ownErr == nil {
						missing = append(missing, u.Type+" "+qn+" (dumped as not describable, but its own describe works)")
					}
					continue
				}
				if ownErr != nil {
					missing = append(missing, u.Type+" "+qn+" (and its own describe fails: "+ownErr.Error()+")")
					continue
				}
				one := catalogProgress.ReplaceAllString(out.String(), "")
				one = strings.TrimSpace(languageHeader.ReplaceAllString(one, ""))
				if one == "" || !(strings.Contains(all, one) || strings.Contains(all, commentOutReport(one))) {
					missing = append(missing, u.Type+" "+qn)
					continue
				}
				described++
			}
			if total == 0 {
				t.Fatalf("module %s has no documents in the fixture; the test proves nothing", module)
			}
			if len(missing) > 0 {
				t.Errorf("describe module %s with all accounts for %d of %d documents; missing %d:\n  %s",
					module, total-len(missing), total, len(missing), strings.Join(missing, "\n  "))
			}
			t.Logf("%s: %d documents, %d described", module, total, described)
		})
	}
}

// openFixtureCopy opens a private copy of a testdata project fixture.
func openFixtureCopy(t *testing.T, fixture, mpr string) (*Executor, *bytes.Buffer) {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", fixture)
	if _, err := os.Stat(filepath.Join(src, mpr)); err != nil {
		t.Skipf("fixture %s not found: %v", fixture, err)
	}
	dir := t.TempDir()
	if err := copyPedAppFile(filepath.Join(src, mpr), filepath.Join(dir, mpr)); err != nil {
		t.Fatal(err)
	}
	if err := copyPedAppTree(filepath.Join(src, "mprcontents"), filepath.Join(dir, "mprcontents")); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	exec := New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: filepath.Join(dir, mpr)}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	return exec, out
}

// Every document kind bare `describe M.N` can resolve is one the module dump
// emits — the two tables are what drifted apart in mendixlabs/mxcli#1421 (the
// dump knew 13 document types, describe 30-odd). The domain model's members
// are emitted from the domain model, not the registry.
func TestModuleDocKindsCoverDescribableTypes(t *testing.T) {
	notDocuments := map[ast.DescribeObjectType]bool{
		ast.DescribeModule:         true, // the statement itself
		ast.DescribeEntity:         true, // domain-model members,
		ast.DescribeExternalEntity: true, // emitted by entity,
		ast.DescribeAssociation:    true, // and association
	}
	inRegistry := map[ast.DescribeObjectType]int{}
	for _, k := range append(append([]moduleDocKind{}, moduleDocKindsBeforeDomainModel...), moduleDocKindsAfterDomainModel...) {
		inRegistry[k.kind]++
	}
	for objectType, kind := range objectTypeToDescribeKind {
		if notDocuments[kind] {
			continue
		}
		if inRegistry[kind] == 0 {
			t.Errorf("%s (%s) is describable but describe module ... with all never emits it; add it to the module document registry",
				objectType, describeObjectTypeLabel(kind))
		}
	}
	for kind, n := range inRegistry {
		if n > 1 {
			t.Errorf("%s is listed %d times in the module document registry", describeObjectTypeLabel(kind), n)
		}
	}
}

// A document that fails to describe is reported where it would have been, as
// a comment, and fails the statement once the rest of the module is written —
// it used to be skipped by an `if err == nil` with exit 0 (mendixlabs/mxcli#1421).
// The enumeration that describes fine is the control.
func TestDescribeModuleWithAll_ReportsDescribeFailure(t *testing.T) {
	mod := mkModule("Sales")
	enum := mkEnumeration(mod.ID, "Status", "Open", "Closed")
	for _, broken := range []bool{false, true} {
		mb := &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
			GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) {
				return &domainmodel.DomainModel{ContainerID: mod.ID}, nil
			},
			GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) { return nil, nil },
			ListEnumerationsFunc:  func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
			ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
				if broken {
					return nil, errors.New("unit 42 does not decode")
				}
				return nil, nil
			},
			// The mock refuses a list it was not given; these return nothing.
			ListMessageDefinitionCollectionsFunc:    func() ([]*model.MessageDefinitionCollection, error) { return nil, nil },
			ListRulesFunc:                           func() ([]*microflows.Rule, error) { return nil, nil },
			ListNanoflowsFunc:                       func() ([]*microflows.Nanoflow, error) { return nil, nil },
			ListMenuDocumentsFunc:                   func() ([]*types.MenuDocument, error) { return nil, nil },
			ListAgentEditorModelsFunc:               func() ([]*agenteditor.Model, error) { return nil, nil },
			ListAgentEditorKnowledgeBasesFunc:       func() ([]*agenteditor.KnowledgeBase, error) { return nil, nil },
			ListAgentEditorConsumedMCPServicesFunc:  func() ([]*agenteditor.ConsumedMCPService, error) { return nil, nil },
			ListAgentEditorPublishedMCPServicesFunc: func() ([]*agenteditor.PublishedMCPService, error) { return nil, nil },
			ListAgentEditorAgentsFunc:               func() ([]*agenteditor.Agent, error) { return nil, nil },
		}
		ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
		err := describeModule(ctx, "Sales", true)
		got := buf.String()
		if !strings.Contains(got, "create or modify enumeration Sales.Status") {
			t.Fatalf("broken=%v: the enumeration is missing:\n%s", broken, got)
		}
		failed := strings.Contains(got, "-- describe failed: listing microflow documents: unit 42 does not decode")
		if broken != failed {
			t.Errorf("broken=%v: describe-failed line present = %v:\n%s", broken, failed, got)
		}
		if broken != (err != nil) {
			t.Errorf("broken=%v: describe module returned %v\n%s", broken, err, got)
		}
		if _, errs := visitor.Build(got); len(errs) > 0 {
			t.Errorf("broken=%v: output does not parse: %v\n%s", broken, errs[0], got)
		}
	}
}
