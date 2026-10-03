// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

func TestSplitHostPort(t *testing.T) {
	cases := []struct{ in, host, port string }{
		{"127.0.0.1:5432", "127.0.0.1", "5432"},
		{"db.example.com:6000", "db.example.com", "6000"},
		{"localhost", "localhost", "5432"},
		{"[::1]:5544", "::1", "5544"},
		{"[::1]", "::1", "5432"},
		{"::1", "::1", "5432"},
		{"::1:5544", "::1", "5544"},
	}
	for _, c := range cases {
		h, p := splitHostPort(c.in)
		if h != c.host || p != c.port {
			t.Errorf("splitHostPort(%q) = (%q,%q), want (%q,%q)", c.in, h, p, c.host, c.port)
		}
	}
}

func TestNormalizePostgresEndpoint(t *testing.T) {
	cases := []struct{ in, host, port, endpoint string }{
		{":5432", "127.0.0.1", "5432", "127.0.0.1:5432"},
		{"[::1]:5544", "::1", "5544", "[::1]:5544"},
		{"[::1]", "::1", "5432", "[::1]:5432"},
		{"::1:5544", "::1", "5544", "[::1]:5544"},
	}
	for _, c := range cases {
		host, port, endpoint, err := normalizePostgresEndpoint(c.in)
		if err != nil {
			t.Errorf("normalizePostgresEndpoint(%q): %v", c.in, err)
			continue
		}
		if host != c.host || port != c.port || endpoint != c.endpoint {
			t.Errorf("normalizePostgresEndpoint(%q) = (%q,%q,%q), want (%q,%q,%q)",
				c.in, host, port, endpoint, c.host, c.port, c.endpoint)
		}
	}
}

func TestPostgresConfigString(t *testing.T) {
	got, err := postgresConfigString("/home/user's db")
	if err != nil || got != "'/home/user''s db'" {
		t.Fatalf("postgresConfigString escaped value = (%q, %v)", got, err)
	}
	if _, err := postgresConfigString("safe'\nport = 1"); err == nil {
		t.Fatal("configuration line injection should be rejected")
	}
}

func TestIsLocalHost(t *testing.T) {
	for _, h := range []string{"127.0.0.1", "localhost", "::1", ""} {
		if !isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = false, want true", h)
		}
	}
	for _, h := range []string{"db.example.com", "10.0.0.5", "postgres"} {
		if isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = true, want false", h)
		}
	}
}

func TestQuoteSQLString(t *testing.T) {
	if got := quoteSQLString("mendix"); got != "'mendix'" {
		t.Errorf("quoteSQLString = %q", got)
	}
	if got := quoteSQLString("it's"); got != "'it''s'" {
		t.Errorf("quoteSQLString(apostrophe) = %q, want doubled", got)
	}
}

func TestEnsureDatabase_Validation(t *testing.T) {
	if err := EnsureDatabase(nil, io.Discard); err == nil {
		t.Error("expected error for a nil database configuration")
	}
	// Non-Postgres type is rejected.
	if err := EnsureDatabase(&DBConfig{Type: "HSQLDB", Name: "a", User: "u"}, io.Discard); err == nil {
		t.Error("expected error for non-PostgreSQL type")
	}
	// Unsafe identifiers are rejected (before any exec).
	if err := EnsureDatabase(&DBConfig{Type: "PostgreSQL", Name: "bad-name;DROP", User: "u", Host: "127.0.0.1:5432"}, io.Discard); err == nil {
		t.Error("expected error for unsafe database name")
	}
	if err := EnsureDatabase(&DBConfig{Type: "PostgreSQL", Name: "ok", User: "bad user", Host: "127.0.0.1:5432"}, io.Discard); err == nil {
		t.Error("expected error for unsafe database user")
	}
	for _, host := range []string{"127.0.0.1:0", "127.0.0.1:5432 -c fsync=off"} {
		if err := EnsureDatabase(&DBConfig{Type: "PostgreSQL", Name: "ok", User: "ok", Host: host}, io.Discard); err == nil || !strings.Contains(err.Error(), "invalid --db-host") {
			t.Errorf("expected an actionable error for invalid host %q, got %v", host, err)
		}
	}
}

func TestEnsureDatabase_NormalizesHostForCaller(t *testing.T) {
	dir, logPath := newStubPATH(t)
	writeStub(t, dir, "psql", `echo "$@" >> "`+logPath+`"; exit 0`)
	db := DBConfig{
		Type: "PostgreSQL", Host: ":5432", Name: "app", User: "mendix", Password: "very-secret",
	}
	if err := EnsureDatabase(&db, io.Discard); err != nil {
		t.Fatal(err)
	}
	if db.Host != "127.0.0.1:5432" {
		t.Fatalf("caller DB host = %q, want canonical loopback endpoint", db.Host)
	}
	params := runtimeConfigParams(LocalRuntimeOptions{DB: db}, nil)
	if got := params["DatabaseHost"]; got != db.Host {
		t.Fatalf("runtime DatabaseHost = %q, want provisioned endpoint %q", got, db.Host)
	}
	if calls := readCalls(t, logPath); !strings.Contains(calls, "-h 127.0.0.1 -p 5432") {
		t.Fatalf("psql did not receive the canonical endpoint:\n%s", calls)
	} else if !strings.Contains(calls, "-X -w") {
		t.Fatalf("connection probe must ignore psqlrc and never prompt:\n%s", calls)
	} else if strings.Contains(calls, db.Password) {
		t.Fatalf("connection password must not be exposed in psql process arguments:\n%s", calls)
	}
}

