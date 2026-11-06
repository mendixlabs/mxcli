// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

const testDBPassword = "test-only-private-db-password"

func TestRunDBPasswordStdinFlag(t *testing.T) {
	f := runCmd.Flags().Lookup("db-password-stdin")
	if f == nil {
		t.Fatal("run does not register --db-password-stdin")
	}
	if f.DefValue != "false" || f.Usage == "" {
		t.Fatal("--db-password-stdin must be opt-in and documented in help")
	}
}

type passwordInput struct {
	io.Reader
	err   error
	bytes int
	calls int
}

func (r *passwordInput) Read(p []byte) (int, error) {
	r.calls++
	if r.err != nil {
		return 0, r.err
	}
	n, err := r.Reader.Read(p)
	r.bytes += n
	return n, err
}

func TestRunDBPassword(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		input     string
		readErr   error
		want      string
		wantErr   string
		neverRead bool
	}{
		{name: "default", want: "", neverRead: true},
		{name: "explicit flag", args: []string{"--db-password", " legacy "}, want: " legacy ", neverRead: true},
		{name: "explicit empty", args: []string{"--db-password="}, want: "", neverRead: true},
		{name: "disabled", args: []string{"--db-password-stdin=false"}, neverRead: true},
		{name: "private", args: []string{"--local", "--db-password-stdin"}, input: testDBPassword, want: testDBPassword},
		{name: "spaces", args: []string{"--local", "--db-password-stdin"}, input: " \tprivate password \t \n", want: " \tprivate password \t "},
		{name: "only spaces", args: []string{"--local", "--db-password-stdin"}, input: " \t \r\n", want: " \t "},
		{name: "LF", args: []string{"--local", "--db-password-stdin"}, input: testDBPassword + "\n", want: testDBPassword},
		{name: "CRLF", args: []string{"--local", "--db-password-stdin"}, input: testDBPassword + "\r\n", want: testDBPassword},
		{name: "hub implies local", args: []string{"--hub", "https://hub.example.com", "--db-password-stdin"}, input: testDBPassword, want: testDBPassword},
		{name: "limit", args: []string{"--local", "--db-password-stdin"}, input: strings.Repeat("x", maxDBPasswordInput), want: strings.Repeat("x", maxDBPasswordInput)},
		{name: "limit includes CRLF", args: []string{"--local", "--db-password-stdin"}, input: strings.Repeat("x", maxDBPasswordInput-2) + "\r\n", want: strings.Repeat("x", maxDBPasswordInput-2)},
		{name: "conflict", args: []string{"--local", "--db-password-stdin", "--db-password", testDBPassword}, wantErr: "cannot be combined with --db-password", neverRead: true},
		{name: "empty flag conflict", args: []string{"--local", "--db-password-stdin", "--db-password="}, wantErr: "cannot be combined with --db-password", neverRead: true},
		{name: "not local", args: []string{"--db-password-stdin"}, wantErr: "requires --local", neverRead: true},
		{name: "detach", args: []string{"--local", "--db-password-stdin", "--detach"}, wantErr: "cannot be combined with --detach", neverRead: true},
		{name: "hsqldb", args: []string{"--local", "--db-password-stdin", "--db-type", " HSQLDB "}, wantErr: "requires a PostgreSQL database", neverRead: true},
		{name: "unknown db", args: []string{"--local", "--db-password-stdin", "--db-type", "other"}, wantErr: "requires a PostgreSQL database", neverRead: true},
		{name: "empty stdin", args: []string{"--local", "--db-password-stdin"}, wantErr: "requires a non-empty password"},
		{name: "empty LF", args: []string{"--local", "--db-password-stdin"}, input: "\n", wantErr: "requires a non-empty password"},
		{name: "empty CRLF", args: []string{"--local", "--db-password-stdin"}, input: "\r\n", wantErr: "requires a non-empty password"},
		{name: "oversized", args: []string{"--local", "--db-password-stdin"}, input: strings.Repeat("x", maxDBPasswordInput*2), wantErr: "input exceeds 64 KiB"},
		{name: "read error", args: []string{"--local", "--db-password-stdin"}, readErr: errors.New(testDBPassword), wantErr: "could not read --db-password-stdin"},
		{name: "embedded LF", args: []string{"--local", "--db-password-stdin"}, input: testDBPassword + "\nsecond\n", wantErr: "without embedded line endings or NUL"},
		{name: "repeated LF", args: []string{"--local", "--db-password-stdin"}, input: testDBPassword + "\n\n", wantErr: "without embedded line endings or NUL"},
		{name: "bare CR", args: []string{"--local", "--db-password-stdin"}, input: testDBPassword + "\r", wantErr: "without embedded line endings or NUL"},
		{name: "NUL", args: []string{"--local", "--db-password-stdin"}, input: testDBPassword + "\x00", wantErr: "without embedded line endings or NUL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("db-password", "", "")
			cmd.Flags().Bool("db-password-stdin", false, "")
			cmd.Flags().Bool("local", false, "")
			cmd.Flags().Bool("detach", false, "")
			cmd.Flags().String("hub", "", "")
			cmd.Flags().String("db-type", "", "")
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatal("could not parse password test flags")
			}
			input := &passwordInput{Reader: strings.NewReader(tc.input), err: tc.readErr}
			cmd.SetIn(input)
			got, err := runDBPassword(cmd)
			if tc.wantErr == "" {
				if err != nil || got != tc.want {
					t.Fatal("password value was not preserved")
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) || got != "" {
				t.Fatal("expected a safe password validation error and no password")
			}
			if err != nil && strings.Contains(err.Error(), testDBPassword) {
				t.Fatal("password appeared in an error")
			}
			if tc.neverRead && input.calls != 0 {
				t.Fatal("stdin was read without valid opt-in")
			}
			if input.bytes > maxDBPasswordInput+1 {
				t.Fatal("password read exceeded its bound")
			}
		})
	}
}

