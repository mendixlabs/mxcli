// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"strings"
	"testing"
)

// applyDefaults is where the empty-means-loopback default actually lives, so a
// test that skips it would assert a shape the runtime never sees.
func TestApplyDefaultsPinsListenAddr(t *testing.T) {
	var o LocalRuntimeOptions
	o.applyDefaults()
	if o.ListenAddr != "127.0.0.1" {
		t.Errorf("applyDefaults left ListenAddr = %q, want 127.0.0.1", o.ListenAddr)
	}

	var w LocalRuntimeOptions
	w.ListenAddr = "0.0.0.0"
	w.applyDefaults()
	if w.ListenAddr != "0.0.0.0" {
		t.Errorf("applyDefaults overwrote an explicit ListenAddr: %q", w.ListenAddr)
	}
}

// The M2EE admin API is a privileged surface and must never be widened onto the
// network by --listen-addr: whatever the app binds to, the admin bind stays on
// loopback. This is the value that reaches both the runtime's
// M2EE_ADMIN_LISTEN_ADDRESSES and mxcli's own client Host (both use
// m2eeAdminListenAddr), so a regression here is silent otherwise — the app
// comes up and only the admin API is on the network. The want string is
// hard-coded, not read from the constant, so changing the constant fails here.
func TestLocalRuntimeEnvAdminBindIsAlwaysLoopback(t *testing.T) {
	const want = "M2EE_ADMIN_LISTEN_ADDRESSES=127.0.0.1"
	for _, listen := range []string{"", "127.0.0.1", "0.0.0.0", "::", "192.168.2.35"} {
		var o LocalRuntimeOptions
		o.AdminPass = "pw"
		o.ListenAddr = listen
		o.applyDefaults()
		env := localRuntimeEnv(o)
		found := false
		for _, kv := range env {
			if kv == want {
				found = true
			}
			if strings.HasPrefix(kv, "M2EE_ADMIN_LISTEN_ADDRESSES=") && kv != want {
				t.Errorf("ListenAddr %q: admin bind leaked %q, want %q", listen, kv, want)
			}
		}
		if !found {
			t.Errorf("ListenAddr %q: env is missing %q", listen, want)
		}
	}
}

// The app's own bind is the appContainerParams value and is unaffected by the
// admin bind: a caller asking for 0.0.0.0 must get 0.0.0.0, or the flag would
// silently do nothing.
func TestAppContainerParamsKeepsWildcard(t *testing.T) {
	for _, listen := range []string{"", "0.0.0.0", "192.168.2.35"} {
		var o LocalRuntimeOptions
		o.AppPort = 8095
		o.ListenAddr = listen
		o.applyDefaults()
		p := appContainerParams(o)
		got, ok := p["runtime_listen_addresses"].(string)
		if !ok {
			t.Fatalf("ListenAddr %q: runtime_listen_addresses is %T, want string", listen, p["runtime_listen_addresses"])
		}
		want := listen
		if want == "" {
			want = "127.0.0.1"
		}
		if got != want {
			t.Errorf("runtime_listen_addresses for %q = %q, want %q", listen, got, want)
		}
		if got := p["runtime_port"]; got != 8095 {
			t.Errorf("runtime_port = %v, want 8095", got)
		}
	}
}