func TestEnsureDatabase_RefusesToProvisionRemoteHost(t *testing.T) {
	dir, logPath := newStubPATH(t)
	writeStub(t, dir, "psql", `echo psql >> "`+logPath+`"; exit 1`)
	writeStub(t, dir, "sudo", `echo sudo >> "`+logPath+`"; exit 0`)
	writeStub(t, dir, "service", `echo service >> "`+logPath+`"; exit 0`)
	writeStub(t, dir, "initdb", `echo initdb >> "`+logPath+`"; exit 0`)
	db := DBConfig{
		Type: "PostgreSQL", Host: "db.example.com:5432", Name: "app", User: "mendix", Password: "secret",
	}
	err := EnsureDatabase(&db, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "host is not local") {
		t.Fatalf("expected remote provisioning to be refused, got %v", err)
	}
	if calls := readCalls(t, logPath); calls != "psql\n" {
		t.Fatalf("remote failure must only run the connection probe, calls:\n%s", calls)
	}
}

// --- #823: initdb/pg_ctl fallback ---

func newStubPATH(t *testing.T) (dir, logPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-stub test not supported on Windows")
	}
	dir = t.TempDir()
	t.Setenv("PATH", dir)
	testutil.SetHome(t, t.TempDir())
	// A stubbed PATH is only hermetic if the server-binary lookup is stubbed
	// too: this host really does have /usr/lib/postgresql/*/bin, so a test that
	// asserts "the tools are missing" would otherwise run a real initdb.
	withServerBinGlobs(t)
	// Pin the privilege level as well, so the same attempts are made whether the
	// suite runs as root in a container or as a user on a laptop.
	withEUID(t, 0)
	return dir, filepath.Join(dir, "calls")
}

// withServerBinGlobs points the server-binary search at dirs (none by default).
func withServerBinGlobs(t *testing.T, dirs ...string) {
	t.Helper()
	old := postgresServerBinGlobs
	postgresServerBinGlobs = dirs
	t.Cleanup(func() { postgresServerBinGlobs = old })
}

// withEUID forces the effective user id the start path branches on.
func withEUID(t *testing.T, uid int) {
	t.Helper()
	old := currentEUID
	currentEUID = func() int { return uid }
	t.Cleanup(func() { currentEUID = old })
}

func writeStub(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("writing stub %s: %v", name, err)
	}
}

func initdbStub(logPath string) string {
	return `data=""; next=0
for arg in "$@"; do
  [ "$next" = 1 ] && { data="$arg"; next=0; }
  [ "$arg" = "-D" ] && next=1
done
/bin/mkdir -p "$data"
echo 16 > "$data/PG_VERSION"
echo "# PostgreSQL configuration" > "$data/postgresql.conf"
echo "local all all trust" > "$data/pg_hba.conf"
echo "host all all 127.0.0.1/32 scram-sha-256" >> "$data/pg_hba.conf"
echo "host all all ::1/128 scram-sha-256" >> "$data/pg_hba.conf"
echo initdb >> "` + logPath + `"`
}

func pgctlStub(logPath string, statusCode, startCode int) string {
	return `last=""; for arg in "$@"; do last="$arg"; done
[ "$last" = status ] && exit ` + strconv.Itoa(statusCode) + `
echo pg_ctl_start >> "` + logPath + `"
exit ` + strconv.Itoa(startCode)
}

