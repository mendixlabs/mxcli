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
	errs := missingCodeActionArgs("java action", ref, []string{"et", "n"}, []string{"et"}, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "et") || !strings.Contains(errs[0], "CE0115") {
		t.Errorf("errs = %v, want one naming et and CE0115", errs)
	}
	// CONTROLS: a missing non-entity-type parameter, all passed, unknown
	// parameters, and a misspelled argument (reported elsewhere).
	if errs := missingCodeActionArgs("java action", ref, []string{"n"}, nil, nil); len(errs) != 0 {
		t.Errorf("missing Integer: %v", errs)
	}
	ref.argNames = []string{"et"}
	if errs := missingCodeActionArgs("java action", ref, []string{"et"}, []string{"et"}, nil); len(errs) != 0 {
		t.Errorf("all passed: %v", errs)
	}
	if errs := missingCodeActionArgs("java action", codeActionCallRef{name: "X.Y"}, nil, nil, nil); len(errs) != 0 {
		t.Errorf("unknown parameters: %v", errs)
	}
	ref.argNames = []string{"ett"}
	if errs := missingCodeActionArgs("java action", ref, []string{"et"}, []string{"et"}, nil); len(errs) != 0 {
		t.Errorf("misspelled: %v", errs)
	}
}

// ako/mxcli#2577: a required (`not null`) parameter left out is CE0115 too —
// measured on 11.14.0 with CommunityCommons.getGUID (item: TypeParameter not
// null), an Integer not null and a type parameter not null. The same two left
// out while nullable build clean, which is what #2464 had measured.
func TestMissingCodeActionArgs_Required2577(t *testing.T) {
	ref := codeActionCallRef{name: "CommunityCommons.getGUID"}
	errs := missingCodeActionArgs("java action", ref, []string{"item"}, nil, []string{"item"})
	if len(errs) != 1 || !strings.Contains(errs[0], "required parameter(s) item") || !strings.Contains(errs[0], "CE0115") {
		t.Errorf("errs = %v, want one naming the required parameter item and CE0115", errs)
	}
	// A nullable parameter left out is fine.
	if errs := missingCodeActionArgs("java action", ref, []string{"item"}, nil, nil); len(errs) != 0 {
		t.Errorf("nullable left out: %v", errs)
	}
	// Passed: nothing.
	ref.argNames = []string{"item"}
	if errs := missingCodeActionArgs("java action", ref, []string{"item"}, nil, []string{"item"}); len(errs) != 0 {
		t.Errorf("passed: %v", errs)
	}
}
