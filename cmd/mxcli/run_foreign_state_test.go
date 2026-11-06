// SPDX-License-Identifier: Apache-2.0

// Copying an app folder copies .mxcli/ with it. These tests copy a project the
// way `cp -R` does and prove the copy neither inherits nor acts on the
// original's run state or handshake, and that no secret is stored.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// copyProject copies the project folder, .mxcli/ included, and returns the
// copy's .mpr.
func copyProject(t *testing.T, mpr string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "copy")
	src := filepath.Dir(mpr)
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dst, filepath.Base(mpr))
}

func TestRefuseIfRunning_ForeignProjectIsNotBlocking(t *testing.T) {
	orig := scratchProject(t)
	live := startSleeper(t)
	_ = docker.WriteRunState(orig, &docker.RunState{
		Project: orig, PID: live, Phase: docker.PhaseRunning, URL: "http://127.0.0.1:8080/",
	})
	cp := copyProject(t, orig)

	if err := refuseIfRunning(orig); err == nil {
		t.Fatal("the original's own live run no longer blocks a second start")
	}
	if err := refuseIfRunning(cp); err != nil {
		t.Errorf("the copy was blocked by the original's run: %v", err)
	}
}

func TestRefuseIfRunning_SymlinkedPathIsTheSameProject(t *testing.T) {
	orig := scratchProject(t)
	live := startSleeper(t)
	_ = docker.WriteRunState(orig, &docker.RunState{Project: orig, PID: live, Phase: docker.PhaseRunning})
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Dir(orig), link); err != nil {
		t.Skip("cannot create a symlink here:", err)
	}
	if err := refuseIfRunning(filepath.Join(link, "App.mpr")); err == nil {
		t.Error("a symlinked path to the project was taken for another project, so a second run could start")
	}
}

func TestRefuseIfRunning_StateWithoutProjectStillBlocks(t *testing.T) {
	mpr := scratchProject(t)
	live := startSleeper(t)
	_ = docker.WriteRunState(mpr, &docker.RunState{PID: live, Phase: docker.PhaseRunning})
	if err := refuseIfRunning(mpr); err == nil {
		t.Error("a state written before the project was recorded must still block")
	}
}

func TestRunStop_ForeignStateDoesNotSignal(t *testing.T) {
	orig := scratchProject(t)
	live := startSleeper(t)
	_ = docker.WriteRunState(orig, &docker.RunState{Project: orig, PID: live, Phase: docker.PhaseRunning})
	cp := copyProject(t, orig)

	var out bytes.Buffer
	if code := runStop(cp, &out); code != runExitOK {
		t.Fatalf("exit %d, output %q", code, out.String())
	}
	if !docker.PidAlive(live) {
		t.Fatal("run stop in the copy terminated the original's process")
	}
	if !strings.Contains(out.String(), "belongs to the run of "+orig) ||
		!strings.Contains(out.String(), "stopped (nothing was running)") {
		t.Errorf("the output does not say the state was ignored:\n%s", out.String())
	}
	if st, _ := docker.ReadRunState(orig); st == nil || st.Phase != docker.PhaseRunning {
		t.Errorf("the original's state was changed: %+v", st)
	}
}

func TestRunWaitAndStatus_ForeignStateIsNotRunning(t *testing.T) {
	fastPolling(t)
	orig := scratchProject(t)
	live := startSleeper(t)
	_ = docker.WriteRunState(orig, &docker.RunState{Project: orig, PID: live, Phase: docker.PhaseRunning, Watch: true})
	cp := copyProject(t, orig)

	var out bytes.Buffer
	if code := runWait(cp, 0, 0, &out); code != runExitNotRunning {
		t.Errorf("run wait in the copy: exit %d, output %q", code, out.String())
	}
	out.Reset()
	if code := runStatus(cp, &out); code != runExitNotRunning {
		t.Errorf("run status in the copy: exit %d, output %q", code, out.String())
	}
	if !strings.Contains(out.String(), "ignoring it") {
		t.Errorf("run status does not say it ignored the copied state:\n%s", out.String())
	}
}

func TestReadDevLoopHandshake_ForeignProject(t *testing.T) {
	orig := handshakeProject(t)
	if err := writeDevLoopHandshake(orig, devLoopHandshake{Project: orig, PID: os.Getpid(), AdminPort: 8090}); err != nil {
		t.Fatal(err)
	}
	cp := copyProject(t, orig)
	if _, err := readDevLoopHandshake(orig); err != nil {
		t.Fatalf("the original's own handshake was refused: %v", err)
	}
	_, err := readDevLoopHandshake(cp)
	if err == nil {
		t.Fatal("the copy accepted the original's handshake")
	}
	if !strings.Contains(err.Error(), orig) || !strings.Contains(err.Error(), "ignoring") {
		t.Errorf("the error does not name the other project: %v", err)
	}
}