func readCalls(t *testing.T, logPath string) string {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// unusedTCPPort returns a currently unbound loopback port. Fallback tests must
// not depend on the developer or CI host having nothing listening on 5432: an
// occupied port intentionally suppresses the user-cluster fallback.
func unusedTCPPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func TestStartLocalPostgres_ServicePaths(t *testing.T) {
	t.Run("ready service skips fallback", func(t *testing.T) {
		dir, logPath := newStubPATH(t)
		writeStub(t, dir, "service", `echo service >> "`+logPath+`"; exit 1`)
		writeStub(t, dir, "pg_isready", "exit 0")
		writeStub(t, dir, "initdb", initdbStub(logPath))
		writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

		if _, err := startLocalPostgres("127.0.0.1", "5432", io.Discard); err != nil {
			t.Fatal(err)
		}
		calls := readCalls(t, logPath)
		if !strings.Contains(calls, "service") || strings.Contains(calls, "initdb") {
			t.Fatalf("unexpected calls:\n%s", calls)
		}
	})

	t.Run("non-ready service uses fallback", func(t *testing.T) {
		dir, logPath := newStubPATH(t)
		port := unusedTCPPort(t)
		oldTimeout := serviceReadyTimeout
		serviceReadyTimeout = 50 * time.Millisecond
		t.Cleanup(func() { serviceReadyTimeout = oldTimeout })
		writeStub(t, dir, "service", `echo service >> "`+logPath+`"`)
		writeStub(t, dir, "pg_isready", "exit 1")
		writeStub(t, dir, "initdb", initdbStub(logPath))
		writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

		if _, err := startLocalPostgres("127.0.0.1", port, io.Discard); err != nil {
			t.Fatal(err)
		}
		calls := readCalls(t, logPath)
		for _, want := range []string{"service", "initdb", "pg_ctl_start"} {
			if !strings.Contains(calls, want) {
				t.Fatalf("%s was not called:\n%s", want, calls)
			}
		}
	})
}

// A service-started PostgreSQL can own the TCP port while pg_isready still
// reports "the database system is starting up" (for example during crash
// recovery). The short service probe must not initialize a competing cluster;
// EnsureDatabase's longer readiness wait is authoritative in this case.
func TestStartLocalPostgres_OccupiedPortSkipsFallback(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)

	dir, logPath := newStubPATH(t)
	oldTimeout := serviceReadyTimeout
	serviceReadyTimeout = 20 * time.Millisecond
	t.Cleanup(func() { serviceReadyTimeout = oldTimeout })
	writeStub(t, dir, "service", `echo service >> "`+logPath+`"`)
	writeStub(t, dir, "pg_isready", "exit 1")
	writeStub(t, dir, "initdb", initdbStub(logPath))
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

	if _, err := startLocalPostgres("127.0.0.1", port, io.Discard); err != nil {
		t.Fatal(err)
	}
	calls := readCalls(t, logPath)
	if !strings.Contains(calls, "service") {
		t.Fatalf("service manager was not attempted:\n%s", calls)
	}
	if strings.Contains(calls, "initdb") || strings.Contains(calls, "pg_ctl_start") {
		t.Fatalf("an occupied port must suppress the user-cluster fallback:\n%s", calls)
	}
}

func TestStartLocalPostgres_Fallback(t *testing.T) {
	tests := []struct {
		name                  string
		tools, initialized    bool
		statusCode, startCode int
		wantErr               bool
		wantInit, wantStart   bool
	}{
		{name: "missing tools", wantErr: true},
		{name: "first init", tools: true, statusCode: 3, wantInit: true, wantStart: true},
		{name: "repeated stopped", tools: true, initialized: true, statusCode: 3, wantStart: true},
		{name: "already running", tools: true, initialized: true},
		{name: "start failure", tools: true, statusCode: 3, startCode: 1, wantErr: true, wantInit: true, wantStart: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, logPath := newStubPATH(t)
			port := unusedTCPPort(t)
			if tt.initialized {
				dataDir := initClusterDir(t)
				if tt.statusCode == 0 {
					writePostmasterPID(t, dataDir, port)
				}
			}
			if tt.tools {
				writeStub(t, dir, "initdb", initdbStub(logPath))
				writeStub(t, dir, "pg_ctl", pgctlStub(logPath, tt.statusCode, tt.startCode))
			}

			_, err := startLocalPostgres("127.0.0.1", port, io.Discard)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			calls := readCalls(t, logPath)
			if strings.Contains(calls, "initdb") != tt.wantInit {
				t.Errorf("initdb calls = %q, want %v", calls, tt.wantInit)
			}
			if strings.Contains(calls, "pg_ctl_start") != tt.wantStart {
				t.Errorf("pg_ctl start calls = %q, want %v", calls, tt.wantStart)
			}
		})
	}
}

