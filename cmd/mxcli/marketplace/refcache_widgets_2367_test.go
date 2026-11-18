// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"path/filepath"
	"testing"
)

// ako/mxcli#2367 (mendixlabs/mxcli#1882): a cached reference held the model
// but not widgets/, so DESCRIBE rendered its pluggable widgets in the generic
// form (`pluggablewidget 'com.mendix.widget.web.timeline.Timeline' …`, with the
// definition's defaults) while the app, which has the definitions, rendered
// `timeline …` — and `marketplace diff` reported every such snippet of an
// untouched module as locally changed.
func TestIsModelFile_KeepsWidgetDefinitions_Issue2367(t *testing.T) {
	for _, rel := range []string{
		filepath.Join("widgets", "com.mendix.widget.web.Timeline.mpk"),
		"widgets",
	} {
		if !isModelFile(rel) {
			t.Errorf("isModelFile(%q) = false: DESCRIBE needs the widget definitions", rel)
		}
	}
	// CONTROL: the rest of the project stays out of the cache.
	for _, rel := range []string{filepath.Join("theme", "web", "main.scss"), filepath.Join("deployment", "x")} {
		if isModelFile(rel) {
			t.Errorf("isModelFile(%q) = true", rel)
		}
	}
	// An entry written without widgets/ must not be served again.
	if refEntryFormat == "f2" {
		t.Error("refEntryFormat still f2: entries cached without widgets/ would keep being served")
	}
}