// Subprocesses isolate Cobra's global flags and the command's os.Exit paths.
// Only the runtime runner is replaced; parsing, validation, DBConfig and state
// recording all use the production command.
func TestRunDBPasswordCommand(t *testing.T) {
	if os.Getenv("MXCLI_TEST_RUN_DB_PASSWORD") == "1" {
		mode := os.Getenv("MXCLI_TEST_PASSWORD_MODE")
		if mode == "read-error" || mode == "default" {
			rootCmd.SetIn(&passwordInput{err: errors.New(testDBPassword)})
		}
		runLocal = func(opts docker.LocalRunOptions) error {
			want := ""
			if strings.HasPrefix(mode, "private") || mode == "legacy" || mode == "runtime-error" || strings.HasPrefix(mode, "typed-stop") {
				want = testDBPassword
			}
			if opts.DB.Password != want {
				return errors.New("password did not reach local runtime options")
			}
			fromStdin, _ := runCmd.Flags().GetBool("db-password-stdin")
			if opts.DBPasswordStdin != fromStdin {
				return errors.New("password input mode did not reach local runtime options")
			}
			if mode == "runtime-error" {
				return fmt.Errorf("runtime rejected password %s", opts.DB.Password)
			}
			if strings.HasPrefix(mode, "typed-stop") {
				opts.State.Ready("http://127.0.0.1:8080/", docker.BuildOutcome{Gen: 1, OK: true, Action: "boot"})
				detail := "connection closed"
				if mode == "typed-stop-secret" {
					detail = opts.DB.Password
				}
				return docker.NewRuntimeStoppedError("runtime stopped: "+detail, "runtime error: "+detail)
			}
			if strings.HasPrefix(mode, "private") {
				if strings.Contains(strings.Join(os.Args, " "), testDBPassword) ||
					strings.Contains(strings.Join(opts.State.State().Args, " "), testDBPassword) ||
					runCmd.Flags().Lookup("db-password").Value.String() != "" {
					return errors.New("private password escaped into argv, state args or flag values")
				}
			}
			if mode == "private-hold" {
				if opts.ScreenshotPassword != "--detach" {
					return errors.New("flag-looking screenshot password was not preserved")
				}
				opts.State.Ready("http://127.0.0.1:8080/", docker.BuildOutcome{Gen: 1, OK: true, Action: "boot"})
				time.Sleep(time.Minute)
			}
			fmt.Fprintln(opts.Stdout, "password reached local runtime options")
			return nil
		}
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"mxcli"}, os.Args[i+1:]...)
				rootCmd.SetArgs(os.Args[1:])
				if err := rootCmd.Execute(); err != nil {
					os.Exit(1)
				}
				os.Exit(0)
			}
		}
		os.Exit(99)
	}
	for _, tc := range []struct {
		name    string
		flags   []string
		input   string
		mode    string
		wantErr string
	}{
		{name: "private", flags: []string{"--local", "--db-password-stdin"}, input: testDBPassword + "\r\n", mode: "private"},
		{name: "default", flags: []string{"--local"}, mode: "default"},
		{name: "legacy", flags: []string{"--local", "--db-password", testDBPassword}, mode: "legacy"},
		{name: "legacy empty", flags: []string{"--local", "--db-password="}},
		{name: "conflict", flags: []string{"--local", "--db-password-stdin", "--db-password="}, wantErr: "cannot be combined with --db-password"},
		{name: "empty", flags: []string{"--local", "--db-password-stdin"}, wantErr: "requires a non-empty password"},
		{name: "oversized", flags: []string{"--local", "--db-password-stdin"}, input: strings.Repeat("x", maxDBPasswordInput+1), wantErr: "input exceeds 64 KiB"},
		{name: "read error", flags: []string{"--local", "--db-password-stdin"}, mode: "read-error", wantErr: "could not read --db-password-stdin"},
		{name: "not local", flags: []string{"--db-password-stdin"}, wantErr: "requires --local"},
		{name: "hsqldb", flags: []string{"--local", "--db-password-stdin", "--db-type", "hsqldb"}, wantErr: "requires a PostgreSQL database"},
		{name: "detach", flags: []string{"--local", "--db-password-stdin", "--detach"}, wantErr: "cannot be combined with --detach"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := filepath.Join(t.TempDir(), "App.mpr")
			args := append([]string{"run", "-p", project}, tc.flags...)
			out, err := runPasswordSubprocess(t, tc.mode, tc.input, args)
			if strings.Contains(out, testDBPassword) {
				t.Fatal("password appeared in command output")
			}
			if tc.wantErr == "" {
				if err != nil || !strings.Contains(out, "password reached local runtime options") {
					t.Fatalf("command did not hand the password to the runtime runner: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(out, tc.wantErr) {
					t.Fatalf("command did not reject invalid password input: %v", err)
				}
				entries, readErr := os.ReadDir(filepath.Dir(project))
				if readErr != nil || len(entries) != 0 {
					t.Fatal("password validation changed the project directory")
				}
			}
		})
	}
}