func TestResolveSuperuser(t *testing.T) {
	t.Run("direct", func(t *testing.T) {
		dir, logPath := newStubPATH(t)
		writeStub(t, dir, "psql", `echo "$@" >> "`+logPath+`"`)
		writeStub(t, dir, "sudo", `echo sudo >> "`+logPath+`"`)
		su, err := resolveSuperuser("127.0.0.1", "5432")
		if err != nil || su.sudo {
			t.Fatalf("su=%+v err=%v", su, err)
		}
		calls := readCalls(t, logPath)
		if !strings.Contains(calls, "-U postgres") || strings.Contains(calls, "sudo") {
			t.Fatalf("unexpected calls:\n%s", calls)
		}
	})

	t.Run("sudo fallback", func(t *testing.T) {
		dir, logPath := newStubPATH(t)
		writeStub(t, dir, "psql", "exit 1")
		// Model a Debian-style cluster: sudo succeeds only through the default
		// Unix socket, and the requested port must still be explicit. A numeric
		// -h would force TCP, where peer authentication cannot work.
		writeStub(t, dir, "sudo", `echo "$@" >> "`+logPath+`"
case " $* " in *" -h "*) exit 1;; esac
case " $* " in *" -p 5544 "*) exit 0;; esac
exit 1`)
		su, err := resolveSuperuser("127.0.0.1", "5544")
		if err != nil || !su.sudo {
			t.Fatalf("su=%+v err=%v", su, err)
		}
		if calls := readCalls(t, logPath); strings.Contains(calls, " -h ") ||
			!strings.Contains(calls, " -p 5544 ") {
			t.Fatalf("sudo fallback must use the default socket at the requested port:\n%s", calls)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		dir, _ := newStubPATH(t)
		writeStub(t, dir, "psql", "exit 1")
		if _, err := resolveSuperuser("127.0.0.1", "5432"); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestResolveSuperuser_RefusesRemoteHost(t *testing.T) {
	dir, logPath := newStubPATH(t)
	writeStub(t, dir, "psql", `echo psql >> "`+logPath+`"; exit 0`)
	writeStub(t, dir, "sudo", `echo sudo >> "`+logPath+`"; exit 0`)
	if _, err := resolveSuperuser("db.example.com", "5432"); err == nil ||
		!strings.Contains(err.Error(), "non-local") {
		t.Fatalf("expected remote provisioning to be refused, got %v", err)
	}
	if calls := readCalls(t, logPath); calls != "" {
		t.Fatalf("remote host must not invoke a local or sudo superuser:\n%s", calls)
	}
}

func TestSuperuserPSQL_SudoUsesPeerSocketPortAndNeverPrompts(t *testing.T) {
	t.Setenv("PGHOST", "127.0.0.1")
	t.Setenv("PGHOSTADDR", "127.0.0.1")
	t.Setenv("PGSERVICE", "wrong-cluster")
	cmd := (superuser{host: "127.0.0.1", port: "5544", sudo: true}).psql("-tAc", "select 1")
	want := []string{
		"sudo", "-n", "-u", "postgres", "--", "psql", "-X", "-v", "ON_ERROR_STOP=1", "-w",
		"-p", "5544", "-U", "postgres", "-d", "postgres",
		"-tAc", "select 1",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("sudo psql args = %#v, want %#v", cmd.Args, want)
	}
	for _, entry := range cmd.Env {
		for _, key := range []string{"PGHOST=", "PGHOSTADDR=", "PGSERVICE="} {
			if strings.HasPrefix(entry, key) {
				t.Fatalf("sudo psql inherited target override %q", entry)
			}
		}
	}
}

func TestEnsureRole_ForcesScramPassword(t *testing.T) {
	dir, _ := newStubPATH(t)
	argsPath := filepath.Join(dir, "psql.args")
	sqlPath := filepath.Join(dir, "psql.stdin")
	writeStub(t, dir, "psql", `echo "$@" >> "`+argsPath+`"; /bin/cat >> "`+sqlPath+`"`)
	su := superuser{host: "127.0.0.1", port: "5432"}
	db := DBConfig{Name: "app", User: "mendix", Password: "s'ecret\\path"}
	if err := ensureRole(su, db, io.Discard); err != nil {
		t.Fatal(err)
	}
	args := readCalls(t, argsPath)
	if strings.Contains(args, db.Password) {
		t.Fatalf("role password must not be exposed in psql process arguments:\n%s", args)
	}
	if !strings.Contains(args, "-f -") {
		t.Fatalf("role DDL should be read from standard input, args:\n%s", args)
	}
	sql := readCalls(t, sqlPath)
	if !strings.Contains(sql, "PASSWORD 's''ecret\\path'") {
		t.Fatalf("role password was not safely quoted in SQL read from stdin:\n%s", sql)
	}
	standardStringsAt := strings.Index(sql, "SET standard_conforming_strings = on")
	setAt := strings.Index(sql, "SET password_encryption = 'scram-sha-256'")
	createAt := strings.Index(sql, "CREATE ROLE mendix")
	if standardStringsAt < 0 || standardStringsAt > setAt {
		t.Fatalf("role creation must make password quoting independent of server settings:\n%s", sql)
	}
	if setAt < 0 || createAt < 0 || setAt > createAt {
		t.Fatalf("role creation must force SCRAM before assigning the password:\n%s", sql)
	}
}

// writePostmasterPID writes a minimal postmaster.pid whose 4th line is the port,
// matching what a running server publishes (PID, data dir, start time, port, …).
func writePostmasterPID(t *testing.T, dataDir, port string) {
	t.Helper()
	home, _ := os.UserHomeDir()
	writePostmasterPIDWithSocket(t, dataDir, port, filepath.Join(home, ".mxcli", "postgres", "sock"))
}

func writePostmasterPIDWithSocket(t *testing.T, dataDir, port, sockDir string) {
	t.Helper()
	body := "12345\n" + dataDir + "\n1700000000\n" + port + "\n" + sockDir + "\n"
	if err := os.WriteFile(filepath.Join(dataDir, "postmaster.pid"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// initClusterDir pre-creates an initialized data directory under $HOME so the
// initdb branch is skipped.
func initClusterDir(t *testing.T) string {
	t.Helper()
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".mxcli", "postgres", "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("16\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "postgresql.conf"), []byte("# PostgreSQL configuration\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hba := "local all all trust\n" +
		"host all all 127.0.0.1/32 scram-sha-256\n" +
		"host all all ::1/128 scram-sha-256\n"
	if err := os.WriteFile(filepath.Join(dataDir, "pg_hba.conf"), []byte(hba), 0o600); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

// --- #823 review: the dead pg_ctlcluster placeholder is gone ---

// The old attempts list carried {"pg_ctlcluster", "--", "start"} — a placeholder
// whose args can never start a cluster, so it only burned a readiness timeout.
// Even when present on PATH it must never be executed now.
func TestStartLocalPostgres_NeverRunsPgCtlCluster(t *testing.T) {
	dir, logPath := newStubPATH(t)
	port := unusedTCPPort(t)
	old := serviceReadyTimeout
	serviceReadyTimeout = 20 * time.Millisecond
	t.Cleanup(func() { serviceReadyTimeout = old })
	// No `service` on PATH; a pg_ctlcluster that would log if ever called.
	writeStub(t, dir, "pg_ctlcluster", `echo pg_ctlcluster >> "`+logPath+`"; exit 0`)
	writeStub(t, dir, "pg_isready", "exit 1")
	writeStub(t, dir, "initdb", initdbStub(logPath))
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

	if _, err := startLocalPostgres("127.0.0.1", port, io.Discard); err != nil {
		t.Fatal(err)
	}
	calls := readCalls(t, logPath)
	if strings.Contains(calls, "pg_ctlcluster") {
		t.Fatalf("pg_ctlcluster must never run, got:\n%s", calls)
	}
	if !strings.Contains(calls, "pg_ctl_start") {
		t.Fatalf("expected fallback to start a user cluster, got:\n%s", calls)
	}
}

// A failing service manager's own output must reach the final error rather than
// being replaced by an unrelated initdb/pg_ctl error two steps later.
func TestStartLocalPostgres_SurfacesServiceOutput(t *testing.T) {
	dir, _ := newStubPATH(t)
	port := unusedTCPPort(t)
	old := serviceReadyTimeout
	serviceReadyTimeout = 20 * time.Millisecond
	t.Cleanup(func() { serviceReadyTimeout = old })
	writeStub(t, dir, "service", `echo "postgresql.service is masked"; exit 1`)
	writeStub(t, dir, "pg_isready", "exit 1")
	// No initdb/pg_ctl on PATH, so the fallback fails — the error should still
	// carry the service manager's message.
	_, err := startLocalPostgres("127.0.0.1", port, io.Discard)
	if err == nil {
		t.Fatal("expected an error when neither a service nor the portable tools work")
	}
	if !strings.Contains(err.Error(), "postgresql.service is masked") {
		t.Fatalf("service output not surfaced: %v", err)
	}
}

// --- #823 review: initdb hardens loopback TCP (scram), trust only on the socket ---

func TestStartUserCluster_InitdbAuthArgs(t *testing.T) {
	dir, logPath := newStubPATH(t)
	port := unusedTCPPort(t)
	argsPath := filepath.Join(dir, "initdb.args")
	writeStub(t, dir, "initdb", `echo "$@" >> "`+argsPath+`"; `+initdbStub(logPath))
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

	if _, err := startLocalPostgres("127.0.0.1", port, io.Discard); err != nil {
		t.Fatal(err)
	}
	args := readCalls(t, argsPath)
	if !strings.Contains(args, "--auth-host=scram-sha-256") {
		t.Errorf("loopback TCP must not be trust; initdb args = %q", args)
	}
	if strings.Contains(args, "--auth-host=trust") {
		t.Errorf("initdb must not set --auth-host=trust; args = %q", args)
	}
	if !strings.Contains(args, "--auth-local=trust") {
		t.Errorf("socket connections should stay trust; args = %q", args)
	}
}

// The socket, listen address, and port must live in the cluster configuration,
// not only in pg_ctl -o flags, so a later plain `pg_ctl -D ... start` remains
// private and starts on the expected endpoint.
func TestStartUserCluster_PersistsSecureConnectionSettings(t *testing.T) {
	dir, logPath := newStubPATH(t)
	home, _ := os.UserHomeDir()
	sockDir := filepath.Join(home, ".mxcli", "postgres", "sock")
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, dir, "initdb", initdbStub(logPath))
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

	if err := startUserCluster("127.0.0.1", "5544", io.Discard); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(home, ".mxcli", "postgres", "data")
	info, err := os.Stat(sockDir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("socket directory = %v, %v; want mode 0700", info, err)
	}
	postgresConf, err := os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(postgresConf), mxcliPostgresConfigBegin); got != 1 {
		t.Fatalf("postgresql.conf should contain one mxcli block, got %d:\n%s", got, postgresConf)
	}
	firstConfig := string(postgresConf)
	for _, want := range []string{
		"listen_addresses = '127.0.0.1'",
		"port = 5544",
		"unix_socket_directories = '" + filepath.Join(home, ".mxcli", "postgres", "sock") + "'",
		"unix_socket_permissions = 0700",
	} {
		if !strings.Contains(string(postgresConf), want) {
			t.Errorf("postgresql.conf missing %q:\n%s", want, postgresConf)
		}
	}
	info, err = os.Stat(filepath.Join(dataDir, "postgresql.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("postgresql.conf mode = %04o, want 0600", got)
	}

	// Reapplying the configuration must replace, not duplicate, the managed block.
	if err := startUserCluster("127.0.0.1", "5544", io.Discard); err != nil {
		t.Fatal(err)
	}
	postgresConf, err = os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(postgresConf), mxcliPostgresConfigBegin); got != 1 {
		t.Fatalf("repeated configuration added duplicate managed blocks: %d\n%s", got, postgresConf)
	}
	if string(postgresConf) != firstConfig {
		t.Fatalf("repeated configuration was not byte-stable:\n%s", postgresConf)
	}
}

// A cluster initialized by the pre-review branch used host trust. Reuse must
// fail safely with a cleanup path instead of silently retaining that bypass.
func TestStartUserCluster_RejectsLegacyHostTrust(t *testing.T) {
	for _, record := range []string{
		"host all all 127.0.0.1/32 trust",
		"host all all 127.0.0.1 255.255.255.255 trust",
	} {
		t.Run(record, func(t *testing.T) {
			_, _ = newStubPATH(t)
			dataDir := initClusterDir(t)
			hba := "local all all trust\n" + record + "\n"
			if err := os.WriteFile(filepath.Join(dataDir, "pg_hba.conf"), []byte(hba), 0o600); err != nil {
				t.Fatal(err)
			}
			err := startUserCluster("127.0.0.1", "5432", io.Discard)
			if err == nil || !strings.Contains(err.Error(), "unsafe host trust") ||
				!strings.Contains(err.Error(), "remove ~/.mxcli/postgres") {
				t.Fatalf("expected an actionable host-trust error, got %v", err)
			}
		})
	}
}

func TestStartUserCluster_RefusesNonLocalHost(t *testing.T) {
	if err := startUserCluster("0.0.0.0", "5432", io.Discard); err == nil ||
		!strings.Contains(err.Error(), "non-local host") {
		t.Fatalf("expected a non-local bind refusal, got %v", err)
	}
}

// The direct superuser must reach the cluster over its private trust socket, not
// over loopback TCP (which is now scram-sha-256 and would reject a passwordless
// connection).
func TestResolveSuperuser_PrefersClusterSocket(t *testing.T) {
	dir, logPath := newStubPATH(t)
	writeStub(t, dir, "psql", `echo "$@" >> "`+logPath+`"`) // exits 0
	writeStub(t, dir, "sudo", `echo sudo >> "`+logPath+`"`)

	su, err := resolveSuperuser("127.0.0.1", "5432")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	sockDir := filepath.Join(home, ".mxcli", "postgres", "sock")
	if su.sock != sockDir {
		t.Fatalf("superuser should use the cluster socket %q, got %+v", sockDir, su)
	}
	if calls := readCalls(t, logPath); !strings.Contains(calls, "-h "+sockDir) {
		t.Fatalf("psql should connect over the socket dir, calls:\n%s", calls)
	}
}

// --- #823 review: an empty host normalises to loopback in the server config ---

func TestStartUserCluster_EmptyHostNormalised(t *testing.T) {
	dir, logPath := newStubPATH(t)
	initClusterDir(t) // skip initdb; status=3 => not running => start
	writeStub(t, dir, "pg_ctl", `last=""; for a in "$@"; do last="$a"; done
[ "$last" = status ] && exit 3
echo pg_ctl_start >> "`+logPath+`"
exit 0`)

	if err := startUserCluster("", "5432", io.Discard); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	conf, err := os.ReadFile(filepath.Join(home, ".mxcli", "postgres", "data", "postgresql.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conf), "listen_addresses = '127.0.0.1'") {
		t.Fatalf("empty host should become 127.0.0.1 in persisted config:\n%s", conf)
	}
}

// --- #823 review: a running cluster is only reused when its port matches ---

func TestStartUserCluster_RunningPortGuard(t *testing.T) {
	t.Run("matching port is left alone", func(t *testing.T) {
		dir, logPath := newStubPATH(t)
		port := unusedTCPPort(t)
		dataDir := initClusterDir(t)
		writePostmasterPID(t, dataDir, port)
		writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 0, 0)) // status=0 => running

		if _, err := startLocalPostgres("127.0.0.1", port, io.Discard); err != nil {
			t.Fatal(err)
		}
		if calls := readCalls(t, logPath); strings.Contains(calls, "pg_ctl_start") {
			t.Fatalf("a matching running cluster must not be restarted:\n%s", calls)
		}
	})

	t.Run("mismatched port errors instead of waiting", func(t *testing.T) {
		dir, logPath := newStubPATH(t)
		port := unusedTCPPort(t)
		portNumber, err := strconv.Atoi(port)
		if err != nil {
			t.Fatal(err)
		}
		livePort := strconv.Itoa(portNumber%65535 + 1)
		dataDir := initClusterDir(t)
		writePostmasterPID(t, dataDir, livePort)
		configPath := filepath.Join(dataDir, "postgresql.conf")
		beforeConfig, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 0, 0)) // status=0 => running

		_, err = startLocalPostgres("127.0.0.1", port, io.Discard)
		if err == nil {
			t.Fatal("expected an error when a cluster runs on a different port")
		}
		if !strings.Contains(err.Error(), livePort) || !strings.Contains(err.Error(), port) {
			t.Fatalf("error should name both the live and requested ports: %v", err)
		}
		if strings.Contains(err.Error(), "--db-port") || !strings.Contains(err.Error(), "--db-host") {
			t.Fatalf("error should recommend the real --db-host flag: %v", err)
		}
		if calls := readCalls(t, logPath); strings.Contains(calls, "pg_ctl_start") {
			t.Fatalf("a wrong-port cluster must not be restarted:\n%s", calls)
		}
		afterConfig, readErr := os.ReadFile(configPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(afterConfig) != string(beforeConfig) {
			t.Fatalf("a rejected endpoint must not rewrite postgresql.conf:\n%s", afterConfig)
		}
	})

	t.Run("unknown running port errors", func(t *testing.T) {
		dir, logPath := newStubPATH(t)
		initClusterDir(t)
		writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 0, 0)) // running, no readable pid file

		err := startUserCluster("127.0.0.1", "5432", io.Discard)
		if err == nil || !strings.Contains(err.Error(), "port could not be read") {
			t.Fatalf("expected an unknown-port error, got %v", err)
		}
	})

	t.Run("unexpected socket directory errors", func(t *testing.T) {
		dir, logPath := newStubPATH(t)
		dataDir := initClusterDir(t)
		writePostmasterPIDWithSocket(t, dataDir, "5432", "/tmp")
		writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 0, 0)) // running on an unsafe socket

		err := startUserCluster("127.0.0.1", "5432", io.Discard)
		if err == nil {
			t.Fatal("expected an error for a cluster using an unexpected socket directory")
		}
		if !strings.Contains(err.Error(), "/tmp") || !strings.Contains(err.Error(), "stop") {
			t.Fatalf("error should name the unsafe socket and how to restart safely: %v", err)
		}
	})
}

