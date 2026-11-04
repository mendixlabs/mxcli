// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDialPlan(t *testing.T) {
	cases := []struct {
		name       string
		host       string
		dial       string
		dockerable bool
		want       []string
	}{
		// localhost dials 127.0.0.1 first: Studio Pro on Windows listens on
		// 127.0.0.1 only, and a resolver that answers localhost with a LAN
		// address first made a plain `--mcp http://localhost:7782/mcp` time out
		// (mendixlabs/mxcli#1684). The resolver's own answer stays as the next
		// try, for a server bound to ::1 only.
		{"localhost where the docker name resolves: 127.0.0.1, localhost, then the gateway", "localhost:7784", "", true, []string{"127.0.0.1:7784", "localhost:7784", "host.docker.internal:7784"}},
		{"localhost where it does not (macOS host, Issue 8)", "localhost:7784", "", false, []string{"127.0.0.1:7784", "localhost:7784"}},
		{"localhost is matched without regard to case", "LocalHost:7784", "", false, []string{"127.0.0.1:7784", "LocalHost:7784"}},
		{"127.0.0.1 where the docker name resolves", "127.0.0.1:7782", "", true, []string{"127.0.0.1:7782", "host.docker.internal:7782"}},
		{"127.0.0.1 where it does not", "127.0.0.1:7782", "", false, []string{"127.0.0.1:7782"}},
		{"another host has no fallback", "myhost:9000", "", true, []string{"myhost:9000"}},
		{"no port is port 80", "localhost", "", true, []string{"127.0.0.1:80", "localhost:80", "host.docker.internal:80"}},
		{"--mcp-dial is the only address", "localhost:7782", "127.0.0.1:7782", true, []string{"127.0.0.1:7782"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dialPlan(c.host, c.dial, c.dockerable)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("dialPlan(%q, %q, %v) = %v, want %v", c.host, c.dial, c.dockerable, got, c.want)
			}
		})
	}
}

func TestDialFirst_TriesThePlanInOrderAndReportsEveryFailure(t *testing.T) {
	var tried []string
	refuse := func(_ context.Context, addr string) (net.Conn, error) {
		tried = append(tried, addr)
		return nil, errors.New("dial " + addr + ": refused")
	}
	_, err := dialFirst(context.Background(), []string{"a:1", "b:1"}, refuse)
	if strings.Join(tried, ",") != "a:1,b:1" {
		t.Errorf("tried %v, want a:1 then b:1", tried)
	}
	if err == nil || !strings.Contains(err.Error(), "a:1") || !strings.Contains(err.Error(), "b:1") {
		t.Errorf("the error does not name both attempts: %v", err)
	}

	tried = nil
	client, server := net.Pipe()
	defer server.Close()
	acceptSecond := func(_ context.Context, addr string) (net.Conn, error) {
		tried = append(tried, addr)
		if addr == "b:1" {
			return client, nil
		}
		return nil, errors.New("refused")
	}
	conn, err := dialFirst(context.Background(), []string{"a:1", "b:1", "c:1"}, acceptSecond)
	if err != nil || conn != client {
		t.Fatalf("dialFirst = %v, %v; want the second address's connection", conn, err)
	}
	if strings.Join(tried, ",") != "a:1,b:1" {
		t.Errorf("tried %v, want to stop at the first that accepts", tried)
	}
}

// pretendDockerHostResolves makes host.docker.internal count as resolving, as
// it does on a Windows host with Docker Desktop installed: Docker Desktop
// writes the name into the hosts file, pointing at the machine's LAN address.
func pretendDockerHostResolves(t *testing.T) {
	t.Helper()
	dockerHostOnce.Do(func() {})
	saved := dockerHostOnce.reachable
	dockerHostOnce.reachable = true
	t.Cleanup(func() { dockerHostOnce.reachable = saved })
}

// mendixlabs/mxcli#1684 (ako/mxcli#1833): on Windows with Docker Desktop,
// `--mcp http://localhost:7782/mcp` without --mcp-dial failed with
// `dial tcp <LAN IP>:7782: i/o timeout` while Studio Pro listened on
// 127.0.0.1. "host.docker.internal resolves" was taken to mean "inside a
// container", so the client dialled the Docker name instead of loopback.
func TestClient_LocalhostReachesStudioProOnTheHostWhenTheDockerNameResolves(t *testing.T) {
	pretendDockerHostResolves(t)
	ped := newFakePED(t, nil)
	port := ped.srv.URL[strings.LastIndex(ped.srv.URL, ":")+1:]
	c, err := NewClient(ClientOptions{URL: "http://localhost:" + port + "/mcp", Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Initialize(); err != nil {
		t.Fatalf("a Studio Pro listening on loopback was not reached: %v", err)
	}
}

// The dial goes to 127.0.0.1, but the request still says Host: localhost —
// Studio Pro answers HTTP 400 "Invalid Hostname" to any other Host header
// (mendixlabs/mxcli#1684), so the dial address must never leak into the URL.
func TestClient_LocalhostKeepsTheLocalhostHostHeader(t *testing.T) {
	var mu sync.Mutex
	var hosts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hosts = append(hosts, r.Host)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"fake","version":"1"}}}`))
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	c, err := NewClient(ClientOptions{URL: "http://localhost:" + port + "/mcp", Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Initialize()
	mu.Lock()
	defer mu.Unlock()
	if len(hosts) == 0 {
		t.Fatal("the server on 127.0.0.1 received no request")
	}
	if hosts[0] != "localhost:"+port {
		t.Errorf("Host header = %q, want %q", hosts[0], "localhost:"+port)
	}
}
