// SPDX-License-Identifier: Apache-2.0

package procalive

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// helperEnv makes the re-executed test binary run TestHelperSleep instead of
// the real tests.
const helperEnv = "PROCALIVE_TEST_SLEEP_HELPER"

// TestHelperSleep is not a test: it is the body of the child process the tests
// below start. It sleeps until it is killed, bounded so a leaked child cannot
// outlive the test run for long.
func TestHelperSleep(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process only")
	}
	time.Sleep(2 * time.Minute)
}

func TestAliveSelf(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Fatal("Alive(os.Getpid()) = false for the running test process")
	}
}

func TestAliveRejectsNonPositivePid(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if Alive(pid) {
			t.Errorf("Alive(%d) = true", pid)
		}
	}
}

func TestAliveFollowsAChildProcess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperSleep$")
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the helper process: %v", err)
	}
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	if !Alive(cmd.Process.Pid) {
		t.Fatalf("Alive(%d) = false for a running child", cmd.Process.Pid)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("killing the helper process: %v", err)
	}
	_ = cmd.Wait() // reaps it: on POSIX an unreaped zombie still answers signal 0
	reaped = true

	if Alive(cmd.Process.Pid) {
		t.Fatalf("Alive(%d) = true for a child that was killed and reaped", cmd.Process.Pid)
	}
}