func runPasswordSubprocess(t *testing.T, mode, input string, args []string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	childArgs := append([]string{"-test.run=^TestRunDBPasswordCommand$", "--"}, args...)
	child := exec.CommandContext(ctx, os.Args[0], childArgs...)
	child.Env = append(os.Environ(), "MXCLI_TEST_RUN_DB_PASSWORD=1", "MXCLI_TEST_PASSWORD_MODE="+mode)
	child.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	child.Stdout, child.Stderr = &out, &out
	err := child.Run()
	return out.String(), err
}

func TestRunDBPasswordStdinRestartRefusedBeforeStop(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestRunDBPasswordRestartProcess$")
	child.Env = append(os.Environ(), "MXCLI_TEST_PASSWORD_HOLD=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	for _, flag := range []string{"--db-password-stdin", "--db-password-stdin=true", "--db-password-stdin=1", "--db-password-stdin=TRUE"} {
		project := filepath.Join(t.TempDir(), "App.mpr")
		if err := docker.WriteRunState(project, &docker.RunState{Phase: docker.PhaseRunning, PID: child.Process.Pid,
			Args: []string{"run", "--local", flag}}); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(docker.RunStatePath(project))
		if err != nil {
			t.Fatal(err)
		}
		out, err := runPasswordSubprocess(t, "", "", []string{"run", "restart", "-p", project})
		if err == nil || !strings.Contains(out, "run restart cannot reuse --db-password-stdin") || strings.Contains(out, "stopped (") {
			t.Fatal("restart must refuse before stopping the run")
		}
		after, err := os.ReadFile(docker.RunStatePath(project))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("refused restart changed recorded state")
		}
		if !docker.PidAlive(child.Process.Pid) {
			t.Fatal("refused restart stopped the running process")
		}
	}
}

