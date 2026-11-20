// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2113: documentation holding `*/` — a CSS comment in a page's notes,
// on FactoryManagement's UserCommons.Claim_NewEdit — was described inside a
// `/** … */` doc comment, which the `*/` ended early: "Parse error: line 5:1 no
// viable alternative at input". Describe now writes it `*\/`, and the visitor
// reads that back as `*/`.
const commentEndDoc = ".custom-model-popup {\n    max-height: 400px; /* Adjust the value to your preference */\n}"

func TestDescribeEntity_DocHoldingCommentEndRoundTrips_Issue2113(t *testing.T) {
	for _, doc := range []string{commentEndDoc, "one line with */ in it", `already *\/ escaped`} {
		text, entityDoc, attrDoc := describeParsedDocs(t, doc)
		// The text a doc comment can carry: indentation has no spelling (#731).
		want := docCommentNormalForm(doc)
		if entityDoc != want {
			t.Errorf("entity documentation %q came back as %q\n%s", want, entityDoc, text)
		}
		if attrDoc != want {
			t.Errorf("attribute documentation %q came back as %q\n%s", want, attrDoc, text)
		}
	}
}

// A microflow, one of the describe paths that wrote the comment by hand, run
// back through the parser.
func TestDescribeMicroflow_DocHoldingCommentEndRoundTrips_Issue2113(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "ACT_Doc")
	mf.Documentation = commentEndDoc
	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeMicroflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "ACT_Doc"}))
	text := buf.String()
	prog, errs := visitor.Build(text)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, text)
	}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			if want := docCommentNormalForm(commentEndDoc); s.Documentation != want {
				t.Errorf("documentation came back as %q, want %q\n%s", s.Documentation, want, text)
			}
			return
		}
	}
	t.Fatalf("no create microflow in describe output:\n%s", text)
}
