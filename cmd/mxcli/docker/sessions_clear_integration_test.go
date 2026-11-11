//go:build integration

// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"database/sql"
	"net/url"
	"os"
	"testing"
)

// Against a real PostgreSQL: MXCLI_TEST_PG_HOST=127.0.0.1:5432 with a database
// "app" (user/password mendix) holding "system$session" and
// "system$session_user", and an empty database "fresh".
func TestClearLocalSessions_RealPostgres(t *testing.T) {
	host := os.Getenv("MXCLI_TEST_PG_HOST")
	if host == "" {
		t.Skip("MXCLI_TEST_PG_HOST not set")
	}
	db := DBConfig{Type: RuntimeDatabaseType(DBTypePostgreSQL), Host: host, Name: "app", User: "mendix", Password: "mendix"}

	n, err := ClearSessions(db)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("cleared 0 sessions from a database seeded with some")
	}
	for _, table := range []string{`"system$session"`, `"system$session_user"`} {
		if got := countRows(t, db, table); got != 0 {
			t.Errorf("%s has %d rows after the clear, want 0", table, got)
		}
	}

	db.Name = "fresh"
	if n, err := ClearSessions(db); err != nil || n != 0 {
		t.Errorf("a database without System tables: n=%d err=%v, want 0, nil", n, err)
	}
}

func countRows(t *testing.T, db DBConfig, table string) int {
	t.Helper()
	h, p := splitHostPort(db.Host)
	u := url.URL{Scheme: "postgres", User: url.UserPassword(db.User, db.Password), Host: h + ":" + p, Path: "/" + db.Name, RawQuery: "sslmode=disable"}
	c, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var n int
	if err := c.QueryRow("select count(*) from " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