func TestRunDBPasswordRestartProcess(t *testing.T) {
	if os.Getenv("MXCLI_TEST_PASSWORD_HOLD") == "1" {
		time.Sleep(time.Minute)
	}
}

func TestRunDBPasswordStdinRestartPreservesFlagLookingValues(t *testing.T) {
	project := filepath.Join(t.TempDir(), "App.mpr")
	child := exec.Command(os.Args[0], "-test.run=^TestRunDBPasswordCommand$", "--",
		"run", "--local", "--screenshot-password", "--detach", "--db-password-stdin", "-p", project)
	child.Env = append(os.Environ(), "MXCLI_TEST_RUN_DB_PASSWORD=1", "MXCLI_TEST_PASSWORD_MODE=private-hold")
	child.Stdin = strings.NewReader(testDBPassword)
	var output bytes.Buffer
	child.Stdout, child.Stderr = &output, &output
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = child.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = child.Process.Kill()
		<-done
		if strings.Contains(output.String(), testDBPassword) {
			t.Error("foreground command printed the private password")
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, _ := docker.ReadRunState(project)
		if state != nil && state.Phase == docker.PhaseRunning && state.PID == child.Process.Pid {
			break
		}
		select {
		case <-done:
			t.Fatal("foreground command exited before it was ready")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("foreground command did not publish its running state")
		}
		time.Sleep(10 * time.Millisecond)
	}
	before, err := os.ReadFile(docker.RunStatePath(project))
	if err != nil {
		t.Fatal(err)
	}
	out, err := runPasswordSubprocess(t, "restart-probe", "", []string{"run", "restart", "-p", project})
	if err == nil || !strings.Contains(out, "run restart cannot reuse --db-password-stdin") || strings.Contains(out, "stopped (") {
		t.Error("restart did not refuse recorded flag-looking values before stopping the run")
	}
	after, err := os.ReadFile(docker.RunStatePath(project))
	if err != nil || !bytes.Equal(before, after) || bytes.Contains(after, []byte(testDBPassword)) {
		t.Error("restart changed run state or persisted the private password")
	}
	select {
	case <-done:
		t.Error("restart stopped the owned foreground command")
	default:
	}
}

func TestRunDBPasswordStdinRuntimeErrorsAreRedacted(t *testing.T) {
	project := filepath.Join(t.TempDir(), "App.mpr")
	out, err := runPasswordSubprocess(t, "runtime-error", testDBPassword,
		[]string{"run", "--local", "--db-password-stdin", "-p", project})
	if err == nil || !strings.Contains(out, "runtime rejected password [redacted]") || strings.Contains(out, testDBPassword) {
		t.Fatal("runtime error did not redact the private password")
	}
	state, err := os.ReadFile(docker.RunStatePath(project))
	if err != nil || bytes.Contains(state, []byte(testDBPassword)) {
		t.Fatal("runtime error leaked the private password into run state")
	}
}

func TestRunDBPasswordStdinTypedStopReasonIsPreserved(t *testing.T) {
	for _, mode := range []string{"typed-stop-secret", "typed-stop-no-secret"} {
		t.Run(mode, func(t *testing.T) {
			project := filepath.Join(t.TempDir(), "App.mpr")
			out, err := runPasswordSubprocess(t, mode, testDBPassword,
				[]string{"run", "--local", "--db-password-stdin", "-p", project})
			if err == nil || strings.Contains(out, testDBPassword) {
				t.Fatal("typed stop error was not returned safely")
			}
			state, err := docker.ReadRunState(project)
			if err != nil || state == nil {
				t.Fatal("could not read stopped run state")
			}
			detail := "connection closed"
			if mode == "typed-stop-secret" {
				detail = "[redacted]"
			}
			if state.Phase != docker.PhaseStopped || state.Ready.IsZero() ||
				state.Reason != "runtime stopped: "+detail || state.Error != "runtime error: "+detail {
				t.Fatal("typed runtime stop lost its ready state, reason or redacted error message")
			}
			if !strings.Contains(out, "runtime error: "+detail) {
				t.Fatal("typed runtime error message was not preserved")
			}
		})
	}
}

