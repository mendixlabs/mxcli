// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2464: a call that leaves out an entity-type parameter passed check
// and exec; mx check reports CE0115. Measured on 11.12.6, only that kind is
// CE0115 when left out — Integer, object, type-parameter object and Microflow
// are not, so those are not reported.
func TestMissingCodeActionArgs_Issue2464(t *testing.T) {
	ref := codeActionCallRef{name: "M.JEntityType"}
	errs := missingCodeActionArgs("java action", ref, []string{"et", "n"}, []string{"et"})
	if len(errs) != 1 || !strings.Contains(errs[0], "et") || !strings.Contains(errs[0], "CE0115") {
		t.Errorf("errs = %v, want one naming et and CE0115", errs)
	}
	// CONTROLS: a missing non-entity-type parameter, all passed, unknown
	// parameters, and a misspelled argument (reported elsewhere).
	if errs := missingCodeActionArgs("java action", ref, []string{"n"}, nil); len(errs) != 0 {
		t.Errorf("missing Integer: %v", errs)
	}
	ref.argNames = []string{"et"}
	if errs := missingCodeActionArgs("java action", ref, []string{"et"}, []string{"et"}); len(errs) != 0 {
		t.Errorf("all passed: %v", errs)
	}
	if errs := missingCodeActionArgs("java action", codeActionCallRef{name: "X.Y"}, nil, nil); len(errs) != 0 {
		t.Errorf("unknown parameters: %v", errs)
	}
	ref.argNames = []string{"ett"}
	if errs := missingCodeActionArgs("java action", ref, []string{"et"}, []string{"et"}); len(errs) != 0 {
		t.Errorf("misspelled: %v", errs)
	}
}
