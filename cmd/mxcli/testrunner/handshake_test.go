// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o600); err != nil {
		t.Fatalf("writing project fixture: %v", err)
	}
	return mpr
}

func TestHandshakeRoundTrip(t *testing.T) {
	mpr := tempProject(t)
	want := Handshake{
		Project: mpr, PID: os.Getpid(),
		AppPort: 8080, AdminPort: 8090, ServePort: 6543,
		Token: "tok", Started: time.Now(),
	}
	if err := WriteHandshake(mpr, want); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}

	got, err := ReadHandshake(mpr)
	if err != nil {
		t.Fatalf("ReadHandshake: %v", err)
	}
	if got.Token != want.Token || got.AppPort != want.AppPort ||
		got.AdminPort != want.AdminPort || got.ServePort != want.ServePort {
		t.Errorf("round trip lost data: got %+v", got)
	}
}

// TestHandshakeIsNotWorldReadable pins the file mode: the handshake carries a
// live token for an endpoint that executes microflows.
func TestHandshakeIsNotWorldReadable(t *testing.T) {
	mpr := tempProject(t)
	if err := WriteHandshake(mpr, Handshake{PID: os.Getpid(), AppPort: 8080, Token: "tok"}); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	info, err := os.Stat(HandshakePath(mpr))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("handshake mode is %o, want 600 — it holds a live token", perm)
	}
}

// TestHandshakeLeavesNoTempFile pins that the write-then-rename never leaves the
// intermediate behind, which would also be a token on disk nobody cleans up.
func TestHandshakeLeavesNoTempFile(t *testing.T) {
	mpr := tempProject(t)
	if err := WriteHandshake(mpr, Handshake{PID: os.Getpid(), AppPort: 8080, Token: "tok"}); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	if _, err := os.Stat(HandshakePath(mpr) + ".tmp"); !os.IsNotExist(err) {
		t.Error("the temp file used for the atomic write was left behind")
	}
}

func TestReadHandshakeMissingExplainsHowToStartOne(t *testing.T) {
	mpr := tempProject(t)
	_, err := ReadHandshake(mpr)
	if err == nil {
		t.Fatal("reading a missing handshake succeeded")
	}
	if !strings.Contains(err.Error(), "--test-endpoint") {
		t.Errorf("error %q does not say how to start a hosting app", err)
	}
}

// TestReadHandshakeRejectsADeadHost pins the staleness check. A dev loop killed
// with SIGKILL leaves the file behind; without this the attach would fail much
// later with a confusing connection error.
func TestReadHandshakeRejectsADeadHost(t *testing.T) {
	mpr := tempProject(t)
	// PID 0x7FFFFFFF is above any real pid_max, so it cannot be live.
	if err := WriteHandshake(mpr, Handshake{PID: 0x7FFFFFFF, AppPort: 8080, Token: "tok", Started: time.Now()}); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	_, err := ReadHandshake(mpr)
	if err == nil {
		t.Fatal("a handshake naming a dead process was accepted")
	}
	if !strings.Contains(err.Error(), "no longer running") {
		t.Errorf("error %q does not identify the host as dead", err)
	}
}

func TestReadHandshakeRejectsIncomplete(t *testing.T) {
	mpr := tempProject(t)
	body, _ := json.Marshal(Handshake{PID: os.Getpid(), AppPort: 8080}) // no token
	path := HandshakePath(mpr)
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadHandshake(mpr); err == nil {
		t.Fatal("a handshake with no token was accepted")
	}
}

func TestRemoveHandshake(t *testing.T) {
	mpr := tempProject(t)
	if err := WriteHandshake(mpr, Handshake{PID: os.Getpid(), AppPort: 8080, Token: "tok"}); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	RemoveHandshake(mpr)
	if _, err := os.Stat(HandshakePath(mpr)); !os.IsNotExist(err) {
		t.Error("the handshake survived RemoveHandshake")
	}
	RemoveHandshake(mpr) // must not panic when already gone
}

// TestGenerateEndpointMDLChainsAfterStartup pins that hosting the endpoint in a
// dev app does not silently drop the app's own startup logic — seed data, for
// instance — which displacing after-startup would.
func TestGenerateEndpointMDLChainsAfterStartup(t *testing.T) {
	mdl := GenerateEndpointMDL("MyModule.ASU_Startup")
	if !strings.Contains(mdl, "CALL MICROFLOW MyModule.ASU_Startup()") {
		t.Errorf("the project's own after-startup is not chained:\n%s", mdl)
	}

	register := strings.Index(mdl, "CALL JAVA ACTION "+endpointRegisterAction)
	chained := strings.Index(mdl, "CALL MICROFLOW MyModule.ASU_Startup()")
	if register > chained {
		t.Error("the endpoint is registered after the chained microflow; a failure in that microflow would then leave no endpoint to diagnose it with")
	}
}