func TestRunDBPasswordStdinLifecycleHints(t *testing.T) {
	for _, stdin := range []bool{false, true} {
		args := []string{"run", "--local"}
		if stdin {
			args = append(args, "--db-password-stdin")
		}
		project := filepath.Join(t.TempDir(), "App.mpr")
		state := &docker.RunState{Project: project, PID: os.Getpid(), Phase: docker.PhaseRunning,
			URL: "http://127.0.0.1:8080/", Args: args}
		if err := docker.WriteRunState(project, state); err != nil {
			t.Fatal(err)
		}
		out, err := runPasswordSubprocess(t, "private", testDBPassword,
			[]string{"run", "--local", "--db-password-stdin", "-p", project})
		if err == nil || !strings.Contains(out, "already running") {
			t.Fatal("command did not refuse a second run")
		}
		checkPasswordLifecycleHint(t, out, stdin)
		state.Phase, state.Reason = docker.PhaseStopped, "runtime stopped"
		line, _ := statusLine(state, statusProbes{alive: neverAlive}, time.Time{}, time.Now())
		checkPasswordLifecycleHint(t, line, stdin)
		state.Phase = docker.PhaseRunning
		line, _ = statusLine(state, statusProbes{alive: alwaysAlive}, time.Now(), time.Now())
		checkPasswordLifecycleHint(t, line, stdin)
		state.Watch = true
		line, _ = statusLine(state, statusProbes{alive: alwaysAlive, handshake: func(*docker.RunState) bool { return false }}, time.Time{}, time.Now())
		checkPasswordLifecycleHint(t, line, stdin)
		state.Watch = false
		state.Last = &docker.BuildOutcome{Gen: 1, OK: true, Action: "boot"}
		if err := docker.WriteRunState(project, state); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(project, []byte("test model"), 0o600); err != nil {
			t.Fatal(err)
		}
		var wait bytes.Buffer
		if runWait(project, time.Second, 0, &wait) != runExitFailed {
			t.Fatal("wait did not reject a change without watch mode")
		}
		checkPasswordLifecycleHint(t, wait.String(), stdin)
	}
}

func checkPasswordLifecycleHint(t *testing.T, hint string, stdin bool) {
	t.Helper()
	if !stdin {
		if !strings.Contains(hint, "mxcli run restart") || strings.Contains(hint, "fresh stdin") {
			t.Fatal("normal run lost its restart hint")
		}
		return
	}
	for _, want := range []string{"mxcli run stop", "new foreground run", "fresh stdin"} {
		if !strings.Contains(hint, want) {
			t.Error("private-password run lacks an actionable fresh-input hint")
		}
	}
	if strings.Contains(hint, "mxcli run restart") {
		t.Error("private-password run recommends unsupported replay")
	}
}

func TestRunArgsNeedPasswordStdin(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"run", "--local"}, false},
		{[]string{"run", "--db-password-stdin"}, true},
		{[]string{"run", "--db-password-stdin=1"}, true},
		{[]string{"run", "--db-password-stdin=TRUE"}, true},
		{[]string{"run", "--db-password-stdin=false"}, false},
		{[]string{"run", "--db-password-stdin", "--db-password-stdin=0"}, false},
		{[]string{"run", "--db-password-stdin=false", "--db-password-stdin"}, true},
		{[]string{"run", "--", "--db-password-stdin"}, false},
		{[]string{"run", "--db-password", "--db-password-stdin"}, false},
		{[]string{"run", "--engine", "--db-password-stdin"}, false},
		{[]string{"run", "--screenshot-password", "--db-password-stdin"}, false},
		{[]string{"run", "--screenshot-password", "--db-password-stdin", "--db-password-stdin"}, true},
	} {
		if runArgsNeedPasswordStdin(tc.args) != tc.want {
			t.Error("restart did not use the effective stdin flag value")
		}
	}
}
