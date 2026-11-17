// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"strings"
	"testing"
)

// ako/mxcli#2313: `run restart` refused a run started with --constant
// X=@env:VAR and printed only the generic hint ("--db-password @env:…"). The
// @env: references themselves are stored and replayed; what restart refuses is
// a value stored redacted, and the refusal did not say which one — so a run
// with one literal flag among several references read as "references do not
// work". It now names each.
func TestRedactedArgNames_NamesEachLiteralSecret_Issue2313(t *testing.T) {
	stored, _ := redactArgs([]string{"run", "--local",
		"--constant", "Encryption.EncryptionKey=@env:HELPDESK_ENC_KEY",
		"--constant", "Helpdesk.OpenRouterKey=sk-literal",
		"--db-password", "hunter2",
		"--runtime-setting=DatabasePassword=x",
		"-p", "/abs/App.mpr"})
	got := redactedArgNames(stored)
	want := []string{"--constant Helpdesk.OpenRouterKey", "--db-password", "--runtime-setting DatabasePassword"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("redactedArgNames = %q, want %q", got, want)
	}
	msg := restartSecretRefusal("/abs/App.mpr", got)
	for _, w := range want {
		if !strings.Contains(msg, w) {
			t.Errorf("refusal does not name %s:\n%s", w, msg)
		}
	}
	if strings.Contains(msg, "EncryptionKey") {
		t.Errorf("refusal names a constant that is a reference, which restart replays:\n%s", msg)
	}
}

// CONTROL: a run whose secrets are all references is restartable — nothing is
// redacted, so nothing is named and restart is not refused.
func TestRedactedArgNames_ReferencesOnly_Issue2313(t *testing.T) {
	stored, changed := redactArgs([]string{"run", "--local",
		"--constant", "Encryption.EncryptionKey=@env:HELPDESK_ENC_KEY",
		"--constant=Helpdesk.OpenRouterKey=@file:/run/secrets/key",
		"--db-password", "@env:MX_DB_PASSWORD"})
	if changed || hasRedactedArgs(stored) || len(redactedArgNames(stored)) != 0 {
		t.Errorf("references were redacted: %q", stored)
	}
}
