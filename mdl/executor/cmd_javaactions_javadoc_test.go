// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// mendixlabs/mxcli#1653: the .java file is written with the action's
// documentation, so the class javadoc Studio Pro and mxbuild generate from it
// is not removed by every `create or modify java action`. The documentation
// handed to WriteJavaSourceFile is the one stored on the action: the stated
// doc comment, or the stored one when the statement has none.
func TestCreateJavaAction_WritesTheSourceWithItsDocumentation(t *testing.T) {
	cases := []struct {
		name, stored, stated string
		set                  bool
		want                 string
	}{
		{"stated doc comment", "Old text.", "Formats an amount.", true, "Formats an amount."},
		{"no doc comment keeps the stored documentation", "Stored text.", "", false, "Stored text."},
		{"an empty doc comment clears it", "Stored text.", "", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mod := mkModule("MyModule")
			stored := storedExposedAction(mod)
			ctx, _ := exposeRewriteCtx(t, stored, mod)
			mb := ctx.Backend.(*mock.MockBackend)
			// The stored documentation is read from the action list.
			mb.ListJavaActionsFunc = func() ([]*types.JavaAction, error) {
				return []*types.JavaAction{{BaseElement: model.BaseElement{ID: stored.ID},
					ContainerID: stored.ContainerID, Name: stored.Name, Documentation: c.stored}}, nil
			}
			var wrote *string
			mb.WriteJavaSourceFileFunc = func(_, _, _ string, _ []*javaactions.JavaActionParameter,
				_ javaactions.CodeActionReturnType, _ []string, _ string, documentation string) error {
				wrote = &documentation
				return nil
			}

			s := rewriteStmt(true)
			s.Documentation, s.DocumentationSet = c.stated, c.set
			assertNoError(t, execCreateJavaAction(ctx, s))

			if wrote == nil {
				t.Fatal("WriteJavaSourceFile was never called")
			}
			if *wrote != c.want {
				t.Errorf("documentation written to the .java file = %q, want %q", *wrote, c.want)
			}
		})
	}
}
