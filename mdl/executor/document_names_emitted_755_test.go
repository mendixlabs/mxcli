// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// R10 (ako/mxcli#755) renamed document types to Studio Pro's names and kept the
// old spellings as deprecated aliases. MDL that mxcli itself prints — generated
// statements and the statements its advice tells the user to run — must use the
// canonical names: advice in a deprecated spelling earns the user a warning for
// doing exactly what they were told.

// assertCanonicalMDL parses src and fails when it records any deprecation.
func assertCanonicalMDL(t *testing.T, src string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("mxcli printed MDL that does not parse: %q: %v", src, errs)
	}
	for _, d := range prog.Deprecations {
		t.Errorf("mxcli printed a deprecated spelling (%s) in %q", d.Code, src)
	}
}

// `contract entity … from odata` generates a create external entity statement.
func TestContractExternalEntityUsesStudioProName(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	et := &types.EdmEntityType{Name: "Customer"}
	doc := &types.EdmxDocument{EntitySets: []*types.EdmEntitySet{{Name: "Customers", EntityType: "NS.Customer"}}}
	if err := outputContractEntityMDL(ctx, et, "Crm.CrmService", doc); err != nil {
		t.Fatal(err)
	}
	assertCanonicalMDL(t, buf.String())
}

// A queued call to a missing queue tells the user how to create it.
func TestMissingQueueAdviceUsesStudioProName(t *testing.T) {
	mb := &mock.MockBackend{ListQueuesFunc: func() ([]*types.Queue, error) { return nil, nil }}
	fb := &flowBuilder{backend: mb}
	fb.buildQueueSettings(&ast.QualifiedName{Module: "Ops", Name: "Jobs"}, nil, "call microflow")
	if len(fb.errors) != 1 {
		t.Fatalf("errors = %v, want one", fb.errors)
	}
	m := regexp.MustCompile("`([^`]+)`").FindStringSubmatch(fb.errors[0])
	if m == nil {
		t.Fatalf("no statement quoted in %q", fb.errors[0])
	}
	assertCanonicalMDL(t, m[1]+";")
}

// MOVE reports and refuses by the kind read off the stored $Type, and its
// refusal names the statement to run instead.
func TestMoveNamesDocumentsAsStudioProDoes(t *testing.T) {
	mod := mkModule("Module")
	doc := &types.DocumentUnit{
		ID:          nextID("unit"),
		ContainerID: mod.ID,
		Name:        "Api",
		Type:        "Rest$ConsumedRestService",
		Kind:        types.DocumentKind("Rest$ConsumedRestService"),
	}
	var probe placementProbe
	mb, h := moveMockBackend(t, mod, doc, &probe)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeQueue,
		Name:         ast.QualifiedName{Module: "Module", Name: "Api"},
		Folder:       "Private",
	})
	assertError(t, err)
	if probe.moved {
		t.Fatal("a mistyped doctype still reparented the document")
	}
	assertContainsStr(t, err.Error(), "is a consumed rest service, not a task queue")
	m := regexp.MustCompile(`'(move [^']+) to \.\.\.'`).FindStringSubmatch(err.Error())
	if m == nil {
		t.Fatalf("no statement advised in %q", err.Error())
	}
	assertCanonicalMDL(t, m[1]+" to folder 'x';")

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeRestClient,
		Name:         ast.QualifiedName{Module: "Module", Name: "Api"},
		Folder:       "Private",
	}))
	if !strings.Contains(buf.String(), "Moved consumed rest service Module.Api") {
		t.Errorf("move reported %q, want the Studio Pro name", buf.String())
	}
}