// --- #823 review: the start error names the server log ---

func TestStartUserCluster_StartErrorNamesLog(t *testing.T) {
	dir, logPath := newStubPATH(t)
	port := unusedTCPPort(t)
	initClusterDir(t)
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 1)) // not running, start fails

	_, err := startLocalPostgres("127.0.0.1", port, io.Discard)
	if err == nil {
		t.Fatal("expected a start failure")
	}
	if !strings.Contains(err.Error(), "server.log") {
		t.Fatalf("start error should name the server log: %v", err)
	}
}

// --- #984: --ensure-db in a non-root devcontainer ---

// withTimeouts shrinks both readiness waits so a test need not sit them out.
func withTimeouts(t *testing.T, d time.Duration) {
	t.Helper()
	oldService, oldReady := serviceReadyTimeout, readyTimeout
	serviceReadyTimeout, readyTimeout = d, d
	t.Cleanup(func() { serviceReadyTimeout, readyTimeout = oldService, oldReady })
}

// Debian's /etc/init.d/postgresql runs under `set -e` and calls
// create_socket_directory before it looks at any cluster; that chmod of
// /var/run/postgresql is refused for a non-root user, so the script aborts
// having started nothing. The elevated attempt must therefore come first — and
// must never be able to block on a password prompt.
func TestStartLocalPostgres_ElevatesServiceStartWhenNotRoot(t *testing.T) {
	dir, logPath := newStubPATH(t)
	withEUID(t, 1000)
	withTimeouts(t, 20*time.Millisecond)
	port := unusedTCPPort(t)
	writeStub(t, dir, "sudo", `echo "sudo $@" >> "`+logPath+`"`)
	writeStub(t, dir, "service", `echo "service $@" >> "`+logPath+`"`)
	writeStub(t, dir, "pg_isready", "exit 1")
	writeStub(t, dir, "initdb", initdbStub(logPath))
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

	if _, err := startLocalPostgres("127.0.0.1", port, io.Discard); err != nil {
		t.Fatal(err)
	}
	calls := readCalls(t, logPath)
	first := strings.SplitN(strings.TrimSpace(calls), "\n", 2)[0]
	if !strings.HasPrefix(first, "sudo ") {
		t.Fatalf("the elevated start must be attempted FIRST, calls:\n%s", calls)
	}
	if !strings.Contains(first, "-n") {
		t.Fatalf("sudo must be non-interactive, or an unattended run hangs on a "+
			"password prompt: %s", first)
	}
	for _, want := range []string{"service", "postgresql", "start"} {
		if !strings.Contains(first, want) {
			t.Fatalf("elevated attempt missing %q: %s", want, first)
		}
	}
	// The unprivileged form still runs, so nothing that worked before regresses.
	if !strings.Contains(calls, "\nservice postgresql start") {
		t.Fatalf("the unprivileged attempt must still run:\n%s", calls)
	}
}

