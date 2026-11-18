// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// ako/mxcli#2366 (mendixlabs/mxcli#1881): with Studio Pro's lock file beside
// the project, `marketplace update` started anyway, had each of ~387 writes
// refused one by one, and ended with "The project may be mid-update …
// Restore from version control" although nothing had changed. The lock is
// checked once, before anything is read or written.
func TestPerformUpdate_RefusesUpFrontWhenStudioProHasTheProjectOpen(t *testing.T) {
	t.Setenv(mpr.AllowStudioProOpenEnv, "")
	target := copyFixture(t)
	if err := os.WriteFile(target+".lock", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := projectDigest(t, target)

	_, err := PerformUpdate(target, copyFixture(t), emptyMpk(t), "Administration",
		"4.3.2", "4.3.3", "00000000-0000-0000-0000-000000000000", testBackend)
	if err == nil {
		t.Fatal("PerformUpdate ran with Studio Pro's lock file present")
	}
	var open *mpr.StudioProOpenError
	if !errors.As(err, &open) {
		t.Errorf("error is not the Studio Pro refusal: %v", err)
	}
	if !errors.Is(err, ErrUpdateNotStarted) {
		t.Errorf("error does not say nothing was changed, so the caller says 'mid-update': %v", err)
	}
	if strings.Count(err.Error(), "Studio Pro has this project open") != 1 {
		t.Errorf("refusal should be one message, got: %v", err)
	}
	if after := projectDigest(t, target); after != before {
		t.Fatal("the project changed")
	}
}