func TestGenerateEndpointMDLNoChainWhenNone(t *testing.T) {
	mdl := GenerateEndpointMDL("")
	if strings.Contains(mdl, "$Chained") {
		t.Errorf("a chained call was emitted with no microflow to chain:\n%s", mdl)
	}
}

func TestDropTestFlows(t *testing.T) {
	got := dropTestFlows("", &TestSuite{Tests: []TestCase{{ID: "test_1"}, {ID: "test_2"}}})
	want := []string{"DROP MICROFLOW MxTest.Test_test_1", "DROP MICROFLOW MxTest.Test_test_2", "DROP MICROFLOW " + errorMessageFlow}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("command %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestDropTestFlowsNeverTouchesTheEndpoint pins the ownership boundary: an
// attach adds only test microflows, so it must remove only those. The endpoint
// and the after-startup setting belong to the dev loop hosting them.
func TestDropTestFlowsNeverTouchesTheEndpoint(t *testing.T) {
	for _, cmd := range dropTestFlows("", &TestSuite{Tests: []TestCase{{ID: "test_1"}}}) {
		for _, forbidden := range []string{"DROP MODULE", endpointStartupFlow, endpointRegisterAction, "AfterStartupMicroflow"} {
			if strings.Contains(cmd, forbidden) {
				t.Errorf("attach cleanup would remove %q, which the hosting dev loop owns: %q", forbidden, cmd)
			}
		}
	}
}

func TestValidateOptionsAttach(t *testing.T) {
	tests := []struct {
		name    string
		opts    RunOptions
		wantErr string
	}{
		{name: "attach alone is fine", opts: RunOptions{Attach: true}},
		{name: "attach with watch is fine", opts: RunOptions{Attach: true, Watch: true}},
		{
			name:    "attach with the legacy runner",
			opts:    RunOptions{Attach: true, LegacyRunner: true},
			wantErr: "--attach cannot be combined with --legacy-runner",
		},
		{
			name:    "attach with skip-build",
			opts:    RunOptions{Attach: true, SkipBuild: true},
			wantErr: "--attach cannot be combined with --skip-build",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOptions(tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestAttachDoesNotRequireLocal pins that --attach implies a local app: needing
// --local as well would be noise, since there is nothing else to attach to.
func TestAttachDoesNotRequireLocal(t *testing.T) {
	if err := validateOptions(RunOptions{Attach: true, Local: false}); err != nil {
		t.Errorf("--attach without --local was rejected: %v", err)
	}
}

// TestAttachUsesTheAdminPasswordNotTheEndpointToken pins a bug found by running
// --attach against a live app: the M2EE admin API and the test endpoint use
// different secrets, and passing the endpoint token to the admin API fails with
// "Authentication failed" at the first reload — after the test microflows have
// already been injected.
func TestAttachUsesTheAdminPasswordNotTheEndpointToken(t *testing.T) {
	mpr := tempProject(t)
	hs := Handshake{
		Project: mpr, PID: os.Getpid(),
		AppPort: 8080, AdminPort: 8090, ServePort: 6543,
		AdminPass: "the-admin-password",
		Token:     "the-endpoint-token",
	}
	if err := WriteHandshake(mpr, hs); err != nil {
		t.Fatalf("WriteHandshake: %v", err)
	}
	got, err := ReadHandshake(mpr)
	if err != nil {
		t.Fatalf("ReadHandshake: %v", err)
	}
	if got.AdminPass != "the-admin-password" {
		t.Fatalf("the handshake does not carry the admin password (got %q)", got.AdminPass)
	}
	if got.AdminPass == got.Token {
		t.Error("the admin password and the endpoint token are the same value; they are different secrets")
	}
}

// A copied app folder carries the original's handshake; attaching with it would
// run the copy's tests against another app, with that app's token.
func TestReadHandshake_ForeignProject(t *testing.T) {
	orig := tempProject(t)
	if err := WriteHandshake(orig, Handshake{
		Project: orig, PID: os.Getpid(), AppPort: 8080, AdminPort: 8090, Token: "tok", Started: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	cp := filepath.Join(t.TempDir(), "App.mpr")
	if err := os.MkdirAll(filepath.Dir(HandshakePath(cp)), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(HandshakePath(orig))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(HandshakePath(cp), body, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadHandshake(orig); err != nil {
		t.Fatalf("the original's own handshake was refused: %v", err)
	}
	_, err = ReadHandshake(cp)
	if err == nil {
		t.Fatal("the copy accepted the original's handshake")
	}
	if !strings.Contains(err.Error(), orig) || !strings.Contains(err.Error(), "ignoring") {
		t.Errorf("the error does not name the other project: %v", err)
	}
}