// Running as root, there is nothing to elevate and sudo must not be involved.
func TestStartLocalPostgres_RootStartsServiceDirectly(t *testing.T) {
	dir, logPath := newStubPATH(t) // pins euid 0
	withTimeouts(t, 20*time.Millisecond)
	port := unusedTCPPort(t)
	writeStub(t, dir, "sudo", `echo "sudo $@" >> "`+logPath+`"`)
	writeStub(t, dir, "service", `echo "service $@" >> "`+logPath+`"`)
	writeStub(t, dir, "pg_isready", "exit 1")
	writeStub(t, dir, "initdb", initdbStub(logPath))
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

	if _, err := startLocalPostgres("127.0.0.1", port, io.Discard); err != nil {
		t.Fatal(err)
	}
	if calls := readCalls(t, logPath); strings.Contains(calls, "sudo") {
		t.Fatalf("root must not shell out to sudo:\n%s", calls)
	}
}

// Debian and Ubuntu wrap only the CLIENT tools into PATH; initdb and pg_ctl live
// in /usr/lib/postgresql/<major>/bin. Without looking there the user-owned
// cluster fallback cannot run at all on the commonest devcontainer base.
func TestStartUserCluster_FindsServerBinariesOutsidePATH(t *testing.T) {
	dir, logPath := newStubPATH(t)
	withTimeouts(t, 20*time.Millisecond)
	port := unusedTCPPort(t)
	writeStub(t, dir, "pg_isready", "exit 1") // a client tool, on PATH

	// Two majors installed side by side, as a long-lived box really has.
	libDir := t.TempDir()
	for _, major := range []string{"9", "16"} {
		binDir := filepath.Join(libDir, major, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeStub(t, binDir, "initdb", `echo "initdb-major-`+major+`" >> "`+logPath+`"
`+initdbStub(logPath))
		writeStub(t, binDir, "pg_ctl", `echo "pg_ctl-major-`+major+`" >> "`+logPath+`"
`+pgctlStub(logPath, 3, 0))
	}
	withServerBinGlobs(t, filepath.Join(libDir, "*", "bin"))

	if _, err := startLocalPostgres("127.0.0.1", port, io.Discard); err != nil {
		t.Fatal(err)
	}
	calls := readCalls(t, logPath)
	for _, want := range []string{"initdb", "pg_ctl_start"} {
		if !strings.Contains(calls, want) {
			t.Fatalf("%s was not run from the versioned bin directory:\n%s", want, calls)
		}
	}
	// Newest major wins. A lexical sort would rank "9" above "16" and pick the
	// wrong installation — and initdb and pg_ctl must agree, since a data
	// directory made by one major cannot be started by another.
	if !strings.Contains(calls, "initdb-major-16") || !strings.Contains(calls, "pg_ctl_start") {
		t.Fatalf("expected the newest major (16) to be used:\n%s", calls)
	}
	if strings.Contains(calls, "-major-9") {
		t.Fatalf("major 9 must not be preferred over 16:\n%s", calls)
	}
}

// mcr.microsoft.com/devcontainers/base grants its non-root user sudo to root
// ONLY (`vscode ALL=(root) NOPASSWD:ALL`), so `sudo -u postgres` is refused
// there. Root may target any account, so the nested form still reaches postgres.
func TestResolveSuperuser_FallsBackToSudoViaRoot(t *testing.T) {
	dir, logPath := newStubPATH(t)
	writeStub(t, dir, "psql", "exit 1") // no direct superuser connection
	writeStub(t, dir, "sudo", `echo "sudo $@" >> "`+logPath+`"
case " $* " in
  *" -n -- sudo "*) exit 0 ;;
  *" -n -u postgres "*) echo "sudo: a password is required" >&2; exit 1 ;;
esac
exit 1`)

	su, err := resolveSuperuser("127.0.0.1", "5432")
	if err != nil {
		t.Fatalf("a root-only sudoers policy must still reach postgres: %v", err)
	}
	if !su.sudo || !su.viaRoot {
		t.Fatalf("expected the via-root superuser, got %+v", su)
	}
	calls := readCalls(t, logPath)
	directAt := strings.Index(calls, "sudo -n -u postgres")
	nestedAt := strings.Index(calls, "sudo -n -- sudo")
	if directAt < 0 || nestedAt < 0 || directAt > nestedAt {
		t.Fatalf("the direct form must be tried before nesting through root:\n%s", calls)
	}
}

func TestSuperuserPSQL_ViaRootNestsAndNeverPrompts(t *testing.T) {
	cmd := (superuser{host: "127.0.0.1", port: "5544", sudo: true, viaRoot: true}).
		psql("-tAc", "select 1")
	want := []string{
		"sudo", "-n", "--", "sudo", "-n", "-u", "postgres", "--", "psql",
		"-X", "-v", "ON_ERROR_STOP=1", "-w",
		"-p", "5544", "-U", "postgres", "-d", "postgres",
		"-tAc", "select 1",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("via-root psql args = %#v, want %#v", cmd.Args, want)
	}
}

// The reported symptom: a permission denial this package already has in hand is
// reported to the user as a bare readiness timeout. A guard that names the wrong
// cause costs the reader more than no guard at all.
func TestEnsureDatabase_ReadinessTimeoutCarriesServiceDiagnostics(t *testing.T) {
	dir, _ := newStubPATH(t)
	withEUID(t, 1000)
	withTimeouts(t, 50*time.Millisecond)
	port := unusedTCPPort(t)
	writeStub(t, dir, "psql", "exit 1")
	writeStub(t, dir, "sudo", `echo "sudo: a password is required" >&2; exit 1`)
	writeStub(t, dir, "service",
		`echo "chmod: changing permissions of '/var/run/postgresql': Operation not permitted" >&2
exit 1`)
	writeStub(t, dir, "pg_isready", "exit 1") // never becomes ready
	writeStub(t, dir, "initdb", initdbStub(filepath.Join(dir, "calls")))
	writeStub(t, dir, "pg_ctl", pgctlStub(filepath.Join(dir, "calls"), 3, 0))

	db := DBConfig{
		Type: "PostgreSQL", Host: net.JoinHostPort("127.0.0.1", port),
		Name: "app", User: "mendix", Password: "secret",
	}
	err := EnsureDatabase(&db, io.Discard)
	if err == nil {
		t.Fatal("expected a readiness failure")
	}
	if !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("expected the readiness timeout, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Operation not permitted") {
		t.Fatalf("the readiness timeout must carry what the service manager "+
			"actually said, got:\n%v", err)
	}
}
