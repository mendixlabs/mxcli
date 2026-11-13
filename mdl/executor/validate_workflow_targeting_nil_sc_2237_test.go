// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// exec validates a CREATE WORKFLOW with no script context (sc == nil). With
// App Settings > Workflows naming a specialization of System.User, a targeting
// constraint's bare member reached the project resolver, which read the nil
// script context and panicked; with System.User the System resolver answered
// first and hid it (ako/mxcli#2237, mendixlabs/mxcli#1851).
func TestWorkflowTargetingXPath_NoScriptContextWithConfiguredUserEntity(t *testing.T) {
	ctx := userEntityFixture(t)
	for _, xpath := range []string{"[Active = true()]", "[FullName = 'Jane']"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s: panicked without a script context: %v", xpath, r)
				}
			}()
			if errs := validateWorkflowTargetingXPath(ctx,
				[]ast.WorkflowActivityNode{targetingTask("xpath", xpath)}, nil); len(errs) != 0 {
				t.Errorf("%s names a member of the configured user entity, but was reported: %v", xpath, errs)
			}
		}()
	}
	// The check still works without a script context.
	if errs := validateWorkflowTargetingXPath(ctx,
		[]ast.WorkflowActivityNode{targetingTask("xpath", "[Accounts.Account/FullName = 'Jane']")}, nil); len(errs) != 1 {
		t.Errorf("a leading Accounts.Account step was not reported: %v", errs)
	}
}
