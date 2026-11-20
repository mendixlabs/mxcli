// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2247 (mendixlabs/mxcli#1857): a menu item that calls the System
// module's built-in System.ShowHomePage — what describe navigation prints for
// the Home item of many starter apps — was "microflow not found" in check,
// because the System module is not stored in the project. Measured with
// `mx dump-mpr --module-names=System` on 11.12.6: it is the System module's
// only microflow. The control is a misspelt System microflow, still refused.
func TestNavigationTargets_SystemShowHomePageResolves(t *testing.T) {
	f := newNavFixture(t)
	script := `mdl 1;
create or modify navigation Responsive
{
  menu item 'Home' ( OnClick: call microflow System.ShowHomePage )
  menu item 'Typo' ( OnClick: call microflow System.ShowHomePages )
};
`
	got, _ := f.check(t, script)
	for _, line := range strings.Split(got, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), "microflow not found: System.ShowHomePage") {
			t.Errorf("System.ShowHomePage was refused:\n%s", got)
		}
	}
	if !strings.Contains(got, "microflow not found: System.ShowHomePages") {
		t.Errorf("control: a misspelt System microflow was not refused:\n%s", got)
	}
}

// ako/mxcli#2235 (mendixlabs/mxcli#1848): System.Images.Completed and the
// other built-in images were "image collection not found: System.Images",
// although mx check accepts them. The System module's two image collections
// resolve now; any image name inside them is left to mxbuild (the list is
// measured on one version only). A collection the System module does not
// have is still refused.
func TestImageRefErrors_SystemImageCollections(t *testing.T) {
	images, collections := map[string]bool{"M.Imgs.Logo": true}, map[string]bool{"M.Imgs": true}
	addSystemImages(images, collections)
	for _, ref := range []string{"System.Images.Completed", "System.Images.Close", "System.WorkflowImages.WorkflowUserTask", "System.Images.SomeLaterImage", "M.Imgs.Logo"} {
		if errs := imageRefErrors([]string{ref}, images, collections); len(errs) != 0 {
			t.Errorf("%s refused: %v", ref, errs)
		}
	}
	errs := imageRefErrors([]string{"System.Imagez.Completed"}, images, collections)
	if len(errs) != 1 || !strings.Contains(errs[0], "System.Images") {
		t.Errorf("control: an unknown System collection: %v, want one error naming System.Images", errs)
	}
}
