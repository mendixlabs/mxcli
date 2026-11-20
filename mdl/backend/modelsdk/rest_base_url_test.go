// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genRest "github.com/mendixlabs/mxcli/modelsdk/gen/rest"
)

// A consumed REST service reads its base URL from a literal or from a
// constant. Only the literal was read, so a service configured per
// environment showed no base URL (ako/mxcli#2426).
func TestRestBaseURLReadsTheConstant(t *testing.T) {
	c := genRest.NewConstantValue()
	c.SetValueQualifiedName("Integration.ApiBaseUrl")
	if lit, con := restBaseURL(c); lit != "" || con != "Integration.ApiBaseUrl" {
		t.Errorf("constant base URL: literal %q, constant %q", lit, con)
	}
	// Control: a literal stays a literal.
	v := genRest.NewValueTemplate()
	v.SetValue("https://api.example.com")
	if lit, con := restBaseURL(v); lit != "https://api.example.com" || con != "" {
		t.Errorf("literal base URL: literal %q, constant %q", lit, con)
	}
	if lit, con := restBaseURL(nil); lit != "" || con != "" {
		t.Errorf("no base URL: literal %q, constant %q", lit, con)
	}
}
