// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// ako/mxcli#2384 M2: a second `mxcli run --local` refused to boot because the
// FIRST app's build server held 6543, and the only remedy offered was to pass
// three port flags. The app and admin ports must keep refusing — adopting
// another app's runtime makes edits appear to do nothing — but the serve port
// is mxcli's own plumbing: the handshake publishes whichever port was chosen.

// busyPort binds a loopback port for the test's duration and returns it.
func busyPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().(*net.TCPAddr).Port
}

// CONTROL: a free documented port is kept, so the docs, the skills, `run
// status` and every example stay true for the ordinary single-app case.
func TestServePort_KeepsTheDocumentedPortWhenFree(t *testing.T) {
	free := busyPortThenRelease(t)
	if got := servePortOrFree(free); got != free {
		t.Errorf("got %d, want the documented port %d when it is free", got, free)
	}
}

// busyPortThenRelease returns a port that was bindable and is now free.
func busyPortThenRelease(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// A busy documented port moves the run to a free one instead of refusing.
func TestServePort_MovesOffABusyDefault(t *testing.T) {
	busy := busyPort(t)
	got := servePortOrFree(busy)
	if got == busy {
		t.Fatalf("got %d, want a different port: %d is already answering", got, busy)
	}
	if got == 0 {
		t.Fatal("got no port at all")
	}
	// The port it moved to must actually be free, or the boot fails later with
	// a worse message than the one this replaced.
	if err := pingTCP(fmt.Sprintf("127.0.0.1:%d", got), 200*time.Millisecond); err == nil {
		t.Errorf("port %d answers already — it is not free", got)
	}
}

// The default is 6543 whenever 6543 is free, and a moved port is never 6543.
func TestServePort_DefaultIsTheDocumentedOne(t *testing.T) {
	if documentedServePort != 6543 {
		t.Fatalf("the documented serve port is %d; the docs, skills and examples say 6543", documentedServePort)
	}
}

// An explicitly requested port is never moved, and a busy one is still
// refused: the caller asked for THAT port, and two apps on different Mendix
// versions must not share one serve process.
func TestServePort_ExplicitIsNeverMovedAndStillRefused(t *testing.T) {
	busy := busyPort(t)
	o := LocalRunOptions{ProjectPath: "/p/app.mpr", ServePort: busy}
	o.applyDefaults()
	if o.ServePort != busy {
		t.Fatalf("applyDefaults moved an explicit --serve-port from %d to %d", busy, o.ServePort)
	}
	err := checkTargetPortsFree(LocalRunOptions{AppPort: busyPortThenRelease(t), AdminPort: busyPortThenRelease(t), ServePort: busy})
	if err == nil {
		t.Fatal("an explicit busy serve port must still be refused")
	}
}

// The fix, through the path a run actually takes: with the documented port
// busy, applyDefaults moves the run off it instead of leaving it to be refused.
//
// This is the test that detects the defect. The servePortOrFree tests above do
// not: with applyDefaults reverted to a fixed 6543 they all still pass, because
// they never go through it — which a stub of exactly that shape showed.
//
// The precondition is "6543 is busy", so the test takes it either way: it binds
// 6543 when it can, and when it cannot, something else already holds it and the
// precondition is met without binding.
func TestApplyDefaults_MovesOffABusyDocumentedPort(t *testing.T) {
	addr := fmt.Sprintf("127.0.0.1:%d", documentedServePort)
	if l, err := net.Listen("tcp", addr); err == nil {
		defer l.Close()
	} else if pingTCP(addr, 500*time.Millisecond) != nil {
		t.Skipf("cannot make port %d busy, and nothing holds it: %v", documentedServePort, err)
	}

	o := LocalRunOptions{ProjectPath: "/p/app.mpr"}
	o.applyDefaults()
	if o.ServePort == documentedServePort {
		t.Fatalf("applyDefaults left the serve port at %d while it is busy — a second "+
			"`run --local` is refused instead of booting", documentedServePort)
	}
	if o.ServePort == 0 {
		t.Fatal("applyDefaults left the serve port unset")
	}
	if pingTCP(fmt.Sprintf("127.0.0.1:%d", o.ServePort), 200*time.Millisecond) == nil {
		t.Errorf("it moved to port %d, which answers already", o.ServePort)
	}
}

// The same through LocalAppOptions, the other defaulting path.
func TestLocalAppApplyDefaults_MovesOffABusyDocumentedPort(t *testing.T) {
	addr := fmt.Sprintf("127.0.0.1:%d", documentedServePort)
	if l, err := net.Listen("tcp", addr); err == nil {
		defer l.Close()
	} else if pingTCP(addr, 500*time.Millisecond) != nil {
		t.Skipf("cannot make port %d busy, and nothing holds it: %v", documentedServePort, err)
	}

	o := LocalAppOptions{ProjectPath: "/p/app.mpr"}
	o.applyDefaults()
	if o.ServePort == documentedServePort {
		t.Fatalf("LocalAppOptions.applyDefaults left the serve port at %d while it is busy", documentedServePort)
	}
}
