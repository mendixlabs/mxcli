// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"testing"
	"time"
)

// TestServeIntegration exercises the real StartServe -> Build -> Stop path
// against an actual `mxbuild --serve`. Skipped unless mxbuild is cached and a
// test .mpr is available (set MXSERVE_TEST_MPR, or it defaults to the spike app).
func TestServeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping serve integration test in -short mode")
	}
	if AnyCachedMxBuildPath() == "" {
		t.Skip("no mxbuild cached; run 'mxcli setup mxbuild -p <app.mpr>'")
	}
	mpr := os.Getenv("MXSERVE_TEST_MPR")
	if mpr == "" {
		mpr = "/tmp/spikeapp/SpikeApp.mpr"
	}
	if _, err := os.Stat(mpr); err != nil {
		t.Skipf("no test .mpr at %s; set MXSERVE_TEST_MPR", mpr)
	}

	srv, err := StartServe(ServeOptions{Port: 6555})
	if err != nil {
		t.Fatalf("StartServe: %v", err)
	}
	defer srv.Stop()

	t0 := time.Now()
	cold, err := srv.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: mpr})
	if err != nil {
		t.Fatalf("cold build: %v\n--- serve log ---\n%s", err, srv.Log())
	}
	if !cold.OK() {
		t.Fatalf("cold build not OK: status=%s msg=%s", cold.Status, cold.Message)
	}
	coldDur := time.Since(t0)

	t1 := time.Now()
	warm, err := srv.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: mpr})
	if err != nil {
		t.Fatalf("warm build: %v", err)
	}
	if !warm.OK() {
		t.Fatalf("warm build not OK: status=%s", warm.Status)
	}
	warmDur := time.Since(t1)

	t.Logf("cold=%s warm=%s (incremental serve should make warm << cold)", coldDur, warmDur)
}
