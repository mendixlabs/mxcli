// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

// `run --local --detach` opened a visible console window for every console
// program the detached run started — java, mxbuild, gradle
// (mendixlabs/mxcli#1484). The run was created DETACHED_PROCESS, which gives
// it no console, so Windows allocated a new visible one for each console child.
// These tests start a helper through setDetachAttrs and ask the helper, and a
// console child of the helper, what console each ended up with — the mechanism,
// not just the flag.

const (
	detachHelperEnv = "MXCLI_TEST_DETACH_HELPER" // "parent" or "child"
	detachOutEnv    = "MXCLI_TEST_DETACH_OUT"    // report file the helper writes
)

// consoleReport is what a helper process says about its own console.
type consoleReport struct {
	PID           uint32   `json:"pid"`
	ConsoleCP     uint32   `json:"consoleCP"`     // 0 = no console attached
	ConsoleWindow uintptr  `json:"consoleWindow"` // 0 = no console window
	Visible       bool     `json:"visible"`
	ConsolePIDs   []uint32 `json:"consolePIDs"` // processes attached to the same console
}

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	user32                 = syscall.NewLazyDLL("user32.dll")
	procGetConsoleCP       = kernel32.NewProc("GetConsoleCP")
	procGetConsoleWindow   = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcList = kernel32.NewProc("GetConsoleProcessList")
	procIsWindowVisible    = user32.NewProc("IsWindowVisible")
)

func describeConsole() consoleReport {
	r := consoleReport{PID: uint32(os.Getpid())}
	cp, _, _ := procGetConsoleCP.Call()
	r.ConsoleCP = uint32(cp)
	hwnd, _, _ := procGetConsoleWindow.Call()
	r.ConsoleWindow = hwnd
	if hwnd != 0 {
		v, _, _ := procIsWindowVisible.Call(hwnd)
		r.Visible = v != 0
	}
	buf := make([]uint32, 32)
	n, _, _ := procGetConsoleProcList.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n > 0 && int(n) <= len(buf) {
		r.ConsolePIDs = buf[:n]
	}
	return r
}

func writeReport(t *testing.T, path string, r consoleReport) {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("encode report: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

func readReport(t *testing.T, path string) consoleReport {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("helper wrote no report: %v", err)
	}
	var r consoleReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("decode report %q: %v", b, err)
	}
	return r
}

// TestDetachConsoleHelper is not a test: it is the detached run ("parent") and
// the console program the run starts ("child", e.g. java).
func TestDetachConsoleHelper(t *testing.T) {
	role, out := os.Getenv(detachHelperEnv), os.Getenv(detachOutEnv)
	if role == "" {
		t.Skip("helper process only")
	}
	if role == "child" {
		writeReport(t, out, describeConsole())
		os.Exit(0)
	}
	// The parent starts a console child exactly as the real run starts java:
	// no console flags of its own.
	child := exec.Command(os.Args[0], "-test.run=^TestDetachConsoleHelper$")
	child.Env = append(os.Environ(), detachHelperEnv+"=child", detachOutEnv+"="+out+".child")
	if err := child.Run(); err != nil {
		t.Fatalf("start console child: %v", err)
	}
	writeReport(t, out, describeConsole())
	os.Exit(0)
}

// TestDetachedRunHasHiddenConsoleItsChildrenShare is the behavioural proof for
// mendixlabs/mxcli#1484: the detached run has a console (so its console
// children inherit it instead of getting a new one), and that console has no
// window (so nothing pops up).
func TestDetachedRunHasHiddenConsoleItsChildrenShare(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestDetachConsoleHelper$")
	cmd.Env = append(os.Environ(), detachHelperEnv+"=parent", detachOutEnv+"="+out)
	setDetachAttrs(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("detached helper: %v", err)
	}
	parent := readReport(t, out)
	child := readReport(t, out+".child")
	t.Logf("detached run:   %+v", parent)
	t.Logf("its console child: %+v", child)

	if parent.ConsoleCP == 0 {
		t.Errorf("the detached run has no console, so Windows gives each console child a new visible one; want a console of its own")
	}
	if parent.ConsoleWindow != 0 {
		t.Errorf("the detached run's console has a window (%#x); want a hidden one", parent.ConsoleWindow)
	}
	if child.ConsoleCP == 0 {
		t.Errorf("the run's console child has no console")
	}
	if child.ConsoleWindow != 0 || child.Visible {
		t.Errorf("the run's console child got a console window (%#x, visible=%v); want none", child.ConsoleWindow, child.Visible)
	}
	shared := false
	for _, p := range child.ConsolePIDs {
		if p == parent.PID {
			shared = true
		}
	}
	if !shared {
		t.Errorf("the run's console child (pid %d) is on console %v, not the run's (pid %d): it got a console of its own", child.PID, child.ConsolePIDs, parent.PID)
	}
}

// The flag set the behaviour above depends on, spelled with literals so a
// change to the constants is caught. CREATE_NO_WINDOW is ignored when combined
// with DETACHED_PROCESS, so the two must not appear together.
func TestSetDetachAttrsFlags(t *testing.T) {
	const (
		createNewProcessGroupFlag = 0x00000200
		detachedProcessFlag       = 0x00000008
		createNoWindowFlag        = 0x08000000
	)
	cmd := exec.Command("cmd")
	setDetachAttrs(cmd)
	flags := cmd.SysProcAttr.CreationFlags
	if flags&createNewProcessGroupFlag == 0 {
		t.Errorf("flags %#x lack CREATE_NEW_PROCESS_GROUP: the run would share the caller's Ctrl-C", flags)
	}
	if flags&createNoWindowFlag == 0 {
		t.Errorf("flags %#x lack CREATE_NO_WINDOW", flags)
	}
	if flags&detachedProcessFlag != 0 {
		t.Errorf("flags %#x contain DETACHED_PROCESS: the run has no console and its children each get a visible one", flags)
	}
}
