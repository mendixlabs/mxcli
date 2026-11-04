// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// On Mendix 11.14.0 an attach's build only ever gets a cold build's answer,
// which asks for a restart whatever changed. The refusal must say that, not
// claim an entity changed, and must not offer restarting the dev loop, which
// leaves the next attach exactly where this one is.
func TestAttachRestartRefusal_ColdAnswerBlamesNoEntity(t *testing.T) {
	msg := attachRestartRefusal(&docker.BuildResult{Status: "Success", RestartRequired: true, ColdRestart: true}).Error()
	if strings.Contains(msg, "entity") || strings.Contains(msg, "Restart that process") {
		t.Errorf("a cold build's restart answer is not evidence of an entity change:\n%s", msg)
	}
	if !strings.Contains(msg, "could not build this change incrementally") || !strings.Contains(msg, "Drop --attach") {
		t.Errorf("want why the answer is unknown and the way out:\n%s", msg)
	}
}

// Control: an incremental build that asks for a restart did see a structural change.
func TestAttachRestartRefusal_IncrementalAnswerNamesTheChange(t *testing.T) {
	msg := attachRestartRefusal(&docker.BuildResult{Status: "Success", RestartRequired: true}).Error()
	if !strings.Contains(msg, "an entity or association changed") {
		t.Errorf("want the structural-change refusal:\n%s", msg)
	}
}