func TestRedactArgs(t *testing.T) {
	in := []string{"run", "--local", "--constant", "A.Key=sekret", "--constant=B.Key=top=secret",
		"--constant", "C.Key=@env:TOKEN", "--db-password", "p4ss", "--hub-secret=u:s", "--watch",
		"--app-port", "8080", "-p", "/x/App.mpr"}
	got, changed := redactArgs(in)
	want := []string{"run", "--local", "--constant", "A.Key=" + redactedValue,
		"--constant=B.Key=" + redactedValue, "--constant", "C.Key=@env:TOKEN",
		"--db-password", redactedValue, "--hub-secret=" + redactedValue, "--watch",
		"--app-port", "8080", "-p", "/x/App.mpr"}
	if !changed || !reflect.DeepEqual(got, want) {
		t.Errorf("redactArgs:\n got  %q\n want %q (changed=%v)", got, want, changed)
	}
	if in[3] != "A.Key=sekret" {
		t.Error("redactArgs modified its input, which is the child's real command line")
	}
	if _, changed := redactArgs([]string{"run", "--local", "--constant", "C.Key=@file:/x/tok"}); changed {
		t.Error("an indirection was treated as a secret")
	}
	if !hasRedactedArgs(got) {
		t.Error("hasRedactedArgs does not see the placeholder")
	}
	if hasRedactedArgs([]string{"run", "--local", "--constant", "C.Key=@env:TOKEN"}) {
		t.Error("hasRedactedArgs fired without a placeholder")
	}
}

func TestRedactArgsRuntimeSettingsAndEmpty(t *testing.T) {
	in := []string{"run", "--runtime-setting", "DatabasePassword=pw", "--runtime-setting=MicroflowConstants={\"A.B\":\"x\"}",
		"--runtime-setting", "ScheduledEventExecution=NONE", "--runtime-setting=DatabasePassword=", "--db-password=", "--db-password", ""}
	got, changed := redactArgs(in)
	want := []string{"run", "--runtime-setting", "DatabasePassword=" + redactedValue,
		"--runtime-setting=MicroflowConstants=" + redactedValue,
		"--runtime-setting", "ScheduledEventExecution=NONE", "--runtime-setting=DatabasePassword=", "--db-password=", "--db-password", ""}
	if !changed || !reflect.DeepEqual(got, want) {
		t.Errorf("redactArgs:\n got  %q\n want %q", got, want)
	}
	if _, c := redactArgs([]string{"run", "--db-password=", "--runtime-setting", "DatabasePassword="}); c || hasRedactedArgs(got[len(got)-3:]) {
		t.Error("an empty secret was treated as redacted")
	}
}

func TestRunStateDoesNotContainSecrets(t *testing.T) {
	mpr := scratchProject(t)
	args, _ := redactArgs(detachArgs([]string{"run", "--local", "--constant", "A.Key=sekret-value", "--db-password=p4ss-word"}, mpr))
	if err := docker.WriteRunState(mpr, &docker.RunState{Project: mpr, PID: 1, Args: args}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(docker.RunStatePath(mpr))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"sekret-value", "p4ss-word"} {
		if strings.Contains(string(body), secret) {
			t.Errorf("run-state.json contains %q:\n%s", secret, body)
		}
	}
	if !strings.Contains(string(body), "A.Key=") {
		t.Errorf("the constant name was lost:\n%s", body)
	}
}

func TestHandshakeDoesNotCarryConstants(t *testing.T) {
	boot := map[string]any{
		"DatabaseName":       "app",
		"MicroflowConstants": map[string]string{"A.Key": "sekret-value"},
	}
	got := bootConfigForHandshake(boot)
	if _, ok := got["MicroflowConstants"]; ok {
		t.Error("MicroflowConstants is still in the handshake's boot config")
	}
	if got["DatabaseName"] != "app" {
		t.Errorf("the rest of the boot config was lost: %v", got)
	}
	if _, ok := boot["MicroflowConstants"]; !ok {
		t.Error("the runtime's own boot config was modified")
	}
	p := handshakeProject(t)
	if err := writeDevLoopHandshake(p, devLoopHandshake{Project: p, PID: os.Getpid(), BootConfig: got}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(devLoopHandshakePath(p))
	if strings.Contains(string(body), "sekret-value") {
		t.Errorf("run-local.json contains a constant value:\n%s", body)
	}
	var h devLoopHandshake
	if err := json.Unmarshal(body, &h); err != nil || len(h.BootConfig) == 0 {
		t.Errorf("the handshake no longer carries a boot config to re-send: %v %v", err, h.BootConfig)
	}
}
