// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ensuredb.go provisions the local PostgreSQL a standalone runtime needs, so a
// fresh session comes up testable without a manual createdb (slice 2 of the
// warm-loop proposal). It is best-effort and devcontainer-shaped: it starts the
// local Postgres service if the port is down, then ensures the app role and
// database exist via a superuser (`sudo -u postgres psql`). For a non-local DB
// host it does nothing but verify reachability — provisioning a remote database
// is not mxcli's business.

// pgIdent is a conservative PostgreSQL identifier (unquoted): a safe database or
// role name. We refuse anything else rather than quote/escape it into DDL.
var pgIdent = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// splitHostPort splits "host:port" into host and port, defaulting the port to
// 5432 when absent.
func splitHostPort(hostPort string) (host, port string) {
	if i := strings.LastIndexByte(hostPort, ':'); i >= 0 {
		return hostPort[:i], hostPort[i+1:]
	}
	return hostPort, "5432"
}

// isLocalHost reports whether host refers to the local machine (so it is safe to
// start a service / use a local superuser).
func isLocalHost(host string) bool {
	switch host {
	case "127.0.0.1", "localhost", "::1", "":
		return true
	}
	return false
}

// quoteSQLString wraps s in single quotes for use as a SQL string literal,
// doubling any embedded single quotes (used only for the role password, which
// cannot be a bind parameter in CREATE ROLE).
func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// EnsureDatabase makes db reachable and the app role + database present. It is a
// no-op when the app can already connect. For a local host whose port is down it
// starts Postgres, then creates the role and database if missing.
func EnsureDatabase(db DBConfig, w io.Writer) error {
	if strings.ToLower(db.Type) != "postgresql" {
		return fmt.Errorf("--ensure-db only supports PostgreSQL (DatabaseType=%q)", db.Type)
	}
	if !pgIdent.MatchString(db.Name) {
		return fmt.Errorf("unsafe database name %q (expected %s)", db.Name, pgIdent)
	}
	if !pgIdent.MatchString(db.User) {
		return fmt.Errorf("unsafe database user %q (expected %s)", db.User, pgIdent)
	}
	host, port := splitHostPort(db.Host)

	// Already usable? Then we're done.
	if canConnectDB(db) {
		return nil
	}

	// Port down: start the local Postgres service (best-effort) or bail for remote.
	if err := pingTCP(db.Host, 2*time.Second); err != nil {
		if !isLocalHost(host) {
			return fmt.Errorf("database not reachable at %s and host is not local; "+
				"start it and create the %q database (user %q)", db.Host, db.Name, db.User)
		}
		fmt.Fprintln(w, "  Starting local PostgreSQL...")
		if err := startLocalPostgres(); err != nil {
			return fmt.Errorf("starting local PostgreSQL: %w", err)
		}
		if err := waitPGReady(host, port, 20*time.Second); err != nil {
			return err
		}
	}

	// Ensure the role and database exist (needs a local superuser).
	if err := ensureRole(db, w); err != nil {
		return err
	}
	if err := ensureDatabase(db, w); err != nil {
		return err
	}

	if !canConnectDB(db) {
		return fmt.Errorf("provisioned PostgreSQL but still cannot connect to %q as %q at %s",
			db.Name, db.User, db.Host)
	}
	fmt.Fprintf(w, "  Database ready: %s (user %q) at %s\n", db.Name, db.User, db.Host)
	return nil
}

// canConnectDB reports whether the app can connect to its database as its user.
func canConnectDB(db DBConfig) bool {
	host, port := splitHostPort(db.Host)
	cmd := exec.Command("psql", "-h", host, "-p", port, "-U", db.User, "-d", db.Name,
		"-tAc", "select 1")
	cmd.Env = append(os.Environ(), "PGPASSWORD="+db.Password, "PGCONNECT_TIMEOUT=3")
	return cmd.Run() == nil
}

// startLocalPostgres starts the local PostgreSQL service, trying the common
// service managers in turn. Success is confirmed later by waitPGReady.
func startLocalPostgres() error {
	attempts := [][]string{
		{"service", "postgresql", "start"},
		{"pg_ctlcluster", "--", "start"}, // placeholder; real cluster args vary
	}
	var lastErr error
	for _, a := range attempts {
		if _, err := exec.LookPath(a[0]); err != nil {
			lastErr = err
			continue
		}
		cmd := exec.Command(a[0], a[1:]...)
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		// `service postgresql start` is the reliable path in the devcontainer; if
		// it ran (even non-zero) Postgres may still be coming up — let waitPGReady
		// decide rather than failing here.
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no known service manager found to start PostgreSQL")
}

// waitPGReady polls pg_isready until the server accepts connections or timeout.
func waitPGReady(host, port string, timeout time.Duration) error {
	if _, err := exec.LookPath("pg_isready"); err != nil {
		// Fall back to a raw TCP check.
		return waitTCP(host+":"+port, timeout)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command("pg_isready", "-h", host, "-p", port)
		if cmd.Run() == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("PostgreSQL did not become ready at %s:%s within %s", host, port, timeout)
}

// waitTCP polls a host:port until it accepts a connection or timeout.
func waitTCP(hostPort string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pingTCP(hostPort, time.Second) == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%s did not accept connections within %s", hostPort, timeout)
}

// superuserPSQL runs a psql command as the postgres superuser (sudo -u postgres).
func superuserPSQL(args ...string) *exec.Cmd {
	full := append([]string{"-u", "postgres", "psql", "-v", "ON_ERROR_STOP=1"}, args...)
	return exec.Command("sudo", full...)
}

// ensureRole creates the app login role if it does not already exist.
func ensureRole(db DBConfig, w io.Writer) error {
	check := superuserPSQL("-tAc", fmt.Sprintf("select 1 from pg_roles where rolname='%s'", db.User))
	out, _ := check.Output()
	if strings.TrimSpace(string(out)) == "1" {
		return nil
	}
	fmt.Fprintf(w, "  Creating role %q...\n", db.User)
	ddl := fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD %s CREATEDB", db.User, quoteSQLString(db.Password))
	cmd := superuserPSQL("-c", ddl)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating role %q: %w\n%s\n"+
			"  (need a local postgres superuser via 'sudo -u postgres'; create the role manually if unavailable)",
			db.User, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureDatabase creates the app database owned by the app role if it is absent.
func ensureDatabase(db DBConfig, w io.Writer) error {
	check := superuserPSQL("-tAc", fmt.Sprintf("select 1 from pg_database where datname='%s'", db.Name))
	out, _ := check.Output()
	if strings.TrimSpace(string(out)) == "1" {
		return nil
	}
	fmt.Fprintf(w, "  Creating database %q owned by %q...\n", db.Name, db.User)
	ddl := fmt.Sprintf("CREATE DATABASE %s OWNER %s", db.Name, db.User)
	cmd := superuserPSQL("-c", ddl)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating database %q: %w\n%s", db.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
