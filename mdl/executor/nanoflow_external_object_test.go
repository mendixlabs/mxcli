// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1735 (ako/mxcli#1939): measured on mxbuild 11.12.6, both
// external-object activities in a nanoflow are CE0009 "This action is not
// supported in nanoflows." — check passed them.
func TestNanoflowRefusesExternalObjectActivities(t *testing.T) {
	for _, del := range []bool{false, true} {
		body := []ast.MicroflowStatement{&ast.ExternalObjectStmt{Delete: del, Variable: "Customer"}}
		errs := validateNanoflowBody(body)
		if len(errs) != 1 || !strings.Contains(errs[0], "external object") {
			t.Errorf("delete=%v: want one refusal naming the external object activity, got %v", del, errs)
		}
	}
}
