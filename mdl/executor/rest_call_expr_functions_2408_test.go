// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2408 (mendixlabs/mxcli#1896): MDL044 refused an unknown function in
// a declare, but the same call in a `call rest service` header, body or URL
// passed check and exec and failed the build with CE0117 at the activity — the
// REST call's expressions were not in checkStmtExprFunctions' list.
func TestRestCallExpressionsAreCheckedForUnknownFunctions_Issue2408(t *testing.T) {
	vs := violationsForMicroflowSource(t, `create microflow Rh.MF () returns String as $Out
begin
  $Resp = call rest service post 'https://example.invalid/' + noSuchUrlFn('p') (
    Headers: ('Authorization': 'Basic ' + base64Encode('user:pass')),
    Body: noSuchBodyFn('payload'),
    Timeout: noSuchTimeoutFn(5),
  ) returns response on error without rollback begin
    return 'error';
  end error;
  return $Resp/Content;
end;`)
	var msgs []string
	for _, v := range vs {
		if v.RuleID == "MDL044" {
			msgs = append(msgs, v.Message)
		}
	}
	all := strings.Join(msgs, "\n")
	for _, fn := range []string{"noSuchUrlFn", "base64Encode", "noSuchBodyFn", "noSuchTimeoutFn"} {
		if !strings.Contains(all, fn) {
			t.Errorf("MDL044 does not name %s; got:\n%s", fn, all)
		}
	}
}

// CONTROL: real expression functions in the same places pass.
func TestRestCallExpressionsWithKnownFunctionsPass_Issue2408(t *testing.T) {
	vs := violationsForMicroflowSource(t, `create microflow Rh.MF () returns String as $Out
begin
  $Resp = call rest service get 'https://example.invalid/' + toLowerCase('P') (
    Headers: ('X-Id': toString(5)),
    Timeout: 5,
  ) returns response on error without rollback begin
    return 'error';
  end error;
  return $Resp/Content;
end;`)
	for _, v := range vs {
		if v.RuleID == "MDL044" {
			t.Errorf("unexpected MDL044: %s", v.Message)
		}
	}
}
